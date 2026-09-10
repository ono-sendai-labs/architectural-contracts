#!/bin/sh
set -eu

arcc="$1"
generated="$2"
pinned="$3"

if ! cmp -s "$generated" "$pinned"; then
	echo "default Bazel stdlib map differs from the pinned routine artifact" >&2
	"$arcc" stdlibmap inspect "$generated" summary >&2 || true
	"$arcc" stdlibmap inspect "$pinned" summary >&2 || true
	exit 1
fi
echo "default Bazel stdlib map is byte-identical to the pinned artifact"
