//go:build integration

package main_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

var arccBin string

func TestMain(m *testing.M) {
	// Create a temp directory for the compiled arcc binary
	tmpDir, err := os.MkdirTemp("", "arcc-build-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binName := "arcc"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	arccBin = filepath.Join(tmpDir, binName)

	// Compile arcc using 'go build'
	cmd := exec.Command("go", "build", "-o", arccBin, "github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "failed to build arcc: %v\noutput:\n%s\n", err, string(out))
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// runArcc runs the compiled arcc binary as a subprocess and returns its stdout, stderr, and exit code.
func runArcc(args []string) (string, string, int) {
	cmd := exec.Command(arccBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Some other error
			exitCode = -1
		}
	}

	return stdout.String(), stderr.String(), exitCode
}

// createTempComponent creates a temporary component inside the 'go/' module tree to load successfully.
func createTempComponent(t *testing.T, name string, manifest string, files map[string]string) (string, string) {
	t.Helper()

	// Create inside the 'go/examples/csvtool' directory so we can import internal packages there if needed
	// Since tests run in 'go/cmd/arcc', 'go/examples/csvtool' is '../../examples/csvtool'
	tmpDir, err := os.MkdirTemp("../../examples/csvtool", "temp-integration-"+name+"-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	absTmpDir, err := filepath.Abs(tmpDir)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	absModuleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to get absolute path of module root: %v", err)
	}

	relDir, err := filepath.Rel(absModuleRoot, absTmpDir)
	if err != nil {
		t.Fatalf("failed to resolve relative path from go module root: %v", err)
	}

	importPath := "github.com/ono-sendai-labs/architectural-contracts/go/" + filepath.ToSlash(relDir)

	manifestPath := filepath.Join(absTmpDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	for filename, content := range files {
		content = strings.ReplaceAll(content, "{{IMPORT_PATH}}", importPath)
		filePath := filepath.Join(absTmpDir, filename)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", filename, err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", filename, err)
		}
	}

	return absTmpDir, manifestPath
}

// normalizeOutput replaces variable paths and file slashes with standard placeholders.
func normalizeOutput(s string, workspaceRoot, tempDir string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\\", "/")
	if tempDir != "" {
		tempDirSlash := filepath.ToSlash(tempDir)
		s = strings.ReplaceAll(s, tempDirSlash, "<TEMP_DIR>")
		s = strings.ReplaceAll(s, tempDir, "<TEMP_DIR>")

		tempDirBase := filepath.Base(tempDir)
		if tempDirBase != "." && tempDirBase != "/" && tempDirBase != "" {
			s = strings.ReplaceAll(s, tempDirBase, "<TEMP_DIR_NAME>")
		}
	}
	if workspaceRoot != "" {
		workspaceRootSlash := filepath.ToSlash(workspaceRoot)
		s = strings.ReplaceAll(s, workspaceRootSlash, "<WORKSPACE_ROOT>")
		s = strings.ReplaceAll(s, workspaceRoot, "<WORKSPACE_ROOT>")
	}
	return s
}

func TestIntegration_Toprow_Success(t *testing.T) {
	stdout, stderr, exitCode := runArcc([]string{"check", "../../examples/csvtool/toprow/component.textproto"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", exitCode, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	want := `Component "toprow" conforms / ambient-authority-free`
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, want)
	}
}

func TestIntegration_Csvfile_Success(t *testing.T) {
	stdout, stderr, exitCode := runArcc([]string{"check", "../../examples/csvtool/csvfile/component.textproto"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", exitCode, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	want := `Component "csvfile" conforms / ambient-authority-free`
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, want)
	}
}

func TestIntegration_Toprow_Failing_UndeclaredDependency(t *testing.T) {
	manifest := `
name: "toprow-fail"
interface_files: "toprow.go"
`
	files := map[string]string{
		"toprow.go": `package toprow
import "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv"
func Hello() {
	_, _ = parsecsv.Parse("")
}
`,
	}

	absTmpDir, manifestPath := createTempComponent(t, "toprow-fail", manifest, files)

	stdout, stderr, exitCode := runArcc([]string{"check", manifestPath})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s", exitCode, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	// Normalize
	wd, _ := os.Getwd()
	workspaceRoot, _ := filepath.Abs(filepath.Join(wd, "../../../"))
	normalizedStdout := normalizeOutput(stdout, workspaceRoot, absTmpDir)

	// We want to make sure it contains UNDECLARED_DEPENDENCY with package parsecsv
	if !strings.Contains(normalizedStdout, "UNDECLARED_DEPENDENCY") {
		t.Errorf("expected UNDECLARED_DEPENDENCY in stdout: %s", normalizedStdout)
	}
	if !strings.Contains(normalizedStdout, `imports undeclared dependency "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv"`) {
		t.Errorf("expected imports undeclared dependency parsecsv in stdout: %s", normalizedStdout)
	}
}

func TestIntegration_Absorbapp_Failing_UndeclaredAuthority(t *testing.T) {
	manifest := `
name: "absorbapp"
interface_files: "main.go"
`
	files := map[string]string{
		"main.go": `package main
import "os"
func Hello() {
	_, _ = os.ReadFile("test.csv")
}
`,
	}

	absTmpDir, manifestPath := createTempComponent(t, "absorbapp", manifest, files)

	stdout1, stderr1, exitCode1 := runArcc([]string{"check", manifestPath})

	if exitCode1 != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s", exitCode1, stderr1)
	}
	if stderr1 != "" {
		t.Errorf("expected empty stderr, got %q", stderr1)
	}

	stdout2, stderr2, exitCode2 := runArcc([]string{"check", manifestPath})

	if exitCode2 != 1 {
		t.Fatalf("expected exit code 1 on second run, got %d. Stderr: %s", exitCode2, stderr2)
	}
	if stderr2 != "" {
		t.Errorf("expected empty stderr on second run, got %q", stderr2)
	}

	wd, _ := os.Getwd()
	workspaceRoot, _ := filepath.Abs(filepath.Join(wd, "../../../"))
	normalizedStdout1 := normalizeOutput(stdout1, workspaceRoot, absTmpDir)
	normalizedStdout2 := normalizeOutput(stdout2, workspaceRoot, absTmpDir)

	if normalizedStdout1 != normalizedStdout2 {
		t.Errorf("outputs of repeated runs differ.\nRUN 1:\n%q\nRUN 2:\n%q", normalizedStdout1, normalizedStdout2)
	}

	// Verify against deterministic golden output to assert stability, finding order, and evidence call-paths.
	want := `Component: absorbapp

Violations:
- [UNDECLARED_AUTHORITY] use of undeclared authority "FILES" in package "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/<TEMP_DIR_NAME>"
  Evidence:
    - github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/<TEMP_DIR_NAME>.Hello at :0
    - os.ReadFile at main.go:4
`
	if strings.TrimSpace(normalizedStdout1) != strings.TrimSpace(want) {
		t.Errorf("normalized stdout does not match golden output.\nGOT:\n%q\nWANT:\n%q", normalizedStdout1, want)
	}
}

func TestIntegration_Malformed_Manifest_Exit2(t *testing.T) {
	// Syntactically invalid manifest
	manifest1 := `
name: "bad-cap"
interface_files: "api.go"
declared_authority: "BOGUS_CAPABILITY"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}

	_, manifestPath1 := createTempComponent(t, "bad-cap", manifest1, files)

	stdout, stderr, exitCode := runArcc([]string{"check", manifestPath1})

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d. Stdout: %s", exitCode, stdout)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "unknown capability") {
		t.Errorf("expected error diagnostics regarding unknown capability, got %q", stderr)
	}
}

func TestIntegration_Invalid_Textproto_Manifest_Exit2(t *testing.T) {
	manifest := `
this is completely invalid textproto data {{{
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}

	_, manifestPath := createTempComponent(t, "invalid-proto", manifest, files)

	stdout, stderr, exitCode := runArcc([]string{"check", manifestPath})

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d. Stdout: %s", exitCode, stdout)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if stderr == "" {
		t.Error("expected non-empty stderr diagnostics, got empty string")
	}
}

func TestIntegration_FormatJSON(t *testing.T) {
	manifest := `
name: "absorbapp"
interface_files: "main.go"
component_dependencies {
	name: "toprow"
	manifest: "../toprow/component.textproto"
}
`
	files := map[string]string{
		"main.go": `package main

import (
	"os"
	"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv"
)

func Hello() {
	_, _ = os.ReadFile("test.csv")
	_, _ = parsecsv.Parse("")
}
`,
	}

	absTmpDir, manifestPath := createTempComponent(t, "absorbapp-json", manifest, files)

	// 1. Run in JSON format
	stdoutJSON, stderrJSON, exitCodeJSON := runArcc([]string{"check", manifestPath, "--format=json"})

	if exitCodeJSON != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s", exitCodeJSON, stderrJSON)
	}
	if stderrJSON != "" {
		t.Errorf("expected empty stderr, got %q", stderrJSON)
	}

	var rep report.ConformanceReport
	if err := json.Unmarshal([]byte(stdoutJSON), &rep); err != nil {
		t.Fatalf("failed to decode JSON output: %v, stdout: %s", err, stdoutJSON)
	}

	if rep.Component != "absorbapp" {
		t.Errorf("rep.Component = %q, want 'absorbapp'", rep.Component)
	}

	var vAuth, vDep *report.Finding
	for idx, v := range rep.Violations {
		if v.Kind == report.UndeclaredAuthority {
			vAuth = &rep.Violations[idx]
		} else if v.Kind == report.UndeclaredDependency {
			vDep = &rep.Violations[idx]
		}
	}

	if vAuth == nil {
		t.Fatalf("expected to find UNDECLARED_AUTHORITY violation, got: %+v", rep.Violations)
	}
	if vDep == nil {
		t.Fatalf("expected to find UNDECLARED_DEPENDENCY violation, got: %+v", rep.Violations)
	}

	// Verify vAuth fields
	if !strings.Contains(vAuth.Message, `use of undeclared authority "FILES"`) {
		t.Errorf("vAuth.Message = %q, expected it to contain FILES", vAuth.Message)
	}
	if vAuth.Location.File != "" || vAuth.Location.Line != 0 {
		t.Errorf("vAuth.Location = %+v, expected empty (zero-value)", vAuth.Location)
	}
	if len(vAuth.Evidence) != 2 {
		t.Fatalf("expected 2 evidence entries for UNDECLARED_AUTHORITY, got %d: %v", len(vAuth.Evidence), vAuth.Evidence)
	}
	if !strings.Contains(vAuth.Evidence[0], ".Hello at :0") {
		t.Errorf("vAuth.Evidence[0] = %q, expected it to contain '.Hello at :0'", vAuth.Evidence[0])
	}
	if !strings.Contains(vAuth.Evidence[1], "os.ReadFile at main.go:9") {
		t.Errorf("vAuth.Evidence[1] = %q, expected it to contain 'os.ReadFile at main.go:9'", vAuth.Evidence[1])
	}

	// Verify vDep fields
	if !strings.Contains(vDep.Message, `imports undeclared dependency "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv"`) {
		t.Errorf("vDep.Message = %q, expected parsecsv undeclared dependency", vDep.Message)
	}
	if vDep.Location.File != "main.go" {
		t.Errorf("vDep.Location.File = %q, expected 'main.go'", vDep.Location.File)
	}
	if vDep.Location.Line != 0 {
		t.Errorf("vDep.Location.Line = %d, expected 0", vDep.Location.Line)
	}

	// Verify rep.Warnings fields
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %+v", len(rep.Warnings), rep.Warnings)
	}
	w := rep.Warnings[0]
	if w.Kind != report.UnusedDependency {
		t.Errorf("w.Kind = %q, expected UNUSED_DEPENDENCY", w.Kind)
	}
	if !strings.Contains(w.Message, `declared component dependency "toprow" is unused`) {
		t.Errorf("w.Message = %q, expected 'declared component dependency \"toprow\" is unused'", w.Message)
	}
	if w.Location.File != "" || w.Location.Line != 0 {
		t.Errorf("w.Location = %+v, expected empty for UNUSED_DEPENDENCY warning", w.Location)
	}

	// 2. Run in Default/Text format and compare semantics
	stdoutText, stderrText, exitCodeText := runArcc([]string{"check", manifestPath})
	if exitCodeText != 1 {
		t.Fatalf("expected text mode exit code 1, got %d. Stderr: %s", exitCodeText, stderrText)
	}
	if stderrText != "" {
		t.Errorf("expected empty text mode stderr, got %q", stderrText)
	}

	wd, _ := os.Getwd()
	workspaceRoot, _ := filepath.Abs(filepath.Join(wd, "../../../"))
	normalizedStdoutText := normalizeOutput(stdoutText, workspaceRoot, absTmpDir)

	// Ensure text mode has exact matching information
	if !strings.Contains(normalizedStdoutText, `use of undeclared authority "FILES"`) {
		t.Errorf("text output missing FILES violation: %s", normalizedStdoutText)
	}
	if !strings.Contains(normalizedStdoutText, "os.ReadFile at main.go:9") {
		t.Errorf("text output missing evidence path: %s", normalizedStdoutText)
	}
	if !strings.Contains(normalizedStdoutText, `imports undeclared dependency "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv"`) {
		t.Errorf("text output missing parsecsv undeclared dependency: %s", normalizedStdoutText)
	}
	if !strings.Contains(normalizedStdoutText, "at main.go:0") {
		t.Errorf("text output missing location info: %s", normalizedStdoutText)
	}
	if !strings.Contains(normalizedStdoutText, `declared component dependency "toprow" is unused`) {
		t.Errorf("text output missing unused dependency warning: %s", normalizedStdoutText)
	}
}
