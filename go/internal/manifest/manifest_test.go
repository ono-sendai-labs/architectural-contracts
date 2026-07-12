package manifest_test

import (
	"bytes"
	"errors"
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
