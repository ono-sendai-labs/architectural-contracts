// Package goanalysis loads Go packages and extracts static facts including typed reference/import edges and analysis-defeating bypass observations.
//
// Component Contract (FR10):
// - What it does: Analyzes Go syntax trees and types to load package structures, validate interface file correctness, scan the typed reference/import vocabulary of member sources, and detect analysis-defeating constructs. There is no SSA, no VTA call graph and no check-time Capslock: every stdlib decision is the StdlibAuthority port's.
// - What it requires: Directory paths on the local filesystem for the component being checked, package manifests, and bounded surface/report bytes plus explicit build-graph or native read seams. Dependency surfaces and the shared packagelayout loader are consumed through explicit component boundaries; component checks read non-member types from compiled export data.
// - What it provides: Structural package facts (typed reference and import edges, bypass observations), exact persisted dependency-interface facts, and independent dependency provenance/freshness/authority axes for checking component boundaries.
// - Ambient Authority: This component is authority-pure and declares no ambient authority.
package goanalysis

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

// LoadRequest describes the component-scoped package roots to load. Members are
// canonical literal import paths; the manifest parser rejects glob
// metacharacters before loading. The loader canonicalizes paths before applying
// membership matching. Interface files are used to retain their containing
// package as an implicit member.
type LoadRequest struct {
	// ComponentName identifies the manifest in load diagnostics.
	ComponentName  string
	ComponentRoot  string
	Members        []string
	InterfaceFiles []string
}

// WithDriverEnv re-exports the packagelayout seam so the cli component
// consumes goanalysis's declared interface instead of calling layout plumbing
// directly. The caller is responsible for serialization via SerializeChecks.
func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error {
	return packagelayout.WithDriverEnv(layoutPath, workspaceDir, fn)
}

// SerializeChecks runs fn while holding the global check mutex, so layout
// mode's process-global state (active layout, driver env) cannot race other
// check executions in the same process.
func SerializeChecks(fn func()) {
	packagelayout.CheckMu.Lock()
	defer packagelayout.CheckMu.Unlock()
	fn()
}

// LoadPackageFacts loads Go package membership and direct-import facts below the
// supplied component root using go/packages. Standard-library membership and
// authority are resolved later through the total StdlibAuthority map. In native
// mode go/packages delegates export discovery to the host Go driver's
// go list -export path, so this operation may execute the host toolchain; layout
// mode instead uses the declared self-exec driver and artifacts.
func LoadPackageFacts(req LoadRequest) (facts.PackageFacts, error) {
	return loadPackageFacts(req)
}

// InterfaceFileExclusion records a declared interface file that the active
// analysis build context excludes. Constraint is a stable, human-readable
// description of the build expression or filename constraint that caused the
// exclusion. It deliberately contains no report-layer types.
type InterfaceFileExclusion struct {
	File       string
	Constraint string
}

// ValidateInterfaceFiles verifies that each interface_files path exists, is relative,
// does not escape, is a regular file, and belongs to a loaded Go package beneath
// componentRoot. It returns build-constraint exclusions separately so callers can
// report them without making the loader depend on the report package.
func ValidateInterfaceFiles(componentRoot string, interfaceFiles []string, loaded facts.PackageFacts) ([]InterfaceFileExclusion, error) {
	return validateInterfaceFiles(componentRoot, interfaceFiles, loaded)
}
