package checker_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func TestCheck_FR3_ConformingImports(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1", Manifest: "dep1/manifest"},
			},
			AbsorbedDependencies: []manifest.AbsorbedDependency{
				{ImportPath: "github.com/foo/bar"},
			},
		},
		Facts: facts.PackageFacts{
			StdlibImports: []string{"fmt"},
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg1",
					Imports:    []string{"github.com/dep1/pkg", "github.com/foo/bar", "fmt"},
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
			},
		},
		Policy: capanalyzer.StrictPolicy(),
	}

	rep := checker.Check(in)
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
			AbsorbedDependencies: []manifest.AbsorbedDependency{
				{ImportPath: "github.com/absorbed/pkg"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg1",
					Imports:    []string{"github.com/a-dep/pkg", "github.com/b-dep/pkg", "github.com/absorbed/pkg"},
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component:              "b-dep",
				OwnCheckRuns:           false,
				CertificationReference: "doc://b-dep-cert",
				Packages:               []string{"github.com/b-dep/pkg"},
			},
			{
				Component:    "a-dep",
				OwnCheckRuns: true,
				Packages:     []string{"github.com/a-dep/pkg"},
			},
		},
	}

	rep := checker.Check(in)

	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(rep.Warnings), rep.Warnings)
	}

	wantDeps := []report.DependencyBoundary{
		{
			Component:    "a-dep",
			OwnCheckRuns: true,
		},
		{
			Component:              "b-dep",
			OwnCheckRuns:           false,
			CertificationReference: "doc://b-dep-cert",
		},
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
					Imports:    []string{"example.com/transitive"},
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "component/member.Run", File: "member.go"},
					},
				},
				{
					ImportPath: "example.com/transitive",
					Imports:    []string{"example.com/only-in-transitive"},
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "example.com/transitive.Helper", File: "transitive.go"},
					},
				},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected one violation from the member package, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if got := rep.Violations[0].Message; got != `package "component/member" imports undeclared dependency "example.com/transitive"` {
		t.Fatalf("unexpected violation: %q", got)
	}
}

func TestCheck_StdlibFactsAreAuthoritativeWhenNil(t *testing.T) {
	for _, stdlibImports := range [][]string{nil, {}} {
		in := checker.Inputs{
			Manifest: manifest.Manifest{Name: "mycomponent"},
			Facts: facts.PackageFacts{
				Packages: []facts.PackageFact{{
					ImportPath: "mycomponent/pkg1",
					Imports:    []string{"fmt"},
				}},
				// Nil and empty slices are empty authoritative sets, not
				// signals to classify imports using a path policy.
				StdlibImports: stdlibImports,
			},
		}

		t.Run(fmt.Sprintf("stdlib-imports-%d", len(stdlibImports)), func(t *testing.T) {
			rep := checker.Check(in)
			if len(rep.Violations) != 1 {
				t.Fatalf("expected one undeclared dependency, got %d: %v", len(rep.Violations), rep.Violations)
			}
			if got := rep.Violations[0].Kind; got != report.UndeclaredDependency {
				t.Fatalf("finding kind = %s, want %s", got, report.UndeclaredDependency)
			}
		})
	}
}

func TestCheck_ExplicitStdlibFactSuppressesDependency(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{
				ImportPath: "mycomponent/pkg1",
				Imports:    []string{"fmt"},
			}},
			StdlibImports: []string{"fmt"},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Fatalf("expected explicit stdlib import to be skipped, got %v", rep.Violations)
	}
}

func TestCheck_EmptyMembersRetainsFR1PackageMembership(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "component"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "component/member",
					Imports:    []string{"component/transitive"},
				},
				{
					ImportPath: "component/transitive",
					Imports:    []string{"example.com/only-in-transitive"},
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "component/transitive.Helper", File: "transitive.go"},
					},
				},
			},
		},
	}

	rep := checker.Check(in)
	want := report.ConformanceReport{
		Component: "component",
		Violations: []report.Finding{{
			Kind:    report.UndeclaredDependency,
			Message: `package "component/transitive" imports undeclared dependency "example.com/only-in-transitive"`,
			Location: report.Location{
				File: "transitive.go",
			},
		}},
	}
	if !reflect.DeepEqual(rep, want) {
		t.Fatalf("FR1 empty-members result changed: got %#v, want %#v", rep, want)
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
					Imports:    []string{"example.com/interface-dependency"},
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "component/interface.API", File: "pkg/api.go", Kind: "type"},
					},
				},
				{
					ImportPath: "component/implementation",
				},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected the implicit interface member to be swept, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if got := rep.Violations[0].Message; got != `package "component/interface" imports undeclared dependency "example.com/interface-dependency"` {
		t.Fatalf("unexpected violation: %q", got)
	}
}

func TestCheck_InterfacePackageSelectionIsIdempotent(t *testing.T) {
	tests := []struct {
		name    string
		members []string
	}{
		{name: "literal", members: []string{"component/interface"}},
		{name: "pattern", members: []string{"component/*"}},
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
						Imports:    []string{"example.com/interface-dependency"},
						ExportedSymbols: []facts.ExportedSymbol{
							{Name: "component/interface.API", File: "pkg/api.go", Kind: "type"},
						},
					}},
				},
			}

			rep := checker.Check(in)
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
	}

	rep := checker.Check(in)
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
					Imports:    []string{"github.com/bad/lib"},
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "mycomponent/pkg1.Func", File: "pkg1.go"},
					},
				},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(rep.Violations))
	}
	v := rep.Violations[0]
	if v.Kind != report.UndeclaredDependency {
		t.Errorf("expected kind %s, got %s", report.UndeclaredDependency, v.Kind)
	}
	expectedMsg := `package "mycomponent/pkg1" imports undeclared dependency "github.com/bad/lib"`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}
	if v.Location.File != "pkg1.go" {
		t.Errorf("expected location file \"pkg1.go\", got %q", v.Location.File)
	}
}

func TestCheck_FR3_StdlibAndIntraComponentAllowed(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			StdlibImports: []string{"os", "net/http"},
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg1",
					Imports:    []string{"mycomponent/pkg2", "os", "net/http"},
				},
				{
					ImportPath: "mycomponent/pkg2",
					Imports:    []string{},
				},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR3_AbsorbedDependencyGlobMatch(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			AbsorbedDependencies: []manifest.AbsorbedDependency{
				{ImportPath: "github.com/foo/*"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg1",
					Imports:    []string{"github.com/foo/bar", "github.com/foo/baz"},
				},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR3_UnusedDependencies(t *testing.T) {
	reason := "needed for testing"
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1", Manifest: "dep1/manifest"},
			},
			AbsorbedDependencies: []manifest.AbsorbedDependency{
				{ImportPath: "github.com/unused/*", Reason: &reason},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg1",
					Imports:    []string{},
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(rep.Warnings))
	}

	expectedWarns := map[string]bool{
		`declared component dependency "dep1" is unused`:               true,
		`declared absorbed dependency "github.com/unused/*" is unused`: true,
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
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkgb",
					Imports:    []string{"github.com/bad2", "github.com/bad1"},
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "mycomponent/pkgb.B", File: "b.go"},
					},
				},
				{
					ImportPath: "mycomponent/pkga",
					Imports:    []string{"github.com/bad3"},
					ExportedSymbols: []facts.ExportedSymbol{
						{Name: "mycomponent/pkga.A", File: "a.go"},
					},
				},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 3 {
		t.Fatalf("expected 3 violations, got %d", len(rep.Violations))
	}

	// Order must be:
	// 1. mycomponent/pkga -> github.com/bad3
	// 2. mycomponent/pkgb -> github.com/bad1
	// 3. mycomponent/pkgb -> github.com/bad2
	v1 := rep.Violations[0]
	if v1.Message != `package "mycomponent/pkga" imports undeclared dependency "github.com/bad3"` || v1.Location.File != "a.go" {
		t.Errorf("violation 1 mismatch: %q at %q", v1.Message, v1.Location.File)
	}

	v2 := rep.Violations[1]
	if v2.Message != `package "mycomponent/pkgb" imports undeclared dependency "github.com/bad1"` || v2.Location.File != "b.go" {
		t.Errorf("violation 2 mismatch: %q at %q", v2.Message, v2.Location.File)
	}

	v3 := rep.Violations[2]
	if v3.Message != `package "mycomponent/pkgb" imports undeclared dependency "github.com/bad2"` || v3.Location.File != "b.go" {
		t.Errorf("violation 3 mismatch: %q at %q", v3.Message, v3.Location.File)
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
	}

	rep := checker.Check(in)
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
	}

	rep := checker.Check(in)
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
	}

	rep := checker.Check(in)
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
	}

	rep := checker.Check(in)
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
	}

	rep := checker.Check(in)
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
					Imports:    []string{"github.com/bad/lib"}, // Undeclared import -> FR3 violation
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
		},
	}

	rep := checker.Check(in)
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
	if v2.Location.File != "pkg/types.go" { // first symbol's file is pkg/types.go
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

func TestStripGenericBrackets(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Foo", "Foo"},
		{"Foo[int]", "Foo"},
		{"Foo[int, string]", "Foo"},
		{"(*example.com/store.DB[int]).Get", "(*example.com/store.DB).Get"},
	}

	for _, tc := range tests {
		got := checker.StripGenericBrackets(tc.input)
		if got != tc.expected {
			t.Errorf("StripGenericBrackets(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestNormalizeInterfaceSymbol(t *testing.T) {
	tests := []struct {
		input    capanalyzer.InterfaceSymbol
		expected string
	}{
		{"example.com/store.Read", "example.com/store.Read"},
		{"(*example.com/store.DB).Get", "(example.com/store.DB).Get"},
		{"(example.com/store.DB).Get", "(example.com/store.DB).Get"},
		{"(*example.com/store.DB[int]).Get", "(example.com/store.DB).Get"},
	}

	for _, tc := range tests {
		got := checker.NormalizeInterfaceSymbol(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeInterfaceSymbol(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestExtractPackagePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com/store.Read", "example.com/store"},
		{"(*example.com/store.DB).Get", "example.com/store"},
		{"(example.com/store.DB).Get", "example.com/store"},
		{"(*example.com/store.DB[int]).Get", "example.com/store"},
		{"fmt.Printf", "fmt"},
	}

	for _, tc := range tests {
		got := checker.ExtractPackagePath(tc.input)
		if got != tc.expected {
			t.Errorf("ExtractPackagePath(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCheck_FR5_UndeclaredInterfaceCall(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					Imports:    []string{"github.com/dep1/pkg"},
				},
			},
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg.Run",
					Callee: "github.com/dep1/pkg.PrivateFunc",
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []capanalyzer.InterfaceSymbol{"github.com/dep1/pkg.PublicFunc"},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected exactly 1 violation, got %d", len(rep.Violations))
	}
	v := rep.Violations[0]
	if v.Kind != report.CallsUndeclaredInterface {
		t.Errorf("expected kind %s, got %s", report.CallsUndeclaredInterface, v.Kind)
	}
	expectedMsg := `call from "mycomponent/pkg.Run" to undeclared interface symbol "github.com/dep1/pkg.PrivateFunc" of dependency "dep1"`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}
}

func TestCheck_FR5_DeclaredInterfaceCall(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					Imports:    []string{"github.com/dep1/pkg"},
				},
			},
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg.Run",
					Callee: "github.com/dep1/pkg.PublicFunc",
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []capanalyzer.InterfaceSymbol{"github.com/dep1/pkg.PublicFunc"},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR5_Normalization(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					Imports:    []string{"github.com/dep1/pkg"},
				},
			},
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg.Run",
					Callee: "(*github.com/dep1/pkg.DB[int]).Get", // opposite receiver form and generic parameter
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []capanalyzer.InterfaceSymbol{"(github.com/dep1/pkg.DB).Get"}, // declared as value receiver
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR5_HigherOrderBoundaryCall(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep1"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					Imports:    []string{"github.com/dep1/pkg"},
				},
			},
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg.Run",
					Callee: "github.com/dep1/pkg.PublicFunc",
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []capanalyzer.InterfaceSymbol{"github.com/dep1/pkg.PublicFunc"},
			},
		},
	}

	rep := checker.Check(in)
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
	}

	rep := checker.Check(in)
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

func TestCheck_MemberOverlapWithAbsorbedExactAndPattern(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:    "component",
			Members: []string{"component/exact", "component/pattern"},
			AbsorbedDependencies: []manifest.AbsorbedDependency{
				{ImportPath: "component/exact"},
				{ImportPath: "component/*"},
			},
		},
		Facts: facts.PackageFacts{Packages: []facts.PackageFact{
			{ImportPath: "component/exact"},
			{ImportPath: "component/pattern"},
		}},
	}

	first := checker.Check(in)
	second := checker.Check(in)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("overlap findings are not deterministic: first=%#v second=%#v", first, second)
	}
	if len(first.Violations) != 2 {
		t.Fatalf("violations = %#v, want two absorbed overlaps", first.Violations)
	}
	for _, want := range []string{
		`member overlap with absorbed dependency "component/*": overlapping packages: component/exact, component/pattern`,
		`member overlap with absorbed dependency "component/exact": overlapping packages: component/exact`,
	} {
		found := false
		for _, violation := range first.Violations {
			if violation.Kind == report.MemberOverlap && violation.Message == want {
				found = true
			}
		}
		if !found {
			t.Errorf("violations = %#v, missing %q", first.Violations, want)
		}
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
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg.Run",
					Callee: "fmt.Println", // stdlib
				},
				{
					Caller: "mycomponent/pkg.Run",
					Callee: "mycomponent/pkg.Helper", // intra-component
				},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(rep.Warnings), rep.Warnings)
	}
}

func TestCheck_FR5_FR3_FR4_Combined_And_Deterministic(t *testing.T) {
	in := checker.Inputs{
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
					Imports: []string{
						"github.com/dep_import_only/pkg",
						"github.com/dep_clean/pkg",
						"github.com/undeclared_dep/pkg",
					},
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
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg1.Run",
					Callee: "github.com/dep_clean/pkg.PublicFunc",
				},
				{
					Caller: "mycomponent/pkg1.Run",
					Callee: "github.com/dep_clean/pkg.PrivateFunc",
				},
				{
					Caller: "mycomponent/pkg1.Run",
					Callee: "github.com/dep_call_only/pkg.PrivateFunc",
				},
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
				Symbols:   []capanalyzer.InterfaceSymbol{"github.com/dep_call_only/pkg.PublicFunc"},
			},
			{
				Component: "dep_import_only",
				Packages:  []string{"github.com/dep_import_only/pkg"},
			},
			{
				Component: "dep_clean",
				Packages:  []string{"github.com/dep_clean/pkg", "mycomponent/pkg1"},
				Symbols:   []capanalyzer.InterfaceSymbol{"github.com/dep_clean/pkg.PublicFunc"},
			},
		},
	}

	// Run multiple times to assert deterministic ordering
	for i := 0; i < 50; i++ {
		rep := checker.Check(in)

		// Assert exactly 5 violations after removing init placement.
		if len(rep.Violations) != 5 {
			t.Fatalf("run %d: expected exactly 5 violations, got %d: %v", i, len(rep.Violations), rep.Violations)
		}

		// Verify violations are in expected alphabetical sorted order
		v0 := rep.Violations[0]
		if v0.Kind != report.CallsUndeclaredInterface || !strings.Contains(v0.Message, "dep_call_only") {
			t.Errorf("expected CallsUndeclaredInterface for dep_call_only at index 0, got kind %s: %s", v0.Kind, v0.Message)
		}

		v1 := rep.Violations[1]
		if v1.Kind != report.CallsUndeclaredInterface || !strings.Contains(v1.Message, "dep_clean") {
			t.Errorf("expected CallsUndeclaredInterface for dep_clean at index 1, got kind %s: %s", v1.Kind, v1.Message)
		}

		v2 := rep.Violations[2]
		if v2.Kind != report.MethodOutsideInterface {
			t.Errorf("expected MethodOutsideInterface at index 2, got kind %s: %s", v2.Kind, v2.Message)
		}

		v3 := rep.Violations[3]
		if v3.Kind != report.MemberOverlap {
			t.Errorf("expected MemberOverlap at index 3, got kind %s: %s", v3.Kind, v3.Message)
		}

		v4 := rep.Violations[4]
		if v4.Kind != report.UndeclaredDependency {
			t.Errorf("expected UndeclaredDependency at index 4, got kind %s: %s", v4.Kind, v4.Message)
		}

		// Assert exactly 2 warnings
		if len(rep.Warnings) != 2 {
			t.Fatalf("run %d: expected exactly 2 warnings, got %d: %v", i, len(rep.Warnings), rep.Warnings)
		}

		w0 := rep.Warnings[0]
		if w0.Kind != report.UnusedDependency || !strings.Contains(w0.Message, "dep_call_only") {
			t.Errorf("expected UnusedDependency warning for dep_call_only at index 0, got kind %s: %s", w0.Kind, w0.Message)
		}

		w1 := rep.Warnings[1]
		if w1.Kind != report.UnusedDependency || !strings.Contains(w1.Message, "unused_dep") {
			t.Errorf("expected UnusedDependency warning for unused_dep at index 1, got kind %s: %s", w1.Kind, w1.Message)
		}
	}
}

func TestCheck_FR6_UndeclaredAuthority(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
		},
		Caps: []capanalyzer.CapabilityFinding{
			{
				Package:    "mycomponent/pkg1",
				Capability: "FILES",
				Class:      capanalyzer.TrueAuthority,
				CallPath: []capanalyzer.Frame{
					{Func: "main.main", File: "main.go", Line: 10},
					{Func: "os.Open", File: "os.go", Line: 20},
				},
			},
		},
		Policy: capanalyzer.StrictPolicy(),
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(rep.Violations))
	}

	v := rep.Violations[0]
	if v.Kind != report.UndeclaredAuthority {
		t.Errorf("expected kind %s, got %s", report.UndeclaredAuthority, v.Kind)
	}

	expectedMsg := `use of undeclared authority "FILES" in package "mycomponent/pkg1"`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}

	expectedEvidence := []string{
		"main.main at main.go:10",
		"os.Open at os.go:20",
	}
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
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:              "mycomponent",
			DeclaredAuthority: []string{"FILES"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
		},
		Caps: []capanalyzer.CapabilityFinding{
			{
				Package:    "mycomponent/pkg1",
				Capability: "FILES",
				Class:      capanalyzer.TrueAuthority,
			},
		},
		Policy: capanalyzer.StrictPolicy(),
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_FR6_WarnSetCapability(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
		},
		Caps: []capanalyzer.CapabilityFinding{
			{
				Package:    "mycomponent/pkg1",
				Capability: "NETWORK",
				Class:      capanalyzer.TrueAuthority,
			},
		},
		Policy: capanalyzer.CapabilityPolicy{
			Warn: map[string]bool{"NETWORK": true},
		},
	}

	rep := checker.Check(in)
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

	expectedMsg := `capability "NETWORK" in package "mycomponent/pkg1" allowed with warning`
	if w.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, w.Message)
	}
}

func TestCheck_FR6_AllowWinsOverWarn(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:              "mycomponent",
			DeclaredAuthority: []string{"FILES"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
		},
		Caps: []capanalyzer.CapabilityFinding{
			{
				Package:    "mycomponent/pkg1",
				Capability: "FILES",
				Class:      capanalyzer.TrueAuthority,
			},
		},
		Policy: capanalyzer.CapabilityPolicy{
			Warn: map[string]bool{"FILES": true},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(rep.Violations))
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(rep.Warnings))
	}
}

func TestCheck_FR6_ClassPreservedAndBothFailStrict(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
		},
		Caps: []capanalyzer.CapabilityFinding{
			{
				Package:    "mycomponent/pkg1",
				Capability: "FILES",
				Class:      capanalyzer.TrueAuthority,
			},
			{
				Package:    "mycomponent/pkg1",
				Capability: "REFLECT",
				Class:      capanalyzer.AnalysisDefeating,
			},
		},
		Policy: capanalyzer.StrictPolicy(),
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(rep.Violations))
	}

	for _, v := range rep.Violations {
		if v.Kind != report.UndeclaredAuthority {
			t.Errorf("expected kind %s, got %s", report.UndeclaredAuthority, v.Kind)
		}
	}

	// Under warn policy, check that class is preserved (TrueAuthority -> ALLOWED_WITH_WARNING, AnalysisDefeating -> ANALYSIS_LIMITATION)
	in.Policy = capanalyzer.CapabilityPolicy{
		Warn: map[string]bool{"FILES": true, "REFLECT": true},
	}
	rep2 := checker.Check(in)
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

	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:              "mycomponent",
			DeclaredAuthority: declAuthority,
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg1"},
			},
		},
		Caps: []capanalyzer.CapabilityFinding{
			{
				Package:    "mycomponent/pkg1",
				Capability: "CGO",
				Class:      capanalyzer.TrueAuthority,
			},
		},
		Policy: policy,
	}

	rep1 := checker.Check(in)
	rep2 := checker.Check(in)

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

func TestCheck_UnresolvedImportsAreDeterministicLimitations(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "component"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "component/member-b"},
				{ImportPath: "component/member-a"},
			},
			UnresolvedImports: []facts.UnresolvedImport{
				{Package: "component/member-b", File: "z.go", ImportPath: "example.com/missing"},
				{Package: "component/member-a", File: "z.go", ImportPath: "example.com/missing"},
				{Package: "component/member-a", File: "a.go", ImportPath: "example.com/other"},
				{Package: "component/member-a", File: "a.go", ImportPath: "example.com/other"},
			},
		},
	}

	rep1 := checker.Check(in)
	rep2 := checker.Check(in)
	if !reflect.DeepEqual(rep1, rep2) {
		t.Fatalf("repeated checks differ: %#v vs %#v", rep1, rep2)
	}
	if len(rep1.Violations) != 0 || len(rep1.Warnings) != 4 {
		t.Fatalf("report = %#v, want four deduplicated warnings and no violations", rep1)
	}
	if got := rep1.Warnings[0].Message; !strings.Contains(got, `z.go`) || !strings.Contains(got, `example.com/missing`) {
		t.Fatalf("first warning = %q, want deterministic z.go/missing diagnostic", got)
	}
	for _, warning := range rep1.Warnings {
		if warning.Kind != report.AnalysisLimitation {
			t.Fatalf("warnings = %#v, want ANALYSIS_LIMITATION", rep1.Warnings)
		}
	}
	if rep1.Warnings[0].Message == rep1.Warnings[1].Message {
		t.Fatalf("warnings = %#v, want distinct file/import observations", rep1.Warnings)
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
	if rep1.Warnings[0].Kind != report.AnalysisLimitation {
		t.Fatalf("warnings = %#v, want ANALYSIS_LIMITATION", rep1.Warnings)
	}
	if !strings.Contains(text1, "ANALYSIS_LIMITATION") || !strings.Contains(text1, "z.go") {
		t.Fatalf("rendered report = %q, want limitation diagnostics from both packages", text1)
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
	}
	repClean := checker.Check(inClean)
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
				{
					ImportPath: "mycomponent/pkg1",
					Imports:    []string{"github.com/dep1/pkg"},
				},
			},
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg1.Run",
					Callee: "github.com/dep1/pkg.PrivateFunc", // Undeclared interface call -> FR5 violation
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component: "dep1",
				Packages:  []string{"github.com/dep1/pkg"},
				Symbols:   []capanalyzer.InterfaceSymbol{"github.com/dep1/pkg.PublicFunc"},
			},
		},
		Caps: []capanalyzer.CapabilityFinding{
			{
				Package:    "mycomponent/pkg1",
				Capability: "FILES",
				Class:      capanalyzer.TrueAuthority,
				CallPath: []capanalyzer.Frame{
					{Func: "main.main", File: "main.go", Line: 10},
					{Func: "os.Open", File: "os.go", Line: 20},
				},
			},
		},
		Policy: capanalyzer.StrictPolicy(),
	}

	repComp := checker.Check(inComposite)
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
- dep1: asserted

Violations:
- [CALLS_UNDECLARED_INTERFACE] call from "mycomponent/pkg1.Run" to undeclared interface symbol "github.com/dep1/pkg.PrivateFunc" of dependency "dep1"
- [UNDECLARED_AUTHORITY] use of undeclared authority "FILES" in package "mycomponent/pkg1"
  Evidence:
    - main.main at main.go:10
    - os.Open at os.go:20
`
	if renderedComposite != expectedComposite {
		t.Errorf("composite report output mismatch.\nexpected:\n%s\ngot:\n%s", expectedComposite, renderedComposite)
	}
}

func TestCheck_AbsorbedFuncValueEscape_OneEscape(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/member"},
			},
			FuncValueEscapes: []facts.FuncValueEscape{
				{
					Symbol:  "example.com/absorbed.Load",
					Package: "mycomponent/member",
					File:    "member/member.go",
					Line:    15,
				},
			},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %+v", len(rep.Warnings), rep.Warnings)
	}

	w := rep.Warnings[0]
	if w.Kind != report.AbsorbedFuncValueEscape {
		t.Errorf("expected warning kind %s, got %s", report.AbsorbedFuncValueEscape, w.Kind)
	}
	if w.Location.File != "member/member.go" || w.Location.Line != 15 {
		t.Errorf("expected location member/member.go:15, got %+v", w.Location)
	}
	if !strings.Contains(w.Message, "mycomponent/member") || !strings.Contains(w.Message, "example.com/absorbed.Load") {
		t.Errorf("expected message to name member package and absorbed function symbol, got %q", w.Message)
	}
	if !strings.Contains(w.Message, "without calling it") {
		t.Errorf("expected message to explain no-direct-call ('without calling it'), got %q", w.Message)
	}
	if !strings.Contains(w.Message, "body is unanalyzed") {
		t.Errorf("expected message to explain unanalyzed body ('body is unanalyzed'), got %q", w.Message)
	}
}

func TestCheck_AbsorbedFuncValueEscape_EmptyOrNil(t *testing.T) {
	inNil := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages:         []facts.PackageFact{{ImportPath: "mycomponent/member"}},
			FuncValueEscapes: nil,
		},
	}
	repNil := checker.Check(inNil)
	for _, w := range repNil.Warnings {
		if w.Kind == report.AbsorbedFuncValueEscape {
			t.Errorf("unexpected ABSORBED_FUNC_VALUE_ESCAPE warning for nil escapes: %+v", w)
		}
	}

	inEmpty := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages:         []facts.PackageFact{{ImportPath: "mycomponent/member"}},
			FuncValueEscapes: []facts.FuncValueEscape{},
		},
	}
	repEmpty := checker.Check(inEmpty)
	for _, w := range repEmpty.Warnings {
		if w.Kind == report.AbsorbedFuncValueEscape {
			t.Errorf("unexpected ABSORBED_FUNC_VALUE_ESCAPE warning for empty escapes: %+v", w)
		}
	}
}

func TestCheck_AbsorbedFuncValueEscape_MultipleEscapesDeterministic(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{Name: "mycomponent"},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/member-a"},
				{ImportPath: "mycomponent/member-b"},
			},
			FuncValueEscapes: []facts.FuncValueEscape{
				{Symbol: "example.com/absorbed.Z", Package: "mycomponent/member-b", File: "b.go", Line: 20},
				{Symbol: "example.com/absorbed.A", Package: "mycomponent/member-a", File: "a.go", Line: 10},
				{Symbol: "example.com/absorbed.M", Package: "mycomponent/member-a", File: "a.go", Line: 5},
			},
		},
	}

	rep1 := checker.Check(in)
	rep2 := checker.Check(in)

	if !reflect.DeepEqual(rep1, rep2) {
		t.Fatalf("repeated Check calls differ: %#v vs %#v", rep1, rep2)
	}
	if len(rep1.Warnings) != 3 {
		t.Fatalf("expected 3 warnings, got %d", len(rep1.Warnings))
	}

	// Verify warnings are sorted deterministically
	text1 := report.RenderText(rep1)
	text2 := report.RenderText(rep2)
	if text1 != text2 {
		t.Fatalf("rendered text outputs differ: %q vs %q", text1, text2)
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
				{
					ImportPath: "mycomponent/pkg",
					Imports:    []string{"example.com/pkgsurface"},
				},
			},
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg.DoStuff",
					Callee: "example.com/pkgsurface.UndeclaredSymbol",
				},
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
	}

	rep := checker.Check(in)
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
				{
					ImportPath: "mycomponent/pkg",
					Imports:    []string{"example.com/declared"},
				},
			},
			CallEdges: []facts.CallEdge{
				{
					Caller: "mycomponent/pkg.DoStuff",
					Callee: "example.com/declared.UndeclaredSymbol",
				},
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component:      "declared_dep",
				Packages:       []string{"example.com/declared"},
				InterfaceStyle: manifest.InterfaceStyleUnspecified,
				Symbols:        []capanalyzer.InterfaceSymbol{"example.com/declared.DeclaredSymbol"},
			},
		},
	}

	rep := checker.Check(in)
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
				{
					ImportPath: "mycomponent/pkg",
					Imports:    []string{"example.com/usedwrapper"},
				},
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
	}

	rep := checker.Check(in)
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected exactly 1 warning (for unused_wrapper), got %d: %+v", len(rep.Warnings), rep.Warnings)
	}
	if rep.Warnings[0].Kind != report.UnusedDependency || !strings.Contains(rep.Warnings[0].Message, "unused_wrapper") {
		t.Errorf("expected UnusedDependency for unused_wrapper, got %+v", rep.Warnings[0])
	}
}

func TestCheck_AutoAttachedDependencies_NeverWarnUnused(t *testing.T) {
	// Matrix of 4 combinations: {used, unused} x {package-surface, declared-style}
	// plus an unused absorbed dependency to verify it still warns.
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "dep_used_pkgsurf", AutoAttached: true},
				{Name: "dep_unused_pkgsurf", AutoAttached: true},
				{Name: "dep_used_declared", AutoAttached: true},
				{Name: "dep_unused_declared", AutoAttached: true},
			},
			AbsorbedDependencies: []manifest.AbsorbedDependency{
				{ImportPath: "example.com/unused_absorbed"},
			},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{
					ImportPath: "mycomponent/pkg",
					Imports:    []string{"example.com/used_pkgsurf", "example.com/used_declared"},
				},
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
	}

	rep := checker.Check(in)
	// Only example.com/unused_absorbed should produce a warning
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected 1 warning (for unused absorbed dep), got %d: %+v", len(rep.Warnings), rep.Warnings)
	}
	if rep.Warnings[0].Kind != report.UnusedDependency || !strings.Contains(rep.Warnings[0].Message, "example.com/unused_absorbed") {
		t.Errorf("expected warning for example.com/unused_absorbed, got %+v", rep.Warnings[0])
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
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Fatalf("expected 0 violations for empty interface_files, got %d: %+v", len(rep.Violations), rep.Violations)
	}
}

func TestCheck_BodilessAbsorbedPackagesAreAnalysisLimitations(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			BodilessAbsorbedPackages: []string{"example.com/bodiless_dep"},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %+v", len(rep.Warnings), rep.Warnings)
	}
	w := rep.Warnings[0]
	if w.Kind != report.AnalysisLimitation {
		t.Errorf("warning kind = %v, want ANALYSIS_LIMITATION", w.Kind)
	}
	if !strings.Contains(w.Message, "example.com/bodiless_dep") || !strings.Contains(w.Message, "no source bodies") {
		t.Errorf("warning message = %q, want naming package and no source bodies", w.Message)
	}
}

func TestCheck_ThreeAnalysisLimitationsAreDistinguishable(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{
				{ImportPath: "mycomponent/pkg"},
			},
			UnresolvedImports: []facts.UnresolvedImport{
				{Package: "mycomponent/pkg", File: "foo.go", ImportPath: "example.com/missing"},
			},
			BodilessAbsorbedPackages: []string{"example.com/bodiless"},
		},
		Caps: []capanalyzer.CapabilityFinding{
			{
				Capability: "FILES",
				Package:    "mycomponent/pkg",
				Class:      capanalyzer.AnalysisDefeating,
			},
		},
		Policy: capanalyzer.CapabilityPolicy{
			Warn: map[string]bool{"FILES": true},
		},
	}

	rep := checker.Check(in)
	if len(rep.Violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(rep.Violations), rep.Violations)
	}
	if len(rep.Warnings) != 3 {
		t.Fatalf("expected 3 warnings, got %d: %+v", len(rep.Warnings), rep.Warnings)
	}

	for _, w := range rep.Warnings {
		if w.Kind != report.AnalysisLimitation {
			t.Errorf("warning kind = %v, want ANALYSIS_LIMITATION", w.Kind)
		}
	}

	// Verify messages are distinct and individually identifiable
	msgs := make(map[string]bool)
	for _, w := range rep.Warnings {
		msgs[w.Message] = true
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 distinct warning messages, got %d", len(msgs))
	}

	rendered := report.RenderText(rep)
	if !strings.Contains(rendered, "unresolved import") {
		t.Errorf("rendered report missing unresolved import limitation")
	}
	if !strings.Contains(rendered, "absorbed package") {
		t.Errorf("rendered report missing absorbed package limitation")
	}
	if !strings.Contains(rendered, "is an analysis limitation") {
		t.Errorf("rendered report missing capability analysis limitation")
	}
}
