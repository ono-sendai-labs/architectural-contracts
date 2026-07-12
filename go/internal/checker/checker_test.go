package checker_test

import (
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

func TestCheck_FR4_ExplicitInitOutsideInterface(t *testing.T) {
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
							Name: "mycomponent/pkg.init",
							File: "pkg/impl.go",
							Kind: "init",
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
	if v.Kind != report.InitOutsideInterface {
		t.Errorf("expected kind %s, got %s", report.InitOutsideInterface, v.Kind)
	}
	expectedMsg := `explicit init declared in non-interface file "pkg/impl.go" in package "mycomponent/pkg"`
	if v.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, v.Message)
	}
	if v.Location.File != "pkg/impl.go" {
		t.Errorf("expected location file \"pkg/impl.go\", got %q", v.Location.File)
	}
}

func TestCheck_FR4_ExplicitInitInInterfaceClean(t *testing.T) {
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
							Name: "mycomponent/pkg.init",
							File: "pkg/types.go",
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
