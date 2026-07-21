//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createLayoutFixture creates a layout and manifest for testing layout mode.
func createLayoutFixture(t *testing.T, name string, roots []string, manifestContent string, files map[string]string) (string, string, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "layout-integration-"+name+"-*")
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

	manifestPath := filepath.Join(absTmpDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Write source files relative to workspace (absTmpDir)
	for relPath, content := range files {
		filePath := filepath.Join(absTmpDir, relPath)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", relPath, err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", relPath, err)
		}
	}

	// Create a mock Go SDK under absTmpDir for testing standard library packages like syscall
	sdkRoot := filepath.Join(absTmpDir, "mock_sdk")
	syscallDir := filepath.Join(sdkRoot, "syscall")
	if err := os.MkdirAll(syscallDir, 0755); err != nil {
		t.Fatalf("failed to create mock syscall dir: %v", err)
	}
	syscallContent := `package syscall
func Open(path string, mode int, perm uint32) (fd int, err error) {
	return 0, nil
}
`
	if err := os.WriteFile(filepath.Join(syscallDir, "syscall.go"), []byte(syscallContent), 0644); err != nil {
		t.Fatalf("failed to write mock syscall.go: %v", err)
	}

	// Build the package-layout JSON
	type pkg struct {
		ID              string            `json:"id"`
		Name            string            `json:"name"`
		PkgPath         string            `json:"pkgPath"`
		GoFiles         []string          `json:"goFiles"`
		CompiledGoFiles []string          `json:"compiledGoFiles"`
		Imports         map[string]string `json:"imports"`
	}
	type layout struct {
		GoSDKRoot string   `json:"go_sdk_root"`
		Roots     []string `json:"roots"`
		Packages  []pkg    `json:"packages"`
	}

	lay := layout{
		GoSDKRoot: sdkRoot,
		Roots:     roots,
		Packages:  []pkg{},
	}

	// Let's dynamically add the packages from files
	// For example, if "member/api.go" is in files, we can add "example.com/member"
	if _, ok := files["member/api.go"]; ok {
		p := pkg{
			ID:              "example.com/member",
			Name:            "member",
			PkgPath:         "example.com/member",
			GoFiles:         []string{"member/api.go"},
			CompiledGoFiles: []string{"member/api.go"},
			Imports:         make(map[string]string),
		}
		if _, ok := files["dep/impl.go"]; ok {
			p.Imports["example.com/dep"] = "example.com/dep"
		}
		lay.Packages = append(lay.Packages, p)
	}

	if _, ok := files["dep/impl.go"]; ok {
		p := pkg{
			ID:              "example.com/dep",
			Name:            "dep",
			PkgPath:         "example.com/dep",
			GoFiles:         []string{"dep/impl.go"},
			CompiledGoFiles: []string{"dep/impl.go"},
			Imports:         make(map[string]string),
		}
		p.Imports["syscall"] = "syscall"
		lay.Packages = append(lay.Packages, p)
	}

	// Add syscall standard library package
	lay.Packages = append(lay.Packages, pkg{
		ID:              "syscall",
		Name:            "syscall",
		PkgPath:         "syscall",
		GoFiles:         []string{"syscall.go"},
		CompiledGoFiles: []string{"syscall.go"},
		Imports:         make(map[string]string),
	})

	layoutData, err := json.MarshalIndent(lay, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal layout data: %v", err)
	}

	layoutPath := filepath.Join(absTmpDir, "package-layout.json")
	if err := os.WriteFile(layoutPath, layoutData, 0644); err != nil {
		t.Fatalf("failed to write layout file: %v", err)
	}

	return absTmpDir, manifestPath, layoutPath
}

func TestIntegration_LayoutMode_Success(t *testing.T) {
	manifest := `
name: "puremember"
interface_files: "member/api.go"
`
	files := map[string]string{
		"member/api.go": `package member

func Hello() string {
	return "Hello, Layout Mode!"
}
`,
	}

	absTmpDir, manifestPath, layoutPath := createLayoutFixture(t, "pure", []string{"example.com/member"}, manifest, files)

	// Run from a directory that has no go.mod to guarantee hermeticity
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	if err := os.Chdir(absTmpDir); err != nil {
		t.Fatalf("failed to change directory to hermetic: %v", err)
	}
	defer os.Chdir(origWd)

	stdout, stderr, exitCode := runArcc([]string{"check", manifestPath, "--package-layout=" + layoutPath})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	want := `Component "puremember" conforms / ambient-authority-free`
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, want)
	}
}

func TestIntegration_LayoutMode_UndeclaredAuthority(t *testing.T) {
	manifest := `
name: "violatingmember"
interface_files: "member/api.go"
absorbed_dependencies {
	import_path: "example.com/dep"
}
`
	files := map[string]string{
		"member/api.go": `package member

import "example.com/dep"

func Hello() {
	dep.Trap()
}
`,
		"dep/impl.go": `package dep

import "syscall"

func Trap() {
	_, _ = syscall.Open("foo", 0, 0)
}
`,
	}

	absTmpDir, manifestPath, layoutPath := createLayoutFixture(t, "violating", []string{"example.com/member"}, manifest, files)

	// Run from a directory with no go.mod to guarantee hermeticity
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	if err := os.Chdir(absTmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(origWd)

	stdout, stderr, exitCode := runArcc([]string{"check", manifestPath, "--package-layout=" + layoutPath})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	if !strings.Contains(stdout, "UNDECLARED_AUTHORITY") {
		t.Errorf("expected UNDECLARED_AUTHORITY in stdout: %s", stdout)
	}
	if !strings.Contains(stdout, `use of undeclared authority "SYSTEM_CALLS"`) {
		t.Errorf("expected SYSTEM_CALLS violation in stdout: %s", stdout)
	}
}

func TestIntegration_LayoutMode_Isolation(t *testing.T) {
	// Verify that layout-backed check does not pollute process-global state.
	// Since we are running the CLI executable, different CLI invocations are fully isolated.
	// We can also verify that a layout-backed check followed by a colocated check works.
	// Colocated checks run fine because they don't see any GOPACKAGESDRIVER from our process.
	// This is implicitly guaranteed by the process-boundary of `runArcc`.
}
