//go:build integration

package goanalysis

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"golang.org/x/tools/go/packages"
)

func TestRewritingHostNilModuleDependencyReachesChecker(t *testing.T) {
	root, err := filepath.Abs("testdata/success")
	if err != nil {
		t.Fatalf("failed to get absolute test fixture path: %v", err)
	}

	originalLoad := loadPackages
	t.Cleanup(func() { loadPackages = originalLoad })
	loadPackages = func(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
		pkgs, err := originalLoad(cfg, patterns...)
		if err != nil {
			return nil, err
		}
		packages.Visit(pkgs, nil, func(pkg *packages.Package) {
			if pkg.PkgPath == "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts" {
				// A rewriting/driver host can retain the real package graph while
				// omitting module provenance for a rewritten dependency.
				pkg.Module = nil
			}
		})
		return pkgs, nil
	}

	loaded, err := LoadPackageFacts(LoadRequest{ComponentRoot: root})
	if err != nil {
		t.Fatalf("LoadPackageFacts() error = %v", err)
	}

	const dependency = "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	var foundImport bool
	for _, pkg := range loaded.Packages {
		for _, imp := range pkg.Imports {
			if imp == dependency {
				foundImport = true
			}
		}
	}
	if !foundImport {
		t.Fatalf("loaded facts do not retain rewritten dependency %q: %+v", dependency, loaded.Packages)
	}
	for _, imp := range loaded.StdlibImports {
		if imp == dependency {
			t.Fatalf("rewritten dependency %q was incorrectly classified as stdlib", dependency)
		}
	}

	reportResult := checker.Check(checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "rewriting-host",
			InterfaceFiles: []string{"a/a.go", "a/api.go", "a/types.go", "a/init.go"},
		},
		Facts: loaded,
	})
	var foundViolation bool
	for _, violation := range reportResult.Violations {
		if violation.Kind == report.UndeclaredDependency && strings.Contains(violation.Message, dependency) {
			foundViolation = true
		}
	}
	if !foundViolation {
		t.Fatalf("checker report = %+v, want UNDECLARED_DEPENDENCY for %q", reportResult, dependency)
	}
}
