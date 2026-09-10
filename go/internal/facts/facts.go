// Package facts defines the pure data models that represent findings, packages,
// cross-component reference and import edges, and dependency interfaces. These
// data structures are core-defined but shell-produced: the checker in the pure
// core consumes them to decide conformance, while the loading logic in
// `goanalysis` (a shell package) builds them from the concrete Go packages.
// Dependencies point inward so that core components never import the shell.
//
// Component Contract (FR10):
//   - What it does: Defines pure structs for packages, exported symbols, typed
//     reference/import edges, dependency interfaces, independent dependency
//     provenance/freshness/authority axes, and the Step 6 typed reference
//     vocabulary — ReferenceEdge, ImportEdge, SourceSite and the exact
//     MemberSet — with validation and total deterministic comparison (refs.go,
//     member.go).
//   - What it requires: Constructed by the shell from static analysis or
//     tests; holds no active logic or behaviors. Reference edges carry only
//     symbol.SymbolID referent identities (declaring-object rule); import
//     edges carry the written canonical import path and its resolution state.
//   - What it provides: The plain data representation of component facts used
//     throughout the checker analysis.
//   - Ambient Authority: This component is guaranteed-pure and holds no
//     ambient authority (performs no filesystem, process, environment, or
//     network operations).
package facts

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

// PackageFacts aggregates the loaded information for the component's internal packages,
// including its typed cross-boundary reference and import edges and any
// analysis-defeating bypass observations.
type PackageFacts struct {
	Packages []PackageFact

	// References are the typed object-reference edges of the member
	// packages (Uses/Selections under the declaring-object rule, DR-04),
	// sorted and duplicate-free. Every boundary and stdlib authority
	// decision is made from these edges.
	References []ReferenceEdge

	// Imports are the typed import edges of the member packages, sorted and
	// duplicate-free. Package imports are edges so that init-time authority
	// is attributed (R3).
	Imports []ImportEdge

	// Bypasses records the analysis-defeating constructs (DR-11) found in
	// member sources: linkname directives, assembly files and cgo use.
	Bypasses []BypassObservation
}

// PackageFact represents metadata and extracted facts about a single loaded Go package.
type PackageFact struct {
	ImportPath      string
	Imports         []string         // direct imports of this package
	ExportedSymbols []ExportedSymbol // top-level exported declarations (+ init declarations)
}

// ExportedSymbol represents a top-level exported declaration or package-init function in Go source.
type ExportedSymbol struct {
	Name string // Canonical symbol key form (e.g., "example.com/store.Read" or "(*example.com/store.DB).Get")
	File string // relative path to the file declaring the symbol (relative to component root)

	// Kind identifies the type of symbol. Must be one of:
	// "func", "type", "var", "const", "method", or "init".
	Kind string

	// Receiver is the declaring type's key for method symbols (e.g. "(*example.com/store.DB)").
	// This field is crucial for driving the FR4 well-formedness rule: methods of
	// interface-file types must themselves be declared in interface files.
	Receiver string
}

// DependencyInterface is a direct component dependency's manifest and interface files
// resolved into its logical declared-interface symbol set (the FR4 symbol set).
//
// These fields are derived by the shell (specifically `goanalysis`) from the
// dependency's own root and manifest (not declared) and are consumed by the checker
// to validate boundary references against the exact declared interface.
//
// Symbols are shared symbol.SymbolID values under the declaring-object rule
// (DR-04): the exact exported declaring objects of the surviving interface
// files — the same set the emitted surface carries — with no dual
// pointer/value receiver keys.
type DependencyInterface struct {
	Component string

	// InterfaceStyle is read from the dependency's own manifest and is not verified by arcc.
	InterfaceStyle manifest.InterfaceStyle

	Packages []string   // all packages under the dependency's component root (derived, not declared)
	Symbols  []SymbolID // the exact declaring-object symbol set (derived, not declared)

	// Provenance, Freshness, and Authority are independent boundary axes. They
	// are populated by the surface consumer; the legacy source-backed resolver
	// remains available during the additive Step 7 transition.
	Provenance DependencyProvenance
	Freshness  DependencyFreshness
	Authority  manifest.AuthorityDeclaration
}
