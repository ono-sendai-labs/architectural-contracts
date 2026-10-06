package packagelayout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/types"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout/internal/hostenv"
	"golang.org/x/tools/go/packages"
)

var (
	envMu              sync.Mutex
	activeMu           sync.RWMutex
	activeLayout       *Layout
	activeLayoutPath   string
	activeWorkspaceDir string
)

func isLayoutMode() bool {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeLayout != nil
}

func getActiveLayout() *Layout {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeLayout
}

func getActiveLayoutPath() string {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeLayoutPath
}

func getActiveWorkspaceDir() string {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeWorkspaceDir
}

// cloneDriverModule returns a defensive copy of module metadata, including the
// recursive replacement chain and error/time pointers exposed by packages.Module.
func cloneDriverModule(module *packages.Module) *packages.Module {
	if module == nil {
		return nil
	}
	clone := *module
	if module.Replace != nil {
		clone.Replace = cloneDriverModule(module.Replace)
	}
	if module.Time != nil {
		timestamp := *module.Time
		clone.Time = &timestamp
	}
	if module.Error != nil {
		clone.Error = &packages.ModuleError{Err: module.Error.Err}
	}
	return &clone
}

// cloneDriverPackage copies package metadata without retaining mutable slices,
// maps, or module pointers from the validated layout. Runtime type/syntax state
// is never part of a layout driver response and is deliberately cleared.
func cloneDriverPackage(pkg *packages.Package) *packages.Package {
	if pkg == nil {
		return nil
	}
	clone := *pkg
	clone.Errors = append([]packages.Error(nil), pkg.Errors...)
	clone.TypeErrors = append([]types.Error(nil), pkg.TypeErrors...)
	clone.GoFiles = slices.Clone(pkg.GoFiles)
	clone.CompiledGoFiles = slices.Clone(pkg.CompiledGoFiles)
	clone.OtherFiles = slices.Clone(pkg.OtherFiles)
	clone.EmbedFiles = slices.Clone(pkg.EmbedFiles)
	clone.EmbedPatterns = slices.Clone(pkg.EmbedPatterns)
	clone.IgnoredFiles = slices.Clone(pkg.IgnoredFiles)
	clone.Module = cloneDriverModule(pkg.Module)
	clone.Types = nil
	clone.Fset = nil
	clone.Syntax = nil
	clone.TypesInfo = nil
	clone.TypesSizes = nil
	clone.Imports = nil
	return &clone
}

// projectDriverPackages returns the full graph with source roles scoped to the
// requested roots. Export-backed non-roots retain identity, export data, and
// every import edge, but no source-shaped field that could make go/packages
// consult dependency syntax. The returned graph is independent of l.
func projectDriverPackages(source []*packages.Package, rootIDs map[string]bool) []*packages.Package {
	clones := make(map[string]*packages.Package, len(source))
	for _, pkg := range source {
		if pkg == nil {
			continue
		}
		clone := cloneDriverPackage(pkg)
		if rootIDs[pkg.ID] {
			// A root is loaded from source for this request. Its export artifact,
			// if a producer happened to attach one, is not its effective role.
			clone.ExportFile = ""
		} else {
			clone.GoFiles = nil
			clone.CompiledGoFiles = nil
			clone.OtherFiles = nil
			clone.EmbedFiles = nil
			clone.EmbedPatterns = nil
			clone.IgnoredFiles = nil
		}
		clones[pkg.ID] = clone
	}

	for _, pkg := range source {
		if pkg == nil {
			continue
		}
		clone := clones[pkg.ID]
		if pkg.Imports == nil {
			continue
		}
		clone.Imports = make(map[string]*packages.Package, len(pkg.Imports))
		for importPath, imported := range pkg.Imports {
			if imported == nil {
				clone.Imports[importPath] = nil
				continue
			}
			if importedClone, ok := clones[imported.ID]; ok {
				clone.Imports[importPath] = importedClone
				continue
			}
			clone.Imports[importPath] = cloneDriverPackage(imported)
		}
	}

	projected := make([]*packages.Package, 0, len(source))
	for _, pkg := range source {
		if pkg != nil {
			projected = append(projected, clones[pkg.ID])
		}
	}
	slices.SortFunc(projected, func(a, b *packages.Package) int {
		return strings.Compare(a.ID, b.ID)
	})
	return projected
}

// validateForDriver selects the source-backed path used by whole-SDK
// generation and the export-backed path used by component checks.
func validateForDriver(l *Layout, workspaceDir string) error {
	if IsMemberOnlyLayout(l) {
		return validateAndResolveForMemberOnly(l, workspaceDir)
	}
	return validateAndResolve(l, workspaceDir)
}

// handleDriverRequest processes a packages.DriverRequest using a validated Layout
// and the query patterns, returning a packages.DriverResponse.
func handleDriverRequest(l *Layout, req *packages.DriverRequest, patterns []string) (*packages.DriverResponse, error) {
	byID := make(map[string]*packages.Package, len(l.Packages))
	byPath := make(map[string]*packages.Package, len(l.Packages))
	var stdIDs []string

	for _, p := range l.Packages {
		byID[p.ID] = p
		byPath[p.PkgPath] = p
		if l.IsStdlibPackage(p) {
			stdIDs = append(stdIDs, p.ID)
		}
	}

	sort.Strings(stdIDs)

	var roots []string
	seen := map[string]bool{}
	addRoot := func(id string) {
		if !seen[id] {
			seen[id] = true
			roots = append(roots, id)
		}
	}

	for _, pat := range patterns {
		switch pat {
		case "std":
			if len(stdIDs) == 0 {
				return nil, fmt.Errorf("query for %q but no standard library packages exist in layout", pat)
			}
			for _, id := range stdIDs {
				addRoot(id)
			}
		default:
			if p, ok := byPath[pat]; ok {
				addRoot(p.ID)
			} else if p, ok := byID[pat]; ok {
				addRoot(p.ID)
			} else {
				return nil, fmt.Errorf("no layout package found for pattern %q", pat)
			}
		}
	}

	sort.Strings(roots)

	rootIDs := make(map[string]bool, len(roots))
	for _, id := range roots {
		rootIDs[id] = true
	}
	if !IsMemberOnlyLayout(l) {
		for _, pkg := range l.Packages {
			if pkg != nil {
				rootIDs[pkg.ID] = true
			}
		}
	}

	pkgsCopy := projectDriverPackages(l.Packages, rootIDs)

	arch := hostenv.NativeTargetDefaults().GOARCH
	if l.Platform != nil {
		arch = l.Platform.GOARCH
	}
	resp := &packages.DriverResponse{
		Compiler: "gc",
		Arch:     arch,
		Roots:    roots,
		Packages: pkgsCopy,
	}
	return resp, nil
}

func runDriverImpl(layout []byte, layoutPath, workspaceDir string, patterns []string, stdin io.Reader, stdout io.Writer) error {
	var req packages.DriverRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		if err != io.EOF {
			return fmt.Errorf("parsing DriverRequest: %w", err)
		}
	}

	parsed, err := Parse(bytes.NewReader(layout))
	if err != nil {
		return err
	}
	if err := attachLayoutPathContext(parsed, layoutPath, workspaceDir); err != nil {
		return fmt.Errorf("resolving layout runtime metadata: %w", err)
	}

	if err := validateForDriver(parsed, workspaceDir); err != nil {
		return fmt.Errorf("validating layout: %w", err)
	}

	resp, err := handleDriverRequest(parsed, &req, patterns)
	if err != nil {
		return fmt.Errorf("handling driver request: %w", err)
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("encoding response: %w", err)
	}

	return nil
}

func withDriverEnvImpl(layoutPath, workspaceDir string, fn func() error) error {
	envMu.Lock()
	defer envMu.Unlock()

	f, err := defaultFS.Open(filepath.ToSlash(layoutPath))
	if err != nil {
		return fmt.Errorf("opening layout file: %w", err)
	}
	defer f.Close()

	layout, err := Parse(f)
	if err != nil {
		return fmt.Errorf("parsing layout file: %w", err)
	}
	if err := attachLayoutPathContext(layout, layoutPath, workspaceDir); err != nil {
		return fmt.Errorf("resolving layout runtime metadata: %w", err)
	}

	if err := validateForDriver(layout, workspaceDir); err != nil {
		return fmt.Errorf("validating layout file: %w", err)
	}

	return hostenv.WithDriverEnv(layoutPath, workspaceDir, func() error {
		activeMu.Lock()
		origActiveLayout := activeLayout
		origActiveLayoutPath := activeLayoutPath
		origActiveWorkspaceDir := activeWorkspaceDir

		activeLayout = layout
		activeLayoutPath = layoutPath
		activeWorkspaceDir = workspaceDir
		activeMu.Unlock()

		defer func() {
			activeMu.Lock()
			activeLayout = origActiveLayout
			activeLayoutPath = origActiveLayoutPath
			activeWorkspaceDir = origActiveWorkspaceDir
			activeMu.Unlock()
		}()

		return fn()
	})
}
