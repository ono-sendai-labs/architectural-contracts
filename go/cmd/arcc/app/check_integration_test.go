//go:build integration

package app_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/teststdlibmap"
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

// writeUnpinnedLayoutFixture writes a package-layout JSON with no platform
// identity: the layout pins no target, so the stdlib map's own SDK key
// (including its completeness) is the only target contract available.
func writeUnpinnedLayoutFixture(t *testing.T, workspace string, roots []string) string {
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
	layout := map[string]any{"roots": roots, "packages": packages}
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
		Loader: goanalysis.LoadPackageFacts,
	}
}

func writePinnedMap(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, teststdlibmap.PinnedBytes(), 0o644); err != nil {
		t.Fatalf("copy pinned stdlib map: %v", err)
	}
}

type invalidLayoutMapCase struct {
	name           string
	fixture        func(*testing.T, stdlibmap.TargetConfig) (string, string, string)
	mapPath        func(*testing.T) string
	wantDiagnostic []string
}

func standardLayoutMapFixture(t *testing.T, target stdlibmap.TargetConfig) (string, string, string) {
	t.Helper()
	workspace := t.TempDir()
	const member = "example.com/artifacts/comp"
	layoutPath := writePinnedLayoutFixture(t, workspace, []string{member}, target)
	manifestPath := writeLayoutManifest(t, workspace, member)
	return workspace, manifestPath, layoutPath
}

// inventoryGapLayoutFixture is deliberately small: it gives the production
// layout driver one member package and one SDK package whose source is enough
// to resolve fmt.Sprintf. The pinned map supplies the real target key and
// inventory; the test removes that one symbol to reach checker classification's
// fail-closed inventory lookup without generating the SDK.
func inventoryGapLayoutFixture(t *testing.T, target stdlibmap.TargetConfig) (string, string, string) {
	t.Helper()
	workspace := t.TempDir()
	const member = "example.com/inventory/component"
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/inventory\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	memberDir := filepath.Join(workspace, "component")
	if err := os.MkdirAll(memberDir, 0o755); err != nil {
		t.Fatal(err)
	}
	memberSource := "package component\n\nimport \"fmt\"\n\nfunc Exported() string {\n\treturn fmt.Sprintf(\"inventory gap\")\n}\n"
	if err := os.WriteFile(filepath.Join(memberDir, "component.go"), []byte(memberSource), 0o644); err != nil {
		t.Fatal(err)
	}

	sdkRoot := filepath.Join(workspace, "mock-sdk", "src")
	fmtDir := filepath.Join(sdkRoot, "fmt")
	if err := os.MkdirAll(fmtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fmtSource := "package fmt\n\nfunc Sprintf(format string, args ...any) string {\n\treturn format\n}\n"
	if err := os.WriteFile(filepath.Join(fmtDir, "fmt.go"), []byte(fmtSource), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(workspace, "component.textproto")
	manifest := "name: \"inventory-gap\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \"" + member + "\"\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	platform := map[string]any{
		"goos": target.GOOS, "goarch": target.GOARCH,
		"build_tags": target.BuildTags, "cgo_enabled": target.CgoEnabled,
		"toolchain_version": target.ToolchainVersion,
	}
	if target.GOEXPERIMENT != "" {
		platform["goexperiment"] = target.GOEXPERIMENT
	}
	layout := map[string]any{
		"go_sdk_root": sdkRoot,
		"platform":    platform,
		"roots":       []string{member},
		"packages": []map[string]any{
			{
				"id": member, "name": "component", "pkgPath": member, "is_stdlib": false,
				"goFiles": []string{"component/component.go"}, "compiledGoFiles": []string{"component/component.go"},
				"imports": map[string]string{},
			},
			{
				"id": "fmt", "name": "fmt", "pkgPath": "fmt", "is_stdlib": true,
				"goFiles": []string{"fmt/fmt.go"}, "compiledGoFiles": []string{"fmt/fmt.go"},
				"imports": map[string]string{},
			},
		},
	}
	data, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	layoutPath := filepath.Join(workspace, "package-layout.json")
	if err := os.WriteFile(layoutPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace, manifestPath, layoutPath
}

func writeMapBytes(t *testing.T, filename string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write stdlib map %s: %v", path, err)
	}
	return path
}

func writeCorruptPinnedMap(t *testing.T) string {
	t.Helper()
	data := teststdlibmap.PinnedBytes()
	if len(data) < 64 {
		t.Fatalf("pinned map is only %d bytes; corrupt fixture needs a prefix", len(data))
	}
	return writeMapBytes(t, "corrupt.stdlib-map.json", data[:64])
}

func writePinnedMapVariant(t *testing.T, filename string, mutate func(map[string]any)) string {
	t.Helper()
	// Mutate the checked artifact's JSON shape so each case preserves all
	// unrelated validity and reaches its intended production validation branch.
	decoded := map[string]any{}
	if err := json.Unmarshal(teststdlibmap.PinnedBytes(), &decoded); err != nil {
		t.Fatalf("decode pinned stdlib map: %v", err)
	}
	mutate(decoded)
	data, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("encode stdlib map variant: %v", err)
	}
	return writeMapBytes(t, filename, data)
}

func writePinnedMapKeyVariant(t *testing.T, filename, field string, value any) string {
	t.Helper()
	return writePinnedMapVariant(t, filename, func(decoded map[string]any) {
		key, ok := decoded["key"].(map[string]any)
		if !ok {
			t.Fatalf("pinned stdlib map has no SDK key object")
		}
		key[field] = value
	})
}

func removePinnedMapSymbol(t *testing.T, filename, symbol string) string {
	t.Helper()
	return writePinnedMapVariant(t, filename, func(decoded map[string]any) {
		records, ok := decoded["symbols"].([]any)
		if !ok {
			t.Fatalf("pinned stdlib map has no symbol inventory")
		}
		remaining := make([]any, 0, len(records)-1)
		removed := false
		for _, raw := range records {
			record, ok := raw.(map[string]any)
			if ok && record["id"] == symbol {
				removed = true
				continue
			}
			remaining = append(remaining, raw)
		}
		if !removed {
			t.Fatalf("pinned stdlib map has no symbol %q", symbol)
		}
		decoded["symbols"] = remaining
	})
}

func runInvalidLayoutMapCase(t *testing.T, target stdlibmap.TargetConfig, tt invalidLayoutMapCase) {
	t.Helper()
	workspace, manifestPath, layoutPath := tt.fixture(t, target)
	mapPath := tt.mapPath(t)
	outputDir := t.TempDir()
	reportPath := filepath.Join(outputDir, "report.json")
	surfacePath := filepath.Join(outputDir, "surface.json")
	assertArtifactAbsent(t, reportPath, "before invocation")
	assertArtifactAbsent(t, surfacePath, "before invocation")

	args := []string{
		"check", manifestPath,
		"--package-layout=" + layoutPath,
		"--report-out=" + reportPath,
		"--surface-out=" + surfacePath,
		"--stdlib-map=" + mapPath,
	}
	code, stderr := runRunnerFromWorkspace2(t, workspace, fullRunner(), args)
	if code != 2 {
		t.Errorf("%s exit = %d, want 2; stderr = %s", tt.name, code, stderr)
	}
	for _, piece := range tt.wantDiagnostic {
		if !strings.Contains(stderr, piece) {
			t.Errorf("%s stderr = %q, want diagnostic fragment %q", tt.name, stderr, piece)
		}
	}
	assertArtifactAbsent(t, reportPath, "after failed invocation")
	assertArtifactAbsent(t, surfacePath, "after failed invocation")
}

func assertArtifactAbsent(t *testing.T, path, phase string) {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		t.Errorf("artifact %s exists %s", path, phase)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat artifact %s %s: %v", path, phase, err)
	}
}

// TestCheck_EmitsArtifactsLayoutMode is the successful layout-mode leg: a
// check with a pinned layout and the declared stdlib map emits the canonical
// report and exact surface, byte-identically across repeated invocations.
func TestCheck_EmitsArtifactsLayoutMode(t *testing.T) {
	target, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		t.Fatalf("NativeTargetConfig: %v", err)
	}

	workspace := t.TempDir()
	layoutPath := writePinnedLayoutFixture(t, workspace, []string{"example.com/artifacts/comp"}, target)
	manifestPath := writeLayoutManifest(t, workspace, "example.com/artifacts/comp")

	mapPath := filepath.Join(t.TempDir(), "map.json")
	writePinnedMap(t, mapPath)

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
}

// TestCheck_LayoutStdlibMapFailuresPublishNothing drives every required
// fail-closed map fault through the production Runner and gives each row fresh
// report/surface destinations, independent of successful-emission coverage.
func TestCheck_LayoutStdlibMapFailuresPublishNothing(t *testing.T) {
	target := teststdlibmap.PinnedTarget()
	for _, tt := range []invalidLayoutMapCase{
		{
			name:           "missing map",
			fixture:        standardLayoutMapFixture,
			mapPath:        func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing.stdlib-map.json") },
			wantDiagnostic: []string{"declared stdlib-map artifact", "missing.stdlib-map.json"},
		},
		{
			name:           "malformed map",
			fixture:        standardLayoutMapFixture,
			mapPath:        writeCorruptPinnedMap,
			wantDiagnostic: []string{"declared stdlib-map artifact", "corrupt.stdlib-map.json", "parse artifact"},
		},
		{
			name:    "target SDK-key mismatch",
			fixture: standardLayoutMapFixture,
			mapPath: func(t *testing.T) string {
				return writePinnedMapKeyVariant(t, "target-mismatch.stdlib-map.json", "goarch", "arm64")
			},
			wantDiagnostic: []string{"target-mismatch.stdlib-map.json", "mismatched fields: goarch"},
		},
		{
			name:    "classifier-hash mismatch",
			fixture: standardLayoutMapFixture,
			mapPath: func(t *testing.T) string {
				return writePinnedMapKeyVariant(t, "classifier-mismatch.stdlib-map.json", "classifierHash", "stale-classifier-hash")
			},
			wantDiagnostic: []string{"classifier-mismatch.stdlib-map.json", "mismatched fields: classifier_hash"},
		},
		{
			name:    "format-version mismatch",
			fixture: standardLayoutMapFixture,
			mapPath: func(t *testing.T) string {
				return writePinnedMapVariant(t, "format-mismatch.stdlib-map.json", func(decoded map[string]any) {
					decoded["formatVersion"] = 2
				})
			},
			wantDiagnostic: []string{"format-mismatch.stdlib-map.json", "unsupported artifact format version"},
		},
		{
			name:    "inventory-incomplete map",
			fixture: inventoryGapLayoutFixture,
			mapPath: func(t *testing.T) string {
				return removePinnedMapSymbol(t, "inventory-gap.stdlib-map.json", "fmt.Sprintf")
			},
			wantDiagnostic: []string{"fmt.Sprintf", "stdlib map has no record"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runInvalidLayoutMapCase(t, target, tt)
		})
	}
}

// TestCheck_UnsafeBuiltinReachesAnalysisDefeating exercises the repaired seam
// through the production Runner: typed member source, the normal scanner and
// checker, and the checked pinned map. Supplying the map artifact explicitly
// keeps this regression independent of whole-SDK generation.
func TestCheck_UnsafeBuiltinReachesAnalysisDefeating(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/unsafecheck\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	componentDir := filepath.Join(workspace, "component")
	if err := os.MkdirAll(componentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `package component

import "unsafe"

func Use(p *int) []int {
	_ = unsafe.Sizeof(p)
	return unsafe.Slice(p, 1)
}
`
	if err := os.WriteFile(filepath.Join(componentDir, "unsafe.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(componentDir, "component.textproto")
	manifest := "name: \"unsafe\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \"example.com/unsafecheck/component\"\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	mapPath := filepath.Join(t.TempDir(), "map.json")
	writePinnedMap(t, mapPath)
	stdout, stderr, code := runRunnerStdout(t, workspace, fullRunner(), []string{
		"check", manifestPath, "--stdlib-map=" + mapPath, "--format=json",
	})
	if code != 1 {
		t.Fatalf("unsafe builtin check exit = %d, want conformance failure 1; stderr = %s", code, stderr)
	}
	var rep report.ConformanceReport
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("decode unsafe builtin report: %v; stdout = %s", err, stdout)
	}
	if len(rep.Violations) != 1 {
		t.Fatalf("unsafe builtin violations = %+v, want one aggregated finding", rep.Violations)
	}
	finding := rep.Violations[0]
	if finding.Class != "AnalysisDefeating" || finding.Kind != "UNDECLARED_AUTHORITY" {
		t.Fatalf("unsafe builtin finding = %+v, want AnalysisDefeating UNDECLARED_AUTHORITY", finding)
	}
	wantSites := map[string]int{
		"unsafe.Sizeof": 6,
		"unsafe.Slice":  7,
	}
	if len(finding.Sites) != len(wantSites) {
		t.Fatalf("unsafe builtin sites = %+v, want %v", finding.Sites, wantSites)
	}
	for _, site := range finding.Sites {
		if wantSites[site.Symbol] != site.Line || site.File != "unsafe.go" {
			t.Errorf("unsafe builtin site = %+v, want unsafe.go and %v", site, wantSites)
		}
		delete(wantSites, site.Symbol)
	}
	for symbol := range wantSites {
		t.Errorf("missing unsafe builtin site for %q", symbol)
	}
}

func TestCheck_LayoutMapKnownImportMissingFromLayoutFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/missingstdlib\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	componentDir := filepath.Join(workspace, "component")
	if err := os.MkdirAll(componentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(componentDir, "api.go"), []byte("package component\n\nimport _ \"fmt\"\n\nfunc Use() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const member = "example.com/missingstdlib/component"
	manifestPath := filepath.Join(componentDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte("name: \"missing-stdlib\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \""+member+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	layout := map[string]any{
		"roots": []string{member},
		"packages": []map[string]any{{
			"id": member, "name": "component", "pkgPath": member,
			"is_stdlib":       false,
			"goFiles":         []string{"component/api.go"},
			"compiledGoFiles": []string{"component/api.go"},
		}},
	}
	layoutData, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	layoutPath := filepath.Join(workspace, "package-layout.json")
	if err := os.WriteFile(layoutPath, layoutData, 0o644); err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(t.TempDir(), "map.json")
	writePinnedMap(t, mapPath)
	reportPath := filepath.Join(workspace, "missing-stdlib.report.json")
	surfacePath := filepath.Join(workspace, "missing-stdlib.surface.json")
	code, stderr := runRunnerFromWorkspace2(t, workspace, fullRunner(), []string{
		"check", manifestPath,
		"--package-layout=" + layoutPath,
		"--stdlib-map=" + mapPath,
		"--report-out=" + reportPath,
		"--surface-out=" + surfacePath,
	})
	if code != 2 {
		t.Fatalf("map-known missing import exit = %d, want 2; stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "no type or layout data") || !strings.Contains(stderr, `"fmt"`) {
		t.Fatalf("stderr = %q, want fmt missing-data diagnostic", stderr)
	}
	if _, err := os.Stat(reportPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("report artifact was published after missing stdlib data (stat error: %v)", err)
	}
	if _, err := os.Stat(surfacePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("surface artifact was published after missing stdlib data (stat error: %v)", err)
	}
}

// TestCheck_UnpinnedLayoutIncompleteSDKKey is the unpinned-layout leg of the
// fail-closed contract (AC 2; review round 2): a declared map whose SDK key
// names no concrete target — missing toolchain_version, goos, goarch — must be
// rejected even for an unpinned layout, where no expected key is otherwise
// available to compare. An otherwise-valid map with a current classifier hash
// must not reach a verdict when its target identity is incomplete; the check
// exits 2 and publishes neither artifact.
func TestCheck_UnpinnedLayoutIncompleteSDKKey(t *testing.T) {
	workspace := t.TempDir()
	layoutPath := writeUnpinnedLayoutFixture(t, workspace, []string{"example.com/artifacts/comp"})
	manifestPath := writeLayoutManifest(t, workspace, "example.com/artifacts/comp")

	mapPath := filepath.Join(t.TempDir(), "map.json")
	writePinnedMap(t, mapPath)
	raw, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	// Strip the generated map's target identity while keeping everything else
	// (which retained classifier hash and full inventories) byte-for-byte: the
	// stripped artifact is exactly the "otherwise valid" state the unpinned
	// branch must reject. MarshalMap's own validator already refuses to write
	// such a map, so the file is produced by editing the persisted JSON.
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode the generated map: %v", err)
	}
	key, ok := decoded["key"].(map[string]any)
	if !ok {
		t.Fatalf("generated map JSON has no SDK key object: %s", raw)
	}
	for _, field := range []string{"toolchainVersion", "goos", "goarch"} {
		if _, ok := key[field]; !ok {
			t.Fatalf("generated map key lacks %q, fixture premise broken", field)
		}
		delete(key, field)
	}
	incomplete, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	incompletePath := filepath.Join(t.TempDir(), "incomplete.json")
	if err := os.WriteFile(incompletePath, incomplete, 0o644); err != nil {
		t.Fatal(err)
	}

	runner := fullRunner()
	reportPath := filepath.Join(workspace, "comp.report.json")
	surfacePath := filepath.Join(workspace, "comp.surface.json")
	code, stderr := runRunnerFromWorkspace2(t, workspace, runner, []string{
		"check", manifestPath,
		"--package-layout=" + layoutPath,
		"--report-out=" + reportPath,
		"--surface-out=" + surfacePath,
		"--stdlib-map=" + incompletePath,
	})
	if code != 2 {
		t.Fatalf("incomplete-key map exit = %d, want 2; stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "no concrete target") {
		t.Errorf("incomplete-key map stderr = %q, want it to name the missing target identity", stderr)
	}
	if _, err := os.Stat(reportPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("report artifact %s was published despite failing closed (stat error: %v)", reportPath, err)
	}
	if _, err := os.Stat(surfacePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("surface artifact %s was published despite failing closed (stat error: %v)", surfacePath, err)
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
	writePinnedMap(t, mapPath)

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

// runRunnerStdout is runRunnerFromWorkspace2 with the stdout text returned.
func runRunnerStdout(t *testing.T, workspace string, runner *app.Runner, args []string) (string, string, int) {
	t.Helper()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	if manifestPath, mapPath, ok := nativeStagingArgs(args); ok {
		var staged nativeArtifactStaging
		stageNativeRunnerDependencies(t, runner, manifestPath, mapPath, map[string]bool{}, &staged)
		t.Cleanup(func() {
			if err := staged.restore(); err != nil {
				t.Errorf("restore staged artifacts: %v", err)
			}
		})
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	var stdout, stderr strings.Builder
	code := runner.Run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func nativeStagingArgs(args []string) (manifestPath, mapPath string, ok bool) {
	if len(args) < 2 || args[0] != "check" {
		return "", "", false
	}
	for _, arg := range args[2:] {
		if strings.HasPrefix(arg, "--package-layout=") {
			return "", "", false
		}
		if strings.HasPrefix(arg, "--stdlib-map=") {
			mapPath = strings.TrimPrefix(arg, "--stdlib-map=")
		}
	}
	if mapPath == "" {
		return "", "", false
	}
	return args[1], mapPath, true
}

type nativeArtifactOriginal struct {
	data   []byte
	mode   os.FileMode
	exists bool
}

// nativeArtifactStaging overwrites both members of every dependency artifact
// pair for the duration of a test and restores any developer-owned files when
// the test finishes. This prevents a stale surface from bypassing fresh report
// production, while keeping integration tests non-mutating to the worktree.
type nativeArtifactStaging struct {
	originals map[string]nativeArtifactOriginal
	order     []string
}

func (s *nativeArtifactStaging) capture(t *testing.T, path string) {
	t.Helper()
	if s.originals == nil {
		s.originals = make(map[string]nativeArtifactOriginal)
	}
	if _, ok := s.originals[path]; ok {
		return
	}
	info, err := os.Stat(path)
	if err == nil {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		s.originals[path] = nativeArtifactOriginal{data: data, mode: info.Mode().Perm(), exists: true}
	} else if os.IsNotExist(err) {
		s.originals[path] = nativeArtifactOriginal{}
	} else {
		t.Fatal(err)
	}
	s.order = append(s.order, path)
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
				restoreErrors = append(restoreErrors, path+": "+err.Error())
			}
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			restoreErrors = append(restoreErrors, path+": "+err.Error())
		}
	}
	s.order = nil
	if len(restoreErrors) > 0 {
		return errors.New("restore staged artifacts: " + strings.Join(restoreErrors, "; "))
	}
	return nil
}

func stageNativeRunnerDependencies(t *testing.T, runner *app.Runner, manifestPath, mapPath string, seen map[string]bool, staged *nativeArtifactStaging) {
	t.Helper()
	absManifest, err := filepath.Abs(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	absManifest = filepath.Clean(absManifest)
	if seen[absManifest] {
		return
	}
	seen[absManifest] = true
	f, err := os.Open(absManifest)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	parsed, err := manifest.Parse(f)
	_ = f.Close()
	if err != nil {
		return
	}
	for _, dep := range parsed.ComponentDependencies {
		depManifest := filepath.Clean(filepath.Join(filepath.Dir(absManifest), dep.Manifest))
		stageNativeRunnerDependencies(t, runner, depManifest, mapPath, seen, staged)
		surfacePath := artifactio.SurfacePath(depManifest)
		reportPath := artifactio.ReportPath(depManifest)
		staged.capture(t, surfacePath)
		staged.capture(t, reportPath)
		originalWD, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(filepath.Dir(absManifest)); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr strings.Builder
		code := runner.Run([]string{
			"check", depManifest,
			"--stdlib-map=" + mapPath,
			"--report-out=" + reportPath,
			"--surface-out=" + surfacePath,
			"--report-verdict-only",
		}, &stdout, &stderr)
		if err := os.Chdir(originalWD); err != nil {
			t.Fatal(err)
		}
		if code != 0 {
			t.Fatalf("stage dependency %s: exit %d stderr %q", depManifest, code, stderr.String())
		}
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

// TestCheck_ExcludedInterfaceFileStillEmitsSurface is the mixed
// surviving/excluded interface-file leg (review round 1, finding 3): a valid
// component with one surviving and one build-excluded interface file emits
// the exact surviving surface and keeps the exclusion warning in the report.
func TestCheck_ExcludedInterfaceFileStillEmitsSurface(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/gated\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	compDir := filepath.Join(workspace, "comp")
	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "api.go"), []byte("package comp\n\ntype Greeter interface{ Hello() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "gated.go"), []byte("//go:build plan9\n\npackage comp\n\ntype Gated interface{ Only() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "impl.go"), []byte("package comp\n\ntype greeter struct{}\n\nfunc (greeter) Hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(compDir, "component.textproto")
	manifest := "name: \"gated\"\ninterface_files: \"api.go\"\ninterface_files: \"gated.go\"\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	mapPath := filepath.Join(t.TempDir(), "map.json")
	writePinnedMap(t, mapPath)

	runner := fullRunner()
	reportPath := filepath.Join(compDir, "gated.report.json")
	surfacePath := filepath.Join(compDir, "gated.surface.json")
	args := []string{
		"check", manifestPath,
		"--report-out=" + reportPath,
		"--surface-out=" + surfacePath,
		"--stdlib-map=" + mapPath,
	}
	stdout, stderr, code := runRunnerStdout(t, workspace, runner, args)
	if code != 0 {
		t.Fatalf("check: exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "INTERFACE_FILE_EXCLUDED") || !strings.Contains(stdout, "gated.go") {
		t.Errorf("stdout = %q, want the exclusion warning", stdout)
	}
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := artifactio.DecodeReport(reportData)
	if err != nil {
		t.Fatalf("decode report: %v", err)
	}
	found := false
	for _, w := range persisted.Report.Warnings {
		if w.Kind == "INTERFACE_FILE_EXCLUDED" {
			found = true
		}
	}
	if !found {
		t.Errorf("report warnings = %+v, want the exclusion warning", persisted.Report.Warnings)
	}

	surfaceData, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatal(err)
	}
	surface, err := artifactio.DecodeSurface(strings.NewReader(string(surfaceData)))
	if err != nil {
		t.Fatalf("decode surface: %v", err)
	}
	for _, sym := range surface.Symbols {
		if strings.Contains(sym, ".Gated") {
			t.Errorf("excluded declaration %q leaked into the surface", sym)
		}
	}
	want := "example.com/gated/comp.Greeter"
	found = false
	for _, sym := range surface.Symbols {
		if sym == want {
			found = true
		}
	}
	if !found {
		t.Errorf("surface symbols = %v, want %q from the surviving interface file", surface.Symbols, want)
	}
}

// stageDesignFixtures stages the goanalysis design fixture trees into a
// standalone temp module under a distinct import prefix: the production
// resolver requires dependency manifests to live outside the analyzed
// component root (refscan/dep is a sibling of the member tree in the fixture
// layout), so the dependency sub-trees are staged beside — not under — the
// component roots. Imports are rewritten consistently so the staged sources
// resolve within the temp module. Returns the staged module root and the
// three component manifests.
func stageDesignFixtures(t *testing.T) (fixRoot, classifyManifest, refscanManifest, bypManifest, depPrefix string) {
	t.Helper()
	const origRoot = "../../../internal/goanalysis/testdata"
	const origPrefix = "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata"
	const fixModule = "example.com/fixtestdata"

	fixRoot = t.TempDir()
	depPrefix = fixModule + "/dep"

	staged := map[string]string{ // staging dir (fixRoot-relative) -> fixture tree
		"classify/member": "classify/member",
		"refscan/member":  "refscan/member",
		"byp/member":      "byp/member",
		"dep":             "refscan/dep",
		"infra":           "classify/infra",
	}
	if err := os.WriteFile(filepath.Join(fixRoot, "go.mod"), []byte("module "+fixModule+"\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for dstRel, srcRel := range staged {
		src, dst := filepath.Join(origRoot, srcRel), filepath.Join(fixRoot, dstRel)
		if err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			// Rewrite the original fixture import prefixes into the staged
			// module's import space.
			text = strings.ReplaceAll(text, origPrefix+"/refscan/member", fixModule+"/refscan/member")
			text = strings.ReplaceAll(text, origPrefix+"/refscan/dep", fixModule+"/dep")
			text = strings.ReplaceAll(text, origPrefix+"/classify/member", fixModule+"/classify/member")
			text = strings.ReplaceAll(text, origPrefix+"/classify/infra", fixModule+"/infra")
			text = strings.ReplaceAll(text, origPrefix+"/byp/member", fixModule+"/byp/member")
			return os.WriteFile(target, []byte(text), 0o644)
		}); err != nil {
			t.Fatal(err)
		}
	}

	// initrow is staged member scaffolding for the aggregate-init fixtures:
	// a blank import of a real-map CAPABILITIES init (database/sql, REFLECT)
	// and of the real map's one UNANALYZED init (archive/zip) exercised at
	// their import sites (design fixture 2 and fixture 6's UNANALYZED row
	// under the production authority).
	initrowSource := "package initrow\n\nimport (\n\t_ \"archive/zip\"\n\t_ \"database/sql\"\n)\n\nfunc UseBlankInitRows() {}\n"
	if err := os.MkdirAll(filepath.Join(fixRoot, "classify", "member", "initrow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixRoot, "classify", "member", "initrow", "initrow.go"), []byte(initrowSource), 0o644); err != nil {
		t.Fatal(err)
	}

	refscanManifestPath := filepath.Join(fixRoot, "refscan", "component.textproto")
	refscan := "name: \"refscan\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\n" +
		"members: \"" + fixModule + "/refscan/member/builtins\"\n" +
		"members: \"" + fixModule + "/refscan/member/concrete\"\n" +
		"members: \"" + fixModule + "/refscan/member/dispatch\"\n" +
		"members: \"" + fixModule + "/refscan/member/kinds\"\n" +
		"component_dependencies {\n  name: \"dep\"\n  manifest: \"../dep/component.textproto\"\n}\n"

	classifyManifestPath := filepath.Join(fixRoot, "classify", "component.textproto")
	classify := "name: \"classify\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\n" +
		"members: \"" + fixModule + "/classify/member/globals\"\n" +
		"members: \"" + fixModule + "/classify/member/rows\"\n" +
		"members: \"" + fixModule + "/classify/member/uses\"\n" +
		"members: \"" + fixModule + "/classify/member/initrow\"\n" +
		"component_dependencies {\n  name: \"infra\"\n  manifest: \"../infra/manifest.component.textproto\"\n}\n" +
		"component_dependencies {\n  name: \"dep\"\n  manifest: \"../dep/component.textproto\"\n}\n"

	bypManifestPath := filepath.Join(fixRoot, "byp", "component.textproto")
	byp := "name: \"byp-fixtures\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \"" + fixModule + "/byp/member/byp\"\n"

	for path, content := range map[string]string{
		refscanManifestPath:  refscan,
		classifyManifestPath: classify,
		bypManifestPath:      byp,
		filepath.Join(fixRoot, "dep", "component.textproto"):            "name: \"dep\"\ninterface_files: \"api.go\"\n",
		filepath.Join(fixRoot, "infra", "manifest.component.textproto"): "name: \"infra\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \"" + fixModule + "/infra/infra\"\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return fixRoot, classifyManifestPath, refscanManifestPath, bypManifestPath, depPrefix
}

// TestDesignFixturesThroughRealCommand runs the design fixture 1-6/9 member
// trees and the analysis-defeating fixture through the production check
// orchestration — the real runner, the real package loader, the resolved
// dependency interfaces, and the real generated stdlib map read through the
// declared-artifact path (AC 4). goanalysis/fixture_test.go pins the
// dispatch and table behaviour at the unit level; this test pins that the
// real command reaches the same documented outcomes.
func TestDesignFixturesThroughRealCommand(t *testing.T) {
	fixRoot, _, _, bypManifest, _ := stageDesignFixtures(t)

	// The package-surface variant of the refscan manifest classifies the same
	// member trees against dep's PACKAGE_SURFACE dependency manifest.
	refscanSurfaceManifest := filepath.Join(fixRoot, "refscan", "surface.component.textproto")
	if err := os.WriteFile(refscanSurfaceManifest, []byte("name: \"refscan\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\n"+
		"members: \""+"example.com/fixtestdata"+"/refscan/member/builtins\"\n"+
		"members: \""+"example.com/fixtestdata"+"/refscan/member/concrete\"\n"+
		"members: \""+"example.com/fixtestdata"+"/refscan/member/dispatch\"\n"+
		"members: \""+"example.com/fixtestdata"+"/refscan/member/kinds\"\n"+
		"component_dependencies {\n  name: \"dep\"\n  manifest: \"../dep/pkg_surface.textproto\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mapPath := filepath.Join(t.TempDir(), "map.json")
	writePinnedMap(t, mapPath)

	runner := fullRunner()
	runJSON := func(t *testing.T, manifest string) (report.ConformanceReport, int) {
		t.Helper()
		stdoutText, stderr, code := runRunnerStdout(t, fixRoot, runner, []string{"check", manifest, "--stdlib-map=" + mapPath, "--format=json"})
		if code != 0 && code != 1 {
			t.Fatalf("check %s: exit %d, stderr: %s", manifest, code, stderr)
		}
		var rep report.ConformanceReport
		if err := json.Unmarshal([]byte(stdoutText), &rep); err != nil {
			t.Fatalf("decode JSON report for %s: %v; stdout: %s", manifest, err, stdoutText)
		}
		return rep, code
	}
	hasSite := func(f report.Finding, file string, line int, sym string) bool {
		for _, s := range f.Sites {
			if s.File == file && s.Line == line && strings.Contains(s.Symbol, sym) {
				return true
			}
		}
		return false
	}
	byClass := func(rep report.ConformanceReport) map[string][]report.Finding {
		out := map[string][]report.Finding{}
		for _, v := range rep.Violations {
			out[v.Class] = append(out[v.Class], v)
		}
		return out
	}

	t.Run("fixtures 1-2-9 and the import table", func(t *testing.T) {
		rep, code := runJSON(t, "classify/component.textproto")
		if code != 1 {
			t.Fatalf("classify fixtures must fail (exit 1), got %d", code)
		}
		if len(rep.Warnings) != 0 {
			t.Errorf("warnings = %+v, want none", rep.Warnings)
		}
		byCls := byClass(rep)
		// Fixture 1: the os.ReadFile function value carries FILES at its exact
		// site; fixture 9: the same FILES finding also aggregates os.Stdin.
		var files *report.Finding
		for idx := range byCls["TrueAuthority"] {
			if byCls["TrueAuthority"][idx].Message == `use of undeclared authority "FILES"` {
				files = &byCls["TrueAuthority"][idx]
			}
		}
		if files == nil {
			t.Fatalf("want a FILES authority finding, got %+v", rep.Violations)
		}
		if len(files.Evidence) != 1 || !strings.Contains(files.Evidence[0], "os.ReadFile") {
			t.Errorf("FILES finding evidence = %+v, want the map's canned os.ReadFile path", files.Evidence)
		}
		if len(files.Sites) != 2 ||
			!hasSite(*files, "member/globals/globals.go", 12, "os.ReadFile") ||
			!hasSite(*files, "member/globals/globals.go", 15, "os.Stdin") {
			t.Errorf("FILES sites = %+v, want the os.ReadFile function value and os.Stdin sites", files.Sites)
		}
		// Fixture 9, continued: os.Stdin carries its own real-map records:
		// MODIFY_SYSTEM_STATE and OPERATING_SYSTEM reach their own verdicts.
		for _, capMsg := range []string{
			`use of undeclared authority "MODIFY_SYSTEM_STATE"`,
			`use of undeclared authority "OPERATING_SYSTEM"`,
		} {
			found := false
			for _, f := range byCls["TrueAuthority"] {
				if f.Message == capMsg && hasSite(f, "member/globals/globals.go", 15, "os.Stdin") {
					found = true
				}
			}
			if !found {
				t.Errorf("want an os.Stdin finding %q; got %+v", capMsg, rep.Violations)
			}
		}
		// io.EOF is SAFE and must contribute no authority at all.
		for _, v := range rep.Violations {
			if strings.Contains(v.Message, "io.EOF") {
				t.Errorf("io.EOF must contribute no authority, got %+v", v)
			}
		}
		// Fixture 2 and fixture 6's UNANALYZED init row: blank imports of
		// map-enumerated packages classify under the aggregate init identity.
		// The real map grades database/sql.init CAPABILITIES{REFLECT} and
		// archive/zip.init UNANALYZED.
		var reflect *report.Finding
		for idx := range byCls["TrueAuthority"] {
			f := &byCls["TrueAuthority"][idx]
			if hasSite(*f, "member/initrow/initrow.go", 5, "database/sql.init") {
				reflect = f
			}
		}
		if reflect == nil {
			t.Fatalf("want a database/sql.init finding, got %+v", rep.Violations)
		}
		sawEvidence := false
		for _, ev := range reflect.Evidence {
			if strings.Contains(ev, "database/sql.init") {
				sawEvidence = true
			}
		}
		if !sawEvidence {
			t.Errorf("database/sql.init finding must carry the map's canned init evidence, got %+v", reflect.Evidence)
		}
		var defeats *report.Finding
		for idx := range byCls["AnalysisDefeating"] {
			f := &byCls["AnalysisDefeating"][idx]
			if hasSite(*f, "member/initrow/initrow.go", 4, "archive/zip.init") {
				defeats = f
			}
		}
		if defeats == nil || len(defeats.Sites) != 2 || !hasSite(*defeats, "member/uses/uses.go", 20, "errors.Is") {
			t.Errorf("AnalysisDefeating sites = %+v, want the UNANALYZED archive/zip.init import and errors.Is reference", defeats)
		}
		if len(defeats.Evidence) != 0 {
			t.Errorf("the AnalysisDefeating finding must carry no evidence, got %+v", defeats.Evidence)
		}
	})

	t.Run("fixtures 3-6 boundary outcomes", func(t *testing.T) {
		rep, code := runJSON(t, "refscan/component.textproto")
		if code != 1 {
			t.Fatalf("refscan fixtures must fail (exit 1), got %d", code)
		}
		// Fixture 3: the Greeter.Method dispatch through the declared
		// interface produces no finding; every other dependency reach is a
		// CALLS_UNDECLARED_INTERFACE at its exact site (fixture 4's concrete
		// access and fixture 5's kind matrix), and dep/sub is an undeclared
		// interface against the declared dependency (fixture 6).
		for _, v := range rep.Violations {
			if v.Kind != "CALLS_UNDECLARED_INTERFACE" {
				t.Errorf("violations = %+v, want only CALLS_UNDECLARED_INTERFACE", rep.Violations)
				break
			}
			if strings.Contains(v.Location.File, "member/dispatch/") {
				t.Errorf("the dispatch site must stay authorized, got %+v", v)
			}
		}
		types := make(map[string]bool, len(rep.Violations))
		sites := 0
		for _, v := range rep.Violations {
			types[string(v.Kind)] = true
			sites += len(v.Sites) + len(v.Evidence)
		}
		if len(types) != 1 || !types["CALLS_UNDECLARED_INTERFACE"] {
			t.Errorf("kinds = %v, want only CALLS_UNDECLARED_INTERFACE", types)
		}
		if len(rep.Violations) != 19 {
			t.Errorf("violations = %d, want the fixture-4 concrete access and the 17 kinds sites", len(rep.Violations))
		}
		hasLoc := func(v report.Finding, file string, line int) bool {
			if v.Location.File == file && v.Location.Line == line {
				return true
			}
			for _, s := range v.Sites {
				if s.File == file && s.Line == line {
					return true
				}
			}
			return false
		}
		locating := func(file string, line int) bool {
			for _, v := range rep.Violations {
				if hasLoc(v, file, line) {
					return true
				}
			}
			return false
		}
		if !locating("member/concrete/concrete.go", 8) || !locating("member/concrete/concrete.go", 9) {
			t.Errorf("fixture 4's rejected concrete access at concrete.go:8-9 was not reported: %+v", rep.Violations)
		}
		if !locating("member/kinds/kinds.go", 28) {
			t.Errorf("fixture 5's dep/sub.Sub edge at kinds.go:28 was not reported: %+v", rep.Violations)
		}
		if len(rep.Warnings) != 0 {
			t.Errorf("warnings = %+v, want none", rep.Warnings)
		}
	})

	t.Run("fixtures 3-6 against the package-surface dependency", func(t *testing.T) {
		rep, code := runJSON(t, "refscan/surface.component.textproto")
		if code != 1 {
			t.Fatalf("package-surface fixtures must fail (exit 1), got %d", code)
		}
		// Against a PACKAGE_SURFACE dependency every declared kind is
		// authorized; only the unowned dep/sub reaches remain — its call at
		// kinds.go:28 and the import rows — as UNDECLARED_DEPENDENCY (the
		// surface dependency owns exactly its member package).
		for _, v := range rep.Violations {
			if v.Kind != "UNDECLARED_DEPENDENCY" || (!strings.Contains(v.Message, "dep/sub") && !strings.Contains(v.Message, "dep/initpkg")) {
				t.Errorf("kind = %q message = %q, want only the unowned dep/sub and dep/initpkg UNDECLARED_DEPENDENCY", v.Kind, v.Message)
			}
		}
	})

	t.Run("analysis-defeating fixture", func(t *testing.T) {
		rep, code := runJSON(t, "byp/component.textproto")
		if code != 1 {
			t.Fatalf("bypass fixture must fail (exit 1), got %d", code)
		}
		if len(rep.Violations) != 1 {
			t.Fatalf("violations = %+v, want the single aggregated AnalysisDefeating finding", rep.Violations)
		}
		v := rep.Violations[0]
		if v.Class != "AnalysisDefeating" || v.Kind != "UNDECLARED_AUTHORITY" {
			t.Errorf("finding = %+v, want UNDECLARED_AUTHORITY class AnalysisDefeating", v)
		}
		wantFiles := map[string]int{"member/byp/link.go": 5, "member/byp/cgo.go": 4, "member/byp/stub.s": 1, "member/byp/stub_plan9.s": 1}
		if len(v.Sites) != len(wantFiles) {
			t.Fatalf("sites = %+v, want the linkname, cgo and both assembly files", v.Sites)
		}
		for _, s := range v.Sites {
			if wantFiles[s.File] != s.Line {
				t.Errorf("site %+v, want it within %v", s, wantFiles)
			}
		}
		if len(v.Evidence) != 0 {
			t.Errorf("the bypass finding must carry no evidence, got %+v", v.Evidence)
		}

		warnManifest := filepath.Join(fixRoot, "byp", "warn.component.textproto")
		data, err := os.ReadFile(bypManifest)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, []byte("analysis_defeating_policy: WARN\n")...)
		if err := os.WriteFile(warnManifest, data, 0o644); err != nil {
			t.Fatal(err)
		}
		warned, warnCode := runJSON(t, "byp/warn.component.textproto")
		if warnCode != 0 {
			t.Fatalf("WARN bypass fixture must pass with visible warnings (exit 0), got %d", warnCode)
		}
		if len(warned.Violations) != 0 || len(warned.Warnings) != 1 {
			t.Fatalf("WARN bypass report = %+v, want no violations and one warning", warned)
		}
		w := warned.Warnings[0]
		if w.Kind != report.AnalysisLimitation || w.Class != "AnalysisDefeating" {
			t.Errorf("WARN bypass finding = %+v, want AnalysisDefeating ANALYSIS_LIMITATION", w)
		}
		if len(w.Sites) != len(v.Sites) {
			t.Fatalf("WARN bypass sites = %+v, want unchanged sites %+v", w.Sites, v.Sites)
		}
		for i := range v.Sites {
			if w.Sites[i] != v.Sites[i] {
				t.Errorf("WARN bypass site %d = %+v, want strict site %+v", i, w.Sites[i], v.Sites[i])
			}
		}
	})
}
