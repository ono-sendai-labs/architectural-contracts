package packagelayout

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

const bazelExecrootPlaceholder = "__BAZEL_EXECROOT__/"

// StdlibExportData identifies the generated package graph used to supply
// target-configured standard-library export files. The metadata file is a
// rules_go flat-package JSON stream; the compiled artifacts it names are
// separately declared action inputs by the Bazel emitter.
type StdlibExportData struct {
	Metadata    string              `json:"metadata"`
	Target      *StdlibExportTarget `json:"target,omitempty"`
	ExportRoots []StdlibExportRoot  `json:"export_roots,omitempty"`
}

// StdlibExportRoot maps the execroot spelling embedded in rules_go metadata to
// the runfiles spelling available to both the action wrapper and replay tests.
// Keeping both forms lets the runtime use declared tree artifacts without
// assuming which of those two equivalent working-directory frames it entered.
type StdlibExportRoot struct {
	RunfilesPath string `json:"runfiles_path"`
	ExecPath     string `json:"exec_path"`
}

// StdlibExportTarget is the target identity attached to a standard-library
// export descriptor. It is checked against the layout platform before the
// descriptor can contribute package records, so a hand-edited layout cannot
// pair one target's exports with another target's source files.
type StdlibExportTarget struct {
	ToolchainVersion string   `json:"toolchain_version"`
	GOOS             string   `json:"goos"`
	GOARCH           string   `json:"goarch"`
	CgoEnabled       bool     `json:"cgo_enabled"`
	BuildTags        []string `json:"build_tags"`
	GOEXPERIMENT     string   `json:"goexperiment"`
}

func cloneStdlibExportData(data *StdlibExportData) (*StdlibExportData, error) {
	if data == nil {
		return nil, nil
	}
	if data.Metadata == "" {
		return nil, errors.New("standard-library export metadata path is required")
	}
	if err := validateExportFilePath(data.Metadata); err != nil {
		return nil, fmt.Errorf("unsafe standard-library export metadata path %q: %w", data.Metadata, err)
	}
	clone := &StdlibExportData{
		Metadata:    data.Metadata,
		ExportRoots: append([]StdlibExportRoot(nil), data.ExportRoots...),
	}
	for i := range clone.ExportRoots {
		root := &clone.ExportRoots[i]
		if root.RunfilesPath == "" || root.ExecPath == "" {
			return nil, fmt.Errorf("standard-library export root %d requires runfiles_path and exec_path", i)
		}
		if err := validateExportFilePath(root.RunfilesPath); err != nil {
			return nil, fmt.Errorf("invalid standard-library export root runfiles path %q: %w", root.RunfilesPath, err)
		}
		if err := validateExportFilePath(root.ExecPath); err != nil {
			return nil, fmt.Errorf("invalid standard-library export root exec path %q: %w", root.ExecPath, err)
		}
	}
	slices.SortFunc(clone.ExportRoots, func(a, b StdlibExportRoot) int {
		if c := strings.Compare(a.ExecPath, b.ExecPath); c != 0 {
			return c
		}
		return strings.Compare(a.RunfilesPath, b.RunfilesPath)
	})
	seenExecPaths := make(map[string]string, len(clone.ExportRoots))
	seenRunfilesPaths := make(map[string]string, len(clone.ExportRoots))
	for _, root := range clone.ExportRoots {
		if previous, exists := seenExecPaths[root.ExecPath]; exists {
			if previous != root.RunfilesPath {
				return nil, fmt.Errorf("conflicting standard-library export root %q maps to %q and %q", root.ExecPath, previous, root.RunfilesPath)
			}
			return nil, fmt.Errorf("duplicate standard-library export root %q", root.ExecPath)
		}
		if previous, exists := seenRunfilesPaths[root.RunfilesPath]; exists {
			return nil, fmt.Errorf("conflicting standard-library export root runfiles path %q maps to %q and %q", root.RunfilesPath, previous, root.ExecPath)
		}
		seenExecPaths[root.ExecPath] = root.RunfilesPath
		seenRunfilesPaths[root.RunfilesPath] = root.ExecPath
	}
	if data.Target != nil {
		clone.Target = &StdlibExportTarget{
			ToolchainVersion: data.Target.ToolchainVersion,
			GOOS:             data.Target.GOOS,
			GOARCH:           data.Target.GOARCH,
			CgoEnabled:       data.Target.CgoEnabled,
			BuildTags:        append([]string(nil), data.Target.BuildTags...),
			GOEXPERIMENT:     data.Target.GOEXPERIMENT,
		}
		sort.Strings(clone.Target.BuildTags)
	}
	return clone, nil
}

type stdlibExportPackage struct {
	ID              string            `json:"ID"`
	Name            string            `json:"Name"`
	PkgPath         string            `json:"PkgPath"`
	GoFiles         []string          `json:"GoFiles"`
	CompiledGoFiles []string          `json:"CompiledGoFiles"`
	OtherFiles      []string          `json:"OtherFiles"`
	ExportFile      string            `json:"ExportFile"`
	Imports         map[string]string `json:"Imports"`
	Standard        bool              `json:"Standard"`
}

// ResolveStdlibExportData merges a host adapter's generated standard-library
// package descriptor into l. It is intentionally separate from descriptor
// discovery: the adapter chooses and declares the files, while this package
// validates the wire graph, normalizes rules_go's execroot marker, and owns
// the package-layout representation consumed by the driver.
func ResolveStdlibExportData(l *Layout, workspaceDir string) error {
	if l == nil || l.StdlibExportData == nil || l.stdlibExportDataResolved {
		return nil
	}

	descriptor, err := cloneStdlibExportData(l.StdlibExportData)
	if err != nil {
		return err
	}
	if err := validateStdlibExportTarget(l.Platform, descriptor.Target); err != nil {
		return err
	}
	l.StdlibExportData = descriptor

	metadataPath := filepath.FromSlash(descriptor.Metadata)
	if workspaceDir != "" {
		metadataPath = filepath.Join(workspaceDir, metadataPath)
	}
	info, err := os.Stat(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("standard-library export metadata %q does not exist: %w", metadataPath, err)
		}
		return fmt.Errorf("error stating standard-library export metadata %q: %w", metadataPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("standard-library export metadata %q is a directory", metadataPath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("standard-library export metadata %q is not a regular file", metadataPath)
	}
	if info.Size() == 0 {
		return fmt.Errorf("standard-library export metadata %q is empty", metadataPath)
	}

	records, err := readStdlibExportPackages(metadataPath, descriptor.ExportRoots)
	if err != nil {
		return fmt.Errorf("reading standard-library export metadata %q: %w", metadataPath, err)
	}
	if err := mergeStdlibExportPackages(l, records); err != nil {
		return err
	}
	l.stdlibExportDataResolved = true
	return nil
}

func validateStdlibExportTarget(platform *Platform, target *StdlibExportTarget) error {
	if target == nil {
		return errors.New("standard-library export metadata has no target configuration")
	}
	if platform == nil {
		return errors.New("standard-library export metadata has a target configuration but the layout has no platform")
	}

	toolchainVersion := ""
	if platform.ToolchainVersion != nil {
		toolchainVersion = *platform.ToolchainVersion
	}
	goexperiment := ""
	if platform.GOEXPERIMENT != nil {
		goexperiment = *platform.GOEXPERIMENT
	}
	actualTags := append([]string(nil), platform.BuildTags...)
	expectedTags := append([]string(nil), target.BuildTags...)
	sort.Strings(actualTags)
	sort.Strings(expectedTags)

	mismatches := []string{}
	if toolchainVersion != target.ToolchainVersion {
		mismatches = append(mismatches, fmt.Sprintf("toolchain_version=%q", target.ToolchainVersion))
	}
	if platform.GOOS != target.GOOS {
		mismatches = append(mismatches, fmt.Sprintf("goos=%q", target.GOOS))
	}
	if platform.GOARCH != target.GOARCH {
		mismatches = append(mismatches, fmt.Sprintf("goarch=%q", target.GOARCH))
	}
	if platform.CgoEnabled != target.CgoEnabled {
		mismatches = append(mismatches, fmt.Sprintf("cgo_enabled=%t", target.CgoEnabled))
	}
	if strings.Join(actualTags, "\x00") != strings.Join(expectedTags, "\x00") {
		mismatches = append(mismatches, fmt.Sprintf("build_tags=%q", strings.Join(expectedTags, ",")))
	}
	if goexperiment != target.GOEXPERIMENT {
		mismatches = append(mismatches, fmt.Sprintf("goexperiment=%q", target.GOEXPERIMENT))
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("standard-library export data configuration mismatch; layout target differs in %s", strings.Join(mismatches, ", "))
	}
	return nil
}

func readStdlibExportPackages(metadataPath string, exportRoots []StdlibExportRoot) ([]stdlibExportPackage, error) {
	f, err := os.Open(metadataPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	var records []stdlibExportPackage
	for index := 0; ; index++ {
		var record stdlibExportPackage
		err := decoder.Decode(&record)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decoding package record %d: %w", index, err)
		}
		if record.ID == "" {
			return nil, fmt.Errorf("standard-library package record %d has empty ID", index)
		}
		if record.PkgPath == "" {
			return nil, fmt.Errorf("standard-library package %q has empty import path", record.ID)
		}
		if err := validateStdlibPackagePath(record.PkgPath); err != nil {
			return nil, fmt.Errorf("standard-library package %q has %w %q", record.ID, err, record.PkgPath)
		}
		if record.Name == "" {
			return nil, fmt.Errorf("standard-library package %q has empty name", record.PkgPath)
		}
		// rules_go's flat package schema uses `omitempty` on Imports, so an
		// absent field is its canonical spelling for a leaf. Materialize an
		// explicit empty map in the effective layout; the member-only layout
		// contract must never pass a nil map to go/packages.
		if record.Imports == nil {
			record.Imports = make(map[string]string)
		}
		record.GoFiles = append([]string(nil), record.GoFiles...)
		record.CompiledGoFiles = append([]string(nil), record.CompiledGoFiles...)
		record.OtherFiles = append([]string(nil), record.OtherFiles...)
		if record.ExportFile != "" {
			record.ExportFile, err = normalizeStdlibExportPath(record.ExportFile, exportRoots)
			if err != nil {
				return nil, fmt.Errorf("standard-library package %q has invalid export file %q: %w", record.PkgPath, record.ExportFile, err)
			}
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, errors.New("standard-library export metadata contains no package records")
	}
	return records, nil
}

func validateStdlibPackagePath(value string) error {
	if strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.Contains(value, "\\") ||
		path.IsAbs(value) || filepath.VolumeName(value) != "" || hasWindowsVolumePrefix(value) {
		return errors.New("invalid standard-library package import path")
	}
	if value == "." || value == ".." || path.Clean(value) != value {
		return errors.New("invalid standard-library package import path")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("invalid standard-library package import path")
		}
	}
	return nil
}

func normalizeStdlibExportPath(value string, exportRoots []StdlibExportRoot) (string, error) {
	if strings.HasPrefix(value, bazelExecrootPlaceholder) {
		value = strings.TrimPrefix(value, bazelExecrootPlaceholder)
		bestLength := -1
		bestExecPath := ""
		bestRunfilesPath := ""
		for _, root := range exportRoots {
			if value == root.ExecPath {
				if len(root.ExecPath) > bestLength {
					bestLength = len(root.ExecPath)
					bestExecPath = root.ExecPath
					bestRunfilesPath = root.RunfilesPath
				}
				continue
			}
			prefix := root.ExecPath + "/"
			if strings.HasPrefix(value, prefix) && len(root.ExecPath) > bestLength {
				bestLength = len(root.ExecPath)
				bestExecPath = root.ExecPath
				bestRunfilesPath = root.RunfilesPath
			}
		}
		if bestLength >= 0 {
			value = bestRunfilesPath + strings.TrimPrefix(value, bestExecPath)
		}
	} else if strings.Contains(value, "__BAZEL_EXECROOT__") {
		return "", errors.New("execroot placeholder is not at the beginning of the path")
	}
	if err := validateExportFilePath(value); err != nil {
		return "", err
	}
	return value, nil
}

func mergeStdlibExportPackages(l *Layout, records []stdlibExportPackage) error {
	byID := make(map[string]stdlibExportPackage, len(records))
	byPath := make(map[string]stdlibExportPackage, len(records))
	for _, record := range records {
		if _, exists := byID[record.ID]; exists {
			return fmt.Errorf("duplicate standard-library package ID %q in export metadata", record.ID)
		}
		if _, exists := byPath[record.PkgPath]; exists {
			return fmt.Errorf("duplicate standard-library package import path %q in export metadata", record.PkgPath)
		}
		byID[record.ID] = record
		byPath[record.PkgPath] = record
	}

	merged := make([]*packages.Package, 0, len(records))
	for _, record := range records {
		imports := make(map[string]*packages.Package, len(record.Imports))
		var importPaths []string
		for importPath := range record.Imports {
			importPaths = append(importPaths, importPath)
		}
		sort.Strings(importPaths)
		for _, importPath := range importPaths {
			if importPath == "C" {
				continue
			}
			rawID := record.Imports[importPath]
			if rawID == "" {
				return fmt.Errorf("standard-library package %q has empty package ID for import path %q", record.PkgPath, importPath)
			}
			target, ok := byID[rawID]
			if !ok {
				return fmt.Errorf("standard-library package %q imports path %q but descriptor package ID %q is absent", record.PkgPath, importPath, rawID)
			}
			if target.PkgPath != importPath && target.PkgPath != "vendor/"+importPath {
				return fmt.Errorf("standard-library package %q imports path %q with descriptor ID %q, whose import path is %q", record.PkgPath, importPath, rawID, target.PkgPath)
			}
			imports[importPath] = &packages.Package{ID: target.PkgPath}
		}

		merged = append(merged, &packages.Package{
			ID:              record.PkgPath,
			Name:            record.Name,
			PkgPath:         record.PkgPath,
			GoFiles:         append([]string(nil), record.GoFiles...),
			CompiledGoFiles: stdlibCompiledGoFiles(record),
			OtherFiles:      append([]string(nil), record.OtherFiles...),
			ExportFile:      record.ExportFile,
			Imports:         imports,
		})
	}
	existingIDs := make(map[string]bool, len(l.Packages))
	existingPaths := make(map[string]bool, len(l.Packages))
	for _, pkg := range l.Packages {
		if pkg == nil {
			return errors.New("package entry is null while merging standard-library export metadata")
		}
		if existingIDs[pkg.ID] {
			return fmt.Errorf("duplicate package ID %q while merging standard-library export metadata", pkg.ID)
		}
		if existingPaths[pkg.PkgPath] {
			return fmt.Errorf("duplicate package import path %q while merging standard-library export metadata", pkg.PkgPath)
		}
		existingIDs[pkg.ID] = true
		existingPaths[pkg.PkgPath] = true
	}
	for _, pkg := range merged {
		if existingIDs[pkg.ID] {
			return fmt.Errorf("standard-library export package %q conflicts with existing package ID %q", pkg.PkgPath, pkg.ID)
		}
		if existingPaths[pkg.PkgPath] {
			return fmt.Errorf("standard-library export package %q conflicts with existing package import path %q", pkg.PkgPath, pkg.PkgPath)
		}
	}

	slices.SortFunc(merged, func(a, b *packages.Package) int { return strings.Compare(a.PkgPath, b.PkgPath) })
	if l.stdlibByID == nil {
		l.stdlibByID = make(map[string]bool)
	}
	if l.stdlibByPath == nil {
		l.stdlibByPath = make(map[string]bool)
	}
	if l.emittedPackageID == nil {
		l.emittedPackageID = make(map[string]bool)
	}
	for _, pkg := range merged {
		l.Packages = append(l.Packages, pkg)
		l.stdlibByID[pkg.ID] = true
		l.stdlibByPath[pkg.PkgPath] = true
		l.emittedPackageID[pkg.ID] = true
	}
	return nil
}

func stdlibCompiledGoFiles(record stdlibExportPackage) []string {
	if len(record.CompiledGoFiles) > 0 {
		return append([]string(nil), record.CompiledGoFiles...)
	}
	// The upstream stdliblist omits CompiledGoFiles for a pure build unless
	// cgo asks for `-compiled`; rules_go's own package registry applies this
	// same fallback before serving the driver response.
	return append([]string(nil), record.GoFiles...)
}

// validateReachableExportClosure checks the export-data contract after all
// source-backed import recovery has completed. The traversal follows only
// declared graph edges, so an unreachable descriptor record cannot hide a
// malformed artifact while the component's actual closure remains the unit of
// validation.
func validateReachableExportClosure(l *Layout, index layoutPackageIndex, roots []*packages.Package, rootIDs map[string]bool, workspaceDir string) error {
	visited := make(map[string]bool, len(l.Packages))
	var visit func(*packages.Package) error
	visit = func(pkg *packages.Package) error {
		if visited[pkg.ID] {
			return nil
		}
		visited[pkg.ID] = true

		if !rootIDs[pkg.ID] {
			if pkg.PkgPath == "unsafe" {
				if pkg.ExportFile != "" {
					return fmt.Errorf("builtin package %q must not carry an export file", pkg.PkgPath)
				}
			} else {
				resolved, err := l.resolveAndCheckExportFile(pkg, pkg.ExportFile, workspaceDir)
				if err != nil {
					return err
				}
				if pkg.Imports == nil {
					return fmt.Errorf("export-backed package %q (import path %q) has omitted Imports map; provide an explicit empty map for a leaf", pkg.ID, pkg.PkgPath)
				}
				pkg.ExportFile = resolved
			}
		}

		var importPaths []string
		for importPath := range pkg.Imports {
			importPaths = append(importPaths, importPath)
		}
		sort.Strings(importPaths)
		for _, importPath := range importPaths {
			if importPath == "C" {
				continue
			}
			reference := pkg.Imports[importPath]
			if reference == nil {
				return fmt.Errorf("package %q (import path %q) imports path %q with a nil package reference", pkg.ID, pkg.PkgPath, importPath)
			}
			target, ok := index.byID[reference.ID]
			if !ok {
				return fmt.Errorf("package %q (import path %q) imports path %q but package ID %q is absent from the layout", pkg.ID, pkg.PkgPath, importPath, reference.ID)
			}
			if target.PkgPath != importPath && (target.PkgPath != "vendor/"+importPath || !l.IsStdlibPackage(target)) {
				return fmt.Errorf("package %q (import path %q) imports path %q with ID %q, but target package import path is %q", pkg.ID, pkg.PkgPath, importPath, target.ID, target.PkgPath)
			}
			if err := visit(target); err != nil {
				return err
			}
		}
		return nil
	}

	for _, root := range roots {
		if err := visit(root); err != nil {
			return err
		}
	}
	return nil
}
