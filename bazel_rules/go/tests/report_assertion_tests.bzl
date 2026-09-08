"""Analysis tests for the report-assertion check rules (Step 5 task 06).

The three `.check` assertion rules consume the provider's canonical
`ArccComponentInfo.report` — they never re-run `arcc check` (design R8, §Build
topology). These tests pin that contract structurally at analysis time:
launcher content names only the report-assertion argv (`arcc verdict`, grep,
diff), and runfiles contain only the report plus the assertion-specific arcc
or golden inputs — no SDK sources, manifest/layout, or component source
closure (AC 3). The producer-chain and laziness tests cover AC 4 and AC 6.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")

_API_CHECK = "//bazel_rules/go/tests/testdata/api:api_component.check"
_API_COMPONENT = "//bazel_rules/go/tests/testdata/api:api_component"
_API_GREP = "//bazel_rules/go/tests:api_component_grep_report_test"
_GREP_NEGATIVE = "//bazel_rules/go/tests:undeclared_member_dep_fails_test"
_GOLDEN = "//bazel_rules/go/tests:api_component_json_report_golden_test"
_CONSUMER = "//bazel_rules/go/tests/testdata/infra/consumer:consumer_component"
_ASSERTED_GREP = "//bazel_rules/go/tests:asserted_component_grep_rejected_test"
_ASSERTED_GOLDEN = "//bazel_rules/go/tests:asserted_component_golden_rejected_test"

def _assert_absent(env, content, token):
    """Starlark-truth has no `not_contains` on strings; absence is asserted here."""
    if token in content:
        env.fail("launcher must not contain %r:\n%s" % (token, content))

def _launcher(env, target):
    """The generated shell launcher of a report-assertion test target."""
    for action in target.actions:
        for output in action.outputs.to_list():
            if output.extension == "sh":
                if action.content == None:
                    env.fail("launcher action %s carries no content" % output.short_path)
                return action.content
    env.fail("no launcher (.sh) action found on %s" % target.label)
    return None

def _runfile_basenames(target):
    return sorted([f.basename for f in target[DefaultInfo].default_runfiles.files.to_list()])

def _check_asserts_recorded_verdict_test(name):
    # AC 1: the normal-path check invokes `arcc verdict --expect=pass` on the
    # provider's report and never reruns the analysis (no arcc check argv, no
    # manifest/layout/stdlib-map inputs).
    analysis_test(
        name = name,
        target = _API_CHECK,
        impl = _check_asserts_recorded_verdict_impl,
        attr_values = {"size": "small"},
    )

def _check_asserts_recorded_verdict_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("verdict")
    env.expect.that_str(content).contains("'--expect=pass'")
    env.expect.that_str(content).contains("api_component.report.json")
    _assert_absent(env, content, "'check'")
    _assert_absent(env, content, "--package-layout")
    _assert_absent(env, content, "--stdlib-map")
    _assert_absent(env, content, "--report-out")

    # AC 3: runfiles are exactly the report and arcc — the launcher must not
    # stage SDK sources, manifests/layouts, or the component's source closure.
    env.expect.that_collection(_runfile_basenames(target)).contains_exactly([
        "api_component.check.sh",
        "api_component.report.json",
        "arcc",
    ])

def _grep_asserts_verdict_and_strings_test(name):
    # AC 2 (positive): the grep rule asserts the recorded verdict first, then
    # fixed-string greps the canonical report artifact.
    analysis_test(
        name = name,
        target = _API_GREP,
        impl = _grep_asserts_verdict_and_strings_impl,
        attr_values = {"size": "small"},
    )

def _grep_asserts_verdict_and_strings_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("verdict")
    env.expect.that_str(content).contains("'--expect=pass'")
    env.expect.that_str(content).contains("grep -F")
    env.expect.that_str(content).contains("api_component.report.json")
    _assert_absent(env, content, "'check'")
    _assert_absent(env, content, "--package-layout")
    env.expect.that_collection(_runfile_basenames(target)).contains_exactly([
        "api_component.report.json",
        "api_component_grep_report_test.sh",
        "arcc",
    ])

def _grep_negative_expects_fail_test(name):
    # AC 1 (negative side): expect_status 1 selects `--expect=fail` — the
    # recorded violation is the expectation, and the analysis is still never
    # rerun.
    analysis_test(
        name = name,
        target = _GREP_NEGATIVE,
        impl = _grep_negative_expects_fail_impl,
        attr_values = {"size": "small"},
    )

def _grep_negative_expects_fail_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("'--expect=fail'")
    env.expect.that_str(content).contains("grep -F")
    env.expect.that_str(content).contains("UNDECLARED_DEPENDENCY")
    env.expect.that_str(content).contains("undeclared_dep_component.report.json")
    _assert_absent(env, content, "'check'")
    env.expect.that_collection(_runfile_basenames(target)).contains_exactly([
        "arcc",
        "undeclared_dep_component.report.json",
        "undeclared_member_dep_fails_test.sh",
    ])

def _golden_diffs_provider_report_test(name):
    # AC 2 (golden): the golden rule diffs the provider's canonical report
    # directly; runfiles are exactly the report and the golden — no arcc, no
    # analysis inputs (AC 3).
    analysis_test(
        name = name,
        target = _GOLDEN,
        impl = _golden_diffs_provider_report_impl,
        attr_values = {"size": "small"},
    )

def _golden_diffs_provider_report_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("diff -u")
    env.expect.that_str(content).contains("api_component.report.json")
    _assert_absent(env, content, "verdict")
    _assert_absent(env, content, "'check'")
    _assert_absent(env, content, "grep -F")
    # Both runfiles are named `api_component.report.json`: the provider's
    # artifact and the golden — the exact same basename is the point, the
    # launcher diffs one against the other. The duplicate pins "exactly two
    # files, nothing else".
    env.expect.that_collection(_runfile_basenames(target)).contains_exactly([
        "api_component.report.json",
        "api_component.report.json",
        "api_component_json_report_golden_test.sh",
    ])

def _dependent_builds_producer_chain_once_test(name):
    # AC 4: the dependent's own analysis action exists exactly once and
    # declares the auto-attached dependency's report and surface as inputs,
    # so testing the dependent's `.check` builds both producers exactly once
    # through the build graph — no duplicate analysis command.
    analysis_test(
        name = name,
        target = _CONSUMER,
        impl = _dependent_builds_producer_chain_once_impl,
        attr_values = {"size": "small"},
    )

def _checked_actions(target):
    return [a for a in target.actions if a.mnemonic == "ArccCheck"]

def _dependent_builds_producer_chain_once_impl(env, target):
    checked = _checked_actions(target)
    if len(checked) != 1:
        env.fail("expected exactly one ArccCheck action on the dependent, found %d" % len(checked))
    inputs = [f.basename for f in checked[0].inputs.to_list()]
    env.expect.that_collection(inputs).contains("runtime_component.report.json")
    env.expect.that_collection(inputs).contains("runtime_component.surface.json")

    info = target[ArccComponentInfo]
    if info.report == None:
        env.fail("a checked dependent must publish its own report")
    env.expect.that_str(info.report.basename).equals("consumer_component.report.json")

def _checked_artifacts_stay_lazy_test(name):
    # AC 6: the report and surface ride in the `arcc` output group only —
    # default outputs stay manifest + layout, so `bazel build //...` without
    # the group schedules no analysis.
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _checked_artifacts_stay_lazy_impl,
        attr_values = {"size": "small"},
    )

def _checked_artifacts_stay_lazy_impl(env, target):
    outputs = [f.basename for f in target[DefaultInfo].files.to_list()]
    env.expect.that_collection(outputs).contains_exactly([
        "api_component.component.textproto",
        "api_component.package-layout.json",
    ])

    info = target[ArccComponentInfo]
    env.expect.that_collection([f.basename for f in target[OutputGroupInfo]["arcc"].to_list()]).contains_exactly([
        "api_component.report.json",
        "api_component.surface.json",
    ])
    env.expect.that_str(info.provenance).equals("checked")

def _asserted_grep_rejected_test(name):
    # AC 5: attaching the grep assertion to an asserted component fails
    # analysis with the checked-report requirement — never a pass or a new
    # analysis.
    analysis_test(
        name = name,
        target = _ASSERTED_GREP,
        expect_failure = True,
        impl = _asserted_assertion_rejected_impl,
        attr_values = {"size": "small"},
    )

def _asserted_golden_rejected_test(name):
    # AC 5: same requirement for the golden rule.
    analysis_test(
        name = name,
        target = _ASSERTED_GOLDEN,
        expect_failure = True,
        impl = _asserted_assertion_rejected_impl,
        attr_values = {"size": "small"},
    )

def _asserted_assertion_rejected_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("asserted component"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("no checked"),
    )

def report_assertion_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _check_asserts_recorded_verdict_test,
            _grep_asserts_verdict_and_strings_test,
            _grep_negative_expects_fail_test,
            _golden_diffs_provider_report_test,
            _dependent_builds_producer_chain_once_test,
            _checked_artifacts_stay_lazy_test,
            _asserted_grep_rejected_test,
            _asserted_golden_rejected_test,
        ],
    )
