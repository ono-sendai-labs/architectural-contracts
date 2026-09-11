"""Structural tests for the persisted-artifact shape golden rules.

These tests keep the intentionally layout/target-sensitive shape assertions
separate from verdict goldens. The launchers must invoke the typed
`artifact-shape` command, stage only the producer artifact, golden, and arcc,
and accept an asserted surface without introducing an ArccCheck.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")

_REPORT_SHAPE = "//bazel_rules/go/tests:api_component_report_shape_golden_test"
_SURFACE_SHAPE = "//bazel_rules/go/tests:api_component_surface_shape_golden_test"
_ASSERTED_SURFACE_SHAPE = "//bazel_rules/go/tests:manual_component_surface_shape_golden_test"

def _launcher(env, target):
    for action in target.actions:
        for output in action.outputs.to_list():
            if output.extension == "sh":
                if action.content == None:
                    env.fail("shape launcher action %s carries no content" % output.short_path)
                return action.content
    env.fail("no shape launcher (.sh) action found on %s" % target.label)
    return None

def _basenames(target):
    return sorted([f.basename for f in target[DefaultInfo].default_runfiles.files.to_list()])

def _report_shape_rule_test(name):
    analysis_test(
        name = name,
        target = _REPORT_SHAPE,
        impl = _report_shape_rule_impl,
        attr_values = {"size": "small"},
    )

def _report_shape_rule_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("artifact-shape")
    env.expect.that_str(content).contains("'report'")
    env.expect.that_str(content).contains("api_component.report.shape.golden.json")
    env.expect.that_str(content).contains("api_component.report.json")
    if "diff -u" in content:
        env.fail("report shape launcher must use the bounded artifact-shape command, not a raw diff")
    env.expect.that_collection(_basenames(target)).contains_exactly([
        "api_component.report.json",
        "api_component.report.shape.golden.json",
        "api_component_report_shape_golden_test.sh",
        "arcc",
    ])

def _surface_shape_rule_test(name):
    analysis_test(
        name = name,
        target = _SURFACE_SHAPE,
        impl = _surface_shape_rule_impl,
        attr_values = {"size": "small"},
    )

def _surface_shape_rule_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("artifact-shape")
    env.expect.that_str(content).contains("'surface'")
    env.expect.that_str(content).contains("api_component.surface.shape.golden.json")
    env.expect.that_str(content).contains("api_component.surface.json")
    if "--package-layout" in content or "'check'" in content:
        env.fail("surface shape launcher must not rerun component analysis")
    env.expect.that_collection(_basenames(target)).contains_exactly([
        "api_component.surface.json",
        "api_component.surface.shape.golden.json",
        "api_component_surface_shape_golden_test.sh",
        "arcc",
    ])

def _asserted_surface_shape_rule_test(name):
    analysis_test(
        name = name,
        target = _ASSERTED_SURFACE_SHAPE,
        impl = _asserted_surface_shape_rule_impl,
        attr_values = {"size": "small"},
    )

def _asserted_surface_shape_rule_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("artifact-shape")
    env.expect.that_str(content).contains("'surface'")
    env.expect.that_str(content).contains("manual_component.surface.shape.golden.json")
    env.expect.that_str(content).contains("manual_component.surface.json")
    if "manual_component.report.json" in content or "'check'" in content:
        env.fail("asserted surface shape launcher must not stage a report or run ArccCheck")
    env.expect.that_collection(_basenames(target)).contains_exactly([
        "arcc",
        "manual_component.surface.json",
        "manual_component.surface.shape.golden.json",
        "manual_component_surface_shape_golden_test.sh",
    ])

def artifact_shape_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _report_shape_rule_test,
            _surface_shape_rule_test,
            _asserted_surface_shape_rule_test,
        ],
    )
