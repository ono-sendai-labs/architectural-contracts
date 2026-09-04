package manifestparity_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

// checkedInComponentManifests parses every checked-in component manifest in
// the Go tree keyed by component name. The Go tree's manifests live in exactly
// two roots — one per package under go/internal and the CLI's under
// go/cmd/arcc — so scanning those two roots enumerates all checked-in
// component manifests in the repository. The test binary runs with the package
// directory as working directory, so the paths are relative to
// go/internal/manifestparity.
func checkedInComponentManifests(t *testing.T) map[string]manifest.Manifest {
	t.Helper()

	roots := []string{"..", filepath.Join("..", "..", "cmd")}
	manifests := map[string]manifest.Manifest{}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("reading %s: %v", root, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name(), "component.textproto")
			content, err := os.ReadFile(path)
			if err != nil {
				// Directories without a checked-in manifest are skipped; a
				// directory that promises one but cannot be parsed is fatal.
				if !os.IsNotExist(err) {
					t.Fatalf("reading %s: %v", path, err)
				}
				continue
			}
			m, err := manifest.Parse(bytes.NewReader(content))
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			manifests[m.Name] = m
		}
	}
	return manifests
}

const modulePrefix = "github.com/ono-sendai-labs/architectural-contracts/go/"

// TestProjectPackagesSingleOwner pins the repaired component ownership model
// (step 3 review finding F1): every project package in the
// manifest/symbol/artifactio slice has exactly one architectural owner, and the
// artifact-I/O shell component does not claim the manifest, schema, or symbol
// packages as its own members — its uses of them are component dependencies.
func TestProjectPackagesSingleOwner(t *testing.T) {
	manifests := checkedInComponentManifests(t)

	// The slice under repair; every other component (e.g. goanalysis, which
	// predates the slice and is remediated separately) is out of scope here.
	slice := []string{"artifactio", "manifest", "schema", "symbol"}

	owner := map[string]string{}
	for _, name := range slice {
		m, ok := manifests[name]
		if !ok {
			t.Fatalf("checked-in manifest for component %q not found", name)
		}
		for _, member := range m.Members {
			if !strings.HasPrefix(member, modulePrefix) {
				continue
			}
			if prev, dup := owner[member]; dup {
				t.Errorf("project package %q is a member of both component %q and component %q; each project package must have exactly one architectural owner", member, prev, name)
				continue
			}
			owner[member] = name
		}
	}

	wantOwner := map[string]string{
		modulePrefix + "internal/artifactio": "artifactio",
		modulePrefix + "internal/manifest":   "manifest",
		modulePrefix + "internal/schema":     "schema",
		modulePrefix + "internal/schema/gen": "schema",
		modulePrefix + "internal/symbol":     "symbol",
	}
	for pkg, want := range wantOwner {
		if got := owner[pkg]; got != want {
			t.Errorf("package %q must be owned by component %q, got owner %q", pkg, want, got)
		}
	}
}

// TestArtifactioDeclaresCoreComponentDependencies pins the shell-to-core
// dependency edges (amended spec, recorded in the task file): the artifactio
// shell must consume the schema surface (generated types and the shared
// capability taxonomy) and the symbol grammar through declared component
// dependencies rather than duplicated membership, and its declared edges must
// cover every core package its production code actually imports. It uses no
// manifest-package API — the capability taxonomy it validates against is
// schema.KnownCapabilities, owned by the schema component — so a manifest
// dependency would be an unused edge. The dependency names and manifest paths
// are contract strings resolved by `arcc check`.
func TestArtifactioDeclaresCoreComponentDependencies(t *testing.T) {
	manifests := checkedInComponentManifests(t)
	m, ok := manifests["artifactio"]
	if !ok {
		t.Fatal("checked-in manifest for component artifactio not found")
	}

	want := map[string]string{
		"schema": "../schema/component.textproto",
		"symbol": "../symbol/component.textproto",
	}
	got := map[string]string{}
	for _, dep := range m.ComponentDependencies {
		got[dep.Name] = dep.Manifest
	}
	for name, path := range want {
		if got[name] != path {
			t.Errorf("artifactio component dependency %q = %q, want %q", name, got[name], path)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("artifactio declares unexpected component dependency %q", name)
		}
	}

	// The declared edges must be sufficient for the actual imports: every
	// core package artifactio's production code imports must be a member of a
	// declared dependency (or of artifactio itself).
	owners := map[string]string{}
	for _, name := range []string{"artifactio", "schema", "symbol"} {
		for _, member := range manifests[name].Members {
			if strings.HasPrefix(member, modulePrefix) {
				owners[member] = name
			}
		}
	}
	imports := projectImports(t, "../../internal/artifactio")
	for imp := range imports {
		owningComp, isCore := owners[imp]
		if !isCore {
			continue
		}
		if !isDeclared(owningComp, want, got) {
			t.Errorf("artifactio imports core package %q (component %q) without a declared component dependency", imp, owningComp)
		}
	}
	if imports[modulePrefix+"internal/manifest"] {
		t.Errorf("artifactio imports the manifest package; the amended spec makes the schema component own the shared capability taxonomy, so the shell must consume it via the schema dependency")
	}
}

// isDeclared reports whether a component dependency on comp is declared.
func isDeclared(comp string, want, got map[string]string) bool {
	_, inWant := want[comp]
	_, inGot := got[comp]
	return inWant && inGot
}

// projectImports parses the non-test Go files under dir (relative to this
// package) and returns the module-internal import paths they use.
func projectImports(t *testing.T, dir string) map[string]bool {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	imports := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s/%s: %v", dir, name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, modulePrefix) {
				imports[path] = true
			}
		}
	}
	return imports
}

// TestSchemaSurfaceCoversCapabilityTaxonomy pins the schema component's
// declared surface: it must be package-surface over both the generated
// protobuf package and the schema vocabulary package, so the
// schema.KnownCapabilities references in the manifest model and the artifact
// shell are authorized by the schema dependency's declared surface rather
// than by an undeclared member symbol.
func TestSchemaSurfaceCoversCapabilityTaxonomy(t *testing.T) {
	manifests := checkedInComponentManifests(t)
	m, ok := manifests["schema"]
	if !ok {
		t.Fatal("checked-in manifest for component schema not found")
	}
	if m.InterfaceStyle != manifest.InterfaceStylePackageSurface {
		t.Fatalf("schema component interface style = %v, want PACKAGE_SURFACE so the vocabulary package is part of its declared surface", m.InterfaceStyle)
	}
	if len(m.InterfaceFiles) != 0 {
		t.Errorf("schema package-surface component must not declare interface files, got %v", m.InterfaceFiles)
	}
	wantMembers := map[string]bool{
		modulePrefix + "internal/schema":     true,
		modulePrefix + "internal/schema/gen": true,
	}
	gotMembers := map[string]bool{}
	for _, member := range m.Members {
		gotMembers[member] = true
	}
	for member := range wantMembers {
		if !gotMembers[member] {
			t.Errorf("schema component members %v are missing the declared member %q", m.Members, member)
		}
	}

	// Both consumers of the taxonomy must reach it through the schema
	// dependency, not through membership in another component.
	for _, consumer := range []string{"manifest", "artifactio"} {
		cm, ok := manifests[consumer]
		if !ok {
			t.Fatalf("checked-in manifest for component %q not found", consumer)
		}
		declared := false
		for _, dep := range cm.ComponentDependencies {
			if dep.Name == "schema" {
				declared = true
			}
		}
		if !declared {
			t.Errorf("%s consumes the capability taxonomy via schema.KnownCapabilities but declares no schema component dependency", consumer)
		}
	}
}

// purePackages are the pure-core packages that must never import the
// artifact-I/O shell (which holds filesystem and runtime authority). The list
// mirrors the design's pure-core inventory for this slice.
var purePackages = []string{
	"internal/capanalyzer",
	"internal/checker",
	"internal/facts",
	"internal/hostpolicy",
	"internal/manifest",
	"internal/schema/gen",
	"internal/report",
	"internal/symbol",
}

// TestPurePackagesDoNotImportArtifactIO keeps the layering inward-only: no
// pure package may import the artifact-I/O shell component or gain filesystem
// authority through it. Test files are excluded; only production imports bind
// a package to the shell.
func TestPurePackagesDoNotImportArtifactIO(t *testing.T) {
	const forbidden = modulePrefix + "internal/artifactio"

	for _, pkg := range purePackages {
		dir := filepath.Join("..", "..", filepath.FromSlash(pkg))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parsing %s/%s: %v", pkg, name, err)
			}
			for _, imp := range f.Imports {
				if path := strings.Trim(imp.Path.Value, `"`); path == forbidden {
					t.Errorf("%s/%s imports the shell package %q; pure packages must not depend on artifactio", pkg, name, forbidden)
				}
			}
		}
	}
}
