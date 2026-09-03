"""Analysis tests for `arcc_deps_aspect` and `merge_by_importpath`.

Every assertion here corresponds to a hazard the aspect has to survive:
embedded libraries (source merge, dependency merge, duplicate import paths),
diamonds, and the standard library.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load("//bazel_rules/go/private:aspect.bzl", "merge_by_importpath")
load("//bazel_rules/go/tests:probe.bzl", "ArccGoPlatformInfo")

_PROBE = "//bazel_rules/go/tests/testdata:api_closure"
_PLATFORM_PROBE = "//bazel_rules/go/tests/testdata:go_platform_probe"

def _merged(target):
    return merge_by_importpath(target[ArccPackageInfo].packages.to_list())

def _basenames(pkg):
    return [src.basename for src in pkg.srcs]

def _closure_is_the_whole_graph_minus_stdlib_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _closure_is_the_whole_graph_minus_stdlib_impl,
        attr_values = {"size": "small"},
    )

def _closure_is_the_whole_graph_minus_stdlib_impl(env, target):
    # Five packages, not six: the embedded library folds into its embedder.
    # "strings", imported by //lowlevel, is absent — arcc handles stdlib
    # authority itself, so the closure never carries it.
    env.expect.that_collection(_merged(target).keys()).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/core",
        "example.com/aspect/extradep",
        "example.com/aspect/lowlevel",
        "example.com/aspect/shared",
    ])

def _edges_are_direct_importpaths_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _edges_are_direct_importpaths_impl,
        attr_values = {"size": "small"},
    )

def _edges_are_direct_importpaths_impl(env, target):
    merged = _merged(target)

    env.expect.that_collection(merged["example.com/aspect/api"].deps).contains_exactly([
        "example.com/aspect/core",
        "example.com/aspect/shared",
    ])

    # //core's own deps plus the one it inherits from its embedded library —
    # and not itself, though the embedded library shares its import path.
    env.expect.that_collection(merged["example.com/aspect/core"].deps).contains_exactly([
        "example.com/aspect/extradep",
        "example.com/aspect/lowlevel",
        "example.com/aspect/shared",
    ])

    # Nothing but stdlib beyond here.
    env.expect.that_collection(merged["example.com/aspect/lowlevel"].deps).contains_exactly([])
    env.expect.that_collection(merged["example.com/aspect/shared"].deps).contains_exactly([])

def _embedded_srcs_merge_into_the_embedder_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _embedded_srcs_merge_into_the_embedder_impl,
        attr_values = {"size": "small"},
    )

def _embedded_srcs_merge_into_the_embedder_impl(env, target):
    merged = _merged(target)

    # extra_impl.go belongs to the embedded library, and appears exactly once.
    env.expect.that_collection(_basenames(merged["example.com/aspect/core"])).contains_exactly([
        "core.go",
        "core_extra.go",
        "extra_impl.go",
    ]).in_order()

    env.expect.that_collection(_basenames(merged["example.com/aspect/api"])).contains_exactly(
        ["api.go"],
    )

def _embed_only_dependency_is_reached_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _embed_only_dependency_is_reached_impl,
        attr_values = {"size": "small"},
    )

def _embed_only_dependency_is_reached_impl(env, target):
    # //extradep hangs off the embedded library alone. If the aspect stopped
    # propagating over `embed`, its sources would be missing from the layout
    # and the check would fail to type-check //core.
    merged = _merged(target)
    env.expect.that_collection(_basenames(merged["example.com/aspect/extradep"])).contains_exactly(
        ["extradep.go"],
    )

def _no_cgo_in_a_pure_go_closure_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _no_cgo_in_a_pure_go_closure_impl,
        attr_values = {"size": "small"},
    )

def _no_cgo_in_a_pure_go_closure_impl(env, target):
    for importpath, pkg in _merged(target).items():
        env.expect.that_bool(pkg.cgo).equals(False)
        env.expect.that_str(pkg.importpath).equals(importpath)

def _go_build_platform_uses_target_mode_test(name):
    analysis_test(
        name = name,
        target = _PLATFORM_PROBE,
        impl = _go_build_platform_uses_target_mode_impl,
        attr_values = {"size": "small"},
    )

def _go_build_platform_uses_target_mode_impl(env, target):
    platform = target[ArccGoPlatformInfo]
    env.expect.that_str(platform.goos).equals("darwin")
    env.expect.that_str(platform.goarch).equals("arm64")
    env.expect.that_collection(platform.tags).contains_exactly(["adapter_probe"])
    env.expect.that_bool(platform.cgo_enabled).equals(False)

def _go_attach_infra_is_conforming_by_default_test(name):
    analysis_test(
        name = name,
        target = _PLATFORM_PROBE,
        impl = _go_attach_infra_is_conforming_by_default_impl,
        attr_values = {"size": "small"},
    )

def _go_attach_infra_is_conforming_by_default_impl(env, target):
    platform = target[ArccGoPlatformInfo]
    env.expect.that_bool(platform.infra_attached).equals(True)
    env.expect.that_collection(platform.registry).contains_exactly([])

_NON_GO_PROBE = "//bazel_rules/go/tests/testdata/nongo:non_go_closure_probe"

def _non_go_target_provides_empty_provider_test(name):
    analysis_test(
        name = name,
        target = _NON_GO_PROBE,
        impl = _non_go_target_provides_empty_provider_impl,
        attr_values = {"size": "small"},
    )

def _non_go_target_provides_empty_provider_impl(env, target):
    # The aspect declares ArccPackageInfo in `provides`, so every target it
    # visits must carry one — including filegroups and other non-Go targets —
    # and a non-Go target contributes no packages.
    env.expect.that_bool(ArccPackageInfo in target).equals(True)
    env.expect.that_collection(target[ArccPackageInfo].packages.to_list()).contains_exactly([])

def arcc_deps_aspect_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _closure_is_the_whole_graph_minus_stdlib_test,
            _edges_are_direct_importpaths_test,
            _embedded_srcs_merge_into_the_embedder_test,
            _embed_only_dependency_is_reached_test,
            _no_cgo_in_a_pure_go_closure_test,
            _non_go_target_provides_empty_provider_test,
            _go_build_platform_uses_target_mode_test,
            _go_attach_infra_is_conforming_by_default_test,
        ],
    )
