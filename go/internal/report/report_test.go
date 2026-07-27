package report_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func TestInterfaceFileExcluded_SerializesStableKind(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "component",
		Warnings: []report.Finding{{
			Kind:     report.InterfaceFileExcluded,
			Message:  `interface file "api_windows.go" excluded by filename suffix "_windows.go"`,
			Location: report.Location{File: "api_windows.go", Line: 1},
		}},
	}

	rendered := report.RenderText(rep)
	if !strings.Contains(rendered, "[INTERFACE_FILE_EXCLUDED]") {
		t.Fatalf("RenderText() = %q, want stable warning kind", rendered)
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"kind":"INTERFACE_FILE_EXCLUDED"`) {
		t.Fatalf("JSON = %s, want stable warning kind", data)
	}
}

func TestAbsorbedFuncValueEscape_SerializesStableKind(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "component",
		Warnings: []report.Finding{{
			Kind:     report.AbsorbedFuncValueEscape,
			Message:  `member package "example.com/app/member" takes function value "example.com/app/absorbed.Load" from absorbed package without calling it; body is unanalyzed`,
			Location: report.Location{File: "member/member.go", Line: 15},
		}},
	}

	rendered := report.RenderText(rep)
	if !strings.Contains(rendered, "[ABSORBED_FUNC_VALUE_ESCAPE]") {
		t.Fatalf("RenderText() = %q, want stable warning kind", rendered)
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"kind":"ABSORBED_FUNC_VALUE_ESCAPE"`) {
		t.Fatalf("JSON = %s, want stable warning kind", data)
	}
}

func TestRenderText_InterfaceFileExcludedWarningUsesDetailedFormat(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "excluded-interface",
		Warnings: []report.Finding{{
			Kind:     report.InterfaceFileExcluded,
			Message:  `interface file "api_windows.go" excluded by filename suffix "_windows.go"`,
			Location: report.Location{File: "api_windows.go", Line: 1},
		}},
	}

	got := report.RenderText(rep)
	want := `Component: excluded-interface

Warnings:
- [INTERFACE_FILE_EXCLUDED] interface file "api_windows.go" excluded by filename suffix "_windows.go"
  at api_windows.go:1
`

	if got != want {
		t.Errorf("RenderText(excluded interface warning) =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderText_Conforms(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "test-comp",
	}

	got := report.RenderText(rep)
	want := `Component "test-comp" conforms; does not exceed declared authority
`

	if got != want {
		t.Errorf("RenderText(conforming) =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderText_ConformsWithDependencies(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "test-comp",
		Dependencies: []report.DependencyBoundary{
			{
				Component:    "dep-certified",
				OwnCheckRuns: true,
			},
			{
				Component:              "dep-asserted-ref",
				OwnCheckRuns:           false,
				CertificationReference: "build://ref-123",
			},
			{
				Component:              "dep-asserted-noref",
				OwnCheckRuns:           false,
				CertificationReference: "",
			},
		},
	}

	got := report.RenderText(rep)
	want := `Component "test-comp" conforms; does not exceed declared authority

Dependencies:
- dep-certified: certified
- dep-asserted-ref: asserted (build://ref-123)
- dep-asserted-noref: asserted
`

	if got != want {
		t.Errorf("RenderText(conforms with deps) =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderText_WithFindingsAndDependencies(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "test-comp",
		Dependencies: []report.DependencyBoundary{
			{
				Component:    "dep-certified",
				OwnCheckRuns: true,
			},
		},
		Violations: []report.Finding{
			{
				Kind:    report.UndeclaredDependency,
				Message: `imported package "os" is not declared in the manifest`,
				Location: report.Location{
					File: "main.go",
					Line: 12,
				},
			},
		},
	}

	got := report.RenderText(rep)
	want := `Component: test-comp

Dependencies:
- dep-certified: certified

Violations:
- [UNDECLARED_DEPENDENCY] imported package "os" is not declared in the manifest
  at main.go:12
`

	if got != want {
		t.Errorf("RenderText(findings with deps) =\n%q\nwant:\n%q", got, want)
	}
}

func TestJSON_DependenciesSerialization(t *testing.T) {
	t.Run("with dependencies", func(t *testing.T) {
		rep := report.ConformanceReport{
			Component: "test-comp",
			Dependencies: []report.DependencyBoundary{
				{
					Component:    "dep-certified",
					OwnCheckRuns: true,
				},
				{
					Component:              "dep-asserted-ref",
					OwnCheckRuns:           false,
					CertificationReference: "ref-456",
				},
			},
		}

		data, err := json.Marshal(rep)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		got := string(data)
		wantSubstrings := []string{
			`"dependencies":[{"component":"dep-certified","own_check_runs":true}`,
			`{"component":"dep-asserted-ref","own_check_runs":false,"certification_reference":"ref-456"}]`,
		}
		for _, want := range wantSubstrings {
			if !strings.Contains(got, want) {
				t.Errorf("JSON = %s, want substring %s", got, want)
			}
		}
	})

	t.Run("without dependencies omits dependencies field", func(t *testing.T) {
		rep := report.ConformanceReport{
			Component: "test-comp",
		}

		data, err := json.Marshal(rep)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		if strings.Contains(string(data), `"dependencies"`) {
			t.Errorf("JSON = %s, should omit dependencies field when empty", string(data))
		}
	})
}

func TestRenderText_WithFindings(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "test-comp",
		Violations: []report.Finding{
			{
				Kind:    report.UndeclaredDependency,
				Message: `imported package "os" is not declared in the manifest`,
				Location: report.Location{
					File: "main.go",
					Line: 12,
				},
			},
		},
		Warnings: []report.Finding{
			{
				Kind:    report.UnusedDependency,
				Message: `declared dependency "github.com/foo/bar" is not used`,
			},
		},
	}

	got := report.RenderText(rep)
	want := `Component: test-comp

Violations:
- [UNDECLARED_DEPENDENCY] imported package "os" is not declared in the manifest
  at main.go:12

Warnings:
- [UNUSED_DEPENDENCY] declared dependency "github.com/foo/bar" is not used
`

	if got != want {
		t.Errorf("RenderText(findings) =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderText_Evidence(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "test-comp",
		Violations: []report.Finding{
			{
				Kind:    report.UndeclaredAuthority,
				Message: `component uses undeclared authority "FILES"`,
				Location: report.Location{
					File: "main.go",
					Line: 15,
				},
				Evidence: []string{
					"main.go:15: main",
					"os.go:42: os.Open",
				},
			},
		},
	}

	got := report.RenderText(rep)
	want := `Component: test-comp

Violations:
- [UNDECLARED_AUTHORITY] component uses undeclared authority "FILES"
  at main.go:15
  Evidence:
    - main.go:15: main
    - os.go:42: os.Open
`

	if got != want {
		t.Errorf("RenderText(evidence) =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderText_Determinism(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "test-comp",
		Violations: []report.Finding{
			{
				Kind:    report.UndeclaredDependency,
				Message: `imported package "os" is not declared`,
				Location: report.Location{
					File: "main.go",
					Line: 12,
				},
			},
			{
				Kind:    report.CallsUndeclaredInterface,
				Message: `calls undeclared interface`,
				Location: report.Location{
					File: "app.go",
					Line: 24,
				},
			},
		},
		Warnings: []report.Finding{
			{
				Kind:    report.UnusedDependency,
				Message: `unused dependency`,
			},
		},
	}

	first := report.RenderText(rep)
	for i := 0; i < 50; i++ {
		subsequent := report.RenderText(rep)
		if first != subsequent {
			t.Fatalf("RenderText is non-deterministic at iteration %d", i)
		}
	}
}
