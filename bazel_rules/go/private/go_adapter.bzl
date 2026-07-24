"""go_adapter: the single seam binding arcc's Bazel rules to a Go ruleset.

Everything arcc's rules need from the host's Go rules is funneled through this
one file: the providers it keys on, how to read a target's import path, its
compiled sources, its direct-dependency import paths and cgo flag, how to
forward a library's Go providers so a component target can stand in for its
interface library, and how to locate the Go SDK root. `aspect.bzl`,
`component.bzl`, and the rest of the rules import ONLY this adapter and never
`@rules_go` directly, so a host with a different Go ruleset (e.g. a monorepo's
in-house rules exposing different providers) ports arcc by replacing just this
file — the rules above it stay byte-identical.

Upstream binds to rules_go (GoInfo / GoArchive / @rules_go//go:toolchain).
"""

load("@rules_go//go:def.bzl", "GoArchive", "GoInfo")

# Providers a Go library target must carry to take part as a component
# interface, an absorbed dependency, or a closure node. Used in
# `attr.label(providers = ...)` on the rule attributes.
GO_PROVIDERS = [GoInfo, GoArchive]

# Toolchains the component rule requests, to reach the Go SDK.
GO_TOOLCHAINS = ["@rules_go//go:toolchain"]

def is_go_target(target):
    """Reports whether target is a Go library arcc can project onto a closure node.

    A dependency edge can point at a filegroup, a proto target, or anything else
    that is not a Go library; those are skipped rather than failed.
    """
    return GoInfo in target and GoArchive in target

def go_importpath(target):
    """The import path of a Go library target ("" for main/unimportable libraries)."""
    return target[GoInfo].importpath

def go_library_srcs(target):
    """The compiled, build-constraint-filtered sources of a Go library target.

    Same filtering contract as `go_target_info().srcs` below.
    """
    return tuple(target[GoInfo].srcs)

def go_target_info(target):
    """Projects a Go library target onto a closure node, or None if it is not one.

    Returns a struct(importpath, srcs, deps, cgo). Host implementations MUST
    honor this contract:

      importpath  string; "" is impossible here (None is returned instead), so
                  callers get either a real import path or nothing.
      srcs        tuple[File] — the COMPILED, build-constraint-filtered source
                  set for the target platform, already embed-merged. It MUST NOT
                  include sources excluded by build constraints (GOOS/GOARCH
                  filename suffixes or //go:build lines): arcc writes exactly
                  these files into the hermetic package layout and type-checks
                  them, so a superset pulls in wrong-platform files, makes the
                  package IllTyped, and silently degrades the whole analysis to a
                  pass. Filtering is the host ruleset's job (rules_go's GoInfo.srcs
                  already is the compiled set); the adapter only forwards it.
      deps        tuple[string] — direct-dependency import paths, sorted, with the
                  standard library excluded (arcc handles stdlib authority itself)
                  and the target's own import path excluded (an embedded library
                  shares its embedder's import path).
      cgo         bool — whether the package is built with cgo.
    """
    go_info = target[GoInfo]
    importpath = go_info.importpath
    if not importpath:
        # `main` packages and other unimportable libraries are never closure
        # members: nothing can depend on them by import path.
        return None

    # GoInfo.srcs is already embed-merged by rules_go, so this must not also walk
    # `embed` to collect sources — that would double-count them.
    srcs = tuple(go_info.srcs)

    deps = tuple(sorted([
        archive.data.importpath
        for archive in target[GoArchive].direct
        if archive.data.importpath != importpath
    ]))

    return struct(
        importpath = importpath,
        srcs = srcs,
        deps = deps,
        # cgo packages compile through preprocessed sources that are not
        # derivable at analysis time; the component rule refuses them rather
        # than emitting a layout that names the wrong files.
        cgo = getattr(go_info, "cgo", False),
    )

def forward_go_providers(target):
    """The Go providers to forward so a wrapper target stands in for `target`.

    Returned as a list to be spread into a rule's provider list, so
    `deps = [":some_component"]` works wherever a Go library is expected.
    """
    return [target[GoInfo], target[GoArchive]]

def go_sdk_root_file(ctx):
    """A File under the Go SDK root, used to derive the SDK `src` path.

    The component rule takes this file's directory (+ "/src") as `go_sdk_root`
    in the emitted layout, so arcc can find standard-library sources hermetically.
    """
    return ctx.toolchains[GO_TOOLCHAINS[0]].sdk.root_file

def go_sdk_srcs(ctx):
    """The Go SDK source files as a depset.

    The `.check` test stages these into the sandbox: the layout names standard-
    library packages by path, and arcc type-checks the closure from their sources.
    """
    return ctx.toolchains[GO_TOOLCHAINS[0]].sdk.srcs
