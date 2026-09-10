// Package goanalysis loads Go packages and extracts static facts including typed reference/import edges and analysis-defeating bypass observations.
//
// Component Contract (FR10):
// - What it does: Analyzes Go syntax trees and types to load package structures, validate interface file correctness, scan the typed reference/import vocabulary of member sources, and detect analysis-defeating constructs. There is no SSA, no VTA call graph and no check-time Capslock: every stdlib decision is the StdlibAuthority port's.
// - What it requires: Directory paths on the local filesystem for the component being checked, package manifests, and bounded surface/report bytes plus explicit build-graph or native read seams. Dependency surfaces are consumed as artifacts; only the main member/closure loader retains NeedDeps until Step 8.
// - What it provides: Structural package facts (typed reference and import edges, bypass observations), exact persisted dependency-interface facts, and independent dependency provenance/freshness/authority axes for checking component boundaries.
// - Ambient Authority: This component is a shell component and requires FILES, EXEC, READ_SYSTEM_STATE, OPERATING_SYSTEM, REFLECT, and UNSAFE_POINTER.
package goanalysis

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"golang.org/x/tools/go/packages"
)

var (
	membershipMu sync.RWMutex
	loadedFiles  = make(map[uintptr]map[string]bool)
	// loadPackages is a loader seam for exercising driver-specific package
	// provenance. Production loads use go/packages directly; integration tests
	// can model a rewriting host without replacing the package analysis graph.
	loadPackages = packages.Load
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
// authority are resolved later through the total StdlibAuthority map.
func LoadPackageFacts(req LoadRequest) (facts.PackageFacts, error) {
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
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
		Dir: dir,
	}

	pkgs, err := loadPackages(cfg, patterns...)
	if err != nil {
		if len(req.Members) > 0 && !packagelayout.IsLayoutMode() {
			return facts.PackageFacts{}, fmt.Errorf("failed to load declared members %v: %w", req.Members, err)
		}
		return facts.PackageFacts{}, fmt.Errorf("failed to load packages: %w", err)
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

	// Collect all load/parse/type errors in the loaded package graph
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
			errMsgs = append(errMsgs, err.Msg)
		}
	})

	if len(errMsgs) > 0 {
		// Deduplicate and sort error messages for reproducible outputs
		uniqueErrs := make(map[string]bool)
		for _, msg := range errMsgs {
			uniqueErrs[msg] = true
		}
		var sortedErrs []string
		for msg := range uniqueErrs {
			sortedErrs = append(sortedErrs, msg)
		}
		sort.Strings(sortedErrs)
		return facts.PackageFacts{}, fmt.Errorf("package load errors:\n%s", strings.Join(sortedErrs, "\n"))
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
	referenceEdges, importEdges, err := ScanReferences(pkgs, members, analysisRoot)
	if err != nil {
		return facts.PackageFacts{}, err
	}
	bypasses, err := ScanAnalysisDefeats(pkgs, members, analysisRoot)
	if err != nil {
		return facts.PackageFacts{}, err
	}

	res := facts.PackageFacts{
		Packages:   factsPkgs,
		References: referenceEdges,
		Imports:    importEdges,
		Bypasses:   bypasses,
	}

	if len(factsPkgs) > 0 {
		ptr := reflect.ValueOf(res.Packages).Pointer()
		membershipMu.Lock()
		loadedFiles[ptr] = sourceFiles
		membershipMu.Unlock()
	}

	return res, nil
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

func packageIsDeclaredMember(pkg *packages.Package, members []string) bool {
	path := hostpolicy.CanonicalizePath(pkg.PkgPath)
	for _, member := range members {
		if hostpolicy.CanonicalizePath(member) == path {
			return true
		}
	}
	return false
}

func packageContainsInterfaceFile(pkg *packages.Package, componentRoot string, interfaceFiles []string) bool {
	if len(interfaceFiles) == 0 {
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
	var sourceFiles map[string]bool
	if len(loaded.Packages) > 0 {
		ptr := reflect.ValueOf(loaded.Packages).Pointer()
		membershipMu.RLock()
		sourceFiles = loadedFiles[ptr]
		membershipMu.RUnlock()
	}

	root := componentRoot
	if packagelayout.IsLayoutMode() {
		root = packagelayout.GetActiveWorkspaceDir()
	}
	bctx := build.Default
	if packagelayout.IsLayoutMode() {
		var err error
		bctx, err = packagelayout.BuildContextForLayout(packagelayout.GetActiveLayout())
		if err != nil {
			return nil, err
		}
	}

	var exclusions []InterfaceFileExclusion
	surviving := 0
	for _, f := range interfaceFiles {
		if filepath.IsAbs(f) {
			return nil, fmt.Errorf("interface file %q is absolute: all interface_files must be relative", f)
		}

		cleaned := filepath.Clean(f)
		if strings.HasPrefix(cleaned, "..") {
			return nil, fmt.Errorf("interface file %q escapes the component root", f)
		}
		cleanedSlash := filepath.ToSlash(cleaned)

		absPath := filepath.Join(root, cleaned)
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("interface file %q does not exist", f)
			}
			return nil, fmt.Errorf("failed to check interface file %q: %w", f, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("interface file %q is not a regular file", f)
		}

		if sourceFiles == nil || !sourceFiles[cleanedSlash] {
			if !packagelayout.FileMatchesBuildConstraintsWithContext(absPath, bctx) {
				exclusions = append(exclusions, InterfaceFileExclusion{
					File:       cleanedSlash,
					Constraint: describeInterfaceFileConstraintWithContext(absPath, bctx),
				})
				continue
			}
			return nil, fmt.Errorf("interface file %q does not belong to any loaded Go package under component root", f)
		}
		surviving++
	}

	slices.SortFunc(exclusions, func(a, b InterfaceFileExclusion) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return strings.Compare(a.Constraint, b.Constraint)
	})
	if len(interfaceFiles) > 0 && surviving == 0 {
		var details []string
		for _, exclusion := range exclusions {
			details = append(details, fmt.Sprintf("%q (%s)", exclusion.File, exclusion.Constraint))
		}
		return exclusions, fmt.Errorf("no interface file survives the analysis platform; excluded files: %s", strings.Join(details, "; "))
	}

	return exclusions, nil
}

func describeInterfaceFileConstraintWithContext(path string, bctx build.Context) string {
	if line, expression, ok := leadingBuildConstraint(path); ok {
		if !expression.Eval(func(tag string) bool {
			return buildContextHasTag(bctx, tag)
		}) {
			return line
		}
		// MatchFile already rejected the file. If its directive matches, the
		// remaining possible source of exclusion is its filename suffix.
		if suffix := filenameBuildConstraint(filepath.Base(path)); suffix != "" {
			return suffix
		}
	} else if suffix := filenameBuildConstraint(filepath.Base(path)); suffix != "" {
		return suffix
	}
	return fmt.Sprintf("filename/build constraint in %q", filepath.Base(path))
}

func leadingBuildConstraint(path string) (string, constraint.Expr, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, false
	}
	defer file.Close()

	var legacy string
	var legacyExpr constraint.Expr
	var goBuild string
	var goBuildExpr constraint.Expr
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") {
			if constraint.IsGoBuild(line) {
				if expression, err := constraint.Parse(line); err == nil && goBuild == "" {
					goBuild = strings.Join(strings.Fields(line), " ")
					goBuildExpr = expression
				}
			}
			if constraint.IsPlusBuild(line) && legacy == "" {
				if expression, err := constraint.Parse(line); err == nil {
					legacy = strings.Join(strings.Fields(line), " ")
					legacyExpr = expression
				}
			}
			continue
		}
		break
	}
	if scanner.Err() != nil {
		return "", nil, false
	}
	if goBuildExpr != nil {
		return goBuild, goBuildExpr, true
	}
	if legacyExpr != nil {
		return legacy, legacyExpr, true
	}
	return "", nil, false
}

func filenameBuildConstraint(name string) string {
	if !strings.HasSuffix(name, ".go") {
		return ""
	}
	base := strings.TrimSuffix(name, ".go")
	base = strings.TrimSuffix(base, "_test")
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return ""
	}
	knownOS := map[string]bool{
		"aix": true, "android": true, "darwin": true, "dragonfly": true,
		"freebsd": true, "hurd": true, "illumos": true, "ios": true,
		"js": true, "linux": true, "nacl": true, "netbsd": true,
		"openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
		"windows": true, "zos": true,
	}
	knownArch := map[string]bool{
		"386": true, "amd64": true, "amd64p32": true, "arm": true,
		"arm64": true, "arm64be": true, "armbe": true, "loong64": true,
		"mips": true, "mips64": true, "mips64le": true, "mips64p32": true,
		"mips64p32le": true, "mipsle": true, "ppc": true, "ppc64": true,
		"ppc64le": true, "riscv": true, "riscv64": true, "s390": true,
		"s390x": true, "sparc": true, "sparc64": true, "wasm": true,
	}
	last := parts[len(parts)-1]
	if len(parts) >= 3 {
		secondLast := parts[len(parts)-2]
		if knownOS[secondLast] && knownArch[last] {
			return fmt.Sprintf("filename suffix %q", "_"+secondLast+"_"+last+".go")
		}
	}
	if knownOS[last] || knownArch[last] {
		return fmt.Sprintf("filename suffix %q", "_"+last+".go")
	}
	return ""
}

func buildContextHasTag(bctx build.Context, tag string) bool {
	if tag == "cgo" {
		return bctx.CgoEnabled
	}
	if tag == bctx.GOOS || tag == bctx.GOARCH || tag == bctx.Compiler {
		return true
	}
	if (bctx.GOOS == "android" && tag == "linux") ||
		(bctx.GOOS == "illumos" && tag == "solaris") ||
		(bctx.GOOS == "ios" && tag == "darwin") {
		return true
	}
	if tag == "unix" {
		switch bctx.GOOS {
		case "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "linux", "netbsd", "openbsd", "solaris":
			return true
		}
	}
	if tag == "boringcrypto" {
		tag = "goexperiment.boringcrypto"
	}
	for _, candidate := range append(append(append([]string{}, bctx.BuildTags...), bctx.ToolTags...), bctx.ReleaseTags...) {
		if candidate == tag {
			return true
		}
	}
	return false
}

// stripAllBrackets recursively removes all brackets and their contents from a string,
// such as generic type parameters/arguments like "[T]" or "[K, V]".
func stripAllBrackets(s string) string {
	var sb strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '[' {
			depth++
		} else if ch == ']' {
			if depth > 0 {
				depth--
			}
		} else if depth == 0 {
			sb.WriteByte(ch)
		}
	}
	return sb.String()
}

// canonicalizeSymbol rewrites package-qualified components in supported
// formatted function and method symbols. It parses the outer receiver/method
// shape first, then scans type expressions token by token so package paths in
// nested generic arguments are handled without replacing type or method names.
// Unsupported or malformed shapes are returned byte-for-byte unchanged.
func canonicalizeSymbol(sym string) string {
	if strings.HasPrefix(sym, "(*") || strings.HasPrefix(sym, "(") {
		close := matchingSymbolParen(sym)
		if close < 0 {
			return sym
		}
		innerStart := 1
		if strings.HasPrefix(sym, "(*") {
			innerStart = 2
		}
		if close <= innerStart {
			return sym
		}
		if close != len(sym)-1 && (close+2 >= len(sym) || sym[close+1] != '.' || !isIdentifier(sym[close+2:])) {
			return sym
		}
		rewritten, ok := rewriteSymbolTypeExpression(sym[innerStart:close])
		if !ok {
			return sym
		}
		return sym[:innerStart] + rewritten + sym[close:]
	}

	dot := topLevelSymbolDot(sym)
	if dot <= 0 || dot+1 >= len(sym) || !isPackagePath(sym[:dot]) {
		return sym
	}
	name, arguments, ok := splitSymbolTail(sym[dot+1:])
	if !ok {
		return sym
	}
	if arguments != "" {
		arguments, ok = rewriteSymbolTypeArguments(arguments)
		if !ok {
			return sym
		}
	}
	return hostpolicy.CanonicalizePath(sym[:dot]) + "." + name + arguments
}

type symbolPathReplacement struct {
	start int
	end   int
	path  string
}

// rewriteSymbolTypeExpression validates the supported type-expression grammar
// before rewriting qualified names. Unsupported or malformed expressions are
// returned unchanged instead of being partially rewritten.
func rewriteSymbolTypeExpression(expr string) (string, bool) {
	parser := symbolTypeParser{expr: expr}
	if !parser.parseType() || !parser.atEnd() {
		return expr, false
	}
	return parser.rewrite(), true
}

func rewriteSymbolTypeArguments(expr string) (string, bool) {
	parser := symbolTypeParser{expr: expr}
	if !parser.parseTypeArguments() || !parser.atEnd() {
		return expr, false
	}
	return parser.rewrite(), true
}

type symbolTypeParser struct {
	expr         string
	pos          int
	replacements []symbolPathReplacement
}

func (p *symbolTypeParser) parseType() bool {
	p.skipSpace()
	if p.pos >= len(p.expr) {
		return false
	}
	if p.consume("*") {
		return p.parseType()
	}
	if p.consume("<-") {
		if !p.consumeName("chan") {
			return false
		}
		return p.parseType()
	}
	if p.expr[p.pos] == '[' {
		return p.parseArrayOrSlice()
	}
	if p.expr[p.pos] == '(' {
		p.pos++
		if !p.parseType() {
			return false
		}
		p.skipSpace()
		return p.consume(")")
	}

	start := p.pos
	for p.pos < len(p.expr) && isPackagePathChar(p.expr[p.pos]) {
		p.pos++
	}
	if start == p.pos {
		return false
	}
	token := p.expr[start:p.pos]
	if token == "map" {
		return p.parseTypeArguments() && p.parseType()
	}
	if token == "chan" {
		p.skipSpace()
		p.consume("<-")
		return p.parseType()
	}
	if dot := strings.LastIndexByte(token, '.'); dot >= 0 {
		if dot == 0 || dot+1 == len(token) || !isPackagePath(token[:dot]) || !isIdentifier(token[dot+1:]) {
			return false
		}
		p.replacements = append(p.replacements, symbolPathReplacement{
			start: start,
			end:   start + dot,
			path:  token[:dot],
		})
	} else if !isIdentifier(token) {
		return false
	}
	if p.peekNonSpace() == '[' {
		return p.parseTypeArguments()
	}
	return true
}

func (p *symbolTypeParser) parseTypeArguments() bool {
	p.skipSpace()
	if !p.consume("[") || !p.parseType() {
		return false
	}
	for {
		p.skipSpace()
		if p.consume("]") {
			return true
		}
		if !p.consume(",") || !p.parseType() {
			return false
		}
	}
}

func (p *symbolTypeParser) parseArrayOrSlice() bool {
	if !p.consume("[") {
		return false
	}
	p.skipSpace()
	if p.consume("]") {
		return p.parseType()
	}
	start := p.pos
	for p.pos < len(p.expr) && p.expr[p.pos] != ']' {
		if !isArrayLengthChar(p.expr[p.pos]) {
			return false
		}
		p.pos++
	}
	if start == p.pos || !p.consume("]") {
		return false
	}
	return p.parseType()
}

func isArrayLengthChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || strings.ContainsRune("_ +-*/%().", rune(ch))
}

func (p *symbolTypeParser) consumeName(name string) bool {
	p.skipSpace()
	if !strings.HasPrefix(p.expr[p.pos:], name) {
		return false
	}
	p.pos += len(name)
	return true
}

func (p *symbolTypeParser) consume(s string) bool {
	if !strings.HasPrefix(p.expr[p.pos:], s) {
		return false
	}
	p.pos += len(s)
	return true
}

func (p *symbolTypeParser) peekNonSpace() byte {
	pos := p.pos
	for pos < len(p.expr) && (p.expr[pos] == ' ' || p.expr[pos] == '\t' || p.expr[pos] == '\n') {
		pos++
	}
	if pos == len(p.expr) {
		return 0
	}
	return p.expr[pos]
}

func (p *symbolTypeParser) skipSpace() {
	for p.pos < len(p.expr) && (p.expr[p.pos] == ' ' || p.expr[p.pos] == '\t' || p.expr[p.pos] == '\n') {
		p.pos++
	}
}

func (p *symbolTypeParser) atEnd() bool {
	p.skipSpace()
	return p.pos == len(p.expr)
}

func (p *symbolTypeParser) rewrite() string {
	result := p.expr
	for i := len(p.replacements) - 1; i >= 0; i-- {
		r := p.replacements[i]
		result = result[:r.start] + hostpolicy.CanonicalizePath(r.path) + result[r.end:]
	}
	return result
}

func matchingSymbolParen(sym string) int {
	parenDepth := 0
	bracketDepth := 0
	for i := 0; i < len(sym); i++ {
		switch sym[i] {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth == 0 {
				return -1
			}
			bracketDepth--
		case '(':
			if bracketDepth == 0 {
				parenDepth++
			}
		case ')':
			if bracketDepth != 0 {
				continue
			}
			parenDepth--
			if parenDepth == 0 {
				return i
			}
			if parenDepth < 0 {
				return -1
			}
		}
	}
	return -1
}

func topLevelSymbolDot(sym string) int {
	bracketDepth := 0
	last := -1
	for i := 0; i < len(sym); i++ {
		switch sym[i] {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth == 0 {
				return -1
			}
			bracketDepth--
		case '.':
			if bracketDepth == 0 {
				last = i
			}
		}
	}
	if bracketDepth != 0 {
		return -1
	}
	return last
}

func isPackagePath(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isPackagePathChar(s[i]) {
			return false
		}
	}
	return true
}

func isPackagePathChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || ch == '.' || ch == '/'
}

func isIdentifier(s string) bool {
	if s == "" || !((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z') || s[0] == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') ||
			(s[i] >= '0' && s[i] <= '9') || s[i] == '_') {
			return false
		}
	}
	return true
}

func splitSymbolTail(s string) (name, arguments string, ok bool) {
	for i := 0; i < len(s); i++ {
		if !((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') ||
			(s[i] >= '0' && s[i] <= '9') || s[i] == '_') {
			if i == 0 || s[i] != '[' {
				return "", "", false
			}
			name = s[:i]
			arguments = s[i:]
			return name, arguments, balancedBrackets(arguments)
		}
	}
	return s, "", isIdentifier(s)
}

func balancedBrackets(s string) bool {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return false
			}
			depth--
		}
	}
	return depth == 0
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
