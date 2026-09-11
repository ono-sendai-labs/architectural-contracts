#!/bin/sh
set -eu

layout="$1"

runtime_count=$(grep -o '"PkgPath": "example.com/injected/runtime"' "$layout" | wc -l | tr -d ' ')
if [ "$runtime_count" -ne 1 ]; then
  echo "expected one injected runtime package in final layout, got $runtime_count" >&2
  exit 1
fi

grep -q '"PkgPath": "example.com/injected/consumer"' "$layout"
grep -q '"ExportFile": .*runtime.x' "$layout"
