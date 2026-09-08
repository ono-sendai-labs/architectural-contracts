package checker_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// checkKey is the target SDK key every stub-authority classification is
// decided under in these tests.
var checkKey = stdlibauthority.SDKKey{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64", MapFormatVersion: 1}

// stubClass is one canned terminal classification.
type stubClass struct {
	safe  bool
	unana bool
	caps  []string
}

// stubAuthority is a deterministic fake StdlibAuthority over an explicit
// package/symbol/init table, mirroring the production reader's total-inventory
// fail-closed contract (unknown symbols are inventory gaps).
type stubAuthority struct {
	packages map[string]bool
	symbols  map[symbol.SymbolID]stubClass
	inits    map[string]stubClass
}

func newStubAuthority() *stubAuthority {
	return &stubAuthority{
		packages: map[string]bool{},
		symbols:  map[symbol.SymbolID]stubClass{},
		inits:    map[string]stubClass{},
	}
}

func (s *stubAuthority) withStdlib(pkg string, inits ...stubClass) *stubAuthority {
	s.packages[pkg] = true
	if len(inits) == 1 {
		s.inits[pkg] = inits[0]
	}
	return s
}

func (s *stubAuthority) withSymbol(id string, class stubClass) *stubAuthority {
	s.symbols[symbol.SymbolID(id)] = class
	return s
}

func (s *stubAuthority) IsStdlibPackage(pkgPath string) bool { return s.packages[pkgPath] }

func (s *stubAuthority) SymbolAuthority(id symbol.SymbolID) (stdlibauthority.Classification, error) {
	class, ok := s.symbols[id]
	if !ok {
		return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{
			Package: facts.SymbolIDPackage(id),
			Symbol:  id.Format(),
		}
	}
	return classificationOf(class), nil
}

func (s *stubAuthority) PackageInitAuthority(pkgPath string) (stdlibauthority.Classification, error) {
	class, ok := s.inits[pkgPath]
	if !ok {
		return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{Package: pkgPath}
	}
	return classificationOf(class), nil
}

func (s *stubAuthority) Evidence(id symbol.SymbolID, cap stdlibauthority.Capability) []stdlibauthority.Frame {
	return []stdlibauthority.Frame{{Function: id.Format(), File: "stub.go", Line: 1}}
}

func (s *stubAuthority) Key() stdlibauthority.SDKKey { return checkKey }

func classificationOf(c stubClass) stdlibauthority.Classification {
	return stdlibauthority.Classification{Safe: c.safe, Unanalyzed: c.unana, Capabilities: c.caps}
}

// site builds a valid source site for a test edge.
func site(file string, line int) facts.SourceSite {
	return facts.SourceSite{File: file, Line: line}
}

// impEdge builds a resolved import edge.
func impEdge(from, path, file string, line int) facts.ImportEdge {
	return facts.ImportEdge{
		ImportingPackage: from,
		ImportPath:       path,
		Resolution:       facts.ImportResolved,
		Site:             site(file, line),
	}
}

// checkRefEdge builds a reference edge to a declaring-object symbol.
func checkRefEdge(kind facts.ReferenceKind, from, id, file string, line int) facts.ReferenceEdge {
	return facts.ReferenceEdge{
		Kind:            kind,
		FromPackage:     from,
		ReferentPackage: facts.SymbolIDPackage(facts.SymbolID(id)),
		Referent:        facts.SymbolID(id),
		Site:            site(file, line),
	}
}

// check runs Check and fails the test on a tool error.
func check(t *testing.T, in checker.Inputs) report.ConformanceReport {
	t.Helper()
	rep, err := checker.Check(in)
	if err != nil {
		t.Fatalf("unexpected tool error: %v", err)
	}
	return rep
}

func TestCheck_FR3_ConformingImports(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1", Manifest: "dep1/manifest"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg1", "github.com/dep1/pkg", "pkg1.go", 3),
				impEdge("mycomponent/pkg1", "fmt", "pkg1.go", 4),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
			},
		},
		Authority: newStubAuthority().withStdlib("fmt", stubClass{safe: true}),
		SDKKey:    checkKey,
		Policy:    capanalyzer.StrictPolicy(),
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(rep.Warnings), rep.Warnings)
	}
}

func TestCheck_PopulatesDependenciesListingAndNoFindings(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "b-dep"},
				{Name: "a-dep"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg1", "github.com/a-dep/pkg", "pkg1.go", 3),
				impEdge("mycomponent/pkg1", "github.com/b-dep/pkg", "pkg1.go", 4),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "b-dep",
				Packages:  []string{"github.com/b-dep/pkg"},
			},
			{
				Component: "a-dep",
				Packages:  []string{"github.com/a-dep/pkg"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)

	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(rep.Warnings), rep.Warnings)
	}

	wantDeps := []report.DependencyBoundary{
		{Component: "a-dep"},
		{Component: "b-dep"},
	}

	if !reflect.DeepEqual(rep.Dependencies, wantDeps) {
		t.Errorf("rep.Dependencies =\n%#v\nwant:\n%#v", rep.Dependencies, wantDeps)
	}
}

func TestCheck_DeclaredMembershipScopesPackageSweep(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:    "component",
			Members: []string{"component/member"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "component/member",
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "component/member.Run", File: "member.go"},
					},
				},
			},
			Imports: []facts.ImportEdge{
				impEdge("component/member", "example.com/transitive", "member.go", 3),
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected one violation from the member package, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if !strings.Contains(rep.Violations[0].Message, `undeclared dependency "example.com/transitive"`) {
		t.Fatalf("unexpected violation: %q", rep.Violations[0].Message)
	}
}

func TestCheck_StdlibMembershipIsMapEnumeration(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg1"}},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg1", "fmt", "pkg1.go", 3),
				impEdge("mycomponent/pkg1", "notstd/fmt", "pkg1.go", 4),
			},
		},
		// The map enumerates fmt and not fmt-like path lookalikes: a
		// package outside the enumeration is never standard library (R6).
		Authority: newStubAuthority().withStdlib("fmt", stubClass{safe: true}),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected one undeclared dependency, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if got := rep.Violations[0].Kind; got != report.UndeclaredDependency {
		t.Fatalf("finding kind = %s, want %s", got, report.UndeclaredDependency)
	}
	if !strings.Contains(rep.Violations[0].Message, "notstd/fmt") {
		t.Errorf("violation must name the unenumerated import, got %q", rep.Violations[0].Message)
	}
}

func TestCheck_EmptyMembersRetainsFR1PackageMembership(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "component"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "component/member"},
				{
					ImportPath: "component/transitive",
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "component/transitive.Helper", File: "transitive.go"},
					},
				},
			},
			Imports: []facts.ImportEdge{
				impEdge("component/member", "component/transitive", "member.go", 3),
				impEdge("component/transitive", "example.com/only-in-transitive", "transitive.go", 3),
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("FR1 empty-members result changed: got %#v", rep.Violations)
	}
	v := rep.Violations[0]
	if v.Kind != report.UndeclaredDependency {
		t.Fatalf("kind = %s, want UNDECLARED_DEPENDENCY", v.Kind)
	}
	if !strings.Contains(v.Message, `undeclared dependency "example.com/only-in-transitive"`) ||
		!strings.Contains(v.Message, "transitive.go:3") {
		t.Fatalf("violation = %q, want the transitive import named at its site", v.Message)
	}
}

func TestCheck_InterfacePackageIsImplicitMember(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "component",
			InterfaceFiles: []string{"pkg/api.go"},
			Members:        []string{"component/implementation"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "component/interface",
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "component/interface.API", File: "pkg/api.go", Kind: "type"},
					},
				},
				{ImportPath: "component/implementation"},
			},
			Imports: []facts.ImportEdge{
				impEdge("component/interface", "example.com/interface-dependency", "pkg/api.go", 3),
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected the implicit interface member to be swept, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if !strings.Contains(rep.Violations[0].Message, `undeclared dependency "example.com/interface-dependency"`) {
		t.Fatalf("unexpected violation: %q", rep.Violations[0].Message)
	}
}

func TestCheck_InterfacePackageSelectionIsIdempotent(t *testing.T) {
	tests := []struct {
		name    string
		members []string
	}{
		{name: "literal", members: []string{"component/interface"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := checker.Inputs{
				Manifest: manifest.Manifest{
					Name:           "component",
					InterfaceFiles: []string{"pkg/api.go"},
					Members:        tt.members,
				},
				Facts: facts.PackageFacts{
					Packages: []facts.PackageFact{{
						ImportPath: "component/interface",
						ExportedSymbols: []facts.ExportedSymbol{
							{Name: "component/interface.API", File: "pkg/api.go", Kind: "type"},
						},
					}},
					Imports: []facts.ImportEdge{
						impEdge("component/interface", "example.com/interface-dependency", "pkg/api.go", 3),
					},
				},
				Authority: newStubAuthority(),
				SDKKey:    checkKey,
			}

			rep := check(t, in)
			if len(rep.Violations) != 1 {
				t.Fatalf("expected one interface-package violation, got %d: %v", len(rep.Violations), rep.Violations)
			}
		})
	}
}

func TestCheck_FR4_IgnoresNonMemberPackageFacts(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "component",
			InterfaceFiles: []string{"api.go"},
			Members:        []string{"component/member"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "component/member",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name: "component/member.Widget",
							File: "api.go",
							Kind: "type",
						},
					},
				},
				{
					ImportPath: "example.com/non-member",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name:     "(*component/member.Widget).Run",
							File:     "non-member_impl.go",
							Kind:     "method",
							Receiver: "(*component/member.Widget)",
						},
					},
				},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Fatalf("non-member facts must not produce FR4 findings, got %v", rep.Violations)
	}
}

func TestCheck_FR3_UndeclaredImport(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg1",
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "mycomponent/pkg1.Func", File: "pkg1.go"},
					},
				},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg1", "github.com/bad/lib", "pkg1.go", 5),
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(rep.Violations))
	}
	v := rep.Violations[0]
	if v.Kind != report.UndeclaredDependency {
		t.Errorf("expected kind %s, got %s", report.UndeclaredDependency, v.Kind)
	}
	expectedMsg := `package "mycomponent/pkg1" imports undeclared dependency "github.com/bad/lib" at pkg1.go:5`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}
	if v.Location.File != "pkg1.go" || v.Location.Line != 5 {
		t.Errorf("expected location pkg1.go:5, got %+v", v.Location)
	}
}

func TestCheck_FR3_StdlibAndIntraComponentAllowed(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
				{ImportPath: "mycomponent/pkg2"},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg1", "mycomponent/pkg2", "pkg1.go", 3),
				impEdge("mycomponent/pkg1", "os", "pkg1.go", 4),
				impEdge("mycomponent/pkg1", "net/http", "pkg1.go", 5),
			},
		},
		Authority: newStubAuthority().withStdlib("os", stubClass{safe: true}).withStdlib("net/http", stubClass{safe: true}),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR3_UnownedImportIsUndeclaredDependency(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg1", "github.com/foo/bar", "pkg1.go", 3),
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if rep.Violations[0].Kind != report.UndeclaredDependency {
		t.Errorf("expected violation kind %s, got %s", report.UndeclaredDependency, rep.Violations[0].Kind)
	}
	if !strings.Contains(rep.Violations[0].Message, "github.com/foo/bar") {
		t.Errorf("expected violation to name the undeclared import, got %q", rep.Violations[0].Message)
	}
}

func TestCheck_FR3_UnresolvedImportIsUndeclaredDependency(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
			Imports: []facts.ImportEdge{
				{
					ImportingPackage: "mycomponent/pkg1",
					ImportPath:       "example.com/missing",
					Resolution:       facts.ImportUnresolved,
					Site:             site("pkg1.go", 3),
				},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(rep.Violations), rep.Violations)
	}
	v := rep.Violations[0]
	if v.Kind != report.UndeclaredDependency {
		t.Fatalf("kind = %s, want UNDECLARED_DEPENDENCY", v.Kind)
	}
	if !strings.Contains(v.Message, "example.com/missing") || !strings.Contains(v.Message, "pkg1.go:3") {
		t.Fatalf("violation = %q, want the unresolved import named at its site", v.Message)
	}
}

func TestCheck_FR3_UnusedDependencies(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1", Manifest: "dep1/manifest"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(rep.Warnings))
	}

	expectedWarns := map[string]bool{
		`declared component dependency "dep1" is unused`: true,
	}

	for _, w := range rep.Warnings {
		if w.Kind != report.UnusedDependency {
			t.Errorf("expected warning kind %s, got %s", report.UnusedDependency, w.Kind)
		}
		if !expectedWarns[w.Message] {
			t.Errorf("unexpected warning message: %q", w.Message)
		}
	}
}

func TestCheck_FR3_DeterministicOutput(t *testing.T) {
	build := func() checker.Inputs {
		return checker.Inputs{
			Manifest: manifest.Manifest{
				Name: "mycomponent",
			},
			Facts: facts.PackageFacts{
				Packages: []facts.PackageFact{
					{ImportPath: "mycomponent/pkgb"},
					{ImportPath: "mycomponent/pkga"},
				},
				Imports: []facts.ImportEdge{
					impEdge("mycomponent/pkgb", "github.com/bad2", "b.go", 2),
					impEdge("mycomponent/pkgb", "github.com/bad1", "b.go", 3),
					impEdge("mycomponent/pkga", "github.com/bad3", "a.go", 4),
				},
			},
			Authority: newStubAuthority(),
			SDKKey:    checkKey,
		}
	}

	var first report.ConformanceReport
	for i := 0; i < 50; i++ {
		rep := check(t, build())
		if i == 0 {
			first = rep
			continue
		}
		if !reflect.DeepEqual(rep.Violations, first.Violations) {
			t.Fatalf("run %d reordered violations: %#v vs %#v", i, rep.Violations, first.Violations)
		}
	}
	if len(first.Violations) != 3 {
		t.Fatalf("expected 3 violations, got %d", len(first.Violations))
	}

	// Order must be deterministic across the three sites: bad3 (a.go:4),
	// bad1 (b.go:3), bad2 (b.go:2).
	wantMsgs := []string{
		`package "mycomponent/pkga" imports undeclared dependency "github.com/bad3" at a.go:4`,
		`package "mycomponent/pkgb" imports undeclared dependency "github.com/bad1" at b.go:3`,
		`package "mycomponent/pkgb" imports undeclared dependency "github.com/bad2" at b.go:2`,
	}
	for i, want := range wantMsgs {
		if first.Violations[i].Message != want {
			t.Errorf("violation %d = %q, want %q", i, first.Violations[i].Message, want)
		}
	}
}

func TestCheck_FR4_MethodOutsideInterface(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "mycomponent",
			InterfaceFiles: []string{"pkg/types.go"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name: "mycomponent/pkg.DB",
							File: "pkg/types.go",
							Kind: "type",
						},
						{
							Name:     "(*mycomponent/pkg.DB).Get",
							File:     "pkg/db_impl.go",
							Kind:     "method",
							Receiver: "(*mycomponent/pkg.DB)",
						},
					},
				},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected exactly 1 violation, got %d", len(rep.Violations))
	}
	v := rep.Violations[0]
	if v.Kind != report.MethodOutsideInterface {
		t.Errorf("expected kind %s, got %s", report.MethodOutsideInterface, v.Kind)
	}
	expectedMsg := `exported method "(*mycomponent/pkg.DB).Get" with receiver "(*mycomponent/pkg.DB)" declared in non-interface file "pkg/db_impl.go"`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}
	if v.Location.File != "pkg/db_impl.go" {
		t.Errorf("expected location file \"pkg/db_impl.go\", got %q", v.Location.File)
	}
}

func TestCheck_FR4_MethodSplitClean(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "mycomponent",
			InterfaceFiles: []string{"pkg/types.go", "pkg/api.go"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name: "mycomponent/pkg.DB",
							File: "pkg/types.go",
							Kind: "type",
						},
						{
							Name:     "(*mycomponent/pkg.DB).Get",
							File:     "pkg/api.go",
							Kind:     "method",
							Receiver: "(*mycomponent/pkg.DB)",
						},
					},
				},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR4_InterfaceTypeImplementationExempt(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "mycomponent",
			InterfaceFiles: []string{"pkg/types.go"}, // Store interface declared in types.go
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name: "mycomponent/pkg.Store", // Interface type
							File: "pkg/types.go",
							Kind: "type",
						},
						{
							Name: "mycomponent/pkg.StoreImpl", // Concrete implementation type
							File: "pkg/impl.go",               // Declared in non-interface file
							Kind: "type",
						},
						{
							Name:     "(*mycomponent/pkg.StoreImpl).Get", // Concrete implementation method
							File:     "pkg/impl.go",                      // Declared in non-interface file
							Kind:     "method",
							Receiver: "(*mycomponent/pkg.StoreImpl)",
						},
					},
				},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR4_MemberImplementationInitClean(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "mycomponent",
			InterfaceFiles: []string{"pkg/types.go"},
			Members:        []string{"mycomponent/pkg/impl"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name: "mycomponent/pkg.API",
							File: "pkg/types.go",
							Kind: "type",
						},
					},
				},
				{
					ImportPath: "mycomponent/pkg/impl",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name: "mycomponent/pkg/impl.init",
							File: "pkg/impl/init.go",
							Kind: "init",
						},
					},
				},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR4_ArchitecturePrivateSymbolClean(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "mycomponent",
			InterfaceFiles: []string{"pkg/types.go"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name: "mycomponent/pkg.HelperFunc", // Exported, but not in any interface file, and not method/init
							File: "pkg/impl.go",
							Kind: "func",
						},
					},
				},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(rep.Warnings), rep.Warnings)
	}
}

func TestCheck_FR4_Combined(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "mycomponent",
			InterfaceFiles: []string{"pkg/types.go"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					ExportedSymbols: []facts.ExportedSymbol{
						{
							Name: "mycomponent/pkg.DB",
							File: "pkg/types.go",
							Kind: "type",
						},
						{
							Name:     "(*mycomponent/pkg.DB).Get",
							File:     "pkg/db_impl.go", // Declared in non-interface file -> FR4 violation
							Kind:     "method",
							Receiver: "(*mycomponent/pkg.DB)",
						},
					},
				},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg", "github.com/bad/lib", "pkg/types.go", 1), // Undeclared import -> FR3 violation
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 2 {
		t.Fatalf("expected exactly 2 violations, got %d", len(rep.Violations))
	}

	// Orders must be sorted alphabetically by message:
	// "exported method..." starts with 'e'
	// "package..." starts with 'p'
	v1 := rep.Violations[0]
	if v1.Kind != report.MethodOutsideInterface {
		t.Errorf("expected kind %s, got %s", report.MethodOutsideInterface, v1.Kind)
	}
	if v1.Location.File != "pkg/db_impl.go" {
		t.Errorf("expected location file \"pkg/db_impl.go\", got %q", v1.Location.File)
	}

	v2 := rep.Violations[1]
	if v2.Kind != report.UndeclaredDependency {
		t.Errorf("expected kind %s, got %s", report.UndeclaredDependency, v2.Kind)
	}
	if v2.Location.File != "pkg/types.go" {
		t.Errorf("expected location file \"pkg/types.go\", got %q", v2.Location.File)
	}

	// Rendered text should include both violations
	rendered := report.RenderText(rep)
	if !strings.Contains(rendered, "METHOD_OUTSIDE_INTERFACE") {
		t.Errorf("expected rendered text to contain METHOD_OUTSIDE_INTERFACE, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "UNDECLARED_DEPENDENCY") {
		t.Errorf("expected rendered text to contain UNDECLARED_DEPENDENCY, got:\n%s", rendered)
	}
}

func TestCheck_FR5_UndeclaredInterfaceReference(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "github.com/dep1/pkg.PrivateFunc", "pkg.go", 9),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []facts.SymbolID{"github.com/dep1/pkg.PublicFunc"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected exactly 1 violation, got %d", len(rep.Violations))
	}
	v := rep.Violations[0]
	if v.Kind != report.CallsUndeclaredInterface {
		t.Errorf("expected kind %s, got %s", report.CallsUndeclaredInterface, v.Kind)
	}
	expectedMsg := `package "mycomponent/pkg" references undeclared interface symbol "github.com/dep1/pkg.PrivateFunc" of dependency "dep1" at pkg.go:9`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}
	if v.Location.File != "pkg.go" || v.Location.Line != 9 {
		t.Errorf("expected location pkg.go:9, got %+v", v.Location)
	}
}

func TestCheck_FR5_DeclaredInterfaceReference(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "github.com/dep1/pkg.PublicFunc", "pkg.go", 9),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []facts.SymbolID{"github.com/dep1/pkg.PublicFunc"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_PluginStructPatternIsSilent(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg", "github.com/dep1/pkg", "pkg.go", 2),
			},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "github.com/dep1/pkg.PublicFunc", "pkg.go", 3),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []facts.SymbolID{"github.com/dep1/pkg.PublicFunc"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(rep.Warnings), rep.Warnings)
	}
}

func TestCheck_MemberOverlapWithComponentDependency(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
				{ImportPath: "mycomponent/pkg/nested"},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"mycomponent/pkg/nested", "otherpkg"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected exactly 1 violation, got %d", len(rep.Violations))
	}
	v := rep.Violations[0]
	if v.Kind != report.MemberOverlap {
		t.Errorf("expected kind %s, got %s", report.MemberOverlap, v.Kind)
	}
	expectedMsg := `member overlap with dependency "dep1": overlapping packages: mycomponent/pkg/nested`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}
}

func TestCheck_FR5_OutOfScope(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "fmt.Println", "pkg.go", 3),            // stdlib
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "mycomponent/pkg.Helper", "pkg.go", 4), // intra-component
			},
		},
		Authority: newStubAuthority().withStdlib("fmt", stubClass{safe: true}).
			withSymbol("fmt.Println", stubClass{safe: true}),
		SDKKey: checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(rep.Warnings), rep.Warnings)
	}
}

func TestCheck_FR5_FR3_FR4_Combined_And_Deterministic(t *testing.T) {
	build := func() checker.Inputs {
		return checker.Inputs{
			Manifest: manifest.Manifest{
				Name:           "mycomponent",
				InterfaceFiles: []string{"pkg1/types.go"},
				ComponentDependencies: []manifest.ComponentDependency{
					{Name: "unused_dep"},
					{Name: "dep_call_only"},
					{Name: "dep_import_only"},
					{Name: "dep_clean"},
				},
			},
			Facts: facts.PackageFacts{
				Packages: []facts.PackageFact{
					{
						ImportPath: "mycomponent/pkg1",
						ExportedSymbols: []facts.ExportedSymbol{
							{
								Name: "mycomponent/pkg1.DB",
								File: "pkg1/types.go",
								Kind: "type",
							},
							{
								Name:     "(*mycomponent/pkg1.DB).Get",
								File:     "pkg2/impl.go",
								Kind:     "method",
								Receiver: "(*mycomponent/pkg1.DB)",
							},
							{
								Name: "init",
								File: "pkg2/impl.go",
								Kind: "init",
							},
						},
					},
				},
				Imports: []facts.ImportEdge{
					impEdge("mycomponent/pkg1", "github.com/dep_import_only/pkg", "pkg1.go", 3),
					impEdge("mycomponent/pkg1", "github.com/dep_clean/pkg", "pkg1.go", 4),
					impEdge("mycomponent/pkg1", "github.com/undeclared_dep/pkg", "pkg1.go", 5),
				},
				References: []facts.ReferenceEdge{
					checkRefEdge(facts.RefFunc, "mycomponent/pkg1", "github.com/dep_clean/pkg.PublicFunc", "pkg1.go", 8),
					checkRefEdge(facts.RefFunc, "mycomponent/pkg1", "github.com/dep_clean/pkg.PrivateFunc", "pkg1.go", 9),
					checkRefEdge(facts.RefFunc, "mycomponent/pkg1", "github.com/dep_call_only/pkg.PrivateFunc", "pkg1.go", 10),
				},
			},
			DepIfaces: []facts.DependencyInterface{
				{
					Component: "unused_dep",
					Packages:  []string{"github.com/unused_dep/pkg"},
				},
				{
					Component: "dep_call_only",
					Packages:  []string{"github.com/dep_call_only/pkg"},
					Symbols:   []facts.SymbolID{"github.com/dep_call_only/pkg.PublicFunc"},
				},
				{
					Component: "dep_import_only",
					Packages:  []string{"github.com/dep_import_only/pkg"},
				},
				{
					Component: "dep_clean",
					Packages:  []string{"github.com/dep_clean/pkg", "mycomponent/pkg1"},
					Symbols:   []facts.SymbolID{"github.com/dep_clean/pkg.PublicFunc"},
				},
			},
			Authority: newStubAuthority(),
			SDKKey:    checkKey,
		}
	}

	// Run multiple times to assert deterministic ordering
	var first report.ConformanceReport
	for i := 0; i < 50; i++ {
		rep := check(t, build())
		if i == 0 {
			first = rep
			continue
		}
		if !reflect.DeepEqual(rep, first) {
			t.Fatalf("run %d nondeterministic: %#v vs %#v", i, rep, first)
		}
	}

	// Assert exactly 5 violations after removing init placement.
	if len(first.Violations) != 5 {
		t.Fatalf("expected exactly 5 violations, got %d: %v", len(first.Violations), first.Violations)
	}

	// Verify violations are in expected alphabetical sorted order:
	// exported method (e), member overlap (m), undeclared dependency import
	// (p "imports"), then the two undeclared-interface references ordered by
	// referent symbol (dep_call_only < dep_clean).
	v0 := first.Violations[0]
	if v0.Kind != report.MethodOutsideInterface {
		t.Errorf("expected MethodOutsideInterface at index 0, got kind %s: %s", v0.Kind, v0.Message)
	}

	v1 := first.Violations[1]
	if v1.Kind != report.MemberOverlap {
		t.Errorf("expected MemberOverlap at index 1, got kind %s: %s", v1.Kind, v1.Message)
	}

	v2 := first.Violations[2]
	if v2.Kind != report.UndeclaredDependency {
		t.Errorf("expected UndeclaredDependency at index 2, got kind %s: %s", v2.Kind, v2.Message)
	}

	v3 := first.Violations[3]
	if v3.Kind != report.CallsUndeclaredInterface || !strings.Contains(v3.Message, "dep_call_only") {
		t.Errorf("expected CallsUndeclaredInterface for dep_call_only at index 3, got kind %s: %s", v3.Kind, v3.Message)
	}

	v4 := first.Violations[4]
	if v4.Kind != report.CallsUndeclaredInterface || !strings.Contains(v4.Message, "dep_clean") {
		t.Errorf("expected CallsUndeclaredInterface for dep_clean at index 4, got kind %s: %s", v4.Kind, v4.Message)
	}

	// Assert exactly 2 warnings
	if len(first.Warnings) != 2 {
		t.Fatalf("expected exactly 2 warnings, got %d: %v", len(first.Warnings), first.Warnings)
	}

	w0 := first.Warnings[0]
	if w0.Kind != report.UnusedDependency || !strings.Contains(w0.Message, "dep_call_only") {
		t.Errorf("expected UnusedDependency warning for dep_call_only at index 0, got kind %s: %s", w0.Kind, w0.Message)
	}

	w1 := first.Warnings[1]
	if w1.Kind != report.UnusedDependency || !strings.Contains(w1.Message, "unused_dep") {
		t.Errorf("expected UnusedDependency warning for unused_dep at index 1, got kind %s: %s", w1.Kind, w1.Message)
	}
}

// authorityInput builds Inputs whose only authority signal is one stdlib
// symbol reference classified by the stub map.
func authorityInput(t *testing.T, id, capability string, policy capanalyzer.CapabilityPolicy) checker.Inputs {
	t.Helper()
	pkg := id[:strings.Index(id, ".")]
	auth := newStubAuthority().withStdlib(pkg, stubClass{safe: true}).
		withSymbol(id, stubClass{caps: []string{capability}})
	return checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg1"}},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg1", id, "pkg1.go", 7),
			},
		},
		Authority: auth,
		SDKKey:    checkKey,
		Policy:    policy,
	}
}

func TestCheck_FR6_UndeclaredAuthority(t *testing.T) {
	in := authorityInput(t, "os.ReadFile", "FILES", capanalyzer.StrictPolicy())

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(rep.Violations))
	}

	v := rep.Violations[0]
	if v.Kind != report.UndeclaredAuthority {
		t.Errorf("expected kind %s, got %s", report.UndeclaredAuthority, v.Kind)
	}

	expectedMsg := `use of undeclared authority "FILES"`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}

	expectedEvidence := []string{"os.ReadFile at stub.go:1"}
	if len(v.Evidence) != len(expectedEvidence) {
		t.Fatalf("expected %d evidence frames, got %d", len(expectedEvidence), len(v.Evidence))
	}
	for i, ev := range v.Evidence {
		if ev != expectedEvidence[i] {
			t.Errorf("evidence frame %d: expected %q, got %q", i, expectedEvidence[i], ev)
		}
	}
}

func TestCheck_FR6_DeclaredAuthority(t *testing.T) {
	in := authorityInput(t, "os.ReadFile", "FILES", capanalyzer.StrictPolicy())
	in.Manifest = manifest.Manifest{
		Name:              "mycomponent",
		DeclaredAuthority: []string{"FILES"},
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR6_WarnSetCapability(t *testing.T) {
	in := authorityInput(t, "net/http.DefaultClient", "NETWORK", capanalyzer.CapabilityPolicy{
		Warn: map[string]bool{"NETWORK": true},
	})

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(rep.Warnings))
	}

	w := rep.Warnings[0]
	if w.Kind != report.AllowedWithWarning {
		t.Errorf("expected kind %s, got %s", report.AllowedWithWarning, w.Kind)
	}

	expectedMsg := `capability "NETWORK" allowed with warning`
	if w.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, w.Message)
	}
}

func TestCheck_FR6_AllowWinsOverWarn(t *testing.T) {
	in := authorityInput(t, "os.ReadFile", "FILES", capanalyzer.CapabilityPolicy{
		Warn: map[string]bool{"FILES": true},
	})
	in.Manifest = manifest.Manifest{
		Name:              "mycomponent",
		DeclaredAuthority: []string{"FILES"},
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(rep.Violations))
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(rep.Warnings))
	}
}

func TestCheck_FR6_ClassPreservedAndBothFailStrict(t *testing.T) {
	// TrueAuthority (FILES from os.ReadFile) and an analysis-defeating
	// bypass construct (assembly) both fail strict policy; under an
	// explicit warn policy each downgrades to its own class-specific kind.
	build := func(policy capanalyzer.CapabilityPolicy) checker.Inputs {
		auth := newStubAuthority().withStdlib("os", stubClass{safe: true}).
			withSymbol("os.ReadFile", stubClass{caps: []string{"FILES"}})
		return checker.Inputs{
			Manifest: manifest.Manifest{Name: "mycomponent"},
			Facts: facts.PackageFacts{
				Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg1"}},
				References: []facts.ReferenceEdge{
					checkRefEdge(facts.RefFunc, "mycomponent/pkg1", "os.ReadFile", "pkg1.go", 7),
				},
				Bypasses: []facts.BypassObservation{
					{Kind: facts.BypassAssembly, Site: site("pkg1.s", 1)},
				},
			},
			Authority: auth,
			SDKKey:    checkKey,
			Policy:    policy,
		}
	}

	rep := check(t, build(capanalyzer.StrictPolicy()))
	if len(rep.Violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(rep.Violations))
	}

	for _, v := range rep.Violations {
		if v.Kind != report.UndeclaredAuthority {
			t.Errorf("expected kind %s, got %s", report.UndeclaredAuthority, v.Kind)
		}
	}

	rep2 := check(t, build(capanalyzer.CapabilityPolicy{
		Warn: map[string]bool{"FILES": true, "": true},
	}))
	if len(rep2.Violations) != 0 {
		t.Errorf("expected 0 violations under warn policy, got %d", len(rep2.Violations))
	}
	if len(rep2.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(rep2.Warnings))
	}

	var gotAllowedWithWarn, gotAnalysisLimitation bool
	for _, w := range rep2.Warnings {
		if w.Kind == report.AllowedWithWarning {
			gotAllowedWithWarn = true
		} else if w.Kind == report.AnalysisLimitation {
			gotAnalysisLimitation = true
		}
	}

	if !gotAllowedWithWarn {
		t.Errorf("expected to get an ALLOWED_WITH_WARNING warning")
	}
	if !gotAnalysisLimitation {
		t.Errorf("expected to get an ANALYSIS_LIMITATION warning")
	}
}

func TestCheck_FR6_PurityNoMutation(t *testing.T) {
	allowedMap := map[string]bool{"NETWORK": true}
	warnMap := map[string]bool{"FILES": true}
	policy := capanalyzer.CapabilityPolicy{
		Allowed: allowedMap,
		Warn:    warnMap,
	}

	declAuthority := []string{"FILES", "CGO"}

	in := authorityInput(t, "os.ReadFile", "FILES", policy)
	in.Manifest = manifest.Manifest{
		Name:              "mycomponent",
		DeclaredAuthority: declAuthority,
	}

	rep1 := check(t, in)
	rep2 := check(t, in)

	if !reflect.DeepEqual(rep1, rep2) {
		t.Errorf("repeated calls to Check with identical inputs yielded different ConformanceReports:\nrep1: %+v\nrep2: %+v", rep1, rep2)
	}

	rendered1 := report.RenderText(rep1)
	rendered2 := report.RenderText(rep2)
	if rendered1 != rendered2 {
		t.Errorf("repeated calls to Check with identical inputs yielded different rendered outputs:\nrendered1: %q\nrendered2: %q", rendered1, rendered2)
	}

	// Verify original policy maps are unmodified
	if len(allowedMap) != 1 || !allowedMap["NETWORK"] || allowedMap["FILES"] || allowedMap["CGO"] {
		t.Errorf("original policy.Allowed map was mutated: %v", allowedMap)
	}
	if len(warnMap) != 1 || !warnMap["FILES"] {
		t.Errorf("original policy.Warn map was mutated: %v", warnMap)
	}
}

func TestCheck_UnresolvedImportsAreDeterministicViolations(t *testing.T) {
	build := func() checker.Inputs {
		return checker.Inputs{
			Manifest: manifest.Manifest{Name: "component"},
			Facts: facts.PackageFacts{
				Packages: []facts.PackageFact{
					{ImportPath: "component/member-b"},
					{ImportPath: "component/member-a"},
				},
				Imports: []facts.ImportEdge{
					impEdge("component/member-b", "example.com/missing", "z.go", 6),
					impEdge("component/member-a", "example.com/missing", "z.go", 5),
					impEdge("component/member-a", "example.com/other", "a.go", 2),
				},
			},
			Authority: newStubAuthority(),
			SDKKey:    checkKey,
		}
	}

	rep1 := check(t, build())
	rep2 := check(t, build())
	if !reflect.DeepEqual(rep1, rep2) {
		t.Fatalf("repeated checks differ: %#v vs %#v", rep1, rep2)
	}
	if len(rep1.Violations) != 3 {
		t.Fatalf("report = %#v, want three site-specific violations", rep1)
	}
	for _, v := range rep1.Violations {
		if v.Kind != report.UndeclaredDependency {
			t.Fatalf("violations = %#v, want UNDECLARED_DEPENDENCY", rep1.Violations)
		}
	}
	wantMsgs := []string{
		`package "component/member-a" imports undeclared dependency "example.com/missing" at z.go:5`,
		`package "component/member-a" imports undeclared dependency "example.com/other" at a.go:2`,
		`package "component/member-b" imports undeclared dependency "example.com/missing" at z.go:6`,
	}
	for i, want := range wantMsgs {
		if rep1.Violations[i].Message != want {
			t.Errorf("violation %d = %q, want %q", i, rep1.Violations[i].Message, want)
		}
	}
	text1 := report.RenderText(rep1)
	text2 := report.RenderText(rep2)
	if text1 != text2 {
		t.Fatalf("repeated text rendering differs: %q vs %q", text1, text2)
	}
	json1, err := json.Marshal(rep1)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	json2, err := json.Marshal(rep2)
	if err != nil {
		t.Fatalf("marshal repeated report: %v", err)
	}
	if !reflect.DeepEqual(json1, json2) {
		t.Fatalf("repeated JSON rendering differs: %s vs %s", json1, json2)
	}
}

func TestCheck_FR6_FeatureCompleteCompositeReport(t *testing.T) {
	// 1. Conforming (empty) report case
	inClean := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}
	repClean := check(t, inClean)
	if len(repClean.Violations) != 0 || len(repClean.Warnings) != 0 {
		t.Errorf("expected clean report, got violations=%d, warnings=%d", len(repClean.Violations), len(repClean.Warnings))
	}
	renderedClean := report.RenderText(repClean)
	expectedClean := "Component \"mycomponent\" conforms; does not exceed declared authority\n"
	if renderedClean != expectedClean {
		t.Errorf("clean report output mismatch.\nexpected:\n%q\ngot:\n%q", expectedClean, renderedClean)
	}

	// 2. Non-conforming report with composite violations: boundary violation (FR5), and authority violation with evidence (FR6)
	inComposite := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
			References: []facts.ReferenceEdge{
				// Undeclared interface reference -> FR5 violation
				checkRefEdge(facts.RefFunc, "mycomponent/pkg1", "github.com/dep1/pkg.PrivateFunc", "pkg1.go", 4),
				// Undeclared stdlib authority -> FR6 violation
				checkRefEdge(facts.RefFunc, "mycomponent/pkg1", "os.ReadFile", "pkg1.go", 7),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []facts.SymbolID{"github.com/dep1/pkg.PublicFunc"},
			},
		},
		Authority: newStubAuthority().withStdlib("os", stubClass{safe: true}).
			withSymbol("os.ReadFile", stubClass{caps: []string{"FILES"}}),
		SDKKey: checkKey,
		Policy: capanalyzer.StrictPolicy(),
	}

	repComp := check(t, inComposite)
	if len(repComp.Violations) != 2 {
		t.Fatalf("expected exactly 2 violations, got %d", len(repComp.Violations))
	}

	v1 := repComp.Violations[0]
	if v1.Kind != report.CallsUndeclaredInterface {
		t.Errorf("expected violation 1 kind %s, got %s", report.CallsUndeclaredInterface, v1.Kind)
	}

	v2 := repComp.Violations[1]
	if v2.Kind != report.UndeclaredAuthority {
		t.Errorf("expected violation 2 kind %s, got %s", report.UndeclaredAuthority, v2.Kind)
	}

	renderedComposite := report.RenderText(repComp)
	expectedComposite := `Component: mycomponent

Dependencies:
- dep1

Violations:
- [CALLS_UNDECLARED_INTERFACE] package "mycomponent/pkg1" references undeclared interface symbol "github.com/dep1/pkg.PrivateFunc" of dependency "dep1" at pkg1.go:4
  at pkg1.go:4
- [UNDECLARED_AUTHORITY] use of undeclared authority "FILES"
  at pkg1.go:7 (1 sites)
  Evidence:
    - os.ReadFile at stub.go:1

Warnings:
- [UNUSED_DEPENDENCY] declared component dependency "dep1" is unused
`
	if renderedComposite != expectedComposite {
		t.Errorf("composite report output mismatch.\nexpected:\n%s\ngot:\n%s", expectedComposite, renderedComposite)
	}
}

func TestCheck_PackageSurface_CallsUndeclaredInterfaceSkipped(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "pkg_surface_dep"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg", "example.com/pkgsurface", "pkg.go", 2),
			},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "example.com/pkgsurface.UndeclaredSymbol", "pkg.go", 3),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component:      "pkg_surface_dep",
				Packages:       []string{"example.com/pkgsurface"},
				InterfaceStyle: manifest.InterfaceStylePackageSurface,
				Symbols:        nil, // Package surface has no explicit interface symbols
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Fatalf("expected 0 violations for package-surface call, got %d: %+v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d: %+v", len(rep.Warnings), rep.Warnings)
	}
}

func TestCheck_DeclaredStyle_CallsUndeclaredInterfaceReported(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "declared_dep"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "example.com/declared.UndeclaredSymbol", "pkg.go", 3),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component:      "declared_dep",
				Packages:       []string{"example.com/declared"},
				InterfaceStyle: manifest.InterfaceStyleUnspecified,
				Symbols:        []facts.SymbolID{"example.com/declared.DeclaredSymbol"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected 1 violation for declared-style call to undeclared symbol, got %d: %+v", len(rep.Violations), rep.Violations)
	}
	if rep.Violations[0].Kind != report.CallsUndeclaredInterface {
		t.Errorf("expected kind %s, got %s", report.CallsUndeclaredInterface, rep.Violations[0].Kind)
	}
}

func TestCheck_PackageSurfaceWrapper_UsedVsUnused(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "used_wrapper"},
				{Name: "unused_wrapper"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg", "example.com/usedwrapper", "pkg.go", 3),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component:      "used_wrapper",
				Packages:       []string{"example.com/usedwrapper"},
				InterfaceStyle: manifest.InterfaceStylePackageSurface,
			},
			{
				Component:      "unused_wrapper",
				Packages:       []string{"example.com/unusedwrapper"},
				InterfaceStyle: manifest.InterfaceStylePackageSurface,
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected exactly 1 warning (for unused_wrapper), got %d: %+v", len(rep.Warnings), rep.Warnings)
	}
	if rep.Warnings[0].Kind != report.UnusedDependency || !strings.Contains(rep.Warnings[0].Message, "unused_wrapper") {
		t.Errorf("expected UnusedDependency for unused_wrapper, got %+v", rep.Warnings[0])
	}
}

func TestCheck_AutoAttachedDependencies_NeverWarnUnused(t *testing.T) {
	// Matrix of 4 combinations: {used, unused} x {package-surface, declared-style}.
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep_used_pkgsurf", AutoAttached: true},
				{Name: "dep_unused_pkgsurf", AutoAttached: true},
				{Name: "dep_used_declared", AutoAttached: true},
				{Name: "dep_unused_declared", AutoAttached: true},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg", "example.com/used_pkgsurf", "pkg.go", 3),
				impEdge("mycomponent/pkg", "example.com/used_declared", "pkg.go", 4),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component:      "dep_used_pkgsurf",
				Packages:       []string{"example.com/used_pkgsurf"},
				InterfaceStyle: manifest.InterfaceStylePackageSurface,
			},
			{
				Component:      "dep_unused_pkgsurf",
				Packages:       []string{"example.com/unused_pkgsurf"},
				InterfaceStyle: manifest.InterfaceStylePackageSurface,
			},
			{
				Component:      "dep_used_declared",
				Packages:       []string{"example.com/used_declared"},
				InterfaceStyle: manifest.InterfaceStyleUnspecified,
			},
			{
				Component:      "dep_unused_declared",
				Packages:       []string{"example.com/unused_declared"},
				InterfaceStyle: manifest.InterfaceStyleUnspecified,
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Warnings) != 0 {
		t.Fatalf("expected 0 warnings (auto-attached deps never warn unused), got %d: %+v", len(rep.Warnings), rep.Warnings)
	}
}

func TestCheck_FR4_EmptyInterfaceFilesVacuous(t *testing.T) {
	// A component with empty interface_files and exported methods in member packages
	// should produce no METHOD_OUTSIDE_INTERFACE violation.
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "mycomponent",
			InterfaceFiles: nil,
			Members:        []string{"mycomponent/pkg"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "Foo", Kind: "type", File: "pkg/foo.go"},
						{Name: "Bar", Kind: "method", Receiver: "(*mycomponent/pkg.Foo)", File: "pkg/foo.go"},
					},
				},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Fatalf("expected 0 violations for empty interface_files, got %d: %+v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_AnalysisDefeatingBypassIsDistinguishableFromAuthorityWarn(t *testing.T) {
	auth := newStubAuthority().withStdlib("os", stubClass{safe: true}).
		withSymbol("os.ReadFile", stubClass{caps: []string{"FILES"}})
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "os.ReadFile", "pkg.go", 4),
			},
			Bypasses: []facts.BypassObservation{
				{Kind: facts.BypassLinkname, Site: site("pkg.go", 9)},
			},
		},
		Authority: auth,
		SDKKey:    checkKey,
		Policy: capanalyzer.CapabilityPolicy{
			Warn: map[string]bool{"FILES": true},
		},
	}

	rep := check(t, in)
	// The bypass construct has no capability, so the FILES-only warn policy
	// still fails it, while the capability downgrades to a warning.
	if len(rep.Violations) != 1 {
		t.Fatalf("expected 1 bypass violation, got %d: %+v", len(rep.Violations), rep.Violations)
	}
	if rep.Violations[0].Class != "AnalysisDefeating" || rep.Violations[0].Kind != report.UndeclaredAuthority {
		t.Errorf("bypass violation = %+v, want an AnalysisDefeating UNDECLARED_AUTHORITY", rep.Violations[0])
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %+v", len(rep.Warnings), rep.Warnings)
	}

	if rep.Warnings[0].Kind != report.AllowedWithWarning {
		t.Errorf("warning kind = %v, want ALLOWED_WITH_WARNING", rep.Warnings[0].Kind)
	}
	if !strings.Contains(rep.Warnings[0].Message, `capability "FILES"`) {
		t.Errorf("warning = %q, want the capability downgrade", rep.Warnings[0].Message)
	}
}

func TestCheck_DependencyOverlapIsToolError(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"example.com/shared"},
			},
			{
				Component: "dep2",
				Packages:  []string{"example.com/shared"},
			},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
	}

	rep, err := checker.Check(in)
	if err == nil {
		t.Fatalf("dependency overlap must fail closed, got report %#v", rep)
	}
	if !strings.Contains(err.Error(), "DEPENDENCY_OVERLAP") {
		t.Errorf("error = %v, want a DEPENDENCY_OVERLAP tool error", err)
	}
}

func TestCheck_NilAuthorityIsToolError(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg", "os", "pkg.go", 3),
			},
		},
		Authority: nil,
		SDKKey:    checkKey,
	}

	if _, err := checker.Check(in); err == nil {
		t.Fatal("a nil authority must fail closed")
	}
}

func TestCheck_InventoryGapIsToolError(t *testing.T) {
	// An import of a package the map does not enumerate reaches the
	// undeclared-dependency rule; an object reference into an enumerated
	// package whose symbol is absent from its total inventory is a tool
	// error (R6), never an undeclared-dependency verdict.
	auth := newStubAuthority().withStdlib("os", stubClass{safe: true})
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "os.Absent", "pkg.go", 3),
			},
		},
		Authority: auth,
		SDKKey:    checkKey,
	}

	rep, err := checker.Check(in)
	if err == nil {
		t.Fatalf("inventory gap must fail closed, got report %#v", rep)
	}
	if !strings.Contains(err.Error(), "os.Absent") {
		t.Errorf("error = %v, want it to name the absent symbol", err)
	}
}
