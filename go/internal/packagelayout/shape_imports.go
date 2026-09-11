package packagelayout

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

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
