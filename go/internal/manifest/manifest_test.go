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
`
	r := bytes.NewReader([]byte(input))
	got, err := manifest.Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	want := manifest.Manifest{
		Name:           "toprow",
		InterfaceFiles: []string{"toprow.go"},
		Authority:      manifest.DeclaredAuthority(),
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
declared_authority: "FILES"
declared_authority: "EXEC"
`
	r := bytes.NewReader([]byte(input))
	got, err := manifest.Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	want := manifest.Manifest{
		Name:           "allfields",
		InterfaceFiles: []string{"file1.go", "file2.go"},
		ComponentDependencies: []manifest.ComponentDependency{
			{Name: "dep1", Manifest: "path/to/dep1/component.textproto"},
			{Name: "dep2", Manifest: "path/to/dep2/component.textproto"},
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
	if !reflect.DeepEqual(got.DeclaredAuthority, want.DeclaredAuthority) {
		t.Errorf("DeclaredAuthority = %v, want %v", got.DeclaredAuthority, want.DeclaredAuthority)
	}
}

func TestParse_RejectsStaleAbsorbedDependencies(t *testing.T) {
	input := `name: "stale"
interface_files: "api.go"
absorbed_dependencies {
  import_path: "example.com/impl"
}
`
	r := bytes.NewReader([]byte(input))
	_, err := manifest.Parse(r)
	if err == nil {
		t.Fatalf("expected Parse to reject absorbed_dependencies; it must never be silently ignored")
	}
	if !strings.Contains(err.Error(), "absorbed_dependencies") {
		t.Errorf("expected error to identify unsupported field absorbed_dependencies, got: %v", err)
	}
}

// occurrences in comments and both quoted string forms must not trip the guard.
func TestParse_RemovedFieldMentionsOutsideFieldPositions(t *testing.T) {
	input := `# a comment explaining that absorbed_dependencies { } used to exist here
# own_check_runs and certification_reference were retired fields
name: 'absorbed_dependencies {'
interface_files: "api.go"
interface_files: "certification_reference: own_check_runs"
`
	r := bytes.NewReader([]byte(input))
	m, err := manifest.Parse(r)
	if err != nil {
		t.Fatalf("expected Parse to accept removed-field mentions in comments and string values, got: %v", err)
	}
	if m.Name != "absorbed_dependencies {" {
		t.Errorf("Name = %q, want %q", m.Name, "absorbed_dependencies {")
	}
}

func TestParse_RejectsStaleVerificationFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		field string
	}{
		{
			name: "own_check_runs",
			input: `name: "stale"
interface_files: "api.go"
own_check_runs: true
`,
			field: "own_check_runs",
		},
		{
			name: "own_check_runs_false",
			input: `name: "stale"
interface_files: "api.go"
own_check_runs: false
`,
			field: "own_check_runs",
		},
		{
			name: "certification_reference",
			input: `name: "stale"
interface_files: "api.go"
certification_reference: "build://surface-check"
`,
			field: "certification_reference",
		},
		{
			name: "own_check_runs_message_form",
			input: `name: "stale"
interface_files: "api.go"
own_check_runs {}
`,
			field: "own_check_runs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manifest.Parse(bytes.NewBufferString(tt.input))
			var removed *manifest.RemovedFieldError
			if !errors.As(err, &removed) {
				t.Fatalf("Parse() error = %v, want RemovedFieldError", err)
			}
			if removed.Field != tt.field {
				t.Errorf("RemovedFieldError.Field = %q, want %q", removed.Field, tt.field)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Errorf("error %q does not name retired field %q", err, tt.field)
			}
		})
	}
}

func TestParse_DeclaredMembershipFields(t *testing.T) {
	input := `name: "surface"
members: "example.com/app"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
component_dependencies {
  name: "runtime"
  manifest: "../runtime/component.textproto"
  auto_attached: true
}
`

	got, err := manifest.Parse(bytes.NewBufferString(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if got.InterfaceStyle != manifest.InterfaceStylePackageSurface {
		t.Errorf("InterfaceStyle = %v, want package surface", got.InterfaceStyle)
	}
	if !reflect.DeepEqual(got.Members, []string{"example.com/app"}) {
		t.Errorf("Members = %v, want [example.com/app]", got.Members)
	}
	if len(got.ComponentDependencies) != 1 || !got.ComponentDependencies[0].AutoAttached {
		t.Errorf("ComponentDependencies = %+v, want one auto-attached dependency", got.ComponentDependencies)
	}
}

func TestParse_DefaultStyleStillRequiresInterfaceFiles(t *testing.T) {
	got, err := manifest.Parse(bytes.NewBufferString(`name: "legacy"`))
	if !errors.Is(err, manifest.ErrEmptyInterfaceFiles) {
		t.Fatalf("Parse() error = %v, want %v", err, manifest.ErrEmptyInterfaceFiles)
	}
	if got.Name != "" {
		t.Errorf("Parse() returned non-zero manifest on error: %+v", got)
	}
}

func TestParse_MemberValidation(t *testing.T) {
	tests := []struct {
		name       string
		members    string
		wantKind   string
		wantMember string
		wantText   string
	}{
		{
			name:       "duplicate",
			members:    "interface_files: \"api.go\"\nmembers: \"example.com/app\"\nmembers: \"example.com/app\"",
			wantKind:   "member",
			wantMember: "example.com/app",
		},
		{
			name:       "glob member star",
			members:    `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE` + "\n" + `members: "example.com/app/*"`,
			wantMember: "example.com/app/*",
			wantText:   "literal import path",
		},
		{
			name:       "glob question mark",
			members:    `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE` + "\n" + `members: "example.com/app?"`,
			wantMember: "example.com/app?",
			wantText:   "literal import path",
		},
		{
			name:       "glob bracket expression",
			members:    `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE` + "\n" + `members: "example.com/v[0-9]"`,
			wantMember: "example.com/v[0-9]",
			wantText:   "literal import path",
		},
		{
			name:       "malformed bracket expression",
			members:    `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE` + "\n" + `members: "example.com/v["`,
			wantMember: "example.com/v[",
			wantText:   "literal import path",
		},
		{
			name:       "glob escape",
			members:    `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE` + "\n" + `members: "example.com/v\\*"`,
			wantMember: `example.com/v\\*`,
			wantText:   "literal import path",
		},
		{
			name:       "declared style glob",
			members:    "interface_files: \"api.go\"\nmembers: \"example.com/app/*\"",
			wantMember: "example.com/app/*",
			wantText:   "literal import path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "name: \"component\"\n" + tt.members
			_, err := manifest.Parse(bytes.NewBufferString(input))
			if err == nil {
				t.Fatalf("Parse() succeeded, want error for %s", tt.name)
			}
			if tt.wantKind != "" {
				var duplicate *manifest.DuplicateDeclarationError
				if !errors.As(err, &duplicate) || duplicate.Kind != tt.wantKind || duplicate.Value != tt.wantMember {
					t.Fatalf("Parse() error = %v, want duplicate %s %q", err, tt.wantKind, tt.wantMember)
				}
			}
			if !strings.Contains(err.Error(), tt.wantMember) {
				t.Errorf("Parse() error = %q, want offending member %q", err, tt.wantMember)
			}
			if tt.wantText != "" && !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("Parse() error = %q, want text %q", err, tt.wantText)
			}
		})
	}
}

func TestParse_PackageSurfaceShape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "requires members",
			input: `name: "surface" interface_style: INTERFACE_STYLE_PACKAGE_SURFACE`,
			want:  "requires at least one member",
		},
		{
			name:  "forbids interface files",
			input: `name: "surface" interface_style: INTERFACE_STYLE_PACKAGE_SURFACE members: "example.com/app" interface_files: "api.go"`,
			want:  "must not declare interface files",
		},
		{
			name:  "accepts literals",
			input: `name: "surface" interface_style: INTERFACE_STYLE_PACKAGE_SURFACE members: "example.com/app"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manifest.Parse(bytes.NewBufferString(tt.input))
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Parse() failed: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse() error = %v, want text %q", err, tt.want)
			}
		})
	}

	for _, m := range []string{"example.com/app*", "example.com/app?", "example.com/[a-z]", "example.com/ap[p", `example.com/ap\\p`} {
		input := `name: "surface" interface_style: INTERFACE_STYLE_PACKAGE_SURFACE members: "` + m + `"`
		_, err := manifest.Parse(bytes.NewBufferString(input))
		if err == nil || !strings.Contains(err.Error(), "literal import path") {
			t.Errorf("Parse(%s) error = %v, want literal-import-path member error", input, err)
		}
	}
}

func TestParse_UnknownInterfaceStyleRejected(t *testing.T) {
	_, err := manifest.Parse(bytes.NewBufferString(`name: "component" interface_style: 99 interface_files: "api.go"`))
	if err == nil || !strings.Contains(err.Error(), "unknown interface style") {
		t.Fatalf("Parse() error = %v, want unknown interface style error", err)
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
			interfaceFiles: []string{"bypass.go", "facts.go", "member.go", "refs.go"},
			dependencies: map[string]string{
				"manifest": "../manifest/component.textproto",
				"symbol":   "../symbol/component.textproto",
			},
		},
		"../report/component.textproto": {
			name:           "report",
			interfaceFiles: []string{"report.go"},
			dependencies:   map[string]string{},
		},
		"../checker/component.textproto": {
			name:           "checker",
			interfaceFiles: []string{"aggregate.go", "checker.go", "classify.go"},
			dependencies: map[string]string{
				"capanalyzer":     "../capanalyzer/component.textproto",
				"facts":           "../facts/component.textproto",
				"manifest":        "../manifest/component.textproto",
				"report":          "../report/component.textproto",
				"stdlibauthority": "../stdlibauthority/component.textproto",
				"symbol":          "../symbol/component.textproto",
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

// TestParse_AuthorityDefaultsAndUnknown covers the persisted authority axis
// (task requirements 1, 3): omitted authority parses as known DECLARED, an
// explicit UNKNOWN with empty declared_authority parses as the distinct
// unknown value, and the contradictory spellings fail with actionable errors.
func TestParse_AuthorityDefaultsAndUnknown(t *testing.T) {
	t.Run("OmittedDefaultsToKnownDeclared", func(t *testing.T) {
		input := `name: "legacy"
interface_files: "api.go"
`
		got, err := manifest.Parse(bytes.NewReader([]byte(input)))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !got.Authority.Known || len(got.Authority.Set) != 0 {
			t.Errorf("Authority = %+v, want known-empty DECLARED{}", got.Authority)
		}
	})

	t.Run("ExplicitDeclaredWithCapabilities", func(t *testing.T) {
		input := `name: "explicit"
interface_files: "api.go"
authority: DECLARED
declared_authority: "NETWORK"
declared_authority: "FILES"
`
		got, err := manifest.Parse(bytes.NewReader([]byte(input)))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !manifest.Equal(got.Authority, manifest.DeclaredAuthority("FILES", "NETWORK")) {
			t.Errorf("Authority = %+v, want sorted DECLARED{FILES NETWORK}", got.Authority)
		}
	})

	t.Run("UnknownWithEmptyDeclaredAuthority", func(t *testing.T) {
		input := `name: "unowned"
interface_files: "api.go"
authority: UNKNOWN
`
		got, err := manifest.Parse(bytes.NewReader([]byte(input)))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if got.Authority.Known {
			t.Errorf("Authority = %+v, want UNKNOWN", got.Authority)
		}
	})

	t.Run("UnknownWithDeclaredAuthorityIsContradictory", func(t *testing.T) {
		input := `name: "contradiction"
interface_files: "api.go"
authority: UNKNOWN
declared_authority: "FILES"
`
		_, err := manifest.Parse(bytes.NewReader([]byte(input)))
		if err == nil {
			t.Fatal("Parse(UNKNOWN + declared_authority), want error")
		}
		if !strings.Contains(err.Error(), "declared_authority") {
			t.Errorf("error = %v, want actionable message naming declared_authority", err)
		}
	})

	t.Run("UnknownAuthorityEnumValueRejected", func(t *testing.T) {
		input := `name: "bogus"
interface_files: "api.go"
authority: 99
`
		_, err := manifest.Parse(bytes.NewReader([]byte(input)))
		if err == nil {
			t.Fatal("Parse(authority: 99), want error")
		}
		if !strings.Contains(err.Error(), "authority") {
			t.Errorf("error = %v, want message naming authority", err)
		}
	})
}
