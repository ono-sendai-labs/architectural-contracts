#!/bin/sh
# SDK-key equality for the asserted surface (Step 5 task 05, req 5 / AC 2):
# the asserted writer assembles the SDK key in Starlark from
# ArccStdlibMapInfo, whose generator-owned fields are mirrored constants
# (go/private:arcc_metadata.bzl — Starlark cannot hash). The mirroring is
# proven, not trusted: the asserted surface's complete SDK key, namespace,
# producer version and format version must equal what the checked emitter
# derived for a component of the same configuration, and the classifier hash
# and map format version must equal the key stamped into the default map
# artifact. Any drift of the mirrored constants fails this test.
#
# Per design I6 the asserted surface is package-level: it must carry no
# symbols and no digest key at all.
set -eu

checked_surface="$1"
asserted_surface="$2"
default_map="$3"

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

surface_field() {
  # One scalar field of the surface JSON by exact key (first occurrence at
  # top level: the surfaces carry every key at most once).
  sed -n "s/.*\"$1\": \"\([^\"]*\)\".*/\1/p" "$2" | head -1
}

map_field() {
  sed -n "s/.*\"$1\": \"\([^\"]*\)\".*/\1/p" "$2" | head -1
}

asserted_tv=$(surface_field toolchainVersion "$asserted_surface")
asserted_goos=$(surface_field goos "$asserted_surface")
asserted_goarch=$(surface_field goarch "$asserted_surface")
asserted_hash=$(surface_field classifierHash "$asserted_surface")
asserted_mfv=$(sed -n 's/.*"mapFormatVersion": \([0-9]*\).*/\1/p' "$asserted_surface" | head -1)
asserted_ns=$(surface_field namespace "$asserted_surface")
asserted_pv=$(surface_field producerVersion "$asserted_surface")
asserted_fv=$(sed -n 's/.*"formatVersion": \([0-9]*\).*/\1/p' "$asserted_surface" | head -1)

[ -n "$asserted_tv" ] && [ -n "$asserted_goos" ] && [ -n "$asserted_goarch" ] ||
  fail "asserted surface carries no SDK key identity fields"
[ -n "$asserted_hash" ] || fail "asserted surface carries no classifierHash"
[ -n "$asserted_mfv" ] || fail "asserted surface carries no mapFormatVersion"
[ -n "$asserted_fv" ] || fail "asserted surface carries no formatVersion"

checked_tv=$(surface_field toolchainVersion "$checked_surface")
checked_goos=$(surface_field goos "$checked_surface")
checked_goarch=$(surface_field goarch "$checked_surface")
checked_hash=$(surface_field classifierHash "$checked_surface")
checked_mfv=$(sed -n 's/.*"mapFormatVersion": \([0-9]*\).*/\1/p' "$checked_surface" | head -1)
checked_ns=$(surface_field namespace "$checked_surface")
checked_pv=$(surface_field producerVersion "$checked_surface")
checked_fv=$(sed -n 's/.*"formatVersion": \([0-9]*\).*/\1/p' "$checked_surface" | head -1)

# The asserted writer's assembled identity equals the checked emitter's for
# the same target configuration (task req 5).
[ "$asserted_tv" = "$checked_tv" ] || fail "toolchainVersion: asserted '$asserted_tv' != checked '$checked_tv'"
[ "$asserted_goos" = "$checked_goos" ] || fail "goos: asserted '$asserted_goos' != checked '$checked_goos'"
[ "$asserted_goarch" = "$checked_goarch" ] || fail "goarch: asserted '$asserted_goarch' != checked '$checked_goarch'"
[ "$asserted_hash" = "$checked_hash" ] || fail "classifierHash: asserted '$asserted_hash' != checked '$checked_hash'"
[ "$asserted_mfv" = "$checked_mfv" ] || fail "mapFormatVersion: asserted '$asserted_mfv' != checked '$checked_mfv'"
[ "$asserted_ns" = "$checked_ns" ] || fail "namespace: asserted '$asserted_ns' != checked '$checked_ns'"
[ "$asserted_pv" = "$checked_pv" ] || fail "producerVersion: asserted '$asserted_pv' != checked '$checked_pv'"
[ "$asserted_fv" = "$checked_fv" ] || fail "formatVersion: asserted '$asserted_fv' != checked '$checked_fv'"
pass "asserted surface identity equals the checked emitter's (SDK key, namespace, producer and format versions)"

# The mirrored constants equal the key the generator actually stamped into
# the map artifact (drift guard for arcc_metadata.bzl).
map_hash=$(map_field classifierHash "$default_map")
map_mfv=$(sed -n 's/.*"mapFormatVersion": \([0-9]*\).*/\1/p' "$default_map" | head -1)
[ "$asserted_hash" = "$map_hash" ] || fail "classifierHash: asserted '$asserted_hash' != map's stamped '$map_hash'"
[ "$asserted_mfv" = "$map_mfv" ] || fail "mapFormatVersion: asserted '$asserted_mfv' != map's stamped '$map_mfv'"
pass "asserted SDK key matches the map artifact's stamped key (classifierHash, mapFormatVersion)"

# I6: the asserted surface is package-level — no symbols and no digest key.
if grep -q '"symbols"' "$asserted_surface"; then
  fail "asserted surface must carry no symbols"
fi
if grep -q '"digest"' "$asserted_surface"; then
  fail "asserted surface must carry no digest"
fi
pass "asserted surface is package-level: no symbols, no digest"

# The checked surface keeps both fields (contrast guard: the asserted surface
# is not simply a truncated copy of the shared schema's usage).
grep -q '"symbols"' "$checked_surface" || fail "checked surface lost its symbols; fixture drift"
grep -q '"digest"' "$checked_surface" || fail "checked surface lost its digest; fixture drift"
pass "checked surface still carries symbols and digest (contrast)"
