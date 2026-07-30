"""Analysis tests for the `_go_component` rule.

The generated files' contents are compared against goldens by
//bazel_rules/go/tests:golden_test — analysis tests cannot read a file that
has not been built yet. What they can see is the provider contract and the
analysis-time errors, which is what is asserted here.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_go//go:def.bzl", "GoInfo")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")

_API_COMPONENT = "//bazel_rules/go/tests/testdata/api:api_component"
_MEMBER_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:member_component"
_REVERSED_MEMBER_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:member_component_reversed"
_PACKAGE_SURFACE_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:package_surface_component"
_PATTERN_MEMBERSHIP_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:pattern_membership_component"
_AUTO_ATTACHED_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:auto_attached_infra_component"
_CLOSURE_ATTACHED_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:closure_attached_infra_component"
_ROOT_COLLECTION_ATTACHED_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:root_collection_attached_infra_component"
_DECLINING_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:declining_infra_component"
_NEVER_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:never_infra_component"
_AUTHORED_WINS_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:authored_wins_infra_component"
_MULTI_INFRA_A = "//bazel_rules/go/tests/testdata/membercomponent:multi_infra_a"
_MULTI_INFRA_B = "//bazel_rules/go/tests/testdata/membercomponent:multi_infra_b"

def _membership_classification_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _membership_classification_impl,
        attr_values = {"size": "small"},
    )

def _membership_classification_impl(env, target):
    info = target[ArccComponentInfo]

    # The component's coverage, as dependents see it: its own members plus
    # what it absorbed. //shared is missing because a component_dep covers it
    # — coverage is what makes a package someone else's responsibility.
    env.expect.that_collection([pkg.importpath for pkg in info.closure.to_list()]).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/core",
        "example.com/aspect/extradep",
        "example.com/aspect/lowlevel",
    ])

    env.expect.that_str(info.component_name).equals("api_component")
    env.expect.that_str(info.component_root).equals("bazel_rules/go/tests/testdata/api")

def _generated_files_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _generated_files_impl,
        attr_values = {"size": "small"},
    )

def _generated_files_impl(env, target):
    info = target[ArccComponentInfo]

    # arcc derives a dependency's layout path from its manifest path by
    # convention, so these two names are load-bearing, not cosmetic.
    env.expect.that_str(info.manifest.basename).equals("api_component.component.textproto")
    env.expect.that_str(info.layout.basename).equals("api_component.package-layout.json")

    env.expect.that_target(target).default_outputs().contains_exactly([
        "bazel_rules/go/tests/testdata/api/api_component.component.textproto",
        "bazel_rules/go/tests/testdata/api/api_component.package-layout.json",
    ])

def _unimported_member_closure_test(name):
    analysis_test(
        name = name,
        target = _MEMBER_COMPONENT,
        impl = _unimported_member_closure_impl,
        attr_values = {"size": "small"},
    )

def _unimported_member_closure_impl(env, target):
    info = target[ArccComponentInfo]
    packages = {pkg.importpath: pkg for pkg in info.closure.to_list()}

    # The interface does not import either package; both are reached only by
    # the explicit members root and its direct dependency.
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/member",
        "example.com/aspect/memberdep",
    ]).in_order()
    env.expect.that_collection(
        [src.basename for pkg in info.closure.to_list() for src in pkg.srcs],
    ).contains_exactly([
        "api.go",
        "member.go",
        "memberdep.go",
    ]).in_order()

    env.expect.that_collection(
        [src.basename for src in packages["example.com/aspect/member"].srcs],
    ).contains_exactly(["member.go"])
    env.expect.that_collection(
        [src.basename for src in packages["example.com/aspect/memberdep"].srcs],
    ).contains_exactly(["memberdep.go"])

def _reordered_member_closure_test(name):
    analysis_test(
        name = name,
        target = _REVERSED_MEMBER_COMPONENT,
        impl = _reordered_member_closure_impl,
        attr_values = {"size": "small"},
    )

def _reordered_member_closure_impl(env, target):
    info = target[ArccComponentInfo]

    # The reversed BUILD order must not affect the provider sequence. The
    # canonical sorted order is also the order used by layout generation.
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/member",
        "example.com/aspect/memberdep",
    ]).in_order()

def _package_surface_closure_test(name):
    analysis_test(
        name = name,
        target = _PACKAGE_SURFACE_COMPONENT,
        impl = _package_surface_closure_impl,
        attr_values = {"size": "small"},
    )

def _package_surface_closure_impl(env, target):
    info = target[ArccComponentInfo]

    # No interface target is present: both closure roots come from members,
    # and the component must not accidentally forward an absent interface's
    # Go providers.
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly([
        "example.com/aspect/member",
        "example.com/aspect/memberdep",
    ]).in_order()
    env.expect.that_bool(GoInfo in target).equals(False)

def _transitive_files_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _transitive_files_impl,
        attr_values = {"size": "small"},
    )

def _transitive_files_impl(env, target):
    info = target[ArccComponentInfo]

    # The check has to reach every manifest and layout in the component
    # dependency graph, not just this component's own.
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "api_component.component.textproto",
        "shared_component.component.textproto",
    ])
    env.expect.that_collection(
        [file.basename for file in info.transitive_layouts.to_list()],
    ).contains_exactly([
        "api_component.package-layout.json",
        "shared_component.package-layout.json",
    ])

    # Contract documents travel with the component but never enter the
    # manifest: they are Bazel-only metadata.
    env.expect.that_collection(
        [file.basename for file in info.contracts.to_list()],
    ).contains_exactly(["contract.md"])

def _absorb_covered_conflict_fails_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests/testdata/conflict:conflict_component",
        impl = _absorb_covered_conflict_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _absorb_covered_conflict_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*is already covered by component_dep shared_component*"),
    )

def _member_absorbed_conflict_fails_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests/testdata/conflict:member_absorbed_conflict_component",
        impl = _member_absorbed_conflict_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _member_absorbed_conflict_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*component member_absorbed_conflict_component:*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*member *//bazel_rules/go/tests/testdata/core:core is also listed in absorbed_deps (*//bazel_rules/go/tests/testdata/core:core)*"),
    )

def _member_covered_conflict_fails_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests/testdata/conflict:member_covered_conflict_component",
        impl = _member_covered_conflict_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _member_covered_conflict_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*component member_covered_conflict_component:*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*member *//bazel_rules/go/tests/testdata/shared:shared is already covered by component_dep shared_component*"),
    )

def _nested_component_root_allowed_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests/testdata/nested:nested_component",
        impl = _nested_component_root_allowed_impl,
        attr_values = {"size": "small"},
    )

def _nested_component_root_allowed_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly(["example.com/aspect/nested"])

def _pattern_membership_test(name):
    analysis_test(
        name = name,
        target = _PATTERN_MEMBERSHIP_COMPONENT,
        impl = _pattern_membership_impl,
        attr_values = {"size": "small"},
    )

def _pattern_membership_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_str(info.component_name).equals("pattern_membership_component")
    env.expect.that_bool(info.layout == None).equals(True)
    env.expect.that_target(target).default_outputs().contains_exactly([
        "bazel_rules/go/tests/testdata/membercomponent/pattern_membership_component.component.textproto",
    ])

def _auto_attached_infra_test(name):
    analysis_test(
        name = name,
        target = _AUTO_ATTACHED_INFRA_COMPONENT,
        impl = _auto_attached_infra_impl,
        attr_values = {"size": "small"},
    )

def _auto_attached_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "auto_attached_infra_component.component.textproto",
        "package_surface_component.component.textproto",
    ])
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly(["example.com/aspect/api"])
    manifest_path = "bazel_rules/go/tests/testdata/membercomponent/auto_attached_infra_component.component.textproto"
    action = env.expect.that_target(target).action_generating(manifest_path)
    action.content().contains("component_dependencies {\n  name: \"package_surface_component\"\n  manifest: \"package_surface_component.component.textproto\"\n  auto_attached: true\n}")

def _declining_infra_test(name):
    analysis_test(
        name = name,
        target = _DECLINING_INFRA_COMPONENT,
        impl = _declining_infra_impl,
        attr_values = {"size": "small"},
    )

def _declining_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "declining_infra_component.component.textproto",
    ])
    manifest_path = "bazel_rules/go/tests/testdata/membercomponent/declining_infra_component.component.textproto"
    action = env.expect.that_target(target).action_generating(manifest_path)
    action.content().split("\n").not_contains("  auto_attached: true")
    action.content().split("\n").not_contains('  name: "package_surface_component"')

    runfile_basenames = [f.basename for f in target[DefaultInfo].default_runfiles.files.to_list()]
    env.expect.that_collection(runfile_basenames).not_contains("package_surface_component.component.textproto")

def _closure_attached_infra_test(name):
    analysis_test(
        name = name,
        target = _CLOSURE_ATTACHED_INFRA_COMPONENT,
        impl = _closure_attached_infra_impl,
        attr_values = {"size": "small"},
    )

def _closure_attached_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "closure_attached_infra_component.component.textproto",
        "package_surface_component.component.textproto",
    ])
    manifest_path = "bazel_rules/go/tests/testdata/membercomponent/closure_attached_infra_component.component.textproto"
    action = env.expect.that_target(target).action_generating(manifest_path)
    action.content().contains("component_dependencies {\n  name: \"package_surface_component\"\n  manifest: \"package_surface_component.component.textproto\"\n  auto_attached: true\n}")

def _root_collection_attachment_test(name):
    analysis_test(
        name = name,
        target = _ROOT_COLLECTION_ATTACHED_INFRA_COMPONENT,
        impl = _root_collection_attachment_impl,
        attr_values = {"size": "small"},
    )

def _root_collection_attachment_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "root_collection_attached_infra_component.component.textproto",
        "package_surface_component.component.textproto",
    ])

def _never_infra_test(name):
    analysis_test(
        name = name,
        target = _NEVER_INFRA_COMPONENT,
        impl = _never_infra_impl,
        attr_values = {"size": "small"},
    )

def _never_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly(["never_infra_component.component.textproto"])

def _authored_wins_infra_test(name):
    analysis_test(
        name = name,
        target = _AUTHORED_WINS_INFRA_COMPONENT,
        impl = _authored_wins_infra_impl,
        attr_values = {"size": "small"},
    )

def _authored_wins_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "authored_wins_infra_component.component.textproto",
        "package_surface_component.component.textproto",
    ])
    manifest_path = "bazel_rules/go/tests/testdata/membercomponent/authored_wins_infra_component.component.textproto"
    action = env.expect.that_target(target).action_generating(manifest_path)
    action.content().contains('name: "package_surface_component"')
    action.content().split("\n").not_contains("  auto_attached: true")

def _deterministic_multi_infra_test(name):
    analysis_test(
        name = name,
        targets = {
            "target_a": _MULTI_INFRA_A,
            "target_b": _MULTI_INFRA_B,
        },
        impl = _deterministic_multi_infra_impl,
        attr_values = {"size": "small"},
    )

def _deterministic_multi_infra_impl(env, target):
    info_a = target.target_a[ArccComponentInfo]
    info_b = target.target_b[ArccComponentInfo]

    env.expect.that_collection(
        [file.basename for file in info_a.transitive_manifests.to_list()],
    ).contains_exactly([
        "package_surface_component.component.textproto",
        "runtime_component.component.textproto",
        "multi_infra_a.component.textproto",
    ])
    env.expect.that_collection(
        [file.basename for file in info_b.transitive_manifests.to_list()],
    ).contains_exactly([
        "package_surface_component.component.textproto",
        "runtime_component.component.textproto",
        "multi_infra_b.component.textproto",
    ])

    action_manifest_a = env.expect.that_target(target.target_a).action_generating("bazel_rules/go/tests/testdata/membercomponent/multi_infra_a.component.textproto")
    action_manifest_b = env.expect.that_target(target.target_b).action_generating("bazel_rules/go/tests/testdata/membercomponent/multi_infra_b.component.textproto")

    manifest_a = action_manifest_a.actual.content
    manifest_b_norm = action_manifest_b.actual.content.replace("multi_infra_b", "multi_infra_a")
    env.expect.that_str(manifest_a).equals(manifest_b_norm)

    action_layout_a = env.expect.that_target(target.target_a).action_generating("bazel_rules/go/tests/testdata/membercomponent/multi_infra_a.package-layout.json")
    action_layout_b = env.expect.that_target(target.target_b).action_generating("bazel_rules/go/tests/testdata/membercomponent/multi_infra_b.package-layout.json")

    layout_a = action_layout_a.actual.content
    layout_b_norm = action_layout_b.actual.content.replace("multi_infra_b", "multi_infra_a")
    env.expect.that_str(layout_a).equals(layout_b_norm)

def go_component_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _membership_classification_test,
            _generated_files_test,
            _unimported_member_closure_test,
            _reordered_member_closure_test,
            _package_surface_closure_test,
            _transitive_files_test,
            _absorb_covered_conflict_fails_test,
            _member_absorbed_conflict_fails_test,
            _member_covered_conflict_fails_test,
            _nested_component_root_allowed_test,
            _pattern_membership_test,
            _auto_attached_infra_test,
            _declining_infra_test,
            _closure_attached_infra_test,
            _root_collection_attachment_test,
            _never_infra_test,
            _authored_wins_infra_test,
            _deterministic_multi_infra_test,
        ],
    )
