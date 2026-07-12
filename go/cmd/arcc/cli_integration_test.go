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
	defer os.RemoveAll(tmpDir)

	binName := "arcc"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	arccBin = filepath.Join(tmpDir, binName)

	// Compile arcc using 'go build'
	cmd := exec.Command("go", "build", "-o", arccBin, "github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build arcc: %v\noutput:\n%s\n", err, string(out))
		os.Exit(1)
	}

	os.Exit(m.Run())
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

	// Create inside the 'go' module directory
	// Since tests run in 'go/cmd/arcc', 'go/' is '../../'
	tmpDir, err := os.MkdirTemp("../..", "temp-integration-"+name+"-*")
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
	s = strings.ReplaceAll(s, "\\", "/")
	if tempDir != "" {
		tempDirSlash := filepath.ToSlash(tempDir)
		s = strings.ReplaceAll(s, tempDirSlash, "<TEMP_DIR>")
		s = strings.ReplaceAll(s, tempDir, "<TEMP_DIR>")
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
import "github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
func Hello() {
	_ = report.Finding{}
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

	// We want to make sure it contains UNDECLARED_DEPENDENCY with package report
	if !strings.Contains(normalizedStdout, "UNDECLARED_DEPENDENCY") {
		t.Errorf("expected UNDECLARED_DEPENDENCY in stdout: %s", normalizedStdout)
	}
	if !strings.Contains(normalizedStdout, `imports undeclared dependency "github.com/ono-sendai-labs/architectural-contracts/go/internal/report"`) {
		t.Errorf("expected imports undeclared dependency report in stdout: %s", normalizedStdout)
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

	stdout, stderr, exitCode := runArcc([]string{"check", manifestPath})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s", exitCode, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	wd, _ := os.Getwd()
	workspaceRoot, _ := filepath.Abs(filepath.Join(wd, "../../../"))
	normalizedStdout := normalizeOutput(stdout, workspaceRoot, absTmpDir)

	// Verify UNDECLARED_AUTHORITY and FILES capability are reported
	if !strings.Contains(normalizedStdout, "UNDECLARED_AUTHORITY") {
		t.Errorf("expected UNDECLARED_AUTHORITY in stdout: %s", normalizedStdout)
	}
	if !strings.Contains(normalizedStdout, `use of undeclared authority "FILES"`) {
		t.Errorf("expected FILES capability in stdout: %s", normalizedStdout)
	}
	// Verify Capslock path evidence exists
	if !strings.Contains(normalizedStdout, "Evidence:") {
		t.Errorf("expected Evidence in stdout: %s", normalizedStdout)
	}
	if !strings.Contains(normalizedStdout, "os.ReadFile") {
		t.Errorf("expected os.ReadFile in Evidence path of stdout: %s", normalizedStdout)
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

func TestIntegration_FormatJSON(t *testing.T) {
	stdout, stderr, exitCode := runArcc([]string{"check", "../../examples/csvtool/toprow/component.textproto", "--format=json"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", exitCode, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	var rep report.ConformanceReport
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("failed to decode JSON output: %v, stdout: %s", err, stdout)
	}

	if rep.Component != "toprow" {
		t.Errorf("rep.Component = %q, want 'toprow'", rep.Component)
	}
	if len(rep.Violations) != 0 {
		t.Errorf("violations count = %d, want 0", len(rep.Violations))
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("expected JSON output to end with a trailing newline")
	}
}
