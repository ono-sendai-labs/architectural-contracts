package packagelayout

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// setupMockStat configures a mock file system where only the specified paths exist.
func setupMockStat(existingPaths []string) func() {
	orig := osStat
	existing := make(map[string]bool)
	for _, p := range existingPaths {
		existing[p] = true
	}
	osStat = func(name string) (os.FileInfo, error) {
		if existing[name] {
			return nil, nil // Nil file info is sufficient since we only check existence
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
	inputJSON := `{"go_sdk_root": "missing brace"`
	_, err := Parse(strings.NewReader(inputJSON))
	if err == nil {
		t.Error("expected parsing error for malformed JSON, got nil")
	}
}

func TestValidateAndResolve_Valid(t *testing.T) {
	defer setupMockStat([]string{
		"/workspace/foo.go",
		"/workspace/foo_compiled.go",
		"/sdk/src/fmt/format.go",
	})()

	l := &Layout{
		GoSDKRoot: "/sdk/src",
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

	err := ValidateAndResolve(l, "/workspace")
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	// Verify path resolutions
	fooPkg := l.Packages[0]
	if fooPkg.GoFiles[0] != "/workspace/foo.go" {
		t.Errorf("expected workspace relative path resolved to /workspace/foo.go, got %q", fooPkg.GoFiles[0])
	}
	if fooPkg.CompiledGoFiles[0] != "/workspace/foo_compiled.go" {
		t.Errorf("expected compiled file resolved to /workspace/foo_compiled.go, got %q", fooPkg.CompiledGoFiles[0])
	}

	fmtPkg := l.Packages[1]
	if fmtPkg.GoFiles[0] != "/sdk/src/fmt/format.go" {
		t.Errorf("expected stdlib file resolved to /sdk/src/fmt/format.go, got %q", fmtPkg.GoFiles[0])
	}
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
				GoSDKRoot: "/sdk",
				Roots:     []string{},
				Packages: []*packages.Package{
					{ID: "foo", Name: "foo", PkgPath: "foo"},
				},
			},
			wantError: "roots list cannot be empty",
		},
		{
			name: "empty package ID",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					{ID: "", Name: "foo", PkgPath: "foo"},
				},
			},
			wantError: "package has empty ID",
		},
		{
			name: "empty package PkgPath",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					{ID: "foo", Name: "foo", PkgPath: ""},
				},
			},
			wantError: "has empty import path",
		},
		{
			name: "empty package Name",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					{ID: "foo", Name: "", PkgPath: "foo"},
				},
			},
			wantError: "has empty name",
		},
		{
			name: "duplicate ID",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					{ID: "foo", Name: "foo", PkgPath: "foo"},
					{ID: "foo", Name: "bar", PkgPath: "bar"},
				},
			},
			wantError: "duplicate package ID: foo",
		},
		{
			name: "duplicate PkgPath",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					{ID: "foo", Name: "foo", PkgPath: "foo"},
					{ID: "bar", Name: "bar", PkgPath: "foo"},
				},
			},
			wantError: "duplicate package import path: foo",
		},
		{
			name: "unknown root",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"nonexistent"},
				Packages: []*packages.Package{
					{ID: "foo", Name: "foo", PkgPath: "foo"},
				},
			},
			wantError: "unknown root: nonexistent",
		},
		{
			name: "unknown import ID",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					{
						ID:      "foo",
						Name:    "foo",
						PkgPath: "foo",
						Imports: map[string]*packages.Package{
							"bar": {ID: "bar_id_nonexistent"},
						},
					},
				},
			},
			wantError: "imports unknown package ID \"bar_id_nonexistent\"",
		},
		{
			name: "empty source file path",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					{ID: "foo", Name: "foo", PkgPath: "foo", GoFiles: []string{""}},
				},
			},
			wantError: "contains empty source file path",
		},
		{
			name: "missing source file",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"example.com/foo"},
				Packages: []*packages.Package{
					{ID: "example.com/foo", Name: "foo", PkgPath: "example.com/foo", GoFiles: []string{"foo.go"}},
				},
			},
			wantError: "source file \"/workspace/foo.go\" for package \"example.com/foo\" does not exist",
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
		req := &packages.DriverRequest{}
		resp, err := HandleDriverRequest(l, req, []string{"example.com/foo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(resp.Roots, []string{"example.com/foo"}) {
			t.Errorf("expected roots [example.com/foo], got %v", resp.Roots)
		}
		if len(resp.Packages) != 3 {
			t.Errorf("expected 3 packages returned, got %d", len(resp.Packages))
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
	// Write a temp layout file
	tmpFile, err := os.CreateTemp("", "layout-test-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	layoutData := &Layout{
		GoSDKRoot: "/sdk",
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

	defer setupMockStat([]string{"/workspace/foo.go"})()

	stdin := strings.NewReader(`{"Mode": 0}`)
	var stdout bytes.Buffer

	err = RunDriver(tmpFile.Name(), "/workspace", []string{"example.com/foo"}, stdin, &stdout)
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

	if resp.Packages[0].GoFiles[0] != "/workspace/foo.go" {
		t.Errorf("expected resolved path in package returned, got %q", resp.Packages[0].GoFiles[0])
	}
}
