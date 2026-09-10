"""The `arcc_stdlib_map` rule: a hermetic stdlib authority map for one target
SDK configuration (Step 4, task 06).

The rule invokes the explicit-input generation path of `arcc stdlibmap
generate` (task 05) as ONE ordinary hermetic action. Its declared inputs are
exactly the SDK sources, the toolchain-owned stdlib package-list file, the
analysis-time target-configuration file, and the arcc generator as an
exec-configuration tool; its single output is the canonical map. No
toolchain `go` binary, tool binary, build cache, home path or network is an
input or requirement: the generator loads the standard library through
`packagelayout`'s GOPACKAGESDRIVER self-exec driver (arcc re-executing
itself) and runs Capslock in-process, so the action sets no GOROOT, GOCACHE
or PATH at all (design I5, Appendix C).

The action blocks network access. Generation is keyed to the target
configuration — the config file stamps the SDK key (target configuration,
classifier fingerprint and map format version are carried by the generator)
— so cross-compilation and build-tag transitions build distinct, correctly
stamped maps on one execution host.

A cgo-enabled target configuration is rejected at analysis time, naming the
target and `--@rules_go//go/config:pure`: hermetic cgo generation needs a
C-toolchain seam this design does not define, and silently serving a cgo-off
map instead would launder cgo-reaching stdlib symbols as analysed (design
§Explicitly out of scope).

The rule is exposed publicly through `defs.bzl`; this file and the default
seam helper stay private so Step 5's analysis action can attach the default
map without changing check execution.
"""

load("//bazel_rules/go:providers.bzl", "ArccStdlibMapInfo")
load(":arcc_metadata.bzl", "CLASSIFIER_HASH", "MAP_FORMAT_VERSION")
load(
    ":go_adapter.bzl",
    "GO_CONTEXT_DATA_ATTRS",
    "GO_TOOLCHAINS",
    "go_stdlib_toolchain",
    "go_target_mode",
)

# The default stdlib-map target the Step 5 analysis action attaches as
# `_stdlib_map`. A host with its own pinned SDK overrides this in its adapter.
DEFAULT_STDLIB_MAP_TARGET = Label("//:arcc_stdlib_map")

def stdlib_map_default_attr(doc = None):
    """The private default-label attribute for the stdlib map (task req 7).

    Step 5's analysis action merges this into its attrs as `_stdlib_map`
    (stdlib_map_default_attr()); until then nothing consumes it, so no
    analysis action exists and check verdict execution is unchanged.
    """
    return attr.label(
        default = DEFAULT_STDLIB_MAP_TARGET,
        cfg = "host",
        providers = [ArccStdlibMapInfo],
        doc = doc or "The stdlib authority map for the execution target configuration.",
    )

def _stdlib_map_config_content(mode):
    """The deterministic target-configuration file the action passes through.

    Exactly the format `stdlibmap.RenderTargetConfig` (task 05) renders and
    `ParseTargetConfig` parses: six fixed key=value lines in a fixed order.
    """
    lines = [
        "toolchain_version=" + mode.toolchain_version,
        "goos=" + mode.goos,
        "goarch=" + mode.goarch,
        "cgo_enabled=" + ("true" if mode.cgo_enabled else "false"),
        "build_tags=" + ",".join(mode.tags),
        "goexperiment=" + mode.goexperiment,
    ]
    return "\n".join(lines) + "\n"

def _arcc_stdlib_map_impl(ctx):
    label = ctx.label
    toolchain = go_stdlib_toolchain(ctx)
    mode = go_target_mode(ctx)

    # Fail fast, naming the target, when the toolchain exposes no version, no
    # package list or no target platform (task req 4): the SDK key and the
    # oracle would be unstampable/absent, and the failure must not surface as
    # a mid-generation error.
    if not mode.goos or not mode.goarch:
        fail("stdlib map %s: the toolchain exposes no target GOOS/GOARCH; arcc cannot key the map" % label.name)
    if mode.toolchain_version == "":
        fail("stdlib map %s: the toolchain exposes no version; arcc cannot stamp the SDK key" % label.name)
    if toolchain.package_list == None:
        fail("stdlib map %s: the toolchain provides no stdlib package-list file" % label.name)

    # cgo-enabled target configurations are out of scope for hermetic
    # generation (design §Explicitly out of scope): rejected at analysis time
    # rather than producing, or silently substituting, a map.
    if mode.cgo_enabled:
        fail(("stdlib map %s: the target configuration is cgo-enabled; cgo-enabled maps " +
              "are out of scope for hermetic generation. Build with " +
              "--@rules_go//go/config:pure (hermetic cgo generation needs the SDK's cgo " +
              "packages preprocessed by a C toolchain, which this design defines no seam " +
              "for); a cgo-off map is never substituted.") % label.name)

    config = ctx.actions.declare_file(label.name + ".stdlib-map-config")
    ctx.actions.write(output = config, content = _stdlib_map_config_content(mode))

    output = ctx.actions.declare_file(label.name + ".stdlib-map.json")

    # The SDK root the generator loads the standard library from: rules_go's
    # ROOT file marks the SDK directory, whose src/ subtree the layout driver
    # discovers. The path is execroot-relative, the working directory of a
    # sandboxed action.
    sdk_root = toolchain.root_file.dirname + "/src"

    # One ordinary hermetic action; no environment is constructed because the
    # generator needs none (no GOROOT, GOCACHE or PATH — design I5). The
    # generator is an exec-configuration tool: it describes the target
    # through its declared configuration inputs.
    ctx.actions.run(
        executable = ctx.executable._arcc,
        arguments = [
            "stdlibmap",
            "generate",
            "--output=" + output.path,
            "--package-list=" + toolchain.package_list.path,
            "--config-file=" + config.path,
            "--sdk-root=" + sdk_root,
        ],
        inputs = depset(
            direct = [toolchain.package_list, config],
            transitive = [toolchain.srcs],
        ),
        tools = [ctx.executable._arcc],
        outputs = [output],
        execution_requirements = {"block-network": "1"},
        mnemonic = "ArccStdlibMap",
        progress_message = "Generating stdlib authority map %s (%s/%s)" % (label, mode.goos, mode.goarch),
        use_default_shell_env = False,
    )

    return [
        DefaultInfo(files = depset([output])),
        ArccStdlibMapInfo(
            map = output,
            toolchain_version = mode.toolchain_version,
            goos = mode.goos,
            goarch = mode.goarch,
            cgo_enabled = mode.cgo_enabled,
            build_tags = mode.tags,
            goexperiment = mode.goexperiment,
            # The generator stamps the same values into the map's SDK key
            # (task req 5). Starlark cannot compute the classifier hash, so
            # the values are mirrored from arcc in arcc_metadata.bzl and
            # pinned against the stamped artifact by asserted_surface_sdk_key_test.
            classifier_hash = CLASSIFIER_HASH,
            map_format_version = MAP_FORMAT_VERSION,
        ),
    ]

arcc_stdlib_map_rule = rule(
    implementation = _arcc_stdlib_map_impl,
    attrs = {} | GO_CONTEXT_DATA_ATTRS | {
        "_arcc": attr.label(
            default = Label("//go/cmd/arcc-stdlibmap:arcc-stdlibmap"),
            executable = True,
            cfg = "exec",
            doc = "The generation-only binary whose explicit-input path runs. " +
                  "It is deliberately independent of the ordinary check path.",
        ),
    },
    toolchains = GO_TOOLCHAINS,
    provides = [ArccStdlibMapInfo],
    doc = "Builds the stdlib authority map for the target SDK configuration, hermetically.",
)
