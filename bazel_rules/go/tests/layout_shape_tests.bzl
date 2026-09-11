"""Structural tests for the final package-layout shape-golden rules.

Layout shape assertions are deliberately independent from semantic verdict and
persisted report/surface shape assertions. Their launcher invokes the typed
`artifact-shape layout` command and stages only the final layout, its explicit
shape golden, and arcc; it never reruns `arcc check` or stages a report.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")

_API_LAYOUT_SHAPE = "//bazel_rules/go/tests:api_component_layout_shape_golden_test"
_MEMBER_LAYOUT_SHAPE = "//bazel_rules/go/tests:member_component_layout_shape_golden_test"

def _launcher(env, target):
    for action in target.actions:
        for output in action.outputs.to_list():
            if output.extension == "sh":
                if action.content == None:
                    env.fail("layout shape launcher action %s carries no content" % output.short_path)
                return action.content
    env.fail("no layout shape launcher (.sh) action found on %s" % target.label)
    return None

def _basenames(target):
    return sorted([f.basename for f in target[DefaultInfo].default_runfiles.files.to_list()])

def _layout_shape_rule_impl(env, target, layout_basename, golden_basename, launcher_basename):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("artifact-shape")
    env.expect.that_str(content).contains("'layout'")
    env.expect.that_str(content).contains(layout_basename)
    env.expect.that_str(content).contains(golden_basename)
    for forbidden in ["'check'", "--report-out", "--surface-out", "--stdlib-map", ".report.json", ".surface.json"]:
        if forbidden in content:
            env.fail("layout shape launcher must not contain %r:\n%s" % (forbidden, content))
    env.expect.that_collection(_basenames(target)).contains_exactly([
        "arcc",
        layout_basename,
        golden_basename,
        launcher_basename,
    ])

def _api_layout_shape_test(name):
    analysis_test(name = name, target = _API_LAYOUT_SHAPE, impl = _api_layout_shape_impl, attr_values = {"size": "small"})

def _member_layout_shape_test(name):
    analysis_test(name = name, target = _MEMBER_LAYOUT_SHAPE, impl = _member_layout_shape_impl, attr_values = {"size": "small"})

def _api_layout_shape_impl(env, target):
    _layout_shape_rule_impl(
        env,
        target,
        "api_component.package-layout.json",
        "api_component.layout.shape.golden.json",
        "api_component_layout_shape_golden_test.sh",
    )

def _member_layout_shape_impl(env, target):
    _layout_shape_rule_impl(
        env,
        target,
        "member_component.package-layout.json",
        "member_component.layout.shape.golden.json",
        "member_component_layout_shape_golden_test.sh",
    )

def layout_shape_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _api_layout_shape_test,
            _member_layout_shape_test,
        ],
    )
