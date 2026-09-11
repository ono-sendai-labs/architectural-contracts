"""Analysis tests for the explicit UNKNOWN asserted-component producer path.

The asserted surface is written at analysis time (design I6), so its full
content is visible to analysis tests through the write action's `content` —
no artifact needs to be built. The SDK key's *values* are pinned against the
checked emitter and the stamped map artifact by //bazel_rules/go/tests:asserted_surface_sdk_key_test,
which can compare built artifacts; the structural assertions live here.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")

_AssertedSurfaceComparisonInfo = provider(fields = ["package_sets"])

def _asserted_surface_comparison_impl(ctx):
    package_sets = []
    for component in [ctx.attr.component_a, ctx.attr.component_b, ctx.attr.component_duplicate]:
        info = component[ArccComponentInfo]
        if info.provenance != "asserted" or info.report != None or info.surface == None:
            fail("asserted surface comparison inputs must be asserted providers")
        package_sets.append(sorted([pkg.importpath for pkg in info.closure.to_list()]))
    return [_AssertedSurfaceComparisonInfo(package_sets = package_sets)]

asserted_surface_comparison = rule(
    implementation = _asserted_surface_comparison_impl,
    attrs = {
        "component_a": attr.label(
            mandatory = True,
            providers = [ArccComponentInfo],
        ),
        "component_b": attr.label(
            mandatory = True,
            providers = [ArccComponentInfo],
        ),
        "component_duplicate": attr.label(
            mandatory = True,
            providers = [ArccComponentInfo],
        ),
    },
)

_ASSERTED_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/manual:manual_component"
_TAGGED_DECLARED_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/manual:tagged_declared_component"
_DIRECT_INVALID_AUTHORITY_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/manual:direct_invalid_authority_component"

def _invalid_direct_authority_fails_test(name):
    analysis_test(
        name = name,
        target = _DIRECT_INVALID_AUTHORITY_COMPONENT,
        expect_failure = True,
        impl = _invalid_direct_authority_fails_impl,
        attr_values = {"size": "small"},
    )

def _invalid_direct_authority_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("direct_invalid_authority_component"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("unknown authority"),
    )

def _manual_tagged_declared_stays_checked_test(name):
    analysis_test(
        name = name,
        target = _TAGGED_DECLARED_COMPONENT,
        impl = _manual_tagged_declared_stays_checked_impl,
        attr_values = {"size": "small"},
    )

def _manual_tagged_declared_stays_checked_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_str(info.provenance).equals("checked")
    if info.report == None or info.surface == None:
        env.fail("manual-tagged DECLARED component must publish report and surface")
    env.expect.that_int(len([action for action in target.actions if action.mnemonic == "ArccCheck"])).equals(1)
    manifest = env.expect.that_target(target).action_generating(info.manifest.short_path).actual.content
    if "authority: UNKNOWN" in manifest:
        env.fail("manual tag must not synthesize UNKNOWN authority")

def _reordered_unknown_provider_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests:reordered_asserted_surface_comparison",
        impl = _reordered_unknown_provider_impl,
        attr_values = {"size": "small"},
    )

def _reordered_unknown_provider_impl(env, target):
    expected = [
        "example.com/aspect/shared",
        "example.com/reportboundary/manual",
    ]
    package_sets = target[_AssertedSurfaceComparisonInfo].package_sets
    env.expect.that_int(len(package_sets)).equals(3)
    for package_set in package_sets:
        env.expect.that_collection(package_set).contains_exactly(expected).in_order()
    if package_sets[0] != package_sets[1] or package_sets[0] != package_sets[2]:
        env.fail("equivalent UNKNOWN providers expose different package collections: %s" % package_sets)

def _explicit_check_of_asserted_component_fails_test(name):
    # AC 5 (task req 7): an explicitly requested `.check` of an asserted
    # component fails analysis with a clear no-checked-report diagnostic;
    # it must never run a check the asserted path avoided and pass from the
    # asserted surface. The guarded arcc_check_test target lives in
    # bazel_rules/go/tests/BUILD.bazel.
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests:asserted_component_check_rejected_test",
        expect_failure = True,
        impl = _explicit_check_of_asserted_component_fails_impl,
        attr_values = {"size": "small"},
    )

def _explicit_check_of_asserted_component_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("asserted component"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.contains("no checked report"),
    )

def _asserted_surface_action(env, target):
    """The FileWrite action producing the asserted `<name>.surface.json`."""
    info = target[ArccComponentInfo]
    actions = []
    for a in target.actions:
        if a.mnemonic != "FileWrite":
            continue
        for f in a.outputs.to_list():
            if f == info.surface:
                actions.append(a)
    if len(actions) != 1:
        fail("expected exactly one write action for the asserted surface, found %d" % len(actions))
    return actions[0]

def _provider_is_structurally_asserted_test(name):
    analysis_test(
        name = name,
        target = _ASSERTED_COMPONENT,
        impl = _provider_is_structurally_asserted_impl,
        attr_values = {"size": "small"},
    )

def _provider_is_structurally_asserted_impl(env, target):
    info = target[ArccComponentInfo]

    # AC 1: the asserted producer publishes the surface with report = None
    # and provenance = "asserted".
    env.expect.that_str(info.provenance).equals("asserted")
    if info.report != None:
        env.fail("asserted component's report must be None")
    if info.surface == None:
        env.fail("asserted component must publish a surface")

    # AC 1: the surface is named by the shared convention, so a dependent can
    # locate it exactly like a checked surface.
    env.expect.that_str(info.surface.basename).equals("manual_component.surface.json")

    # No component analysis action exists (AC 1): only the analysis-time
    # writes — manifest, layout, asserted surface — and never an ArccCheck.
    env.expect.that_collection([action.mnemonic for action in target.actions]).contains_exactly([
        "FileWrite",
        "FileWrite",
        "FileWrite",
    ])

def _surface_content_is_package_level_test(name):
    analysis_test(
        name = name,
        target = _ASSERTED_COMPONENT,
        impl = _surface_content_is_package_level_impl,
        attr_values = {"size": "small"},
    )

def _surface_content_is_package_level_impl(env, target):
    action = _asserted_surface_action(env, target)
    content = json.decode(action.content)

    # AC 2: exactly the I6 content, under the schema's field names —
    # packages from the layout, namespace, SDK key, format and producer
    # versions; no symbols and no digest.
    env.expect.that_int(content["formatVersion"]).equals(1)
    env.expect.that_str(content["component"]).equals("manual_component")
    env.expect.that_str(content["interfaceStyle"]).equals("INTERFACE_STYLE_PACKAGE_SURFACE")
    env.expect.that_str(content["authority"]["authority"]).equals("UNKNOWN")
    env.expect.that_collection(content["packages"]).contains_exactly([
        "example.com/reportboundary/manual",
    ])
    env.expect.that_str(content["namespace"]).equals("upstream")
    env.expect.that_str(content["producerVersion"]).equals("arcc 0.0.0-dev")

    sdk_key = content["sdkKey"]
    env.expect.that_str(sdk_key["toolchainVersion"]).contains("go1.")
    env.expect.that_str(sdk_key["goos"]).equals("linux")
    env.expect.that_str(sdk_key["goarch"]).equals("amd64")
    # The generator-owned key fields are mirrored in arcc_metadata.bzl; their
    # values are equality-pinned against the checked emitter by
    # asserted_surface_sdk_key_test. Here: present and well-formed.
    env.expect.that_int(len(sdk_key["classifierHash"])).equals(64)
    env.expect.that_int(sdk_key["mapFormatVersion"]).equals(1)

    # The asserted surface carries no symbols and the empty digest (I6);
    # canonically, zero-valued fields are omitted.
    if "symbols" in content:
        env.fail("asserted surface must carry no symbols")
    if "digest" in content:
        env.fail("asserted surface's digest is the empty string; it must not be present")
    if "cgoEnabled" in sdk_key:
        env.fail("zero-valued SDK-key fields must be omitted (cgo off)")

def _surface_schema_has_no_provenance_bit_test(name):
    analysis_test(
        name = name,
        target = _ASSERTED_COMPONENT,
        impl = _surface_schema_has_no_provenance_bit_impl,
        attr_values = {"size": "small"},
    )

def _surface_schema_has_no_provenance_bit_impl(env, target):
    # AC 3: only the producer/provider topology distinguishes asserted from
    # checked — the file carries no checked/asserted bit. The asserted
    # surface's keys must be a strict subset of the shared schema's fields.
    content = json.decode(_asserted_surface_action(env, target).content)
    env.expect.that_collection(content.keys()).contains_exactly([
        "formatVersion",
        "component",
        "interfaceStyle",
        "authority",
        "packages",
        "namespace",
        "sdkKey",
        "producerVersion",
    ])

def _default_outputs_unchanged_test(name):
    analysis_test(
        name = name,
        target = _ASSERTED_COMPONENT,
        impl = _default_outputs_unchanged_impl,
        attr_values = {"size": "small"},
    )

def _default_outputs_unchanged_impl(env, target):
    # AC 6: asserted artifacts are not materialized as default outputs —
    # only the manifest and the layout are (task req 4).
    env.expect.that_target(target).default_outputs().contains_exactly([
        "bazel_rules/go/tests/testdata/reportboundary/manual/manual_component.component.textproto",
        "bazel_rules/go/tests/testdata/reportboundary/manual/manual_component.package-layout.json",
    ])

    # The asserted surface rides in the `arcc` output group alone.
    group = target[OutputGroupInfo]["arcc"]
    env.expect.that_collection([f.basename for f in group.to_list()]).contains_exactly([
        "manual_component.surface.json",
    ])

def asserted_surface_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _provider_is_structurally_asserted_test,
            _surface_content_is_package_level_test,
            _surface_schema_has_no_provenance_bit_test,
            _default_outputs_unchanged_test,
            _manual_tagged_declared_stays_checked_test,
            _reordered_unknown_provider_test,
            _invalid_direct_authority_fails_test,
            _explicit_check_of_asserted_component_fails_test,
        ],
    )
