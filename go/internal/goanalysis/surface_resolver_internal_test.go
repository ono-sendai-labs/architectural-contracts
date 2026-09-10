package goanalysis

import (
	"bytes"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
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
