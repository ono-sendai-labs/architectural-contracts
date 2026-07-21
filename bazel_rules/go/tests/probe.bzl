"""Test-only rule that applies `arcc_deps_aspect` and republishes its closure.

`analysis_test` takes a plain label, so something has to attach the aspect for
it. This is the smallest thing that can.
"""

load("@rules_go//go:def.bzl", "GoInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load("//bazel_rules/go/private:aspect.bzl", "arcc_deps_aspect")

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
