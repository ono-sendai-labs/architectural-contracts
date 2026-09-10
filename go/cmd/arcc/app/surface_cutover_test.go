package app_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
)

func writeNativeTestSurface(t *testing.T, dependencyRoot, component string, style gen.InterfaceStyle, packages, symbols []string) {
	t.Helper()
	m := &gen.SurfaceManifest{
		FormatVersion:  artifactio.SurfaceFormatVersion,
		Component:      component,
		InterfaceStyle: style,
		Authority:      &gen.AuthorityDeclaration{Authority: gen.Authority_DECLARED},
		Packages:       append([]string(nil), packages...),
		Symbols:        append([]string(nil), symbols...),
		Namespace:      hostpolicy.NamespaceID,
		SdkKey: &gen.SDKKey{
			ToolchainVersion: fullSDKKey.ToolchainVersion,
			Goos:             fullSDKKey.GOOS,
			Goarch:           fullSDKKey.GOARCH,
			ClassifierHash:   fullSDKKey.ClassifierHash,
			MapFormatVersion: fullSDKKey.MapFormatVersion,
		},
		ProducerVersion: "test-producer",
	}
	if style == gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE {
		m.Symbols = nil
	}
	data, err := artifactio.MarshalSurface(m)
	if err != nil {
		t.Fatalf("MarshalSurface(%s): %v", component, err)
	}
	if err := os.WriteFile(filepath.Join(dependencyRoot, "component.surface.json"), data, 0o644); err != nil {
		t.Fatalf("write surface for %s: %v", component, err)
	}
}

func surfaceTestRunner(loader app.PackageLoader) *app.Runner {
	return &app.Runner{
		Loader: loader,
		AuthorityResolver: func(app.AuthorityRequest) (stdlibauthority.StdlibAuthority, error) {
			return testAuthority{}, nil
		},
	}
}

func TestRunner_Check_ConsumesNativeSurfaceWithoutDependencySource(t *testing.T) {
	root := t.TempDir()
	consumerRoot := filepath.Join(root, "consumer")
	dependencyRoot := filepath.Join(root, "dependency")
	if err := os.MkdirAll(consumerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dependencyRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	consumerManifest := `name: "consumer"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/consumer"
component_dependencies {
  name: "dependency"
  manifest: "../dependency/component.textproto"
}
`
	consumerManifestPath := filepath.Join(consumerRoot, "component.textproto")
	if err := os.WriteFile(consumerManifestPath, []byte(consumerManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependencyRoot, "component.textproto"), []byte("name: \"dependency\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	surfaceData, err := artifactio.MarshalSurface(&gen.SurfaceManifest{
		FormatVersion:  artifactio.SurfaceFormatVersion,
		Component:      "dependency",
		InterfaceStyle: gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE,
		Authority:      &gen.AuthorityDeclaration{Authority: gen.Authority_UNKNOWN},
		Packages:       []string{"example.com/dependency"},
		Namespace:      hostpolicy.NamespaceID,
		SdkKey: &gen.SDKKey{
			ToolchainVersion: fullSDKKey.ToolchainVersion,
			Goos:             fullSDKKey.GOOS,
			Goarch:           fullSDKKey.GOARCH,
			ClassifierHash:   fullSDKKey.ClassifierHash,
			MapFormatVersion: fullSDKKey.MapFormatVersion,
		},
		ProducerVersion: "surface-fixture",
	})
	if err != nil {
		t.Fatalf("MarshalSurface: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dependencyRoot, "component.surface.json"), surfaceData, 0o644); err != nil {
		t.Fatal(err)
	}

	runner, _, _ := seamRunner(t, seamRunnerOpts{pkgPath: "example.com/consumer"})
	stdout, stderr, code := runRunnerFromWorkspace(t, root, runner, []string{
		"check", consumerManifestPath, "--format=json",
	})
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q, stdout = %q", code, stderr, stdout)
	}
	var report struct {
		Dependencies []struct {
			Component  string `json:"component"`
			Provenance string `json:"provenance"`
			Freshness  string `json:"freshness"`
			Authority  string `json:"authority"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode JSON report: %v; stdout = %q", err, stdout)
	}
	if len(report.Dependencies) != 1 {
		t.Fatalf("dependencies = %#v, want one surface boundary", report.Dependencies)
	}
	got := report.Dependencies[0]
	if got.Component != "dependency" || got.Provenance != "ASSERTED" || got.Freshness != "UNKNOWN" || got.Authority != "UNKNOWN" {
		t.Errorf("surface boundary = %#v, want dependency/ASSERTED/UNKNOWN/UNKNOWN", got)
	}
	if strings.Contains(stdout, "dependency package load") {
		t.Errorf("report unexpectedly contains source-loader diagnostics: %q", stdout)
	}
}

func TestRunner_Check_NativeFreshnessStatusesAreByteOnly(t *testing.T) {
	root := t.TempDir()
	consumerRoot := filepath.Join(root, "consumer")
	dependencyRoot := filepath.Join(root, "dependency")
	if err := os.MkdirAll(consumerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dependencyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	depManifestBytes := []byte("name: \"dependency\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \"example.com/dependency\"\n")
	if err := os.WriteFile(filepath.Join(dependencyRoot, "component.textproto"), depManifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependencyRoot, "go.mod"), []byte("module example.com/dependency\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalSource := []byte("package dependency\n\nfunc Exported() {}\n")
	depSourcePath := filepath.Join(dependencyRoot, "dep.go")
	if err := os.WriteFile(depSourcePath, originalSource, 0o644); err != nil {
		t.Fatal(err)
	}
	derived, err := surface.Derive(surface.Input{
		Component:       "dependency",
		Style:           manifest.InterfaceStylePackageSurface,
		Authority:       manifest.UnknownAuthority(),
		Namespace:       hostpolicy.NamespaceID,
		Key:             &fullSDKKey,
		ProducerVersion: "freshness-test",
		MemberPackages:  []string{"example.com/dependency"},
		Manifest:        depManifestBytes,
		Sources:         []surface.SourceFile{{Path: "dep.go", Bytes: originalSource}},
	})
	if err != nil {
		t.Fatalf("surface.Derive: %v", err)
	}
	data, err := artifactio.MarshalSurface(derived)
	if err != nil {
		t.Fatalf("MarshalSurface: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dependencyRoot, "component.surface.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	consumerManifest := "name: \"consumer\"\ninterface_style: INTERFACE_STYLE_PACKAGE_SURFACE\nmembers: \"example.com/consumer\"\ncomponent_dependencies { name: \"dependency\" manifest: \"../dependency/component.textproto\" }\n"
	consumerManifestPath := filepath.Join(consumerRoot, "component.textproto")
	if err := os.WriteFile(consumerManifestPath, []byte(consumerManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	var packageLoaderCalls, dependencyResolverCalls, sourceByteReads, directoryReads int
	runner := &app.Runner{
		Loader: func(goanalysis.LoadRequest) (facts.PackageFacts, error) {
			packageLoaderCalls++
			return facts.PackageFacts{
				Packages: []facts.PackageFact{{ImportPath: "example.com/consumer"}},
				Imports: []facts.ImportEdge{{
					ImportingPackage: "example.com/consumer",
					ImportPath:       "example.com/dependency",
					Resolution:       facts.ImportResolved,
					Site:             facts.SourceSite{File: "consumer.go", Line: 1},
				}},
			}, nil
		},
		AuthorityResolver: func(app.AuthorityRequest) (stdlibauthority.StdlibAuthority, error) {
			return testAuthority{}, nil
		},
		DependencySurfaceResolver: func(req goanalysis.DependencySurfaceRequest) (facts.DependencyInterface, error) {
			dependencyResolverCalls++
			originalReadFile := req.ReadFile
			req.ReadFile = func(path string) ([]byte, error) {
				if strings.HasSuffix(path, ".go") {
					sourceByteReads++
				}
				if originalReadFile != nil {
					return originalReadFile(path)
				}
				return os.ReadFile(path)
			}
			originalReadDir := req.ReadDir
			req.ReadDir = func(path string) ([]fs.DirEntry, error) {
				directoryReads++
				if originalReadDir != nil {
					return originalReadDir(path)
				}
				return os.ReadDir(path)
			}
			return goanalysis.ResolveDependencySurface(req)
		},
	}
	readFreshness := func() report.ConformanceReport {
		t.Helper()
		stdout, stderr, code := runRunnerFromWorkspace(t, root, runner, []string{"check", consumerManifestPath, "--format=json"})
		if code != 0 {
			t.Fatalf("freshness check exit = %d, stderr = %q", code, stderr)
		}
		var got report.ConformanceReport
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("decode freshness report: %v", err)
		}
		return got
	}
	if got := readFreshness(); got.Dependencies[0].Freshness != report.DependencyFreshnessVerified {
		t.Fatalf("unchanged freshness = %q, want VERIFIED", got.Dependencies[0].Freshness)
	}
	if err := os.WriteFile(depSourcePath, []byte("this is intentionally not valid Go source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFreshness(); got.Dependencies[0].Freshness != report.DependencyFreshnessStale || len(got.Warnings) != 1 || got.Warnings[0].Kind != report.DependencySurfaceStale {
		t.Fatalf("changed freshness/report = %#v, want STALE plus one stale warning", got)
	}
	if err := os.Rename(depSourcePath, depSourcePath+".unreadable"); err != nil {
		t.Fatal(err)
	}
	got := readFreshness()
	if got.Dependencies[0].Freshness != report.DependencyFreshnessUnknown {
		t.Fatalf("unreadable freshness = %q, want UNKNOWN", got.Dependencies[0].Freshness)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("UNKNOWN freshness warnings = %#v, want none", got.Warnings)
	}
	if packageLoaderCalls != 3 {
		t.Errorf("package-loader calls = %d, want exactly one consumer load per check and no dependency load", packageLoaderCalls)
	}
	if dependencyResolverCalls != 3 {
		t.Errorf("dependency-surface resolver calls = %d, want one per freshness check", dependencyResolverCalls)
	}
	if sourceByteReads == 0 || directoryReads == 0 {
		t.Errorf("freshness byte-reader instrumentation = source bytes %d, directories %d; want both to prove the native audit path ran", sourceByteReads, directoryReads)
	}
}
