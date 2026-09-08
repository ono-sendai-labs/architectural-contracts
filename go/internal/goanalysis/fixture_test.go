//go:build integration

package goanalysis_test

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// loadScanFixture loads the refscan member packages and the dependency
// package, and returns the component-level pieces the fixture tests need.
func loadScanFixture(t *testing.T) (members facts.MemberSet, refs []facts.ReferenceEdge, imports []facts.ImportEdge) {
	t.Helper()
	root, err := filepath.Abs(refscanRoot)
	if err != nil {
		t.Fatalf("failed to resolve fixture root: %v", err)
	}
	memberPkgs := []string{
		refscanMember + "/builtins",
		refscanMember + "/concrete",
		refscanMember + "/dispatch",
		refscanMember + "/kinds",
	}
	members, err = facts.NewMemberSet(memberPkgs...)
	if err != nil {
		t.Fatalf("invalid member set: %v", err)
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, memberPkgs...)
	if err != nil {
		t.Fatalf("failed to load member packages: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("member packages have load errors")
	}
	refs, imports, err = goanalysis.ScanReferences(pkgs, members, root)
	if err != nil {
		t.Fatalf("unexpected error scanning references: %v", err)
	}
	return members, refs, imports
}

// containsSymbol reports whether the sorted symbol list contains id.
func containsSymbol(symbols []facts.SymbolID, id facts.SymbolID) bool {
	for _, sym := range symbols {
		if sym == id {
			return true
		}
	}
	return false
}

// resolveFixtureDep resolves the refscan dependency through the production
// source-loading resolver, once as a declared interface (api.go is the
// interface file) and once as PACKAGE_SURFACE.
func resolveFixtureDep(t *testing.T) (declared, surface facts.DependencyInterface) {
	t.Helper()
	declaringRoot, err := filepath.Abs(refscanRoot + "/member")
	if err != nil {
		t.Fatalf("failed to resolve declaring root: %v", err)
	}
	declared, err = goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/component.textproto",
	})
	if err != nil {
		t.Fatalf("unexpected error resolving declared dependency: %v", err)
	}
	surface, err = goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/pkg_surface.textproto",
	})
	if err != nil {
		t.Fatalf("unexpected error resolving package-surface dependency: %v", err)
	}
	return declared, surface
}

// TestFixture3_InterfaceDispatchPins design fixture 3: member code invokes
// dep.Greeter.Greet() while the runtime implementation is unexported. The
// typed reference names the declared interface object, so the check passes
// with no dynamically discovered implementor anywhere in the allowed set.
func TestFixture3_InterfaceDispatch(t *testing.T) {
	members, refs, _ := loadScanFixture(t)
	declared, _ := resolveFixtureDep(t)
	if !containsSymbol(declared.Symbols, facts.SymbolID(refscanDep+".Greeter")) {
		t.Fatalf("declared dependency must list Greeter, got %v", declared.Symbols)
	}
	for _, sym := range declared.Symbols {
		if sym == facts.SymbolID("("+refscanDep+".greeterImpl).Greet") || sym == facts.SymbolID(refscanDep+".Impl") {
			t.Fatalf("no implementor may appear in the declared set: %v", declared.Symbols)
		}
	}

	index, err := checker.NewBoundaryIndex(members, []facts.DependencyInterface{declared})
	if err != nil {
		t.Fatalf("boundary index: %v", err)
	}
	var sawGreet bool
	for _, e := range refs {
		if e.FromPackage != refscanMember+"/dispatch" || e.Site.Line != 9 {
			continue
		}
		decision, err := index.Classify(e)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.Kind != facts.RefMethod || e.Referent != facts.SymbolID(refscanDep+".Greeter") {
			t.Fatalf("dispatch reference must name the declared interface, got %+v", e)
		}
		if decision.Status != checker.ReferenceDeclaredDependency {
			t.Errorf("declared interface dispatch must pass, got %q (%s)", decision.Status, decision.ViolationKind())
		}
		sawGreet = true
	}
	if !sawGreet {
		t.Fatalf("the dispatch site was not observed")
	}
}

// TestFixture4_ConcreteImplementationRejected pins design fixture 4 — the
// precision correction: reaching an implementation method that is not in the
// dependency interface is CALLS_UNDECLARED_INTERFACE at the exact site, even
// though the concrete type implements the listed interface.
func TestFixture4_ConcreteImplementationRejected(t *testing.T) {
	members, refs, _ := loadScanFixture(t)
	declared, _ := resolveFixtureDep(t)
	index, err := checker.NewBoundaryIndex(members, []facts.DependencyInterface{declared})
	if err != nil {
		t.Fatalf("boundary index: %v", err)
	}
	var sawMethod bool
	for _, e := range refs {
		if e.FromPackage != refscanMember+"/concrete" {
			continue
		}
		decision, err := index.Classify(e)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		switch {
		case e.Site.Line == 8 && e.Kind == facts.RefType:
			if decision.Status != checker.ReferenceCallsUndeclaredInterface || decision.Dependency != "dep" {
				t.Errorf("concrete type reference must be rejected, got %+v", decision)
			}
		case e.Site.Line == 9 && e.Kind == facts.RefMethod:
			if decision.Status != checker.ReferenceCallsUndeclaredInterface || decision.Dependency != "dep" {
				t.Errorf("concrete method call must be rejected, got %+v", decision)
			}
			if decision.ViolationKind() != "CALLS_UNDECLARED_INTERFACE" {
				t.Errorf("violation kind = %q, want CALLS_UNDECLARED_INTERFACE", decision.ViolationKind())
			}
			sawMethod = true
		}
	}
	if !sawMethod {
		t.Fatalf("the concrete method site was not observed")
	}
}

// TestFixture5_ReferenceKindMatrix pins design fixture 5: every reference
// kind of the declaring-object table, classified against a declared-interface
// dependency and a PACKAGE_SURFACE dependency. Only the api.go symbols are
// declared, so every other kind is a violation against the declared dep and
// authorized against the package-surface dep. Duplicate Uses/Selections
// observations collapse per site without duplicating findings.
func TestFixture5_ReferenceKindMatrix(t *testing.T) {
	members, refs, _ := loadScanFixture(t)
	declared, surface := resolveFixtureDep(t)
	declaredIndex, err := checker.NewBoundaryIndex(members, []facts.DependencyInterface{declared})
	if err != nil {
		t.Fatalf("boundary index: %v", err)
	}
	surfaceIndex, err := checker.NewBoundaryIndex(members, []facts.DependencyInterface{surface})
	if err != nil {
		t.Fatalf("boundary index: %v", err)
	}

	kindsEdges := map[int]facts.ReferenceEdge{}
	for _, e := range refs {
		if e.FromPackage == refscanMember+"/kinds" {
			kindsEdges[e.Site.Line] = e
		}
	}
	// Lines: 11 Base type, 12 field, 13 var, 14 const, 15 type (Plain),
	// 16 alias, 17 field via alias, 18 Outer type, 19 promoted field,
	// 20/21 generic instantiation, 22 value-receiver selection, 23 pointer
	// type, 24 pointer-receiver selection, 25 func value, 28 promoted-name
	// function.
	for line, want := range map[int]checker.ReferenceStatus{
		11: checker.ReferenceCallsUndeclaredInterface,
		12: checker.ReferenceCallsUndeclaredInterface,
		13: checker.ReferenceCallsUndeclaredInterface,
		14: checker.ReferenceCallsUndeclaredInterface,
		15: checker.ReferenceCallsUndeclaredInterface,
		16: checker.ReferenceCallsUndeclaredInterface,
		17: checker.ReferenceCallsUndeclaredInterface,
		18: checker.ReferenceCallsUndeclaredInterface,
		19: checker.ReferenceCallsUndeclaredInterface,
		20: checker.ReferenceCallsUndeclaredInterface,
		21: checker.ReferenceCallsUndeclaredInterface,
		22: checker.ReferenceCallsUndeclaredInterface,
		23: checker.ReferenceCallsUndeclaredInterface,
		24: checker.ReferenceCallsUndeclaredInterface,
		25: checker.ReferenceCallsUndeclaredInterface,
	} {
		// Line 28 names dep/sub.Sub, which no fixture dependency owns: it is
		// an UNDECLARED_DEPENDENCY against both, asserted below.
		e, ok := kindsEdges[line]
		if !ok {
			t.Fatalf("expected an edge at kinds.go line %d, saw none", line)
		}
		decision, err := declaredIndex.Classify(e)
		if err != nil {
			t.Fatalf("unexpected error at line %d: %v", line, err)
		}
		if decision.Status != want {
			t.Errorf("kinds.go:%d (%s %s) = %q, want %q", line, e.Kind, e.Referent, decision.Status, want)
		}
		surfaceDecision, err := surfaceIndex.Classify(e)
		if err != nil {
			t.Fatalf("unexpected error at line %d: %v", line, err)
		}
		if surfaceDecision.Status != checker.ReferenceDeclaredDependency {
			t.Errorf("kinds.go:%d (%s %s) = %q against PACKAGE_SURFACE, want authorized", line, e.Kind, e.Referent, surfaceDecision.Status)
		}
	}

	// Distinct sites stay distinct edges (Base field read at two sites), so
	// duplicate observations do not duplicate findings but do not collapse
	// distinct sites either.
	var fieldEdges int
	for _, e := range refs {
		if e.FromPackage == refscanMember+"/kinds" && e.Kind == facts.RefField && e.Referent == facts.SymbolID(refscanDep+".Base") {
			fieldEdges++
		}
	}
	if fieldEdges != 4 {
		t.Errorf("expected four distinct Base field sites (lines 12, 17, 19, 27), got %d", fieldEdges)
	}
	e28, ok := kindsEdges[28]
	if !ok {
		t.Fatalf("expected the dep/sub.Sub edge at kinds.go line 28")
	}
	// The declared dep's source root owns dep/sub, so the edge is an
	// undeclared interface there; the PACKAGE_SURFACE dep's owned packages
	// are exactly its declared member, so the edge is an undeclared
	// dependency there.
	if decision, err := declaredIndex.Classify(e28); err != nil || decision.Status != checker.ReferenceCallsUndeclaredInterface {
		t.Errorf("dep/sub.Sub against declared dep = %+v (%v), want calls_undeclared_interface", decision, err)
	}
	if decision, err := surfaceIndex.Classify(e28); err != nil || decision.Status != checker.ReferenceUndeclaredDependency {
		t.Errorf("dep/sub.Sub against surface dep = %+v (%v), want undeclared_dependency", decision, err)
	}
}

// TestFixture6_ResolutionMatchesSurface pins acceptance criterion 6: the
// resolved declared-interface set equals the Step 5 surface extraction over
// the same surviving interface files, and contains no symbol admitted only
// through types.Implements.
func TestFixture6_ResolutionMatchesSurface(t *testing.T) {
	declaringRoot, err := filepath.Abs(refscanRoot + "/member")
	if err != nil {
		t.Fatalf("failed to resolve declaring root: %v", err)
	}
	resolved, err := goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/component.textproto",
	})
	if err != nil {
		t.Fatalf("unexpected error resolving dependency: %v", err)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: filepath.Join(declaringRoot, ".."),
	}
	depPath := refscanDep
	depPkgs, err := packages.Load(cfg, depPath)
	if err != nil {
		t.Fatalf("failed to load dependency package: %v", err)
	}
	if packages.PrintErrors(depPkgs) > 0 {
		t.Fatalf("dependency package has load errors")
	}
	var apiFiles []*ast.File
	var infos *types.Info
	for _, p := range depPkgs {
		for _, f := range p.Syntax {
			pos := p.Fset.Position(f.Pos())
			if filepath.Base(pos.Filename) == "api.go" {
				apiFiles = append(apiFiles, f)
				infos = p.TypesInfo
			}
		}
	}
	if len(apiFiles) == 0 {
		t.Fatalf("interface file api.go was not loaded")
	}
	want := symbol.ExtractSurface(apiFiles, infos)
	got := make([]symbol.SymbolID, 0, len(resolved.Symbols))
	for _, id := range resolved.Symbols {
		got = append(got, symbol.SymbolID(id))
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("resolved symbols\n%v\n!= surface extraction\n%v", got, want)
	}
	for _, id := range got {
		if id == symbol.SymbolID("("+depPath+".greeterImpl).Greet") || id == symbol.SymbolID("("+depPath+".Impl).Greet") {
			t.Errorf("implements-only symbol %q must not appear", id)
		}
	}
}
