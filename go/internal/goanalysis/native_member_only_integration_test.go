//go:build integration

package goanalysis

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestNativeMemberOnlyLoadUsesExportDataForDependency(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/native\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "member"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "dep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "member", "member.go"), []byte("package member\n\nimport \"example.com/native/dep\"\n\nfunc Use() string { return dep.Name }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "dep", "dep.go"), []byte("package dep\n\nconst Name = \"dep\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := packages.Load(&packages.Config{Mode: componentLoadMode, Dir: workspace}, "example.com/native/member")
	if err != nil {
		t.Fatalf("native packages.Load: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("native roots = %d, want 1", len(pkgs))
	}
	var member, dependency *packages.Package
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		switch pkg.PkgPath {
		case "example.com/native/member":
			member = pkg
		case "example.com/native/dep":
			dependency = pkg
		}
	})
	if member == nil || dependency == nil {
		t.Fatalf("native graph missing member or dependency: member=%v dependency=%v", member != nil, dependency != nil)
	}
	if member.Syntax == nil || member.TypesInfo == nil {
		t.Fatalf("native member lacks syntax/type info: syntax=%v info=%v", member.Syntax != nil, member.TypesInfo != nil)
	}
	if dependency.Syntax != nil || dependency.TypesInfo != nil {
		t.Fatalf("native dependency was source-loaded: syntax=%v info=%v", dependency.Syntax != nil, dependency.TypesInfo != nil)
	}
	if dependency.Types == nil || !dependency.Types.Complete() {
		t.Fatalf("native dependency types are incomplete: %#v", dependency.Types)
	}
}
