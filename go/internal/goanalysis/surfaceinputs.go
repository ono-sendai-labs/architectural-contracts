package goanalysis

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
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

// DependencyArtifactBinding is the layout's structural binding record,
// re-exported through goanalysis so CLI orchestration keeps one shell port.
type DependencyArtifactBinding = packagelayout.DependencyArtifactBinding

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
	var dir string
	var patterns []string

	if packagelayout.IsLayoutMode() {
		layout := packagelayout.GetActiveLayout()
		if len(req.Members) > 0 {
			if err := validateLayoutMembership(layoutRequestMembers(req, layout), layout.Roots); err != nil {
				return SurfaceInputs{}, err
			}
		}
		dir = packagelayout.GetActiveWorkspaceDir()
		patterns = layout.Roots
	} else if len(req.Members) > 0 {
		dir = req.ComponentRoot
		patterns = append(patterns, req.Members...)
		patterns = append(patterns, interfacePackagePatterns(req.InterfaceFiles)...)
	} else {
		dir = req.ComponentRoot
		patterns = []string{"./..."}
	}

	cfg := &packages.Config{
		Mode: componentLoadMode,
		Dir:  dir,
	}
	pkgs, err := loadPackages(cfg, patterns...)
	if err != nil {
		return SurfaceInputs{}, fmt.Errorf("failed to load member packages for surface emission: %w", err)
	}
	if len(pkgs) == 0 {
		return SurfaceInputs{}, fmt.Errorf("no packages found under root %q", req.ComponentRoot)
	}
	if err := validateLoaderPackagePaths(pkgs); err != nil {
		return SurfaceInputs{}, err
	}
	if len(req.Members) > 0 && !packagelayout.IsLayoutMode() {
		if err := validateDeclaredPackages(req.Members, pkgs); err != nil {
			return SurfaceInputs{}, err
		}
	}

	// Member selection mirrors LoadPackageFacts: in layout mode the layout
	// roots are the component's packages; in native mode the declared
	// members plus the interface-file packages, or everything under the
	// component root when nothing is declared.
	var members []*packages.Package
	for _, p := range pkgs {
		if packagelayout.IsLayoutMode() || len(req.Members) == 0 ||
			packageIsDeclaredMember(p, req.Members) ||
			packageContainsInterfaceFile(p, req.ComponentRoot, req.InterfaceFiles) {
			members = append(members, p)
		}
	}
	if len(members) == 0 {
		return SurfaceInputs{}, fmt.Errorf("no member packages found under root %q", req.ComponentRoot)
	}

	// Collect load/parse/type errors over the loaded graph, with the same
	// layout unresolved-import tolerance as LoadPackageFacts.
	var errMsgs []string
	unresolvedPaths := make(map[string]bool)
	if packagelayout.IsLayoutMode() {
		for _, observation := range packagelayout.GetActiveLayout().UnresolvedImports {
			unresolvedPaths[observation.ImportPath] = true
		}
	}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			if isExpectedUnresolvedLayoutImport(e.Msg, unresolvedPaths) {
				continue
			}
			errMsgs = append(errMsgs, describePackageLoadError(p, e))
		}
	})
	if len(errMsgs) > 0 {
		unique := make(map[string]bool)
		var sorted []string
		for _, msg := range errMsgs {
			if !unique[msg] {
				unique[msg] = true
				sorted = append(sorted, msg)
			}
		}
		sort.Strings(sorted)
		return SurfaceInputs{}, fmt.Errorf("package load errors:\n%s", strings.Join(sorted, "\n"))
	}

	memberPathSet := make(map[string]bool, len(members))
	for _, member := range members {
		memberPathSet[hostpolicy.CanonicalizePath(member.PkgPath)] = true
	}
	memberOnlyLoad := !packagelayout.IsLayoutMode()
	strictNonMemberSources := false
	if packagelayout.IsLayoutMode() {
		layout := packagelayout.GetActiveLayout()
		memberOnlyLoad = packagelayout.IsMemberOnlyLayout(layout)
		strictNonMemberSources = memberOnlyLoad
	}
	if memberOnlyLoad {
		if hasExportBackedNonMembers(pkgs, memberPathSet) {
			if err := completeLoadedExportTypes(pkgs, memberPathSet); err != nil {
				return SurfaceInputs{}, err
			}
		}
		if err := validateLoadedPackageGraph(pkgs, memberPathSet, strictNonMemberSources); err != nil {
			return SurfaceInputs{}, err
		}
	}

	memberPaths := make([]string, 0, len(members))
	sourcePaths := make([]string, 0, len(members))
	sourceRoot := dir
	for _, p := range members {
		memberPaths = append(memberPaths, hostpolicy.CanonicalizePath(p.PkgPath))
		files := p.GoFiles
		if len(files) == 0 {
			files = p.CompiledGoFiles
		}
		for _, f := range files {
			rel, err := filepath.Rel(sourceRoot, f)
			if err != nil {
				return SurfaceInputs{}, fmt.Errorf("member source %q is not under the load root %q: %w", f, sourceRoot, err)
			}
			sourcePaths = append(sourcePaths, filepath.ToSlash(rel))
		}
	}
	sort.Strings(memberPaths)
	sort.Strings(sourcePaths)

	ifaceASTs, info, err := collectInterfaceDeclarations(members, interfaceFileRoot(req.ComponentRoot), req.InterfaceFiles, sourceRoot)
	if err != nil {
		return SurfaceInputs{}, err
	}

	return SurfaceInputs{
		InterfaceFiles: ifaceASTs,
		InterfaceInfo:  info,
		MemberPackages: memberPaths,
		SourcePaths:    sourcePaths,
	}, nil
}

// interfaceFileRoot is the directory declared interface files resolve
// against, mirroring ValidateInterfaceFiles: natively that is the component
// root (the manifest's directory); in layout mode the frame is the driver's
// working directory, because Bazel emitters write interface file paths
// relative to the runfiles root, not the manifest's directory.
func interfaceFileRoot(componentRoot string) string {
	if packagelayout.IsLayoutMode() {
		return packagelayout.GetActiveWorkspaceDir()
	}
	return componentRoot
}

// collectInterfaceDeclarations resolves each declared interface file to its
// AST in the loaded member package that owns it, requiring exactly one
// owning package across all interface files so a single types.Info covers
// every declaration (see LoadSurfaceInputs).
func collectInterfaceDeclarations(members []*packages.Package, componentRoot string, interfaceFiles []string, loadRoot string) ([]*ast.File, *types.Info, error) {
	if len(interfaceFiles) == 0 {
		return nil, nil, nil
	}

	// Map each surviving interface file's absolute path to its owning
	// package and AST. Files excluded by build constraints were already
	// reported by ValidateInterfaceFiles and are simply absent here.
	owners := map[*packages.Package]bool{}
	asts := make([]*ast.File, 0, len(interfaceFiles))
	for _, iface := range interfaceFiles {
		abs := filepath.Clean(filepath.Join(componentRoot, iface))
		found := false
		for _, p := range members {
			// Match against the loaded source files, then locate the AST by
			// the fileset position of its package clause.
			matchesFile := false
			for _, f := range p.GoFiles {
				if filepath.Clean(f) == abs {
					matchesFile = true
					break
				}
			}
			if !matchesFile {
				continue
			}
			var matched *ast.File
			for _, syntax := range p.Syntax {
				if syntax == nil {
					continue
				}
				if pos := p.Fset.Position(syntax.Package); filepath.Clean(pos.Filename) == abs {
					matched = syntax
					break
				}
			}
			if matched == nil {
				return nil, nil, fmt.Errorf("interface file %q was loaded but produced no AST", iface)
			}
			if !owners[p] && len(owners) > 0 {
				return nil, nil, fmt.Errorf("interface files span multiple packages; %q is in a different package than the other interface files", iface)
			}
			owners[p] = true
			asts = append(asts, matched)
			found = true
			break
		}
		if !found {
			return nil, nil, fmt.Errorf("interface file %q does not belong to any loaded member package", iface)
		}
	}

	var info *types.Info
	for p := range owners {
		if p.TypesInfo == nil {
			return nil, nil, fmt.Errorf("interface file %q's package carries no type information", interfaceFiles[0])
		}
		info = p.TypesInfo
	}
	return asts, info, nil
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

// ActivePlatformIdentity reports the active layout's pinned platform as a
// PlatformIdentity. When no layout is active or the layout declares no
// platform block, ok is false: an unpinned layout carries no declared
// target, so the caller cannot compare a declared map's key against it.
func ActivePlatformIdentity() (PlatformIdentity, bool) {
	if !packagelayout.IsLayoutMode() {
		return PlatformIdentity{}, false
	}
	layout := packagelayout.GetActiveLayout()
	if layout == nil || layout.Platform == nil {
		return PlatformIdentity{}, false
	}
	platform := layout.Platform
	goexperiment := ""
	if platform.GOEXPERIMENT != nil {
		goexperiment = *platform.GOEXPERIMENT
	}
	toolchain := ""
	if platform.ToolchainVersion != nil {
		toolchain = *platform.ToolchainVersion
	}
	return PlatformIdentity{
		ToolchainVersion: toolchain,
		GOOS:             platform.GOOS,
		GOARCH:           platform.GOARCH,
		CgoEnabled:       platform.CgoEnabled,
		BuildTags:        platform.BuildTags,
		GOEXPERIMENT:     goexperiment,
	}, true
}

// LayoutModeActive reports whether a package layout driver environment is
// active for this check. It mediates the layout-mode predicate across this
// component's interface for callers that already depend on goanalysis.
func LayoutModeActive() bool {
	return packagelayout.IsLayoutMode()
}

// ActiveDependencyArtifactBindings exposes the validated layout bindings to
// the CLI shell without making the CLI depend directly on packagelayout. The
// returned slice is a defensive copy; native mode has no build-graph binding.
func ActiveDependencyArtifactBindings() (bindings []packagelayout.DependencyArtifactBinding, workspace string, active bool) {
	if !packagelayout.IsLayoutMode() {
		return nil, "", false
	}
	layout := packagelayout.GetActiveLayout()
	if layout == nil {
		return nil, "", true
	}
	return append([]packagelayout.DependencyArtifactBinding(nil), layout.DependencyArtifactBindings...), packagelayout.GetActiveWorkspaceDir(), true
}

// CanonicalNamespace returns the host's canonical namespace identifier, the
// namespace stamped into emitted surfaces. It mediates the hostpolicy value
// across this component's interface so callers that already depend on
// goanalysis need no direct hostpolicy dependency.
func CanonicalNamespace() string {
	return hostpolicy.NamespaceID
}
