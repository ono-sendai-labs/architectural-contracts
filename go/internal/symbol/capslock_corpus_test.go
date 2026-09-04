package symbol_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// corpusInventory is a CapslockInventory derived from real parsed/type-checked
// declarations: every inventoried canonical SymbolID was produced by
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

func (c corpusInventory) DeclaresMethod(pkg, typeName, method string) bool {
	return c.decls["("+pkg+"."+typeName+")."+method]
}

// qualifierPath renders package qualifiers as full import paths, the shape
// the Capslock/SSA formatter prints.
func qualifierPath(p *types.Package) string { return p.Path() }

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

// corpusFixtureTypes builds real typed declarations for the corpus's
// type-argument forms: named types in digit-leading and dotted module-host
// packages, and function/struct/interface types whose go/types rendering is
// the exact Capslock formatter spelling.
func corpusFixtureTypes(t *testing.T) (*types.Package, *types.Named) {
	t.Helper()
	pkgx := types.NewPackage("example.com/x", "x")
	nx := types.NewNamed(types.NewTypeName(token.NoPos, pkgx, "T", nil), types.NewStruct(nil, nil), nil)
	pkgx.MarkComplete()
	return pkgx, nx
}

// corpusAndNotArray prints the array type of a real parsed/type-checked
// declaration whose length uses the &^ constant-expression operator, so the
// rendered text originates from a checked declaration rather than being
// hand-authored.
func corpusAndNotArray(t *testing.T) string {
	t.Helper()
	const src = `package ctestarray

const N = 13

type wide [N &^ 3]byte
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "array.go", src, 0)
	if err != nil {
		t.Fatalf("parse array fixture: %v", err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	conf := types.Config{}
	if _, err := conf.Check("example.com/ctestarray", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("check array fixture: %v", err)
	}
	var arrayType ast.Expr
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		spec := gd.Specs[0].(*ast.TypeSpec)
		if spec.Name.Name == "wide" {
			arrayType = spec.Type
		}
	}
	if arrayType == nil {
		t.Fatalf("fixture array type not found")
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, arrayType); err != nil {
		t.Fatalf("print array type: %v", err)
	}
	return buf.String()
}

// TestCapslockCorpus is the differential regression corpus (AC5): positive
// rows are rendered from real parsed and type-checked declarations — typed
// go/types values rendered with the formatter's full-import-path qualifier,
// and the printed type expression of a checked &^ array declaration — and
// every successful normalization must resolve to an inventoried canonical
// SymbolID. The deferred review spellings are pinned by asserting the
// generated text matches them exactly. Negative rows are syntactically
// invalid or unresolvable, including the deferred rejection cases (F3/F4).
func TestCapslockCorpus(t *testing.T) {
	const pkgPath = "example.com/ctest"
	inv := buildCorpusInventory(t, pkgPath, `package ctest

type Embed struct{}

type T struct{ Embed }

func (T) M() {}

func Load[K comparable, V any](k K, v V) (V, bool) { return v, true }
`)

	pkgx, nx := corpusFixtureTypes(t)
	pkg2x := types.NewPackage("example.com/2x", "2x")
	t2x := types.NewNamed(types.NewTypeName(token.NoPos, pkg2x, "T", nil), types.NewStruct(nil, nil), nil)
	pkg2x.MarkComplete()
	// The type-argument packages are part of the caller's world, as Capslock's
	// full import-path spellings imply.
	inv.packages[pkg2x.Path()] = true
	inv.packages[pkgx.Path()] = true

	andNotArray := corpusAndNotArray(t)
	if andNotArray != "[N &^ 3]byte" {
		t.Fatalf("checked array type rendered %q; want the deferred &^ spelling", andNotArray)
	}

	// Type-argument texts rendered from typed declarations.
	qualified := types.TypeString(nx, qualifierPath)
	if qualified != "example.com/x.T" {
		t.Fatalf("rendered qualified type %q; want the deferred spelling", qualified)
	}
	digitLeading := types.TypeString(t2x, qualifierPath)
	if digitLeading != "example.com/2x.T" {
		t.Fatalf("rendered digit-leading type %q; want the deferred spelling", digitLeading)
	}
	qualifiedUnnamedParam := types.TypeString(types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, pkgx, "", nx)), nil, false), qualifierPath)
	if qualifiedUnnamedParam != "func(example.com/x.T)" {
		t.Fatalf("rendered signature %q; want the deferred spelling", qualifiedUnnamedParam)
	}
	namedVariadic := types.TypeString(types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, pkgx, "x", types.NewSlice(types.Typ[types.Int]))), nil, true), qualifierPath)
	if namedVariadic != "func(x ...int)" {
		t.Fatalf("rendered signature %q; want the deferred spelling", namedVariadic)
	}
	qualifiedEmbedded := types.TypeString(types.NewStruct(
		[]*types.Var{types.NewField(token.NoPos, pkg2x, "", t2x, true)}, nil), qualifierPath)
	if qualifiedEmbedded != "struct{example.com/2x.T}" {
		t.Fatalf("rendered struct %q; want the deferred spelling", qualifiedEmbedded)
	}
	ifaceParam := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, pkgx, "", nx)),
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String])), false)
	iface := types.NewInterfaceType([]*types.Func{types.NewFunc(token.NoPos, pkgx, "M", ifaceParam)}, nil)
	qualifiedIfaceParam := types.TypeString(iface, qualifierPath)
	if qualifiedIfaceParam != "interface{M(example.com/x.T) string}" {
		t.Fatalf("rendered interface %q; want the deferred spelling", qualifiedIfaceParam)
	}

	for _, tc := range []struct {
		name string
		fn   symbol.CapslockFunction
		want string
	}{
		{"deferred digit-leading type argument", symbol.CapslockFunction{Name: "ctest.Load[" + digitLeading + "]", Package: pkgPath}, "example.com/ctest.Load"},
		{"deferred qualified unnamed parameter", symbol.CapslockFunction{Name: "ctest.Load[" + qualifiedUnnamedParam + "]", Package: pkgPath}, "example.com/ctest.Load"},
		{"deferred named variadic parameter", symbol.CapslockFunction{Name: "ctest.Load[" + namedVariadic + "]", Package: pkgPath}, "example.com/ctest.Load"},
		{"qualified embedded struct field", symbol.CapslockFunction{Name: "ctest.Load[" + qualifiedEmbedded + "]", Package: pkgPath}, "example.com/ctest.Load"},
		{"qualified interface method parameter", symbol.CapslockFunction{Name: "ctest.Load[" + qualifiedIfaceParam + "]", Package: pkgPath}, "example.com/ctest.Load"},
		{"and-not array length from checked declaration", symbol.CapslockFunction{Name: "ctest.Load[" + andNotArray + "]", Package: pkgPath}, "example.com/ctest.Load"},
		{"deferred tuple type argument is rejected", symbol.CapslockFunction{Name: "ctest.Load[(int, string)]", Package: pkgPath}, ""},
		{"deferred named-list type argument is rejected", symbol.CapslockFunction{Name: "ctest.Load[(x int)]", Package: pkgPath}, ""},
		{"pointer receiver method", symbol.CapslockFunction{Name: "(*ctest.T).M", Package: pkgPath}, "(example.com/ctest.T).M"},
		{"value receiver method", symbol.CapslockFunction{Name: "(ctest.T).M", Package: pkgPath}, "(example.com/ctest.T).M"},
		{"method on inventoried receiver not declared", symbol.CapslockFunction{Name: "(*ctest.T).Nope", Package: pkgPath}, ""},
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
			if !inv.decls[tc.want] {
				t.Fatalf("result %q is not an inventoried canonical SymbolID", got)
			}
		})
	}
}

func qualifiedType(pkg *types.Package) types.Type {
	return pkg.Scope().Lookup("T").Type()
}
