package packagelayout

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout/internal/hostenv"
	"golang.org/x/tools/go/packages"
)

const bazelExecrootPlaceholder = "__BAZEL_EXECROOT__/"

// resolveStdlibExportData reads the export-data descriptor and merges its
// records into the layout package graph.
func resolveStdlibExportData(l *Layout, workspaceDir string) error {
	if l == nil || l.StdlibExportData == nil || l.stdlibExportDataResolved {
		return nil
	}

	descriptor, err := cloneStdlibExportData(l.StdlibExportData)
	if err != nil {
		return err
	}
	l.StdlibExportData = descriptor

	metadataPath := filepath.FromSlash(descriptor.Metadata)
	if workspaceDir != "" {
		metadataPath = filepath.Join(workspaceDir, metadataPath)
	}
	info, err := fs.Stat(defaultFS, filepath.ToSlash(metadataPath))
	if err != nil {
		if hostenv.IsNotExist(err) {
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

	if err := validateStdlibExportTarget(l.Platform, descriptor.Target); err != nil {
		return err
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
		return nil
	}
	if platform == nil {
		return errors.New("layout has no target platform, but standard-library export data declares one")
	}

	var mismatches []string
	if platform.GOOS != target.GOOS {
		mismatches = append(mismatches, fmt.Sprintf("goos (layout=%q, export=%q)", platform.GOOS, target.GOOS))
	}
	if platform.GOARCH != target.GOARCH {
		mismatches = append(mismatches, fmt.Sprintf("goarch (layout=%q, export=%q)", platform.GOARCH, target.GOARCH))
	}
	if platform.CgoEnabled != target.CgoEnabled {
		mismatches = append(mismatches, fmt.Sprintf("cgo_enabled (layout=%t, export=%t)", platform.CgoEnabled, target.CgoEnabled))
	}

	platformTags := append([]string(nil), platform.BuildTags...)
	targetTags := append([]string(nil), target.BuildTags...)
	sort.Strings(platformTags)
	sort.Strings(targetTags)
	if !slices.Equal(platformTags, targetTags) {
		mismatches = append(mismatches, fmt.Sprintf("build_tags (layout=%v, export=%v)", platformTags, targetTags))
	}

	platformVersion := ""
	if platform.ToolchainVersion != nil {
		platformVersion = *platform.ToolchainVersion
	}
	if platformVersion != target.ToolchainVersion {
		mismatches = append(mismatches, fmt.Sprintf("toolchain_version (layout=%q, export=%q)", platformVersion, target.ToolchainVersion))
	}

	platformExp := ""
	if platform.GOEXPERIMENT != nil {
		platformExp = *platform.GOEXPERIMENT
	}
	if platformExp != target.GOEXPERIMENT {
		mismatches = append(mismatches, fmt.Sprintf("goexperiment (layout=%q, export=%q)", platformExp, target.GOEXPERIMENT))
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("standard-library export data configuration mismatch; layout target differs in %s", strings.Join(mismatches, ", "))
	}
	return nil
}

type stdlibExportPackage struct {
	ID              string            `json:"ID"`
	Name            string            `json:"Name"`
	PkgPath         string            `json:"PkgPath"`
	ExportFile      string            `json:"ExportFile"`
	GoFiles         []string          `json:"GoFiles"`
	CompiledGoFiles []string          `json:"CompiledGoFiles"`
	OtherFiles      []string          `json:"OtherFiles"`
	Imports         map[string]string `json:"Imports"`
}

func readStdlibExportPackages(metadataPath string, exportRoots []StdlibExportRoot) ([]stdlibExportPackage, error) {
	f, err := defaultFS.Open(filepath.ToSlash(metadataPath))
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

func decodeStdlibExportFiles(r io.Reader) ([]string, error) {
	decoder := json.NewDecoder(r)
	var files []string
	for recordNumber := 0; ; recordNumber++ {
		var record struct {
			ExportFile string `json:"ExportFile"`
		}
		err := decoder.Decode(&record)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decoding standard-library export metadata record %d: %w", recordNumber, err)
		}
		if record.ExportFile != "" {
			files = append(files, record.ExportFile)
		}
	}
	return files, nil
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
		bestRunfiles := ""
		for _, root := range exportRoots {
			prefix := strings.TrimPrefix(root.ExecPath, bazelExecrootPlaceholder)
			if prefix == "" {
				continue
			}
			if value == prefix {
				return root.RunfilesPath, nil
			}
			if strings.HasPrefix(value, prefix+"/") {
				if len(prefix) > bestLength {
					bestLength = len(prefix)
					bestExecPath = prefix
					bestRunfiles = root.RunfilesPath
				}
			}
		}
		if bestLength >= 0 {
			rel := strings.TrimPrefix(value, bestExecPath+"/")
			return path.Join(bestRunfiles, rel), nil
		}
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
		importPaths := make([]string, 0, len(record.Imports))
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
	return append([]string(nil), record.GoFiles...)
}

// validateReachableExportClosure checks the export-data contract after all
// source-backed import recovery has completed.
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
