#!/usr/bin/env bash
# Drives the hermetic-loading spike end to end:
#   1. build the self-exec binary,
#   2. generate a package layout the ordinary way (stand-in for a Bazel aspect),
#   3. analyze from a directory with NO go.mod, loading only via the driver.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
bin="$(mktemp -d)/spikedriver"
layout="$(mktemp -d)/layout.json"

echo "== build =="
( cd "$here" && go build -o "$bin" . )

echo "== gen layout (uses go list; stands in for Bazel aspect output) =="
"$bin" gen "$here/testdata" ./... > "$layout"
echo "layout: $(grep -c '"PkgPath"' "$layout") packages, $(wc -c < "$layout") bytes"

# Analyze from an empty dir with no go.mod, to prove no module is needed.
work="$(mktemp -d)"
cd "$work"
echo "cwd = $work (no go.mod: $( [ -f go.mod ] && echo present || echo absent ))"

echo
echo "== analyze example.com/svc/svc (expect FILES) =="
"$bin" analyze "$layout" example.com/svc/svc

echo
echo "== analyze example.com/svc/clean (expect no capabilities) =="
"$bin" analyze "$layout" example.com/svc/clean
