// Package artifactio encodes and decodes the persisted archcontracts
// artifacts — surface manifests and standard-library maps — as canonical,
// byte-stable JSON, hashes them, and replaces files atomically.
//
// Component Contract (FR10):
//   - What it does: Normalizes artifact messages (sorting every repeated field by
//     its documented canonical sort key and rejecting non-canonical states),
//     encodes them with protojson followed by encoding/json compaction and
//     indentation so semantically identical artifacts serialize to identical
//     bytes, decodes them with bounded reads and semantic validation, computes
//     lowercase SHA-256 digests over the canonical bytes, and replaces target
//     files atomically via a temporary sibling and rename.
//   - What it requires: The generated artifact message types and known
//     capability taxonomy (schema), the SymbolID grammar (symbol) for
//     validation, and write access to the target directory for atomic
//     replacement.
//   - What it provides: MarshalSurface/MarshalMap, DecodeSurface/DecodeMap,
//     SurfaceDigest/MapDigest, and WriteFileAtomic with injectable write seams.
//     No cache regeneration policy, CLI commands, Bazel actions, or dependency
//     freshness computation.
//   - Ambient Authority: This is a shell component holding FILES,
//     READ_SYSTEM_STATE, MODIFY_SYSTEM_STATE, REFLECT, RUNTIME, SYSTEM_CALLS,
//     and UNSAFE_POINTER for filesystem writes and protobuf unmarshaling.
package artifactio

import (
	"bytes"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Canonical format contract (DR-15): an artifact is encoded with protojson,
// compacted with encoding/json to strip protojson's randomized inter-token
// spacing, re-indented with two-space indentation, and terminated with exactly
// one trailing newline ('\n'). protojson emits fields in field-number order,
// and format_version is field 1 of every top-level artifact message, so
// "formatVersion" is always the first key in the output. Repeated marshals of
// the same (or a semantically identical, differently ordered) message are
// byte-identical across supported protobuf versions.

const (
	// SurfaceFormatVersion is the only supported major surface format
	// version; readers and writers reject any other value.
	SurfaceFormatVersion = 1
	// MapFormatVersion is the only supported major stdlib-map format
	// version; readers and writers reject any other value.
	MapFormatVersion = 1
	// MaxSurfaceBytes caps a surface read before decoding (DR-15).
	MaxSurfaceBytes = 16 << 20 // 16 MiB
	// MaxMapBytes caps a stdlib-map read before decoding (DR-15).
	MaxMapBytes = 64 << 20 // 64 MiB
)

// ErrUnsupportedVersion reports an artifact whose major format_version is not
// one this build understands.
var ErrUnsupportedVersion = fmt.Errorf("unsupported artifact format version")

// canonicalJSON renders msg as the canonical artifact bytes described in the
// package contract.
func canonicalJSON(msg proto.Message) ([]byte, error) {
	raw, err := protojson.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal %T: %w", msg, err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, fmt.Errorf("compact %T: %w", msg, err)
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("indent %T: %w", msg, err)
	}
	indented.WriteByte('\n')
	return indented.Bytes(), nil
}
