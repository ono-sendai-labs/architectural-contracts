package goanalysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"golang.org/x/tools/go/packages"
)

// LoadPackageFacts loads Go package membership, direct-import, and standard-library facts
// below the supplied component root using go/packages.
func LoadPackageFacts(componentRoot string) (facts.PackageFacts, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
		Dir: componentRoot,
	}

	pkgs, err := packages.Load(cfg, "./...")
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

	var factsPkgs []facts.PackageFact
	for _, p := range pkgs {
		var imports []string
		for impPath := range p.Imports {
			imports = append(imports, impPath)
		}
		sort.Strings(imports)

		isStd := isStdlibPackage(p)

		factsPkgs = append(factsPkgs, facts.PackageFact{
			ImportPath:      p.PkgPath,
			IsStdlib:        isStd,
			Imports:         imports,
			ExportedSymbols: nil, // keep empty for this increment
		})
	}

	// Sort package facts by ImportPath for reproducibility
	sort.Slice(factsPkgs, func(i, j int) bool {
		return factsPkgs[i].ImportPath < factsPkgs[j].ImportPath
	})

	return facts.PackageFacts{
		Packages:  factsPkgs,
		CallEdges: nil, // keep empty for this increment
	}, nil
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
