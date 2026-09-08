package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func writeReportFile(t *testing.T, r report.ConformanceReport) string {
	t.Helper()
	data, err := report.MarshalReport(r)
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "component.report.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return path
}

func TestVerdict_ExitCodes(t *testing.T) {
	passingPath := writeReportFile(t, report.ConformanceReport{Component: "clean"})
	failingPath := writeReportFile(t, report.ConformanceReport{
		Component:  "dirty",
		Violations: []report.Finding{{Kind: report.UndeclaredAuthority, Message: "FILES"}},
	})

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStderr string
	}{
		{
			name:     "pass expected and pass actual",
			args:     []string{"verdict", passingPath, "--expect=pass"},
			wantExit: 0,
		},
		{
			name:     "fail expected and fail actual",
			args:     []string{"verdict", failingPath, "--expect=fail"},
			wantExit: 0,
		},
		{
			name:       "pass expected but fail actual",
			args:       []string{"verdict", failingPath, "--expect=pass"},
			wantExit:   1,
			wantStderr: "expected pass, got fail",
		},
		{
			name:       "fail expected but pass actual",
			args:       []string{"verdict", passingPath, "--expect=fail"},
			wantExit:   1,
			wantStderr: "expected fail, got pass",
		},
		{
			name:       "missing report file",
			args:       []string{"verdict", filepath.Join(t.TempDir(), "missing.json"), "--expect=pass"},
			wantExit:   2,
			wantStderr: "error:",
		},
		{
			name:       "missing expect flag",
			args:       []string{"verdict", passingPath},
			wantExit:   2,
			wantStderr: "error:",
		},
		{
			name:       "invalid expect value",
			args:       []string{"verdict", passingPath, "--expect=maybe"},
			wantExit:   2,
			wantStderr: "--expect",
		},
		{
			name:       "duplicate expect flag",
			args:       []string{"verdict", passingPath, "--expect=pass", "--expect=fail"},
			wantExit:   2,
			wantStderr: "duplicate",
		},
		{
			name:       "missing report argument",
			args:       []string{"verdict", "--expect=pass"},
			wantExit:   2,
			wantStderr: "error:",
		},
		{
			name:       "extra positional argument",
			args:       []string{"verdict", passingPath, "extra", "--expect=pass"},
			wantExit:   2,
			wantStderr: "error:",
		},
		{
			name:       "unknown option",
			args:       []string{"verdict", passingPath, "--bogus"},
			wantExit:   2,
			wantStderr: "unknown option",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			runner := &app.Runner{}
			got := runner.Run(tt.args, &stdout, &stderr)
			if got != tt.wantExit {
				t.Errorf("Run(%q) exit = %d, want %d (stderr: %q)", tt.args, got, tt.wantExit, stderr.String())
			}
			if tt.wantStderr != "" && !bytes.Contains([]byte(stderr.String()), []byte(tt.wantStderr)) {
				t.Errorf("stderr = %q, want containing %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestVerdict_MalformedFileContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.report.json")
	if err := os.WriteFile(path, []byte("{not a report"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout, stderr bytes.Buffer
	got := (&app.Runner{}).Run([]string{"verdict", path, "--expect=pass"}, &stdout, &stderr)
	if got != 2 {
		t.Errorf("exit = %d, want 2 (stderr: %q)", got, stderr.String())
	}
}

func TestVerdict_UsageListsCommand(t *testing.T) {
	var stdout, _ bytes.Buffer
	runner := &app.Runner{}
	runner.Run([]string{"help"}, &stdout, &bytes.Buffer{})
	if !bytes.Contains(stdout.Bytes(), []byte("arcc verdict")) {
		t.Errorf("usage output does not document the verdict command:\n%s", stdout.String())
	}
}
