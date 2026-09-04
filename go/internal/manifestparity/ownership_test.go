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
// dependency edges: the artifactio component must consume the schema surface
// (generated types and the shared capability taxonomy) and the symbol grammar
// through declared component dependencies rather than duplicated membership.
// These are exactly the core components artifactio imports; it uses no
// manifest-package API (the capability taxonomy it validates against is
// schema.KnownCapabilities, owned by the schema component), so a manifest
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
