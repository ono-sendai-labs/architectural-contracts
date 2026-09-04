package symbol_test

import (
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// mapInventory is a CapslockInventory backed by fixed package and declaration
// sets, standing in for Step 4's independently computed inventory.
type mapInventory struct {
	packages     map[string]bool
	declarations map[string]bool // "pkg.Name" keys
}

func (m mapInventory) KnownPackage(importPath string) bool { return m.packages[importPath] }

func (m mapInventory) Declares(pkg, name string) bool { return m.declarations[pkg+"."+name] }

func (m mapInventory) DeclaresMethod(pkg, typeName, method string) bool {
	return m.declarations["("+pkg+"."+typeName+")."+method]
}

func newMapInventory(pkgs, decls []string) mapInventory {
	inv := mapInventory{packages: map[string]bool{}, declarations: map[string]bool{}}
	for _, p := range pkgs {
		inv.packages[p] = true
	}
	for _, d := range decls {
		inv.declarations[d] = true
	}
	return inv
}

// TestParseCapslockFunction pins the structured normalization contract
// (task req 1, AC4): the caller supplies Capslock's structured package field
// and the independently inventoried declarations/packages, and normalization
// fails closed whenever the inventory does not confirm exactly one canonical
// declaration.
func TestParseCapslockFunction(t *testing.T) {
	inv := newMapInventory(
		[]string{"example.com/store", "gopkg.in/yaml.v2", "os"},
		[]string{"example.com/store.Load", "(example.com/store.Box).Get", "example.com/store.Box", "gopkg.in/yaml.v2.Unmarshal", "os.ReadFile"},
	)

	for _, tc := range []struct {
		name string
		fn   symbol.CapslockFunction
		want string
	}{
		{
			name: "top-level with structured package",
			fn:   symbol.CapslockFunction{Name: "store.Load", Package: "example.com/store"},
			want: "example.com/store.Load",
		},
		{
			name: "dotful ambiguous spelling disambiguated by structured package",
			fn:   symbol.CapslockFunction{Name: "yaml.v2.Unmarshal", Package: "gopkg.in/yaml.v2"},
			want: "gopkg.in/yaml.v2.Unmarshal",
		},
		{
			name: "method with structured package",
			fn:   symbol.CapslockFunction{Name: "(*store.Box[int]).Get", Package: "example.com/store"},
			want: "(example.com/store.Box).Get",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := symbol.ParseCapslockFunction(tc.fn, inv)
			if err != nil {
				t.Fatalf("ParseCapslockFunction(%+v) error = %v", tc.fn, err)
			}
			if string(got) != tc.want {
				t.Fatalf("ParseCapslockFunction(%+v) = %q; want %q", tc.fn, got, tc.want)
			}
			if _, err := symbol.Parse(got.Format()); err != nil {
				t.Fatalf("normalized result %q does not parse as a canonical SymbolID: %v", got, err)
			}
		})
	}

	for _, tc := range []struct {
		name string
		fn   symbol.CapslockFunction
		inv  symbol.CapslockInventory
	}{
		{"nil inventory", symbol.CapslockFunction{Name: "os.ReadFile", Package: "os"}, nil},
		{"malformed structured package", symbol.CapslockFunction{Name: "os.ReadFile", Package: "/os//"}, inv},
		{"package path with empty segment", symbol.CapslockFunction{Name: "os.ReadFile", Package: "os//x"}, inv},
		{"declaration not in inventory", symbol.CapslockFunction{Name: "os.WriteFile", Package: "os"}, inv},
		{"receiver type not in inventory", symbol.CapslockFunction{Name: "(*store.Nox).Get", Package: "example.com/store"}, inv},
		{"method not declared on inventoried receiver", symbol.CapslockFunction{Name: "(*store.Box).NotDeclared", Package: "example.com/store"}, inv},
		{"ambiguous spelling with unknown package", symbol.CapslockFunction{Name: "yaml.v2.Unmarshal", Package: "example.com/other"}, inv},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := symbol.ParseCapslockFunction(tc.fn, tc.inv); err == nil {
				t.Fatalf("ParseCapslockFunction(%+v) = nil error; want rejection", tc.fn)
			}
		})
	}
}

// TestParseCapslockFunction_CanonicalizesNamespace pins review finding 2 of
// round 1: inventory-backed normalization resolves and confirms against the
// canonical namespace, so an inventory keyed by the canonical IDs produced by
// FromObject confirms names whose text is in the host's source namespace.
func TestParseCapslockFunction_CanonicalizesNamespace(t *testing.T) {
	orig := hostpolicy.CanonicalizePath
	defer func() { hostpolicy.CanonicalizePath = orig }()
	hostpolicy.CanonicalizePath = func(p string) string {
		// A host rewrite: module-hosted packages move under a canonical host.
		switch {
		case p == "os":
			return "canonical.example/os"
		case strings.HasPrefix(p, "example.com/"):
			return "canonical.example/" + strings.TrimPrefix(p, "example.com/")
		}
		return p
	}

	inv := newMapInventory(
		[]string{"canonical.example/os", "canonical.example/store"},
		[]string{"canonical.example/os.ReadFile", "canonical.example/store.Box", "(canonical.example/store.Box).Get"},
	)

	for _, tc := range []struct {
		name string
		fn   symbol.CapslockFunction
		want symbol.SymbolID
	}{
		{"top-level", symbol.CapslockFunction{Name: "os.ReadFile", Package: "os"}, "canonical.example/os.ReadFile"},
		{"method", symbol.CapslockFunction{Name: "(*store.Box).Get", Package: "example.com/store"}, "(canonical.example/store.Box).Get"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := symbol.ParseCapslockFunction(tc.fn, inv)
			if err != nil {
				t.Fatalf("ParseCapslockFunction(%+v) error = %v", tc.fn, err)
			}
			if got != tc.want {
				t.Fatalf("ParseCapslockFunction(%+v) = %q; want %q", tc.fn, got, tc.want)
			}
		})
	}
}
