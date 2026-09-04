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
	methods  map[string]string
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

// MethodOwner resolves a method spelling to its canonical declaring-object
// ID: a concrete method's "(pkg.T).Method", or, per the declaring-object
// rule, the declaring interface "pkg.Interface" for an interface method
// spec.
func (c corpusInventory) MethodOwner(pkg, typeName, method string) (string, bool) {
	owner, ok := c.methods[pkg+"."+typeName+"."+method]
	return owner, ok
}

// addMethod records the declaring-object ID of a checked method: its receiver
// base named type's package and name key the spelling, and symbol.FromObject
// yields the canonical owner (the method ID, or the declaring interface for
// an interface method spec).
func (c corpusInventory) addMethod(fn *types.Func) {
	sig := fn.Type().(*types.Signature)
	base := types.Unalias(sig.Recv().Type())
	if ptr, ok := base.(*types.Pointer); ok {
		base = types.Unalias(ptr.Elem())
	}
	named := base.(*types.Named)
	id, err := symbol.FromObject(fn)
	if err != nil {
		return
	}
	c.methods[named.Obj().Pkg().Path()+"."+named.Obj().Name()+"."+fn.Name()] = id.Format()
}

// typeCheckSource parses and type-checks one fixture package, optionally
// resolving imports from a map of pre-checked fixture packages.
func typeCheckSource(t *testing.T, path, src string, importer types.Importer) (*types.Package, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture %q: %v", path, err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: importer}
	pkg, err := conf.Check(path, fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("check fixture %q: %v", path, err)
	}
	return pkg, info
}

type mapImporter map[string]*types.Package

func (m mapImporter) Import(path string) (*types.Package, error) {
	if p, ok := m[path]; ok {
		return p, nil
	}
	return nil, &importError{path: path}
}

type importError struct{ path string }

func (e *importError) Error() string { return "no fixture package " + e.path }

// fixtureObject returns the checked object named name in pkg.
func fixtureObject(info *types.Info, pkg *types.Package, name string) types.Object {
	for _, obj := range info.Defs {
		if obj != nil && obj.Pkg() == pkg && obj.Name() == name {
			return obj
		}
	}
	return nil
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

// TestCapslockCorpus is the differential regression corpus (AC5). Every
// positive type-argument text is rendered from real parsed and type-checked
// declarations: the imported fixture packages example.com/x and example.com/2x
// (digit-leading import-path element) supply the qualified named types, and
// ctest itself declares the function, struct, interface and bare-embedded
// forms that are rendered with the Capslock formatter's full-import-path
// qualifier; the &^ array length is printed from a checked declaration's AST.
// Every successful normalization must resolve to an inventoried canonical
// SymbolID, and the deferred review spellings are pinned by asserting the
// generated text matches them exactly. Negative rows are syntactically
// invalid or unresolvable.
func TestCapslockCorpus(t *testing.T) {
	// Imported fixture packages, checked first.
	pkg2x, info2x := typeCheckSource(t, "example.com/2x", `package twoway

type T struct{}
`, nil)
	pkgx, infox := typeCheckSource(t, "example.com/x", `package xpkg

type T struct{}
`, nil)

	const pkgPath = "example.com/ctest"
	pkg, info := typeCheckSource(t, pkgPath, `package ctest

import (
	two "example.com/2x"
	x "example.com/x"
)

type Embed struct{}

type T struct{ Embed }

func (T) M() {}

type Iface interface{ Wait() }

func Load[K comparable, V any](k K, v V) (V, bool) { return v, true }

type qualStruct struct{ two.T }

type qualFunc func(x.T)

type variadicFunc func(x ...int)

type ifaceParamT interface{ M(x.T) string }

type bareStruct struct{ T }
`, mapImporter{pkg2x.Path(): pkg2x, pkgx.Path(): pkgx})

	inv := corpusInventory{
		packages: map[string]bool{pkgPath: true, pkg2x.Path(): true, pkgx.Path(): true},
		decls:    map[string]bool{},
		methods:  map[string]string{},
	}
	for _, obj := range info.Defs {
		if obj == nil || obj.Pkg() != pkg {
			continue
		}
		if id, err := symbol.FromObject(obj); err == nil {
			inv.decls[id.Format()] = true
		}
		if fn, ok := obj.(*types.Func); ok {
			if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
				inv.addMethod(fn)
			}
		}
	}

	// Positive type-argument texts, rendered from the checked declarations
	// with the formatter's full-import-path qualifier.
	qualifierPath := func(p *types.Package) string { return p.Path() }
	digitLeading := types.TypeString(fixtureObject(info2x, pkg2x, "T").Type(), qualifierPath)
	if digitLeading != "example.com/2x.T" {
		t.Fatalf("rendered digit-leading type %q; want the deferred spelling", digitLeading)
	}
	qualified := types.TypeString(fixtureObject(infox, pkgx, "T").Type(), qualifierPath)
	if qualified != "example.com/x.T" {
		t.Fatalf("rendered qualified type %q; want the deferred spelling", qualified)
	}
	qualifiedUnnamedParam := types.TypeString(fixtureObject(info, pkg, "qualFunc").Type().Underlying(), qualifierPath)
	if qualifiedUnnamedParam != "func(example.com/x.T)" {
		t.Fatalf("rendered signature %q; want the deferred spelling", qualifiedUnnamedParam)
	}
	namedVariadic := types.TypeString(fixtureObject(info, pkg, "variadicFunc").Type().Underlying(), qualifierPath)
	if namedVariadic != "func(x ...int)" {
		t.Fatalf("rendered signature %q; want the deferred spelling", namedVariadic)
	}
	qualifiedEmbedded := types.TypeString(fixtureObject(info, pkg, "qualStruct").Type().Underlying(), qualifierPath)
	if qualifiedEmbedded != "struct{example.com/2x.T}" {
		t.Fatalf("rendered struct %q; want the deferred spelling", qualifiedEmbedded)
	}
	qualifiedIfaceParam := types.TypeString(fixtureObject(info, pkg, "ifaceParamT").Type().Underlying(), qualifierPath)
	if qualifiedIfaceParam != "interface{M(example.com/x.T) string}" {
		t.Fatalf("rendered interface %q; want the deferred spelling", qualifiedIfaceParam)
	}
	// A same-package embedded field is printed bare by the formatter, so the
	// relative qualifier drops the fixture's own package.
	bareEmbedded := types.TypeString(fixtureObject(info, pkg, "bareStruct").Type().Underlying(),
		func(p *types.Package) string {
			if p == pkg {
				return ""
			}
			return p.Path()
		})
	if bareEmbedded != "struct{T}" {
		t.Fatalf("rendered struct %q; want the bare embedded form", bareEmbedded)
	}
	andNotArray := corpusAndNotArray(t)
	if andNotArray != "[N &^ 3]byte" {
		t.Fatalf("checked array type rendered %q; want the deferred &^ spelling", andNotArray)
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
		{"bare embedded struct field", symbol.CapslockFunction{Name: "ctest.Load[" + bareEmbedded + "]", Package: pkgPath}, "example.com/ctest.Load"},
		{"and-not array length from checked declaration", symbol.CapslockFunction{Name: "ctest.Load[" + andNotArray + "]", Package: pkgPath}, "example.com/ctest.Load"},
		{"deferred tuple type argument is rejected", symbol.CapslockFunction{Name: "ctest.Load[(int, string)]", Package: pkgPath}, ""},
		{"deferred named-list type argument is rejected", symbol.CapslockFunction{Name: "ctest.Load[(x int)]", Package: pkgPath}, ""},
		{"pointer receiver method", symbol.CapslockFunction{Name: "(*ctest.T).M", Package: pkgPath}, "(example.com/ctest.T).M"},
		{"value receiver method", symbol.CapslockFunction{Name: "(ctest.T).M", Package: pkgPath}, "(example.com/ctest.T).M"},
		{"interface method spec resolves to its declaring interface", symbol.CapslockFunction{Name: "(*ctest.Iface).Wait", Package: pkgPath}, "example.com/ctest.Iface"},
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
