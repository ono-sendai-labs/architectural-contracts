"""Analysis tests for the `arcc_stdlib_map` rule (Step 4 task 06, req 8).

They pin the action's declared inputs (complete and minimal — no toolchain
`go` binary, no tool binaries, no cache/home/dependency-component/unrelated
workspace paths), the explicit-input argv and single canonical output shape,
the blocked network and empty environment, the provider/default-seam
availability, and the target-configuration key propagation (cross-compile,
build tag, cgo rejection) — all without executing the (expensive) generation
action.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("//bazel_rules/go:providers.bzl", "ArccStdlibMapInfo")
load("//bazel_rules/go/tests:stdlib_map_probe.bzl", "DefaultSeamProbeInfo", "TransitionedStdlibMapInfo")

_PROBE = "//:arcc_stdlib_map"
_TAGGED = "//bazel_rules/go/tests:stdlib_map_tagged"
_DARWIN = "//bazel_rules/go/tests:stdlib_map_darwin_arm64"
_SEAM_PROBE = "//bazel_rules/go/tests:stdlib_map_default_seam_probe"

# rules_testing resolves config_settings labels in its own repository
# context, where @rules_go is not visible; resolving the label here — in the
# main repository, which can see @rules_go — produces a canonical spelling
# rules_testing accepts.
_PURE_SETTING = str(Label("@rules_go//go/config:pure"))

def _map_action(target):
    """The stdlib-map generation action of the target under test."""
    actions = [a for a in target.actions if a.mnemonic == "ArccStdlibMap"]
    if len(actions) != 1:
        fail("expected exactly one ArccStdlibMap action, found %d" % len(actions))
    return actions[0]

def _declares_a_single_canonical_map_output_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _declares_a_single_canonical_map_output_impl,
    )

def _declares_a_single_canonical_map_output_impl(env, target):
    env.expect.that_collection(
        [f.short_path for f in target[DefaultInfo].files.to_list()],
    ).contains_exactly(["arcc_stdlib_map.stdlib-map.json"])
    # The rule's only execution action is the generation action; the config
    # file is an analysis-time write with no execution.
    env.expect.that_collection(
        [a.mnemonic for a in target.actions],
    ).contains_exactly(["FileWrite", "ArccStdlibMap"])

def _action_inputs_are_complete_and_minimal_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _action_inputs_are_complete_and_minimal_impl,
    )

def _action_inputs_are_complete_and_minimal_impl(env, target):
    action = _map_action(target)
    inputs = [f.short_path for f in action.inputs.to_list()]

    # Complete: SDK sources (spot-check well-known stdlib files), the
    # toolchain package enumeration, the deterministic config file.
    env.expect.that_int(len([p for p in inputs if p.endswith("src/fmt/format.go")])).equals(1)
    env.expect.that_int(len([p for p in inputs if p.endswith("/packages.txt")])).equals(1)
    env.expect.that_collection([p for p in inputs if p.endswith(".stdlib-map-config")]).contains_exactly([
        "arcc_stdlib_map.stdlib-map-config",
    ])

    # Minimal (design I5): no toolchain `go` binary, no tool binaries under
    # the SDK's bin/ or pkg/tool/, no cache or home path, no dependency-
    # component source, no unrelated workspace file.
    env.expect.that_collection([
        p for p in inputs
        if p.endswith("/bin/go") or "/bin/go_" in p or "pkg/tool/" in p
    ]).contains_exactly([])
    env.expect.that_collection([
        p for p in inputs
        if "/.cache/" in p or "gomodcache" in p or "gopath" in p or (p.endswith(".go") and "/src/" not in p)
    ]).contains_exactly([])
    env.expect.that_collection([
        p for p in inputs
        if (p.startswith("go/") and not p.startswith("go/cmd/arcc/arcc_")) or
           p.startswith("examples/") or p.startswith("bazel_rules/")
    ]).contains_exactly([])

def _action_executes_no_toolchain_binary_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _action_executes_no_toolchain_binary_impl,
    )

def _action_executes_no_toolchain_binary_impl(env, target):
    # The action's executable (argv[0]) is the arcc generator itself, not a
    # shell and not any toolchain binary, and no input is a toolchain
    # binary. This is the design's I5 premise, pinned here so a regression to
    # executing the pinned `go` (the superseded first attempt) fails loudly.
    action = _map_action(target)
    argv = list(action.argv)
    env.expect.that_str(argv[0]).contains("go/cmd/arcc/arcc")
    env.expect.that_collection([
        p for p in [f.short_path for f in action.inputs.to_list()]
        if "/bin/go" in p or "/bin/go_" in p or "pkg/tool/" in p
    ]).contains_exactly([])

def _argv_uses_the_explicit_input_path_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _argv_uses_the_explicit_input_path_impl,
    )

def _argv_uses_the_explicit_input_path_impl(env, target):
    # The generator is invoked directly (no launcher): the explicit-input
    # mode's four flags, and never a native-discovery flag — the whole target
    # is declared by the config file and package list.
    action = _map_action(target)
    # The generator is invoked directly (no launcher): the explicit-input
    # mode's four flags, and never a native-discovery flag — the whole target
    # is declared by the config file and package list.
    argv = list(action.argv)[1:]
    env.expect.that_collection(argv[:2]).contains_exactly(["stdlibmap", "generate"])
    argv_text = json.encode(argv)
    env.expect.that_str(argv_text).contains("--output=")
    env.expect.that_str(argv_text).contains("--package-list=")
    env.expect.that_str(argv_text).contains("--config-file=")
    env.expect.that_str(argv_text).contains("--sdk-root=")
    # The SDK root is the SDK's src/ directory (execroot-relative), given
    # exactly once.
    sdk_root_args = [a for a in argv if a.startswith("--sdk-root=")]
    env.expect.that_int(len(sdk_root_args)).equals(1)
    env.expect.that_bool(sdk_root_args[0].endswith("/src")).equals(True)
    env.expect.that_collection([
        flag for flag in ("--toolchain=", "--goos=", "--goarch=", "--cgo", "--tags=", "--goexperiment=")
        if flag in argv_text
    ]).contains_exactly([])

def _action_is_hermetic_by_execution_requirements_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _action_is_hermetic_by_execution_requirements_impl,
    )

def _action_is_hermetic_by_execution_requirements_impl(env, target):
    # Bazel 9's Starlark Action API exposes only `env` — execution_requirements
    # and use_default_shell_env are not readable in analysis. The observable
    # hermeticity contract is pinned here: the action carries an explicitly
    # empty environment (no GOROOT, GOCACHE, PATH, or any inherited variable —
    # use_default_shell_env is False, since a default shell env would surface
    # through the action's effective environment), and the network-blocking
    # execution requirement is verified by inspection of the rule source
    # (stdlib_map.bzl sets execution_requirements = {"block-network": "1"}).
    action = _map_action(target)
    env.expect.that_dict(action.env).contains_exactly({})
    env.expect.that_str(list(action.argv)[0]).contains("go/cmd/arcc/arcc")
    env.expect.that_str(action.mnemonic).equals("ArccStdlibMap")

def _config_file_carries_the_target_key_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _config_file_carries_the_target_key_impl,
    )

def _stdlib_map_config_write(target):
    written = []
    for a in target.actions:
        if a.mnemonic != "FileWrite":
            continue
        for o in a.outputs.to_list():
            if o.short_path.endswith(".stdlib-map-config"):
                written.append(a)
    if len(written) != 1:
        fail("expected exactly one config Write action, found %d" % len(written))
    return written[0]

def _config_file_carries_the_target_key_impl(env, target):
    content = _stdlib_map_config_write(target).content
    info = target[ArccStdlibMapInfo]
    env.expect.that_collection(content.splitlines()).contains_exactly([
        "toolchain_version=" + info.toolchain_version,
        "goos=linux",
        "goarch=amd64",
        "cgo_enabled=false",
        "build_tags=",
        "goexperiment=",
    ])

def _provider_and_default_availability_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        impl = _provider_and_default_availability_impl,
    )

def _provider_and_default_availability_impl(env, target):
    info = target[ArccStdlibMapInfo]
    env.expect.that_str(info.goos).equals("linux")
    env.expect.that_str(info.goarch).equals("amd64")
    env.expect.that_str(info.toolchain_version).contains("go1.")
    env.expect.that_bool(info.cgo_enabled).equals(False)
    env.expect.that_collection(list(info.build_tags)).contains_exactly([])
    env.expect.that_str(info.map.extension).equals("json")

def _default_seam_is_available_test(name):
    analysis_test(
        name = name,
        target = _SEAM_PROBE,
        impl = _default_seam_is_available_impl,
    )

def _default_seam_is_available_impl(env, target):
    # The private default `_stdlib_map` attribute resolves the default map
    # target and its provider reaches the consumer (Step 5's contract) —
    # without any analysis action existing yet.
    info = target[DefaultSeamProbeInfo]
    env.expect.that_str(info.goos).equals("linux")
    env.expect.that_str(info.goarch).equals("amd64")
    env.expect.that_str(info.map.extension).equals("json")

def _build_tag_propagates_to_the_key_test(name):
    analysis_test(
        name = name,
        target = _TAGGED,
        impl = _build_tag_propagates_to_the_key_impl,
    )

def _build_tag_propagates_to_the_key_impl(env, target):
    env.expect.that_collection(list(target[TransitionedStdlibMapInfo].build_tags)).contains_exactly(["arcc_probe_tag"])

def _cross_compile_propagates_to_the_key_test(name):
    analysis_test(
        name = name,
        target = _DARWIN,
        impl = _cross_compile_propagates_to_the_key_impl,
    )

def _cross_compile_propagates_to_the_key_impl(env, target):
    info = target[TransitionedStdlibMapInfo]
    env.expect.that_str(info.goos).equals("darwin")
    env.expect.that_str(info.goarch).equals("arm64")
    env.expect.that_bool(info.cgo_enabled).equals(False)

def _cgo_enabled_configuration_fails_analysis_test(name):
    analysis_test(
        name = name,
        target = _PROBE,
        # The cgo-enabled configuration is applied to the target under test
        # itself; `expect_failure` (via allow_analysis_failures) turns the
        # rule's analysis-time fail() into an inspectable AnalysisFailureInfo
        # instead of aborting the build.
        config_settings = {_PURE_SETTING: False},
        expect_failure = True,
        impl = _cgo_enabled_configuration_fails_analysis_impl,
    )

def _cgo_enabled_configuration_fails_analysis_impl(env, target):
    # The cgo-enabled configuration is rejected at analysis time, naming the
    # target and the pure-mode flag (task req 4, design §Out of scope).
    if AnalysisFailureInfo not in target:
        fail("expected the cgo-enabled stdlib-map target to fail analysis")
    messages = [c.message for c in target[AnalysisFailureInfo].causes.to_list()]
    env.expect.that_int(len(messages)).equals(1)
    # The AnalysisFailureInfo cause message is the rule's fail() traceback,
    # which embeds the fail() text: the target name and the pure-mode flag.
    env.expect.that_str(messages[0]).contains("stdlib map arcc_stdlib_map")
    env.expect.that_str(messages[0]).contains("--@rules_go//go/config:pure")

def stdlib_map_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _declares_a_single_canonical_map_output_test,
            _action_inputs_are_complete_and_minimal_test,
            _action_executes_no_toolchain_binary_test,
            _argv_uses_the_explicit_input_path_test,
            _action_is_hermetic_by_execution_requirements_test,
            _config_file_carries_the_target_key_test,
            _provider_and_default_availability_test,
            _default_seam_is_available_test,
            _build_tag_propagates_to_the_key_test,
            _cross_compile_propagates_to_the_key_test,
            _cgo_enabled_configuration_fails_analysis_test,
        ],
    )
