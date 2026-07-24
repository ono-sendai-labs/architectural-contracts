// Package goanalysis loads Go packages and extracts static facts including imports, exported symbols, and static callgraphs.
//
// Component Contract (FR10):
// - What it does: Analyzes Go syntax trees and types to load package structures, validate interface file correctness, and build a static callgraph.
// - What it requires: Directory paths on the local filesystem and package manifests to resolve dependency interfaces.
// - What it provides: Structural package facts and dependency interface symbols for checking component boundaries.
// - Ambient Authority: This component is a shell component and requires FILES, EXEC, READ_SYSTEM_STATE, OPERATING_SYSTEM, REFLECT, and UNSAFE_POINTER.
package goanalysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

var (
	membershipMu sync.RWMutex
	loadedFiles  = make(map[uintptr]map[string]bool)
)

// LoadPackageFacts loads Go package membership, direct-import, and standard-library facts
// below the supplied component root using go/packages.
func LoadPackageFacts(componentRoot string) (facts.PackageFacts, error) {
	var dir string
	var patterns []string

	if packagelayout.IsLayoutMode() {
		dir = packagelayout.GetActiveWorkspaceDir()
		patterns = packagelayout.GetActiveLayout().Roots
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

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return facts.PackageFacts{}, fmt.Errorf("failed to load packages: %w", err)
	}

	if len(pkgs) == 0 {
		return facts.PackageFacts{}, fmt.Errorf("no packages found under root %q", componentRoot)
	}

	// Collect all load/parse/type errors in the loaded package graph
	var errMsgs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, err := range p.Errors {
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
			compPkgPaths[r] = true
		}
	} else {
		for _, p := range pkgs {
			compPkgPaths[p.PkgPath] = true
		}
	}

	isStdlib := stdlibClassifier()
	stdlibImportSet := make(map[string]bool)

	var factsPkgs []facts.PackageFact
	for _, p := range pkgs {
		if packagelayout.IsLayoutMode() && !compPkgPaths[p.PkgPath] && !compPkgPaths[p.ID] {
			continue
		}
		var imports []string
		for impPath, impPkg := range p.Imports {
			canonImp := hostpolicy.CanonicalizePath(impPath)
			imports = append(imports, canonImp)
			if isStdlib(impPath, impPkg) {
				stdlibImportSet[canonImp] = true
			}
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
			IsStdlib:        isStdlib(p.PkgPath, p),
			Imports:         imports,
			ExportedSymbols: exportedSymbols,
		})
	}

	// Sort package facts by ImportPath for reproducibility
	sort.Slice(factsPkgs, func(i, j int) bool {
		return factsPkgs[i].ImportPath < factsPkgs[j].ImportPath
	})

	// StdlibImports is the loader-authoritative set of standard-library imports,
	// in canonical form, that the checker skips. Always non-nil after a real load.
	stdlibImports := make([]string, 0, len(stdlibImportSet))
	for imp := range stdlibImportSet {
		stdlibImports = append(stdlibImports, imp)
	}
	sort.Strings(stdlibImports)

	sourceFiles := make(map[string]bool)
	for _, p := range pkgs {
		if packagelayout.IsLayoutMode() && !compPkgPaths[p.PkgPath] && !compPkgPaths[p.ID] {
			continue
		}
		for _, absFile := range p.GoFiles {
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

	// Build SSA and construct the VTA call graph
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	allFuncs := ssautil.AllFunctions(prog)
	cg := vta.CallGraph(allFuncs, nil)

	type edgeKey struct {
		caller string
		callee string
	}
	edgesMap := make(map[edgeKey]bool)

	for fn, node := range cg.Nodes {
		if fn == nil || node == nil {
			continue
		}
		callerPkg := getFuncPackagePath(fn)
		if !compPkgPaths[callerPkg] {
			continue
		}
		callerSym := getFuncSymbol(fn)

		for _, edge := range node.Out {
			if edge.Callee == nil || edge.Callee.Func == nil {
				continue
			}
			calleeFn := edge.Callee.Func
			calleePkg := getFuncPackagePath(calleeFn)
			if callerPkg == calleePkg {
				continue
			}
			calleeSym := getFuncSymbol(calleeFn)

			key := edgeKey{caller: string(callerSym), callee: string(calleeSym)}
			passes := passesFuncValue(edge.Site)
			if oldPasses, exists := edgesMap[key]; exists {
				edgesMap[key] = oldPasses || passes
			} else {
				edgesMap[key] = passes
			}
		}
	}

	var callEdges []facts.CallEdge
	for k, passes := range edgesMap {
		callEdges = append(callEdges, facts.CallEdge{
			Caller:          capanalyzer.InterfaceSymbol(k.caller),
			Callee:          capanalyzer.InterfaceSymbol(k.callee),
			PassesFuncValue: passes,
		})
	}

	sort.Slice(callEdges, func(i, j int) bool {
		if callEdges[i].Caller != callEdges[j].Caller {
			return callEdges[i].Caller < callEdges[j].Caller
		}
		return callEdges[i].Callee < callEdges[j].Callee
	})

	res := facts.PackageFacts{
		Packages:      factsPkgs,
		CallEdges:     callEdges,
		StdlibImports: stdlibImports,
	}

	if len(factsPkgs) > 0 {
		ptr := reflect.ValueOf(res.Packages).Pointer()
		membershipMu.Lock()
		loadedFiles[ptr] = sourceFiles
		membershipMu.Unlock()
	}

	return res, nil
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

func isStdlibPackage(p *packages.Package) bool {
	// Standard library packages do not belong to a module, or belong to the special "std" module.
	if p.Module == nil {
		return true
	}
	if p.Module.Path == "" || p.Module.Path == "std" {
		return true
	}
	return false
}

// stdlibClassifier returns a stdlib predicate suitable for the current load mode.
// In package-layout mode, packages carry no *packages.Module, so module-based
// detection would misclassify every package as standard library; there it defers
// to the host stdlib policy (hostpolicy.IsStdlibPath). Otherwise it uses the
// authoritative module metadata, falling back to the host policy only when a
// package reference is absent.
func stdlibClassifier() func(pkgPath string, pkg *packages.Package) bool {
	if packagelayout.IsLayoutMode() {
		return func(pkgPath string, _ *packages.Package) bool {
			return hostpolicy.IsStdlibPath(pkgPath)
		}
	}
	return func(pkgPath string, pkg *packages.Package) bool {
		if pkg == nil {
			return hostpolicy.IsStdlibPath(pkgPath)
		}
		return isStdlibPackage(pkg)
	}
}

func extractSymbols(p *packages.Package, componentRoot string) ([]facts.ExportedSymbol, error) {
	var symbols []facts.ExportedSymbol

	// Emit symbol keys in the canonical namespace so they line up with the
	// canonicalized package facts, dependency-interface symbols, and call edges.
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

	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].File != symbols[j].File {
			return symbols[i].File < symbols[j].File
		}
		if symbols[i].Kind != symbols[j].Kind {
			return symbols[i].Kind < symbols[j].Kind
		}
		return symbols[i].Name < symbols[j].Name
	})

	return symbols, nil
}

// ValidateInterfaceFiles verifies that each interface_files path exists, is relative,
// does not escape, is a regular file, and belongs to a loaded Go package beneath componentRoot.
func ValidateInterfaceFiles(componentRoot string, interfaceFiles []string, loaded facts.PackageFacts) error {
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

	for _, f := range interfaceFiles {
		if filepath.IsAbs(f) {
			return fmt.Errorf("interface file %q is absolute: all interface_files must be relative", f)
		}

		cleaned := filepath.Clean(f)
		if strings.HasPrefix(cleaned, "..") {
			return fmt.Errorf("interface file %q escapes the component root", f)
		}
		cleanedSlash := filepath.ToSlash(cleaned)

		absPath := filepath.Join(root, cleaned)
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("interface file %q does not exist", f)
			}
			return fmt.Errorf("failed to check interface file %q: %w", f, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("interface file %q is not a regular file", f)
		}

		if sourceFiles == nil || !sourceFiles[cleanedSlash] {
			// A declared interface file that exists on disk but is absent from
			// the loaded package was excluded by build constraints for the
			// analysis platform (e.g. a //go:build-gated or _GOOS.go interface
			// file, as when wrapping a cross-platform library). It is not part
			// of the surface analyzed on this platform, so skip it; only a file
			// that should compile here yet is missing is an error.
			if !packagelayout.FileMatchesBuildConstraints(absPath) {
				continue
			}
			return fmt.Errorf("interface file %q does not belong to any loaded Go package under component root", f)
		}
	}

	return nil
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

// canonicalizeSymbol rewrites the package-path portion embedded in a formatted
// function/method symbol key (e.g. "pkg/path.Fn", "(*pkg/path.T).M") through
// hostpolicy.CanonicalizePath, leaving the type/function/method tail unchanged.
// This keeps interface symbols and call-edge symbols in the same namespace as the
// canonicalized package paths, so the checker's boundary comparisons and the
// capability analyzer's prune keys line up. Default identity => no-op upstream.
func canonicalizeSymbol(sym string) string {
	pkg := extractPackageFromStr(sym)
	if pkg == "" {
		return sym
	}
	canon := hostpolicy.CanonicalizePath(pkg)
	if canon == pkg {
		return sym
	}
	// The package path occurs once, ahead of the type/func tail (as a bare prefix
	// or inside a receiver), so replacing its first occurrence is unambiguous.
	return strings.Replace(sym, pkg, canon, 1)
}

// extractPackageFromStr parses the package path from a formatted function/method string.
func extractPackageFromStr(s string) string {
	s = stripAllBrackets(s)
	if strings.HasPrefix(s, "(*") {
		idx := strings.LastIndex(s, ")")
		if idx != -1 {
			receiver := s[2:idx]
			dotIdx := strings.LastIndex(receiver, ".")
			if dotIdx != -1 {
				return receiver[:dotIdx]
			}
		}
	} else if strings.HasPrefix(s, "(") {
		idx := strings.LastIndex(s, ")")
		if idx != -1 {
			receiver := s[1:idx]
			dotIdx := strings.LastIndex(receiver, ".")
			if dotIdx != -1 {
				return receiver[:dotIdx]
			}
		}
	} else {
		dotIdx := strings.LastIndex(s, ".")
		if dotIdx != -1 {
			return s[:dotIdx]
		}
	}
	return ""
}

// getFuncPackagePath returns the package path of an ssa.Function, falling back to parsing
// its string representation if Pkg is nil.
func getFuncPackagePath(fn *ssa.Function) string {
	if fn == nil {
		return ""
	}
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		return fn.Pkg.Pkg.Path()
	}
	return extractPackageFromStr(fn.String())
}

// getFuncSymbol returns the InterfaceSymbol key of an ssa.Function, with its
// embedded package path canonicalized so call-edge symbols share one namespace
// with dependency-interface symbols and package facts.
func getFuncSymbol(fn *ssa.Function) capanalyzer.InterfaceSymbol {
	if fn == nil {
		return ""
	}
	return capanalyzer.InterfaceSymbol(canonicalizeSymbol(stripAllBrackets(fn.String())))
}

// passesFuncValue checks if a call site passes any function-typed value.
func passesFuncValue(site ssa.CallInstruction) bool {
	if site == nil {
		return false
	}
	common := site.Common()
	if common == nil {
		return false
	}
	for _, arg := range common.Args {
		if arg == nil {
			continue
		}
		if _, ok := arg.Type().Underlying().(*types.Signature); ok {
			return true
		}
	}
	return false
}

// ResolveDependencyInterface turns a component dependency into its derived facts.
func ResolveDependencyInterface(
	declaringRoot string,
	analyzedRoot string,
	dep manifest.ComponentDependency,
) (facts.DependencyInterface, error) {
	// 1. Resolve dep.Manifest relative to declaringRoot
	manifestPath := filepath.Clean(filepath.Join(declaringRoot, dep.Manifest))
	depRoot := filepath.Dir(manifestPath)

	// 2. Reject unreadable/invalid manifests
	f, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return facts.DependencyInterface{}, fmt.Errorf("dependency manifest does not exist at %q: %w", manifestPath, err)
		}
		return facts.DependencyInterface{}, fmt.Errorf("failed to open dependency manifest at %q: %w", manifestPath, err)
	}
	defer f.Close()

	depManifest, err := manifest.Parse(f)
	if err != nil {
		return facts.DependencyInterface{}, fmt.Errorf("failed to parse dependency manifest at %q: %w", manifestPath, err)
	}

	// 3. Reject dependency name mismatches
	if depManifest.Name != dep.Name {
		return facts.DependencyInterface{}, fmt.Errorf("dependency name mismatch: expected %q, got %q in manifest", dep.Name, depManifest.Name)
	}

	// 4. Reject roots overlapping the analyzed component as actionable tool errors
	cleanAnalyzed := filepath.Clean(analyzedRoot)
	cleanDepRoot := filepath.Clean(depRoot)
	if cleanDepRoot == cleanAnalyzed ||
		strings.HasPrefix(cleanDepRoot, cleanAnalyzed+string(filepath.Separator)) ||
		strings.HasPrefix(cleanAnalyzed, cleanDepRoot+string(filepath.Separator)) {
		return facts.DependencyInterface{}, fmt.Errorf("root overlap error: dependency root %q overlaps with analyzed root %q", cleanDepRoot, cleanAnalyzed)
	}

	// 5. Load all packages below the dependency root
	var loadDir string
	var patterns []string
	var depPkgs []*packages.Package

	if packagelayout.IsLayoutMode() {
		loadDir = packagelayout.GetActiveWorkspaceDir()
		// Try to find the dependency's package-layout JSON
		depLayoutPath := strings.TrimSuffix(manifestPath, ".component.textproto") + ".package-layout.json"
		f, errOpen := os.Open(depLayoutPath)
		if errOpen != nil {
			// fallback/alternative name check
			depLayoutPath2 := filepath.Join(depRoot, "package-layout.json")
			f, errOpen = os.Open(depLayoutPath2)
			if errOpen != nil {
				return facts.DependencyInterface{}, fmt.Errorf("failed to open dependency package-layout: %w", errOpen)
			}
			depLayoutPath = depLayoutPath2
		}

		depLayout, errParse := packagelayout.Parse(f)
		f.Close()
		if errParse != nil {
			return facts.DependencyInterface{}, fmt.Errorf("failed to parse dependency package-layout: %w", errParse)
		}
		patterns = depLayout.Roots

		cfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
				packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
			Dir: loadDir,
		}

		err = packagelayout.WithTemporaryLayout(depLayoutPath, func() error {
			var loadErr error
			depPkgs, loadErr = packages.Load(cfg, patterns...)
			return loadErr
		})
		if err != nil {
			return facts.DependencyInterface{}, fmt.Errorf("failed to load dependency packages in layout mode: %w", err)
		}
	} else {
		loadDir = cleanDepRoot
		patterns = []string{"./..."}

		cfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
				packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
			Dir: loadDir,
		}

		depPkgs, err = packages.Load(cfg, patterns...)
		if err != nil {
			return facts.DependencyInterface{}, fmt.Errorf("failed to load dependency packages: %w", err)
		}
	}

	if len(depPkgs) == 0 {
		return facts.DependencyInterface{}, fmt.Errorf("no packages found under dependency root %q", cleanDepRoot)
	}

	// Check package load/parse/type errors
	var errMsgs []string
	packages.Visit(depPkgs, nil, func(p *packages.Package) {
		for _, err := range p.Errors {
			errMsgs = append(errMsgs, err.Msg)
		}
	})
	if len(errMsgs) > 0 {
		sort.Strings(errMsgs)
		return facts.DependencyInterface{}, fmt.Errorf("dependency package load errors:\n%s", strings.Join(errMsgs, "\n"))
	}

	// 6. Collect package paths and source files
	var pkgPaths []string
	sourceFiles := make(map[string]bool)
	for _, p := range depPkgs {
		pkgPaths = append(pkgPaths, hostpolicy.CanonicalizePath(p.PkgPath))
		for _, absFile := range p.GoFiles {
			var rel string
			var err error
			if packagelayout.IsLayoutMode() {
				rel, err = filepath.Rel(packagelayout.GetActiveWorkspaceDir(), absFile)
			} else {
				rel, err = filepath.Rel(cleanDepRoot, absFile)
			}
			if err != nil {
				continue
			}
			sourceFiles[filepath.ToSlash(filepath.Clean(rel))] = true
		}
	}
	sort.Strings(pkgPaths)

	// 7. Collect package facts (extract symbols) and register in loadedFiles for ValidateInterfaceFiles
	var factsPkgs []facts.PackageFact
	for _, p := range depPkgs {
		var imports []string
		for impPath := range p.Imports {
			imports = append(imports, impPath)
		}
		sort.Strings(imports)

		isStd := isStdlibPackage(p)

		var extractRoot string
		if packagelayout.IsLayoutMode() {
			extractRoot = packagelayout.GetActiveWorkspaceDir()
		} else {
			extractRoot = cleanDepRoot
		}
		exportedSymbols, err := extractSymbols(p, extractRoot)
		if err != nil {
			return facts.DependencyInterface{}, err
		}

		factsPkgs = append(factsPkgs, facts.PackageFact{
			ImportPath:      p.PkgPath,
			IsStdlib:        isStd,
			Imports:         imports,
			ExportedSymbols: exportedSymbols,
		})
	}

	sort.Slice(factsPkgs, func(i, j int) bool {
		return factsPkgs[i].ImportPath < factsPkgs[j].ImportPath
	})

	depPackageFacts := facts.PackageFacts{
		Packages: factsPkgs,
	}
	if len(factsPkgs) > 0 {
		ptr := reflect.ValueOf(depPackageFacts.Packages).Pointer()
		membershipMu.Lock()
		loadedFiles[ptr] = sourceFiles
		membershipMu.Unlock()
	}

	// 8. Validate interface files
	err = ValidateInterfaceFiles(cleanDepRoot, depManifest.InterfaceFiles, depPackageFacts)
	if err != nil {
		return facts.DependencyInterface{}, fmt.Errorf("invalid interface files in dependency %q: %w", dep.Name, err)
	}

	// Map interface files for O(1) lookup
	interfaceFilesMap := make(map[string]bool)
	for _, f := range depManifest.InterfaceFiles {
		interfaceFilesMap[filepath.ToSlash(filepath.Clean(f))] = true
	}

	// 9. Collect exported interface types declared in interface files
	interfaceTypes := make(map[string]*types.Interface)
	for _, p := range depPkgs {
		for _, file := range p.Syntax {
			if file == nil {
				continue
			}
			pos := p.Fset.Position(file.Pos())
			absPath := pos.Filename
			if absPath == "" {
				continue
			}
			var relPath string
			var err error
			if packagelayout.IsLayoutMode() {
				relPath, err = filepath.Rel(packagelayout.GetActiveWorkspaceDir(), absPath)
			} else {
				relPath, err = filepath.Rel(cleanDepRoot, absPath)
			}
			if err != nil {
				continue
			}
			relPath = filepath.ToSlash(relPath)
			if !interfaceFilesMap[relPath] {
				continue
			}

			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.TYPE {
					continue
				}
				for _, spec := range genDecl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || !ast.IsExported(typeSpec.Name.Name) {
						continue
					}
					obj := p.Types.Scope().Lookup(typeSpec.Name.Name)
					if obj == nil {
						continue
					}
					typeName, ok := obj.(*types.TypeName)
					if !ok {
						continue
					}
					if iface, ok := typeName.Type().Underlying().(*types.Interface); ok {
						interfaceTypes[typeName.Type().String()] = iface
					}
				}
			}
		}
	}

	// 10. Collect all concrete named types in the dependency packages
	var concreteTypes []*types.Named
	for _, p := range depPkgs {
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if obj == nil {
				continue
			}
			typeName, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			if named, ok := typeName.Type().(*types.Named); ok {
				if _, isIface := named.Underlying().(*types.Interface); !isIface {
					concreteTypes = append(concreteTypes, named)
				}
			}
		}
	}

	// 11. Find concrete methods implementing those interface types
	concreteMethods := make(map[string]bool)
	for _, named := range concreteTypes {
		ptrType := types.NewPointer(named)

		for _, iface := range interfaceTypes {
			if types.Implements(named, iface) || types.Implements(ptrType, iface) {
				mset := types.NewMethodSet(ptrType)
				for i := 0; i < mset.Len(); i++ {
					m := mset.At(i)
					methodName := m.Obj().Name()
					formattedTypeName := stripGenericBrackets(types.TypeString(named, nil))

					ptrKey := canonicalizeSymbol("(*" + formattedTypeName + ")." + methodName)
					valKey := canonicalizeSymbol("(" + formattedTypeName + ")." + methodName)

					concreteMethods[ptrKey] = true
					concreteMethods[valKey] = true
				}
			}
		}
	}

	// 12. Combine, deduplicate, and sort derived symbols
	symbolSet := make(map[string]bool)
	for _, p := range factsPkgs {
		for _, sym := range p.ExportedSymbols {
			if interfaceFilesMap[sym.File] {
				symbolSet[sym.Name] = true
			}
		}
	}

	for key := range concreteMethods {
		symbolSet[key] = true
	}

	var symbols []capanalyzer.InterfaceSymbol
	for sym := range symbolSet {
		symbols = append(symbols, capanalyzer.InterfaceSymbol(sym))
	}
	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i] < symbols[j]
	})

	return facts.DependencyInterface{
		Component: dep.Name,
		Packages:  pkgPaths,
		Symbols:   symbols,
	}, nil
}
