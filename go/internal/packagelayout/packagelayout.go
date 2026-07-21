// Package packagelayout defines the schema, validation, and driver protocol
// for arcc's hermetic package-layout loader.
package packagelayout

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// osStat is a seam for mocking file existence in unit tests.
var osStat = os.Stat

// Layout is the package layout schema representing a Bazel-produced package graph.
type Layout struct {
	GoSDKRoot string              `json:"go_sdk_root"`
	Roots     []string            `json:"roots"`
	Packages  []*packages.Package `json:"packages"`
}

// IsStdlib reports whether importPath is a standard-library package, using the
// same heuristic the go tool uses: the first path segment contains no dot.
func IsStdlib(importPath string) bool {
	if importPath == "" {
		return false
	}
	first := importPath
	if i := strings.IndexByte(importPath, '/'); i >= 0 {
		first = importPath[:i]
	}
	return !strings.Contains(first, ".")
}

// Parse parses a Layout from JSON data.
func Parse(r io.Reader) (*Layout, error) {
	var l Layout
	dec := json.NewDecoder(r)
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("malformed layout JSON: %w", err)
	}
	return &l, nil
}

// ValidateAndResolve validates the layout's structural consistency and resolves
// all workspace-relative and SDK-relative source paths, checking that each file exists.
func ValidateAndResolve(l *Layout, workspaceDir string) error {
	if l.GoSDKRoot == "" {
		// GoSDKRoot is required if standard library packages are present.
		// To be safe, we make it required if any stdlib package is referenced.
		for _, p := range l.Packages {
			if IsStdlib(p.PkgPath) {
				return errors.New("go_sdk_root is required when standard library packages are present in layout")
			}
		}
	}

	if len(l.Roots) == 0 {
		return errors.New("roots list cannot be empty")
	}

	// Index packages by ID and PkgPath to detect duplicates and enable graph traversal.
	byID := make(map[string]*packages.Package, len(l.Packages))
	byPath := make(map[string]*packages.Package, len(l.Packages))

	for _, p := range l.Packages {
		if p.ID == "" {
			return errors.New("package has empty ID")
		}
		if p.PkgPath == "" {
			return fmt.Errorf("package %q has empty import path", p.ID)
		}
		if p.Name == "" {
			return fmt.Errorf("package %q has empty name", p.ID)
		}

		if _, ok := byID[p.ID]; ok {
			return fmt.Errorf("duplicate package ID: %s", p.ID)
		}
		if _, ok := byPath[p.PkgPath]; ok {
			return fmt.Errorf("duplicate package import path: %s", p.PkgPath)
		}

		byID[p.ID] = p
		byPath[p.PkgPath] = p
	}

	// Validate roots.
	for _, root := range l.Roots {
		if _, ok := byID[root]; !ok {
			if _, okPath := byPath[root]; !okPath {
				return fmt.Errorf("unknown root: %s", root)
			}
		}
	}

	// Validate imports and resolve source paths.
	for _, p := range l.Packages {
		// Validate that every imported package ID exists in the layout.
		for impPath, impID := range p.Imports {
			if impPath == "" {
				return fmt.Errorf("package %q has empty import path key in Imports map", p.ID)
			}
			if impID == nil {
				return fmt.Errorf("package %q imports nil package reference for path %q", p.ID, impPath)
			}
			if _, ok := byID[impID.ID]; !ok {
				return fmt.Errorf("package %q imports unknown package ID %q", p.ID, impID.ID)
			}
		}

		// Resolve GoFiles.
		resolvedGoFiles, err := resolveAndCheckFiles(p, p.GoFiles, l.GoSDKRoot, workspaceDir)
		if err != nil {
			return err
		}
		p.GoFiles = resolvedGoFiles

		// Resolve CompiledGoFiles.
		resolvedCompiledFiles, err := resolveAndCheckFiles(p, p.CompiledGoFiles, l.GoSDKRoot, workspaceDir)
		if err != nil {
			return err
		}
		p.CompiledGoFiles = resolvedCompiledFiles
	}

	return nil
}

func resolveAndCheckFiles(p *packages.Package, files []string, sdkRoot, workspaceDir string) ([]string, error) {
	resolved := make([]string, len(files))
	for i, f := range files {
		if f == "" {
			return nil, fmt.Errorf("package %q contains empty source file path", p.ID)
		}

		var path string
		if IsStdlib(p.PkgPath) {
			// Resolve relative to sdkRoot.
			// Standard library files can be mapped to sdkRoot/PkgPath/BaseName.
			path = filepath.Join(sdkRoot, p.PkgPath, filepath.Base(f))
		} else {
			// Resolve relative to workspaceDir.
			if filepath.IsAbs(f) {
				path = f
			} else if workspaceDir != "" {
				path = filepath.Join(workspaceDir, f)
			} else {
				path = f
			}
		}

		// Check file existence.
		if _, err := osStat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("source file %q for package %q does not exist", path, p.ID)
			}
			return nil, fmt.Errorf("error stating source file %q for package %q: %w", path, p.ID, err)
		}

		resolved[i] = path
	}
	return resolved, nil
}

// HandleDriverRequest processes a packages.DriverRequest using a validated Layout
// and the query patterns, returning a packages.DriverResponse.
func HandleDriverRequest(l *Layout, req *packages.DriverRequest, patterns []string) (*packages.DriverResponse, error) {
	byID := make(map[string]*packages.Package, len(l.Packages))
	byPath := make(map[string]*packages.Package, len(l.Packages))
	var stdIDs []string

	for _, p := range l.Packages {
		byID[p.ID] = p
		byPath[p.PkgPath] = p
		if IsStdlib(p.PkgPath) {
			stdIDs = append(stdIDs, p.ID)
		}
	}

	// Deterministic standard library order.
	sort.Strings(stdIDs)

	var roots []string
	seen := map[string]bool{}
	addRoot := func(id string) {
		if !seen[id] {
			seen[id] = true
			roots = append(roots, id)
		}
	}

	for _, pat := range patterns {
		switch pat {
		case "std":
			if len(stdIDs) == 0 {
				return nil, fmt.Errorf("query for %q but no standard library packages exist in layout", pat)
			}
			for _, id := range stdIDs {
				addRoot(id)
			}
		default:
			// Match exact import path (PkgPath) or package ID.
			if p, ok := byPath[pat]; ok {
				addRoot(p.ID)
			} else if p, ok := byID[pat]; ok {
				addRoot(p.ID)
			} else {
				return nil, fmt.Errorf("no layout package found for pattern %q", pat)
			}
		}
	}

	// Always sort roots for determinism.
	sort.Strings(roots)

	// Ensure the response Packages list is also deterministic by sorting by ID.
	sort.Slice(l.Packages, func(i, j int) bool {
		return l.Packages[i].ID < l.Packages[j].ID
	})

	resp := &packages.DriverResponse{
		Compiler: "gc",
		Arch:     runtime.GOARCH,
		Roots:    roots,
		Packages: l.Packages,
	}
	return resp, nil
}

// RunDriver executes the complete GOPACKAGESDRIVER protocol logic. It reads
// the DriverRequest from stdin, resolves patterns from args, reads the layout
// file, validates and resolves paths, processes the query, and writes the
// JSON response to stdout.
func RunDriver(layoutPath string, workspaceDir string, patterns []string, stdin io.Reader, stdout io.Writer) error {
	// Read and parse DriverRequest.
	var req packages.DriverRequest
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			return fmt.Errorf("parsing DriverRequest: %w", err)
		}
	}

	if layoutPath == "" {
		return errors.New("package-layout file path is required")
	}

	f, err := os.Open(layoutPath)
	if err != nil {
		return fmt.Errorf("opening package-layout: %w", err)
	}
	defer f.Close()

	layout, err := Parse(f)
	if err != nil {
		return err
	}

	if err := ValidateAndResolve(layout, workspaceDir); err != nil {
		return fmt.Errorf("validating layout: %w", err)
	}

	resp, err := HandleDriverRequest(layout, &req, patterns)
	if err != nil {
		return fmt.Errorf("handling driver request: %w", err)
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("encoding response: %w", err)
	}

	return nil
}
