package goanalysis

import (
	"go/ast"
	"go/types"
)

// SurfaceInputs carries what surface emission (internal/surface) needs from
// one load of the component's member packages (plan Step 5 task 3). The
// declaration ASTs and the single types.Info covering them come from the
// same load, so extraction observes exactly the compiled member source.
//
// In layout mode the load root is the active workspace dir; paths are
// relative to it. In native mode the load root is the component root.
type SurfaceInputs struct {
	// InterfaceFiles are the ASTs of the surviving (non-constraint-excluded)
	// declared interface files. Declared-interface components only; nil for
	// PACKAGE_SURFACE components.
	InterfaceFiles []*ast.File
	// InterfaceInfo is the package type info covering InterfaceFiles.
	InterfaceInfo *types.Info
	// MemberPackages are the component member packages' canonical import
	// paths, sorted.
	MemberPackages []string
	// SourcePaths are the member packages' source files as slash paths
	// relative to the load root, sorted. Digest inputs for surface emission.
	SourcePaths []string
}

// LoadSurfaceInputs loads the component's member packages once and returns
// the surface-emission inputs derived from them: the surviving interface
// files' ASTs with their package's type info, the member import paths, and
// the member source paths. It performs no capability analysis and feeds no
// checker decision; it exists only so one analysis invocation can also emit
// the exact surface (plan Step 5 task 3).
//
// All declared interface files must belong to exactly one loaded member
// package: surface extraction needs one types.Info covering every interface
// declaration, and interface files spanning packages would make that
// extraction ambiguous.
func LoadSurfaceInputs(req LoadRequest) (SurfaceInputs, error) {
	return loadSurfaceInputs(req)
}

// PlatformIdentity is the active layout's pinned target declaration as plain
// values (plan Step 5 task 3): the complete target configuration surface
// emission needs to validate a declared stdlib map's key against the layout,
// without exposing the layout package's types across the component boundary.
type PlatformIdentity struct {
	// ToolchainVersion is the pinned toolchain version (`go1.N.M`).
	ToolchainVersion string
	// GOOS and GOARCH are the target operating system and architecture.
	GOOS, GOARCH string
	// CgoEnabled is the target cgo state.
	CgoEnabled bool
	// BuildTags are the target's user build tags.
	BuildTags []string
	// GOEXPERIMENT is the target's GOEXPERIMENT setting (empty when unset).
	GOEXPERIMENT string
}
