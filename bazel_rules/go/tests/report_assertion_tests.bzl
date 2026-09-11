"""Analysis tests for the report-assertion check rules (Step 9 task 01).

The three `.check` assertion rules consume the provider's canonical
`ArccComponentInfo.report` — they never re-run `arcc check` (design R8, §Build
topology). These tests pin that contract structurally at analysis time:
launcher content names only the report-assertion argv (`arcc verdict`, grep,
and the verdict-golden file form), and runfiles contain only the report plus
the assertion-specific arcc or golden inputs — no SDK sources, manifest/layout,
or component source closure. The producer-chain and laziness tests cover the
same topology for the migrated semantic goldens.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")

_API_CHECK = "//bazel_rules/go/tests/testdata/api:api_component.check"
_API_COMPONENT = "//bazel_rules/go/tests/testdata/api:api_component"
_API_GREP = "//bazel_rules/go/tests:api_component_grep_report_test"
_GREP_NEGATIVE = "//bazel_rules/go/tests:undeclared_member_dep_fails_test"
_VERDICT_GOLDEN = "//bazel_rules/go/tests:api_component_verdict_golden_test"
_FAIL_VERDICT_GOLDEN = "//bazel_rules/go/tests:undeclared_dep_component_verdict_golden_test"
_HOSTILE_VERDICT_GOLDEN = "//bazel_rules/go/tests:hostile_verdict_golden_launcher_probe_test"
_CONSUMER = "//bazel_rules/go/tests/testdata/infra/consumer:consumer_component"
_ASSERTED_GREP = "//bazel_rules/go/tests:asserted_component_grep_rejected_test"
_ASSERTED_VERDICT_GOLDEN = "//bazel_rules/go/tests:asserted_component_verdict_golden_test"
_VERDICT_PORTABILITY = "//bazel_rules/go/tests:verdict_golden_portability_test"

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

def _verdict_golden_asserts_provider_report_test(name):
    # AC 1/4: a verdict golden consumes the canonical provider report and a
    # one-line verdict file through the shared `arcc verdict` behavior; it
    # does not compare the report bytes or rerun analysis.
    analysis_test(
        name = name,
        target = _VERDICT_GOLDEN,
        impl = _verdict_golden_asserts_provider_report_impl,
        attr_values = {"size": "small"},
    )

def _verdict_golden_asserts_provider_report_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("verdict")
    env.expect.that_str(content).contains("--expect-file=")
    env.expect.that_str(content).contains("api_component.report.json")
    env.expect.that_str(content).contains("api_component.verdict.golden")
    _assert_absent(env, content, "diff -u")
    _assert_absent(env, content, "'check'")
    _assert_absent(env, content, "--package-layout")
    _assert_absent(env, content, "--stdlib-map")
    env.expect.that_collection(_runfile_basenames(target)).contains_exactly([
        "api_component.report.json",
        "api_component.verdict.golden",
        "api_component_verdict_golden_test.sh",
        "arcc",
    ])

def _failing_verdict_golden_pins_fail_test(name):
    # AC 1/6: the violating checked component has a separate stable `fail`
    # golden, so the mismatch path remains actionable and can name both
    # expected and actual values through `arcc verdict`.
    analysis_test(
        name = name,
        target = _FAIL_VERDICT_GOLDEN,
        impl = _failing_verdict_golden_pins_fail_impl,
        attr_values = {"size": "small"},
    )

def _failing_verdict_golden_pins_fail_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("undeclared_dep_component.report.json")
    env.expect.that_str(content).contains("undeclared_dep_component.verdict.golden")
    env.expect.that_str(content).contains("--expect-file=")
    _assert_absent(env, content, "diff -u")
    env.expect.that_collection(_runfile_basenames(target)).contains_exactly([
        "arcc",
        "undeclared_dep_component.report.json",
        "undeclared_dep_component.verdict.golden",
        "undeclared_dep_component_verdict_golden_test.sh",
    ])

def _hostile_verdict_golden_stays_data_test(name):
    # AC 6/7: the golden is passed as a path to the CLI. Its hostile contents
    # never become generated shell source, and the runfiles remain minimal.
    analysis_test(
        name = name,
        target = _HOSTILE_VERDICT_GOLDEN,
        impl = _hostile_verdict_golden_stays_data_impl,
        attr_values = {"size": "small"},
    )

def _hostile_verdict_golden_stays_data_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("--expect-file=")
    env.expect.that_str(content).contains("hostile_verdict_input.txt")
    _assert_absent(env, content, "$(touch SHOULD_NOT_RUN)")
    _assert_absent(env, content, "diff -u")
    env.expect.that_collection(_runfile_basenames(target)).contains_exactly([
        "api_component.report.json",
        "hostile_verdict_golden_launcher_probe_test.sh",
        "arcc",
        "hostile_verdict_input.txt",
    ])

def _verdict_golden_rejects_asserted_provider_test(name):
    # AC 5: an asserted provider has no checked verdict and is rejected during
    # analysis by the same fail-closed guard as the other assertion rules.
    analysis_test(
        name = name,
        target = _ASSERTED_VERDICT_GOLDEN,
        expect_failure = True,
        impl = _asserted_verdict_golden_rejected_impl,
        attr_values = {"size": "small"},
    )

def _asserted_verdict_golden_rejected_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("asserted component"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("no checked verdict"),
    )

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

def _asserted_assertion_rejected_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("asserted component"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("no checked"),
    )

def _hostile_grep_strings_stay_data_test(name):
    # Review round 1 (important): expected strings are BUILD-authored data and
    # must never become generated shell code. The launcher passes them as
    # shell-quoted operands (`grep -F --`) and as printf data in the
    # missing-string diagnostic, so quotes, command substitution, `%` signs
    # and newlines stay inert even when the assertion fails.
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests:hostile_grep_strings_launcher_probe_test",
        impl = _hostile_grep_strings_stay_data_impl,
        attr_values = {"size": "small"},
    )

def _hostile_grep_strings_stay_data_impl(env, target):
    content = _launcher(env, target)
    env.expect.that_str(content).contains("grep -F --")
    env.expect.that_str(content).contains("printf '%s\\n'")
    # Each hostile string appears exactly once, as a shell-quoted operand.
    env.expect.that_str(content).contains("'$(whoami)'")
    env.expect.that_str(content).contains("''\\''; rm -rf / #'")
    env.expect.that_str(content).contains("'a\nb'")
    env.expect.that_str(content).contains("'-e'")
    env.expect.that_str(content).contains("'100%s'")
    # The old diagnostic interpolated the string raw into an echo: gone.
    _assert_absent(env, content, "expected string '")

def _verdict_golden_portability_guard_test(name):
    # Every file matching the verdict-golden naming convention is scanned by
    # the runtime guard, including synthetic contamination probes that prove
    # the diagnostic identifies both the file and forbidden content class.
    analysis_test(
        name = name,
        target = _VERDICT_PORTABILITY,
        impl = _verdict_golden_portability_guard_impl,
        attr_values = {"size": "small"},
    )

def _verdict_golden_portability_guard_impl(env, target):
    runfiles = _runfile_basenames(target)
    env.expect.that_collection(runfiles).contains("verdict_golden_portability_test.sh")
    env.expect.that_collection([name for name in runfiles if name.endswith(".verdict.golden")]).contains_at_least([
        "api_component.verdict.golden",
        "reportboundary_consumer.verdict.golden",
        "undeclared_dep_component.verdict.golden",
    ])

def report_assertion_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _check_asserts_recorded_verdict_test,
            _grep_asserts_verdict_and_strings_test,
            _grep_negative_expects_fail_test,
            _verdict_golden_asserts_provider_report_test,
            _failing_verdict_golden_pins_fail_test,
            _hostile_verdict_golden_stays_data_test,
            _dependent_builds_producer_chain_once_test,
            _checked_artifacts_stay_lazy_test,
            _asserted_grep_rejected_test,
            _verdict_golden_rejects_asserted_provider_test,
            _hostile_grep_strings_stay_data_test,
            _verdict_golden_portability_guard_test,
        ],
    )
