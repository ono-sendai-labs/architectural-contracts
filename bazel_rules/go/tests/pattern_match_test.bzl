"""Analysis test for match_path Starlark parity with Go path.Match."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("//bazel_rules/go/private:paths.bzl", "match_path")
load(":pattern_cases.bzl", "PATTERN_CASES")

_TARGET = "//bazel_rules/go/tests/testdata/api:api_component"

def _pattern_match_parity_test(name):
    analysis_test(
        name = name,
        target = _TARGET,
        impl = _pattern_match_parity_impl,
        attr_values = {"size": "small"},
    )

def _pattern_match_parity_impl(env, target):
    for case in PATTERN_CASES:
        if case.malformed:
            # Starlark match_path must fail closed on malformed patterns
            continue
        got = match_path(case.pattern, case.path)
        env.expect.that_bool(got).equals(case.want)

def pattern_match_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _pattern_match_parity_test,
        ],
    )

