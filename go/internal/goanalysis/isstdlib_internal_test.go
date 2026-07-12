//go:build integration

package goanalysis

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestIsStdlibPackage(t *testing.T) {
	root, err := filepath.Abs("testdata/success")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedModule,
		Dir:  root,
	}

	pkgs, err := packages.Load(cfg, "fmt", "./a")
	if err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}

	var fmtPkg, aPkg *packages.Package
	for _, p := range pkgs {
		switch p.PkgPath {
		case "fmt":
			fmtPkg = p
		case "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a":
			aPkg = p
		}
	}
	if fmtPkg == nil {
		t.Fatalf("fmt package not found in load result")
	}
	if aPkg == nil {
		t.Fatalf("component package a not found in load result")
	}

	if !isStdlibPackage(fmtPkg) {
		t.Errorf("expected isStdlibPackage(fmt) to be true")
	}
	if isStdlibPackage(aPkg) {
		t.Errorf("expected isStdlibPackage(a) to be false")
	}
}
