package artifactio

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// MarshalMap returns the canonical bytes of a stdlib authority map. The input
// message is defensively copied before normalization, so the caller's message
// is never mutated. Duplicate entries, contradictory terminal classifications,
// and other non-canonical states are rejected rather than silently repaired.
func MarshalMap(m *gen.StdlibMap) ([]byte, error) {
	canonical, err := normalizeMap(m)
	if err != nil {
		return nil, err
	}
	return canonicalJSON(canonical)
}

// DecodeMap reads at most MaxMapBytes bytes from r, parses them as a stdlib
// map, and validates its semantic invariants. Unknown JSON fields are ignored
// (forward-compatible minor additions); malformed JSON, an unsupported major
// format_version, or a non-canonical semantic state fails with a specific
// error.
func DecodeMap(r io.Reader) (*gen.StdlibMap, error) {
	data, err := boundedRead(r, MaxMapBytes)
	if err != nil {
		return nil, err
	}
	var m gen.StdlibMap
	if err := decodeInto(data, &m); err != nil {
		return nil, err
	}
	if err := validateMap(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// MapDigest returns the lowercase hex SHA-256 digest of the canonical encoding
// of m. Hashing a decoded artifact operates on its re-canonicalized
// representation, not on the source's whitespace or entry ordering.
func MapDigest(m *gen.StdlibMap) (string, error) {
	canonical, err := normalizeMap(m)
	if err != nil {
		return "", err
	}
	data, err := canonicalJSON(canonical)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

// normalizeMap validates m and returns a canonically sorted clone.
func normalizeMap(m *gen.StdlibMap) (*gen.StdlibMap, error) {
	if m == nil {
		return nil, fmt.Errorf("stdlib map is nil")
	}
	if err := validateMap(m); err != nil {
		return nil, err
	}
	c := proto.Clone(m).(*gen.StdlibMap)
	slices.SortFunc(c.Packages, func(a, b *gen.PackageInventory) int {
		return strings.Compare(a.Path, b.Path)
	})
	slices.SortFunc(c.Symbols, compareSymbolRecords)
	slices.SortFunc(c.Inits, func(a, b *gen.InitRecord) int {
		return strings.Compare(a.Package, b.Package)
	})
	slices.SortFunc(c.Evidence, compareEvidence)
	for _, e := range c.Evidence {
		slices.SortFunc(e.Frames, compareFrames)
	}
	if c.Key != nil {
		c.Key.BuildTags = slices.Clone(c.Key.BuildTags)
		slices.Sort(c.Key.BuildTags)
	}
	for _, s := range c.Symbols {
		s.Capabilities = slices.Clone(s.Capabilities)
		sortStrings(s.Capabilities)
	}
	for _, i := range c.Inits {
		i.Capabilities = slices.Clone(i.Capabilities)
		sortStrings(i.Capabilities)
	}
	return c, nil
}

// compareSymbolRecords orders symbol records by (package, id), the sort key
// documented on StdlibMap.symbols.
func compareSymbolRecords(a, b *gen.SymbolRecord) int {
	if c := strings.Compare(a.Package, b.Package); c != 0 {
		return c
	}
	return strings.Compare(a.Id, b.Id)
}

// compareEvidence orders evidence entries by (symbol_id, capability), the sort
// key documented on StdlibMap.evidence.
func compareEvidence(a, b *gen.Evidence) int {
	if c := strings.Compare(a.SymbolId, b.SymbolId); c != 0 {
		return c
	}
	return strings.Compare(a.Capability, b.Capability)
}

// compareFrames orders evidence frames by (function, file, line) so evidence
// paths canonicalize regardless of the order the generator recorded them in.
func compareFrames(a, b *gen.Frame) int {
	if c := strings.Compare(a.Function, b.Function); c != 0 {
		return c
	}
	if c := strings.Compare(a.File, b.File); c != 0 {
		return c
	}
	return int(a.Line) - int(b.Line)
}

// validateMap enforces the stdlib map's semantic invariants: total-inventory
// consistency, terminal classifications, and SDK-key agreement. It mutates
// nothing.
func validateMap(m *gen.StdlibMap) error {
	if m.FormatVersion != MapFormatVersion {
		return fmt.Errorf("stdlib map %w: got %d, supported: %d", ErrUnsupportedVersion, m.FormatVersion, MapFormatVersion)
	}
	if m.Key == nil {
		return fmt.Errorf("stdlib map has no SDK key")
	}
	if m.Key.MapFormatVersion != m.FormatVersion {
		return fmt.Errorf("stdlib map format_version %d disagrees with the SDK key's map_format_version %d", m.FormatVersion, m.Key.MapFormatVersion)
	}
	if err := rejectDuplicates("SDK key build tag", m.Key.BuildTags); err != nil {
		return err
	}

	// Package inventory: sorted keys with no duplicates; non-importable
	// packages must not appear in the symbol or init inventories.
	seen := make(map[string]bool, len(m.Packages))
	for _, p := range m.Packages {
		if seen[p.Path] {
			return fmt.Errorf("duplicate package inventory entry: %q", p.Path)
		}
		if p.Path == "" {
			return fmt.Errorf("package inventory entry with empty path")
		}
		seen[p.Path] = true
	}

	symSeen := make(map[string]bool, len(m.Symbols))
	for _, s := range m.Symbols {
		key := s.Package + "\x00" + s.Id
		if symSeen[key] {
			return fmt.Errorf("duplicate symbol inventory entry: %s in %s (or a contradictory reclassification)", s.Id, s.Package)
		}
		symSeen[key] = true
		if err := validateRecordClassification(s.Classification, s.Capabilities, fmt.Sprintf("symbol %s in %s", s.Id, s.Package)); err != nil {
			return err
		}
		if _, err := symbol.Parse(s.Id); err != nil {
			return fmt.Errorf("symbol %s in %s: %w", s.Id, s.Package, err)
		}
		if !seen[s.Package] {
			return fmt.Errorf("symbol %s references package %q outside the total package inventory", s.Id, s.Package)
		}
	}

	initSeen := make(map[string]bool, len(m.Inits))
	for _, i := range m.Inits {
		if initSeen[i.Package] {
			return fmt.Errorf("duplicate init inventory entry: %q (or a contradictory reclassification)", i.Package)
		}
		initSeen[i.Package] = true
		if err := validateRecordClassification(i.Classification, i.Capabilities, fmt.Sprintf("init of %s", i.Package)); err != nil {
			return err
		}
		if !seen[i.Package] {
			return fmt.Errorf("init references package %q outside the total package inventory", i.Package)
		}
	}

	evSeen := make(map[string]bool, len(m.Evidence))
	for _, e := range m.Evidence {
		key := e.SymbolId + "\x00" + e.Capability
		if evSeen[key] {
			return fmt.Errorf("duplicate evidence entry for (%s, %s)", e.SymbolId, e.Capability)
		}
		evSeen[key] = true
		if !manifest.KnownCapabilities[e.Capability] {
			return fmt.Errorf("evidence capability %q is not a known capability", e.Capability)
		}
	}
	return nil
}

// validateRecordClassification enforces the terminal-classification contract
// (DR-05): exactly one of SAFE, a non-empty capability set, or UNANALYZED.
func validateRecordClassification(class gen.Classification, capabilities []string, what string) error {
	switch class {
	case gen.Classification_SAFE, gen.Classification_UNANALYZED:
		if len(capabilities) > 0 {
			return fmt.Errorf("%s: classification %s carries capabilities %v", what, class, capabilities)
		}
	case gen.Classification_CAPABILITIES:
		if len(capabilities) == 0 {
			return fmt.Errorf("%s: classification CAPABILITIES carries an empty capability list", what)
		}
		if err := rejectDuplicates(what+" capability", capabilities); err != nil {
			return err
		}
		for _, c := range capabilities {
			if !manifest.KnownCapabilities[c] {
				return fmt.Errorf("%s: capability %q is not a known capability", what, c)
			}
		}
	default:
		return fmt.Errorf("%s: classification is CLASSIFICATION_UNSPECIFIED; a persisted record must be terminal", what)
	}
	return nil
}
