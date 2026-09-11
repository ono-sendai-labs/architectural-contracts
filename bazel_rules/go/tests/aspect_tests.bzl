"""Analysis tests for `arcc_deps_aspect` and `merge_by_importpath`.

Every assertion here corresponds to a hazard the aspect has to survive:
embedded libraries (source merge, dependency merge, duplicate import paths),
diamonds, and the standard library.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_testing//lib/private:util.bzl", "get_test_name_from_function")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load("//bazel_rules/go/private:aspect.bzl", "merge_by_importpath")
load("//bazel_rules/go/tests:probe.bzl", "ArccGoPlatformInfo")

_PROBE = "//bazel_rules/go/tests/testdata:api_closure"
_EMBED_PROBE = "//bazel_rules/go/tests/testdata:embedded_core_closure"
_PLATFORM_PROBE = "//bazel_rules/go/tests/testdata:go_platform_probe"

_TestPackageNodeInfo = provider(fields = ["node"])
_TestMergedPackageInfo = provider(fields = ["packages"])

def _test_package_node_impl(ctx):
    return [
        DefaultInfo(),
        _TestPackageNodeInfo(node = struct(
            importpath = ctx.attr.importpath,
            srcs = tuple(),
            deps = tuple(),
            cgo = False,
            export_file = ctx.file.export_file,
            label = str(ctx.label),
        )),
    ]

_test_package_node = rule(
    implementation = _test_package_node_impl,
    attrs = {
        "importpath": attr.string(mandatory = True),
        "export_file": attr.label(allow_single_file = True),
    },
    provides = [_TestPackageNodeInfo],
)

def _test_merge_probe_impl(ctx):
    merged = merge_by_importpath([
        node[_TestPackageNodeInfo].node
        for node in ctx.attr.nodes
    ])
    return [
        DefaultInfo(),
        _TestMergedPackageInfo(packages = merged),
    ]

_test_merge_probe = rule(
    implementation = _test_merge_probe_impl,
    attrs = {
        "nodes": attr.label_list(
            mandatory = True,
            providers = [_TestPackageNodeInfo],
        ),
    },
    provides = [_TestMergedPackageInfo],
)

def _test_merge_closure_probe_impl(ctx):
    merged = merge_by_importpath(ctx.attr.target[ArccPackageInfo].packages.to_list())
    return [
        DefaultInfo(),
        _TestMergedPackageInfo(packages = merged),
    ]

_test_merge_closure_probe = rule(
    implementation = _test_merge_closure_probe_impl,
    attrs = {
        "target": attr.label(
            mandatory = True,
            providers = [ArccPackageInfo],
        ),
    },
    provides = [_TestMergedPackageInfo],
)

def _test_merge_order_probe_impl(ctx):
    forward = merge_by_importpath([
        node[_TestPackageNodeInfo].node
        for node in ctx.attr.forward
    ])
    reverse = merge_by_importpath([
        node[_TestPackageNodeInfo].node
        for node in ctx.attr.reverse
    ])
    forward_pkg = forward[ctx.attr.importpath]
    reverse_pkg = reverse[ctx.attr.importpath]
    if (
        forward_pkg.export_file.path != reverse_pkg.export_file.path or
        forward_pkg.importpath != reverse_pkg.importpath or
        forward_pkg.srcs != reverse_pkg.srcs or
        forward_pkg.deps != reverse_pkg.deps or
        forward_pkg.cgo != reverse_pkg.cgo
    ):
        fail("merge order changed package metadata for %s" % ctx.attr.importpath)
    return [
        DefaultInfo(),
        _TestMergedPackageInfo(packages = forward),
    ]

_test_merge_order_probe = rule(
    implementation = _test_merge_order_probe_impl,
    attrs = {
        "importpath": attr.string(mandatory = True),
        "forward": attr.label_list(
            mandatory = True,
            providers = [_TestPackageNodeInfo],
        ),
        "reverse": attr.label_list(
            mandatory = True,
            providers = [_TestPackageNodeInfo],
        ),
    },
    provides = [_TestMergedPackageInfo],
)

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

def _export_files_are_collected_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _export_files_are_collected_impl,
        attr_values = {"size": "small"},
    )

def _export_files_are_collected_impl(env, target):
    merged = _merged(target)
    env.expect.that_collection(
        [importpath for importpath, pkg in merged.items() if pkg.export_file != None],
    ).contains_exactly(sorted(merged.keys()))
    for pkg in merged.values():
        env.expect.that_bool(pkg.export_file.path != "").equals(True)

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

def _embedded_srcs_are_projected_test(name):
    analysis_test(
        name = name,
        target = _EMBED_PROBE,
        impl = _embedded_srcs_are_projected_impl,
        attr_values = {"size": "small"},
    )

def _embedded_srcs_are_projected_impl(env, target):
    packages = target[ArccPackageInfo].packages.to_list()
    embedder = [pkg for pkg in packages if len(pkg.srcs) == 3]
    env.expect.that_int(len(embedder)).equals(1)

    # GoInfo.srcs is already embed-merged by rules_go. The adapter projects
    # that complete source set while retaining the embedded target's own node
    # so its transitive dependencies remain traversable.
    env.expect.that_collection(_basenames(embedder[0])).contains_exactly([
        "core.go",
        "core_extra.go",
        "extra_impl.go",
    ])

def _embed_only_dependency_is_reached_test(name):
    analysis_test(
        name = name,
        target = _EMBED_PROBE,
        impl = _embed_only_dependency_is_reached_impl,
        attr_values = {"size": "small"},
    )

def _embed_only_dependency_is_reached_impl(env, target):
    # //extradep hangs off the embedded library alone. If the aspect stopped
    # propagating over `embed`, its sources would be missing from the layout
    # and the check would fail to type-check //core.
    packages = target[ArccPackageInfo].packages.to_list()
    env.expect.that_collection(
        [pkg.importpath for pkg in packages],
    ).contains("example.com/aspect/extradep")

def _embedded_export_conflict_fails_test(name):
    probe = name + "_probe"
    _test_merge_closure_probe(
        name = probe,
        target = _EMBED_PROBE,
        tags = ["manual"],
    )
    analysis_test(
        name = name,
        target = ":" + probe,
        impl = _embedded_export_conflict_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _embedded_export_conflict_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*example.com/aspect/core*conflicting export_file*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*core:core*core_extra*"),
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
        attr_values = {"size": "small", "tags": ["manual"]},
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
        attr_values = {"size": "small", "tags": ["manual"]},
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

def _embedded_export_files_merge_test(name):
    first = name + "_first"
    second = name + "_second"
    probe = name + "_probe"
    _test_package_node(
        name = first,
        importpath = "example.com/export/embedded",
        export_file = "//bazel_rules/go/tests:test_export_a.data",
        tags = ["manual"],
    )
    _test_package_node(
        name = second,
        importpath = "example.com/export/embedded",
        export_file = "//bazel_rules/go/tests:test_export_a.data",
        tags = ["manual"],
    )
    _test_merge_order_probe(
        name = probe,
        importpath = "example.com/export/embedded",
        forward = [":" + first, ":" + second],
        reverse = [":" + second, ":" + first],
        tags = ["manual"],
    )
    analysis_test(
        name = name,
        target = ":" + probe,
        impl = _embedded_export_files_merge_impl,
        attr_values = {"size": "small"},
    )

def _embedded_export_files_merge_impl(env, target):
    pkg = target[_TestMergedPackageInfo].packages["example.com/export/embedded"]
    env.expect.that_str(pkg.export_file.short_path).equals(
        "bazel_rules/go/tests/test_export_a.data",
    )
    env.expect.that_collection(pkg.srcs).contains_exactly([])
    env.expect.that_collection(pkg.deps).contains_exactly([])
    env.expect.that_bool(pkg.cgo).equals(False)

def _export_merge_failure_test_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*example.com/export/*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*export_file*"),
    )

def _missing_export_file_fails_impl(env, target):
    _export_merge_failure_test_impl(env, target)
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*missing_export_file_fails_test_missing*<missing>*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*missing_export_file_fails_test_present*bazel_rules/go/tests/test_export_a.data*"),
    )

def _conflicting_export_files_fail_impl(env, target):
    _export_merge_failure_test_impl(env, target)
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*conflicting_export_files_fail_test_first*bazel_rules/go/tests/test_export_a.data*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*conflicting_export_files_fail_test_second*bazel_rules/go/tests/test_export_b.data*"),
    )

def _missing_export_file_fails_test(name):
    missing = name + "_missing"
    present = name + "_present"
    probe = name + "_probe"
    _test_package_node(
        name = missing,
        importpath = "example.com/export/missing",
        tags = ["manual"],
    )
    _test_package_node(
        name = present,
        importpath = "example.com/export/missing",
        export_file = "//bazel_rules/go/tests:test_export_a.data",
        tags = ["manual"],
    )
    _test_merge_probe(
        name = probe,
        nodes = [":" + missing, ":" + present],
        tags = ["manual"],
    )
    analysis_test(
        name = name,
        target = ":" + probe,
        impl = _missing_export_file_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _conflicting_export_files_fail_test(name):
    first = name + "_first"
    second = name + "_second"
    probe = name + "_probe"
    _test_package_node(
        name = first,
        importpath = "example.com/export/conflict",
        export_file = "//bazel_rules/go/tests:test_export_a.data",
        tags = ["manual"],
    )
    _test_package_node(
        name = second,
        importpath = "example.com/export/conflict",
        export_file = "//bazel_rules/go/tests:test_export_b.data",
        tags = ["manual"],
    )
    _test_merge_probe(
        name = probe,
        nodes = [":" + first, ":" + second],
        tags = ["manual"],
    )
    analysis_test(
        name = name,
        target = ":" + probe,
        impl = _conflicting_export_files_fail_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def arcc_deps_aspect_test_suite(name):
    _aspect_test_suite(
        name,
        [
            _closure_is_the_whole_graph_minus_stdlib_test,
            _export_files_are_collected_test,
            _edges_are_direct_importpaths_test,
            _embedded_srcs_are_projected_test,
            _embed_only_dependency_is_reached_test,
            _embedded_export_conflict_fails_test,
            _no_cgo_in_a_pure_go_closure_test,
            _non_go_target_provides_empty_provider_test,
            _embedded_export_files_merge_test,
            _missing_export_file_fails_test,
            _conflicting_export_files_fail_test,
        ],
    )

def arcc_deps_aspect_full_test_suite(name):
    _aspect_test_suite(
        name,
        [
            _go_build_platform_uses_target_mode_test,
            _go_attach_infra_is_conforming_by_default_test,
        ],
        tags = ["manual"],
    )

def _aspect_test_suite(name, tests, tags = []):
    test_targets = []
    for setup_func in tests:
        test_name = get_test_name_from_function(setup_func)
        setup_func(name = test_name)
        test_targets.append(test_name)
    native.test_suite(name = name, tests = test_targets, tags = tags)
