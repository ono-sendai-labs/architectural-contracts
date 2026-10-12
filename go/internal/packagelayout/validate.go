package packagelayout

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout/internal/hostenv"
	"golang.org/x/tools/go/packages"
)

// defaultFS is the filesystem used by path-based layout validation and driver entry points.
// Unit tests may override it with an in-memory or custom fs.FS.
var defaultFS fs.FS = hostenv.FS()

type subFS struct {
	base fs.FS
	root string
}

func newSubFS(base fs.FS, root string) fs.FS {
	if root == "" || root == "." {
		return base
	}
	return subFS{base: base, root: filepath.ToSlash(root)}
}

func (s subFS) resolve(name string) string {
	if name == "" || name == "." {
		return s.root
	}
	return path.Join(s.root, filepath.ToSlash(name))
}

func (s subFS) Open(name string) (fs.File, error)     { return s.base.Open(s.resolve(name)) }
func (s subFS) Stat(name string) (fs.FileInfo, error) { return fs.Stat(s.base, s.resolve(name)) }
func (s subFS) ReadFile(name string) ([]byte, error)  { return fs.ReadFile(s.base, s.resolve(name)) }
func (s subFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(s.base, s.resolve(name))
}

// layoutPackageIndex is the identity index shared by source-backed and
// member-only validation. Package IDs are the graph references; import paths
// are the semantic keys used by import maps and root declarations.
type layoutPackageIndex struct {
	byID   map[string]*packages.Package
	byPath map[string]*packages.Package
}

func indexLayoutPackages(l *Layout) (layoutPackageIndex, error) {
	index := layoutPackageIndex{
		byID:   make(map[string]*packages.Package, len(l.Packages)),
		byPath: make(map[string]*packages.Package, len(l.Packages)),
	}
	if l.importsOmitted == nil {
		l.importsOmitted = make(map[string]bool)
	}

	for _, p := range l.Packages {
		if p.ID == "" {
			return layoutPackageIndex{}, errors.New("package has empty ID")
		}
		if p.PkgPath == "" {
			return layoutPackageIndex{}, fmt.Errorf("package %q has empty import path", p.ID)
		}
		if p.Name == "" {
			return layoutPackageIndex{}, fmt.Errorf("package %q has empty name", p.ID)
		}
		if _, ok := index.byID[p.ID]; ok {
			return layoutPackageIndex{}, fmt.Errorf("duplicate package ID: %s", p.ID)
		}
		if _, ok := index.byPath[p.PkgPath]; ok {
			return layoutPackageIndex{}, fmt.Errorf("duplicate package import path: %s", p.PkgPath)
		}

		index.byID[p.ID] = p
		index.byPath[p.PkgPath] = p
		if _, recorded := l.importsOmitted[p.ID]; !recorded {
			l.importsOmitted[p.ID] = p.Imports == nil
		}
	}
	return index, nil
}

// layoutRootPackages resolves root declarations once and returns the unique
// package roles in deterministic ID order. A root may be named by either its
// package ID or import path, matching the existing layout contract.
func layoutRootPackages(roots []string, index layoutPackageIndex) ([]*packages.Package, map[string]bool, error) {
	if len(roots) == 0 {
		return nil, nil, errors.New("roots list cannot be empty")
	}

	rootByID := make(map[string]*packages.Package, len(roots))
	for _, root := range roots {
		pkg, ok := index.byID[root]
		if !ok {
			pkg, ok = index.byPath[root]
		}
		if !ok {
			return nil, nil, fmt.Errorf("unknown root: %s", root)
		}
		rootByID[pkg.ID] = pkg
	}

	rootPackages := make([]*packages.Package, 0, len(rootByID))
	for _, pkg := range rootByID {
		rootPackages = append(rootPackages, pkg)
	}
	slices.SortFunc(rootPackages, func(a, b *packages.Package) int {
		return strings.Compare(a.ID, b.ID)
	})
	rootIDs := make(map[string]bool, len(rootPackages))
	for _, pkg := range rootPackages {
		rootIDs[pkg.ID] = true
	}
	return rootPackages, rootIDs, nil
}

// validateAndResolve validates the layout's structural consistency and resolves
// all workspace-relative and SDK-relative source paths, checking that each file exists.
func validateAndResolve(l *Layout, workspaceDir string) error {
	return validateAndResolveWithSDK(l, workspaceDir, nil)
}

func validateAndResolveWithSDK(l *Layout, workspaceDir string, sdkFS fs.FS) (err error) {
	if l == nil {
		return errors.New("package layout is required")
	}
	defer func() {
		err = withLayoutComponent(l, err)
	}()
	if err := resolveStdlibExportData(l, workspaceDir); err != nil {
		return fmt.Errorf("resolving standard-library export data: %w", err)
	}
	if err := resolveOrdinaryImportData(l, workspaceDir); err != nil {
		return fmt.Errorf("resolving ordinary import data: %w", err)
	}
	bindingCloned, err := cloneDependencyArtifactBindings(l.DependencyArtifactBindings)
	if err != nil {
		return fmt.Errorf("invalid dependency artifact bindings: %w", err)
	}
	l.DependencyArtifactBindings = bindingCloned

	tags, err := BuildTagsForLayout(l)
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
	for _, p := range l.Packages {
		if !l.emittedPackageID[p.ID] {
			continue
		}
		l.stdlibByPath[p.PkgPath] = l.stdlibByID[p.ID]
	}

	// First, discover and merge standard library packages if GoSDKRoot is set and sdkFS is nil.
	if l.GoSDKRoot != "" && sdkFS == nil {
		stdlibTags := tags
		stdlibTags.GOROOT = filepath.Dir(l.GoSDKRoot)
		stdPkgs, err := discoverStdlibWithTags(newSubFS(defaultFS, l.GoSDKRoot), stdlibTags)
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

	if l.GoSDKRoot == "" && sdkFS == nil {
		for _, p := range l.Packages {
			if l.IsStdlibPackage(p) {
				return errors.New("go_sdk_root is required when standard library packages are present in layout")
			}
		}
	}

	if len(l.Roots) == 0 {
		return errors.New("roots list cannot be empty")
	}

	index, err := indexLayoutPackages(l)
	if err != nil {
		return err
	}
	byID, byPath := index.byID, index.byPath
	rootPackages, rootIDs, err := layoutRootPackages(l.Roots, index)
	if err != nil {
		return err
	}

	// Phase 1: Resolve GoFiles and CompiledGoFiles for all packages.
	for _, p := range l.Packages {
		resolvedGoFiles, err := l.resolveAndCheckFiles(p, p.GoFiles, l.GoSDKRoot, workspaceDir, sdkFS)
		if err != nil {
			return err
		}
		p.GoFiles = resolvedGoFiles

		resolvedCompiledFiles, err := l.resolveAndCheckFiles(p, p.CompiledGoFiles, l.GoSDKRoot, workspaceDir, sdkFS)
		if err != nil {
			return err
		}
		p.CompiledGoFiles = resolvedCompiledFiles
	}

	// Phase 1b: Drop sources excluded by build constraints for the target platform.
	for _, p := range l.Packages {
		if l.IsStdlibPackage(p) {
			continue
		}
		hadGoSources := hasGoSources(p.GoFiles) || hasGoSources(p.CompiledGoFiles)
		p.GoFiles = filterByBuildConstraintsWithTags(defaultFS, p.GoFiles, tags)
		p.CompiledGoFiles = filterByBuildConstraintsWithTags(defaultFS, p.CompiledGoFiles, tags)
		if hadGoSources && !hasGoSources(p.GoFiles) && !hasGoSources(p.CompiledGoFiles) {
			return fmt.Errorf("package %q has no Go sources after the declared platform excluded every Go source", p.PkgPath)
		}
	}

	// Phase 2: Recover imports from surviving non-stdlib sources.
	resolveImport := func(importPath string) (*packages.Package, bool) {
		if target, ok := byPath[importPath]; ok {
			return target, true
		}
		if target, ok := byPath["vendor/"+importPath]; ok && l.IsStdlibPackage(target) {
			return target, true
		}
		return nil, false
	}

	var unresolvedImports []UnresolvedImport
	for _, p := range l.Packages {
		if !l.IsStdlibPackage(p) {
			importsOmitted := l.importsOmitted[p.ID]
			sortedFiles := survivingSourceFiles(p)

			importSources := make(map[string][]string)
			for _, file := range sortedFiles {
				source, err := fs.ReadFile(defaultFS, filepath.ToSlash(file))
				if err != nil {
					return fmt.Errorf("parsing source file %q of package %q: %w", file, p.ID, err)
				}
				_, imports, _, err := scanGoSource(file, source)
				if err != nil {
					return fmt.Errorf("parsing source file %q of package %q: %w", file, p.ID, err)
				}
				for _, importPath := range imports {
					importSources[importPath] = append(importSources[importPath], file)
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
					if importsOmitted {
						if p.Imports == nil {
							p.Imports = make(map[string]*packages.Package)
						}
						if _, exists := p.Imports[impPath]; !exists {
							p.Imports[impPath] = &packages.Package{ID: target.ID}
						}
					} else if l.StdlibExportData != nil && l.IsStdlibPackage(target) {
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

			if !importsOmitted {
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
			if target.PkgPath != impPath && (target.PkgPath != "vendor/"+impPath || !l.IsStdlibPackage(target)) {
				return fmt.Errorf("package %q imports path %q with ID %q, but target package import path is %q", p.ID, impPath, target.ID, target.PkgPath)
			}
		}
	}

	for _, rootPkg := range rootPackages {
		if len(survivingSourceFiles(rootPkg)) == 0 {
			return fmt.Errorf("package %q has no source files", rootPkg.PkgPath)
		}
	}

	if l.StdlibExportData != nil {
		if err := validateReachableExportClosure(l, index, rootPackages, rootIDs, workspaceDir); err != nil {
			return err
		}
	}

	return nil
}

// validateAndResolveForMemberOnly validates the layout shape used by a
// component load that keeps only member roots on source.
func validateAndResolveForMemberOnly(l *Layout, workspaceDir string) (err error) {
	if l == nil {
		return errors.New("member-only package layout is required")
	}
	defer func() {
		err = withLayoutComponent(l, err)
	}()
	if err := resolveStdlibExportData(l, workspaceDir); err != nil {
		return fmt.Errorf("resolving standard-library export data: %w", err)
	}
	if err := resolveOrdinaryImportData(l, workspaceDir); err != nil {
		return fmt.Errorf("resolving ordinary import data: %w", err)
	}
	if bindingCloned, err := cloneDependencyArtifactBindings(l.DependencyArtifactBindings); err != nil {
		return fmt.Errorf("invalid dependency artifact bindings: %w", err)
	} else {
		l.DependencyArtifactBindings = bindingCloned
	}

	tags, err := BuildTagsForLayout(l)
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
	for _, p := range l.Packages {
		if l.emittedPackageID[p.ID] {
			l.stdlibByPath[p.PkgPath] = l.stdlibByID[p.ID]
		}
	}
	if l.GoSDKRoot == "" {
		for _, p := range l.Packages {
			if l.IsStdlibPackage(p) {
				return errors.New("go_sdk_root is required when standard library packages are present in layout")
			}
		}
	}

	index, err := indexLayoutPackages(l)
	if err != nil {
		return err
	}
	rootPackages, rootIDs, err := layoutRootPackages(l.Roots, index)
	if err != nil {
		return err
	}

	// Only member roots are source-resolved.
	for _, p := range rootPackages {
		resolvedGoFiles, err := l.resolveAndCheckFiles(p, p.GoFiles, l.GoSDKRoot, workspaceDir, nil)
		if err != nil {
			return err
		}
		p.GoFiles = resolvedGoFiles
		resolvedCompiledFiles, err := l.resolveAndCheckFiles(p, p.CompiledGoFiles, l.GoSDKRoot, workspaceDir, nil)
		if err != nil {
			return err
		}
		p.CompiledGoFiles = resolvedCompiledFiles
		resolvedIgnoredFiles, err := l.resolveAndCheckFiles(p, p.IgnoredFiles, l.GoSDKRoot, workspaceDir, nil)
		if err != nil {
			return err
		}
		p.IgnoredFiles = resolvedIgnoredFiles
		resolvedOtherFiles, err := l.resolveAndCheckFiles(p, p.OtherFiles, l.GoSDKRoot, workspaceDir, nil)
		if err != nil {
			return err
		}
		p.OtherFiles = resolvedOtherFiles

		if !l.IsStdlibPackage(p) {
			hadGoSources := hasGoSources(p.GoFiles) || hasGoSources(p.CompiledGoFiles)
			var ignoredByConstraints []string
			p.GoFiles, ignoredByConstraints = splitGoFilesByBuildConstraints(defaultFS, p.GoFiles, tags)
			selectedCompiledFiles, ignoredCompiledFiles := splitGoFilesByBuildConstraints(defaultFS, p.CompiledGoFiles, tags)
			p.CompiledGoFiles = selectedCompiledFiles
			p.IgnoredFiles = mergeFileRoles(p.IgnoredFiles, ignoredByConstraints, ignoredCompiledFiles)
			if hadGoSources && !hasGoSources(p.GoFiles) && !hasGoSources(p.CompiledGoFiles) {
				return fmt.Errorf("package %q has no Go sources after the declared platform excluded every Go source", p.PkgPath)
			}
		}
		if len(survivingSourceFiles(p)) == 0 {
			return fmt.Errorf("package %q has no source files", p.PkgPath)
		}
	}

	resolveImport := func(importPath string) (*packages.Package, bool) {
		if target, ok := index.byPath[importPath]; ok {
			return target, true
		}
		if target, ok := index.byPath["vendor/"+importPath]; ok && l.IsStdlibPackage(target) {
			return target, true
		}
		return nil, false
	}

	for _, p := range rootPackages {
		sourceImports, err := sourceImportsForPackage(defaultFS, p)
		if err != nil {
			return err
		}
		if p.Imports == nil {
			p.Imports = make(map[string]*packages.Package)
			var paths []string
			for importPath := range sourceImports {
				paths = append(paths, importPath)
			}
			sort.Strings(paths)
			for _, importPath := range paths {
				target, ok := resolveImport(importPath)
				if !ok {
					return fmt.Errorf("package %q source import path %q has no declared package node", p.PkgPath, importPath)
				}
				p.Imports[importPath] = &packages.Package{ID: target.ID}
			}
			continue
		}

		var missing []string
		for importPath := range sourceImports {
			if _, ok := resolveImport(importPath); !ok {
				missing = append(missing, importPath)
				continue
			}
			if _, ok := p.Imports[importPath]; !ok {
				missing = append(missing, importPath)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("package %q has source imports absent from declared Imports: %s", p.PkgPath, strings.Join(missing, ", "))
		}
		var extra []string
		for importPath := range p.Imports {
			if importPath == "C" {
				continue
			}
			if _, ok := resolveImport(importPath); ok && !sourceImports[importPath] {
				extra = append(extra, importPath)
			}
		}
		if len(extra) > 0 {
			sort.Strings(extra)
			return fmt.Errorf("package %q declares source imports not contributed by surviving sources: %s", p.PkgPath, strings.Join(extra, ", "))
		}
	}

	if err := validateReachableExportClosure(l, index, rootPackages, rootIDs, workspaceDir); err != nil {
		return err
	}
	return nil
}

func sourceImportsForPackage(fsys fs.FS, p *packages.Package) (map[string]bool, error) {
	imports := make(map[string]bool)
	for _, file := range survivingSourceFiles(p) {
		source, err := fs.ReadFile(fsys, filepath.ToSlash(file))
		if err != nil {
			return nil, fmt.Errorf("parsing source file %q of package %q: %w", file, p.ID, err)
		}
		_, paths, _, err := scanGoSource(file, source)
		if err != nil {
			return nil, fmt.Errorf("parsing source file %q of package %q: %w", file, p.ID, err)
		}
		for _, importPath := range paths {
			if importPath != "C" {
				imports[importPath] = true
			}
		}
	}
	return imports, nil
}

func (l *Layout) resolveAndCheckFiles(p *packages.Package, files []string, sdkRoot, workspaceDir string, sdkFS fs.FS) ([]string, error) {
	resolved := make([]string, len(files))
	for i, f := range files {
		if f == "" {
			return nil, fmt.Errorf("package %q contains empty source file path", p.ID)
		}

		var pathStr string
		if l.IsStdlibPackage(p) {
			if err := validateStdlibPackagePath(p.PkgPath); err != nil {
				return nil, fmt.Errorf("package %q has %w %q", p.ID, err, p.PkgPath)
			}
			if sdkFS != nil {
				relPath := path.Join(p.PkgPath, path.Base(filepath.ToSlash(f)))
				if _, err := fs.Stat(sdkFS, relPath); err != nil {
					if hostenv.IsNotExist(err) {
						return nil, fmt.Errorf("source file %q for package %q does not exist", relPath, p.ID)
					}
					return nil, fmt.Errorf("error stating source file %q for package %q: %w", relPath, p.ID, err)
				}
				if sdkRoot != "" {
					resolvedPath, err := resolveSDKSourcePath(sdkRoot, p.PkgPath, f)
					if err != nil {
						return nil, fmt.Errorf("package %q: %w", p.ID, err)
					}
					pathStr = resolvedPath
				} else {
					pathStr = relPath
				}
				resolved[i] = pathStr
				continue
			}

			// sdkFS == nil: Resolve relative to sdkRoot.
			resolvedPath, err := resolveSDKSourcePath(sdkRoot, p.PkgPath, f)
			if err != nil {
				return nil, fmt.Errorf("package %q: %w", p.ID, err)
			}
			pathStr = resolvedPath
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
				pathStr = filepath.Join(workspaceDir, cleanedF)
				rel, err := filepath.Rel(workspaceDir, pathStr)
				if err != nil {
					return nil, fmt.Errorf("failed to compute relative path: %w", err)
				}
				if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || strings.HasPrefix(rel, "../") {
					return nil, fmt.Errorf("package %q contains source file path escaping workspace: %q", p.ID, f)
				}
			} else {
				pathStr = cleanedF
			}
		}

		// Check file existence.
		if _, err := fs.Stat(defaultFS, filepath.ToSlash(pathStr)); err != nil {
			if hostenv.IsNotExist(err) {
				return nil, fmt.Errorf("source file %q for package %q does not exist", pathStr, p.ID)
			}
			return nil, fmt.Errorf("error stating source file %q for package %q: %w", pathStr, p.ID, err)
		}

		resolved[i] = pathStr
	}
	return resolved, nil
}

func resolveSDKSourcePath(sdkRoot, packagePath, file string) (string, error) {
	if sdkRoot == "" {
		return filepath.Join(filepath.FromSlash(packagePath), filepath.Base(filepath.FromSlash(file))), nil
	}
	resolved := filepath.Join(sdkRoot, filepath.FromSlash(packagePath), filepath.Base(filepath.FromSlash(file)))
	rel, err := filepath.Rel(sdkRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("failed to check SDK source containment: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("standard-library source path %q escapes SDK root %q", file, sdkRoot)
	}
	return resolved, nil
}

// resolveAndCheckExportFile resolves one compiler export artifact in the
// emitter/workspace frame.
func (l *Layout) resolveAndCheckExportFile(p *packages.Package, exportFile, workspaceDir string) (string, error) {
	if exportFile == "" {
		return "", fmt.Errorf("package %q (import path %q) has empty export file path %q", p.ID, p.PkgPath, exportFile)
	}
	if err := validateExportFilePath(exportFile); err != nil {
		return "", fmt.Errorf("package %q (import path %q) has %w %q", p.ID, p.PkgPath, err, exportFile)
	}

	resolved := filepath.FromSlash(exportFile)
	if workspaceDir != "" {
		resolved = filepath.Join(workspaceDir, resolved)
		rel, err := filepath.Rel(workspaceDir, resolved)
		if err != nil {
			return "", fmt.Errorf("package %q (import path %q) failed to resolve export file %q: %w", p.ID, p.PkgPath, exportFile, err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("package %q (import path %q) has export file path escaping workspace %q", p.ID, p.PkgPath, exportFile)
		}
	}

	info, err := fs.Stat(defaultFS, filepath.ToSlash(resolved))
	if err != nil {
		if hostenv.IsNotExist(err) {
			return "", fmt.Errorf("export file %q for package %q does not exist", resolved, p.PkgPath)
		}
		return "", fmt.Errorf("error stating export file %q for package %q: %w", resolved, p.PkgPath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("export file %q for package %q is a directory", resolved, p.PkgPath)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("export file %q for package %q is not a regular file", resolved, p.PkgPath)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("export file %q for package %q is empty", resolved, p.PkgPath)
	}

	f, err := defaultFS.Open(filepath.ToSlash(resolved))
	if err != nil {
		return "", fmt.Errorf("export file %q for package %q is unreadable: %w", resolved, p.PkgPath, err)
	}
	if f == nil {
		return "", fmt.Errorf("export file %q for package %q is unreadable: open returned a nil file", resolved, p.PkgPath)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("export file %q for package %q is unreadable: closing it failed: %w", resolved, p.PkgPath, err)
	}
	return resolved, nil
}

func filterByBuildConstraints(files []string) []string {
	return filterByBuildConstraintsWithTags(defaultFS, files, nativeBuildTags())
}

func filterByBuildConstraintsWithTags(fsys fs.FS, files []string, tags BuildTags) []string {
	kept := make([]string, 0, len(files))
	for _, f := range files {
		if fileMatchesBuildConstraintsWithTags(fsys, f, tags) {
			kept = append(kept, f)
		}
	}
	return kept
}

func splitGoFilesByBuildConstraints(fsys fs.FS, files []string, tags BuildTags) (selected, ignored []string) {
	selected = make([]string, 0, len(files))
	ignored = make([]string, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		if seen[file] {
			continue
		}
		seen[file] = true
		if fileMatchesBuildConstraintsWithTags(fsys, file, tags) {
			selected = append(selected, file)
		} else {
			ignored = append(ignored, file)
		}
	}
	sort.Strings(selected)
	sort.Strings(ignored)
	return selected, ignored
}

func mergeFileRoles(lists ...[]string) []string {
	seen := make(map[string]bool)
	for _, list := range lists {
		for _, file := range list {
			seen[file] = true
		}
	}
	merged := make([]string, 0, len(seen))
	for file := range seen {
		merged = append(merged, file)
	}
	sort.Strings(merged)
	return merged
}

func hasGoSources(files []string) bool {
	for _, file := range files {
		if strings.HasSuffix(file, ".go") {
			return true
		}
	}
	return false
}

// survivingSourceFiles returns the deterministic, de-duplicated union of the
// source fields that the packages driver may provide.
func survivingSourceFiles(p *packages.Package) []string {
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

func (l *Layout) packageViews() []PackageShapeView {
	if l == nil || l.Packages == nil {
		return nil
	}
	views := make([]PackageShapeView, len(l.Packages))
	for i, pkg := range l.Packages {
		if pkg == nil {
			views[i] = PackageShapeView{IsNull: true}
			continue
		}
		view := PackageShapeView{
			ID:              pkg.ID,
			PkgPath:         pkg.PkgPath,
			Name:            pkg.Name,
			GoFiles:         pkg.GoFiles,
			CompiledGoFiles: pkg.CompiledGoFiles,
			IgnoredFiles:    pkg.IgnoredFiles,
			OtherFiles:      pkg.OtherFiles,
			ExportFile:      pkg.ExportFile,
			IsStdlib:        l.IsStdlibPackage(pkg),
		}
		if pkg.Imports != nil {
			view.Imports = make(map[string]*PackageShapeImportTarget, len(pkg.Imports))
			for importPath, target := range pkg.Imports {
				if target == nil {
					view.Imports[importPath] = nil
					continue
				}
				view.Imports[importPath] = &PackageShapeImportTarget{
					ID:      target.ID,
					PkgPath: target.PkgPath,
				}
			}
		}
		views[i] = view
	}
	return views
}

func (l *Layout) marshalJSON() ([]byte, error) {
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
	ordinaryImportData, err := cloneOrdinaryImportData(l.OrdinaryImportData)
	if err != nil {
		return nil, fmt.Errorf("invalid ordinary import data: %w", err)
	}
	stdlibExportData, err := cloneStdlibExportData(l.StdlibExportData)
	if err != nil {
		return nil, fmt.Errorf("invalid standard-library export data: %w", err)
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
		if p.Imports != nil && len(p.Imports) == 0 {
			fields["Imports"] = json.RawMessage("{}")
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
		OrdinaryImportData         *OrdinaryImportData         `json:"ordinary_import_data,omitempty"`
		StdlibExportData           *StdlibExportData           `json:"stdlib_export_data,omitempty"`
		DependencyArtifactBindings []DependencyArtifactBinding `json:"dependency_artifact_bindings,omitempty"`
	}{
		GoSDKRoot:                  l.GoSDKRoot,
		Platform:                   l.Platform,
		Roots:                      roots,
		Packages:                   packageJSON,
		OrdinaryImportData:         ordinaryImportData,
		StdlibExportData:           stdlibExportData,
		DependencyArtifactBindings: bindingCloned,
	})
}

func (l *Layout) unmarshalJSON(data []byte) error {
	var wire struct {
		GoSDKRoot                  string                      `json:"go_sdk_root"`
		Platform                   *Platform                   `json:"platform,omitempty"`
		Roots                      []string                    `json:"roots"`
		Packages                   []json.RawMessage           `json:"packages"`
		OrdinaryImportData         *OrdinaryImportData         `json:"ordinary_import_data,omitempty"`
		StdlibExportData           *StdlibExportData           `json:"stdlib_export_data,omitempty"`
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
	importsOmitted := make(map[string]bool, len(wire.Packages))
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
		var packageFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &packageFields); err != nil {
			return fmt.Errorf("decoding package %q fields: %w", p.ID, err)
		}
		imports, present := packageFields["Imports"]
		legacyImportsSpelling := false
		if !present {
			imports, present = packageFields["imports"]
			legacyImportsSpelling = true
		}
		if present && string(imports) != "null" {
			if p.Imports == nil {
				p.Imports = make(map[string]*packages.Package)
			}
			importsOmitted[p.ID] = legacyImportsSpelling && string(imports) == "{}"
		} else {
			importsOmitted[p.ID] = true
		}
		packagesList[i] = &p
		stdlibByID[p.ID] = metadata.IsStdlib
		emittedPackageID[p.ID] = true
	}

	l.GoSDKRoot = wire.GoSDKRoot
	l.Platform = wire.Platform
	l.Roots = wire.Roots
	l.Packages = packagesList
	l.OrdinaryImportData = wire.OrdinaryImportData
	l.StdlibExportData = wire.StdlibExportData
	l.DependencyArtifactBindings = bindingCloned
	l.stdlibByID = stdlibByID
	l.stdlibByPath = make(map[string]bool, len(stdlibByID))
	l.emittedPackageID = emittedPackageID
	l.UnresolvedImports = nil
	l.importsOmitted = importsOmitted
	l.stdlibExportDataResolved = false
	l.ordinaryImportDataResolved = false
	return nil
}

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
	if strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.Contains(value, "\\") || path.IsAbs(value) || filepath.VolumeName(value) != "" || hasWindowsVolumePrefix(value) || value == "." {
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

func cloneOrdinaryImportData(data *OrdinaryImportData) (*OrdinaryImportData, error) {
	if data == nil {
		return nil, nil
	}
	if data.Metadata == "" {
		return nil, errors.New("ordinary import metadata path is required")
	}
	if err := validateExportFilePath(data.Metadata); err != nil {
		return nil, fmt.Errorf("unsafe ordinary import metadata path %q: %w", data.Metadata, err)
	}
	return &OrdinaryImportData{Metadata: data.Metadata}, nil
}

func cloneStdlibExportData(data *StdlibExportData) (*StdlibExportData, error) {
	if data == nil {
		return nil, nil
	}
	if data.Metadata == "" {
		return nil, errors.New("standard-library export metadata path is required")
	}
	if err := validateExportFilePath(data.Metadata); err != nil {
		return nil, fmt.Errorf("unsafe standard-library export metadata path %q: %w", data.Metadata, err)
	}
	clone := &StdlibExportData{
		Metadata:    data.Metadata,
		ExportRoots: append([]StdlibExportRoot(nil), data.ExportRoots...),
	}
	for i := range clone.ExportRoots {
		root := &clone.ExportRoots[i]
		if root.RunfilesPath == "" || root.ExecPath == "" {
			return nil, fmt.Errorf("standard-library export root %d requires runfiles_path and exec_path", i)
		}
		if err := validateExportFilePath(root.RunfilesPath); err != nil {
			return nil, fmt.Errorf("invalid standard-library export root runfiles path %q: %w", root.RunfilesPath, err)
		}
		if err := validateExportFilePath(root.ExecPath); err != nil {
			return nil, fmt.Errorf("invalid standard-library export root exec path %q: %w", root.ExecPath, err)
		}
	}
	slices.SortFunc(clone.ExportRoots, func(a, b StdlibExportRoot) int {
		if c := strings.Compare(a.ExecPath, b.ExecPath); c != 0 {
			return c
		}
		return strings.Compare(a.RunfilesPath, b.RunfilesPath)
	})
	seenExecPaths := make(map[string]string, len(clone.ExportRoots))
	seenRunfilesPaths := make(map[string]string, len(clone.ExportRoots))
	for _, root := range clone.ExportRoots {
		if previous, exists := seenExecPaths[root.ExecPath]; exists {
			if previous != root.RunfilesPath {
				return nil, fmt.Errorf("conflicting standard-library export root %q maps to %q and %q", root.ExecPath, previous, root.RunfilesPath)
			}
			return nil, fmt.Errorf("duplicate standard-library export root %q", root.ExecPath)
		}
		if previous, exists := seenRunfilesPaths[root.RunfilesPath]; exists {
			return nil, fmt.Errorf("conflicting standard-library export root runfiles path %q maps to %q and %q", root.RunfilesPath, previous, root.ExecPath)
		}
		seenExecPaths[root.ExecPath] = root.RunfilesPath
		seenRunfilesPaths[root.RunfilesPath] = root.ExecPath
	}
	if data.Target != nil {
		target, err := cloneStdlibExportTarget(data.Target)
		if err != nil {
			return nil, err
		}
		clone.Target = target
	}
	return clone, nil
}

func cloneStdlibExportTarget(target *StdlibExportTarget) (*StdlibExportTarget, error) {
	if target == nil {
		return nil, nil
	}
	if target.ToolchainVersion == "" {
		return nil, errors.New("standard-library export target requires toolchain_version")
	}
	if target.GOOS == "" {
		return nil, errors.New("standard-library export target requires goos")
	}
	if target.GOARCH == "" {
		return nil, errors.New("standard-library export target requires goarch")
	}
	tags := append([]string(nil), target.BuildTags...)
	sort.Strings(tags)
	return &StdlibExportTarget{
		ToolchainVersion: target.ToolchainVersion,
		GOOS:             target.GOOS,
		GOARCH:           target.GOARCH,
		CgoEnabled:       target.CgoEnabled,
		BuildTags:        tags,
		GOEXPERIMENT:     target.GOEXPERIMENT,
	}, nil
}

func validateExportFilePath(value string) error {
	if strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.Contains(value, "\\") || path.IsAbs(value) || filepath.VolumeName(value) != "" || hasWindowsVolumePrefix(value) || value == "." {
		return errors.New("unsafe export file path")
	}
	cleaned := path.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return errors.New("unsafe export file path")
	}
	if cleaned != value {
		return errors.New("non-normalized export file path")
	}
	return nil
}
