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

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"golang.org/x/tools/go/packages"
)

func TestValidateLoaderPackagePathsChecksDependencyGraph(t *testing.T) {
	original := hostpolicy.CanonicalizePath
	t.Cleanup(func() { hostpolicy.CanonicalizePath = original })
	hostpolicy.CanonicalizePath = func(path string) string {
		if path == "host/dep" {
			return "canonical/dep"
		}
		return path
	}

	dependency := &packages.Package{ID: "host/dep", PkgPath: "host/dep"}
	root := &packages.Package{
		ID:      "host/root",
		PkgPath: "host/root",
		Imports: map[string]*packages.Package{"host/dep": dependency},
	}
	err := validateLoaderPackagePaths([]*packages.Package{root})
	if err == nil || !strings.Contains(err.Error(), "host/dep") || !strings.Contains(err.Error(), "canonical/dep") {
		t.Fatalf("validateLoaderPackagePaths() error = %v, want both loader and canonical paths", err)
	}
}

func TestLoadPackageFactsRejectsNonIdentityLoaderPath(t *testing.T) {
	originalPolicy := hostpolicy.CanonicalizePath
	originalLoad := loadPackages
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = originalPolicy
		loadPackages = originalLoad
	})
	hostpolicy.CanonicalizePath = func(path string) string {
		return "canonical/" + path
	}
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{{ID: "host/component", PkgPath: "host/component"}}, nil
	}

	_, err := LoadPackageFacts(LoadRequest{ComponentRoot: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "host/component") || !strings.Contains(err.Error(), "canonical/host/component") {
		t.Fatalf("LoadPackageFacts() error = %v, want loader-path contract error", err)
	}
}

func TestLoadPackageFactsRejectsNonIdentityLoaderPathInLayoutMode(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "component.go"), []byte("package component\n"), 0644); err != nil {
		t.Fatal(err)
	}
	layoutPath := filepath.Join(workspace, "package-layout.json")
	layout := `{"roots":["host/component"],"packages":[{"id":"host/component","name":"component","pkgPath":"host/component","goFiles":["component.go"],"compiledGoFiles":["component.go"],"imports":{},"is_stdlib":false}]}`
	if err := os.WriteFile(layoutPath, []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	originalPolicy := hostpolicy.CanonicalizePath
	originalStdlib := hostpolicy.IsStdlibPath
	originalLoad := loadPackages
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = originalPolicy
		hostpolicy.IsStdlibPath = originalStdlib
		loadPackages = originalLoad
	})
	hostpolicy.CanonicalizePath = func(path string) string {
		if path == "host/component" {
			return "canonical/component"
		}
		return path
	}
	hostpolicy.IsStdlibPath = func(string) bool { return false }
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{{ID: "host/component", PkgPath: "host/component"}}, nil
	}

	err := packagelayout.WithDriverEnv(layoutPath, workspace, func() error {
		_, err := LoadPackageFacts(LoadRequest{ComponentRoot: workspace})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "host/component") || !strings.Contains(err.Error(), "canonical/component") {
		t.Fatalf("layout LoadPackageFacts() error = %v, want loader-path contract error", err)
	}
}

func TestLoadPackageFactsCanonicalizesExternalMemberSpelling(t *testing.T) {
	originalPolicy := hostpolicy.CanonicalizePath
	originalLoad := loadPackages
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = originalPolicy
		loadPackages = originalLoad
	})
	hostpolicy.CanonicalizePath = func(path string) string {
		if path == "host/component" {
			return "canonical/component"
		}
		return path
	}
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{{
			ID:      "canonical/component",
			PkgPath: "canonical/component",
			GoFiles: []string{"component.go"},
			Module:  &packages.Module{Path: "example.com/component"},
		}}, nil
	}

	loaded, err := LoadPackageFacts(LoadRequest{
		ComponentRoot: t.TempDir(),
		Members:       []string{"host/component"},
	})
	if err != nil {
		t.Fatalf("LoadPackageFacts() error = %v", err)
	}
	if len(loaded.Packages) != 1 || loaded.Packages[0].ImportPath != "canonical/component" {
		t.Fatalf("loaded packages = %+v, want canonical member", loaded.Packages)
	}
}

func TestCanonicalizeSymbolRewritesEveryPackagePath(t *testing.T) {
	original := hostpolicy.CanonicalizePath
	t.Cleanup(func() { hostpolicy.CanonicalizePath = original })
	paths := map[string]string{
		"a/b":     "canonical/a/b",
		"c/d":     "canonical/c/d",
		"e/f":     "canonical/e/f",
		"foo/bar": "canonical/foo/bar",
		"pkg":     "canonical/pkg",
	}
	hostpolicy.CanonicalizePath = func(path string) string {
		if canonical, ok := paths[path]; ok {
			return canonical
		}
		return path
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "generic pointer receiver", in: "(*a/b.T[c/d.U]).M", want: "(*canonical/a/b.T[canonical/c/d.U]).M"},
		{name: "value receiver", in: "(a/b.T).M", want: "(canonical/a/b.T).M"},
		{name: "ordinary function", in: "pkg.F", want: "canonical/pkg.F"},
		{name: "multiple and nested arguments", in: "(*a/b.T[c/d.U, e/f.V[foo/bar.W]]).M", want: "(*canonical/a/b.T[canonical/c/d.U, canonical/e/f.V[canonical/foo/bar.W]]).M"},
		{name: "overlapping prefixes", in: "(*a/b.T[a/bc.U]).M", want: "(*canonical/a/b.T[a/bc.U]).M"},
		{name: "malformed receiver", in: "(*a/b.T[c/d.U].M", want: "(*a/b.T[c/d.U].M"},
		{name: "unsupported balanced receiver", in: "(*a/b.T[bad? c/d.U]).M", want: "(*a/b.T[bad? c/d.U]).M"},
		{name: "unsupported selector", in: "a/b.T.M", want: "a/b.T.M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalizeSymbol(tt.in); got != tt.want {
				t.Fatalf("canonicalizeSymbol(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveDependencyInterfaceCanonicalizesLocalFacts(t *testing.T) {
	workspace := t.TempDir()
	declaringRoot := filepath.Join(workspace, "declaring")
	depRoot := filepath.Join(workspace, "dep")
	if err := os.MkdirAll(depRoot, 0755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(depRoot, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte("name: \"dep\"\ninterface_files: \"api.go\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	apiPath := filepath.Join(depRoot, "api.go")
	if err := os.WriteFile(apiPath, []byte("package dep\nfunc Exported() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, apiPath, "package dep\nfunc Exported() {}\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	typesInfo := &types.Info{Defs: make(map[*ast.Ident]types.Object)}
	typesPkg, err := (&types.Config{}).Check("canonical/dep", fset, []*ast.File{file}, typesInfo)
	if err != nil {
		t.Fatal(err)
	}

	originalPolicy := hostpolicy.CanonicalizePath
	originalLoad := loadPackages
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = originalPolicy
		loadPackages = originalLoad
	})
	hostpolicy.CanonicalizePath = func(path string) string {
		if path == "host/import" {
			return "canonical/import"
		}
		return path
	}
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{{
			ID:        "canonical/dep",
			PkgPath:   "canonical/dep",
			Name:      "dep",
			GoFiles:   []string{apiPath},
			Fset:      fset,
			Syntax:    []*ast.File{file},
			Types:     typesPkg,
			TypesInfo: typesInfo,
			Imports: map[string]*packages.Package{
				"host/import": {ID: "canonical/import", PkgPath: "canonical/import"},
			},
		}}, nil
	}

	result, err := ResolveDependencyInterface(declaringRoot, declaringRoot, manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/component.textproto",
	})
	if err != nil {
		t.Fatalf("ResolveDependencyInterface() error = %v", err)
	}
	if got, want := result.Packages, []string{"canonical/dep"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("result packages = %v, want %v", got, want)
	}
	if len(result.Symbols) != 1 || result.Symbols[0] != "canonical/dep.Exported" {
		t.Fatalf("result symbols = %v, want canonical/dep.Exported", result.Symbols)
	}
}

func TestResolveDependencyInterfaceRejectsNonIdentityLoaderPath(t *testing.T) {
	workspace := t.TempDir()
	declaringRoot := filepath.Join(workspace, "declaring")
	depRoot := filepath.Join(workspace, "dep")
	if err := os.MkdirAll(depRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depRoot, "component.textproto"), []byte("name: \"dep\"\ninterface_files: \"api.go\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	originalPolicy := hostpolicy.CanonicalizePath
	originalLoad := loadPackages
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = originalPolicy
		loadPackages = originalLoad
	})
	hostpolicy.CanonicalizePath = func(path string) string {
		if path == "host/dep" {
			return "canonical/dep"
		}
		return path
	}
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{{ID: "host/dep", PkgPath: "host/dep"}}, nil
	}

	_, err := ResolveDependencyInterface(declaringRoot, declaringRoot, manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/component.textproto",
	})
	if err == nil || !strings.Contains(err.Error(), "host/dep") || !strings.Contains(err.Error(), "canonical/dep") {
		t.Fatalf("ResolveDependencyInterface() error = %v, want loader-path contract error", err)
	}
}

func TestCanonicalizePackageFact(t *testing.T) {
	original := hostpolicy.CanonicalizePath
	t.Cleanup(func() { hostpolicy.CanonicalizePath = original })
	hostpolicy.CanonicalizePath = func(path string) string {
		if strings.HasPrefix(path, "host/") {
			return "canonical/" + strings.TrimPrefix(path, "host/")
		}
		return path
	}

	got := canonicalizePackageFact(facts.PackageFact{
		ImportPath: "host/dep",
		Imports:    []string{"host/z", "host/a", "host/a"},
	})
	if got.ImportPath != "canonical/dep" {
		t.Fatalf("canonicalized ImportPath = %q, want canonical/dep", got.ImportPath)
	}
	wantImports := []string{"canonical/a", "canonical/a", "canonical/z"}
	if len(got.Imports) != len(wantImports) {
		t.Fatalf("canonicalized Imports = %v, want %v", got.Imports, wantImports)
	}
	for i := range wantImports {
		if got.Imports[i] != wantImports[i] {
			t.Fatalf("canonicalized Imports = %v, want %v", got.Imports, wantImports)
		}
	}
}
