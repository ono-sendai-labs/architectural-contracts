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
			name: "mismatched import key",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					{
						ID:      "foo",
						Name:    "foo",
						PkgPath: "foo",
						Imports: map[string]*packages.Package{
							"example.com/missing": {ID: "bar"},
						},
					},
					{
						ID:      "bar",
						Name:    "bar",
						PkgPath: "example.com/bar",
					},
				},
			},
			wantError: "package \"foo\" imports path \"example.com/missing\" with ID \"bar\", but target package import path is \"example.com/bar\"",
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
		{
			name: "null package entry",
			layout: &Layout{
				GoSDKRoot: "/sdk",
				Roots:     []string{"foo"},
				Packages: []*packages.Package{
					nil,
				},
			},
			wantError: "package entry at index 0 is null",
		},
		{
			name: "workspace absolute path rejected",
			layout: &Layout{
				GoSDKRoot: "/sdk",
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
				GoSDKRoot: "/sdk",
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
				GoSDKRoot: "/sdk",
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

func TestRunDriver_NullPackageEntry(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "layout-null-test-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a layout with a null package entry
	layoutData := `{"go_sdk_root": "/sdk", "roots": ["foo"], "packages": [null]}`
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
