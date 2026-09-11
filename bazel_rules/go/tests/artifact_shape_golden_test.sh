#!/usr/bin/env bash
# Compare one persisted artifact or final package layout with its typed,
# schema-aware shape golden. The arcc command performs bounded production
# decoding/schema validation and canonical-byte validation before comparing
# snapshots; this script never edits JSON fields. Use `report`, `surface`, or
# `stdlib-map` for persisted artifact shapes and `layout` only for final
# package-layout shapes. Semantic verdict goldens use `arcc verdict` and must
# never receive raw host output.
set -euo pipefail

arcc="$1"
kind="$2"
artifact="$3"
golden="$4"

"$arcc" artifact-shape "$kind" "$artifact" --golden="$golden"
