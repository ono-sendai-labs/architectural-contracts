//go:build integration

package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
)

// writePinnedLayoutFixture writes a package-layout JSON for workspace whose
// roots are the given package paths, with a pinned platform derived from the
// host toolchain's target configuration, so the declared stdlib map's key can
// be validated against the layout's target.
func writePinnedLayoutFixture(t *testing.T, workspace string, roots []string, target stdlibmap.TargetConfig) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/artifacts\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packages := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		name := root[strings.LastIndex(root, "/")+1:]
		relFile := filepath.ToSlash(filepath.Join(name, name+".go"))
		if err := os.MkdirAll(filepath.Dir(filepath.Join(workspace, relFile)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, relFile), []byte("package "+name+"\n\nfunc Exported() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		packages = append(packages, map[string]any{
			"id": root, "name": name, "pkgPath": root, "is_stdlib": false,
			"goFiles": []string{relFile}, "compiledGoFiles": []string{relFile}, "imports": map[string]string{},
		})
	}
	platform := map[string]any{
		"goos": target.GOOS, "goarch": target.GOARCH,
		"build_tags": target.BuildTags, "cgo_enabled": target.CgoEnabled,
		"toolchain_version": target.ToolchainVersion,
	}
	if target.GOEXPERIMENT != "" {
		platform["goexperiment"] = target.GOEXPERIMENT
	}
	layout := map[string]any{"roots": roots, "packages": packages, "platform": platform}
	data, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	layoutPath := filepath.Join(workspace, "package-layout.json")
	if err := os.WriteFile(layoutPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return layoutPath
}

func writeLayoutManifest(t *testing.T, workspace, pkgPath string) string {
	t.Helper()
	name := pkgPath[strings.LastIndex(pkgPath, "/")+1:]
	manifest := "name: \"" + name + "\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \"" + pkgPath + "\"\n"
	manifestPath := filepath.Join(workspace, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifestPath
}

// fullRunner constructs the production runner exactly as main.go does.
func fullRunner() *app.Runner {
	return &app.Runner{
		Loader:   goanalysis.LoadPackageFacts,
		Analyzer: capslockadapter.NewAdapter(),
	}
}

func generateNativeMap(t *testing.T, path string) {
	t.Helper()
	runner := &app.Runner{}
	var stdout, stderr strings.Builder
	if code := runner.Run([]string{"stdlibmap", "generate", "--output=" + path}, &stdout, &stderr); code != 0 {
		t.Fatalf("stdlibmap generate: exit %d, stderr: %s", code, stderr.String())
	}
}

// TestCheck_EmitsArtifactsLayoutMode is the layout-mode leg (ACs 1, 4, 5): a
// check with a pinned layout and the declared stdlib map emits the canonical
// report and exact surface, byte-identically across repeated invocations; a
// corrupt declared map and a missing declared map fail closed with exit 2.
func TestCheck_EmitsArtifactsLayoutMode(t *testing.T) {
	target, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		t.Fatalf("NativeTargetConfig: %v", err)
	}

	workspace := t.TempDir()
	layoutPath := writePinnedLayoutFixture(t, workspace, []string{"example.com/artifacts/comp"}, target)
	manifestPath := writeLayoutManifest(t, workspace, "example.com/artifacts/comp")

	mapPath := filepath.Join(t.TempDir(), "map.json")
	generateNativeMap(t, mapPath)

	runner := fullRunner()
	reportPath := filepath.Join(workspace, "comp.report.json")
	surfacePath := filepath.Join(workspace, "comp.surface.json")
	args := []string{
		"check", manifestPath,
		"--package-layout=" + layoutPath,
		"--report-out=" + reportPath,
		"--surface-out=" + surfacePath,
		"--stdlib-map=" + mapPath,
	}

	runCheck := func(args []string) (int, string) {
		return runRunnerFromWorkspace2(t, workspace, runner, args)
	}

	code, stderr := runCheck(args)
	if code != 0 {
		t.Fatalf("layout check: exit %d, stderr: %s", code, stderr)
	}
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report artifact: %v", err)
	}
	surfaceData, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("surface artifact: %v", err)
	}
	persisted, err := artifactio.DecodeReport(reportData)
	if err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if persisted.Verdict != "pass" {
		t.Errorf("verdict = %q, want pass", persisted.Verdict)
	}
	surface, err := artifactio.DecodeSurface(strings.NewReader(string(surfaceData)))
	if err != nil {
		t.Fatalf("decode surface: %v", err)
	}
	if len(surface.Packages) != 1 || surface.Packages[0] != "example.com/artifacts/comp" {
		t.Errorf("surface packages = %v, want the layout member", surface.Packages)
	}
	if surface.SdkKey.GetToolchainVersion() != target.ToolchainVersion {
		t.Errorf("surface key toolchain = %q, want %q", surface.SdkKey.GetToolchainVersion(), target.ToolchainVersion)
	}

	// Determinism: an unchanged invocation produces identical bytes.
	reportData2 := overwriteAndRerun(t, runner, args, reportPath)
	surfaceData2 := overwriteAndRerun(t, runner, args, surfacePath)
	if string(reportData) != string(reportData2) {
		t.Error("report artifact bytes differ between identical invocations")
	}
	if string(surfaceData) != string(surfaceData2) {
		t.Error("surface artifact bytes differ between identical invocations")
	}

	// Corrupt declared map: exit 2 with context.
	mapBytes, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(corruptPath, mapBytes[:64], 0o644); err != nil {
		t.Fatal(err)
	}
	code, stderr = runRunnerFromWorkspace2(t, workspace, runner, withReplaced(args, mapPath, corruptPath))
	if code != 2 {
		t.Errorf("corrupt map exit = %d, want 2; stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "stdlib-map") && !strings.Contains(stderr, "stdlib-map artifact") {
		t.Errorf("corrupt map stderr = %q, want it to name the artifact", stderr)
	}

	// Missing declared map: exit 2 with context.
	code, stderr = runRunnerFromWorkspace2(t, workspace, runner, withReplaced(args, mapPath, filepath.Join(t.TempDir(), "absent.json")))
	if code != 2 {
		t.Errorf("missing map exit = %d, want 2; stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "absent.json") {
		t.Errorf("missing map stderr = %q, want it to name the path", stderr)
	}
}

// TestCheck_EmitsArtifactsNativeMode is the native leg (ACs 1, 5): a real
// loaded component with --stdlib-map publishes both artifacts deterministically.
func TestCheck_EmitsArtifactsNativeMode(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/natart\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	compDir := filepath.Join(workspace, "comp")
	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "comp.go"), []byte("package comp\n\nfunc Exported() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(compDir, "component.textproto")
	manifest := "name: \"natart\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \"example.com/natart/comp\"\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	mapPath := filepath.Join(t.TempDir(), "map.json")
	generateNativeMap(t, mapPath)

	runner := fullRunner()
	reportPath := filepath.Join(compDir, "natart.report.json")
	surfacePath := filepath.Join(compDir, "natart.surface.json")
	args := []string{
		"check", manifestPath,
		"--report-out=" + reportPath,
		"--surface-out=" + surfacePath,
		"--stdlib-map=" + mapPath,
	}
	code, _ := runRunnerFromWorkspace2(t, workspace, runner, args)
	if code != 0 {
		t.Fatalf("native check: exit %d", code)
	}
	surfaceData, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("surface artifact: %v", err)
	}
	surface, err := artifactio.DecodeSurface(strings.NewReader(string(surfaceData)))
	if err != nil {
		t.Fatalf("decode surface: %v", err)
	}
	if len(surface.Packages) != 1 || surface.Packages[0] != "example.com/natart/comp" {
		t.Errorf("surface packages = %v", surface.Packages)
	}
	if len(surface.Symbols) != 0 {
		t.Errorf("PACKAGE_SURFACE surface declares %d symbols, want none", len(surface.Symbols))
	}
	if surface.Digest == "" {
		t.Error("surface digest is empty")
	}

	// Determinism.
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := runRunnerFromWorkspace2(t, workspace, runner, args); code != 0 {
		t.Fatalf("repeat native check: exit %d", code)
	}
	surfaceData2, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(surfaceData) != string(surfaceData2) {
		t.Error("surface bytes differ between identical invocations")
	}
	reportData2, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(reportData) != string(reportData2) {
		t.Error("report bytes differ between identical invocations")
	}
}

// runRunnerFromWorkspace2 is the shared workspace-chdir helper; goanalysis's
// layout mode and native loading both resolve paths relative to the process
// working directory, so checks run with the workspace as cwd.
func runRunnerFromWorkspace2(t *testing.T, workspace string, runner *app.Runner, args []string) (int, string) {
	t.Helper()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	var stdout, stderr strings.Builder
	code := runner.Run(args, &stdout, &stderr)
	return code, stderr.String()
}

func overwriteAndRerun(t *testing.T, runner *app.Runner, args []string, path string) []byte {
	t.Helper()
	if err := os.WriteFile(path, []byte("trashed"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	if code := runner.Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("repeat check: exit %d, stderr: %s", code, stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func withReplaced(args []string, old, new string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == old {
			a = new
		}
		if a == "--stdlib-map="+old {
			a = "--stdlib-map=" + new
		}
		out = append(out, a)
	}
	return out
}
