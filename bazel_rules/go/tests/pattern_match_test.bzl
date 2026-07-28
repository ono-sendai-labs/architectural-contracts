"""Analysis test for match_path Starlark parity with Go path.Match."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("//bazel_rules/go/private:paths.bzl", "match_path")
load(":pattern_cases.bzl", "PATTERN_CASES")

_TARGET = "//bazel_rules/go/tests/testdata/api:api_component"

_PatternMatchProbeInfo = provider(fields = ["matched"])

def _pattern_match_probe_impl(ctx):
    matched = match_path(ctx.attr.pattern, ctx.attr.path)
    return [
        _PatternMatchProbeInfo(matched = matched),
        DefaultInfo(),
    ]

_pattern_match_probe = rule(
    implementation = _pattern_match_probe_impl,
    attrs = {
        "pattern": attr.string(mandatory = True),
        "path": attr.string(mandatory = True),
    },
)

def _pattern_match_valid_parity_impl(env, target):
    for case in PATTERN_CASES:
        if case.malformed:
            continue
        got = match_path(case.pattern, case.path)
        env.expect.that_bool(got).equals(case.want)

def _malformed_pattern_test_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("malformed pattern:"),
    )

def _pattern_match_valid_parity_test(name):
    analysis_test(
        name = name,
        target = _TARGET,
        impl = _pattern_match_valid_parity_impl,
        attr_values = {"size": "small"},
    )

def _pattern_match_malformed_tests(name):
    idx = 0
    test_names = []
    for case in PATTERN_CASES:
        if case.malformed:
            idx += 1
            probe_name = "%s_probe_%d" % (name, idx)
            test_name = "%s_%d" % (name, idx)
            _pattern_match_probe(
                name = probe_name,
                pattern = case.pattern,
                path = case.path,
                tags = ["manual"],
            )
            analysis_test(
                name = test_name,
                target = ":" + probe_name,
                expect_failure = True,
                impl = _malformed_pattern_test_impl,
                attr_values = {"size": "small"},
            )
            test_names.append(test_name)
    native.test_suite(
        name = name,
        tests = test_names,
    )

def pattern_match_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _pattern_match_valid_parity_test,
            _pattern_match_malformed_tests,
        ],
    )

