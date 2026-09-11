package artifactio_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
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
			data, err := artifactio.MarshalReport(tt.input)
			if err != nil {
				t.Fatalf("MarshalReport() error = %v", err)
			}
			var got artifactio.PersistedReport
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("output is not valid JSON: %v\n%s", err, data)
			}
			if got.FormatVersion != artifactio.ReportFormatVersion {
				t.Errorf("format_version = %d, want %d", got.FormatVersion, artifactio.ReportFormatVersion)
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

	first, err := artifactio.MarshalReport(forward)
	if err != nil {
		t.Fatalf("MarshalReport(forward) error = %v", err)
	}
	for i := 0; i < 3; i++ {
		again, err := artifactio.MarshalReport(reverse)
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

	first, err := artifactio.MarshalReport(input)
	if err != nil {
		t.Fatalf("first MarshalReport() error = %v", err)
	}
	second, err := artifactio.MarshalReport(input)
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

	first, err := artifactio.MarshalReport(forward)
	if err != nil {
		t.Fatalf("MarshalReport(forward) error = %v", err)
	}
	again, err := artifactio.MarshalReport(reverse)
	if err != nil {
		t.Fatalf("MarshalReport(reverse) error = %v", err)
	}
	if string(first) != string(again) {
		t.Fatalf("canonical bytes differ between dependency/evidence orderings:\n%s\n---\n%s", first, again)
	}
}

func TestMarshalReport_DirectJSONCannotStampContradictoryVerdict(t *testing.T) {
	stamped := artifactio.PersistedReport{
		FormatVersion: artifactio.ReportFormatVersion,
		Verdict:       report.VerdictFail,
		Report:        report.ConformanceReport{Component: "clean"},
	}
	data, err := json.Marshal(stamped)
	if err != nil {
		t.Fatalf("json.Marshal(PersistedReport) error = %v", err)
	}
	var got artifactio.PersistedReport
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
	data, err := artifactio.MarshalReport(input)
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	got, err := artifactio.DecodeReport(data)
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

	got, err := artifactio.DecodeReport(legacy)
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
	passing, err := artifactio.MarshalReport(report.ConformanceReport{Component: "c"})
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	failing, err := artifactio.MarshalReport(report.ConformanceReport{
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
			wantErr: artifactio.ErrMalformedReport,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: artifactio.ErrMalformedReport,
		},
		{
			name:    "unknown verdict",
			input:   string(stampVerdict(t, passing, "maybe")),
			wantErr: artifactio.ErrUnknownVerdict,
		},
		{
			name:    "verdict contradicts report",
			input:   string(stampVerdict(t, passing, "fail")),
			wantErr: artifactio.ErrVerdictMismatch,
		},
		{
			name:    "fail stamp contradicts passing report too",
			input:   string(stampVerdict(t, failing, "pass")),
			wantErr: artifactio.ErrVerdictMismatch,
		},
		{
			name:    "missing report field",
			input:   `{"format_version":1,"verdict":"pass"}`,
			wantErr: artifactio.ErrMalformedReport,
		},
		{
			name:    "null report field",
			input:   `{"format_version":1,"verdict":"pass","report":null}`,
			wantErr: artifactio.ErrMalformedReport,
		},
		{
			name:    "missing verdict field",
			input:   `{"format_version":1,"report":{"component":"c"}}`,
			wantErr: artifactio.ErrUnknownVerdict,
		},
		{
			name:    "missing format_version field",
			input:   `{"verdict":"pass","report":{"component":"c"}}`,
			wantErr: artifactio.ErrUnsupportedReportVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := artifactio.DecodeReport([]byte(tt.input))
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
	data, err := artifactio.MarshalReport(report.ConformanceReport{Component: "c"})
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw["format_version"] = artifactio.ReportFormatVersion + 1
	bumped, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := artifactio.DecodeReport(bumped); !errors.Is(err, artifactio.ErrUnsupportedReportVersion) {
		t.Errorf("DecodeReport() error = %v, want wrapping ErrUnsupportedReportVersion", err)
	}
}

func TestDecodeReport_EnforcesSizeLimitDirectly(t *testing.T) {
	oversized := strings.Repeat("a", int(artifactio.MaxReportBytes)+1)
	_, err := artifactio.DecodeReport([]byte(oversized))
	if err == nil {
		t.Fatal("DecodeReport() succeeded on oversized input, want error")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("error = %v, want a size-limit diagnostic", err)
	}
}

func TestUnmarshalJSON_EnforcesSizeLimit(t *testing.T) {
	// Valid JSON over the cap: encoding/json invokes UnmarshalJSON only for
	// syntactically valid top-level values, so the oversized fixture must
	// parse before the size gate fires.
	component := strings.Repeat("c", int(artifactio.MaxReportBytes))
	oversized := `{"format_version":1,"verdict":"pass","report":{"component":"` + component + `"}}`
	var persisted artifactio.PersistedReport
	err := json.Unmarshal([]byte(oversized), &persisted)
	if err == nil {
		t.Fatal("json.Unmarshal() succeeded on oversized input, want error")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("error = %v, want a size-limit diagnostic", err)
	}
}

func TestDecodeReport_IgnoresUnknownFields(t *testing.T) {
	data, err := artifactio.MarshalReport(report.ConformanceReport{Component: "c"})
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
	got, err := artifactio.DecodeReport(extended)
	if err != nil {
		t.Fatalf("DecodeReport() error = %v", err)
	}
	if got.Report.Component != "c" {
		t.Errorf("decoded component = %q, want %q", got.Report.Component, "c")
	}
}

func writeTempReport(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "component.report.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp report: %v", err)
	}
	return path
}

func TestReadReportFile_RoundTrip(t *testing.T) {
	input := report.ConformanceReport{
		Component: "csvtool",
		Violations: []report.Finding{
			{Kind: report.UndeclaredAuthority, Message: "FILES", Location: report.Location{File: "a.go", Line: 7}},
		},
	}
	data, err := artifactio.MarshalReport(input)
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	path := writeTempReport(t, data)

	got, err := artifactio.ReadReportFile(path)
	if err != nil {
		t.Fatalf("ReadReportFile() error = %v", err)
	}
	if got.Verdict != report.VerdictFail {
		t.Errorf("verdict = %q, want %q", got.Verdict, report.VerdictFail)
	}
	if !reflect.DeepEqual(got.Report, input) {
		t.Errorf("decoded report = %#v, want %#v", got.Report, input)
	}
}

func TestReadReportFile_OpenError(t *testing.T) {
	_, err := artifactio.ReadReportFile(filepath.Join(t.TempDir(), "missing.report.json"))
	if err == nil {
		t.Fatal("ReadReportFile() succeeded for a missing file, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want wrapping os.ErrNotExist", err)
	}
}

func TestReadReportFile_RejectsMalformed(t *testing.T) {
	path := writeTempReport(t, []byte("{not a report"))
	_, err := artifactio.ReadReportFile(path)
	if !errors.Is(err, artifactio.ErrMalformedReport) {
		t.Errorf("error = %v, want wrapping artifactio.ErrMalformedReport", err)
	}
}

func TestReadReportFile_RejectsOversizedInput(t *testing.T) {
	oversized := strings.Repeat("a", int(artifactio.MaxReportBytes)+1)
	path := writeTempReport(t, []byte(oversized))
	_, err := artifactio.ReadReportFile(path)
	if err == nil {
		t.Fatal("ReadReportFile() succeeded on an oversized input, want error")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("error = %v, want a size-limit diagnostic", err)
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
	first, err := artifactio.MarshalReport(mk(sitesFwd))
	if err != nil {
		t.Fatalf("MarshalReport error = %v", err)
	}
	second, err := artifactio.MarshalReport(mk(sitesRev))
	if err != nil {
		t.Fatalf("MarshalReport error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("reordered sites produced different bytes:\n%s\n---\n%s", first, second)
	}

	decoded, err := artifactio.DecodeReport(first)
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
