// Package hostpolicy defines host-overridable policies for import-path
// canonicalization and standard-library classification.
//
// arcc's pure checker compares import paths by string equality and skips
// standard-library imports. That works directly when the paths produced by the
// loader (the shell) already share one namespace with the paths declared in
// manifests — which is the case for a normal go/packages load. A host that
// rewrites import paths at build time (for example, a monorepo that imports arcc
// under a different module prefix and a "doubled final segment" layout, and
// whose synthetic top-level segment has no dot) breaks both assumptions:
//   - the same logical package appears in two leaf forms across the loader, the
//     manifests, and the dependency-interface resolver, so string equality fails;
//   - the standard-library heuristic ("first path segment contains no dot")
//     misclassifies the host's rewritten paths as standard library.
//
// Rather than scatter reconciliation logic through the pure checker, the shell
// funnels every path it emits through CanonicalizePath, and classifies
// standard-library membership through IsStdlibPath, before the facts reach the
// checker. Upstream defaults reproduce the standard Go tooling behavior, so a
// plain go/packages build is unaffected. A host overrides these once at init to
// describe its own rewrite; see the exported vars for the contract.
package hostpolicy

import "strings"

// CanonicalizePath maps a raw package or import path to arcc's canonical
// comparison form. The shell routes every path that will be compared by the
// checker through this hook: package import paths, a package's direct imports,
// the packages of a resolved dependency interface, absorbed-dependency patterns,
// and capability-finding package paths.
//
// The default is the identity function, which is correct whenever the loader and
// the manifests already agree on a single namespace. A host that rewrites import
// paths overrides this with a total, idempotent function that maps every form a
// logical package may take (as reported by the loader, and as written in
// manifests) to one canonical string. Idempotence matters because a path may be
// canonicalized more than once as it flows through the shell.
//
// It must remain safe for concurrent reads; hosts set it once during init.
var CanonicalizePath = func(p string) string { return p }

// IsStdlibPath reports the host's path-policy verdict for whether an import path
// denotes a Go standard-library package. In package-layout mode this policy is
// checked against emitter-provided build-graph provenance. In native mode it is
// checked against module provenance for non-SDK packages. It remains the safe
// fallback only when no package reference exists.
//
// The default is the heuristic the go tool uses: the first path segment contains
// no dot (with the empty path treated as non-stdlib). A host whose rewritten
// package paths have a dotless first segment (and would thus be misread as
// standard library) overrides this to exclude its own namespace.
var IsStdlibPath = func(importPath string) bool {
	if importPath == "" {
		return false
	}
	first := importPath
	if i := strings.IndexByte(importPath, '/'); i >= 0 {
		first = importPath[:i]
	}
	return !strings.Contains(first, ".")
}
