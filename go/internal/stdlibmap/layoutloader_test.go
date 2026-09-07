package stdlibmap

import (
	"testing"
)

// TestLayoutLoaderLoadsThroughDriver is the driver-loading leg (task req 5):
// the layout loader serves a fixture package through the GOPACKAGESDRIVER
// self-exec driver with an empty PATH and no GOROOT, GOPACKAGESDRIVER or
// GOCACHE in the environment — proof that no toolchain binary is executed and
// go/packages never falls back to the go list driver.
func TestLayoutLoaderLoadsThroughDriver(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("GOROOT", "")
	t.Setenv("GOCACHE", "")
	t.Setenv("GOPACKAGESDRIVER", "")

	sdkRoot := fixtureSDK(t)
	layout := fixtureSDKLayout(t, sdkRoot)
	loader := &LayoutLoader{Layout: layout}
	loaded, err := loader.Load([]string{"fmt"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	pkg, ok := loaded["fmt"]
	if !ok || pkg == nil {
		t.Fatalf("Load() returned no types package for fmt: %v", loaded)
	}
	if pkg.Path() != "fmt" {
		t.Fatalf("loaded package path = %q, want fmt", pkg.Path())
	}
	if pkg.Scope().Lookup("Println") == nil {
		t.Fatal("the loaded fmt package does not declare Println")
	}
}

// TestGenerateViaLayoutFailsWithoutNativeDriver goes one step further than the
// loader test: the whole generation (inventory load and Capslock load) runs
// inside the driver environment.
func TestGenerateViaLayoutRunsUnderDriver(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("GOROOT", "")
	t.Setenv("GOCACHE", "")
	t.Setenv("GOPACKAGESDRIVER", "")

	sdkRoot := fixtureSDK(t)
	layout := fixtureSDKLayout(t, sdkRoot)
	entries, err := ReconcilePackageList([]string{"fmt", "strings", "unsafe", "internal/testcap"}, layout, sdkRoot)
	if err != nil {
		t.Fatalf("ReconcilePackageList() error = %v", err)
	}
	out, err := GenerateViaLayout(GenerationInput{
		Target:      TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64"},
		RuleVersion: RuleVersion,
		Oracle:      PackageOracleFunc(func() ([]PackageEntry, error) { return entries, nil }),
	}, layout)
	if err != nil {
		t.Fatalf("GenerateViaLayout() error = %v", err)
	}
	if out.Map.Key == nil || out.Map.Key.ToolchainVersion != "go1.26.4" {
		t.Fatalf("generated key does not match the config: %+v", out.Map.Key)
	}
	// Every importable package's symbols have terminal classifications: the
	// fmt package is inventoried and fmt.Println is SAFE.
	var found bool
	for _, s := range out.Map.Symbols {
		if s.Id == "fmt.Println" {
			found = true
			if s.Classification.String() != "SAFE" {
				t.Fatalf("fmt.Println classified %s, want SAFE", s.Classification)
			}
		}
	}
	if !found {
		t.Fatal("the map has no record for fmt.Println")
	}
}
