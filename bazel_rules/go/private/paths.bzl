"""The runfiles-root-relative path frame shared across the Go rules.

Every path arcc reads — interface files, layout sources, the manifest and
layout given on the command line — is expressed relative to the runfiles root,
because the check runs with the runfiles root as its working directory. It is
the one frame in which a main-repo source and an external-repo source can both
be named without `..` segments, which arcc rejects as escaping the workspace.

The `_go_component` rule writes paths in this frame; the `arcc_check_test`
launcher `cd`s to the runfiles root so those paths resolve. They must therefore
agree on how the frame is computed, which is why this lives in one place.
"""

def runfiles_path(ctx, file):
    """Path of `file` relative to the runfiles root."""
    short_path = file.short_path
    if short_path.startswith("../"):
        return short_path[len("../"):]
    return ctx.workspace_name + "/" + short_path
