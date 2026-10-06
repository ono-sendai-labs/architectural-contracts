package packagelayout

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

// PackageShapeView captures the package-level facts needed for layout shape
// emission and validation without exposing x/tools package objects to consumers.
type PackageShapeView struct {
	IsNull          bool
	ID              string
	PkgPath         string
	Name            string
	GoFiles         []string
	CompiledGoFiles []string
	IgnoredFiles    []string
	OtherFiles      []string
	ExportFile      string
	IsStdlib        bool
	Imports         map[string]*PackageShapeImportTarget
}

// PackageShapeImportTarget captures target information for one imported package.
type PackageShapeImportTarget struct {
	ID      string
	PkgPath string
}

// ShapeImports projects a package import map into its stable JSON identity form.
func (v PackageShapeView) ShapeImports(canonicalize func(string) string) (map[string]string, error) {
	if v.Imports == nil {
		return nil, nil
	}
	if canonicalize == nil {
		canonicalize = func(path string) string { return path }
	}
	shaped := make(map[string]string, len(v.Imports))
	for importPath, target := range v.Imports {
		if target == nil || target.ID == "" {
			return nil, fmt.Errorf("import %q has no package ID", importPath)
		}
		canonicalImport := canonicalize(importPath)
		canonicalID := canonicalize(target.ID)
		if _, exists := shaped[canonicalImport]; exists {
			return nil, fmt.Errorf("canonical import path %q has multiple entries", canonicalImport)
		}
		shaped[canonicalImport] = canonicalID
	}
	return shaped, nil
}

// PackageViews projects l.Packages into a slice of PackageShapeView.
func (l *Layout) PackageViews() []PackageShapeView {
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

// ShapeImports projects a package import map into its stable JSON identity
// form. The caller supplies the host namespace policy because package-layout
// owns the package graph while the host chooses the canonical import namespace.
// A canonical collision is rejected even when both entries point at the same
// ID: silently collapsing two source edges would make a shape golden hide a
// malformed layout.
func ShapeImports(imports map[string]*packages.Package, canonicalize func(string) string) (map[string]string, error) {
	if imports == nil {
		return nil, nil
	}
	if canonicalize == nil {
		canonicalize = func(path string) string { return path }
	}
	shaped := make(map[string]string, len(imports))
	for importPath, pkg := range imports {
		if pkg == nil || pkg.ID == "" {
			return nil, fmt.Errorf("import %q has no package ID", importPath)
		}
		canonicalImport := canonicalize(importPath)
		canonicalID := canonicalize(pkg.ID)
		if _, exists := shaped[canonicalImport]; exists {
			return nil, fmt.Errorf("canonical import path %q has multiple entries", canonicalImport)
		}
		shaped[canonicalImport] = canonicalID
	}
	return shaped, nil
}
