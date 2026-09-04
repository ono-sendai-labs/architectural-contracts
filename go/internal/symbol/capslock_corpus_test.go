package symbol_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// corpusInventory is a CapslockInventory derived from a real type-checked
// package: every inventoried canonical SymbolID was produced by
// symbol.FromObject over the fixture's declared objects.
type corpusInventory struct {
	packages map[string]bool
	decls    map[string]bool // canonical SymbolIDs
}

func (c corpusInventory) KnownPackage(importPath string) bool { return c.packages[importPath] }

// Declares reports whether the inventory contains the top-level declaration
// pkg.Name or any method declared by the receiver type (pkg.Name).M.
func (c corpusInventory) Declares(pkg, name string) bool {
	if c.decls[pkg+"."+name] {
		return true
	}
	prefix := "(" + pkg + "." + name + ")."
	for id := range c.decls {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

// buildCorpusInventory parses and type-checks a real fixture package and
// converts every declared object to its canonical SymbolID, so the corpus
// rows originate from actual parsed/type-checked Go declarations (task req 7).
func buildCorpusInventory(t *testing.T, path, src string) corpusInventory {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	conf := types.Config{}
	pkg, err := conf.Check(path, fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("check fixture: %v", err)
	}
	inv := corpusInventory{packages: map[string]bool{path: true}, decls: map[string]bool{}}
	for _, obj := range info.Defs {
		if obj == nil || obj.Pkg() != pkg {
			continue
		}
		id, err := symbol.FromObject(obj)
		if err != nil {
			continue // fields and interface specs key their declaring type, which is Defs-visible itself
		}
		inv.decls[id.Format()] = true
	}
	if len(inv.decls) == 0 {
		t.Fatalf("fixture inventory is empty")
	}
	return inv
}

// TestCapslockCorpus is the differential regression corpus (AC5): positive
// rows are Capslock spellings of declarations of a real parsed/type-checked
// fixture package, and every successful normalization must resolve to an
// inventoried canonical SymbolID. Negative rows are syntactically invalid or
// unresolvable, including the deferred review cases (F3/F4), which are
// permanently pinned here.
func TestCapslockCorpus(t *testing.T) {
	const pkgPath = "example.com/ctest"
	inv := buildCorpusInventory(t, pkgPath, `package ctest

type Embed struct{}

type T struct{ Embed }

func (T) M() {}

func Load[K comparable, V any](k K, v V) (V, bool) { return v, true }
`)

	for _, tc := range []struct {
		name string
		fn   symbol.CapslockFunction
		want string
	}{
		{"deferred digit-leading type argument", symbol.CapslockFunction{Name: "ctest.Load[example.com/2x.T]", Package: pkgPath}, "example.com/ctest.Load"},
		{"deferred qualified unnamed parameter", symbol.CapslockFunction{Name: "ctest.Load[func(example.com/x.T)]", Package: pkgPath}, "example.com/ctest.Load"},
		{"deferred named variadic parameter", symbol.CapslockFunction{Name: "ctest.Load[func(x ...int)]", Package: pkgPath}, "example.com/ctest.Load"},
		{"qualified embedded struct field", symbol.CapslockFunction{Name: "ctest.Load[struct{example.com/ctest.T}]", Package: pkgPath}, "example.com/ctest.Load"},
		{"qualified interface method parameter", symbol.CapslockFunction{Name: "ctest.Load[interface{M(example.com/ctest.T) string}]", Package: pkgPath}, "example.com/ctest.Load"},
		{"and-not array length", symbol.CapslockFunction{Name: "ctest.Load[[N &^ 3]byte]", Package: pkgPath}, "example.com/ctest.Load"},
		{"deferred tuple type argument is rejected", symbol.CapslockFunction{Name: "ctest.Load[(int, string)]", Package: pkgPath}, ""},
		{"deferred named-list type argument is rejected", symbol.CapslockFunction{Name: "ctest.Load[(x int)]", Package: pkgPath}, ""},
		{"pointer receiver method", symbol.CapslockFunction{Name: "(*ctest.T).M", Package: pkgPath}, "(example.com/ctest.T).M"},
		{"value receiver method", symbol.CapslockFunction{Name: "(ctest.T).M", Package: pkgPath}, "(example.com/ctest.T).M"},
		{"structured field contradicts name", symbol.CapslockFunction{Name: "ctest.Load[int]", Package: "example.com/other"}, ""},
		{"declaration not in inventory", symbol.CapslockFunction{Name: "ctest.Nope", Package: pkgPath}, ""},
		{"syntactically invalid type argument", symbol.CapslockFunction{Name: "ctest.Load[2x.T]", Package: pkgPath}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := symbol.ParseCapslockFunction(tc.fn, inv)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("ParseCapslockFunction(%+v) = %q; want rejection", tc.fn, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCapslockFunction(%+v) error = %v", tc.fn, err)
			}
			if got.Format() != tc.want {
				t.Fatalf("ParseCapslockFunction(%+v) = %q; want %q", tc.fn, got, tc.want)
			}
			if _, err := symbol.Parse(got.Format()); err != nil {
				t.Fatalf("normalized result %q does not parse as a canonical SymbolID: %v", got, err)
			}
			// Every successful normalization resolves to an inventoried
			// canonical declaration.
			if got.Format() != tc.want || !inv.decls[tc.want] {
				t.Fatalf("result %q is not an inventoried canonical SymbolID", got)
			}
		})
	}
}
