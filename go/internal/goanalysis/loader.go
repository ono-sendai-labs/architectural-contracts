package goanalysis

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/internal/hostfs"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"golang.org/x/tools/go/gcexportdata"
	"golang.org/x/tools/go/packages"
)

var (
	membershipMu sync.RWMutex
	loadedFiles  = make(map[uintptr]map[string]bool)
	// exportDataStat is a seam for checking fail-closed size accounting without
	// constructing multi-terabyte fixtures in tests.
	exportDataStat = hostfs.Stat
	// loadPackages is a loader seam for exercising driver-specific package
	// provenance. Production loads use go/packages directly; integration tests
	// can model a rewriting host without replacing the package analysis graph.
	loadPackages = packages.Load
	// phaseObserver is deliberately nil in production. Integration tests can
	// install a same-package observer to measure the production loader's phase
	// boundaries without adding timing data to facts or persisted artifacts.
	phaseObserver analysisPhaseObserver
)

type analysisPhase string

const (
	analysisPhaseLoadPackageFacts analysisPhase = "LoadPackageFacts"
	analysisPhaseLoadPackages     analysisPhase = "packages.Load"
	analysisPhaseScanReferences   analysisPhase = "ScanReferences"
)

type analysisPhaseObserver func(analysisPhase, time.Duration)

// componentLoadMode is the narrow type-loading contract for component checks.
// Roots receive syntax and TypesInfo; reachable non-roots are loaded from their
// compiler export data. NeedExportFile keeps the artifact path available for the
// post-load completeness check; NeedDeps would ask go/packages to source-load
// the whole closure and would defeat the compositional loader boundary (DR-02).
const componentLoadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedImports | packages.NeedSyntax | packages.NeedTypes |
	packages.NeedTypesInfo | packages.NeedModule | packages.NeedExportFile

func loadPackageFacts(req LoadRequest) (facts.PackageFacts, error) {
	observer := phaseObserver
	var analysisStarted time.Time
	if observer != nil {
		analysisStarted = time.Now()
		defer func() {
			observer(analysisPhaseLoadPackageFacts, time.Since(analysisStarted))
		}()
	}

	componentRoot := req.ComponentRoot
	var dir string
	var patterns []string

	if packagelayout.IsLayoutMode() {
		layout := packagelayout.GetActiveLayout()
		if len(req.Members) > 0 {
			if err := validateLayoutMembership(layoutRequestMembers(req, layout), layout.Roots); err != nil {
				return facts.PackageFacts{}, err
			}
		}
		dir = packagelayout.GetActiveWorkspaceDir()
		patterns = layout.Roots
	} else if len(req.Members) > 0 {
		dir = componentRoot
		patterns = append(patterns, req.Members...)
		patterns = append(patterns, interfacePackagePatterns(req.InterfaceFiles)...)
	} else {
		dir = componentRoot
		patterns = []string{"./..."}
	}

	cfg := &packages.Config{
		Mode: componentLoadMode,
		Dir:  dir,
	}
	var loadStarted time.Time
	if observer != nil {
		loadStarted = time.Now()
	}
	pkgs, err := loadPackages(cfg, patterns...)
	if observer != nil {
		observer(analysisPhaseLoadPackages, time.Since(loadStarted))
	}
	if err != nil {
		return facts.PackageFacts{}, fmt.Errorf("failed to load package facts: %w", err)
	}

	if len(pkgs) == 0 {
		return facts.PackageFacts{}, fmt.Errorf("no packages found under root %q", componentRoot)
	}

	if err := validateLoaderPackagePaths(pkgs); err != nil {
		return facts.PackageFacts{}, err
	}

	if len(req.Members) > 0 && !packagelayout.IsLayoutMode() {
		if err := validateDeclaredPackages(req.Members, pkgs); err != nil {
			return facts.PackageFacts{}, err
		}
	}

	var errMsgs []string
	unresolvedPaths := make(map[string]bool)
	if packagelayout.IsLayoutMode() {
		for _, observation := range packagelayout.GetActiveLayout().UnresolvedImports {
			unresolvedPaths[observation.ImportPath] = true
		}
	}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, err := range p.Errors {
			if isExpectedUnresolvedLayoutImport(err.Msg, unresolvedPaths) {
				continue
			}
			errMsgs = append(errMsgs, describePackageLoadError(p, err))
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
		return facts.PackageFacts{}, fmt.Errorf("package load errors:\n%s", strings.Join(sorted, "\n"))
	}

	compPkgPaths := make(map[string]bool)
	if packagelayout.IsLayoutMode() {
		for _, r := range packagelayout.GetActiveLayout().Roots {
			compPkgPaths[hostpolicy.CanonicalizePath(r)] = true
		}
	} else if len(req.Members) > 0 {
		for _, p := range pkgs {
			if packageIsDeclaredMember(p, req.Members) || packageContainsInterfaceFile(p, componentRoot, req.InterfaceFiles) {
				compPkgPaths[hostpolicy.CanonicalizePath(p.PkgPath)] = true
			}
		}
	} else {
		for _, p := range pkgs {
			compPkgPaths[hostpolicy.CanonicalizePath(p.PkgPath)] = true
		}
	}
	strictNonMemberSources := false
	if packagelayout.IsLayoutMode() {
		strictNonMemberSources = packagelayout.IsMemberOnlyLayout(packagelayout.GetActiveLayout())
	}
	memberOnlyLoad := !packagelayout.IsLayoutMode()
	if packagelayout.IsLayoutMode() {
		memberOnlyLoad = packagelayout.IsMemberOnlyLayout(packagelayout.GetActiveLayout())
	}
	if memberOnlyLoad {
		if hasExportBackedNonMembers(pkgs, compPkgPaths) {
			if err := completeLoadedExportTypes(pkgs, compPkgPaths); err != nil {
				return facts.PackageFacts{}, err
			}
		}
		if err := validateLoadedPackageGraph(pkgs, compPkgPaths, strictNonMemberSources); err != nil {
			return facts.PackageFacts{}, err
		}
	}
	var exportDataDiagnostics facts.ExportDataDiagnostics
	if memberOnlyLoad {
		exportDataDiagnostics, err = collectExportDataDiagnostics(pkgs, compPkgPaths, dir)
		if err != nil {
			return facts.PackageFacts{}, err
		}
	}
	isEffectiveMember := func(pkgPath string) bool {
		return compPkgPaths[hostpolicy.CanonicalizePath(pkgPath)]
	}

	var factsPkgs []facts.PackageFact
	for _, p := range pkgs {
		if !isEffectiveMember(p.PkgPath) && !isEffectiveMember(p.ID) {
			continue
		}
		var imports []string
		for impPath := range p.Imports {
			imports = append(imports, hostpolicy.CanonicalizePath(impPath))
		}
		sort.Strings(imports)

		var analysisRoot string
		if packagelayout.IsLayoutMode() {
			analysisRoot = packagelayout.GetActiveWorkspaceDir()
		} else {
			analysisRoot = componentRoot
		}
		exportedSymbols, err := extractSymbols(p, analysisRoot)
		if err != nil {
			return facts.PackageFacts{}, err
		}

		factsPkgs = append(factsPkgs, facts.PackageFact{
			ImportPath:      hostpolicy.CanonicalizePath(p.PkgPath),
			Imports:         imports,
			ExportedSymbols: exportedSymbols,
		})
	}

	// Sort package facts by ImportPath for reproducibility
	slices.SortFunc(factsPkgs, func(a, b facts.PackageFact) int {
		return strings.Compare(a.ImportPath, b.ImportPath)
	})

	sourceFiles := make(map[string]bool)
	for _, p := range pkgs {
		if !isEffectiveMember(p.PkgPath) && !isEffectiveMember(p.ID) {
			continue
		}
		for _, absFile := range packageSourceFiles(p) {
			var rel string
			var err error
			if packagelayout.IsLayoutMode() {
				rel, err = filepath.Rel(packagelayout.GetActiveWorkspaceDir(), absFile)
			} else {
				rel, err = filepath.Rel(componentRoot, absFile)
			}
			if err != nil {
				continue
			}
			sourceFiles[filepath.ToSlash(filepath.Clean(rel))] = true
		}
	}

	// Typed reference/import scan and analysis-defeating bypass scan over
	// the member packages. Membership is exact; the scan names the
	// declaring object the source writes (DR-04) — no SSA, no VTA call
	// graph, and no check-time Capslock.
	analysisRoot := componentRoot
	if packagelayout.IsLayoutMode() {
		analysisRoot = packagelayout.GetActiveWorkspaceDir()
	}
	memberList := make([]string, 0, len(compPkgPaths))
	for pkgPath := range compPkgPaths {
		memberList = append(memberList, pkgPath)
	}
	sort.Strings(memberList)
	members, err := facts.NewMemberSet(memberList...)
	if err != nil {
		return facts.PackageFacts{}, fmt.Errorf("building the member set: %w", err)
	}
	var scanStarted time.Time
	if observer != nil {
		scanStarted = time.Now()
	}
	referenceEdges, importEdges, err := scanReferences(pkgs, members, analysisRoot)
	if observer != nil {
		observer(analysisPhaseScanReferences, time.Since(scanStarted))
	}
	if err != nil {
		return facts.PackageFacts{}, err
	}
	bypasses, err := scanAnalysisDefeats(pkgs, members, analysisRoot)
	if err != nil {
		return facts.PackageFacts{}, err
	}

	res := facts.PackageFacts{
		Packages:              factsPkgs,
		ExportDataDiagnostics: exportDataDiagnostics,
		References:            referenceEdges,
		Imports:               importEdges,
		Bypasses:              bypasses,
	}

	if len(factsPkgs) > 0 {
		ptr := reflect.ValueOf(res.Packages).Pointer()
		membershipMu.Lock()
		loadedFiles[ptr] = sourceFiles
		membershipMu.Unlock()
	}

	return res, nil
}

func loadSurfaceInputs(req LoadRequest) (SurfaceInputs, error) {
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

// layoutRequestMembers includes the package owning each declared interface
// file. Bazel layouts list that interface package as a root even when the
// authored manifest keeps it implicit (the explicit `members` list names only
// additional member roots). Comparing the effective set keeps the layout
// membership check fail-closed without rejecting a valid declared interface.
func layoutRequestMembers(req LoadRequest, layout *packagelayout.Layout) []string {
	members := append([]string(nil), req.Members...)
	if layout == nil || len(req.InterfaceFiles) == 0 {
		return members
	}
	for _, pkg := range layout.Packages {
		if pkg == nil {
			continue
		}
		ownsInterface := false
		for _, file := range append(append([]string{}, pkg.GoFiles...), pkg.CompiledGoFiles...) {
			fileSlash := filepath.ToSlash(file)
			for _, interfaceFile := range req.InterfaceFiles {
				cleanInterface := filepath.ToSlash(filepath.Clean(interfaceFile))
				if fileSlash == cleanInterface || strings.HasSuffix(fileSlash, "/"+cleanInterface) {
					ownsInterface = true
					break
				}
			}
			if ownsInterface {
				break
			}
		}
		if ownsInterface {
			members = append(members, pkg.PkgPath)
		}
	}
	return members
}

// validateLayoutMembership enforces the layout/manifest ownership contract at
// the loader boundary. It compares fresh canonical sets and leaves both input
// slices untouched so the original declarations remain available to callers.
func validateLayoutMembership(members, roots []string) error {
	memberSet := canonicalPathSet(members)
	rootSet := canonicalPathSet(roots)
	if equalStringSets(memberSet, rootSet) {
		return nil
	}

	missingFromLayout := setDifference(memberSet, rootSet)
	missingFromManifest := setDifference(rootSet, memberSet)
	return fmt.Errorf(
		"package layout roots do not match manifest members: manifest members: %v; layout roots: %v; missing from layout: %v; missing from manifest: %v",
		sortedSetValues(memberSet), sortedSetValues(rootSet), missingFromLayout, missingFromManifest,
	)
}

func canonicalPathSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[hostpolicy.CanonicalizePath(p)] = true
	}
	return set
}

func equalStringSets(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}

func setDifference(left, right map[string]bool) []string {
	var difference []string
	for value := range left {
		if !right[value] {
			difference = append(difference, value)
		}
	}
	sort.Strings(difference)
	return difference
}

func sortedSetValues(set map[string]bool) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func isExpectedUnresolvedLayoutImport(message string, unresolvedPaths map[string]bool) bool {
	if len(unresolvedPaths) == 0 || !strings.Contains(message, "no metadata for ") {
		return false
	}
	for importPath := range unresolvedPaths {
		if strings.Contains(message, "no metadata for "+importPath) {
			return true
		}
	}
	return false
}

func describePackageLoadError(pkg *packages.Package, loadErr packages.Error) string {
	if pkg != nil && pkg.ExportFile != "" {
		return fmt.Sprintf("package %q (export file %q): %s", pkg.PkgPath, pkg.ExportFile, loadErr.Msg)
	}
	return loadErr.Msg
}

func interfacePackagePatterns(interfaceFiles []string) []string {
	seen := make(map[string]bool)
	var patterns []string
	for _, file := range interfaceFiles {
		cleaned := filepath.Clean(file)
		dir := filepath.ToSlash(filepath.Dir(cleaned))
		pattern := "."
		if dir != "." {
			pattern += "/" + dir
		}
		if !seen[pattern] {
			seen[pattern] = true
			patterns = append(patterns, pattern)
		}
	}
	return patterns
}

func validateDeclaredPackages(members []string, pkgs []*packages.Package) error {
	for _, member := range members {
		canonical := hostpolicy.CanonicalizePath(member)
		found := false
		for _, pkg := range pkgs {
			if hostpolicy.CanonicalizePath(pkg.PkgPath) != canonical {
				continue
			}
			found = true
			if len(pkg.GoFiles) == 0 && len(pkg.CompiledGoFiles) == 0 {
				return fmt.Errorf("declared member %q has no source package", member)
			}
			break
		}
		if !found {
			return fmt.Errorf("declared member %q could not be loaded", member)
		}
	}
	return nil
}

// validateLoaderPackagePaths enforces the host-policy contract at the loader
// boundary. packages.Visit traverses roots and every reachable dependency once,
// so a rewritten transitive dependency cannot disappear during later
// membership filtering.
func validateLoaderPackagePaths(pkgs []*packages.Package) error {
	var validationErr error
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if validationErr != nil || pkg == nil || pkg.PkgPath == "" {
			return
		}
		canonical := hostpolicy.CanonicalizePath(pkg.PkgPath)
		if canonical != pkg.PkgPath {
			validationErr = fmt.Errorf("loader package path %q is not canonical: CanonicalizePath rewrites it to %q", pkg.PkgPath, canonical)
		}
	})
	return validationErr
}

func hasExportBackedNonMembers(pkgs []*packages.Package, memberPaths map[string]bool) bool {
	found := false
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg == nil || memberPaths[hostpolicy.CanonicalizePath(pkg.PkgPath)] || pkg.PkgPath == "unsafe" {
			return
		}
		if pkg.ExportFile != "" {
			found = true
		}
	})
	return found
}

// collectExportDataDiagnostics counts the resolved compiler artifacts that a
// member-only load can consume. The package graph may mention one artifact from
// several package records, so paths are canonicalized and deduplicated before
// statting and summing. This runs after layout/native load validation, making a
// missing artifact or a size-accounting overflow a tool error rather than a
// partial report value.
func collectExportDataDiagnostics(pkgs []*packages.Package, memberPaths map[string]bool, baseDir string) (facts.ExportDataDiagnostics, error) {
	paths := make(map[string]bool)
	pathPackages := make(map[string][]string)
	var missing []string
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg == nil || memberPaths[hostpolicy.CanonicalizePath(pkg.PkgPath)] || pkg.PkgPath == "unsafe" {
			return
		}
		if pkg.ExportFile == "" {
			missing = append(missing, fmt.Sprintf("non-member package %q has no export artifact", pkg.PkgPath))
			return
		}
		resolved := filepath.Clean(pkg.ExportFile)
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(baseDir, resolved)
		}
		absolute, err := hostfs.Abs(resolved)
		if err != nil {
			missing = append(missing, fmt.Sprintf("export artifact %q for package %q cannot be resolved: %v", pkg.ExportFile, pkg.PkgPath, err))
			return
		}
		absolute = filepath.Clean(absolute)
		paths[absolute] = true
		pathPackages[absolute] = append(pathPackages[absolute], pkg.PkgPath)
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return facts.ExportDataDiagnostics{}, fmt.Errorf("invalid non-member export inputs:\n%s", strings.Join(missing, "\n"))
	}

	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	var total uint64
	for _, path := range sortedPaths {
		info, err := exportDataStat(path)
		if err != nil {
			packagesForPath := append([]string(nil), pathPackages[path]...)
			sort.Strings(packagesForPath)
			return facts.ExportDataDiagnostics{}, fmt.Errorf("stating non-member export artifact %q for packages %s: %w", path, strings.Join(packagesForPath, ", "), err)
		}
		if info == nil {
			packagesForPath := append([]string(nil), pathPackages[path]...)
			sort.Strings(packagesForPath)
			return facts.ExportDataDiagnostics{}, fmt.Errorf("stating non-member export artifact %q for packages %s returned no file info", path, strings.Join(packagesForPath, ", "))
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			packagesForPath := append([]string(nil), pathPackages[path]...)
			sort.Strings(packagesForPath)
			return facts.ExportDataDiagnostics{}, fmt.Errorf("non-member export artifact %q for packages %s is not a regular file", path, strings.Join(packagesForPath, ", "))
		}
		if info.Size() < 0 {
			packagesForPath := append([]string(nil), pathPackages[path]...)
			sort.Strings(packagesForPath)
			return facts.ExportDataDiagnostics{}, fmt.Errorf("non-member export artifact %q for packages %s has negative size %d", path, strings.Join(packagesForPath, ", "), info.Size())
		}
		size := uint64(info.Size())
		if ^uint64(0)-total < size {
			packagesForPath := append([]string(nil), pathPackages[path]...)
			sort.Strings(packagesForPath)
			return facts.ExportDataDiagnostics{}, fmt.Errorf("non-member export artifact byte total overflows uint64 at %q for packages %s", path, strings.Join(packagesForPath, ", "))
		}
		total += size
	}

	return facts.ExportDataDiagnostics{
		NonMemberExportArtifactCount: uint64(len(sortedPaths)),
		NonMemberExportBytes:         total,
	}, nil
}

// completeLoadedExportTypes fills the incomplete placeholder packages that
// go/packages creates for transitive export-only nodes when NeedDeps is absent.
// The first source root's direct imports are loaded by go/packages itself; this
// pass reads each remaining declared export artifact through the same gc reader
// and preserves the package pointers already referenced by member TypesInfo.
func completeLoadedExportTypes(pkgs []*packages.Package, memberPaths map[string]bool) error {
	var loaded []*packages.Package
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg != nil {
			loaded = append(loaded, pkg)
		}
	})

	view := make(map[string]*types.Package, len(loaded))
	var fset *token.FileSet
	for _, pkg := range loaded {
		if pkg.Types != nil {
			view[pkg.PkgPath] = pkg.Types
		}
		if fset == nil && pkg.Fset != nil {
			fset = pkg.Fset
		}
	}
	if fset == nil {
		fset = token.NewFileSet()
	}

	slices.SortFunc(loaded, func(a, b *packages.Package) int {
		return strings.Compare(a.PkgPath, b.PkgPath)
	})
	for _, pkg := range loaded {
		if memberPaths[hostpolicy.CanonicalizePath(pkg.PkgPath)] || pkg.PkgPath == "unsafe" {
			continue
		}
		if err := validateExportProducerVersion(pkg.ExportFile); err != nil {
			return fmt.Errorf("package %q: %w", pkg.PkgPath, err)
		}
		if pkg.Types != nil && pkg.Types.Complete() {
			continue
		}
		if pkg.ExportFile == "" {
			continue
		}
		file, err := hostfs.OpenExportData(pkg.ExportFile)
		if err != nil {
			return fmt.Errorf("reading export data for package %q from %q: %w", pkg.PkgPath, pkg.ExportFile, err)
		}
		reader, err := gcexportdata.NewReader(file)
		if err == nil {
			var imported *types.Package
			imported, err = gcexportdata.Read(reader, fset, view, pkg.PkgPath)
			if err == nil {
				pkg.Types = imported
				view[pkg.PkgPath] = imported
			}
		}
		closeErr := file.Close()
		if err != nil {
			return fmt.Errorf("reading export data for package %q from %q: %w", pkg.PkgPath, pkg.ExportFile, err)
		}
		if closeErr != nil {
			return fmt.Errorf("closing export data for package %q from %q: %w", pkg.PkgPath, pkg.ExportFile, closeErr)
		}
		if pkg.Types == nil || !pkg.Types.Complete() {
			return fmt.Errorf("export data for package %q from %q is incomplete", pkg.PkgPath, pkg.ExportFile)
		}
	}
	return nil
}

func validateExportProducerVersion(path string) error {
	if path == "" {
		return nil
	}
	file, err := hostfs.OpenExportData(path)
	if err != nil {
		return fmt.Errorf("read export artifact %q: %w", path, err)
	}
	buffer := make([]byte, 16<<10)
	readCount, readErr := file.Read(buffer)
	closeErr := file.Close()
	if readErr != nil {
		return fmt.Errorf("read export artifact %q: %w", path, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close export artifact %q: %w", path, closeErr)
	}
	data := buffer[:readCount]
	marker := []byte("go object ")
	start := bytes.Index(data, marker)
	if start < 0 {
		return nil
	}
	line := data[start:]
	if end := bytes.IndexByte(line, '\n'); end >= 0 {
		line = line[:end]
	}
	var producerMinor int
	for _, field := range strings.Fields(string(line)) {
		if !strings.HasPrefix(field, "go1.") {
			continue
		}
		minorText := strings.TrimPrefix(field, "go1.")
		if dot := strings.IndexByte(minorText, '.'); dot >= 0 {
			minorText = minorText[:dot]
		}
		producerMinor, err = strconv.Atoi(minorText)
		if err != nil {
			return fmt.Errorf("export artifact %q has unsupported producer version %q", path, field)
		}
		break
	}
	if producerMinor == 0 {
		return nil
	}
	current := packagelayout.NativeGoVersion()
	if !strings.HasPrefix(current, "go1.") {
		return nil
	}
	currentMinorText := strings.TrimPrefix(current, "go1.")
	if dot := strings.IndexByte(currentMinorText, '.'); dot >= 0 {
		currentMinorText = currentMinorText[:dot]
	}
	currentMinor, err := strconv.Atoi(currentMinorText)
	if err != nil {
		return nil
	}
	if producerMinor > currentMinor || producerMinor+2 < currentMinor {
		return fmt.Errorf("export artifact %q was produced by unsupported Go version go1.%d (loader runtime is %s)", path, producerMinor, current)
	}
	return nil
}

// validateLoadedPackageGraph enforces the member-only post-load contract. The
// layout driver strips dependency source metadata before go/packages sees it;
// strictNonMemberSources therefore turns a source-list regression into a tool
// error in the hermetic path. Native go list retains source names as metadata
// even though it does not parse them when NeedDeps is absent, so native callers
// still enforce the stronger syntax/type-info and complete-types checks.
func validateLoadedPackageGraph(pkgs []*packages.Package, memberPaths map[string]bool, strictNonMemberSources bool) error {
	var violations []string
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg == nil {
			return
		}
		isMember := memberPaths[hostpolicy.CanonicalizePath(pkg.PkgPath)]
		if isMember {
			if pkg.Syntax == nil || pkg.TypesInfo == nil {
				violations = append(violations, fmt.Sprintf("member package %q lacks full syntax/type-info data", pkg.PkgPath))
			}
			return
		}
		if pkg.PkgPath == "unsafe" {
			// go/packages deliberately synthesizes an empty Syntax/TypesInfo
			// pair for the compiler builtin package; it has no source or
			// export artifact to validate.
			return
		}

		if len(pkg.Syntax) > 0 || pkg.TypesInfo != nil {
			violations = append(violations, fmt.Sprintf("non-member package %q retains source syntax or type-info data", pkg.PkgPath))
		}
		if strictNonMemberSources && (len(pkg.GoFiles) > 0 || len(pkg.CompiledGoFiles) > 0) {
			violations = append(violations, fmt.Sprintf("non-member package %q retains source file lists", pkg.PkgPath))
		}
		if pkg.Types == nil || !pkg.Types.Complete() {
			violations = append(violations, fmt.Sprintf("non-member package %q has incomplete export-backed types", pkg.PkgPath))
		}
	})
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("member-only package load invariant failed:\n%s", strings.Join(violations, "\n"))
}

func packageIsDeclaredMember(pkg *packages.Package, members []string) bool {
	path := hostpolicy.CanonicalizePath(pkg.PkgPath)
	for _, member := range members {
		if hostpolicy.CanonicalizePath(member) == path {
			return true
		}
	}
	return false
}

func packageIsComponentRoot(pkg *packages.Package, componentRoot string) bool {
	cleanRoot := filepath.Clean(componentRoot)
	for _, file := range pkg.GoFiles {
		if filepath.Clean(filepath.Dir(file)) == cleanRoot {
			return true
		}
	}
	for _, file := range pkg.CompiledGoFiles {
		if filepath.Clean(filepath.Dir(file)) == cleanRoot {
			return true
		}
	}
	return false
}

func packageContainsInterfaceFile(pkg *packages.Package, componentRoot string, interfaceFiles []string) bool {
	if len(interfaceFiles) == 0 || !packageIsComponentRoot(pkg, componentRoot) {
		return false
	}
	files := make(map[string]bool, len(pkg.GoFiles)+len(pkg.CompiledGoFiles))
	for _, file := range pkg.GoFiles {
		files[filepath.Clean(file)] = true
	}
	for _, file := range pkg.CompiledGoFiles {
		files[filepath.Clean(file)] = true
	}
	for _, interfaceFile := range interfaceFiles {
		path := filepath.Clean(filepath.Join(componentRoot, interfaceFile))
		if files[path] {
			return true
		}
	}
	return false
}

// stripGenericBrackets removes a trailing generic instantiation such as "[T]"
// or "[K, V]" from a formatted receiver type, so generic receiver keys match
// the bracket-free type declaration keys used elsewhere (e.g. by FR4 checks).
func stripGenericBrackets(s string) string {
	if idx := strings.IndexByte(s, '['); idx != -1 && strings.HasSuffix(s, "]") {
		return s[:idx]
	}
	return s
}

func extractSymbols(p *packages.Package, componentRoot string) ([]facts.ExportedSymbol, error) {
	var symbols []facts.ExportedSymbol

	// Emit symbol keys in the canonical namespace so they line up with the
	// canonicalized package facts, dependency-interface symbols, and typed
	// reference/import edges.
	canonPkgPath := hostpolicy.CanonicalizePath(p.PkgPath)

	for _, file := range p.Syntax {
		if file == nil {
			continue
		}
		pos := p.Fset.Position(file.Pos())
		absPath := pos.Filename
		if absPath == "" {
			continue
		}
		relPath, err := filepath.Rel(componentRoot, absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to make file path %q relative to root %q: %w", absPath, componentRoot, err)
		}
		relPath = filepath.ToSlash(relPath)

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					if ast.IsExported(d.Name.Name) {
						obj := p.TypesInfo.Defs[d.Name]
						if obj != nil {
							if fn, ok := obj.(*types.Func); ok {
								sig := fn.Type().(*types.Signature)
								if sig != nil && sig.Recv() != nil {
									recvType := sig.Recv().Type()
									formattedRecv := stripGenericBrackets(types.TypeString(recvType, nil))

									var receiverKey string
									if strings.HasPrefix(formattedRecv, "*") {
										receiverKey = "(*" + formattedRecv[1:] + ")"
									} else {
										receiverKey = "(" + formattedRecv + ")"
									}
									receiverKey = canonicalizeSymbol(receiverKey)

									symbols = append(symbols, facts.ExportedSymbol{
										Name:     receiverKey + "." + d.Name.Name,
										File:     relPath,
										Kind:     "method",
										Receiver: receiverKey,
									})
								}
							}
						}
					}
				} else {
					if d.Name.Name == "init" {
						symbols = append(symbols, facts.ExportedSymbol{
							Name:     canonPkgPath + ".init",
							File:     relPath,
							Kind:     "init",
							Receiver: "",
						})
					} else if ast.IsExported(d.Name.Name) {
						symbols = append(symbols, facts.ExportedSymbol{
							Name:     canonPkgPath + "." + d.Name.Name,
							File:     relPath,
							Kind:     "func",
							Receiver: "",
						})
					}
				}

			case *ast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, ident := range s.Names {
							if ast.IsExported(ident.Name) {
								kind := "var"
								if d.Tok == token.CONST {
									kind = "const"
								}
								symbols = append(symbols, facts.ExportedSymbol{
									Name:     canonPkgPath + "." + ident.Name,
									File:     relPath,
									Kind:     kind,
									Receiver: "",
								})
							}
						}
					case *ast.TypeSpec:
						if ast.IsExported(s.Name.Name) {
							symbols = append(symbols, facts.ExportedSymbol{
								Name:     canonPkgPath + "." + s.Name.Name,
								File:     relPath,
								Kind:     "type",
								Receiver: "",
							})
						}
					}
				}
			}
		}
	}

	slices.SortFunc(symbols, func(a, b facts.ExportedSymbol) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})

	return symbols, nil
}

func packageSourceFiles(p *packages.Package) []string {
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
