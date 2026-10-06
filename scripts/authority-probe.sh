#!/usr/bin/env bash
set -euo pipefail

# authority-probe.sh measures the ambient authority exercised by a component
# by staging its dependencies in topological order, removing all
# declared_authority lines from its manifest, and analyzing it with arcc check
# against the pinned stdlib authority map (design Appendix B).
#
# Usage:
#   scripts/authority-probe.sh <manifest-rel-path-or-component-name>
#
# Exit codes:
#   0: No undeclared authority findings (pure / authority-free).
#   2: Undeclared authority findings detected (prints the capability table).
#   1: Configuration or runtime error.

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <manifest-rel-path-or-component-name>" >&2
  exit 1
fi

raw_target="$1"
raw_target="${raw_target#component=}"
root_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# Ensure the pinned Go 1.26.4 toolchain and CGO_ENABLED=0 are used if available.
if [[ "$(go env GOVERSION 2>/dev/null)" != "go1.26.4" || "$(go env CGO_ENABLED 2>/dev/null)" != "0" ]]; then
  bazel_out=$(bazel info output_base 2>/dev/null || true)
  if [[ -n "$bazel_out" ]]; then
    sdk_go="$bazel_out/external/rules_go++go_sdk+main___download_0_linux_amd64/bin"
    if [[ -d "$sdk_go" ]]; then
      export PATH="$sdk_go:$PATH"
      export CGO_ENABLED=0
    fi
  fi
fi

# Normalize target to relative manifest path inside go/ directory (e.g. internal/facts/component.textproto)
target="${raw_target#go/}"
if [[ "$target" != *"component.textproto"* ]]; then
  matched=$(sed -nE 's/^[[:space:]]*(selfcheck_[a-z_]*component[^;]+).*/\1/p' "$root_dir/justfile" | grep -E "/${target}/component\.textproto| ${target}/component\.textproto" | head -n1 || true)
  if [[ -n "$matched" ]]; then
    target=$(echo "$matched" | grep -o '[^ ]*component\.textproto')
  else
    # Fallback heuristic: check if internal/<target>/component.textproto exists
    if [[ -f "$root_dir/go/internal/$target/component.textproto" ]]; then
      target="internal/$target/component.textproto"
    elif [[ -f "$root_dir/go/$target/component.textproto" ]]; then
      target="$target/component.textproto"
    fi
  fi
fi

# 1. Build bin/arcc
mkdir -p "$root_dir/bin"
(cd "$root_dir/go" && go build -o ../bin/arcc ./cmd/arcc)

# 2. Create a temporary staging area
selfcheck_stage=$(mktemp -d "${TMPDIR:-/tmp}/arcc-authority-probe.XXXXXX")
trap 'rm -rf "$selfcheck_stage"' EXIT
cp -a "$root_dir/go" "$selfcheck_stage/go"
arcc_bin="$root_dir/bin/arcc"
stage_helper="$root_dir/scripts/selfcheck-staging.sh"
map_path="$selfcheck_stage/go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json"
cd "$selfcheck_stage/go"
source "$stage_helper"

# 3. Stage preceding components in topological order parsed from justfile
target_found=false
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  line_manifest=$(echo "$line" | grep -o '[^ ]*component\.textproto')
  if [[ "$line_manifest" == "$target" ]]; then
    target_found=true
    break
  fi
  eval "$line"
done < <(sed -nE 's/^[[:space:]]*(selfcheck_[a-z_]*component[^;]+).*/\1/p' "$root_dir/justfile")

if [[ ! -f "$target" ]]; then
  echo "authority-probe: manifest not found: $target" >&2
  exit 1
fi

# 4. Rewrite the target manifest with all declared_authority: lines removed
sed -i '/^[[:space:]]*declared_authority:/d' "$target"

# 5. Run arcc check with the pinned stdlib map
target_base="${target%.textproto}"
report_out="$target_base.report.json"
"$arcc_bin" check "$target" --stdlib-map="$map_path" --report-out="$report_out" --report-verdict-only >/dev/null 2>&1 || true

if [[ ! -f "$report_out" ]]; then
  echo "authority-probe: failed to produce report for $target" >&2
  exit 1
fi

# Extract UNDECLARED_AUTHORITY findings and print table capability | member symbol | stdlib symbol | file:line
table=$(jq -r '
  .report.violations[]? | select(.kind == "UNDECLARED_AUTHORITY") as $v |
  (($v.message | capture("use of undeclared authority \"(?<cap>[^\"]+)\"")) // {cap: ($v.class // "UNDECLARED_AUTHORITY")}) as $c |
  if ($v.sites // []) | length > 0 then
    $v.sites[] | "\($c.cap) | - | \(.symbol // "-") | \(.file):\(.line)"
  elif $v.location.file != "" then
    "\($c.cap) | - | - | \($v.location.file):\($v.location.line)"
  else
    "\($c.cap) | - | - | -"
  end
' "$report_out")

# 6. Exit 0 when table is empty, 2 otherwise
if [[ -n "$table" ]]; then
  echo "$table"
  exit 2
else
  exit 0
fi
