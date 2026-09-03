package goanalysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

func TestLoadPackageFactsRejectsPatternMembershipBeforeLoading(t *testing.T) {
	originalLoadPackages := loadPackages
	t.Cleanup(func() { loadPackages = originalLoadPackages })
	loadPackages = func(*packages.Config, ...string) ([]*packages.Package, error) {
		t.Fatal("pattern membership reached packages.Load")
		return nil, nil
	}

	tests := []struct {
		name    string
		members []string
	}{
		{
			name:    "all patterns",
			members: []string{"example.com/runtime/*"},
		},
		{
			name:    "mixed literal and pattern",
			members: []string{"example.com/runtime", "example.com/runtime/*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadPackageFacts(LoadRequest{
				ComponentName: "pattern_surface_comp",
				ComponentRoot: t.TempDir(),
				Members:       tt.members,
			})
			if err == nil {
				t.Fatalf("LoadPackageFacts() error = nil, want pattern-membership load error")
			}
			for _, want := range append([]string{"pattern_surface_comp", "pattern membership"}, tt.members...) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want to contain %q", err, want)
				}
			}
		})
	}
}

func TestValidateLayoutMembershipSets(t *testing.T) {
	tests := []struct {
		name       string
		members    []string
		roots      []string
		wantErr    bool
		wantPieces []string
	}{
		{
			name:    "same set different order",
			members: []string{"canonical/b", "canonical/a"},
			roots:   []string{"canonical/a", "canonical/b"},
		},
		{
			name:       "missing layout root",
			members:    []string{"canonical/a", "canonical/b"},
			roots:      []string{"canonical/a"},
			wantErr:    true,
			wantPieces: []string{"canonical/a", "canonical/b", "missing from layout: [canonical/b]", "missing from manifest: []"},
		},
		{
			name:       "extra layout root",
			members:    []string{"canonical/a"},
			roots:      []string{"canonical/a", "canonical/b"},
			wantErr:    true,
			wantPieces: []string{"canonical/a", "canonical/b", "missing from layout: []", "missing from manifest: [canonical/b]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLayoutMembership(tt.members, tt.roots)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("validateLayoutMembership() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("validateLayoutMembership() succeeded for mismatched sets")
			}
			for _, piece := range tt.wantPieces {
				if !strings.Contains(err.Error(), piece) {
					t.Errorf("error %q does not contain %q", err, piece)
				}
			}
		})
	}
}

func TestValidateLayoutMembershipCanonicalizesWithoutMutatingInputs(t *testing.T) {
	originalPolicy := hostpolicy.CanonicalizePath
	t.Cleanup(func() { hostpolicy.CanonicalizePath = originalPolicy })
	hostpolicy.CanonicalizePath = func(path string) string {
		return strings.TrimPrefix(path, "host/")
	}
	members := []string{"host/a"}
	roots := []string{"a"}
	if err := validateLayoutMembership(members, roots); err != nil {
		t.Fatalf("validateLayoutMembership() error = %v", err)
	}
	if !reflect.DeepEqual(members, []string{"host/a"}) || !reflect.DeepEqual(roots, []string{"a"}) {
		t.Fatalf("validateLayoutMembership mutated inputs: members=%v roots=%v", members, roots)
	}
}

func TestLoadPackageFactsRejectsLayoutMembershipMismatchBeforeLoading(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(workspace, name+".go"), []byte("package "+name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	originalLoad := loadPackages
	t.Cleanup(func() { loadPackages = originalLoad })
	called := false
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		called = true
		return nil, nil
	}

	tests := []struct {
		name    string
		members []string
		roots   []string
		want    string
	}{
		{
			name:    "missing root",
			members: []string{"example.com/a", "example.com/b"},
			roots:   []string{"example.com/a"},
			want:    "missing from layout: [example.com/b]",
		},
		{
			name:    "extra root",
			members: []string{"example.com/a"},
			roots:   []string{"example.com/a", "example.com/b"},
			want:    "missing from manifest: [example.com/b]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			layoutPath := filepath.Join(workspace, tt.name+".json")
			packagesJSON := `{"id":"example.com/a","name":"a","pkgPath":"example.com/a","goFiles":["a.go"],"compiledGoFiles":["a.go"],"imports":{},"is_stdlib":false}`
			if len(tt.roots) == 2 {
				packagesJSON += `,{"id":"example.com/b","name":"b","pkgPath":"example.com/b","goFiles":["b.go"],"compiledGoFiles":["b.go"],"imports":{},"is_stdlib":false}`
			}
			data := fmt.Sprintf(`{"roots":["%s"],"packages":[%s]}`, strings.Join(tt.roots, `","`), packagesJSON)
			if err := os.WriteFile(layoutPath, []byte(data), 0644); err != nil {
				t.Fatal(err)
			}
			err := packagelayout.WithDriverEnv(layoutPath, workspace, func() error {
				_, err := LoadPackageFacts(LoadRequest{ComponentRoot: workspace, Members: tt.members})
				return err
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "manifest members") || !strings.Contains(err.Error(), "layout roots") {
				t.Fatalf("LoadPackageFacts() error = %v, want mismatch details including %q", err, tt.want)
			}
			if called {
				t.Fatal("loadPackages was called before layout membership mismatch was rejected")
			}
		})
	}
}

func TestLoadPackageFactsUsesMatchingLayoutRoots(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/layout\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range []string{"a", "b"} {
		dir := filepath.Join(workspace, pkg)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, pkg+".go"), []byte("package "+pkg+"\n\nfunc Exported() {}\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	layoutPath := filepath.Join(workspace, "package-layout.json")
	layout := `{
		"roots": ["example.com/layout/b", "example.com/layout/a"],
		"packages": [
			{"id":"example.com/layout/a","name":"a","pkgPath":"example.com/layout/a","is_stdlib":false,"goFiles":["a/a.go"],"compiledGoFiles":["a/a.go"],"imports":{}},
			{"id":"example.com/layout/b","name":"b","pkgPath":"example.com/layout/b","is_stdlib":false,"goFiles":["b/b.go"],"compiledGoFiles":["b/b.go"],"imports":{}}
		]
	}`
	if err := os.WriteFile(layoutPath, []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	originalLoad := loadPackages
	t.Cleanup(func() { loadPackages = originalLoad })
	var gotPatterns []string
	loadPackages = func(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
		gotPatterns = append([]string(nil), patterns...)
		return originalLoad(cfg, patterns...)
	}

	var loaded facts.PackageFacts
	err := packagelayout.WithDriverEnv(layoutPath, workspace, func() error {
		var err error
		loaded, err = LoadPackageFacts(LoadRequest{
			ComponentRoot: workspace,
			Members:       []string{"example.com/layout/a", "example.com/layout/b"},
		})
		return err
	})
	if err != nil {
		t.Fatalf("LoadPackageFacts() error = %v", err)
	}

	if !reflect.DeepEqual(gotPatterns, []string{"example.com/layout/b", "example.com/layout/a"}) {
		t.Fatalf("package load patterns = %v, want active layout roots in layout order", gotPatterns)
	}
	var gotPaths []string
	for _, pkg := range loaded.Packages {
		gotPaths = append(gotPaths, pkg.ImportPath)
	}
	sort.Strings(gotPaths)
	wantPaths := []string{"example.com/layout/a", "example.com/layout/b"}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("loaded package facts = %v, want exactly the active layout roots %v", gotPaths, wantPaths)
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
		{name: "receiver without method", in: "(*a/b.T[c/d.U])", want: "(*canonical/a/b.T[canonical/c/d.U])"},
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
	apiSource := "package dep\ntype t struct{}\nfunc (t) Method() {}\nfunc Exported() {}\n"
	if err := os.WriteFile(apiPath, []byte(apiSource), 0644); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, apiPath, apiSource, 0)
	if err != nil {
		t.Fatal(err)
	}
	typesInfo := &types.Info{Defs: make(map[*ast.Ident]types.Object)}
	typesPkg, err := (&types.Config{}).Check("host/dep", fset, []*ast.File{file}, typesInfo)
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
		switch path {
		case "host/dep":
			return "canonical/dep"
		case "host/import":
			return "canonical/import"
		}
		return path
	}
	loadedPackage := &packages.Package{
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
	}
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{loadedPackage}, nil
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
	wantSymbols := map[string]bool{
		"canonical/dep.Exported":   true,
		"(canonical/dep.t).Method": true,
	}
	if len(result.Symbols) != len(wantSymbols) {
		t.Fatalf("result symbols = %v, want %v", result.Symbols, wantSymbols)
	}
	for _, symbol := range result.Symbols {
		if !wantSymbols[string(symbol)] {
			t.Fatalf("result symbols = %v, unexpected symbol %q", result.Symbols, symbol)
		}
	}
	extracted, err := extractSymbols(loadedPackage, depRoot)
	if err != nil {
		t.Fatalf("extractSymbols() error = %v", err)
	}
	var method facts.ExportedSymbol
	for _, symbol := range extracted {
		if symbol.Kind == "method" && symbol.Name == "(canonical/dep.t).Method" {
			method = symbol
			break
		}
	}
	if method.Name != "(canonical/dep.t).Method" || method.Receiver != "(canonical/dep.t)" {
		t.Fatalf("method symbol = %+v, want canonical name and receiver", method)
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

func TestResolveDependencyInterface_PackageSurface_LayoutMismatch(t *testing.T) {
	workspace := t.TempDir()
	declaringRoot := filepath.Join(workspace, "declaring")
	depRoot := filepath.Join(workspace, "dep")
	if err := os.MkdirAll(declaringRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(depRoot, 0755); err != nil {
		t.Fatal(err)
	}
	manifestContent := `name: "dep"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/layout/a"
`
	if err := os.WriteFile(filepath.Join(depRoot, "component.textproto"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depRoot, "b.go"), []byte("package b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	layoutPath := filepath.Join(depRoot, "package-layout.json")
	layout := `{
		"roots": ["example.com/layout/b"],
		"packages": [
			{"id":"example.com/layout/b","name":"b","pkgPath":"example.com/layout/b","is_stdlib":false,"goFiles":["dep/b.go"],"compiledGoFiles":["dep/b.go"],"imports":{}}
		]
	}`
	if err := os.WriteFile(layoutPath, []byte(layout), 0644); err != nil {
		t.Fatal(err)
	}

	err := packagelayout.WithDriverEnv(layoutPath, workspace, func() error {
		_, err := ResolveDependencyInterface(declaringRoot, declaringRoot, manifest.ComponentDependency{
			Name:     "dep",
			Manifest: "../dep/component.textproto",
		})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "package layout roots do not match manifest members") {
		t.Fatalf("ResolveDependencyInterface() error = %v, want layout roots mismatch error", err)
	}
}

func TestResolveDependencyInterface_PackageSurface_Canonicalization(t *testing.T) {
	workspace := t.TempDir()
	declaringRoot := filepath.Join(workspace, "declaring")
	depRoot := filepath.Join(workspace, "dep")
	if err := os.MkdirAll(declaringRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(depRoot, 0755); err != nil {
		t.Fatal(err)
	}
	manifestContent := `name: "dep"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "host/dep"
`
	if err := os.WriteFile(filepath.Join(depRoot, "component.textproto"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}
	apiPath := filepath.Join(depRoot, "api.go")
	if err := os.WriteFile(apiPath, []byte("package dep\n\nfunc Exported() {}\n"), 0644); err != nil {
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
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, apiPath, "package dep\n\nfunc Exported() {}\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	typesPkg := types.NewPackage("canonical/dep", "dep")
	typesPkg.Scope().Insert(types.NewFunc(token.NoPos, typesPkg, "Exported", types.NewSignatureType(nil, nil, nil, nil, nil, false)))

	loadedPackage := &packages.Package{
		ID:        "canonical/dep",
		PkgPath:   "canonical/dep",
		Name:      "dep",
		GoFiles:   []string{apiPath},
		Fset:      fset,
		Syntax:    []*ast.File{file},
		Types:     typesPkg,
		TypesInfo: &types.Info{},
		Imports:   map[string]*packages.Package{},
	}
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{loadedPackage}, nil
	}

	result, err := ResolveDependencyInterface(declaringRoot, declaringRoot, manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/component.textproto",
	})
	if err != nil {
		t.Fatalf("ResolveDependencyInterface() error = %v", err)
	}
	if len(result.Packages) != 1 || result.Packages[0] != "canonical/dep" {
		t.Fatalf("result packages = %v, want ['canonical/dep']", result.Packages)
	}
	found := false
	for _, sym := range result.Symbols {
		if sym == "canonical/dep.Exported" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("result symbols = %v, missing 'canonical/dep.Exported'", result.Symbols)
	}
}

func TestResolveDependencyInterface_PatternMembership_Canonicalization(t *testing.T) {
	workspace := t.TempDir()
	declaringRoot := filepath.Join(workspace, "declaring")
	depRoot := filepath.Join(workspace, "dep")
	if err := os.MkdirAll(declaringRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(depRoot, 0755); err != nil {
		t.Fatal(err)
	}
	manifestContent := `name: "dep"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "host/*"
`
	if err := os.WriteFile(filepath.Join(depRoot, "component.textproto"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}
	apiPath := filepath.Join(declaringRoot, "api.go")
	if err := os.WriteFile(apiPath, []byte("package dep\n\nfunc Exported() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	originalPolicy := hostpolicy.CanonicalizePath
	originalLoad := loadPackages
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = originalPolicy
		loadPackages = originalLoad
	})
	hostpolicy.CanonicalizePath = func(path string) string {
		if strings.HasPrefix(path, "host/") {
			return "canonical/" + strings.TrimPrefix(path, "host/")
		}
		return path
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, apiPath, "package dep\n\nfunc Exported() {}\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	typesPkg := types.NewPackage("canonical/dep", "dep")
	typesPkg.Scope().Insert(types.NewFunc(token.NoPos, typesPkg, "Exported", types.NewSignatureType(nil, nil, nil, nil, nil, false)))

	loadedPackage := &packages.Package{
		ID:        "canonical/dep",
		PkgPath:   "canonical/dep",
		Name:      "dep",
		GoFiles:   []string{apiPath},
		Fset:      fset,
		Syntax:    []*ast.File{file},
		Types:     typesPkg,
		TypesInfo: &types.Info{},
		Imports:   map[string]*packages.Package{},
	}
	loadPackages = func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return []*packages.Package{loadedPackage}, nil
	}

	result, err := ResolveDependencyInterface(declaringRoot, declaringRoot, manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/component.textproto",
	})
	if err != nil {
		t.Fatalf("ResolveDependencyInterface() error = %v", err)
	}
	if len(result.Packages) != 1 || result.Packages[0] != "canonical/dep" {
		t.Fatalf("result packages = %v, want ['canonical/dep']", result.Packages)
	}
	found := false
	for _, sym := range result.Symbols {
		if sym == "canonical/dep.Exported" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("result symbols = %v, missing 'canonical/dep.Exported'", result.Symbols)
	}
}
