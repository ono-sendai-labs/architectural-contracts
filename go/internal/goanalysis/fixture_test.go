//go:build integration

package goanalysis_test

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
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

// classifyRoot and classifyMember locate the classify fixture tree: real
// typed member sources whose stdlib and dependency edges are classified
// through checker.ClassifyEdges with a deterministic fake authority map.
const (
	classifyRoot   = "testdata/classify"
	classifyMember = "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/member"
	// classifyInfra is the auto-attached infra dependency's package.
	classifyInfra = "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/infra/infra"
)

// fixtureAuthority is a deterministic fake StdlibAuthority enumerating
// exactly the packages the classify fixtures import, with canned terminal
// records and evidence. It is deliberately not the real map reader: the
// fixtures pin the classification dispatch, not the artifact decoding
// (which stdlibmapreader_test.go pins).
type fixtureAuthority struct {
	key      stdlibauthority.SDKKey
	packages map[string]bool
	symbols  map[facts.SymbolID]stdlibauthority.Classification
	inits    map[string]stdlibauthority.Classification
	evidence map[string][]stdlibauthority.Frame
}

func testSDKKey() stdlibauthority.SDKKey {
	return stdlibauthority.SDKKey{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64", MapFormatVersion: 1}
}

func newFixtureAuthority(t *testing.T) *fixtureAuthority {
	t.Helper()
	auth := &fixtureAuthority{
		key:      testSDKKey(),
		packages: map[string]bool{},
		symbols:  map[facts.SymbolID]stdlibauthority.Classification{},
		inits:    map[string]stdlibauthority.Classification{},
		evidence: map[string][]stdlibauthority.Frame{},
	}
	for _, pkg := range []string{"os", "io", "image/png", "go/ast"} {
		auth.packages[pkg] = true
	}
	auth.symbols[facts.SymbolID("os.ReadFile")] = stdlibauthority.Classification{Capabilities: []string{"FILES"}}
	auth.symbols[facts.SymbolID("os.Stdin")] = stdlibauthority.Classification{Capabilities: []string{"CHDIR", "FILES"}}
	auth.symbols[facts.SymbolID("io.EOF")] = stdlibauthority.Classification{Safe: true}
	auth.inits["image/png"] = stdlibauthority.Classification{Capabilities: []string{"FILES"}}
	auth.inits["go/ast"] = stdlibauthority.Classification{Unanalyzed: true}
	auth.inits["os"] = stdlibauthority.Classification{Safe: true}
	auth.inits["io"] = stdlibauthority.Classification{Safe: true}
	auth.evidence["os.ReadFile FILES"] = []stdlibauthority.Frame{{Function: "os.ReadFile", File: "os/file.go", Line: 331}}
	auth.evidence["image/png.init FILES"] = []stdlibauthority.Frame{{Function: "image/png.init", File: "image/png/reader.go", Line: 10}}
	return auth
}

func (f *fixtureAuthority) IsStdlibPackage(pkgPath string) bool { return f.packages[pkgPath] }

func (f *fixtureAuthority) SymbolAuthority(id symbol.SymbolID) (stdlibauthority.Classification, error) {
	if class, ok := f.symbols[id]; ok {
		return class, nil
	}
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{
		Package: facts.SymbolIDPackage(id),
		Symbol:  id.Format(),
	}
}

func (f *fixtureAuthority) PackageInitAuthority(pkgPath string) (stdlibauthority.Classification, error) {
	if class, ok := f.inits[pkgPath]; ok {
		return class, nil
	}
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{Package: pkgPath}
}

func (f *fixtureAuthority) Evidence(id symbol.SymbolID, cap stdlibauthority.Capability) []stdlibauthority.Frame {
	return f.evidence[id.Format()+" "+cap]
}

func (f *fixtureAuthority) Key() stdlibauthority.SDKKey { return f.key }

// loadClassifyFixturePkgs loads the classify member packages.
func loadClassifyFixturePkgs(t *testing.T) []*packages.Package {
	t.Helper()
	root, err := filepath.Abs(classifyRoot)
	if err != nil {
		t.Fatalf("failed to resolve classify fixture root: %v", err)
	}
	memberPkgs := []string{classifyMember + "/globals", classifyMember + "/rows", classifyMember + "/uses"}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, memberPkgs...)
	if err != nil {
		t.Fatalf("failed to load classify member packages: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("classify member packages have load errors")
	}
	return pkgs
}

// classifyMemberSet is the classify fixture's member set.
func classifyMemberSet(t *testing.T) facts.MemberSet {
	t.Helper()
	ms, err := facts.NewMemberSet(classifyMember+"/globals", classifyMember+"/rows", classifyMember+"/uses")
	if err != nil {
		t.Fatalf("invalid member set: %v", err)
	}
	return ms
}

// scanClassifyFixture scans the loaded classify member packages.
func scanClassifyFixture(t *testing.T, pkgs []*packages.Package) (facts.MemberSet, []facts.ReferenceEdge, []facts.ImportEdge) {
	t.Helper()
	members := classifyMemberSet(t)
	root, err := filepath.Abs(classifyRoot)
	if err != nil {
		t.Fatalf("failed to resolve classify fixture root: %v", err)
	}
	refs, imports, err := goanalysis.ScanReferences(pkgs, members, root)
	if err != nil {
		t.Fatalf("unexpected error scanning references: %v", err)
	}
	return members, refs, imports
}

// loadClassifyFixture scans the classify member packages.
func loadClassifyFixture(t *testing.T) (members facts.MemberSet, refs []facts.ReferenceEdge, imports []facts.ImportEdge) {
	t.Helper()
	return scanClassifyFixture(t, loadClassifyFixturePkgs(t))
}

// loadClassifyFixtureRefs scans the classify member packages and returns only
// the reference edges.
func loadClassifyFixtureRefs(t *testing.T) []facts.ReferenceEdge {
	t.Helper()
	_, refs, _ := loadClassifyFixture(t)
	return refs
}

// classifyFixture runs the full classification over the classify fixture with
// the fake authority and the given dependencies.
func classifyFixture(t *testing.T, deps []facts.DependencyInterface) (checker.Classified, []facts.ReferenceEdge, []facts.ImportEdge) {
	t.Helper()
	members, refs, imports := loadClassifyFixture(t)
	got, err := checker.ClassifyEdges(members, deps, refs, imports, newFixtureAuthority(t), testSDKKey())
	if err != nil {
		t.Fatalf("unexpected classification error: %v", err)
	}
	return got, refs, imports
}

// TestFixture1_FuncValueCarriesStdlibAuthority pins design fixture 1: taking
// os.ReadFile as a value without ever calling it is a reference edge, and its
// map classification contributes FILES authority at that exact source site.
func TestFixture1_FuncValueCarriesStdlibAuthority(t *testing.T) {
	got, refs, _ := classifyFixture(t, nil)
	var site facts.SourceSite
	var sawEdge bool
	for _, e := range refs {
		if e.Referent == facts.SymbolID("os.ReadFile") {
			site, sawEdge = e.Site, true
			if e.Kind != facts.RefFunc {
				t.Fatalf("os.ReadFile must be a func reference, got %q", e.Kind)
			}
		}
	}
	if !sawEdge {
		t.Fatalf("the os.ReadFile reference was not scanned")
	}
	var obs checker.AuthorityObservation
	count := 0
	for _, o := range got.Authority {
		if o.Referent == facts.SymbolID("os.ReadFile") {
			obs, count = o, count+1
		}
	}
	if count != 1 {
		t.Fatalf("want exactly one os.ReadFile observation, got %d", count)
	}
	if obs.Class != capanalyzer.TrueAuthority || obs.Capability != "FILES" || obs.Site != site {
		t.Errorf("os.ReadFile must contribute FILES at the taken site, got %+v", obs)
	}
	if len(obs.Evidence) == 0 || obs.Evidence[0].Function != "os.ReadFile" {
		t.Errorf("observation must carry the map's canned evidence, got %+v", obs.Evidence)
	}
}

// TestFixture9_StdlibGlobalsUseSymbolClassifications pins design fixture 9:
// os.Stdin carries its non-SAFE map authority and io.EOF contributes none,
// based solely on the exact map records.
func TestFixture9_StdlibGlobalsUseSymbolClassifications(t *testing.T) {
	got, refs, _ := classifyFixture(t, nil)
	bySymbol := map[facts.SymbolID][]string{}
	for _, o := range got.Authority {
		bySymbol[o.Referent] = append(bySymbol[o.Referent], o.Capability)
	}
	if caps := bySymbol[facts.SymbolID("os.Stdin")]; len(caps) != 2 || caps[0] != "CHDIR" || caps[1] != "FILES" {
		t.Errorf("os.Stdin must carry its map capabilities CHDIR,FILES, got %v", caps)
	}
	if _, ok := bySymbol[facts.SymbolID("io.EOF")]; ok {
		t.Errorf("io.EOF is SAFE in the map and must contribute no authority")
	}
	var eofEdge bool
	for _, e := range refs {
		if e.Referent == facts.SymbolID("io.EOF") {
			eofEdge = true
		}
	}
	if !eofEdge {
		t.Fatalf("the io.EOF reference was not scanned")
	}
}

// TestFixture2_BlankImportClassifiesAggregateInit pins design fixture 2: a
// blank import of a map-enumerated package attributes the package's init
// authority under the aggregate pkg.init identity at the import site — no
// object-symbol lookup is needed for an import edge.
func TestFixture2_BlankImportClassifiesAggregateInit(t *testing.T) {
	got, _, imports := classifyFixture(t, nil)
	var site facts.SourceSite
	var sawImport bool
	for _, e := range imports {
		if e.ImportPath == "image/png" {
			site, sawImport = e.Site, true
		}
	}
	if !sawImport {
		t.Fatalf("the blank import of image/png was not scanned")
	}
	var obs checker.AuthorityObservation
	count := 0
	for _, o := range got.Authority {
		if o.Referent == facts.SymbolID("image/png.init") {
			obs, count = o, count+1
		}
	}
	if count != 1 {
		t.Fatalf("want exactly one image/png.init observation, got %d (%+v)", count, got.Authority)
	}
	if obs.Class != capanalyzer.TrueAuthority || obs.Capability != "FILES" || obs.Site != site {
		t.Errorf("init authority must be attributed at the import site, got %+v", obs)
	}
}

// TestFixture6_ImportTable pins design fixture 6, the import table, against
// real typed source: the member import is ignored, the stdlib-map import with
// an authority-bearing init carries its aggregate init authority, the
// stdlib-map import with an UNANALYZED init produces an AnalysisDefeating
// observation under the aggregate pkg.init identity, declared and
// auto-attached dependency imports are accepted and mark their dependencies
// used, unowned packages with type data present are UNDECLARED_DEPENDENCY at
// the exact sites, and two direct dependencies claiming one package fail
// closed with DEPENDENCY_OVERLAP before any row is classified. The seventh
// row — a written stdlib path whose loader entry carries no type data — is
// driven through the typed loader's nil-entry seam and classified fail-closed
// in TestFixture6_MissingTypeDataFailsClosed below.
func TestFixture6_ImportTable(t *testing.T) {
	declared, _ := resolveFixtureDep(t)
	infra := facts.DependencyInterface{
		Component:      "infra",
		InterfaceStyle: manifest.InterfaceStylePackageSurface,
		Packages:       []string{classifyInfra},
	}
	got, _, imports := classifyFixture(t, []facts.DependencyInterface{declared, infra})

	rowsPkg := classifyMember + "/rows"
	byPath := map[string]facts.ImportEdge{}
	for _, e := range imports {
		if e.ImportingPackage == rowsPkg {
			byPath[e.ImportPath] = e
		}
	}
	scanned := []string{
		"sort", "errors", "strconv", refscanDep, refscanDep + "/sub",
		classifyMember + "/globals", classifyInfra, "image/png", "go/ast", "os",
	}
	for _, path := range scanned {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("import of %q was not scanned", path)
		}
	}
	// The rows package is the import-only table: every one of its imports is
	// blank (the member, top-level declared dependency, dep/sub,
	// auto-attached infra, and unowned rows included) and it makes no object
	// reference at all.
	for _, path := range scanned {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("import of %q was not scanned", path)
		}
	}
	for _, e := range loadClassifyFixtureRefs(t) {
		if e.FromPackage == rowsPkg {
			t.Errorf("the import-only rows package must not reference objects, got %+v", e)
		}
	}

	// Unowned resolved packages: UNDECLARED_DEPENDENCY for each import at
	// its own site (blank rows in rows.go, named imports in uses.go), plus
	// the object references into them (sort.Ints, errors.Is).
	unowned := map[string]bool{"sort": true, "errors": true, "strconv": true}
	bySite := map[facts.SourceSite]facts.ImportEdge{}
	for _, e := range imports {
		bySite[e.Site] = e
	}
	for _, b := range got.Boundary {
		if b.Kind != "UNDECLARED_DEPENDENCY" || !unowned[b.ReferentPackage] {
			t.Errorf("unexpected boundary observation %+v", b)
			continue
		}
		if b.Referent == "" {
			edge, ok := bySite[b.Site]
			if !ok || edge.ImportPath != b.ReferentPackage || edge.Resolution != facts.ImportResolved {
				t.Errorf("import violation must sit at its own resolved import site, got %+v", b)
			}
		}
	}
	var wantObs int
	for _, e := range imports {
		if unowned[e.ImportPath] {
			wantObs++
		}
	}
	for _, e := range loadClassifyFixtureRefs(t) {
		if unowned[e.ReferentPackage] {
			wantObs++
		}
	}
	if len(got.Boundary) != wantObs {
		t.Errorf("want %d UNDECLARED_DEPENDENCY observations (imports + references into unowned packages), got %d: %+v", wantObs, len(got.Boundary), got.Boundary)
	}

	// Declared and auto-attached dependency imports: accepted behind the
	// boundary and counted used.
	if got, want := got.UsedDependencies, []string{"dep", "infra"}; !reflect.DeepEqual(got, want) {
		t.Errorf("used dependencies = %v, want %v", got, want)
	}

	// Stdlib-map imports: the UNANALYZED init of go/ast produces exactly one
	// AnalysisDefeating observation under the aggregate pkg.init identity at
	// the import site; image/png's FILES init is fixture 2's observation.
	// The globals package's stdlib symbol observations (fixture 1/9's
	// os.ReadFile and os.Stdin) appear exactly once each; io.EOF is SAFE.
	var defeating int
	readFileObs, stdinObs := 0, 0
	for _, o := range got.Authority {
		switch {
		case o.Referent == facts.SymbolID("go/ast.init"):
			defeating++
			if o.Class != capanalyzer.AnalysisDefeating || o.Capability != "" || o.Site != byPath["go/ast"].Site {
				t.Errorf("go/ast.init observation wrong: %+v", o)
			}
		case o.Referent == facts.SymbolID("image/png.init"):
			if o.Class != capanalyzer.TrueAuthority || o.Capability != "FILES" || o.Site != byPath["image/png"].Site {
				t.Errorf("image/png.init observation wrong: %+v", o)
			}
		case o.Referent == facts.SymbolID("os.ReadFile"):
			readFileObs++
			if o.Class != capanalyzer.TrueAuthority || o.Capability != "FILES" {
				t.Errorf("os.ReadFile observation wrong: %+v", o)
			}
		case o.Referent == facts.SymbolID("os.Stdin"):
			stdinObs++
		case o.Referent == facts.SymbolID("io.EOF"):
			t.Errorf("io.EOF is SAFE and must contribute no authority")
		default:
			t.Errorf("unexpected authority observation %+v", o)
		}
	}
	if defeating != 1 {
		t.Errorf("want exactly one AnalysisDefeating init observation for go/ast.init, got %d", defeating)
	}
	if readFileObs != 1 || stdinObs != 2 {
		t.Errorf("want one os.ReadFile and two os.Stdin observations, got %d and %d", readFileObs, stdinObs)
	}

	// Member and dependency imports produce no boundary observation.
	for _, b := range got.Boundary {
		switch b.ReferentPackage {
		case classifyMember + "/globals", refscanDep, refscanDep + "/sub", classifyInfra:
			t.Errorf("member or dependency import must be ignored, got %+v", b)
		}
	}

	// Overlapping direct dependencies: fail closed before classification.
	second := facts.DependencyInterface{
		Component:      "dep-also",
		InterfaceStyle: manifest.InterfaceStylePackageSurface,
		Packages:       declared.Packages,
	}
	members, refs, imps := loadClassifyFixture(t)
	_, err := checker.ClassifyEdges(members, []facts.DependencyInterface{declared, second}, refs, imps, newFixtureAuthority(t), testSDKKey())
	if err == nil || !strings.Contains(err.Error(), "DEPENDENCY_OVERLAP") {
		t.Fatalf("overlapping ownership must fail closed with DEPENDENCY_OVERLAP, got %v", err)
	}
}

// TestFixture6_MissingTypeDataFailsClosed drives the import table's seventh
// row through the typed loader: the rows package's written blank import of
// the map-enumerated package "os" is given a nil loader entry (the seam
// pinned in refscan_test.go), the scan therefore records
// ImportMissingTypeData, and classification fails closed with an actionable
// tool error naming the written path and site — never a false
// UNDECLARED_DEPENDENCY and never a pass.
func TestFixture6_MissingTypeDataFailsClosed(t *testing.T) {
	pkgs := loadClassifyFixturePkgs(t)
	for _, p := range pkgs {
		if p.PkgPath == classifyMember+"/rows" {
			p.Imports["os"] = nil
		}
	}
	members, refs, imports := scanClassifyFixture(t, pkgs)
	var missing bool
	for _, e := range imports {
		if e.ImportingPackage == classifyMember+"/rows" && e.ImportPath == "os" {
			if e.Resolution != facts.ImportMissingTypeData {
				t.Fatalf("nil loader entry must be ImportMissingTypeData, got %q", e.Resolution)
			}
			missing = true
		}
	}
	if !missing {
		t.Fatalf("the os import edge was not observed")
	}
	declared, _ := resolveFixtureDep(t)
	infra := facts.DependencyInterface{
		Component:      "infra",
		InterfaceStyle: manifest.InterfaceStylePackageSurface,
		Packages:       []string{classifyInfra},
	}
	got, err := checker.ClassifyEdges(members, []facts.DependencyInterface{declared, infra}, refs, imports, newFixtureAuthority(t), testSDKKey())
	if err == nil {
		t.Fatalf("missing type data for a written stdlib import must fail closed, got %+v", got)
	}
	if strings.Contains(err.Error(), "UNDECLARED_DEPENDENCY") {
		t.Errorf("missing type data must not render as a violation: %v", err)
	}
	if !strings.Contains(err.Error(), `"os"`) || !strings.Contains(err.Error(), "member/rows/rows.go") {
		t.Errorf("error must name the written path and site, got %v", err)
	}
}
