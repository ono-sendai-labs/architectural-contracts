#!/bin/sh
# Tool-error semantics of the checked-analysis command (task req 8, AC 2):
# `arcc check --report-verdict-only` masks violations (exit 1 becomes 0 with a
# fail verdict in the report) but must still fail with exit 2 on a tool error,
# and must not leave a report behind — this is exactly what makes the
# Bazel action fail on a tool error while succeeding on violations.
set -eu

arcc="$1"
broken_manifest="$2"

out_dir="${TEST_TMPDIR:-${TMPDIR:-/tmp}}/malformed_manifest_tool_error"
rm -rf "$out_dir"
mkdir -p "$out_dir"

status=0
"$arcc" check "$broken_manifest" \
    --report-out="$out_dir/broken.report.json" \
    --report-verdict-only \
    --format=json >"$out_dir/stdout" 2>"$out_dir/stderr" || status=$?

if [ "$status" -ne 2 ]; then
  echo "FAIL: expected tool-error exit 2, got $status" >&2
  cat "$out_dir/stdout" "$out_dir/stderr" >&2
  exit 1
fi

if [ -e "$out_dir/broken.report.json" ]; then
  echo "FAIL: tool error must not publish a report" >&2
  exit 1
fi

if ! grep -q "failed to parse manifest" "$out_dir/stderr"; then
  echo "FAIL: tool error does not name the offending manifest" >&2
  cat "$out_dir/stderr" >&2
  exit 1
fi

echo "PASS: malformed manifest is a tool error (exit 2), no report published"
