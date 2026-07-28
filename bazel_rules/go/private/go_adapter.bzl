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

The platform seam returns target settings when the host exposes them. A host
whose Go providers expose no target-platform metadata may return a fixed
constant instead; that is conforming behavior, not a degraded fallback.
"""

load("@rules_go//go:def.bzl", "GoArchive", "GoInfo")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load(":paths.bzl", "match_path", "runfiles_path")

# Providers a Go library target must carry to take part as a component
# interface, an absorbed dependency, or a closure node. Used in
# `attr.label(providers = ...)` on the rule attributes.
GO_PROVIDERS = [GoInfo, GoArchive]

# Toolchains the component rule requests, to reach the Go SDK.
GO_TOOLCHAINS = ["@rules_go//go:toolchain"]

# The arcc binary that arcc_check_test runs. Upstream builds it at the repo root;
# a host that builds arcc under a different label overrides this in its adapter,
# so check.bzl stays byte-identical across hosts.
ARCC_TARGET = "//:arcc"

def is_go_target(target):
    """Reports whether target is a Go library arcc can project onto a closure node.

    A dependency edge can point at a filegroup, a proto target, or anything else
    that is not a Go library; those are skipped rather than failed.
    """
    return GoInfo in target and GoArchive in target

def go_importpath(target):
    """The import path of a Go library target ("" for main/unimportable libraries)."""
    return target[GoInfo].importpath

def go_build_platform(target):
    """Returns GOOS/GOARCH/tags/cgo for the target being analyzed.

    rules_go stores target settings in GoInfo.mode. GoSDK.goos and GoSDK.goarch
    describe the execution host and must not be used here. rules_go's pure
    setting means that cgo is unavailable, so cgo_enabled = not pure is an
    approximation: it does not prove that a C compiler is available.

    A conforming host whose Go providers expose no target-platform metadata may
    return a fixed constant. The caller must treat that as the host contract,
    not as a degraded best-effort result.
    """
    mode = target[GoInfo].mode
    return struct(
        goos = mode.goos,
        goarch = mode.goarch,
        tags = tuple(mode.tags),
        cgo_enabled = not mode.pure,
    )

def go_attach_infra(target, infra):
    """Reports whether an infrastructure component should attach to `target`.

    `target` is the Go target being wrapped and `infra` is one entry from
    `INFRA_COMPONENTS`. A host may inspect either input when its analysis-phase
    graph exposes the relevant injected packages. Returning True
    unconditionally is also conforming: hosts that cannot observe
    toolchain-injected packages during analysis cannot evaluate a closure-based
    predicate for those packages, and package-level pruning is a no-op until
    the package is reached.
    """
    return True

# Infrastructure components are intentionally empty for the upstream ruleset.
# Hosts replace this registry with entries shaped like the examples below.
# A concrete component label can be attached when the infrastructure target is
# addressable:
#
#   struct(
#       name = "runtime",
#       component = "//toolchain/runtime:component",
#       import_path_patterns = [],
#   )
#
# A visibility-gated runtime can instead keep its component target addressable
# while describing the runtime's inaccessible packages by import-path pattern:
#
#   struct(
#       name = "injected_runtime",
#       component = "//toolchain/runtime:component",
#       import_path_patterns = ["example.com/toolchain/runtime/*"],
#   )
#
# Here `component` is the target attached by Step 9, while
# `import_path_patterns` identifies the package-surface membership that cannot
# be named as labels because of visibility. `name`, `component`, and
# `import_path_patterns` are the fields Step 9 uses to name the attached
# component and its package-surface membership. Keeping the examples commented
# means existing upstream components acquire no new dependency until a host
# supplies a real registry entry.
INFRA_COMPONENTS = []

def go_infra_deps():
    """Returns the list of component labels from INFRA_COMPONENTS."""
    deps = []
    for entry in INFRA_COMPONENTS:
        if hasattr(entry, "component") and entry.component:
            deps.append(entry.component)
    return deps

def go_infra_components(ctx = None):
    """Returns the infra component registry entries."""
    if ctx != None and hasattr(ctx.attr, "test_infra_attach") and ctx.attr.test_infra_attach:
        patterns = getattr(ctx.attr, "test_infra_patterns", [])
        attach_mode = ctx.attr.test_infra_attach
        deps = getattr(ctx.attr, "infra_deps", [])

        def _test_predicate(roots, entry):
            if attach_mode == "ALWAYS":
                return True
            if attach_mode == "NEVER":
                return False
            if attach_mode == "CLOSURE":
                search_patterns = getattr(entry, "import_path_patterns", [])
                if not search_patterns:
                    search_patterns = ["*runtime*", "*injected*", "*member*"]
                for root in roots:
                    if ArccPackageInfo in root:
                        for pkg in root[ArccPackageInfo].packages.to_list():
                            for p in search_patterns:
                                if match_path(p, pkg.importpath):
                                    return True
                return False
            return True

        entries = []
        for dep in deps:
            comp_name = dep[ArccComponentInfo].component_name
            entries.append(struct(
                name = comp_name,
                component = str(dep.label),
                import_path_patterns = patterns,
                attach_predicate = _test_predicate,
            ))
        return entries

    if ctx != None and hasattr(ctx.attr, "infra_components") and ctx.attr.infra_components:
        return ctx.attr.infra_components
    return INFRA_COMPONENTS

def go_attached_infra(ctx, roots, infra_deps):
    """Evaluates INFRA_COMPONENTS attachment against component roots.

    Returns a list of generic attached component records:
        struct(
            target = dep,
            info = dep[ArccComponentInfo],
            patterns = list_of_import_path_patterns,
        )
    """
    registry = go_infra_components(ctx)
    attached = []

    for dep in infra_deps:
        info = dep[ArccComponentInfo]
        if ctx != None and info.component_name == ctx.label.name:
            continue

        entry = None
        for reg in registry:
            reg_comp = getattr(reg, "component", None)
            if reg_comp != None:
                if str(reg_comp) == str(dep.label) or (hasattr(reg_comp, "name") and reg_comp == dep.label):
                    entry = reg
                    break
            if getattr(reg, "name", None) == info.component_name:
                entry = reg
                break

        if entry == None:
            entry = struct(
                name = info.component_name,
                component = str(dep.label),
                import_path_patterns = [],
            )

        attach_fn = getattr(entry, "attach_predicate", None)
        if attach_fn != None:
            should_attach = attach_fn(roots, entry)
        else:
            should_attach = go_attach_infra(roots, entry)

        if should_attach:
            patterns = getattr(entry, "import_path_patterns", [])
            attached.append(struct(
                target = dep,
                info = info,
                patterns = patterns,
            ))

    return attached

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
      srcs        tuple[File] — the package's Go source set, already embed-merged.
                  This may be the full DECLARED set rather than the per-platform
                  compiled subset: rules_go's GoInfo.srcs, for instance, lists
                  every _GOOS.go variant, because its compiler (not the provider)
                  applies build constraints. That is fine — arcc's layout loader
                  filters the sources by build constraint (GOOS/GOARCH filename
                  suffixes and //go:build lines) for the target platform before
                  type-checking, so a host need not pre-filter. A host MAY pass an
                  already-filtered set; the loader's pass is then a no-op.
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

def _dirname(path):
    if "/" not in path:
        return ""
    return path.rsplit("/", 1)[0]

def go_sdk_root(ctx):
    """The `go_sdk_root` value for the emitted layout: a runfiles-root-relative
    path to the Go SDK's `src` directory, from which arcc reads standard-library
    sources hermetically. Returned as a string so a host that knows its SDK
    location by convention can supply it directly, rather than deriving it from a
    toolchain-provided File.
    """
    root_file = ctx.toolchains[GO_TOOLCHAINS[0]].sdk.root_file
    return _dirname(runfiles_path(ctx, root_file)) + "/src"

def go_sdk_srcs(ctx):
    """The Go SDK source files to stage into the `.check` sandbox, as a depset.

    The layout names standard-library packages by path and arcc type-checks the
    closure from their sources, so they must be present at check time. A host
    whose build already makes the SDK sources available (e.g. via the go_sdk_root
    location) may return an empty depset.
    """
    return ctx.toolchains[GO_TOOLCHAINS[0]].sdk.srcs
