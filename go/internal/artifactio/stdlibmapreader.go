// Stdlib map reader: converts the persisted stdlib authority map (DR-05) into
// the core-side stdlibauthority.StdlibAuthority port. Decoding stays in this
// shell (the package's FR10 contract applies); the port itself is pure core.
package artifactio

import (
	"fmt"
	"io"
	"slices"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// Compile-time assertion: the artifact-backed reader satisfies the core-side
// StdlibAuthority port (design §stdlibmap, "pinned artifact reader").
var _ stdlibauthority.StdlibAuthority = (*stdlibMapReader)(nil)

// NewStdlibMapReader converts a validated in-memory persisted stdlib map into
// a StdlibAuthority. It validates the message's semantic invariants even
// though it skips the JSON decoder (technical requirement 5): an invalid or
// duplicate terminal state handed in by any caller is rejected here, never
// indexed. When expected is non-nil, the reader fails closed with
// ErrKeyMismatch unless the map's SDK key agrees with it on every field (N3).
//
// The returned authority holds immutable indexes built once from m; it does
// not alias m's mutable slices. Lookup results are defensive copies.
func NewStdlibMapReader(m *gen.StdlibMap, expected *stdlibauthority.SDKKey) (stdlibauthority.StdlibAuthority, error) {
	if m == nil {
		return nil, fmt.Errorf("stdlib map is nil")
	}
	if err := validateMap(m); err != nil {
		return nil, err
	}
	key, err := sdkKeyFromProto(m.Key)
	if err != nil {
		return nil, err
	}
	if expected != nil {
		if fields := stdlibauthority.EqualKeys(key, *expected); len(fields) > 0 {
			return nil, &stdlibauthority.KeyMismatchError{Fields: fields}
		}
	}

	r := &stdlibMapReader{
		packages: make(map[string]bool, len(m.Packages)),
		symbols:  make(map[string]stdlibauthority.Classification, len(m.Symbols)),
		inits:    make(map[string]stdlibauthority.Classification, len(m.Inits)),
		evidence: make(map[string][]stdlibauthority.Frame, len(m.Evidence)),
		key:      key,
	}
	for _, p := range m.Packages {
		r.packages[p.Path] = true
	}
	for _, s := range m.Symbols {
		r.symbols[s.Package+"\x00"+s.Id] = classificationFromProto(s.Classification, s.Capabilities)
	}
	for _, i := range m.Inits {
		r.inits[i.Package] = classificationFromProto(i.Classification, i.Capabilities)
	}
	for _, e := range m.Evidence {
		frames := make([]stdlibauthority.Frame, 0, len(e.Frames))
		for _, f := range e.Frames {
			frames = append(frames, stdlibauthority.Frame{Function: f.Function, File: f.File, Line: int(f.Line)})
		}
		r.evidence[e.SymbolId+"\x00"+e.Capability] = frames
	}
	return r, nil
}

// OpenStdlibMap decodes the canonical stdlib-map artifact from r and opens it
// for lookup, with the same fail-closed key check as NewStdlibMapReader.
func OpenStdlibMap(r io.Reader, expected *stdlibauthority.SDKKey) (stdlibauthority.StdlibAuthority, error) {
	m, err := DecodeMap(r)
	if err != nil {
		return nil, err
	}
	return NewStdlibMapReader(m, expected)
}

// sdkKeyFromProto converts the persisted SDK-key message to the core value,
// canonicalizing build tags to sorted order as EqualKeys expects.
func sdkKeyFromProto(k *gen.SDKKey) (stdlibauthority.SDKKey, error) {
	if k == nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("stdlib map has no SDK key")
	}
	tags := slices.Clone(k.BuildTags)
	slices.Sort(tags)
	return stdlibauthority.SDKKey{
		ToolchainVersion: k.ToolchainVersion,
		GOOS:             k.Goos,
		GOARCH:           k.Goarch,
		CgoEnabled:       k.CgoEnabled,
		BuildTags:        tags,
		GOEXPERIMENT:     k.Goexperiment,
		ClassifierHash:   k.ClassifierHash,
		MapFormatVersion: k.MapFormatVersion,
	}, nil
}

// classificationFromProto converts a validated persisted record's terminal
// classification to the core model. The record passed validateMap, so the
// classification is exactly one terminal state and the capability set is
// non-empty exactly for CAPABILITIES; defensive-copies the capability slice
// away from the caller's message.
func classificationFromProto(class gen.Classification, capabilities []string) stdlibauthority.Classification {
	switch class {
	case gen.Classification_SAFE:
		return stdlibauthority.Classification{Safe: true}
	case gen.Classification_CAPABILITIES:
		// The persisted record passed validateMap, which rejects empty,
		// duplicate, and unknown capability names; canonicalize the clone to
		// the sorted form the core terminal model requires.
		caps := slices.Clone(capabilities)
		slices.Sort(caps)
		return stdlibauthority.Classification{Capabilities: caps}
	case gen.Classification_UNANALYZED:
		return stdlibauthority.Classification{Unanalyzed: true}
	default:
		// Unreachable after validateMap; kept total so a future enum value
		// cannot silently index as an empty classification.
		return stdlibauthority.Classification{}
	}
}

// stdlibMapReader is the immutable index view of one decoded stdlib map. All
// maps are built once at construction; lookup returns defensive copies.
type stdlibMapReader struct {
	packages map[string]bool
	symbols  map[string]stdlibauthority.Classification
	inits    map[string]stdlibauthority.Classification
	evidence map[string][]stdlibauthority.Frame
	key      stdlibauthority.SDKKey
}

// IsStdlibPackage implements the port: membership is exactly the map's
// package enumeration, including non-importable internal packages (R6).
func (r *stdlibMapReader) IsStdlibPackage(pkgPath string) bool {
	return r.packages[pkgPath]
}

// SymbolAuthority implements the port's fail-closed symbol lookup (R6): a
// symbol absent from an enumerated package's total inventory is an
// ErrInventoryGap naming the absent package and symbol.
func (r *stdlibMapReader) SymbolAuthority(id symbol.SymbolID) (stdlibauthority.Classification, error) {
	if class, ok := r.symbols[r.symbolKey(id)]; ok {
		return copyClassification(class), nil
	}
	pkg, _ := symPackage(id.Format())
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{Package: pkg, Symbol: id.Format()}
}

// PackageInitAuthority implements the port's fail-closed init lookup (R3):
// a package absent from the enumeration, or enumerated without an init
// record, is an ErrInventoryGap naming the package.
func (r *stdlibMapReader) PackageInitAuthority(pkgPath string) (stdlibauthority.Classification, error) {
	if class, ok := r.inits[pkgPath]; ok {
		return copyClassification(class), nil
	}
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{Package: pkgPath}
}

// Evidence implements the port: the generator's canned path for one
// (symbol, capability) pair, in the persisted caller-to-capability order
// (DR-17), returned as a defensive copy.
func (r *stdlibMapReader) Evidence(id symbol.SymbolID, cap stdlibauthority.Capability) []stdlibauthority.Frame {
	frames, ok := r.evidence[id.Format()+"\x00"+cap]
	if !ok {
		return nil
	}
	return slices.Clone(frames)
}

// Key implements the port: the target SDK configuration the map describes.
// The build-tag slice is a defensive copy, so callers cannot mutate the
// reader's key state.
func (r *stdlibMapReader) Key() stdlibauthority.SDKKey {
	key := r.key
	key.BuildTags = slices.Clone(key.BuildTags)
	return key
}

// symbolKey builds the reader's symbol index key from a SymbolID and its
// declaring package.
func (r *stdlibMapReader) symbolKey(id symbol.SymbolID) string {
	pkg, _ := symPackage(id.Format())
	return pkg + "\x00" + id.Format()
}

// copyClassification returns a defensive copy of a classification whose
// capability slice callers cannot mutate.
func copyClassification(c stdlibauthority.Classification) stdlibauthority.Classification {
	c.Capabilities = slices.Clone(c.Capabilities)
	return c
}
