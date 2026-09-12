"""Analysis tests for the three host SDK adapter contracts."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_testing//lib/private:util.bzl", "get_test_name_from_function")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")

_FAKE_COMPONENT = "//bazel_rules/go/tests:fake_sdk_export_component"
_MISSING_COMPONENT = "//bazel_rules/go/tests:fake_sdk_export_missing_component"
_MISMATCH_COMPONENT = "//bazel_rules/go/tests:fake_sdk_export_mismatch_component"
_SOURCE_COMPONENT = "//bazel_rules/go/tests:fake_sdk_export_source_component"
_OPPOSITE_MAP_LAYOUT = "//bazel_rules/go/tests:fake_sdk_export_opposite_map_component_layout"
_MISSING_SOURCE = "//bazel_rules/go/tests:stdlib_source_data_missing_probe"

def _action(target, mnemonic):
    actions = [action for action in target.actions if action.mnemonic == mnemonic]
    if len(actions) != 1:
        fail("expected one %s action, found %d" % (mnemonic, len(actions)))
    return actions[0]

def _fake_export_descriptor_controls_check_inputs_test(name):
    analysis_test(
        name = name,
        target = _FAKE_COMPONENT,
        impl = _fake_export_descriptor_controls_check_inputs_impl,
        attr_values = {"size": "small"},
    )

def _fake_export_descriptor_controls_check_inputs_impl(env, target):
    info = target[ArccComponentInfo]
    layout = env.expect.that_target(target).action_generating(
        "bazel_rules/go/tests/fake_sdk_export_component.package-layout.base.json",
    ).actual.content
    check = _action(target, "ArccCheck")
    inputs = [file.basename for file in check.inputs.to_list()]

    # The fake descriptor is visible in the layout contract and replaces the
    # upstream GoStdLib material rather than being appended to it.
    env.expect.that_str(layout).contains('"stdlib_export_data":')
    env.expect.that_str(layout).contains("test_sdk_export_metadata.json")
    env.expect.that_str(layout).contains("test_sdk_export_a.x")
    env.expect.that_str(layout).contains("test_sdk_export_b.x")
    env.expect.that_str(layout).contains('"goos": "linux"')
    env.expect.that_str(layout).contains('"goarch": "amd64"')
    env.expect.that_str(layout).contains('"build_tags": []')
    env.expect.that_str(layout).contains('"cgo_enabled": false')
    env.expect.that_collection(inputs).contains("test_sdk_export_metadata.json")
    env.expect.that_collection(inputs).contains("test_sdk_export_a.x")
    env.expect.that_collection(inputs).contains("test_sdk_export_b.x")
    env.expect.that_collection([
        basename
        for basename in inputs
        if basename in ["stdlib.pkg.json", "gocache", "pkg"]
    ]).contains_exactly([])
    env.expect.that_collection([
        file.short_path
        for file in check.inputs.to_list()
        if "/src/" in file.short_path or "/pkg/tool/" in file.short_path or
           file.short_path.endswith("/bin/go")
    ]).contains_exactly([])

    # The regular component action still has its own member-only sources and
    # ordinary export inputs; the sentinel files are specifically the SDK
    # export role, not a replacement for the package closure.
    env.expect.that_str(info.report.basename).equals("fake_sdk_export_component.report.json")

def _fake_export_descriptor_failure_test(name, target, impl):
    analysis_test(
        name = name,
        target = target,
        impl = impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _missing_export_descriptor_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*fake_sdk_export_missing_component*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*compiled export artifacts*"),
    )

def _missing_export_descriptor_test(name):
    _fake_export_descriptor_failure_test(
        name,
        _MISSING_COMPONENT,
        _missing_export_descriptor_impl,
    )

def _mismatched_export_descriptor_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*fake_sdk_export_mismatch_component*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*mismatched fields: goos*"),
    )

def _mismatched_export_descriptor_test(name):
    _fake_export_descriptor_failure_test(
        name,
        _MISMATCH_COMPONENT,
        _mismatched_export_descriptor_impl,
    )

def _incomplete_source_descriptor_test(name):
    analysis_test(
        name = name,
        target = _MISSING_SOURCE,
        impl = _incomplete_source_descriptor_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _incomplete_source_descriptor_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*stdlib_source_data_missing_probe*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*package oracle*SDK root*target identity*"),
    )

def _source_in_export_descriptor_test(name):
    analysis_test(
        name = name,
        target = _SOURCE_COMPONENT,
        impl = _source_in_export_descriptor_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _source_in_export_descriptor_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*fake_sdk_export_source_component*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*source material*"),
    )

def _opposite_map_transition_test(name):
    analysis_test(
        name = name,
        target = _OPPOSITE_MAP_LAYOUT,
        impl = _opposite_map_transition_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _opposite_map_transition_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*component target and selected authority map mismatch*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*mismatched fields: goos, goarch*"),
    )

def sdk_adapter_test_suite(name):
    tests = [
        _fake_export_descriptor_controls_check_inputs_test,
        _missing_export_descriptor_test,
        _mismatched_export_descriptor_test,
        _incomplete_source_descriptor_test,
        _source_in_export_descriptor_test,
        _opposite_map_transition_test,
    ]
    test_targets = []
    for setup_func in tests:
        test_name = get_test_name_from_function(setup_func)
        setup_func(name = test_name)
        test_targets.append(test_name)
    native.test_suite(name = name, tests = test_targets, tags = ["manual"])
