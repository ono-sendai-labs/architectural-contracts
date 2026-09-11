package packagelayout

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	ordinaryImportDataFormatVersion = 1
	// ordinaryImportDataLogicalPath is serialized in generated layouts so
	// equivalent component graphs remain byte-identical even when their output
	// files have different target names. The driver resolves it to the sibling
	// declared output using the layout artifact path.
	ordinaryImportDataLogicalPath = "__ARCC_ORDINARY_IMPORT_DATA__"
)

// OrdinaryImportData points at the build-time source-import projection for a
// component's ordinary packages. It is kept separate from the package records
// because Bazel analysis can name source Files but cannot read their contents;
// the graph action produces this descriptor before the check action runs.
type OrdinaryImportData struct {
	Metadata string `json:"metadata"`
}

// ImportGraphRequest is the input to WriteImportGraph. File paths are
// execroot-relative paths in a Bazel action (and may be absolute for a native
// caller). The request is deliberately source-only: it records no compiler or
// toolchain executable and is safe to process with arcc's own binary.
type ImportGraphRequest struct {
	Platform *Platform                 `json:"platform,omitempty"`
	Packages []ImportGraphPackageInput `json:"packages"`
}

// ImportGraphPackageInput identifies the source files belonging to one
// ordinary package for the target configuration.
type ImportGraphPackageInput struct {
	ID      string   `json:"id"`
	PkgPath string   `json:"pkg_path"`
	Files   []string `json:"files"`
}

// ImportGraph is the canonical output of WriteImportGraph. Its imports are
// exact direct source imports, rather than the broader set of declared
// rules_go archive dependencies.
type ImportGraph struct {
	FormatVersion int                  `json:"format_version"`
	Platform      *Platform            `json:"platform,omitempty"`
	Packages      []ImportGraphPackage `json:"packages"`
	Errors        []string             `json:"errors,omitempty"`
}

// ImportGraphPackage carries one package's exact direct import paths.
type ImportGraphPackage struct {
	ID      string   `json:"id"`
	PkgPath string   `json:"pkg_path"`
	Imports []string `json:"imports"`
}

// cloneOrdinaryImportData validates and copies the layout descriptor metadata.
func cloneOrdinaryImportData(data *OrdinaryImportData) (*OrdinaryImportData, error) {
	if data == nil {
		return nil, nil
	}
	if data.Metadata == "" {
		return nil, errors.New("ordinary import metadata path is required")
	}
	if err := validateExportFilePath(data.Metadata); err != nil {
		return nil, fmt.Errorf("unsafe ordinary import metadata path %q: %w", data.Metadata, err)
	}
	return &OrdinaryImportData{Metadata: data.Metadata}, nil
}

// withLayoutComponent adds the owning component to every layout validation
// diagnostic. Generated component layouts carry this identity; legacy
// hand-written layouts retain their previous messages when it is absent.
func withLayoutComponent(l *Layout, err error) error {
	if err == nil || l == nil || l.Component == "" {
		return err
	}
	return fmt.Errorf("component %q: %w", l.Component, err)
}

func attachLayoutPathContext(l *Layout, layoutPath, workspaceDir string) error {
	if l == nil {
		return nil
	}
	if l.Component == "" {
		l.Component = componentNameFromLayoutPath(layoutPath)
	}
	if l.OrdinaryImportData == nil || l.OrdinaryImportData.Metadata != ordinaryImportDataLogicalPath {
		return nil
	}
	metadataPath, err := ordinaryImportMetadataSibling(layoutPath, workspaceDir)
	if err != nil {
		return err
	}
	l.OrdinaryImportData = &OrdinaryImportData{Metadata: metadataPath}
	return nil
}

func componentNameFromLayoutPath(layoutPath string) string {
	const suffix = ".package-layout.json"
	base := filepath.Base(layoutPath)
	return strings.TrimSuffix(base, suffix)
}

func ordinaryImportMetadataSibling(layoutPath, workspaceDir string) (string, error) {
	const layoutSuffix = ".package-layout.json"
	const graphSuffix = ".package-imports.json"
	base := filepath.Base(layoutPath)
	if !strings.HasSuffix(base, layoutSuffix) {
		return "", fmt.Errorf("ordinary import metadata token requires a layout path ending in %q, got %q", layoutSuffix, layoutPath)
	}
	graphPath := filepath.Join(filepath.Dir(layoutPath), strings.TrimSuffix(base, layoutSuffix)+graphSuffix)
	if workspaceDir == "" {
		if filepath.IsAbs(graphPath) {
			return "", fmt.Errorf("cannot resolve ordinary import metadata sibling %q without a workspace directory", graphPath)
		}
		return filepath.ToSlash(graphPath), nil
	}
	rel, err := filepath.Rel(workspaceDir, graphPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve ordinary import metadata sibling: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("ordinary import metadata sibling %q escapes workspace %q", graphPath, workspaceDir)
	}
	return filepath.ToSlash(rel), nil
}

// ResolveOrdinaryImportData reads and applies the generated exact source-import
// graph. It runs after the stdlib descriptor has been merged so every import
// path can be resolved to the final package identity before validation or
// packages.Load.
func ResolveOrdinaryImportData(l *Layout, workspaceDir string) error {
	if l == nil || l.OrdinaryImportData == nil || l.ordinaryImportDataResolved {
		return nil
	}

	descriptor, err := cloneOrdinaryImportData(l.OrdinaryImportData)
	if err != nil {
		return err
	}
	metadataPath := filepath.FromSlash(descriptor.Metadata)
	if workspaceDir != "" {
		metadataPath = filepath.Join(workspaceDir, metadataPath)
	}
	info, err := os.Stat(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("ordinary import metadata %q does not exist: %w", metadataPath, err)
		}
		return fmt.Errorf("error stating ordinary import metadata %q: %w", metadataPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("ordinary import metadata %q is a directory", metadataPath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("ordinary import metadata %q is not a regular file", metadataPath)
	}
	if info.Size() == 0 {
		return fmt.Errorf("ordinary import metadata %q is empty", metadataPath)
	}

	graph, err := readImportGraph(metadataPath)
	if err != nil {
		return fmt.Errorf("reading ordinary import metadata %q: %w", metadataPath, err)
	}
	if len(graph.Errors) > 0 {
		return fmt.Errorf("ordinary import graph source projection failed: %s", strings.Join(graph.Errors, "; "))
	}
	if err := validateImportGraphPlatform(l.Platform, graph.Platform); err != nil {
		return err
	}

	index, err := indexLayoutPackages(l)
	if err != nil {
		return err
	}
	rootPackages, _, err := layoutRootPackages(l.Roots, index)
	if err != nil {
		return err
	}
	rootIDs := make(map[string]bool, len(rootPackages))
	for _, pkg := range rootPackages {
		rootIDs[pkg.ID] = true
	}

	byID := make(map[string]ImportGraphPackage, len(graph.Packages))
	byPath := make(map[string]ImportGraphPackage, len(graph.Packages))
	pending := make(map[string]map[string]*packages.Package, len(graph.Packages))
	for _, record := range graph.Packages {
		if record.ID == "" {
			return errors.New("ordinary import metadata has a package with empty ID")
		}
		if record.PkgPath == "" {
			return fmt.Errorf("ordinary import metadata package %q has empty import path", record.ID)
		}
		if _, exists := byID[record.ID]; exists {
			return fmt.Errorf("duplicate ordinary import metadata package ID %q", record.ID)
		}
		if _, exists := byPath[record.PkgPath]; exists {
			return fmt.Errorf("duplicate ordinary import metadata package import path %q", record.PkgPath)
		}
		byID[record.ID] = record
		byPath[record.PkgPath] = record

		target, ok := index.byID[record.ID]
		if !ok {
			return fmt.Errorf("ordinary import metadata package %q is absent from the layout", record.PkgPath)
		}
		if target.PkgPath != record.PkgPath {
			return fmt.Errorf("ordinary import metadata package %q has layout path %q", record.PkgPath, target.PkgPath)
		}
		if l.IsStdlibPackage(target) {
			return fmt.Errorf("ordinary import metadata package %q is marked as standard library", record.PkgPath)
		}

		imports := make(map[string]*packages.Package, len(record.Imports))
		seenImports := make(map[string]bool, len(record.Imports))
		for _, importPath := range record.Imports {
			if importPath == "" {
				return fmt.Errorf("ordinary package %q has an empty import path", record.PkgPath)
			}
			if importPath == "C" {
				return fmt.Errorf("ordinary package %q includes cgo pseudo-import C", record.PkgPath)
			}
			if seenImports[importPath] {
				return fmt.Errorf("ordinary package %q repeats import path %q", record.PkgPath, importPath)
			}
			seenImports[importPath] = true
			imported, ok := index.byPath[importPath]
			if !ok {
				if candidate, vendorOK := index.byPath["vendor/"+importPath]; vendorOK && l.IsStdlibPackage(candidate) {
					imported, ok = candidate, true
				}
			}
			if !ok {
				return fmt.Errorf("ordinary package %q imports path %q but it is absent from the layout", record.PkgPath, importPath)
			}
			imports[importPath] = &packages.Package{ID: imported.ID}
		}
		if !rootIDs[target.ID] {
			pending[target.ID] = imports
		}
	}

	for _, pkg := range l.Packages {
		if rootIDs[pkg.ID] || l.IsStdlibPackage(pkg) {
			continue
		}
		if _, ok := byID[pkg.ID]; !ok {
			return fmt.Errorf("ordinary package %q has no record in ordinary import metadata", pkg.PkgPath)
		}
	}
	for id, imports := range pending {
		pkg := index.byID[id]
		pkg.Imports = imports
		if l.importsOmitted == nil {
			l.importsOmitted = make(map[string]bool)
		}
		l.importsOmitted[id] = false
	}

	l.OrdinaryImportData = descriptor
	l.ordinaryImportDataResolved = true
	return nil
}

func readImportGraph(metadataPath string) (ImportGraph, error) {
	f, err := os.Open(metadataPath)
	if err != nil {
		return ImportGraph{}, err
	}
	defer f.Close()

	var graph ImportGraph
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&graph); err != nil {
		return ImportGraph{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ImportGraph{}, errors.New("ordinary import metadata contains multiple JSON values")
		}
		return ImportGraph{}, fmt.Errorf("decoding trailing JSON: %w", err)
	}
	if graph.FormatVersion != ordinaryImportDataFormatVersion {
		return ImportGraph{}, fmt.Errorf("unsupported ordinary import metadata format version %d", graph.FormatVersion)
	}
	return graph, nil
}

// MergeImportGraphLayout applies the exact ordinary source-import graph to a
// base layout artifact. The action-time merge makes the emitted package-layout
// JSON self-describing for inspection; ResolveOrdinaryImportData repeats the
// identity resolution at the runtime boundary and replaces placeholder IDs for
// target-configured stdlib records.
func MergeImportGraphLayout(baseLayoutPath, graphPath, outputPath string) error {
	baseBytes, err := os.ReadFile(baseLayoutPath)
	if err != nil {
		return fmt.Errorf("reading base package layout %q: %w", baseLayoutPath, err)
	}
	layout, err := Parse(strings.NewReader(string(baseBytes)))
	if err != nil {
		return fmt.Errorf("parsing base package layout %q: %w", baseLayoutPath, err)
	}
	graph, err := readImportGraph(graphPath)
	if err != nil {
		return fmt.Errorf("reading ordinary import graph %q: %w", graphPath, err)
	}
	if err := validateImportGraphPlatform(layout.Platform, graph.Platform); err != nil {
		return err
	}
	if len(graph.Errors) > 0 {
		// Keep the default layout output buildable for a source that is itself
		// malformed. The checked action will read the same descriptor and
		// fail closed with the source-scanning diagnostic before packages.Load.
		if err := os.WriteFile(outputPath, baseBytes, 0o666); err != nil {
			return fmt.Errorf("writing base package layout %q: %w", outputPath, err)
		}
		return nil
	}
	index, err := indexLayoutPackages(layout)
	if err != nil {
		return err
	}
	rootPackages, _, err := layoutRootPackages(layout.Roots, index)
	if err != nil {
		return err
	}
	rootIDs := make(map[string]bool, len(rootPackages))
	for _, pkg := range rootPackages {
		rootIDs[pkg.ID] = true
	}

	seenRecords := make(map[string]bool, len(graph.Packages))
	for _, record := range graph.Packages {
		if record.ID == "" || record.PkgPath == "" {
			return fmt.Errorf("ordinary import graph has a package with incomplete identity: ID=%q PkgPath=%q", record.ID, record.PkgPath)
		}
		if seenRecords[record.ID] {
			return fmt.Errorf("ordinary import graph repeats package ID %q", record.ID)
		}
		seenRecords[record.ID] = true
		pkg, ok := index.byID[record.ID]
		if !ok {
			return fmt.Errorf("ordinary import graph package %q is absent from the base layout", record.PkgPath)
		}
		if pkg.PkgPath != record.PkgPath {
			return fmt.Errorf("ordinary import graph package %q has base layout path %q", record.PkgPath, pkg.PkgPath)
		}
		if rootIDs[pkg.ID] {
			continue
		}
		imports := make(map[string]*packages.Package, len(record.Imports))
		seenImports := make(map[string]bool, len(record.Imports))
		for _, importPath := range record.Imports {
			if importPath == "" {
				return fmt.Errorf("ordinary package %q has an empty import path", record.PkgPath)
			}
			if importPath == "C" {
				return fmt.Errorf("ordinary package %q includes cgo pseudo-import C", record.PkgPath)
			}
			if seenImports[importPath] {
				return fmt.Errorf("ordinary package %q repeats import path %q", record.PkgPath, importPath)
			}
			seenImports[importPath] = true
			// Ordinary package IDs are their import paths. Stdlib package IDs
			// are host-specific and are repaired by the runtime resolver after
			// its descriptor has been materialized, so the emitted artifact uses
			// the path as a deterministic placeholder here.
			imports[importPath] = &packages.Package{ID: importPath}
		}
		pkg.Imports = imports
		if layout.importsOmitted == nil {
			layout.importsOmitted = make(map[string]bool)
		}
		layout.importsOmitted[pkg.ID] = false
	}
	for _, pkg := range layout.Packages {
		if rootIDs[pkg.ID] || layout.IsStdlibPackage(pkg) {
			continue
		}
		if !seenRecords[pkg.ID] {
			return fmt.Errorf("ordinary package %q has no record in ordinary import graph", pkg.PkgPath)
		}
	}

	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding merged package layout: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(outputPath, data, 0o666); err != nil {
		return fmt.Errorf("writing merged package layout %q: %w", outputPath, err)
	}
	return nil
}

func validateImportGraphPlatform(actual, expected *Platform) error {
	if expected == nil {
		if actual != nil {
			return errors.New("ordinary import metadata has a target configuration but the layout has no platform")
		}
		return nil
	}
	if actual == nil {
		return errors.New("ordinary import metadata has a target configuration but the layout has no platform")
	}
	actualTags := append([]string(nil), actual.BuildTags...)
	expectedTags := append([]string(nil), expected.BuildTags...)
	sort.Strings(actualTags)
	sort.Strings(expectedTags)
	actualVersion := ""
	if actual.ToolchainVersion != nil {
		actualVersion = *actual.ToolchainVersion
	}
	expectedVersion := ""
	if expected.ToolchainVersion != nil {
		expectedVersion = *expected.ToolchainVersion
	}
	actualExperiment := ""
	if actual.GOEXPERIMENT != nil {
		actualExperiment = *actual.GOEXPERIMENT
	}
	expectedExperiment := ""
	if expected.GOEXPERIMENT != nil {
		expectedExperiment = *expected.GOEXPERIMENT
	}
	mismatches := make([]string, 0, 6)
	if actualVersion != expectedVersion {
		mismatches = append(mismatches, fmt.Sprintf("toolchain_version=%q", expectedVersion))
	}
	if actual.GOOS != expected.GOOS {
		mismatches = append(mismatches, fmt.Sprintf("goos=%q", expected.GOOS))
	}
	if actual.GOARCH != expected.GOARCH {
		mismatches = append(mismatches, fmt.Sprintf("goarch=%q", expected.GOARCH))
	}
	if actual.CgoEnabled != expected.CgoEnabled {
		mismatches = append(mismatches, fmt.Sprintf("cgo_enabled=%t", expected.CgoEnabled))
	}
	if strings.Join(actualTags, "\x00") != strings.Join(expectedTags, "\x00") {
		mismatches = append(mismatches, fmt.Sprintf("build_tags=%q", strings.Join(expectedTags, ",")))
	}
	if actualExperiment != expectedExperiment {
		mismatches = append(mismatches, fmt.Sprintf("goexperiment=%q", expectedExperiment))
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("ordinary import metadata configuration mismatch in %s", strings.Join(mismatches, ", "))
	}
	return nil
}

// WriteImportGraph parses the selected source files and writes a deterministic
// exact direct-import descriptor. It is intended for the non-analysis Bazel
// graph-projection action and does not invoke a Go toolchain.
func WriteImportGraph(configPath, outputPath string) error {
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading import graph request %q: %w", configPath, err)
	}
	var request ImportGraphRequest
	if err := json.Unmarshal(configBytes, &request); err != nil {
		return fmt.Errorf("decoding import graph request %q: %w", configPath, err)
	}
	bctx, err := BuildContextForLayout(&Layout{Platform: request.Platform})
	if err != nil {
		return fmt.Errorf("building import graph target context: %w", err)
	}
	inputs := append([]ImportGraphPackageInput(nil), request.Packages...)
	slices.SortFunc(inputs, func(a, b ImportGraphPackageInput) int {
		if a.PkgPath != b.PkgPath {
			return strings.Compare(a.PkgPath, b.PkgPath)
		}
		return strings.Compare(a.ID, b.ID)
	})
	graph := ImportGraph{
		FormatVersion: ordinaryImportDataFormatVersion,
		Platform:      clonePlatform(request.Platform),
		Packages:      make([]ImportGraphPackage, 0, len(inputs)),
	}
	seenIDs := make(map[string]bool, len(inputs))
	seenPaths := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		if input.ID == "" || input.PkgPath == "" {
			return fmt.Errorf("import graph request has package with empty identity: ID=%q PkgPath=%q", input.ID, input.PkgPath)
		}
		if seenIDs[input.ID] || seenPaths[input.PkgPath] {
			return fmt.Errorf("import graph request has duplicate package identity %q", input.PkgPath)
		}
		seenIDs[input.ID] = true
		seenPaths[input.PkgPath] = true
		files := append([]string(nil), input.Files...)
		sort.Strings(files)
		imports := make(map[string]bool)
		packageName := ""
		seenFiles := make(map[string]bool, len(files))
		for _, file := range files {
			if file == "" || seenFiles[file] {
				if file == "" {
					return fmt.Errorf("ordinary package %q has an empty source file path", input.PkgPath)
				}
				continue
			}
			seenFiles[file] = true
			if !FileMatchesBuildConstraintsWithContext(file, bctx) {
				continue
			}
			source, err := os.ReadFile(file)
			if err != nil {
				graph.Errors = append(graph.Errors, fmt.Sprintf("ordinary package %q source file %q: %v", input.PkgPath, file, err))
				continue
			}
			name, fileImports, _, err := scanGoSource(file, source)
			if err != nil {
				graph.Errors = append(graph.Errors, fmt.Sprintf("ordinary package %q source file %q: %v", input.PkgPath, file, err))
				continue
			}
			if packageName == "" {
				packageName = name
			} else if packageName != name {
				graph.Errors = append(graph.Errors, fmt.Sprintf("ordinary package %q has source package names %q and %q", input.PkgPath, packageName, name))
				continue
			}
			for _, importPath := range fileImports {
				imports[importPath] = true
			}
		}
		sortedImports := make([]string, 0, len(imports))
		for importPath := range imports {
			sortedImports = append(sortedImports, importPath)
		}
		sort.Strings(sortedImports)
		if sortedImports == nil {
			sortedImports = []string{}
		}
		graph.Packages = append(graph.Packages, ImportGraphPackage{
			ID:      input.ID,
			PkgPath: input.PkgPath,
			Imports: sortedImports,
		})
	}
	sort.Strings(graph.Errors)

	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding import graph: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(outputPath, data, 0o666); err != nil {
		return fmt.Errorf("writing import graph %q: %w", outputPath, err)
	}
	return nil
}

func clonePlatform(platform *Platform) *Platform {
	if platform == nil {
		return nil
	}
	clone := *platform
	clone.BuildTags = append([]string(nil), platform.BuildTags...)
	if clone.BuildTags == nil {
		clone.BuildTags = []string{}
	}
	if platform.ToolchainVersion != nil {
		version := *platform.ToolchainVersion
		clone.ToolchainVersion = &version
	}
	if platform.GOEXPERIMENT != nil {
		experiment := *platform.GOEXPERIMENT
		clone.GOEXPERIMENT = &experiment
	}
	sort.Strings(clone.BuildTags)
	return &clone
}
