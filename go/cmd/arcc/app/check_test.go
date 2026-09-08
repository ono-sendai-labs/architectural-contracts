package app_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

// seamRunner returns a Runner with hermetic seams: a fake loader whose facts
// and error are caller-provided, a mock analyzer, a fake surface-inputs
// loader, a fake SDK-key resolver, and a recording artifact writer that can
// be made to fail. The manifest and member sources live in dir.
type seamRunnerOpts struct {
	pkgPath       string
	loaderErr     error
	loaderCalled  *bool
	findings      []capanalyzer.CapabilityFinding
	resolverErr   error
	writerErrPath string
	resolverKey   stdlibauthority.SDKKey
	writerErr     error
}

func seamRunner(t *testing.T, o seamRunnerOpts) (*app.Runner, map[string][]byte, *[]string) {
	t.Helper()
	recorded := map[string][]byte{}
	var written []string
	fullKey := o.resolverKey
	if fullKey.ToolchainVersion == "" {
		fullKey = fullSDKKey
	}
	runner := &app.Runner{
		Loader: func(goanalysis.LoadRequest) (facts.PackageFacts, error) {
			if o.loaderCalled != nil {
				*o.loaderCalled = true
			}
			if o.loaderErr != nil {
				return facts.PackageFacts{}, o.loaderErr
			}
			return facts.PackageFacts{
				Packages: []facts.PackageFact{{ImportPath: o.pkgPath}},
			}, nil
		},
		Analyzer:            &mockAnalyzer{findings: o.findings},
		SurfaceInputsLoader: func(goanalysis.LoadRequest) (goanalysis.SurfaceInputs, error) { return goanalysis.SurfaceInputs{}, nil },
		KeyResolver: func(req app.SDKKeyRequest) (stdlibauthority.SDKKey, error) {
			if o.resolverErr != nil {
				return stdlibauthority.SDKKey{}, o.resolverErr
			}
			return fullKey, nil
		},
		ArtifactWriter: func(path string, data []byte) error {
			if o.writerErr != nil && (o.writerErrPath == "" || o.writerErrPath == path) {
				return o.writerErr
			}
			recorded[path] = append([]byte(nil), data...)
			written = append(written, path)
			return nil
		},
	}
	return runner, recorded, &written
}

var fullSDKKey = stdlibauthority.SDKKey{
	ToolchainVersion: "go1.26.4",
	GOOS:             "linux",
	GOARCH:           "amd64",
	ClassifierHash:   "abc123",
	MapFormatVersion: 1,
}

// artifactFixture writes a package-surface component: the manifest plus one
// member source file, so the digest sources read from disk succeed.
func artifactFixture(t *testing.T, pkgPath string) (dir, manifestPath string) {
	t.Helper()
	dir = t.TempDir()
	short := pkgPath[strings.LastIndex(pkgPath, "/")+1:]
	manifest := fmt.Sprintf("name: %q\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: %q\n", short, pkgPath)
	manifestPath = filepath.Join(dir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "member.go"), []byte("package "+short+"\n\nfunc Exported() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, manifestPath
}

func TestRunner_Check_ParseDiagnostics(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantPieces []string
	}{
		{
			name:       "empty report-out",
			args:       []string{"--report-out="},
			wantPieces: []string{"--report-out", "empty"},
		},
		{
			name:       "missing report-out value",
			args:       []string{"--report-out"},
			wantPieces: []string{"--report-out", "missing"},
		},
		{
			name:       "duplicate report-out",
			args:       []string{"--report-out=a", "--report-out=b"},
			wantPieces: []string{"--report-out", "duplicate"},
		},
		{
			name:       "empty surface-out",
			args:       []string{"--surface-out="},
			wantPieces: []string{"--surface-out", "empty"},
		},
		{
			name:       "missing surface-out value",
			args:       []string{"--surface-out"},
			wantPieces: []string{"--surface-out", "missing"},
		},
		{
			name:       "duplicate surface-out",
			args:       []string{"--surface-out=a", "--surface-out=b"},
			wantPieces: []string{"--surface-out", "duplicate"},
		},
		{
			name:       "empty stdlib-map",
			args:       []string{"--stdlib-map=", "--surface-out=s.json"},
			wantPieces: []string{"--stdlib-map", "empty"},
		},
		{
			name:       "missing stdlib-map value",
			args:       []string{"--stdlib-map"},
			wantPieces: []string{"--stdlib-map", "missing"},
		},
		{
			name:       "duplicate stdlib-map",
			args:       []string{"--stdlib-map=a", "--stdlib-map=b", "--surface-out=s.json"},
			wantPieces: []string{"--stdlib-map", "duplicate"},
		},
		{
			name:       "duplicate verdict-only",
			args:       []string{"--report-verdict-only", "--report-verdict-only", "--report-out=r.json"},
			wantPieces: []string{"--report-verdict-only", "duplicate"},
		},
		{
			name:       "verdict-only without report-out",
			args:       []string{"--report-verdict-only"},
			wantPieces: []string{"--report-verdict-only", "--report-out"},
		},
		{
			name:       "stdlib-map without surface-out",
			args:       []string{"--stdlib-map=m.json"},
			wantPieces: []string{"--stdlib-map", "--surface-out"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, manifestPath := artifactFixture(t, "example.com/temp/diag")
			var called bool
			runner, _, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/diag", loaderCalled: &called})
			args := append([]string{"check", manifestPath}, tt.args...)
			stdout, stderr, code := runRunnerFromWorkspace(t, t.TempDir(), runner, args)
			if code != 2 {
				t.Fatalf("exit = %d, want 2; stderr = %q", code, stderr)
			}
			if called {
				t.Error("analysis ran despite a usage error")
			}
			for _, piece := range tt.wantPieces {
				if !strings.Contains(stderr, piece) {
					t.Errorf("stderr = %q, want it to name %q", stderr, piece)
				}
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty on usage error", stdout)
			}
		})
	}
}

func TestRunner_Check_LayoutSurfaceRequiresStdlibMap(t *testing.T) {
	_, manifestPath := artifactFixture(t, "example.com/temp/diag")
	var called bool
	runner, _, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/diag", loaderCalled: &called})
	stdout, stderr, code := runRunnerFromWorkspace(t, t.TempDir(), runner, []string{
		"check", manifestPath, "--package-layout=layout.json", "--surface-out=s.json",
	})
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr = %q", code, stderr)
	}
	if called {
		t.Error("analysis ran despite a usage error")
	}
	if !strings.Contains(stderr, "--stdlib-map") {
		t.Errorf("stderr = %q, want it to name --stdlib-map", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestRunner_Check_EmitsBothArtifacts(t *testing.T) {
	_, manifestPath := artifactFixture(t, "example.com/temp/emit")
	runner, recorded, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/emit"})
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "c.report.json")
	surfacePath := filepath.Join(dir, "c.surface.json")
	mapPath := filepath.Join(dir, "map.json")

	stdout, _, code := runRunnerFromWorkspace(t, t.TempDir(), runner, []string{
		"check", manifestPath,
		"--report-out=" + reportPath,
		"--surface-out=" + surfacePath,
		"--stdlib-map=" + mapPath,
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	persisted, err := artifactio.DecodeReport(recorded[reportPath])
	if err != nil {
		t.Fatalf("decode report artifact: %v", err)
	}
	if persisted.Verdict != "pass" {
		t.Errorf("report verdict = %q, want pass", persisted.Verdict)
	}
	surface, err := artifactio.DecodeSurface(strings.NewReader(string(recorded[surfacePath])))
	if err != nil {
		t.Fatalf("decode surface artifact: %v", err)
	}
	if surface.Component != "emit" {
		t.Errorf("surface component = %q, want emit", surface.Component)
	}
	if surface.SdkKey.GetToolchainVersion() != fullSDKKey.ToolchainVersion ||
		surface.SdkKey.GetClassifierHash() != fullSDKKey.ClassifierHash {
		t.Errorf("surface SDK key = %+v, want the resolver's key", surface.SdkKey)
	}
	// Ordinary display is preserved alongside the artifacts.
	if !strings.Contains(stdout, "conforms") {
		t.Errorf("stdout = %q, want the text report", stdout)
	}
}

func TestRunner_Check_VerdictOnlySeparatesPolicyFromExecution(t *testing.T) {
	_, manifestPath := artifactFixture(t, "example.com/temp/verdict")
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "c.report.json")
	surfacePath := filepath.Join(dir, "c.surface.json")
	args := []string{
		"check", manifestPath,
		"--report-out=" + reportPath,
		"--surface-out=" + surfacePath,
		"--stdlib-map=" + filepath.Join(dir, "map.json"),
		"--report-verdict-only",
	}

	// A violating component: analysis ran, so verdict-only exits 0.
	violating := &mockAnalyzer{findings: []capanalyzer.CapabilityFinding{{
		Package:    "example.com/temp/verdict",
		Capability: "FILES",
		Class:      capanalyzer.TrueAuthority,
		CallPath:   []capanalyzer.Frame{{Func: "pkg.F", File: "member.go", Line: 3}},
	}}}
	runner, recorded, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/verdict", findings: violating.findings})
	_, _, code := runRunnerFromWorkspace(t, t.TempDir(), runner, args)
	if code != 0 {
		t.Fatalf("violating verdict-only exit = %d, want 0", code)
	}
	persisted, err := artifactio.DecodeReport(recorded[reportPath])
	if err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if persisted.Verdict != "fail" {
		t.Errorf("verdict = %q, want fail", persisted.Verdict)
	}

	// A tool error still exits 2 in verdict-only mode.
	var called bool
	failing, _, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/verdict", loaderErr: errors.New("boom"), loaderCalled: &called})
	_, _, code = runRunnerFromWorkspace(t, t.TempDir(), failing, args)
	if code != 2 {
		t.Fatalf("tool-error verdict-only exit = %d, want 2", code)
	}
}

func TestRunner_Check_OrdinaryExitsPreserved(t *testing.T) {
	_, manifestPath := artifactFixture(t, "example.com/temp/exits")
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "c.report.json")
	base := []string{"check", manifestPath, "--report-out=" + reportPath}

	// Violation without verdict-only: exit 1.
	violating := &mockAnalyzer{findings: []capanalyzer.CapabilityFinding{{
		Package:    "example.com/temp/exits",
		Capability: "FILES",
		Class:      capanalyzer.TrueAuthority,
		CallPath:   []capanalyzer.Frame{{Func: "pkg.F", File: "member.go", Line: 3}},
	}}}
	runner, recorded, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/exits", findings: violating.findings})
	stdout, stderr, code := runRunnerFromWorkspace(t, t.TempDir(), runner, base)
	if code != 1 {
		t.Fatalf("violation exit = %d, want 1; stderr = %q", code, stderr)
	}
	persisted, err := artifactio.DecodeReport(recorded[reportPath])
	if err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if persisted.Verdict != "fail" {
		t.Errorf("verdict = %q, want fail", persisted.Verdict)
	}
	if !strings.Contains(stdout, "Violations") {
		t.Errorf("stdout = %q, want the text report", stdout)
	}

	// Tool error: exit 2.
	var called bool
	failing, _, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/exits", loaderErr: errors.New("boom"), loaderCalled: &called})
	if _, _, code = runRunnerFromWorkspace(t, t.TempDir(), failing, base); code != 2 {
		t.Fatalf("tool error exit = %d, want 2", code)
	}
}

func TestRunner_Check_KeyResolverFailsClosed(t *testing.T) {
	_, manifestPath := artifactFixture(t, "example.com/temp/keyfail")
	dir := t.TempDir()
	args := []string{
		"check", manifestPath,
		"--report-out=" + filepath.Join(dir, "r.json"),
		"--surface-out=" + filepath.Join(dir, "s.json"),
		"--stdlib-map=" + filepath.Join(dir, "map.json"),
	}
	runner, recorded, _ := seamRunner(t, seamRunnerOpts{
		pkgPath:     "example.com/temp/keyfail",
		resolverErr: errors.New("stdlib map for the target SDK key is unavailable"),
	})
	_, stderr, code := runRunnerFromWorkspace(t, t.TempDir(), runner, args)
	if code != 2 {
		t.Fatalf("key failure exit = %d, want 2; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "SDK key") && !strings.Contains(stderr, "sdk key") {
		t.Errorf("stderr = %q, want it to name the SDK key failure", stderr)
	}
	// No artifact is published when the key fails closed.
	if len(recorded) != 0 {
		t.Errorf("artifacts written despite key failure: %v", recorded)
	}
}

func TestRunner_Check_WriteFailurePreservesPreviousTarget(t *testing.T) {
	_, manifestPath := artifactFixture(t, "example.com/temp/wfail")
	dir := t.TempDir()
	surfacePath := filepath.Join(dir, "c.surface.json")
	previous := []byte(`{"previous": true}`)
	if err := os.WriteFile(surfacePath, previous, 0o644); err != nil {
		t.Fatal(err)
	}
	runner, _, _ := seamRunner(t, seamRunnerOpts{
		pkgPath:       "example.com/temp/wfail",
		writerErr:     errors.New("disk full"),
		writerErrPath: surfacePath,
	})
	_, stderr, code := runRunnerFromWorkspace(t, t.TempDir(), runner, []string{
		"check", manifestPath,
		"--report-out=" + filepath.Join(dir, "c.report.json"),
		"--surface-out=" + surfacePath,
		"--stdlib-map=" + filepath.Join(dir, "map.json"),
	})
	if code != 2 {
		t.Fatalf("write failure exit = %d, want 2; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, surfacePath) {
		t.Errorf("stderr = %q, want it to name the target path", stderr)
	}
	got, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(previous) {
		t.Errorf("previous surface target was disturbed: %q", got)
	}
}

func TestRunner_Check_ArtifactBytesDeterministic(t *testing.T) {
	_, manifestPath := artifactFixture(t, "example.com/temp/det")
	first, rec1, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/det"})
	second, rec2, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/temp/det"})
	dir := t.TempDir()
	args := []string{
		"check", manifestPath,
		"--report-out=" + filepath.Join(dir, "c.report.json"),
		"--surface-out=" + filepath.Join(dir, "c.surface.json"),
		"--stdlib-map=" + filepath.Join(dir, "map.json"),
	}
	for _, tc := range []struct {
		runner   *app.Runner
		recorded map[string][]byte
	}{
		{first, rec1},
		{second, rec2},
	} {
		if _, _, code := runRunnerFromWorkspace(t, t.TempDir(), tc.runner, args); code != 0 {
			t.Fatalf("invocation exited %d", code)
		}
	}
	if len(rec1[""]) != 0 {
		t.Fatal("sanity: recordings empty")
	}
	for path, data := range rec1 {
		if string(rec2[path]) != string(data) {
			t.Errorf("artifact %s differs between identical invocations", path)
		}
	}
	if len(rec1) != 2 {
		t.Errorf("recorded %d artifacts, want 2 (report and surface)", len(rec1))
	}
}
