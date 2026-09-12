"""Analysis-time input-role tests for the Step 13 producer-chain fixtures."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")

_PACKAGE = "//bazel_rules/go/tests/testdata/producerchain:"
_VARIANTS = [
    "d01w01",
    "d01w02",
    "d01w04",
    "d04w01",
    "d04w02",
    "d04w04",
    "d16w01",
    "d16w02",
    "d16w04",
    "s0000",
    "s0008",
    "s0032",
    "s0128",
]

def _action_with_mnemonic(target, mnemonic):
    actions = [action for action in target.actions if action.mnemonic == mnemonic]
    if len(actions) != 1:
        fail("expected exactly one %s action, found %d" % (mnemonic, len(actions)))
    return actions[0]

def _has_suffix(paths, suffix):
    return any([path.endswith(suffix) for path in paths])

def _producer_chain_roles_impl(env, target):
    """Checks the three action boundaries for one measured component."""
    info = target[ArccComponentInfo]
    component = target.label.name
    if not component.endswith("_component"):
        env.fail("producer-chain analysis test target has unexpected name %s" % component)
    variant = component[:-len("_component")]

    graph = _action_with_mnemonic(target, "ArccImportGraph")
    layout = _action_with_mnemonic(target, "ArccLayout")
    check = _action_with_mnemonic(target, "ArccCheck")

    graph_inputs = [file.short_path for file in graph.inputs.to_list()]
    layout_inputs = [file.short_path for file in layout.inputs.to_list()]
    check_inputs = [file.short_path for file in check.inputs.to_list()]

    # ArccImportGraph is the only producer allowed to receive ordinary
    # non-member Go source. Its request and generated descriptor are private
    # to the two metadata actions, while the arcc executable/runfiles are
    # execution machinery rather than semantic inputs.
    graph_basenames = [path.rsplit("/", 1)[-1] for path in graph_inputs]
    env.expect.that_bool(_has_suffix(graph_basenames, ".package-imports.request.json")).equals(True)
    env.expect.that_bool(_has_suffix(graph_basenames, ".go")).equals(True)
    env.expect.that_bool(_has_suffix(graph_basenames, ".package-layout.base.json")).equals(False)
    env.expect.that_collection([
        path
        for path in graph_inputs
        if path.endswith(".x") or path.endswith(".surface.json") or
           path.endswith(".report.json") or path.endswith(".stdlib-map.json")
    ]).contains_exactly([])
    env.expect.that_dict(graph.env).contains_exactly({})

    member_source = variant + "_member.go"
    env.expect.that_collection([
        path
        for path in graph_basenames
        if path.endswith(".go") and path == member_source
    ]).contains_exactly([])

    # ArccLayout is a pure merge: only the base layout and the projection
    # descriptor cross its boundary, with no source or export material.
    layout_basenames = [path.rsplit("/", 1)[-1] for path in layout_inputs]
    env.expect.that_bool(_has_suffix(layout_basenames, ".package-imports.json")).equals(True)
    env.expect.that_bool(_has_suffix(layout_basenames, ".package-layout.base.json")).equals(True)
    env.expect.that_bool(_has_suffix(layout_basenames, ".package-imports.request.json")).equals(False)
    env.expect.that_collection([
        path
        for path in layout_inputs
        if path.endswith(".go") or path.endswith(".x") or
           path.endswith(".surface.json") or path.endswith(".report.json") or
           path.endswith(".stdlib-map.json")
    ]).contains_exactly([])
    env.expect.that_dict(layout.env).contains_exactly({})

    # The leaf action keeps the final N2 allowlist. It gets only the measured
    # member source plus export-backed ordinary dependencies, dependency
    # surface/report artifacts, the merged layout, and the target map.
    check_basenames = [path.rsplit("/", 1)[-1] for path in check_inputs]
    env.expect.that_bool(_has_suffix(check_basenames, ".package-layout.json")).equals(True)
    env.expect.that_bool(_has_suffix(check_basenames, ".package-imports.json")).equals(True)
    env.expect.that_bool(_has_suffix(check_basenames, ".stdlib-map.json")).equals(True)
    env.expect.that_bool(_has_suffix(check_basenames, ".x")).equals(True)
    env.expect.that_bool(_has_suffix(check_basenames, ".surface.json")).equals(True)
    env.expect.that_bool(_has_suffix(check_basenames, ".report.json")).equals(True)
    env.expect.that_bool(_has_suffix(check_basenames, member_source)).equals(True)
    env.expect.that_bool(_has_suffix(check_basenames, ".package-layout.base.json")).equals(False)
    env.expect.that_bool(_has_suffix(check_basenames, ".package-imports.request.json")).equals(False)
    env.expect.that_collection([
        basename
        for basename in check_basenames
        if basename.endswith(".go") and basename != member_source
    ]).contains_exactly([])
    env.expect.that_dict(check.env).contains_exactly({})

    # The producer's output names are the component provider artifacts; the
    # check action must remain one action producing both of them.
    env.expect.that_collection([file.basename for file in check.outputs.to_list()]).contains_exactly([
        info.report.basename,
        info.surface.basename,
    ])

def producer_chain_analysis_tests():
    """Declares one analysis test for every measured component variant."""
    for variant in _VARIANTS:
        analysis_test(
            name = "producer_chain_%s_roles_test" % variant,
            target = _PACKAGE + variant + "_component",
            impl = _producer_chain_roles_impl,
            attr_values = {"size": "small"},
        )
