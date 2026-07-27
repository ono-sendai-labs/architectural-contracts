#!/usr/bin/env bash
# A label-list attribute must reject command-line target patterns at load time.
set -euo pipefail

invalid_root="${TEST_TMPDIR:-${TMPDIR:-/tmp}}/invalid_members_package_$$"
trap 'rm -rf "${invalid_root}"' EXIT
mkdir -p "${invalid_root}/invalid_members"
cat > "${invalid_root}/invalid_members/BUILD.bazel" <<'EOF'
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component")

go_component(
    name = "invalid",
    interface = "//bazel_rules/go/tests/testdata/api:api",
    members = ["//bazel_rules/go/tests/testdata/member/..."],
)
EOF

diagnostics="${invalid_root}/diagnostics.txt"
main_workspace="${TEST_SRCDIR}/_main"
if [[ ! -f "${main_workspace}/MODULE.bazel" ]]; then
  main_workspace="${TEST_SRCDIR}"
fi
if bazel build \
    --noshow_progress \
    --package_path="${invalid_root}:${main_workspace}" \
    //invalid_members:invalid >"${diagnostics}" 2>&1; then
  echo "FAIL: target pattern unexpectedly accepted in members" >&2
  exit 1
fi

grep -F "invalid label" "${diagnostics}" >/dev/null
grep -F "attribute 'members'" "${diagnostics}" >/dev/null
echo "OK: target pattern rejected by typed members label_list"
