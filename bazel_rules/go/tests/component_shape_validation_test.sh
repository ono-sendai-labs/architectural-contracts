#!/usr/bin/env bash
# Shape validation belongs to the public symbolic macro, so exercise it from
# temporary BUILD files and assert the direct load diagnostics.
set -euo pipefail

invalid_root="${TEST_TMPDIR:-${TMPDIR:-/tmp}}/component_shape_packages_$$"
trap 'rm -rf "${invalid_root}"' EXIT
mkdir -p "${invalid_root}/declared_missing"
mkdir -p "${invalid_root}/declared_pattern"
mkdir -p "${invalid_root}/surface_interface"
mkdir -p "${invalid_root}/surface_members"
mkdir -p "${invalid_root}/surface_empty"
mkdir -p "${invalid_root}/unknown_style"

cat > "${invalid_root}/declared_missing/BUILD.bazel" <<'EOF'
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component")

go_component(
    name = "declared_missing",
)
EOF

cat > "${invalid_root}/declared_pattern/BUILD.bazel" <<'EOF'
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component")

go_component(
    name = "declared_pattern",
    interface = "//bazel_rules/go/tests/testdata/api:api",
    members = ["example.com/app/*"],
)
EOF

cat > "${invalid_root}/surface_interface/BUILD.bazel" <<'EOF'
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component")

go_component(
    name = "surface_interface",
    interface = "//bazel_rules/go/tests/testdata/api:api",
    interface_style = "PACKAGE_SURFACE",
    members = ["//bazel_rules/go/tests/testdata/member"],
)
EOF

cat > "${invalid_root}/surface_members/BUILD.bazel" <<'EOF'
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component")

go_component(
    name = "surface_members",
    interface_style = "PACKAGE_SURFACE",
)
EOF

cat > "${invalid_root}/surface_empty/BUILD.bazel" <<'EOF'
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component")

go_component(
    name = "surface_empty",
    interface_style = "PACKAGE_SURFACE",
    members = [],
)
EOF

cat > "${invalid_root}/unknown_style/BUILD.bazel" <<'EOF'
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component")

go_component(
    name = "unknown_style",
    interface = "//bazel_rules/go/tests/testdata/api:api",
    interface_style = "NOT_A_STYLE",
)
EOF

main_workspace="${TEST_SRCDIR}/_main"
if [[ ! -f "${main_workspace}/MODULE.bazel" ]]; then
  main_workspace="${TEST_SRCDIR}"
fi

run_failure() {
  local package_name="$1"
  local diagnostic="$2"
  shift 2

  if bazel build \
      --noshow_progress \
      --package_path="${invalid_root}:${main_workspace}" \
      "//${package_name}:${package_name}" >"${diagnostic}" 2>&1; then
    echo "FAIL: ${package_name} unexpectedly analyzed" >&2
    exit 1
  fi
  for expected in "$@"; do
    grep -F "${expected}" "${diagnostic}" >/dev/null
  done
}

run_failure declared_missing "${invalid_root}/declared_missing.txt" \
  "component declared_missing" "requires interface"
run_failure declared_pattern "${invalid_root}/declared_pattern.txt" \
  "component declared_pattern" "declared-style members must be literal target labels"
run_failure surface_interface "${invalid_root}/surface_interface.txt" \
  "component surface_interface" "interface" "does not allow"
run_failure surface_members "${invalid_root}/surface_members.txt" \
  "component surface_members" "non-empty members"
run_failure surface_empty "${invalid_root}/surface_empty.txt" \
  "component surface_empty" "non-empty members"
run_failure unknown_style "${invalid_root}/unknown_style.txt" \
  "component unknown_style" "PACKAGE_SURFACE" "declared"

echo "OK: component shape failures name components and conflicting attributes"
