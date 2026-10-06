package packagelayout

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// discoverStdlibFS walks the standard library source tree in sdk,
// identifies importable packages using go/build, and returns them as a sorted slice of *packages.Package.
func discoverStdlibFS(sdk fs.FS) ([]*packages.Package, error) {
	if sdk == nil {
		return nil, nil
	}
	return discoverStdlibWithTags(sdk, nativeBuildTags())
}

func discoverStdlibWithTags(sdk fs.FS, tags BuildTags) ([]*packages.Package, error) {
	pkgs, _, err := discoverStdlibTree(sdk, tags)
	return pkgs, err
}

// discoverStdlibTree walks the standard library source tree rooted at sdk
// and classifies every directory under the go/build rules for tags: pkgs are
// the discovered packages (sorted, with the resolved import graph), noGoDirs
// the directories that exist but whose every Go file the target's build
// constraints exclude or that carry no Go sources at all (`go list std` still
// lists such packages, with no files).
func discoverStdlibTree(sdk fs.FS, tags BuildTags) ([]*packages.Package, []string, error) {
	fi, err := fs.Stat(sdk, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("accessing SDK root: %w", err)
	}
	if !fi.IsDir() {
		return nil, nil, errors.New("SDK root is not a directory")
	}

	var packageDirs []string
	var noGoDirs []string
	err = fs.WalkDir(sdk, ".", func(pathStr string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking SDK root at %s: %w", pathStr, err)
		}

		if d.IsDir() {
			importPath := pathStr
			if importPath == "." || importPath == "" {
				return nil
			}

			// go list std excludes the top-level cmd tree, every testdata
			// directory, tool-scratch directories (segments beginning with
			// `_` or `.`), nested modules (a directory carrying go.mod roots
			// a separate module the std pattern does not enumerate), and the
			// non-importable builtin package — but includes the GOROOT-level
			// vendor tree.
			if importPath == "cmd" || importPath == "testdata" || importPath == "builtin" ||
				strings.HasPrefix(importPath, "cmd/") || strings.HasSuffix(importPath, "/testdata") ||
				strings.Contains(importPath, "/testdata/") ||
				hasToolScratchSegment(importPath) {
				return fs.SkipDir
			}
			if fi, err := fs.Stat(sdk, path.Join(importPath, "go.mod")); err == nil && !fi.IsDir() {
				return fs.SkipDir
			}
			// With cgo disabled, the go tool's std-package walk ignores the
			// runtime/cgo directory entirely (its non-cgo Go files would
			// otherwise make the package look buildable).
			if importPath == "runtime/cgo" && !tags.CgoEnabled {
				return fs.SkipDir
			}
			packageDirs = append(packageDirs, importPath)
		}
		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	sort.Strings(packageDirs)

	type discoveredPackage struct {
		pkg     *packages.Package
		imports []string
	}
	discovered := make([]discoveredPackage, 0, len(packageDirs)+1)

	for _, dir := range packageDirs {
		name, goFiles, imports, err := scanSDKPackageDir(sdk, dir, tags)
		if err != nil {
			if _, ok := err.(*noGoPackageError); ok {
				noGoDirs = append(noGoDirs, dir)
				continue
			}
			return nil, nil, fmt.Errorf("inspecting standard-library package at %q: %w", dir, err)
		}

		if name == "main" {
			continue
		}

		importPath := dir
		discovered = append(discovered, discoveredPackage{
			pkg: &packages.Package{
				ID:              importPath,
				Name:            name,
				PkgPath:         importPath,
				GoFiles:         goFiles,
				CompiledGoFiles: append([]string{}, goFiles...),
				Imports:         make(map[string]*packages.Package),
			},
			imports: imports,
		})
	}

	sort.Strings(noGoDirs)

	// Older or reduced SDK source trees may omit unsafe even though it is a
	// compiler builtin. Keep one synthetic record only when the source tree did
	// not provide the real package; a complete SDK's unsafe.go is authoritative.
	unsafeFound := false
	for _, item := range discovered {
		if item.pkg.PkgPath == "unsafe" {
			unsafeFound = true
			break
		}
	}
	if !unsafeFound {
		discovered = append(discovered, discoveredPackage{pkg: &packages.Package{
			ID:      "unsafe",
			Name:    "unsafe",
			PkgPath: "unsafe",
			Imports: make(map[string]*packages.Package),
		}})
	}

	if len(discovered) <= 1 {
		return nil, nil, errors.New("invalid SDK root: structurally invalid (no standard-library packages discovered)")
	}

	known := make(map[string]bool, len(discovered))
	for _, item := range discovered {
		known[item.pkg.PkgPath] = true
	}
	for i := range discovered {
		for _, imp := range discovered[i].imports {
			// C is cgo's pseudo-package, not a package in the import graph.
			if imp == "C" {
				continue
			}
			resolved := imp
			if !known[resolved] {
				vendorPath := "vendor/" + resolved
				if known[vendorPath] {
					resolved = vendorPath
				}
			}
			if known[resolved] {
				// Key by the import path as written in source (imp), not the
				// resolved target: go/types resolves an import by the path in the
				// source, even when it is satisfied by a vendored copy. The target
				// package ID is the resolved (possibly vendor/-prefixed) path.
				discovered[i].pkg.Imports[imp] = &packages.Package{ID: resolved}
			}
		}
	}

	pkgs := make([]*packages.Package, 0, len(discovered)+len(noGoDirs))
	for _, item := range discovered {
		pkgs = append(pkgs, item.pkg)
	}
	slices.SortFunc(pkgs, func(a, b *packages.Package) int {
		return strings.Compare(a.ID, b.ID)
	})

	return pkgs, noGoDirs, nil
}

// hasToolScratchSegment reports whether any import-path segment begins with
// `_` or `.` — directories the go tool ignores for package resolution.
func hasToolScratchSegment(importPath string) bool {
	for _, seg := range strings.Split(importPath, "/") {
		if strings.HasPrefix(seg, "_") || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func stdlibLayout(sdk fs.FS, platform *Platform) (*Layout, error) {
	if platform == nil {
		return nil, errors.New("computing the whole-stdlib layout: a target platform is required")
	}
	pseudo := &Layout{Platform: platform}
	tags, err := BuildTagsForLayout(pseudo)
	if err != nil {
		return nil, fmt.Errorf("computing the whole-stdlib layout: %w", err)
	}
	pkgs, noGoDirs, err := discoverStdlibTree(sdk, tags)
	if err != nil {
		return nil, fmt.Errorf("computing the whole-stdlib layout: %w", err)
	}
	// Retain the standard library's source-less directories as package nodes:
	// `go list std` still lists a package whose every Go file the target's
	// constraints exclude (crypto/internal/fips140test, runtime/cgo with cgo
	// off, arena without its experiment), so the driver must be able to serve
	// its pattern — with no files, exactly as go list reports it.
	for _, dir := range noGoDirs {
		if hasNonTestGoSource(sdk, dir, tags) || !hasBuildableTestSource(sdk, dir, tags) {
			// A non-test directory whose every file the target's constraints
			// exclude is not listed by `go list std`; no node.
			continue
		}
		name, ok := noGoPackageName(sdk, dir)
		if !ok {
			continue
		}
		pkgs = append(pkgs, &packages.Package{
			ID:      dir,
			Name:    name,
			PkgPath: dir,
			Imports: make(map[string]*packages.Package),
		})
	}
	l := &Layout{
		Platform: platform,
		Roots:    make([]string, 0, len(pkgs)),
		Packages: pkgs,
	}
	// Roots are the importable packages: a source-less node is enumerated but
	// cannot be loaded or inventoried, so it is not a root.
	for _, p := range pkgs {
		if len(p.GoFiles) > 0 || len(p.CompiledGoFiles) > 0 {
			l.Roots = append(l.Roots, p.ID)
		}
	}
	sort.Strings(l.Roots)
	l.stdlibByID = make(map[string]bool, len(pkgs))
	l.stdlibByPath = make(map[string]bool, len(pkgs))
	l.emittedPackageID = make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		l.stdlibByID[p.ID] = true
		l.stdlibByPath[p.PkgPath] = true
		l.emittedPackageID[p.ID] = true
	}
	if err := validateAndResolveWithSDK(l, "", sdk); err != nil {
		return nil, fmt.Errorf("computing the whole-stdlib layout: %w", err)
	}
	return l, nil
}
