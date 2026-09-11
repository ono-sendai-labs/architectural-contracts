package goanalysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestLoadPackageFacts_UsesMemberOnlyLoadMode(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "member.go")
	if err := os.WriteFile(sourcePath, []byte("package member\n"), 0o644); err != nil {
		t.Fatalf("writing member source: %v", err)
	}
	fset := token.NewFileSet()
	syntax, err := parser.ParseFile(fset, sourcePath, "package member\n", parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing member source: %v", err)
	}

	previousLoader := loadPackages
	t.Cleanup(func() { loadPackages = previousLoader })
	var gotMode packages.LoadMode
	loadPackages = func(cfg *packages.Config, _ ...string) ([]*packages.Package, error) {
		gotMode = cfg.Mode
		return []*packages.Package{{
			ID:              "example.com/member",
			Name:            "member",
			PkgPath:         "example.com/member",
			GoFiles:         []string{sourcePath},
			CompiledGoFiles: []string{sourcePath},
			Imports:         map[string]*packages.Package{},
			Types:           types.NewPackage("example.com/member", "member"),
			Fset:            fset,
			Syntax:          []*ast.File{syntax},
			TypesInfo: &types.Info{
				Defs:       map[*ast.Ident]types.Object{},
				Uses:       map[*ast.Ident]types.Object{},
				Selections: map[*ast.SelectorExpr]*types.Selection{},
			},
		}}, nil
	}

	if _, err := LoadPackageFacts(LoadRequest{ComponentRoot: root}); err != nil {
		t.Fatalf("LoadPackageFacts() error = %v", err)
	}

	wantMode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
		packages.NeedImports | packages.NeedSyntax | packages.NeedTypes |
		packages.NeedTypesInfo | packages.NeedModule
	if gotMode != wantMode {
		t.Fatalf("load mode = %v, want exactly member-only mode %v", gotMode, wantMode)
	}
}
