//go:build integration

package main_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

// runArccHermetic runs the compiled arcc binary with no Go toolchain reachable
// on PATH, and is how every layout-mode subprocess test invokes arcc.
//
// A working directory without a go.mod is not on its own enough to establish
// hermeticity: `go list std` succeeds perfectly well outside a module. So a
// regression that let go/packages fall back from the self-exec driver to
// `go list` — a driver that started reporting NotHandled for Capslock's nested
// "std" query, say — would still satisfy every assertion in this file, and
// would first surface under Bazel, where no Go toolchain is present at all.
// Withholding `go` turns that silent fallback into a test failure here.
func runArccHermetic(t *testing.T, args []string) (string, string, int) {
	t.Helper()

	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	// An empty directory keeps PATH well-formed while resolving no binaries,
	// so a fallback fails as "executable not found" rather than as a
	// malformed-environment error that could mask the real cause.
	env = append(env, "PATH="+t.TempDir())

	return runArccEnv(env, args)
}

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

	// Create a mock Go SDK under absTmpDir for testing standard library packages like syscall, os and strings
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

	osDir := filepath.Join(sdkRoot, "os")
	if err := os.MkdirAll(osDir, 0755); err != nil {
		t.Fatalf("failed to create mock os dir: %v", err)
	}
	osContent := `package os
var DevNull = "/dev/null"
func Open(name string) (file *File, err error) {
	return nil, nil
}
type File struct{}
`
	if err := os.WriteFile(filepath.Join(osDir, "file.go"), []byte(osContent), 0644); err != nil {
		t.Fatalf("failed to write mock file.go: %v", err)
	}

	stringsDir := filepath.Join(sdkRoot, "strings")
	if err := os.MkdirAll(stringsDir, 0755); err != nil {
		t.Fatalf("failed to create mock strings dir: %v", err)
	}
	stringsContent := `package strings
func ToLower(s string) string {
	return s
}
`
	if err := os.WriteFile(filepath.Join(stringsDir, "strings.go"), []byte(stringsContent), 0644); err != nil {
		t.Fatalf("failed to write mock strings.go: %v", err)
	}

	// Build the package-layout JSON
	type pkg struct {
		ID              string            `json:"id"`
		Name            string            `json:"name"`
		PkgPath         string            `json:"pkgPath"`
		IsStdlib        bool              `json:"is_stdlib"`
		GoFiles         []string          `json:"goFiles"`
		CompiledGoFiles []string          `json:"compiledGoFiles"`
		Imports         map[string]string `json:"imports,omitempty"`
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
	if _, ok := files["member/api.go"]; ok {
		p := pkg{
			ID:              "example.com/member",
			Name:            "member",
			PkgPath:         "example.com/member",
			GoFiles:         []string{"member/api.go"},
			CompiledGoFiles: []string{"member/api.go"},
		}
		if _, ok := files["dep/impl.go"]; ok {
			p.Imports = make(map[string]string)
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
		}
		lay.Packages = append(lay.Packages, p)
	}

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

func createPlatformLayoutFixtures(t *testing.T) (string, string, string, string, string) {
	t.Helper()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "component.textproto")
	manifest := "name: \"platformmember\"\ninterface_files: \"member/interface.go\"\nabsorbed_dependencies { import_path: \"example.com/linux\" }\nabsorbed_dependencies { import_path: \"example.com/windows\" }\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	for name, content := range map[string]string{
		"member/interface.go":   "package member\n",
		"member/api_linux.go":   "package member\nimport _ \"example.com/linux\"\nfunc Hello() {}\n",
		"member/api_windows.go": "package member\nimport _ \"example.com/windows\"\nfunc Hello() {}\n",
		"linux/impl.go":         "package linux\n",
		"windows/impl.go":       "package windows\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create source directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	sdkRoot := filepath.Join(root, "mock_sdk")
	for name, content := range map[string]string{
		"fmt/fmt.go":         "package fmt\n",
		"os/os.go":           "package os\ntype File struct{}\nvar DevNull string\n",
		"strings/strings.go": "package strings\nfunc ToLower(s string) string { return s }\n",
		"syscall/syscall.go": "package syscall\nfunc Open(string, int, uint32) (int, error) { return 0, nil }\n",
	} {
		path := filepath.Join(sdkRoot, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create SDK directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write SDK file %s: %v", name, err)
		}
	}

	packages := []map[string]any{
		{
			"id": "example.com/member", "name": "member", "pkgPath": "example.com/member",
			"goFiles":         []string{"member/api_linux.go", "member/api_windows.go", "member/interface.go"},
			"compiledGoFiles": []string{"member/api_linux.go", "member/api_windows.go", "member/interface.go"},
		},
		{
			"id": "example.com/linux", "name": "linux", "pkgPath": "example.com/linux",
			"goFiles": []string{"linux/impl.go"}, "compiledGoFiles": []string{"linux/impl.go"},
		},
		{
			"id": "example.com/windows", "name": "windows", "pkgPath": "example.com/windows",
			"goFiles": []string{"windows/impl.go"}, "compiledGoFiles": []string{"windows/impl.go"},
		},
	}
	write := func(name, goos string, imports map[string]string) string {
		t.Helper()
		layout := map[string]any{
			"go_sdk_root": sdkRoot,
			"platform":    map[string]any{"goos": goos, "goarch": "amd64", "build_tags": []string{}, "cgo_enabled": false},
			"roots":       []string{"example.com/member"},
			"packages":    append([]map[string]any(nil), packages...),
		}
		layoutPackages := layout["packages"].([]map[string]any)
		if imports != nil {
			layoutPackages[0]["imports"] = imports
		}
		data, err := json.MarshalIndent(layout, "", "  ")
		if err != nil {
			t.Fatalf("marshal %s layout: %v", name, err)
		}
		path := filepath.Join(root, name+".json")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write %s layout: %v", name, err)
		}
		return path
	}
	linux := write("linux", "linux", nil)
	windows := write("windows", "windows", nil)
	inconsistent := write("inconsistent", "linux", map[string]string{
		"example.com/linux":   "example.com/linux",
		"example.com/windows": "example.com/windows",
	})
	return root, manifestPath, linux, windows, inconsistent
}

// createLayoutFixtureInModule is like createLayoutFixture but creates the temp dir inside the go module tree to preserve go.mod resolution.
func createLayoutFixtureInModule(t *testing.T, name string, roots []string, manifestContent string, files map[string]string) (string, string, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("../../examples/csvtool", "layout-integration-"+name+"-*")
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

	// Create a mock Go SDK under absTmpDir for testing standard library packages like syscall, os and strings
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

	osDir := filepath.Join(sdkRoot, "os")
	if err := os.MkdirAll(osDir, 0755); err != nil {
		t.Fatalf("failed to create mock os dir: %v", err)
	}
	osContent := `package os
var DevNull = "/dev/null"
func Open(name string) (file *File, err error) {
	return nil, nil
}
type File struct{}
`
	if err := os.WriteFile(filepath.Join(osDir, "file.go"), []byte(osContent), 0644); err != nil {
		t.Fatalf("failed to write mock file.go: %v", err)
	}

	stringsDir := filepath.Join(sdkRoot, "strings")
	if err := os.MkdirAll(stringsDir, 0755); err != nil {
		t.Fatalf("failed to create mock strings dir: %v", err)
	}
	stringsContent := `package strings
func ToLower(s string) string {
	return s
}
`
	if err := os.WriteFile(filepath.Join(stringsDir, "strings.go"), []byte(stringsContent), 0644); err != nil {
		t.Fatalf("failed to write mock strings.go: %v", err)
	}

	// Build the package-layout JSON
	type pkg struct {
		ID              string            `json:"id"`
		Name            string            `json:"name"`
		PkgPath         string            `json:"pkgPath"`
		IsStdlib        bool              `json:"is_stdlib"`
		GoFiles         []string          `json:"goFiles"`
		CompiledGoFiles []string          `json:"compiledGoFiles"`
		Imports         map[string]string `json:"imports,omitempty"`
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
	if _, ok := files["member/api.go"]; ok {
		p := pkg{
			ID:              "example.com/member",
			Name:            "member",
			PkgPath:         "example.com/member",
			GoFiles:         []string{"member/api.go"},
			CompiledGoFiles: []string{"member/api.go"},
		}
		if _, ok := files["dep/impl.go"]; ok {
			p.Imports = make(map[string]string)
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
		}
		lay.Packages = append(lay.Packages, p)
	}

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

import (
	"os"
	"strings"
)

func Hello() string {
	_ = os.DevNull
	return strings.ToLower("Hello, Layout Mode!")
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

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + layoutPath})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	want := `Component "puremember" conforms; does not exceed declared authority`
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, want)
	}
}

func TestIntegration_LayoutMode_UnresolvedImportWarning(t *testing.T) {
	manifest := `
name: "unresolvedmember"
interface_files: "member/api.go"
`
	files := map[string]string{
		"member/api.go": `package member

import _ "example.com/missing"

func Hello() {}
`,
	}

	absTmpDir, manifestPath, layoutPath := createLayoutFixture(t, "unresolved", []string{"example.com/member"}, manifest, files)
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(absTmpDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer os.Chdir(origWd)
	stdout, stderr, exitCode := runArccEnv(os.Environ(), []string{"check", manifestPath, "--package-layout=" + layoutPath})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if !strings.Contains(stdout, "ANALYSIS_LIMITATION") || !strings.Contains(stdout, "member/api.go") || !strings.Contains(stdout, "example.com/missing") {
		t.Fatalf("stdout = %q, want unresolved-import analysis limitation", stdout)
	}
}

func TestIntegration_LayoutMode_PlatformVariantsUseSameBinary(t *testing.T) {
	root, manifestPath, linuxLayout, windowsLayout, inconsistentLayout := createPlatformLayoutFixtures(t)
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer os.Chdir(origWd)

	for _, tc := range []struct {
		layoutPath   string
		unusedImport string
	}{{linuxLayout, "example.com/windows"}, {windowsLayout, "example.com/linux"}} {
		stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + tc.layoutPath})
		if exitCode != 0 || stderr != "" {
			t.Fatalf("platform layout %s failed: exit=%d stderr=%q stdout=%q", tc.layoutPath, exitCode, stderr, stdout)
		}
		if !strings.Contains(stdout, "Component: platformmember") || strings.Contains(stdout, "UNDECLARED_DEPENDENCY") {
			t.Fatalf("platform layout %s stdout = %q, want only platform-correct dependency findings", tc.layoutPath, stdout)
		}
		if !strings.Contains(stdout, "UNUSED_DEPENDENCY") || !strings.Contains(stdout, tc.unusedImport) {
			t.Fatalf("platform layout %s stdout = %q, want excluded dependency %q", tc.layoutPath, stdout, tc.unusedImport)
		}
	}

	_, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + inconsistentLayout})
	if exitCode != 2 {
		t.Fatalf("inconsistent layout exit code = %d, want 2; stderr = %q", exitCode, stderr)
	}
	if !strings.Contains(stderr, "not contributed") || !strings.Contains(stderr, "example.com/windows") || !strings.Contains(stderr, "example.com/member") {
		t.Fatalf("inconsistent layout stderr = %q, want precise platform import mismatch", stderr)
	}
}

func TestIntegration_LayoutMode_InterfaceExclusionFollowsDeclaredPlatform(t *testing.T) {
	root, manifestPath, linuxLayout, windowsLayout, _ := createPlatformLayoutFixtures(t)
	manifest := "name: \"platform-interface\"\ninterface_files: \"member/api_linux.go\"\ninterface_files: \"member/api_windows.go\"\nabsorbed_dependencies { import_path: \"example.com/linux\" }\nabsorbed_dependencies { import_path: \"example.com/windows\" }\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer os.Chdir(origWd)

	for _, tc := range []struct {
		layoutPath string
		file       string
	}{
		{layoutPath: linuxLayout, file: "member/api_windows.go"},
		{layoutPath: windowsLayout, file: "member/api_linux.go"},
	} {
		textOutput, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + tc.layoutPath})
		if exitCode != 0 || stderr != "" {
			t.Fatalf("layout %s text check failed: exit=%d stderr=%q stdout=%q", tc.layoutPath, exitCode, stderr, textOutput)
		}
		if !strings.Contains(textOutput, "INTERFACE_FILE_EXCLUDED") || !strings.Contains(textOutput, tc.file) {
			t.Fatalf("text output = %q, want exclusion for %s", textOutput, tc.file)
		}

		jsonOutput, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + tc.layoutPath, "--format=json"})
		if exitCode != 0 || stderr != "" {
			t.Fatalf("layout %s JSON check failed: exit=%d stderr=%q stdout=%q", tc.layoutPath, exitCode, stderr, jsonOutput)
		}
		var rendered report.ConformanceReport
		if err := json.Unmarshal([]byte(jsonOutput), &rendered); err != nil {
			t.Fatalf("decode JSON output %q: %v", jsonOutput, err)
		}
		var found bool
		for _, warning := range rendered.Warnings {
			if warning.Kind == report.InterfaceFileExcluded && warning.Location.File == tc.file {
				found = true
			}
		}
		if !found {
			t.Fatalf("JSON report = %#v, want exclusion warning for %s", rendered, tc.file)
		}
	}

	if err := os.WriteFile(manifestPath, []byte("name: \"all-gated\"\ninterface_files: \"member/api_linux.go\"\n"), 0o644); err != nil {
		t.Fatalf("write all-gated manifest: %v", err)
	}
	_, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + windowsLayout})
	if exitCode != 2 || !strings.Contains(stderr, "no interface file survives") || !strings.Contains(stderr, "member/api_linux.go") {
		t.Fatalf("all-gated layout result: exit=%d stderr=%q, want fail-closed exclusion error", exitCode, stderr)
	}
}

func TestIntegration_LayoutMode_ExcludedUnresolvedImportHasNoWarning(t *testing.T) {
	root, manifestPath, linuxLayout, _, _ := createPlatformLayoutFixtures(t)
	excluded := filepath.Join(root, "member", "api_windows.go")
	if err := os.WriteFile(excluded, []byte("package member\nimport \"example.com/excluded\"\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatalf("write excluded source: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer os.Chdir(origWd)

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + linuxLayout})
	if exitCode != 0 || stderr != "" {
		t.Fatalf("excluded unresolved layout failed: exit=%d stderr=%q stdout=%q", exitCode, stderr, stdout)
	}
	if strings.Contains(stdout, "example.com/excluded") || strings.Contains(stdout, "ANALYSIS_LIMITATION") {
		t.Fatalf("stdout = %q, want no limitation for excluded import", stdout)
	}
}

func TestIntegration_LayoutMode_UnresolvedDiagnosticsAreStable(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "component.textproto")
	manifest := "name: \"deterministic\"\ninterface_files: \"member/interface.go\"\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	for name, content := range map[string]string{
		"member/interface.go": "package member\n",
		"member/a_linux.go":   "package member\nimport \"example.com/missing\"\nfunc A() {}\n",
		"member/z_linux.go":   "package member\nimport \"example.com/missing\"\nfunc Z() {}\n",
		"other/impl_linux.go": "package other\nimport \"example.com/other-missing\"\nfunc Other() {}\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create source directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	sdkRoot := filepath.Join(root, "mock_sdk")
	for name, content := range map[string]string{
		"fmt/fmt.go":         "package fmt\n",
		"os/os.go":           "package os\ntype File struct{}\nvar DevNull string\n",
		"strings/strings.go": "package strings\nfunc ToLower(s string) string { return s }\n",
		"syscall/syscall.go": "package syscall\nfunc Open(string, int, uint32) (int, error) { return 0, nil }\n",
	} {
		path := filepath.Join(sdkRoot, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create SDK directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write SDK file %s: %v", name, err)
		}
	}
	source := func(files ...string) map[string]any {
		return map[string]any{"goFiles": files, "compiledGoFiles": files}
	}
	member := source("member/a_linux.go", "member/z_linux.go", "member/interface.go")
	member["id"] = "example.com/member"
	member["name"] = "member"
	member["pkgPath"] = "example.com/member"
	other := source("other/impl_linux.go")
	other["id"] = "example.com/other"
	other["name"] = "other"
	other["pkgPath"] = "example.com/other"
	layout := map[string]any{
		"go_sdk_root": sdkRoot,
		"platform":    map[string]any{"goos": "linux", "goarch": "amd64", "build_tags": []string{}, "cgo_enabled": false},
		"roots":       []string{"example.com/member", "example.com/other"},
		"packages":    []map[string]any{member, other},
	}
	layoutPath := filepath.Join(root, "layout.json")
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		t.Fatalf("marshal layout: %v", err)
	}
	if err := os.WriteFile(layoutPath, data, 0644); err != nil {
		t.Fatalf("write layout: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer os.Chdir(origWd)

	textArgs := []string{"check", manifestPath, "--package-layout=" + layoutPath}
	text1, stderr, exitCode := runArccHermetic(t, textArgs)
	if exitCode != 0 || stderr != "" {
		t.Fatalf("first text check failed: exit=%d stderr=%q stdout=%q", exitCode, stderr, text1)
	}
	text2, stderr, exitCode := runArccHermetic(t, textArgs)
	if exitCode != 0 || stderr != "" || text1 != text2 {
		t.Fatalf("repeated text check differed: exit=%d stderr=%q first=%q second=%q", exitCode, stderr, text1, text2)
	}

	jsonArgs := append(textArgs, "--format=json")
	json1, stderr, exitCode := runArccHermetic(t, jsonArgs)
	if exitCode != 0 || stderr != "" {
		t.Fatalf("first JSON check failed: exit=%d stderr=%q stdout=%q", exitCode, stderr, json1)
	}
	json2, stderr, exitCode := runArccHermetic(t, jsonArgs)
	if exitCode != 0 || stderr != "" || json1 != json2 {
		t.Fatalf("repeated JSON check differed: exit=%d stderr=%q first=%q second=%q", exitCode, stderr, json1, json2)
	}
	var rendered report.ConformanceReport
	if err := json.Unmarshal([]byte(json1), &rendered); err != nil {
		t.Fatalf("decode JSON report: %v", err)
	}
	if len(rendered.Violations) != 0 || len(rendered.Warnings) != 3 {
		t.Fatalf("rendered report = %#v, want three limitation warnings", rendered)
	}
	if !strings.Contains(text1, "member/a_linux.go") || !strings.Contains(text1, "member/z_linux.go") || !strings.Contains(text1, "other/impl_linux.go") {
		t.Fatalf("text report = %q, want all source-level observations", text1)
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

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + layoutPath})

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
	// 1. Run a layout-backed check
	layoutManifest := `
name: "layout-sameproc"
interface_files: "member/api.go"
`
	layoutFiles := map[string]string{
		"member/api.go": `package member
func Hello() string { return "layout" }
`,
	}
	absTmpDir, manifestPath, layoutPath := createLayoutFixture(t, "seq-layout", []string{"example.com/member"}, layoutManifest, layoutFiles)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(absTmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	runner := &app.Runner{
		Loader:   goanalysis.LoadPackageFacts,
		Analyzer: capslockadapter.NewAdapter(),
	}

	var stdout, stderr bytes.Buffer
	code1 := runner.Run([]string{"check", manifestPath, "--package-layout=" + layoutPath}, &stdout, &stderr)

	// Restore working directory immediately before doing colocated creation
	if err := os.Chdir(origWd); err != nil {
		t.Fatalf("failed to restore chdir: %v", err)
	}

	if code1 != 0 {
		t.Fatalf("layout check failed: code %d, stderr: %s, stdout: %s", code1, stderr.String(), stdout.String())
	}

	// Verify layout mode is deactivated after the run
	if packagelayout.IsLayoutMode() {
		t.Error("expected layout mode to be deactivated after run")
	}
	if val, present := os.LookupEnv("GOPACKAGESDRIVER"); present {
		t.Errorf("expected GOPACKAGESDRIVER to be unset, got %q", val)
	}

	// 2. Run a colocated check in the same process
	colocatedManifest := `
name: "colocated-sameproc"
interface_files: "helper.go"
`
	colocatedFiles := map[string]string{
		"helper.go": `package colocated
func Helper() {}
`,
	}
	// Note: createTempComponent creates the temp directory inside the go module tree, which is required for correct colocated loading
	_, colocatedManifestPath := createTempComponent(t, "seq-colocated", colocatedManifest, colocatedFiles)

	stdout.Reset()
	stderr.Reset()
	code2 := runner.Run([]string{"check", colocatedManifestPath}, &stdout, &stderr)
	if code2 != 0 {
		t.Fatalf("colocated check failed: code %d, stderr: %s, stdout: %s", code2, stderr.String(), stdout.String())
	}
}

func TestIntegration_LayoutMode_ConcurrentIsolation(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}

	// Setup fixtures under origWd first!
	layoutManifest := `
name: "layout-concurrent"
interface_files: "member/api.go"
`
	layoutFiles := map[string]string{
		"member/api.go": `package member
func Hello() string { return "layout" }
`,
	}
	layoutTmpDir, layoutManifestPath, layoutPath := createLayoutFixtureInModule(t, "con-layout", []string{"example.com/member"}, layoutManifest, layoutFiles)

	colocatedManifest := `
name: "colocated-concurrent"
interface_files: "helper.go"
`
	colocatedFiles := map[string]string{
		"helper.go": `package colocated
func Helper() {}
`,
	}
	_, colocatedManifestPath := createTempComponent(t, "con-colocated", colocatedManifest, colocatedFiles)

	runner := &app.Runner{
		Loader:   goanalysis.LoadPackageFacts,
		Analyzer: capslockadapter.NewAdapter(),
	}

	var wg sync.WaitGroup
	var errs []string
	var errsMu sync.Mutex

	recordErr := func(msg string) {
		errsMu.Lock()
		errs = append(errs, msg)
		errsMu.Unlock()
	}

	// Launch concurrent checks.
	// Since colocated checks are CWD-independent, and layout checks only depend on layoutTmpDir,
	// we will run them all while the main test process temporarily switches to layoutTmpDir.
	// Because Runner.Run is serialized under CheckMu, they will execute one by one safely!
	if err := os.Chdir(layoutTmpDir); err != nil {
		t.Fatalf("failed to chdir to layoutTmpDir: %v", err)
	}
	defer os.Chdir(origWd)

	numGers := 4
	for i := 0; i < numGers; i++ {
		wg.Add(2)
		// Goroutine A: Layout check
		go func(id int) {
			defer wg.Done()
			var stdout, stderr bytes.Buffer
			code := runner.Run([]string{"check", layoutManifestPath, "--package-layout=" + layoutPath}, &stdout, &stderr)
			if code != 0 {
				recordErr(fmt.Sprintf("layout check %d failed with code %d. stderr: %s", id, code, stderr.String()))
			}
		}(i)

		// Goroutine B: Colocated check
		go func(id int) {
			defer wg.Done()
			var stdout, stderr bytes.Buffer
			code := runner.Run([]string{"check", colocatedManifestPath}, &stdout, &stderr)
			if code != 0 {
				recordErr(fmt.Sprintf("colocated check %d failed with code %d. stderr: %s", id, code, stderr.String()))
			}
		}(i)
	}

	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("Concurrent isolation failures:\n%s", strings.Join(errs, "\n"))
	}
}

func TestIntegration_LayoutMode_DependencyResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "layout-dep-integration-*")
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

	primaryDir := filepath.Join(absTmpDir, "primary")
	depDir := filepath.Join(absTmpDir, "depcomponent")
	if err := os.MkdirAll(primaryDir, 0755); err != nil {
		t.Fatalf("failed to create primary dir: %v", err)
	}
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatalf("failed to create dep dir: %v", err)
	}

	// 1. Create dependency component files
	depManifest := `
name: "depcomponent"
interface_files: "depcomponent/api.go"
`
	if err := os.WriteFile(filepath.Join(depDir, "component.textproto"), []byte(depManifest), 0644); err != nil {
		t.Fatalf("failed to write dep manifest: %v", err)
	}

	depSrc := `package depcomponent
func ExportedFunc() {}
`
	if err := os.WriteFile(filepath.Join(depDir, "api.go"), []byte(depSrc), 0644); err != nil {
		t.Fatalf("failed to write dep source: %v", err)
	}

	// 2. Create primary component files
	primaryManifest := `
name: "primary"
interface_files: "primary/member/api.go"
component_dependencies {
	name: "depcomponent"
	manifest: "../depcomponent/component.textproto"
}
`
	primaryManifestPath := filepath.Join(primaryDir, "component.textproto")
	if err := os.WriteFile(primaryManifestPath, []byte(primaryManifest), 0644); err != nil {
		t.Fatalf("failed to write primary manifest: %v", err)
	}

	primarySrc := `package member
import "example.com/depcomponent"
func Hello() {
	depcomponent.ExportedFunc()
}
`
	if err := os.MkdirAll(filepath.Join(primaryDir, "member"), 0755); err != nil {
		t.Fatalf("failed to create primary member dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(primaryDir, "member", "api.go"), []byte(primarySrc), 0644); err != nil {
		t.Fatalf("failed to write primary source: %v", err)
	}

	// 3. Create mock Go SDK
	sdkRoot := filepath.Join(absTmpDir, "mock_sdk")
	if err := os.MkdirAll(filepath.Join(sdkRoot, "syscall"), 0755); err != nil {
		t.Fatalf("failed to create mock syscall dir: %v", err)
	}
	syscallContent := `package syscall
`
	if err := os.WriteFile(filepath.Join(sdkRoot, "syscall", "syscall.go"), []byte(syscallContent), 0644); err != nil {
		t.Fatalf("failed to write mock syscall: %v", err)
	}

	// 4. Build package-layouts
	type pkg struct {
		ID              string            `json:"id"`
		Name            string            `json:"name"`
		PkgPath         string            `json:"pkgPath"`
		IsStdlib        bool              `json:"is_stdlib"`
		GoFiles         []string          `json:"goFiles"`
		CompiledGoFiles []string          `json:"compiledGoFiles"`
		Imports         map[string]string `json:"imports"`
	}
	type layout struct {
		GoSDKRoot string   `json:"go_sdk_root"`
		Roots     []string `json:"roots"`
		Packages  []pkg    `json:"packages"`
	}

	depLayout := layout{
		GoSDKRoot: sdkRoot,
		Roots:     []string{"example.com/depcomponent"},
		Packages: []pkg{
			{
				ID:              "example.com/depcomponent",
				Name:            "depcomponent",
				PkgPath:         "example.com/depcomponent",
				GoFiles:         []string{"depcomponent/api.go"},
				CompiledGoFiles: []string{"depcomponent/api.go"},
				Imports:         make(map[string]string),
			},
			{
				ID:              "syscall",
				Name:            "syscall",
				PkgPath:         "syscall",
				IsStdlib:        true,
				GoFiles:         []string{"syscall.go"},
				CompiledGoFiles: []string{"syscall.go"},
				Imports:         make(map[string]string),
			},
		},
	}
	depLayoutData, _ := json.MarshalIndent(depLayout, "", "  ")
	if err := os.WriteFile(filepath.Join(depDir, "package-layout.json"), depLayoutData, 0644); err != nil {
		t.Fatalf("failed to write dep layout: %v", err)
	}

	primaryLayout := layout{
		GoSDKRoot: sdkRoot,
		Roots:     []string{"example.com/member"},
		Packages: []pkg{
			{
				ID:              "example.com/member",
				Name:            "member",
				PkgPath:         "example.com/member",
				GoFiles:         []string{"primary/member/api.go"},
				CompiledGoFiles: []string{"primary/member/api.go"},
				Imports: map[string]string{
					"example.com/depcomponent": "example.com/depcomponent",
				},
			},
			{
				ID:              "example.com/depcomponent",
				Name:            "depcomponent",
				PkgPath:         "example.com/depcomponent",
				GoFiles:         []string{"depcomponent/api.go"},
				CompiledGoFiles: []string{"depcomponent/api.go"},
				Imports:         make(map[string]string),
			},
			{
				ID:              "syscall",
				Name:            "syscall",
				PkgPath:         "syscall",
				IsStdlib:        true,
				GoFiles:         []string{"syscall.go"},
				CompiledGoFiles: []string{"syscall.go"},
				Imports:         make(map[string]string),
			},
		},
	}
	primaryLayoutData, _ := json.MarshalIndent(primaryLayout, "", "  ")
	primaryLayoutPath := filepath.Join(primaryDir, "package-layout.json")
	if err := os.WriteFile(primaryLayoutPath, primaryLayoutData, 0644); err != nil {
		t.Fatalf("failed to write primary layout: %v", err)
	}

	// 5. Run layout-backed check
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	if err := os.Chdir(absTmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(origWd)

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", primaryManifestPath, "--package-layout=" + primaryLayoutPath})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	want := `Component "primary" conforms; does not exceed declared authority`
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, want)
	}
}

func TestIntegration_LayoutMode_FILES_Violation(t *testing.T) {
	manifest := `
name: "violatingfiles"
interface_files: "member/api.go"
`
	files := map[string]string{
		"member/api.go": `package member

import (
	"os"
	"strings"
)

func OpenSomething() {
	_ = strings.ToLower("FOO")
	_, _ = os.Open("foo.txt")
}
`,
	}

	absTmpDir, manifestPath, layoutPath := createLayoutFixture(t, "violatingfiles", []string{"example.com/member"}, manifest, files)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	if err := os.Chdir(absTmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(origWd)

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + layoutPath})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	if !strings.Contains(stdout, "UNDECLARED_AUTHORITY") {
		t.Errorf("expected UNDECLARED_AUTHORITY in stdout: %s", stdout)
	}
	if !strings.Contains(stdout, `use of undeclared authority "FILES"`) {
		t.Errorf("expected FILES violation in stdout: %s", stdout)
	}
	if !strings.Contains(stdout, "OpenSomething") {
		t.Errorf("expected evidence call path in stdout: %s", stdout)
	}
}

func TestIntegration_LayoutMode_MissingSource_Exit2(t *testing.T) {
	manifest := `
name: "missingsource"
interface_files: "member/api.go"
`
	files := map[string]string{
		"member/api.go": `package member
func Hello() {}
`,
	}

	absTmpDir, manifestPath, layoutPath := createLayoutFixture(t, "missingsource", []string{"example.com/member"}, manifest, files)

	// Mutate layout to point to a nonexistent file
	layoutBytes, err := os.ReadFile(layoutPath)
	if err != nil {
		t.Fatalf("failed to read layout: %v", err)
	}

	// We replace "member/api.go" with "member/nonexistent.go" in layout json to trigger missing source error
	mutatedLayout := strings.ReplaceAll(string(layoutBytes), "member/api.go", "member/nonexistent.go")
	if err := os.WriteFile(layoutPath, []byte(mutatedLayout), 0644); err != nil {
		t.Fatalf("failed to write mutated layout: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	if err := os.Chdir(absTmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(origWd)

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + layoutPath})

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}

	if !strings.Contains(stderr, "nonexistent.go") {
		t.Errorf("expected stderr to report nonexistent.go, got %q", stderr)
	}
	if !strings.Contains(stderr, "example.com/member") {
		t.Errorf("expected stderr to report package example.com/member, got %q", stderr)
	}
}
