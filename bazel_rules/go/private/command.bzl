"""arcc invocation, factored out so every enforcement mode builds the
identical command (R7).

Two argv construction points, one per contract: `arcc_check_argv` is the
analysis argv — the component analysis action (component.bzl's ArccCheck, via
its generated frame wrapper) runs it in always-green report-verdict-only
mode, and `arcc_checked_analysis_test` re-executes the exact same command, so
the analysis and its execution coverage cannot drift. `arcc_verdict_argv` is
the report-assertion argv — the `.check` and grep assertion rules consume the
provider's canonical report through it (Step 5 task 06) and never re-run the
analysis; the golden rule diffs the provider report directly against its
golden, needing no tool.
"""

def arcc_check_argv(
        arcc,
        manifest,
        layout,
        stdlib_map = None,
        report_out = None,
        surface_out = None,
        verdict_only = False):
    """Argv for `arcc check`, the analysis argv (R7).

    The component analysis action (checked provenance) and
    `arcc_checked_analysis_test` — its execution coverage — build the same
    command through this one function, so the two can never drift. The report
    assertion rules never build this argv; they consume the analysis
    action's persisted report through `arcc_verdict_argv` instead.

    Every path is frame-relative to the caller's working-directory contract
    (see paths.bzl): the analysis action recreates that frame in the sandbox;
    `arcc_checked_analysis_test` runs it with the runfiles root as the working
    directory. Callers that emit artifacts pass `report_out`/`surface_out`
    (the artifact destinations) and `stdlib_map` (the declared map the
    surface's SDK key is stamped from); `verdict_only` selects the
    always-green report-verdict-only mode the action requires (design §Build
    topology). The JSON display encoding is unconditional: the persisted
    artifact is canonical JSON regardless.
    """
    argv = [
        arcc,
        "check",
        manifest,
    ]
    if layout:
        argv.append("--package-layout=" + layout)
    if stdlib_map:
        argv.append("--stdlib-map=" + stdlib_map)
    if report_out:
        argv.append("--report-out=" + report_out)
    if surface_out:
        argv.append("--surface-out=" + surface_out)
    if verdict_only:
        argv.append("--report-verdict-only")
    argv.append("--format=json")
    return argv

_ARCC_VERDICTS = ("pass", "fail")

def arcc_verdict_argv(arcc, report, expect):
    """Argv for `arcc verdict <report> --expect=pass|fail`, the report-assertion argv.

    The `.check`, grep, and golden assertion rules consume the provider's
    canonical `ArccComponentInfo.report` through this one construction point
    (Step 5 task 06): the recorded verdict is asserted, never recomputed, and
    the argv cannot drift between the rules. `expect` must be one of the two
    verdict constants — it is interpolated into generated shell code.
    """
    if expect not in _ARCC_VERDICTS:
        fail("arcc verdict expectation must be one of %s, got %r" % (
            ", ".join(_ARCC_VERDICTS),
            expect,
        ))
    return [arcc, "verdict", report, "--expect=" + expect]
