"""arcc invocation, factored out so the test rule and a future validation
action build the identical command (R7).

`arcc_check_test` runs this as a `bazel test`; the planned `_arcc_validation`
action will run the same argv to fail a `bazel build` instead. Keeping the
command in one place means the two enforcement modes can never drift.
"""

def arcc_check_argv(arcc, manifest, layout):
    """Argv for `arcc check`. Every path is runfiles-root-relative (see
    paths.bzl): the caller runs it with the runfiles root as the working
    directory."""
    argv = [
        arcc,
        "check",
        manifest,
    ]
    if layout:
        argv.append("--package-layout=" + layout)
    argv.append("--format=json")
    return argv
