// Package packagelayout defines the schema, validation, and driver protocol
// for arcc's hermetic package-layout loader.
//
// Component Contract (FR10):
//   - What it does: Defines the layout schema (whole package graph, target
//     platform block with goos/goarch/build_tags/cgo_enabled and the pinned
//     toolchain_version/goexperiment), validates and resolves it (build
//     constraints for the declared target, source existence, transitively
//     complete import graph, GOROOT vendor resolution), discovers the
//     standard library from an SDK root exactly as `go list std` does
//     (discoverStdlib tree walk, no-Go source-less nodes), computes a
//     validated whole-stdlib layout for a pinned target (StdlibLayout), and
//     serves validated layouts through the GOPACKAGESDRIVER self-exec
//     protocol (HandleDriverRequest/RunDriver, with the target GOARCH as the
//     response Arch).
//   - What it requires: A validated layout file (or SDK root plus platform
//     for StdlibLayout), a workspace directory for emitter-listed sources,
//     and the ARCC_PACKAGE_LAYOUT/ARCC_DRIVER_MODE environment markers set
//     by WithDriverEnv/WithTemporaryLayout when invoked as the driver
//     subprocess.
//   - What it provides: Layout/Platform and direct dependency artifact bindings,
//     Parse, MarshalJSON/UnmarshalJSON,
//     ValidateAndResolve, BuildContextForLayout (target-derived release and
//     tool tags), FileMatchesBuildConstraints(Context), SurvivingSourceFiles,
//     discoverStdlibWithContext, StdlibLayout, IsStdlibPackage,
//     HandleDriverRequest, RunDriver, WithTemporaryLayout, WithDriverEnv,
//     IsLayoutMode/GetActiveLayout, and CheckMu.
//   - Ambient Authority: This is a shell component serving package loading.
//     It holds FILES (reads the layout file and every declared SDK source
//     file — the layout builder reads only the SDK root), EXEC (the driver
//     subprocess re-executes arcc's own binary; no toolchain binary), READ_
//     SYSTEM_STATE (walking the SDK tree and reading build constraint
//     headers), OPERATING_SYSTEM and MODIFY_SYSTEM_STATE/ENV (WithDriverEnv/
//     WithTemporaryLayout set and restore process environment variables), and
//     REFLECT/RUNTIME (go/packages and go/types type loading). No global
//     logging or network access. Explicit-input consumers therefore add no
//     EXEC beyond arcc's own self-exec driver (design I5).
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
	"path"
	"path/filepath"
	"runtime"
	"slices"
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
	Platform  *Platform           `json:"platform,omitempty"`
	Roots     []string            `json:"roots"`
	Packages  []*packages.Package `json:"packages"`
	// DependencyArtifactBindings associate every direct component dependency
	// with the artifacts its provider published. The paths use the same
	// runfiles frame as the manifest and package layout; this is structural
	// build metadata, not a verdict, authority, or freshness claim.
	DependencyArtifactBindings []DependencyArtifactBinding `json:"dependency_artifact_bindings,omitempty"`

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

// DependencyArtifactProvenance identifies how a dependency's artifact pair was
// produced in the build graph. It deliberately has no authority or verdict
// semantics: those are decoded by the later surface consumer from the
// artifacts and their report.
type DependencyArtifactProvenance string

const (
	DependencyArtifactProvenanceChecked  DependencyArtifactProvenance = "checked"
	DependencyArtifactProvenanceAsserted DependencyArtifactProvenance = "asserted"
)

// DependencyArtifactBinding binds one manifest dependency name to the surface
// and optional report files published by its ArccComponentInfo provider.
// Surface and report are runfiles-frame paths, not filesystem paths. A checked
// provider must publish both files; an asserted provider publishes only the
// package-level surface. This record carries no semantic trust claim.
type DependencyArtifactBinding struct {
	Dependency   string                       `json:"dependency"`
	Surface      string                       `json:"surface"`
	Report       string                       `json:"report,omitempty"`
	AutoAttached bool                         `json:"auto_attached"`
	Provenance   DependencyArtifactProvenance `json:"provenance"`
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
	// ToolchainVersion is the pinned toolchain's version in the `go1.N.M`
	// spelling. A non-nil pointer marks the layout as pinned: the build
	// context's release tags derive from it (go1.1 … go1.N) instead of being
	// copied from build.Default, so constraints describe the pinned target
	// rather than the binary that produced the layout. Absent (nil) leaves
	// the default behaviour.
	ToolchainVersion *string `json:"toolchain_version,omitempty"`
	// GOEXPERIMENT is the target's enabled Go experiments, comma-separated,
	// as the toolchain's GOEXPERIMENT setting spells them. A non-nil pointer
	// marks the layout as pinned: the build context's tool tags derive
	// `goexperiment.<name>` entries from it (plus the baseline experiments
	// and the target GOARCH's default arch feature tags) instead of copying
	// build.Default. Absent (nil) leaves the default behaviour.
	GOEXPERIMENT *string `json:"goexperiment,omitempty"`
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

	// A pinned platform describes a target toolchain, so the tool-derived
	// tags derive from the target instead of the binary that produced the
	// layout. A platform block that names neither optional field keeps the
	// legacy behaviour exactly (host-default release and tool tags).
	pinned := platform.ToolchainVersion != nil || platform.GOEXPERIMENT != nil
	if pinned {
		if platform.ToolchainVersion != nil {
			tags, err := releaseTagsForVersion(*platform.ToolchainVersion)
			if err != nil {
				return build.Context{}, err
			}
			bctx.ReleaseTags = tags
		}
		goexperiment := ""
		if platform.GOEXPERIMENT != nil {
			goexperiment = *platform.GOEXPERIMENT
		}
		experiments, err := effectiveExperimentTags(platform.GOOS, platform.GOARCH, goexperiment)
		if err != nil {
			return build.Context{}, err
		}
		// The target GOARCH's default arch feature tags; a non-default arch
		// feature level (GOAMD64=v2+, GOARM64=v9, …) is out of scope and must
		// not be silently taken from the host.
		bctx.ToolTags = defaultArchToolTags(platform.GOARCH)
		bctx.ToolTags = append(bctx.ToolTags, experiments...)
		sort.Strings(bctx.ToolTags)
	}
	return bctx, nil
}

// baselineExperimentTags mirrors internal/buildcfg's baseline experiment
// configuration for this module's Go toolchain version (the experiments
// enabled by default for a target configuration, before any GOEXPERIMENT
// override): regabi wrappers/args on the register-ABI ports, DWARF5 where the
// platform's debug tooling supports it, and the always-on experiments. Keep
// this synchronized with the toolchain this module pins (see
// supportedGoTargets).
func baselineExperimentTags(goos, goarch string) []string {
	var tags []string
	switch goarch {
	case "amd64", "arm64", "loong64", "ppc64", "ppc64le", "riscv64":
		tags = append(tags, "goexperiment.regabiwrappers", "goexperiment.regabiargs")
	}
	switch goos {
	case "darwin", "ios", "aix":
	default:
		tags = append(tags, "goexperiment.dwarf5")
	}
	tags = append(tags, "goexperiment.randomizedheapbase64", "goexperiment.greenteagc")
	return tags
}

// effectiveExperimentTags derives the `goexperiment.<name>` tool tags of a
// target configuration: the baseline experiments for the target plus the
// GOEXPERIMENT value's overrides, using the toolchain's own semantics — a
// comma-separated list where `none` disables every experiment and a `no`
// prefix disables a single one. Every experiment name must be a valid Go
// identifier; a malformed name fails naming it. The result is sorted.
func effectiveExperimentTags(goos, goarch, goexperiment string) ([]string, error) {
	enabled := map[string]bool{}
	for _, tag := range baselineExperimentTags(goos, goarch) {
		enabled[strings.TrimPrefix(tag, "goexperiment.")] = true
	}
	for _, f := range strings.Split(goexperiment, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if f == "none" {
			enabled = map[string]bool{}
			continue
		}
		name, on := f, true
		if off, ok := strings.CutPrefix(f, "no"); ok {
			name, on = off, false
		}
		if !isGoIdentifier(name) {
			return nil, fmt.Errorf("platform.goexperiment has invalid experiment name %q", name)
		}
		enabled[name] = on
	}
	var tags []string
	for name, on := range enabled {
		if on {
			tags = append(tags, "goexperiment."+name)
		}
	}
	sort.Strings(tags)
	return tags, nil
}

// releaseTagsForVersion derives the release tags a `go1.N(.M)` toolchain
// version enables: `go1.1` … `go1.N` (the last element is the current release).
func releaseTagsForVersion(version string) ([]string, error) {
	major, ok := strings.CutPrefix(version, "go1.")
	if !ok {
		return nil, fmt.Errorf("platform.toolchain_version %q is not a go1.N(.M) toolchain version", version)
	}
	major, _, _ = strings.Cut(major, ".")
	n, err := strconv.Atoi(major)
	if err != nil || n < 1 {
		return nil, fmt.Errorf("platform.toolchain_version %q is not a go1.N(.M) toolchain version", version)
	}
	tags := make([]string, n)
	for i := range tags {
		tags[i] = "go1." + strconv.Itoa(i+1)
	}
	return tags, nil
}

// isGoIdentifier reports whether name is a valid Go identifier (used for
// experiment names, which are identifiers).
func isGoIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// defaultArchToolTags returns the arch feature tool tags the Go toolchain
// enables by default for the target GOARCH — the same tags
// internal/buildcfg's gogoarchTags derives for a target configuration with no
// GO$GOARCH override, mirrored here because buildcfg is internal to the
// toolchain and a layout must describe a foreign target. The host's
// (possibly non-default) feature levels are never copied.
func defaultArchToolTags(goarch string) []string {
	switch goarch {
	case "386":
		return []string{"386.sse2"}
	case "amd64":
		return []string{"amd64.v1"}
	case "arm":
		return []string{"arm.5", "arm.6", "arm.7"}
	case "arm64":
		return []string{"arm64.v8.0"}
	case "mips", "mipsle", "mips64", "mips64le":
		return []string{goarch + ".hardfloat"}
	case "ppc64", "ppc64le":
		return []string{goarch + ".power8"}
	case "riscv64":
		return []string{"riscv64.rva20u64"}
	default:
		return nil
	}
}

func discoverStdlibWithContext(sdkRoot string, bctx build.Context) ([]*packages.Package, error) {
	pkgs, _, err := discoverStdlibTree(sdkRoot, bctx)
	return pkgs, err
}

// discoverStdlibTree walks the standard library source tree rooted at sdkRoot
// and classifies every directory under the go/build rules for bctx: pkgs are
// the discovered packages (sorted, with the resolved import graph), noGoDirs
// the directories that exist but whose every Go file the target's build
// constraints exclude or that carry no Go sources at all (`go list std` still
// lists such packages, with no files).
func discoverStdlibTree(sdkRoot string, bctx build.Context) ([]*packages.Package, []string, error) {

	fi, err := os.Stat(sdkRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("accessing SDK root %q: %w", sdkRoot, err)
	}
	if !fi.IsDir() {
		return nil, nil, fmt.Errorf("SDK root %q is not a directory", sdkRoot)
	}

	var packageDirs []string
	var noGoDirs []string
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

			// go list std excludes the top-level cmd tree, every testdata
			// directory, tool-scratch directories (segments beginning with
			// `_` or `.`), nested modules (a directory carrying go.mod roots
			// a separate module the std pattern does not enumerate), and the
			// non-importable builtin package — but includes the GOROOT-level
			// vendor tree.
			if importPath == "cmd" || importPath == "testdata" || importPath == "builtin" ||
				strings.HasPrefix(importPath, "cmd/") || strings.HasSuffix(importPath, "/testdata") ||
				strings.Contains(importPath, "/testdata/") ||
				hasToolScratchSegment(importPath) {
				return filepath.SkipDir
			}
			if fi, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && !fi.IsDir() {
				return filepath.SkipDir
			}
			// With cgo disabled, the go tool's std-package walk ignores the
			// runtime/cgo directory entirely (its non-cgo Go files would
			// otherwise make the package look buildable).
			if importPath == "runtime/cgo" && !bctx.CgoEnabled {
				return filepath.SkipDir
			}
			packageDirs = append(packageDirs, path)
		}
		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	sort.Strings(packageDirs)
	sort.Strings(noGoDirs)

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
				noGoDirs = append(noGoDirs, path)
				continue
			}
			return nil, nil, fmt.Errorf("inspecting standard-library package at %q: %w", path, err)
		}

		if bpkg.Name == "main" {
			continue
		}

		rel, err := filepath.Rel(sdkRoot, path)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to compute relative path of %s from SDK root: %w", path, err)
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
		return nil, nil, fmt.Errorf("invalid SDK root %q: structurally invalid (no standard-library packages discovered)", sdkRoot)
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

	pkgs := make([]*packages.Package, 0, len(discovered)+len(noGoDirs))
	for _, item := range discovered {
		pkgs = append(pkgs, item.pkg)
	}
	slices.SortFunc(pkgs, func(a, b *packages.Package) int {
		return strings.Compare(a.ID, b.ID)
	})

	return pkgs, noGoDirs, nil
}

// hasToolScratchSegment reports whether any import-path segment begins with
// `_` or `.` — directories the go tool ignores for package resolution.
func hasToolScratchSegment(importPath string) bool {
	for _, seg := range strings.Split(importPath, "/") {
		if strings.HasPrefix(seg, "_") || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// noGoPackageName reports the package name a source-less standard-library
// directory declares: the package clause of its first parseable .go file,
// reading the files the way `go list` does when it names a package whose
// every file the target's constraints exclude. ok=false when no file
// declares a package clause.
func noGoPackageName(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.PackageClauseOnly)
		if err != nil || f == nil || f.Name == nil {
			continue
		}
		return f.Name.Name, true
	}
	return "", false
}

// hasNonTestGoSource reports whether dir contains any non-test .go file.
// `go list std` omits a package whose every non-test file the target's
// constraints exclude, but it still names a test-only package (one whose
// every .go file is a _test.go file), so only the latter becomes a node.
func hasNonTestGoSource(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if !strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
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

// cloneDependencyArtifactBindings validates and canonically sorts bindings
// without mutating the caller's slice. Dependency names are the primary key;
// the remaining fields are a deterministic tie-break for defensive callers
// even though duplicate names are rejected.
func cloneDependencyArtifactBindings(bindings []DependencyArtifactBinding) ([]DependencyArtifactBinding, error) {
	if err := validateDependencyArtifactBindings(bindings); err != nil {
		return nil, err
	}
	cloned := slices.Clone(bindings)
	slices.SortFunc(cloned, func(a, b DependencyArtifactBinding) int {
		if c := strings.Compare(a.Dependency, b.Dependency); c != 0 {
			return c
		}
		if c := strings.Compare(a.Surface, b.Surface); c != 0 {
			return c
		}
		if c := strings.Compare(a.Report, b.Report); c != 0 {
			return c
		}
		if a.AutoAttached != b.AutoAttached {
			if a.AutoAttached {
				return 1
			}
			return -1
		}
		return strings.Compare(string(a.Provenance), string(b.Provenance))
	})
	return cloned, nil
}

// validateDependencyArtifactBindings enforces the wire contract for direct
// dependency artifact metadata before any package driver uses the layout.
// Paths are slash-separated runfiles-frame paths: absolute, parent-escaping,
// platform-specific, and non-clean spellings are rejected rather than
// allowing a later consumer to resolve a different artifact.
func validateDependencyArtifactBindings(bindings []DependencyArtifactBinding) error {
	seenDependencies := make(map[string]bool, len(bindings))
	seenSurfaces := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		if binding.Dependency == "" {
			return errors.New("dependency artifact binding has empty dependency name")
		}
		if seenDependencies[binding.Dependency] {
			return fmt.Errorf("duplicate dependency artifact binding for dependency %q", binding.Dependency)
		}
		seenDependencies[binding.Dependency] = true

		if binding.Surface == "" {
			return fmt.Errorf("dependency artifact binding for dependency %q has empty surface path", binding.Dependency)
		}
		if err := validateDependencyArtifactPath(binding.Dependency, "surface", binding.Surface); err != nil {
			return err
		}
		if previous, exists := seenSurfaces[binding.Surface]; exists && previous != binding.Dependency {
			return fmt.Errorf("duplicate dependency surface path %q assigned to dependencies %q and %q", binding.Surface, previous, binding.Dependency)
		}
		seenSurfaces[binding.Surface] = binding.Dependency

		switch binding.Provenance {
		case DependencyArtifactProvenanceChecked:
			if binding.Report == "" {
				return fmt.Errorf("checked dependency artifact binding for dependency %q has no report path", binding.Dependency)
			}
		case DependencyArtifactProvenanceAsserted:
			if binding.Report != "" {
				return fmt.Errorf("asserted dependency artifact binding for dependency %q must not have a report path", binding.Dependency)
			}
		default:
			return fmt.Errorf("unknown dependency artifact provenance %q for dependency %q", binding.Provenance, binding.Dependency)
		}
		if binding.Report != "" {
			if err := validateDependencyArtifactPath(binding.Dependency, "report", binding.Report); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDependencyArtifactPath(dependency, kind, value string) error {
	if strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") || path.IsAbs(value) || filepath.VolumeName(value) != "" || hasWindowsVolumePrefix(value) || value == "." {
		return fmt.Errorf("unsafe dependency artifact path %q for dependency %q (%s path)", value, dependency, kind)
	}
	if value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("unsafe dependency artifact path %q for dependency %q (%s path)", value, dependency, kind)
	}
	if path.Clean(value) != value {
		return fmt.Errorf("non-normalized dependency artifact path %q for dependency %q (%s path)", value, dependency, kind)
	}
	return nil
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

// MarshalJSON serializes the Layout with deterministic canonical ordering of
// Roots, dependency artifact bindings, and Packages without mutating the
// original caller-owned data. The layout-only `is_stdlib` field is emitted for
// emitter-listed packages and carries build-graph provenance, never a
// duplicated import-path heuristic.
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
		slices.SortFunc(pkgs, func(a, b *packages.Package) int {
			return strings.Compare(a.ID, b.ID)
		})
	} else {
		pkgs = []*packages.Package{}
	}
	bindingCloned, err := cloneDependencyArtifactBindings(l.DependencyArtifactBindings)
	if err != nil {
		return nil, fmt.Errorf("invalid dependency artifact bindings: %w", err)
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
		GoSDKRoot                  string                      `json:"go_sdk_root"`
		Platform                   *Platform                   `json:"platform,omitempty"`
		Roots                      []string                    `json:"roots"`
		Packages                   []json.RawMessage           `json:"packages"`
		DependencyArtifactBindings []DependencyArtifactBinding `json:"dependency_artifact_bindings,omitempty"`
	}{
		GoSDKRoot:                  l.GoSDKRoot,
		Platform:                   l.Platform,
		Roots:                      roots,
		Packages:                   packageJSON,
		DependencyArtifactBindings: bindingCloned,
	})
}

// UnmarshalJSON decodes the upstream go/packages package shape and retains the
// layout-only is_stdlib side metadata. Missing is_stdlib intentionally decodes
// as false for JSON syntax compatibility; validation never reconstructs the
// missing provenance from the package path.
func (l *Layout) UnmarshalJSON(data []byte) error {
	var wire struct {
		GoSDKRoot                  string                      `json:"go_sdk_root"`
		Platform                   *Platform                   `json:"platform,omitempty"`
		Roots                      []string                    `json:"roots"`
		Packages                   []json.RawMessage           `json:"packages"`
		DependencyArtifactBindings []DependencyArtifactBinding `json:"dependency_artifact_bindings,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	bindingCloned, err := cloneDependencyArtifactBindings(wire.DependencyArtifactBindings)
	if err != nil {
		return fmt.Errorf("invalid dependency artifact bindings: %w", err)
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
	l.DependencyArtifactBindings = bindingCloned
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
	bindingCloned, err := cloneDependencyArtifactBindings(l.DependencyArtifactBindings)
	if err != nil {
		return fmt.Errorf("invalid dependency artifact bindings: %w", err)
	}
	l.DependencyArtifactBindings = bindingCloned

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
	// Index emitter-provided provenance by both layout identities before SDK
	// discovery can add structural packages. The path index is only an accessor
	// for the same declared bit; it is never inferred from the path spelling.
	for _, p := range l.Packages {
		if !l.emittedPackageID[p.ID] {
			continue
		}
		l.stdlibByPath[p.PkgPath] = l.stdlibByID[p.ID]
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
		// GoSDKRoot is required if an explicit layout package carries SDK
		// provenance. An import path alone is intentionally insufficient to
		// identify standard-library data; the authority map makes that decision
		// later, while this loader resolves only structural package resources.
		for _, p := range l.Packages {
			if l.IsStdlibPackage(p) {
				return errors.New("go_sdk_root is required when standard library packages are present in layout")
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
	slices.SortFunc(unresolvedImports, func(a, b UnresolvedImport) int {
		if c := strings.Compare(a.Package, b.Package); c != 0 {
			return c
		}
		if c := strings.Compare(a.SourceFile, b.SourceFile); c != 0 {
			return c
		}
		return strings.Compare(a.ImportPath, b.ImportPath)
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
	slices.SortFunc(pkgsCopy, func(a, b *packages.Package) int {
		return strings.Compare(a.ID, b.ID)
	})

	// The driver response's Arch becomes go/packages' types.Sizes target. A
	// layout that pins a platform describes that target, not the binary
	// serving the layout, so the platform's GOARCH wins; layouts without a
	// platform block keep the running binary's arch.
	arch := runtime.GOARCH
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

// StdlibLayout computes the whole-standard-library layout for sdkRoot under
// the pinned target platform: the packages are the standard library as
// discovered with go/build (mirroring `go list std`'s exclusions, resolving
// the GOROOT vendor tree, synthesising unsafe only when the tree omits it),
// every package is marked standard library, the roots are the discovered
// (importable) packages, and the layout is validated and resolved — source
// files are absolute under sdkRoot and the import graph is transitively
// complete — so it can be served through the driver directly.
func StdlibLayout(sdkRoot string, platform *Platform) (*Layout, error) {
	if platform == nil {
		return nil, errors.New("computing the whole-stdlib layout: a target platform is required")
	}
	pseudo := &Layout{GoSDKRoot: sdkRoot, Platform: platform}
	bctx, err := BuildContextForLayout(pseudo)
	if err != nil {
		return nil, fmt.Errorf("computing the whole-stdlib layout: %w", err)
	}
	bctx.GOROOT = filepath.Dir(sdkRoot)
	pkgs, noGoDirs, err := discoverStdlibTree(sdkRoot, bctx)
	if err != nil {
		return nil, fmt.Errorf("computing the whole-stdlib layout: %w", err)
	}
	// Retain the standard library's source-less directories as package nodes:
	// `go list std` still lists a package whose every Go file the target's
	// constraints exclude (crypto/internal/fips140test, runtime/cgo with cgo
	// off, arena without its experiment), so the driver must be able to serve
	// its pattern — with no files, exactly as go list reports it.
	for _, dir := range noGoDirs {
		if hasNonTestGoSource(dir) {
			// A non-test directory whose every file the target's constraints
			// exclude is not listed by `go list std`; no node.
			continue
		}
		name, ok := noGoPackageName(dir)
		if !ok {
			continue
		}
		rel, err := filepath.Rel(sdkRoot, dir)
		if err != nil {
			return nil, fmt.Errorf("computing the whole-stdlib layout: %w", err)
		}
		pkgs = append(pkgs, &packages.Package{
			ID:      filepath.ToSlash(rel),
			Name:    name,
			PkgPath: filepath.ToSlash(rel),
			Imports: make(map[string]*packages.Package),
		})
	}
	l := &Layout{
		GoSDKRoot: sdkRoot,
		Platform:  platform,
		Roots:     make([]string, 0, len(pkgs)),
		Packages:  pkgs,
	}
	// Roots are the importable packages: a source-less node is enumerated but
	// cannot be loaded or inventoried, so it is not a root.
	for _, p := range pkgs {
		if len(p.GoFiles) > 0 || len(p.CompiledGoFiles) > 0 {
			l.Roots = append(l.Roots, p.ID)
		}
	}
	sort.Strings(l.Roots)
	l.stdlibByID = make(map[string]bool, len(pkgs))
	l.stdlibByPath = make(map[string]bool, len(pkgs))
	l.emittedPackageID = make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		l.stdlibByID[p.ID] = true
		l.stdlibByPath[p.PkgPath] = true
		l.emittedPackageID[p.ID] = true
	}
	if err := ValidateAndResolve(l, ""); err != nil {
		return nil, fmt.Errorf("computing the whole-stdlib layout: %w", err)
	}
	return l, nil
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
