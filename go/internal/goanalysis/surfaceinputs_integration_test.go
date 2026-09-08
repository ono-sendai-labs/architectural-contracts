//go:build integration

package goanalysis_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
)

// TestLoadSurfaceInputs exercises the surface-emission load leg (plan Step 5
// task 3) over the dep_resolve fixture: the declared interface files' ASTs
// come from one loaded package with type information, member import paths are
// canonical and sorted, and source paths are slash paths relative to the
// component root.
func TestLoadSurfaceInputs(t *testing.T) {
	root, err := filepath.Abs("testdata/dep_resolve/dep")
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := goanalysis.LoadSurfaceInputs(goanalysis.LoadRequest{
		ComponentName:  "dep",
		ComponentRoot:  root,
		InterfaceFiles: []string{"types.go", "api.go"},
	})
	if err != nil {
		t.Fatalf("LoadSurfaceInputs: %v", err)
	}

	wantPkgs := []string{
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep",
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep/subpkg",
	}
	if !reflect.DeepEqual(inputs.MemberPackages, wantPkgs) {
		t.Errorf("member packages = %v, want %v", inputs.MemberPackages, wantPkgs)
	}
	if len(inputs.InterfaceFiles) != 2 {
		t.Errorf("interface ASTs = %d, want 2", len(inputs.InterfaceFiles))
	}
	if inputs.InterfaceInfo == nil {
		t.Error("interface type info is nil")
	}
	if len(inputs.SourcePaths) == 0 {
		t.Error("no member source paths collected")
	}
	for _, p := range inputs.SourcePaths {
		if strings.Contains(p, "\\") || filepath.IsAbs(p) {
			t.Errorf("source path %q is not a relative slash path", p)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil {
			t.Errorf("source path %q does not resolve under the component root: %v", p, err)
		}
	}
}

// TestLoadSurfaceInputs_MultiPackageInterfacesFailsClosed verifies that
// declared interface files spanning two loaded packages are an actionable
// error, never an ambiguous surface.
func TestLoadSurfaceInputs_MultiPackageInterfacesFailsClosed(t *testing.T) {
	root, err := filepath.Abs("testdata/dep_resolve/declaring")
	if err != nil {
		t.Fatal(err)
	}
	// Write a manifest-less scenario: interface files in two different
	// packages under one root.
	aDir := filepath.Join(root, "surface_a")
	bDir := filepath.Join(root, "surface_b")
	for _, d := range []string{aDir, bDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(aDir, "a.go"), []byte("package surface_a\n\ntype Alpha interface{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bDir, "b.go"), []byte("package surface_b\n\ntype Beta interface{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = goanalysis.LoadSurfaceInputs(goanalysis.LoadRequest{
		ComponentName:  "split",
		ComponentRoot:  root,
		InterfaceFiles: []string{"surface_a/a.go", "surface_b/b.go"},
	})
	if err == nil {
		t.Fatal("expected an error for interface files spanning packages")
	}
	if !strings.Contains(err.Error(), "span multiple packages") {
		t.Errorf("error %q does not name the span problem", err)
	}
}
