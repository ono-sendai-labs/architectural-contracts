"""Analysis tests for the host-neutral standard-library export seam."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_testing//lib/private:util.bzl", "get_test_name_from_function")
load("//bazel_rules/go/tests:probe.bzl", "StdlibExportDataInfo")

_PROBE = "//bazel_rules/go/tests:stdlib_export_data_probe"
_MISSING = "//bazel_rules/go/tests:stdlib_export_data_missing_probe"
_MISMATCH = "//bazel_rules/go/tests:stdlib_export_data_mismatch_probe"

def _valid_export_data_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _valid_export_data_impl,
        attr_values = {"size": "small"},
    )

def _valid_export_data_impl(env, target):
    data = target[StdlibExportDataInfo]
    env.expect.that_collection([data.metadata.basename]).contains_exactly(["stdlib.pkg.json"])
    env.expect.that_int(len(data.export_files.to_list())).equals(2)
    env.expect.that_collection([f.basename for f in data.inputs.to_list()]).contains_exactly([
        "gocache",
        "pkg",
        "stdlib.pkg.json",
    ])
    env.expect.that_collection([
        f.short_path
        for f in data.inputs.to_list()
        if f.short_path.endswith(".go") or "/src/" in f.short_path or
           "/bin/" in f.short_path or "/pkg/tool/" in f.short_path or
           "/.cache/" in f.short_path
    ]).contains_exactly([])
    env.expect.that_str(data.target.goos).equals("linux")
    env.expect.that_str(data.target.goarch).equals("amd64")
    env.expect.that_bool(data.target.cgo_enabled).equals(False)

def _missing_export_data_fails_analysis_test(name):
    analysis_test(
        name = name,
        target = _MISSING,
        impl = _missing_export_data_fails_analysis_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _missing_export_data_fails_analysis_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*stdlib_export_data_missing_probe*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*export metadata*"),
    )

def _mismatched_export_data_fails_analysis_test(name):
    analysis_test(
        name = name,
        target = _MISMATCH,
        impl = _mismatched_export_data_fails_analysis_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _mismatched_export_data_fails_analysis_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*stdlib_export_data_mismatch_probe*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*configuration mismatch*"),
    )

def stdlib_export_data_test_suite(name):
    tests = [
        _valid_export_data_test,
        _missing_export_data_fails_analysis_test,
        _mismatched_export_data_fails_analysis_test,
    ]
    test_targets = []
    for setup_func in tests:
        test_name = get_test_name_from_function(setup_func)
        setup_func(name = test_name)
        test_targets.append(test_name)
    native.test_suite(name = name, tests = test_targets, tags = ["manual"])
