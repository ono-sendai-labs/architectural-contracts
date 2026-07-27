#!/usr/bin/env bash
# Reordered concrete member labels must leave generated layout and manifest bytes unchanged.
set -euo pipefail

first_layout=""
second_layout=""
first_manifest=""
second_manifest=""

for path in "$@"; do
  if [[ "${path}" == *.package-layout.json ]]; then
    if [[ -z "${first_layout}" ]]; then
      first_layout="${path}"
    else
      second_layout="${path}"
    fi
  elif [[ "${path}" == *.component.textproto ]]; then
    if [[ -z "${first_manifest}" ]]; then
      first_manifest="${path}"
    else
      second_manifest="${path}"
    fi
  fi
done

if [[ -z "${first_layout}" || -z "${second_layout}" ]]; then
  echo "FAIL: expected two member package-layout outputs, got: $*" >&2
  exit 1
fi

if [[ -z "${first_manifest}" || -z "${second_manifest}" ]]; then
  echo "FAIL: expected two member component.textproto outputs, got: $*" >&2
  exit 1
fi

cmp "${first_layout}" "${second_layout}"
diff -u <(grep -v '^name: ' "${first_manifest}") <(grep -v '^name: ' "${second_manifest}")
echo "OK: reordered member labels produce identical layout and manifest bytes"
