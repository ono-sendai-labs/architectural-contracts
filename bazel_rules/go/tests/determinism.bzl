"""Test-only provider for reusing the checked routine stdlib-map artifact.

The routine determinism harness executes the real default map once in its
shared producer-chain invocation. Both isolated determinism output bases then
consume this checked canonical artifact with the same complete SDK-key
metadata while rebuilding all component producers. This is the bounded-map
reuse permitted by design N5; the full lane still executes two independent
real generations.
"""

load("//bazel_rules/go:providers.bzl", "ArccStdlibMapInfo")
load("//bazel_rules/go/private:arcc_metadata.bzl", "CLASSIFIER_HASH", "MAP_FORMAT_VERSION")

def _pinned_stdlib_map_impl(ctx):
    output = ctx.actions.declare_file(ctx.label.name + ".stdlib-map.json")
    ctx.actions.symlink(output = output, target_file = ctx.file.src)
    return [
        DefaultInfo(
            files = depset([output]),
            runfiles = ctx.runfiles(files = [output]),
        ),
        ArccStdlibMapInfo(
            map = output,
            toolchain_version = "go1.26.4",
            goos = "linux",
            goarch = "amd64",
            cgo_enabled = False,
            build_tags = (),
            goexperiment = "",
            classifier_hash = CLASSIFIER_HASH,
            map_format_version = MAP_FORMAT_VERSION,
        ),
    ]

pinned_stdlib_map = rule(
    implementation = _pinned_stdlib_map_impl,
    attrs = {
        "src": attr.label(
            allow_single_file = True,
            mandatory = True,
            doc = "The checked canonical routine stdlib-map artifact.",
        ),
    },
    provides = [ArccStdlibMapInfo],
    doc = "Republishes the checked routine map for test-only isolated builds.",
)
