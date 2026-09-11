package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

const maxLayoutShapeBytes int64 = 16 << 20

const (
	layoutSDKRootPlaceholder          = "<GO_SDK_ROOT>"
	layoutWorkspacePlaceholder        = "<WORKSPACE_ROOT>/"
	layoutExportPlaceholder           = "<EXPORT_ROOT>/"
	layoutArtifactPlaceholder         = "<ARTIFACT_ROOT>/"
	layoutImportMetadataPlaceholder   = "<IMPORT_METADATA_ROOT>/"
	layoutStdlibMetadataPlaceholder   = "<STDLIB_METADATA_ROOT>/"
	layoutStdlibRunfilesPlaceholder   = "<STDLIB_RUNFILES_ROOT>/"
	layoutStdlibExecPlaceholder       = "<STDLIB_EXEC_ROOT>/"
	layoutTargetGOOSPlaceholder       = "<TARGET_GOOS>"
	layoutTargetGOARCHPlaceholder     = "<TARGET_GOARCH>"
	layoutTargetCgoPlaceholder        = "<TARGET_CGO_ENABLED>"
	layoutTargetTagsPlaceholder       = "<TARGET_BUILD_TAGS>"
	layoutTargetToolchainPlaceholder  = "<TARGET_TOOLCHAIN_VERSION>"
	layoutTargetExperimentPlaceholder = "<TARGET_GOEXPERIMENT>"
)

type layoutShapeMismatchError struct {
	detail string
}

func (e *layoutShapeMismatchError) Error() string {
	return "layout shape mismatch: " + e.detail
}

func isLayoutShapeMismatch(err error) bool {
	_, ok := err.(*layoutShapeMismatchError)
	return ok
}

// snapshotLayoutShape validates a final member-only package layout and emits
// the intentionally small, typed shape contract used by Bazel goldens. Paths
// and target-specific values are replaced only after the complete layout has
// been parsed, canonically re-encoded, and checked for its final export-data
// shape (design §Golden restructure).
func snapshotLayoutShape(data []byte) ([]byte, error) {
	if int64(len(data)) > maxLayoutShapeBytes {
		return nil, fmt.Errorf("package-layout artifact exceeds the %d byte limit", maxLayoutShapeBytes)
	}
	layout, err := packagelayout.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse package-layout artifact: %w", err)
	}
	canonical, err := marshalCanonicalLayout(layout)
	if err != nil {
		return nil, fmt.Errorf("re-encode package-layout artifact: %w", err)
	}
	if !bytes.Equal(data, canonical) {
		return nil, fmt.Errorf("package-layout artifact is not canonical")
	}
	if err := validateLayoutShape(layout); err != nil {
		return nil, fmt.Errorf("validate package-layout shape: %w", err)
	}
	return marshalLayoutShape(layout)
}

func compareLayoutShape(artifact, golden []byte) error {
	actual, err := snapshotLayoutShape(artifact)
	if err != nil {
		return fmt.Errorf("snapshot package-layout artifact: %w", err)
	}
	if int64(len(golden)) > maxLayoutShapeBytes {
		return fmt.Errorf("package-layout shape golden exceeds the %d byte limit", maxLayoutShapeBytes)
	}
	wantData := golden
	var want layoutShapeSnapshot
	if err := decodeOneJSON(wantData, &want); err != nil {
		return fmt.Errorf("parse package-layout shape golden: %w", err)
	}
	canonicalizeLayoutShapePaths(&want)
	wantCanonical, err := marshalLayoutShapeSnapshot(want)
	if err != nil {
		return fmt.Errorf("re-encode package-layout shape golden: %w", err)
	}
	if !bytes.Equal(wantData, wantCanonical) {
		return fmt.Errorf("package-layout shape golden is not canonical")
	}
	if bytes.Equal(actual, wantCanonical) {
		return nil
	}
	return &layoutShapeMismatchError{detail: layoutShapeDiff(wantCanonical, actual)}
}

func decodeOneJSON(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if token, err := decoder.Token(); err == nil {
		return fmt.Errorf("trailing JSON value %v", token)
	} else if err != io.EOF {
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

func marshalCanonicalLayout(layout *packagelayout.Layout) ([]byte, error) {
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

type layoutShapeSnapshot struct {
	GoSDKRoot                  string                     `json:"go_sdk_root"`
	Platform                   layoutTargetShape          `json:"platform"`
	Roots                      []string                   `json:"roots"`
	Packages                   []layoutPackageShape       `json:"packages"`
	OrdinaryImportData         *layoutOrdinaryImportShape `json:"ordinary_import_data,omitempty"`
	StdlibExportData           *layoutStdlibExportShape   `json:"stdlib_export_data,omitempty"`
	DependencyArtifactBindings []layoutDependencyShape    `json:"dependency_artifact_bindings,omitempty"`
}

type layoutPackageShape struct {
	CompiledGoFiles []string          `json:"CompiledGoFiles"`
	ExportFile      string            `json:"ExportFile"`
	GoFiles         []string          `json:"GoFiles"`
	ID              string            `json:"ID"`
	IgnoredFiles    []string          `json:"IgnoredFiles"`
	Imports         map[string]string `json:"Imports"`
	Name            string            `json:"Name"`
	OtherFiles      []string          `json:"OtherFiles"`
	PkgPath         string            `json:"PkgPath"`
	IsStdlib        bool              `json:"is_stdlib"`
}

type layoutOrdinaryImportShape struct {
	Metadata string `json:"metadata"`
}

type layoutStdlibExportShape struct {
	Metadata    string                  `json:"metadata"`
	Target      layoutTargetShape       `json:"target"`
	ExportRoots []layoutExportRootShape `json:"export_roots"`
}

type layoutExportRootShape struct {
	RunfilesPath string `json:"runfiles_path"`
	ExecPath     string `json:"exec_path"`
}

type layoutDependencyShape struct {
	Dependency   string `json:"dependency"`
	Surface      string `json:"surface"`
	Report       string `json:"report"`
	AutoAttached bool   `json:"auto_attached"`
	Provenance   string `json:"provenance"`
}

type layoutTargetShape struct {
	GOOS             any `json:"goos"`
	GOARCH           any `json:"goarch"`
	BuildTags        any `json:"build_tags"`
	CgoEnabled       any `json:"cgo_enabled"`
	ToolchainVersion any `json:"toolchain_version"`
	GOEXPERIMENT     any `json:"goexperiment"`
}

func marshalLayoutShape(layout *packagelayout.Layout) ([]byte, error) {
	shape, err := buildLayoutShape(layout)
	if err != nil {
		return nil, err
	}
	return marshalLayoutShapeSnapshot(shape)
}

func marshalLayoutShapeSnapshot(shape layoutShapeSnapshot) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(shape)
	if err != nil {
		return nil, fmt.Errorf("marshal package-layout shape: %w", err)
	}
	return buffer.Bytes(), nil
}

func buildLayoutShape(layout *packagelayout.Layout) (layoutShapeSnapshot, error) {
	if err := validateLayoutShape(layout); err != nil {
		return layoutShapeSnapshot{}, err
	}

	roots := make([]string, len(layout.Roots))
	for i, root := range layout.Roots {
		roots[i] = hostpolicy.CanonicalizePath(root)
	}
	sort.Strings(roots)

	packages := make([]layoutPackageShape, len(layout.Packages))
	for i, pkg := range layout.Packages {
		imports, err := packagelayout.ShapeImports(pkg.Imports, hostpolicy.CanonicalizePath)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("package %q imports: %w", pkg.ID, err)
		}
		goFiles, err := shapeLayoutPaths(pkg.GoFiles, layoutWorkspacePlaceholder)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("package %q GoFiles: %w", pkg.ID, err)
		}
		compiledGoFiles, err := shapeLayoutPaths(pkg.CompiledGoFiles, layoutWorkspacePlaceholder)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("package %q CompiledGoFiles: %w", pkg.ID, err)
		}
		ignoredFiles, err := shapeLayoutPaths(pkg.IgnoredFiles, layoutWorkspacePlaceholder)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("package %q IgnoredFiles: %w", pkg.ID, err)
		}
		otherFiles, err := shapeLayoutPaths(pkg.OtherFiles, layoutWorkspacePlaceholder)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("package %q OtherFiles: %w", pkg.ID, err)
		}
		exportFile, err := shapeOptionalLayoutPath(pkg.ExportFile, layoutExportPlaceholder)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("package %q ExportFile: %w", pkg.ID, err)
		}
		packages[i] = layoutPackageShape{
			CompiledGoFiles: compiledGoFiles,
			ExportFile:      exportFile,
			GoFiles:         goFiles,
			ID:              hostpolicy.CanonicalizePath(pkg.ID),
			IgnoredFiles:    ignoredFiles,
			Imports:         imports,
			Name:            pkg.Name,
			OtherFiles:      otherFiles,
			PkgPath:         hostpolicy.CanonicalizePath(pkg.PkgPath),
			IsStdlib:        layout.IsStdlibPackage(pkg),
		}
	}
	slices.SortFunc(packages, func(a, b layoutPackageShape) int {
		return strings.Compare(a.ID, b.ID)
	})

	ordinaryImportData := (*layoutOrdinaryImportShape)(nil)
	if layout.OrdinaryImportData != nil {
		metadata, err := shapeMetadataPath(layout.OrdinaryImportData.Metadata, layoutImportMetadataPlaceholder, true)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("ordinary import metadata: %w", err)
		}
		ordinaryImportData = &layoutOrdinaryImportShape{Metadata: metadata}
	}

	stdlibExportData := (*layoutStdlibExportShape)(nil)
	if layout.StdlibExportData != nil {
		metadata, err := shapeMetadataPath(layout.StdlibExportData.Metadata, layoutStdlibMetadataPlaceholder, false)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("stdlib export metadata: %w", err)
		}
		exportRoots := make([]layoutExportRootShape, len(layout.StdlibExportData.ExportRoots))
		for i, root := range layout.StdlibExportData.ExportRoots {
			runfilesPath, err := shapeRequiredLayoutPath(root.RunfilesPath, layoutStdlibRunfilesPlaceholder)
			if err != nil {
				return layoutShapeSnapshot{}, fmt.Errorf("stdlib export root %d runfiles path: %w", i, err)
			}
			execPath, err := shapeRequiredLayoutPath(root.ExecPath, layoutStdlibExecPlaceholder)
			if err != nil {
				return layoutShapeSnapshot{}, fmt.Errorf("stdlib export root %d exec path: %w", i, err)
			}
			exportRoots[i] = layoutExportRootShape{RunfilesPath: runfilesPath, ExecPath: execPath}
		}
		slices.SortFunc(exportRoots, func(a, b layoutExportRootShape) int {
			if c := strings.Compare(a.ExecPath, b.ExecPath); c != 0 {
				return c
			}
			return strings.Compare(a.RunfilesPath, b.RunfilesPath)
		})
		stdlibExportData = &layoutStdlibExportShape{
			Metadata:    metadata,
			Target:      shapeStdlibExportTarget(layout.StdlibExportData.Target),
			ExportRoots: exportRoots,
		}
	}

	bindings := make([]layoutDependencyShape, len(layout.DependencyArtifactBindings))
	for i, binding := range layout.DependencyArtifactBindings {
		surface, err := shapeRequiredLayoutPath(binding.Surface, layoutArtifactPlaceholder)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("dependency %q surface: %w", binding.Dependency, err)
		}
		report, err := shapeOptionalLayoutPath(binding.Report, layoutArtifactPlaceholder)
		if err != nil {
			return layoutShapeSnapshot{}, fmt.Errorf("dependency %q report: %w", binding.Dependency, err)
		}
		bindings[i] = layoutDependencyShape{
			Dependency:   binding.Dependency,
			Surface:      surface,
			Report:       report,
			AutoAttached: binding.AutoAttached,
			Provenance:   string(binding.Provenance),
		}
	}
	slices.SortFunc(bindings, func(a, b layoutDependencyShape) int {
		return strings.Compare(a.Dependency, b.Dependency)
	})

	return layoutShapeSnapshot{
		GoSDKRoot:                  layoutSDKRootPlaceholder,
		Platform:                   shapeLayoutTarget(layout.Platform),
		Roots:                      roots,
		Packages:                   packages,
		OrdinaryImportData:         ordinaryImportData,
		StdlibExportData:           stdlibExportData,
		DependencyArtifactBindings: bindings,
	}, nil
}

func shapeLayoutTarget(platform *packagelayout.Platform) layoutTargetShape {
	if platform == nil {
		return layoutTargetShape{}
	}
	goexperiment := ""
	if platform.GOEXPERIMENT != nil {
		goexperiment = *platform.GOEXPERIMENT
	}
	toolchain := ""
	if platform.ToolchainVersion != nil {
		toolchain = *platform.ToolchainVersion
	}
	return shapeLayoutTargetValues(platform.GOOS, platform.GOARCH, platform.BuildTags, platform.CgoEnabled, toolchain, goexperiment)
}

func shapeStdlibExportTarget(target *packagelayout.StdlibExportTarget) layoutTargetShape {
	if target == nil {
		return layoutTargetShape{}
	}
	return shapeLayoutTargetValues(target.GOOS, target.GOARCH, target.BuildTags, target.CgoEnabled, target.ToolchainVersion, target.GOEXPERIMENT)
}

func shapeLayoutTargetValues(_ string, _ string, buildTagValues []string, _ bool, toolchainValue, experimentValue string) layoutTargetShape {
	buildTags := any([]string{})
	if len(buildTagValues) > 0 {
		buildTags = []string{layoutTargetTagsPlaceholder}
	}
	goexperiment := any("")
	if experimentValue != "" {
		goexperiment = layoutTargetExperimentPlaceholder
	}
	toolchain := any(layoutTargetToolchainPlaceholder)
	if toolchainValue == "" {
		toolchain = ""
	}
	return layoutTargetShape{
		GOOS:             layoutTargetGOOSPlaceholder,
		GOARCH:           layoutTargetGOARCHPlaceholder,
		BuildTags:        buildTags,
		CgoEnabled:       layoutTargetCgoPlaceholder,
		ToolchainVersion: toolchain,
		GOEXPERIMENT:     goexperiment,
	}
}

func shapeLayoutPaths(values []string, placeholder string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	shaped := make([]string, len(values))
	for i, value := range values {
		var err error
		shaped[i], err = shapeRequiredLayoutPath(value, placeholder)
		if err != nil {
			return nil, fmt.Errorf("file %d: %w", i, err)
		}
	}
	return shaped, nil
}

func shapeRequiredLayoutPath(value, placeholder string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("path is empty")
	}
	if err := validateLayoutPath(value); err != nil {
		return "", err
	}
	return placeholder + path.Base(value), nil
}

func shapeOptionalLayoutPath(value, placeholder string) (string, error) {
	if value == "" {
		return "", nil
	}
	return shapeRequiredLayoutPath(value, placeholder)
}

func shapeMetadataPath(value, placeholder string, allowLogicalToken bool) (string, error) {
	if allowLogicalToken && value == "__ARCC_ORDINARY_IMPORT_DATA__" {
		return value, nil
	}
	return shapeRequiredLayoutPath(value, placeholder)
}

func validateLayoutPath(value string) error {
	if strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || hasWindowsVolumePrefix(value) {
		return fmt.Errorf("path %q is absolute or uses a host-specific separator", value)
	}
	if value == "." || value == ".." || strings.HasPrefix(value, "../") || path.Clean(value) != value {
		return fmt.Errorf("path %q is not a normalized relative path", value)
	}
	return nil
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func validateLayoutShape(layout *packagelayout.Layout) error {
	if layout == nil {
		return fmt.Errorf("layout is nil")
	}
	if layout.GoSDKRoot == "" || !strings.HasSuffix(layout.GoSDKRoot, "/src") {
		return fmt.Errorf("go_sdk_root %q must be a relative SDK path ending in /src", layout.GoSDKRoot)
	}
	if err := validateLayoutPath(layout.GoSDKRoot); err != nil {
		return fmt.Errorf("go_sdk_root: %w", err)
	}
	if layout.Platform == nil {
		return fmt.Errorf("platform is required for final layout shape")
	}
	if layout.Platform.GOOS == "" || layout.Platform.GOARCH == "" {
		return fmt.Errorf("platform requires goos and goarch")
	}
	if layout.Platform.ToolchainVersion != nil && *layout.Platform.ToolchainVersion == "" {
		return fmt.Errorf("platform.toolchain_version must not be empty when present")
	}
	if layout.OrdinaryImportData == nil {
		return fmt.Errorf("ordinary_import_data is required for final layout shape")
	}
	if _, err := shapeMetadataPath(layout.OrdinaryImportData.Metadata, layoutImportMetadataPlaceholder, true); err != nil {
		return fmt.Errorf("ordinary_import_data.metadata: %w", err)
	}
	if layout.StdlibExportData == nil || layout.StdlibExportData.Target == nil {
		return fmt.Errorf("stdlib_export_data and its target are required for final layout shape")
	}
	if _, err := shapeMetadataPath(layout.StdlibExportData.Metadata, layoutStdlibMetadataPlaceholder, false); err != nil {
		return fmt.Errorf("stdlib_export_data.metadata: %w", err)
	}
	if len(layout.StdlibExportData.ExportRoots) == 0 {
		return fmt.Errorf("stdlib_export_data.export_roots is empty")
	}
	if err := validateStdlibTargetMatchesPlatform(layout.Platform, layout.StdlibExportData.Target); err != nil {
		return err
	}
	for i, root := range layout.StdlibExportData.ExportRoots {
		if _, err := shapeRequiredLayoutPath(root.RunfilesPath, layoutStdlibRunfilesPlaceholder); err != nil {
			return fmt.Errorf("stdlib_export_data.export_roots[%d].runfiles_path: %w", i, err)
		}
		if _, err := shapeRequiredLayoutPath(root.ExecPath, layoutStdlibExecPlaceholder); err != nil {
			return fmt.Errorf("stdlib_export_data.export_roots[%d].exec_path: %w", i, err)
		}
	}

	if len(layout.Roots) == 0 || layout.Roots == nil {
		return fmt.Errorf("roots is empty")
	}
	roots := make(map[string]bool, len(layout.Roots))
	for _, root := range layout.Roots {
		if root == "" {
			return fmt.Errorf("roots contains an empty package path")
		}
		canonical := hostpolicy.CanonicalizePath(root)
		if roots[canonical] {
			return fmt.Errorf("roots contains duplicate package path %q after canonicalization", canonical)
		}
		roots[canonical] = true
	}
	if len(layout.Packages) == 0 {
		return fmt.Errorf("packages is empty")
	}
	seenIDs := make(map[string]bool, len(layout.Packages))
	seenPaths := make(map[string]bool, len(layout.Packages))
	for _, pkg := range layout.Packages {
		if pkg == nil {
			return fmt.Errorf("packages contains a null package")
		}
		if pkg.ID == "" || pkg.PkgPath == "" || pkg.Name == "" {
			return fmt.Errorf("package %q must have ID, PkgPath, and Name", pkg.ID)
		}
		canonicalID := hostpolicy.CanonicalizePath(pkg.ID)
		canonicalPath := hostpolicy.CanonicalizePath(pkg.PkgPath)
		if seenIDs[canonicalID] {
			return fmt.Errorf("duplicate package ID %q after canonicalization", canonicalID)
		}
		if seenPaths[canonicalPath] {
			return fmt.Errorf("duplicate package import path %q after canonicalization", canonicalPath)
		}
		seenIDs[canonicalID] = true
		seenPaths[canonicalPath] = true
		for name, files := range map[string][]string{
			"GoFiles":         pkg.GoFiles,
			"CompiledGoFiles": pkg.CompiledGoFiles,
			"IgnoredFiles":    pkg.IgnoredFiles,
			"OtherFiles":      pkg.OtherFiles,
		} {
			for i, file := range files {
				if _, err := shapeRequiredLayoutPath(file, layoutWorkspacePlaceholder); err != nil {
					return fmt.Errorf("package %q %s[%d]: %w", pkg.ID, name, i, err)
				}
			}
		}
		if pkg.ExportFile != "" {
			if _, err := shapeRequiredLayoutPath(pkg.ExportFile, layoutExportPlaceholder); err != nil {
				return fmt.Errorf("package %q ExportFile: %w", pkg.ID, err)
			}
		}
		if !roots[canonicalPath] && canonicalPath != "unsafe" && pkg.ExportFile == "" {
			return fmt.Errorf("non-member package %q has no ExportFile", pkg.ID)
		}
		if !roots[canonicalPath] && canonicalPath != "unsafe" && pkg.Imports == nil {
			return fmt.Errorf("non-member package %q has no explicit Imports map", pkg.ID)
		}
		if pkg.Imports != nil {
			for importPath, imported := range pkg.Imports {
				if importPath == "" {
					return fmt.Errorf("package %q has an empty import path", pkg.ID)
				}
				if imported == nil || imported.ID == "" {
					return fmt.Errorf("package %q import %q has no package ID", pkg.ID, importPath)
				}
			}
		}
	}
	if err := validateLayoutDependencies(layout.DependencyArtifactBindings); err != nil {
		return err
	}
	return nil
}

func validateStdlibTargetMatchesPlatform(platform *packagelayout.Platform, target *packagelayout.StdlibExportTarget) error {
	if platform.GOOS != target.GOOS || platform.GOARCH != target.GOARCH || platform.CgoEnabled != target.CgoEnabled ||
		!slices.Equal(sortedStrings(platform.BuildTags), sortedStrings(target.BuildTags)) {
		return fmt.Errorf("stdlib_export_data.target does not match platform")
	}
	if platform.ToolchainVersion != nil && *platform.ToolchainVersion != target.ToolchainVersion {
		return fmt.Errorf("stdlib_export_data.target toolchain_version does not match platform")
	}
	if platform.GOEXPERIMENT != nil && *platform.GOEXPERIMENT != target.GOEXPERIMENT {
		return fmt.Errorf("stdlib_export_data.target goexperiment does not match platform")
	}
	return nil
}

func validateLayoutDependencies(bindings []packagelayout.DependencyArtifactBinding) error {
	seen := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		if binding.Dependency == "" {
			return fmt.Errorf("dependency artifact binding has empty dependency")
		}
		if seen[binding.Dependency] {
			return fmt.Errorf("duplicate dependency artifact binding %q", binding.Dependency)
		}
		seen[binding.Dependency] = true
		if _, err := shapeRequiredLayoutPath(binding.Surface, layoutArtifactPlaceholder); err != nil {
			return fmt.Errorf("dependency %q surface: %w", binding.Dependency, err)
		}
		if _, err := shapeOptionalLayoutPath(binding.Report, layoutArtifactPlaceholder); err != nil {
			return fmt.Errorf("dependency %q report: %w", binding.Dependency, err)
		}
	}
	return nil
}

func sortedStrings(values []string) []string {
	copy := slices.Clone(values)
	sort.Strings(copy)
	return copy
}

func canonicalizeLayoutShapePaths(shape *layoutShapeSnapshot) {
	for i, root := range shape.Roots {
		shape.Roots[i] = hostpolicy.CanonicalizePath(root)
	}
	sort.Strings(shape.Roots)
	for i := range shape.Packages {
		pkg := &shape.Packages[i]
		pkg.ID = hostpolicy.CanonicalizePath(pkg.ID)
		pkg.PkgPath = hostpolicy.CanonicalizePath(pkg.PkgPath)
		if pkg.Imports == nil {
			continue
		}
		imports := make(map[string]string, len(pkg.Imports))
		for importPath, id := range pkg.Imports {
			imports[hostpolicy.CanonicalizePath(importPath)] = hostpolicy.CanonicalizePath(id)
		}
		pkg.Imports = imports
	}
	slices.SortFunc(shape.Packages, func(a, b layoutPackageShape) int {
		return strings.Compare(a.ID, b.ID)
	})
}

func layoutShapeDiff(want, got []byte) string {
	wantLines := strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	gotLines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	n := min(len(wantLines), len(gotLines))
	for i := 0; i < n; i++ {
		if wantLines[i] != gotLines[i] {
			return fmt.Sprintf("at line %d:\n- %s\n+ %s", i+1, wantLines[i], gotLines[i])
		}
	}
	return fmt.Sprintf("line count differs (golden %d, actual %d)", len(wantLines), len(gotLines))
}
