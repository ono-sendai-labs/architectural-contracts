package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
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
