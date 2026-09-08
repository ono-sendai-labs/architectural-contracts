//go:build integration

package goanalysis_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

func TestResolveDependencyInterface_Success(t *testing.T) {
	// 1. Resolve path to testdata/dep_resolve
	declaringRoot, err := filepath.Abs("testdata/dep_resolve/declaring")
	if err != nil {
		t.Fatalf("failed to get absolute path to declaring: %v", err)
	}
	analyzedRoot := declaringRoot // The analyzed component root is the declaring root in this scenario

	dep := manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/component.textproto",
	}

	result, err := goanalysis.ResolveDependencyInterface(declaringRoot, analyzedRoot, dep)
	if err != nil {
		t.Fatalf("unexpected error resolving dependency: %v", err)
	}

	if result.Component != "dep" {
		t.Errorf("expected component name %q, got %q", "dep", result.Component)
	}

	// 2. Assert Packages
	expectedPkgs := []string{
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep",
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep/subpkg",
	}
	if !reflect.DeepEqual(result.Packages, expectedPkgs) {
		t.Errorf("expected packages:\n%v\ngot:\n%v", expectedPkgs, result.Packages)
	}

	// 3. Assert Symbols — the exact declaring-object set of the surviving
	// interface files (types.go + api.go): no implements-closure injection,
	// no dual receiver keys, shared facts.SymbolID values.
	pkgPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep"
	expectedSymbols := []facts.SymbolID{
		facts.SymbolID("(" + pkgPath + ".Base).GetValue"),
		facts.SymbolID("(" + pkgPath + ".Box).Get"),
		facts.SymbolID(pkgPath + ".Base"),
		facts.SymbolID(pkgPath + ".Box"),
		facts.SymbolID(pkgPath + ".ExportedConst"),
		facts.SymbolID(pkgPath + ".ExportedVar"),
		facts.SymbolID(pkgPath + ".Greeter"),
		facts.SymbolID(pkgPath + ".Hello"),
		facts.SymbolID(pkgPath + ".init"),
	}

	if !reflect.DeepEqual(result.Symbols, expectedSymbols) {
		t.Errorf("expected symbols:\n%v\ngot:\n%v", expectedSymbols, result.Symbols)
	}

	// The implements closure is gone: the exported concrete implementation is
	// not part of the resolved set even though it satisfies Greeter.
	for _, forbidden := range []facts.SymbolID{
		facts.SymbolID("(" + pkgPath + ".GreeterImpl).Greet"),
		facts.SymbolID(pkgPath + ".PrivateFunc"),
	} {
		for _, sym := range result.Symbols {
			if sym == forbidden {
				t.Errorf("implements-closure symbol %q must not appear in the resolved set: %v", forbidden, result.Symbols)
			}
		}
	}
}

func TestResolveDependencyInterface_DeclaredStyleWithMembers(t *testing.T) {
	declaringRoot, err := filepath.Abs("testdata/dep_resolve/declaring")
	if err != nil {
		t.Fatalf("failed to get absolute path to declaring: %v", err)
	}

	dep := manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/declared_with_members.textproto",
	}

	result, err := goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, dep)
	if err != nil {
		t.Fatalf("unexpected error resolving declared-style dependency with members: %v", err)
	}

	if result.Component != "dep" {
		t.Errorf("expected component name %q, got %q", "dep", result.Component)
	}
	if result.InterfaceStyle != manifest.InterfaceStyleUnspecified {
		t.Errorf("expected InterfaceStyleUnspecified, got %v", result.InterfaceStyle)
	}
	if len(result.Symbols) == 0 {
		t.Errorf("expected non-empty symbols from declared interface files")
	}
}

func TestResolveDependencyInterface_Errors(t *testing.T) {
	declaringRoot, err := filepath.Abs("testdata/dep_resolve/declaring")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	tests := []struct {
		name         string
		analyzedRoot string
		dep          manifest.ComponentDependency
		wantErr      string
	}{
		{
			name:         "missing manifest",
			analyzedRoot: declaringRoot,
			dep: manifest.ComponentDependency{
				Name:     "dep",
				Manifest: "../nonexistent/component.textproto",
			},
			wantErr: "does not exist",
		},
		{
			name:         "mismatched component name",
			analyzedRoot: declaringRoot,
			dep: manifest.ComponentDependency{
				Name:     "wrong-name",
				Manifest: "../dep/component.textproto",
			},
			wantErr: "name mismatch",
		},
		{
			name:         "root overlap - identical",
			analyzedRoot: filepath.Clean(filepath.Join(declaringRoot, "../dep")),
			dep: manifest.ComponentDependency{
				Name:     "dep",
				Manifest: "../dep/component.textproto",
			},
			wantErr: "overlap",
		},
		{
			name:         "malformed manifest",
			analyzedRoot: declaringRoot,
			dep: manifest.ComponentDependency{
				Name:     "dep",
				Manifest: "../dep/malformed.textproto",
			},
			wantErr: "failed to parse dependency manifest",
		},
		{
			name:         "missing interface file",
			analyzedRoot: declaringRoot,
			dep: manifest.ComponentDependency{
				Name:     "dep",
				Manifest: "../dep/missing_interface.textproto",
			},
			wantErr: "does not exist",
		},
		{
			name:         "escaping interface file",
			analyzedRoot: declaringRoot,
			dep: manifest.ComponentDependency{
				Name:     "dep",
				Manifest: "../dep/escaping_interface.textproto",
			},
			wantErr: "escapes the component root",
		},
		{
			name:         "root overlap - analyzed nested in dependency",
			analyzedRoot: filepath.Join(filepath.Dir(declaringRoot), "dep", "subpkg"),
			dep: manifest.ComponentDependency{
				Name:     "dep",
				Manifest: "../dep/component.textproto",
			},
			wantErr: "overlap",
		},
		{
			name:         "root overlap - dependency nested in analyzed",
			analyzedRoot: filepath.Dir(declaringRoot),
			dep: manifest.ComponentDependency{
				Name:     "dep",
				Manifest: "../dep/component.textproto",
			},
			wantErr: "overlap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := goanalysis.ResolveDependencyInterface(declaringRoot, tt.analyzedRoot, tt.dep)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
			} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestResolveDependencyInterface_PackageSurface(t *testing.T) {
	declaringRoot, err := filepath.Abs("testdata/dep_resolve/declaring")
	if err != nil {
		t.Fatalf("failed to get absolute path to declaring: %v", err)
	}

	dep := manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/pkg_surface.textproto",
	}

	result, err := goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, dep)
	if err != nil {
		t.Fatalf("unexpected error resolving package-surface dependency: %v", err)
	}

	if result.Component != "dep" {
		t.Errorf("expected component name %q, got %q", "dep", result.Component)
	}
	if result.InterfaceStyle != manifest.InterfaceStylePackageSurface {
		t.Errorf("expected InterfaceStylePackageSurface, got %v", result.InterfaceStyle)
	}

	expectedPkgs := []string{
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep",
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep/subpkg",
	}
	if !reflect.DeepEqual(result.Packages, expectedPkgs) {
		t.Errorf("expected packages:\n%v\ngot:\n%v", expectedPkgs, result.Packages)
	}

	// Should contain exported declaring objects from both member packages,
	// keyed as single canonical SymbolIDs (no dual receiver keys).
	pkgPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep"
	subPkgPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep/subpkg"

	mustContain := []facts.SymbolID{
		facts.SymbolID(pkgPath + ".PrivateFunc"),
		facts.SymbolID(subPkgPath + ".Subhello"),
		facts.SymbolID("(" + pkgPath + ".Base).GetValue"),
		facts.SymbolID("(" + pkgPath + ".GreeterImpl).Greet"),
	}

	for _, expected := range mustContain {
		found := false
		for _, sym := range result.Symbols {
			if sym == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected symbol %q not found in package-surface symbols: %v", expected, result.Symbols)
		}
	}

	// No dual-receiver keys survive the SymbolID conversion.
	for _, sym := range result.Symbols {
		if strings.Contains(string(sym), "(*") {
			t.Errorf("pointer-marker symbol %q must not appear in the resolved set", sym)
		}
	}
}

func TestResolveDependencyInterface_PackageSurface_NoCertification(t *testing.T) {
	declaringRoot, err := filepath.Abs("testdata/dep_resolve/declaring")
	if err != nil {
		t.Fatalf("failed to get absolute path to declaring: %v", err)
	}

	dep := manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/pkg_surface_nocert.textproto",
	}

	_, err = goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, dep)
	if err != nil {
		t.Fatalf("unexpected error resolving package-surface dependency: %v", err)
	}

}
