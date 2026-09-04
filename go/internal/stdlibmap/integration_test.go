//go:build integration

package stdlibmap_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// TestRealSDKInventory is the real-toolchain coverage (task AC 6): the native
// oracle enumerates every `go list std` package of the local toolchain, the
// native loader inventories every importable package, and every inventoried
// ID parses canonically. No network access is involved: all packages come
// from the local SDK.
func TestRealSDKInventory(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	entries, err := stdlibmap.NativeStdPackageList(context.Background())
	if err != nil {
		t.Fatalf("NativeStdPackageList: %v", err)
	}
	if len(entries) < 100 {
		t.Fatalf("enumerated %d packages; the oracle is not total", len(entries))
	}

	loader := &stdlibmap.NativeLoader{}
	inv, err := stdlibmap.BuildInventory(entries, loader)
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}

	// Every enumerated importable package is represented: one init each, and
	// every non-importable package excluded from the symbol inventory.
	enumerated := map[string]bool{}
	importable := 0
	for _, e := range entries {
		enumerated[e.Path] = true
		if e.Importable {
			importable++
		}
	}
	if len(inv.Packages) != len(entries) {
		t.Fatalf("inventory packages = %d, enumerated = %d", len(inv.Packages), len(entries))
	}
	if len(inv.Inits) != importable {
		t.Fatalf("inits = %d, importable packages = %d", len(inv.Inits), importable)
	}

	symbolsByPkg := map[string]int{}
	for _, id := range inv.Symbols {
		if _, err := symbol.Parse(id.String()); err != nil {
			t.Fatalf("inventoried ID %q does not parse canonically: %v", id, err)
		}
		pkg := string(id[:strings.LastIndexByte(id.String(), '.')])
		symbolsByPkg[pkg]++
	}
	for _, want := range []string{"os", "strings", "fmt", "net/http"} {
		if symbolsByPkg[want] == 0 {
			t.Fatalf("expected symbols for %q; got %v", want, symbolsByPkg)
		}
	}
	// Doc-only packages (e.g. runtime/race) legitimately have zero exported
	// symbols for a target configuration; totality is about representation,
	// not non-emptiness, so no per-package non-empty assertion is made.

	// Determinism: a second inventory over the same enumeration is identical.
	inv2, err := stdlibmap.BuildInventory(entries, loader)
	if err != nil {
		t.Fatalf("second BuildInventory: %v", err)
	}
	if !equalStrings(idStrings(inv.Symbols), idStrings(inv2.Symbols)) {
		t.Fatalf("inventory is not deterministic between runs")
	}
	if !equalStrings(idStrings(inv.Inits), idStrings(inv2.Inits)) {
		t.Fatalf("init inventory is not deterministic between runs")
	}
}

func idStrings(ids []symbol.SymbolID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
