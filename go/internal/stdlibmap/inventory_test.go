package stdlibmap

import (
	"fmt"
	"go/ast"
	"go/importer"
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

import "errors"

const Max = 10

var Store string

var cache = map[string]int{}

func Open(name string) (*F, error) {
	if name == "" {
		return nil, errors.New("empty")
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
	return LoaderFunc(func(paths []string) (map[string]*types.Package, error) {
		out := make(map[string]*types.Package, len(paths))
		for _, p := range paths {
			if p != pkgPath {
				return nil, fmt.Errorf("fixture loader: unexpected package %q", p)
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "fixture.go", src, 0)
			if err != nil {
				return nil, fmt.Errorf("fixture loader: %w", err)
			}
			pkg := types.NewPackage(pkgPath, file.Name.Name)
			conf := types.Config{
				Importer: importer.Default(),
			}
			var errs []error
			conf.Error = func(err error) { errs = append(errs, err) }
			files := []*ast.File{file}
			pkg, checkErr := conf.Check(pkgPath, fset, files, nil)
			if checkErr != nil {
				errs = append(errs, checkErr)
			}
			pkg.MarkComplete()
			out[p] = pkg
			if len(errs) > 0 {
				return nil, fmt.Errorf("fixture loader: type errors in %q: %v", p, errs)
			}
		}
		return out, nil
	})
}

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
