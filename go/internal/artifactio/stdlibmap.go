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

// normalizeMap validates m and returns a canonically sorted clone. Validation
// rejects nil repeated entries first, so every pointer access below is safe.
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

// symPackage returns the package path embedded in a SymbolID's text, using the
// same split the grammar uses: "(pkg.T).M" names package pkg through receiver
// pkg.T; every other form splits at the last dot.
func symPackage(id string) (string, bool) {
	if strings.HasPrefix(id, "(") {
		end := strings.Index(id, ")")
		if end < 0 {
			return "", false
		}
		recv := id[1:end]
		dot := strings.LastIndexByte(recv, '.')
		if dot < 0 {
			return "", false
		}
		return recv[:dot], true
	}
	dot := strings.LastIndexByte(id, '.')
	if dot < 0 {
		return "", false
	}
	return id[:dot], true
}

// validateMap enforces the stdlib map's semantic invariants: total-inventory
// consistency, terminal classifications, SDK-key agreement, and the evidence
// relationship. It mutates nothing and rejects nil repeated entries so
// normalization's pointer accesses are safe.
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

	// Package inventory: nil-free, sorted keys with no duplicates, concrete
	// paths; non-importable packages must not appear in the symbol or init
	// inventories.
	importable := make(map[string]bool, len(m.Packages))
	seen := make(map[string]bool, len(m.Packages))
	for _, p := range m.Packages {
		if p == nil {
			return fmt.Errorf("nil package inventory entry")
		}
		if seen[p.Path] {
			return fmt.Errorf("duplicate package inventory entry: %q", p.Path)
		}
		if err := validPackagePath(p.Path); err != nil {
			return fmt.Errorf("package inventory path %q: %w", p.Path, err)
		}
		seen[p.Path] = true
		importable[p.Path] = p.Importable
	}

	symRecords := make(map[string]*gen.SymbolRecord, len(m.Symbols))
	for _, s := range m.Symbols {
		if s == nil {
			return fmt.Errorf("nil symbol record")
		}
		key := s.Package + "\x00" + s.Id
		if _, dup := symRecords[key]; dup {
			return fmt.Errorf("duplicate symbol inventory entry: %s in %s (or a contradictory reclassification)", s.Id, s.Package)
		}
		symRecords[key] = s
		if !importable[s.Package] {
			return fmt.Errorf("symbol %s references non-importable or unlisted package %q", s.Id, s.Package)
		}
		if err := validateRecordClassification(s.Classification, s.Capabilities, fmt.Sprintf("symbol %s in %s", s.Id, s.Package)); err != nil {
			return err
		}
		parsed, err := symbol.Parse(s.Id)
		if err != nil {
			return fmt.Errorf("symbol %s in %s: %w", s.Id, s.Package, err)
		}
		if pkg, ok := symPackage(parsed.Format()); !ok || pkg != s.Package {
			return fmt.Errorf("symbol %q declares package %q but its grammar names package %q", s.Id, s.Package, pkg)
		}
	}

	initSeen := make(map[string]bool, len(m.Inits))
	for _, i := range m.Inits {
		if i == nil {
			return fmt.Errorf("nil init record")
		}
		if initSeen[i.Package] {
			return fmt.Errorf("duplicate init inventory entry: %q (or a contradictory reclassification)", i.Package)
		}
		initSeen[i.Package] = true
		if !importable[i.Package] {
			return fmt.Errorf("init references non-importable or unlisted package %q", i.Package)
		}
		if err := validateRecordClassification(i.Classification, i.Capabilities, fmt.Sprintf("init of %s", i.Package)); err != nil {
			return err
		}
	}

	evSeen := make(map[string]bool, len(m.Evidence))
	for _, e := range m.Evidence {
		if e == nil {
			return fmt.Errorf("nil evidence entry")
		}
		for _, f := range e.Frames {
			if f == nil {
				return fmt.Errorf("nil frame in evidence for (%s, %s)", e.SymbolId, e.Capability)
			}
		}
		key := e.SymbolId + "\x00" + e.Capability
		if evSeen[key] {
			return fmt.Errorf("duplicate evidence entry for (%s, %s)", e.SymbolId, e.Capability)
		}
		evSeen[key] = true
		if !manifest.KnownCapabilities[e.Capability] {
			return fmt.Errorf("evidence capability %q is not a known capability", e.Capability)
		}
		if _, err := symbol.Parse(e.SymbolId); err != nil {
			return fmt.Errorf("evidence symbol %q: %w", e.SymbolId, err)
		}
		pkg, ok := symPackage(e.SymbolId)
		if !ok {
			return fmt.Errorf("evidence symbol %q has no declaring package", e.SymbolId)
		}
		record := symRecords[pkg+"\x00"+e.SymbolId]
		if record == nil {
			return fmt.Errorf("evidence references symbol %q, which is absent from the inventory of %q", e.SymbolId, pkg)
		}
		if record.Classification != gen.Classification_CAPABILITIES {
			return fmt.Errorf("evidence for %q references a %s record; evidence exists only for CAPABILITIES symbols", e.SymbolId, record.Classification)
		}
		if !slices.Contains(record.Capabilities, e.Capability) {
			return fmt.Errorf("evidence capability %q is not among the capabilities of %q", e.Capability, e.SymbolId)
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
