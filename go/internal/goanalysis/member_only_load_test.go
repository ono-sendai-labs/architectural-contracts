package goanalysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
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
		packages.NeedTypesInfo | packages.NeedModule | packages.NeedExportFile
	if gotMode != wantMode {
		t.Fatalf("load mode = %v, want exactly member-only mode %v", gotMode, wantMode)
	}
}

func TestLoadPackageFacts_RejectsSourceLoadedNonMember(t *testing.T) {
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
	member := &packages.Package{
		ID:              "example.com/member",
		Name:            "member",
		PkgPath:         "example.com/member",
		GoFiles:         []string{sourcePath},
		CompiledGoFiles: []string{sourcePath},
		Imports:         map[string]*packages.Package{"example.com/dep": {ID: "example.com/dep"}},
		Types:           types.NewPackage("example.com/member", "member"),
		Fset:            fset,
		Syntax:          []*ast.File{syntax},
		TypesInfo: &types.Info{
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
		},
	}
	dependency := &packages.Package{
		ID:              "example.com/dep",
		Name:            "dep",
		PkgPath:         "example.com/dep",
		GoFiles:         []string{"dependency.go"},
		CompiledGoFiles: []string{"dependency.go"},
		Types:           types.NewPackage("example.com/dep", "dep"),
		Imports:         map[string]*packages.Package{},
	}

	previousLoader := loadPackages
	t.Cleanup(func() { loadPackages = previousLoader })
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{member, dependency}, nil
	}

	_, err = LoadPackageFacts(LoadRequest{ComponentRoot: root, Members: []string{"example.com/member"}})
	if err == nil {
		t.Fatal("LoadPackageFacts() succeeded with source-loaded non-member")
	}
	for _, want := range []string{"example.com/dep", "non-member", "incomplete export-backed types"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want substring %q", err, want)
		}
	}
}

func TestValidateLoadedPackageGraphRejectsNonMemberSourceLists(t *testing.T) {
	dependency := &packages.Package{
		ID:              "example.com/dep",
		PkgPath:         "example.com/dep",
		GoFiles:         []string{"dep.go"},
		CompiledGoFiles: []string{"dep.go"},
		Types:           types.NewPackage("example.com/dep", "dep"),
		Imports:         map[string]*packages.Package{},
	}
	member := &packages.Package{
		ID:      "example.com/member",
		PkgPath: "example.com/member",
		Types:   types.NewPackage("example.com/member", "member"),
		Syntax:  []*ast.File{},
		TypesInfo: &types.Info{
			Uses:       map[*ast.Ident]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
		},
		Imports: map[string]*packages.Package{"example.com/dep": dependency},
	}

	err := validateLoadedPackageGraph([]*packages.Package{member}, map[string]bool{"example.com/member": true}, true)
	if err == nil || !strings.Contains(err.Error(), `non-member package "example.com/dep" retains source file lists`) {
		t.Fatalf("validateLoadedPackageGraph() error = %v, want a source-list violation", err)
	}
}

func TestValidateLoadedPackageGraphRejectsNonMemberSyntaxAndTypeInfo(t *testing.T) {
	for _, tt := range []struct {
		name       string
		withSyntax bool
		withInfo   bool
	}{
		{name: "syntax", withSyntax: true},
		{name: "type info", withInfo: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dependencyTypes := types.NewPackage("example.com/dep", "dep")
			dependencyTypes.MarkComplete()
			dependency := &packages.Package{
				ID:      "example.com/dep",
				PkgPath: "example.com/dep",
				Types:   dependencyTypes,
				Imports: map[string]*packages.Package{},
			}
			if tt.withSyntax {
				dependency.Syntax = []*ast.File{{}}
			}
			if tt.withInfo {
				dependency.TypesInfo = &types.Info{}
			}
			member := &packages.Package{
				ID:        "example.com/member",
				PkgPath:   "example.com/member",
				Syntax:    []*ast.File{},
				TypesInfo: &types.Info{},
				Imports:   map[string]*packages.Package{"example.com/dep": dependency},
			}

			err := validateLoadedPackageGraph([]*packages.Package{member}, map[string]bool{"example.com/member": true}, true)
			if err == nil || !strings.Contains(err.Error(), `non-member package "example.com/dep" retains source syntax or type-info data`) {
				t.Fatalf("validateLoadedPackageGraph() error = %v, want syntax/type-info violation", err)
			}
		})
	}
}

func TestMemberOnlyLayoutRejectsMissingExportBeforeLoading(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "member.go"), []byte("package member\n\nimport _ \"example.com/dep\"\n"), 0o644); err != nil {
		t.Fatalf("writing member source: %v", err)
	}
	layoutPath := filepath.Join(workspace, "component.package-layout.json")
	layout := `{"roots":["example.com/member"],"packages":[
{"id":"example.com/member","name":"member","pkgPath":"example.com/member","goFiles":["member.go"],"compiledGoFiles":["member.go"],"imports":{"example.com/dep":"example.com/dep"}},
{"id":"example.com/dep","name":"dep","pkgPath":"example.com/dep","exportFile":"exports/missing.a","imports":{}}
]}`
	if err := os.WriteFile(layoutPath, []byte(layout), 0o644); err != nil {
		t.Fatalf("writing package layout: %v", err)
	}

	previousLoader := loadPackages
	t.Cleanup(func() { loadPackages = previousLoader })
	called := false
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		called = true
		return nil, nil
	}
	err := packagelayout.WithDriverEnv(layoutPath, workspace, func() error {
		_, err := LoadPackageFacts(LoadRequest{ComponentRoot: workspace, Members: []string{"example.com/member"}})
		return err
	})
	if err == nil {
		t.Fatal("WithDriverEnv() succeeded with a missing export artifact")
	}
	for _, want := range []string{"example.com/dep", "export file", "does not exist"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want substring %q", err, want)
		}
	}
	if called {
		t.Fatal("loadPackages was called before missing export validation completed")
	}
}
