#!/usr/bin/env bash
# Reordered concrete member labels must leave the final package-layout bytes unchanged.
# Manifest semantics are owned by manifestparity; this check covers only the
# deterministic layout producer and therefore never couples the two artifact
# categories.
set -euo pipefail

first_layout=""
second_layout=""

for path in "$@"; do
  if [[ "${path}" == *.package-layout.json ]]; then
    if [[ -z "${first_layout}" ]]; then
      first_layout="${path}"
    else
      second_layout="${path}"
    fi
  fi
done

if [[ -z "${first_layout}" || -z "${second_layout}" ]]; then
  echo "FAIL: expected two member package-layout outputs, got: $*" >&2
  exit 1
fi

cmp "${first_layout}" "${second_layout}"
echo "OK: reordered member labels produce identical package-layout bytes"
