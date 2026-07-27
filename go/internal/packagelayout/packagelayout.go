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

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"golang.org/x/tools/go/packages"
)

// osStat is a seam for mocking file existence in unit tests.
var osStat = os.Stat

// Layout is the package layout schema representing a Bazel-produced package graph.
type Layout struct {
	GoSDKRoot string              `json:"go_sdk_root"`
	Platform  *Platform           `json:"platform,omitempty"`
	Roots     []string            `json:"roots"`
	Packages  []*packages.Package `json:"packages"`

	// The layout wire schema extends each emitter-listed package with an
	// `is_stdlib` boolean. It records build-graph provenance (SDK/toolchain
	// versus enumerated target); emitters must not derive it by re-running an
	// import-path heuristic. Upstream packages.Package cannot retain this field,
	// so validation-owned metadata keeps it beside the decoded package graph.
	stdlibByID       map[string]bool
	stdlibByPath     map[string]bool
	emittedPackageID map[string]bool

	// UnresolvedImports contains imports observed in surviving source files that
	// do not resolve to a layout or SDK package. It is loader-owned state and is
	// deliberately excluded from the layout JSON schema.
	UnresolvedImports []UnresolvedImport `json:"-"`
	importsOmitted    map[string]bool
}

// UnresolvedImport records a surviving source-level import for which the
// layout has no package node. The source file is absolute after validation.
type UnresolvedImport struct {
	Package    string
	SourceFile string
	ImportPath string
}

// Platform declares the target used when evaluating Go build constraints.
//
// When present, goos and goarch are required target values, build_tags are the
// user-supplied build tags, and cgo_enabled controls the cgo build constraint.
// When absent, validation uses a copy of build.Default for compatibility with
// hand-written layouts. The remaining build.Context defaults, including GOROOT,
// compiler, tool tags, and release tags, are retained in either case.
type Platform struct {
	// GOOS is the target operating system, such as linux or windows.
	GOOS string `json:"goos"`
	// GOARCH is the target architecture, such as amd64 or arm64.
	GOARCH string `json:"goarch"`
	// BuildTags are user-supplied build tags enabled for this target.
	BuildTags []string `json:"build_tags"`
	// CgoEnabled controls whether files guarded by the cgo tag are selected.
	CgoEnabled bool `json:"cgo_enabled"`
}

// IsStdlib reports whether importPath is a standard-library package. It defers to
// the host stdlib policy, whose default is the heuristic the go tool uses (the
// first path segment contains no dot). Package-layout mode has no module metadata,
// so a host that rewrites import paths into a dotless-first-segment namespace
// overrides hostpolicy.IsStdlibPath to keep those paths from being misclassified.
func IsStdlib(importPath string) bool {
	return hostpolicy.IsStdlibPath(importPath)
}

// IsStdlibPackage reports the validated standard-library provenance for a
// package in layout mode. Missing provenance is non-stdlib (fail closed).
func (l *Layout) IsStdlibPackage(p *packages.Package) bool {
	if l == nil || p == nil {
		return false
	}
	if l.stdlibByID != nil {
		if verdict, ok := l.stdlibByID[p.ID]; ok {
			return verdict
		}
	}
	if l.stdlibByPath != nil {
		if verdict, ok := l.stdlibByPath[p.PkgPath]; ok {
			return verdict
		}
	}
	return false
}

// IsStdlibPackage reports the validated provenance of the active layout
// package. It is intentionally not a path heuristic.
func IsStdlibPackage(p *packages.Package) bool {
	activeMu.RLock()
	l := activeLayout
	activeMu.RUnlock()
	return l.IsStdlibPackage(p)
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

// supportedGoTargets mirrors the Go 1.26 target list reported by
// `go tool dist list`, the supported-target source for this module's Go
// toolchain. Keep this list synchronized when the module's Go version changes;
// unlike go/build's internal tag tables, it excludes retired and broken ports.
var supportedGoTargets = [...]string{
	"aix/ppc64",
	"android/386", "android/amd64", "android/arm", "android/arm64",
	"darwin/amd64", "darwin/arm64",
	"dragonfly/amd64",
	"freebsd/386", "freebsd/amd64", "freebsd/arm", "freebsd/arm64",
	"illumos/amd64",
	"ios/amd64", "ios/arm64",
	"js/wasm",
	"linux/386", "linux/amd64", "linux/arm", "linux/arm64", "linux/loong64",
	"linux/mips", "linux/mips64", "linux/mips64le", "linux/mipsle",
	"linux/ppc64", "linux/ppc64le", "linux/riscv64", "linux/s390x",
	"netbsd/386", "netbsd/amd64", "netbsd/arm", "netbsd/arm64",
	"openbsd/386", "openbsd/amd64", "openbsd/arm", "openbsd/arm64",
	"openbsd/ppc64", "openbsd/riscv64",
	"plan9/386", "plan9/amd64", "plan9/arm",
	"solaris/amd64",
	"wasip1/wasm",
	"windows/386", "windows/amd64", "windows/arm64",
}

func supportedGOOS(goos string) bool {
	for _, target := range supportedGoTargets {
		if targetOS, _, _ := strings.Cut(target, "/"); targetOS == goos {
			return true
		}
	}
	return false
}

func supportedGOARCH(goarch string) bool {
	for _, target := range supportedGoTargets {
		_, targetArch, _ := strings.Cut(target, "/")
		if targetArch == goarch {
			return true
		}
	}
	return false
}

func supportedGoTarget(goos, goarch string) bool {
	target := goos + "/" + goarch
	for _, supported := range supportedGoTargets {
		if supported == target {
			return true
		}
	}
	return false
}

// BuildContextForLayout returns the build context declared by l. It always
// starts with a copy of build.Default, so deriving a context never changes the
// process-wide defaults or shares the layout's build-tags slice with them.
func BuildContextForLayout(l *Layout) (build.Context, error) {
	bctx := build.Default
	bctx.BuildTags = append([]string(nil), build.Default.BuildTags...)
	bctx.ToolTags = append([]string(nil), build.Default.ToolTags...)
	bctx.ReleaseTags = append([]string(nil), build.Default.ReleaseTags...)
	if l == nil || l.Platform == nil {
		return bctx, nil
	}

	platform := l.Platform
	if !supportedGOOS(platform.GOOS) {
		return build.Context{}, fmt.Errorf("platform.goos has invalid value %q", platform.GOOS)
	}
	if !supportedGOARCH(platform.GOARCH) {
		return build.Context{}, fmt.Errorf("platform.goarch has invalid value %q", platform.GOARCH)
	}
	if !supportedGoTarget(platform.GOOS, platform.GOARCH) {
		return build.Context{}, fmt.Errorf("platform.goarch has unsupported value %q for goos %q", platform.GOARCH, platform.GOOS)
	}
	bctx.GOOS = platform.GOOS
	bctx.GOARCH = platform.GOARCH
	bctx.BuildTags = append([]string(nil), platform.BuildTags...)
	bctx.CgoEnabled = platform.CgoEnabled
	return bctx, nil
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
			if !known[resolved] {
				vendorPath := "vendor/" + resolved
				if known[vendorPath] {
					resolved = vendorPath
				}
			}
			if known[resolved] {
				// Key by the import path as written in source (imp), not the
				// resolved target: go/types resolves an import by the path in the
				// source, even when it is satisfied by a vendored copy. The target
				// package ID is the resolved (possibly vendor/-prefixed) path.
				discovered[i].pkg.Imports[imp] = &packages.Package{ID: resolved}
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
// without mutating the original caller-owned data. The layout-only `is_stdlib`
// field is emitted for emitter-listed packages and carries build-graph
// provenance, never a duplicated import-path heuristic.
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

	packageJSON := make([]json.RawMessage, len(pkgs))
	for i, p := range pkgs {
		if p == nil {
			packageJSON[i] = json.RawMessage("null")
			continue
		}
		raw, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("marshalling package %q: %w", p.ID, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("marshalling package %q: %w", p.ID, err)
		}
		emitted := l.emittedPackageID == nil || l.emittedPackageID[p.ID]
		if emitted {
			fields["is_stdlib"] = json.RawMessage("false")
			if l.stdlibByID != nil && l.stdlibByID[p.ID] {
				fields["is_stdlib"] = json.RawMessage("true")
			}
		}
		packageJSON[i], err = json.Marshal(fields)
		if err != nil {
			return nil, fmt.Errorf("marshalling package %q: %w", p.ID, err)
		}
	}

	return json.Marshal(&struct {
		GoSDKRoot string            `json:"go_sdk_root"`
		Platform  *Platform         `json:"platform,omitempty"`
		Roots     []string          `json:"roots"`
		Packages  []json.RawMessage `json:"packages"`
	}{
		GoSDKRoot: l.GoSDKRoot,
		Platform:  l.Platform,
		Roots:     roots,
		Packages:  packageJSON,
	})
}

// UnmarshalJSON decodes the upstream go/packages package shape and retains the
// layout-only is_stdlib side metadata. Missing is_stdlib intentionally decodes
// as false for JSON syntax compatibility; validation rejects that false value
// when the host path policy identifies an explicitly listed package as stdlib.
func (l *Layout) UnmarshalJSON(data []byte) error {
	var wire struct {
		GoSDKRoot string            `json:"go_sdk_root"`
		Platform  *Platform         `json:"platform,omitempty"`
		Roots     []string          `json:"roots"`
		Packages  []json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	packagesList := make([]*packages.Package, len(wire.Packages))
	stdlibByID := make(map[string]bool, len(wire.Packages))
	emittedPackageID := make(map[string]bool, len(wire.Packages))
	for i, raw := range wire.Packages {
		if string(raw) == "null" {
			continue
		}
		var p packages.Package
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("decoding package at index %d: %w", i, err)
		}
		var metadata struct {
			IsStdlib bool `json:"is_stdlib"`
		}
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return fmt.Errorf("decoding package %q provenance: %w", p.ID, err)
		}
		packagesList[i] = &p
		stdlibByID[p.ID] = metadata.IsStdlib
		emittedPackageID[p.ID] = true
	}

	l.GoSDKRoot = wire.GoSDKRoot
	l.Platform = wire.Platform
	l.Roots = wire.Roots
	l.Packages = packagesList
	l.stdlibByID = stdlibByID
	l.stdlibByPath = make(map[string]bool, len(stdlibByID))
	l.emittedPackageID = emittedPackageID
	l.UnresolvedImports = nil
	l.importsOmitted = nil
	return nil
}

// ValidateAndResolve validates the layout's structural consistency and resolves
// all workspace-relative and SDK-relative source paths, checking that each file exists.
func ValidateAndResolve(l *Layout, workspaceDir string) error {
	bctx, err := BuildContextForLayout(l)
	if err != nil {
		return err
	}
	l.UnresolvedImports = nil
	for idx, p := range l.Packages {
		if p == nil {
			return fmt.Errorf("package entry at index %d is null", idx)
		}
	}
	if l.stdlibByID == nil {
		l.stdlibByID = make(map[string]bool, len(l.Packages))
	}
	if l.stdlibByPath == nil {
		l.stdlibByPath = make(map[string]bool, len(l.Packages))
	}
	if l.emittedPackageID == nil {
		l.emittedPackageID = make(map[string]bool, len(l.Packages))
		for _, p := range l.Packages {
			l.emittedPackageID[p.ID] = true
		}
	}
	// Validate emitter provenance before SDK discovery can add structural
	// packages. An omitted is_stdlib field is the false value and therefore
	// deliberately disagrees with a policy that identifies the explicit package
	// as standard library.
	for _, p := range l.Packages {
		if !l.emittedPackageID[p.ID] {
			continue
		}
		declared := l.stdlibByID[p.ID]
		policy := hostpolicy.IsStdlibPath(p.PkgPath)
		if declared != policy {
			return fmt.Errorf("package %q standard-library provenance disagreement: declared %t, path-policy %t", p.PkgPath, declared, policy)
		}
		l.stdlibByPath[p.PkgPath] = declared
	}

	// First, discover and merge standard library packages if GoSDKRoot is set.
	if l.GoSDKRoot != "" {
		stdlibContext := bctx
		stdlibContext.GOROOT = filepath.Dir(l.GoSDKRoot)
		stdPkgs, err := discoverStdlibWithContext(l.GoSDKRoot, stdlibContext)
		if err != nil {
			return fmt.Errorf("discovering standard library: %w", err)
		}
		existing := make(map[string]bool)
		for _, p := range l.Packages {
			existing[p.ID] = true
			existing[p.PkgPath] = true
		}
		for _, stdPkg := range stdPkgs {
			if !existing[stdPkg.ID] && !existing[stdPkg.PkgPath] {
				l.Packages = append(l.Packages, stdPkg)
				l.stdlibByID[stdPkg.ID] = true
				l.stdlibByPath[stdPkg.PkgPath] = true
				l.emittedPackageID[stdPkg.ID] = false
			}
		}
	}

	if l.GoSDKRoot == "" {
		// GoSDKRoot is required if standard library packages are present or referenced.
		for _, p := range l.Packages {
			if l.IsStdlibPackage(p) {
				return errors.New("go_sdk_root is required when standard library packages are present in layout")
			}
			for impPath := range p.Imports {
				if hostpolicy.IsStdlibPath(impPath) {
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
	if l.importsOmitted == nil {
		l.importsOmitted = make(map[string]bool)
	}

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
		if _, recorded := l.importsOmitted[p.ID]; !recorded {
			l.importsOmitted[p.ID] = p.Imports == nil
		}
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
		resolvedGoFiles, err := l.resolveAndCheckFiles(p, p.GoFiles, l.GoSDKRoot, workspaceDir)
		if err != nil {
			return err
		}
		p.GoFiles = resolvedGoFiles

		resolvedCompiledFiles, err := l.resolveAndCheckFiles(p, p.CompiledGoFiles, l.GoSDKRoot, workspaceDir)
		if err != nil {
			return err
		}
		p.CompiledGoFiles = resolvedCompiledFiles
	}

	// Phase 1b: Drop sources excluded by build constraints for the target
	// platform. A host may hand arcc the full declared source set rather than the
	// per-platform compiled subset — rules_go's GoInfo.srcs, for one, lists every
	// _GOOS.go variant (its compiler, not the provider, applies constraints).
	// Type-checking wrong-platform files makes the package IllTyped, which silently
	// degrades the whole analysis, so arcc filters here the way the go tool does
	// when it reads a package directory. Standard-library packages arrive already
	// filtered from discoverStdlib and are left untouched.
	for _, p := range l.Packages {
		if l.IsStdlibPackage(p) {
			continue
		}
		hadGoSources := hasGoSources(p.GoFiles) || hasGoSources(p.CompiledGoFiles)
		p.GoFiles = filterByBuildConstraintsWithContext(p.GoFiles, bctx)
		p.CompiledGoFiles = filterByBuildConstraintsWithContext(p.CompiledGoFiles, bctx)
		if hadGoSources && !hasGoSources(p.GoFiles) && !hasGoSources(p.CompiledGoFiles) {
			return fmt.Errorf("package %q has no Go sources after the declared platform excluded every Go source", p.PkgPath)
		}
	}

	// Phase 2: Recover imports from surviving non-stdlib sources. The source
	// scan happens before mutating omitted import maps so T8 can distinguish the
	// two emitter shapes and retain file provenance for unresolved imports.
	resolveImport := func(importPath string) (*packages.Package, bool) {
		if target, ok := byPath[importPath]; ok {
			return target, true
		}
		// A standard-library package may be discovered under vendor/ while its
		// source-level import remains the bare path. The target's path is the
		// authoritative indication here because the source-level path may be a
		// dotted vendored path.
		if target, ok := byPath["vendor/"+importPath]; ok && l.IsStdlibPackage(target) {
			return target, true
		}
		return nil, false
	}

	var unresolvedImports []UnresolvedImport
	for _, p := range l.Packages {
		if !l.IsStdlibPackage(p) {
			fset := token.NewFileSet()
			sortedFiles := SurvivingSourceFiles(p)

			importSources := make(map[string][]string)
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
					if impPath == "C" {
						// C is cgo's pseudo-package, not a graph node.
						continue
					}
					importSources[impPath] = append(importSources[impPath], file)
				}
			}

			var sortedImports []string
			for impPath := range importSources {
				sortedImports = append(sortedImports, impPath)
			}
			sort.Strings(sortedImports)

			for _, impPath := range sortedImports {
				sources := importSources[impPath]
				if target, resolvable := resolveImport(impPath); resolvable {
					if l.importsOmitted[p.ID] {
						if p.Imports == nil {
							p.Imports = make(map[string]*packages.Package)
						}
						if _, exists := p.Imports[impPath]; !exists {
							p.Imports[impPath] = &packages.Package{ID: target.ID}
						}
					}
				} else {
					for _, file := range sources {
						unresolvedImports = append(unresolvedImports, UnresolvedImport{
							Package:    p.PkgPath,
							SourceFile: file,
							ImportPath: impPath,
						})
					}
				}

			}

			if !l.importsOmitted[p.ID] {
				declared := make(map[string]bool)
				for impPath := range p.Imports {
					if impPath == "C" {
						continue
					}
					if _, resolvable := resolveImport(impPath); resolvable {
						declared[impPath] = true
					}
				}
				var extra []string
				for impPath := range declared {
					if _, contributed := importSources[impPath]; !contributed {
						extra = append(extra, impPath)
					}
				}
				var missing []string
				for impPath, sources := range importSources {
					if _, resolvable := resolveImport(impPath); !resolvable || declared[impPath] {
						continue
					}
					for _, file := range sources {
						missing = append(missing, fmt.Sprintf("%q from source file %q", impPath, file))
					}
				}
				sort.Strings(extra)
				sort.Strings(missing)
				if len(extra) > 0 {
					return fmt.Errorf("package %q declares resolvable imports not contributed by surviving sources: %s", p.PkgPath, strings.Join(extra, ", "))
				}
				if len(missing) > 0 {
					return fmt.Errorf("package %q has resolvable imports absent from declared Imports: %s", p.PkgPath, strings.Join(missing, "; "))
				}
			}
		}
	}
	sort.Slice(unresolvedImports, func(i, j int) bool {
		if unresolvedImports[i].Package != unresolvedImports[j].Package {
			return unresolvedImports[i].Package < unresolvedImports[j].Package
		}
		if unresolvedImports[i].SourceFile != unresolvedImports[j].SourceFile {
			return unresolvedImports[i].SourceFile < unresolvedImports[j].SourceFile
		}
		return unresolvedImports[i].ImportPath < unresolvedImports[j].ImportPath
	})
	uniqueUnresolved := unresolvedImports[:0]
	for _, observation := range unresolvedImports {
		if len(uniqueUnresolved) == 0 || uniqueUnresolved[len(uniqueUnresolved)-1] != observation {
			uniqueUnresolved = append(uniqueUnresolved, observation)
		}
	}
	l.UnresolvedImports = uniqueUnresolved

	// Phase 3: Validate imports for all packages.
	for _, p := range l.Packages {
		var impPaths []string
		for impPath := range p.Imports {
			impPaths = append(impPaths, impPath)
		}
		sort.Strings(impPaths)

		for _, impPath := range impPaths {
			if impPath == "C" {
				// C is cgo's pseudo-package, not a package in the graph.
				continue
			}
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
			// A standard-library package may import a vendored package by its bare
			// path; the resolving target's PkgPath is then the vendor/-prefixed
			// form (see discoverStdlib), so accept that as well.
			if target.PkgPath != impPath && (target.PkgPath != "vendor/"+impPath || !l.IsStdlibPackage(target)) {
				return fmt.Errorf("package %q imports path %q with ID %q, but target package import path is %q", p.ID, impPath, target.ID, target.PkgPath)
			}
		}
	}

	// Roots are the packages whose source is owned by the component. A root
	// without any source left after normalization and constraint filtering
	// cannot be analyzed, so reject it while leaving bodiless transitive
	// packages available for later reporting.
	rootNames := append([]string{}, l.Roots...)
	sort.Strings(rootNames)
	for _, root := range rootNames {
		rootPkg, ok := byID[root]
		if !ok {
			rootPkg = byPath[root]
		}
		if len(SurvivingSourceFiles(rootPkg)) == 0 {
			return fmt.Errorf("package %q has no source files", rootPkg.PkgPath)
		}
	}

	return nil
}

// filterByBuildConstraints keeps only the .go sources compiled for the current
// target platform, honoring filename suffixes (_windows.go, _amd64.go, ...) and
// //go:build / // +build lines — the same rules the go tool applies when reading
// a package directory. The context reflects the declared target platform
// rather than the platform for which the arcc binary was compiled.
//
// Filtering is a safety net, not a gate: a file whose constraints cannot be
// evaluated (e.g. it is not present on disk, as when a unit test mocks file
// existence) is kept rather than dropped, so this never removes a file it failed
// to read. Non-.go entries are passed through unchanged.
func filterByBuildConstraints(files []string) []string {
	return filterByBuildConstraintsWithContext(files, build.Default)
}

func filterByBuildConstraintsWithContext(files []string, bctx build.Context) []string {
	kept := make([]string, 0, len(files))
	for _, f := range files {
		if FileMatchesBuildConstraintsWithContext(f, bctx) {
			kept = append(kept, f)
		}
	}
	return kept
}

// FileMatchesBuildConstraints reports whether the .go file at path is compiled
// for the current target platform, honoring filename suffixes (_windows.go,
// _amd64.go, ...) and //go:build / // +build lines — the same rule
// filterByBuildConstraints applies per file. Non-.go paths, and files whose
// constraints cannot be evaluated (e.g. unreadable), are reported as matching,
// so this never reports a file as excluded merely because it failed to read it.
func FileMatchesBuildConstraints(path string) bool {
	return FileMatchesBuildConstraintsWithContext(path, build.Default)
}

// FileMatchesBuildConstraintsWithContext is FileMatchesBuildConstraints for an
// explicit analysis context. Non-Go paths, and files whose constraints cannot
// be evaluated because they cannot be read, are reported as matching.
func FileMatchesBuildConstraintsWithContext(path string, bctx build.Context) bool {
	if !strings.HasSuffix(path, ".go") {
		return true
	}
	match, err := bctx.MatchFile(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		return true
	}
	return match
}

func hasGoSources(files []string) bool {
	for _, file := range files {
		if strings.HasSuffix(file, ".go") {
			return true
		}
	}
	return false
}

// SurvivingSourceFiles returns the deterministic, de-duplicated union of the
// source fields that the packages driver may provide. Some providers expose a
// source only through CompiledGoFiles, so import recovery and root validation
// must consider both fields after platform filtering.
func SurvivingSourceFiles(p *packages.Package) []string {
	seen := make(map[string]bool, len(p.GoFiles)+len(p.CompiledGoFiles))
	files := make([]string, 0, len(p.GoFiles)+len(p.CompiledGoFiles))
	for _, file := range append(append([]string{}, p.GoFiles...), p.CompiledGoFiles...) {
		if !seen[file] {
			seen[file] = true
			files = append(files, file)
		}
	}
	sort.Strings(files)
	return files
}

func (l *Layout) resolveAndCheckFiles(p *packages.Package, files []string, sdkRoot, workspaceDir string) ([]string, error) {
	resolved := make([]string, len(files))
	for i, f := range files {
		if f == "" {
			return nil, fmt.Errorf("package %q contains empty source file path", p.ID)
		}

		var path string
		if l.IsStdlibPackage(p) {
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
		if l.IsStdlibPackage(p) {
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
