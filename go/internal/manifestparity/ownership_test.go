package manifestparity_test

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifestparity"
)

// checkedInManifestInventory walks the repository production roots recursively.
// The inventory helper owns the explicit testdata exclusion and duplicate-name
// failure; keeping this path-bearing form available lets ownership diagnostics
// name the actual manifest that made a bad claim.
func checkedInManifestInventory(t *testing.T) manifestparity.ManifestInventory {
	t.Helper()

	root := filepath.Join("..", "..", "..")
	inventory, err := manifestparity.DiscoverProductionManifests(root)
	if err != nil {
		t.Fatalf("discovering checked-in production manifests: %v", err)
	}
	return inventory
}

// checkedInComponentManifests is the compatibility view used by the older
// ownership tests. It is backed by the complete recursive inventory rather
// than a shallow directory scan.
func checkedInComponentManifests(t *testing.T) map[string]manifest.Manifest {
	t.Helper()

	inventory := checkedInManifestInventory(t)
	if err := manifestparity.AuditForeignManifestOwnership(inventory); err != nil {
		t.Fatalf("checked-in foreign ownership audit: %v", err)
	}
	return inventory.ByName()
}

func checkedInBazelComponents(t *testing.T) manifestparity.BazelInventory {
	t.Helper()

	root := filepath.Join("..", "..", "..")
	inventory, err := manifestparity.DiscoverProductionBazelComponents(root)
	if err != nil {
		t.Fatalf("discovering production Bazel components: %v", err)
	}
	if err := manifestparity.AuditForeignBazelOwnership(inventory); err != nil {
		t.Fatalf("checked-in Bazel foreign ownership audit: %v", err)
	}
	return inventory
}

func bazelComponent(t *testing.T, inventory manifestparity.BazelInventory, name string) manifestparity.BazelComponent {
	t.Helper()
	for _, component := range inventory.Components {
		if component.Name == name {
			return component
		}
	}
	t.Fatalf("production go_component %q not found", name)
	return manifestparity.BazelComponent{}
}

func bazelMemberImportPaths(t *testing.T, component manifestparity.BazelComponent) []string {
	t.Helper()
	members := make([]string, len(component.Members))
	for i, member := range component.Members {
		if member.ImportPath == "" {
			t.Fatalf("go_component %q member label %q in %q has no resolved import path", component.Name, member.Label, component.BuildPath)
		}
		members[i] = member.ImportPath
	}
	return members
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
	slice := []string{"artifactio", "manifest", "schema", "stdlibauthority", "symbol"}

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
		modulePrefix + "internal/artifactio":      "artifactio",
		modulePrefix + "internal/manifest":        "manifest",
		modulePrefix + "internal/schema":          "schema",
		modulePrefix + "internal/schema/gen":      "schema",
		modulePrefix + "internal/symbol":          "symbol",
		modulePrefix + "internal/stdlibauthority": "stdlibauthority",
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
		"report":           "../report/component.textproto",
		"schema":           "../schema/component.textproto",
		"stdlibauthority":  "../stdlibauthority/component.textproto",
		"surface":          "../surface/component.textproto",
		"symbol":           "../symbol/component.textproto",
		"protobuf-runtime": "../protobufruntime/component.textproto",
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
	for _, name := range []string{"artifactio", "report", "schema", "stdlibauthority", "surface", "symbol"} {
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

// TestProtobufRuntimeHasOneConcreteOwnerAndBuildParity audits the adopted
// foreign boundary introduced after surface consumption. The checked-in
// manifest is the native import-path spelling; the BUILD target is the Bazel
// label spelling. They must remain the same concrete, sorted package set so a
// consumer cannot resolve one closure while the build graph owns another.
func TestProtobufRuntimeHasOneConcreteOwnerAndBuildParity(t *testing.T) {
	manifests := checkedInComponentManifests(t)
	bazelComponents := checkedInBazelComponents(t)
	wrapper, ok := manifests["protobuf-runtime"]
	if !ok {
		t.Fatal("checked-in manifest for component protobuf-runtime not found")
	}
	if wrapper.InterfaceStyle != manifest.InterfaceStylePackageSurface {
		t.Fatalf("protobuf-runtime interface style = %v, want PACKAGE_SURFACE", wrapper.InterfaceStyle)
	}
	if wrapper.Authority.Known || len(wrapper.DeclaredAuthority) != 0 {
		t.Fatalf("protobuf-runtime authority = %+v/%v, want UNKNOWN with no declared authority", wrapper.Authority, wrapper.DeclaredAuthority)
	}
	if !slices.IsSorted(wrapper.Members) {
		t.Fatalf("protobuf-runtime manifest members are not sorted: %v", wrapper.Members)
	}
	if hasDuplicate(wrapper.Members) {
		t.Fatalf("protobuf-runtime manifest members contain duplicates: %v", wrapper.Members)
	}

	bazelWrapper := bazelComponent(t, bazelComponents, "protobuf-runtime")
	if bazelWrapper.InterfaceStyle != "PACKAGE_SURFACE" {
		t.Fatalf("protobuf-runtime Bazel interface style = %q, want PACKAGE_SURFACE", bazelWrapper.InterfaceStyle)
	}
	buildMembers := bazelMemberImportPaths(t, bazelWrapper)
	if !slices.IsSorted(buildMembers) || hasDuplicate(buildMembers) {
		t.Fatalf("protobuf-runtime BUILD members are not sorted and unique: %v", buildMembers)
	}
	if !slices.Equal(buildMembers, wrapper.Members) {
		t.Fatalf("protobuf-runtime BUILD members = %v, want checked-in manifest members %v", buildMembers, wrapper.Members)
	}
}

// TestProtobufRuntimeConsumersDeclareTheSharedBoundary keeps the repository
// audit explicit for direct and foreign-closure consumers. The capslockadapter
// component is native-only under the cgo rule, but its manifest still needs the
// same edge as the Bazel-capable schema/manifest/artifactio consumers.
func TestProtobufRuntimeConsumersDeclareTheSharedBoundary(t *testing.T) {
	manifests := checkedInComponentManifests(t)
	for _, name := range []string{"schema", "manifest", "artifactio", "capslockadapter"} {
		component, ok := manifests[name]
		if !ok {
			t.Fatalf("checked-in manifest for component %q not found", name)
		}
		found := false
		for _, dep := range component.ComponentDependencies {
			if dep.Name == "protobuf-runtime" && dep.Manifest == "../protobufruntime/component.textproto" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("component %q does not declare the shared protobuf-runtime boundary", name)
		}
	}
}

// TestProtobufRuntimeNativeSurfaceIsAsserted checks the pinned native
// convention artifact. It deliberately has no digest, symbols, or report
// verdict: native staging must consume this as an UNKNOWN package-level
// assertion, never as a checked claim derived from foreign source.
func TestProtobufRuntimeNativeSurfaceIsAsserted(t *testing.T) {
	const surfacePath = "../protobufruntime/component.surface.json"
	data, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("reading %s: %v", surfacePath, err)
	}
	var got struct {
		FormatVersion  int    `json:"formatVersion"`
		Component      string `json:"component"`
		InterfaceStyle string `json:"interfaceStyle"`
		Authority      struct {
			Authority string `json:"authority"`
		} `json:"authority"`
		Packages  []string `json:"packages"`
		Symbols   []string `json:"symbols"`
		Namespace string   `json:"namespace"`
		SDKKey    struct {
			ToolchainVersion string `json:"toolchainVersion"`
			GOOS             string `json:"goos"`
			GOARCH           string `json:"goarch"`
			ClassifierHash   string `json:"classifierHash"`
			MapFormatVersion int    `json:"mapFormatVersion"`
		} `json:"sdkKey"`
		ProducerVersion string `json:"producerVersion"`
		Digest          string `json:"digest"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode %s: %v", surfacePath, err)
	}
	wrapper, err := manifest.Parse(bytes.NewReader(mustRead(t, filepath.Join("..", "protobufruntime", "component.textproto"))))
	if err != nil {
		t.Fatalf("parse protobuf-runtime manifest: %v", err)
	}
	if got.FormatVersion != 1 || got.Component != "protobuf-runtime" || got.InterfaceStyle != "INTERFACE_STYLE_PACKAGE_SURFACE" {
		t.Fatalf("native protobuf surface identity = %+v", got)
	}
	if got.Authority.Authority != "UNKNOWN" || len(got.Symbols) != 0 || got.Digest != "" {
		t.Fatalf("native protobuf surface is not an empty-digest UNKNOWN assertion: %+v", got)
	}
	if !slices.Equal(got.Packages, wrapper.Members) || !slices.IsSorted(got.Packages) {
		t.Fatalf("native protobuf surface packages = %v, want sorted manifest members %v", got.Packages, wrapper.Members)
	}
	if got.Namespace != "upstream" || got.SDKKey.ToolchainVersion != "go1.26.4" || got.SDKKey.GOOS != "linux" || got.SDKKey.GOARCH != "amd64" || got.SDKKey.ClassifierHash == "" || got.SDKKey.MapFormatVersion != 1 || got.ProducerVersion == "" {
		t.Fatalf("native protobuf surface target identity = %+v", got)
	}
	mapData, err := os.ReadFile(filepath.Join("..", "teststdlibmap", "testdata", "linux_amd64.stdlib-map.json"))
	if err != nil {
		t.Fatalf("read pinned stdlib map: %v", err)
	}
	var pinned struct {
		Key struct {
			ToolchainVersion string `json:"toolchainVersion"`
			GOOS             string `json:"goos"`
			GOARCH           string `json:"goarch"`
			ClassifierHash   string `json:"classifierHash"`
			MapFormatVersion int    `json:"mapFormatVersion"`
		} `json:"key"`
	}
	if err := json.Unmarshal(mapData, &pinned); err != nil {
		t.Fatalf("decode pinned stdlib map: %v", err)
	}
	if got.SDKKey.ToolchainVersion != pinned.Key.ToolchainVersion || got.SDKKey.GOOS != pinned.Key.GOOS || got.SDKKey.GOARCH != pinned.Key.GOARCH || got.SDKKey.ClassifierHash != pinned.Key.ClassifierHash || got.SDKKey.MapFormatVersion != pinned.Key.MapFormatVersion {
		t.Fatalf("native protobuf surface SDK key = %+v, want pinned map key %+v", got.SDKKey, pinned.Key)
	}
}

// TestPackageLayoutHasOneBoundaryAndAcyclicConsumers pins the Step 7
// packagelayout migration: its implementation is owned by one package-surface
// component, while both shell consumers reach it through that explicit edge.
// The complete checked-in component graph is walked as a regression against
// accidentally introducing a cycle through the new shared boundary.
func TestPackageLayoutHasOneBoundaryAndAcyclicConsumers(t *testing.T) {
	manifests := checkedInComponentManifests(t)
	const packageLayout = modulePrefix + "internal/packagelayout"

	owners := []string{}
	for name, component := range manifests {
		for _, member := range component.Members {
			if member == packageLayout {
				owners = append(owners, name)
			}
		}
	}
	slices.Sort(owners)
	if !slices.Equal(owners, []string{"packagelayout"}) {
		t.Fatalf("packagelayout owners = %v, want exactly [packagelayout]", owners)
	}

	layout, ok := manifests["packagelayout"]
	if !ok {
		t.Fatal("checked-in manifest for component packagelayout not found")
	}
	if layout.InterfaceStyle != manifest.InterfaceStylePackageSurface {
		t.Fatalf("packagelayout interface style = %v, want PACKAGE_SURFACE", layout.InterfaceStyle)
	}
	if len(layout.InterfaceFiles) != 0 || !slices.Equal(layout.Members, []string{packageLayout}) {
		t.Fatalf("packagelayout boundary shape = interface_files %v, members %v", layout.InterfaceFiles, layout.Members)
	}
	if !hasDependency(layout, "x-tools", "../xtools/component.textproto") {
		t.Fatal("packagelayout must consume the shared x-tools boundary")
	}

	for _, consumer := range []string{"goanalysis", "stdlibmap"} {
		component, ok := manifests[consumer]
		if !ok {
			t.Fatalf("checked-in manifest for component %q not found", consumer)
		}
		if hasMember(component, packageLayout) {
			t.Fatalf("component %q duplicates packagelayout ownership", consumer)
		}
		if !hasDependency(component, "packagelayout", "../packagelayout/component.textproto") {
			t.Errorf("component %q does not declare the packagelayout boundary", consumer)
		}
		imports := projectImports(t, filepath.Join("..", "..", "internal", consumer))
		if !imports[packageLayout] {
			t.Errorf("component %q production code no longer exposes its packagelayout import for boundary coverage", consumer)
		}
	}

	state := map[string]uint8{}
	var visit func(string)
	visit = func(name string) {
		switch state[name] {
		case 2:
			return
		case 1:
			t.Fatalf("component dependency graph contains a cycle at %q", name)
		}
		state[name] = 1
		component, ok := manifests[name]
		if !ok {
			t.Fatalf("component dependency names unknown component %q", name)
		}
		for _, dep := range component.ComponentDependencies {
			if _, ok := manifests[dep.Name]; !ok {
				t.Fatalf("component %q depends on unknown component %q", name, dep.Name)
			}
			visit(dep.Name)
		}
		state[name] = 2
	}
	for name := range manifests {
		visit(name)
	}
}

func hasMember(component manifest.Manifest, member string) bool {
	return slices.Contains(component.Members, member)
}

func hasDependency(component manifest.Manifest, name, manifestPath string) bool {
	for _, dep := range component.ComponentDependencies {
		if dep.Name == name && dep.Manifest == manifestPath {
			return true
		}
	}
	return false
}

func hasDuplicate(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i] == values[i-1] {
			return true
		}
	}
	return false
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
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
	"internal/stdlibauthority",
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
