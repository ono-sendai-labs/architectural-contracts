package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

type mockLoader struct {
	calledWith string
	facts      facts.PackageFacts
	err        error
}

func (m *mockLoader) Load(root string) (facts.PackageFacts, error) {
	m.calledWith = root
	return m.facts, m.err
}

type mockAnalyzer struct {
	calledWith capanalyzer.AnalyzeRequest
	findings   []capanalyzer.CapabilityFinding
	err        error
}

func (m *mockAnalyzer) Analyze(req capanalyzer.AnalyzeRequest) ([]capanalyzer.CapabilityFinding, error) {
	m.calledWith = req
	return m.findings, m.err
}

func TestRunner_VersionAndHelp(t *testing.T) {
	runner := &app.Runner{}

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "version",
			args:       []string{"--version"},
			wantExit:   0,
			wantStdout: "arcc 0.0.0-dev\n",
		},
		{
			name:       "help",
			args:       []string{"help"},
			wantExit:   0,
			wantStdout: "Usage:",
		},
		{
			name:       "no args",
			args:       []string{},
			wantExit:   0,
			wantStdout: "Usage:",
		},
		{
			name:       "unknown command",
			args:       []string{"unknown"},
			wantExit:   2,
			wantStderr: "unknown command",
		},
		{
			name:       "bad arity",
			args:       []string{"check"},
			wantExit:   2,
			wantStderr: "Usage:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runner.Run(tt.args, &stdout, &stderr)
			if exitCode != tt.wantExit {
				t.Errorf("Run() exitCode = %d, want %d", exitCode, tt.wantExit)
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want to contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func createTempComponent(t *testing.T, name string, manifestContent string, files map[string]string) (string, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Write go.mod so packages load successfully
	goMod := fmt.Sprintf("module example.com/temp/%s\n\ngo 1.21\n", name)
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	manifestPath := filepath.Join(tmpDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write temp manifest: %v", err)
	}

	for f, content := range files {
		absPath := filepath.Join(tmpDir, f)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file %s: %v", f, err)
		}
	}
	return tmpDir, manifestPath
}

func TestRunner_Check_Success(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "success", manifestContent, files)

	loader := func(root string) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(root)
	}

	analyzer := &mockAnalyzer{}
	runner := &app.Runner{
		Loader:   loader,
		Analyzer: analyzer,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() returned %d, want 0. Stderr: %s", exitCode, stderr.String())
	}

	got := stdout.String()
	want := `Component "test-comp" conforms / ambient-authority-free`
	if !strings.Contains(got, want) {
		t.Errorf("stdout = %q, want to contain %q", got, want)
	}

	// Verify analyzer request was correct
	if len(analyzer.calledWith.Packages) != 1 || analyzer.calledWith.Packages[0] != "example.com/temp/success" {
		t.Errorf("analyzer called with packages %v, want ['example.com/temp/success']", analyzer.calledWith.Packages)
	}
	if len(analyzer.calledWith.PruneAt) != 0 {
		t.Errorf("analyzer called with PruneAt %v, want empty", analyzer.calledWith.PruneAt)
	}
}

func TestRunner_Check_Success_JSON(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "success-json", manifestContent, files)

	loader := func(root string) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(root)
	}

	analyzer := &mockAnalyzer{}
	runner := &app.Runner{
		Loader:   loader,
		Analyzer: analyzer,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath, "--format=json"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() returned %d, want 0. Stderr: %s", exitCode, stderr.String())
	}

	var rep report.ConformanceReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v, stdout: %s", err, stdout.String())
	}

	if rep.Component != "test-comp" {
		t.Errorf("unmarshaled report component = %q, want %q", rep.Component, "test-comp")
	}

	if len(rep.Violations) != 0 {
		t.Errorf("violations count = %d, want 0", len(rep.Violations))
	}

	// Check for a trailing newline
	gotOut := stdout.String()
	if !strings.HasSuffix(gotOut, "\n") {
		t.Errorf("JSON output does not end with a trailing newline")
	}
}

func TestRunner_Check_Violation(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "violation", manifestContent, files)

	loader := func(root string) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(root)
	}

	analyzer := &mockAnalyzer{
		findings: []capanalyzer.CapabilityFinding{
			{
				Package:    "example.com/temp/violation",
				Capability: "FILES",
				Class:      capanalyzer.TrueAuthority,
				CallPath: []capanalyzer.Frame{
					{Func: "main.Hello", File: "api.go", Line: 3},
				},
			},
		},
	}
	runner := &app.Runner{
		Loader:   loader,
		Analyzer: analyzer,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("Run() returned %d, want 1. Stderr: %s", exitCode, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "UNDECLARED_AUTHORITY") {
		t.Errorf("stdout = %q, want to contain UNDECLARED_AUTHORITY", got)
	}
}

func TestRunner_Check_DeclaredAuthority_Pass(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
declared_authority: "FILES"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "declared-auth", manifestContent, files)

	loader := func(root string) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(root)
	}

	analyzer := &mockAnalyzer{
		findings: []capanalyzer.CapabilityFinding{
			{
				Package:    "example.com/temp/declared-auth",
				Capability: "FILES",
				Class:      capanalyzer.TrueAuthority,
				CallPath: []capanalyzer.Frame{
					{Func: "main.Hello", File: "api.go", Line: 3},
				},
			},
		},
	}
	runner := &app.Runner{
		Loader:   loader,
		Analyzer: analyzer,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() returned %d, want 0. Stderr: %s", exitCode, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "conforms") {
		t.Errorf("stdout = %q, want to contain conforms", got)
	}
}

func TestRunner_Check_InterfaceValidationError(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "missing.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "missing-file", manifestContent, files)

	loader := func(root string) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(root)
	}

	runner := &app.Runner{
		Loader:   loader,
		Analyzer: &mockAnalyzer{},
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("Run() returned %d, want 2. Stdout: %s", exitCode, stdout.String())
	}

	gotErr := stderr.String()
	if !strings.Contains(gotErr, "missing.go") {
		t.Errorf("stderr = %q, want to mention missing.go", gotErr)
	}
}

func TestRunner_Check_LoaderError(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "loader-error", manifestContent, files)

	loader := func(root string) (facts.PackageFacts, error) {
		return facts.PackageFacts{}, fmt.Errorf("injected loader error")
	}

	runner := &app.Runner{
		Loader:   loader,
		Analyzer: &mockAnalyzer{},
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("Run() returned %d, want 2. Stdout: %s", exitCode, stdout.String())
	}

	gotErr := stderr.String()
	if !strings.Contains(gotErr, "injected loader error") {
		t.Errorf("stderr = %q, want to mention injected loader error", gotErr)
	}
}

func TestRunner_Check_AnalyzerError(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "analyzer-error", manifestContent, files)

	loader := func(root string) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(root)
	}

	analyzer := &mockAnalyzer{
		err: fmt.Errorf("injected analyzer error"),
	}
	runner := &app.Runner{
		Loader:   loader,
		Analyzer: analyzer,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("Run() returned %d, want 2. Stdout: %s", exitCode, stdout.String())
	}

	gotErr := stderr.String()
	if !strings.Contains(gotErr, "injected analyzer error") {
		t.Errorf("stderr = %q, want to mention injected analyzer error", gotErr)
	}
}
