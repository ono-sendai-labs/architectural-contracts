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
	if got := packageByPath(parsed, "example.com/other").Imports; got == nil || len(got) != 0 {
		t.Fatalf("parsed explicit empty Imports = %#v, want a non-nil empty map", got)
	}
}

func TestValidateAndResolveForMemberOnlyMergesStdlibExportMetadata(t *testing.T) {
	layout, workspace := stdlibExportLayoutFixture(t, false)

	if err := ValidateAndResolveForMemberOnly(layout, workspace); err != nil {
		t.Fatalf("ValidateAndResolveForMemberOnly() error = %v", err)
	}

	fmtPackage := packageByPath(layout, "fmt")
	if !layout.IsStdlibPackage(fmtPackage) {
		t.Fatal("fmt was not marked as stdlib from descriptor provenance")
	}
	if got, want := fmtPackage.ID, "fmt"; got != want {
		t.Errorf("fmt ID = %q, want %q", got, want)
	}
	if got, want := fmtPackage.ExportFile, filepath.Join(workspace, "bazel-out", "exports", "fmt.a"); got != want {
		t.Errorf("fmt ExportFile = %q, want %q", got, want)
	}
	if got, want := fmtPackage.CompiledGoFiles, fmtPackage.GoFiles; !reflect.DeepEqual(got, want) {
		t.Errorf("fmt CompiledGoFiles = %v, want the metadata GoFiles fallback %v", got, want)
	}
	if got, want := fmtPackage.Imports["io"].ID, "io"; got != want {
		t.Errorf("fmt io edge ID = %q, want %q", got, want)
	}
	if got := packageByPath(layout, "io").Imports; got == nil || len(got) != 0 {
		t.Errorf("io Imports = %#v, want an explicit empty map", got)
	}
	if got := packageByPath(layout, "unsafe").ExportFile; got != "" {
		t.Errorf("unsafe ExportFile = %q, want empty", got)
	}
	member := packageByPath(layout, "example.com/member")
	if len(member.GoFiles) != 1 || len(member.CompiledGoFiles) != 1 {
		t.Errorf("member source fields = GoFiles %v, CompiledGoFiles %v, want one source in each", member.GoFiles, member.CompiledGoFiles)
	}
	if member.ExportFile != "" {
		t.Errorf("member ExportFile = %q, want source-owned root without an export role", member.ExportFile)
	}
}

func TestValidateAndResolveForMemberOnlyRejectsMalformedStdlibExportMetadata(t *testing.T) {
	tests := []struct {
		name       string
		metadata   string
		configure  func(*Layout, string)
		wantErrors []string
	}{
		{
			name: "missing metadata",
			configure: func(layout *Layout, _ string) {
				layout.StdlibExportData.Metadata = "missing.stdlib.pkg.json"
			},
			wantErrors: []string{"standard-library export metadata", "does not exist"},
		},
		{
			name: "duplicate package path",
			metadata: `{"ID":"stdlib/fmt","Name":"fmt","PkgPath":"fmt","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/fmt.a","Imports":{},"Standard":true}
{"ID":"stdlib/fmt2","Name":"fmt","PkgPath":"fmt","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/fmt.a","Imports":{},"Standard":true}
`,
			wantErrors: []string{"duplicate standard-library package import path", "fmt"},
		},
		{
			name: "duplicate package ID",
			metadata: `{"ID":"stdlib/fmt","Name":"fmt","PkgPath":"fmt","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/fmt.a","Imports":{},"Standard":true}
{"ID":"stdlib/fmt","Name":"io","PkgPath":"io","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/io.a","Imports":{},"Standard":true}
`,
			wantErrors: []string{"duplicate standard-library package ID", "stdlib/fmt"},
		},
		{
			name: "missing export",
			metadata: `{"ID":"stdlib/fmt","Name":"fmt","PkgPath":"fmt","ExportFile":"","Imports":{},"Standard":true}
`,
			wantErrors: []string{"empty export file path", "fmt"},
		},
		{
			name: "incomplete graph",
			metadata: `{"ID":"stdlib/fmt","Name":"fmt","PkgPath":"fmt","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/fmt.a","Imports":{"io":"stdlib/missing"},"Standard":true}
`,
			wantErrors: []string{"standard-library package", "fmt", "stdlib/missing"},
		},
		{
			name: "unsafe export path",
			metadata: `{"ID":"stdlib/fmt","Name":"fmt","PkgPath":"fmt","ExportFile":"../fmt.a","Imports":{},"Standard":true}
`,
			wantErrors: []string{"unsafe export file path", "fmt"},
		},
		{
			name: "target mismatch",
			metadata: `{"ID":"stdlib/fmt","Name":"fmt","PkgPath":"fmt","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/fmt.a","Imports":{},"Standard":true}
`,
			configure: func(layout *Layout, _ string) {
				layout.StdlibExportData.Target.GOARCH = "arm64"
			},
			wantErrors: []string{"configuration mismatch", "arm64"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, workspace := stdlibExportLayoutFixture(t, true)
			if tt.metadata != "" {
				if err := os.WriteFile(filepath.Join(workspace, "stdlib.pkg.json"), []byte(tt.metadata), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.configure != nil {
				tt.configure(layout, workspace)
			}

			err := ValidateAndResolveForMemberOnly(layout, workspace)
			if err == nil {
				t.Fatal("ValidateAndResolveForMemberOnly() error = nil, want metadata error")
			}
			for _, want := range tt.wantErrors {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want substring %q", err, want)
				}
			}
		})
	}
}

func TestStdlibExportMetadataMergeIsDeterministic(t *testing.T) {
	first, workspace := stdlibExportLayoutFixture(t, false)
	firstMetadata, err := os.ReadFile(filepath.Join(workspace, "stdlib.pkg.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAndResolveForMemberOnly(first, workspace); err != nil {
		t.Fatalf("first ValidateAndResolveForMemberOnly() error = %v", err)
	}

	second := &Layout{
		GoSDKRoot: first.GoSDKRoot,
		Platform:  first.Platform,
		Roots:     append([]string(nil), first.Roots...),
		Packages: []*packages.Package{{
			ID: "member-id", Name: "member", PkgPath: "example.com/member",
			GoFiles: []string{"member.go"}, CompiledGoFiles: []string{"member.go"},
		}},
		StdlibExportData: &StdlibExportData{
			Metadata: "stdlib.pkg.json",
			Target:   first.StdlibExportData.Target,
		},
	}
	if err := os.WriteFile(filepath.Join(workspace, "stdlib.pkg.json"), reverseStdlibExportMetadata(firstMetadata), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAndResolveForMemberOnly(second, workspace); err != nil {
		t.Fatalf("second ValidateAndResolveForMemberOnly() error = %v", err)
	}

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("stdlib metadata merge is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestStdlibExportMetadataMapsExecrootArtifactsToRunfilesFrame(t *testing.T) {
	layout, workspace := stdlibExportLayoutFixture(t, false)
	runfilesDir := filepath.Join(workspace, "rules_go+", "stdlib_", "gocache")
	if err := os.MkdirAll(runfilesDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fmt.a", "io.a"} {
		if err := os.WriteFile(filepath.Join(runfilesDir, name), []byte(name+" export data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	layout.StdlibExportData.ExportRoots = []StdlibExportRoot{{
		RunfilesPath: "rules_go+/stdlib_/gocache",
		ExecPath:     "bazel-out/exports",
	}}

	if err := ValidateAndResolveForMemberOnly(layout, workspace); err != nil {
		t.Fatalf("ValidateAndResolveForMemberOnly() error = %v", err)
	}
	for _, importPath := range []string{"fmt", "io"} {
		pkg := packageByPath(layout, importPath)
		want := filepath.Join(workspace, "rules_go+", "stdlib_", "gocache", importPath+".a")
		if pkg.ExportFile != want {
			t.Errorf("%s ExportFile = %q, want runfiles path %q", importPath, pkg.ExportFile, want)
		}
	}
}

func TestStdlibExportMetadataRejectsConflictingExportRoots(t *testing.T) {
	layout, workspace := stdlibExportLayoutFixture(t, false)
	layout.StdlibExportData.ExportRoots = []StdlibExportRoot{
		{RunfilesPath: "rules_go+/stdlib_/gocache", ExecPath: "bazel-out/exports"},
		{RunfilesPath: "rules_go+/stdlib_/pkg", ExecPath: "bazel-out/exports"},
	}

	err := ValidateAndResolveForMemberOnly(layout, workspace)
	if err == nil || !strings.Contains(err.Error(), "conflicting standard-library export root") {
		t.Fatalf("ValidateAndResolveForMemberOnly() error = %v, want conflicting-root error", err)
	}
}

func stdlibExportLayoutFixture(t *testing.T, rewriteMetadata bool) (*Layout, string) {
	t.Helper()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "bazel-out", "exports"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"member.go":               []byte("package member\nimport \"fmt\"\n"),
		"bazel-out/exports/fmt.a": []byte("fmt export data"),
		"bazel-out/exports/io.a":  []byte("io export data"),
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	metadata := `{"ID":"stdlib/fmt","Name":"fmt","PkgPath":"fmt","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/fmt.a","Imports":{"io":"stdlib/io"},"Standard":true}
{"ID":"stdlib/io","Name":"io","PkgPath":"io","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/io.a","Imports":{},"Standard":true}
{"ID":"stdlib/unsafe","Name":"unsafe","PkgPath":"unsafe","ExportFile":"","Imports":{},"Standard":true}
`
	if rewriteMetadata {
		metadata = `{"ID":"stdlib/fmt","Name":"fmt","PkgPath":"fmt","ExportFile":"__BAZEL_EXECROOT__/bazel-out/exports/fmt.a","Imports":{},"Standard":true}
`
	}
	if err := os.WriteFile(filepath.Join(workspace, "stdlib.pkg.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}
	version := "go1.26.4"
	return &Layout{
		GoSDKRoot: filepath.Join(workspace, "sdk", "src"),
		Platform: &Platform{
			GOOS: "linux", GOARCH: "amd64", ToolchainVersion: &version,
		},
		Roots: []string{"member-id"},
		Packages: []*packages.Package{{
			ID: "member-id", Name: "member", PkgPath: "example.com/member",
			GoFiles: []string{"member.go"}, CompiledGoFiles: []string{"member.go"},
		}},
		StdlibExportData: &StdlibExportData{
			Metadata: "stdlib.pkg.json",
			Target: &StdlibExportTarget{
				ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64",
			},
		},
	}, workspace
}

func reverseStdlibExportMetadata(data []byte) []byte {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return []byte(strings.Join(lines, "\n") + "\n")
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
