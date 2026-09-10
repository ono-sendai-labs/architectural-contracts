package report_test

import (
	"encoding/json"
	"reflect"
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
			{Component: "dep-a"},
			{Component: "dep-b"},
			{Component: "dep-c"},
		},
	}

	got := report.RenderText(rep)
	want := `Component "test-comp" conforms; does not exceed declared authority

Dependencies:
- dep-a
- dep-b
- dep-c
`

	if got != want {
		t.Errorf("RenderText(conforms with deps) =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderText_WithFindingsAndDependencies(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "test-comp",
		Dependencies: []report.DependencyBoundary{
			{Component: "dep-certified"},
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
- dep-certified

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
				{Component: "dep-certified"},
				{Component: "dep-asserted-ref"},
			},
		}

		data, err := json.Marshal(rep)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		got := string(data)
		wantSubstrings := []string{
			`"dependencies":[{"component":"dep-certified"},{"component":"dep-asserted-ref"}]`,
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

func TestDependencyBoundary_StatusAxesRenderOrthogonally(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "consumer",
		Dependencies: []report.DependencyBoundary{
			{
				Component:         "checked",
				Provenance:        report.DependencyProvenanceCheckedPass,
				Freshness:         report.DependencyFreshnessBuildGraph,
				Authority:         report.DependencyAuthorityDeclared,
				DeclaredAuthority: []string{"FILES"},
			},
			{
				Component:  "failed",
				Provenance: report.DependencyProvenanceCheckedFail,
				Freshness:  report.DependencyFreshnessBuildGraph,
				Authority:  report.DependencyAuthorityDeclared,
			},
			{
				Component:         "checked-stale",
				Provenance:        report.DependencyProvenanceCheckedPass,
				Freshness:         report.DependencyFreshnessStale,
				Authority:         report.DependencyAuthorityDeclared,
				DeclaredAuthority: []string{"FILES"},
			},
			{
				Component:  "failed-stale",
				Provenance: report.DependencyProvenanceCheckedFail,
				Freshness:  report.DependencyFreshnessStale,
				Authority:  report.DependencyAuthorityDeclared,
			},
			{
				Component:  "unknown",
				Provenance: report.DependencyProvenanceAsserted,
				Freshness:  report.DependencyFreshnessStale,
				Authority:  report.DependencyAuthorityUnknown,
			},
		},
	}

	textOutput := report.RenderText(rep)
	lineFor := func(component string) string {
		for _, line := range strings.Split(textOutput, "\n") {
			if strings.HasPrefix(line, "- "+component) {
				return line
			}
		}
		return ""
	}
	for _, tc := range []struct {
		component string
		contains  []string
		absent    []string
	}{
		{component: "checked", contains: []string{"certified"}},
		{component: "failed", contains: []string{"check failed"}, absent: []string{"certified"}},
		{component: "checked-stale", contains: []string{"stale"}, absent: []string{"certified"}},
		{component: "failed-stale", contains: []string{"check failed", "stale"}, absent: []string{"certified"}},
		{component: "unknown", contains: []string{"asserted", "stale", "untrusted"}, absent: []string{"certified"}},
	} {
		line := lineFor(tc.component)
		if line == "" {
			t.Errorf("text = %q, missing boundary %q", textOutput, tc.component)
			continue
		}
		for _, word := range tc.contains {
			if !strings.Contains(line, word) {
				t.Errorf("boundary line = %q, want status word %q", line, word)
			}
		}
		for _, word := range tc.absent {
			if strings.Contains(line, word) {
				t.Errorf("boundary line = %q, must not contain status word %q", line, word)
			}
		}
	}

	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded struct {
		Dependencies []struct {
			Component         string   `json:"component"`
			Provenance        string   `json:"provenance"`
			Freshness         string   `json:"freshness"`
			Authority         string   `json:"authority"`
			DeclaredAuthority []string `json:"declared_authority"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(decoded.Dependencies) != 5 {
		t.Fatalf("decoded dependencies = %#v, want 5 entries", decoded.Dependencies)
	}
	byComponent := make(map[string]struct {
		Provenance        string
		Freshness         string
		Authority         string
		DeclaredAuthority []string
	}, len(decoded.Dependencies))
	for _, dependency := range decoded.Dependencies {
		byComponent[dependency.Component] = struct {
			Provenance        string
			Freshness         string
			Authority         string
			DeclaredAuthority []string
		}{dependency.Provenance, dependency.Freshness, dependency.Authority, dependency.DeclaredAuthority}
	}
	if got := byComponent["checked"]; got.Provenance != "CHECKED_PASS" || got.Freshness != "BUILD_GRAPH" || got.Authority != "DECLARED" || !reflect.DeepEqual(got.DeclaredAuthority, []string{"FILES"}) {
		t.Errorf("checked boundary axes = %#v", got)
	}
	if got := byComponent["checked-stale"]; got.Provenance != "CHECKED_PASS" || got.Freshness != "STALE" || got.Authority != "DECLARED" || !reflect.DeepEqual(got.DeclaredAuthority, []string{"FILES"}) {
		t.Errorf("checked-stale boundary axes = %#v", got)
	}
	if got := byComponent["failed"]; got.Provenance != "CHECKED_FAIL" || got.Freshness != "BUILD_GRAPH" || got.Authority != "DECLARED" {
		t.Errorf("failed boundary axes = %#v", got)
	}
	if got := byComponent["failed-stale"]; got.Provenance != "CHECKED_FAIL" || got.Freshness != "STALE" || got.Authority != "DECLARED" {
		t.Errorf("failed-stale boundary axes = %#v", got)
	}
	if got := byComponent["unknown"]; got.Provenance != "ASSERTED" || got.Freshness != "STALE" || got.Authority != "UNKNOWN" {
		t.Errorf("unknown boundary axes = %#v", got)
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

func TestRenderText_AggregatedAuthorityFindingPrintsFirstSiteAndCount(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "m",
		Violations: []report.Finding{{
			Kind:    report.UndeclaredAuthority,
			Message: "use of undeclared authority \"FILES\"",
			Class:   "TrueAuthority",
			SDKKey:  "go1.26.4 linux amd64 map1",
			Sites: []report.AuthoritySite{
				{File: "member/a.go", Line: 3, Symbol: "os.ReadFile"},
				{File: "member/b.go", Line: 7, Symbol: "os.Stdin"},
				{File: "member/c.go", Line: 11, Symbol: "os.Create"},
			},
		}},
	}
	got := report.RenderText(rep)
	want := "Component: m\n\nViolations:\n" +
		"- [UNDECLARED_AUTHORITY] use of undeclared authority \"FILES\"\n" +
		"  at member/a.go:3 (3 sites)\n"
	if got != want {
		t.Errorf("RenderText(aggregate) =\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderText_AggregatedWarningFinding(t *testing.T) {
	rep := report.ConformanceReport{
		Component: "m",
		Warnings: []report.Finding{{
			Kind:    report.AnalysisLimitation,
			Message: "analysis-defeating constructs escape typed reference analysis",
			Class:   "AnalysisDefeating",
			SDKKey:  "go1.26.4 linux amd64 map1",
			Sites: []report.AuthoritySite{
				{File: "member/link.go", Line: 3},
				{File: "member/stub.s", Line: 1},
			},
		}},
	}
	got := report.RenderText(rep)
	if !strings.Contains(got, "  at member/link.go:3 (2 sites)\n") {
		t.Errorf("RenderText(aggregate warning) missing first-site + count: %q", got)
	}
	if strings.Count(got, "member/") != 1 || strings.Contains(got, "at member/stub.s") {
		t.Errorf("aggregated finding must render one entry for the first site only: %q", got)
	}
}

func TestFinding_JSONSitesClassSDKKeyRoundTrip(t *testing.T) {
	f := report.Finding{
		Kind:    report.UndeclaredAuthority,
		Message: "m",
		Class:   "TrueAuthority",
		SDKKey:  "k",
		Sites:   []report.AuthoritySite{{File: "a.go", Line: 2, Symbol: "os.ReadFile"}},
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got report.Finding
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, f) {
		t.Errorf("round trip = %+v, want %+v", got, f)
	}
}
