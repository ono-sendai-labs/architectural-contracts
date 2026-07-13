package manifest_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

func TestParse_ValidRoundTrip(t *testing.T) {
	input := `name: "toprow"
interface_files: "toprow.go"
absorbed_dependencies {
  import_path: "example.com/csvtool/internal/parsecsv"
  reason: "CSV parsing impl detail"
}
`
	r := bytes.NewReader([]byte(input))
	got, err := manifest.Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	wantReason := "CSV parsing impl detail"
	want := manifest.Manifest{
		Name:           "toprow",
		InterfaceFiles: []string{"toprow.go"},
		AbsorbedDependencies: []manifest.AbsorbedDependency{
			{
				ImportPath: "example.com/csvtool/internal/parsecsv",
				Reason:     &wantReason,
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() got:\n%+v\nwant:\n%+v", got, want)
	}
}

func TestParse_AllFieldsMap(t *testing.T) {
	input := `name: "allfields"
interface_files: "file1.go"
interface_files: "file2.go"
component_dependencies {
  name: "dep1"
  manifest: "path/to/dep1/component.textproto"
}
component_dependencies {
  name: "dep2"
  manifest: "path/to/dep2/component.textproto"
}
absorbed_dependencies {
  import_path: "github.com/some/pkg1"
}
absorbed_dependencies {
  import_path: "github.com/some/pkg2"
  reason: ""
}
absorbed_dependencies {
  import_path: "github.com/some/pkg3"
  reason: "custom reason"
}
declared_authority: "FILES"
declared_authority: "EXEC"
`
	r := bytes.NewReader([]byte(input))
	got, err := manifest.Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	reasonEmpty := ""
	reasonCustom := "custom reason"
	want := manifest.Manifest{
		Name:           "allfields",
		InterfaceFiles: []string{"file1.go", "file2.go"},
		ComponentDependencies: []manifest.ComponentDependency{
			{Name: "dep1", Manifest: "path/to/dep1/component.textproto"},
			{Name: "dep2", Manifest: "path/to/dep2/component.textproto"},
		},
		AbsorbedDependencies: []manifest.AbsorbedDependency{
			{ImportPath: "github.com/some/pkg1", Reason: nil},
			{ImportPath: "github.com/some/pkg2", Reason: &reasonEmpty},
			{ImportPath: "github.com/some/pkg3", Reason: &reasonCustom},
		},
		DeclaredAuthority: []string{"FILES", "EXEC"},
	}

	// Compare field by field to inspect closely
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if !reflect.DeepEqual(got.InterfaceFiles, want.InterfaceFiles) {
		t.Errorf("InterfaceFiles = %v, want %v", got.InterfaceFiles, want.InterfaceFiles)
	}
	if !reflect.DeepEqual(got.ComponentDependencies, want.ComponentDependencies) {
		t.Errorf("ComponentDependencies = %+v, want %+v", got.ComponentDependencies, want.ComponentDependencies)
	}
	if len(got.AbsorbedDependencies) != len(want.AbsorbedDependencies) {
		t.Fatalf("AbsorbedDependencies len = %d, want %d", len(got.AbsorbedDependencies), len(want.AbsorbedDependencies))
	}
	for i := range want.AbsorbedDependencies {
		gotDep := got.AbsorbedDependencies[i]
		wantDep := want.AbsorbedDependencies[i]
		if gotDep.ImportPath != wantDep.ImportPath {
			t.Errorf("AbsorbedDependencies[%d].ImportPath = %q, want %q", i, gotDep.ImportPath, wantDep.ImportPath)
		}
		if (gotDep.Reason == nil) != (wantDep.Reason == nil) {
			t.Errorf("AbsorbedDependencies[%d].Reason nilness mismatch: got %v, want %v", i, gotDep.Reason == nil, wantDep.Reason == nil)
		} else if gotDep.Reason != nil && *gotDep.Reason != *wantDep.Reason {
			t.Errorf("AbsorbedDependencies[%d].Reason = %q, want %q", i, *gotDep.Reason, *wantDep.Reason)
		}
	}
	if !reflect.DeepEqual(got.DeclaredAuthority, want.DeclaredAuthority) {
		t.Errorf("DeclaredAuthority = %v, want %v", got.DeclaredAuthority, want.DeclaredAuthority)
	}
}

// errorReader always returns a custom error on Read.
type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("custom read error")
}

func TestParse_Failures(t *testing.T) {
	t.Run("MalformedTextproto", func(t *testing.T) {
		input := `name: "broken" interface_files: [ "missing_quote ]`
		r := bytes.NewReader([]byte(input))
		got, err := manifest.Parse(r)
		if err == nil {
			t.Errorf("expected error for malformed textproto, got nil error and manifest: %+v", got)
		}
		if got.Name != "" || len(got.InterfaceFiles) > 0 {
			t.Errorf("expected zero Manifest on failure, got %+v", got)
		}
	})

	t.Run("ReaderError", func(t *testing.T) {
		got, err := manifest.Parse(errorReader{})
		if err == nil {
			t.Errorf("expected error for reader error, got nil")
		}
		if got.Name != "" || len(got.InterfaceFiles) > 0 {
			t.Errorf("expected zero Manifest on failure, got %+v", got)
		}
	})
}

func TestParse_Validation(t *testing.T) {
	t.Run("EmptyName", func(t *testing.T) {
		input := `interface_files: "file.go"`
		r := bytes.NewReader([]byte(input))
		got, err := manifest.Parse(r)
		if err == nil {
			t.Fatalf("expected error for empty name, got nil")
		}
		if !errors.Is(err, manifest.ErrEmptyName) {
			t.Errorf("expected error %v, got %v", manifest.ErrEmptyName, err)
		}
		if got.Name != "" {
			t.Errorf("expected empty manifest on error, got %+v", got)
		}
	})

	t.Run("EmptyInterfaceFiles", func(t *testing.T) {
		input := `name: "component"`
		r := bytes.NewReader([]byte(input))
		got, err := manifest.Parse(r)
		if err == nil {
			t.Fatalf("expected error for empty interface_files, got nil")
		}
		if !errors.Is(err, manifest.ErrEmptyInterfaceFiles) {
			t.Errorf("expected error %v, got %v", manifest.ErrEmptyInterfaceFiles, err)
		}
		if got.Name != "" {
			t.Errorf("expected empty manifest on error, got %+v", got)
		}
	})

	t.Run("DuplicateInterfaceFiles", func(t *testing.T) {
		input := `name: "component"
interface_files: "file.go"
interface_files: "file.go"
`
		r := bytes.NewReader([]byte(input))
		_, err := manifest.Parse(r)
		var dupErr *manifest.DuplicateDeclarationError
		if !errors.As(err, &dupErr) || dupErr.Kind != "interface file" || dupErr.Value != "file.go" {
			t.Fatalf("expected DuplicateDeclarationError for interface file 'file.go', got: %v", err)
		}
	})

	t.Run("DuplicateComponentDependencies", func(t *testing.T) {
		input := `name: "component"
interface_files: "file.go"
component_dependencies {
  name: "dep1"
  manifest: "path1"
}
component_dependencies {
  name: "dep1"
  manifest: "path2"
}
`
		r := bytes.NewReader([]byte(input))
		_, err := manifest.Parse(r)
		var dupErr *manifest.DuplicateDeclarationError
		if !errors.As(err, &dupErr) || dupErr.Kind != "component dependency" || dupErr.Value != "dep1" {
			t.Fatalf("expected DuplicateDeclarationError for component dependency 'dep1', got: %v", err)
		}
	})

	t.Run("DuplicateAbsorbedDependencies", func(t *testing.T) {
		input := `name: "component"
interface_files: "file.go"
absorbed_dependencies {
  import_path: "github.com/some/pkg"
}
absorbed_dependencies {
  import_path: "github.com/some/pkg"
}
`
		r := bytes.NewReader([]byte(input))
		_, err := manifest.Parse(r)
		var dupErr *manifest.DuplicateDeclarationError
		if !errors.As(err, &dupErr) || dupErr.Kind != "absorbed dependency" || dupErr.Value != "github.com/some/pkg" {
			t.Fatalf("expected DuplicateDeclarationError for absorbed dependency 'github.com/some/pkg', got: %v", err)
		}
	})

	t.Run("DuplicateDeclaredAuthorities", func(t *testing.T) {
		input := `name: "component"
interface_files: "file.go"
declared_authority: "FILES"
declared_authority: "FILES"
`
		r := bytes.NewReader([]byte(input))
		_, err := manifest.Parse(r)
		var dupErr *manifest.DuplicateDeclarationError
		if !errors.As(err, &dupErr) || dupErr.Kind != "declared authority" || dupErr.Value != "FILES" {
			t.Fatalf("expected DuplicateDeclarationError for declared authority 'FILES', got: %v", err)
		}
	})

	t.Run("UnknownCapability", func(t *testing.T) {
		input := `name: "component"
interface_files: "file.go"
declared_authority: "FILE"
`
		r := bytes.NewReader([]byte(input))
		_, err := manifest.Parse(r)
		var unknownErr *manifest.UnknownCapabilityError
		if !errors.As(err, &unknownErr) || unknownErr.Capability != "FILE" {
			t.Fatalf("expected UnknownCapabilityError for capability 'FILE', got: %v", err)
		}
	})
}

func TestSelfHostingManifests(t *testing.T) {
	manifestPaths := []string{
		"../capanalyzer/component.textproto",
		"../facts/component.textproto",
		"../report/component.textproto",
		"../checker/component.textproto",
	}

	for _, p := range manifestPaths {
		t.Run(p, func(t *testing.T) {
			content, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("failed to read manifest file %s: %v", p, err)
			}
			m, err := manifest.Parse(bytes.NewReader(content))
			if err != nil {
				t.Fatalf("failed to parse manifest: %v", err)
			}
			if m.Name == "" {
				t.Error("expected non-empty component name")
			}
			if len(m.InterfaceFiles) == 0 {
				t.Error("expected at least one interface file")
			}
			if len(m.DeclaredAuthority) > 0 {
				t.Errorf("expected empty declared authority, got %v", m.DeclaredAuthority)
			}
		})
	}
}
