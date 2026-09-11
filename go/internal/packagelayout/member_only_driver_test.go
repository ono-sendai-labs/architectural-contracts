package packagelayout

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestHandleDriverRequest_ProjectsMemberOnlyPackageRoles(t *testing.T) {
	root := &packages.Package{
		ID:              "example.com/member",
		Name:            "member",
		PkgPath:         "example.com/member",
		GoFiles:         []string{"member/member.go"},
		CompiledGoFiles: []string{"member/member.go"},
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
	if !reflect.DeepEqual(responseRoot.GoFiles, root.GoFiles) || !reflect.DeepEqual(responseRoot.CompiledGoFiles, root.CompiledGoFiles) {
		t.Fatalf("member source fields = %v/%v, want %v/%v", responseRoot.GoFiles, responseRoot.CompiledGoFiles, root.GoFiles, root.CompiledGoFiles)
	}
	if len(responseDependency.GoFiles) != 0 || len(responseDependency.CompiledGoFiles) != 0 {
		t.Fatalf("dependency source fields = %v/%v, want both empty", responseDependency.GoFiles, responseDependency.CompiledGoFiles)
	}
	if responseDependency.ExportFile != dependency.ExportFile {
		t.Fatalf("dependency export file = %q, want %q", responseDependency.ExportFile, dependency.ExportFile)
	}
	if got := responseRoot.Imports["example.com/dep"]; got == nil || got.ID != dependency.ID {
		t.Fatalf("member import graph = %+v, want dep edge", responseRoot.Imports)
	}

	responseRoot.GoFiles[0] = "mutated.go"
	delete(responseRoot.Imports, "example.com/dep")
	responseDependency.ExportFile = "mutated.a"
	if !reflect.DeepEqual(root.GoFiles, []string{"member/member.go"}) || root.Imports["example.com/dep"] == nil {
		t.Fatalf("mutating response root changed the validated layout: root=%+v", root)
	}
	if dependency.ExportFile != "exports/dep.a" {
		t.Fatalf("mutating response dependency changed the validated layout: export=%q", dependency.ExportFile)
	}
}

func TestRunDriver_ValidatesExportLayoutWithoutReadingNonMemberSources(t *testing.T) {
	workspace := t.TempDir()
	rootSource := filepath.Join(workspace, "member.go")
	if err := os.WriteFile(rootSource, []byte("package member\n\nimport _ \"example.com/dep\"\n"), 0o644); err != nil {
		t.Fatalf("writing member source: %v", err)
	}
	for _, name := range []string{"dep.a", "leaf.a"} {
		path := filepath.Join(workspace, "exports", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating export directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("export data"), 0o644); err != nil {
			t.Fatalf("writing export artifact: %v", err)
		}
	}
	layout := &Layout{
		Roots: []string{"member-id"},
		Packages: []*packages.Package{
			{ID: "member-id", Name: "member", PkgPath: "example.com/member", GoFiles: []string{"member.go"}, CompiledGoFiles: []string{"member.go"}, Imports: map[string]*packages.Package{"example.com/dep": {ID: "dep-id"}}},
			{ID: "dep-id", Name: "dep", PkgPath: "example.com/dep", GoFiles: []string{"missing/dep.go"}, ExportFile: "exports/dep.a", Imports: map[string]*packages.Package{"example.com/leaf": {ID: "leaf-id"}}},
			{ID: "leaf-id", Name: "leaf", PkgPath: "example.com/leaf", GoFiles: []string{"missing/leaf.go"}, ExportFile: "exports/leaf.a", Imports: map[string]*packages.Package{}},
		},
	}
	data, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("marshalling layout: %v", err)
	}
	layoutPath := filepath.Join(workspace, "component.package-layout.json")
	if err := os.WriteFile(layoutPath, data, 0o644); err != nil {
		t.Fatalf("writing layout: %v", err)
	}

	var output bytes.Buffer
	err = RunDriver(layoutPath, workspace, []string{"example.com/member"}, strings.NewReader(`{"Mode":0}`), &output)
	if err != nil {
		t.Fatalf("RunDriver() error = %v", err)
	}
	var response packages.DriverResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decoding driver response: %v", err)
	}
	for _, pkg := range response.Packages {
		if pkg.PkgPath == "example.com/dep" || pkg.PkgPath == "example.com/leaf" {
			if len(pkg.GoFiles) != 0 || len(pkg.CompiledGoFiles) != 0 {
				t.Errorf("non-member %q source fields = %v/%v, want empty", pkg.PkgPath, pkg.GoFiles, pkg.CompiledGoFiles)
			}
		}
	}
}
