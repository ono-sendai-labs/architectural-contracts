#!/bin/sh
# Target-identity coherence for the checked analysis action (task req 8, AC 6):
# a component analyzed under a non-default target configuration (darwin/arm64,
# pure/cgo-off, an extra build tag) emits a surface whose SDK key matches the
# stdlib map attached to ITS configuration exactly, and that key is distinct
# from the default configuration's map key — so a cgo-off/tagged surface can
# never silently validate against the wrong SDK's map.
#
# Identity fields cover the surface's complete SDK-key surface: toolchain
# version, GOOS, GOARCH, cgo state (asserted OFF: the field must be absent
# from the surface, `cgo_enabled=false` must match the map, and
# `cgo_enabled=true` must be rejected), and build tags. GOEXPERIMENT is
# stamped by the pinned SDK itself — both configurations share it, so the
# empty value is asserted as an explicit checked field against both maps;
# a non-default GOEXPERIMENT would carry the field and be compared the same
# way.
set -eu

arcc="$1"
default_map="$2"
darwin_map="$3"
default_surface="$4"
darwin_surface="$5"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

# The complete SDK-key identity the surface carries, as stdlibmap inspect
# --expect-key specs. Zero-valued fields (cgo off, no GOEXPERIMENT) are
# asserted as explicit checked fields with their zero values, not omitted.
surface_key_specs() {
  surface="$1"
  tv=$(sed -n 's/.*"toolchainVersion": "\([^"]*\)".*/\1/p' "$surface")
  goos=$(sed -n 's/.*"goos": "\([^"]*\)".*/\1/p' "$surface")
  goarch=$(sed -n 's/.*"goarch": "\([^"]*\)".*/\1/p' "$surface")
  cgo=$(sed -n 's/.*"cgoEnabled": \(true\|false\).*/\1/p' "$surface")
  goex=$(sed -n 's/.*"goexperiment": "\([^"]*\)".*/\1/p' "$surface")
  tags=$(tr -d '\n ' < "$surface" | sed -n 's/.*"buildTags":\[\([^]]*\)\].*/\1/p' | tr -d '"' | tr ',' ' ')

  if [ -z "$tv" ] || [ -z "$goos" ] || [ -z "$goarch" ]; then
    fail "surface $surface carries no SDK key identity fields"
  fi

  specs="toolchain_version=$tv,goos=$goos,goarch=$goarch"
  # Zero-valued identity fields are checked explicitly, not defaulted:
  # cgo_enabled and goexperiment must MATCH the map's value (false/empty),
  # never be silently ignored.
  cgo_val=${cgo:-false}
  specs="$specs,cgo_enabled=$cgo_val,goexperiment=${goex:-}"
  for tag in $tags; do
    specs="$specs,tag=$tag"
  done
  echo "$specs"
}

default_specs=$(surface_key_specs "$default_surface")
darwin_specs=$(surface_key_specs "$darwin_surface")
# The cgo-on variant of the transitioned identity, for the rejection below.
SPECS_CGO_ON=$(printf '%s' "$darwin_specs" | sed 's/cgo_enabled=false/cgo_enabled=true/')

case "$darwin_specs" in
  *goos=darwin*) ;;
  *) fail "transitioned surface key is not darwin: $darwin_specs" ;;
esac
case "$darwin_specs" in
  *tag=adapter_probe*) ;;
  *) fail "transitioned surface key does not carry the build tag: $darwin_specs" ;;
esac
case "$darwin_specs" in
  *cgo_enabled=false*) ;;
  *) fail "transitioned surface key does not assert cgo off: $darwin_specs" ;;
esac
if [ "$default_specs" = "$darwin_specs" ]; then
  fail "default and transitioned surface keys are identical; the configurations are not distinguished"
fi
pass "transitioned surface SDK key is complete, cgo-off, and configuration-distinct: $darwin_specs"

# The surface's key must identify the map attached to its own configuration.
tmp_err="${TEST_TMPDIR:-${TMPDIR:-/tmp}}/inspect.err"
if ! "$arcc" stdlibmap inspect "$darwin_map" --expect-key="$darwin_specs" >/dev/null 2>"$tmp_err"; then
  echo "stderr was:" >&2; cat "$tmp_err" >&2
  fail "transitioned surface SDK key does not match its configuration's stdlib map"
fi
pass "transitioned surface SDK key matches its attached map exactly (cgo off, GOEXPERIMENT checked)"

# The cgo axis is asserted, not defaulted: the same identity with cgo enabled
# must be rejected by the same map.
if "$arcc" stdlibmap inspect "$darwin_map" --expect-key="$SPECS_CGO_ON" >/dev/null 2>&1; then
  fail "transitioned map accepted a cgo-enabled variant of the surface's SDK key"
fi
pass "cgo-off identity asserted: the map rejects the cgo-enabled variant"

# ...and must NOT identify the default configuration's map.
if "$arcc" stdlibmap inspect "$default_map" --expect-key="$darwin_specs" >/dev/null 2>&1; then
  fail "default stdlib map accepted the transitioned surface's SDK key"
fi
pass "transitioned surface SDK key is rejected by the default configuration's map"
