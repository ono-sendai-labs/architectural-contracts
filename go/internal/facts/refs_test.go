package facts_test

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
)

func refEdge(kind facts.ReferenceKind, from, referentPkg, referent, file string, line int) facts.ReferenceEdge {
	return facts.ReferenceEdge{
		Kind:            kind,
		FromPackage:     from,
		ReferentPackage: referentPkg,
		Referent:        facts.SymbolID(referent),
		Site:            facts.SourceSite{File: file, Line: line},
	}
}

// T1 / AC1: reference identity is canonical — valid referents for every
// declaring-object kind validate, invalid facts are rejected.
func TestReferenceEdgeValidation(t *testing.T) {
	valid := []struct {
		name string
		edge facts.ReferenceEdge
	}{
		{"func", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "run/runner.go", 12)},
		{"method", refEdge(facts.RefMethod, "example.com/app/runner", "os", "(os.File).Read", "run/runner.go", 30)},
		{"type", refEdge(facts.RefType, "example.com/app/runner", "io", "io.Reader", "run/runner.go", 7)},
		{"field", refEdge(facts.RefField, "example.com/app/runner", "example.com/dep", "example.com/dep.T", "run/runner.go", 41)},
		{"interface method spec keys declaring interface", refEdge(facts.RefField, "example.com/app/runner", "example.com/dep", "example.com/dep.Greeter", "run/runner.go", 44)},
		{"var", refEdge(facts.RefVar, "example.com/app/runner", "os", "os.Stdin", "run/runner.go", 3)},
		{"const", refEdge(facts.RefConst, "example.com/app/runner", "io", "io.EOF", "run/runner.go", 9)},
		{"alias", refEdge(facts.RefType, "example.com/app/runner", "example.com/dep", "example.com/dep.A", "run/runner.go", 15)},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if err := tc.edge.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}

	invalid := []struct {
		name string
		edge facts.ReferenceEdge
	}{
		{"unknown kind", refEdge("interface", "example.com/app/runner", "io", "io.Reader", "run/runner.go", 1)},
		{"empty kind", refEdge("", "example.com/app/runner", "io", "io.Reader", "run/runner.go", 1)},
		{"referent not grammar valid", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.", "run/runner.go", 1)},
		{"referent pointer marker", refEdge(facts.RefMethod, "example.com/app/runner", "os", "(*os.File).Read", "run/runner.go", 1)},
		{"referent generic brackets", refEdge(facts.RefType, "example.com/app/runner", "example.com/dep", "example.com/dep.T[int]", "run/runner.go", 1)},
		{"referent package mismatch", refEdge(facts.RefFunc, "example.com/app/runner", "os", "io.EOF", "run/runner.go", 1)},
		{"empty referent", refEdge(facts.RefFunc, "example.com/app/runner", "os", "", "run/runner.go", 1)},
		{"empty from package", refEdge(facts.RefFunc, "", "os", "os.ReadFile", "run/runner.go", 1)},
		{"empty file", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "", 1)},
		{"absolute site path", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "/outside/member.go", 1)},
		{"parent-traversing site path", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "../outside.go", 1)},
		{"dot component in site path", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "run/./runner.go", 1)},
		{"trailing slash site path", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "run/runner.go/", 1)},
		{"windows drive backslash site path", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", `C:\outside.go`, 1)},
		{"windows drive slash site path", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "C:/outside.go", 1)},
		{"embedded backslash site path", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", `run\runner.go`, 1)},
		{"line below one", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "run/runner.go", 0)},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			if err := tc.edge.Validate(); err == nil {
				t.Fatalf("Validate() = nil, want error for %s", tc.name)
			}
		})
	}
}

// T2 / AC2: structural duplicate keys are precise — only values equal in
// every semantic field deduplicate.
func TestReferenceEdgeIdentity(t *testing.T) {
	base := refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "run/runner.go", 12)

	if base.Key() != refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "run/runner.go", 12).Key() {
		t.Fatal("identical edges must have equal keys")
	}
	distinct := []struct {
		name string
		edge facts.ReferenceEdge
	}{
		{"different referent", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.WriteFile", "run/runner.go", 12)},
		{"different site line", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "run/runner.go", 13)},
		{"different site file", refEdge(facts.RefFunc, "example.com/app/runner", "os", "os.ReadFile", "run/other.go", 12)},
		{"different from package", refEdge(facts.RefFunc, "example.com/app/other", "os", "os.ReadFile", "run/runner.go", 12)},
		{"different kind", refEdge(facts.RefVar, "example.com/app/runner", "os", "os.ReadFile", "run/runner.go", 12)},
	}
	for _, tc := range distinct {
		t.Run(tc.name, func(t *testing.T) {
			if base.Key() == tc.edge.Key() {
				t.Fatalf("edge differing by %s must have a distinct key", tc.name)
			}
		})
	}
}

func importEdge(importing, path string, res facts.ImportResolution, file string, line int) facts.ImportEdge {
	return facts.ImportEdge{
		ImportingPackage: importing,
		ImportPath:       path,
		Resolution:       res,
		Site:             facts.SourceSite{File: file, Line: line},
	}
}

// T3 / AC3: import facts retain written-edge state — resolved vs
// missing-type-data vs unresolved are distinguishable without legacy slices,
// including blank imports.
func TestImportEdgeStates(t *testing.T) {
	resolved := importEdge("example.com/app/runner", "example.com/dep/store", facts.ImportResolved, "run/runner.go", 5)
	blank := importEdge("example.com/app/runner", "example.com/dep/register", facts.ImportResolved, "run/blanks.go", 4)
	missing := importEdge("example.com/app/runner", "os", facts.ImportMissingTypeData, "run/runner.go", 6)
	unresolved := importEdge("example.com/app/runner", "example.com/nope/ghost", facts.ImportUnresolved, "run/runner.go", 7)

	for _, e := range []facts.ImportEdge{resolved, blank, missing, unresolved} {
		if err := e.Validate(); err != nil {
			t.Fatalf("Validate(%+v) = %v, want nil", e, err)
		}
	}
	if resolved.Resolution != facts.ImportResolved {
		t.Fatalf("resolved edge lost its state: %v", resolved.Resolution)
	}
	if blank.Resolution != facts.ImportResolved {
		t.Fatalf("blank import lost its state: %v", blank.Resolution)
	}
	if missing.Resolution != facts.ImportMissingTypeData {
		t.Fatalf("missing-type-data edge lost its state: %v", missing.Resolution)
	}
	if unresolved.Resolution != facts.ImportUnresolved {
		t.Fatalf("unresolved edge lost its state: %v", unresolved.Resolution)
	}
	// The states must be distinguishable as written values, not collapsed.
	if resolved.Resolution == missing.Resolution || resolved.Resolution == unresolved.Resolution || missing.Resolution == unresolved.Resolution {
		t.Fatal("import resolution states must be pairwise distinct")
	}

	invalid := []struct {
		name string
		edge facts.ImportEdge
	}{
		{"empty importing package", importEdge("", "os", facts.ImportResolved, "run/runner.go", 1)},
		{"empty import path", importEdge("example.com/app/runner", "", facts.ImportResolved, "run/runner.go", 1)},
		{"unknown resolution", importEdge("example.com/app/runner", "os", "maybe", "run/runner.go", 1)},
		{"empty file", importEdge("example.com/app/runner", "os", facts.ImportResolved, "", 1)},
		{"absolute site path", importEdge("example.com/app/runner", "os", facts.ImportResolved, "/outside/runner.go", 1)},
		{"parent-traversing site path", importEdge("example.com/app/runner", "os", facts.ImportResolved, "../elsewhere.go", 1)},
		{"line below one", importEdge("example.com/app/runner", "os", facts.ImportResolved, "run/runner.go", 0)},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			if err := tc.edge.Validate(); err == nil {
				t.Fatalf("Validate() = nil, want error for %s", tc.name)
			}
		})
	}
}

func TestImportEdgeIdentity(t *testing.T) {
	base := importEdge("example.com/app/runner", "os", facts.ImportResolved, "run/runner.go", 6)
	if base.Key() != importEdge("example.com/app/runner", "os", facts.ImportResolved, "run/runner.go", 6).Key() {
		t.Fatal("identical import edges must have equal keys")
	}
	if base.Key() == importEdge("example.com/app/runner", "os", facts.ImportMissingTypeData, "run/runner.go", 6).Key() {
		t.Fatal("edges differing by resolution state must have distinct keys")
	}
	if base.Key() == importEdge("example.com/app/runner", "os", facts.ImportResolved, "run/other.go", 6).Key() {
		t.Fatal("edges differing by site must have distinct keys")
	}
}

// T5 / AC5: fact ordering is deterministic — the sort helpers produce
// identical order from any input order, and dedup removes only exact
// duplicates.
func TestDeterministicOrdering(t *testing.T) {
	refs := []facts.ReferenceEdge{
		refEdge(facts.RefFunc, "example.com/app/b", "os", "os.ReadFile", "b/b.go", 2),
		refEdge(facts.RefConst, "example.com/app/a", "io", "io.EOF", "a/a.go", 5),
		refEdge(facts.RefFunc, "example.com/app/a", "os", "os.ReadFile", "a/a.go", 1),
		refEdge(facts.RefFunc, "example.com/app/a", "os", "os.ReadFile", "a/a.go", 1), // exact duplicate
		refEdge(facts.RefVar, "example.com/app/b", "os", "os.Stdin", "b/b.go", 3),
	}
	want := facts.SortReferenceEdges(facts.CloneReferenceEdges(refs))

	rotated := facts.CloneReferenceEdges(refs)
	rotated = append(rotated[:0:0], rotated[2:]...)
	rotated = append(rotated, refs[:2]...)
	got := facts.SortReferenceEdges(rotated)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted order depends on input order:\n got %v\nwant %v", got, want)
	}

	deduped := facts.DedupReferenceEdges(want)
	if len(deduped) != 4 {
		t.Fatalf("DedupReferenceEdges kept %d edges, want 4 (exact duplicate dropped, distinct sites kept): %v", len(deduped), deduped)
	}

	imports := []facts.ImportEdge{
		importEdge("example.com/app/b", "os", facts.ImportResolved, "b/b.go", 1),
		importEdge("example.com/app/a", "io", facts.ImportUnresolved, "a/a.go", 9),
		importEdge("example.com/app/a", "os", facts.ImportResolved, "a/a.go", 2),
		importEdge("example.com/app/a", "os", facts.ImportResolved, "a/a.go", 2), // exact duplicate
	}
	wantImports := facts.SortImportEdges(append([]facts.ImportEdge(nil), imports...))
	gotImports := facts.SortImportEdges([]facts.ImportEdge{imports[2], imports[0], imports[3], imports[1]})
	if !reflect.DeepEqual(gotImports, wantImports) {
		t.Fatalf("import sort order depends on input order:\n got %v\nwant %v", gotImports, wantImports)
	}
	dedupedImports := facts.DedupImportEdges(wantImports)
	if len(dedupedImports) != 3 {
		t.Fatalf("DedupImportEdges kept %d edges, want 3: %v", len(dedupedImports), dedupedImports)
	}

	// Empty and nil inputs are stable no-ops.
	if got := facts.DedupReferenceEdges(nil); got != nil && len(got) != 0 {
		t.Fatalf("DedupReferenceEdges(nil) = %v, want empty", got)
	}
	if got := facts.SortImportEdges(nil); got != nil && len(got) != 0 {
		t.Fatalf("SortImportEdges(nil) = %v, want empty", got)
	}

	// AC5 regression: the same referent observed at two distinct sites
	// survives sort-then-dedup; only exact duplicates (same referent AND
	// same site) collapse.
	sameReferent := []facts.ReferenceEdge{
		refEdge(facts.RefFunc, "example.com/app/a", "os", "os.ReadFile", "a/one.go", 7),
		refEdge(facts.RefFunc, "example.com/app/a", "os", "os.ReadFile", "a/one.go", 7), // exact duplicate
		refEdge(facts.RefFunc, "example.com/app/a", "os", "os.ReadFile", "a/two.go", 7), // distinct site
		refEdge(facts.RefFunc, "example.com/app/a", "os", "os.ReadFile", "a/one.go", 8), // distinct site
	}
	dedupedSame := facts.DedupReferenceEdges(facts.SortReferenceEdges(sameReferent))
	if len(dedupedSame) != 3 {
		t.Fatalf("DedupReferenceEdges kept %d edges, want 3 (two distinct sites, one exact duplicate dropped): %v", len(dedupedSame), dedupedSame)
	}
	sites := map[string]int{}
	for _, e := range dedupedSame {
		sites[e.Site.File+":"+fmt.Sprint(e.Site.Line)]++
	}
	wantSites := []string{"a/one.go:7", "a/one.go:8", "a/two.go:7"}
	for _, w := range wantSites {
		if sites[w] != 1 {
			t.Fatalf("site %s kept %d times, want exactly once; kept edges: %v", w, sites[w], dedupedSame)
		}
	}

	sameReferentImports := []facts.ImportEdge{
		importEdge("example.com/app/a", "os", facts.ImportResolved, "a/one.go", 3),
		importEdge("example.com/app/a", "os", facts.ImportResolved, "a/one.go", 3), // exact duplicate
		importEdge("example.com/app/a", "os", facts.ImportResolved, "a/two.go", 3), // distinct site
	}
	dedupedImportSites := facts.DedupImportEdges(facts.SortImportEdges(sameReferentImports))
	if len(dedupedImportSites) != 2 {
		t.Fatalf("DedupImportEdges kept %d edges, want 2 (distinct site preserved): %v", len(dedupedImportSites), dedupedImportSites)
	}
	if dedupedImportSites[0].Site.File != "a/one.go" || dedupedImportSites[1].Site.File != "a/two.go" {
		t.Fatalf("import dedup lost a distinct site: %v", dedupedImportSites)
	}
}

// The site comparator must be total over the whole int domain, not just the
// validated 1..MaxInt range, so unvalidated values still sort without
// overflow antisymmetry violations.
func TestCompareSourceSiteTotality(t *testing.T) {
	max := facts.SourceSite{File: "a", Line: math.MaxInt}
	neg := facts.SourceSite{File: "a", Line: -1}
	if facts.CompareSourceSite(max, neg) <= 0 {
		t.Fatal("MaxInt line must sort after negative line without overflow")
	}
	// Antisymmetry for every pair of extremes.
	for _, pair := range [][2]facts.SourceSite{
		{max, neg},
		{neg, max},
		{facts.SourceSite{File: "a", Line: math.MaxInt}, facts.SourceSite{File: "a", Line: math.MinInt}},
	} {
		if facts.CompareSourceSite(pair[0], pair[1]) != -facts.CompareSourceSite(pair[1], pair[0]) {
			t.Fatalf("comparator violates antisymmetry for %+v vs %+v", pair[0], pair[1])
		}
	}
	// A consistent sort over shuffled extreme values terminates and is deterministic.
	edges := []facts.ReferenceEdge{
		{Kind: facts.RefFunc, FromPackage: "p", ReferentPackage: "q", Referent: "q.A", Site: facts.SourceSite{File: "a", Line: math.MaxInt}},
		{Kind: facts.RefFunc, FromPackage: "p", ReferentPackage: "q", Referent: "q.A", Site: facts.SourceSite{File: "a", Line: math.MinInt}},
	}
	facts.SortReferenceEdges(edges)
}
