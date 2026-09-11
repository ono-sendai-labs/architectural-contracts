package packagelayout

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestValidateAndResolveForMemberOnlyPreservesMemberAnalysisFileRoles(t *testing.T) {
	workspace := t.TempDir()
	write := func(name, source string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(source), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("selected.go", "package member\n")
	write("ignored.go", "//go:build windows\n\npackage member\n\n//go:linkname hidden runtime.nanotime1\n")
	write("stub.s", "TEXT ·stub(SB),0,$0-0\n\tRET\n")

	layout := &Layout{
		Platform: &Platform{GOOS: "linux", GOARCH: "amd64", CgoEnabled: false},
		Roots:    []string{"example.com/member"},
		Packages: []*packages.Package{{
			ID:              "example.com/member",
			Name:            "member",
			PkgPath:         "example.com/member",
			GoFiles:         []string{"selected.go", "ignored.go"},
			CompiledGoFiles: []string{"selected.go", "ignored.go"},
			OtherFiles:      []string{"stub.s"},
			Imports:         map[string]*packages.Package{},
		}},
	}

	if err := ValidateAndResolveForMemberOnly(layout, workspace); err != nil {
		t.Fatalf("ValidateAndResolveForMemberOnly() error = %v", err)
	}

	pkg := layout.Packages[0]
	if got, want := pkg.GoFiles, []string{filepath.Join(workspace, "selected.go")}; !reflect.DeepEqual(got, want) {
		t.Errorf("GoFiles = %v, want %v", got, want)
	}
	if got, want := pkg.CompiledGoFiles, []string{filepath.Join(workspace, "selected.go")}; !reflect.DeepEqual(got, want) {
		t.Errorf("CompiledGoFiles = %v, want %v", got, want)
	}
	if got, want := pkg.IgnoredFiles, []string{filepath.Join(workspace, "ignored.go")}; !reflect.DeepEqual(got, want) {
		t.Errorf("IgnoredFiles = %v, want %v", got, want)
	}
	if got, want := pkg.OtherFiles, []string{filepath.Join(workspace, "stub.s")}; !reflect.DeepEqual(got, want) {
		t.Errorf("OtherFiles = %v, want %v", got, want)
	}
}

func TestHandleDriverRequestProjectsEveryMemberAnalysisFileRole(t *testing.T) {
	root := &packages.Package{
		ID:              "example.com/member",
		Name:            "member",
		PkgPath:         "example.com/member",
		GoFiles:         []string{"member/selected.go"},
		CompiledGoFiles: []string{"member/selected.go"},
		IgnoredFiles:    []string{"member/ignored.go"},
		OtherFiles:      []string{"member/stub.s"},
		Imports: map[string]*packages.Package{
			"example.com/dep": {ID: "example.com/dep"},
		},
	}
	dependency := &packages.Package{
		ID:              "example.com/dep",
		Name:            "dep",
		PkgPath:         "example.com/dep",
		GoFiles:         []string{"dep/dep.go"},
		CompiledGoFiles: []string{"dep/dep.go"},
		IgnoredFiles:    []string{"dep/ignored.go"},
		OtherFiles:      []string{"dep/dep.s"},
		ExportFile:      "exports/dep.a",
		Imports:         map[string]*packages.Package{},
	}
	layout := &Layout{
		Roots:    []string{"example.com/member"},
		Packages: []*packages.Package{root, dependency},
	}

	response, err := HandleDriverRequest(layout, &packages.DriverRequest{}, []string{"example.com/member"})
	if err != nil {
		t.Fatalf("HandleDriverRequest() error = %v", err)
	}
	byPath := make(map[string]*packages.Package, len(response.Packages))
	for _, pkg := range response.Packages {
		byPath[pkg.PkgPath] = pkg
	}
	responseRoot := byPath[root.PkgPath]
	responseDependency := byPath[dependency.PkgPath]
	if responseRoot == nil || responseDependency == nil {
		t.Fatalf("response packages = %+v, want both member and dependency", response.Packages)
	}
	if got, want := responseRoot.IgnoredFiles, root.IgnoredFiles; !reflect.DeepEqual(got, want) {
		t.Errorf("member IgnoredFiles = %v, want %v", got, want)
	}
	if got, want := responseRoot.OtherFiles, root.OtherFiles; !reflect.DeepEqual(got, want) {
		t.Errorf("member OtherFiles = %v, want %v", got, want)
	}
	if len(responseDependency.GoFiles) != 0 || len(responseDependency.CompiledGoFiles) != 0 ||
		len(responseDependency.IgnoredFiles) != 0 || len(responseDependency.OtherFiles) != 0 {
		t.Fatalf("dependency source roles = Go:%v Compiled:%v Ignored:%v Other:%v, want all empty",
			responseDependency.GoFiles,
			responseDependency.CompiledGoFiles,
			responseDependency.IgnoredFiles,
			responseDependency.OtherFiles,
		)
	}
}
