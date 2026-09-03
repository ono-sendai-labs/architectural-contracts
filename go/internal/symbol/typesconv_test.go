package symbol_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// Fixtures for the declaring-object rule (DR-04, task req 4): one small
// self-contained package per scenario group, type-checked in-test so the
// tests stay fast and hermetic under Bazel.

const storeFixture = `package store

import "example.com/foreign"

type Cache struct {
	Entries map[string]string
}

func (c Cache) Size() int { return len(c.Entries) }

func (c *Cache) Clear() { c.Entries = nil }

type Store struct {
	items []string
}

func (s Store) Get() string   { return "" }
func (s *Store) Put(v string) {}

type Box[T any] struct {
	V T
}

func (b Box[T]) Unwrap() T { return b.V }

type Widget struct {
	Cache
}

type Greeter interface {
	Greet()
}

type Greeter2 interface {
	Greeter
	Bye()
}

type DB = Store

func (DB) Touch() {}

type Remote = foreign.Thing

type record struct {
	f int
}

const MaxSize = 100

var DefaultLimit = MaxSize

func Hello() {}

func init() {}
`

const foreignFixture = `package foreign

type Thing struct {
	N int
}

func (t Thing) Value() int { return t.N }
`

const storeUseFixture = `package store

func useAll() {
	s := &Store{}
	_ = s.Get()
	s.Put("x")
	c := &Cache{}
	_ = c.Size()
	c.Clear()
	b := Box[int]{}
	_ = b.Unwrap()
	_ = b.V
	w := Widget{}
	_ = w.Entries
	_ = w.Size()
	var g Greeter = &greeterImpl{}
	g.Greet()
	var g2 Greeter2 = &greeterImpl{}
	g2.Greet()
	g2.Bye()
	var db DB
	db.Touch()
	th := Remote{}
	_ = th.Value()
	_ = DefaultLimit
	_ = Hello
	_ = MaxSize
}

type greeterImpl struct{}

func (greeterImpl) Greet()  {}
func (greeterImpl) Bye()    {}
func (greeterImpl) Greet2() {}
`

// typeCheckFixtures parses and type-checks the fixture packages, wiring the
// store package's foreign alias import to the foreign fixture, and returns
// the shared types.Info.
func typeCheckFixtures(t *testing.T) *types.Info {
	t.Helper()
	fset := token.NewFileSet()
	parse := func(name, src string) *ast.File {
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		return f
	}
	ffile := parse("foreign.go", foreignFixture)
	sfile := parse("store.go", storeFixture)
	ufile := parse("store_use.go", storeUseFixture)
	info := &types.Info{
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Types:      map[ast.Expr]types.TypeAndValue{},
	}
	conf := types.Config{}
	foreign, err := conf.Check("example.com/foreign", fset, []*ast.File{ffile}, info)
	if err != nil {
		t.Fatalf("check foreign fixture: %v", err)
	}
	conf = types.Config{Importer: importerFunc(func(path string) (*types.Package, error) {
		if path == "example.com/foreign" {
			return foreign, nil
		}
		return importer.Default().Import(path)
	})}
	if _, err := conf.Check("example.com/store", fset, []*ast.File{sfile, ufile}, info); err != nil {
		t.Fatalf("check store fixture: %v", err)
	}
	return info
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

// TestFromObject_GrammarTable walks every object declared by the fixture and
// pins the ID the declaring-object rule produces for each kind: funcs, vars,
// consts, named and alias types, methods (pointer/value/alias/generic
// origin), interface method specs, struct fields and init.
func TestFromObject_GrammarTable(t *testing.T) {
	info := typeCheckFixtures(t)

	got := map[symbol.SymbolID]bool{}
	failed := map[string]bool{}
	for _, obj := range info.Defs {
		if obj == nil || obj.Pkg() == nil {
			continue
		}
		id, err := symbol.FromObject(obj)
		if err != nil {
			failed[obj.Name()] = true
			continue
		}
		got[id] = true
	}

	want := []string{
		// package-level objects key themselves
		"example.com/store.Hello",
		"example.com/store.DefaultLimit",
		"example.com/store.MaxSize",
		"example.com/store.Store",
		"example.com/store.Cache",
		"example.com/store.Box",
		"example.com/store.Widget",
		"example.com/store.Greeter",
		"example.com/store.Greeter2",
		"example.com/store.record",
		"example.com/store.greeterImpl",
		// aliases have their own top-level ID
		"example.com/store.DB",
		"example.com/store.Remote",
		"example.com/store.useAll",
		"example.com/store.init",
		// declared methods: receiver base name, no pointer, no type args
		"(example.com/store.Store).Get",
		"(example.com/store.Store).Put",
		"(example.com/store.Cache).Size",
		"(example.com/store.Cache).Clear",
		"(example.com/store.Box).Unwrap",
		"(example.com/store.Store).Touch", // declared through the DB alias
		"(example.com/store.greeterImpl).Greet",
		"(example.com/store.greeterImpl).Bye",
		"(example.com/store.greeterImpl).Greet2",
		// interface method specs key the declaring interface
		"example.com/store.Greeter",  // Greet spec
		"example.com/store.Greeter2", // Bye spec
		// fields key the declaring struct type
		"example.com/store.Store",  // items
		"example.com/store.Cache",  // Entries
		"example.com/store.Box",    // V
		"example.com/store.record", // f
		"example.com/store.Widget", // the embedded Cache field itself
		// the foreign package
		"example.com/foreign.Thing",
		"(example.com/foreign.Thing).Value", // method
		"example.com/foreign.Thing",         // field N
	}
	wantSet := map[symbol.SymbolID]bool{}
	for _, w := range want {
		wantSet[symbol.SymbolID(w)] = true
	}
	if len(got) != len(wantSet) {
		t.Fatalf("FromObject set mismatch:\n got extra: %v\n missing: %v",
			minus(got, wantSet), minus(wantSet, got))
	}
	for id := range wantSet {
		if !got[id] {
			t.Errorf("missing ID %q", id)
		}
	}

	// Objects that are not declared symbols — local variables and the type
	// parameter T — are rejected with an actionable error.
	wantFailed := []string{"s", "c", "b", "w", "g", "g2", "db", "th", "T"}
	for _, name := range wantFailed {
		if !failed[name] {
			t.Errorf("expected FromObject to reject %q, but it converted", name)
		}
	}
}

func minus(a, b map[symbol.SymbolID]bool) []string {
	var out []string
	for id := range a {
		if !b[id] {
			out = append(out, string(id))
		}
	}
	sort.Strings(out)
	return out
}

// TestFromSelection_DeclaringObject pins selection conversion: fields and
// interface method specs map to their declaring type, promotions to the
// actual declaring member, and generic instantiations to their origin.
func TestFromSelection_DeclaringObject(t *testing.T) {
	info := typeCheckFixtures(t)

	got := map[symbol.SymbolID]bool{}
	for _, sel := range info.Selections {
		id, err := symbol.FromSelection(sel)
		if err != nil {
			t.Fatalf("FromSelection(%v) error = %v", sel, err)
		}
		got[id] = true
	}

	want := []string{
		// direct methods
		"(example.com/store.Store).Get",
		"(example.com/store.Store).Put",
		"(example.com/store.Cache).Clear",
		"(example.com/store.Box).Unwrap", // generic origin, not Box[int]
		"(example.com/store.Store).Touch",
		"(example.com/foreign.Thing).Value",
		// fields: declaring struct type
		"example.com/store.Cache",   // promoted w.Entries
		"example.com/store.Box",     // b.V
		"example.com/foreign.Thing", // field N, selected inside the fixture
		// promoted method: the actual declaring member
		"(example.com/store.Cache).Size",
		// interface method specs: declaring interface
		"example.com/store.Greeter",  // g.Greet and promoted g2.Greet
		"example.com/store.Greeter2", // g2.Bye
	}
	wantSet := map[symbol.SymbolID]bool{}
	for _, w := range want {
		wantSet[symbol.SymbolID(w)] = true
	}
	if len(got) != len(wantSet) {
		t.Fatalf("FromSelection set mismatch:\n got extra: %v\n missing: %v",
			minus(got, wantSet), minus(wantSet, got))
	}
	for id := range wantSet {
		if !got[id] {
			t.Errorf("missing ID %q", id)
		}
	}
}

// TestFromObject_RejectsNonSymbols pins actionable rejection of objects that
// are not declared symbols.
func TestFromObject_RejectsNonSymbols(t *testing.T) {
	src := `package p

func f() {
	x := 1
	_ = x
	_ = len("x")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("p", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	var local, builtin types.Object
	for _, obj := range info.Defs {
		if obj == nil {
			continue
		}
		if v, ok := obj.(*types.Var); ok && v.Parent() != nil && v.Parent() != pkg.Scope() {
			local = v
		}
	}
	for _, obj := range info.Uses {
		if _, ok := obj.(*types.Builtin); ok {
			builtin = obj
			break
		}
	}
	if local == nil || builtin == nil {
		t.Fatalf("fixture objects not found (local=%v builtin=%v)", local, builtin)
	}
	for _, tc := range []struct {
		name string
		obj  types.Object
	}{
		{"local variable", local},
		{"builtin", builtin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := symbol.FromObject(tc.obj); err == nil {
				t.Fatalf("FromObject(%v) = nil error; want rejection", tc.obj)
			}
		})
	}
}
