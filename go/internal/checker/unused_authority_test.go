package checker_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func TestCheck_UnusedAuthority_OverDeclaredIsNonFatal(t *testing.T) {
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:              "mycomponent",
			DeclaredAuthority: []string{"FILES"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
		},
		Authority: newStubAuthority(),
		SDKKey:    checkKey,
		Policy:    capanalyzer.StrictPolicy(),
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 {
		t.Fatalf("violations = %#v, want no violations for an unused declaration", rep.Violations)
	}
	want := []report.Finding{{
		Kind:    report.UnusedAuthority,
		Message: `declared authority "FILES" is unused`,
	}}
	if !reflect.DeepEqual(rep.Warnings, want) {
		t.Fatalf("warnings = %#v, want %#v", rep.Warnings, want)
	}
	if got := report.VerdictOf(rep); got != report.VerdictPass {
		t.Fatalf("VerdictOf(unused authority warning) = %q, want %q", got, report.VerdictPass)
	}
}

func TestCheck_UnusedAuthority_ExactSymbolDeclaration(t *testing.T) {
	in := authorityInput(t, "os.ReadFile", "FILES", capanalyzer.StrictPolicy())
	in.Manifest.DeclaredAuthority = []string{"FILES"}

	rep := check(t, in)
	if len(rep.Violations) != 0 || len(rep.Warnings) != 0 {
		t.Fatalf("exactly exercised declaration produced findings: violations=%#v warnings=%#v", rep.Violations, rep.Warnings)
	}
}

func TestCheck_UnusedAuthority_ExactPackageInitDeclaration(t *testing.T) {
	auth := newStubAuthority().withStdlib("example.com/initpkg", stubClass{caps: []string{"FILES"}})
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:              "mycomponent",
			DeclaredAuthority: []string{"FILES"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
			Imports: []facts.ImportEdge{
				impEdge("mycomponent/pkg", "example.com/initpkg", "pkg.go", 3),
			},
		},
		Authority: auth,
		SDKKey:    checkKey,
		Policy:    capanalyzer.StrictPolicy(),
	}

	rep := check(t, in)
	if len(rep.Violations) != 0 || len(rep.Warnings) != 0 {
		t.Fatalf("exactly exercised init declaration produced findings: violations=%#v warnings=%#v", rep.Violations, rep.Warnings)
	}
}

func TestCheck_UnusedAuthority_DiffIsIndependentOfPolicy(t *testing.T) {
	tests := []struct {
		name             string
		policy           capanalyzer.CapabilityPolicy
		wantViolations   []report.Kind
		wantWarningKinds []report.Kind
	}{
		{
			name:             "strict",
			policy:           capanalyzer.StrictPolicy(),
			wantViolations:   []report.Kind{report.UndeclaredAuthority},
			wantWarningKinds: []report.Kind{report.UnusedAuthority},
		},
		{
			name:             "warn",
			policy:           capanalyzer.CapabilityPolicy{Warn: map[string]bool{"NETWORK": true}},
			wantWarningKinds: []report.Kind{report.AllowedWithWarning, report.UnusedAuthority},
		},
		{
			name:             "allow",
			policy:           capanalyzer.CapabilityPolicy{Allowed: map[string]bool{"NETWORK": true}},
			wantWarningKinds: []report.Kind{report.UnusedAuthority},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := newStubAuthority().withStdlib("os", stubClass{safe: true}).
				withSymbol("os.ReadFile", stubClass{caps: []string{"NETWORK"}})
			in := checker.Inputs{
				Manifest: manifest.Manifest{
					Name:              "mycomponent",
					DeclaredAuthority: []string{"FILES"},
				},
				Facts: facts.PackageFacts{
					Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
					References: []facts.ReferenceEdge{
						checkRefEdge(facts.RefFunc, "mycomponent/pkg", "os.ReadFile", "pkg.go", 7),
					},
				},
				Authority: auth,
				SDKKey:    checkKey,
				Policy:    tt.policy,
			}

			rep := check(t, in)
			if got := findingKinds(rep.Violations); !reflect.DeepEqual(got, tt.wantViolations) {
				t.Errorf("violation kinds = %#v, want %#v", got, tt.wantViolations)
			}
			if got := findingKinds(rep.Warnings); !reflect.DeepEqual(got, tt.wantWarningKinds) {
				t.Errorf("warning kinds = %#v, want %#v", got, tt.wantWarningKinds)
			}
			if countKind(rep.Warnings, report.UnusedAuthority) != 1 {
				t.Errorf("unused-authority warnings = %#v, want one regardless of policy", rep.Warnings)
			}
		})
	}
}

func TestCheck_UnusedAuthority_DeterministicCapabilityDiff(t *testing.T) {
	build := func() checker.Inputs {
		auth := newStubAuthority().withStdlib("os", stubClass{safe: true}).
			withSymbol("os.Open", stubClass{caps: []string{"FILES", "NETWORK"}})
		return checker.Inputs{
			Manifest: manifest.Manifest{
				Name:              "mycomponent",
				DeclaredAuthority: []string{"READ_SYSTEM_STATE", "NETWORK", "FILES"},
			},
			Facts: facts.PackageFacts{
				Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
				References: []facts.ReferenceEdge{
					checkRefEdge(facts.RefFunc, "mycomponent/pkg", "os.Open", "z.go", 9),
					checkRefEdge(facts.RefFunc, "mycomponent/pkg", "os.Open", "a.go", 2),
					checkRefEdge(facts.RefFunc, "mycomponent/pkg", "os.Open", "a.go", 2),
				},
			},
			Authority: auth,
			SDKKey:    checkKey,
			Policy:    capanalyzer.StrictPolicy(),
		}
	}

	var first report.ConformanceReport
	var firstJSON []byte
	for i := 0; i < 25; i++ {
		rep := check(t, build())
		if len(rep.Violations) != 0 {
			t.Fatalf("run %d violations = %#v, want declarations to allow exercised capabilities", i, rep.Violations)
		}
		if len(rep.Warnings) != 1 || rep.Warnings[0].Kind != report.UnusedAuthority || rep.Warnings[0].Message != `declared authority "READ_SYSTEM_STATE" is unused` {
			t.Fatalf("run %d warnings = %#v, want one deterministic unused subset", i, rep.Warnings)
		}
		if rep.Warnings[0].Location != (report.Location{}) || rep.Warnings[0].Evidence != nil || rep.Warnings[0].Class != "" || rep.Warnings[0].SDKKey != "" || rep.Warnings[0].Sites != nil {
			t.Fatalf("run %d unused warning carries fabricated source data: %#v", i, rep.Warnings[0])
		}
		data, err := artifactio.MarshalReport(rep)
		if err != nil {
			t.Fatalf("run %d MarshalReport: %v", i, err)
		}
		if i == 0 {
			first = rep
			firstJSON = data
			continue
		}
		if !reflect.DeepEqual(rep, first) {
			t.Fatalf("run %d report differs from first run: %#v vs %#v", i, rep, first)
		}
		if string(data) != string(firstJSON) {
			t.Fatalf("run %d persisted report differs from first run:\n%s\n---\n%s", i, data, firstJSON)
		}
	}
}

func TestCheck_UnusedAuthority_AnalysisDefeatingDoesNotMaskDeclaration(t *testing.T) {
	tests := []struct {
		name           string
		policy         capanalyzer.CapabilityPolicy
		wantViolations []report.Kind
		wantWarnings   []report.Kind
	}{
		{
			name:           "strict",
			policy:         capanalyzer.StrictPolicy(),
			wantViolations: []report.Kind{report.UndeclaredAuthority},
			wantWarnings:   []report.Kind{report.UnusedAuthority},
		},
		{
			name:         "explicitly downgraded",
			policy:       capanalyzer.CapabilityPolicy{Warn: map[string]bool{"": true}},
			wantWarnings: []report.Kind{report.AnalysisLimitation, report.UnusedAuthority},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := newStubAuthority().withStdlib("unanalyzed.example", stubClass{safe: true}).
				withSymbol("unanalyzed.example.Symbol", stubClass{unana: true})
			in := checker.Inputs{
				Manifest: manifest.Manifest{
					Name:              "mycomponent",
					DeclaredAuthority: []string{"FILES"},
				},
				Facts: facts.PackageFacts{
					Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
					References: []facts.ReferenceEdge{
						checkRefEdge(facts.RefFunc, "mycomponent/pkg", "unanalyzed.example.Symbol", "pkg.go", 11),
					},
				},
				Authority: auth,
				SDKKey:    checkKey,
				Policy:    tt.policy,
			}

			rep := check(t, in)
			if got := findingKinds(rep.Violations); !reflect.DeepEqual(got, tt.wantViolations) {
				t.Errorf("violation kinds = %#v, want %#v", got, tt.wantViolations)
			}
			if got := findingKinds(rep.Warnings); !reflect.DeepEqual(got, tt.wantWarnings) {
				t.Errorf("warning kinds = %#v, want %#v", got, tt.wantWarnings)
			}
			if unused := findKind(rep.Warnings, report.UnusedAuthority); unused == nil {
				t.Fatal("missing UNUSED_AUTHORITY warning")
			} else if unused.Location != (report.Location{}) || unused.Evidence != nil || unused.Class != "" || unused.SDKKey != "" || unused.Sites != nil {
				t.Errorf("unused warning carries fabricated data: %#v", *unused)
			}
		})
	}
}

func TestCheck_UnusedAuthority_CoexistsWithOtherWarnings(t *testing.T) {
	auth := newStubAuthority().withStdlib("os", stubClass{safe: true}).
		withSymbol("os.ReadFile", stubClass{caps: []string{"NETWORK"}})
	in := checker.Inputs{
		Manifest: manifest.Manifest{
			Name: "mycomponent",
			ComponentDependencies: []manifest.ComponentDependency{
				{Name: "unused-dep"},
			},
			DeclaredAuthority: []string{"FILES"},
		},
		Facts: facts.PackageFacts{
			Packages: []facts.PackageFact{{ImportPath: "mycomponent/pkg"}},
			References: []facts.ReferenceEdge{
				checkRefEdge(facts.RefFunc, "mycomponent/pkg", "os.ReadFile", "pkg.go", 4),
			},
		},
		DepIfaces: []facts.DependencyInterface{
			{
				Component:  "failed-dep",
				Packages:   []string{"example.com/failed"},
				Provenance: facts.DependencyProvenanceCheckedFail,
				Freshness:  facts.DependencyFreshnessBuildGraph,
				Authority:  manifest.UnknownAuthority(),
			},
			{
				Component:  "stale-dep",
				Packages:   []string{"example.com/stale"},
				Provenance: facts.DependencyProvenanceAsserted,
				Freshness:  facts.DependencyFreshnessStale,
				Authority:  manifest.UnknownAuthority(),
			},
		},
		Authority: auth,
		SDKKey:    checkKey,
		Policy:    capanalyzer.CapabilityPolicy{Warn: map[string]bool{"NETWORK": true}},
	}

	rep := check(t, in)
	wantKinds := []report.Kind{
		report.AllowedWithWarning,
		report.UnusedAuthority,
		report.UnusedDependency,
		report.DependencyCheckFailed,
		report.DependencySurfaceStale,
	}
	if got := findingKinds(rep.Warnings); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("warning kinds = %#v, want %#v", got, wantKinds)
	}
	if got := report.RenderText(rep); !strings.Contains(got, `[UNUSED_AUTHORITY] declared authority "FILES" is unused`) {
		t.Fatalf("rendered report = %q, want UNUSED_AUTHORITY in the standard warning section", got)
	}
}

func findingKinds(findings []report.Finding) []report.Kind {
	if len(findings) == 0 {
		return nil
	}
	kinds := make([]report.Kind, len(findings))
	for i, finding := range findings {
		kinds[i] = finding.Kind
	}
	return kinds
}

func countKind(findings []report.Finding, kind report.Kind) int {
	count := 0
	for _, finding := range findings {
		if finding.Kind == kind {
			count++
		}
	}
	return count
}

func findKind(findings []report.Finding, kind report.Kind) *report.Finding {
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}
