"""Analysis tests for the host-injected runtime adapter contract."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test")
load("@rules_testing//lib/private:util.bzl", "get_test_name_from_function")
load(
    "//bazel_rules/go/private:go_adapter.bzl",
    "extra_runtime_packages",
    "runtime_injection_attrs",
)

RuntimeInjectionProbeInfo = provider(fields = ["attrs", "packages"])

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

def runtime_injection_test_suite(name):
    test_name = get_test_name_from_function(_runtime_injection_defaults_test)
    _runtime_injection_defaults_test(name = test_name)
    native.test_suite(name = name, tests = [test_name])
