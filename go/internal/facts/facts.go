// Package facts defines the pure data models that represent findings, packages,
// and cross-component call boundaries. These data structures are core-defined but
// shell-produced: the checker in the pure core consumes them to decide conformance,
// while the loading logic in `goanalysis` (a shell package, Steps 6/9) builds them
// from the concrete Go packages. Dependencies point inward so that core components
// never import the shell.
//
// Component Contract (FR10):
// - What it does: Defines pure structs for packages, exported symbols, static call edges, and dependency interfaces.
// - What it requires: Constructed by the shell from static analysis or tests; holds no active logic or behaviors.
// - What it provides: The plain data representation of component facts used throughout the checker analysis.
// - Ambient Authority: This component is guaranteed-pure and holds no ambient authority (performs no filesystem, process, environment, or network operations).
package facts

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
)

// PackageFacts aggregates the loaded information for the component's internal packages,
// including all direct imports, exported symbols, and inter-package static call edges.
type PackageFacts struct {
	Packages  []PackageFact
	CallEdges []CallEdge // inter-package static call edges (from VTA call graph)

	// StdlibImports is the set of direct imports (across all component packages,
	// in canonical form) that the shell classified as Go standard library. It is
	// populated by the loader, which has authoritative module/SDK metadata that
	// the pure checker lacks. When non-nil it is authoritative and the checker
	// skips exactly these imports; when nil (e.g. hand-built facts in unit tests)
	// the checker falls back to its own string heuristic. A real load always
	// sets it (possibly empty, but non-nil).
	StdlibImports []string
}

// PackageFact represents metadata and extracted facts about a single loaded Go package.
type PackageFact struct {
	ImportPath      string
	IsStdlib        bool
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

	// PassesFuncValue flags call sites passing function-typed values.
	// This drives the FR5 HIGHER_ORDER_BOUNDARY_CALL warning (to warn that authority
	// exercised by the passed function might escape pruned attribution when called).
	PassesFuncValue bool
}

// DependencyInterface is a direct component dependency's manifest and interface files
// resolved into its logical declared-interface symbol set (the FR4 symbol set).
//
// These fields are derived by the shell (specifically `goanalysis`) from the
// dependency's own root and manifest (not declared) and are consumed by the checker
// to validate boundary calls and build the prune set.
type DependencyInterface struct {
	Component string
	Packages  []string                      // all packages under the dependency's component root (derived, not declared)
	Symbols   []capanalyzer.InterfaceSymbol // the FR4 symbol set (derived, not declared)
}
