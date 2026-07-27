package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

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
		{
			name:       "unknown option",
			args:       []string{"check", "component.textproto", "--bogus"},
			wantExit:   2,
			wantStderr: "unknown option: --bogus",
		},
		{
			name:       "empty layout value",
			args:       []string{"check", "component.textproto", "--package-layout="},
			wantExit:   2,
			wantStderr: "empty package layout value",
		},
		{
			name:       "missing layout value (no equals)",
			args:       []string{"check", "component.textproto", "--package-layout"},
			wantExit:   2,
			wantStderr: "missing package layout value",
		},
		{
			name:       "duplicate layout",
			args:       []string{"check", "component.textproto", "--package-layout=a.json", "--package-layout=b.json"},
			wantExit:   2,
			wantStderr: "duplicate option: --package-layout",
		},
		{
			name:       "duplicate format",
			args:       []string{"check", "component.textproto", "--format=json", "--format=json"},
			wantExit:   2,
			wantStderr: "duplicate option: --format=json",
		},
		{
			name:       "unexpected extra positional argument",
			args:       []string{"check", "component.textproto", "extra-arg"},
			wantExit:   2,
			wantStderr: "error: check command requires exactly one argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runner.Run(tt.args, &stdout, &stderr)
			if exitCode != tt.wantExit {
				t.Errorf("Run() exitCode = %d, want %d", exitCode, tt.wantExit)
			}
			if tt.wantStdout != "" {
				if !strings.Contains(stdout.String(), tt.wantStdout) {
					t.Errorf("stdout = %q, want to contain %q", stdout.String(), tt.wantStdout)
				}
			} else {
				if stdout.Len() != 0 {
					t.Errorf("stdout = %q, want empty", stdout.String())
				}
			}
			if tt.wantStderr != "" {
				if !strings.Contains(stderr.String(), tt.wantStderr) {
					t.Errorf("stderr = %q, want to contain %q", stderr.String(), tt.wantStderr)
				}
			} else {
				if stderr.Len() != 0 {
					t.Errorf("stderr = %q, want empty", stderr.String())
				}
			}
		})
	}
}

func createTempComponent(t *testing.T, name string, manifestContent string, files map[string]string) (string, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Write go.mod so packages load successfully
	safeName := strings.ReplaceAll(name, " ", "-")
	goMod := fmt.Sprintf("module example.com/temp/%s\n\ngo 1.21\n", safeName)
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

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
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
	want := `Component "test-comp" conforms; does not exceed declared authority`
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

func TestRunner_Check_InterfaceFileExclusion_WarnsInTextAndJSON(t *testing.T) {
	falseExpression := "!" + runtime.GOOS
	manifestContent := `
name: "excluded-interface"
interface_files: "api.go"
interface_files: "gated.go"
`
	files := map[string]string{
		"api.go":   "package main\n\nfunc Hello() {}\n",
		"gated.go": "//go:build " + falseExpression + "\n\npackage main\n\nfunc Gated() {}\n",
	}
	_, manifestPath := createTempComponent(t, "excluded-interface", manifestContent, files)
	runner := &app.Runner{
		Loader:   func(req goanalysis.LoadRequest) (facts.PackageFacts, error) { return goanalysis.LoadPackageFacts(req) },
		Analyzer: &mockAnalyzer{},
	}

	for _, args := range [][]string{{"check", manifestPath}, {"check", manifestPath, "--format=json"}} {
		var stdout, stderr bytes.Buffer
		if got := runner.Run(args, &stdout, &stderr); got != 0 {
			t.Fatalf("Run(%v) exit = %d, stderr = %q, stdout = %q", args, got, stderr.String(), stdout.String())
		}
		if !strings.Contains(stdout.String(), "INTERFACE_FILE_EXCLUDED") || !strings.Contains(stdout.String(), "gated.go") || !strings.Contains(stdout.String(), falseExpression) {
			t.Fatalf("Run(%v) stdout = %q, want exclusion warning", args, stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(%v) stderr = %q, want empty", args, stderr.String())
		}
	}

	var stdout, stderr bytes.Buffer
	if got := runner.Run([]string{"check", manifestPath, "--format=json"}, &stdout, &stderr); got != 0 {
		t.Fatalf("JSON Run() exit = %d, stderr = %q", got, stderr.String())
	}
	var rendered report.ConformanceReport
	if err := json.Unmarshal(stdout.Bytes(), &rendered); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; output = %q", err, stdout.String())
	}
	if len(rendered.Violations) != 0 || len(rendered.Warnings) != 1 || rendered.Warnings[0].Kind != report.InterfaceFileExcluded {
		t.Fatalf("rendered report = %#v, want one exclusion warning and no violations", rendered)
	}
}

func TestRunner_Check_InterfaceFilesAllExcludedFailsClosed(t *testing.T) {
	falseExpression := "!" + runtime.GOOS
	manifestContent := `
name: "all-excluded-interface"
interface_files: "gated.go"
`
	files := map[string]string{
		"keep.go":  "package main\n\nfunc Keep() {}\n",
		"gated.go": "//go:build " + falseExpression + "\n\npackage main\n\nfunc Gated() {}\n",
	}
	_, manifestPath := createTempComponent(t, "all-excluded-interface", manifestContent, files)
	runner := &app.Runner{
		Loader:   func(req goanalysis.LoadRequest) (facts.PackageFacts, error) { return goanalysis.LoadPackageFacts(req) },
		Analyzer: &mockAnalyzer{},
	}

	var stdout, stderr bytes.Buffer
	if got := runner.Run([]string{"check", manifestPath}, &stdout, &stderr); got != 2 {
		t.Fatalf("Run() exit = %d, want tool error 2; stdout = %q, stderr = %q", got, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "no interface file survives") || !strings.Contains(stderr.String(), "gated.go") || !strings.Contains(stderr.String(), falseExpression) {
		t.Fatalf("stdout = %q, stderr = %q, want fail-closed exclusion context", stdout.String(), stderr.String())
	}
}

func TestRunner_PassesManifestMembershipToLoader(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
members: "example.com/temp/loader-request"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "loader-request", manifestContent, files)

	var gotRequest goanalysis.LoadRequest
	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		gotRequest = req
		return goanalysis.LoadPackageFacts(req)
	}
	runner := &app.Runner{Loader: loader, Analyzer: &mockAnalyzer{}}

	var stdout, stderr bytes.Buffer
	if exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Run() returned %d. Stderr: %s", exitCode, stderr.String())
	}

	if gotRequest.ComponentRoot == "" ||
		!reflect.DeepEqual(gotRequest.Members, []string{"example.com/temp/loader-request"}) ||
		!reflect.DeepEqual(gotRequest.InterfaceFiles, []string{"api.go"}) {
		t.Fatalf("loader request = %+v, want manifest membership and interface context", gotRequest)
	}
}

func TestRunner_Check_DeclaredOutsideRootAndFR1Fallback(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/workspace\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	componentDir := filepath.Join(workspace, "component")
	outsideDir := filepath.Join(workspace, "outside")
	unrelatedDir := filepath.Join(componentDir, "unrelated")
	for _, dir := range []string{componentDir, outsideDir, unrelatedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(componentDir, "api.go"):       "package component\n\nfunc API() {}\n",
		filepath.Join(outsideDir, "outside.go"):     "package outside\n\nimport \"os\"\n\nfunc Outside() { _, _ = os.Getwd() }\n",
		filepath.Join(unrelatedDir, "unrelated.go"): "package unrelated\n\nfunc Unrelated() {}\n",
	}
	for file, content := range files {
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	manifestPath := filepath.Join(componentDir, "component.textproto")
	manifestWithMembers := `
name: "component"
interface_files: "api.go"
members: "example.com/workspace/outside"
`
	if err := os.WriteFile(manifestPath, []byte(manifestWithMembers), 0644); err != nil {
		t.Fatal(err)
	}

	var loaded facts.PackageFacts
	// Capture facts through a wrapper so the test observes the complete app path
	// while retaining the production loader and its package graph.
	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		got, err := goanalysis.LoadPackageFacts(req)
		if err == nil {
			loaded = got
		}
		return got, err
	}
	analyzer := &mockAnalyzer{findings: []capanalyzer.CapabilityFinding{{
		Package:    "os",
		Capability: "FILES",
		Class:      capanalyzer.TrueAuthority,
		CallPath: []capanalyzer.Frame{{
			Func: "example.com/workspace/outside.Outside",
			File: "../outside/outside.go",
			Line: 5,
		}},
	}}}
	runner := &app.Runner{Loader: loader, Analyzer: analyzer}

	var stdout, stderr bytes.Buffer
	if exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("declared run exit code = %d, want 1; stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "UNDECLARED_AUTHORITY") {
		t.Fatalf("declared run output = %q, want authority finding", stdout.String())
	}
	assertDeclaredOutsideFacts(t, loaded)
	wantDeclaredPackages := []string{"example.com/workspace/component", "example.com/workspace/outside"}
	if !reflect.DeepEqual(analyzer.calledWith.Packages, wantDeclaredPackages) {
		t.Fatalf("declared analyzer packages = %v, want %v", analyzer.calledWith.Packages, wantDeclaredPackages)
	}

	if err := os.WriteFile(manifestPath, []byte(`
name: "component"
interface_files: "api.go"
`), 0644); err != nil {
		t.Fatal(err)
	}
	analyzer.findings = nil
	stdout.Reset()
	stderr.Reset()
	if exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("FR1 fallback exit code = %d, want 0; stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	wantFR1Packages := []string{"example.com/workspace/component", "example.com/workspace/component/unrelated"}
	if !reflect.DeepEqual(analyzer.calledWith.Packages, wantFR1Packages) {
		t.Fatalf("FR1 analyzer packages = %v, want %v", analyzer.calledWith.Packages, wantFR1Packages)
	}
	for _, pkg := range loaded.Packages {
		if pkg.ImportPath == "example.com/workspace/outside" {
			t.Fatalf("FR1 facts unexpectedly retained outside package: %+v", loaded.Packages)
		}
	}
}

func assertDeclaredOutsideFacts(t *testing.T, loaded facts.PackageFacts) {
	t.Helper()
	if len(loaded.Packages) != 2 {
		t.Fatalf("declared facts packages = %+v, want component and outside only", loaded.Packages)
	}
	var outside facts.PackageFact
	for _, pkg := range loaded.Packages {
		if pkg.ImportPath == "example.com/workspace/outside" {
			outside = pkg
		}
		if pkg.ImportPath == "example.com/workspace/component/unrelated" {
			t.Fatalf("declared facts unexpectedly retained unrelated package: %+v", loaded.Packages)
		}
	}
	if !reflect.DeepEqual(outside.Imports, []string{"os"}) {
		t.Fatalf("outside imports = %v, want [os]", outside.Imports)
	}
	foundSymbol := false
	for _, symbol := range outside.ExportedSymbols {
		if symbol.Name == "example.com/workspace/outside.Outside" {
			foundSymbol = true
		}
	}
	if !foundSymbol {
		t.Fatalf("outside symbols = %+v, want Outside", outside.ExportedSymbols)
	}
	foundCall := false
	for _, edge := range loaded.CallEdges {
		if strings.HasPrefix(string(edge.Caller), "example.com/workspace/outside.Outside") && strings.HasPrefix(string(edge.Callee), "os.") {
			foundCall = true
		}
	}
	if !foundCall {
		t.Fatalf("outside call edges = %+v, want Outside -> os edge", loaded.CallEdges)
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

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
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

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
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

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
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
	tests := []struct {
		name            string
		manifestContent string
		wantErrSub      string
	}{
		{
			name: "missing_file",
			manifestContent: `
name: "test-comp"
interface_files: "missing.go"
`,
			wantErrSub: "missing.go",
		},
		{
			name: "escaping_file",
			manifestContent: `
name: "test-comp"
interface_files: "../escaping.go"
`,
			wantErrSub: "escapes the component root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"api.go": "package main\n\nfunc Hello() {}\n",
			}
			_, manifestPath := createTempComponent(t, "iface-val-"+tt.name, tt.manifestContent, files)

			loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
				return goanalysis.LoadPackageFacts(req)
			}

			analyzer := &mockAnalyzer{}
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
			if !strings.Contains(gotErr, tt.wantErrSub) {
				t.Errorf("stderr = %q, want to contain %q", gotErr, tt.wantErrSub)
			}

			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}

			// Assert that the analyzer was NOT called
			if len(analyzer.calledWith.Packages) != 0 {
				t.Errorf("analyzer was called despite interface validation error")
			}
		})
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

	loader := func(_ goanalysis.LoadRequest) (facts.PackageFacts, error) {
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

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
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

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
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

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunner_Check_Success_MultiPackage(t *testing.T) {
	manifestContent := `
name: "test-comp-multi"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go":           "package main\n\nfunc Hello() {}\n",
		"subpkg/helper.go": "package subpkg\n\nfunc Help() {}\n",
	}
	_, manifestPath := createTempComponent(t, "multipackage", manifestContent, files)

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
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

	wantPkgs := []string{"example.com/temp/multipackage", "example.com/temp/multipackage/subpkg"}
	if len(analyzer.calledWith.Packages) != 2 ||
		analyzer.calledWith.Packages[0] != wantPkgs[0] ||
		analyzer.calledWith.Packages[1] != wantPkgs[1] {
		t.Errorf("analyzer called with packages %v, want %v", analyzer.calledWith.Packages, wantPkgs)
	}
	if len(analyzer.calledWith.PruneAt) != 0 {
		t.Errorf("analyzer called with PruneAt %v, want empty", analyzer.calledWith.PruneAt)
	}
}

func TestRunner_Check_Success_WarningsOnly(t *testing.T) {
	// 1. Create a parent directory and a dependency in a sibling directory
	parentDir := t.TempDir()
	depDir := filepath.Join(parentDir, "unused-dep")
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatalf("failed to create dep dir: %v", err)
	}

	depManifest := `
name: "unused-dep"
interface_files: "api.go"
`
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte("module example.com/temp/unused-dep\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to write dep go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "component.textproto"), []byte(depManifest), 0644); err != nil {
		t.Fatalf("failed to write dep manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "api.go"), []byte("package unused_dep\n\nfunc Unused() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write dep api.go: %v", err)
	}

	// 2. Create the analyzed component in a sibling directory
	analyzedDir := filepath.Join(parentDir, "warnings-only")
	if err := os.MkdirAll(analyzedDir, 0755); err != nil {
		t.Fatalf("failed to create analyzed dir: %v", err)
	}

	manifestContent := `
name: "test-comp-warn"
interface_files: "api.go"
component_dependencies: {
  name: "unused-dep"
  manifest: "../unused-dep/component.textproto"
}
`
	if err := os.WriteFile(filepath.Join(analyzedDir, "go.mod"), []byte("module example.com/temp/warnings-only\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to write analyzed go.mod: %v", err)
	}
	manifestPath := filepath.Join(analyzedDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write analyzed manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(analyzedDir, "api.go"), []byte("package main\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write analyzed api.go: %v", err)
	}

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
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
	if !strings.Contains(got, "Warnings:") {
		t.Errorf("stdout = %q, want to contain 'Warnings:'", got)
	}
	if !strings.Contains(got, "declared component dependency \"unused-dep\" is unused") {
		t.Errorf("stdout = %q, want warning about unused dependency", got)
	}
	if strings.Contains(got, "Violations:") {
		t.Errorf("stdout = %q, should not contain violations", got)
	}
}

func TestRunner_Check_BoundaryWiring_PruningAndErrors(t *testing.T) {
	// 1. Setup sibling directories inside parentDir
	parentDir := t.TempDir()
	depDir := filepath.Join(parentDir, "dep-a")
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatalf("failed to create dep dir: %v", err)
	}

	depManifest := `
name: "dep-a"
interface_files: "api.go"
`
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte("module example.com/temp/dep-a\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to write dep go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "component.textproto"), []byte(depManifest), 0644); err != nil {
		t.Fatalf("failed to write dep manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "api.go"), []byte("package depa\n\nfunc FetchData() {}\nfunc HigherOrder(fn func()) {}\n"), 0644); err != nil {
		t.Fatalf("failed to write dep api.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "other.go"), []byte("package depa\n\nfunc UndeclaredFunc() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write dep other.go: %v", err)
	}

	// 2. Create the analyzed component
	analyzedDir := filepath.Join(parentDir, "analyzed")
	if err := os.MkdirAll(analyzedDir, 0755); err != nil {
		t.Fatalf("failed to create analyzed dir: %v", err)
	}

	manifestContent := `
name: "analyzed-comp"
interface_files: "api.go"
component_dependencies: {
  name: "dep-a"
  manifest: "../dep-a/component.textproto"
}
absorbed_dependencies: {
  import_path: "example.com/temp/absorbed-b"
}
`
	if err := os.WriteFile(filepath.Join(analyzedDir, "go.mod"), []byte("module example.com/temp/analyzed\n\ngo 1.21\n\nrequire example.com/temp/dep-a v0.0.0\nreplace example.com/temp/dep-a => ../dep-a\n"), 0644); err != nil {
		t.Fatalf("failed to write analyzed go.mod: %v", err)
	}
	manifestPath := filepath.Join(analyzedDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write analyzed manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(analyzedDir, "api.go"), []byte("package main\n\nimport \"example.com/temp/dep-a\"\n\nfunc myCallback() {}\n\nfunc Hello() {\n\tdepa.FetchData()\n\tdepa.UndeclaredFunc()\n\tdepa.HigherOrder(myCallback)\n}\n"), 0644); err != nil {
		t.Fatalf("failed to write analyzed api.go: %v", err)
	}

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	analyzer := &mockAnalyzer{}
	runner := &app.Runner{
		Loader:   loader,
		Analyzer: analyzer,
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("Run() returned %d, want 1. Stderr: %s\nStdout: %s", exitCode, stderr.String(), stdout.String())
	}

	// Assert that the boundary violations and warnings are emitted correctly in the output
	gotOut := stdout.String()
	if !strings.Contains(gotOut, "CALLS_UNDECLARED_INTERFACE") {
		t.Errorf("expected stdout to contain CALLS_UNDECLARED_INTERFACE, got: %s", gotOut)
	}
	if !strings.Contains(gotOut, "HIGHER_ORDER_BOUNDARY_CALL") {
		t.Errorf("expected stdout to contain HIGHER_ORDER_BOUNDARY_CALL, got: %s", gotOut)
	}
	if !strings.Contains(gotOut, `call from "example.com/temp/analyzed.Hello" to undeclared interface symbol "example.com/temp/dep-a.UndeclaredFunc" of dependency "dep-a"`) {
		t.Errorf("expected stdout to contain the undeclared call violation message, got: %s", gotOut)
	}
	if !strings.Contains(gotOut, `higher-order boundary call from "example.com/temp/analyzed.Hello" to "example.com/temp/dep-a.HigherOrder" of dependency "dep-a" passes function value`) {
		t.Errorf("expected stdout to contain the higher-order warning message, got: %s", gotOut)
	}

	// 3. Verify that PruneAt contains both dependency interface symbols and package init keys, sorted
	expectedPruneAt := []string{
		"example.com/temp/dep-a.FetchData",
		"example.com/temp/dep-a.HigherOrder",
		"func example.com/temp/dep-a.init",
	}

	if len(analyzer.calledWith.PruneAt) != len(expectedPruneAt) {
		t.Errorf("expected %d prune keys, got %v", len(expectedPruneAt), analyzer.calledWith.PruneAt)
	}

	for _, exp := range expectedPruneAt {
		found := false
		for _, got := range analyzer.calledWith.PruneAt {
			if string(got) == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected prune key %q not found in PruneAt: %v", exp, analyzer.calledWith.PruneAt)
		}
	}

	// Verify deterministic alphabetical sorting of PruneAt
	for i := 1; i < len(analyzer.calledWith.PruneAt); i++ {
		if analyzer.calledWith.PruneAt[i] < analyzer.calledWith.PruneAt[i-1] {
			t.Errorf("PruneAt is not sorted alphabetically: %v", analyzer.calledWith.PruneAt)
		}
	}

	// 4. Test error case: dependency resolution failure yields exit code 2
	badManifestContent := `
name: "analyzed-comp"
interface_files: "api.go"
component_dependencies: {
  name: "nonexistent-dep"
  manifest: "../nonexistent/component.textproto"
}
`
	badManifestPath := filepath.Join(analyzedDir, "bad_component.textproto")
	if err := os.WriteFile(badManifestPath, []byte(badManifestContent), 0644); err != nil {
		t.Fatalf("failed to write bad manifest: %v", err)
	}

	var stdoutBad, stderrBad bytes.Buffer
	badExitCode := runner.Run([]string{"check", badManifestPath}, &stdoutBad, &stderrBad)
	if badExitCode != 2 {
		t.Errorf("expected exit code 2 on resolution failure, got %d. Stderr: %s", badExitCode, stderrBad.String())
	}
	if !strings.Contains(stderrBad.String(), "failed to resolve dependency") {
		t.Errorf("expected stderr to contain error message about dependency resolution, got: %s", stderrBad.String())
	}
}
