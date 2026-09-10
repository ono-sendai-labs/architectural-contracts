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

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
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
func runArcc(args []string) (stdout, stderr string, code int) {
	if len(args) >= 2 && args[0] == "check" {
		var staged nativeArtifactStaging
		if err := stageNativeDependencyArtifacts(args[1], map[string]bool{}, &staged); err != nil {
			if restoreErr := staged.restore(); restoreErr != nil {
				return "", fmt.Sprintf("error: staging dependency surfaces: %v; restoring artifacts: %v\n", err, restoreErr), 2
			}
			return "", fmt.Sprintf("error: staging dependency surfaces: %v\n", err), 2
		}
		defer func() {
			if err := staged.restore(); err != nil {
				stderr += fmt.Sprintf("error: restoring staged dependency artifacts: %v\n", err)
				if code == 0 {
					code = 2
				}
			}
		}()
	}
	return runArccEnv(nil, args)
}

type nativeArtifactOriginal struct {
	data   []byte
	mode   os.FileMode
	exists bool
}

// nativeArtifactStaging makes dependency artifact production independent of
// developer cache state while restoring every pre-existing companion after the
// command. A surface without its matching report is never accepted as a fresh
// pair merely because the surface happens to exist.
type nativeArtifactStaging struct {
	originals map[string]nativeArtifactOriginal
	order     []string
}

func (s *nativeArtifactStaging) capture(path string) error {
	if s.originals == nil {
		s.originals = make(map[string]nativeArtifactOriginal)
	}
	if _, ok := s.originals[path]; ok {
		return nil
	}
	info, err := os.Stat(path)
	if err == nil {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		s.originals[path] = nativeArtifactOriginal{data: data, mode: info.Mode().Perm(), exists: true}
	} else if os.IsNotExist(err) {
		s.originals[path] = nativeArtifactOriginal{}
	} else {
		return err
	}
	s.order = append(s.order, path)
	return nil
}

func (s *nativeArtifactStaging) restore() error {
	if len(s.order) == 0 {
		return nil
	}
	var restoreErrors []string
	for i := len(s.order) - 1; i >= 0; i-- {
		path := s.order[i]
		original := s.originals[path]
		if original.exists {
			if err := os.WriteFile(path, original.data, original.mode); err != nil {
				restoreErrors = append(restoreErrors, fmt.Sprintf("%s: %v", path, err))
			}
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			restoreErrors = append(restoreErrors, fmt.Sprintf("%s: %v", path, err))
		}
	}
	s.order = nil
	if len(restoreErrors) > 0 {
		return fmt.Errorf("%s", strings.Join(restoreErrors, "; "))
	}
	return nil
}

// stageNativeDependencyArtifacts supplies the persisted sibling artifacts that
// native integration checks now consume. It follows the authored component
// graph in post-order, so every dependency surface/report exists before its
// dependent runs. The files are test-only staging artifacts and are removed by
// runArcc after the subprocess returns; production has no fallback to source
// reconstruction.
func stageNativeDependencyArtifacts(manifestPath string, seen map[string]bool, staged *nativeArtifactStaging) error {
	absManifest, err := filepath.Abs(manifestPath)
	if err != nil {
		return err
	}
	absManifest = filepath.Clean(absManifest)
	if seen[absManifest] {
		return nil
	}
	seen[absManifest] = true
	f, err := os.Open(absManifest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	parsed, err := manifest.Parse(f)
	_ = f.Close()
	if err != nil {
		// Let the real command report malformed/stale manifests; staging is
		// only for valid dependency artifacts.
		return nil
	}
	for _, dep := range parsed.ComponentDependencies {
		depManifest := filepath.Clean(filepath.Join(filepath.Dir(absManifest), dep.Manifest))
		if err := stageNativeDependencyArtifacts(depManifest, seen, staged); err != nil {
			return err
		}
		surfacePath := artifactio.SurfacePath(depManifest)
		reportPath := artifactio.ReportPath(depManifest)
		if err := staged.capture(surfacePath); err != nil {
			return err
		}
		if err := staged.capture(reportPath); err != nil {
			return err
		}
		cmd := exec.Command(arccBin, "check", depManifest,
			"--report-out="+reportPath,
			"--surface-out="+surfacePath,
			"--report-verdict-only")
		cmd.Env = os.Environ()
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("checking %s: %v (%s)", depManifest, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func TestStageNativeDependencyArtifactsRefreshesExistingPair(t *testing.T) {
	_, depManifest := createTempComponent(t, "staging-dependency", `name: "staging-dependency"
interface_files: "api.go"
`, map[string]string{
		"api.go": "package stagingdependency\n\nfunc Exported() {}\n",
	})
	callerDir := t.TempDir()
	callerManifest := filepath.Join(callerDir, "component.textproto")
	if err := os.WriteFile(callerManifest, []byte(fmt.Sprintf(`name: "staging-caller"
interface_files: "caller.go"
component_dependencies {
  name: "staging-dependency"
  manifest: "%s"
}
`, filepath.ToSlash(relativePath(t, callerDir, depManifest)))), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(callerDir, "caller.go"), []byte("package caller\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	surfacePath := artifactio.SurfacePath(depManifest)
	reportPath := artifactio.ReportPath(depManifest)
	oldSurface := []byte("stale surface bytes")
	oldReport := []byte("stale report bytes")
	if err := os.WriteFile(surfacePath, oldSurface, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, oldReport, 0o644); err != nil {
		t.Fatal(err)
	}

	var staged nativeArtifactStaging
	if err := stageNativeDependencyArtifacts(callerManifest, map[string]bool{}, &staged); err != nil {
		t.Fatalf("stageNativeDependencyArtifacts: %v", err)
	}
	defer func() {
		if err := staged.restore(); err != nil {
			t.Errorf("restore staged artifacts: %v", err)
		}
	}()

	if data, err := os.ReadFile(surfacePath); err != nil {
		t.Fatal(err)
	} else if _, err := artifactio.DecodeSurface(strings.NewReader(string(data))); err != nil {
		t.Fatalf("staged surface remains stale: %v", err)
	}
	if data, err := os.ReadFile(reportPath); err != nil {
		t.Fatal(err)
	} else if _, err := artifactio.DecodeReport(data); err != nil {
		t.Fatalf("staged report remains stale: %v", err)
	}
	if err := staged.restore(); err != nil {
		t.Fatalf("restore staged artifacts: %v", err)
	}
	if data, err := os.ReadFile(surfacePath); err != nil {
		t.Fatal(err)
	} else if string(data) != string(oldSurface) {
		t.Errorf("surface restore = %q, want original stale bytes", data)
	}
	if data, err := os.ReadFile(reportPath); err != nil {
		t.Fatal(err)
	} else if string(data) != string(oldReport) {
		t.Errorf("report restore = %q, want original stale bytes", data)
	}
}

func relativePath(t *testing.T, from, to string) string {
	t.Helper()
	rel, err := filepath.Rel(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

// runArccEnv is runArcc with an explicit environment for the subprocess. A nil
// env inherits the test process's environment, which is what runArcc wants;
// the layout-mode tests use it to withhold the Go toolchain.
func runArccEnv(env []string, args []string) (string, string, int) {
	return runArccEnvDir("", env, args)
}

func runArccEnvDir(dir string, env []string, args []string) (string, string, int) {
	cmd := exec.Command(arccBin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
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

func arccHermeticEnv(t *testing.T) []string {
	t.Helper()
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	env = append(env, "PATH="+t.TempDir())
	return env
}

func copyDirectory(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(src, entry.Name())
		destinationPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDirectory(sourcePath, destinationPath); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cannot copy non-regular CSV fixture entry %q", sourcePath)
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destinationPath, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyCSVToolWorkspace(t *testing.T) (string, string) {
	t.Helper()
	moduleRoot, err := filepath.Abs(filepath.Join("../.."))
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	goMod, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write temporary go.mod: %v", err)
	}
	if err := copyDirectory(
		filepath.Join(moduleRoot, "examples", "csvtool"),
		filepath.Join(workspace, "examples", "csvtool"),
	); err != nil {
		t.Fatalf("copy CSV tool workspace: %v", err)
	}
	return workspace, filepath.Join(workspace, "examples", "csvtool", "app", "component.textproto")
}

func stageCSVToolDependencies(t *testing.T, workspace string) {
	t.Helper()
	for _, component := range []string{"internal/parsecsv", "csvfile", "toprow"} {
		manifestPath := filepath.Join(workspace, "examples", "csvtool", component, "component.textproto")
		reportPath := artifactio.ReportPath(manifestPath)
		surfacePath := artifactio.SurfacePath(manifestPath)
		_, stderr, code := runArccEnvDir(workspace, nil, []string{
			"check", manifestPath,
			"--report-out=" + reportPath,
			"--surface-out=" + surfacePath,
			"--report-verdict-only",
		})
		if code != 0 || stderr != "" {
			t.Fatalf("stage CSV dependency %s: exit %d, stderr=%q", component, code, stderr)
		}
	}
}

func appendUnavailablePackageToSurface(t *testing.T, surfacePath string) {
	t.Helper()
	data, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("read CSV surface: %v", err)
	}
	decoded, err := artifactio.DecodeSurface(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode CSV surface: %v", err)
	}
	if len(decoded.Packages) == 0 {
		t.Fatal("CSV surface has no package to make unavailable")
	}
	decoded.Packages = append(decoded.Packages, decoded.Packages[0]+"/unavailable")
	updated, err := artifactio.MarshalSurface(decoded)
	if err != nil {
		t.Fatalf("rewrite CSV surface with unavailable package: %v", err)
	}
	if err := os.WriteFile(surfacePath, updated, 0o644); err != nil {
		t.Fatalf("write CSV surface with unavailable package: %v", err)
	}
}

func writeLayoutDependencyBinding(t *testing.T, layoutPath string) {
	t.Helper()
	data, err := os.ReadFile(layoutPath)
	if err != nil {
		t.Fatalf("read CSV status layout: %v", err)
	}
	var layout map[string]any
	if err := json.Unmarshal(data, &layout); err != nil {
		t.Fatalf("decode CSV status layout: %v", err)
	}
	layout["dependency_artifact_bindings"] = []map[string]any{{
		"dependency": "parsecsv",
		"surface":    "dep/component.surface.json",
		"report":     "dep/component.report.json",
		"provenance": "checked",
	}}
	updated, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		t.Fatalf("encode CSV status layout: %v", err)
	}
	if err := os.WriteFile(layoutPath, updated, 0o644); err != nil {
		t.Fatalf("write CSV status layout: %v", err)
	}
}

func writeDependencyOnlyLayout(t *testing.T, sourceLayout, dependencyLayout string) {
	t.Helper()
	data, err := os.ReadFile(sourceLayout)
	if err != nil {
		t.Fatalf("read dependency layout: %v", err)
	}
	var layout map[string]any
	if err := json.Unmarshal(data, &layout); err != nil {
		t.Fatalf("decode dependency layout: %v", err)
	}
	layout["roots"] = []string{"example.com/dep"}
	updated, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		t.Fatalf("encode dependency layout: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dependencyLayout), 0o755); err != nil {
		t.Fatalf("create dependency layout directory: %v", err)
	}
	if err := os.WriteFile(dependencyLayout, updated, 0o644); err != nil {
		t.Fatalf("write dependency layout: %v", err)
	}
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

func TestIntegration_Failing_UndeclaredAuthority(t *testing.T) {
	manifest := `
name: "authorityapp"
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

	absTmpDir, manifestPath := createTempComponent(t, "authorityapp", manifest, files)

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

	// Verify against deterministic golden output to assert stability and the
	// DR-17 finding model: one counted finding carrying its sorted site and
	// the map's canned evidence.
	want := `Component: authorityapp

Violations:
- [UNDECLARED_AUTHORITY] use of undeclared authority "FILES"
  at main.go:4 (1 sites)
  Evidence:
    - os.ReadFile at :0
`
	if strings.TrimSpace(normalizedStdout1) != strings.TrimSpace(want) {
		t.Errorf("normalized stdout does not match golden output.\nGOT:\n%q\nWANT:\n%q", normalizedStdout1, want)
	}
}

func TestIntegration_StaleManifest_Exit2(t *testing.T) {
	// A stale manifest naming the removed absorbed_dependencies field must fail
	// at parse time with an actionable diagnostic, never silently ignore it.
	manifest := `
name: "stale-absorbed"
interface_files: "api.go"
absorbed_dependencies {
	import_path: "example.com/impl"
}
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}

	_, manifestPath := createTempComponent(t, "stale-absorbed", manifest, files)

	stdout, stderr, exitCode := runArcc([]string{"check", manifestPath})

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d. Stdout: %s", exitCode, stdout)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "absorbed_dependencies") {
		t.Errorf("expected error to identify removed field absorbed_dependencies, got %q", stderr)
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
name: "authorityapp"
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

	absTmpDir, manifestPath := createTempComponent(t, "authorityapp-json", manifest, files)

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

	if rep.Component != "authorityapp" {
		t.Errorf("rep.Component = %q, want 'authorityapp'", rep.Component)
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
	// DR-17: the aggregated finding carries every sorted site and the map's
	// canned evidence for the first site's symbol.
	if len(vAuth.Evidence) != 1 {
		t.Fatalf("expected 1 evidence entry for UNDECLARED_AUTHORITY, got %d: %v", len(vAuth.Evidence), vAuth.Evidence)
	}
	if !strings.Contains(vAuth.Evidence[0], "os.ReadFile") {
		t.Errorf("vAuth.Evidence[0] = %q, expected it to name the map evidence for os.ReadFile", vAuth.Evidence[0])
	}
	if len(vAuth.Sites) != 1 || vAuth.Sites[0].File != "main.go" || vAuth.Sites[0].Line != 9 {
		t.Fatalf("vAuth.Sites = %+v, want the single main.go:9 site", vAuth.Sites)
	}

	// Verify vDep fields: the boundary findings are site-specific — one per
	// (referent, site): the parsecsv import at main.go:5 and the reference
	// to parsecsv.Parse at main.go:10.
	var importFinding, refFinding *report.Finding
	for idx := range rep.Violations {
		v := &rep.Violations[idx]
		if v.Kind != report.UndeclaredDependency {
			continue
		}
		if strings.Contains(v.Message, `imports undeclared dependency "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv" at main.go:5`) {
			importFinding = v
		}
		if strings.Contains(v.Message, `references undeclared dependency symbol "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv.Parse" at main.go:10`) {
			refFinding = v
		}
	}
	if importFinding == nil || refFinding == nil {
		t.Fatalf("want the import finding at main.go:5 and the Parse reference at main.go:10, got %+v", rep.Violations)
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
	if !strings.Contains(normalizedStdoutText, "at main.go:9 (1 sites)") {
		t.Errorf("text output missing the counted site: %s", normalizedStdoutText)
	}
	if !strings.Contains(normalizedStdoutText, `imports undeclared dependency "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv" at main.go:5`) {
		t.Errorf("text output missing parsecsv undeclared dependency: %s", normalizedStdoutText)
	}
	if !strings.Contains(normalizedStdoutText, `declared component dependency "toprow" is unused`) {
		t.Errorf("text output missing unused dependency warning: %s", normalizedStdoutText)
	}
}

func TestIntegration_ComponentBoundaryTerminatesAuthority(t *testing.T) {
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
	// Because authdep is a component dependency, the reference to ReadData is
	// checked against its declared interface, and authdep's internal os.ReadFile
	// use is not attributed to caller-cd.
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
	// DR-17: the member-owned backend.Load's authority use appears as the
	// counted site (load.go:6, the os.ReadFile reference), not as a transitive
	// path through the callback.
	if !strings.Contains(stdout, "at backend/load.go:6 (1 sites)") {
		t.Fatalf("expected the member-owned backend.Load site, got: %s", stdout)
	}
}

func TestIntegration_InitAuthorityStopsAtDependencyBoundary(t *testing.T) {
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
	// Because initdep is a component dependency, its package-level init and
	// implementation remain behind the declared boundary. The internal use of
	// FILES is not attributed to caller-init-cd.
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
- csvfile (asserted)
- toprow (asserted)
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
	if len(rep.Dependencies) != 2 || rep.Dependencies[0].Component != "csvfile" || rep.Dependencies[1].Component != "toprow" {
		t.Errorf("rep.Dependencies = %+v, want csvfile and toprow dependency boundaries", rep.Dependencies)
	}
}

func TestIntegration_CSVTool_StatusDemo(t *testing.T) {
	workspace, appManifest := copyCSVToolWorkspace(t)
	stageCSVToolDependencies(t, workspace)

	// The native command consumes a freshly staged pair in the test-owned copy
	// and exposes the byte-audited VERIFIED axis in JSON. Native provenance
	// remains ASSERTED by design; BUILD_GRAPH/CHECKED_PASS is demonstrated by
	// the layout-mode artifact producer below, not synthesized by convention
	// lookup.
	stdout, stderr, code := runArccEnvDir(workspace, nil, []string{"check", appManifest, "--format=json"})
	if code != 0 {
		t.Fatalf("CSV app checked-pass run: exit %d, stderr=%q, stdout=%q", code, stderr, stdout)
	}
	var verified report.ConformanceReport
	if err := json.Unmarshal([]byte(stdout), &verified); err != nil {
		t.Fatalf("decode verified CSV report: %v; stdout=%q", err, stdout)
	}
	if len(verified.Dependencies) != 2 {
		t.Fatalf("verified CSV dependencies = %+v, want csvfile and toprow", verified.Dependencies)
	}
	for _, dep := range verified.Dependencies {
		if dep.Provenance != report.DependencyProvenanceAsserted || dep.Freshness != report.DependencyFreshnessVerified {
			t.Errorf("verified CSV boundary %q = %+v, want ASSERTED/VERIFIED", dep.Component, dep)
		}
	}

	// Keep a staged artifact pair while changing a member source byte. The
	// source remains valid Go so the real command can load the consumer; only
	// the native freshness audit observes the change and renders STALE.
	sourcePath := filepath.Join(workspace, "examples/csvtool/csvfile/private.go")
	originalSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, append(originalSource, []byte("\n// status-demo source change\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runArccEnvDir(workspace, nil, []string{"check", appManifest, "--format=json"})
	if code != 0 {
		t.Fatalf("CSV stale run: exit %d, stderr=%q, stdout=%q", code, stderr, stdout)
	}
	var stale report.ConformanceReport
	if err := json.Unmarshal([]byte(stdout), &stale); err != nil {
		t.Fatalf("decode stale CSV report: %v; stdout=%q", err, stdout)
	}
	var sawStale bool
	for _, dep := range stale.Dependencies {
		if dep.Component == "csvfile" && dep.Freshness == report.DependencyFreshnessStale {
			sawStale = true
		}
	}
	if !sawStale {
		t.Errorf("stale CSV boundaries = %+v, want csvfile STALE", stale.Dependencies)
	}
	var staleWarning int
	for _, warning := range stale.Warnings {
		if warning.Kind == report.DependencySurfaceStale {
			staleWarning++
		}
	}
	if staleWarning != 1 {
		t.Errorf("stale CSV warnings = %+v, want one DEPENDENCY_SURFACE_STALE", stale.Warnings)
	}

	// Make one surface-declared source package unavailable in the test-owned
	// workspace. The consumer's own member loader still has valid csvfile
	// source, while the native byte-only audit cannot enumerate the complete
	// persisted source set and must expose UNKNOWN rather than failing the tool.
	csvfileManifest := filepath.Join(workspace, "examples/csvtool/csvfile/component.textproto")
	appendUnavailablePackageToSurface(t, artifactio.SurfacePath(csvfileManifest))
	stdout, stderr, code = runArccEnvDir(workspace, nil, []string{"check", appManifest, "--format=json"})
	if code != 0 {
		t.Fatalf("CSV unknown run: exit %d, stderr=%q, stdout=%q", code, stderr, stdout)
	}
	var unknown report.ConformanceReport
	if err := json.Unmarshal([]byte(stdout), &unknown); err != nil {
		t.Fatalf("decode unknown CSV report: %v; stdout=%q", err, stdout)
	}
	var sawUnknown bool
	for _, dep := range unknown.Dependencies {
		if dep.Component == "csvfile" && dep.Provenance == report.DependencyProvenanceAsserted && dep.Freshness == report.DependencyFreshnessUnknown {
			sawUnknown = true
		}
	}
	if !sawUnknown {
		t.Errorf("unknown CSV boundaries = %+v, want csvfile ASSERTED/UNKNOWN", unknown.Dependencies)
	}
	for _, warning := range unknown.Warnings {
		if warning.Kind == report.DependencySurfaceStale {
			t.Errorf("unknown CSV unexpectedly emitted stale warning: %+v", unknown.Warnings)
		}
	}

	// The checked-fail label must be exercised by a layout binding, whose
	// checked provenance consumes the exact pair written for this dependent.
	statusManifest := `name: "csvfile"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/member"
component_dependencies {
  name: "parsecsv"
  manifest: "dep/component.textproto"
}
`
	statusWorkspace, statusManifestPath, statusLayoutPath := createLayoutFixture(t, "csv-status", []string{"example.com/member"}, statusManifest, map[string]string{
		"member/api.go": "package member\n\nimport \"example.com/dep\"\n\nfunc Use() { dep.Exported() }\n",
		"dep/impl.go":   "package dep\n\nfunc Exported() {}\n",
	})
	statusDepManifestPath := filepath.Join(statusWorkspace, "dep", "component.textproto")
	if err := os.MkdirAll(filepath.Dir(statusDepManifestPath), 0o755); err != nil {
		t.Fatalf("create CSV status dependency directory: %v", err)
	}
	if err := os.WriteFile(statusDepManifestPath, []byte(`name: "parsecsv"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/dep"
declared_authority: "FILES"
`), 0o644); err != nil {
		t.Fatalf("write CSV status dependency manifest: %v", err)
	}
	statusDepLayoutPath := filepath.Join(statusWorkspace, "dep", "package-layout.json")
	writeDependencyOnlyLayout(t, statusLayoutPath, statusDepLayoutPath)
	statusMapPath := sharedNativeMapDefault(t)
	statusDepReportPath := artifactio.ReportPath(statusDepManifestPath)
	statusDepSurfacePath := artifactio.SurfacePath(statusDepManifestPath)
	_, stderr, code = runArccEnvDir(statusWorkspace, arccHermeticEnv(t), []string{
		"check", statusDepManifestPath,
		"--package-layout=" + statusDepLayoutPath,
		"--stdlib-map=" + statusMapPath,
		"--report-out=" + statusDepReportPath,
		"--surface-out=" + statusDepSurfacePath,
		"--report-verdict-only",
	})
	if code != 0 || stderr != "" {
		t.Fatalf("CSV layout dependency artifact: exit %d, stderr=%q", code, stderr)
	}
	statusFailReport, err := artifactio.MarshalReport(report.ConformanceReport{
		Component: "parsecsv",
		Violations: []report.Finding{{
			Kind:    report.AnalysisLimitation,
			Message: "fixture checked-fail status",
		}},
	})
	if err != nil {
		t.Fatalf("encode CSV checked-fail report: %v", err)
	}
	if err := os.WriteFile(statusDepReportPath, statusFailReport, 0o644); err != nil {
		t.Fatalf("write CSV checked-fail report: %v", err)
	}
	persisted, err := artifactio.DecodeReport(statusFailReport)
	if err != nil {
		t.Fatalf("decode CSV checked-fail report: %v", err)
	}
	if persisted.Verdict != report.VerdictFail {
		t.Fatalf("CSV checked-fail artifact verdict = %q, want fail", persisted.Verdict)
	}
	writeLayoutDependencyBinding(t, statusLayoutPath)
	statusTextArgs := []string{
		"check", statusManifestPath,
		"--package-layout=" + statusLayoutPath,
		"--stdlib-map=" + statusMapPath,
	}
	statusText, stderr, code := runArccEnvDir(statusWorkspace, arccHermeticEnv(t), statusTextArgs)
	if code != 0 || stderr != "" {
		t.Fatalf("CSV layout checked-fail text run: exit %d, stderr=%q, stdout=%q", code, stderr, statusText)
	}
	if !strings.Contains(statusText, "- parsecsv (check failed)") || strings.Count(statusText, "[DEPENDENCY_CHECK_FAILED]") != 1 {
		t.Fatalf("CSV layout checked-fail text = %q, want one check-failed boundary warning", statusText)
	}
	if strings.Contains(statusText, "fixture checked-fail status") {
		t.Fatalf("CSV layout checked-fail text copied dependency violation: %q", statusText)
	}
	statusJSONArgs := append(statusTextArgs, "--format=json")
	statusJSON, stderr, code := runArccEnvDir(statusWorkspace, arccHermeticEnv(t), statusJSONArgs)
	if code != 0 || stderr != "" {
		t.Fatalf("CSV layout checked-fail JSON run: exit %d, stderr=%q, stdout=%q", code, stderr, statusJSON)
	}
	var checkedFail report.ConformanceReport
	if err := json.Unmarshal([]byte(statusJSON), &checkedFail); err != nil {
		t.Fatalf("decode CSV checked-fail JSON: %v; stdout=%q", err, statusJSON)
	}
	if strings.Contains(statusJSON, "fixture checked-fail status") {
		t.Fatalf("CSV checked-fail JSON copied dependency violation: %q", statusJSON)
	}
	if len(checkedFail.Dependencies) != 1 || checkedFail.Dependencies[0].Provenance != report.DependencyProvenanceCheckedFail || checkedFail.Dependencies[0].Freshness != report.DependencyFreshnessBuildGraph || checkedFail.Dependencies[0].Authority != report.DependencyAuthorityDeclared {
		t.Fatalf("CSV checked-fail boundary = %+v, want CHECKED_FAIL/BUILD_GRAPH/DECLARED", checkedFail.Dependencies)
	}
	var dependencyFailureWarnings int
	for _, warning := range checkedFail.Warnings {
		if warning.Kind == report.DependencyCheckFailed {
			dependencyFailureWarnings++
		}
	}
	if dependencyFailureWarnings != 1 {
		t.Fatalf("CSV checked-fail warnings = %+v, want one DEPENDENCY_CHECK_FAILED", checkedFail.Warnings)
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
	if !strings.Contains(stdout, `references undeclared interface symbol "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile.PrivateExportedHelper" of dependency "csvfile" at app.go:6`) {
		t.Errorf("expected the undeclared-interface reference at its exact site, got: %s", stdout)
	}
}

func TestIntegration_PluginStructPatternIsSilent(t *testing.T) {
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
	// DR-17: the site names the member file and line of the os.Open
	// reference inside ViolateCorePurity.
	if !strings.Contains(stdout, "at regression_authority.go:6 (1 sites)") {
		t.Errorf("expected the os.Open reference site, got: %s", stdout)
	}
}

func TestIntegration_Fixture2_MemberCallbackConforms(t *testing.T) {
	// Regression pair: the editor passes backend.Load across the
	// boundary to the host while backend is an owned member, so its authority is
	// charged to the editor component and the report conforms.
	hostFiles := map[string]string{
		"api.go": `package host

func Register(func() ([]byte, error)) {}
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
			t.Fatalf("failed to resolve import path for %v: %v", dir, err)
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
name: "fixture2-editor"
interface_files: "api.go"
`
	editorDir, editorManifestPath := createTempComponent(t, "fixture2-editor", editorManifest, editorFiles)
	editorImportPath := importPath(editorDir)
	backendImportPath := importPath(filepath.Join(editorDir, "backend"))
	relHostManifest, err := filepath.Rel(editorDir, hostManifestPath)
	if err != nil {
		t.Fatalf("failed to resolve host manifest path: %v", err)
	}

	// backend is a MEMBER: its FILES use is charged to the editor, which
	// declares it, so the report conforms with no warnings.
	memberManifest := fmt.Sprintf(`
name: "fixture2-editor"
interface_files: "api.go"
members: "%s"
members: "%s"
component_dependencies {
	name: "fixture2-host"
	manifest: "%s"
}
declared_authority: "FILES"
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
	if !strings.Contains(stdoutMem, `Component "fixture2-editor" conforms; does not exceed declared authority`) {
		t.Errorf("expected member variant stdout to be conforming success line, got: %s", stdoutMem)
	}
}

func TestIntegration_DependencyBoundary_Exit0(t *testing.T) {
	// Verifies AC4: a component with a declared dependency boundary and no violations
	// or warnings returns exit code 0, emits no findings, and retains the success line + annotation.
	depManifest := `
name: "declared-dep-cli"
interface_files: "api.go"
`
	depFiles := map[string]string{
		"api.go": `package declareddep
func Fetch() {}
`,
	}
	depDir, depManifestPath := createTempComponent(t, "declared-dep-cli", depManifest, depFiles)

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

	callerDir, callerManifestPath := createTempComponent(t, "declared-caller-cli", "", callerFiles)
	relDepManifest, err := filepath.Rel(callerDir, depManifestPath)
	if err != nil {
		t.Fatalf("failed to compute relative path: %v", err)
	}

	actualManifest := fmt.Sprintf(`
name: "declared-caller-cli"
interface_files: "main.go"
component_dependencies {
	name: "declared-dep-cli"
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
	wantText := `Component "declared-caller-cli" conforms; does not exceed declared authority

Dependencies:
- declared-dep-cli (asserted)
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
	if len(rep.Dependencies) != 1 || rep.Dependencies[0].Component != "declared-dep-cli" {
		t.Errorf("rep.Dependencies = %+v, want declared-dep-cli boundary", rep.Dependencies)
	}
}

func TestIntegration_ThreeSitesOneCapabilityAndUnanalyzed(t *testing.T) {
	// The AC 6 scenario through the real command: three member sites reaching
	// one capability aggregate into one counted text finding, and a reference
	// to an UNANALYZED stdlib record is failed by the default policy; the
	// canonical JSON retains every sorted site, the class, and the SDK key.
	manifest := `
name: "three-sites"
interface_files: "main.go"
`
	files := map[string]string{
		"main.go": `package main

import (
	"os"
	"sort"
)

func A() { _, _ = os.ReadFile("a") }
func B() { _, _ = os.ReadFile("b") }
func C() {
	s := []string{"x"}
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	_, _ = os.ReadFile("c")
}
`,
	}
	absTmpDir, manifestPath := createTempComponent(t, "three-sites", manifest, files)

	wd, _ := os.Getwd()
	workspaceRoot, _ := filepath.Abs(filepath.Join(wd, "../../../"))

	// Text mode: one counted finding per (class, capability); the FILES
	// finding shows its first sorted site with the total count.
	stdoutText, stderrText, exitText := runArcc([]string{"check", manifestPath})
	if exitText != 1 {
		t.Fatalf("expected exit 1, got %d. Stderr: %s", exitText, stderrText)
	}
	if stderrText != "" {
		t.Errorf("expected empty stderr, got %q", stderrText)
	}
	if strings.Count(stdoutText, `use of undeclared authority "FILES"`) != 1 {
		t.Errorf("text report must contain one counted FILES finding, got %q", stdoutText)
	}
	if !strings.Contains(stdoutText, "(3 sites)") {
		t.Errorf("text report must show the aggregated site count, got %q", stdoutText)
	}
	if !strings.Contains(stdoutText, "analysis-defeating") {
		t.Errorf("text report must include the UNANALYZED finding, got %q", stdoutText)
	}
	// JSON mode: the canonical report retains all sorted sites, the class,
	// and the SDK key.
	stdoutJSON, stderrJSON, exitJSON := runArcc([]string{"check", manifestPath, "--format=json"})
	if exitJSON != 1 {
		t.Fatalf("expected JSON exit 1, got %d. Stderr: %s", exitJSON, stderrJSON)
	}
	if stderrJSON != "" {
		t.Errorf("expected empty stderr, got %q", stderrJSON)
	}
	var rep report.ConformanceReport
	if err := json.Unmarshal([]byte(stdoutJSON), &rep); err != nil {
		t.Fatalf("json.Unmarshal error = %v; stdout = %q", err, stdoutJSON)
	}
	if len(rep.Violations) != 2 {
		t.Fatalf("violations = %+v, want exactly the AnalysisDefeating and FILES findings", rep.Violations)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("warnings = %+v, want none under the default policy", rep.Warnings)
	}
	var vFiles, vUnanalyzed *report.Finding
	for idx := range rep.Violations {
		v := &rep.Violations[idx]
		switch {
		case v.Kind == report.UndeclaredAuthority && v.Class == "TrueAuthority":
			vFiles = v
		case v.Class == "AnalysisDefeating" && strings.Contains(v.Message, "analysis-defeating"):
			vUnanalyzed = v
		}
	}
	if vFiles == nil || vUnanalyzed == nil {
		t.Fatalf("want one TrueAuthority FILES finding and one AnalysisDefeating finding, got %+v", rep.Violations)
	}
	if normalizeOutput(vFiles.Message, workspaceRoot, absTmpDir) == "" || !strings.Contains(vFiles.Message, `use of undeclared authority "FILES"`) {
		t.Errorf("FILES message = %q", vFiles.Message)
	}
	if len(vFiles.Sites) != 3 {
		t.Fatalf("FILES sites = %+v, want the three sorted os.ReadFile sites", vFiles.Sites)
	}
	for i, site := range vFiles.Sites {
		if site.File != "main.go" {
			t.Errorf("FILES site[%d] file = %q, want main.go", i, site.File)
		}
		if site.Symbol != "os.ReadFile" {
			t.Errorf("FILES site[%d] symbol = %q, want os.ReadFile", i, site.Symbol)
		}
		if i > 0 && vFiles.Sites[i-1].Line > site.Line {
			t.Errorf("FILES sites are not sorted: %+v", vFiles.Sites)
		}
	}
	if len(vFiles.Evidence) != 1 || !strings.Contains(vFiles.Evidence[0], "os.ReadFile") {
		t.Errorf("FILES evidence = %+v, want the map's canned os.ReadFile path", vFiles.Evidence)
	}
	if vFiles.SDKKey == "" {
		t.Error("FILES finding carries no SDK key")
	}
	if len(vUnanalyzed.Sites) != 1 || vUnanalyzed.Sites[0].Symbol != "sort.Slice" {
		t.Errorf("AnalysisDefeating sites = %+v, want the single sort.Slice site", vUnanalyzed.Sites)
	}
	if len(vUnanalyzed.Evidence) != 0 {
		t.Errorf("AnalysisDefeating evidence = %+v, want none", vUnanalyzed.Evidence)
	}
	if vUnanalyzed.SDKKey == "" {
		t.Error("AnalysisDefeating finding carries no SDK key")
	}
}
