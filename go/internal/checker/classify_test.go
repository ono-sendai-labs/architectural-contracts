package checker_test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// fakeAuthority is a deterministic StdlibAuthority fake for targeted edge
// behavior: exact terminal records, canned evidence, and a fixed SDK key.
type fakeAuthority struct {
	key      stdlibauthority.SDKKey
	packages map[string]bool
	symbols  map[facts.SymbolID]stdlibauthority.Classification
	inits    map[string]stdlibauthority.Classification
	evidence map[string][]stdlibauthority.Frame
}

func testKey() stdlibauthority.SDKKey {
	return stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		MapFormatVersion: 1,
	}
}

func newFakeAuthority() *fakeAuthority {
	return &fakeAuthority{
		key:      testKey(),
		packages: map[string]bool{},
		symbols:  map[facts.SymbolID]stdlibauthority.Classification{},
		inits:    map[string]stdlibauthority.Classification{},
		evidence: map[string][]stdlibauthority.Frame{},
	}
}

func (f *fakeAuthority) IsStdlibPackage(pkgPath string) bool { return f.packages[pkgPath] }

func (f *fakeAuthority) SymbolAuthority(id symbol.SymbolID) (stdlibauthority.Classification, error) {
	if class, ok := f.symbols[id]; ok {
		return class, nil
	}
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{
		Package: facts.SymbolIDPackage(id),
		Symbol:  id.Format(),
	}
}

func (f *fakeAuthority) PackageInitAuthority(pkgPath string) (stdlibauthority.Classification, error) {
	if class, ok := f.inits[pkgPath]; ok {
		return class, nil
	}
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{Package: pkgPath}
}

func (f *fakeAuthority) Evidence(id symbol.SymbolID, cap stdlibauthority.Capability) []stdlibauthority.Frame {
	return f.evidence[id.Format()+"\x00"+cap]
}

func (f *fakeAuthority) Key() stdlibauthority.SDKKey { return f.key }

func classifyRefEdge(kind facts.ReferenceKind, from, pkg string, sym facts.SymbolID, file string, line int) facts.ReferenceEdge {
	return facts.ReferenceEdge{
		Kind:            kind,
		FromPackage:     from,
		ReferentPackage: pkg,
		Referent:        sym,
		Site:            facts.SourceSite{File: file, Line: line},
	}
}

func importEdge(from, path string, res facts.ImportResolution, file string, line int) facts.ImportEdge {
	return facts.ImportEdge{
		ImportingPackage: from,
		ImportPath:       path,
		Resolution:       res,
		Site:             facts.SourceSite{File: file, Line: line},
	}
}

func mustMembers(t *testing.T, paths ...string) facts.MemberSet {
	t.Helper()
	ms, err := facts.NewMemberSet(paths...)
	if err != nil {
		t.Fatalf("member set: %v", err)
	}
	return ms
}

// classifyAll runs ClassifyEdges with the test target key.
func classifyAll(t *testing.T, members facts.MemberSet, deps []facts.DependencyInterface, refs []facts.ReferenceEdge, imports []facts.ImportEdge, auth stdlibauthority.StdlibAuthority) (checker.Classified, error) {
	t.Helper()
	return checker.ClassifyEdges(members, deps, refs, imports, auth, testKey())
}

// TestClassifyReferences_StdlibAuthorityTable drives the stdlib rows of the
// object-reference dispatch: capabilities become one TrueAuthority observation
// per capability carrying the site, symbol, SDK key and evidence; SAFE
// contributes nothing; UNANALYZED becomes AnalysisDefeating.
func TestClassifyReferences_StdlibAuthorityTable(t *testing.T) {
	auth := newFakeAuthority()
	auth.packages["os"] = true
	auth.packages["io"] = true
	auth.packages["unsafeasm"] = true
	auth.symbols[facts.SymbolID("os.ReadFile")] = stdlibauthority.Classification{Capabilities: []string{"FILES"}}
	auth.symbols[facts.SymbolID("os.Stdin")] = stdlibauthority.Classification{Capabilities: []string{"CHDIR", "FILES"}}
	auth.symbols[facts.SymbolID("io.EOF")] = stdlibauthority.Classification{Safe: true}
	auth.symbols[facts.SymbolID("unsafeasm.Go")] = stdlibauthority.Classification{Unanalyzed: true}
	auth.evidence["os.ReadFile\x00FILES"] = []stdlibauthority.Frame{{Function: "os.ReadFile", File: "os/file.go", Line: 331}}

	members := mustMembers(t, "example.com/comp/api")
	refs := []facts.ReferenceEdge{
		classifyRefEdge(facts.RefFunc, "example.com/comp/api", "os", "os.ReadFile", "member/api/api.go", 8),
		classifyRefEdge(facts.RefVar, "example.com/comp/api", "os", "os.Stdin", "member/api/api.go", 9),
		classifyRefEdge(facts.RefConst, "example.com/comp/api", "io", "io.EOF", "member/api/api.go", 10),
		classifyRefEdge(facts.RefFunc, "example.com/comp/api", "unsafeasm", "unsafeasm.Go", "member/api/api.go", 11),
		classifyRefEdge(facts.RefFunc, "example.com/comp/api", "example.com/comp/api", "example.com/comp/api.Local", "member/api/api.go", 12),
	}

	got, err := classifyAll(t, members, nil, refs, nil, auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var trueAuth, defeating []checker.AuthorityObservation
	for _, obs := range got.Authority {
		if obs.Class == capanalyzer.TrueAuthority {
			trueAuth = append(trueAuth, obs)
		} else {
			defeating = append(defeating, obs)
		}
	}
	if len(got.Boundary) != 0 {
		t.Errorf("no boundary observations expected, got %+v", got.Boundary)
	}
	// os.ReadFile → FILES; os.Stdin → CHDIR, FILES; io.EOF → none.
	if len(trueAuth) != 3 {
		t.Fatalf("want 3 TrueAuthority observations, got %d: %+v", len(trueAuth), got.Authority)
	}
	// Ordered by site: line 8 ReadFile, line 9 Stdin CHDIR then FILES, line 11 defeating.
	if trueAuth[0].Capability != "FILES" || trueAuth[0].Referent != facts.SymbolID("os.ReadFile") ||
		trueAuth[0].Site != (facts.SourceSite{File: "member/api/api.go", Line: 8}) {
		t.Errorf("ReadFile observation wrong: %+v", trueAuth[0])
	}
	if len(trueAuth[0].Evidence) != 1 || trueAuth[0].Evidence[0].Function != "os.ReadFile" {
		t.Errorf("ReadFile evidence must come from the authority port: %+v", trueAuth[0].Evidence)
	}
	if trueAuth[1].Capability != "CHDIR" || trueAuth[1].Referent != facts.SymbolID("os.Stdin") {
		t.Errorf("Stdin CHDIR observation wrong: %+v", trueAuth[1])
	}
	if trueAuth[2].Capability != "FILES" || trueAuth[2].Referent != facts.SymbolID("os.Stdin") ||
		trueAuth[2].Site.Line != 9 {
		t.Errorf("Stdin FILES observation wrong: %+v", trueAuth[2])
	}
	for _, obs := range trueAuth {
		if fields := stdlibauthority.EqualKeys(obs.SDKKey, testKey()); len(fields) > 0 {
			t.Errorf("observations must carry the map's SDK key, mismatched %v (%+v)", fields, obs.SDKKey)
		}
	}
	if len(defeating) != 1 || defeating[0].Referent != facts.SymbolID("unsafeasm.Go") ||
		defeating[0].Capability != "" || defeating[0].Site.Line != 11 {
		t.Errorf("UNANALYZED symbol must produce exactly one AnalysisDefeating observation, got %+v", defeating)
	}
}

// TestClassifyReferences_DependencyTable drives the dependency rows of the
// object-reference dispatch through the same operation.
func TestClassifyReferences_DependencyTable(t *testing.T) {
	auth := newFakeAuthority()
	auth.packages["os"] = true
	auth.symbols[facts.SymbolID("os.ReadFile")] = stdlibauthority.Classification{Safe: true}
	members := mustMembers(t, "example.com/comp/api")
	declared := depInterface("dep", manifest.InterfaceStyleUnspecified,
		[]string{"example.com/dep"}, []facts.SymbolID{facts.SymbolID("example.com/dep.Greeter")})
	surface := depInterface("surf", manifest.InterfaceStylePackageSurface,
		[]string{"example.com/surf"}, nil)
	refs := []facts.ReferenceEdge{
		// PACKAGE_SURFACE: everything exported is authorized.
		classifyRefEdge(facts.RefFunc, "example.com/comp/api", "example.com/surf", "example.com/surf.Anything", "member/api/api.go", 8),
		// Declared interface, listed symbol: pass.
		classifyRefEdge(facts.RefType, "example.com/comp/api", "example.com/dep", "example.com/dep.Greeter", "member/api/api.go", 9),
		// Declared interface, unlisted symbol: CALLS_UNDECLARED_INTERFACE.
		classifyRefEdge(facts.RefMethod, "example.com/comp/api", "example.com/dep", "(example.com/dep.Impl).Greet", "member/api/api.go", 10),
		// Unowned: UNDECLARED_DEPENDENCY.
		classifyRefEdge(facts.RefFunc, "example.com/comp/api", "example.com/other", "example.com/other.Go", "member/api/api.go", 11),
	}
	got, err := classifyAll(t, members, []facts.DependencyInterface{declared, surface}, refs, nil, auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Authority) != 0 {
		t.Errorf("no authority observations expected, got %+v", got.Authority)
	}
	if len(got.Boundary) != 2 {
		t.Fatalf("want 2 boundary observations, got %+v", got.Boundary)
	}
	byLine := map[int]checker.BoundaryObservation{}
	for _, b := range got.Boundary {
		byLine[b.Site.Line] = b
	}
	if b := byLine[10]; b.Kind != "CALLS_UNDECLARED_INTERFACE" || b.Dependency != "dep" ||
		b.Referent != facts.SymbolID("(example.com/dep.Impl).Greet") {
		t.Errorf("line 10 boundary wrong: %+v", b)
	}
	if b := byLine[11]; b.Kind != "UNDECLARED_DEPENDENCY" || b.ReferentPackage != "example.com/other" {
		t.Errorf("line 11 boundary wrong: %+v", b)
	}
}

// TestClassifyImports_Table drives the import dispatch rows.
func TestClassifyImports_Table(t *testing.T) {
	auth := newFakeAuthority()
	auth.packages["stdinit"] = true
	auth.inits["stdinit"] = stdlibauthority.Classification{Capabilities: []string{"FILES"}}
	auth.evidence["stdinit.init\x00FILES"] = []stdlibauthority.Frame{{Function: "stdinit.init", File: "stdinit/init.go", Line: 12}}

	members := mustMembers(t, "example.com/comp/api", "example.com/comp/other")
	declared := depInterface("dep", manifest.InterfaceStyleUnspecified,
		[]string{"example.com/dep"}, []facts.SymbolID{facts.SymbolID("example.com/dep.Greeter")})
	auto := depInterface("infra", manifest.InterfaceStylePackageSurface,
		[]string{"example.com/infra"}, nil)
	imports := []facts.ImportEdge{
		importEdge("example.com/comp/api", "example.com/comp/other", facts.ImportResolved, "member/api/api.go", 4), // member
		importEdge("example.com/comp/api", "stdinit", facts.ImportResolved, "member/api/api.go", 5),                // stdlib init
		importEdge("example.com/comp/api", "example.com/dep", facts.ImportResolved, "member/api/api.go", 6),        // declared dep
		importEdge("example.com/comp/api", "example.com/infra", facts.ImportResolved, "member/api/api.go", 7),      // auto-attached
		importEdge("example.com/comp/api", "example.com/unowned", facts.ImportResolved, "member/api/api.go", 8),    // unowned resolved
		importEdge("example.com/comp/api", "example.com/ghost", facts.ImportUnresolved, "member/api/api.go", 9),    // unowned unresolved
	}
	got, err := classifyAll(t, members, []facts.DependencyInterface{declared, auto}, nil, imports, auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Boundary) != 2 {
		t.Fatalf("want 2 UNDECLARED_DEPENDENCY boundary observations, got %+v", got.Boundary)
	}
	for _, b := range got.Boundary {
		if b.Kind != "UNDECLARED_DEPENDENCY" {
			t.Errorf("boundary kind = %q, want UNDECLARED_DEPENDENCY", b.Kind)
		}
	}
	lines := []int{}
	for _, b := range got.Boundary {
		lines = append(lines, b.Site.Line)
	}
	sort.Ints(lines)
	if lines[0] != 8 || lines[1] != 9 {
		t.Errorf("unowned imports at lines 8 and 9 expected, got %v", lines)
	}
	if len(got.Authority) != 1 {
		t.Fatalf("want 1 init authority observation, got %+v", got.Authority)
	}
	obs := got.Authority[0]
	if obs.Class != capanalyzer.TrueAuthority || obs.Capability != "FILES" ||
		obs.Referent != facts.SymbolID("stdinit.init") ||
		obs.Site != (facts.SourceSite{File: "member/api/api.go", Line: 5}) {
		t.Errorf("stdlib init observation wrong: %+v", obs)
	}
	if len(obs.Evidence) != 1 || obs.Evidence[0].Function != "stdinit.init" {
		t.Errorf("init evidence must come from the authority port: %+v", obs.Evidence)
	}
	if len(got.UsedDependencies) != 2 || got.UsedDependencies[0] != "dep" || got.UsedDependencies[1] != "infra" {
		t.Errorf("used dependencies = %v, want [dep infra]", got.UsedDependencies)
	}
}

// TestClassifyImports_MemberImportIgnored pins that member imports never
// produce observations even at sites that also reference the package.
func TestClassifyImports_MemberImportIgnored(t *testing.T) {
	auth := newFakeAuthority()
	members := mustMembers(t, "example.com/comp/api", "example.com/comp/other")
	got, err := classifyAll(t, members, nil, nil,
		[]facts.ImportEdge{importEdge("example.com/comp/api", "example.com/comp/other", facts.ImportResolved, "member/api/api.go", 4)}, auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Boundary) != 0 || len(got.Authority) != 0 || len(got.UsedDependencies) != 0 {
		t.Errorf("member import must be fully ignored, got %+v", got)
	}
}

// TestClassifyImports_OverlappingDependencies pins R14: two direct
// dependencies claiming one package fail classification before any row is
// classified, with a deterministic DEPENDENCY_OVERLAP tool error.
func TestClassifyImports_OverlappingDependencies(t *testing.T) {
	auth := newFakeAuthority()
	members := mustMembers(t, "example.com/comp/api")
	first := depInterface("a", manifest.InterfaceStylePackageSurface, []string{"example.com/shared"}, nil)
	second := depInterface("b", manifest.InterfaceStylePackageSurface, []string{"example.com/shared"}, nil)
	imports := []facts.ImportEdge{importEdge("example.com/comp/api", "example.com/shared", facts.ImportResolved, "member/api/api.go", 4)}
	_, err := classifyAll(t, members, []facts.DependencyInterface{second, first}, nil, imports, auth)
	if err == nil || !strings.Contains(err.Error(), "DEPENDENCY_OVERLAP") || !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Fatalf("want deterministic DEPENDENCY_OVERLAP tool error, got %v", err)
	}
}

// TestClassifyImports_MissingTypeDataFailsClosed pins the fail-closed row:
// missing type/layout data for a written non-member import is a tool error,
// never an UNDECLARED_DEPENDENCY and never a pass — for stdlib, dependency,
// and unknown written paths alike.
func TestClassifyImports_MissingTypeDataFailsClosed(t *testing.T) {
	auth := newFakeAuthority()
	auth.packages["os"] = true
	members := mustMembers(t, "example.com/comp/api")
	declared := depInterface("dep", manifest.InterfaceStylePackageSurface, []string{"example.com/dep"}, nil)
	for _, path := range []string{"os", "example.com/dep", "example.com/nowhere"} {
		imports := []facts.ImportEdge{importEdge("example.com/comp/api", path, facts.ImportMissingTypeData, "member/api/api.go", 4)}
		got, err := classifyAll(t, members, []facts.DependencyInterface{declared}, nil, imports, auth)
		if err == nil {
			t.Errorf("import %q with missing type data must be a tool error, got %+v", path, got)
			continue
		}
		if strings.Contains(err.Error(), "UNDECLARED_DEPENDENCY") {
			t.Errorf("missing type data must not render as a violation: %v", err)
		}
	}
	// A member's own path with missing data is still just an ignored member
	// import (member → ignore precedes the layout fault).
	got, err := classifyAll(t, members, []facts.DependencyInterface{declared}, nil,
		[]facts.ImportEdge{importEdge("example.com/comp/api", "example.com/comp/api", facts.ImportMissingTypeData, "member/api/api.go", 4)}, auth)
	if err != nil {
		t.Errorf("member written path must be ignored: %v", err)
	} else if len(got.Boundary) != 0 || len(got.Authority) != 0 {
		t.Errorf("member import must produce no observations: %+v", got)
	}
}

// TestClassify_ToolErrors pins every fail-closed fault: symbol inventory gap,
// init record gap, invalid terminal classification, and SDK-key mismatch.
func TestClassify_ToolErrors(t *testing.T) {
	members := mustMembers(t, "example.com/comp/api")

	t.Run("symbol inventory gap", func(t *testing.T) {
		auth := newFakeAuthority()
		auth.packages["os"] = true
		refs := []facts.ReferenceEdge{classifyRefEdge(facts.RefFunc, "example.com/comp/api", "os", "os.Deleted", "member/api/api.go", 8)}
		got, err := classifyAll(t, members, nil, refs, nil, auth)
		if err == nil {
			t.Fatalf("inventory gap must be a tool error, got %+v", got)
		}
		if !errors.Is(err, stdlibauthority.ErrInventoryGap) {
			t.Errorf("error must wrap ErrInventoryGap, got %v", err)
		}
		if !strings.Contains(err.Error(), "member/api/api.go") || !strings.Contains(err.Error(), "os.Deleted") {
			t.Errorf("error must be actionable (site and symbol), got %v", err)
		}
	})

	t.Run("init inventory gap", func(t *testing.T) {
		auth := newFakeAuthority()
		auth.packages["os"] = true // enumerated, but no init record
		imports := []facts.ImportEdge{importEdge("example.com/comp/api", "os", facts.ImportResolved, "member/api/api.go", 4)}
		got, err := classifyAll(t, members, nil, nil, imports, auth)
		if err == nil {
			t.Fatalf("missing init entry must be a tool error, got %+v", got)
		}
		if !errors.Is(err, stdlibauthority.ErrInventoryGap) {
			t.Errorf("error must wrap ErrInventoryGap, got %v", err)
		}
	})

	t.Run("invalid classification", func(t *testing.T) {
		auth := newFakeAuthority()
		auth.packages["os"] = true
		auth.symbols[facts.SymbolID("os.ReadFile")] = stdlibauthority.Classification{} // no terminal state
		refs := []facts.ReferenceEdge{classifyRefEdge(facts.RefFunc, "example.com/comp/api", "os", "os.ReadFile", "member/api/api.go", 8)}
		got, err := classifyAll(t, members, nil, refs, nil, auth)
		if err == nil {
			t.Fatalf("invalid terminal classification must be a tool error, got %+v", got)
		}
		if !strings.Contains(err.Error(), "terminal state") {
			t.Errorf("error must name the invalid classification, got %v", err)
		}
	})

	t.Run("key mismatch", func(t *testing.T) {
		auth := newFakeAuthority()
		auth.key = stdlibauthority.SDKKey{ToolchainVersion: "go1.25.0", GOOS: "linux", GOARCH: "amd64", MapFormatVersion: 1}
		refs := []facts.ReferenceEdge{classifyRefEdge(facts.RefFunc, "example.com/comp/api", "example.com/other", "example.com/other.Go", "member/api/api.go", 8)}
		got, err := classifyAll(t, members, nil, refs, nil, auth)
		if err == nil {
			t.Fatalf("key mismatch must be a tool error, got %+v", got)
		}
		if !errors.Is(err, stdlibauthority.ErrKeyMismatch) {
			t.Errorf("error must wrap ErrKeyMismatch, got %v", err)
		}
	})
}

// TestClassify_ImportAndReferenceRulesSeparate pins acceptance criterion 6:
// a dependency package with init authority and an object outside its declared
// symbol surface is accepted as an import while the object reference
// independently produces CALLS_UNDECLARED_INTERFACE.
func TestClassify_ImportAndReferenceRulesSeparate(t *testing.T) {
	auth := newFakeAuthority()
	members := mustMembers(t, "example.com/comp/api")
	declared := depInterface("dep", manifest.InterfaceStyleUnspecified,
		[]string{"example.com/dep"}, []facts.SymbolID{facts.SymbolID("example.com/dep.Greeter")})
	imports := []facts.ImportEdge{importEdge("example.com/comp/api", "example.com/dep", facts.ImportResolved, "member/api/api.go", 4)}
	refs := []facts.ReferenceEdge{classifyRefEdge(facts.RefMethod, "example.com/comp/api", "example.com/dep", "(example.com/dep.Impl).Greet", "member/api/api.go", 9)}
	got, err := classifyAll(t, members, []facts.DependencyInterface{declared}, refs, imports, auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Boundary) != 1 || got.Boundary[0].Kind != "CALLS_UNDECLARED_INTERFACE" {
		t.Errorf("object reference must independently violate, got %+v", got.Boundary)
	}
	found := false
	for _, dep := range got.UsedDependencies {
		if dep == "dep" {
			found = true
		}
	}
	if !found {
		t.Errorf("import must be accepted behind the boundary and mark dep used, got %v", got.UsedDependencies)
	}
}
