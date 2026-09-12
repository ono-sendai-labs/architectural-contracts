"""The single host adapter for the rules_arcc Bazel integration.

Everything arcc's generic rules need from a Go ruleset is funneled through this
file: ordinary package records, the three SDK contracts, provider forwarding,
runtime injection and infrastructure attachment. The SDK contracts are
deliberately separate:

* `go_stdlib_source_data` is source/oracle/root material for `arcc_stdlib_map`
  only. It is never an input to a component check.
* `go_stdlib_export_data` is compiled stdlib package metadata and export
  artifacts for `ArccCheck`. It contains no SDK source tree or toolchain binary.
* `go_target_identity` is the one target platform/SDK-key identity used by map
  configuration, layouts, export-data validation and surfaces. It describes the
  target, never the execution host.

`aspect.bzl`, `component.bzl`, `stdlib_map.bzl`, and the rest of the generic
rules import only this adapter and consume host-neutral structs, files, depsets,
and private rule attributes. A host using another Go ruleset ports arcc by
replacing this file; the generic rules stay byte-identical.

Upstream binds to rules_go (GoInfo / GoArchive / @rules_go//go:toolchain).

The platform seam returns target settings when the host exposes them. A host
whose Go providers expose no target-platform metadata may return a fixed
constant instead; that is conforming behavior, not a degraded fallback.
"""

load("@rules_go//go:def.bzl", "GoArchive", "GoInfo")
load("@rules_go//go/private:providers.bzl", "GoConfigInfo", "GoStdLib")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load(":paths.bzl", "runfiles_path")

# Providers a Go library target must carry to take part as a component
# interface or a closure node. Used in
# `attr.label(providers = ...)` on the rule attributes.
GO_PROVIDERS = [GoInfo, GoArchive]

# Toolchains the component rule requests, to reach the Go SDK.
GO_TOOLCHAINS = ["@rules_go//go:toolchain"]

def merge_private_rule_attrs(base, additions, contract):
    """Merges a host contract's private attrs without widening a rule API.

    Adapter-owned discovery attrs are composed at the rule boundary. A
    collision is a host-porting error rather than a last-write-wins override,
    and an unprefixed attr would become an author-facing API by accident.
    Keeping this check here lets both the map and component rules use the same
    contract without knowing anything about the provider implementation.
    """
    additions = additions or {}
    collisions = sorted([name for name in additions.keys() if name in base])
    if collisions:
        fail("adapter contract %s collides with existing rule attributes: %s" % (
            contract,
            ", ".join(collisions),
        ))
    public = sorted([name for name in additions.keys() if not name.startswith("_")])
    if public:
        fail("adapter contract %s may add only private rule attributes: %s" % (
            contract,
            ", ".join(public),
        ))
    merged = dict(base)
    merged.update(additions)
    return merged

# The arcc binary the component's analysis action and the check assertion rules
# run. Upstream builds it at the repo root;
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

def go_attach_infra(roots, infra, package_view = None):
    """Reports whether an infrastructure component should attach to `roots`.

    `roots` contains every root target of the component being wrapped: the
    interface, if present, plus all declared members. Layout-generation and
    attachment inputs use this union (M9), rather than one distinguished
    target. `infra` is one entry from `INFRA_COMPONENTS`. `package_view`, when
    supplied, is the canonical merged importpath -> package-record view for the
    component's ordinary and injected roots. A host may inspect either input
    when its analysis-phase graph exposes the relevant injected packages.
    Returning True unconditionally is conforming: an infra dependency is
    attached structurally when its package set is present in the component's
    graph, and a component's own authority is charged because its packages are
    roots. Hosts that cannot observe toolchain-injected packages during analysis
    can use the empty upstream default or a host-specific predicate.
    """
    _ = roots, infra, package_view
    return True

def runtime_injection_attrs(deps_aspect):
    """Returns private rule attrs for host-injected runtime targets.

    The upstream rules_go adapter has no hidden runtime targets, so this
    contract is deliberately empty. A host adapter may return attributes such
    as a private label list whose `aspects` contains `deps_aspect`; the caller
    supplies arcc's package-projecting aspect so the host does not need to
    reproduce provider traversal in the generic component rule. Returned
    attributes are merged into `go_component`'s private rule attrs with
    collision protection and are never part of the public authoring macro.

    Args:
      deps_aspect: the host-neutral dependency aspect that projects targets to
        ArccPackageInfo-compatible package records.

    Returns:
      A dictionary of private (`_`-prefixed) rule attributes, or `{}` upstream.
    """
    _ = deps_aspect
    return {}

def extra_runtime_packages(ctx, root_packages):
    """Returns host-injected package records for the effective package view.

    The upstream rules_go adapter exposes no injected runtime packages, so the
    default is `[]`. A host replacement may read its private attributes from
    `ctx` and project them to records with the same fields as
    `go_target_info`: `importpath`, `srcs`, `deps`, `cgo`, `export_file`, and
    `label`. `root_packages` contains the ordinary root projection and is an
    input snapshot: this function MUST NOT mutate it. Host output must be
    deterministic, or contain only metadata that `merge_by_importpath` can
    deterministically merge and validate.

    The component rule concatenates the returned records with the ordinary
    records and performs one merge before cgo validation, layout construction,
    infra attachment, membership classification, and export staging.
    """
    _ = ctx, root_packages
    return []

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
    return INFRA_COMPONENTS

def _canonical_target_label(label):
    """Returns the canonical textual identity of a Bazel target label."""
    return str(label)

def go_attached_infra(ctx, roots, infra_deps, registry = None, package_view = None):
    """Evaluates INFRA_COMPONENTS attachment against component roots.

    When `ctx` is present, only an infra candidate with the exact same
    canonical target label is exempted as the consuming component itself.
    Component display names are deliberately not used for this identity:
    different Bazel packages may publish the same short component name. The
    registry name remains lookup metadata, not a self-exemption convention.

    `package_view`, when present, is the already-merged package map supplied by
    the runtime-injection hook. It is passed to the host attachment predicate
    as its third argument; the first argument remains the original root-target
    list for compatibility with the M9 root contract.

    Returns a list of generic attached component records:
        struct(
            target = dep,
            info = dep[ArccComponentInfo],
            patterns = list_of_import_path_patterns,
        )
    """
    if registry == None:
        registry = go_infra_components(ctx)
    attached = []

    for dep in infra_deps:
        info = dep[ArccComponentInfo]
        if ctx != None and _canonical_target_label(dep.label) == _canonical_target_label(ctx.label):
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
            should_attach = attach_fn(roots, entry, package_view)
        else:
            should_attach = go_attach_infra(roots, entry, package_view)

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

    Returns a struct(importpath, srcs, deps, cgo, export_file, label). Host implementations MUST
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
      export_file File — the compiler export artifact for this package. With the
                        pinned upstream rules_go 0.61.1 provider this is exactly
                        `GoArchive.data.export_file`; hosts adapt their provider
                        shape here rather than in the aspect or component rule.
      label       string — the generic target label used for deterministic
                          fail-closed diagnostics when duplicate package metadata
                          is merged.
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

    # rules_go 0.61.1 publishes the archive consumed by dependents as this
    # provider field. Keep this ruleset-specific access in the adapter: upper
    # layers receive only the generic File contract above.
    export_file = target[GoArchive].data.export_file

    return struct(
        importpath = importpath,
        srcs = srcs,
        deps = deps,
        # cgo packages compile through preprocessed sources that are not
        # derivable at analysis time; the component rule refuses them rather
        # than emitting a layout that names the wrong files.
        cgo = getattr(go_info, "cgo", False),
        export_file = export_file,
        label = str(target.label),
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
    paths when a layout is used by a source-backed producer. The component check
    does not declare this source tree; its stdlib type information comes from
    `go_stdlib_export_data` below. Returned as a string so a host that knows its
    SDK location by convention can supply it directly, rather than deriving it
    from a toolchain-provided File in a generic rule.
    """
    root_file = _go_sdk_toolchain(ctx).root_file
    if root_file == None:
        fail("component %s: the Go adapter exposes no SDK root" % ctx.label.name)
    return _dirname(runfiles_path(ctx, root_file)) + "/src"

def go_sdk_srcs(ctx):
    """Compatibility accessor for the source-only SDK contract.

    New generic code should consume `go_stdlib_source_data(ctx)` so the source,
    oracle, root and target identity remain one descriptor. Component
    `ArccCheck` actions consume target-configured export data and never stage
    this depset.
    """
    return go_stdlib_source_data(ctx).srcs

# --- stdlib-map toolchain material (Step 4 task 06) ------------------------------
#
# The stdlib-map rule needs more of the toolchain than the check rule does:
# the pinned SDK sources, the toolchain-owned stdlib package enumeration, the
# exact toolchain version, and the target build configuration. Deliberately
# absent: the toolchain `go` binary and the tool binaries — the map action
# executes no toolchain binary at all (design I5); its generator loads the
# SDK through the layout driver. Everything below reads rules_go material so
# the layers above never load `@rules_go` (the adapter is the single seam).

# Rule attributes that carry the target build configuration. The two public
# contract-attribute functions below return private attrs for the map and
# component rules respectively. A host can extend either dictionary with its
# own private discovery attrs without editing those generic rules. The
# transition enables rules_go's generated export metadata without changing the
# target GOOS/GOARCH, cgo, tags, or experiment identity.
def _stdlib_export_data_transition_impl(settings, attr):
    _ = settings, attr
    return {
        "@rules_go//go/config:export_stdlib": True,
    }

_stdlib_export_data_transition = transition(
    implementation = _stdlib_export_data_transition_impl,
    inputs = [],
    outputs = ["@rules_go//go/config:export_stdlib"],
)

GO_CONTEXT_DATA_ATTRS = {
    "_go_context_data": attr.label(
        default = Label("@rules_go//:go_context_data"),
        cfg = _stdlib_export_data_transition,
        providers = [GoConfigInfo, GoStdLib],
        doc = "rules_go's build-configuration collector: supplies the target " +
              "GOOS/GOARCH/cgo/tags and generated stdlib export metadata.",
    ),
}

def sdk_source_attrs():
    """Returns private attrs needed by the SDK-source/map contract.

    Upstream uses the same target-configuration collector that supplies the
    target identity. A host with a different source oracle may add private
    discovery attrs here; `stdlib_map.bzl` merges them with collision checks.
    """
    return dict(GO_CONTEXT_DATA_ATTRS)

def sdk_export_data_attrs():
    """Returns private attrs needed by the SDK-export-data/check contract.

    The upstream default requests rules_go's target-configured stdlib export
    material. A host may replace or extend this private dictionary for its own
    export-data provider; `component.bzl` never names that provider or field.
    """
    return dict(GO_CONTEXT_DATA_ATTRS)

def _go_sdk_toolchain(ctx):
    """Returns the ruleset-specific SDK object inside this adapter only."""
    return ctx.toolchains[GO_TOOLCHAINS[0]].sdk

def _sdk_experiments(sdk):
    experiments = sdk.experiments or []
    if type(experiments) == type(""):
        experiments = [e for e in experiments.split(",") if e]
    return tuple(sorted(experiments))

def _normalize_toolchain_version(version):
    version = version or ""
    if version and not version.startswith("go"):
        return "go" + version
    return version

def _target_build_tags(identity):
    """Reads the canonical build_tags field, accepting the old `tags` alias."""
    build_tags = getattr(identity, "build_tags", None)
    if build_tags == None:
        build_tags = getattr(identity, "tags", None)
    if build_tags == None:
        return None
    return tuple(sorted(build_tags))

def target_identity_mismatches(actual, expected):
    """Returns target-identity fields that differ between two adapter records."""
    if actual == None:
        return ["target identity"]
    if expected == None:
        return []
    actual_tags = _target_build_tags(actual)
    expected_tags = _target_build_tags(expected)
    fields = [
        ("toolchain_version", getattr(actual, "toolchain_version", None), getattr(expected, "toolchain_version", None)),
        ("goos", getattr(actual, "goos", None), getattr(expected, "goos", None)),
        ("goarch", getattr(actual, "goarch", None), getattr(expected, "goarch", None)),
        ("cgo_enabled", getattr(actual, "cgo_enabled", None), getattr(expected, "cgo_enabled", None)),
        ("build_tags", actual_tags, expected_tags),
        ("goexperiment", getattr(actual, "goexperiment", None), getattr(expected, "goexperiment", None)),
    ]
    return [name for name, actual_value, expected_value in fields if actual_value != expected_value]

def validate_target_identity_match(ctx, actual, expected, material):
    """Fails analysis when two complete target identities disagree."""
    validate_target_identity(ctx, actual, material + " actual target")
    validate_target_identity(ctx, expected, material + " selected target")
    mismatches = target_identity_mismatches(actual, expected)
    if mismatches:
        fail(("component %s: %s mismatch; mismatched fields: %s") % (
            ctx.label.name,
            material,
            ", ".join(mismatches),
        ))
    return actual

def validate_target_identity(ctx, identity, material):
    """Fails analysis if `identity` is incomplete or not canonically sorted."""
    missing = []
    for field in ["toolchain_version", "goos", "goarch", "cgo_enabled", "goexperiment"]:
        if getattr(identity, field, None) == None:
            missing.append(field)
    tags = _target_build_tags(identity)
    if tags == None:
        missing.append("build_tags")
    elif tuple(getattr(identity, "build_tags", tags)) != tags:
        fail("%s %s: target identity build_tags must be sorted" % (ctx.label.name, material))
    if not getattr(identity, "toolchain_version", ""):
        missing.append("toolchain_version")
    if not getattr(identity, "goos", ""):
        missing.append("goos")
    if not getattr(identity, "goarch", ""):
        missing.append("goarch")
    if missing:
        fail("%s %s: incomplete target identity; missing fields: %s" % (
            ctx.label.name,
            material,
            ", ".join(sorted(set(missing))),
        ))
    return identity

def go_stdlib_toolchain(ctx):
    """Compatibility view of the source contract's toolchain material.

    `go_stdlib_source_data` is the canonical API. This accessor remains for
    hosts that used the pre-Step-12 adapter name, but generic rules no longer
    deconstruct the SDK provider themselves.
    """
    source = go_stdlib_source_data(ctx)
    return struct(
        srcs = source.srcs,
        package_list = source.package_list,
        root_file = source.root_file,
        version = source.version,
        experiments = source.experiments,
        sdk_root = source.sdk_root,
        target = source.target,
    )

def go_target_identity(ctx):
    """The complete target platform/SDK identity for this analysis.

    Read from rules_go's GoConfigInfo (collected by `@rules_go//:go_context_data`
    in this rule's configuration): goos/goarch follow toolchain resolution, so a
    platform transition yields the target platform, not the execution host.
    cgo_enabled = not pure mirrors the adapter's documented approximation. The
    identity includes the exact toolchain version, sorted build tags and
    GOEXPERIMENT so every producer uses one target description.
    """
    sdk = _go_sdk_toolchain(ctx)
    config = _go_context_data_target(ctx)[GoConfigInfo]
    version = _normalize_toolchain_version(sdk.version)
    tags = sorted(getattr(config, "tags", []) or [])
    identity = struct(
        toolchain_version = version,
        goos = config.goos,
        goarch = config.goarch,
        cgo_enabled = not config.pure,
        build_tags = tuple(tags),
        # Keep `tags` as a compatibility alias for test-only hosts while all
        # generic consumers use the canonical `build_tags` field.
        tags = tuple(tags),
        goexperiment = ",".join(_sdk_experiments(sdk)),
    )
    return validate_target_identity(ctx, identity, "target SDK identity")

def go_target_mode(ctx):
    """Compatibility alias for `go_target_identity`."""
    return go_target_identity(ctx)

def _go_context_data_target(ctx):
    """Returns the context-data target through direct and transitioned attrs."""
    context_data = ctx.attr._go_context_data
    if type(context_data) == type([]):
        if len(context_data) != 1:
            fail("component %s: Go adapter context-data transition returned %d targets" % (
                ctx.label.name,
                len(context_data),
            ))
        return context_data[0]
    return context_data

def go_stdlib_source_data(ctx):
    """Returns the source/oracle/root contract for `arcc_stdlib_map` only.

    `srcs` is the SDK source depset, `package_list` is the independent package
    oracle, `sdk_root` is the execroot-relative `src/` directory used by the
    layout driver, and `target` is the exact identity stamped into the map.
    No executable or compiler cache is part of this descriptor. The generic
    map rule declares only these files and never asks this contract for export
    data. Component checks use `go_stdlib_export_data` instead.
    """
    sdk = _go_sdk_toolchain(ctx)
    root_file = sdk.root_file
    source = struct(
        srcs = sdk.srcs,
        package_list = sdk.package_list,
        root_file = root_file,
        sdk_root = root_file.dirname + "/src" if root_file != None else "",
        layout_root = _dirname(runfiles_path(ctx, root_file)) + "/src" if root_file != None else "",
        version = sdk.version,
        experiments = _sdk_experiments(sdk),
        target = go_target_identity(ctx),
    )
    return validate_sdk_source_data(ctx, source)

def validate_sdk_source_data(ctx, source):
    """Fails analysis when source/oracle/root material is incomplete."""
    missing = []
    if source == None:
        missing.append("descriptor")
    else:
        if getattr(source, "srcs", None) == None or not source.srcs.to_list():
            missing.append("SDK sources")
        if getattr(source, "package_list", None) == None:
            missing.append("package oracle")
        if not getattr(source, "sdk_root", ""):
            missing.append("SDK root")
        if getattr(source, "target", None) == None:
            missing.append("target identity")
    if missing:
        fail("stdlib map %s: incomplete SDK source descriptor; missing %s" % (
            ctx.label.name,
            ", ".join(missing),
        ))
    validate_target_identity(ctx, source.target, "SDK source descriptor")
    return source

def _stdlib_mode_mismatches(actual, expected):
    """Compatibility alias for the one target-identity comparison."""
    return target_identity_mismatches(actual, expected)

def validate_sdk_export_data(ctx, descriptor, expected_mode = None):
    """Validates a host-neutral SDK export descriptor at the rule boundary.

    This validation is also applied to test-only or host-supplied descriptors,
    so a replacement cannot bypass the target-key check or silently omit the
    metadata/artifact inputs that `ArccCheck` must declare.
    """
    missing = []
    if descriptor == None:
        missing.append("descriptor")
    else:
        for field in ["metadata", "export_files", "inputs", "target"]:
            if getattr(descriptor, field, None) == None:
                missing.append(field)
    if missing:
        fail("component %s: incomplete SDK export-data descriptor; missing %s" % (
            ctx.label.name,
            ", ".join(missing),
        ))

    validate_target_identity(ctx, descriptor.target, "SDK export-data descriptor")
    if expected_mode != None:
        mismatches = target_identity_mismatches(descriptor.target, expected_mode)
        if mismatches:
            fail(("component %s: standard-library export data configuration mismatch " +
                  "with the selected authority map; mismatched fields: %s") % (
                ctx.label.name,
                ", ".join(mismatches),
            ))

    export_files = descriptor.export_files.to_list()
    if not export_files:
        fail("component %s: SDK export-data descriptor has no compiled export artifacts" % ctx.label.name)
    descriptor_inputs = descriptor.inputs.to_list()
    forbidden_source_inputs = [
        file.short_path
        for file in descriptor_inputs
        if file.extension == "go" or "/src/" in file.short_path or
           "/pkg/tool/" in file.short_path or file.short_path.endswith("/bin/go")
    ]
    if forbidden_source_inputs:
        fail(("component %s: SDK export-data descriptor includes forbidden source material " +
              "or toolchain binary: %s") % (
            ctx.label.name,
            ", ".join(sorted(forbidden_source_inputs)),
        ))
    input_paths = {file.path: True for file in descriptor_inputs}
    required_paths = [descriptor.metadata.path] + [file.path for file in export_files]
    missing_inputs = [path for path in required_paths if path not in input_paths]
    if missing_inputs:
        fail(("component %s: SDK export-data descriptor omits declared material from " +
              "its inputs: %s") % (ctx.label.name, ", ".join(sorted(missing_inputs))))
    return descriptor

def go_stdlib_export_data(ctx, expected_mode = None):
    """Returns the host-neutral target stdlib export-data descriptor.

    The upstream ruleset stores the package graph and each package's direct
    import edges in its generated `_list_json` artifact. With export generation
    enabled, the JSON's `ExportFile` values point into the generated cache and
    compiled stdlib archive tree. The returned descriptor deliberately exposes
    only those generated artifacts:

      metadata    File — package identities, direct imports, and ExportFile paths;
      export_files depset[File] — generated cache/archive trees containing those
                  compiler export artifacts;
      inputs      depset[File] — exactly metadata plus export_files, for a check
                  action to declare without pulling in SDK sources or tools;
      target      struct — the same complete target identity returned by
                  go_target_identity.

    `expected_mode`, when supplied, is any record carrying the SDK-key fields
    (the stdlib-map provider is one such record). The comparison happens during
    analysis and names the consuming target on mismatch; this prevents a
    transitioned component from pairing one target's layout with another
    target's stdlib data. No toolchain executable is returned or run here.
    """
    target_mode = go_target_identity(ctx)

    context_data = _go_context_data_target(ctx)
    if GoStdLib not in context_data:
        fail(("component %s: the Go adapter context provides no standard-library " +
              "export-data descriptor") % ctx.label.name)
    config = context_data[GoConfigInfo]
    if not getattr(config, "export_stdlib", False):
        fail(("component %s: GoStdLib does not provide target-configured export " +
              "metadata; enable rules_go export_stdlib for this target") % ctx.label.name)

    stdlib = context_data[GoStdLib]
    metadata = getattr(stdlib, "_list_json", None)
    cache_dir = getattr(stdlib, "cache_dir", None)
    libs = getattr(stdlib, "libs", None)
    missing = []
    if metadata == None:
        missing.append("package metadata")
    if cache_dir == None and libs == None:
        missing.append("compiled export artifacts")
    if missing:
        fail(("component %s: GoStdLib is missing target-configured export material: %s") % (
            ctx.label.name,
            ", ".join(missing),
        ))

    export_depsets = [artifact for artifact in [cache_dir, libs] if artifact != None]
    export_files = depset(transitive = export_depsets)
    if not export_files.to_list():
        fail(("component %s: GoStdLib exposes no compiled standard-library " +
              "export artifacts") % ctx.label.name)

    return validate_sdk_export_data(ctx, struct(
        metadata = metadata,
        export_files = export_files,
        inputs = depset(direct = [metadata], transitive = [export_files]),
        target = target_mode,
    ), expected_mode = expected_mode)
