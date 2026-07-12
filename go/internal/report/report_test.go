package report_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func TestRenderText_Conforms(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "test-comp",
	}

	got := report.RenderText(rep)
	want := `Component "test-comp" conforms / ambient-authority-free
`

	if got != want {
		t.Errorf("RenderText(conforming) =\n%q\nwant:\n%q", got, want)
	}
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
