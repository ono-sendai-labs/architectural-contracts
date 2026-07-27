"""Analysis tests for the `_go_component` rule.

The generated files' contents are compared against goldens by
//bazel_rules/go/tests:golden_test — analysis tests cannot read a file that
has not been built yet. What they can see is the provider contract and the
analysis-time errors, which is what is asserted here.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")

_API_COMPONENT = "//bazel_rules/go/tests/testdata/api:api_component"
_MEMBER_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:member_component"
_REVERSED_MEMBER_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:member_component_reversed"

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
    env.expect.that_collection(packages.keys()).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/core",
        "example.com/aspect/extradep",
        "example.com/aspect/lowlevel",
        "example.com/aspect/member",
        "example.com/aspect/memberdep",
        "example.com/aspect/shared",
    ])
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
        "example.com/aspect/core",
        "example.com/aspect/extradep",
        "example.com/aspect/lowlevel",
        "example.com/aspect/member",
        "example.com/aspect/memberdep",
        "example.com/aspect/shared",
    ]).in_order()
    env.expect.that_collection(
        [src.basename for pkg in info.closure.to_list() for src in pkg.srcs],
    ).contains_exactly([
        "api.go",
        "core.go",
        "core_extra.go",
        "extra_impl.go",
        "extradep.go",
        "lowlevel.go",
        "member.go",
        "memberdep.go",
        "shared.go",
    ]).in_order()

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

def _nested_component_root_fails_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests/testdata/nested:nested_component",
        impl = _nested_component_root_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _nested_component_root_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*nested inside this component's root*"),
    )

def go_component_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _membership_classification_test,
            _generated_files_test,
            _unimported_member_closure_test,
            _reordered_member_closure_test,
            _transitive_files_test,
            _absorb_covered_conflict_fails_test,
            _nested_component_root_fails_test,
        ],
    )
