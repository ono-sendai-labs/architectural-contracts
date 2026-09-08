#!/usr/bin/env bash
# Compares the manifest and layout `go_component` generates for the fixture
# graph against checked-in goldens. The rule's default outputs are two files
# (the analysis artifacts ride in the `arcc` output group), so this is the
# test that says what they contain — the analysis tests can only see the
# providers.
#
# To update the goldens after an intentional change:
#
#   bazel build //bazel_rules/go/tests/testdata/api:api_component
#   cp bazel-bin/bazel_rules/go/tests/testdata/api/api_component.component.textproto \
#      bazel_rules/go/tests/goldens/
#   sed -e 's|"go_sdk_root": ".*"|"go_sdk_root": "<GO_SDK_ROOT>"|' \
#      bazel-bin/bazel_rules/go/tests/testdata/api/api_component.package-layout.json \
#      > bazel_rules/go/tests/goldens/api_component.package-layout.json
#
# Args: <generated manifest> <generated layout> <golden manifest> <golden layout>
set -euo pipefail

generated_manifest="$1"
generated_layout="$2"
golden_manifest="$3"
golden_layout="$4"

# The first two arguments come from a single $(rootpaths) expansion, so their
# order is the rule's DefaultInfo order rather than something this script can
# see. Check it rather than trusting it.
if [[ "${generated_manifest}" != *.component.textproto || "${generated_layout}" != *.package-layout.json ]]; then
  echo "FAIL: expected <manifest> <layout>, got '${generated_manifest}' '${generated_layout}'" >&2
  exit 1
fi

status=0

if ! diff -u "${golden_manifest}" "${generated_manifest}"; then
  echo "FAIL: generated manifest differs from ${golden_manifest} (< golden, > generated)" >&2
  status=1
fi

# The Go SDK root names the platform and the resolved SDK repository, neither
# of which the rule chooses; pinning it would make the golden fail on a
# different host or after a rules_go upgrade. Its shape is asserted instead.
sdk_root="$(sed -n 's/^  "go_sdk_root": "\(.*\)",\?$/\1/p' "${generated_layout}")"
if [[ "${sdk_root}" != *"go_sdk"*/src ]]; then
  echo "FAIL: go_sdk_root is '${sdk_root}', want a Go SDK repository path ending in /src" >&2
  status=1
fi

normalized_layout="${TEST_TMPDIR:-/tmp}/normalized.package-layout.json"
sed -e 's|"go_sdk_root": ".*"|"go_sdk_root": "<GO_SDK_ROOT>"|' "${generated_layout}" > "${normalized_layout}"

if ! diff -u "${golden_layout}" "${normalized_layout}"; then
  echo "FAIL: generated layout differs from ${golden_layout} (< golden, > generated)" >&2
  status=1
fi

if [[ "${status}" -eq 0 ]]; then
  echo "OK: manifest and layout match their goldens"
fi

exit "${status}"
