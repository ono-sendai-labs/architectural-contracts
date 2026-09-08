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
    "INFRA_COMPONENTS",
    "go_attach_infra",
    "go_build_platform",
)

ArccGoPlatformInfo = provider(
    fields = ["goos", "goarch", "tags", "cgo_enabled", "infra_attached", "registry"],
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
