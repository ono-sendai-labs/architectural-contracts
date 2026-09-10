package goanalysis_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
)

func resolverSDKKey() stdlibauthority.SDKKey {
	return stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		BuildTags:        []string{"feature-a", "feature-b"},
		GOEXPERIMENT:     "",
		ClassifierHash:   "classifier-hash",
		MapFormatVersion: 1,
	}
}

func resolverSurface(t *testing.T, component string, authority manifest.AuthorityDeclaration, style manifest.InterfaceStyle, packages, symbols []string, sources []surface.SourceFile, manifestBytes []byte) []byte {
	t.Helper()
	persistedAuthority, capabilities, err := manifest.ToPersisted(authority)
	if err != nil {
		t.Fatalf("manifest.ToPersisted: %v", err)
	}
	key := resolverSDKKey()
	m := &gen.SurfaceManifest{
		FormatVersion:  artifactio.SurfaceFormatVersion,
		Component:      component,
		InterfaceStyle: gen.InterfaceStyle(style),
		Authority:      &gen.AuthorityDeclaration{Authority: persistedAuthority, DeclaredAuthority: capabilities},
		Packages:       packages,
		Symbols:        symbols,
		Namespace:      "namespace-a",
		SdkKey: &gen.SDKKey{
			ToolchainVersion: key.ToolchainVersion,
			Goos:             key.GOOS,
			Goarch:           key.GOARCH,
			CgoEnabled:       key.CgoEnabled,
			BuildTags:        key.BuildTags,
			Goexperiment:     key.GOEXPERIMENT,
			ClassifierHash:   key.ClassifierHash,
			MapFormatVersion: key.MapFormatVersion,
		},
		ProducerVersion: "test-producer",
	}
	if style == manifest.InterfaceStylePackageSurface {
		m.Symbols = nil
	}
	if sources != nil || manifestBytes != nil {
		derived, err := surface.Derive(surface.Input{
			Component:       component,
			Style:           manifest.InterfaceStylePackageSurface,
			Authority:       authority,
			Namespace:       "namespace-a",
			Key:             &key,
			ProducerVersion: "test-producer",
			MemberPackages:  packages,
			Manifest:        manifestBytes,
			Sources:         sources,
		})
		if err != nil {
			t.Fatalf("surface.Derive: %v", err)
		}
		m.Digest = derived.Digest
	}
	data, err := artifactio.MarshalSurface(m)
	if err != nil {
		t.Fatalf("artifactio.MarshalSurface: %v", err)
	}
	return data
}

func resolverReport(t *testing.T, component string, failed bool) []byte {
	t.Helper()
	r := report.ConformanceReport{Component: component}
	if failed {
		r.Violations = []report.Finding{{Kind: report.UndeclaredAuthority, Message: "dependency failed"}}
	}
	data, err := artifactio.MarshalReport(r)
	if err != nil {
		t.Fatalf("artifactio.MarshalReport: %v", err)
	}
	return data
}

func TestResolveDependencySurface_LayoutStatuses(t *testing.T) {
	manifestBytes := []byte("name: \"dep\"\n")
	declared, err := manifest.NewDeclared("FILES")
	if err != nil {
		t.Fatalf("manifest.NewDeclared: %v", err)
	}
	memberSources := []surface.SourceFile{{Path: "dep.go", Bytes: []byte("package dep\n")}}

	tests := []struct {
		name        string
		binding     packagelayout.DependencyArtifactBinding
		surface     []byte
		report      []byte
		wantProv    facts.DependencyProvenance
		wantFresh   facts.DependencyFreshness
		wantStyle   manifest.InterfaceStyle
		wantSymbols []facts.SymbolID
		wantAuth    manifest.AuthorityDeclaration
	}{
		{
			name: "checked pass exact declared symbols",
			binding: packagelayout.DependencyArtifactBinding{
				Dependency: "dep", Surface: "dep.surface.json", Report: "dep.report.json",
				Provenance: packagelayout.DependencyArtifactProvenanceChecked,
			},
			surface:     resolverSurface(t, "dep", declared, manifest.InterfaceStyleUnspecified, []string{"example.com/dep"}, []string{"example.com/dep.API"}, memberSources, manifestBytes),
			report:      resolverReport(t, "dep", false),
			wantProv:    facts.DependencyProvenanceCheckedPass,
			wantFresh:   facts.DependencyFreshnessBuildGraph,
			wantStyle:   manifest.InterfaceStyleUnspecified,
			wantSymbols: []facts.SymbolID{"example.com/dep.API"},
			wantAuth:    declared,
		},
		{
			name: "checked fail",
			binding: packagelayout.DependencyArtifactBinding{
				Dependency: "dep", Surface: "dep.surface.json", Report: "dep.report.json",
				Provenance: packagelayout.DependencyArtifactProvenanceChecked,
			},
			surface:   resolverSurface(t, "dep", declared, manifest.InterfaceStylePackageSurface, []string{"example.com/dep"}, nil, memberSources, manifestBytes),
			report:    resolverReport(t, "dep", true),
			wantProv:  facts.DependencyProvenanceCheckedFail,
			wantFresh: facts.DependencyFreshnessBuildGraph,
			wantStyle: manifest.InterfaceStylePackageSurface,
			wantAuth:  declared,
		},
		{
			name: "asserted package surface",
			binding: packagelayout.DependencyArtifactBinding{
				Dependency: "dep", Surface: "dep.surface.json",
				Provenance: packagelayout.DependencyArtifactProvenanceAsserted,
			},
			surface:   assertedResolverSurface(t, "dep", []string{"example.com/dep"}),
			wantProv:  facts.DependencyProvenanceAsserted,
			wantFresh: facts.DependencyFreshnessBuildGraph,
			wantStyle: manifest.InterfaceStylePackageSurface,
			wantAuth:  manifest.UnknownAuthority(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{"workspace/dep.surface.json": tt.surface}
			if tt.report != nil {
				files["workspace/dep.report.json"] = tt.report
			}
			readFile := func(path string) ([]byte, error) {
				data, ok := files[path]
				if !ok {
					return nil, fmt.Errorf("missing test file %q", path)
				}
				return append([]byte(nil), data...), nil
			}
			got, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
				Dependency:   manifest.ComponentDependency{Name: "dep", Manifest: "dep.component.textproto"},
				Binding:      &tt.binding,
				Namespace:    "namespace-a",
				ExpectedSDK:  resolverSDKKey(),
				Mode:         goanalysis.DependencySurfaceLayout,
				WorkspaceDir: "workspace",
				ReadFile:     readFile,
			})
			if err != nil {
				t.Fatalf("ResolveDependencySurface: %v", err)
			}
			if got.Component != "dep" || got.Provenance != tt.wantProv || got.Freshness != tt.wantFresh || got.InterfaceStyle != tt.wantStyle {
				t.Fatalf("resolved status = %+v, want component/status/style dep/%s/%s/%v", got, tt.wantProv, tt.wantFresh, tt.wantStyle)
			}
			if !equalResolverSymbols(got.Symbols, tt.wantSymbols) {
				t.Errorf("symbols = %v, want %v", got.Symbols, tt.wantSymbols)
			}
			if !manifest.Equal(got.Authority, tt.wantAuth) {
				t.Errorf("authority = %+v, want %+v", got.Authority, tt.wantAuth)
			}
		})
	}
}

func assertedResolverSurface(t *testing.T, component string, packages []string) []byte {
	t.Helper()
	data := resolverSurface(t, component, manifest.UnknownAuthority(), manifest.InterfaceStylePackageSurface, packages, nil, nil, nil)
	var err error
	m, err := artifactio.DecodeSurface(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("decode asserted fixture: %v", err)
	}
	// Asserted surfaces intentionally carry no content digest: the absence of a
	// derived digest is not a claim about the component's source.
	m.Digest = ""
	data, err = artifactio.MarshalSurface(m)
	if err != nil {
		t.Fatalf("encode asserted fixture: %v", err)
	}
	return data
}

func TestResolveDependencySurface_NativeStatusesAreAsserted(t *testing.T) {
	manifestBytes := []byte("name: \"dep\"\n")
	declared, err := manifest.NewDeclared("FILES")
	if err != nil {
		t.Fatalf("manifest.NewDeclared: %v", err)
	}
	sources := []surface.SourceFile{{Path: "dep.go", Bytes: []byte("package dep\n")}}
	surfaceBytes := resolverSurface(t, "dep", declared, manifest.InterfaceStyleUnspecified, []string{"example.com/dep"}, []string{"example.com/dep.API"}, sources, manifestBytes)

	for _, tt := range []struct {
		name       string
		reportData []byte
	}{
		{name: "passing audit report", reportData: resolverReport(t, "dep", false)},
		{name: "failing audit report", reportData: resolverReport(t, "dep", true)},
		{name: "no audit report"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{
				"/declaring/dep.component.surface.json": surfaceBytes,
				"/declaring/dep.component.textproto":    manifestBytes,
			}
			if tt.reportData != nil {
				files["/declaring/dep.component.report.json"] = tt.reportData
			}
			got, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
				DeclaringRoot: "/declaring",
				Dependency:    manifest.ComponentDependency{Name: "dep", Manifest: "dep.component.textproto"},
				Namespace:     "namespace-a",
				ExpectedSDK:   resolverSDKKey(),
				Mode:          goanalysis.DependencySurfaceNative,
				ReadFile: func(path string) ([]byte, error) {
					data, ok := files[path]
					if !ok {
						return nil, fs.ErrNotExist
					}
					return append([]byte(nil), data...), nil
				},
				ReadSourceFiles: func(root string, packages []string) ([]surface.SourceFile, error) {
					return sources, nil
				},
			})
			if err != nil {
				t.Fatalf("ResolveDependencySurface: %v", err)
			}
			if got.Provenance != facts.DependencyProvenanceAsserted || got.Freshness != facts.DependencyFreshnessVerified {
				t.Fatalf("native status = %q/%q, want ASSERTED/VERIFIED", got.Provenance, got.Freshness)
			}
		})
	}
}

func TestResolveDependencySurface_ValidationFailsClosed(t *testing.T) {
	declared, err := manifest.NewDeclared("FILES")
	if err != nil {
		t.Fatalf("manifest.NewDeclared: %v", err)
	}
	manifestBytes := []byte("name: \"dep\"\n")
	baseSurface := resolverSurface(t, "dep", declared, manifest.InterfaceStyleUnspecified, []string{"example.com/dep"}, []string{"example.com/dep.API"}, []surface.SourceFile{{Path: "dep.go", Bytes: []byte("package dep\n")}}, manifestBytes)
	passReport := resolverReport(t, "dep", false)

	tests := []struct {
		name        string
		makeSurface func(t *testing.T) []byte
		binding     packagelayout.DependencyArtifactBinding
		report      []byte
		want        string
	}{
		{
			name: "component mismatch",
			makeSurface: func(t *testing.T) []byte {
				return resolverSurface(t, "other", declared, manifest.InterfaceStyleUnspecified, []string{"example.com/dep"}, []string{"example.com/dep.API"}, []surface.SourceFile{{Path: "dep.go", Bytes: []byte("package dep\n")}}, manifestBytes)
			},
			binding: checkedResolverBinding(), report: passReport, want: "component mismatch",
		},
		{
			name: "namespace mismatch",
			makeSurface: func(t *testing.T) []byte {
				data := baseSurface
				return mutateResolverSurface(t, data, func(m *gen.SurfaceManifest) { m.Namespace = "other-namespace" })
			},
			binding: checkedResolverBinding(), report: passReport, want: "namespace mismatch",
		},
		{
			name: "format version mismatch",
			makeSurface: func(t *testing.T) []byte {
				return replaceResolverSurfaceJSON(t, baseSurface, `"formatVersion": 1`, `"formatVersion": 2`)
			},
			binding: checkedResolverBinding(), report: passReport, want: "format",
		},
		{
			name: "interface style shape",
			makeSurface: func(t *testing.T) []byte {
				return []byte(`{"formatVersion":1,"component":"dep","interfaceStyle":"INTERFACE_STYLE_PACKAGE_SURFACE","symbols":["example.com/dep.API"]}`)
			},
			binding: checkedResolverBinding(), report: passReport, want: "PACKAGE_SURFACE",
		},
		{
			name: "symbol grammar",
			makeSurface: func(t *testing.T) []byte {
				return replaceResolverSurfaceJSON(t, baseSurface, `"example.com/dep.API"`, `"not a symbol!"`)
			},
			binding: checkedResolverBinding(), report: passReport, want: "symbol",
		},
		{
			name: "authority shape",
			makeSurface: func(t *testing.T) []byte {
				return []byte(`{"formatVersion":1,"component":"dep","authority":{"authority":"UNKNOWN","declaredAuthority":["FILES"]}}`)
			},
			binding: checkedResolverBinding(), report: passReport, want: "UNKNOWN",
		},
		{
			name: "SDK goexperiment mismatch",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.SdkKey.Goexperiment = "regabiargs" })
			},
			binding: checkedResolverBinding(), report: passReport, want: "goexperiment",
		},
		{
			name: "SDK toolchain mismatch",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.SdkKey.ToolchainVersion = "go1.25.0" })
			},
			binding: checkedResolverBinding(), report: passReport, want: "toolchain_version",
		},
		{
			name: "SDK GOOS mismatch",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.SdkKey.Goos = "darwin" })
			},
			binding: checkedResolverBinding(), report: passReport, want: "goos",
		},
		{
			name: "SDK GOARCH mismatch",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.SdkKey.Goarch = "arm64" })
			},
			binding: checkedResolverBinding(), report: passReport, want: "goarch",
		},
		{
			name: "SDK cgo mismatch",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.SdkKey.CgoEnabled = true })
			},
			binding: checkedResolverBinding(), report: passReport, want: "cgo_enabled",
		},
		{
			name: "SDK build tags mismatch",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.SdkKey.BuildTags = []string{"feature-c"} })
			},
			binding: checkedResolverBinding(), report: passReport, want: "build_tags",
		},
		{
			name: "SDK classifier mismatch",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.SdkKey.ClassifierHash = "different-classifier" })
			},
			binding: checkedResolverBinding(), report: passReport, want: "classifier_hash",
		},
		{
			name: "SDK map format mismatch",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.SdkKey.MapFormatVersion = 2 })
			},
			binding: checkedResolverBinding(), report: passReport, want: "map_format_version",
		},
		{
			name: "symbol outside package set",
			makeSurface: func(t *testing.T) []byte {
				return mutateResolverSurface(t, baseSurface, func(m *gen.SurfaceManifest) { m.Symbols = []string{"example.com/other.API"} })
			},
			binding: checkedResolverBinding(), report: passReport, want: "outside its concrete package set",
		},
		{
			name: "asserted binding with report",
			makeSurface: func(t *testing.T) []byte {
				return assertedResolverSurface(t, "dep", []string{"example.com/dep"})
			},
			binding: packagelayout.DependencyArtifactBinding{
				Dependency: "dep", Surface: "dep.surface.json", Report: "dep.report.json",
				Provenance: packagelayout.DependencyArtifactProvenanceAsserted,
			},
			report: passReport, want: "must not have a report path",
		},
		{
			name:        "checked binding missing report",
			makeSurface: func(t *testing.T) []byte { return baseSurface },
			binding: packagelayout.DependencyArtifactBinding{
				Dependency: "dep", Surface: "dep.surface.json",
				Provenance: packagelayout.DependencyArtifactProvenanceChecked,
			},
			want: "no report path",
		},
		{
			name:        "report component mismatch",
			makeSurface: func(t *testing.T) []byte { return baseSurface },
			binding:     checkedResolverBinding(), report: resolverReport(t, "other", false), want: "report component mismatch",
		},
		{
			name:        "invalid report",
			makeSurface: func(t *testing.T) []byte { return baseSurface },
			binding:     checkedResolverBinding(), report: []byte("{not-json"), want: "report",
		},
		{
			name:        "missing surface",
			makeSurface: func(t *testing.T) []byte { return nil },
			binding:     checkedResolverBinding(), report: passReport, want: "surface",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{"workspace/dep.surface.json": tt.makeSurface(t)}
			if tt.report != nil {
				files["workspace/dep.report.json"] = tt.report
			}
			got, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
				Dependency:   manifest.ComponentDependency{Name: "dep", Manifest: "dep.component.textproto"},
				Binding:      &tt.binding,
				Namespace:    "namespace-a",
				ExpectedSDK:  resolverSDKKey(),
				Mode:         goanalysis.DependencySurfaceLayout,
				WorkspaceDir: "workspace",
				ReadFile: func(path string) ([]byte, error) {
					data, ok := files[path]
					if !ok {
						return nil, fs.ErrNotExist
					}
					return data, nil
				},
			})
			if err == nil {
				t.Fatalf("ResolveDependencySurface() = %+v, want error containing %q", got, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ResolveDependencySurface() error = %q, want substring %q", err, tt.want)
			}
			if !reflect.DeepEqual(got, facts.DependencyInterface{}) {
				t.Fatalf("failed resolution returned partial facts: %+v", got)
			}
		})
	}
}

func checkedResolverBinding() packagelayout.DependencyArtifactBinding {
	return packagelayout.DependencyArtifactBinding{
		Dependency: "dep", Surface: "dep.surface.json", Report: "dep.report.json",
		Provenance: packagelayout.DependencyArtifactProvenanceChecked,
	}
}

func mutateResolverSurface(t *testing.T, data []byte, mutate func(*gen.SurfaceManifest)) []byte {
	t.Helper()
	m, err := artifactio.DecodeSurface(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("decode resolver surface fixture: %v", err)
	}
	mutate(m)
	encoded, err := artifactio.MarshalSurface(m)
	if err != nil {
		t.Fatalf("encode resolver surface fixture: %v", err)
	}
	return encoded
}

func replaceResolverSurfaceJSON(t *testing.T, data []byte, old, replacement string) []byte {
	t.Helper()
	replaced := strings.Replace(string(data), old, replacement, 1)
	if replaced == string(data) {
		t.Fatalf("resolver fixture anchor %q not found", old)
	}
	return []byte(replaced)
}

func TestResolveDependencySurface_NativeFreshnessStatesRemainByteOnly(t *testing.T) {
	declared, err := manifest.NewDeclared("FILES")
	if err != nil {
		t.Fatalf("manifest.NewDeclared: %v", err)
	}
	manifestBytes := []byte("name: \"dep\"\n")
	sources := []surface.SourceFile{{Path: "dep.go", Bytes: []byte("package dep\n")}}
	data := resolverSurface(t, "dep", declared, manifest.InterfaceStyleUnspecified, []string{"example.com/dep"}, []string{"example.com/dep.API"}, sources, manifestBytes)

	tests := []struct {
		name       string
		surface    []byte
		readSource func(string, []string) ([]surface.SourceFile, error)
		want       facts.DependencyFreshness
	}{
		{name: "changed bytes", surface: data, readSource: func(string, []string) ([]surface.SourceFile, error) {
			return []surface.SourceFile{{Path: "dep.go", Bytes: []byte("package dep // changed\n")}}, nil
		}, want: facts.DependencyFreshnessStale},
		{name: "unreadable source root", surface: data, readSource: func(string, []string) ([]surface.SourceFile, error) {
			return nil, fs.ErrPermission
		}, want: facts.DependencyFreshnessUnknown},
		{name: "empty asserted digest", surface: assertedResolverSurface(t, "dep", []string{"example.com/dep"}), readSource: func(string, []string) ([]surface.SourceFile, error) {
			t.Fatal("empty asserted digest must not enumerate source bytes")
			return nil, nil
		}, want: facts.DependencyFreshnessUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{
				"/declaring/dep.component.surface.json": data,
				"/declaring/dep.component.textproto":    manifestBytes,
			}
			if tt.surface != nil {
				files["/declaring/dep.component.surface.json"] = tt.surface
			}
			got, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
				DeclaringRoot: "/declaring",
				Dependency:    manifest.ComponentDependency{Name: "dep", Manifest: "dep.component.textproto"},
				Namespace:     "namespace-a",
				ExpectedSDK:   resolverSDKKey(),
				Mode:          goanalysis.DependencySurfaceNative,
				ReadFile: func(path string) ([]byte, error) {
					value, ok := files[path]
					if !ok {
						return nil, fs.ErrNotExist
					}
					return value, nil
				},
				ReadSourceFiles: tt.readSource,
			})
			if err != nil {
				t.Fatalf("ResolveDependencySurface: %v", err)
			}
			if got.Freshness != tt.want {
				t.Errorf("freshness = %q, want %q", got.Freshness, tt.want)
			}
		})
	}
}

func TestResolveDependencySurface_RejectsNonCanonicalPackagePath(t *testing.T) {
	oldCanonicalize, oldIsCanonical := hostpolicy.CanonicalizePath, hostpolicy.IsCanonicalPath
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = oldCanonicalize
		hostpolicy.IsCanonicalPath = oldIsCanonical
	})
	hostpolicy.CanonicalizePath = func(path string) string {
		if path == "example.com/dep" {
			return "vendor/example.com/dep"
		}
		return path
	}
	hostpolicy.IsCanonicalPath = func(path string) bool { return path != "example.com/dep" }

	declared := mustDeclaredAuthority(t)
	surfaceBytes := resolverSurface(t, "dep", declared, manifest.InterfaceStyleUnspecified, []string{"example.com/dep"}, []string{"example.com/dep.API"}, nil, nil)
	files := map[string][]byte{
		"workspace/dep.surface.json": surfaceBytes,
		"workspace/dep.report.json":  resolverReport(t, "dep", false),
	}
	got, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
		Dependency:   manifest.ComponentDependency{Name: "dep", Manifest: "dep.component.textproto"},
		Binding:      &packagelayout.DependencyArtifactBinding{Dependency: "dep", Surface: "dep.surface.json", Report: "dep.report.json", Provenance: packagelayout.DependencyArtifactProvenanceChecked},
		Namespace:    "namespace-a",
		ExpectedSDK:  resolverSDKKey(),
		Mode:         goanalysis.DependencySurfaceLayout,
		WorkspaceDir: "workspace",
		ReadFile: func(path string) ([]byte, error) {
			return files[path], nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("ResolveDependencySurface() error = %v, want canonical-path error", err)
	}
	if !reflect.DeepEqual(got, facts.DependencyInterface{}) {
		t.Fatalf("failed resolution returned partial facts: %+v", got)
	}
}

func TestResolveDependencySurface_NamespaceCanonicalizersAreIndependent(t *testing.T) {
	oldCanonicalize, oldIsCanonical, oldNamespace := hostpolicy.CanonicalizePath, hostpolicy.IsCanonicalPath, hostpolicy.NamespaceID
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = oldCanonicalize
		hostpolicy.IsCanonicalPath = oldIsCanonical
		hostpolicy.NamespaceID = oldNamespace
	})

	type namespaceCase struct {
		name   string
		prefix string
		id     string
	}
	cases := []namespaceCase{
		{name: "canonicalizer a", prefix: "a/", id: "namespace-a"},
		{name: "canonicalizer b", prefix: "b/", id: "namespace-b"},
	}
	surfaces := make(map[string][]byte, len(cases))
	for _, tc := range cases {
		data := resolverSurface(t, "dep", manifest.UnknownAuthority(), manifest.InterfaceStylePackageSurface, []string{tc.prefix + "example.com/dep"}, nil, nil, nil)
		surfaces[tc.name] = mutateResolverSurface(t, data, func(m *gen.SurfaceManifest) { m.Namespace = tc.id })
	}
	for _, producer := range cases {
		t.Run(producer.name, func(t *testing.T) {
			hostpolicy.NamespaceID = producer.id
			hostpolicy.CanonicalizePath = func(path string) string {
				if strings.HasPrefix(path, producer.prefix) {
					return path
				}
				return producer.prefix + path
			}
			hostpolicy.IsCanonicalPath = func(path string) bool { return strings.HasPrefix(path, producer.prefix) }

			for _, consumer := range cases {
				t.Run("surface "+consumer.name, func(t *testing.T) {
					files := map[string][]byte{"workspace/dep.surface.json": surfaces[consumer.name]}
					_, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
						Dependency:   manifest.ComponentDependency{Name: "dep", Manifest: "dep.component.textproto"},
						Binding:      &packagelayout.DependencyArtifactBinding{Dependency: "dep", Surface: "dep.surface.json", Provenance: packagelayout.DependencyArtifactProvenanceAsserted},
						Namespace:    consumer.id,
						ExpectedSDK:  resolverSDKKey(),
						Mode:         goanalysis.DependencySurfaceLayout,
						WorkspaceDir: "workspace",
						ReadFile: func(path string) ([]byte, error) {
							return files[path], nil
						},
					})
					if producer.name == consumer.name && err != nil {
						t.Fatalf("own surface rejected: %v", err)
					}
					if producer.name != consumer.name && err == nil {
						t.Fatal("foreign namespace surface accepted")
					}
				})
			}
		})
	}
}

func TestResolveDependencySurface_NativeDefaultReaderUsesOnlyMemberBytes(t *testing.T) {
	root := t.TempDir()
	depRoot := root + "/dep"
	if err := os.MkdirAll(depRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manifestBytes := []byte("name: \"dep\"\n")
	sourceBytes := []byte("package dep\n\nfunc API() {}\n")
	for path, data := range map[string][]byte{
		depRoot + "/go.mod":              []byte("module example.com/dep\n\ngo 1.26\n"),
		depRoot + "/dep.go":              sourceBytes,
		depRoot + "/dep_test.go":         []byte("package dep\nfunc TestOnly() {}\n"),
		depRoot + "/unrelated.txt":       []byte("not a source file"),
		depRoot + "/component.textproto": manifestBytes,
	} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}
	surfaceBytes := resolverSurface(t, "dep", mustDeclaredAuthority(t), manifest.InterfaceStyleUnspecified, []string{"example.com/dep"}, []string{"example.com/dep.API"}, []surface.SourceFile{{Path: "dep.go", Bytes: sourceBytes}}, manifestBytes)
	if err := os.WriteFile(depRoot+"/component.surface.json", surfaceBytes, 0o644); err != nil {
		t.Fatalf("WriteFile(surface): %v", err)
	}
	var readFiles, readDirs []string
	readFile := func(path string) ([]byte, error) {
		readFiles = append(readFiles, path)
		return os.ReadFile(path)
	}
	readDir := func(path string) ([]fs.DirEntry, error) {
		readDirs = append(readDirs, path)
		return os.ReadDir(path)
	}

	got, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
		DeclaringRoot: root,
		Dependency:    manifest.ComponentDependency{Name: "dep", Manifest: "dep/component.textproto"},
		Namespace:     "namespace-a",
		ExpectedSDK:   resolverSDKKey(),
		Mode:          goanalysis.DependencySurfaceNative,
		ReadFile:      readFile,
		ReadDir:       readDir,
	})
	if err != nil {
		t.Fatalf("ResolveDependencySurface: %v", err)
	}
	if got.Freshness != facts.DependencyFreshnessVerified {
		t.Errorf("freshness = %q, want VERIFIED", got.Freshness)
	}
	if len(readDirs) == 0 {
		t.Fatal("default freshness reader did not enumerate a package directory")
	}
	for _, path := range readFiles {
		if strings.HasSuffix(path, "dep_test.go") || strings.HasSuffix(path, "unrelated.txt") {
			t.Errorf("default freshness reader opened excluded file %q", path)
		}
	}
}

func TestResolveDependencySurface_NativeDefaultReaderUnknownForBuildConstraints(t *testing.T) {
	declared := mustDeclaredAuthority(t)
	manifestBytes := []byte("name: \"dep\"\n")
	memberBytes := []byte("package dep\n\nfunc API() {}\n")
	cases := []struct {
		name string
		file string
		data []byte
	}{
		{
			name: "inactive filename",
			file: "dep_windows.go",
			data: []byte("package dep\n\nfunc WindowsOnly() {}\n"),
		},
		{
			name: "go build directive",
			file: "tagged.go",
			data: []byte("//go:build never\n\npackage dep\n"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			depRoot := filepath.Join(root, "dep")
			if err := os.MkdirAll(depRoot, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			files := map[string][]byte{
				"go.mod":              []byte("module example.com/dep\n\ngo 1.26\n"),
				"dep.go":              memberBytes,
				tc.file:               tc.data,
				"component.textproto": manifestBytes,
			}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(depRoot, name), data, 0o644); err != nil {
					t.Fatalf("WriteFile(%s): %v", name, err)
				}
			}
			surfaceBytes := resolverSurface(t, "dep", declared, manifest.InterfaceStyleUnspecified,
				[]string{"example.com/dep"}, []string{"example.com/dep.API"},
				[]surface.SourceFile{{Path: "dep.go", Bytes: memberBytes}}, manifestBytes)
			if err := os.WriteFile(filepath.Join(depRoot, "component.surface.json"), surfaceBytes, 0o644); err != nil {
				t.Fatalf("WriteFile(surface): %v", err)
			}

			got, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
				DeclaringRoot: root,
				Dependency:    manifest.ComponentDependency{Name: "dep", Manifest: "dep/component.textproto"},
				Namespace:     "namespace-a",
				ExpectedSDK:   resolverSDKKey(),
				Mode:          goanalysis.DependencySurfaceNative,
			})
			if err != nil {
				t.Fatalf("ResolveDependencySurface: %v", err)
			}
			if got.Freshness != facts.DependencyFreshnessUnknown {
				t.Fatalf("freshness = %q, want UNKNOWN when source selection is constrained", got.Freshness)
			}
		})
	}
}

func mustDeclaredAuthority(t *testing.T) manifest.AuthorityDeclaration {
	t.Helper()
	declared, err := manifest.NewDeclared("FILES")
	if err != nil {
		t.Fatalf("manifest.NewDeclared: %v", err)
	}
	return declared
}

func equalResolverSymbols(a, b []facts.SymbolID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
