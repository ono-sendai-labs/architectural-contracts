"""Analysis tests for the host-injected runtime adapter contract."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_testing//lib/private:util.bzl", "get_test_name_from_function")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load(
    "//bazel_rules/go/private:go_adapter.bzl",
    "extra_runtime_packages",
    "runtime_injection_attrs",
)

RuntimeInjectionProbeInfo = provider(fields = ["attrs", "packages"])

_INJECTED_COMPONENT = "//bazel_rules/go/tests/testdata/injected_runtime:runtime_injection_component"
_CGO_COMPONENT = "//bazel_rules/go/tests:injected_cgo_component"
_CONFLICT_COMPONENT = "//bazel_rules/go/tests:injected_conflict_component"
_ORDINARY_CONFLICT_COMPONENT = "//bazel_rules/go/tests:injected_ordinary_conflict_component"
_ORDER_COMPONENT_A = "//bazel_rules/go/tests:injected_order_component_a"
_ORDER_COMPONENT_B = "//bazel_rules/go/tests:injected_order_component_b"

def _fake_runtime_package_impl(ctx):
    return [
        DefaultInfo(),
        ArccPackageInfo(packages = depset(direct = [struct(
            importpath = ctx.attr.importpath,
            srcs = tuple(),
            deps = tuple(),
            cgo = ctx.attr.cgo,
            export_file = ctx.file.export_file,
            label = str(ctx.label),
        )])),
    ]

fake_runtime_package = rule(
    implementation = _fake_runtime_package_impl,
    attrs = {
        "importpath": attr.string(mandatory = True),
        "export_file": attr.label(allow_single_file = True),
        "cgo": attr.bool(default = False),
    },
    provides = [ArccPackageInfo],
    doc = "Test-only host-neutral injected package record.",
)

def _runtime_injection_defaults_probe_impl(ctx):
    return [
        DefaultInfo(),
        RuntimeInjectionProbeInfo(
            attrs = runtime_injection_attrs(None),
            packages = extra_runtime_packages(ctx, []),
        ),
    ]

runtime_injection_defaults_probe = rule(
    implementation = _runtime_injection_defaults_probe_impl,
    attrs = {},
    provides = [RuntimeInjectionProbeInfo],
    doc = "Exposes the upstream runtime-injection adapter defaults for analysis tests.",
)

def _runtime_injection_defaults_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests:runtime_injection_defaults_probe",
        impl = _runtime_injection_defaults_impl,
        attr_values = {"size": "small"},
    )

def _runtime_injection_defaults_impl(env, target):
    env.expect.that_dict(target[RuntimeInjectionProbeInfo].attrs).contains_exactly({})
    env.expect.that_collection(target[RuntimeInjectionProbeInfo].packages).contains_exactly([])

def _injected_runtime_is_merged_and_attached_test(name):
    analysis_test(
        name = name,
        target = _INJECTED_COMPONENT,
        impl = _injected_runtime_is_merged_and_attached_impl,
        attr_values = {"size": "small"},
    )

def _injected_runtime_is_merged_and_attached_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/core",
        "example.com/aspect/extradep",
        "example.com/aspect/lowlevel",
    ])

    manifest = env.expect.that_target(target).action_generating(
        "bazel_rules/go/tests/testdata/injected_runtime/runtime_injection_component.component.textproto",
    ).actual.content
    env.expect.that_str(manifest).contains(
        "component_dependencies {\n" +
        "  name: \"runtime_boundary\"\n" +
        "  manifest: \"runtime_boundary.component.textproto\"\n" +
        "  auto_attached: true\n" +
        "}",
    )
    env.expect.that_collection(manifest.split("\n")).not_contains("members: \"example.com/injected/runtime\"")

    base_layout = env.expect.that_target(target).action_generating(
        "bazel_rules/go/tests/testdata/injected_runtime/runtime_injection_component.package-layout.base.json",
    ).actual.content
    packages = [
        package
        for package in json.decode(base_layout)["packages"]
        if package["PkgPath"] == "example.com/injected/runtime"
    ]
    env.expect.that_int(len(packages)).equals(1)
    env.expect.that_str(packages[0]["ExportFile"]).contains("runtime.x")
    bindings = [
        binding
        for binding in json.decode(base_layout)["dependency_artifact_bindings"]
        if binding["dependency"] == "runtime_boundary"
    ]
    env.expect.that_int(len(bindings)).equals(1)
    env.expect.that_bool(bindings[0]["auto_attached"]).equals(True)
    env.expect.that_str(bindings[0]["provenance"]).equals("asserted")

    check_action = env.expect.that_target(target).action_generating(info.report.short_path).actual
    check_inputs = [file.basename for file in check_action.inputs.to_list()]
    env.expect.that_collection(check_inputs).contains("runtime_boundary.surface.json")
    env.expect.that_collection(check_inputs).contains("runtime.x")
    env.expect.that_collection([name for name in check_inputs if name == "runtime.go"]).contains_exactly([])
    env.expect.that_int(
        len([action for action in target.actions if action.mnemonic == "ArccCheck"]),
    ).equals(1)

def _injected_cgo_fails_test(name):
    analysis_test(
        name = name,
        target = _CGO_COMPONENT,
        impl = _injected_cgo_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _injected_cgo_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*component injected_cgo_component: package example.com/injected/cgo is built with cgo*"),
    )

def _injected_export_conflict_fails_test(name):
    analysis_test(
        name = name,
        target = _CONFLICT_COMPONENT,
        impl = _injected_export_conflict_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _injected_export_conflict_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*example.com/injected/conflict*conflicting export_file*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*injected_conflict_node_a*bazel_rules/go/tests/test_export_a.data*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*injected_conflict_node_b*bazel_rules/go/tests/test_export_b.data*"),
    )

def _injected_ordinary_export_conflict_fails_test(name):
    analysis_test(
        name = name,
        target = _ORDINARY_CONFLICT_COMPONENT,
        impl = _injected_ordinary_export_conflict_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _injected_ordinary_export_conflict_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*example.com/aspect/api*conflicting export_file*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*injected_ordinary_conflict_package*bazel_rules/go/tests/test_export_b.data*"),
    )

def _injected_package_order_is_deterministic_test(name):
    analysis_test(
        name = name,
        targets = {
            "component_a": _ORDER_COMPONENT_A,
            "component_b": _ORDER_COMPONENT_B,
        },
        impl = _injected_package_order_is_deterministic_impl,
        attr_values = {"size": "small"},
    )

def _injected_package_order_is_deterministic_impl(env, target):
    layout_a = env.expect.that_target(target.component_a).action_generating(
        "bazel_rules/go/tests/injected_order_component_a.package-layout.base.json",
    ).actual.content
    layout_b = env.expect.that_target(target.component_b).action_generating(
        "bazel_rules/go/tests/injected_order_component_b.package-layout.base.json",
    ).actual.content
    env.expect.that_str(layout_a).equals(layout_b)

def runtime_injection_test_suite(name):
    test_names = []
    for setup_func in [
        _runtime_injection_defaults_test,
        _injected_runtime_is_merged_and_attached_test,
        _injected_cgo_fails_test,
        _injected_export_conflict_fails_test,
        _injected_ordinary_export_conflict_fails_test,
        _injected_package_order_is_deterministic_test,
    ]:
        test_name = get_test_name_from_function(setup_func)
        setup_func(name = test_name)
        test_names.append(test_name)
    native.test_suite(name = name, tests = test_names)
