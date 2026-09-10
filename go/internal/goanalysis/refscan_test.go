//go:build integration

package goanalysis_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

const refscanRoot = "testdata/refscan"

const refscanDep = "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"

const refscanMember = "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/member"

const unsafeRefscanMember = "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/unsafe/member"

func loadRefScan(t *testing.T) ([]*packages.Package, facts.MemberSet, string) {
	t.Helper()
	root, err := filepath.Abs(refscanRoot)
	if err != nil {
		t.Fatalf("failed to resolve fixture root: %v", err)
	}
	return loadRefScanPackages(t, root, []string{
		refscanMember + "/builtins",
		refscanMember + "/concrete",
		refscanMember + "/dispatch",
		refscanMember + "/kinds",
	})
}

func loadUnsafeRefScan(t *testing.T) ([]*packages.Package, facts.MemberSet, string) {
	t.Helper()
	root, err := filepath.Abs("testdata/unsafe")
	if err != nil {
		t.Fatalf("failed to resolve unsafe fixture root: %v", err)
	}
	return loadRefScanPackages(t, root, []string{unsafeRefscanMember})
}

func loadRefScanPackages(t *testing.T, root string, members []string) ([]*packages.Package, facts.MemberSet, string) {
	t.Helper()
	ms, err := facts.NewMemberSet(members...)
	if err != nil {
		t.Fatalf("invalid member set: %v", err)
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: root,
	}
	// Explicit package paths: testdata elements are excluded from wildcard
	// matching by the go tool but resolve when named.
	pkgs, err := packages.Load(cfg, members...)
	if err != nil {
		t.Fatalf("failed to load reference-scan fixture packages: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("reference-scan fixture packages have load errors")
	}
	return pkgs, ms, root
}

func edgeSet(edges []facts.ReferenceEdge) map[facts.ReferenceKey]int {
	out := make(map[facts.ReferenceKey]int, len(edges))
	for _, e := range edges {
		out[e.Key()]++
	}
	return out
}

func TestScanReferences_ObservesAllSites(t *testing.T) {
	pkgs, ms, root := loadRefScan(t)

	refs, imports, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err != nil {
		t.Fatalf("unexpected error scanning references: %v", err)
	}

	set := edgeSet(refs)
	expect := func(kind facts.ReferenceKind, fromPkg, referentPkg, referent, file string, line int) {
		t.Helper()
		key := facts.ReferenceKey{
			Kind:            kind,
			FromPackage:     fromPkg,
			ReferentPackage: referentPkg,
			Referent:        facts.SymbolID(referent),
			Site:            facts.SourceSite{File: file, Line: line},
		}
		if set[key] != 1 {
			t.Errorf("expected exactly one edge %+v, got %d (in %d total edges)", key, set[key], len(refs))
		}
	}

	kindsPkg := refscanMember + "/kinds"
	kindsFile := "member/kinds/kinds.go"
	sub := refscanDep + "/sub"
	expect(facts.RefType, kindsPkg, refscanDep, refscanDep+".Base", kindsFile, 11)
	expect(facts.RefField, kindsPkg, refscanDep, refscanDep+".Base", kindsFile, 12)
	expect(facts.RefVar, kindsPkg, refscanDep, refscanDep+".ExportedVar", kindsFile, 13)
	expect(facts.RefConst, kindsPkg, refscanDep, refscanDep+".ExportedConst", kindsFile, 14)
	expect(facts.RefType, kindsPkg, refscanDep, refscanDep+".Plain", kindsFile, 15)
	expect(facts.RefType, kindsPkg, refscanDep, refscanDep+".Alias", kindsFile, 16)
	expect(facts.RefField, kindsPkg, refscanDep, refscanDep+".Base", kindsFile, 17)
	expect(facts.RefType, kindsPkg, refscanDep, refscanDep+".Outer", kindsFile, 18)
	expect(facts.RefField, kindsPkg, refscanDep, refscanDep+".Base", kindsFile, 19) // promoted through embedding
	expect(facts.RefType, kindsPkg, refscanDep, refscanDep+".Box", kindsFile, 20)   // generic instantiation
	expect(facts.RefType, kindsPkg, refscanDep, refscanDep+".Box", kindsFile, 21)
	expect(facts.RefMethod, kindsPkg, refscanDep, "("+refscanDep+".Box).Get", kindsFile, 22)
	expect(facts.RefType, kindsPkg, refscanDep, refscanDep+".Ptr", kindsFile, 23)
	expect(facts.RefMethod, kindsPkg, refscanDep, "("+refscanDep+".Ptr).Touch", kindsFile, 24)
	expect(facts.RefFunc, kindsPkg, refscanDep, refscanDep+".Free", kindsFile, 25) // func value, never called
	expect(facts.RefFunc, kindsPkg, sub, sub+".Sub", kindsFile, 28)

	dispatchPkg := refscanMember + "/dispatch"
	expect(facts.RefFunc, dispatchPkg, refscanDep, refscanDep+".NewGreeter", "member/dispatch/dispatch.go", 8)
	// Fixture 3: dispatch through the declared interface names the interface
	// declaration, not the unexported runtime implementation.
	expect(facts.RefMethod, dispatchPkg, refscanDep, refscanDep+".Greeter", "member/dispatch/dispatch.go", 9)

	concretePkg := refscanMember + "/concrete"
	// Fixture 4: the concrete implementation type is named directly.
	expect(facts.RefType, concretePkg, refscanDep, refscanDep+".Impl", "member/concrete/concrete.go", 8)
	expect(facts.RefMethod, concretePkg, refscanDep, "("+refscanDep+".Impl).Greet", "member/concrete/concrete.go", 9)

	// Universe builtins, labels and other package-less objects produce no edges;
	// package-scoped builtins are retained like any other external object.
	expect(facts.RefFunc, refscanMember+"/builtins", "fmt", "fmt.Println", "member/builtins/builtins.go", 14)
	expect(facts.RefFunc, refscanMember+"/builtins", refscanDep, refscanDep+".Hello", "member/builtins/builtins.go", 14)
	for _, e := range refs {
		switch {
		case e.FromPackage == refscanMember+"/builtins" && e.Site.File == "member/builtins/builtins.go" &&
			e.Site.Line != 14:
			t.Errorf("unexpected edge from builtins fixture at line %d: %+v", e.Site.Line, e)
		case !ms.Contains(e.FromPackage):
			t.Errorf("edge observed from non-member package %q: %+v", e.FromPackage, e)
		}
	}

	// Every written import declaration is an import edge, including the dot
	// and blank imports.
	fileOf := func(pkg string) string {
		switch pkg {
		case kindsPkg:
			return kindsFile
		case dispatchPkg:
			return "member/dispatch/dispatch.go"
		case concretePkg:
			return "member/concrete/concrete.go"
		default:
			return "member/builtins/builtins.go"
		}
	}
	importSet := make(map[facts.ImportKey]int)
	for _, e := range imports {
		importSet[e.Key()]++
	}
	importEdge := func(fromPkg, path string, line int, res facts.ImportResolution) {
		t.Helper()
		key := facts.ImportKey{
			ImportingPackage: fromPkg,
			ImportPath:       path,
			Resolution:       res,
			Site:             facts.SourceSite{File: fileOf(fromPkg), Line: line},
		}
		if importSet[key] != 1 {
			t.Errorf("expected exactly one import edge %+v, got %d (in %d total imports)", key, importSet[key], len(imports))
		}
	}
	importEdge(kindsPkg, refscanDep, 4, facts.ImportResolved)
	importEdge(kindsPkg, sub, 5, facts.ImportResolved)
	importEdge(kindsPkg, refscanDep+"/initpkg", 8, facts.ImportResolved)
	importEdge(dispatchPkg, refscanDep, 4, facts.ImportResolved)
	importEdge(concretePkg, refscanDep, 4, facts.ImportResolved)
	importEdge(refscanMember+"/builtins", "fmt", 4, facts.ImportResolved)
}

func TestScanReferences_UnsafeBuiltinsAndUniverseBuiltins(t *testing.T) {
	pkgs, ms, root := loadUnsafeRefScan(t)

	refs, _, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err != nil {
		t.Fatalf("unexpected error scanning unsafe references: %v", err)
	}

	want := map[facts.ReferenceKey]bool{}
	for _, tc := range []struct {
		name string
		line int
	}{
		{name: "Sizeof", line: 9},
		{name: "StringData", line: 10},
		{name: "Slice", line: 11},
	} {
		want[facts.ReferenceKey{
			Kind:            facts.RefFunc,
			FromPackage:     unsafeRefscanMember,
			ReferentPackage: "unsafe",
			Referent:        facts.SymbolID("unsafe." + tc.name),
			Site:            facts.SourceSite{File: "member/unsafe.go", Line: tc.line},
		}] = true
	}
	if len(refs) != len(want) {
		t.Fatalf("unsafe reference count = %d, want %d: %+v", len(refs), len(want), refs)
	}
	for _, edge := range refs {
		if !want[edge.Key()] {
			t.Errorf("unexpected unsafe reference edge: %+v", edge)
		}
		delete(want, edge.Key())
	}
	for key := range want {
		t.Errorf("missing unsafe reference edge: %+v", key)
	}
}

func TestScanReferences_DedupsUsesAndSelections(t *testing.T) {
	pkgs, ms, root := loadRefScan(t)

	refs, _, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err != nil {
		t.Fatalf("unexpected error scanning references: %v", err)
	}

	kindsPkg := refscanMember + "/kinds"
	sel := facts.ReferenceKey{
		Kind:            facts.RefMethod,
		FromPackage:     kindsPkg,
		ReferentPackage: refscanDep,
		Referent:        facts.SymbolID("(" + refscanDep + ".Box).Get"),
		Site:            facts.SourceSite{File: "member/kinds/kinds.go", Line: 22},
	}
	if n := edgeSet(refs)[sel]; n != 1 {
		t.Fatalf("Uses/Selections overlap for %+v collapsed to %d edges, want exactly 1", sel, n)
	}

	// Distinct sites for the same referent remain distinct edges: the field
	// of Base is read at two different sites.
	field := facts.ReferenceKey{
		Kind:            facts.RefField,
		FromPackage:     kindsPkg,
		ReferentPackage: refscanDep,
		Referent:        facts.SymbolID(refscanDep + ".Base"),
		Site:            facts.SourceSite{File: "member/kinds/kinds.go"},
	}
	if n := edgeSet(refs)[baseAt(field, 12)]; n != 1 {
		t.Errorf("expected exactly one Base field edge at line 12, got %d", n)
	}
	if n := edgeSet(refs)[baseAt(field, 17)]; n != 1 {
		t.Errorf("expected exactly one Base field edge at line 17, got %d", n)
	}

	// Repeated scans over nondeterministic map order yield identical output.
	refs2, _, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err != nil {
		t.Fatalf("unexpected error on second scan: %v", err)
	}
	if !reflect.DeepEqual(facts.SortReferenceEdges(refs), facts.SortReferenceEdges(refs2)) {
		t.Errorf("scan output is not deterministic between runs")
	}
}

// baseAt returns the Base-type key with a new site line, for building
// expectations.
func baseAt(base facts.ReferenceKey, line int) facts.ReferenceKey {
	base.Site.Line = line
	return base
}

func TestScanReferences_MemberOnlyInspection(t *testing.T) {
	pkgs, ms, root := loadRefScan(t)

	refs, imports, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err != nil {
		t.Fatalf("unexpected error scanning references: %v", err)
	}
	for _, e := range refs {
		if !ms.Contains(e.FromPackage) {
			t.Errorf("reference edge observed from non-member package %q: %+v", e.FromPackage, e)
		}
	}
	for _, e := range imports {
		if !ms.Contains(e.ImportingPackage) {
			t.Errorf("import edge observed from non-member package %q: %+v", e.ImportingPackage, e)
		}
	}
}

func TestScanReferences_BadRoot(t *testing.T) {
	pkgs, ms, root := loadRefScan(t)
	// Without module provenance there is no site root to widen to: a root
	// that does not contain the member files must fail closed naming the
	// escaping site.
	for _, p := range pkgs {
		p.Module = nil
	}
	_, _, err := goanalysis.ScanReferences(pkgs, ms, filepath.Join(root, "nowhere"))
	if err == nil || !strings.Contains(err.Error(), "outside the component root") {
		t.Errorf("expected an actionable error naming the escaping site, got: %v", err)
	}
}

func TestScanReferences_IncompleteMemberTypeInfo(t *testing.T) {
	pkgs, ms, root := loadRefScan(t)
	// Simulate incomplete type information for one member package: the scan
	// must fail closed with an actionable error naming the package, not
	// silently omit its facts.
	for _, p := range pkgs {
		if p.PkgPath == refscanMember+"/kinds" {
			p.TypesInfo = nil
		}
	}
	_, _, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err == nil || !strings.Contains(err.Error(), refscanMember+"/kinds") {
		t.Errorf("expected a fail-closed error naming the incomplete member package, got: %v", err)
	}
}

func TestScanReferences_IncompleteImportPackageData(t *testing.T) {
	pkgs, ms, root := loadRefScan(t)
	// A nil import-package entry must not be classified as resolved type
	// data: the deliberate missing-type-data state is preserved.
	for _, p := range pkgs {
		if p.PkgPath == refscanMember+"/builtins" {
			p.Imports["fmt"] = nil
		}
	}
	_, imports, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range imports {
		if e.ImportingPackage == refscanMember+"/builtins" && e.ImportPath == "fmt" {
			if e.Resolution != facts.ImportMissingTypeData {
				t.Errorf("nil import package data must not be ImportResolved, got %q", e.Resolution)
			}
			return
		}
	}
	t.Fatalf("the fmt import edge of the builtins package was not observed")
}

func TestScanReferences_NilTypeInfoMaps(t *testing.T) {
	pkgs, ms, root := loadRefScan(t)
	for _, p := range pkgs {
		if p.PkgPath == refscanMember+"/kinds" {
			p.TypesInfo.Uses = nil
		}
	}
	_, _, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err == nil || !strings.Contains(err.Error(), refscanMember+"/kinds") {
		t.Errorf("expected a fail-closed error for nil Uses map, got: %v", err)
	}

	pkgs, ms, root = loadRefScan(t)
	for _, p := range pkgs {
		if p.PkgPath == refscanMember+"/kinds" {
			p.TypesInfo.Selections = nil
		}
	}
	_, _, err = goanalysis.ScanReferences(pkgs, ms, root)
	if err == nil || !strings.Contains(err.Error(), refscanMember+"/kinds") {
		t.Errorf("expected a fail-closed error for nil Selections map, got: %v", err)
	}

	// Without the loader's import metadata, a written import cannot be
	// distinguished resolved from unresolved: fail closed.
	pkgs, ms, root = loadRefScan(t)
	for _, p := range pkgs {
		if p.PkgPath == refscanMember+"/kinds" {
			p.Imports = nil
		}
	}
	_, _, err = goanalysis.ScanReferences(pkgs, ms, root)
	if err == nil || !strings.Contains(err.Error(), refscanMember+"/kinds") {
		t.Errorf("expected a fail-closed error for a nil Imports map, got: %v", err)
	}
}

func TestScanReferences_ImportResolvedByCanonicalKey(t *testing.T) {
	pkgs, ms, root := loadRefScan(t)
	// Simulate a host whose canonical spelling differs from the source
	// literal: the loader keys p.Imports by the canonical spelling. The
	// written import path must resolve through the canonical form.
	const rewrittenDispatch = "rewritten.example.com/refscan/member/dispatch"
	const rewrittenDep = "rewritten.example.com/refscan/dep"
	prev := hostpolicy.CanonicalizePath
	hostpolicy.CanonicalizePath = func(p string) string {
		switch p {
		case refscanMember + "/dispatch":
			return rewrittenDispatch
		case refscanDep:
			return rewrittenDep
		default:
			return p
		}
	}
	defer func() { hostpolicy.CanonicalizePath = prev }()

	ms, err := facts.NewMemberSet(rewrittenDispatch)
	if err != nil {
		t.Fatalf("invalid member set: %v", err)
	}
	_, imports, err := goanalysis.ScanReferences(pkgs, ms, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range imports {
		if e.ImportingPackage == rewrittenDispatch && e.ImportPath == rewrittenDep {
			if e.Resolution != facts.ImportResolved {
				t.Errorf("import must resolve through the canonical loader key, got %q", e.Resolution)
			}
			return
		}
	}
	t.Fatalf("the dispatch package's dep import edge was not observed")
}
