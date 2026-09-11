#!/usr/bin/env bash
# Assert byte identity at the persisted asserted-surface boundary. The three
# inputs have the same component name and semantic package set, while their
# raw member declarations differ in order or contain a duplicate.
set -euo pipefail

if [[ "$#" -ne 3 ]]; then
  echo "usage: $0 <ordered> <reversed> <duplicate>" >&2
  exit 2
fi

cmp -s "$1" "$2" || {
  echo "reordered UNKNOWN asserted surfaces differ" >&2
  exit 1
}
cmp -s "$1" "$3" || {
  echo "duplicate-member UNKNOWN asserted surface differs" >&2
  exit 1
}
