#!/usr/bin/env bash
# Compare one persisted artifact with its typed, schema-aware shape golden.
# The arcc command performs bounded production decoding and canonical-byte
# validation before comparing snapshots; this script never edits JSON fields.
set -euo pipefail

arcc="$1"
kind="$2"
artifact="$3"
golden="$4"

"$arcc" artifact-shape "$kind" "$artifact" --golden="$golden"
