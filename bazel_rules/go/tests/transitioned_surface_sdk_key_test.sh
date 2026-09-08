#!/bin/sh
# Target-identity coherence for the checked analysis action (task req 8, AC 6):
# a component analyzed under a non-default target configuration (darwin/arm64,
# pure/cgo-off, an extra build tag) emits a surface whose SDK key matches the
# stdlib map attached to ITS configuration exactly, and that key is distinct
# from the default configuration's map key — so a cgo-off/tagged surface can
# never silently validate against the wrong SDK's map.
#
# Identity fields cover the surface's complete SDK-key surface: toolchain
# version, GOOS, GOARCH, cgo state, build tags (GOEXPERIMENT is stamped by the
# SDK itself and is identical across the configurations this exercise
# distinguishes; a key with a non-default GOEXPERIMENT would carry the field
# and be compared the same way).
set -eu

arcc="$1"
default_map="$2"
darwin_map="$3"
default_surface="$4"
darwin_surface="$5"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

# The complete SDK-key identity the surface carries, as stdlibmap inspect
# --expect-key specs. Fields the surface omits (zero-valued) are left out of
# the expectation, exactly as the map omits them.
surface_key_specs() {
  surface="$1"
  tv=$(sed -n 's/.*"toolchainVersion": "\([^"]*\)".*/\1/p' "$surface")
  goos=$(sed -n 's/.*"goos": "\([^"]*\)".*/\1/p' "$surface")
  goarch=$(sed -n 's/.*"goarch": "\([^"]*\)".*/\1/p' "$surface")
  cgo=$(sed -n 's/.*"cgoEnabled": \(true\|false\).*/\1/p' "$surface")
  tags=$(tr -d '\n ' < "$surface" | sed -n 's/.*"buildTags":\[\([^]]*\)\].*/\1/p' | tr -d '"' | tr ',' ' ')

  if [ -z "$tv" ] || [ -z "$goos" ] || [ -z "$goarch" ]; then
    fail "surface $surface carries no SDK key identity fields"
  fi

  specs="toolchain_version=$tv,goos=$goos,goarch=$goarch"
  if [ -n "$cgo" ]; then
    specs="$specs,cgo_enabled=$cgo"
  fi
  for tag in $tags; do
    specs="$specs,tag=$tag"
  done
  echo "$specs"
}

default_specs=$(surface_key_specs "$default_surface")
darwin_specs=$(surface_key_specs "$darwin_surface")

case "$darwin_specs" in
  *goos=darwin*) ;;
  *) fail "transitioned surface key is not darwin: $darwin_specs" ;;
esac
case "$darwin_specs" in
  *tag=adapter_probe*) ;;
  *) fail "transitioned surface key does not carry the build tag: $darwin_specs" ;;
esac
if [ "$default_specs" = "$darwin_specs" ]; then
  fail "default and transitioned surface keys are identical; the configurations are not distinguished"
fi
pass "transitioned surface SDK key is complete and configuration-distinct: $darwin_specs"

# The surface's key must identify the map attached to its own configuration.
tmp_err="${TEST_TMPDIR:-${TMPDIR:-/tmp}}/inspect.err"
if ! "$arcc" stdlibmap inspect "$darwin_map" --expect-key="$darwin_specs" >/dev/null 2>"$tmp_err"; then
  echo "stderr was:" >&2; cat "$tmp_err" >&2
  fail "transitioned surface SDK key does not match its configuration's stdlib map"
fi
pass "transitioned surface SDK key matches its attached map exactly"

# ...and must NOT identify the default configuration's map.
if "$arcc" stdlibmap inspect "$default_map" --expect-key="$darwin_specs" >/dev/null 2>&1; then
  fail "default stdlib map accepted the transitioned surface's SDK key"
fi
pass "transitioned surface SDK key is rejected by the default configuration's map"
