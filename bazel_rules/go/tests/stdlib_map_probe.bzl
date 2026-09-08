"""Test-only wrappers around `arcc_stdlib_map` targets under different target
configurations, so SDK key propagation can be exercised on one execution host
(Step 4 task 06, reqs 5 and 8).

Each wrapper applies one transition to its `map` attribute and republishes
the built map with a test-local provider, mirroring probe.bzl's style: a
platforms option change for the cross-compile, the pure flag for the cgo
toggle, and the tags flag for a build-tag change.

The wrappers exist because rules_testing's `config_settings` resolves label
keys in rules_testing's own repository context, where @rules_go is not
visible; a transition in this repository is.
"""

load("//bazel_rules/go:providers.bzl", "ArccStdlibMapInfo")
load("//bazel_rules/go/private:stdlib_map.bzl", "stdlib_map_default_attr")

TransitionedStdlibMapInfo = provider(
    fields = ["toolchain_version", "goos", "goarch", "cgo_enabled", "build_tags", "goexperiment", "map"],
)

def _republish_impl(ctx):
    info = ctx.attr.map[0][ArccStdlibMapInfo]
    # Republish under the wrapper's own short_path: the map artifacts of
    # different configurations share one short_path (they are the same rule
    # output name in different configuration trees), and runfiles collapse on
    # short_path — a symlink per wrapper keeps them distinct.
    published = ctx.actions.declare_file(ctx.label.name + ".stdlib-map.json")
    ctx.actions.symlink(output = published, target_file = info.map)
    return [
        DefaultInfo(
            files = depset([published]),
            runfiles = ctx.runfiles(files = [published]),
        ),
        TransitionedStdlibMapInfo(
            toolchain_version = info.toolchain_version,
            goos = info.goos,
            goarch = info.goarch,
            cgo_enabled = info.cgo_enabled,
            build_tags = info.build_tags,
            goexperiment = info.goexperiment,
            map = published,
        ),
    ]

def _platforms_transition_impl(settings, attr):
    _ = settings, attr
    return {
        "//command_line_option:platforms": [
            "//bazel_rules/go/tests/testdata:darwin_arm64",
        ],
    }

_platforms_transition = transition(
    implementation = _platforms_transition_impl,
    inputs = [],
    outputs = ["//command_line_option:platforms"],
)

def _tagged_transition_impl(settings, attr):
    return {
        "@rules_go//go/config:tags": [attr.tag],
    }

_tagged_transition = transition(
    implementation = _tagged_transition_impl,
    inputs = [],
    outputs = ["@rules_go//go/config:tags"],
)

def _map_wrapper(transition, extra_attrs):
    return rule(
        implementation = _republish_impl,
        attrs = dict(extra_attrs) | {
            "map": attr.label(
                mandatory = True,
                providers = [ArccStdlibMapInfo],
                cfg = transition,
                doc = "The stdlib map target to build under the transitioned configuration.",
            ),
        },
    )

darwin_arm64_stdlib_map = _map_wrapper(_platforms_transition, {})
tagged_stdlib_map = _map_wrapper(_tagged_transition, {
    "tag": attr.string(
        mandatory = True,
        doc = "The build tag to add.",
    ),
})

# --- default-seam probe (task req 7) ----------------------------------------------

DefaultSeamProbeInfo = provider(
    fields = ["goos", "goarch", "map"],
)

def _default_seam_probe_impl(ctx):
    """The probe rule Step 5's analysis action will mirror: it consumes the
    private default `_stdlib_map` attribute and nothing else."""
    info = ctx.attr._stdlib_map[ArccStdlibMapInfo]
    return [DefaultSeamProbeInfo(
        goos = info.goos,
        goarch = info.goarch,
        map = info.map,
    )]

# The probe carries the default seam exactly as Step 5 will: one private
# attribute with the adapter's default target, no analysis action attached.
default_seam_probe = rule(
    implementation = _default_seam_probe_impl,
    attrs = {
        "_stdlib_map": stdlib_map_default_attr(),
    },
)
