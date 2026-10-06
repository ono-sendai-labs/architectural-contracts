#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
probe_sh="$script_dir/authority-probe.sh"

echo "=== Testing authority-probe on facts (expected: empty, exit 0) ==="
rc=0
out_facts=$("$probe_sh" internal/facts/component.textproto) || rc=$?
if [[ $rc -ne 0 ]]; then
  echo "authority-probe test failed: expected exit 0 for facts, got $rc" >&2
  exit 1
fi
if [[ -n "$out_facts" ]]; then
  echo "authority-probe test failed: expected empty output for facts, got:" >&2
  echo "$out_facts" >&2
  exit 1
fi

# Also verify invocation by component name
rc=0
out_facts_short=$("$probe_sh" facts) || rc=$?
if [[ $rc -ne 0 || -n "$out_facts_short" ]]; then
  echo "authority-probe test failed: short component name 'facts' failed" >&2
  exit 1
fi

echo "=== Testing authority-probe on artifactio (expected: exit 2 with FILES, REFLECT, UNSAFE_POINTER) ==="
rc=0
out_artifactio=$("$probe_sh" internal/artifactio/component.textproto) || rc=$?
if [[ $rc -ne 2 ]]; then
  echo "authority-probe test failed: expected exit 2 for artifactio, got $rc" >&2
  exit 1
fi

for cap in FILES REFLECT UNSAFE_POINTER; do
  if ! echo "$out_artifactio" | grep -q "$cap"; then
    echo "authority-probe test failed: expected capability $cap in artifactio probe table, but not found:" >&2
    echo "$out_artifactio" >&2
    exit 1
  fi
done

echo "authority-probe regression test passed"
