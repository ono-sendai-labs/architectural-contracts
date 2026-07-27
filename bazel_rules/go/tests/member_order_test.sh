#!/usr/bin/env bash
# Reordered concrete member labels must leave generated layout bytes unchanged.
set -euo pipefail

first=""
second=""
for path in "$@"; do
  if [[ "${path}" == *.package-layout.json ]]; then
    if [[ -z "${first}" ]]; then
      first="${path}"
    else
      second="${path}"
    fi
  fi
done

if [[ -z "${first}" || -z "${second}" ]]; then
  echo "FAIL: expected two member package-layout outputs, got: $*" >&2
  exit 1
fi

cmp "${first}" "${second}"
echo "OK: reordered member labels produce identical layout bytes"
