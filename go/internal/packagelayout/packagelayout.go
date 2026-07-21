// Package packagelayout defines the schema, validation, and driver protocol
// for arcc's hermetic package-layout loader.
package packagelayout

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// osStat is a seam for mocking file existence in unit tests.
var osStat = os.Stat

// Layout is the package layout schema representing a Bazel-produced package graph.
type Layout struct {
	GoSDKRoot string              `json:"go_sdk_root"`
	Roots     []string            `json:"roots"`
	Packages  []*packages.Package `json:"packages"`
}

// IsStdlib reports whether importPath is a standard-library package, using the
// same heuristic the go tool uses: the first path segment contains no dot.
func IsStdlib(importPath string) bool {
	if importPath == "" {
		return false
	}
	first := importPath
	if i := strings.IndexByte(importPath, '/'); i >= 0 {
		first = importPath[:i]
	}
	return !strings.Contains(first, ".")
}

// discoverStdlib walks the standard library source tree rooted at sdkRoot,
// identifies importable packages using go/build, and returns them as a sorted slice of *packages.Package.
func discoverStdlib(sdkRoot string) ([]*packages.Package, error) {
	if sdkRoot == "" {
		return nil, nil
	}

	bctx := build.Default
	bctx.GOROOT = filepath.Dir(sdkRoot)
	return discoverStdlibWithContext(sdkRoot, bctx)
}

func discoverStdlibWithContext(sdkRoot string, bctx build.Context) ([]*packages.Package, error) {

	fi, err := os.Stat(sdkRoot)
	if err != nil {
		return nil, fmt.Errorf("accessing SDK root %q: %w", sdkRoot, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("SDK root %q is not a directory", sdkRoot)
	}

	var packageDirs []string
	err = filepath.Walk(sdkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walking SDK root at %s: %w", path, err)
		}

		if info.IsDir() {
			rel, err := filepath.Rel(sdkRoot, path)
			if err != nil {
				return fmt.Errorf("failed to compute relative path of %s from SDK root: %w", path, err)
			}
			importPath := filepath.ToSlash(rel)
			if importPath == "." || importPath == "" {
				return nil
			}

			// go list std excludes the top-level cmd tree and every testdata
			// directory, but includes the GOROOT-level vendor tree.
			if importPath == "cmd" || importPath == "testdata" ||
				strings.HasPrefix(importPath, "cmd/") || strings.HasSuffix(importPath, "/testdata") ||
				strings.Contains(importPath, "/testdata/") {
				return filepath.SkipDir
			}
			packageDirs = append(packageDirs, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Strings(packageDirs)

	type discoveredPackage struct {
		pkg     *packages.Package
		imports []string
	}
	discovered := make([]discoveredPackage, 0, len(packageDirs)+1)

	for _, path := range packageDirs {
		bpkg, err := bctx.ImportDir(path, 0)
		if err != nil {
			var noGoErr *build.NoGoError
			if errors.As(err, &noGoErr) {
				continue
			}
			return nil, fmt.Errorf("inspecting standard-library package at %q: %w", path, err)
		}

		if bpkg.Name == "main" {
			continue
		}

		rel, err := filepath.Rel(sdkRoot, path)
		if err != nil {
			return nil, fmt.Errorf("failed to compute relative path of %s from SDK root: %w", path, err)
		}
		importPath := filepath.ToSlash(rel)
		goFiles := append([]string{}, bpkg.GoFiles...)
		goFiles = append(goFiles, bpkg.CgoFiles...)
		sort.Strings(goFiles)

		imports := append([]string{}, bpkg.Imports...)
		sort.Strings(imports)
		discovered = append(discovered, discoveredPackage{
			pkg: &packages.Package{
				ID:              importPath,
				Name:            bpkg.Name,
				PkgPath:         importPath,
				GoFiles:         goFiles,
				CompiledGoFiles: append([]string{}, goFiles...),
				Imports:         make(map[string]*packages.Package),
			},
			imports: imports,
		})
	}

	// Older or reduced SDK source trees may omit unsafe even though it is a
	// compiler builtin. Keep one synthetic record only when the source tree did
	// not provide the real package; a complete SDK's unsafe.go is authoritative.
	unsafeFound := false
	for _, item := range discovered {
		if item.pkg.PkgPath == "unsafe" {
			unsafeFound = true
			break
		}
	}
	if !unsafeFound {
		discovered = append(discovered, discoveredPackage{pkg: &packages.Package{
			ID:      "unsafe",
			Name:    "unsafe",
			PkgPath: "unsafe",
			Imports: make(map[string]*packages.Package),
		}})
	}

	if len(discovered) <= 1 {
		return nil, fmt.Errorf("invalid SDK root %q: structurally invalid (no standard-library packages discovered)", sdkRoot)
	}

	known := make(map[string]bool, len(discovered))
	for _, item := range discovered {
		known[item.pkg.PkgPath] = true
	}
	for i := range discovered {
		for _, imp := range discovered[i].imports {
			// C is cgo's pseudo-package, not a package in the import graph.
			if imp == "C" {
				continue
			}
			resolved := imp
			if !IsStdlib(resolved) {
				vendorPath := "vendor/" + resolved
				if !known[resolved] && known[vendorPath] {
					resolved = vendorPath
				}
			}
			if IsStdlib(resolved) {
				discovered[i].pkg.Imports[resolved] = &packages.Package{ID: resolved}
			}
		}
	}

	pkgs := make([]*packages.Package, 0, len(discovered))
	for _, item := range discovered {
		pkgs = append(pkgs, item.pkg)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].ID < pkgs[j].ID
	})

	return pkgs, nil
}

// Parse parses a Layout from JSON data and requires EOF after the decoded layout to prevent trailing garbage.
func Parse(r io.Reader) (*Layout, error) {
	var l Layout
	dec := json.NewDecoder(r)
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("malformed layout JSON: %w", err)
	}
	if t, err := dec.Token(); err == nil {
		return nil, fmt.Errorf("trailing garbage after layout JSON: %v", t)
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trailing garbage after layout JSON: %w", err)
	}
	return &l, nil
}

// MarshalJSON serializes the Layout with deterministic canonical ordering of Roots and Packages
// without mutating the original caller-owned data.
func (l *Layout) MarshalJSON() ([]byte, error) {
	var roots []string
	if l.Roots != nil {
		roots = make([]string, len(l.Roots))
		copy(roots, l.Roots)
		sort.Strings(roots)
	} else {
		roots = []string{}
	}

	var pkgs []*packages.Package
	if l.Packages != nil {
		pkgs = make([]*packages.Package, len(l.Packages))
		copy(pkgs, l.Packages)
		sort.Slice(pkgs, func(i, j int) bool {
			return pkgs[i].ID < pkgs[j].ID
		})
	} else {
		pkgs = []*packages.Package{}
	}

	return json.Marshal(&struct {
		GoSDKRoot string              `json:"go_sdk_root"`
		Roots     []string            `json:"roots"`
		Packages  []*packages.Package `json:"packages"`
	}{
		GoSDKRoot: l.GoSDKRoot,
		Roots:     roots,
		Packages:  pkgs,
	})
}

// ValidateAndResolve validates the layout's structural consistency and resolves
// all workspace-relative and SDK-relative source paths, checking that each file exists.
func ValidateAndResolve(l *Layout, workspaceDir string) error {
	for idx, p := range l.Packages {
		if p == nil {
			return fmt.Errorf("package entry at index %d is null", idx)
		}
	}

	stdPkgIDs := make(map[string]bool)

	// First, discover and merge standard library packages if GoSDKRoot is set.
	if l.GoSDKRoot != "" {
		stdPkgs, err := discoverStdlib(l.GoSDKRoot)
		if err != nil {
			return fmt.Errorf("discovering standard library: %w", err)
		}
		for _, p := range stdPkgs {
			stdPkgIDs[p.ID] = true
			stdPkgIDs[p.PkgPath] = true
		}
		existing := make(map[string]bool)
		for _, p := range l.Packages {
			existing[p.ID] = true
			existing[p.PkgPath] = true
		}
		for _, stdPkg := range stdPkgs {
			if !existing[stdPkg.ID] && !existing[stdPkg.PkgPath] {
				l.Packages = append(l.Packages, stdPkg)
			}
		}
	}

	if l.GoSDKRoot == "" {
		// GoSDKRoot is required if standard library packages are present or referenced.
		for _, p := range l.Packages {
			if IsStdlib(p.PkgPath) {
				return errors.New("go_sdk_root is required when standard library packages are present in layout")
			}
			for impPath := range p.Imports {
				if IsStdlib(impPath) {
					return fmt.Errorf("go_sdk_root is required when standard library package %q is imported by %q", impPath, p.ID)
				}
			}
		}
	}

	if len(l.Roots) == 0 {
		return errors.New("roots list cannot be empty")
	}

	// Index packages by ID and PkgPath to detect duplicates and enable graph traversal.
	byID := make(map[string]*packages.Package, len(l.Packages))
	byPath := make(map[string]*packages.Package, len(l.Packages))

	for _, p := range l.Packages {
		if p.ID == "" {
			return errors.New("package has empty ID")
		}
		if p.PkgPath == "" {
			return fmt.Errorf("package %q has empty import path", p.ID)
		}
		if p.Name == "" {
			return fmt.Errorf("package %q has empty name", p.ID)
		}

		if _, ok := byID[p.ID]; ok {
			return fmt.Errorf("duplicate package ID: %s", p.ID)
		}
		if _, ok := byPath[p.PkgPath]; ok {
			return fmt.Errorf("duplicate package import path: %s", p.PkgPath)
		}

		byID[p.ID] = p
		byPath[p.PkgPath] = p
	}

	// Validate roots.
	for _, root := range l.Roots {
		if _, ok := byID[root]; !ok {
			if _, okPath := byPath[root]; !okPath {
				return fmt.Errorf("unknown root: %s", root)
			}
		}
	}

	// Phase 1: Resolve GoFiles and CompiledGoFiles for all packages.
	for _, p := range l.Packages {
		resolvedGoFiles, err := resolveAndCheckFiles(p, p.GoFiles, l.GoSDKRoot, workspaceDir)
		if err != nil {
			return err
		}
		p.GoFiles = resolvedGoFiles

		resolvedCompiledFiles, err := resolveAndCheckFiles(p, p.CompiledGoFiles, l.GoSDKRoot, workspaceDir)
		if err != nil {
			return err
		}
		p.CompiledGoFiles = resolvedCompiledFiles
	}

	// Phase 2: Recover standard library imports for non-stdlib packages.
	for _, p := range l.Packages {
		if !IsStdlib(p.PkgPath) {
			fset := token.NewFileSet()
			sortedFiles := make([]string, len(p.GoFiles))
			copy(sortedFiles, p.GoFiles)
			sort.Strings(sortedFiles)

			importMap := make(map[string]bool)
			for _, file := range sortedFiles {
				f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
				if err != nil {
					return fmt.Errorf("parsing source file %q of package %q: %w", file, p.ID, err)
				}
				for _, impSpec := range f.Imports {
					if impSpec.Path == nil {
						continue
					}
					impPath, err := strconv.Unquote(impSpec.Path.Value)
					if err != nil {
						return fmt.Errorf("invalid import literal %s in source file %q of package %q: %w", impSpec.Path.Value, file, p.ID, err)
					}
					importMap[impPath] = true
				}
			}

			var sortedImports []string
			for impPath := range importMap {
				sortedImports = append(sortedImports, impPath)
			}
			sort.Strings(sortedImports)

			for _, impPath := range sortedImports {
				if IsStdlib(impPath) && stdPkgIDs[impPath] {
					if p.Imports == nil {
						p.Imports = make(map[string]*packages.Package)
					}
					if _, exists := p.Imports[impPath]; !exists {
						p.Imports[impPath] = &packages.Package{ID: impPath}
					}
				}
			}
		}
	}

	// Phase 3: Validate imports for all packages.
	for _, p := range l.Packages {
		var impPaths []string
		for impPath := range p.Imports {
			impPaths = append(impPaths, impPath)
		}
		sort.Strings(impPaths)

		for _, impPath := range impPaths {
			impID := p.Imports[impPath]
			if impPath == "" {
				return fmt.Errorf("package %q has empty import path key in Imports map", p.ID)
			}
			if impID == nil {
				return fmt.Errorf("package %q imports nil package reference for path %q", p.ID, impPath)
			}
			target, ok := byID[impID.ID]
			if !ok {
				return fmt.Errorf("package %q imports unknown package ID %q", p.ID, impID.ID)
			}
			if target.PkgPath != impPath {
				return fmt.Errorf("package %q imports path %q with ID %q, but target package import path is %q", p.ID, impPath, target.ID, target.PkgPath)
			}
		}
	}

	return nil
}

func resolveAndCheckFiles(p *packages.Package, files []string, sdkRoot, workspaceDir string) ([]string, error) {
	resolved := make([]string, len(files))
	for i, f := range files {
		if f == "" {
			return nil, fmt.Errorf("package %q contains empty source file path", p.ID)
		}

		var path string
		if IsStdlib(p.PkgPath) {
			// Resolve relative to sdkRoot.
			// Standard library files can be mapped to sdkRoot/PkgPath/BaseName.
			path = filepath.Join(sdkRoot, p.PkgPath, filepath.Base(f))
		} else {
			// Resolve relative to workspaceDir.
			if filepath.IsAbs(f) {
				return nil, fmt.Errorf("package %q contains absolute source file path: %q", p.ID, f)
			}

			cleanedF := filepath.Clean(f)
			if strings.HasPrefix(cleanedF, ".."+string(filepath.Separator)) || cleanedF == ".." || strings.HasPrefix(cleanedF, "../") {
				return nil, fmt.Errorf("package %q contains source file path escaping workspace: %q", p.ID, f)
			}

			if workspaceDir != "" {
				path = filepath.Join(workspaceDir, cleanedF)
				rel, err := filepath.Rel(workspaceDir, path)
				if err != nil {
					return nil, fmt.Errorf("failed to compute relative path: %w", err)
				}
				if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || strings.HasPrefix(rel, "../") {
					return nil, fmt.Errorf("package %q contains source file path escaping workspace: %q", p.ID, f)
				}
			} else {
				path = cleanedF
			}
		}

		// Check file existence.
		if _, err := osStat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("source file %q for package %q does not exist", path, p.ID)
			}
			return nil, fmt.Errorf("error stating source file %q for package %q: %w", path, p.ID, err)
		}

		resolved[i] = path
	}
	return resolved, nil
}

// HandleDriverRequest processes a packages.DriverRequest using a validated Layout
// and the query patterns, returning a packages.DriverResponse.
func HandleDriverRequest(l *Layout, req *packages.DriverRequest, patterns []string) (*packages.DriverResponse, error) {
	byID := make(map[string]*packages.Package, len(l.Packages))
	byPath := make(map[string]*packages.Package, len(l.Packages))
	var stdIDs []string

	for _, p := range l.Packages {
		byID[p.ID] = p
		byPath[p.PkgPath] = p
		if IsStdlib(p.PkgPath) {
			stdIDs = append(stdIDs, p.ID)
		}
	}

	// Deterministic standard library order.
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
			// Match exact import path (PkgPath) or package ID.
			if p, ok := byPath[pat]; ok {
				addRoot(p.ID)
			} else if p, ok := byID[pat]; ok {
				addRoot(p.ID)
			} else {
				return nil, fmt.Errorf("no layout package found for pattern %q", pat)
			}
		}
	}

	// Always sort roots for determinism.
	sort.Strings(roots)

	// Ensure the response Packages list is also deterministic by sorting by ID.
	pkgsCopy := make([]*packages.Package, len(l.Packages))
	copy(pkgsCopy, l.Packages)
	sort.Slice(pkgsCopy, func(i, j int) bool {
		return pkgsCopy[i].ID < pkgsCopy[j].ID
	})

	resp := &packages.DriverResponse{
		Compiler: "gc",
		Arch:     runtime.GOARCH,
		Roots:    roots,
		Packages: pkgsCopy,
	}
	return resp, nil
}

// RunDriver executes the complete GOPACKAGESDRIVER protocol logic. It reads
// the DriverRequest from stdin, resolves patterns from args, reads the layout
// file, validates and resolves paths, processes the query, and writes the
// JSON response to stdout.
func RunDriver(layoutPath string, workspaceDir string, patterns []string, stdin io.Reader, stdout io.Writer) error {
	// Read and parse DriverRequest.
	var req packages.DriverRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			return fmt.Errorf("parsing DriverRequest: %w", err)
		}
	}

	if layoutPath == "" {
		return errors.New("package-layout file path is required")
	}

	f, err := os.Open(layoutPath)
	if err != nil {
		return fmt.Errorf("opening package-layout: %w", err)
	}
	defer f.Close()

	layout, err := Parse(f)
	if err != nil {
		return err
	}

	if err := ValidateAndResolve(layout, workspaceDir); err != nil {
		return fmt.Errorf("validating layout: %w", err)
	}

	resp, err := HandleDriverRequest(layout, &req, patterns)
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

var (
	envMu              sync.Mutex
	activeMu           sync.RWMutex
	activeLayout       *Layout
	activeLayoutPath   string
	activeWorkspaceDir string
	// CheckMu is a global mutex to serialize all concurrent check executions
	// in the same process, protecting against environment and global state races.
	CheckMu sync.Mutex
)

// IsLayoutMode reports whether package-layout mode is currently active.
func IsLayoutMode() bool {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeLayout != nil
}

// GetActiveLayout returns the active package layout, or nil if not in layout mode.
func GetActiveLayout() *Layout {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeLayout
}

// GetActiveLayoutPath returns the path to the active package layout file.
func GetActiveLayoutPath() string {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeLayoutPath
}

// GetActiveWorkspaceDir returns the active workspace directory.
func GetActiveWorkspaceDir() string {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeWorkspaceDir
}

// WithTemporaryLayout temporarily overrides the active layout and environment variables
// for dependency resolution, and restores them afterwards.
func WithTemporaryLayout(layoutPath string, fn func() error) error {
	activeMu.Lock()
	if activeLayout == nil {
		activeMu.Unlock()
		return errors.New("not in layout mode")
	}

	f, err := os.Open(layoutPath)
	if err != nil {
		activeMu.Unlock()
		return fmt.Errorf("opening dependency layout file: %w", err)
	}
	defer f.Close()

	layout, err := Parse(f)
	if err != nil {
		activeMu.Unlock()
		return fmt.Errorf("parsing dependency layout file: %w", err)
	}

	if err := ValidateAndResolve(layout, activeWorkspaceDir); err != nil {
		activeMu.Unlock()
		return fmt.Errorf("validating dependency layout file: %w", err)
	}

	// Capture original layout state and env
	origLayoutPath := activeLayoutPath
	origLayout := activeLayout
	origEnvLayout, hasEnvLayout := os.LookupEnv("ARCC_PACKAGE_LAYOUT")

	// Set temporary layout
	activeLayoutPath = layoutPath
	activeLayout = layout
	os.Setenv("ARCC_PACKAGE_LAYOUT", layoutPath)

	activeMu.Unlock()

	// Run callback
	err = fn()

	activeMu.Lock()
	// Restore original layout state and env
	activeLayoutPath = origLayoutPath
	activeLayout = origLayout
	if !hasEnvLayout {
		os.Unsetenv("ARCC_PACKAGE_LAYOUT")
	} else {
		os.Setenv("ARCC_PACKAGE_LAYOUT", origEnvLayout)
	}
	activeMu.Unlock()

	return err
}

// WithDriverEnv configures GOPACKAGESDRIVER to point to the current executable
// and loads the layout file, caching it and making it available in layout mode.
// It restores the original environment and variables when fn returns.
func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error {
	envMu.Lock()
	defer envMu.Unlock()

	f, err := os.Open(layoutPath)
	if err != nil {
		return fmt.Errorf("opening layout file: %w", err)
	}
	defer f.Close()

	layout, err := Parse(f)
	if err != nil {
		return fmt.Errorf("parsing layout file: %w", err)
	}

	if err := ValidateAndResolve(layout, workspaceDir); err != nil {
		return fmt.Errorf("validating layout file: %w", err)
	}

	// Capture original environment presence and values
	origDriver, hasDriver := os.LookupEnv("GOPACKAGESDRIVER")
	origLayout, hasLayout := os.LookupEnv("ARCC_PACKAGE_LAYOUT")
	origWorkspace, hasWorkspace := os.LookupEnv("ARCC_WORKSPACE_DIR")
	origDriverMode, hasDriverMode := os.LookupEnv("ARCC_DRIVER_MODE")

	activeMu.Lock()
	origActiveLayout := activeLayout
	origActiveLayoutPath := activeLayoutPath
	origActiveWorkspaceDir := activeWorkspaceDir

	executable, err := os.Executable()
	if err != nil {
		activeMu.Unlock()
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Set environment variables
	os.Setenv("GOPACKAGESDRIVER", executable)
	os.Setenv("ARCC_PACKAGE_LAYOUT", layoutPath)
	os.Setenv("ARCC_WORKSPACE_DIR", workspaceDir)
	os.Setenv("ARCC_DRIVER_MODE", "1")

	activeLayout = layout
	activeLayoutPath = layoutPath
	activeWorkspaceDir = workspaceDir
	activeMu.Unlock()

	// Run the callback
	err = fn()

	activeMu.Lock()
	// Restore original environment and variables
	if !hasDriver {
		os.Unsetenv("GOPACKAGESDRIVER")
	} else {
		os.Setenv("GOPACKAGESDRIVER", origDriver)
	}

	if !hasLayout {
		os.Unsetenv("ARCC_PACKAGE_LAYOUT")
	} else {
		os.Setenv("ARCC_PACKAGE_LAYOUT", origLayout)
	}

	if !hasWorkspace {
		os.Unsetenv("ARCC_WORKSPACE_DIR")
	} else {
		os.Setenv("ARCC_WORKSPACE_DIR", origWorkspace)
	}

	if !hasDriverMode {
		os.Unsetenv("ARCC_DRIVER_MODE")
	} else {
		os.Setenv("ARCC_DRIVER_MODE", origDriverMode)
	}

	activeLayout = origActiveLayout
	activeLayoutPath = origActiveLayoutPath
	activeWorkspaceDir = origActiveWorkspaceDir
	activeMu.Unlock()

	return err
}

// init serves the GOPACKAGESDRIVER self-exec protocol when this process was
// spawned as the driver, then exits without returning to the caller.
//
// The dispatch is deliberately an init rather than a call from cmd/arcc's main.
// WithDriverEnv points GOPACKAGESDRIVER at os.Executable(), so the driver
// subprocess is whatever binary ran the check — and for the in-process
// layout-mode integration tests that binary is the `go test` binary, whose main
// belongs to the testing framework. An init is the only hook that runs before
// it in both cases.
//
// Selection is keyed solely on the private ARCC_DRIVER_MODE marker, which
// WithDriverEnv sets and restores, so driver mode can never be reached through
// a user-facing subcommand. No blank import is needed to keep this alive:
// cmd/arcc/app depends on this package directly.
func init() {
	if os.Getenv("ARCC_DRIVER_MODE") == "1" {
		layoutPath := os.Getenv("ARCC_PACKAGE_LAYOUT")
		workspaceDir := os.Getenv("ARCC_WORKSPACE_DIR")
		patterns := os.Args[1:]
		if err := RunDriver(layoutPath, workspaceDir, patterns, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "GOPACKAGESDRIVER error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}
