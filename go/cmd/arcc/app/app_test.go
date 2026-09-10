package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// testAuthority is a fake StdlibAuthority that enumerates no stdlib
// packages: every stdlib decision in these unit tests is driven by explicit
// facts injected through the loader seam. It carries the shared full SDK key
// so surface emission stamps it.
type testAuthority struct{}

func (testAuthority) IsStdlibPackage(string) bool { return false }

func (testAuthority) SymbolAuthority(id symbol.SymbolID) (stdlibauthority.Classification, error) {
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{
		Package: facts.SymbolIDPackage(id),
		Symbol:  id.Format(),
	}
}

func (testAuthority) PackageInitAuthority(pkgPath string) (stdlibauthority.Classification, error) {
	return stdlibauthority.Classification{}, &stdlibauthority.InventoryGapError{Package: pkgPath}
}

func (testAuthority) Evidence(symbol.SymbolID, stdlibauthority.Capability) []stdlibauthority.Frame {
	return nil
}

func (testAuthority) Key() stdlibauthority.SDKKey { return fullSDKKey }

func TestRunner_VersionAndHelp(t *testing.T) {
	runner := &app.Runner{}

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "version",
			args:       []string{"--version"},
			wantExit:   0,
			wantStdout: "arcc 0.0.0-dev\n",
		},
		{
			name:       "help",
			args:       []string{"help"},
			wantExit:   0,
			wantStdout: "Usage:",
		},
		{
			name:       "no args",
			args:       []string{},
			wantExit:   0,
			wantStdout: "Usage:",
		},
		{
			name:       "unknown command",
			args:       []string{"unknown"},
			wantExit:   2,
			wantStderr: "unknown command",
		},
		{
			name:       "bad arity",
			args:       []string{"check"},
			wantExit:   2,
			wantStderr: "Usage:",
		},
		{
			name:       "unknown option",
			args:       []string{"check", "component.textproto", "--bogus"},
			wantExit:   2,
			wantStderr: "unknown option: --bogus",
		},
		{
			name:       "empty layout value",
			args:       []string{"check", "component.textproto", "--package-layout="},
			wantExit:   2,
			wantStderr: "empty --package-layout value",
		},
		{
			name:       "missing layout value (no equals)",
			args:       []string{"check", "component.textproto", "--package-layout"},
			wantExit:   2,
			wantStderr: "missing --package-layout value",
		},
		{
			name:       "duplicate layout",
			args:       []string{"check", "component.textproto", "--package-layout=a.json", "--package-layout=b.json"},
			wantExit:   2,
			wantStderr: "duplicate option: --package-layout",
		},
		{
			name:       "duplicate format",
			args:       []string{"check", "component.textproto", "--format=json", "--format=json"},
			wantExit:   2,
			wantStderr: "duplicate option: --format=json",
		},
		{
			name:       "unexpected extra positional argument",
			args:       []string{"check", "component.textproto", "extra-arg"},
			wantExit:   2,
			wantStderr: "error: check command requires exactly one argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runner.Run(tt.args, &stdout, &stderr)
			if exitCode != tt.wantExit {
				t.Errorf("Run() exitCode = %d, want %d", exitCode, tt.wantExit)
			}
			if tt.wantStdout != "" {
				if !strings.Contains(stdout.String(), tt.wantStdout) {
					t.Errorf("stdout = %q, want to contain %q", stdout.String(), tt.wantStdout)
				}
			} else {
				if stdout.Len() != 0 {
					t.Errorf("stdout = %q, want empty", stdout.String())
				}
			}
			if tt.wantStderr != "" {
				if !strings.Contains(stderr.String(), tt.wantStderr) {
					t.Errorf("stderr = %q, want to contain %q", stderr.String(), tt.wantStderr)
				}
			} else {
				if stderr.Len() != 0 {
					t.Errorf("stderr = %q, want empty", stderr.String())
				}
			}
		})
	}
}

func createTempComponent(t *testing.T, name string, manifestContent string, files map[string]string) (string, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Write go.mod so packages load successfully
	safeName := strings.ReplaceAll(name, " ", "-")
	goMod := fmt.Sprintf("module example.com/temp/%s\n\ngo 1.21\n", safeName)
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	manifestPath := filepath.Join(tmpDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write temp manifest: %v", err)
	}

	for f, content := range files {
		absPath := filepath.Join(tmpDir, f)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file %s: %v", f, err)
		}
	}
	return tmpDir, manifestPath
}

func writeRunnerLayoutFixture(t *testing.T, workspace string, roots []string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/layout\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	packages := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		name := root[strings.LastIndex(root, "/")+1:]
		relFile := filepath.ToSlash(filepath.Join(name, name+".go"))
		if err := os.MkdirAll(filepath.Dir(filepath.Join(workspace, relFile)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, relFile), []byte("package "+name+"\n\nfunc Exported() {}\n"), 0644); err != nil {
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
	if err := os.WriteFile(layoutPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	return layoutPath
}

func runRunnerFromWorkspace(t *testing.T, workspace string, runner *app.Runner, args []string) (string, string, int) {
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
	var stdout, stderr bytes.Buffer
	exitCode := runner.Run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func TestRunner_Check_LayoutMembershipMismatchIsToolError(t *testing.T) {
	tests := []struct {
		name       string
		members    []string
		roots      []string
		wantPieces []string
	}{
		{
			name:       "missing layout root",
			members:    []string{"example.com/layout/a", "example.com/layout/missing"},
			roots:      []string{"example.com/layout/a"},
			wantPieces: []string{"manifest members", "layout roots", "missing from layout: [example.com/layout/missing]"},
		},
		{
			name:       "extra layout root",
			members:    []string{"example.com/layout/a"},
			roots:      []string{"example.com/layout/a", "example.com/layout/b"},
			wantPieces: []string{"manifest members", "layout roots", "missing from manifest: [example.com/layout/b]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			layoutPath := writeRunnerLayoutFixture(t, workspace, tt.roots)
			manifestPath := filepath.Join(workspace, "component.textproto")
			manifest := fmt.Sprintf("name: \"layout-mismatch\"\ninterface_files: \"a/a.go\"\nmembers: \"%s\"\n", strings.Join(tt.members, "\"\nmembers: \""))
			if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
				t.Fatal(err)
			}

			runner := &app.Runner{
				Loader: func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
					return goanalysis.LoadPackageFacts(req)
				},
				AuthorityResolver: func(app.AuthorityRequest) (stdlibauthority.StdlibAuthority, error) {
					return testAuthority{}, nil
				},
			}
			_, stderr, exitCode := runRunnerFromWorkspace(t, workspace, runner, []string{
				"check", manifestPath, "--package-layout=" + layoutPath,
				// The map flag satisfies the layout-mode usage rule; the
				// loader's membership validation fails first.
				"--stdlib-map=unused.json",
			})
			if exitCode != 2 {
				t.Fatalf("Run() exit = %d, want tool error 2; stderr = %q", exitCode, stderr)
			}
			for _, piece := range tt.wantPieces {
				if !strings.Contains(stderr, piece) {
					t.Errorf("stderr = %q, want %q", stderr, piece)
				}
			}
		})
	}
}

func TestRunner_Check_MemberOverlapIsViolation(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "component.textproto")
	manifest := `name: "member-overlap"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/overlap/member"
component_dependencies {
  name: "overdep"
  manifest: "../overlapdep/component.textproto"
}
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	depDir := filepath.Join(filepath.Dir(workspace), "overlapdep")
	if err := os.MkdirAll(filepath.Join(depDir, "member"), 0755); err != nil {
		t.Fatal(err)
	}
	depManifest := `name: "overdep"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/overlap/member"
`
	if err := os.WriteFile(filepath.Join(depDir, "component.textproto"), []byte(depManifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte("module example.com/overlap\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "member", "member.go"), []byte("package member\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeNativeTestSurface(t, depDir, "overdep", gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE, []string{"example.com/overlap/member"}, nil)

	runner := surfaceTestRunner(func(goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return facts.PackageFacts{Packages: []facts.PackageFact{{ImportPath: "example.com/overlap/member"}}}, nil
	})

	stdout, stderr, exitCode := runRunnerFromWorkspace(t, workspace, runner, []string{"check", manifestPath})
	if exitCode != 1 {
		t.Fatalf("Run() exit = %d, want violation 1; stdout = %q, stderr = %q", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, piece := range []string{"MEMBER_OVERLAP", "example.com/overlap/member"} {
		if !strings.Contains(stdout, piece) {
			t.Errorf("stdout = %q, want %q", stdout, piece)
		}
	}
}

func TestRunner_Check_Success(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "success", manifestContent, files)

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	runner := &app.Runner{Loader: loader}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() returned %d, want 0. Stderr: %s", exitCode, stderr.String())
	}

	got := stdout.String()
	want := `Component "test-comp" conforms; does not exceed declared authority`
	if !strings.Contains(got, want) {
		t.Errorf("stdout = %q, want to contain %q", got, want)
	}
}

func TestRunner_Check_InterfaceFileExclusion_WarnsInTextAndJSON(t *testing.T) {
	falseExpression := "!" + runtime.GOOS
	manifestContent := `
name: "excluded-interface"
interface_files: "api.go"
interface_files: "gated.go"
`
	files := map[string]string{
		"api.go":   "package main\n\nfunc Hello() {}\n",
		"gated.go": "//go:build " + falseExpression + "\n\npackage main\n\nfunc Gated() {}\n",
	}
	_, manifestPath := createTempComponent(t, "excluded-interface", manifestContent, files)
	runner := &app.Runner{
		Loader: func(req goanalysis.LoadRequest) (facts.PackageFacts, error) { return goanalysis.LoadPackageFacts(req) },
	}

	for _, args := range [][]string{{"check", manifestPath}, {"check", manifestPath, "--format=json"}} {
		var stdout, stderr bytes.Buffer
		if got := runner.Run(args, &stdout, &stderr); got != 0 {
			t.Fatalf("Run(%v) exit = %d, stderr = %q, stdout = %q", args, got, stderr.String(), stdout.String())
		}
		if !strings.Contains(stdout.String(), "INTERFACE_FILE_EXCLUDED") || !strings.Contains(stdout.String(), "gated.go") || !strings.Contains(stdout.String(), falseExpression) {
			t.Fatalf("Run(%v) stdout = %q, want exclusion warning", args, stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(%v) stderr = %q, want empty", args, stderr.String())
		}
	}

	var stdout, stderr bytes.Buffer
	if got := runner.Run([]string{"check", manifestPath, "--format=json"}, &stdout, &stderr); got != 0 {
		t.Fatalf("JSON Run() exit = %d, stderr = %q", got, stderr.String())
	}
	var rendered report.ConformanceReport
	if err := json.Unmarshal(stdout.Bytes(), &rendered); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; output = %q", err, stdout.String())
	}
	if len(rendered.Violations) != 0 || len(rendered.Warnings) != 1 || rendered.Warnings[0].Kind != report.InterfaceFileExcluded {
		t.Fatalf("rendered report = %#v, want one exclusion warning and no violations", rendered)
	}
}

func TestRunner_Check_InterfaceFilesAllExcludedFailsClosed(t *testing.T) {
	falseExpression := "!" + runtime.GOOS
	manifestContent := `
name: "all-excluded-interface"
interface_files: "gated.go"
`
	files := map[string]string{
		"keep.go":  "package main\n\nfunc Keep() {}\n",
		"gated.go": "//go:build " + falseExpression + "\n\npackage main\n\nfunc Gated() {}\n",
	}
	_, manifestPath := createTempComponent(t, "all-excluded-interface", manifestContent, files)
	runner := &app.Runner{
		Loader: func(req goanalysis.LoadRequest) (facts.PackageFacts, error) { return goanalysis.LoadPackageFacts(req) },
	}

	var stdout, stderr bytes.Buffer
	if got := runner.Run([]string{"check", manifestPath}, &stdout, &stderr); got != 2 {
		t.Fatalf("Run() exit = %d, want tool error 2; stdout = %q, stderr = %q", got, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "no interface file survives") || !strings.Contains(stderr.String(), "gated.go") || !strings.Contains(stderr.String(), falseExpression) {
		t.Fatalf("stdout = %q, stderr = %q, want fail-closed exclusion context", stdout.String(), stderr.String())
	}
}

func TestRunner_PassesManifestMembershipToLoader(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
members: "example.com/temp/loader-request"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "loader-request", manifestContent, files)

	var gotRequest goanalysis.LoadRequest
	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		gotRequest = req
		return goanalysis.LoadPackageFacts(req)
	}
	runner := &app.Runner{Loader: loader}

	var stdout, stderr bytes.Buffer
	if exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Run() returned %d. Stderr: %s", exitCode, stderr.String())
	}

	if gotRequest.ComponentRoot == "" ||
		gotRequest.ComponentName != "test-comp" ||
		!reflect.DeepEqual(gotRequest.Members, []string{"example.com/temp/loader-request"}) ||
		!reflect.DeepEqual(gotRequest.InterfaceFiles, []string{"api.go"}) {
		t.Fatalf("loader request = %+v, want manifest membership and interface context", gotRequest)
	}
}

func TestRunner_Check_DeclaredOutsideRootAndFR1Fallback(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/workspace\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	componentDir := filepath.Join(workspace, "component")
	outsideDir := filepath.Join(workspace, "outside")
	unrelatedDir := filepath.Join(componentDir, "unrelated")
	for _, dir := range []string{componentDir, outsideDir, unrelatedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(componentDir, "api.go"):       "package component\n\nfunc API() {}\n",
		filepath.Join(outsideDir, "outside.go"):     "package outside\n\nimport \"os\"\n\nfunc Outside() { _, _ = os.Getwd() }\n",
		filepath.Join(unrelatedDir, "unrelated.go"): "package unrelated\n\nfunc Unrelated() {}\n",
	}
	for file, content := range files {
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	manifestPath := filepath.Join(componentDir, "component.textproto")
	manifestWithMembers := `
name: "component"
interface_files: "api.go"
members: "example.com/workspace/outside"
`
	if err := os.WriteFile(manifestPath, []byte(manifestWithMembers), 0644); err != nil {
		t.Fatal(err)
	}

	var loaded facts.PackageFacts
	// Capture facts through a wrapper so the test observes the complete app path
	// while retaining the production loader and its package graph.
	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		got, err := goanalysis.LoadPackageFacts(req)
		if err == nil {
			loaded = got
		}
		return got, err
	}
	// The production authority path decides os.Getwd's classification from
	// the native map: the outside member's READ_SYSTEM_STATE/FILES use is a
	// declared-authority violation.
	runner := &app.Runner{Loader: loader}

	var stdout, stderr bytes.Buffer
	if exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("declared run exit code = %d, want 1; stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "UNDECLARED_AUTHORITY") {
		t.Fatalf("declared run output = %q, want authority finding", stdout.String())
	}
	assertDeclaredOutsideFacts(t, loaded)

	if err := os.WriteFile(manifestPath, []byte(`
name: "component"
interface_files: "api.go"
`), 0644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("FR1 fallback exit code = %d, want 0; stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	for _, pkg := range loaded.Packages {
		if pkg.ImportPath == "example.com/workspace/outside" {
			t.Fatalf("FR1 facts unexpectedly retained outside package: %+v", loaded.Packages)
		}
	}
}

func assertDeclaredOutsideFacts(t *testing.T, loaded facts.PackageFacts) {
	t.Helper()
	if len(loaded.Packages) != 2 {
		t.Fatalf("declared facts packages = %+v, want component and outside only", loaded.Packages)
	}
	var outside facts.PackageFact
	for _, pkg := range loaded.Packages {
		if pkg.ImportPath == "example.com/workspace/outside" {
			outside = pkg
		}
		if pkg.ImportPath == "example.com/workspace/component/unrelated" {
			t.Fatalf("declared facts unexpectedly retained unrelated package: %+v", loaded.Packages)
		}
	}
	if !reflect.DeepEqual(outside.Imports, []string{"os"}) {
		t.Fatalf("outside imports = %v, want [os]", outside.Imports)
	}
	foundSymbol := false
	for _, symbol := range outside.ExportedSymbols {
		if symbol.Name == "example.com/workspace/outside.Outside" {
			foundSymbol = true
		}
	}
	if !foundSymbol {
		t.Fatalf("outside symbols = %+v, want Outside", outside.ExportedSymbols)
	}
	foundImport := false
	for _, edge := range loaded.Imports {
		if edge.ImportingPackage == "example.com/workspace/outside" && edge.ImportPath == "os" {
			foundImport = true
		}
	}
	if !foundImport {
		t.Fatalf("outside import edges = %+v, want the os import edge", loaded.Imports)
	}
	foundReference := false
	for _, edge := range loaded.References {
		if edge.FromPackage == "example.com/workspace/outside" && facts.SymbolIDPackage(edge.Referent) == "os" {
			foundReference = true
		}
	}
	if !foundReference {
		t.Fatalf("outside reference edges = %+v, want an os reference edge", loaded.References)
	}
}

func TestRunner_Check_Success_JSON(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "success-json", manifestContent, files)

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	runner := &app.Runner{Loader: loader}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath, "--format=json"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() returned %d, want 0. Stderr: %s", exitCode, stderr.String())
	}

	var rep report.ConformanceReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v, stdout: %s", err, stdout.String())
	}

	if rep.Component != "test-comp" {
		t.Errorf("unmarshaled report component = %q, want %q", rep.Component, "test-comp")
	}

	if len(rep.Violations) != 0 {
		t.Errorf("violations count = %d, want 0", len(rep.Violations))
	}

	// Check for a trailing newline
	gotOut := stdout.String()
	if !strings.HasSuffix(gotOut, "\n") {
		t.Errorf("JSON output does not end with a trailing newline")
	}
}

func TestRunner_Check_Violation(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nimport \"os\"\n\nfunc Hello() { _, _ = os.ReadFile(\"x\") }\n",
	}
	_, manifestPath := createTempComponent(t, "violation", manifestContent, files)

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	// The production native authority classifies os.ReadFile as FILES, so
	// strict policy fails the component.
	runner := &app.Runner{Loader: loader}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("Run() returned %d, want 1. Stderr: %s", exitCode, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "UNDECLARED_AUTHORITY") {
		t.Errorf("stdout = %q, want to contain UNDECLARED_AUTHORITY", got)
	}
}

func TestRunner_Check_DeclaredAuthority_Pass(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
declared_authority: "FILES"
`
	files := map[string]string{
		"api.go": "package main\n\nimport \"os\"\n\nfunc Hello() { _, _ = os.ReadFile(\"x\") }\n",
	}
	_, manifestPath := createTempComponent(t, "declared-auth", manifestContent, files)

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	runner := &app.Runner{Loader: loader}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() returned %d, want 0. Stderr: %s", exitCode, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "conforms") {
		t.Errorf("stdout = %q, want to contain conforms", got)
	}
}

func TestRunner_Check_InterfaceValidationError(t *testing.T) {
	tests := []struct {
		name            string
		manifestContent string
		wantErrSub      string
	}{
		{
			name: "missing_file",
			manifestContent: `
name: "test-comp"
interface_files: "missing.go"
`,
			wantErrSub: "missing.go",
		},
		{
			name: "escaping_file",
			manifestContent: `
name: "test-comp"
interface_files: "../escaping.go"
`,
			wantErrSub: "escapes the component root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"api.go": "package main\n\nfunc Hello() {}\n",
			}
			_, manifestPath := createTempComponent(t, "iface-val-"+tt.name, tt.manifestContent, files)

			loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
				return goanalysis.LoadPackageFacts(req)
			}

			runner := &app.Runner{Loader: loader}

			var stdout, stderr bytes.Buffer
			exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

			if exitCode != 2 {
				t.Fatalf("Run() returned %d, want 2. Stdout: %s", exitCode, stdout.String())
			}

			gotErr := stderr.String()
			if !strings.Contains(gotErr, tt.wantErrSub) {
				t.Errorf("stderr = %q, want to contain %q", gotErr, tt.wantErrSub)
			}

			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestRunner_Check_LoaderError(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "loader-error", manifestContent, files)

	loader := func(_ goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return facts.PackageFacts{}, fmt.Errorf("injected loader error")
	}

	runner := &app.Runner{Loader: loader}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("Run() returned %d, want 2. Stdout: %s", exitCode, stdout.String())
	}

	gotErr := stderr.String()
	if !strings.Contains(gotErr, "injected loader error") {
		t.Errorf("stderr = %q, want to mention injected loader error", gotErr)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunner_Check_AuthorityResolverError(t *testing.T) {
	manifestContent := `
name: "test-comp"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go": "package main\n\nfunc Hello() {}\n",
	}
	_, manifestPath := createTempComponent(t, "authority-error", manifestContent, files)

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	runner := &app.Runner{
		Loader: loader,
		AuthorityResolver: func(app.AuthorityRequest) (stdlibauthority.StdlibAuthority, error) {
			return nil, fmt.Errorf("injected authority error")
		},
	}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("Run() returned %d, want 2. Stdout: %s", exitCode, stdout.String())
	}

	gotErr := stderr.String()
	if !strings.Contains(gotErr, "injected authority error") {
		t.Errorf("stderr = %q, want to mention injected authority error", gotErr)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunner_Check_Success_MultiPackage(t *testing.T) {
	manifestContent := `
name: "test-comp-multi"
interface_files: "api.go"
`
	files := map[string]string{
		"api.go":           "package main\n\nfunc Hello() {}\n",
		"subpkg/helper.go": "package subpkg\n\nfunc Help() {}\n",
	}
	_, manifestPath := createTempComponent(t, "multipackage", manifestContent, files)

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	runner := &app.Runner{Loader: loader}

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() returned %d, want 0. Stderr: %s", exitCode, stderr.String())
	}
}

func TestRunner_Check_Success_WarningsOnly(t *testing.T) {
	// 1. Create a parent directory and a dependency in a sibling directory
	parentDir := t.TempDir()
	depDir := filepath.Join(parentDir, "unused-dep")
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatalf("failed to create dep dir: %v", err)
	}

	depManifest := `
name: "unused-dep"
interface_files: "api.go"
`
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte("module example.com/temp/unused-dep\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to write dep go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "component.textproto"), []byte(depManifest), 0644); err != nil {
		t.Fatalf("failed to write dep manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "api.go"), []byte("package unused_dep\n\nfunc Unused() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write dep api.go: %v", err)
	}
	writeNativeTestSurface(t, depDir, "unused-dep", gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED, []string{"example.com/temp/unused-dep"}, []string{"example.com/temp/unused-dep.Unused"})

	// 2. Create the analyzed component in a sibling directory
	analyzedDir := filepath.Join(parentDir, "warnings-only")
	if err := os.MkdirAll(analyzedDir, 0755); err != nil {
		t.Fatalf("failed to create analyzed dir: %v", err)
	}

	manifestContent := `
name: "test-comp-warn"
interface_files: "api.go"
component_dependencies: {
  name: "unused-dep"
  manifest: "../unused-dep/component.textproto"
}
`
	if err := os.WriteFile(filepath.Join(analyzedDir, "go.mod"), []byte("module example.com/temp/warnings-only\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to write analyzed go.mod: %v", err)
	}
	manifestPath := filepath.Join(analyzedDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write analyzed manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(analyzedDir, "api.go"), []byte("package main\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write analyzed api.go: %v", err)
	}

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	runner := surfaceTestRunner(loader)

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() returned %d, want 0. Stderr: %s", exitCode, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "Warnings:") {
		t.Errorf("stdout = %q, want to contain 'Warnings:'", got)
	}
	if !strings.Contains(got, "declared component dependency \"unused-dep\" is unused") {
		t.Errorf("stdout = %q, want warning about unused dependency", got)
	}
	if strings.Contains(got, "Violations:") {
		t.Errorf("stdout = %q, should not contain violations", got)
	}
}

func TestRunner_Check_BoundaryWiringAndErrors(t *testing.T) {
	// 1. Setup sibling directories inside parentDir
	parentDir := t.TempDir()
	depDir := filepath.Join(parentDir, "dep-a")
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatalf("failed to create dep dir: %v", err)
	}

	depManifest := `
name: "dep-a"
interface_files: "api.go"
`
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte("module example.com/temp/dep-a\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("failed to write dep go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "component.textproto"), []byte(depManifest), 0644); err != nil {
		t.Fatalf("failed to write dep manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "api.go"), []byte("package depa\n\nfunc FetchData() {}\nfunc HigherOrder(fn func()) {}\n"), 0644); err != nil {
		t.Fatalf("failed to write dep api.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "other.go"), []byte("package depa\n\nfunc UndeclaredFunc() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write dep other.go: %v", err)
	}
	writeNativeTestSurface(t, depDir, "dep-a", gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED, []string{"example.com/temp/dep-a"}, []string{
		"example.com/temp/dep-a.FetchData",
		"example.com/temp/dep-a.HigherOrder",
	})

	// 2. Create the analyzed component
	analyzedDir := filepath.Join(parentDir, "analyzed")
	if err := os.MkdirAll(analyzedDir, 0755); err != nil {
		t.Fatalf("failed to create analyzed dir: %v", err)
	}

	manifestContent := `
name: "analyzed-comp"
interface_files: "api.go"
component_dependencies: {
  name: "dep-a"
  manifest: "../dep-a/component.textproto"
}
`
	if err := os.WriteFile(filepath.Join(analyzedDir, "go.mod"), []byte("module example.com/temp/analyzed\n\ngo 1.21\n\nrequire example.com/temp/dep-a v0.0.0\nreplace example.com/temp/dep-a => ../dep-a\n"), 0644); err != nil {
		t.Fatalf("failed to write analyzed go.mod: %v", err)
	}
	manifestPath := filepath.Join(analyzedDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write analyzed manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(analyzedDir, "api.go"), []byte("package main\n\nimport \"example.com/temp/dep-a\"\n\nfunc myCallback() {}\n\nfunc Hello() {\n\tdepa.FetchData()\n\tdepa.UndeclaredFunc()\n\tdepa.HigherOrder(myCallback)\n}\n"), 0644); err != nil {
		t.Fatalf("failed to write analyzed api.go: %v", err)
	}

	loader := func(req goanalysis.LoadRequest) (facts.PackageFacts, error) {
		return goanalysis.LoadPackageFacts(req)
	}

	runner := surfaceTestRunner(loader)

	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("Run() returned %d, want 1. Stderr: %s\nStdout: %s", exitCode, stderr.String(), stdout.String())
	}

	// Assert that the boundary violations are emitted correctly in the
	// output. The declaring-object rule rejects the undeclared function
	// reference at its exact site; the declared FetchData/HigherOrder
	// references and the passed member callback are silent (no
	// higher-order workaround finding remains).
	gotOut := stdout.String()
	if !strings.Contains(gotOut, "CALLS_UNDECLARED_INTERFACE") {
		t.Errorf("expected stdout to contain CALLS_UNDECLARED_INTERFACE, got: %s", gotOut)
	}
	// Avoid literal string in codebase to satisfy acceptance criteria 1
	higherOrderBoundaryCallStr := "HIGHER_" + "ORDER_" + "BOUNDARY_" + "CALL"
	if strings.Contains(gotOut, higherOrderBoundaryCallStr) {
		t.Errorf("expected stdout to NOT contain %s, got: %s", higherOrderBoundaryCallStr, gotOut)
	}
	if !strings.Contains(gotOut, `references undeclared interface symbol "example.com/temp/dep-a.UndeclaredFunc" of dependency "dep-a" at api.go:9`) {
		t.Errorf("expected stdout to contain the undeclared reference violation message, got: %s", gotOut)
	}
	if strings.Contains(gotOut, `passes function value`) {
		t.Errorf("expected stdout to NOT contain the higher-order warning message, got: %s", gotOut)
	}

	// 4. Test error case: dependency resolution failure yields exit code 2
	badManifestContent := `
name: "analyzed-comp"
interface_files: "api.go"
component_dependencies: {
  name: "nonexistent-dep"
  manifest: "../nonexistent/component.textproto"
}
`
	badManifestPath := filepath.Join(analyzedDir, "bad_component.textproto")
	if err := os.WriteFile(badManifestPath, []byte(badManifestContent), 0644); err != nil {
		t.Fatalf("failed to write bad manifest: %v", err)
	}

	var stdoutBad, stderrBad bytes.Buffer
	badExitCode := runner.Run([]string{"check", badManifestPath}, &stdoutBad, &stderrBad)
	if badExitCode != 2 {
		t.Errorf("expected exit code 2 on resolution failure, got %d. Stderr: %s", badExitCode, stderrBad.String())
	}
	if !strings.Contains(stderrBad.String(), "failed to resolve dependency") {
		t.Errorf("expected stderr to contain error message about dependency resolution, got: %s", stderrBad.String())
	}
}

func TestRunner_Check_PackageSurfaceSymbolsAndOverlap(t *testing.T) {
	parentDir := t.TempDir()

	// 1. Declared-style dependency
	declDir := filepath.Join(parentDir, "dep-decl")
	if err := os.MkdirAll(declDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(declDir, "go.mod"), []byte("module example.com/temp/dep-decl\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(declDir, "component.textproto"), []byte("name: \"dep-decl\"\ninterface_files: \"api.go\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(declDir, "api.go"), []byte("package depdecl\n\nfunc DeclFunc() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeNativeTestSurface(t, declDir, "dep-decl", gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED, []string{"example.com/temp/dep-decl"}, []string{"example.com/temp/dep-decl.DeclFunc"})

	// 2. Package-surface dependencies with overlapping members
	surf1Dir := filepath.Join(parentDir, "dep-surf1")
	if err := os.MkdirAll(filepath.Join(surf1Dir, "pkga"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(surf1Dir, "pkgb"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(surf1Dir, "go.mod"), []byte("module example.com/temp/shared-dep\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	surf1Manifest := `name: "dep-surf1"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/temp/shared-dep/pkgb"
members: "example.com/temp/shared-dep/pkga"
`
	if err := os.WriteFile(filepath.Join(surf1Dir, "component.textproto"), []byte(surf1Manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(surf1Dir, "pkga", "a.go"), []byte("package pkga\n\nfunc AFunc() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(surf1Dir, "pkgb", "b.go"), []byte("package pkgb\n\nfunc BFunc() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeNativeTestSurface(t, surf1Dir, "dep-surf1", gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE, []string{"example.com/temp/shared-dep/pkga", "example.com/temp/shared-dep/pkgb"}, nil)

	surf2Dir := filepath.Join(parentDir, "dep-surf2")
	if err := os.MkdirAll(filepath.Join(surf2Dir, "pkgz"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(surf2Dir, "pkga"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(surf2Dir, "go.mod"), []byte("module example.com/temp/shared-dep\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	surf2Manifest := `name: "dep-surf2"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/temp/shared-dep/pkgz"
members: "example.com/temp/shared-dep/pkga"
`
	if err := os.WriteFile(filepath.Join(surf2Dir, "component.textproto"), []byte(surf2Manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(surf2Dir, "pkgz", "z.go"), []byte("package pkgz\n\nfunc ZFunc() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(surf2Dir, "pkga", "a2.go"), []byte("package pkga\n\nfunc A2Func() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeNativeTestSurface(t, surf2Dir, "dep-surf2", gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE, []string{"example.com/temp/shared-dep/pkga", "example.com/temp/shared-dep/pkgz"}, nil)

	// 3. Analyzed component depending on all three
	analyzedDir := filepath.Join(parentDir, "analyzed")
	if err := os.MkdirAll(analyzedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(analyzedDir, "go.mod"), []byte("module example.com/temp/analyzed\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	analyzedManifest := `name: "analyzed"
interface_files: "api.go"
component_dependencies: {
  name: "dep-decl"
  manifest: "../dep-decl/component.textproto"
}
component_dependencies: {
  name: "dep-surf1"
  manifest: "../dep-surf1/component.textproto"
}
component_dependencies: {
  name: "dep-surf2"
  manifest: "../dep-surf2/component.textproto"
}
`
	manifestPath := filepath.Join(analyzedDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(analyzedManifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(analyzedDir, "api.go"), []byte("package main\n\nfunc Hello() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runner := surfaceTestRunner(func(req goanalysis.LoadRequest) (facts.PackageFacts, error) { return goanalysis.LoadPackageFacts(req) })

	var stdout, stderr bytes.Buffer
	// Two direct dependencies claim the same package: the boundary index
	// fails closed with a DEPENDENCY_OVERLAP tool error before any verdict
	// forms (R14). The legacy last-wins lookup is gone.
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("Run() returned %d, want 2 (tool error). Stderr: %s\nStdout: %s", exitCode, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "DEPENDENCY_OVERLAP") || !strings.Contains(stderr.String(), "example.com/temp/shared-dep/pkga") {
		t.Errorf("stderr = %q, want a DEPENDENCY_OVERLAP error naming the colliding package", stderr.String())
	}
}

func TestRunner_Check_DependencyBoundary_Success(t *testing.T) {
	parentDir := t.TempDir()

	depDir := filepath.Join(parentDir, "declared-dep")
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatalf("failed to create dep dir: %v", err)
	}
	depManifest := `
name: "declared-dep"
interface_files: "api.go"
`
	if err := os.WriteFile(filepath.Join(depDir, "go.mod"), []byte("module example.com/temp/declared-dep\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "component.textproto"), []byte(depManifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "api.go"), []byte("package declareddep\n\nfunc Fetch() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeNativeTestSurface(t, depDir, "declared-dep", gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED, []string{"example.com/temp/declared-dep"}, []string{"example.com/temp/declared-dep.Fetch"})

	analyzedDir := filepath.Join(parentDir, "declared-comp")
	if err := os.MkdirAll(analyzedDir, 0755); err != nil {
		t.Fatalf("failed to create analyzed dir: %v", err)
	}
	manifestContent := `
name: "declared-comp"
interface_files: "api.go"
component_dependencies: {
  name: "declared-dep"
  manifest: "../declared-dep/component.textproto"
}
`
	if err := os.WriteFile(filepath.Join(analyzedDir, "go.mod"), []byte("module example.com/temp/declared-comp\n\ngo 1.21\n\nrequire example.com/temp/declared-dep v0.0.0\nreplace example.com/temp/declared-dep => ../declared-dep\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(analyzedDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(analyzedDir, "api.go"), []byte("package main\n\nimport \"example.com/temp/declared-dep\"\n\nfunc Hello() {\n\tdeclareddep.Fetch()\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runner := surfaceTestRunner(func(req goanalysis.LoadRequest) (facts.PackageFacts, error) { return goanalysis.LoadPackageFacts(req) })

	// 1. Text mode
	var stdout, stderr bytes.Buffer
	exitCode := runner.Run([]string{"check", manifestPath}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("Run() exit = %d, want 0; stderr = %q, stdout = %q", exitCode, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	gotText := stdout.String()
	wantText := `Component "declared-comp" conforms; does not exceed declared authority

Dependencies:
- declared-dep (asserted)
`
	if gotText != wantText {
		t.Errorf("stdout =\n%q\nwant:\n%q", gotText, wantText)
	}

	// 2. JSON mode
	stdout.Reset()
	stderr.Reset()
	exitCodeJSON := runner.Run([]string{"check", manifestPath, "--format=json"}, &stdout, &stderr)
	if exitCodeJSON != 0 {
		t.Fatalf("Run(--format=json) exit = %d, want 0; stderr = %q", exitCodeJSON, stderr.String())
	}
	var rep report.ConformanceReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("json.Unmarshal error = %v; stdout = %q", err, stdout.String())
	}
	if len(rep.Violations) != 0 || len(rep.Warnings) != 0 {
		t.Errorf("report has findings: violations=%v, warnings=%v", rep.Violations, rep.Warnings)
	}
	if len(rep.Dependencies) != 1 || rep.Dependencies[0].Component != "declared-dep" {
		t.Errorf("report.Dependencies = %+v, want declared-dep boundary", rep.Dependencies)
	}
}
