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
	for _, pkg := range []string{"os", "io", "image/png"} {
		auth.packages[pkg] = true
	}
	auth.symbols[facts.SymbolID("os.ReadFile")] = stdlibauthority.Classification{Capabilities: []string{"FILES"}}
	auth.symbols[facts.SymbolID("os.Stdin")] = stdlibauthority.Classification{Capabilities: []string{"CHDIR", "FILES"}}
	auth.symbols[facts.SymbolID("io.EOF")] = stdlibauthority.Classification{Safe: true}
	auth.inits["image/png"] = stdlibauthority.Classification{Capabilities: []string{"FILES"}}
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

// loadClassifyFixture scans the classify member packages.
func loadClassifyFixture(t *testing.T) (members facts.MemberSet, refs []facts.ReferenceEdge, imports []facts.ImportEdge) {
	t.Helper()
	root, err := filepath.Abs(classifyRoot)
	if err != nil {
		t.Fatalf("failed to resolve classify fixture root: %v", err)
	}
	memberPkgs := []string{classifyMember + "/globals", classifyMember + "/rows"}
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
		t.Fatalf("failed to load classify member packages: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("classify member packages have load errors")
	}
	refs, imports, err = goanalysis.ScanReferences(pkgs, members, root)
	if err != nil {
		t.Fatalf("unexpected error scanning references: %v", err)
	}
	return members, refs, imports
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
// symbol lookup and no call edge involved.
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
// real typed source: member imports are ignored, a stdlib-map import carries
// its aggregate init authority, a declared dependency's import is accepted
// and marks the dependency used, an unowned package with type data present is
// UNDECLARED_DEPENDENCY at the exact site, and two direct dependencies
// claiming one package fail closed with DEPENDENCY_OVERLAP before any row is
// classified. The seventh row — a written declared/stdlib path with missing
// type data — is a loader-seam state (ImportMissingTypeData, produced by the
// nil-entry seam pinned in refscan_test.go) and is pinned fail-closed at the
// classifier level in checker's TestClassifyImports_MissingTypeDataFailsClosed.
func TestFixture6_ImportTable(t *testing.T) {
	declared, _ := resolveFixtureDep(t)
	got, _, imports := classifyFixture(t, []facts.DependencyInterface{declared})

	rowsPkg := classifyMember + "/rows"
	byPath := map[string]facts.ImportEdge{}
	for _, e := range imports {
		if e.ImportingPackage == rowsPkg {
			byPath[e.ImportPath] = e
		}
	}
	for _, path := range []string{"sort", refscanDep, classifyMember + "/globals", "image/png"} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("import of %q was not scanned", path)
		}
	}

	// Unowned resolved package: UNDECLARED_DEPENDENCY both for the import
	// itself (empty referent, import site) and for the object reference
	// naming sort.Ints (import row 6 and reference row V2 are separate
	// observations at separate sites).
	var sortObs int
	for _, b := range got.Boundary {
		if b.ReferentPackage == "sort" {
			sortObs++
			if b.Kind != "UNDECLARED_DEPENDENCY" {
				t.Errorf("sort boundary wrong: %+v", b)
			}
			if b.Referent == "" && b.Site != byPath["sort"].Site {
				t.Errorf("import violation must sit at the import site, got %+v", b)
			}
		}
	}
	if sortObs != 2 {
		t.Errorf("want exactly two UNDECLARED_DEPENDENCY observations for sort (import + reference), got %d (%+v)", sortObs, got.Boundary)
	}

	// Declared dependency: accepted behind the boundary and counted used.
	found := false
	for _, dep := range got.UsedDependencies {
		if dep == "dep" {
			found = true
		}
	}
	if !found {
		t.Errorf("the declared dependency import must mark dep used, got %v", got.UsedDependencies)
	}

	// Member import: no observation anywhere.
	for _, b := range got.Boundary {
		if b.ReferentPackage == classifyMember+"/globals" {
			t.Errorf("member import must be ignored, got %+v", b)
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
