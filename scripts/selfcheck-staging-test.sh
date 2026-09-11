#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# The production helper is sourced by justfile and is deliberately exercised
# here through the same function boundary as the recipe.
source "$script_dir/selfcheck-staging.sh"

test_root=$(mktemp -d "${TMPDIR:-/tmp}/arcc-selfcheck-staging-test.XXXXXX")
trap 'rm -rf -- "$test_root"' EXIT

mkdir -p "$test_root/stage-only" "$test_root/schema" \
  "$test_root/protobuf-runtime" "$test_root/x-tools"
touch "$test_root/stage-only/component.textproto"
touch "$test_root/schema/component.textproto"
touch "$test_root/protobuf-runtime/component.textproto"
touch "$test_root/protobuf-runtime/component.surface.json"
touch "$test_root/x-tools/component.textproto"
touch "$test_root/x-tools/component.surface.json"
touch "$test_root/map.json"

fake_arcc="$test_root/fake-arcc"
cp "$script_dir/testdata/selfcheck-fake-arcc" "$fake_arcc"
chmod +x "$fake_arcc"
export SELFCHECK_FAKE_LOG="$test_root/fake-arcc.log"

cd "$test_root"

# A report-producing analysis can return zero for a fail verdict when it is
# asked for report-only mode. The separate verdict assertion must reject that
# stage and name the component instead of accepting the report artifact.
if selfcheck_stage_component "$fake_arcc" "$test_root/map.json" stage-only/component.textproto \
    >"$test_root/stage-only.stdout" 2>"$test_root/stage-only.stderr"; then
  echo "stage-only failure unexpectedly passed the staging gate" >&2
  exit 1
fi
grep -Fq "stage-only" "$test_root/stage-only.stderr"
grep -Fq "verdict stage-only/component.report.json --expect=pass" "$SELFCHECK_FAKE_LOG"

# The generated-protobuf schema failure is the one explicit non-pass outcome.
selfcheck_stage_component "$fake_arcc" "$test_root/map.json" schema/component.textproto fail
grep -Fq "verdict schema/component.report.json --expect=fail" "$SELFCHECK_FAKE_LOG"

# Asserted wrappers have a surface but intentionally have no analysis report
# and therefore do not invoke either `check` or `verdict`.
before_asserted_calls=$(wc -l <"$SELFCHECK_FAKE_LOG")
selfcheck_asserted_component protobuf-runtime/component.textproto
selfcheck_asserted_component x-tools/component.textproto
after_asserted_calls=$(wc -l <"$SELFCHECK_FAKE_LOG")
if [[ "$before_asserted_calls" != "$after_asserted_calls" ]]; then
  echo "asserted wrapper unexpectedly invoked the analysis command" >&2
  exit 1
fi
test ! -e protobuf-runtime/component.report.json
test ! -e x-tools/component.report.json

echo "selfcheck staging verdict regression passed"
