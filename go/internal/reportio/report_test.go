package reportio

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func TestMarshalReport_DerivesVerdictAndFormatVersion(t *testing.T) {
	tests := []struct {
		name        string
		input       report.ConformanceReport
		wantVerdict report.Verdict
	}{
		{
			name:        "passing report",
			input:       report.ConformanceReport{Component: "clean"},
			wantVerdict: report.VerdictPass,
		},
		{
			name: "failing report",
			input: report.ConformanceReport{
				Component:  "dirty",
				Violations: []report.Finding{{Kind: report.UndeclaredDependency, Message: "m"}},
			},
			wantVerdict: report.VerdictFail,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := MarshalReport(tt.input)
			if err != nil {
				t.Fatalf("MarshalReport() error = %v", err)
			}
			var got PersistedReport
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("output is not valid JSON: %v\n%s", err, data)
			}
			if got.FormatVersion != ReportFormatVersion {
				t.Errorf("format_version = %d, want %d", got.FormatVersion, ReportFormatVersion)
			}
			if got.Verdict != tt.wantVerdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, tt.wantVerdict)
			}
			if got.Report.Component != tt.input.Component {
				t.Errorf("embedded report component = %q, want %q", got.Report.Component, tt.input.Component)
			}
		})
	}
}

func TestMarshalReport_DeterministicAcrossOrderings(t *testing.T) {
	findings := []report.Finding{
		{Kind: report.UndeclaredAuthority, Message: "zulu", Location: report.Location{File: "b.go", Line: 2}},
		{Kind: report.UndeclaredDependency, Message: "alpha", Location: report.Location{File: "a.go", Line: 9}},
		{Kind: report.UndeclaredDependency, Message: "alpha", Location: report.Location{File: "a.go", Line: 1}},
	}
	forward := report.ConformanceReport{Component: "c", Violations: findings}
	reverse := report.ConformanceReport{Component: "c", Violations: []report.Finding{findings[2], findings[1], findings[0]}}

	first, err := MarshalReport(forward)
	if err != nil {
		t.Fatalf("MarshalReport(forward) error = %v", err)
	}
	for i := 0; i < 3; i++ {
		again, err := MarshalReport(reverse)
		if err != nil {
			t.Fatalf("MarshalReport(reverse) error = %v", err)
		}
		if string(first) != string(again) {
			t.Fatalf("canonical bytes differ between orderings (iteration %d):\n%s\n---\n%s", i, first, again)
		}
	}
}

func TestMarshalReport_DiagnosticsRemainCanonicalAcrossRepeatedEmission(t *testing.T) {
	input := report.ConformanceReport{
		Component: "c",
		Diagnostics: report.Diagnostics{
			NonMemberExportArtifactCount: 4,
			NonMemberExportBytes:         1234,
		},
		Violations: []report.Finding{{Kind: report.UndeclaredDependency, Message: "m"}},
	}

	first, err := MarshalReport(input)
	if err != nil {
		t.Fatalf("first MarshalReport() error = %v", err)
	}
	second, err := MarshalReport(input)
	if err != nil {
		t.Fatalf("second MarshalReport() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeated diagnostic report emission changed bytes:\n%s\n---\n%s", first, second)
	}
	if !strings.Contains(string(first), `"non_member_export_artifact_count": 4`) || !strings.Contains(string(first), `"non_member_export_bytes": 1234`) {
		t.Fatalf("canonical report omitted diagnostics: %s", first)
	}
}

func TestMarshalReport_DeterministicAcrossDependencyAndEvidenceOrderings(t *testing.T) {
	evidenceA := []string{"alpha frame", "beta frame"}
	evidenceB := []string{"beta frame", "alpha frame"}
	forward := report.ConformanceReport{
		Component: "c",
		Dependencies: []report.DependencyBoundary{
			{Component: "zeta"},
			{Component: "alpha"},
		},
		Violations: []report.Finding{
			{Kind: report.UndeclaredAuthority, Message: "same", Location: report.Location{File: "a.go", Line: 1}, Evidence: evidenceA},
			{Kind: report.UndeclaredAuthority, Message: "same", Location: report.Location{File: "a.go", Line: 1}, Evidence: evidenceB},
		},
	}
	reverse := report.ConformanceReport{
		Component: "c",
		Dependencies: []report.DependencyBoundary{
			{Component: "alpha"},
			{Component: "zeta"},
		},
		Violations: []report.Finding{
			{Kind: report.UndeclaredAuthority, Message: "same", Location: report.Location{File: "a.go", Line: 1}, Evidence: evidenceB},
			{Kind: report.UndeclaredAuthority, Message: "same", Location: report.Location{File: "a.go", Line: 1}, Evidence: evidenceA},
		},
	}

	first, err := MarshalReport(forward)
	if err != nil {
		t.Fatalf("MarshalReport(forward) error = %v", err)
	}
	again, err := MarshalReport(reverse)
	if err != nil {
		t.Fatalf("MarshalReport(reverse) error = %v", err)
	}
	if string(first) != string(again) {
		t.Fatalf("canonical bytes differ between dependency/evidence orderings:\n%s\n---\n%s", first, again)
	}
}

func TestMarshalReport_DirectJSONCannotStampContradictoryVerdict(t *testing.T) {
	stamped := PersistedReport{
		FormatVersion: ReportFormatVersion,
		Verdict:       report.VerdictFail,
		Report:        report.ConformanceReport{Component: "clean"},
	}
	data, err := json.Marshal(stamped)
	if err != nil {
		t.Fatalf("json.Marshal(PersistedReport) error = %v", err)
	}
	var got PersistedReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("round-trip of a stamped value failed: %v", err)
	}
	if got.Verdict != report.VerdictPass {
		t.Errorf("direct serialization emitted verdict %q; the derivation must always win over a stamped value", got.Verdict)
	}
}

func TestDecodeReport_RoundTrip(t *testing.T) {
	input := report.ConformanceReport{
		Component: "c",
		Violations: []report.Finding{
			{Kind: report.CallsUndeclaredInterface, Message: "m", Location: report.Location{File: "f.go", Line: 3}},
		},
		Warnings: []report.Finding{
			{Kind: report.UnusedDependency, Message: "w"},
		},
	}
	data, err := MarshalReport(input)
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	got, err := DecodeReport(data)
	if err != nil {
		t.Fatalf("DecodeReport() error = %v", err)
	}
	if !reflect.DeepEqual(got.Report, input) {
		t.Errorf("decoded report = %#v, want %#v", got.Report, input)
	}
	if got.Verdict != report.VerdictFail {
		t.Errorf("decoded verdict = %q, want %q", got.Verdict, report.VerdictFail)
	}
}

func TestDecodeReport_LegacyArtifactDefaultsDiagnostics(t *testing.T) {
	legacy := []byte(`{"format_version":1,"verdict":"fail","report":{"component":"legacy","violations":[{"kind":"UNDECLARED_DEPENDENCY","message":"old"}],"warnings":[]}}`)

	got, err := DecodeReport(legacy)
	if err != nil {
		t.Fatalf("DecodeReport() error = %v", err)
	}
	if got.Report.Diagnostics != (report.Diagnostics{}) {
		t.Fatalf("legacy diagnostics = %#v, want zero value", got.Report.Diagnostics)
	}
	if got.Verdict != report.VerdictFail {
		t.Fatalf("legacy verdict = %q, want %q", got.Verdict, report.VerdictFail)
	}
}

func TestDecodeReport_RejectsInvalidInput(t *testing.T) {
	passing, err := MarshalReport(report.ConformanceReport{Component: "c"})
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	failing, err := MarshalReport(report.ConformanceReport{
		Component:  "c",
		Violations: []report.Finding{{Kind: report.UndeclaredAuthority, Message: "m"}},
	})
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}

	stampVerdict := func(t *testing.T, data []byte, verdict string) []byte {
		t.Helper()
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal fixture: %v", err)
		}
		raw["verdict"] = verdict
		out, err := json.Marshal(raw)
		if err != nil {
			t.Fatalf("marshal fixture: %v", err)
		}
		return out
	}

	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "garbage input",
			input:   "{not json",
			wantErr: ErrMalformedReport,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: ErrMalformedReport,
		},
		{
			name:    "unknown verdict",
			input:   string(stampVerdict(t, passing, "maybe")),
			wantErr: ErrUnknownVerdict,
		},
		{
			name:    "verdict contradicts report",
			input:   string(stampVerdict(t, passing, "fail")),
			wantErr: ErrVerdictMismatch,
		},
		{
			name:    "fail stamp contradicts passing report too",
			input:   string(stampVerdict(t, failing, "pass")),
			wantErr: ErrVerdictMismatch,
		},
		{
			name:    "missing report field",
			input:   `{"format_version":1,"verdict":"pass"}`,
			wantErr: ErrMalformedReport,
		},
		{
			name:    "null report field",
			input:   `{"format_version":1,"verdict":"pass","report":null}`,
			wantErr: ErrMalformedReport,
		},
		{
			name:    "missing verdict field",
			input:   `{"format_version":1,"report":{"component":"c"}}`,
			wantErr: ErrUnknownVerdict,
		},
		{
			name:    "missing format_version field",
			input:   `{"verdict":"pass","report":{"component":"c"}}`,
			wantErr: ErrUnsupportedReportVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeReport([]byte(tt.input))
			if err == nil {
				t.Fatalf("DecodeReport() succeeded with %#v, want error %v", got, tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("DecodeReport() error = %v, want wrapping %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeReport_RejectsUnsupportedFormatVersion(t *testing.T) {
	data, err := MarshalReport(report.ConformanceReport{Component: "c"})
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw["format_version"] = ReportFormatVersion + 1
	bumped, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := DecodeReport(bumped); !errors.Is(err, ErrUnsupportedReportVersion) {
		t.Errorf("DecodeReport() error = %v, want wrapping ErrUnsupportedReportVersion", err)
	}
}

func TestDecodeReport_EnforcesSizeLimitDirectly(t *testing.T) {
	oversized := strings.Repeat("a", int(MaxReportBytes)+1)
	_, err := DecodeReport([]byte(oversized))
	if err == nil {
		t.Fatal("DecodeReport() succeeded on oversized input, want error")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("error = %v, want a size-limit diagnostic", err)
	}
}

func TestUnmarshalJSON_EnforcesSizeLimit(t *testing.T) {
	component := strings.Repeat("c", int(MaxReportBytes))
	oversized := `{"format_version":1,"verdict":"pass","report":{"component":"` + component + `"}}`
	var persisted PersistedReport
	err := json.Unmarshal([]byte(oversized), &persisted)
	if err == nil {
		t.Fatal("json.Unmarshal() succeeded on oversized input, want error")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("error = %v, want a size-limit diagnostic", err)
	}
}

func TestDecodeReport_IgnoresUnknownFields(t *testing.T) {
	data, err := MarshalReport(report.ConformanceReport{Component: "c"})
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw["future_field"] = "ignored"
	extended, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := DecodeReport(extended)
	if err != nil {
		t.Fatalf("DecodeReport() error = %v", err)
	}
	if got.Report.Component != "c" {
		t.Errorf("decoded component = %q, want %q", got.Report.Component, "c")
	}
}

func TestVerdictGolden_RoundTripAndCanonicalBytes(t *testing.T) {
	tests := []struct {
		name    string
		verdict report.Verdict
		want    string
	}{
		{name: "pass", verdict: report.VerdictPass, want: "pass\n"},
		{name: "fail", verdict: report.VerdictFail, want: "fail\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := MarshalVerdict(tt.verdict)
			if err != nil {
				t.Fatalf("MarshalVerdict() error = %v", err)
			}
			if string(data) != tt.want {
				t.Fatalf("MarshalVerdict() = %q, want %q", data, tt.want)
			}
			got, err := DecodeVerdictGolden(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("DecodeVerdictGolden() error = %v", err)
			}
			if got != tt.verdict {
				t.Errorf("DecodeVerdictGolden() = %q, want %q", got, tt.verdict)
			}
		})
	}
}

func TestVerdictGolden_RejectsNonCanonicalContent(t *testing.T) {
	for _, input := range []string{
		"pass",
		"pass\n\n",
		"maybe\n",
		"$(touch SHOULD_NOT_RUN)\n",
	} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			if _, err := DecodeVerdictGolden(strings.NewReader(input)); !errors.Is(err, ErrInvalidVerdictGolden) {
				t.Errorf("DecodeVerdictGolden(%q) error = %v, want ErrInvalidVerdictGolden", input, err)
			}
		})
	}
}

func TestVerdictGolden_RejectsUnknownVerdict(t *testing.T) {
	if _, err := MarshalVerdict(report.Verdict("maybe")); !errors.Is(err, ErrInvalidVerdictGolden) {
		t.Errorf("MarshalVerdict() error = %v, want ErrInvalidVerdictGolden", err)
	}
}

func TestMarshalReport_SitesClassAndSDKKeyCanonical(t *testing.T) {
	sitesFwd := []report.AuthoritySite{
		{File: "member/c.go", Line: 11, Symbol: "os.Create"},
		{File: "member/a.go", Line: 3, Symbol: "os.ReadFile"},
		{File: "member/a.go", Line: 3, Symbol: "os.Create"},
		{File: "member/a.go", Line: 3, Symbol: "os.ReadFile"}, // exact duplicate
	}
	sitesRev := []report.AuthoritySite{
		{File: "member/a.go", Line: 3, Symbol: "os.ReadFile"},
		{File: "member/a.go", Line: 3, Symbol: "os.ReadFile"},
		{File: "member/a.go", Line: 3, Symbol: "os.Create"},
		{File: "member/a.go", Line: 3, Symbol: "os.Create"},
		{File: "member/c.go", Line: 11, Symbol: "os.Create"},
	}
	mk := func(sites []report.AuthoritySite) report.ConformanceReport {
		return report.ConformanceReport{
			Component: "c",
			Violations: []report.Finding{{
				Kind:     report.UndeclaredAuthority,
				Message:  "use of undeclared authority \"FILES\"",
				Class:    "TrueAuthority",
				SDKKey:   "sdk{toolchain_version:\"go1.26.4\" goos:\"linux\" goarch:\"amd64\" cgo_enabled:false build_tags:[] goexperiment:\"\" classifier_hash:\"\" map_format_version:1}",
				Sites:    sites,
				Evidence: []string{"os.ReadFile at os/file.go:331"},
			}},
		}
	}
	first, err := MarshalReport(mk(sitesFwd))
	if err != nil {
		t.Fatalf("MarshalReport error = %v", err)
	}
	second, err := MarshalReport(mk(sitesRev))
	if err != nil {
		t.Fatalf("MarshalReport error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("reordered sites produced different bytes:\n%s\n---\n%s", first, second)
	}

	decoded, err := DecodeReport(first)
	if err != nil {
		t.Fatalf("DecodeReport error = %v", err)
	}
	v := decoded.Report.Violations[0]
	if v.Class != "TrueAuthority" || v.SDKKey == "" {
		t.Errorf("round trip lost class/sdk_key: %+v", v)
	}
	wantSites := []report.AuthoritySite{
		{File: "member/a.go", Line: 3, Symbol: "os.Create"},
		{File: "member/a.go", Line: 3, Symbol: "os.ReadFile"},
		{File: "member/c.go", Line: 11, Symbol: "os.Create"},
	}
	if !reflect.DeepEqual(v.Sites, wantSites) {
		t.Errorf("canonical sites = %+v, want %+v", v.Sites, wantSites)
	}
	if !reflect.DeepEqual(v.Evidence, []string{"os.ReadFile at os/file.go:331"}) {
		t.Errorf("round trip lost first-site evidence: %+v", v)
	}
}

func TestMarshalReport_UnusedAuthorityWarningRemainsPassing(t *testing.T) {
	input := report.ConformanceReport{
		Component: "component",
		Warnings: []report.Finding{{
			Kind:    report.UnusedAuthority,
			Message: `declared authority "FILES" is unused`,
		}},
	}

	data, err := MarshalReport(input)
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	if !strings.Contains(string(data), `"kind": "UNUSED_AUTHORITY"`) {
		t.Fatalf("serialized report = %s, want UNUSED_AUTHORITY", data)
	}
	got, err := DecodeReport(data)
	if err != nil {
		t.Fatalf("DecodeReport() error = %v", err)
	}
	if got.Verdict != report.VerdictPass {
		t.Fatalf("persisted verdict = %q, want %q for a warning-only report", got.Verdict, report.VerdictPass)
	}
	if !reflect.DeepEqual(got.Report, input) {
		t.Fatalf("decoded report = %#v, want %#v", got.Report, input)
	}
}
