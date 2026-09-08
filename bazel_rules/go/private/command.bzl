"""arcc invocation, factored out so the test rule and a future validation
action build the identical command (R7).

`arcc_check_test` runs this as a `bazel test`; the planned `_arcc_validation`
action will run the same argv to fail a `bazel build` instead. Keeping the
command in one place means the two enforcement modes can never drift.
"""

def arcc_check_argv(
        arcc,
        manifest,
        layout,
        stdlib_map = None,
        report_out = None,
        surface_out = None,
        verdict_only = False):
    """Argv for `arcc check`, shared by every enforcement mode (R7).

    The `.check` test rule (assertion) and the component analysis action
    (checked provenance) build the same command through this one function, so
    the two can never drift.

    Every path is frame-relative to the caller's working-directory contract
    (see paths.bzl): the test launcher runs it with the runfiles root as the
    working directory; the analysis action recreates that frame in the
    sandbox. Callers that emit artifacts pass `report_out`/`surface_out` (the
    artifact destinations) and `stdlib_map` (the declared map the surface's
    SDK key is stamped from); `verdict_only` selects the always-green
    report-verdict-only mode the action requires (design §Build topology).
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
