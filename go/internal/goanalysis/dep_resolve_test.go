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
