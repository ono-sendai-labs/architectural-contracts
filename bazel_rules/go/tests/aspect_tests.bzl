"""Analysis tests for `arcc_deps_aspect` and `merge_by_importpath`.

Every assertion here corresponds to a hazard the aspect has to survive:
embedded libraries (source merge, dependency merge, duplicate import paths),
diamonds, and the standard library.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load("//bazel_rules/go/private:aspect.bzl", "merge_by_importpath")

_PROBE = "//bazel_rules/go/tests/testdata:api_closure"

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

def arcc_deps_aspect_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _closure_is_the_whole_graph_minus_stdlib_test,
            _edges_are_direct_importpaths_test,
            _embedded_srcs_merge_into_the_embedder_test,
            _embed_only_dependency_is_reached_test,
            _no_cgo_in_a_pure_go_closure_test,
        ],
    )
