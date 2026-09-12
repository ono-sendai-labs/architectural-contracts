"""Test-only rule that applies `arcc_deps_aspect` and republishes its closure.

`analysis_test` takes a plain label, so something has to attach the aspect for
it. This is the smallest thing that can.
"""

load("@rules_go//go:def.bzl", "GoInfo")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo", "ArccStdlibMapInfo")
load("//bazel_rules/go/private:aspect.bzl", "arcc_deps_aspect")
load(
    "//bazel_rules/go/private:go_adapter.bzl",
    "GO_CONTEXT_DATA_ATTRS",
    "GO_TOOLCHAINS",
    "INFRA_COMPONENTS",
    "go_attach_infra",
    "go_build_platform",
    "go_stdlib_export_data",
    "go_stdlib_export_data_unprojected",
    "sdk_source_attrs",
    "validate_sdk_source_data",
)

ArccGoPlatformInfo = provider(
    fields = ["goos", "goarch", "tags", "cgo_enabled", "infra_attached", "registry"],
)

StdlibExportDataInfo = provider(
    fields = ["metadata", "export_files", "inputs", "target"],
)

def _fake_component_info_impl(ctx):
    """Publishes deliberately malformed provider shapes for fail-closed tests."""
    surface = None
    report = None
    outputs = []
    if ctx.attr.mode != "missing_surface":
        surface = ctx.actions.declare_file(ctx.label.name + ".surface.json")
        ctx.actions.write(output = surface, content = "{}\n")
        outputs.append(surface)
    if ctx.attr.mode in ["checked", "asserted_with_report", "conflict"]:
        report = ctx.actions.declare_file(ctx.label.name + ".report.json")
        ctx.actions.write(output = report, content = "{}\n")
        outputs.append(report)

    provenance = {
        "checked": "checked",
        "checked_without_report": "checked",
        "asserted": "asserted",
        "asserted_with_report": "asserted",
        "conflict": "checked",
        "unknown": "future",
        "missing_surface": "checked",
    }.get(ctx.attr.mode, "future")
    return [
        DefaultInfo(files = depset(outputs)),
        ArccComponentInfo(
            component_name = ctx.attr.component_name,
            component_root = ctx.label.package,
            manifest = None,
            layout = None,
            transitive_manifests = depset(),
            transitive_layouts = depset(),
            transitive_artifacts = depset(),
            closure = depset(),
            contracts = depset(),
            surface = surface,
            report = report,
            provenance = provenance,
        ),
    ]

fake_component_info = rule(
    implementation = _fake_component_info_impl,
    attrs = {
        "component_name": attr.string(mandatory = True),
        "mode": attr.string(mandatory = True),
    },
    provides = [ArccComponentInfo],
    doc = "Test-only malformed ArccComponentInfo provider shape.",
)

def _darwin_arm64_transition_impl(settings, attr):
    return {
        "//command_line_option:platforms": [
            "//bazel_rules/go/tests/testdata:darwin_arm64",
        ],
        "@rules_go//go/config:tags": ["adapter_probe"],
        "@rules_go//go/config:pure": True,
    }

_darwin_arm64_transition = transition(
    implementation = _darwin_arm64_transition_impl,
    inputs = [],
    outputs = [
        "//command_line_option:platforms",
        "@rules_go//go/config:pure",
        "@rules_go//go/config:tags",
    ],
)

def _arcc_closure_probe_impl(ctx):
    return [ctx.attr.interface[ArccPackageInfo]]

arcc_closure_probe = rule(
    implementation = _arcc_closure_probe_impl,
    attrs = {
        "interface": attr.label(
            mandatory = True,
            providers = [GoInfo],
            aspects = [arcc_deps_aspect],
            doc = "The go_library whose closure to collect.",
        ),
    },
    provides = [ArccPackageInfo],
    doc = "Exposes the arcc package closure of a go_library for analysis tests.",
)

def _arcc_non_go_closure_probe_impl(ctx):
    return [ctx.attr.target[ArccPackageInfo]]

arcc_non_go_closure_probe = rule(
    implementation = _arcc_non_go_closure_probe_impl,
    attrs = {
        "target": attr.label(
            mandatory = True,
            aspects = [arcc_deps_aspect],
            doc = "A non-Go target whose aspect-applied provider to expose.",
        ),
    },
    provides = [ArccPackageInfo],
    doc = "Exposes the arcc provider a non-Go target receives from arcc_deps_aspect.",
)

def _go_platform_probe_impl(ctx):
    target = ctx.attr.target[0]
    platform = go_build_platform(target)
    infra = struct(
        name = "probe_runtime",
        component = "//probe:runtime_component",
        import_path_patterns = [],
    )
    return [ArccGoPlatformInfo(
        goos = platform.goos,
        goarch = platform.goarch,
        tags = platform.tags,
        cgo_enabled = platform.cgo_enabled,
        infra_attached = go_attach_infra(target, infra),
        registry = INFRA_COMPONENTS,
    )]

go_platform_probe = rule(
    implementation = _go_platform_probe_impl,
    attrs = {
        "target": attr.label(
            mandatory = True,
            providers = [GoInfo],
            cfg = _darwin_arm64_transition,
            doc = "A Go target configured for a target platform distinct from the SDK host.",
        ),
    },
    doc = "Exposes adapter platform and infrastructure-seam results for analysis tests.",
)

def _stdlib_export_data_probe_impl(ctx):
    data = go_stdlib_export_data(
        ctx,
        expected_mode = ctx.attr.map[ArccStdlibMapInfo],
    )
    return [StdlibExportDataInfo(
        metadata = data.metadata,
        export_files = data.export_files,
        inputs = data.inputs,
        target = data.target,
    )]

stdlib_export_data_probe = rule(
    implementation = _stdlib_export_data_probe_impl,
    attrs = {} | GO_CONTEXT_DATA_ATTRS | {
        "map": attr.label(
            default = "//:arcc_stdlib_map",
            providers = [ArccStdlibMapInfo],
            doc = "The authority map whose target identity must match the export data.",
        ),
    },
    toolchains = GO_TOOLCHAINS,
    provides = [StdlibExportDataInfo],
    doc = "Exposes the host-neutral stdlib export-data adapter contract for analysis tests.",
)

def _stdlib_export_data_unprojected_consumer_probe_impl(ctx):
    # The rules_go provider intentionally has export material here, but this
    # consumer has no projected map descriptor. The public adapter must fail
    # closed rather than handing the raw cache tree to a caller.
    go_stdlib_export_data(ctx)
    return []

stdlib_export_data_unprojected_consumer_probe = rule(
    implementation = _stdlib_export_data_unprojected_consumer_probe_impl,
    attrs = {} | GO_CONTEXT_DATA_ATTRS,
    toolchains = GO_TOOLCHAINS,
    doc = "Negative probe proving public export consumers cannot receive the raw tree.",
)

def _stdlib_export_data_missing_probe_impl(ctx):
    go_stdlib_export_data(ctx)
    return []

stdlib_export_data_missing_probe = rule(
    implementation = _stdlib_export_data_missing_probe_impl,
    attrs = {
        # Deliberately omit the adapter's export-enabling transition. The
        # provider is otherwise the same rules_go context target, so the
        # adapter must diagnose missing target-configured export material.
        "_go_context_data": attr.label(
            default = Label("@rules_go//:go_context_data"),
        ),
    },
    toolchains = GO_TOOLCHAINS,
    doc = "Test-only missing-stdlib-export-material seam.",
)

def _stdlib_export_data_mismatch_probe_impl(ctx):
    go_stdlib_export_data_unprojected(
        ctx,
        expected_mode = ctx.attr.map[0][ArccStdlibMapInfo],
    )
    return []

stdlib_export_data_mismatch_probe = rule(
    implementation = _stdlib_export_data_mismatch_probe_impl,
    attrs = {} | GO_CONTEXT_DATA_ATTRS | {
        "map": attr.label(
            mandatory = True,
            providers = [ArccStdlibMapInfo],
            cfg = _darwin_arm64_transition,
            doc = "A deliberately transitioned map identity for the negative seam.",
        ),
    },
    toolchains = GO_TOOLCHAINS,
    doc = "Test-only mismatched-stdlib-export-configuration seam.",
)

def _stdlib_source_data_missing_probe_impl(ctx):
    validate_sdk_source_data(ctx, struct(
        srcs = depset([ctx.file.sentinel]),
        package_list = None,
        sdk_root = "",
        target = None,
    ))
    return []

stdlib_source_data_missing_probe = rule(
    implementation = _stdlib_source_data_missing_probe_impl,
    attrs = sdk_source_attrs() | {
        "sentinel": attr.label(
            mandatory = True,
            allow_single_file = True,
            doc = "Test-only source descriptor sentinel.",
        ),
    },
    toolchains = GO_TOOLCHAINS,
    doc = "Test-only incomplete SDK source descriptor seam.",
)

def _transitioned_checked_map_impl(ctx):
    map_published = ctx.actions.declare_file(ctx.label.name + ".stdlib-map.json")
    ctx.actions.symlink(output = map_published, target_file = ctx.attr.map[0][ArccStdlibMapInfo].map)
    return [
        DefaultInfo(
            files = depset([map_published]),
            runfiles = ctx.runfiles(files = [map_published]),
        ),
    ]

transitioned_checked_map = rule(
    implementation = _transitioned_checked_map_impl,
    attrs = {
        "map": attr.label(
            default = "//:arcc_stdlib_map",
            providers = [ArccStdlibMapInfo],
            cfg = _darwin_arm64_transition,
            doc = "The stdlib map to build in the darwin/arm64 transition configuration.",
        ),
    },
    doc = "Republishes the stdlib map of the darwin/arm64 transition configuration.",
)

def _transitioned_component_layout_impl(ctx):
    info = ctx.attr.component[0][ArccComponentInfo]
    return [
        DefaultInfo(
            files = depset([info.layout]),
            runfiles = ctx.runfiles(files = [info.layout]),
        ),
    ]

transitioned_component_layout = rule(
    implementation = _transitioned_component_layout_impl,
    attrs = {
        "component": attr.label(
            mandatory = True,
            providers = [ArccComponentInfo],
            cfg = _darwin_arm64_transition,
            doc = "The component to build in darwin/arm64 configuration.",
        ),
    },
)

def _checked_analysis_surface_impl(ctx):
    info = ctx.attr.component[ArccComponentInfo]
    # Republish under the wrapper's own short_path: the surfaces of different
    # configurations share one short_path (same rule output name in different
    # configuration trees) and runfiles collapse on short_path.
    published = ctx.actions.declare_file(ctx.label.name + ".surface.json")
    ctx.actions.symlink(output = published, target_file = info.surface)
    return [
        DefaultInfo(
            files = depset([published]),
            runfiles = ctx.runfiles(files = [published]),
        ),
    ]

checked_analysis_surface = rule(
    implementation = _checked_analysis_surface_impl,
    attrs = {
        "component": attr.label(
            mandatory = True,
            providers = [ArccComponentInfo],
            doc = "The checked component whose surface to republish as a data label.",
        ),
    },
    doc = "Republishes a component's checked surface so sh_tests can take it as data.",
)

def _transitioned_checked_surface_impl(ctx):
    info = ctx.attr.component[0][ArccComponentInfo]
    published = ctx.actions.declare_file(ctx.label.name + ".surface.json")
    ctx.actions.symlink(output = published, target_file = info.surface)
    return [
        DefaultInfo(
            files = depset([published]),
            runfiles = ctx.runfiles(files = [published]),
        ),
    ]

transitioned_checked_surface = rule(
    implementation = _transitioned_checked_surface_impl,
    attrs = {
        "component": attr.label(
            mandatory = True,
            providers = [ArccComponentInfo],
            cfg = _darwin_arm64_transition,
            doc = "The component to analyze in darwin/arm64 configuration.",
        ),
    },
    doc = "Republishes a transitioned component's checked surface (its ArccCheck " +
          "action runs under the transition, with the transition's stdlib map — " +
          "see transitioned_checked_map for that map).",
)
