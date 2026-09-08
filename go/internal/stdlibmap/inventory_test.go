package stdlibmap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// --- hermetic fixture loader -------------------------------------------------

const fixturePkgPath = "example.com/inventory"

const fixtureSrc = `package inventory

const Max = 10

var Store string

var cache = map[string]int{}

func Open(name string) (*F, error) {
	if name == "" {
		return nil, nil
	}
	return &F{Name: name}, nil
}

func helper() {}

type Base struct {
	ID string
}

func (b Base) Stamp() string { return b.ID }

type F struct {
	Base
	Name string
	age  int
}

func (f F) Read() string { return f.Name }

func (f *F) Write(name string) { f.Name = name }

func (f F) seek() {}

type Closer interface {
	Close() error
}

type Box[T any] struct {
	Value T
}

func (b Box[T]) Get() T { return b.Value }

type hidden struct {
	Visible int
}

func (h *hidden) Touch() {}

type Alias = F

type H = hidden
`

// fixtureLoader type-checks fixturePkgPath from fixtureSrc in memory. It is
// the hermetic Loader seam used by the inventory tests: no network, no
// toolchain invocation.
func fixtureLoader(t *testing.T, pkgPath, src string) Loader {
	t.Helper()
	return typeCheckLoader(t, map[string]string{pkgPath: src})
}

// typeCheckLoader is the hermetic multi-package Loader seam: it type-checks
// every fixture package in memory, resolving imports among the fixtures.
func typeCheckLoader(t *testing.T, sources map[string]string) Loader {
	t.Helper()
	cache := map[string]*types.Package{}
	var check func(path string) (*types.Package, error)
	check = func(path string) (*types.Package, error) {
		if p, ok := cache[path]; ok {
			return p, nil
		}
		src, ok := sources[path]
		if !ok {
			return nil, fmt.Errorf("fixture loader: no fixture source for %q", path)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fixture.go", src, 0)
		if err != nil {
			return nil, fmt.Errorf("fixture loader: %w", err)
		}
		conf := types.Config{Importer: importerFunc(check)}
		var errs []error
		conf.Error = func(err error) { errs = append(errs, err) }
		pkg, checkErr := conf.Check(path, fset, []*ast.File{file}, nil)
		if checkErr != nil {
			errs = append(errs, checkErr)
		}
		pkg.MarkComplete()
		cache[path] = pkg
		if len(errs) > 0 {
			return nil, fmt.Errorf("fixture loader: type errors in %q: %v", path, errs)
		}
		return pkg, nil
	}
	return LoaderFunc(func(paths []string) (map[string]*types.Package, error) {
		out := make(map[string]*types.Package, len(paths))
		for _, p := range paths {
			pkg, err := check(p)
			if err != nil {
				return nil, err
			}
			out[p] = pkg
		}
		return out, nil
	})
}

type importerFunc func(path string) (*types.Package, error)

// Import implements types.Importer.
func (f importerFunc) Import(path string) (*types.Package, error) {
	if p, err := f(path); err != nil || p != nil {
		return p, err
	}
	return nil, fmt.Errorf("fixture loader: cannot import %q", path)
}

// --- round-1 finding: cross-package aliases must not contribute foreign methods

const foreignAliasSrc = `package inventory

import "example.com/foreignpkg"

type A = foreignpkg.B
`

const foreignSrc = `package foreignpkg

type B struct{}

func (B) M() string { return "m" }
`

func TestBuildInventoryForeignAliasMethods(t *testing.T) {
	loader := typeCheckLoader(t, map[string]string{
		"example.com/inventory":  foreignAliasSrc,
		"example.com/foreignpkg": foreignSrc,
	})
	// Only the alias package is inventoried: the foreign source stays
	// available to the fixture importer, so a walk that claimed the foreign
	// type's members would surface them here (round-2 isolation fix).
	inv, err := BuildInventory([]PackageEntry{
		{Path: "example.com/inventory", Importable: true},
	}, loader)
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}
	for _, id := range inv.Symbols {
		if id.String() != "example.com/inventory.A" {
			t.Fatalf("alias package contributed %q; want only the alias key", id)
		}
	}
	if !containsID(inv.Symbols, "example.com/inventory.A") {
		t.Fatalf("alias key missing from inventory")
	}
	// The foreign method must not be attributed to the alias package.
	if containsID(inv.Symbols, "(example.com/foreignpkg.B).M") {
		t.Fatalf("foreign method (example.com/foreignpkg.B).M leaked into the alias package's inventory")
	}
	// The single importable package keeps exactly one aggregate init.
	if len(inv.Inits) != 1 || inv.Inits[0].String() != "example.com/inventory.init" {
		t.Fatalf("inits = %v, want one for the alias package", inv.Inits)
	}
}

func containsID(ids []symbol.SymbolID, want string) bool {
	for _, id := range ids {
		if id.String() == want {
			return true
		}
	}
	return false
}

// --- round-1 finding: BuildInventory must normalize raw package entries itself

func TestBuildInventoryRejectsRawEntries(t *testing.T) {
	dummy := LoaderFunc(func(paths []string) (map[string]*types.Package, error) {
		return nil, fmt.Errorf("fixture loader: unexpected load %v", paths)
	})
	// A public path MAY be declared non-importable: that is the explicit
	// oracle's build-excluded verdict (task req 6), retained in the
	// enumeration — only internal-marked-importable is rejected here.
	for name, entries := range map[string][]PackageEntry{
		"duplicate path":             {{Path: "a/b", Importable: true}, {Path: "a/b", Importable: true}},
		"malformed path":             {{Path: "a/b ", Importable: true}},
		"empty path":                 {{Path: "", Importable: true}},
		"internal marked importable": {{Path: "a/internal/x", Importable: true}},
	} {
		inv, err := BuildInventory(entries, dummy)
		if err == nil {
			t.Fatalf("BuildInventory(%s): want error, got nil inventory %v", name, inv)
		}
		if inv != nil {
			t.Fatalf("BuildInventory(%s): returned a partial inventory", name)
		}
	}
}

func TestBuildInventoryNormalizesRawEntries(t *testing.T) {
	loader := fixtureLoader(t, fixturePkgPath, fixtureSrc)
	inv, err := BuildInventory([]PackageEntry{
		{Path: fixturePkgPath, Importable: true},
		{Path: "example.com/internal/secret", Importable: false},
	}, loader)
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}
	want := []PackageEntry{
		{Path: "example.com/internal/secret", Importable: false},
		{Path: fixturePkgPath, Importable: true},
	}
	if !reflect.DeepEqual(inv.Packages, want) {
		t.Fatalf("packages = %+v, want sorted %+v", inv.Packages, want)
	}
}

// --- round-1 finding: the inventory exposes Capslock normalization context ---

const capslockFixtureSrc = `package osx

type File struct {
	Fd int
}

func (f *File) Read(b []byte) (int, error) { return 0, nil }

func (f File) Close() error { return nil }

func (f File) secret() {}

type I interface {
	M()
}

func Open() (*File, error) { return nil, nil }

var Default *File
`

func TestInventoryImplementsCapslockInventory(t *testing.T) {
	inv, err := BuildInventory(
		[]PackageEntry{{Path: "example.com/osx", Importable: true}},
		typeCheckLoader(t, map[string]string{"example.com/osx": capslockFixtureSrc}))
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}
	var _ symbol.CapslockInventory = inv

	cases := []struct {
		name     string
		capslock symbol.CapslockFunction
		want     string
		wantErr  string
	}{
		{
			name:     "pointer method",
			capslock: symbol.CapslockFunction{Name: "(*osx.File).Read", Package: "example.com/osx"},
			want:     "(example.com/osx.File).Read",
		},
		{
			name:     "top-level func",
			capslock: symbol.CapslockFunction{Name: "example.com/osx.Open", Package: "example.com/osx"},
			want:     "example.com/osx.Open",
		},
		{
			name:     "interface method spec",
			capslock: symbol.CapslockFunction{Name: "(example.com/osx.I).M", Package: "example.com/osx"},
			want:     "example.com/osx.I",
		},
		{
			name:     "unresolved method",
			capslock: symbol.CapslockFunction{Name: "(*example.com/osx.File).Write", Package: "example.com/osx"},
			wantErr:  "does not confirm a declaration",
		},
		{
			name:     "interface method on a struct",
			capslock: symbol.CapslockFunction{Name: "(*example.com/osx.File).M", Package: "example.com/osx"},
			wantErr:  "does not confirm a declaration",
		},
		{
			name:     "unknown symbol",
			capslock: symbol.CapslockFunction{Name: "example.com/osx.Missing", Package: "example.com/osx"},
			wantErr:  "does not confirm a declaration",
		},
		{
			name:     "unknown package",
			capslock: symbol.CapslockFunction{Name: "example.com/otherpkg.Y", Package: "example.com/otherpkg"},
			wantErr:  "does not confirm a declaration",
		},
		{
			name:     "unexported helper is not a declaration",
			capslock: symbol.CapslockFunction{Name: "(*example.com/osx.File).secret", Package: "example.com/osx"},
			wantErr:  "does not confirm a declaration",
		},
	}
	for _, tc := range cases {
		id, err := symbol.ParseCapslockFunction(tc.capslock, inv)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("%s: ParseCapslockFunction(%v) = (%q, %v); want error %q", tc.name, tc.capslock.Name, id, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: ParseCapslockFunction(%v): %v", tc.name, tc.capslock.Name, err)
		}
		if id.String() != tc.want {
			t.Fatalf("%s: normalized ID = %q, want %q", tc.name, id, tc.want)
		}
	}
}

// --- hermetic fixture loader (original single-package tests) ------------------

func mustIDs(t *testing.T, raw []symbol.SymbolID) []string {
	t.Helper()
	out := make([]string, len(raw))
	for i, id := range raw {
		out[i] = id.String()
	}
	return out
}

// --- AC2: every externally referencable declaration is inventoried -----------

func TestBuildInventoryFullDeclarationSet(t *testing.T) {
	inv, err := BuildInventory([]PackageEntry{{Path: fixturePkgPath, Importable: true}}, fixtureLoader(t, fixturePkgPath, fixtureSrc))
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}

	want := []string{
		"(example.com/inventory.Base).Stamp",
		"(example.com/inventory.Box).Get",
		"(example.com/inventory.F).Read",
		"(example.com/inventory.F).Write",
		"(example.com/inventory.hidden).Touch",
		"example.com/inventory.Alias",
		"example.com/inventory.Base",
		"example.com/inventory.Box",
		"example.com/inventory.Closer",
		"example.com/inventory.F",
		"example.com/inventory.H",
		"example.com/inventory.Max",
		"example.com/inventory.Open",
		"example.com/inventory.Store",
		"example.com/inventory.hidden",
	}
	got := mustIDs(t, inv.Symbols)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inventory symbols =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	wantInits := []string{fixturePkgPath + ".init"}
	gotInits := mustIDs(t, inv.Inits)
	if !reflect.DeepEqual(gotInits, wantInits) {
		t.Fatalf("inits = %v, want %v", gotInits, wantInits)
	}

	if len(inv.Packages) != 1 || inv.Packages[0].Path != fixturePkgPath {
		t.Fatalf("packages = %+v, want the single fixture package", inv.Packages)
	}
}

func TestBuildInventoryExcludesNonPersistedMembers(t *testing.T) {
	inv, err := BuildInventory([]PackageEntry{{Path: fixturePkgPath, Importable: true}}, fixtureLoader(t, fixturePkgPath, fixtureSrc))
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}
	set := map[string]bool{}
	for _, id := range inv.Symbols {
		set[id.String()] = true
	}
	for _, banned := range []string{
		"example.com/inventory.F.Close",        // interface method spec
		"example.com/inventory.Closer.Close",   // interface method spec
		"example.com/inventory.F.Name",         // field
		"example.com/inventory.Base.ID",        // field
		"example.com/inventory.F.ID",           // promoted field
		"example.com/inventory.F.Stamp",        // promoted method
		"(example.com/inventory.F).seek",       // unexported method
		"example.com/inventory.helper",         // unexported func
		"example.com/inventory.cache",          // unexported var
		"(example.com/inventory.Box[int]).Get", // instantiation, never persisted
		"example.com/inventory.F.init",         // init#N is never separate
	} {
		if set[banned] {
			t.Fatalf("inventory contains non-persisted member %q", banned)
		}
	}
}

func TestBuildInventoryNonImportableHasNoSymbols(t *testing.T) {
	inv, err := BuildInventory([]PackageEntry{
		{Path: "example.com/internal/secret", Importable: false},
		{Path: fixturePkgPath, Importable: true},
	}, fixtureLoader(t, fixturePkgPath, fixtureSrc))
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}
	if got := mustIDs(t, inv.Inits); len(got) != 1 || got[0] != fixturePkgPath+".init" {
		t.Fatalf("inits = %v, want only the importable package's init", got)
	}
	for _, id := range inv.Symbols {
		if strings.HasPrefix(id.String(), "example.com/internal/") {
			t.Fatalf("non-importable package contributed symbol %q", id)
		}
	}
}

// --- AC3: inventory failure cannot become partial output ----------------------

func TestBuildInventoryLoadError(t *testing.T) {
	failing := LoaderFunc(func(paths []string) (map[string]*types.Package, error) {
		return nil, fmt.Errorf("loading %q: SDK export data unavailable", paths[0])
	})
	inv, err := BuildInventory([]PackageEntry{{Path: fixturePkgPath, Importable: true}}, failing)
	if err == nil {
		t.Fatalf("BuildInventory: want load error, got nil")
	}
	if !strings.Contains(err.Error(), "SDK export data unavailable") {
		t.Fatalf("BuildInventory error %q: want actionable load context", err)
	}
	if inv != nil {
		t.Fatalf("BuildInventory returned a partial inventory on load error")
	}
}

func TestBuildInventoryMissingPackageInResult(t *testing.T) {
	incomplete := LoaderFunc(func(paths []string) (map[string]*types.Package, error) {
		return map[string]*types.Package{}, nil
	})
	inv, err := BuildInventory([]PackageEntry{{Path: fixturePkgPath, Importable: true}}, incomplete)
	if err == nil || !strings.Contains(err.Error(), "not loaded") {
		t.Fatalf("BuildInventory = (%v, %v); want incomplete-result error", inv, err)
	}
}

func TestBuildInventoryTypeError(t *testing.T) {
	broken := `package inventory

func Broken() { undefinedSymbol() }
`
	_, err := BuildInventory([]PackageEntry{{Path: fixturePkgPath, Importable: true}}, fixtureLoader(t, fixturePkgPath, broken))
	if err == nil || !strings.Contains(err.Error(), "type errors") {
		t.Fatalf("BuildInventory = %v; want actionable type error", err)
	}
}

func TestBuildInventoryNilLoader(t *testing.T) {
	_, err := BuildInventory([]PackageEntry{{Path: fixturePkgPath, Importable: true}}, nil)
	if err == nil || !strings.Contains(err.Error(), "loader is required") {
		t.Fatalf("BuildInventory = %v; want nil-loader error", err)
	}
}

// The canonical-ID collision guard is exercised directly on the merge step:
// two distinct declaration objects canonicalizing to one ID must fail
// generation (task req 6). In valid Go two scope objects cannot share a
// canonical name, so the collision can only arise from a loader returning
// incoherent declarations.
func TestMergeObservedRejectsCollision(t *testing.T) {
	objA := types.NewFunc(token.NoPos, types.NewPackage("example.com/x", "x"), "F", nil)
	objB := types.NewFunc(token.NoPos, types.NewPackage("example.com/x", "x"), "F", nil)
	_, err := mergeObserved([]observed{
		{id: "example.com/x.F", obj: objA},
		{id: "example.com/x.F", obj: objB},
	})
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("mergeObserved = %v; want collision error", err)
	}
	// The same object observed twice deduplicates.
	out, err := mergeObserved([]observed{{id: "example.com/x.F", obj: objA}, {id: "example.com/x.F", obj: objA}})
	if err != nil || len(out) != 1 {
		t.Fatalf("mergeObserved same object = (%v, %v); want one entry", out, err)
	}
}
