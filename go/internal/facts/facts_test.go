package facts_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

func TestFactsRoundTrip(t *testing.T) {
	// 1. Construct ExportedSymbols
	symMethod := facts.ExportedSymbol{
		Name:     "(*example.com/store.DB).Get",
		File:     "store.go",
		Kind:     "method",
		Receiver: "(*example.com/store.DB)",
	}

	symInit := facts.ExportedSymbol{
		Name: "example.com/store.init",
		File: "init.go",
		Kind: "init",
	}

	symFunc := facts.ExportedSymbol{
		Name: "example.com/store.Read",
		File: "api.go",
		Kind: "func",
	}

	// 2. Construct PackageFact
	pkgFact := facts.PackageFact{
		ImportPath:      "example.com/store",
		Imports:         []string{"os", "github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"},
		ExportedSymbols: []facts.ExportedSymbol{symMethod, symInit, symFunc},
	}

	// 3. Construct CallEdge
	edge := facts.CallEdge{
		Caller: "example.com/caller.Run",
		Callee: "(*example.com/store.DB).Get",
	}

	// 4. Construct PackageFacts
	pkgFacts := facts.PackageFacts{
		Packages:      []facts.PackageFact{pkgFact},
		CallEdges:     []facts.CallEdge{edge},
		StdlibImports: []string{"os"},
	}

	// 5. Construct DependencyInterface
	depIface := facts.DependencyInterface{
		Component:      "store-component",
		InterfaceStyle: manifest.InterfaceStylePackageSurface,
		Packages:       []string{"example.com/store"},
		Symbols:        []facts.SymbolID{"(*example.com/store.DB).Get", "example.com/store.Read"},
	}

	// Verify pkgFacts round-trip fields
	if len(pkgFacts.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgFacts.Packages))
	}
	p := pkgFacts.Packages[0]
	if p.ImportPath != "example.com/store" {
		t.Errorf("expected import path example.com/store, got %q", p.ImportPath)
	}
	if len(pkgFacts.StdlibImports) != 1 || pkgFacts.StdlibImports[0] != "os" {
		t.Errorf("unexpected stdlib imports: %v", pkgFacts.StdlibImports)
	}
	if len(p.Imports) != 2 || p.Imports[0] != "os" || p.Imports[1] != "github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer" {
		t.Errorf("unexpected imports: %v", p.Imports)
	}

	if len(p.ExportedSymbols) != 3 {
		t.Fatalf("expected 3 exported symbols, got %d", len(p.ExportedSymbols))
	}
	s0 := p.ExportedSymbols[0]
	if s0.Name != "(*example.com/store.DB).Get" || s0.File != "store.go" || s0.Kind != "method" || s0.Receiver != "(*example.com/store.DB)" {
		t.Errorf("unexpected s0 fields: %+v", s0)
	}
	s1 := p.ExportedSymbols[1]
	if s1.Name != "example.com/store.init" || s1.File != "init.go" || s1.Kind != "init" || s1.Receiver != "" {
		t.Errorf("unexpected s1 fields: %+v", s1)
	}
	s2 := p.ExportedSymbols[2]
	if s2.Name != "example.com/store.Read" || s2.File != "api.go" || s2.Kind != "func" || s2.Receiver != "" {
		t.Errorf("unexpected s2 fields: %+v", s2)
	}

	if len(pkgFacts.CallEdges) != 1 {
		t.Fatalf("expected 1 call edge, got %d", len(pkgFacts.CallEdges))
	}
	e := pkgFacts.CallEdges[0]
	if e.Caller != "example.com/caller.Run" || e.Callee != "(*example.com/store.DB).Get" {
		t.Errorf("unexpected call edge fields: %+v", e)
	}

	// Verify depIface round-trip fields
	if depIface.Component != "store-component" {
		t.Errorf("expected component store-component, got %q", depIface.Component)
	}
	if depIface.InterfaceStyle != manifest.InterfaceStylePackageSurface {
		t.Errorf("expected InterfaceStylePackageSurface, got %v", depIface.InterfaceStyle)
	}
	if len(depIface.Packages) != 1 || depIface.Packages[0] != "example.com/store" {
		t.Errorf("unexpected packages: %v", depIface.Packages)
	}
	if len(depIface.Symbols) != 2 || depIface.Symbols[0] != "(*example.com/store.DB).Get" || depIface.Symbols[1] != "example.com/store.Read" {
		t.Errorf("unexpected symbols: %v", depIface.Symbols)
	}
}
