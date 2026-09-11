package packagelayout

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestValidateAndResolveForMemberOnlyAcceptsDeepExportClosure(t *testing.T) {
	layout, workspace := exportLayoutFixture(t)

	if err := ValidateAndResolveForMemberOnly(layout, workspace); err != nil {
		t.Fatalf("ValidateAndResolveForMemberOnly() error = %v", err)
	}

	for _, want := range []struct {
		path string
		file string
	}{
		{path: "example.com/dep", file: "exports/dep.a"},
		{path: "example.com/leaf", file: "exports/leaf.a"},
	} {
		pkg := packageByPath(layout, want.path)
		if got, want := pkg.ExportFile, filepath.Join(workspace, want.file); got != want {
			t.Errorf("%s ExportFile = %q, want %q", pkg.PkgPath, got, want)
		}
	}
	if pkg := packageByPath(layout, "unsafe"); pkg.ExportFile != "" {
		t.Errorf("unsafe ExportFile = %q, want empty", pkg.ExportFile)
	}
	if got := packageByPath(layout, "example.com/dep").Imports["example.com/leaf"].ID; got != "leaf-id" {
		t.Errorf("dep leaf edge ID = %q, want leaf-id", got)
	}
	if got := packageByPath(layout, "example.com/leaf").Imports; got == nil || len(got) != 0 {
		t.Errorf("leaf Imports = %#v, want an explicit empty map", got)
	}
}

func TestValidateAndResolveForMemberOnlyRecoversOmittedRootImports(t *testing.T) {
	layout, workspace := exportLayoutFixture(t)
	root := packageByPath(layout, "example.com/member")
	root.Imports = nil

	if err := ValidateAndResolveForMemberOnly(layout, workspace); err != nil {
		t.Fatalf("ValidateAndResolveForMemberOnly() error = %v", err)
	}
	if got := root.Imports["example.com/dep"].ID; got != "dep-id" {
		t.Errorf("recovered dependency ID = %q, want dep-id", got)
	}
	if got := root.Imports["unsafe"].ID; got != "unsafe-id" {
		t.Errorf("recovered unsafe ID = %q, want unsafe-id", got)
	}
}

func TestValidateAndResolveForMemberOnlyRejectsIncompleteGraph(t *testing.T) {
	layout, workspace := exportLayoutFixture(t)
	root := packageByPath(layout, "example.com/member")
	root.Imports = map[string]*packages.Package{
		"example.com/dep":     {ID: "dep-id"},
		"unsafe":              {ID: "unsafe-id"},
		"example.com/missing": {ID: "missing-id"},
	}

	err := ValidateAndResolveForMemberOnly(layout, workspace)
	if err == nil {
		t.Fatal("ValidateAndResolveForMemberOnly() error = nil, want missing graph node error")
	}
	for _, want := range []string{"example.com/member", "example.com/missing", "missing-id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %q", err, want)
		}
	}
}

func TestValidateAndResolveForMemberOnlyRejectsInvalidExportArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Layout, string)
		want      string
	}{
		{
			name: "empty path",
			configure: func(layout *Layout, _ string) {
				packageByPath(layout, "example.com/dep").ExportFile = ""
			},
			want: "has empty export file path",
		},
		{
			name: "missing file",
			configure: func(layout *Layout, _ string) {
				packageByPath(layout, "example.com/dep").ExportFile = "exports/missing.a"
			},
			want: "does not exist",
		},
		{
			name: "empty file",
			configure: func(layout *Layout, workspace string) {
				path := filepath.Join(workspace, "exports", "empty.a")
				if err := os.WriteFile(path, nil, 0644); err != nil {
					panic(err)
				}
				packageByPath(layout, "example.com/dep").ExportFile = "exports/empty.a"
			},
			want: "is empty",
		},
		{
			name: "directory",
			configure: func(layout *Layout, workspace string) {
				packageByPath(layout, "example.com/dep").ExportFile = "exports"
				_ = workspace
			},
			want: "is a directory",
		},
		{
			name: "absolute path",
			configure: func(layout *Layout, workspace string) {
				packageByPath(layout, "example.com/dep").ExportFile = filepath.Join(workspace, "exports", "dep.a")
			},
			want: "unsafe export file path",
		},
		{
			name: "parent escape",
			configure: func(layout *Layout, _ string) {
				packageByPath(layout, "example.com/dep").ExportFile = "../dep.a"
			},
			want: "unsafe export file path",
		},
		{
			name: "non-normalized",
			configure: func(layout *Layout, _ string) {
				packageByPath(layout, "example.com/dep").ExportFile = "exports/../exports/dep.a"
			},
			want: "non-normalized export file path",
		},
		{
			name: "unreadable",
			configure: func(layout *Layout, _ string) {
				packageByPath(layout, "example.com/dep").ExportFile = "exports/dep.a"
			},
			want: "is unreadable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, workspace := exportLayoutFixture(t)
			tt.configure(layout, workspace)
			if tt.name == "unreadable" {
				oldOpen := osOpen
				osOpen = func(string) (*os.File, error) {
					return nil, errors.New("permission denied")
				}
				defer func() { osOpen = oldOpen }()
			}

			err := ValidateAndResolveForMemberOnly(layout, workspace)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateAndResolveForMemberOnly() error = %v, want substring %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "example.com/dep") {
				t.Errorf("error = %q, want package name", err)
			}
		})
	}
}

func TestValidateAndResolveForMemberOnlyRequiresExplicitExportImports(t *testing.T) {
	layout, workspace := exportLayoutFixture(t)
	packageByPath(layout, "example.com/leaf").Imports = nil

	err := ValidateAndResolveForMemberOnly(layout, workspace)
	if err == nil || !strings.Contains(err.Error(), "omitted Imports map") {
		t.Fatalf("ValidateAndResolveForMemberOnly() error = %v, want omitted Imports error", err)
	}
}

func TestValidateAndResolveSourceBackedDoesNotRequireExportFiles(t *testing.T) {
	workspace := t.TempDir()
	for name, source := range map[string]string{
		"root.go": "package root\nimport \"example.com/dep\"\n",
		"dep.go":  "package dep\n",
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	layout := &Layout{
		Roots: []string{"root-id"},
		Packages: []*packages.Package{
			{
				ID: "root-id", Name: "root", PkgPath: "example.com/root",
				GoFiles: []string{"root.go"},
				Imports: map[string]*packages.Package{"example.com/dep": {ID: "dep-id"}},
			},
			{
				ID: "dep-id", Name: "dep", PkgPath: "example.com/dep",
				GoFiles: []string{"dep.go"},
			},
		},
	}
	if err := ValidateAndResolve(layout, workspace); err != nil {
		t.Fatalf("ValidateAndResolve() error = %v", err)
	}
}

func TestExportDataLayoutJSONIsDeterministicAndPreservesBindings(t *testing.T) {
	first := &Layout{
		Roots: []string{"example.com/member", "example.com/other"},
		Packages: []*packages.Package{
			{
				ID: "other-id", Name: "other", PkgPath: "example.com/other",
				ExportFile: "exports/other.a", Imports: map[string]*packages.Package{},
			},
			{
				ID: "member-id", Name: "member", PkgPath: "example.com/member",
				ExportFile: "", Imports: map[string]*packages.Package{
					"example.com/other": {ID: "other-id"},
				},
			},
		},
	}
	second := &Layout{
		Roots: []string{"example.com/other", "example.com/member"},
		Packages: []*packages.Package{
			{
				ID: "member-id", Name: "member", PkgPath: "example.com/member",
				ExportFile: "", Imports: map[string]*packages.Package{
					"example.com/other": {ID: "other-id"},
				},
			},
			{
				ID: "other-id", Name: "other", PkgPath: "example.com/other",
				ExportFile: "exports/other.a", Imports: map[string]*packages.Package{},
			},
		},
	}
	originalRoots := append([]string(nil), first.Roots...)
	originalPackageIDs := []string{first.Packages[0].ID, first.Packages[1].ID}

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical layout JSON differs:\n%s\n%s", firstJSON, secondJSON)
	}
	if !reflect.DeepEqual(first.Roots, originalRoots) || first.Packages[0].ID != originalPackageIDs[0] || first.Packages[1].ID != originalPackageIDs[1] {
		t.Fatalf("MarshalJSON mutated caller-owned collections: roots=%v packages=%v", first.Roots, originalPackageIDs)
	}
	if !strings.Contains(string(firstJSON), `"ExportFile":"exports/other.a"`) {
		t.Fatalf("layout JSON = %s, want ExportFile binding", firstJSON)
	}

	parsed, err := Parse(bytes.NewReader(firstJSON))
	if err != nil {
		t.Fatal(err)
	}
	if got := packageByPath(parsed, "example.com/other").ExportFile; got != "exports/other.a" {
		t.Fatalf("parsed export binding = %q, want exports/other.a", got)
	}
}

func exportLayoutFixture(t *testing.T) (*Layout, string) {
	t.Helper()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "exports"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"root.go":        []byte("package member\nimport (\n _ \"example.com/dep\"\n _ \"unsafe\"\n)\n"),
		"exports/dep.a":  []byte("dep export data"),
		"exports/leaf.a": []byte("leaf export data"),
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	return &Layout{
		// The SDK root is deliberately unrelated to the export frame. The
		// member-only resolver must not look for exports below it.
		GoSDKRoot: filepath.Join(workspace, "sdk", "src"),
		Roots:     []string{"member-id"},
		Packages: []*packages.Package{
			{
				ID: "leaf-id", Name: "leaf", PkgPath: "example.com/leaf",
				GoFiles: []string{"missing/leaf.go"}, ExportFile: "exports/leaf.a",
				Imports: map[string]*packages.Package{},
			},
			{
				ID: "member-id", Name: "member", PkgPath: "example.com/member",
				GoFiles: []string{"root.go"}, CompiledGoFiles: []string{"root.go"},
				Imports: map[string]*packages.Package{
					"example.com/dep": {ID: "dep-id"},
					"unsafe":          {ID: "unsafe-id"},
				},
			},
			{
				ID: "unsafe-id", Name: "unsafe", PkgPath: "unsafe", Imports: nil,
			},
			{
				ID: "dep-id", Name: "dep", PkgPath: "example.com/dep",
				GoFiles: []string{"missing/dep.go"}, ExportFile: "exports/dep.a",
				Imports: map[string]*packages.Package{"example.com/leaf": {ID: "leaf-id"}},
			},
		},
	}, workspace
}

func packageByPath(layout *Layout, importPath string) *packages.Package {
	for _, pkg := range layout.Packages {
		if pkg != nil && pkg.PkgPath == importPath {
			return pkg
		}
	}
	panic("package not found: " + importPath)
}
