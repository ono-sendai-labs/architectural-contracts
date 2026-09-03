package symbol_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// importerDefault wraps importer.Default so fixtures resolve cleanly.
func importerDefault() types.Importer { return importer.Default() }

// parsePackage parses src into one file and type-checks it with the given
// importer, returning the pieces ExtractSurface needs.
func parsePackage(t *testing.T, path, src string, imp types.Importer) ([]*ast.File, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	info := &types.Info{
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	conf := types.Config{Importer: imp}
	if _, err := conf.Check(path, fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("check %s: %v", path, err)
	}
	return []*ast.File{file}, info
}

// TestExtractSurface_ExactDeclared pins the exact declared surface (task req
// 6): exported package-level objects and methods, pkg.init, and alias
// expansion; no implements-closure injection, no unexported names except the
// alias targets needed for member resolution.
func TestExtractSurface_ExactDeclared(t *testing.T) {
	src := `package surface

type target struct {
	F int
}

func (target) Hit() {}

type DB = target

type Store struct{ X int }

func (s *Store) Get() string   { return "" }
func (s Store) Put(v string) {}

type Greeter interface{ Greet() }

const MaxSize = 1

var DefaultLimit = MaxSize

func Hello() {}

func init() {}
`
	files, info := parsePackage(t, "example.com/surface", src, importerDefault())
	got := symbol.ExtractSurface(files, info)
	want := []symbol.SymbolID{
		"(example.com/surface.Store).Get",
		"(example.com/surface.Store).Put",
		"(example.com/surface.target).Hit",
		"example.com/surface.DB", // the alias
		"example.com/surface.DefaultLimit",
		"example.com/surface.Greeter",
		"example.com/surface.Hello",
		"example.com/surface.MaxSize",
		"example.com/surface.Store",
		"example.com/surface.init",
		"example.com/surface.target",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractSurface() = %v; want %v", got, want)
	}
}

// importerWithForeign resolves the fixture's cross-package import to the
// checked foreign fixture package.
func importerWithForeign(t *testing.T) types.Importer {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "foreign.go", foreignFixture, 0)
	if err != nil {
		t.Fatalf("parse foreign fixture: %v", err)
	}
	conf := types.Config{}
	pkg, err := conf.Check("example.com/foreign", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatalf("check foreign fixture: %v", err)
	}
	return importerFunc(func(path string) (*types.Package, error) {
		if path == "example.com/foreign" {
			return pkg, nil
		}
		return importer.Default().Import(path)
	})
}

// TestExtractSurface_CrossPackageAliasBounded pins task req 5's bound: a
// cross-package alias emits only the alias key; the foreign target is never
// claimed.
func TestExtractSurface_CrossPackageAliasBounded(t *testing.T) {
	src := `package bounded

import "example.com/foreign"

type Remote = foreign.Thing
`
	files, info := parsePackage(t, "example.com/bounded", src, importerWithForeign(t))
	got := symbol.ExtractSurface(files, info)
	want := []symbol.SymbolID{"example.com/bounded.Remote"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractSurface() = %v; want %v (no foreign target)", got, want)
	}
}

// TestExtractSurface_Deterministic pins task req 6 / AC 5: repeated or
// reordered observations collapse to one sorted, duplicate-free result.
func TestExtractSurface_Deterministic(t *testing.T) {
	src := `package det

type A struct{}

func (A) M() {}
func (A) N() {}

func init() {}
func init() {}

func F() {}
`
	files, info := parsePackage(t, "example.com/det", src, importerDefault())
	first := symbol.ExtractSurface(files, info)
	second := symbol.ExtractSurface(files, info)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated extraction differs: %v vs %v", first, second)
	}
	// Reverse the file order; the result must be identical.
	reversed := []*ast.File{files[len(files)-1], files[0]}
	third := symbol.ExtractSurface(reversed, info)
	if !reflect.DeepEqual(third, first) {
		t.Fatalf("reversed extraction differs: %v vs %v", third, first)
	}
	want := []symbol.SymbolID{
		"(example.com/det.A).M",
		"(example.com/det.A).N",
		"example.com/det.A",
		"example.com/det.F",
		"example.com/det.init",
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("ExtractSurface() = %v; want %v (sorted, duplicate-free, one init)", first, want)
	}
}

// TestExtractSurface_CanonicalizesNamespace pins task req 6's
// canonicalization hook: package paths pass through hostpolicy.CanonicalizePath.
func TestExtractSurface_CanonicalizesNamespace(t *testing.T) {
	orig := hostpolicy.CanonicalizePath
	defer func() { hostpolicy.CanonicalizePath = orig }()
	hostpolicy.CanonicalizePath = func(p string) string {
		return "canonical.example/" + p
	}
	src := "package raw\n\nfunc F() {}\n"
	files, info := parsePackage(t, "raw", src, importerDefault())
	got := symbol.ExtractSurface(files, info)
	want := []symbol.SymbolID{"canonical.example/raw.F"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractSurface() = %v; want %v", got, want)
	}
}
