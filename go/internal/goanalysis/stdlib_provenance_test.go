package goanalysis

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"golang.org/x/tools/go/packages"
)

func TestIsStdlibPackageUsesOrderedProvenance(t *testing.T) {
	originalPolicy := hostpolicy.IsStdlibPath
	t.Cleanup(func() { hostpolicy.IsStdlibPath = originalPolicy })
	hostpolicy.IsStdlibPath = func(path string) bool {
		return path == "fmt" || path == "std.example/stdlib"
	}

	tests := []struct {
		name string
		pkg  *packages.Package
		want bool
	}{
		{name: "SDK package with nil module", pkg: &packages.Package{PkgPath: "fmt"}, want: true},
		{name: "non-SDK nil module", pkg: &packages.Package{PkgPath: "rewritten.example/dep"}, want: false},
		{name: "standard module agrees with policy", pkg: &packages.Package{PkgPath: "std.example/stdlib", Module: &packages.Module{Path: "std"}}, want: true},
		{name: "standard module disagrees with policy", pkg: &packages.Package{PkgPath: "rewritten.example/stdlib", Module: &packages.Module{Path: "std"}}, want: false},
		{name: "ordinary module", pkg: &packages.Package{PkgPath: "example.com/dep", Module: &packages.Module{Path: "example.com/dep"}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStdlibPackage(tt.pkg); got != tt.want {
				t.Fatalf("isStdlibPackage(%q) = %v, want %v", tt.pkg.PkgPath, got, tt.want)
			}
		})
	}
}
