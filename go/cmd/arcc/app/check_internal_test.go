package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
)

// fixtureMap returns a semantically valid stdlib map whose key matches the
// given target under this arcc's classifier rules, so the resolver's
// classifier check passes and target checks are exercised in isolation.
func fixtureMap(t *testing.T, target stdlibmap.TargetConfig) *gen.StdlibMap {
	t.Helper()
	key, err := deriveSDKKey(target)
	if err != nil {
		t.Fatalf("deriveSDKKey: %v", err)
	}
	return &gen.StdlibMap{
		FormatVersion: 1,
		Key: &gen.SDKKey{
			ToolchainVersion: key.ToolchainVersion,
			Goos:             key.GOOS,
			Goarch:           key.GOARCH,
			CgoEnabled:       key.CgoEnabled,
			BuildTags:        key.BuildTags,
			Goexperiment:     key.GOEXPERIMENT,
			ClassifierHash:   key.ClassifierHash,
			MapFormatVersion: key.MapFormatVersion,
		},
		Packages: []*gen.PackageInventory{{Path: "os", Importable: true}},
		Symbols: []*gen.SymbolRecord{
			{Package: "os", Id: "(os.File).Read", Classification: gen.Classification_CAPABILITIES, Capabilities: []string{"FILES"}},
		},
		Inits: []*gen.InitRecord{{Package: "os", Classification: gen.Classification_SAFE}},
		Evidence: []*gen.Evidence{{
			SymbolId:   "(os.File).Read",
			Capability: "FILES",
			Frames:     []*gen.Frame{{Function: "(os.File).Read", File: "f.go", Line: 1}},
		}},
	}
}

func writeMapArtifact(t *testing.T, m *gen.StdlibMap) string {
	t.Helper()
	data, err := artifactio.MarshalMap(m)
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	path := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestAuthorityFromDeclaredMapNativeTargetMismatch fails an explicit map made for
// another native target (review round 1, finding 1): in native mode the
// resolver discovers the current target and rejects a map whose key differs.
func TestAuthorityFromDeclaredMapNativeTargetMismatch(t *testing.T) {
	target := stdlibmap.TargetConfig{
		ToolchainVersion: "go1.26.4",
		GOOS:             "plan9",
		GOARCH:           "amd64",
	}
	path := writeMapArtifact(t, fixtureMap(t, target))
	_, err := authorityFromDeclaredMap(AuthorityRequest{StdlibMapPath: path, InLayoutMode: false})
	if err == nil {
		t.Fatal("expected a native target mismatch error")
	}
	if !strings.Contains(err.Error(), "mismatched fields: goos") {
		t.Errorf("error = %q, want it to name the mismatched target field", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name the artifact path", err)
	}
}

// TestAuthorityFromDeclaredMapNativeTargetMatch accepts an explicit map whose key
// matches the discovered native target.
func TestAuthorityFromDeclaredMapNativeTargetMatch(t *testing.T) {
	discovered, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		t.Skipf("no native toolchain target available: %v", err)
	}
	path := writeMapArtifact(t, fixtureMap(t, discovered))
	auth, err := authorityFromDeclaredMap(AuthorityRequest{StdlibMapPath: path, InLayoutMode: false})
	if err != nil {
		t.Fatalf("authorityFromDeclaredMap: %v", err)
	}
	if key := auth.Key(); key.GOOS != discovered.GOOS || key.GOARCH != discovered.GOARCH {
		t.Errorf("key = %+v, want the discovered target", key)
	}
}

// TestAuthorityFromDeclaredMapClassifierMismatch fails a map stamped with a
// classifier hash this arcc does not compute (I2).
func TestAuthorityFromDeclaredMapClassifierMismatch(t *testing.T) {
	discovered, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		t.Skipf("no native toolchain target available: %v", err)
	}
	m := fixtureMap(t, discovered)
	m.Key.ClassifierHash = "stale-classifier-hash"
	path := writeMapArtifact(t, m)
	_, err = authorityFromDeclaredMap(AuthorityRequest{StdlibMapPath: path, InLayoutMode: false})
	if err == nil {
		t.Fatal("expected a classifier mismatch error")
	}
	if !strings.Contains(err.Error(), "classifier_hash") {
		t.Errorf("error = %q, want it to name classifier_hash", err)
	}
}

// TestAuthorityFromDeclaredMapFormatMismatch fails a map with an unsupported
// format version.
func TestAuthorityFromDeclaredMapFormatMismatch(t *testing.T) {
	discovered, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		t.Skipf("no native toolchain target available: %v", err)
	}
	m := fixtureMap(t, discovered)
	data, err := artifactio.MarshalMap(m)
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["formatVersion"] = 999
	tampered, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = authorityFromDeclaredMap(AuthorityRequest{StdlibMapPath: path, InLayoutMode: false})
	if err == nil {
		t.Fatal("expected a format mismatch error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want it to name the artifact path", err)
	}
}

// TestAuthorityFromDeclaredMapLayoutTargetMismatch fails a map whose key differs
// from the layout's pinned platform (N3).
func TestAuthorityFromDeclaredMapLayoutTargetMismatch(t *testing.T) {
	target := stdlibmap.TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64"}
	path := writeMapArtifact(t, fixtureMap(t, target))
	platform := goanalysis.PlatformIdentity{
		ToolchainVersion: "go1.26.4",
		GOOS:             "darwin",
		GOARCH:           "arm64",
	}
	_, err := authorityFromDeclaredMap(AuthorityRequest{StdlibMapPath: path, InLayoutMode: true, LayoutPlatform: &platform})
	if err == nil {
		t.Fatal("expected a declared target mismatch error")
	}
	if !strings.Contains(err.Error(), "mismatched fields: goarch") {
		t.Errorf("error = %q, want it to name the mismatched target field", err)
	}
}

// TestAuthorityFromDeclaredMapLayoutTargetMatch accepts a map matching the pinned
// layout platform and returns its key.
func TestAuthorityFromDeclaredMapLayoutTargetMatch(t *testing.T) {
	target := stdlibmap.TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64"}
	path := writeMapArtifact(t, fixtureMap(t, target))
	platform := goanalysis.PlatformIdentity{
		ToolchainVersion: target.ToolchainVersion,
		GOOS:             target.GOOS,
		GOARCH:           target.GOARCH,
	}
	auth, err := authorityFromDeclaredMap(AuthorityRequest{StdlibMapPath: path, InLayoutMode: true, LayoutPlatform: &platform})
	if err != nil {
		t.Fatalf("authorityFromDeclaredMap: %v", err)
	}
	if key := auth.Key(); key.GOOS != target.GOOS || key.ToolchainVersion != target.ToolchainVersion {
		t.Errorf("key = %+v, want the pinned target", key)
	}
}

// TestSurvivingInterfaceFiles filters build-constraint exclusions out of the
// manifest's interface-file list (review round 1, finding 3).
func TestSurvivingInterfaceFiles(t *testing.T) {
	got := survivingInterfaceFiles(
		[]string{"api.go", "gated.go", "sub/other.go"},
		[]goanalysis.InterfaceFileExclusion{{File: "gated.go", Constraint: "//go:build plan9"}},
	)
	want := []string{"api.go", "sub/other.go"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("survivingInterfaceFiles = %v, want %v", got, want)
	}
	all := survivingInterfaceFiles([]string{"a.go", "b.go"}, []goanalysis.InterfaceFileExclusion{
		{File: "a.go", Constraint: "x"}, {File: "b.go", Constraint: "y"},
	})
	if len(all) != 0 {
		t.Errorf("all-excluded case returned %v, want empty", all)
	}
}
