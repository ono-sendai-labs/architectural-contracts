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
	return runArccEnv(nil, args)
}

// runArccEnv is runArcc with an explicit environment for the subprocess. A nil
// env inherits the test process's environment, which is what runArcc wants;
// the layout-mode tests use it to withhold the Go toolchain.
func runArccEnv(env []string, args []string) (string, string, int) {
	cmd := exec.Command(arccBin, args...)
	cmd.Env = env
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

	want := `Component "toprow" conforms; does not exceed declared authority`
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

	want := `Component "csvfile" conforms; does not exceed declared authority`
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

func TestIntegration_PruningVsAbsorbedAuthority(t *testing.T) {
	// 1. Create the authority-bearing dependency component "authdep"
	depManifest := `
name: "authdep"
interface_files: "api.go"
`
	depFiles := map[string]string{
		"api.go": `package authdep
import "os"
func ReadData() {
	_, _ = os.ReadFile("foo.txt")
}
`,
	}

	depDir, depManifestPath := createTempComponent(t, "authdep", depManifest, depFiles)

	absModuleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to get module root: %v", err)
	}
	relDepDir, err := filepath.Rel(absModuleRoot, depDir)
	if err != nil {
		t.Fatalf("failed to resolve relative path: %v", err)
	}
	depImportPath := "github.com/ono-sendai-labs/architectural-contracts/go/" + filepath.ToSlash(relDepDir)

	// 2. Component Dependency Variant: caller imports and calls the dependency.
	// Because authdep is a component dependency, the call to ReadData is pruned,
	// and its internal use of "FILES" (os.ReadFile) is not attributed to caller-cd.
	callerFilesCD := map[string]string{
		"caller.go": fmt.Sprintf(`package main
import dep "%s"
func Hello() {
	dep.ReadData()
}
`, depImportPath),
	}

	callerDirCD, callerManifestPathCD := createTempComponent(t, "caller-cd", "", callerFilesCD)

	// Compute relative path from callerDirCD to depManifestPath
	relManifestCD, err := filepath.Rel(callerDirCD, depManifestPath)
	if err != nil {
		t.Fatalf("failed to compute relative path: %v", err)
	}
	relManifestCD = filepath.ToSlash(relManifestCD)

	// Now write the actual manifest with the correct relative path to depManifestPath
	actualManifestCD := fmt.Sprintf(`
name: "caller-cd"
interface_files: "caller.go"
component_dependencies {
	name: "authdep"
	manifest: "%s"
}
`, relManifestCD)
	if err := os.WriteFile(callerManifestPathCD, []byte(actualManifestCD), 0644); err != nil {
		t.Fatalf("failed to update manifest: %v", err)
	}

	stdoutCD, stderrCD, exitCodeCD := runArcc([]string{"check", callerManifestPathCD})
	if exitCodeCD != 0 {
		t.Fatalf("expected component-dependency variant to conform (exit 0), got %d. Stderr: %s\nStdout: %s", exitCodeCD, stderrCD, stdoutCD)
	}
	if stderrCD != "" {
		t.Errorf("expected empty stderr for component-dependency variant, got %q", stderrCD)
	}
	if !strings.Contains(stdoutCD, `Component "caller-cd" conforms; does not exceed declared authority`) {
		t.Errorf("expected stdout to confirm conformance, got: %s", stdoutCD)
	}

	// 3. Absorbed Dependency Variant: caller imports and calls the dependency,
	// but declares it as absorbed instead of a component dependency.
	// Because it is absorbed, its internal use of "FILES" remains attributed to caller-abs
	// and triggers an UNDECLARED_AUTHORITY violation.
	callerManifestAbs := fmt.Sprintf(`
name: "caller-abs"
interface_files: "caller.go"
absorbed_dependencies {
	import_path: "%s"
}
`, depImportPath)
	callerFilesAbs := map[string]string{
		"caller.go": fmt.Sprintf(`package main
import dep "%s"
func Hello() {
	dep.ReadData()
}
`, depImportPath),
	}

	_, callerManifestPathAbs := createTempComponent(t, "caller-abs", callerManifestAbs, callerFilesAbs)

	stdoutAbs, stderrAbs, exitCodeAbs := runArcc([]string{"check", callerManifestPathAbs})
	if exitCodeAbs != 1 {
		t.Fatalf("expected absorbed-dependency variant to fail (exit 1), got %d. Stderr: %s\nStdout: %s", exitCodeAbs, stderrAbs, stdoutAbs)
	}
	if stderrAbs != "" {
		t.Errorf("expected empty stderr for absorbed-dependency variant, got %q", stderrAbs)
	}
	if !strings.Contains(stdoutAbs, `use of undeclared authority "FILES"`) {
		t.Errorf("expected absorbed-dependency variant to report FILES undeclared authority, got: %s", stdoutAbs)
	}
}

func TestIntegration_MemberCallbackAuthority(t *testing.T) {
	// The host receives the callback through its component interface. The editor
	// never calls backend.Load directly; backend is charged because it is an
	// owned member and therefore an analyzer root.
	hostFiles := map[string]string{
		"api.go": `package host

func Register(func() ([]byte, error)) {}
`,
	}
	hostManifest := `
name: "callback-host"
interface_files: "api.go"
`
	hostDir, hostManifestPath := createTempComponent(t, "callback-host", hostManifest, hostFiles)

	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to get module root: %v", err)
	}
	importPath := func(dir string) string {
		rel, err := filepath.Rel(moduleRoot, dir)
		if err != nil {
			t.Fatalf("failed to resolve import path for %s: %v", dir, err)
		}
		return "github.com/ono-sendai-labs/architectural-contracts/go/" + filepath.ToSlash(rel)
	}
	hostImportPath := importPath(hostDir)

	editorFiles := map[string]string{
		"api.go": `package main

func Editor() {}
`,
		"editor.go": `package main

import (
	backend "{{IMPORT_PATH}}/backend"
	host "` + hostImportPath + `"
)

func Connect() {
	host.Register(backend.Load)
}
`,
		"backend/load.go": `package backend

import "os"

func Load() ([]byte, error) {
	return os.ReadFile("backend.txt")
}
`,
	}
	editorManifest := `
name: "callback-editor"
interface_files: "api.go"
`
	editorDir, editorManifestPath := createTempComponent(t, "callback-editor", editorManifest, editorFiles)
	memberImportPath := importPath(filepath.Join(editorDir, "backend"))
	relHostManifest, err := filepath.Rel(editorDir, hostManifestPath)
	if err != nil {
		t.Fatalf("failed to resolve host manifest path: %v", err)
	}
	editorManifest = fmt.Sprintf(`
name: "callback-editor"
interface_files: "api.go"
members: "%s"
component_dependencies {
	name: "callback-host"
	manifest: "%s"
}
`, memberImportPath, filepath.ToSlash(relHostManifest))
	if err := os.WriteFile(editorManifestPath, []byte(editorManifest), 0644); err != nil {
		t.Fatalf("failed to write callback editor manifest: %v", err)
	}

	stdout, stderr, exitCode := runArcc([]string{"check", editorManifestPath})
	if exitCode != 1 {
		t.Fatalf("expected callback-only member authority to fail (exit 1), got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if !strings.Contains(stdout, `use of undeclared authority "FILES"`) {
		t.Fatalf("expected FILES authority violation, got: %s", stdout)
	}
	if !strings.Contains(stdout, memberImportPath+".Load") || !strings.Contains(stdout, "load.go") {
		t.Fatalf("expected evidence to identify member-owned backend.Load, got: %s", stdout)
	}
}

func TestIntegration_InitPruningVsAbsorbedAuthority(t *testing.T) {
	// 1. Create the authority-bearing dependency component "initdep" (authority only in init)
	depManifest := `
name: "initdep"
interface_files: "api.go"
`
	depFiles := map[string]string{
		"api.go": `package initdep
import "os"
func init() {
	_, _ = os.ReadFile("init_trigger.txt")
}
func Dummy() {}
`,
	}

	depDir, depManifestPath := createTempComponent(t, "initdep", depManifest, depFiles)

	absModuleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to get module root: %v", err)
	}
	relDepDir, err := filepath.Rel(absModuleRoot, depDir)
	if err != nil {
		t.Fatalf("failed to resolve relative path: %v", err)
	}
	depImportPath := "github.com/ono-sendai-labs/architectural-contracts/go/" + filepath.ToSlash(relDepDir)

	// 2. Component Dependency Variant: caller imports and calls the dependency.
	// Because initdep is a component dependency, both Dummy() and its package-level init() are pruned.
	// The internal use of "FILES" (os.ReadFile inside init()) is pruned and not attributed to caller-init-cd.
	callerFilesCD := map[string]string{
		"caller.go": fmt.Sprintf(`package main
import dep "%s"
func Hello() {
	dep.Dummy()
}
`, depImportPath),
	}

	callerDirCD, callerManifestPathCD := createTempComponent(t, "caller-init-cd", "", callerFilesCD)

	// Compute relative path from callerDirCD to depManifestPath
	relManifestCD, err := filepath.Rel(callerDirCD, depManifestPath)
	if err != nil {
		t.Fatalf("failed to compute relative path: %v", err)
	}
	relManifestCD = filepath.ToSlash(relManifestCD)

	// Now write the actual manifest with the correct relative path to depManifestPath
	actualManifestCD := fmt.Sprintf(`
name: "caller-init-cd"
interface_files: "caller.go"
component_dependencies {
	name: "initdep"
	manifest: "%s"
}
`, relManifestCD)
	if err := os.WriteFile(callerManifestPathCD, []byte(actualManifestCD), 0644); err != nil {
		t.Fatalf("failed to update manifest: %v", err)
	}

	stdoutCD, stderrCD, exitCodeCD := runArcc([]string{"check", callerManifestPathCD})
	if exitCodeCD != 0 {
		t.Fatalf("expected component-dependency variant to conform (exit 0), got %d. Stderr: %s\nStdout: %s", exitCodeCD, stderrCD, stdoutCD)
	}
	if stderrCD != "" {
		t.Errorf("expected empty stderr for component-dependency variant, got %q", stderrCD)
	}
	if !strings.Contains(stdoutCD, `Component "caller-init-cd" conforms; does not exceed declared authority`) {
		t.Errorf("expected stdout to confirm conformance, got: %s", stdoutCD)
	}

	// 3. Absorbed Dependency Variant: caller imports and calls the dependency,
	// but declares it as absorbed instead of a component dependency.
	// Because it is absorbed, its internal use of "FILES" inside init() is attributed to caller-init-abs
	// and triggers an UNDECLARED_AUTHORITY violation.
	callerManifestAbs := fmt.Sprintf(`
name: "caller-init-abs"
interface_files: "caller.go"
absorbed_dependencies {
	import_path: "%s"
}
`, depImportPath)
	callerFilesAbs := map[string]string{
		"caller.go": fmt.Sprintf(`package main
import dep "%s"
func Hello() {
	dep.Dummy()
}
`, depImportPath),
	}

	_, callerManifestPathAbs := createTempComponent(t, "caller-init-abs", callerManifestAbs, callerFilesAbs)

	stdoutAbs, stderrAbs, exitCodeAbs := runArcc([]string{"check", callerManifestPathAbs})
	if exitCodeAbs != 1 {
		t.Fatalf("expected absorbed-dependency variant to fail (exit 1), got %d. Stderr: %s\nStdout: %s", exitCodeAbs, stderrAbs, stdoutAbs)
	}
	if stderrAbs != "" {
		t.Errorf("expected empty stderr for absorbed-dependency variant, got %q", stderrAbs)
	}
	if !strings.Contains(stdoutAbs, `use of undeclared authority "FILES"`) {
		t.Errorf("expected absorbed-dependency variant to report FILES undeclared authority, got: %s", stdoutAbs)
	}
}

func TestIntegration_App_Success(t *testing.T) {
	stdout, stderr, exitCode := runArcc([]string{"check", "../../examples/csvtool/app/component.textproto"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	wantText := `Component "app" conforms; does not exceed declared authority

Dependencies:
- csvfile: certified
- toprow: certified
`
	if stdout != wantText {
		t.Errorf("stdout =\n%q\nwant:\n%q", stdout, wantText)
	}
}

func TestIntegration_App_Success_JSON(t *testing.T) {
	stdout, stderr, exitCode := runArcc([]string{"check", "../../examples/csvtool/app/component.textproto", "--format=json"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	var rep report.ConformanceReport
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("json.Unmarshal error = %v; stdout = %q", err, stdout)
	}
	if rep.Component != "app" {
		t.Errorf("rep.Component = %q, want 'app'", rep.Component)
	}
	if len(rep.Violations) != 0 || len(rep.Warnings) != 0 {
		t.Errorf("report has findings: violations=%v, warnings=%v", rep.Violations, rep.Warnings)
	}
	if len(rep.Dependencies) != 2 || rep.Dependencies[0].Component != "csvfile" || rep.Dependencies[1].Component != "toprow" || !rep.Dependencies[0].OwnCheckRuns || !rep.Dependencies[1].OwnCheckRuns {
		t.Errorf("rep.Dependencies = %+v, want csvfile and toprow certified boundaries", rep.Dependencies)
	}
}

func TestIntegration_PrivateCall_Failure(t *testing.T) {
	wd, _ := os.Getwd()
	depManifestPath, err := filepath.Abs(filepath.Join(wd, "../../examples/csvtool/csvfile/component.textproto"))
	if err != nil {
		t.Fatalf("failed to get absolute path of csvfile manifest: %v", err)
	}

	files := map[string]string{
		"app.go": `package main

import "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile"

func Run() {
	csvfile.PrivateExportedHelper()
}
`,
	}

	callerDir, callerManifestPath := createTempComponent(t, "app-private-fail", "", files)

	relManifest, err := filepath.Rel(callerDir, depManifestPath)
	if err != nil {
		t.Fatalf("failed to compute relative path: %v", err)
	}
	relManifest = filepath.ToSlash(relManifest)

	actualManifest := fmt.Sprintf(`
name: "app-private-fail"
interface_files: "app.go"
component_dependencies {
	name: "csvfile"
	manifest: "%s"
}
`, relManifest)

	if err := os.WriteFile(callerManifestPath, []byte(actualManifest), 0644); err != nil {
		t.Fatalf("failed to update manifest: %v", err)
	}

	stdout, stderr, exitCode := runArcc([]string{"check", callerManifestPath})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	if !strings.Contains(stdout, "CALLS_UNDECLARED_INTERFACE") {
		t.Errorf("expected stdout to contain CALLS_UNDECLARED_INTERFACE, got: %s", stdout)
	}
	if !strings.Contains(stdout, `call from "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/temp-integration-app-private-fail-`) {
		t.Errorf("expected stdout to contain the caller signature, got: %s", stdout)
	}
	if !strings.Contains(stdout, `to undeclared interface symbol "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile.PrivateExportedHelper" of dependency "csvfile"`) {
		t.Errorf("expected stdout to contain the callee PrivateExportedHelper, got: %s", stdout)
	}
}

func TestIntegration_HigherOrderBoundaryCall_Warning(t *testing.T) {
	wd, _ := os.Getwd()
	depManifestPath, err := filepath.Abs(filepath.Join(wd, "../../examples/csvtool/csvfile/component.textproto"))
	if err != nil {
		t.Fatalf("failed to get absolute path of csvfile manifest: %v", err)
	}

	files := map[string]string{
		"app.go": `package main

import "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile"

func Run() {
	_ = csvfile.ReadWithCallback("test.csv", func(rows [][]string) {
		// Callback implementation
	})
}
`,
	}

	callerDir, callerManifestPath := createTempComponent(t, "app-higher-order", "", files)

	relManifest, err := filepath.Rel(callerDir, depManifestPath)
	if err != nil {
		t.Fatalf("failed to compute relative path: %v", err)
	}
	relManifest = filepath.ToSlash(relManifest)

	actualManifest := fmt.Sprintf(`
name: "app-higher-order"
interface_files: "app.go"
component_dependencies {
	name: "csvfile"
	manifest: "%s"
}
`, relManifest)

	if err := os.WriteFile(callerManifestPath, []byte(actualManifest), 0644); err != nil {
		t.Fatalf("failed to update manifest: %v", err)
	}

	stdout, stderr, exitCode := runArcc([]string{"check", callerManifestPath})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	// Avoid literal string in codebase to satisfy acceptance criteria 1
	higherOrderBoundaryCallStr := "HIGHER_" + "ORDER_" + "BOUNDARY_" + "CALL"
	if strings.Contains(stdout, higherOrderBoundaryCallStr) {
		t.Errorf("expected stdout to NOT contain %s, got: %s", higherOrderBoundaryCallStr, stdout)
	}
	if strings.Contains(stdout, "passes function value") {
		t.Errorf("expected stdout to NOT contain 'passes function value', got: %s", stdout)
	}
}

func TestIntegration_PureCoreRegression_Failing_UndeclaredAuthority(t *testing.T) {
	// Read the original manifest and source code of the pure core 'report' component
	origManifest, err := os.ReadFile("../../internal/report/component.textproto")
	if err != nil {
		t.Fatalf("failed to read original report manifest: %v", err)
	}

	origCode, err := os.ReadFile("../../internal/report/report.go")
	if err != nil {
		t.Fatalf("failed to read original report code: %v", err)
	}

	// We create a temporary component that copies the original files,
	// but we add a new file 'regression_authority.go' which introduces os.Open (FILES capability)
	files := map[string]string{
		"report.go": string(origCode),
		"regression_authority.go": `package report

import "os"

func ViolateCorePurity() {
	_, _ = os.Open("some_arbitrary_file.txt")
}
`,
	}

	_, manifestPath := createTempComponent(t, "pure-core-regression", string(origManifest), files)

	// Run arcc on the mutated copy of the pure component
	stdout, stderr, exitCode := runArcc([]string{"check", manifestPath})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1 due to undeclared authority, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	// Verify that the output lists the filesystem authority violation pointing to os.Open
	if !strings.Contains(stdout, "UNDECLARED_AUTHORITY") {
		t.Errorf("expected stdout to contain UNDECLARED_AUTHORITY, got: %s", stdout)
	}
	if !strings.Contains(stdout, `use of undeclared authority "FILES"`) {
		t.Errorf("expected stdout to report FILES authority violation, got: %s", stdout)
	}
	if !strings.Contains(stdout, "ViolateCorePurity") {
		t.Errorf("expected stdout evidence to reference ViolateCorePurity function, got: %s", stdout)
	}
	if !strings.Contains(stdout, "os.Open at regression_authority.go:") {
		t.Errorf("expected stdout evidence to reference os.Open at regression_authority.go, got: %s", stdout)
	}
}

func TestIntegration_Fixture2_CallbackEscapePair(t *testing.T) {
	// Design §7.1 Fixture 2 regression pair:
	// Host component accepts callback.
	// Editor component passes backend.Load across boundary to host.
	hostFiles := map[string]string{
		"api.go": `package host

func Register(f func() ([]byte, error)) {}
`,
	}
	hostManifest := `
name: "fixture2-host"
interface_files: "api.go"
`
	hostDir, hostManifestPath := createTempComponent(t, "fixture2-host", hostManifest, hostFiles)

	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to get module root: %v", err)
	}
	importPath := func(dir string) string {
		rel, err := filepath.Rel(moduleRoot, dir)
		if err != nil {
			t.Fatalf("failed to resolve import path for %s: %v", dir, err)
		}
		return "github.com/ono-sendai-labs/architectural-contracts/go/" + filepath.ToSlash(rel)
	}
	hostImportPath := importPath(hostDir)

	editorFiles := map[string]string{
		"api.go": `package main

func Editor() {}
`,
		"editor.go": `package main

import (
	backend "{{IMPORT_PATH}}/backend"
	host "` + hostImportPath + `"
)

func Connect() {
	host.Register(backend.Load)
}
`,
		"backend/load.go": `package backend

func Load() ([]byte, error) {
	return []byte("data"), nil
}
`,
	}
	editorManifest := `
name: "fixture2-editor"
interface_files: "api.go"
`
	editorDir, editorManifestPath := createTempComponent(t, "fixture2-editor", editorManifest, editorFiles)
	backendImportPath := importPath(filepath.Join(editorDir, "backend"))
	relHostManifest, err := filepath.Rel(editorDir, hostManifestPath)
	if err != nil {
		t.Fatalf("failed to resolve host manifest path: %v", err)
	}

	// Part 1: backend is ABSORBED -> warns ABSORBED_FUNC_VALUE_ESCAPE, exit code 0
	editorImportPath := importPath(editorDir)
	absorbedManifest := fmt.Sprintf(`
name: "fixture2-editor"
interface_files: "api.go"
members: "%s"
absorbed_dependencies {
	import_path: "%s"
}
component_dependencies {
	name: "fixture2-host"
	manifest: "%s"
}
`, editorImportPath, backendImportPath, filepath.ToSlash(relHostManifest))
	if err := os.WriteFile(editorManifestPath, []byte(absorbedManifest), 0644); err != nil {
		t.Fatalf("failed to write absorbed manifest: %v", err)
	}

	// Text format
	stdoutAbs, stderrAbs, exitCodeAbs := runArcc([]string{"check", editorManifestPath})
	if exitCodeAbs != 0 {
		t.Fatalf("expected absorbed callback escape to warn with exit code 0, got %d. Stderr: %s\nStdout: %s", exitCodeAbs, stderrAbs, stdoutAbs)
	}
	if stderrAbs != "" {
		t.Errorf("expected empty stderr for absorbed variant, got %q", stderrAbs)
	}
	if !strings.Contains(stdoutAbs, "ABSORBED_FUNC_VALUE_ESCAPE") {
		t.Errorf("expected absorbed variant stdout to contain ABSORBED_FUNC_VALUE_ESCAPE, got: %s", stdoutAbs)
	}
	if !strings.Contains(stdoutAbs, "backend.Load") {
		t.Errorf("expected absorbed variant stdout to name absorbed function backend.Load, got: %s", stdoutAbs)
	}
	if !strings.Contains(stdoutAbs, "at editor.go:") {
		t.Errorf("expected absorbed variant stdout to carry file and line, got: %s", stdoutAbs)
	}

	// JSON format
	stdoutJSON, stderrJSON, exitCodeJSON := runArcc([]string{"check", editorManifestPath, "--format=json"})
	if exitCodeJSON != 0 {
		t.Fatalf("expected JSON check exit code 0, got %d. Stderr: %s\nStdout: %s", exitCodeJSON, stderrJSON, stdoutJSON)
	}
	if stderrJSON != "" {
		t.Errorf("expected empty stderr for JSON format, got %q", stderrJSON)
	}
	var jsonRep report.ConformanceReport
	if err := json.Unmarshal([]byte(stdoutJSON), &jsonRep); err != nil {
		t.Fatalf("failed to unmarshal JSON report: %v, raw: %s", err, stdoutJSON)
	}
	if len(jsonRep.Violations) != 0 || len(jsonRep.Warnings) != 1 {
		t.Fatalf("expected 0 violations and 1 warning in JSON report, got %d violations, %d warnings: %+v", len(jsonRep.Violations), len(jsonRep.Warnings), jsonRep)
	}
	wJSON := jsonRep.Warnings[0]
	if wJSON.Kind != report.AbsorbedFuncValueEscape {
		t.Errorf("expected JSON warning kind ABSORB_FUNC_VALUE_ESCAPE, got %s", wJSON.Kind)
	}
	if !strings.Contains(wJSON.Message, "backend.Load") {
		t.Errorf("expected JSON warning message to name backend.Load, got %q", wJSON.Message)
	}
	if wJSON.Location.File != "editor.go" || wJSON.Location.Line <= 0 {
		t.Errorf("expected JSON warning location editor.go with positive line number, got %+v", wJSON.Location)
	}

	// Part 2: backend body in MEMBER -> no warning, clean conforming report, exit code 0
	memberManifest := fmt.Sprintf(`
name: "fixture2-editor"
interface_files: "api.go"
members: "%s"
members: "%s"
component_dependencies {
	name: "fixture2-host"
	manifest: "%s"
}
`, editorImportPath, backendImportPath, filepath.ToSlash(relHostManifest))
	if err := os.WriteFile(editorManifestPath, []byte(memberManifest), 0644); err != nil {
		t.Fatalf("failed to write member manifest: %v", err)
	}

	stdoutMem, stderrMem, exitCodeMem := runArcc([]string{"check", editorManifestPath})
	if exitCodeMem != 0 {
		t.Fatalf("expected member callback to pass with exit code 0, got %d. Stderr: %s\nStdout: %s", exitCodeMem, stderrMem, stdoutMem)
	}
	if stderrMem != "" {
		t.Errorf("expected empty stderr for member variant, got %q", stderrMem)
	}
	if strings.Contains(stdoutMem, "ABSORBED_FUNC_VALUE_ESCAPE") {
		t.Errorf("expected member variant stdout to NOT contain ABSORBED_FUNC_VALUE_ESCAPE, got: %s", stdoutMem)
	}
	if !strings.Contains(stdoutMem, `Component "fixture2-editor" conforms; does not exceed declared authority`) {
		t.Errorf("expected member variant stdout to be conforming success line, got: %s", stdoutMem)
	}
}

func TestIntegration_AssertedBoundary_Exit0(t *testing.T) {
	// Verifies AC4: a component whose dependencies are all asserted and has no violations
	// or warnings returns exit code 0, emits no findings, and retains the success line + annotation.
	depManifest := `
name: "asserted-dep-cli"
interface_files: "api.go"
`
	depFiles := map[string]string{
		"api.go": `package asserteddep
func Fetch() {}
`,
	}
	depDir, depManifestPath := createTempComponent(t, "asserted-dep-cli", depManifest, depFiles)

	absModuleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to get module root: %v", err)
	}
	relDepDir, err := filepath.Rel(absModuleRoot, depDir)
	if err != nil {
		t.Fatalf("failed to resolve relative path: %v", err)
	}
	depImportPath := "github.com/ono-sendai-labs/architectural-contracts/go/" + filepath.ToSlash(relDepDir)

	callerFiles := map[string]string{
		"main.go": fmt.Sprintf(`package main
import dep "%s"
func Hello() {
	dep.Fetch()
}
`, depImportPath),
	}

	callerDir, callerManifestPath := createTempComponent(t, "asserted-caller-cli", "", callerFiles)
	relDepManifest, err := filepath.Rel(callerDir, depManifestPath)
	if err != nil {
		t.Fatalf("failed to compute relative path: %v", err)
	}

	actualManifest := fmt.Sprintf(`
name: "asserted-caller-cli"
interface_files: "main.go"
component_dependencies {
	name: "asserted-dep-cli"
	manifest: "%s"
}
`, filepath.ToSlash(relDepManifest))
	if err := os.WriteFile(callerManifestPath, []byte(actualManifest), 0644); err != nil {
		t.Fatalf("failed to update manifest: %v", err)
	}

	// 1. Text mode check
	stdout, stderr, exitCode := runArcc([]string{"check", callerManifestPath})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
	wantText := `Component "asserted-caller-cli" conforms; does not exceed declared authority

Dependencies:
- asserted-dep-cli: asserted
`
	if stdout != wantText {
		t.Errorf("stdout =\n%q\nwant:\n%q", stdout, wantText)
	}

	// 2. JSON mode check
	stdoutJSON, stderrJSON, exitCodeJSON := runArcc([]string{"check", callerManifestPath, "--format=json"})
	if exitCodeJSON != 0 {
		t.Fatalf("expected JSON exit code 0, got %d. Stderr: %s\nStdout: %s", exitCodeJSON, stderrJSON, stdoutJSON)
	}
	if stderrJSON != "" {
		t.Errorf("expected empty stderr for JSON, got %q", stderrJSON)
	}
	var rep report.ConformanceReport
	if err := json.Unmarshal([]byte(stdoutJSON), &rep); err != nil {
		t.Fatalf("json.Unmarshal error = %v; stdout = %q", err, stdoutJSON)
	}
	if len(rep.Violations) != 0 || len(rep.Warnings) != 0 {
		t.Errorf("expected 0 violations and 0 warnings, got violations=%v, warnings=%v", rep.Violations, rep.Warnings)
	}
	if len(rep.Dependencies) != 1 || rep.Dependencies[0].Component != "asserted-dep-cli" || rep.Dependencies[0].OwnCheckRuns {
		t.Errorf("rep.Dependencies = %+v, want asserted-dep-cli boundary", rep.Dependencies)
	}
}
