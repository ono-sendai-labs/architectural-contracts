"""Analysis test for match_path Starlark parity with Go path.Match."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("//bazel_rules/go/private:paths.bzl", "match_path")

_TARGET = "//bazel_rules/go/tests/testdata/api:api_component"

_CASES = [
    ("example.com/foo", "example.com/foo", True),
    ("example.com/foo", "example.com/bar", False),
    ("example.com/foo/*", "example.com/foo/bar", True),
    ("example.com/foo/*", "example.com/foo/bar/baz", False),
    ("example.com/foo/*", "example.com/foo", False),
    ("example.com/foo/b*", "example.com/foo/bar", True),
    ("example.com/foo/b*", "example.com/foo/car", False),
    ("example.com/foo/ba?", "example.com/foo/bar", True),
    ("example.com/foo/ba?", "example.com/foo/b", False),
    ("example.com/foo/ba?", "example.com/foo/barr", False),
    ("example.com/foo/ba?", "example.com/foo/ba/", False),
    ("*.com/*/*", "example.com/foo/bar", True),
    ("*.com/*/*", "example.com/foo/bar/baz", False),
    ("", "", True),
    ("", "a", False),
    ("a", "", False),
    ("example.com/foo*", "example.com/foobar", True),
    ("example.com/foo*", "example.com/foo/bar", False),
    ("example.com/foo/\\*", "example.com/foo/*", True),
    ("example.com/foo/\\*", "example.com/foo/bar", False),
    ("example.com/v[0-9]", "example.com/v1", True),
    ("example.com/v[0-9]", "example.com/va", False),
]

def _pattern_match_parity_test(name):
    analysis_test(
        name = name,
        target = _TARGET,
        impl = _pattern_match_parity_impl,
        attr_values = {"size": "small"},
    )

def _pattern_match_parity_impl(env, target):
    for pattern, path_str, want in _CASES:
        got = match_path(pattern, path_str)
        env.expect.that_bool(got).equals(want)

def pattern_match_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _pattern_match_parity_test,
        ],
    )
