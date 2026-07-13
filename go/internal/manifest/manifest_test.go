package manifest_test

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func isStdlib(importPath string) bool {
	if importPath == "C" {
		return true
	}
	first := importPath
	if idx := strings.Index(importPath, "/"); idx != -1 {
		first = importPath[:idx]
	}
	return !strings.Contains(first, ".")
}

func importToComponent(imp string) (string, bool) {
	const repoImportPrefix = "github.com/ono-sendai-labs/architectural-contracts/go/internal/"
	if strings.HasPrefix(imp, repoImportPrefix) {
		sub := imp[len(repoImportPrefix):]
		parts := strings.Split(sub, "/")
		if len(parts) > 0 {
			return parts[0], true
		}
	}
	return "", false
}

func isSubdirOrEqual(parent, child string) bool {
	parentClean := filepath.Clean(parent)
	childClean := filepath.Clean(child)
	if parentClean == childClean {
		return true
	}
	rel, err := filepath.Rel(parentClean, childClean)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && rel != "."
}

func getNonTestImports(t *testing.T, dir string) map[string]bool {
	imports := make(map[string]bool)
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
			continue
		}
		filePath := filepath.Join(dir, file.Name())
		f, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("failed to parse Go file %s: %v", filePath, err)
		}
		for _, imp := range f.Imports {
			pathVal := strings.Trim(imp.Path.Value, `"`)
			if !isStdlib(pathVal) {
				imports[pathVal] = true
			}
		}
	}
	return imports
}

func TestSelfHostingManifests(t *testing.T) {
	type expectedManifest struct {
		name           string
		interfaceFiles []string
		dependencies   map[string]string // name -> manifest path
	}

	expected := map[string]expectedManifest{
		"../capanalyzer/component.textproto": {
			name:           "capanalyzer",
			interfaceFiles: []string{"capanalyzer.go"},
			dependencies:   map[string]string{},
		},
		"../facts/component.textproto": {
			name:           "facts",
			interfaceFiles: []string{"facts.go"},
			dependencies: map[string]string{
				"capanalyzer": "../capanalyzer/component.textproto",
			},
		},
		"../report/component.textproto": {
			name:           "report",
			interfaceFiles: []string{"report.go"},
			dependencies:   map[string]string{},
		},
		"../checker/component.textproto": {
			name:           "checker",
			interfaceFiles: []string{"checker.go"},
			dependencies: map[string]string{
				"capanalyzer": "../capanalyzer/component.textproto",
				"facts":       "../facts/component.textproto",
				"manifest":    "../manifest/component.textproto",
				"report":      "../report/component.textproto",
			},
		},
	}

	// 1. Assert pairwise disjointness of roots
	roots := make(map[string]string)
	for p := range expected {
		absPath, err := filepath.Abs(p)
		if err != nil {
			t.Fatalf("failed to get absolute path for %s: %v", p, err)
		}
		roots[p] = filepath.Dir(absPath)
	}

	for p1, r1 := range roots {
		for p2, r2 := range roots {
			if p1 == p2 {
				continue
			}
			if isSubdirOrEqual(r1, r2) {
				t.Errorf("manifest roots are not disjoint: root of %s (%s) is a subdirectory of or equal to root of %s (%s)", p1, r1, p2, r2)
			}
		}
	}

	// 2. Validate each manifest and its dependencies against package source
	for p, exp := range expected {
		t.Run(p, func(t *testing.T) {
			content, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("failed to read manifest file %s: %v", p, err)
			}
			m, err := manifest.Parse(bytes.NewReader(content))
			if err != nil {
				t.Fatalf("failed to parse manifest: %v", err)
			}

			// Validate basic fields
			if m.Name != exp.name {
				t.Errorf("expected component name %q, got %q", exp.name, m.Name)
			}

			if len(m.DeclaredAuthority) > 0 {
				t.Errorf("expected empty declared authority, got %v", m.DeclaredAuthority)
			}

			if len(m.AbsorbedDependencies) > 0 {
				t.Errorf("expected empty absorbed dependencies, got %v", m.AbsorbedDependencies)
			}

			// Validate interface files
			if len(m.InterfaceFiles) != len(exp.interfaceFiles) {
				t.Errorf("expected %d interface files, got %d", len(exp.interfaceFiles), len(m.InterfaceFiles))
			} else {
				for i, file := range exp.interfaceFiles {
					if m.InterfaceFiles[i] != file {
						t.Errorf("expected interface file at index %d to be %q, got %q", i, file, m.InterfaceFiles[i])
					}
					// Assert each named file exists beneath its manifest root
					fullPath := filepath.Join(filepath.Dir(p), file)
					if info, err := os.Stat(fullPath); err != nil {
						t.Errorf("expected interface file %q to exist at %q: %v", file, fullPath, err)
					} else if info.IsDir() {
						t.Errorf("expected interface file %q to be a file, but it is a directory", fullPath)
					}
				}
			}

			// Validate component dependencies
			if len(m.ComponentDependencies) != len(exp.dependencies) {
				t.Errorf("expected %d component dependencies, got %d", len(exp.dependencies), len(m.ComponentDependencies))
			} else {
				for _, dep := range m.ComponentDependencies {
					expectedPath, ok := exp.dependencies[dep.Name]
					if !ok {
						t.Errorf("unexpected declared component dependency %q", dep.Name)
					} else if dep.Manifest != expectedPath {
						t.Errorf("expected dependency %q manifest path %q, got %q", dep.Name, expectedPath, dep.Manifest)
					}
				}
			}

			// Compare component edges to core package's non-stdlib imports
			manifestDir := filepath.Dir(p)
			nonStdlibImports := getNonTestImports(t, manifestDir)

			// Map imports to expected component dependencies
			importedComps := make(map[string]bool)
			for imp := range nonStdlibImports {
				if compName, ok := importToComponent(imp); ok {
					importedComps[compName] = true
				}
			}

			// Assert exact match between actual imported components and declared dependencies
			for compName := range importedComps {
				if _, ok := exp.dependencies[compName]; !ok {
					t.Errorf("package imports component %q but it is not declared as a dependency in the manifest", compName)
				}
			}
			for compName := range exp.dependencies {
				if !importedComps[compName] {
					t.Errorf("manifest declares dependency on component %q but it is not imported by any non-test file", compName)
				}
			}
		})
	}
}
