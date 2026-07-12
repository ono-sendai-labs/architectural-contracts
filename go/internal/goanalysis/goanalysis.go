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

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"golang.org/x/tools/go/packages"
)

var (
	membershipMu sync.RWMutex
	loadedFiles  = make(map[uintptr]map[string]bool)
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

		exportedSymbols, err := extractSymbols(p, componentRoot)
		if err != nil {
			return facts.PackageFacts{}, err
		}

		factsPkgs = append(factsPkgs, facts.PackageFact{
			ImportPath:      p.PkgPath,
			IsStdlib:        isStd,
			Imports:         imports,
			ExportedSymbols: exportedSymbols,
		})
	}

	// Sort package facts by ImportPath for reproducibility
	sort.Slice(factsPkgs, func(i, j int) bool {
		return factsPkgs[i].ImportPath < factsPkgs[j].ImportPath
	})

	sourceFiles := make(map[string]bool)
	for _, p := range pkgs {
		for _, absFile := range p.GoFiles {
			rel, err := filepath.Rel(componentRoot, absFile)
			if err != nil {
				continue
			}
			sourceFiles[filepath.ToSlash(filepath.Clean(rel))] = true
		}
	}

	res := facts.PackageFacts{
		Packages:  factsPkgs,
		CallEdges: nil, // keep empty for this increment
	}

	if len(factsPkgs) > 0 {
		ptr := reflect.ValueOf(res.Packages).Pointer()
		membershipMu.Lock()
		loadedFiles[ptr] = sourceFiles
		membershipMu.Unlock()
	}

	return res, nil
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

func extractSymbols(p *packages.Package, componentRoot string) ([]facts.ExportedSymbol, error) {
	var symbols []facts.ExportedSymbol

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
									formattedRecv := types.TypeString(recvType, nil)

									var receiverKey string
									if strings.HasPrefix(formattedRecv, "*") {
										receiverKey = "(*" + formattedRecv[1:] + ")"
									} else {
										receiverKey = "(" + formattedRecv + ")"
									}

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
							Name:     p.PkgPath + ".init",
							File:     relPath,
							Kind:     "init",
							Receiver: "",
						})
					} else if ast.IsExported(d.Name.Name) {
						symbols = append(symbols, facts.ExportedSymbol{
							Name:     p.PkgPath + "." + d.Name.Name,
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
									Name:     p.PkgPath + "." + ident.Name,
									File:     relPath,
									Kind:     kind,
									Receiver: "",
								})
							}
						}
					case *ast.TypeSpec:
						if ast.IsExported(s.Name.Name) {
							symbols = append(symbols, facts.ExportedSymbol{
								Name:     p.PkgPath + "." + s.Name.Name,
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

	for _, f := range interfaceFiles {
		if filepath.IsAbs(f) {
			return fmt.Errorf("interface file %q is absolute: all interface_files must be relative", f)
		}

		cleaned := filepath.Clean(f)
		if strings.HasPrefix(cleaned, "..") {
			return fmt.Errorf("interface file %q escapes the component root", f)
		}
		cleanedSlash := filepath.ToSlash(cleaned)

		absPath := filepath.Join(componentRoot, cleaned)
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("interface file %q does not exist", f)
			}
			return fmt.Errorf("failed to check interface file %q: %w", f, err)
		}
		if info.IsDir() {
			return fmt.Errorf("interface file %q is a directory, not a regular file", f)
		}

		if sourceFiles == nil || !sourceFiles[cleanedSlash] {
			return fmt.Errorf("interface file %q does not belong to any loaded Go package under component root", f)
		}
	}

	return nil
}
