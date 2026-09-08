package facts

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// SymbolID is the single reference identity used by the Step 6 reference
// vocabulary. It is an alias for symbol.SymbolID so the core's fact values
// carry exactly the declaring-object grammar of symbol (DR-04): no AST
// spelling, no instantiated generic brackets, no SSA names and no dual
// pointer/value receiver keys are representable.
type SymbolID = symbol.SymbolID

// SourceSite is a component-relative source position shared by reference,
// import and finding observations. File is relative to the component root and
// Line is 1-based, as reported for AST nodes of member packages.
type SourceSite struct {
	File string
	Line int
}

// CompareSourceSite totally orders two sites by file bytes, then line. The
// line comparison uses explicit less/greater branches so the order stays
// antisymmetric even for far-apart (or unvalidated) line values, where an
// integer subtraction would overflow.
func CompareSourceSite(a, b SourceSite) int {
	if c := strings.Compare(a.File, b.File); c != 0 {
		return c
	}
	switch {
	case a.Line < b.Line:
		return -1
	case a.Line > b.Line:
		return 1
	default:
		return 0
	}
}

// Validate checks that the site is a canonical component-relative position:
// a non-empty relative slash path with no absolute prefix, no ".", "..",
// empty (double-slash or trailing-slash) path elements, and a 1-based line.
// The separator policy is slash-only (POSIX/GNU spellings, which is what
// component-relative paths use on every platform here): any backslash —
// leading, embedded, or in a drive-letter prefix — is rejected rather than
// normalized, so a validated site is unambiguously a component-relative
// slash path on every host.
func (s SourceSite) Validate() error {
	if s.File == "" {
		return fmt.Errorf("source site: empty file")
	}
	if s.Line < 1 {
		return fmt.Errorf("source site %q: line %d is not 1-based", s.File, s.Line)
	}
	if strings.Contains(s.File, `\`) {
		return fmt.Errorf("source site %q must use slash separators, not backslashes", s.File)
	}
	if s.File == "." || strings.HasPrefix(s.File, "/") {
		return fmt.Errorf("source site %q is not a component-relative path", s.File)
	}
	if isDriveLetterPath(s.File) {
		return fmt.Errorf("source site %q is not component-relative: drive-letter paths are not component-relative", s.File)
	}
	for _, elem := range strings.Split(s.File, "/") {
		switch elem {
		case "", ".", "..":
			return fmt.Errorf("source site %q is not a clean component-relative path", s.File)
		}
	}
	return nil
}

// isDriveLetterPath reports whether s starts with a Windows drive-letter
// element such as "C:" (its "C:/..." spelling accepts the canonical slash
// separator but still escapes the component root).
func isDriveLetterPath(s string) bool {
	drive := strings.SplitN(s, "/", 2)[0]
	if len(drive) == 2 && drive[1] == ':' {
		c := drive[0]
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	return false
}

// ReferenceKind classifies what kind of object a reference edge names, under
// the declaring-object rule (DR-04). Interface method specs and struct fields
// carry RefField: their Referent is the declaring interface's or struct type's
// SymbolID, which is the key that authorises the reference. Aliases are
// RefType with the alias's own SymbolID.
type ReferenceKind string

const (
	RefFunc   ReferenceKind = "func"
	RefMethod ReferenceKind = "method"
	RefType   ReferenceKind = "type"
	RefField  ReferenceKind = "field"
	RefVar    ReferenceKind = "var"
	RefConst  ReferenceKind = "const"
)

// ReferenceEdge records one observation of member source naming a declared
// object outside (or inside) the component. It is produced by the reference
// scan (a shell concern, Task 02) and consumed by the pure checker.
//
// Identity is structural: two observations are the same edge exactly when
// every field matches. The same referent at the same site is an exact
// duplicate; a different site or object is a distinct edge.
type ReferenceEdge struct {
	Kind            ReferenceKind
	FromPackage     string // the referencing member package's canonical import path
	ReferentPackage string // canonical import path of the referenced object's package
	Referent        SymbolID
	Site            SourceSite
}

// ReferenceKey is the structural identity of a reference edge: all semantic
// fields, nothing more.
type ReferenceKey struct {
	Kind            ReferenceKind
	FromPackage     string
	ReferentPackage string
	Referent        SymbolID
	Site            SourceSite
}

// Key returns the edge's structural identity.
func (e ReferenceEdge) Key() ReferenceKey {
	return ReferenceKey{
		Kind:            e.Kind,
		FromPackage:     e.FromPackage,
		ReferentPackage: e.ReferentPackage,
		Referent:        e.Referent,
		Site:            e.Site,
	}
}

// Validate checks that the edge is well-formed: a known Kind, canonical
// package paths, a grammar-valid SymbolID whose package matches
// ReferentPackage, and a component-relative 1-based site.
func (e ReferenceEdge) Validate() error {
	switch e.Kind {
	case RefFunc, RefMethod, RefType, RefField, RefVar, RefConst:
	default:
		return fmt.Errorf("reference edge: unknown kind %q", e.Kind)
	}
	if e.FromPackage == "" {
		return fmt.Errorf("reference edge to %s: empty FromPackage", e.Referent)
	}
	if err := symbol.ValidateCanonicalPath(e.FromPackage); err != nil {
		return fmt.Errorf("reference edge to %s: %w", e.Referent, err)
	}
	if err := symbol.ValidateCanonicalPath(e.ReferentPackage); err != nil {
		return fmt.Errorf("reference edge to %s: %w", e.Referent, err)
	}
	id, err := symbol.Parse(string(e.Referent))
	if err != nil {
		return fmt.Errorf("reference edge from %s: %w", e.FromPackage, err)
	}
	if got := SymbolIDPackage(id); got != e.ReferentPackage {
		return fmt.Errorf("reference edge from %s: referent %s names package %q but edge records %q", e.FromPackage, id, got, e.ReferentPackage)
	}
	if err := e.Site.Validate(); err != nil {
		return fmt.Errorf("reference edge from %s to %s: %w", e.FromPackage, e.Referent, err)
	}
	return nil
}

// SymbolIDPackage extracts the package-path prefix of a parsed SymbolID under
// the v1 grammar: everything before the last dot of a "pkg.Name" spelling, or
// everything inside the leading parentheses of a "(pkg.T).Method" spelling.
func SymbolIDPackage(id SymbolID) string {
	text := string(id)
	if strings.HasPrefix(text, "(") {
		if close := strings.Index(text, ")."); close >= 0 {
			recv := text[1:close]
			dot := strings.LastIndexByte(recv, '.')
			if dot < 0 {
				return ""
			}
			return recv[:dot]
		}
	}
	dot := strings.LastIndexByte(text, '.')
	if dot < 0 {
		return ""
	}
	return text[:dot]
}

// CompareSymbolIDs totally orders two SymbolIDs byte-wise, mirroring
// symbol.Compare for deterministic artifact ordering.
func CompareSymbolIDs(a, b SymbolID) int {
	return symbol.Compare(a, b)
}

// CompareReferenceEdges totally orders two edges by their full identity.
func CompareReferenceEdges(a, b ReferenceEdge) int {
	ka, kb := a.Key(), b.Key()
	if c := strings.Compare(string(ka.Kind), string(kb.Kind)); c != 0 {
		return c
	}
	if c := strings.Compare(ka.FromPackage, kb.FromPackage); c != 0 {
		return c
	}
	if c := strings.Compare(ka.ReferentPackage, kb.ReferentPackage); c != 0 {
		return c
	}
	if c := symbol.Compare(ka.Referent, kb.Referent); c != 0 {
		return c
	}
	return CompareSourceSite(ka.Site, kb.Site)
}

// SortReferenceEdges returns a new slice of the edges in deterministic
// (total, byte-stable) order.
func SortReferenceEdges(edges []ReferenceEdge) []ReferenceEdge {
	out := make([]ReferenceEdge, len(edges))
	copy(out, edges)
	slices.SortStableFunc(out, CompareReferenceEdges)
	return out
}

// CloneReferenceEdges returns a shallow copy of the slice.
func CloneReferenceEdges(edges []ReferenceEdge) []ReferenceEdge {
	out := make([]ReferenceEdge, len(edges))
	copy(out, edges)
	return out
}

// DedupReferenceEdges removes exact duplicates (same referent and same site)
// from an already-sorted edge list, keeping one observation per distinct
// identity. It never drops edges that differ in any semantic field.
func DedupReferenceEdges(edges []ReferenceEdge) []ReferenceEdge {
	out := edges[:0:0]
	for i, e := range edges {
		if i > 0 && CompareReferenceEdges(edges[i-1], e) == 0 {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ImportResolution states how the written import path of an ImportEdge
// resolved against the loaded layout and type data. The states are exactly
// what the checker's import classification needs to tell a real unowned
// package from a missing-type/layout fault, without consulting the legacy
// package-import slices.
type ImportResolution string

const (
	// ImportResolved means the written path denotes a package the load
	// presented with complete type data.
	ImportResolved ImportResolution = "resolved"
	// ImportMissingTypeData means the written path is known to the layout
	// (a member, a stdlib package or a declared dependency), but no type
	// or layout data was available for it. This is a build-graph fault to
	// be reported as a tool error upstream, never an unowned package.
	ImportMissingTypeData ImportResolution = "missing_type_data"
	// ImportUnresolved means the written path matched no layout or SDK
	// package: a genuine unowned-package observation (UNDECLARED_DEPENDENCY
	// when type data exists nowhere for it).
	ImportUnresolved ImportResolution = "unresolved"
)

// ImportEdge records one import declaration of a member package, including
// blank imports (which need no separate flag: the written path and site are
// the observation).
//
// Identity is structural over all fields; see ReferenceEdge.
type ImportEdge struct {
	ImportingPackage string // the importing member package's canonical import path
	ImportPath       string // the written, canonical import path
	Resolution       ImportResolution
	Site             SourceSite
}

// ImportKey is the structural identity of an import edge.
type ImportKey struct {
	ImportingPackage string
	ImportPath       string
	Resolution       ImportResolution
	Site             SourceSite
}

// Key returns the edge's structural identity.
func (e ImportEdge) Key() ImportKey {
	return ImportKey{
		ImportingPackage: e.ImportingPackage,
		ImportPath:       e.ImportPath,
		Resolution:       e.Resolution,
		Site:             e.Site,
	}
}

// Validate checks that the import edge is well-formed: canonical package
// paths, a known resolution state, and a component-relative 1-based site.
func (e ImportEdge) Validate() error {
	switch e.Resolution {
	case ImportResolved, ImportMissingTypeData, ImportUnresolved:
	default:
		return fmt.Errorf("import edge for %q: unknown resolution %q", e.ImportPath, e.Resolution)
	}
	if e.ImportingPackage == "" {
		return fmt.Errorf("import edge for %q: empty ImportingPackage", e.ImportPath)
	}
	if err := symbol.ValidateCanonicalPath(e.ImportingPackage); err != nil {
		return fmt.Errorf("import edge for %q: %w", e.ImportPath, err)
	}
	if e.ImportPath == "" {
		return fmt.Errorf("import edge in %s: empty ImportPath", e.ImportingPackage)
	}
	if err := symbol.ValidateCanonicalPath(e.ImportPath); err != nil {
		return fmt.Errorf("import edge in %s: %w", e.ImportingPackage, err)
	}
	if err := e.Site.Validate(); err != nil {
		return fmt.Errorf("import edge in %s for %q: %w", e.ImportingPackage, e.ImportPath, err)
	}
	return nil
}

// CompareImportEdges totally orders two edges by their full identity.
func CompareImportEdges(a, b ImportEdge) int {
	ka, kb := a.Key(), b.Key()
	if c := strings.Compare(ka.ImportingPackage, kb.ImportingPackage); c != 0 {
		return c
	}
	if c := strings.Compare(ka.ImportPath, kb.ImportPath); c != 0 {
		return c
	}
	if c := strings.Compare(string(ka.Resolution), string(kb.Resolution)); c != 0 {
		return c
	}
	return CompareSourceSite(ka.Site, kb.Site)
}

// SortImportEdges returns a new slice of the edges in deterministic order.
func SortImportEdges(edges []ImportEdge) []ImportEdge {
	out := make([]ImportEdge, len(edges))
	copy(out, edges)
	slices.SortStableFunc(out, CompareImportEdges)
	return out
}

// DedupImportEdges removes exact duplicates from an already-sorted edge
// list, keeping one observation per distinct identity.
func DedupImportEdges(edges []ImportEdge) []ImportEdge {
	out := edges[:0:0]
	for i, e := range edges {
		if i > 0 && CompareImportEdges(edges[i-1], e) == 0 {
			continue
		}
		out = append(out, e)
	}
	return out
}
