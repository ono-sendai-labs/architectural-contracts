// Package packagelayout defines the schema, validation, and driver protocol
// for arcc's hermetic package-layout loader.
//
// Component Contract (FR10):
//   - Declared Interface Style: layout.go (schema types, JSON codecs, shape
//     views) and api.go (driver runner, environment hook, stdlib layout
//     discovery, and import graph actions).
//   - Declared Authority: REFLECT and UNSAFE_POINTER only, from encoding/json
//     in the layout JSON codecs and import-data descriptors (C9). Filesystem
//     and environment access are delegated to packagelayout-hostenv or
//     received via explicit fs.FS parameters.
package packagelayout

import (
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/tools/go/packages"
)

// Layout is the package layout schema representing a Bazel-produced package graph.
type Layout struct {
	Component                  string                      `json:"-"`
	GoSDKRoot                  string                      `json:"go_sdk_root"`
	Platform                   *Platform                   `json:"platform,omitempty"`
	Roots                      []string                    `json:"roots"`
	Packages                   []*packages.Package         `json:"packages"`
	OrdinaryImportData         *OrdinaryImportData         `json:"ordinary_import_data,omitempty"`
	StdlibExportData           *StdlibExportData           `json:"stdlib_export_data,omitempty"`
	DependencyArtifactBindings []DependencyArtifactBinding `json:"dependency_artifact_bindings,omitempty"`

	stdlibByID                 map[string]bool
	stdlibByPath               map[string]bool
	emittedPackageID           map[string]bool
	ordinaryImportDataResolved bool
	stdlibExportDataResolved   bool

	UnresolvedImports []UnresolvedImport `json:"-"`
	importsOmitted    map[string]bool
}

// IsMemberOnlyLayout reports whether l carries the export-data contract used by
// component checks.
func IsMemberOnlyLayout(l *Layout) bool {
	if l == nil {
		return false
	}
	if l.OrdinaryImportData != nil || l.StdlibExportData != nil {
		return true
	}
	for _, pkg := range l.Packages {
		if pkg != nil && pkg.ExportFile != "" {
			return true
		}
	}
	return false
}

// DependencyArtifactProvenance identifies how a dependency's artifact pair was
// produced in the build graph.
type DependencyArtifactProvenance string

const (
	DependencyArtifactProvenanceChecked  DependencyArtifactProvenance = "checked"
	DependencyArtifactProvenanceAsserted DependencyArtifactProvenance = "asserted"
)

// DependencyArtifactBinding binds one manifest dependency name to the surface
// and optional report files published by its ArccComponentInfo provider.
type DependencyArtifactBinding struct {
	Dependency   string                       `json:"dependency"`
	Surface      string                       `json:"surface"`
	Report       string                       `json:"report,omitempty"`
	AutoAttached bool                         `json:"auto_attached"`
	Provenance   DependencyArtifactProvenance `json:"provenance"`
}

// UnresolvedImport records a surviving source-level import for which the
// layout has no package node.
type UnresolvedImport struct {
	Package    string
	SourceFile string
	ImportPath string
}

// Platform declares the target used when evaluating Go build constraints.
type Platform struct {
	GOOS             string   `json:"goos"`
	GOARCH           string   `json:"goarch"`
	BuildTags        []string `json:"build_tags"`
	CgoEnabled       bool     `json:"cgo_enabled"`
	ToolchainVersion *string  `json:"toolchain_version,omitempty"`
	GOEXPERIMENT     *string  `json:"goexperiment,omitempty"`
}

// OrdinaryImportData points at the build-time source-import projection for a
// component's ordinary packages.
type OrdinaryImportData struct {
	Metadata string `json:"metadata"`
}

// StdlibExportData identifies the generated package graph used to supply
// target-configured standard-library export files.
type StdlibExportData struct {
	Metadata    string              `json:"metadata"`
	Target      *StdlibExportTarget `json:"target,omitempty"`
	ExportRoots []StdlibExportRoot  `json:"export_roots,omitempty"`
}

// StdlibExportRoot maps the execroot spelling embedded in rules_go metadata to
// the runfiles spelling available to both the action wrapper and replay tests.
type StdlibExportRoot struct {
	RunfilesPath string `json:"runfiles_path"`
	ExecPath     string `json:"exec_path"`
}

// StdlibExportTarget is the target identity attached to a standard-library
// export descriptor.
type StdlibExportTarget struct {
	ToolchainVersion string   `json:"toolchain_version"`
	GOOS             string   `json:"goos"`
	GOARCH           string   `json:"goarch"`
	CgoEnabled       bool     `json:"cgo_enabled"`
	BuildTags        []string `json:"build_tags"`
	GOEXPERIMENT     string   `json:"goexperiment"`
}

// PackageShapeView captures the package-level facts needed for layout shape
// emission and validation without exposing x/tools package objects to consumers.
type PackageShapeView struct {
	IsNull          bool
	ID              string
	PkgPath         string
	Name            string
	GoFiles         []string
	CompiledGoFiles []string
	IgnoredFiles    []string
	OtherFiles      []string
	ExportFile      string
	IsStdlib        bool
	Imports         map[string]*PackageShapeImportTarget
}

// PackageShapeImportTarget captures target information for one imported package.
type PackageShapeImportTarget struct {
	ID      string
	PkgPath string
}

// ShapeImports projects a package import map into its stable JSON identity form.
func (v PackageShapeView) ShapeImports(canonicalize func(string) string) (map[string]string, error) {
	if v.Imports == nil {
		return nil, nil
	}
	if canonicalize == nil {
		canonicalize = func(path string) string { return path }
	}
	shaped := make(map[string]string, len(v.Imports))
	for importPath, target := range v.Imports {
		if target == nil || target.ID == "" {
			return nil, fmt.Errorf("import %q has no package ID", importPath)
		}
		canonicalImport := canonicalize(importPath)
		canonicalID := canonicalize(target.ID)
		if _, exists := shaped[canonicalImport]; exists {
			return nil, fmt.Errorf("canonical import path %q has multiple entries", canonicalImport)
		}
		shaped[canonicalImport] = canonicalID
	}
	return shaped, nil
}

// PackageViews projects l.Packages into a slice of PackageShapeView.
func (l *Layout) PackageViews() []PackageShapeView {
	return l.packageViews()
}

// BuildTags models the platform identities, compiler, release tags, and custom
// build tags needed to evaluate Go build constraints without ambient authority.
type BuildTags struct {
	GOOS        string
	GOARCH      string
	GOROOT      string
	GOPATH      string
	Compiler    string
	CgoEnabled  bool
	ReleaseTags []string
	BuildTags   []string
	ToolTags    []string
}

// HasTag reports whether the given tag is satisfied by the BuildTags configuration.
func (b BuildTags) HasTag(tag string) bool {
	if tag == "cgo" {
		return b.CgoEnabled
	}
	if tag == b.GOOS || tag == b.GOARCH || tag == b.Compiler {
		return true
	}
	if (b.GOOS == "android" && tag == "linux") ||
		(b.GOOS == "illumos" && tag == "solaris") ||
		(b.GOOS == "ios" && tag == "darwin") {
		return true
	}
	if tag == "unix" {
		switch b.GOOS {
		case "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "linux", "netbsd", "openbsd", "solaris":
			return true
		}
	}
	if tag == "boringcrypto" {
		tag = "goexperiment.boringcrypto"
	}
	for _, candidate := range b.BuildTags {
		if candidate == tag {
			return true
		}
	}
	for _, candidate := range b.ToolTags {
		if candidate == tag {
			return true
		}
	}
	for _, candidate := range b.ReleaseTags {
		if candidate == tag {
			return true
		}
	}
	return false
}

// IsStdlibPackage reports the validated standard-library provenance for a
// package in layout mode. Missing provenance is non-stdlib (fail closed).
func (l *Layout) IsStdlibPackage(p *packages.Package) bool {
	if l == nil || p == nil {
		return false
	}
	if l.stdlibByID != nil {
		if verdict, ok := l.stdlibByID[p.ID]; ok {
			return verdict
		}
	}
	if l.stdlibByPath != nil {
		if verdict, ok := l.stdlibByPath[p.PkgPath]; ok {
			return verdict
		}
	}
	return false
}

// Parse parses a Layout from JSON data and requires EOF after the decoded layout to prevent trailing garbage.
func Parse(r io.Reader) (*Layout, error) {
	var l Layout
	dec := json.NewDecoder(r)
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("malformed layout JSON: %w", err)
	}
	if t, err := dec.Token(); err == nil {
		return nil, fmt.Errorf("trailing garbage after layout JSON: %v", t)
	} else if err != io.EOF {
		return nil, fmt.Errorf("trailing garbage after layout JSON: %w", err)
	}
	return &l, nil
}

// MarshalJSON serializes the Layout with deterministic canonical ordering of
// Roots, dependency artifact bindings, and Packages without mutating the
// original caller-owned data.
func (l *Layout) MarshalJSON() ([]byte, error) {
	return l.marshalJSON()
}

// UnmarshalJSON decodes the upstream go/packages package shape and retains the
// layout-only is_stdlib side metadata.
func (l *Layout) UnmarshalJSON(data []byte) error {
	return l.unmarshalJSON(data)
}
