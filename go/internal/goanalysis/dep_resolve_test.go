//go:build integration

package goanalysis_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
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

	// 3. Assert Symbols - should contain declared interface symbols and concrete implementation methods of interface type
	// Let's list expected interface symbols in sorted order.
	pkgPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep"
	expectedSymbols := []capanalyzer.InterfaceSymbol{
		capanalyzer.InterfaceSymbol("(*" + pkgPath + ".Base).GetValue"),
		capanalyzer.InterfaceSymbol("(*" + pkgPath + ".Box).Get"),
		capanalyzer.InterfaceSymbol("(*" + pkgPath + ".GreeterImpl).Greet"),
		capanalyzer.InterfaceSymbol("(" + pkgPath + ".GreeterImpl).Greet"),
		capanalyzer.InterfaceSymbol(pkgPath + ".Base"),
		capanalyzer.InterfaceSymbol(pkgPath + ".Box"),
		capanalyzer.InterfaceSymbol(pkgPath + ".ExportedConst"),
		capanalyzer.InterfaceSymbol(pkgPath + ".ExportedVar"),
		capanalyzer.InterfaceSymbol(pkgPath + ".Greeter"),
		capanalyzer.InterfaceSymbol(pkgPath + ".Hello"),
		capanalyzer.InterfaceSymbol(pkgPath + ".init"),
	}

	// Convert result symbols to a map for checking, or sort and compare
	if len(result.Symbols) != len(expectedSymbols) {
		t.Errorf("expected %d symbols, got %d:\n%+v", len(expectedSymbols), len(result.Symbols), result.Symbols)
	}

	for _, expected := range expectedSymbols {
		found := false
		for _, sym := range result.Symbols {
			if sym == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected symbol %q not found in result symbols", expected)
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
	if !result.OwnCheckRuns {
		t.Errorf("expected OwnCheckRuns true, got false")
	}
	if result.CertificationReference != "ref-456" {
		t.Errorf("expected CertificationReference %q, got %q", "ref-456", result.CertificationReference)
	}

	expectedPkgs := []string{
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep",
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep/subpkg",
	}
	if !reflect.DeepEqual(result.Packages, expectedPkgs) {
		t.Errorf("expected packages:\n%v\ngot:\n%v", expectedPkgs, result.Packages)
	}

	// Should contain exported symbols from both member packages, including PrivateFunc and Subhello
	pkgPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep"
	subPkgPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep/subpkg"

	mustContain := []capanalyzer.InterfaceSymbol{
		capanalyzer.InterfaceSymbol(pkgPath + ".PrivateFunc"),
		capanalyzer.InterfaceSymbol(subPkgPath + ".Subhello"),
		capanalyzer.InterfaceSymbol("(*" + pkgPath + ".Base).GetValue"),
		capanalyzer.InterfaceSymbol("(" + pkgPath + ".Base).GetValue"),
		capanalyzer.InterfaceSymbol("(*" + pkgPath + ".GreeterImpl).Greet"),
		capanalyzer.InterfaceSymbol("(" + pkgPath + ".GreeterImpl).Greet"),
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
}

func TestResolveDependencyInterface_PackageSurface_NoCert(t *testing.T) {
	declaringRoot, err := filepath.Abs("testdata/dep_resolve/declaring")
	if err != nil {
		t.Fatalf("failed to get absolute path to declaring: %v", err)
	}

	dep := manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/pkg_surface_nocert.textproto",
	}

	result, err := goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, dep)
	if err != nil {
		t.Fatalf("unexpected error resolving package-surface dependency: %v", err)
	}

	if result.OwnCheckRuns {
		t.Errorf("expected OwnCheckRuns false, got true")
	}
	if result.CertificationReference != "" {
		t.Errorf("expected empty CertificationReference, got %q", result.CertificationReference)
	}
}

func TestResolveDependencyInterface_PatternMembership(t *testing.T) {
	declaringRoot, err := filepath.Abs("testdata/dep_resolve/declaring_pattern")
	if err != nil {
		t.Fatalf("failed to get absolute path to declaring_pattern: %v", err)
	}

	dep := manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/pattern_surface.textproto",
	}

	result, err := goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, dep)
	if err != nil {
		t.Fatalf("unexpected error resolving pattern-membership dependency: %v", err)
	}

	if result.InterfaceStyle != manifest.InterfaceStylePackageSurface {
		t.Errorf("expected InterfaceStylePackageSurface, got %v", result.InterfaceStyle)
	}
	if result.OwnCheckRuns {
		t.Errorf("expected OwnCheckRuns false, got true")
	}
	if result.CertificationReference != "scheduled-job" {
		t.Errorf("expected CertificationReference %q, got %q", "scheduled-job", result.CertificationReference)
	}

	if len(result.Packages) == 0 {
		t.Fatalf("expected non-empty packages for pattern-membership dependency")
	}

	pkgPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/dep_resolve/dep"
	foundPrivateFunc := false
	for _, sym := range result.Symbols {
		if sym == capanalyzer.InterfaceSymbol(pkgPath+".PrivateFunc") {
			foundPrivateFunc = true
			break
		}
	}
	if !foundPrivateFunc {
		t.Errorf("expected %q in pattern-membership symbols", pkgPath+".PrivateFunc")
	}
}

func TestResolveDependencyInterface_PatternNoMatchFailsClosed(t *testing.T) {
	declaringRoot, err := filepath.Abs("testdata/dep_resolve/declaring")
	if err != nil {
		t.Fatalf("failed to get absolute path to declaring: %v", err)
	}

	dep := manifest.ComponentDependency{
		Name:     "dep",
		Manifest: "../dep/pattern_nomatch.textproto",
	}

	_, err = goanalysis.ResolveDependencyInterface(declaringRoot, declaringRoot, dep)
	if err == nil {
		t.Fatalf("expected error for pattern matching nothing, got nil")
	}
	if !strings.Contains(err.Error(), "dep") {
		t.Errorf("expected error message naming dependency %q, got: %v", "dep", err)
	}
}
