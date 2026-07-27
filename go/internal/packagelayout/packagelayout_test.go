package packagelayout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"
)

type mockFileInfo struct {
	name  string
	isDir bool
}

func (m mockFileInfo) Name() string { return m.name }
func (m mockFileInfo) Size() int64  { return 0 }
func (m mockFileInfo) Mode() os.FileMode {
	if m.isDir {
		return os.ModeDir
	}
	return 0
}
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() any           { return nil }

// setupMockStat configures a mock file system where the specified paths and their parents exist.
func setupMockStat(existingPaths []string) func() {
	orig := osStat
	existing := make(map[string]os.FileInfo)

	paths := append([]string{}, existingPaths...)
	paths = append(paths, "/sdk", "/sdk/src")

	for _, p := range paths {
		isDir := false
		if p == "/sdk" || p == "/sdk/src" || p == "/" {
			isDir = true
		}
		existing[p] = mockFileInfo{name: filepath.Base(p), isDir: isDir}
		// Add all parent directories
		dir := filepath.Dir(p)
		for dir != "." && dir != "/" {
			if _, ok := existing[dir]; !ok {
				existing[dir] = mockFileInfo{name: filepath.Base(dir), isDir: true}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		// Add root "/"
		existing["/"] = mockFileInfo{name: "/", isDir: true}
	}
	osStat = func(name string) (os.FileInfo, error) {
		if fi, ok := existing[name]; ok {
			return fi, nil
		}
		return nil, os.ErrNotExist
	}
	return func() {
		osStat = orig
	}
}

func TestIsStdlib(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"fmt", true},
		{"net/http", true},
		{"compress/gzip", true},
		{"example.com/foo", false},
		{"github.com/foo/bar", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsStdlib(tt.path); got != tt.want {
			t.Errorf("IsStdlib(%q) = %v; want %v", tt.path, got, tt.want)
		}
	}
}

func TestParse(t *testing.T) {
	inputJSON := `{
		"go_sdk_root": "/usr/local/go/src",
		"roots": ["example.com/foo"],
		"packages": [
			{
				"id": "example.com/foo",
				"name": "foo",
				"pkgPath": "example.com/foo",
				"goFiles": ["foo.go"]
			}
		]
	}`

	l, err := Parse(strings.NewReader(inputJSON))
	if err != nil {
		t.Fatalf("unexpected error parsing layout: %v", err)
	}

	if l.GoSDKRoot != "/usr/local/go/src" {
		t.Errorf("expected SDK root to be /usr/local/go/src, got %q", l.GoSDKRoot)
	}
	if len(l.Roots) != 1 || l.Roots[0] != "example.com/foo" {
		t.Errorf("unexpected roots: %v", l.Roots)
	}
	if len(l.Packages) != 1 || l.Packages[0].ID != "example.com/foo" {
		t.Errorf("unexpected packages: %v", l.Packages)
	}
}

func TestParse_Malformed(t *testing.T) {
	t.Run("missing closing brace", func(t *testing.T) {
		inputJSON := `{"go_sdk_root": "missing brace"`
		_, err := Parse(strings.NewReader(inputJSON))
		if err == nil {
			t.Error("expected parsing error for malformed JSON, got nil")
		}
	})

	t.Run("trailing JSON object", func(t *testing.T) {
		input := `{"go_sdk_root": "/sdk", "roots": ["foo"], "packages": []} {"trailing": true}`
		_, err := Parse(strings.NewReader(input))
		if err == nil {
			t.Error("expected parsing error for trailing JSON object, got nil")
		} else if !strings.Contains(err.Error(), "trailing garbage") {
			t.Errorf("expected error containing 'trailing garbage', got %v", err)
		}
	})

	t.Run("trailing garbage text", func(t *testing.T) {
		input := `{"go_sdk_root": "/sdk", "roots": ["foo"], "packages": []} invalidgarbage`
		_, err := Parse(strings.NewReader(input))
		if err == nil {
			t.Error("expected parsing error for trailing garbage text, got nil")
		} else if !strings.Contains(err.Error(), "trailing garbage") {
			t.Errorf("expected error containing 'trailing garbage', got %v", err)
		}
	})
}

func TestParse_RoundTrip(t *testing.T) {
	original := &Layout{
		GoSDKRoot: "/usr/local/go/src",
		Roots:     []string{"example.com/foo"},
		Packages: []*packages.Package{
			{
				ID:              "example.com/foo",
				Name:            "foo",
				PkgPath:         "example.com/foo",
				GoFiles:         []string{"foo.go"},
				CompiledGoFiles: []string{"foo.compiled.go"},
				Imports: map[string]*packages.Package{
					"example.com/bar": {ID: "example.com/bar"},
				},
			},
			{
				ID:      "example.com/bar",
				Name:    "bar",
				PkgPath: "example.com/bar",
				GoFiles: []string{"bar.go"},
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	// Parse back
	parsed, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Verify all fields are preserved
	if parsed.GoSDKRoot != original.GoSDKRoot {
		t.Errorf("SDK root mismatch: expected %q, got %q", original.GoSDKRoot, parsed.GoSDKRoot)
	}
	if !reflect.DeepEqual(parsed.Roots, original.Roots) {
		t.Errorf("Roots mismatch: expected %v, got %v", original.Roots, parsed.Roots)
	}
	if len(parsed.Packages) != 2 {
		t.Fatalf("Packages length mismatch: expected 2, got %d", len(parsed.Packages))
	}

	// Check package ordering and fields
	// Packages should be sorted by ID because of MarshalJSON
	barPkg := parsed.Packages[0]
	fooPkg := parsed.Packages[1] // "example.com/foo" > "example.com/bar", so "bar" is at 0, "foo" is at 1

	if barPkg.ID != "example.com/bar" || fooPkg.ID != "example.com/foo" {
		t.Errorf("unexpected packages sorting: pkg0=%s, pkg1=%s", barPkg.ID, fooPkg.ID)
	}

	if !reflect.DeepEqual(fooPkg.GoFiles, []string{"foo.go"}) {
		t.Errorf("GoFiles mismatch: expected [foo.go], got %v", fooPkg.GoFiles)
	}
	if !reflect.DeepEqual(fooPkg.CompiledGoFiles, []string{"foo.compiled.go"}) {
		t.Errorf("CompiledGoFiles mismatch: expected [foo.compiled.go], got %v", fooPkg.CompiledGoFiles)
	}

	// Check imported package ID
	imported, ok := fooPkg.Imports["example.com/bar"]
	if !ok {
		t.Fatal("expected import of example.com/bar")
	}
	if imported.ID != "example.com/bar" {
		t.Errorf("expected imported package ID 'example.com/bar', got %q", imported.ID)
	}
}

func TestValidateAndResolve_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	workspace := filepath.Join(tmpDir, "workspace")
	sdkRoot := filepath.Join(tmpDir, "sdk", "src")
	if err := os.MkdirAll(filepath.Join(sdkRoot, "fmt"), 0755); err != nil {
		t.Fatalf("failed to create SDK fixture: %v", err)
	}
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("failed to create workspace fixture: %v", err)
	}
	for path, content := range map[string]string{
		filepath.Join(workspace, "foo.go"):          "package foo\nimport \"fmt\"\n",
		filepath.Join(workspace, "foo_compiled.go"): "package foo",
		filepath.Join(sdkRoot, "fmt", "format.go"):  "package fmt",
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write fixture %q: %v", path, err)
		}
	}

	l := &Layout{
		GoSDKRoot: sdkRoot,
		Roots:     []string{"example.com/foo"},
		Packages: []*packages.Package{
			{
				ID:              "example.com/foo",
				Name:            "foo",
				PkgPath:         "example.com/foo",
				GoFiles:         []string{"foo.go"},
				CompiledGoFiles: []string{"foo_compiled.go"},
				Imports: map[string]*packages.Package{
					"fmt": {ID: "fmt"},
				},
			},
			{
				ID:      "fmt",
				Name:    "fmt",
				PkgPath: "fmt",
				GoFiles: []string{"/any/absolute/path/format.go"},
			},
		},
	}

	err := ValidateAndResolve(l, workspace)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	// Verify path resolutions
	fooPkg := l.Packages[0]
	if fooPkg.GoFiles[0] != filepath.Join(workspace, "foo.go") {
		t.Errorf("expected workspace relative path resolved to %q, got %q", filepath.Join(workspace, "foo.go"), fooPkg.GoFiles[0])
	}
	if fooPkg.CompiledGoFiles[0] != filepath.Join(workspace, "foo_compiled.go") {
		t.Errorf("expected compiled file resolved to %q, got %q", filepath.Join(workspace, "foo_compiled.go"), fooPkg.CompiledGoFiles[0])
	}

	fmtPkg := l.Packages[1]
	if fmtPkg.GoFiles[0] != filepath.Join(sdkRoot, "fmt", "format.go") {
		t.Errorf("expected stdlib file resolved to %q, got %q", filepath.Join(sdkRoot, "fmt", "format.go"), fmtPkg.GoFiles[0])
	}
}

func TestValidateAndResolve_RootSourceFiles(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		wantError string
	}{
		{
			name:      "root addressed by ID",
			root:      "root-id",
			wantError: `package "example.com/root" has no source files`,
		},
		{
			name:      "root addressed by import path",
			root:      "example.com/root",
			wantError: `package "example.com/root" has no source files`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := &Layout{
				Roots: []string{tt.root},
				Packages: []*packages.Package{
					{ID: "root-id", Name: "root", PkgPath: "example.com/root"},
				},
			}
			if err := ValidateAndResolve(layout, t.TempDir()); err == nil {
				t.Fatal("expected bodiless root validation error")
			} else if err.Error() != tt.wantError {
				t.Fatalf("ValidateAndResolve() error = %q, want %q", err, tt.wantError)
			}
		})
	}

	t.Run("sourced root remains valid", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, "root.go"), []byte("package root\n"), 0644); err != nil {
			t.Fatalf("writing root fixture: %v", err)
		}

		layout := &Layout{
			Roots: []string{"root-id"},
			Packages: []*packages.Package{
				{ID: "root-id", Name: "root", PkgPath: "example.com/root", GoFiles: []string{"root.go"}},
			},
		}
		if err := ValidateAndResolve(layout, workspace); err != nil {
			t.Fatalf("ValidateAndResolve() unexpected error: %v", err)
		}
	})

	t.Run("bodiless non-root remains valid", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, "root.go"), []byte("package root\n"), 0644); err != nil {
			t.Fatalf("writing root fixture: %v", err)
		}

		layout := &Layout{
			Roots: []string{"root-id"},
			Packages: []*packages.Package{
				{
					ID: "root-id", Name: "root", PkgPath: "example.com/root", GoFiles: []string{"root.go"},
				},
				{ID: "dep-id", Name: "dep", PkgPath: "example.com/dep"},
			},
		}
		if err := ValidateAndResolve(layout, workspace); err != nil {
			t.Fatalf("ValidateAndResolve() rejected bodiless non-root: %v", err)
		}
	})
}

func TestValidateAndResolve_Errors(t *testing.T) {
	tests := []struct {
		name      string
		layout    *Layout
		exist     []string
		wantError string
	}{
		{
			name: "missing sdk root for stdlib",
			layout: &Layout{
				Roots: []string{"fmt"},
				Packages: []*packages.Package{
					{ID: "fmt", Name: "fmt", PkgPath: "fmt", GoFiles: []string{"format.go"}},
				},
			},
			wantError: "go_sdk_root is required when standard library packages are present",
		},
		{
			name: "empty roots",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "foo", PkgPath: "example.com/foo"},
				},
			},
			wantError: "roots list cannot be empty",
		},
		{
			name: "empty package ID",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{ID: "", Name: "foo", PkgPath: "example.com/foo"},
				},
			},
			wantError: "package has empty ID",
		},
		{
			name: "empty package PkgPath",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "foo", PkgPath: ""},
				},
			},
			wantError: "has empty import path",
		},
		{
			name: "empty package Name",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "", PkgPath: "example.com/foo"},
				},
			},
			wantError: "has empty name",
		},
		{
			name: "duplicate ID",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "foo", PkgPath: "example.com/foo"},
					{ID: "example.com/foo", Name: "bar", PkgPath: "example.com/bar"},
				},
			},
			wantError: "duplicate package ID: example.com/foo",
		},
		{
			name: "duplicate PkgPath",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "foo", PkgPath: "example.com/foo"},
					{ID: "example.com/bar", Name: "bar", PkgPath: "example.com/foo"},
				},
			},
			wantError: "duplicate package import path: example.com/foo",
		},
		{
			name: "unknown root",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"nonexistent"},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "foo", PkgPath: "example.com/foo"},
				},
			},
			wantError: "unknown root: nonexistent",
		},
		{
			name: "unknown import ID",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{
						ID:      "example.com/foo",
						Name:    "foo",
						PkgPath: "example.com/foo",
						Imports: map[string]*packages.Package{
							"example.com/bar": {ID: "bar_id_nonexistent"},
						},
					},
				},
			},
			wantError: "imports unknown package ID \"bar_id_nonexistent\"",
		},
		{
			name: "mismatched import key",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{
						ID:      "example.com/foo",
						Name:    "foo",
						PkgPath: "example.com/foo",
						Imports: map[string]*packages.Package{
							"example.com/missing": {ID: "example.com/bar"},
						},
					},
					{
						ID:      "example.com/bar",
						Name:    "bar",
						PkgPath: "example.com/bar",
					},
				},
			},
			wantError: "package \"example.com/foo\" imports path \"example.com/missing\" with ID \"example.com/bar\", but target package import path is \"example.com/bar\"",
		},
		{
			name: "multiple malformed imports sorted lexicographically first",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{
						ID:      "example.com/foo",
						Name:    "foo",
						PkgPath: "example.com/foo",
						Imports: map[string]*packages.Package{
							"example.com/z_bad": {ID: "missing_z"},
							"example.com/a_bad": {ID: "missing_a"},
						},
					},
				},
			},
			wantError: "package \"example.com/foo\" imports unknown package ID \"missing_a\"",
		},
		{
			name: "empty source file path",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "foo", PkgPath: "example.com/foo", GoFiles: []string{""}},
				},
			},
			wantError: "contains empty source file path",
		},
		{
			name: "missing source file",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "foo", PkgPath: "example.com/foo", GoFiles: []string{"foo.go"}},
				},
			},
			wantError: "source file \"/workspace/foo.go\" for package \"example.com/foo\" does not exist",
		},
		{
			name: "null package entry",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					nil,
				},
			},
			wantError: "package entry at index 0 is null",
		},
		{
			name: "workspace absolute path rejected",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{
						ID:      "example.com/foo",
						Name:    "foo",
						PkgPath: "example.com/foo",
						GoFiles: []string{"/absolute/path/to/foo.go"},
					},
				},
			},
			wantError: "contains absolute source file path: \"/absolute/path/to/foo.go\"",
		},
		{
			name: "workspace path traversal escape direct",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{
						ID:      "example.com/foo",
						Name:    "foo",
						PkgPath: "example.com/foo",
						GoFiles: []string{"../outside.go"},
					},
				},
			},
			wantError: "escaping workspace",
		},
		{
			name: "workspace path traversal escape via subdir",
			layout: &Layout{
				GoSDKRoot: "",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{
						ID:      "example.com/foo",
						Name:    "foo",
						PkgPath: "example.com/foo",
						GoFiles: []string{"subdir/../../outside.go"},
					},
				},
			},
			wantError: "escaping workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer setupMockStat(tt.exist)()
			err := ValidateAndResolve(tt.layout, "/workspace")
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("expected error containing %q, got %q", tt.wantError, err.Error())
			}
		})
	}
}

func TestHandleDriverRequest(t *testing.T) {
	l := &Layout{
		GoSDKRoot: "/sdk",
		Roots:     []string{"example.com/foo"},
		Packages: []*packages.Package{
			{ID: "example.com/foo", Name: "foo", PkgPath: "example.com/foo"},
			{ID: "fmt", Name: "fmt", PkgPath: "fmt"},
			{ID: "os", Name: "os", PkgPath: "os"},
		},
	}

	t.Run("exact query", func(t *testing.T) {
		testLayout := &Layout{
			GoSDKRoot: "/sdk",
			Roots:     []string{"example.com/foo"},
			Packages: []*packages.Package{
				{
					ID:      "example.com/foo",
					Name:    "foo",
					PkgPath: "example.com/foo",
					Imports: map[string]*packages.Package{
						"example.com/bar": {ID: "example.com/bar"},
					},
				},
				{
					ID:      "example.com/bar",
					Name:    "bar",
					PkgPath: "example.com/bar",
				},
			},
		}

		req := &packages.DriverRequest{}
		resp, err := HandleDriverRequest(testLayout, req, []string{"example.com/foo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Marshal the driver response to JSON, and unmarshal it back (as external driver does)
		respData, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal driver response: %v", err)
		}

		var decodedResp packages.DriverResponse
		if err := json.Unmarshal(respData, &decodedResp); err != nil {
			t.Fatalf("failed to unmarshal driver response: %v", err)
		}

		// Assert roots
		if !reflect.DeepEqual(decodedResp.Roots, []string{"example.com/foo"}) {
			t.Errorf("expected roots [example.com/foo], got %v", decodedResp.Roots)
		}

		// Assert packages count and presence
		if len(decodedResp.Packages) != 2 {
			t.Errorf("expected 2 packages returned, got %d", len(decodedResp.Packages))
		}

		// Find the decoded packages
		var fooPkg, barPkg *packages.Package
		for _, p := range decodedResp.Packages {
			if p.ID == "example.com/foo" {
				fooPkg = p
			} else if p.ID == "example.com/bar" {
				barPkg = p
			}
		}

		if fooPkg == nil {
			t.Fatal("expected package example.com/foo not found in response")
		}
		if barPkg == nil {
			t.Fatal("expected package example.com/bar not found in response")
		}

		// Assert direct-import edge
		importedPkg, ok := fooPkg.Imports["example.com/bar"]
		if !ok {
			t.Fatal("expected import map to contain 'example.com/bar'")
		}
		if importedPkg == nil {
			t.Fatal("expected imported package reference to be non-nil")
		}
		if importedPkg.ID != "example.com/bar" {
			t.Errorf("expected imported package ID 'example.com/bar', got %q", importedPkg.ID)
		}
	})

	t.Run("std query", func(t *testing.T) {
		req := &packages.DriverRequest{}
		resp, err := HandleDriverRequest(l, req, []string{"std"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Roots must be standard library packages sorted deterministically
		expectedRoots := []string{"fmt", "os"}
		if !reflect.DeepEqual(resp.Roots, expectedRoots) {
			t.Errorf("expected roots %v, got %v", expectedRoots, resp.Roots)
		}
	})

	t.Run("mixed query", func(t *testing.T) {
		req := &packages.DriverRequest{}
		resp, err := HandleDriverRequest(l, req, []string{"example.com/foo", "std"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Roots must be sorted
		expectedRoots := []string{"example.com/foo", "fmt", "os"}
		if !reflect.DeepEqual(resp.Roots, expectedRoots) {
			t.Errorf("expected roots %v, got %v", expectedRoots, resp.Roots)
		}
	})

	t.Run("unknown pattern error", func(t *testing.T) {
		req := &packages.DriverRequest{}
		_, err := HandleDriverRequest(l, req, []string{"example.com/unknown"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no layout package found for pattern") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestRunDriver_Integration(t *testing.T) {
	workspaceDir := t.TempDir()
	workspaceFile := filepath.Join(workspaceDir, "foo.go")
	if err := os.WriteFile(workspaceFile, []byte("package foo"), 0644); err != nil {
		t.Fatalf("failed to create workspace source: %v", err)
	}

	// Write a temp layout file
	tmpFile, err := os.CreateTemp("", "layout-test-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	layoutData := &Layout{
		GoSDKRoot: "",
		Roots:     []string{"example.com/foo"},
		Packages: []*packages.Package{
			{
				ID:      "example.com/foo",
				Name:    "foo",
				PkgPath: "example.com/foo",
				GoFiles: []string{"foo.go"},
			},
		},
	}

	data, err := json.Marshal(layoutData)
	if err != nil {
		t.Fatalf("failed to marshal layout: %v", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	stdin := strings.NewReader(`{"Mode": 0}`)
	var stdout bytes.Buffer

	err = RunDriver(tmpFile.Name(), workspaceDir, []string{"example.com/foo"}, stdin, &stdout)
	if err != nil {
		t.Fatalf("RunDriver returned error: %v", err)
	}

	var resp packages.DriverResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if !reflect.DeepEqual(resp.Roots, []string{"example.com/foo"}) {
		t.Errorf("unexpected roots in response: %v", resp.Roots)
	}

	if len(resp.Packages) != 1 || resp.Packages[0].ID != "example.com/foo" {
		t.Errorf("unexpected packages in response: %v", resp.Packages)
	}

	if resp.Packages[0].GoFiles[0] != workspaceFile {
		t.Errorf("expected resolved path %q in package returned, got %q", workspaceFile, resp.Packages[0].GoFiles[0])
	}
}

func TestRunDriver_NullPackageEntry(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "layout-null-test-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a layout with a null package entry
	layoutData := `{"go_sdk_root": "", "roots": ["foo"], "packages": [null]}`
	if _, err := tmpFile.WriteString(layoutData); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	stdin := strings.NewReader(`{"Mode": 0}`)
	var stdout bytes.Buffer

	err = RunDriver(tmpFile.Name(), "/workspace", []string{"foo"}, stdin, &stdout)
	if err == nil {
		t.Fatal("expected RunDriver to fail with null package entry, got nil")
	}
	if !strings.Contains(err.Error(), "package entry at index 0 is null") {
		t.Errorf("expected error containing 'package entry at index 0 is null', got %v", err)
	}
}

func TestDiscoverStdlib(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "src")

	// Create SDK structure
	files := map[string]string{
		"fmt/format.go":          "package fmt",
		"os/file_unix.go":        "//go:build !windows\npackage os",
		"os/file_windows.go":     "//go:build windows\npackage os",
		"cmd/go/main.go":         "package main",
		"vendor/somepkg/some.go": "package somepkg",
		"testdata/test.go":       "package testdata",
		"internal/poll/poll.go":  "package poll",
		"nonpackage/doc.txt":     "some text",
	}

	for rel, content := range files {
		path := filepath.Join(sdkSrc, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	pkgs, err := discoverStdlib(sdkSrc)
	if err != nil {
		t.Fatalf("discoverStdlib failed: %v", err)
	}

	// Verify packages found
	found := make(map[string]*packages.Package)
	for _, p := range pkgs {
		found[p.ID] = p
	}

	// Check required standard packages
	if _, ok := found["unsafe"]; !ok {
		t.Error("expected synthetic unsafe package to be present")
	}
	if _, ok := found["fmt"]; !ok {
		t.Error("expected fmt package to be present")
	}
	if _, ok := found["os"]; !ok {
		t.Error("expected os package to be present")
	}
	if _, ok := found["internal/poll"]; !ok {
		t.Error("expected internal/poll package to be present")
	}

	// Check excluded packages/dirs
	if _, ok := found["cmd/go"]; ok {
		t.Error("expected cmd/go package to be excluded")
	}
	if _, ok := found["vendor/somepkg"]; !ok {
		t.Error("expected vendor/somepkg package to be included")
	}
	if _, ok := found["testdata"]; ok {
		t.Error("expected testdata to be excluded")
	}
	if _, ok := found["nonpackage"]; ok {
		t.Error("expected nonpackage directory to be ignored")
	}

	// Check build constraint filtering for os package
	osPkg, ok := found["os"]
	if !ok {
		t.Fatal("os package not found")
	}

	hasUnix := false
	hasWindows := false
	for _, f := range osPkg.GoFiles {
		base := filepath.Base(f)
		if base == "file_unix.go" {
			hasUnix = true
		} else if base == "file_windows.go" {
			hasWindows = true
		}
	}

	// Let's use runtime.GOOS to verify build constraint selection
	if runtime.GOOS == "windows" {
		if !hasWindows || hasUnix {
			t.Errorf("windows build constraint failed: GoFiles=%v", osPkg.GoFiles)
		}
	} else {
		if !hasUnix || hasWindows {
			t.Errorf("non-windows build constraint failed: GoFiles=%v", osPkg.GoFiles)
		}
	}
}

func TestDiscoverStdlib_VendoredImportKeyedByBarePath(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "src")

	// A standard-library package that imports a golang.org/x package the SDK
	// vendors — exactly how net imports "golang.org/x/net/dns/dnsmessage". go/types
	// resolves an import by the path as written (the bare path), so the discovered
	// stdlib package's Imports must be keyed by that bare path, with the vendored
	// package as the target. If it is keyed by the vendor/ path instead, type
	// checking the stdlib package fails with "could not import <bare path>".
	files := map[string]string{
		"resolver/resolver.go":                   "package resolver\nimport _ \"golang.org/x/example/foo\"\n",
		"vendor/golang.org/x/example/foo/foo.go": "package foo",
	}
	for rel, content := range files {
		path := filepath.Join(sdkSrc, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	pkgs, err := discoverStdlib(sdkSrc)
	if err != nil {
		t.Fatalf("discoverStdlib failed: %v", err)
	}

	resolver := packageByID(pkgs, "resolver")
	if resolver == nil {
		t.Fatal("resolver package not discovered")
	}

	const barePath = "golang.org/x/example/foo"
	const vendorPath = "vendor/golang.org/x/example/foo"

	imp, ok := resolver.Imports[barePath]
	if !ok {
		var keys []string
		for k := range resolver.Imports {
			keys = append(keys, k)
		}
		t.Fatalf("resolver.Imports missing bare key %q (go/types looks up vendored imports by the bare path); keys = %v", barePath, keys)
	}
	if imp.ID != vendorPath {
		t.Errorf("resolver.Imports[%q].ID = %q, want the vendored package %q", barePath, imp.ID, vendorPath)
	}
}

func TestDiscoverStdlib_DropsCgoPseudoImport(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "src")
	cgoFile := filepath.Join(sdkSrc, "cgo", "cgo.go")
	if err := os.MkdirAll(filepath.Dir(cgoFile), 0755); err != nil {
		t.Fatalf("failed to create SDK directory: %v", err)
	}
	if err := os.WriteFile(cgoFile, []byte("package cgo\n/*\n#include <stdlib.h>\n*/\nimport \"C\"\n"), 0644); err != nil {
		t.Fatalf("failed to write cgo fixture: %v", err)
	}

	bctx := build.Default
	bctx.GOROOT = filepath.Dir(sdkSrc)
	bctx.CgoEnabled = true
	pkgs, err := discoverStdlibWithContext(sdkSrc, bctx)
	if err != nil {
		t.Fatalf("discoverStdlibWithContext failed: %v", err)
	}

	cgoPkg := packageByID(pkgs, "cgo")
	if cgoPkg == nil {
		t.Fatal("cgo fixture package was not discovered")
	}
	if _, ok := cgoPkg.Imports["C"]; ok {
		t.Fatalf("cgo pseudo-package leaked into imports: %v", cgoPkg.Imports)
	}
}

func TestDiscoverStdlibWithContextUsesDeclaredPlatform(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "src")
	pkgDir := filepath.Join(sdkSrc, "platformpkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name := range map[string]string{
		"platformpkg_windows.go": "package platformpkg\n",
		"platformpkg_linux.go":   "package platformpkg\n",
	} {
		if err := os.WriteFile(filepath.Join(pkgDir, name), []byte("package platformpkg\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := build.Default
	ctx.GOROOT = filepath.Dir(sdkSrc)
	ctx.GOOS = "windows"
	ctx.GOARCH = "amd64"
	pkgs, err := discoverStdlibWithContext(sdkSrc, ctx)
	if err != nil {
		t.Fatalf("discoverStdlibWithContext() error = %v", err)
	}
	platformPkg := packageByID(pkgs, "platformpkg")
	if platformPkg == nil || !reflect.DeepEqual(platformPkg.GoFiles, []string{"platformpkg_windows.go"}) {
		t.Fatalf("platformpkg GoFiles = %#v, want only windows source", platformPkg)
	}
}

func TestDiscoverStdlib_RealSDK(t *testing.T) {
	sdkSrc := filepath.Join(build.Default.GOROOT, "src")
	info, err := os.Stat(sdkSrc)
	if err != nil || !info.IsDir() {
		t.Skipf("Go SDK source tree is unavailable at %q: %v", sdkSrc, err)
	}

	pkgs, err := discoverStdlib(sdkSrc)
	if err != nil {
		t.Fatalf("discoverStdlib against real SDK failed: %v", err)
	}

	byID := make(map[string]*packages.Package, len(pkgs))
	byPath := make(map[string]*packages.Package, len(pkgs))
	unsafeCount := 0
	vendorCount := 0
	for _, pkg := range pkgs {
		if _, exists := byID[pkg.ID]; exists {
			t.Fatalf("duplicate package ID %q", pkg.ID)
		}
		if _, exists := byPath[pkg.PkgPath]; exists {
			t.Fatalf("duplicate package import path %q", pkg.PkgPath)
		}
		byID[pkg.ID] = pkg
		byPath[pkg.PkgPath] = pkg
		if pkg.PkgPath == "unsafe" {
			unsafeCount++
		}
		if strings.HasPrefix(pkg.PkgPath, "vendor/") {
			vendorCount++
		}
	}
	if unsafeCount != 1 {
		t.Fatalf("unsafe package count = %d, want exactly one", unsafeCount)
	}
	if vendorCount == 0 {
		t.Fatal("real SDK did not expose any vendor-prefixed standard-library packages")
	}
	for _, pkg := range pkgs {
		for importPath, imported := range pkg.Imports {
			if importPath == "C" {
				t.Fatalf("real SDK package %q contains cgo pseudo-import C", pkg.ID)
			}
			if imported == nil {
				t.Fatalf("package %q has nil import reference for %q", pkg.ID, importPath)
			}
			if _, exists := byID[imported.ID]; !exists {
				t.Fatalf("package %q imports undiscovered package %q", pkg.ID, imported.ID)
			}
		}
	}

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "member.go"), []byte("package member\n"), 0644); err != nil {
		t.Fatalf("failed to write member fixture: %v", err)
	}

	l := &Layout{
		GoSDKRoot: sdkSrc,
		Roots:     []string{"example.com/member"},
		Packages: []*packages.Package{{
			ID:      "example.com/member",
			Name:    "member",
			PkgPath: "example.com/member",
			GoFiles: []string{"member.go"},
		}},
	}
	if err := ValidateAndResolve(l, workspace); err != nil {
		t.Fatalf("minimal layout with real SDK failed validation: %v", err)
	}
}

func TestValidateAndResolve_ImportRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "src")
	workspace := filepath.Join(tmpDir, "workspace")

	// Create mock standard library and workspace files
	files := map[string]string{
		"src/fmt/format.go": "package fmt",
		"src/os/file.go":    "package os",
		"workspace/api.go":  "package api\nimport \"fmt\"\nimport \"os\"",
	}

	for rel, content := range files {
		path := filepath.Join(tmpDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	l := &Layout{
		GoSDKRoot: sdkSrc,
		Roots:     []string{"example.com/api"},
		Packages: []*packages.Package{
			{
				ID:      "example.com/api",
				Name:    "api",
				PkgPath: "example.com/api",
				GoFiles: []string{"api.go"},
			},
		},
	}

	err := ValidateAndResolve(l, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify api package's imports got recovered
	var apiPkg *packages.Package
	for _, p := range l.Packages {
		if p.ID == "example.com/api" {
			apiPkg = p
			break
		}
	}

	if apiPkg == nil {
		t.Fatal("api package not found in layout")
	}

	if _, ok := apiPkg.Imports["fmt"]; !ok {
		t.Error("expected recovered import 'fmt'")
	}
	if _, ok := apiPkg.Imports["os"]; !ok {
		t.Error("expected recovered import 'os'")
	}

	// Verify standard library packages are merged in
	fmtFound := false
	osFound := false
	for _, p := range l.Packages {
		if p.ID == "fmt" {
			fmtFound = true
		} else if p.ID == "os" {
			osFound = true
		}
	}

	if !fmtFound {
		t.Error("expected merged fmt package in layout")
	}
	if !osFound {
		t.Error("expected merged os package in layout")
	}
}

func TestValidateAndResolve_LayoutWinsOnCollision(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "src")
	workspace := filepath.Join(tmpDir, "workspace")

	files := map[string]string{
		"src/fmt/format.go": "package fmt",
		"src/fmt/fmt.go":    "package fmt\n// customized",
	}

	for rel, content := range files {
		path := filepath.Join(tmpDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	l := &Layout{
		GoSDKRoot: sdkSrc,
		Roots:     []string{"fmt"},
		Packages: []*packages.Package{
			{
				ID:      "fmt",
				Name:    "fmt",
				PkgPath: "fmt",
				GoFiles: []string{"fmt.go"}, // layout-provided version
			},
		},
	}

	err := ValidateAndResolve(l, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify layout-provided package is unchanged and wins
	var fmtPkg *packages.Package
	for _, p := range l.Packages {
		if p.ID == "fmt" {
			if fmtPkg != nil {
				t.Fatal("found duplicate fmt package")
			}
			fmtPkg = p
		}
	}

	if fmtPkg == nil {
		t.Fatal("fmt package not found")
	}

	if len(fmtPkg.GoFiles) != 1 || filepath.Base(fmtPkg.GoFiles[0]) != "fmt.go" {
		t.Errorf("expected layout-provided fmt package to win, got GoFiles=%v", fmtPkg.GoFiles)
	}
}

func TestDiscoverStdlib_FailureClasses(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("absent", func(t *testing.T) {
		nonexistent := filepath.Join(tmpDir, "does-not-exist")
		_, err := discoverStdlib(nonexistent)
		if err == nil {
			t.Error("expected error for nonexistent SDK root, got nil")
		}
		if !strings.Contains(err.Error(), "accessing SDK root") {
			t.Errorf("expected error message containing 'accessing SDK root', got %v", err)
		}
	})

	t.Run("non-directory", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "regular-file.go")
		if err := os.WriteFile(filePath, []byte("package main"), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		_, err := discoverStdlib(filePath)
		if err == nil {
			t.Error("expected error for non-directory SDK root, got nil")
		}
		if !strings.Contains(err.Error(), "is not a directory") {
			t.Errorf("expected error message containing 'is not a directory', got %v", err)
		}
	})

	t.Run("empty-or-invalid", func(t *testing.T) {
		emptyDir := filepath.Join(tmpDir, "empty-dir")
		if err := os.MkdirAll(emptyDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		_, err := discoverStdlib(emptyDir)
		if err == nil {
			t.Error("expected error for empty SDK root, got nil")
		}
		if !strings.Contains(err.Error(), "structurally invalid") && !strings.Contains(err.Error(), "no standard-library packages discovered") {
			t.Errorf("expected error message containing 'structurally invalid' or 'no standard-library packages discovered', got %v", err)
		}
	})

	t.Run("unreadable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skip permissions check on Windows")
		}
		unreadableDir := filepath.Join(tmpDir, "unreadable-dir")
		if err := os.MkdirAll(unreadableDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		defer os.Chmod(unreadableDir, 0755)
		if err := os.Chmod(unreadableDir, 0000); err != nil {
			t.Fatalf("failed to chmod dir: %v", err)
		}
		_, err := discoverStdlib(unreadableDir)
		if err == nil {
			t.Error("expected error for unreadable directory, got nil")
		}
	})
}

func TestDriverQueries_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "src")

	// Create a valid mock SDK structure with dependencies:
	// errors/errors.go
	// io/io.go
	// net/net.go (imports a package only available under vendor/)
	// vendor/golang.org/x/net/dns/dnsmessage/message.go
	// example.com/component/component.go (the minimal layout root)
	files := map[string]string{
		"src/errors/errors.go":    "package errors",
		"src/io/io.go":            "package io",
		"src/net/net.go":          "package net\nimport \"golang.org/x/net/dns/dnsmessage\"",
		"src/net/file_unix.go":    "//go:build !windows\npackage net",
		"src/net/file_windows.go": "//go:build windows\npackage net",
		"src/vendor/golang.org/x/net/dns/dnsmessage/message.go": "package dnsmessage\nimport \"golang.org/x/net/internal/helper\"",
		"src/vendor/golang.org/x/net/internal/helper/helper.go": "package helper",
		"workspace/component.go":                                "package component\nimport \"net\"",
	}

	for rel, content := range files {
		path := filepath.Join(tmpDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	// Create a minimal layout
	l := &Layout{
		GoSDKRoot: sdkSrc,
		Roots:     []string{"example.com/component"},
		Packages: []*packages.Package{
			{
				ID:      "example.com/component",
				Name:    "component",
				PkgPath: "example.com/component",
				GoFiles: []string{"component.go"},
			},
		},
	}

	err := ValidateAndResolve(l, filepath.Join(tmpDir, "workspace"))
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	// Verify standard packages are discovered
	found := make(map[string]*packages.Package)
	for _, p := range l.Packages {
		found[p.ID] = p
	}

	// 1. Assert packages found, including the vendored standard-library target.
	for _, id := range []string{"unsafe", "errors", "io", "net", "vendor/golang.org/x/net/dns/dnsmessage", "vendor/golang.org/x/net/internal/helper"} {
		if _, ok := found[id]; !ok {
			t.Errorf("expected package %q to be discovered", id)
		}
	}

	// 2. Assert CompiledGoFiles exists and equals GoFiles.
	netPkg, ok := found["net"]
	if !ok {
		t.Fatal("net package not found")
	}
	if len(netPkg.CompiledGoFiles) == 0 {
		t.Error("expected CompiledGoFiles to be populated")
	}
	if !reflect.DeepEqual(netPkg.CompiledGoFiles, netPkg.GoFiles) {
		t.Errorf("expected CompiledGoFiles to equal GoFiles, got %v vs %v", netPkg.CompiledGoFiles, netPkg.GoFiles)
	}

	// 3. Assert Unix vs. Windows filtering
	hasUnix := false
	hasWindows := false
	for _, f := range netPkg.GoFiles {
		base := filepath.Base(f)
		if base == "file_unix.go" {
			hasUnix = true
		} else if base == "file_windows.go" {
			hasWindows = true
		}
	}
	if runtime.GOOS == "windows" {
		if !hasWindows || hasUnix {
			t.Errorf("windows build constraint filtering failed, files: %v", netPkg.GoFiles)
		}
	} else {
		if !hasUnix || hasWindows {
			t.Errorf("non-windows build constraint filtering failed, files: %v", netPkg.GoFiles)
		}
	}

	// 4. A vendored standard-library import is keyed by the bare path as written
	//    in source (that is how go/types resolves it), with the vendored package
	//    as the target ID.
	barePath := "golang.org/x/net/dns/dnsmessage"
	vendorPath := "vendor/golang.org/x/net/dns/dnsmessage"
	if netPkg.Imports == nil {
		t.Fatal("expected Imports map in net package, got nil")
	}
	if imp, ok := netPkg.Imports[barePath]; !ok {
		t.Errorf("expected net to import %q, got %v", barePath, netPkg.Imports)
	} else if imp.ID != vendorPath {
		t.Errorf("net import %q should target the vendored package %q, got %q", barePath, vendorPath, imp.ID)
	}
	vendorPkg := found[vendorPath]
	bareHelper := "golang.org/x/net/internal/helper"
	vendorHelper := "vendor/golang.org/x/net/internal/helper"
	if imp, ok := vendorPkg.Imports[bareHelper]; !ok {
		t.Errorf("expected vendored package to import %q, got %v", bareHelper, vendorPkg.Imports)
	} else if imp.ID != vendorHelper {
		t.Errorf("vendored import %q should target %q, got %q", bareHelper, vendorHelper, imp.ID)
	}
	componentPkg := found["example.com/component"]
	if _, ok := componentPkg.Imports["net"]; !ok {
		t.Errorf("expected component to retain its layout-resolved net import, got %v", componentPkg.Imports)
	}
	if _, ok := componentPkg.Imports[vendorPath]; ok {
		t.Error("component imports must not be rewritten through SDK vendor resolution")
	}

	// 5. Test Driver Queries exact-import and "std" meta-pattern
	t.Run("exact-query-and-std", func(t *testing.T) {
		req := &packages.DriverRequest{}

		// Query exact.
		resp1, err := HandleDriverRequest(l, req, []string{"net"})
		if err != nil {
			t.Fatalf("HandleDriverRequest for exact 'net' failed: %v", err)
		}
		if !reflect.DeepEqual(resp1.Roots, []string{"net"}) {
			t.Errorf("unexpected roots for exact query: %v", resp1.Roots)
		}
		if _, err := HandleDriverRequest(l, req, []string{vendorPath}); err != nil {
			t.Fatalf("HandleDriverRequest for exact vendor package failed: %v", err)
		}

		// Query std
		resp2, err := HandleDriverRequest(l, req, []string{"std"})
		if err != nil {
			t.Fatalf("HandleDriverRequest for 'std' failed: %v", err)
		}
		// Expect roots to be sorted and include the vendored package.
		expectedRoots := []string{"errors", "io", "net", "unsafe", vendorPath, "vendor/golang.org/x/net/internal/helper"}
		if !reflect.DeepEqual(resp2.Roots, expectedRoots) {
			t.Errorf("unexpected roots for std query: %v", resp2.Roots)
		}

		// Check byte-stability of repeated JSON serialization
		data1, err := json.Marshal(resp2)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		data2, err := json.Marshal(resp2)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		if !bytes.Equal(data1, data2) {
			t.Error("JSON serialization of response is not byte-stable")
		}
	})
}

func TestValidateAndResolve_ImportRecovery_Focused(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "sdk", "src")
	workspace := filepath.Join(tmpDir, "workspace")
	for rel, content := range map[string]string{
		filepath.Join("fmt", "fmt.go"): "package fmt",
		filepath.Join("os", "os.go"):   "package os",
	} {
		path := filepath.Join(sdkSrc, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create SDK directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write SDK file: %v", err)
		}
	}
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("create workspace directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "api.go"), []byte("package api\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n"), 0644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	l := &Layout{
		GoSDKRoot: sdkSrc,
		Roots:     []string{"example.com/api"},
		Packages: []*packages.Package{{
			ID:      "example.com/api",
			Name:    "api",
			PkgPath: "example.com/api",
			GoFiles: []string{"api.go"},
		}},
	}
	if err := ValidateAndResolve(l, workspace); err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	api := packageByID(l.Packages, "example.com/api")
	if api == nil {
		t.Fatal("recovered package is missing")
	}
	if got := sortedImportIDs(api); !reflect.DeepEqual(got, []string{"fmt", "os"}) {
		t.Fatalf("recovered imports = %v, want [fmt os]", got)
	}
}

func TestValidateAndResolve_PlatformImportShapes(t *testing.T) {
	workspace := t.TempDir()
	writeLayoutSource := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeLayoutSource("api_linux.go", "package api\nimport \"example.com/linux\"\n")
	writeLayoutSource("api_windows.go", "package api\nimport \"example.com/windows\"\n")
	writeLayoutSource("linux.go", "package linux\n")
	writeLayoutSource("windows.go", "package windows\n")

	newLayout := func(imports map[string]*packages.Package) *Layout {
		return &Layout{
			Platform: &Platform{GOOS: "linux", GOARCH: "amd64"},
			Roots:    []string{"example.com/api"},
			Packages: []*packages.Package{
				{ID: "example.com/api", Name: "api", PkgPath: "example.com/api", GoFiles: []string{"api_linux.go", "api_windows.go"}, Imports: imports},
				{ID: "example.com/linux", Name: "linux", PkgPath: "example.com/linux", GoFiles: []string{"linux.go"}, Imports: map[string]*packages.Package{}},
				{ID: "example.com/windows", Name: "windows", PkgPath: "example.com/windows", GoFiles: []string{"windows.go"}, Imports: map[string]*packages.Package{}},
			},
		}
	}

	t.Run("filtered declared imports pass", func(t *testing.T) {
		layout := newLayout(map[string]*packages.Package{"example.com/linux": {ID: "example.com/linux"}})
		if err := ValidateAndResolve(layout, workspace); err != nil {
			t.Fatalf("ValidateAndResolve() error = %v", err)
		}
		api := packageByID(layout.Packages, "example.com/api")
		if _, ok := api.Imports["example.com/linux"]; !ok {
			t.Fatalf("resolved imports = %v, want linux import", api.Imports)
		}
	})

	t.Run("imports omitted are recovered after filtering", func(t *testing.T) {
		layout := newLayout(nil)
		if err := ValidateAndResolve(layout, workspace); err != nil {
			t.Fatalf("ValidateAndResolve() error = %v", err)
		}
		api := packageByID(layout.Packages, "example.com/api")
		if got := sortedImportIDs(api); !reflect.DeepEqual(got, []string{"example.com/linux"}) {
			t.Fatalf("recovered imports = %v, want linux only", got)
		}
	})

	t.Run("declared union is rejected", func(t *testing.T) {
		layout := newLayout(map[string]*packages.Package{
			"example.com/linux":   {ID: "example.com/linux"},
			"example.com/windows": {ID: "example.com/windows"},
		})
		err := ValidateAndResolve(layout, workspace)
		if err == nil || !strings.Contains(err.Error(), "example.com/windows") || !strings.Contains(err.Error(), "not contributed") {
			t.Fatalf("ValidateAndResolve() error = %v, want union mismatch naming windows", err)
		}
	})

	t.Run("surviving import missing from declarations is rejected", func(t *testing.T) {
		layout := newLayout(map[string]*packages.Package{})
		err := ValidateAndResolve(layout, workspace)
		if err == nil || !strings.Contains(err.Error(), "example.com/api") || !strings.Contains(err.Error(), "api_linux.go") || !strings.Contains(err.Error(), "example.com/linux") {
			t.Fatalf("ValidateAndResolve() error = %v, want source import mismatch, got %v", err, err)
		}
	})
}

func TestValidateAndResolve_UnresolvedImportsAreFacts(t *testing.T) {
	workspace := t.TempDir()
	for name, content := range map[string]string{
		"api_a.go": "package api\nimport \"example.com/missing\"\n",
		"api_b.go": "package api\nimport \"example.com/missing\"\n",
		"dep.go":   "package dep\n",
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	newLayout := func(imports map[string]*packages.Package) *Layout {
		return &Layout{
			Roots: []string{"example.com/api"},
			Packages: []*packages.Package{
				{ID: "example.com/api", Name: "api", PkgPath: "example.com/api", GoFiles: []string{"api_b.go", "api_a.go"}, Imports: imports},
				{ID: "example.com/dep", Name: "dep", PkgPath: "example.com/dep", GoFiles: []string{"dep.go"}, Imports: map[string]*packages.Package{}},
			},
		}
	}

	for _, tc := range []struct {
		name    string
		imports map[string]*packages.Package
	}{
		{name: "omitted", imports: nil},
		{name: "declared", imports: map[string]*packages.Package{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := newLayout(tc.imports)
			if err := ValidateAndResolve(layout, workspace); err != nil {
				t.Fatalf("ValidateAndResolve() error = %v", err)
			}
			want := []UnresolvedImport{
				{Package: "example.com/api", SourceFile: filepath.Join(workspace, "api_a.go"), ImportPath: "example.com/missing"},
				{Package: "example.com/api", SourceFile: filepath.Join(workspace, "api_b.go"), ImportPath: "example.com/missing"},
			}
			if !reflect.DeepEqual(layout.UnresolvedImports, want) {
				t.Fatalf("UnresolvedImports = %#v, want %#v", layout.UnresolvedImports, want)
			}
			if got := sortedImportIDs(packageByID(layout.Packages, "example.com/api")); len(got) != 0 {
				t.Fatalf("unresolved import was added to package graph: %v", got)
			}
		})
	}
}

func TestValidateAndResolve_LayoutPrecedence_Focused(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "sdk", "src")
	workspace := filepath.Join(tmpDir, "workspace")
	for _, root := range []string{filepath.Join(sdkSrc, "fmt"), workspace} {
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(sdkSrc, "fmt", "stdlib.go"):          "package fmt",
		filepath.Join(sdkSrc, "fmt", "layout.go"):          "package fmt",
		filepath.Join(sdkSrc, "fmt", "layout_compiled.go"): "package fmt",
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write fixture file: %v", err)
		}
	}

	l := &Layout{
		GoSDKRoot: sdkSrc,
		Roots:     []string{"layout-fmt"},
		Packages: []*packages.Package{{
			ID:              "layout-fmt",
			Name:            "fmt",
			PkgPath:         "fmt",
			GoFiles:         []string{"layout.go"},
			CompiledGoFiles: []string{"layout_compiled.go"},
			Dir:             "layout-authority",
		}},
	}
	if err := ValidateAndResolve(l, workspace); err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	fmtPkg := packageByID(l.Packages, "layout-fmt")
	if fmtPkg == nil {
		t.Fatal("layout-provided fmt package is missing")
	}
	if got := filepath.Base(fmtPkg.GoFiles[0]); got != "layout.go" {
		t.Fatalf("layout-provided GoFiles = %q, want layout.go", got)
	}
	if got := filepath.Base(fmtPkg.CompiledGoFiles[0]); got != "layout_compiled.go" {
		t.Fatalf("layout-provided CompiledGoFiles = %q, want layout_compiled.go", got)
	}
	if fmtPkg.Dir != "layout-authority" {
		t.Fatalf("layout-provided metadata was replaced: Dir=%q", fmtPkg.Dir)
	}
	if got := countPackagesByPath(l.Packages, "fmt"); got != 1 {
		t.Fatalf("found %d packages with import path fmt, want one", got)
	}
}

func TestHandleDriverRequest_DeterministicOutput_Focused(t *testing.T) {
	l := &Layout{Packages: []*packages.Package{
		{ID: "z.example/root", Name: "root", PkgPath: "z.example/root", Imports: map[string]*packages.Package{
			"os": {ID: "os"},
		}},
		{ID: "os", Name: "os", PkgPath: "os"},
		{ID: "fmt", Name: "fmt", PkgPath: "fmt"},
	}}
	req := &packages.DriverRequest{}
	resp, err := HandleDriverRequest(l, req, []string{"z.example/root", "std"})
	if err != nil {
		t.Fatalf("HandleDriverRequest failed: %v", err)
	}
	if got, want := resp.Roots, []string{"fmt", "os", "z.example/root"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("roots = %v, want %v", got, want)
	}
	ids := make([]string, len(resp.Packages))
	for i, pkg := range resp.Packages {
		ids[i] = pkg.ID
	}
	if want := []string{"fmt", "os", "z.example/root"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("response package IDs = %v, want %v", ids, want)
	}
	first, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal first response: %v", err)
	}
	second, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal second response: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("repeated response serialization is not byte-stable")
	}
}

func packageByID(pkgs []*packages.Package, id string) *packages.Package {
	for _, pkg := range pkgs {
		if pkg.ID == id {
			return pkg
		}
	}
	return nil
}

func sortedImportIDs(pkg *packages.Package) []string {
	ids := make([]string, 0, len(pkg.Imports))
	for path, imported := range pkg.Imports {
		if imported != nil {
			ids = append(ids, path)
		}
	}
	sort.Strings(ids)
	return ids
}

func countPackagesByPath(pkgs []*packages.Package, path string) int {
	count := 0
	for _, pkg := range pkgs {
		if pkg.PkgPath == path {
			count++
		}
	}
	return count
}

func TestValidateAndResolve_ImportRecovery_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "sdk", "src")
	workspace := filepath.Join(tmpDir, "workspace")

	// Create mock standard library and workspace files
	files := map[string]string{
		"sdk/src/fmt/format.go":      "package fmt",
		"sdk/src/os/file.go":         "package os",
		"sdk/src/strings/strings.go": "package strings",

		// Multiple files in api package
		"workspace/api1.go":    "package api\nimport \"fmt\"\nimport _ \"os\"\n",
		"workspace/api2.go":    "package api\nimport . \"strings\"\nimport str \"strings\"\n",
		"workspace/api_bad.go": "package api\nimport \"non_stdlib_pkg\"\n", // non-stdlib import
	}

	for rel, content := range files {
		path := filepath.Join(tmpDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	l := &Layout{
		GoSDKRoot: sdkSrc,
		Roots:     []string{"example.com/api"},
		Packages: []*packages.Package{
			{
				ID:      "example.com/api",
				Name:    "api",
				PkgPath: "example.com/api",
				GoFiles: []string{"api1.go", "api2.go", "api_bad.go"},
				Imports: map[string]*packages.Package{
					"fmt":     {ID: "layout-fmt"}, // already-provided stdlib edge targeting a distinct ID
					"os":      {ID: "os"},
					"strings": {ID: "strings"},
				},
			},
			{
				ID:      "layout-fmt",
				Name:    "fmt",
				PkgPath: "fmt",
				GoFiles: []string{"format.go"},
			},
		},
	}

	err := ValidateAndResolve(l, workspace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify api package's declared imports remain intact.
	apiPkg := packageByID(l.Packages, "example.com/api")
	if apiPkg == nil {
		t.Fatal("api package not found in layout")
	}

	// fmt was already-provided with a custom ID, so it should be preserved unchanged
	fmtImport, ok := apiPkg.Imports["fmt"]
	if !ok {
		t.Error("expected already-provided import 'fmt' to be preserved")
	} else if fmtImport.ID != "layout-fmt" {
		t.Errorf("expected already-provided import 'fmt' to preserve custom ID 'layout-fmt', got %q", fmtImport.ID)
	}
	// os was declared for the blank import
	if _, ok := apiPkg.Imports["os"]; !ok {
		t.Error("expected recovered blank import 'os'")
	}
	// strings was declared for the dot/aliased imports
	if _, ok := apiPkg.Imports["strings"]; !ok {
		t.Error("expected recovered aliased/dot import 'strings'")
	}
	// non_stdlib_pkg should NOT be recovered (leave non-standard missing imports)
	if _, ok := apiPkg.Imports["non_stdlib_pkg"]; ok {
		t.Error("non_stdlib_pkg should NOT be synthesized/recovered")
	}
}

func TestValidateAndResolve_ImportRecovery_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	sdkSrc := filepath.Join(tmpDir, "sdk", "src")
	workspace := filepath.Join(tmpDir, "workspace")

	if err := os.MkdirAll(filepath.Join(sdkSrc, "fmt"), 0755); err != nil {
		t.Fatalf("failed to create SDK: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sdkSrc, "fmt", "fmt.go"), []byte("package fmt"), 0644); err != nil {
		t.Fatalf("failed to write SDK fmt: %v", err)
	}
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	t.Run("malformed source syntax", func(t *testing.T) {
		badSource := "packge api\n" // misspelled package keyword in imports-only mode
		filePath := filepath.Join(workspace, "api_malformed.go")
		if err := os.WriteFile(filePath, []byte(badSource), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		defer os.Remove(filePath)

		l := &Layout{
			GoSDKRoot: sdkSrc,
			Roots:     []string{"example.com/api"},
			Packages: []*packages.Package{
				{
					ID:      "example.com/api",
					Name:    "api",
					PkgPath: "example.com/api",
					GoFiles: []string{"api_malformed.go"},
				},
			},
		}

		err := ValidateAndResolve(l, workspace)
		if err == nil {
			t.Fatal("expected error for malformed source syntax, got nil")
		}
		if !strings.Contains(err.Error(), "parsing source file") || !strings.Contains(err.Error(), "api_malformed.go") || !strings.Contains(err.Error(), "example.com/api") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("malformed import literal", func(t *testing.T) {
		// Import with invalid escape sequence or token
		badImport := "package api\nimport \"fmt\\x\"\n"
		filePath := filepath.Join(workspace, "api_bad_import.go")
		if err := os.WriteFile(filePath, []byte(badImport), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		defer os.Remove(filePath)

		l := &Layout{
			GoSDKRoot: sdkSrc,
			Roots:     []string{"example.com/api"},
			Packages: []*packages.Package{
				{
					ID:      "example.com/api",
					Name:    "api",
					PkgPath: "example.com/api",
					GoFiles: []string{"api_bad_import.go"},
				},
			},
		}

		err := ValidateAndResolve(l, workspace)
		if err == nil {
			t.Fatal("expected error for malformed import literal, got nil")
		}
		if !strings.Contains(err.Error(), "parsing source file") || !strings.Contains(err.Error(), "api_bad_import.go") || !strings.Contains(err.Error(), "example.com/api") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestValidateAndResolve_ImportRecovery_IncompleteGraph(t *testing.T) {
	tests := []struct {
		name        string
		importPath  string
		expectError string
	}{
		{
			name:        "dotted third-party import",
			importPath:  "example.com/missing",
			expectError: "no metadata for example.com/missing",
		},
		{
			name:        "SDK-absent standard-library-shaped import",
			importPath:  "missingstd",
			expectError: "no metadata for missingstd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			sdkSrc := filepath.Join(runtime.GOROOT(), "src")
			// This case needs a real SDK source tree to discover the stdlib
			// against. `go test` has one via runtime.GOROOT(); the rules_go
			// test environment points GOROOT at a non-existent placeholder, so
			// skip there rather than fail.
			if info, err := os.Stat(sdkSrc); err != nil || !info.IsDir() {
				t.Skipf("no Go SDK source tree at %s; skipping", sdkSrc)
			}
			workspace := filepath.Join(tmpDir, "workspace")

			if err := os.MkdirAll(workspace, 0755); err != nil {
				t.Fatalf("failed to create workspace: %v", err)
			}

			// Write workspace source that imports and uses the test path
			srcName := filepath.Base(tt.importPath)
			srcContent := fmt.Sprintf("package api\nimport %q\nfunc F() {\n\t%s.Foo()\n}\n", tt.importPath, srcName)
			if err := os.WriteFile(filepath.Join(workspace, "api.go"), []byte(srcContent), 0644); err != nil {
				t.Fatalf("failed to write source file: %v", err)
			}

			l := &Layout{
				GoSDKRoot: sdkSrc,
				Roots:     []string{"example.com/api"},
				Packages: []*packages.Package{
					{
						ID:              "example.com/api",
						Name:            "api",
						PkgPath:         "example.com/api",
						GoFiles:         []string{"api.go"},
						CompiledGoFiles: []string{"api.go"},
					},
				},
			}

			// Marshal layout to a temporary JSON file BEFORE we run ValidateAndResolve (which mutates in-place to absolute paths)
			layoutFile := filepath.Join(tmpDir, "layout.json")
			layoutData, err := json.Marshal(l)
			if err != nil {
				t.Fatalf("failed to marshal layout: %v", err)
			}
			if err := os.WriteFile(layoutFile, layoutData, 0644); err != nil {
				t.Fatalf("failed to write layout file: %v", err)
			}

			err = ValidateAndResolve(l, workspace)
			if err != nil {
				t.Fatalf("unexpected error during ValidateAndResolve: %v", err)
			}

			// Verify the import is NOT synthesized in l.Packages[0].Imports
			apiPkg := l.Packages[0]
			if apiPkg.Imports != nil {
				if _, exists := apiPkg.Imports[tt.importPath]; exists {
					t.Errorf("unwanted synthesized import for path %q", tt.importPath)
				}
			}

			// Run packages.Load inside WithDriverEnv to collect load/type errors
			var loadErr error
			err = WithDriverEnv(layoutFile, workspace, func() error {
				cfg := &packages.Config{
					Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
						packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
						packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
					Dir: workspace,
				}
				pkgs, err := packages.Load(cfg, "example.com/api")
				if err != nil {
					return err
				}
				var errMsgs []string
				packages.Visit(pkgs, nil, func(p *packages.Package) {
					for _, err := range p.Errors {
						errMsgs = append(errMsgs, err.Msg)
					}
				})
				if len(errMsgs) > 0 {
					loadErr = fmt.Errorf("package load errors:\n%s", strings.Join(errMsgs, "\n"))
				}
				return nil
			})

			if err != nil {
				t.Fatalf("driver env callback error: %v", err)
			}

			if loadErr == nil {
				t.Fatal("expected package load errors, got nil")
			}

			if !strings.Contains(loadErr.Error(), tt.expectError) {
				t.Errorf("expected load error to contain %q, got:\n%v", tt.expectError, loadErr)
			}
		})
	}
}

func TestValidateAndResolveFiltersBuildConstraints(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skipf("fixture only carries linux/windows/darwin variants; host GOOS is %q", runtime.GOOS)
	}

	ws := t.TempDir()
	pkgDir := filepath.Join(ws, "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// common compiles everywhere; each plat_<goos>.go compiles only on its OS
	// (filename-suffix constraint); tagged_only.go compiles only on windows
	// (//go:build line, no filename suffix).
	write("common.go", "package pkg\n")
	write("plat_linux.go", "package pkg\n")
	write("plat_windows.go", "package pkg\n")
	write("plat_darwin.go", "package pkg\n")
	write("tagged_only.go", "//go:build windows\n\npackage pkg\n")

	rel := func(n string) string { return "pkg/" + n }
	all := []string{
		rel("common.go"), rel("plat_linux.go"), rel("plat_windows.go"),
		rel("plat_darwin.go"), rel("tagged_only.go"),
	}
	layout := &Layout{
		Roots: []string{"example.com/pkg"},
		Packages: []*packages.Package{{
			ID:              "example.com/pkg",
			Name:            "pkg",
			PkgPath:         "example.com/pkg",
			GoFiles:         append([]string{}, all...),
			CompiledGoFiles: append([]string{}, all...),
			Imports:         map[string]*packages.Package{},
		}},
	}

	if err := ValidateAndResolve(layout, ws); err != nil {
		t.Fatalf("ValidateAndResolve: %v", err)
	}

	got := map[string]bool{}
	for _, f := range layout.Packages[0].CompiledGoFiles {
		got[filepath.Base(f)] = true
	}

	// The always-on file and the current OS's variant survive.
	for _, want := range []string{"common.go", "plat_" + runtime.GOOS + ".go"} {
		if !got[want] {
			t.Errorf("expected %q to be kept after build-constraint filtering; kept: %v", want, got)
		}
	}
	// Other-OS filename variants are dropped (a superset would make the package IllTyped).
	for _, variant := range []string{"plat_linux.go", "plat_windows.go", "plat_darwin.go"} {
		if variant != "plat_"+runtime.GOOS+".go" && got[variant] {
			t.Errorf("expected %q to be filtered out on %s; kept: %v", variant, runtime.GOOS, got)
		}
	}
	// The //go:build windows file is dropped everywhere but windows, proving line
	// constraints (not just filename suffixes) are honored.
	if runtime.GOOS != "windows" && got["tagged_only.go"] {
		t.Errorf("expected //go:build windows file to be filtered out on %s; kept: %v", runtime.GOOS, got)
	}
}

func TestFileMatchesBuildConstraints(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	// A file gated to the current platform is compiled; its negation is not. This
	// underpins ValidateInterfaceFiles skipping build-constraint-excluded
	// interface files (e.g. a //go:build-gated file when wrapping a
	// cross-platform library).
	included := write("included.go", "//go:build "+runtime.GOOS+"\n\npackage p\n")
	excluded := write("excluded.go", "//go:build !"+runtime.GOOS+"\n\npackage p\n")
	plain := write("plain.go", "package p\n")
	notGo := write("data.txt", "not go\n")

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"current-GOOS build tag is compiled", included, true},
		{"negated-GOOS build tag is excluded", excluded, false},
		{"unconstrained .go is compiled", plain, true},
		{"non-.go path passes through", notGo, true},
		{"unreadable file kept as safety net", filepath.Join(dir, "absent.go"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FileMatchesBuildConstraints(tc.path); got != tc.want {
				t.Errorf("FileMatchesBuildConstraints(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestBuildContextForLayout(t *testing.T) {
	t.Run("absent platform copies defaults", func(t *testing.T) {
		got, err := BuildContextForLayout(&Layout{})
		if err != nil {
			t.Fatalf("BuildContextForLayout() error = %v", err)
		}
		if got.GOOS != build.Default.GOOS || got.GOARCH != build.Default.GOARCH || got.CgoEnabled != build.Default.CgoEnabled {
			t.Fatalf("default context = %#v, want GOOS=%q GOARCH=%q CgoEnabled=%v", got, build.Default.GOOS, build.Default.GOARCH, build.Default.CgoEnabled)
		}
		got.BuildTags = append(got.BuildTags, "mutated")
		if slicesEqual(got.BuildTags, build.Default.BuildTags) {
			t.Fatal("derived BuildTags shares build.Default backing storage")
		}
	})

	t.Run("declared values override defaults", func(t *testing.T) {
		layout := &Layout{Platform: &Platform{
			GOOS:       "windows",
			GOARCH:     "386",
			BuildTags:  []string{"purego"},
			CgoEnabled: true,
		}}
		got, err := BuildContextForLayout(layout)
		if err != nil {
			t.Fatalf("BuildContextForLayout() error = %v", err)
		}
		if got.GOOS != "windows" || got.GOARCH != "386" || !got.CgoEnabled || !slicesEqual(got.BuildTags, []string{"purego"}) {
			t.Fatalf("declared context = %#v", got)
		}
		if got.GOROOT != build.Default.GOROOT || got.Compiler != build.Default.Compiler {
			t.Fatalf("declared context discarded default toolchain fields: %#v", got)
		}
	})

	for _, tc := range []struct {
		name     string
		platform Platform
		want     string
	}{
		{name: "empty goos", platform: Platform{GOARCH: "amd64"}, want: `platform.goos has invalid value ""`},
		{name: "unsupported goos", platform: Platform{GOOS: "unknown", GOARCH: "amd64"}, want: `platform.goos has invalid value "unknown"`},
		{name: "retired goos", platform: Platform{GOOS: "hurd", GOARCH: "amd64"}, want: `platform.goos has invalid value "hurd"`},
		{name: "empty goarch", platform: Platform{GOOS: "linux"}, want: `platform.goarch has invalid value ""`},
		{name: "unsupported goarch", platform: Platform{GOOS: "linux", GOARCH: "unknown"}, want: `platform.goarch has invalid value "unknown"`},
		{name: "retired goarch", platform: Platform{GOOS: "linux", GOARCH: "amd64p32"}, want: `platform.goarch has invalid value "amd64p32"`},
		{name: "unsupported target combination", platform: Platform{GOOS: "windows", GOARCH: "arm"}, want: `platform.goarch has unsupported value "arm" for goos "windows"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildContextForLayout(&Layout{Platform: &tc.platform})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("BuildContextForLayout() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPlatformConstraintsUseDeclaredContext(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	tests := []struct {
		name     string
		platform Platform
		file     string
		want     bool
	}{
		{name: "custom tag", platform: Platform{GOOS: "linux", GOARCH: "amd64", BuildTags: []string{"purego"}}, file: write("purego.go", "//go:build purego\n\npackage p\n"), want: true},
		{name: "custom tag absent", platform: Platform{GOOS: "linux", GOARCH: "amd64"}, file: filepath.Join(dir, "purego.go"), want: false},
		{name: "declared target", platform: Platform{GOOS: "windows", GOARCH: "amd64"}, file: write("only_windows.go", "package p\n"), want: true},
		{name: "declared target rejects host variant", platform: Platform{GOOS: "windows", GOARCH: "amd64"}, file: write("only_linux.go", "package p\n"), want: false},
		{name: "cgo enabled", platform: Platform{GOOS: "linux", GOARCH: "amd64", CgoEnabled: true}, file: write("cgo.go", "//go:build cgo\n\npackage p\n"), want: true},
		{name: "cgo disabled", platform: Platform{GOOS: "linux", GOARCH: "amd64", CgoEnabled: false}, file: filepath.Join(dir, "cgo.go"), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := BuildContextForLayout(&Layout{Platform: &tc.platform})
			if err != nil {
				t.Fatal(err)
			}
			if got := FileMatchesBuildConstraintsWithContext(tc.file, ctx); got != tc.want {
				t.Errorf("FileMatchesBuildConstraintsWithContext() = %v, want %v", got, tc.want)
			}
			bulk := filterByBuildConstraintsWithContext([]string{tc.file}, ctx)
			if (len(bulk) == 1) != tc.want {
				t.Errorf("filterByBuildConstraintsWithContext() kept %v, want kept=%v", bulk, tc.want)
			}
		})
	}
}

func TestValidateAndResolveRejectsConstraintEmptyPackage(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "pkg", "only_windows.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layout := &Layout{
		Platform: &Platform{GOOS: "linux", GOARCH: "amd64"},
		Roots:    []string{"example.com/pkg"},
		Packages: []*packages.Package{{
			ID:              "example.com/pkg",
			Name:            "pkg",
			PkgPath:         "example.com/pkg",
			GoFiles:         []string{"pkg/only_windows.go"},
			CompiledGoFiles: []string{"pkg/only_windows.go"},
			Imports:         map[string]*packages.Package{},
		}},
	}
	if err := ValidateAndResolve(layout, workspace); err == nil || !strings.Contains(err.Error(), `example.com/pkg`) || !strings.Contains(err.Error(), "excluded every Go source") {
		t.Fatalf("ValidateAndResolve() error = %v, want package-specific constraint-empty error", err)
	}
}

func TestValidateAndResolveAllowsPreExistingBodilessNonRoot(t *testing.T) {
	workspace := t.TempDir()
	rootPath := filepath.Join(workspace, "root.go")
	if err := os.WriteFile(rootPath, []byte("package root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	layout := &Layout{
		Platform: &Platform{GOOS: "linux", GOARCH: "amd64"},
		Roots:    []string{"example.com/root"},
		Packages: []*packages.Package{
			{ID: "example.com/root", Name: "root", PkgPath: "example.com/root", GoFiles: []string{"root.go"}, CompiledGoFiles: []string{"root.go"}, Imports: map[string]*packages.Package{}},
			{ID: "example.com/empty", Name: "empty", PkgPath: "example.com/empty", Imports: map[string]*packages.Package{}},
		},
	}
	if err := ValidateAndResolve(layout, workspace); err != nil {
		t.Fatalf("ValidateAndResolve() rejected pre-existing bodiless non-root: %v", err)
	}
}

func TestPlatformJSONRoundTripIsDeterministic(t *testing.T) {
	platform := &Platform{GOOS: "linux", GOARCH: "amd64", BuildTags: []string{"z", "a"}, CgoEnabled: false}
	layout := &Layout{
		GoSDKRoot: "/sdk/src",
		Platform:  platform,
		Roots:     []string{"z", "a"},
		Packages: []*packages.Package{
			{ID: "z", Name: "z", PkgPath: "z"},
			{ID: "a", Name: "a", PkgPath: "a"},
		},
	}
	originalRoots := append([]string(nil), layout.Roots...)
	originalTags := append([]string(nil), platform.BuildTags...)
	first, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || !reflect.DeepEqual(layout.Roots, originalRoots) || !reflect.DeepEqual(platform.BuildTags, originalTags) {
		t.Fatalf("marshal was not deterministic or mutated caller data: first=%s second=%s", first, second)
	}
	parsed, err := Parse(bytes.NewReader(first))
	if err != nil || parsed.Platform == nil || !reflect.DeepEqual(parsed.Platform, platform) {
		t.Fatalf("platform round trip = %#v, %v; want %#v", parsed.Platform, err, platform)
	}
	without := &Layout{Roots: []string{"a"}, Packages: []*packages.Package{{ID: "a", Name: "a", PkgPath: "a"}}}
	data, err := json.Marshal(without)
	if err != nil || strings.Contains(string(data), `"platform"`) {
		t.Fatalf("absent platform serialization = %s, %v", data, err)
	}
}

func slicesEqual(a, b []string) bool {
	return reflect.DeepEqual(a, b)
}
