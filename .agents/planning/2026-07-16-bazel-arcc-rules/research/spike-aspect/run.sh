#!/usr/bin/env bash
# Reproduce the _arcc_deps aspect spike. Requires bazel (tested: 9.2.0) and a
# network-reachable rules_go 0.61.1 / Go SDK 1.24.5 (bzlmod-downloaded).
#
# The --pure / BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN dance only avoids needing a host
# C toolchain in this sandbox; it is NOT a consumer requirement (see findings).
set -euo pipefail
cd "$(dirname "$0")"
export BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1

echo "== build //api:api_closure (aspect + layout emission) =="
bazel build //api:api_closure

bin="$(bazel info bazel-bin)/api"
echo; echo "== closure summary (spike assertions) =="
cat "$bin/api_closure.closure.txt"
echo; echo "== emitted package-layout.json (design §5.4) =="
cat "$bin/api_closure.package-layout.json"

echo; echo "== assertions =="
grep -q "package_count=5" "$bin/api_closure.closure.txt" && echo "OK  closure has exactly 5 packages (embed-dup merged, diamond deduped)"
grep -q "example.com/aspect/core | .* srcs=core/core.go,core/core_extra.go,core/extra_impl.go" "$bin/api_closure.closure.txt" && echo "OK  embed srcs merged into core"
grep -q "example.com/aspect/core | .* deps=example.com/aspect/extradep,example.com/aspect/lowlevel,example.com/aspect/shared" "$bin/api_closure.closure.txt" && echo "OK  embed-only dep (extradep) captured on core"
grep -q "sdk_stdlib_src_count=5329" "$bin/api_closure.closure.txt" && echo "OK  full SDK stdlib source tree available as inputs"
