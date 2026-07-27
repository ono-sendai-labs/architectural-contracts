"""Test-only rule that applies `arcc_deps_aspect` and republishes its closure.

`analysis_test` takes a plain label, so something has to attach the aspect for
it. This is the smallest thing that can.
"""

load("@rules_go//go:def.bzl", "GoInfo")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
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
