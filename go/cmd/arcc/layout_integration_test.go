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

	stdout, stderr, exitCode := runArcc([]string{"check", primaryManifestPath, "--package-layout=" + primaryLayoutPath})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	want := `Component "primary" conforms / ambient-authority-free`
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, want)
	}
}
