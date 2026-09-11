package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func writeReportFile(t *testing.T, r report.ConformanceReport) string {
	t.Helper()
	data, err := artifactio.MarshalReport(r)
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
			if tt.wantStderr != "" && !bytes.Contains(stderr.Bytes(), []byte(tt.wantStderr)) {
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

func TestVerdict_ExpectFile(t *testing.T) {
	passingPath := writeReportFile(t, report.ConformanceReport{Component: "clean"})
	failingPath := writeReportFile(t, report.ConformanceReport{
		Component:  "dirty",
		Violations: []report.Finding{{Kind: report.UndeclaredAuthority, Message: "FILES"}},
	})

	tests := []struct {
		name       string
		reportPath string
		golden     string
		wantExit   int
		wantStderr string
	}{
		{
			name:       "matching pass golden",
			reportPath: passingPath,
			golden:     "pass\n",
		},
		{
			name:       "matching fail golden",
			reportPath: failingPath,
			golden:     "fail\n",
		},
		{
			name:       "mismatch identifies both verdicts",
			reportPath: passingPath,
			golden:     "fail\n",
			wantExit:   1,
			wantStderr: "expected fail, got pass",
		},
		{
			name:       "hostile golden is data",
			reportPath: passingPath,
			golden:     "$(touch SHOULD_NOT_RUN)\n",
			wantExit:   2,
			wantStderr: "verdict golden",
		},
		{
			name:       "missing final newline is not canonical",
			reportPath: passingPath,
			golden:     "pass",
			wantExit:   2,
			wantStderr: "verdict golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goldenPath := filepath.Join(t.TempDir(), "component.verdict.golden")
			if err := os.WriteFile(goldenPath, []byte(tt.golden), 0o644); err != nil {
				t.Fatalf("write golden: %v", err)
			}
			var stdout, stderr bytes.Buffer
			got := (&app.Runner{}).Run([]string{
				"verdict",
				tt.reportPath,
				"--expect-file=" + goldenPath,
			}, &stdout, &stderr)
			if got != tt.wantExit {
				t.Errorf("Run() exit = %d, want %d (stdout: %q, stderr: %q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			if tt.wantStderr != "" && !bytes.Contains(stderr.Bytes(), []byte(tt.wantStderr)) {
				t.Errorf("stderr = %q, want containing %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestVerdict_ExpectFileRejectsConflictingOptions(t *testing.T) {
	reportPath := writeReportFile(t, report.ConformanceReport{Component: "clean"})
	goldenPath := filepath.Join(t.TempDir(), "component.verdict.golden")
	if err := os.WriteFile(goldenPath, []byte("pass\n"), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	var stdout, stderr bytes.Buffer
	got := (&app.Runner{}).Run([]string{
		"verdict",
		reportPath,
		"--expect=pass",
		"--expect-file=" + goldenPath,
	}, &stdout, &stderr)
	if got != 2 {
		t.Errorf("Run() exit = %d, want 2 (stderr: %q)", got, stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("one of --expect or --expect-file")) {
		t.Errorf("stderr = %q, want conflicting-option diagnostic", stderr.String())
	}
}

func TestVerdictGolden_ImportPrefixRewritePreservesBytesAndResult(t *testing.T) {
	input := report.ConformanceReport{
		Component: "component",
		Violations: []report.Finding{{
			Kind:    report.UndeclaredDependency,
			Message: `package "example.com/upstream/member" imports undeclared dependency "example.com/upstream/dep"`,
			Location: report.Location{
				File: "example.com/upstream/member/member.go",
				Line: 12,
			},
		}},
	}
	original, err := artifactio.MarshalReport(input)
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	rewrite := func(data []byte) []byte {
		return bytes.ReplaceAll(data, []byte("example.com/upstream/"), []byte("host.example/canonical/"))
	}
	rewritten := rewrite(original)
	if twice := rewrite(rewritten); !bytes.Equal(twice, rewritten) {
		t.Fatalf("simulated host prefix rewrite is not idempotent:\nfirst: %s\nsecond: %s", rewritten, twice)
	}

	originalReport := filepath.Join(t.TempDir(), "original.report.json")
	rewrittenReport := filepath.Join(t.TempDir(), "rewritten.report.json")
	for path, data := range map[string][]byte{
		originalReport:  original,
		rewrittenReport: rewritten,
	} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write report %s: %v", path, err)
		}
	}

	originalDecoded, err := artifactio.ReadReportFile(originalReport)
	if err != nil {
		t.Fatalf("ReadReportFile(original) error = %v", err)
	}
	rewrittenDecoded, err := artifactio.ReadReportFile(rewrittenReport)
	if err != nil {
		t.Fatalf("ReadReportFile(rewritten) error = %v", err)
	}
	originalVerdictBytes, err := artifactio.MarshalVerdict(originalDecoded.Verdict)
	if err != nil {
		t.Fatalf("MarshalVerdict(original) error = %v", err)
	}
	rewrittenVerdictBytes, err := artifactio.MarshalVerdict(rewrittenDecoded.Verdict)
	if err != nil {
		t.Fatalf("MarshalVerdict(rewritten) error = %v", err)
	}
	if !bytes.Equal(originalVerdictBytes, rewrittenVerdictBytes) {
		t.Fatalf("verdict golden bytes changed after prefix rewrite: %q vs %q", originalVerdictBytes, rewrittenVerdictBytes)
	}

	goldenPath := filepath.Join(t.TempDir(), "component.verdict.golden")
	if err := os.WriteFile(goldenPath, originalVerdictBytes, 0o644); err != nil {
		t.Fatalf("write verdict golden: %v", err)
	}
	var originalStdout, originalStderr bytes.Buffer
	originalExit := (&app.Runner{}).Run([]string{"verdict", originalReport, "--expect-file=" + goldenPath}, &originalStdout, &originalStderr)
	var rewrittenStdout, rewrittenStderr bytes.Buffer
	rewrittenExit := (&app.Runner{}).Run([]string{"verdict", rewrittenReport, "--expect-file=" + goldenPath}, &rewrittenStdout, &rewrittenStderr)
	if originalExit != 0 || rewrittenExit != 0 {
		t.Fatalf("verdict-golden results = (%d, %d), want (0, 0); stderr = (%q, %q)", originalExit, rewrittenExit, originalStderr.String(), rewrittenStderr.String())
	}
	if originalStdout.String() != rewrittenStdout.String() {
		t.Errorf("verdict-golden output changed after prefix rewrite: %q vs %q", originalStdout.String(), rewrittenStdout.String())
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
