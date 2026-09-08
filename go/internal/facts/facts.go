// Package facts defines the pure data models that represent findings, packages,
// cross-component reference and import edges, and dependency interfaces. These
// data structures are core-defined but shell-produced: the checker in the pure
// core consumes them to decide conformance, while the loading logic in
// `goanalysis` (a shell package) builds them from the concrete Go packages.
// Dependencies point inward so that core components never import the shell.
//
// Component Contract (FR10):
//   - What it does: Defines pure structs for packages, exported symbols, static
//     call edges, dependency interfaces, and the Step 6 typed reference
//     vocabulary — ReferenceEdge, ImportEdge, SourceSite and the exact
//     MemberSet — with validation and total deterministic comparison
//     (refs.go, member.go).
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
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

// PackageFacts aggregates the loaded information for the component's internal packages,
// including all direct imports, exported symbols, and inter-package static call edges.
type PackageFacts struct {
	Packages []PackageFact

	// CallEdges is legacy compatibility data from the SSA/VTA pipeline,
	// kept only until the Step 6 Task 05 cutover replaces call edges with
	// ReferenceEdge. It is not converted to or from reference facts.
	CallEdges []CallEdge

	// StdlibImports is the loader-authoritative set of direct standard-library
	// imports across all component packages. Paths are canonical, unique, and
	// deterministically sorted. Every loader initializes the slice, including
	// when the authoritative result is empty; the pure checker treats nil and
	// empty values as an empty authoritative set.
	//
	// Legacy compatibility data kept until the Task 05 cutover; import
	// edges are superseded by ImportEdge.
	StdlibImports []string

	// UnresolvedImports records source-level imports that survived layout
	// filtering but did not resolve to a layout or SDK package. The loader owns
	// this observation; the pure checker renders it as an analysis limitation.
	//
	// Legacy compatibility data kept until the Task 05 cutover; the
	// observation is superseded by ImportEdge with ImportUnresolved
	// resolution.
	UnresolvedImports []UnresolvedImport
}

// UnresolvedImport is a pure observation of an import edge that the loader
// could not resolve. File is component-relative when produced by goanalysis.
type UnresolvedImport struct {
	Package    string
	File       string
	ImportPath string
}

// PackageFact represents metadata and extracted facts about a single loaded Go package.
type PackageFact struct {
	ImportPath      string
	Imports         []string         // direct imports of this package
	ExportedSymbols []ExportedSymbol // top-level exported declarations (+ init declarations)
}

// ExportedSymbol represents a top-level exported declaration or package-init function in Go source.
type ExportedSymbol struct {
	Name string // Capslock/go-types key form (e.g., "example.com/store.Read" or "(*example.com/store.DB).Get")
	File string // relative path to the file declaring the symbol (relative to component root)

	// Kind identifies the type of symbol. Must be one of:
	// "func", "type", "var", "const", "method", or "init".
	Kind string

	// Receiver is the declaring type's key for method symbols (e.g. "(*example.com/store.DB)").
	// This field is crucial for driving the FR4 well-formedness rule: methods of
	// interface-file types must themselves be declared in interface files.
	Receiver string
}

// CallEdge represents a static call from a caller symbol to a callee symbol.
// Used for the FR5 cross-component interface-boundary check.
type CallEdge struct {
	Caller capanalyzer.InterfaceSymbol // calling function/method (in some package)
	Callee capanalyzer.InterfaceSymbol // called function/method
}

// DependencyInterface is a direct component dependency's manifest and interface files
// resolved into its logical declared-interface symbol set (the FR4 symbol set).
//
// These fields are derived by the shell (specifically `goanalysis`) from the
// dependency's own root and manifest (not declared) and are consumed by the checker
// to validate boundary references and build the prune set.
//
// Symbols are shared symbol.SymbolID values under the declaring-object rule
// (DR-04): the exact exported declaring objects of the surviving interface
// files, with no implements-closure expansion and no dual pointer/value
// receiver keys.
type DependencyInterface struct {
	Component string

	// InterfaceStyle is read from the dependency's own manifest and is not verified by arcc.
	InterfaceStyle manifest.InterfaceStyle

	Packages []string   // all packages under the dependency's component root (derived, not declared)
	Symbols  []SymbolID // the exact declaring-object symbol set (derived, not declared)
}
