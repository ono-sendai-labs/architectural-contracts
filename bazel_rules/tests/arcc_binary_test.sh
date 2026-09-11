#!/usr/bin/env bash
# Smoke test for arcc built from source under Bazel: the stable //:arcc alias
# must resolve to a fully linked, runnable binary.
#
# The exact version string is pinned by the Go test //go/cmd/arcc:arcc_test,
# which also runs under Bazel; duplicating it here would only add a second
# place to update.
set -euo pipefail

arcc="${PWD}/$1"

version_output="$("${arcc}" --version)"
if [[ ! "${version_output}" =~ ^arcc\ [^[:space:]]+$ ]]; then
  echo "FAIL: 'arcc --version' printed '${version_output}', want 'arcc <version>'" >&2
  exit 1
fi

# A no-argument invocation prints usage and exits 0. Together with --version
# this exercises the binary past process start-up.
usage_output="$("${arcc}")"
if ! grep -q "Usage:" <<<"${usage_output}"; then
  echo "FAIL: 'arcc' with no arguments did not print usage" >&2
  exit 1
fi

echo "OK: ${version_output}"
