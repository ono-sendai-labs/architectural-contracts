package report_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func TestVerdictOf(t *testing.T) {
	tests := []struct {
		name  string
		input report.ConformanceReport
		want  report.Verdict
	}{
		{
			name:  "no violations passes",
			input: report.ConformanceReport{Component: "a"},
			want:  report.VerdictPass,
		},
		{
			name: "warnings only still passes",
			input: report.ConformanceReport{
				Component: "a",
				Warnings:  []report.Finding{{Kind: report.UnusedDependency, Message: "unused"}},
			},
			want: report.VerdictPass,
		},
		{
			name: "one violation fails",
			input: report.ConformanceReport{
				Component:  "a",
				Violations: []report.Finding{{Kind: report.UndeclaredAuthority, Message: "x"}},
			},
			want: report.VerdictFail,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := report.VerdictOf(tt.input); got != tt.want {
				t.Errorf("VerdictOf(%v) = %q, want %q", tt.input.Component, got, tt.want)
			}
		})
	}
}

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
			data, err := report.MarshalReport(tt.input)
			if err != nil {
				t.Fatalf("MarshalReport() error = %v", err)
			}
			var got report.PersistedReport
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("output is not valid JSON: %v\n%s", err, data)
			}
			if got.FormatVersion != report.ReportFormatVersion {
				t.Errorf("format_version = %d, want %d", got.FormatVersion, report.ReportFormatVersion)
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

	first, err := report.MarshalReport(forward)
	if err != nil {
		t.Fatalf("MarshalReport(forward) error = %v", err)
	}
	for i := 0; i < 3; i++ {
		again, err := report.MarshalReport(reverse)
		if err != nil {
			t.Fatalf("MarshalReport(reverse) error = %v", err)
		}
		if string(first) != string(again) {
			t.Fatalf("canonical bytes differ between orderings (iteration %d):\n%s\n---\n%s", i, first, again)
		}
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
	data, err := report.MarshalReport(input)
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	got, err := report.DecodeReport(data)
	if err != nil {
		t.Fatalf("DecodeReport() error = %v", err)
	}
	if !reflectDeepEqualReports(got.Report, input) {
		t.Errorf("decoded report = %#v, want %#v", got.Report, input)
	}
	if got.Verdict != report.VerdictFail {
		t.Errorf("decoded verdict = %q, want %q", got.Verdict, report.VerdictFail)
	}
}

func reflectDeepEqualReports(a, b report.ConformanceReport) bool {
	return (a.Component == b.Component) && equalFindings(a.Violations, b.Violations) && equalFindings(a.Warnings, b.Warnings)
}

func equalFindings(a, b []report.Finding) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestDecodeReport_RejectsInvalidInput(t *testing.T) {
	passing, err := report.MarshalReport(report.ConformanceReport{Component: "c"})
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	failing, err := report.MarshalReport(report.ConformanceReport{
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
			wantErr: report.ErrMalformedReport,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: report.ErrMalformedReport,
		},
		{
			name:    "unknown verdict",
			input:   string(stampVerdict(t, passing, "maybe")),
			wantErr: report.ErrUnknownVerdict,
		},
		{
			name:    "verdict contradicts report",
			input:   string(stampVerdict(t, passing, "fail")),
			wantErr: report.ErrVerdictMismatch,
		},
		{
			name:    "fail stamp contradicts passing report too",
			input:   string(stampVerdict(t, failing, "pass")),
			wantErr: report.ErrVerdictMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := report.DecodeReport([]byte(tt.input))
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
	data, err := report.MarshalReport(report.ConformanceReport{Component: "c"})
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw["format_version"] = report.ReportFormatVersion + 1
	bumped, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := report.DecodeReport(bumped); !errors.Is(err, report.ErrUnsupportedReportVersion) {
		t.Errorf("DecodeReport() error = %v, want wrapping ErrUnsupportedReportVersion", err)
	}
}

func TestDecodeReport_IgnoresUnknownFields(t *testing.T) {
	data, err := report.MarshalReport(report.ConformanceReport{Component: "c"})
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
	got, err := report.DecodeReport(extended)
	if err != nil {
		t.Fatalf("DecodeReport() error = %v", err)
	}
	if got.Report.Component != "c" {
		t.Errorf("decoded component = %q, want %q", got.Report.Component, "c")
	}
}

func TestVerdictString(t *testing.T) {
	if report.VerdictPass.String() != "pass" || report.VerdictFail.String() != "fail" {
		t.Errorf("Verdict String() values unexpected: %q %q", report.VerdictPass, report.VerdictFail)
	}
}
