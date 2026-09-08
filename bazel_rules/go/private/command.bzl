"""arcc invocation, factored out so every enforcement mode builds the
identical command (R7).

`arcc_check_argv` is the single construction point: the `.check` assertion
rules run it as `bazel test` enforcement, the component analysis action
(`component.bzl`'s ArccCheck, via its generated frame wrapper) runs the same
argv in always-green report-verdict-only mode, and the report-golden
assertion rule reuses it with a text-format override. One construction site
means the enforcement modes can never drift.
"""

def arcc_check_argv(
        arcc,
        manifest,
        layout,
        stdlib_map = None,
        report_out = None,
        surface_out = None,
        verdict_only = False,
        format_json = True):
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
    report-verdict-only mode the action requires (design §Build topology);
    `format_json` selects the report encoding (False for the text-golden
    assertion rule).
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
    if format_json:
        argv.append("--format=json")
    return argv
