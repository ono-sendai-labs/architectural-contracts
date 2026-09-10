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
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/teststdlibmap"
)

// layoutTestKey and layoutTestAuthority are the fake authority used by the
// checker-level assertions of these tests: a map enumerating exactly the mock
// SDK's packages as safe.
var layoutTestKey = stdlibauthority.SDKKey{ToolchainVersion: "mock-sdk", GOOS: "linux", GOARCH: "amd64", MapFormatVersion: 1}

type layoutTestAuthority struct{}

func (layoutTestAuthority) IsStdlibPackage(pkgPath string) bool {
	switch pkgPath {
	case "fmt", "os", "strings", "syscall":
		return true
	}
	return false
}

func (layoutTestAuthority) SymbolAuthority(id symbol.SymbolID) (stdlibauthority.Classification, error) {
	switch facts.SymbolIDPackage(id) {
	case "fmt", "os", "strings", "syscall":
		return stdlibauthority.Classification{Safe: true}, nil
	}
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{
		Package: facts.SymbolIDPackage(id),
		Symbol:  id.Format(),
	}
}

func (layoutTestAuthority) PackageInitAuthority(pkgPath string) (stdlibauthority.Classification, error) {
	switch pkgPath {
	case "fmt", "os", "strings", "syscall":
		return stdlibauthority.Classification{Safe: true}, nil
	}
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{Package: pkgPath}
}

func (layoutTestAuthority) Evidence(symbol.SymbolID, stdlibauthority.Capability) []stdlibauthority.Frame {
	return nil
}

func (layoutTestAuthority) Key() stdlibauthority.SDKKey { return layoutTestKey }

var (
	sharedMapMu   sync.Mutex
	sharedMapByOS = map[string]string{}
	sharedMapErr  error
)

// nativeToolchainVersion reports the host toolchain's version, the identity
// the shared maps are generated and validated against.
func nativeToolchainVersion(t *testing.T) string {
	t.Helper()
	target, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		t.Fatalf("native target config: %v", err)
	}
	return target.ToolchainVersion
}

// sharedNativeMap generates the real stdlib authority map for a target GOOS
// once per test process (layout mode requires the declared --stdlib-map
// artifact, fail-closed before any verdict, and pinned layouts validate its
// full target key). The mock SDK trees only exercise layout loading, while
// every standard-library decision reads the map.
func sharedNativeMap(t *testing.T, goos string) string {
	t.Helper()
	sharedMapMu.Lock()
	defer sharedMapMu.Unlock()
	if path, ok := sharedMapByOS[goos]; ok {
		return path
	}
	if sharedMapErr != nil {
		t.Fatalf("shared native map: %v", sharedMapErr)
	}
	target := stdlibmap.TargetConfig{ToolchainVersion: nativeToolchainVersion(t), GOOS: goos, GOARCH: "amd64"}
	var path string
	if goos == teststdlibmap.PinnedGOOS {
		path = teststdlibmap.WritePinned(t)
	} else {
		path = teststdlibmap.WriteSynthetic(t, target,
			[]string{"fmt", "os", "strings", "syscall"},
			[]string{"fmt.Println", "os.DevNull", "strings.ToLower", "syscall.Open"})
	}
	sharedMapByOS[goos] = path
	return path
}

func sharedNativeMapDefault(t *testing.T) string {
	t.Helper()
	return sharedNativeMap(t, "linux")
}

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

	// The analyzed component root and its dependency component roots must be
	// siblings, so the fixture uses one workspace directory holding all three.
	absTmpDir, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatalf("resolve workspace: %v", err)
	}
	componentDir := filepath.Join(absTmpDir, "platformmember")
	if err := os.MkdirAll(componentDir, 0755); err != nil {
		t.Fatalf("create component dir: %v", err)
	}

	manifestPath := filepath.Join(componentDir, "component.textproto")
	manifest := "name: \"platformmember\"\ninterface_files: \"platformmember/member/interface.go\"\ncomponent_dependencies { name: \"linuxdep\" manifest: \"../linuxdep/component.textproto\" }\ncomponent_dependencies { name: \"windowsdep\" manifest: \"../windowsdep/component.textproto\" }\n"
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
		path := filepath.Join(componentDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create source directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
	sdkRoot := filepath.Join(absTmpDir, "mock_sdk")
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

	// Dependency components: one package-surface component per platform package.
	pkgJSON := func(id, name, goFile string) string {
		return fmt.Sprintf(`{
			"go_sdk_root": %q,
			"roots": [%q],
			"packages": [
				{
					"id": %q, "name": %q, "pkgPath": %q,
					"goFiles": [%q], "compiledGoFiles": [%q]
				}
			]
		}`, sdkRoot, id, id, name, id, goFile, goFile)
	}
	for _, dep := range []struct{ dir, pkgName, id, goFile string }{
		{"linuxdep", "linux", "example.com/linux", "platformmember/linux/impl.go"},
		{"windowsdep", "windows", "example.com/windows", "platformmember/windows/impl.go"},
	} {
		depDir := filepath.Join(absTmpDir, dep.dir)
		if err := os.MkdirAll(depDir, 0755); err != nil {
			t.Fatalf("create dep dir: %v", err)
		}
		depManifest := fmt.Sprintf("name: %q\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: %q\n", dep.dir, dep.id)
		if err := os.WriteFile(filepath.Join(depDir, "component.textproto"), []byte(depManifest), 0644); err != nil {
			t.Fatalf("write dep manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(depDir, "package-layout.json"), []byte(pkgJSON(dep.id, dep.pkgName, dep.goFile)), 0644); err != nil {
			t.Fatalf("write dep layout: %v", err)
		}
	}

	packages := []map[string]any{
		{
			"id": "example.com/member", "name": "member", "pkgPath": "example.com/member",
			"goFiles":         []string{"platformmember/member/api_linux.go", "platformmember/member/api_windows.go", "platformmember/member/interface.go"},
			"compiledGoFiles": []string{"platformmember/member/api_linux.go", "platformmember/member/api_windows.go", "platformmember/member/interface.go"},
		},
		{
			"id": "example.com/linux", "name": "linux", "pkgPath": "example.com/linux",
			"goFiles": []string{"platformmember/linux/impl.go"}, "compiledGoFiles": []string{"platformmember/linux/impl.go"},
		},
		{
			"id": "example.com/windows", "name": "windows", "pkgPath": "example.com/windows",
			"goFiles": []string{"platformmember/windows/impl.go"}, "compiledGoFiles": []string{"platformmember/windows/impl.go"},
		},
	}
	write := func(name, goos string, imports map[string]string) string {
		t.Helper()
		layout := map[string]any{
			"go_sdk_root": sdkRoot,
			"platform":    map[string]any{"goos": goos, "goarch": "amd64", "build_tags": []string{}, "cgo_enabled": false, "toolchain_version": nativeToolchainVersion(t)},
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
		path := filepath.Join(absTmpDir, name+".json")
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
	return absTmpDir, manifestPath, linux, windows, inconsistent
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

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})

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

func TestIntegration_LayoutMode_ExplicitStdlibFactsAndConformance(t *testing.T) {
	root := t.TempDir()
	sdkRoot := filepath.Join(root, "mock_sdk")
	for name, content := range map[string]string{
		"fmt/fmt.go": "package fmt\nfunc Println(...any) (int, error) { return 0, nil }\n",
		"os/os.go":   "package os\nvar DevNull string\n",
	} {
		path := filepath.Join(sdkRoot, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create SDK directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write SDK file %s: %v", name, err)
		}
	}
	memberPath := filepath.Join(root, "member", "api.go")
	if err := os.MkdirAll(filepath.Dir(memberPath), 0755); err != nil {
		t.Fatalf("create member directory: %v", err)
	}
	if err := os.WriteFile(memberPath, []byte("package member\nimport (\"fmt\"; \"os\")\nfunc Hello() { fmt.Println(os.DevNull) }\n"), 0644); err != nil {
		t.Fatalf("write member file: %v", err)
	}

	layoutPath := filepath.Join(root, "package-layout.json")
	layout := map[string]any{
		"go_sdk_root": sdkRoot,
		"roots":       []string{"example.com/member"},
		"packages": []map[string]any{
			{
				"id": "example.com/member", "name": "member", "pkgPath": "example.com/member",
				"goFiles": []string{"member/api.go"}, "compiledGoFiles": []string{"member/api.go"},
				"imports": map[string]string{"fmt": "fmt", "os": "os"},
			},
			{
				"id": "fmt", "name": "fmt", "pkgPath": "fmt", "is_stdlib": true,
				"goFiles": []string{"fmt/fmt.go"}, "compiledGoFiles": []string{"fmt/fmt.go"},
			},
		},
	}
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		t.Fatalf("marshal layout: %v", err)
	}
	if err := os.WriteFile(layoutPath, data, 0644); err != nil {
		t.Fatalf("write layout: %v", err)
	}

	var loaded facts.PackageFacts
	err = packagelayout.WithDriverEnv(layoutPath, root, func() error {
		var loadErr error
		loaded, loadErr = goanalysis.LoadPackageFacts(goanalysis.LoadRequest{ComponentRoot: root})
		return loadErr
	})
	if err != nil {
		t.Fatalf("layout LoadPackageFacts() error = %v", err)
	}
	if len(loaded.Packages) != 1 || loaded.Packages[0].ImportPath != "example.com/member" {
		t.Fatalf("loaded package facts = %+v, want member root", loaded.Packages)
	}
	// The loader produces typed import edges, not stdlib classification:
	// standard-library membership is the authority map's decision.
	sawFmt, sawOS := false, false
	for _, e := range loaded.Imports {
		switch e.ImportPath {
		case "fmt":
			sawFmt = true
		case "os":
			sawOS = true
		}
	}
	if !sawFmt || !sawOS {
		t.Errorf("import edges = %+v, want fmt and os", loaded.Imports)
	}
	// The checker classifies through the authority port: with a map
	// enumerating fmt and os as safe, the member's imports and references
	// are not undeclared dependencies.
	reportResult, checkErr := checker.Check(checker.Inputs{
		Manifest:  manifest.Manifest{Name: "explicit-stdlib", InterfaceFiles: []string{"member/api.go"}},
		Facts:     loaded,
		Authority: layoutTestAuthority{},
		SDKKey:    layoutTestKey,
	})
	if checkErr != nil {
		t.Fatalf("checker tool error: %v", checkErr)
	}
	for _, violation := range reportResult.Violations {
		if violation.Kind == report.UndeclaredDependency && (strings.Contains(violation.Message, `"fmt"`) || strings.Contains(violation.Message, `"os"`)) {
			t.Fatalf("stdlib import produced undeclared dependency: %+v", violation)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
	stdout, stderr, exitCode := runArccEnv(os.Environ(), []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})
	// The loader tolerates a layout-unresolved import, and the typed import
	// edge reports it at its exact site: an import the map does not
	// enumerate and no dependency owns is an UNDECLARED_DEPENDENCY, not an
	// analysis limitation.
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if !strings.Contains(stdout, "UNDECLARED_DEPENDENCY") || !strings.Contains(stdout, "member/api.go") || !strings.Contains(stdout, "example.com/missing") {
		t.Fatalf("stdout = %q, want the unresolved import as an undeclared dependency", stdout)
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
		goos         string
		unusedImport string
	}{{linuxLayout, "linux", "windowsdep"}, {windowsLayout, "windows", "linuxdep"}} {
		stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + tc.layoutPath, "--stdlib-map=" + sharedNativeMap(t, tc.goos)})
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

	_, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + inconsistentLayout, "--stdlib-map=" + sharedNativeMap(t, "linux")})
	if exitCode != 2 {
		t.Fatalf("inconsistent layout exit code = %d, want 2; stderr = %q", exitCode, stderr)
	}
	if !strings.Contains(stderr, "not contributed") || !strings.Contains(stderr, "example.com/windows") || !strings.Contains(stderr, "example.com/member") {
		t.Fatalf("inconsistent layout stderr = %q, want precise platform import mismatch", stderr)
	}
}

func TestIntegration_LayoutMode_InterfaceExclusionFollowsDeclaredPlatform(t *testing.T) {
	root, manifestPath, linuxLayout, windowsLayout, _ := createPlatformLayoutFixtures(t)
	manifest := "name: \"platform-interface\"\ninterface_files: \"platformmember/member/api_linux.go\"\ninterface_files: \"platformmember/member/api_windows.go\"\ncomponent_dependencies { name: \"linuxdep\" manifest: \"../linuxdep/component.textproto\" }\ncomponent_dependencies { name: \"windowsdep\" manifest: \"../windowsdep/component.textproto\" }\n"
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
		goos       string
		file       string
	}{
		{layoutPath: linuxLayout, goos: "linux", file: "platformmember/member/api_windows.go"},
		{layoutPath: windowsLayout, goos: "windows", file: "platformmember/member/api_linux.go"},
	} {
		textOutput, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + tc.layoutPath, "--stdlib-map=" + sharedNativeMap(t, tc.goos)})
		if exitCode != 0 || stderr != "" {
			t.Fatalf("layout %s text check failed: exit=%d stderr=%q stdout=%q", tc.layoutPath, exitCode, stderr, textOutput)
		}
		if !strings.Contains(textOutput, "INTERFACE_FILE_EXCLUDED") || !strings.Contains(textOutput, tc.file) {
			t.Fatalf("text output = %q, want exclusion for %s", textOutput, tc.file)
		}

		jsonOutput, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + tc.layoutPath, "--stdlib-map=" + sharedNativeMap(t, tc.goos), "--format=json"})
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

	if err := os.WriteFile(manifestPath, []byte("name: \"all-gated\"\ninterface_files: \"platformmember/member/api_linux.go\"\n"), 0o644); err != nil {
		t.Fatalf("write all-gated manifest: %v", err)
	}
	_, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + windowsLayout, "--stdlib-map=" + sharedNativeMap(t, "windows")})
	if exitCode != 2 || !strings.Contains(stderr, "no interface file survives") || !strings.Contains(stderr, "member/api_linux.go") {
		t.Fatalf("all-gated layout result: exit=%d stderr=%q, want fail-closed exclusion error", exitCode, stderr)
	}
}

func TestIntegration_LayoutMode_ExcludedUnresolvedImportHasNoWarning(t *testing.T) {
	root, manifestPath, linuxLayout, _, _ := createPlatformLayoutFixtures(t)
	excluded := filepath.Join(root, "platformmember", "member", "api_windows.go")
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

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + linuxLayout, "--stdlib-map=" + sharedNativeMap(t, "linux")})
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
		"platform":    map[string]any{"goos": "linux", "goarch": "amd64", "build_tags": []string{}, "cgo_enabled": false, "toolchain_version": nativeToolchainVersion(t)},
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

	textArgs := []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMap(t, "linux")}
	text1, stderr, exitCode := runArccHermetic(t, textArgs)
	if exitCode != 1 || stderr != "" {
		t.Fatalf("first text check failed: exit=%d stderr=%q stdout=%q", exitCode, stderr, text1)
	}
	text2, stderr, exitCode := runArccHermetic(t, textArgs)
	if exitCode != 1 || stderr != "" || text1 != text2 {
		t.Fatalf("repeated text check differed: exit=%d stderr=%q first=%q second=%q", exitCode, stderr, text1, text2)
	}

	jsonArgs := append(textArgs, "--format=json")
	json1, stderr, exitCode := runArccHermetic(t, jsonArgs)
	if exitCode != 1 || stderr != "" {
		t.Fatalf("first JSON check failed: exit=%d stderr=%q stdout=%q", exitCode, stderr, json1)
	}
	json2, stderr, exitCode := runArccHermetic(t, jsonArgs)
	if exitCode != 1 || stderr != "" || json1 != json2 {
		t.Fatalf("repeated JSON check differed: exit=%d stderr=%q first=%q second=%q", exitCode, stderr, json1, json2)
	}
	var rendered report.ConformanceReport
	if err := json.Unmarshal([]byte(json1), &rendered); err != nil {
		t.Fatalf("decode JSON report: %v", err)
	}
	if len(rendered.Warnings) != 0 || len(rendered.Violations) != 3 {
		t.Fatalf("rendered report = %#v, want three undeclared-dependency violations", rendered)
	}
	if !strings.Contains(text1, "member/a_linux.go") || !strings.Contains(text1, "member/z_linux.go") || !strings.Contains(text1, "other/impl_linux.go") {
		t.Fatalf("text report = %q, want all source-level observations", text1)
	}
}

func TestIntegration_LayoutMode_UndeclaredAuthority(t *testing.T) {
	manifest := `
name: "violatingmember"
interface_files: "member/api.go"
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

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d. Stderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}

	// The typed check attributes no transitive authority: the member's
	// reference into example.com/dep, which no declared dependency owns, is
	// an undeclared dependency at the exact sites.
	if !strings.Contains(stdout, "UNDECLARED_DEPENDENCY") {
		t.Errorf("expected UNDECLARED_DEPENDENCY in stdout: %s", stdout)
	}
	if !strings.Contains(stdout, `"example.com/dep"`) {
		t.Errorf("expected the undeclared dependency named in stdout: %s", stdout)
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
		Loader: goanalysis.LoadPackageFacts,
	}

	var stdout, stderr bytes.Buffer
	code1 := runner.Run([]string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)}, &stdout, &stderr)

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
		Loader: goanalysis.LoadPackageFacts,
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
			code := runner.Run([]string{"check", layoutManifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)}, &stdout, &stderr)
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

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", primaryManifestPath, "--package-layout=" + primaryLayoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})

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

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})

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
	// DR-17: the finding carries the sorted member site of the os.Open
	// reference inside OpenSomething.
	if !strings.Contains(stdout, "at member/api.go:") {
		t.Errorf("expected the os.Open reference site in stdout: %s", stdout)
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

	stdout, stderr, exitCode := runArccHermetic(t, []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})

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
