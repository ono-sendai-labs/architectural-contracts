#!/bin/sh
set -eu

layout="$1"

grep -q '"toolchain_version": "go1.26.4"' "$layout"
grep -q '"goos": "darwin"' "$layout"
grep -q '"goarch": "arm64"' "$layout"
grep -q '"build_tags": \[' "$layout"
grep -q '"adapter_probe"' "$layout"
grep -q '"cgo_enabled": false' "$layout"
grep -q '"goexperiment": ""' "$layout"
grep -q 'test_sdk_export_metadata.json' "$layout"
grep -q 'test_sdk_export_a.x' "$layout"
grep -q 'test_sdk_export_b.x' "$layout"
