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

import (
	"fmt"
	"strings"
)

// CanonicalizePath maps a host/external package or import spelling into the
// loader's canonical package namespace. The shell routes every path that will
// be compared by the checker through this hook: package import paths, a
// package's direct imports, the packages of a resolved dependency interface,
// membership patterns,
// and capability-finding package paths.
//
// The default is the identity function, which is correct whenever the loader and
// the manifests already agree on a single namespace. A host that rewrites import
// paths overrides this with a total, idempotent function that maps every form a
// logical package may take (as reported by the loader, and as written in
// manifests) to one canonical string. Every package path reported by the loader
// must already be a fixed point: CanonicalizePath(loaderPath) == loaderPath.
// The loader enforces that invariant before it filters packages or constructs
// facts. Idempotence matters because a path may be canonicalized more than once
// as it flows through the shell.
//
// It must remain safe for concurrent reads; hosts set it once during init.
var CanonicalizePath = func(p string) string { return p }

// NamespaceID is the stable identifier of the package namespace the host's
// CanonicalizePath produces. It is process configuration: a host sets it once
// during init, alongside CanonicalizePath, and it must not change afterward.
// Future surface emitters record it on every emitted surface; future surface
// consumers compare it exactly (string equality) and reject artifacts from a
// different namespace with a tool error rather than comparing incompatible
// symbol paths.
//
// The default is "upstream": the namespace of paths produced by ordinary Go
// tooling, which CanonicalizePath leaves untouched. A host that rewrites
// import prefixes overrides this with its own stable ID.
var NamespaceID = "upstream"

// IsCanonicalPath reports whether a package path is already a fixed point of
// CanonicalizePath, i.e. CanonicalizePath(p) == p. The invariant that ties the
// two hooks together is:
//
//	IsCanonicalPath(p) == (CanonicalizePath(p) == p)
//
// and CanonicalizePath must be idempotent:
//
//	CanonicalizePath(CanonicalizePath(p)) == CanonicalizePath(p)
//
// for every accepted input. A host that overrides CanonicalizePath overrides
// this with a predicate identifying precisely the fixed points of its
// canonicalizer; a host whose hooks disagree must fix the hooks, not paper
// over the disagreement — ValidateCanonicalPath surfaces it as an error.
//
// The default agrees with the identity CanonicalizePath: every path is
// canonical. It must remain safe for concurrent reads; hosts set it once
// during init.
var IsCanonicalPath = func(p string) bool { return CanonicalizePath(p) == p }

// ValidateCanonicalPath checks that a candidate package path emitted by the
// loader or the build graph is canonical under the current host policy. Shell
// adapters call it before the path is compared against manifest data or used
// to construct facts. It returns a contextual error when the path is not a
// fixed point of CanonicalizePath or when the host's IsCanonicalPath predicate
// disagrees with the canonicalizer; it never rewrites the path and never
// repairs a disagreeing override.
func ValidateCanonicalPath(p string) error {
	if canon := CanonicalizePath(p); canon != p {
		return fmt.Errorf("loader path %q is not canonical: CanonicalizePath rewrites it to %q", p, canon)
	}
	if !IsCanonicalPath(p) {
		return fmt.Errorf("loader path %q is not canonical: reported non-canonical by IsCanonicalPath", p)
	}
	return nil
}

// ValidateStdlibPaths checks the namespace-free standard-library invariant on a
// caller-supplied list of package paths drawn from the stdlib map: for every
// supplied path, canonicalization must be the identity and IsCanonicalPath
// must hold. Hosts must never rewrite standard-library package paths, so a
// rewriting CanonicalizePath or a disagreeing IsCanonicalPath makes this
// validation fail, naming the offending path.
//
// The helper validates only the paths it is given; it performs no heuristic
// stdlib discovery or classification of its own.
func ValidateStdlibPaths(pkgPaths []string) error {
	for _, p := range pkgPaths {
		if canon := CanonicalizePath(p); canon != p || !IsCanonicalPath(p) {
			return fmt.Errorf("standard-library package path %q is not canonical under the current host policy: CanonicalizePath maps it to %q, IsCanonicalPath reports %v", p, canon, IsCanonicalPath(p))
		}
	}
	return nil
}

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
