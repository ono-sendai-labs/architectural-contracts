// Package hostpolicy defines the host-overridable policy for import-path
// canonicalization and namespace validation.
//
// arcc's checker compares import paths by string equality. That works directly
// when the paths produced by the loader (the shell) already share one namespace
// with the paths declared in manifests — which is the case for a normal
// go/packages load. A host that rewrites import paths at build time (for example,
// a monorepo that imports arcc under a different module prefix and a "doubled
// final segment" layout) must describe that namespace once here.
//
// Rather than scatter reconciliation logic through the pure checker, the shell
// funnels every path it emits through CanonicalizePath before facts reach the
// checker. Standard-library membership is deliberately not a host-policy
// decision: the total StdlibAuthority map is the sole check-time definition.
// Upstream defaults reproduce ordinary Go tooling, and a host overrides the
// namespace hooks once at init to describe its own rewrite.
package hostpolicy

import (
	"fmt"
)

// CanonicalizePath maps a host/external package or import spelling into the
// loader's canonical package namespace. The shell routes every path that will
// be compared by the checker through this hook: package import paths, a
// package's direct imports, the packages of a resolved dependency interface,
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
// Every surface emitter records it on the emitted surface; consumers compare it
// exactly (string equality) and reject artifacts from a different namespace
// with a tool error rather than comparing incompatible symbol paths.
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
