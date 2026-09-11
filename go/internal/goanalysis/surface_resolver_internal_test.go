package goanalysis

import (
	"bytes"
	"os"
	"slices"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

func TestResolveDependencySurface_DoesNotCallPackageLoader(t *testing.T) {
	key := stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		ClassifierHash:   "classifier-hash",
		MapFormatVersion: 1,
	}
	surfaceBytes, err := artifactio.MarshalSurface(&gen.SurfaceManifest{
		FormatVersion:  artifactio.SurfaceFormatVersion,
		Component:      "dep",
		InterfaceStyle: gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE,
		Authority:      &gen.AuthorityDeclaration{Authority: gen.Authority_UNKNOWN},
		Packages:       []string{"example.com/dep"},
		Namespace:      "namespace-a",
		SdkKey: &gen.SDKKey{
			ToolchainVersion: key.ToolchainVersion,
			Goos:             key.GOOS,
			Goarch:           key.GOARCH,
			ClassifierHash:   key.ClassifierHash,
			MapFormatVersion: key.MapFormatVersion,
		},
		ProducerVersion: "test",
	})
	if err != nil {
		t.Fatalf("MarshalSurface: %v", err)
	}

	previousLoader := loadPackages
	called := false
	loadPackages = func(*packages.Config, ...string) ([]*packages.Package, error) {
		called = true
		t.Fatal("surface resolver called go/packages loader")
		return nil, nil
	}
	t.Cleanup(func() { loadPackages = previousLoader })

	got, err := ResolveDependencySurface(DependencySurfaceRequest{
		Dependency:   manifest.ComponentDependency{Name: "dep", Manifest: "dep.component.textproto"},
		Binding:      &packagelayout.DependencyArtifactBinding{Dependency: "dep", Surface: "dep.surface.json", Provenance: packagelayout.DependencyArtifactProvenanceAsserted},
		Namespace:    "namespace-a",
		ExpectedSDK:  key,
		Mode:         DependencySurfaceLayout,
		WorkspaceDir: "workspace",
		ReadFile: func(path string) ([]byte, error) {
			if path == "workspace/dep.surface.json" {
				return bytes.Clone(surfaceBytes), nil
			}
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("ResolveDependencySurface: %v", err)
	}
	if called {
		t.Fatal("package loader seam was called")
	}
	if got.Component != "dep" {
		t.Fatalf("component = %q, want dep", got.Component)
	}
}

func TestResolveXToolsSurface_DoesNotReadOrLoadForeignSource(t *testing.T) {
	key := stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		ClassifierHash:   "adc198ed82f2ae81de8f2d00e26a0c90a22a1317a19579476ab0d7e33cc52eed",
		MapFormatVersion: 1,
	}
	surfaceBytes, err := os.ReadFile("../xtools/component.surface.json")
	if err != nil {
		t.Fatalf("read checked-in x-tools surface: %v", err)
	}

	previousLoader := loadPackages
	loadPackages = func(*packages.Config, ...string) ([]*packages.Package, error) {
		t.Fatal("x-tools surface resolution called go/packages loader")
		return nil, nil
	}
	t.Cleanup(func() { loadPackages = previousLoader })

	got, err := ResolveDependencySurface(DependencySurfaceRequest{
		Dependency: manifest.ComponentDependency{Name: "x-tools", Manifest: "../xtools/component.textproto"},
		Binding: &packagelayout.DependencyArtifactBinding{
			Dependency: "x-tools",
			Surface:    "xtools.surface.json",
			Provenance: packagelayout.DependencyArtifactProvenanceAsserted,
		},
		Namespace:    "upstream",
		ExpectedSDK:  key,
		Mode:         DependencySurfaceLayout,
		WorkspaceDir: "workspace",
		ReadFile: func(path string) ([]byte, error) {
			if path != "workspace/xtools.surface.json" {
				t.Fatalf("surface resolver attempted an undeclared foreign read: %q", path)
			}
			return bytes.Clone(surfaceBytes), nil
		},
	})
	if err != nil {
		t.Fatalf("ResolveDependencySurface: %v", err)
	}
	if got.Component != "x-tools" || got.Provenance != facts.DependencyProvenanceAsserted || got.Freshness != facts.DependencyFreshnessBuildGraph {
		t.Fatalf("x-tools dependency status = %+v, want asserted/build-graph", got)
	}
	if len(got.Packages) != 31 || !slices.Contains(got.Packages, "golang.org/x/tools/go/packages") || !slices.Contains(got.Packages, "golang.org/x/tools/go/ssa/ssautil") {
		t.Fatalf("x-tools dependency packages = %v, want the complete package-surface closure", got.Packages)
	}
}
