#!/usr/bin/env bash
set -euo pipefail

# Keep this check deliberately small and diagnostic. The artifact is inspected
# by arcc's production bounded decoder; this script only extracts the stable
# toolchain field from its deterministic summary and compares the independent
# repository pins and routine target fields.

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ci_file="$root/.github/workflows/ci.yml"
module_file="$root/MODULE.bazel"
artifact="$root/go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json"

mapfile -t github_pins < <(sed -nE 's/^[[:space:]]*go-version:[[:space:]]*"([^"]+)"[[:space:]]*$/\1/p' "$ci_file")
module_pin=$(sed -nE 's/^[[:space:]]*go_sdk\.download\(version = "([^"]+)"\).*$/\1/p' "$module_file")

if [[ ${#github_pins[@]} -ne 2 ]]; then
  echo "routine stdlib-map pin mismatch: expected exactly two GitHub setup-go pins" >&2
  echo "expected: 2 setup-go pins" >&2
  echo "actual: ${#github_pins[@]}" >&2
  exit 1
fi
if [[ -z $module_pin ]]; then
  echo "routine stdlib-map pin mismatch: MODULE.bazel go_sdk.download pin is missing" >&2
  echo "expected: one go_sdk.download(version = \"...\")" >&2
  echo "actual: missing" >&2
  exit 1
fi

summary=$(cd "$root/go" && go run ./cmd/arcc stdlibmap inspect "$artifact")
artifact_key=$(sed -nE 's/^sdk key: (.*)$/\1/p' <<<"$summary")
artifact_toolchain=$(sed -nE 's/.*toolchain_version:"([^"]+)".*/\1/p' <<<"$artifact_key")
artifact_goos=$(sed -nE 's/.*goos:"([^"]+)".*/\1/p' <<<"$artifact_key")
artifact_goarch=$(sed -nE 's/.*goarch:"([^"]+)".*/\1/p' <<<"$artifact_key")
artifact_cgo=$(sed -nE 's/.*cgo_enabled:([^ ]+).*/\1/p' <<<"$artifact_key")
artifact_experiment=$(sed -nE 's/.*goexperiment:"([^"]*)".*/\1/p' <<<"$artifact_key")
artifact_tags=$(sed -nE 's/.*build_tags:\[([^]]*)\].*/\1/p' <<<"$artifact_key")

fail=0
compare() {
  local label=$1 expected=$2 actual=$3
  if [[ $expected != "$actual" ]]; then
    echo "routine stdlib-map pin mismatch: $label" >&2
    echo "expected: $expected" >&2
    echo "actual: $actual" >&2
    fail=1
  fi
}

compare "GitHub setup-go pins agree" "${github_pins[0]}" "${github_pins[1]}"
compare "GitHub setup-go vs Bazel go_sdk.download" "${github_pins[0]#go}" "${module_pin#go}"
compare "GitHub setup-go vs artifact toolchain_version" "go${github_pins[0]#go}" "$artifact_toolchain"
compare "artifact GOOS" "linux" "$artifact_goos"
compare "artifact GOARCH" "amd64" "$artifact_goarch"
compare "artifact cgo_enabled" "false" "$artifact_cgo"
compare "artifact GOEXPERIMENT" "" "$artifact_experiment"
compare "artifact build_tags" "" "$artifact_tags"

if (( fail != 0 )); then
  exit 1
fi

echo "routine stdlib-map pins agree: GitHub=${github_pins[0]} Bazel=${module_pin} artifact=${artifact_toolchain} linux/amd64 cgo=false"
