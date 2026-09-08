package report_test

import (
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

func TestVerdictString(t *testing.T) {
	if report.VerdictPass.String() != "pass" || report.VerdictFail.String() != "fail" {
		t.Errorf("Verdict String() values unexpected: %q %q", report.VerdictPass, report.VerdictFail)
	}
}
