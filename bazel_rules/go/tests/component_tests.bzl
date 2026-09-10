"""Analysis tests for the `_go_component` rule.

The generated files' contents are compared against goldens by
//bazel_rules/go/tests:golden_test — analysis tests cannot read a file that
has not been built yet. What they can see is the provider contract and the
analysis-time errors, which is what is asserted here.
"""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_go//go:def.bzl", "GoInfo")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")

_API_COMPONENT = "//bazel_rules/go/tests/testdata/api:api_component"
_MEMBER_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:member_component"
_REVERSED_MEMBER_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:member_component_reversed"
_PACKAGE_SURFACE_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:package_surface_component"
_AUTO_ATTACHED_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:auto_attached_infra_component"
_CLOSURE_ATTACHED_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:closure_attached_infra_component"
_ROOT_COLLECTION_ATTACHED_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:root_collection_attached_infra_component"
_DECLINING_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:declining_infra_component"
_NEVER_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:never_infra_component"
_AUTHORED_WINS_INFRA_COMPONENT = "//bazel_rules/go/tests/testdata/membercomponent:authored_wins_infra_component"
_MULTI_INFRA_A = "//bazel_rules/go/tests/testdata/membercomponent:multi_infra_a"
_MULTI_INFRA_B = "//bazel_rules/go/tests/testdata/membercomponent:multi_infra_b"
_BINDINGS_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/consumer:bindings_component"

_MISSING_SURFACE_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/consumer:missing_surface_component"
_CHECKED_WITHOUT_REPORT_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/consumer:checked_without_report_component"
_ASSERTED_WITH_REPORT_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/consumer:asserted_with_report_component"
_UNKNOWN_PROVENANCE_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/consumer:unknown_provenance_component"
_DUPLICATE_DEPENDENCY_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/consumer:duplicate_dependency_component"
_CONFLICTING_EDGE_COMPONENT = "//bazel_rules/go/tests/testdata/reportboundary/consumer:conflicting_edge_component"

def _membership_classification_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _membership_classification_impl,
        attr_values = {"size": "small"},
    )

def _membership_classification_impl(env, target):
    info = target[ArccComponentInfo]

    # The component's coverage, as dependents see it: its own members plus
    # what it owns. //shared is missing because a component_dep covers it
    # — coverage is what makes a package someone else's responsibility.
    env.expect.that_collection([pkg.importpath for pkg in info.closure.to_list()]).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/core",
        "example.com/aspect/extradep",
        "example.com/aspect/lowlevel",
    ])

    env.expect.that_str(info.component_name).equals("api_component")
    env.expect.that_str(info.component_root).equals("bazel_rules/go/tests/testdata/api")

def _generated_files_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _generated_files_impl,
        attr_values = {"size": "small"},
    )

def _generated_files_impl(env, target):
    info = target[ArccComponentInfo]

    # arcc derives a dependency's layout path from its manifest path by
    # convention, so these two names are load-bearing, not cosmetic.
    env.expect.that_str(info.manifest.basename).equals("api_component.component.textproto")
    env.expect.that_str(info.layout.basename).equals("api_component.package-layout.json")

    env.expect.that_target(target).default_outputs().contains_exactly([
        "bazel_rules/go/tests/testdata/api/api_component.component.textproto",
        "bazel_rules/go/tests/testdata/api/api_component.package-layout.json",
    ])

def _unimported_member_closure_test(name):
    analysis_test(
        name = name,
        target = _MEMBER_COMPONENT,
        impl = _unimported_member_closure_impl,
        attr_values = {"size": "small"},
    )

def _unimported_member_closure_impl(env, target):
    info = target[ArccComponentInfo]
    packages = {pkg.importpath: pkg for pkg in info.closure.to_list()}

    # The interface does not import either package; both are reached only by
    # the explicit members root and its direct dependency.
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/member",
        "example.com/aspect/memberdep",
    ]).in_order()
    env.expect.that_collection(
        [src.basename for pkg in info.closure.to_list() for src in pkg.srcs],
    ).contains_exactly([
        "api.go",
        "member.go",
        "memberdep.go",
    ]).in_order()

    env.expect.that_collection(
        [src.basename for src in packages["example.com/aspect/member"].srcs],
    ).contains_exactly(["member.go"])
    env.expect.that_collection(
        [src.basename for src in packages["example.com/aspect/memberdep"].srcs],
    ).contains_exactly(["memberdep.go"])

def _reordered_member_closure_test(name):
    analysis_test(
        name = name,
        target = _REVERSED_MEMBER_COMPONENT,
        impl = _reordered_member_closure_impl,
        attr_values = {"size": "small"},
    )

def _reordered_member_closure_impl(env, target):
    info = target[ArccComponentInfo]

    # The reversed BUILD order must not affect the provider sequence. The
    # canonical sorted order is also the order used by layout generation.
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly([
        "example.com/aspect/api",
        "example.com/aspect/member",
        "example.com/aspect/memberdep",
    ]).in_order()

def _package_surface_closure_test(name):
    analysis_test(
        name = name,
        target = _PACKAGE_SURFACE_COMPONENT,
        impl = _package_surface_closure_impl,
        attr_values = {"size": "small"},
    )

def _package_surface_closure_impl(env, target):
    info = target[ArccComponentInfo]

    # No interface target is present: both closure roots come from members,
    # and the component must not accidentally forward an absent interface's
    # Go providers.
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly([
        "example.com/aspect/member",
        "example.com/aspect/memberdep",
    ]).in_order()
    env.expect.that_bool(GoInfo in target).equals(False)

def _transitive_files_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _transitive_files_impl,
        attr_values = {"size": "small"},
    )

def _transitive_files_impl(env, target):
    info = target[ArccComponentInfo]

    # The check has to reach every manifest and layout in the component
    # dependency graph, not just this component's own.
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "api_component.component.textproto",
        "shared_component.component.textproto",
    ])
    env.expect.that_collection(
        [file.basename for file in info.transitive_layouts.to_list()],
    ).contains_exactly([
        "api_component.package-layout.json",
        "shared_component.package-layout.json",
    ])

    # Contract documents travel with the component but never enter the
    # manifest: they are Bazel-only metadata.
    env.expect.that_collection(
        [file.basename for file in info.contracts.to_list()],
    ).contains_exactly(["contract.md"])

def _member_covered_conflict_fails_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests/testdata/conflict:member_covered_conflict_component",
        impl = _member_covered_conflict_fails_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _member_covered_conflict_fails_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*component member_covered_conflict_component:*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*member *//bazel_rules/go/tests/testdata/shared:shared is already covered by component_dep shared_component*"),
    )

def _nested_component_root_allowed_test(name):
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests/testdata/nested:nested_component",
        impl = _nested_component_root_allowed_impl,
        attr_values = {"size": "small"},
    )

def _nested_component_root_allowed_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly(["example.com/aspect/nested"])

def _auto_attached_infra_test(name):
    analysis_test(
        name = name,
        target = _AUTO_ATTACHED_INFRA_COMPONENT,
        impl = _auto_attached_infra_impl,
        attr_values = {"size": "small"},
    )

def _auto_attached_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "auto_attached_infra_component.component.textproto",
        "package_surface_component.component.textproto",
    ])
    env.expect.that_collection(
        [pkg.importpath for pkg in info.closure.to_list()],
    ).contains_exactly(["example.com/aspect/api"])
    manifest_path = "bazel_rules/go/tests/testdata/membercomponent/auto_attached_infra_component.component.textproto"
    action = env.expect.that_target(target).action_generating(manifest_path)
    action.content().contains("component_dependencies {\n  name: \"package_surface_component\"\n  manifest: \"package_surface_component.component.textproto\"\n  auto_attached: true\n}")

def _declining_infra_test(name):
    analysis_test(
        name = name,
        target = _DECLINING_INFRA_COMPONENT,
        impl = _declining_infra_impl,
        attr_values = {"size": "small"},
    )

def _declining_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "declining_infra_component.component.textproto",
    ])
    manifest_path = "bazel_rules/go/tests/testdata/membercomponent/declining_infra_component.component.textproto"
    action = env.expect.that_target(target).action_generating(manifest_path)
    action.content().split("\n").not_contains("  auto_attached: true")
    action.content().split("\n").not_contains('  name: "package_surface_component"')

    runfile_basenames = [f.basename for f in target[DefaultInfo].default_runfiles.files.to_list()]
    env.expect.that_collection(runfile_basenames).not_contains("package_surface_component.component.textproto")

def _closure_attached_infra_test(name):
    analysis_test(
        name = name,
        target = _CLOSURE_ATTACHED_INFRA_COMPONENT,
        impl = _closure_attached_infra_impl,
        attr_values = {"size": "small"},
    )

def _closure_attached_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "closure_attached_infra_component.component.textproto",
        "package_surface_component.component.textproto",
    ])
    manifest_path = "bazel_rules/go/tests/testdata/membercomponent/closure_attached_infra_component.component.textproto"
    action = env.expect.that_target(target).action_generating(manifest_path)
    action.content().contains("component_dependencies {\n  name: \"package_surface_component\"\n  manifest: \"package_surface_component.component.textproto\"\n  auto_attached: true\n}")

def _root_collection_attachment_test(name):
    analysis_test(
        name = name,
        target = _ROOT_COLLECTION_ATTACHED_INFRA_COMPONENT,
        impl = _root_collection_attachment_impl,
        attr_values = {"size": "small"},
    )

def _root_collection_attachment_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly([
        "root_collection_attached_infra_component.component.textproto",
        "package_surface_component.component.textproto",
    ])

def _never_infra_test(name):
    analysis_test(
        name = name,
        target = _NEVER_INFRA_COMPONENT,
        impl = _never_infra_impl,
        attr_values = {"size": "small"},
    )

def _never_infra_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_collection(
        [file.basename for file in info.transitive_manifests.to_list()],
    ).contains_exactly(["never_infra_component.component.textproto"])

def _authored_auto_conflict_test(name):
    analysis_test(
        name = name,
        target = _AUTHORED_WINS_INFRA_COMPONENT,
        impl = _authored_auto_conflict_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _authored_auto_conflict_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*authored and auto-attached dependencies named package_surface_component*"),
    )

def _deterministic_multi_infra_test(name):
    analysis_test(
        name = name,
        targets = {
            "target_a": _MULTI_INFRA_A,
            "target_b": _MULTI_INFRA_B,
        },
        impl = _deterministic_multi_infra_impl,
        attr_values = {"size": "small"},
    )

def _deterministic_multi_infra_impl(env, target):
    info_a = target.target_a[ArccComponentInfo]
    info_b = target.target_b[ArccComponentInfo]

    env.expect.that_collection(
        [file.basename for file in info_a.transitive_manifests.to_list()],
    ).contains_exactly([
        "package_surface_component.component.textproto",
        "runtime_component.component.textproto",
        "multi_infra_a.component.textproto",
    ])
    env.expect.that_collection(
        [file.basename for file in info_b.transitive_manifests.to_list()],
    ).contains_exactly([
        "package_surface_component.component.textproto",
        "runtime_component.component.textproto",
        "multi_infra_b.component.textproto",
    ])

    action_manifest_a = env.expect.that_target(target.target_a).action_generating("bazel_rules/go/tests/testdata/membercomponent/multi_infra_a.component.textproto")
    action_manifest_b = env.expect.that_target(target.target_b).action_generating("bazel_rules/go/tests/testdata/membercomponent/multi_infra_b.component.textproto")

    manifest_a = action_manifest_a.actual.content
    manifest_b_norm = action_manifest_b.actual.content.replace("multi_infra_b", "multi_infra_a")
    env.expect.that_str(manifest_a).equals(manifest_b_norm)

    action_layout_a = env.expect.that_target(target.target_a).action_generating("bazel_rules/go/tests/testdata/membercomponent/multi_infra_a.package-layout.json")
    action_layout_b = env.expect.that_target(target.target_b).action_generating("bazel_rules/go/tests/testdata/membercomponent/multi_infra_b.package-layout.json")

    layout_a = action_layout_a.actual.content
    layout_b_norm = action_layout_b.actual.content.replace("multi_infra_b", "multi_infra_a")
    env.expect.that_str(layout_a).equals(layout_b_norm)

def _dependency_artifact_bindings_test(name):
    analysis_test(
        name = name,
        target = _BINDINGS_COMPONENT,
        impl = _dependency_artifact_bindings_impl,
        attr_values = {"size": "small"},
    )

def _dependency_artifact_bindings_impl(env, target):
    info = target[ArccComponentInfo]
    layout = env.expect.that_target(target).action_generating(info.layout.short_path).actual.content
    bindings = json.decode(layout)["dependency_artifact_bindings"]
    env.expect.that_collection([binding["dependency"] for binding in bindings]).contains_exactly([
        "manual_component",
        "runtime_component",
        "shared_component",
    ]).in_order()
    env.expect.that_bool(bindings[0].get("auto_attached", False)).equals(False)
    env.expect.that_str(bindings[0]["provenance"]).equals("asserted")
    env.expect.that_bool(bindings[0].get("report", "") == "").equals(True)
    env.expect.that_bool(bindings[1]["auto_attached"]).equals(True)
    env.expect.that_str(bindings[1]["provenance"]).equals("checked")
    env.expect.that_bool(bindings[2]["auto_attached"]).equals(False)

    # Authored checked/asserted edges plus an auto-attached checked edge exercise
    # every binding field. The collection is canonicalized by dependency name.
    env.expect.that_str(layout).contains('"dependency_artifact_bindings": [')
    env.expect.that_str(layout).contains('"dependency": "manual_component"')
    env.expect.that_str(layout).contains('reportboundary/manual/manual_component.surface.json')
    env.expect.that_str(layout).contains('"dependency": "shared_component"')
    env.expect.that_str(layout).contains('"report": "_main/bazel_rules/go/tests/testdata/shared/shared_component.report.json"')
    env.expect.that_str(layout).contains('"auto_attached": true')
    env.expect.that_str(layout).contains('"dependency": "runtime_component"')
    env.expect.that_str(layout).contains('"report": "_main/bazel_rules/go/tests/testdata/infra/runtime/runtime_component.report.json"')
    env.expect.that_str(layout).contains('"provenance": "checked"')
    env.expect.that_str(layout).contains('"provenance": "asserted"')

def _dependency_artifact_inputs_test(name):
    analysis_test(
        name = name,
        target = _BINDINGS_COMPONENT,
        impl = _dependency_artifact_inputs_impl,
        attr_values = {"size": "small"},
    )

def _dependency_artifact_inputs_impl(env, target):
    info = target[ArccComponentInfo]
    action = env.expect.that_target(target).action_generating(info.report.short_path).actual
    inputs = {file.basename: True for file in action.inputs.to_list()}
    for artifact in [
        "shared_component.surface.json",
        "shared_component.report.json",
        "manual_component.surface.json",
        "runtime_component.surface.json",
        "runtime_component.report.json",
    ]:
        env.expect.that_bool(inputs.get(artifact, False)).equals(True)

def _dependency_provider_failure_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*component *:*"),
    )
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*dependency*"),
    )

def _dependency_provider_failure_test(name, target):
    analysis_test(
        name = name,
        target = target,
        impl = _dependency_provider_failure_impl,
        attr_values = {"size": "small"},
        expect_failure = True,
    )

def _missing_surface_provider_test(name):
    _dependency_provider_failure_test(name, _MISSING_SURFACE_COMPONENT)

def _checked_without_report_provider_test(name):
    _dependency_provider_failure_test(name, _CHECKED_WITHOUT_REPORT_COMPONENT)

def _asserted_with_report_provider_test(name):
    _dependency_provider_failure_test(name, _ASSERTED_WITH_REPORT_COMPONENT)

def _unknown_provenance_provider_test(name):
    _dependency_provider_failure_test(name, _UNKNOWN_PROVENANCE_COMPONENT)

def _duplicate_dependency_provider_test(name):
    _dependency_provider_failure_test(name, _DUPLICATE_DEPENDENCY_COMPONENT)

def _conflicting_edge_provider_test(name):
    _dependency_provider_failure_test(name, _CONFLICTING_EDGE_COMPONENT)

def _checked_action(env, target):
    """The component's ArccCheck action, as (subject, raw action)."""
    info = target[ArccComponentInfo]
    subject = env.expect.that_target(target).action_generating(info.report.short_path)
    subject.mnemonic().equals("ArccCheck")
    return subject, subject.actual

def _checked_component_outputs_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _checked_component_outputs_impl,
        attr_values = {"size": "small"},
    )

def _checked_component_outputs_impl(env, target):
    info = target[ArccComponentInfo]

    # Structural outputs (AC 1): canonical names arcc's dependency-surface
    # convention keys off in Step 7, one provenance value for a regular
    # producer, one action producing both.
    env.expect.that_str(info.provenance).equals("checked")
    env.expect.that_str(info.report.basename).equals("api_component.report.json")
    env.expect.that_str(info.surface.basename).equals("api_component.surface.json")

    check_action, raw_action = _checked_action(env, target)
    surface_action = env.expect.that_target(target).action_generating(info.surface.short_path)
    surface_action.mnemonic().equals("ArccCheck")
    env.expect.that_str(str(raw_action)).equals(str(surface_action.actual))

    # Lazy analysis (AC 3): report and surface ride in the `arcc` output
    # group and appear nowhere in the default outputs, so `bazel build //...`
    # does not run component analysis.
    env.expect.that_target(target).default_outputs().contains_exactly([
        "bazel_rules/go/tests/testdata/api/api_component.component.textproto",
        "bazel_rules/go/tests/testdata/api/api_component.package-layout.json",
    ])
    env.expect.that_target(target).output_group("arcc").contains_exactly([
        info.report.short_path,
        info.surface.short_path,
    ])

def _checked_action_command_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _checked_action_command_impl,
        attr_values = {"size": "small"},
    )

def _checked_action_command_impl(env, target):
    info = target[ArccComponentInfo]
    action, _ = _checked_action(env, target)

    # The exact shared command (command.bzl), in report-verdict-only mode.
    # argv[0] is the generated frame wrapper; argv[1] the arcc tool path
    # (execroot-relative, Bazel-managed — the tool, never a toolchain
    # binary); the rest is the factored command.
    argv = action.actual.argv
    env.expect.that_str(argv[1].rsplit("/", 1)[-1]).equals("arcc")
    env.expect.that_str(argv[2]).equals("check")
    env.expect.that_str(argv[3]).equals(info.manifest.path)
    action.contains_flag_values([
        ("--package-layout", info.layout.path),
        ("--report-out", info.report.path),
        ("--surface-out", info.surface.path),
        ("--format", "json"),
    ])
    env.expect.that_collection(argv).contains("--stdlib-map=" + _stdlib_map_path(env, action.actual))
    env.expect.that_collection(argv).contains("--report-verdict-only")

    # Hermetic (I5): no environment whatsoever.
    action.env().contains_exactly({})

def _stdlib_map_path(env, action):
    """The execroot-relative path of the declared stdlib map from the action's inputs."""
    for f in action.inputs.to_list():
        if f.basename.endswith(".stdlib-map.json"):
            return f.path
    env.fail("ArccCheck action declares no stdlib-map input")

def _checked_action_inputs_test(name):
    analysis_test(
        name = name,
        target = _API_COMPONENT,
        impl = _checked_action_inputs_impl,
        attr_values = {"size": "small"},
    )

def _checked_action_inputs_impl(env, target):
    info = target[ArccComponentInfo]
    action, raw = _checked_action(env, target)
    argv = raw.argv

    # Step 5 transition inputs (AC 5): manifest, layout, today's source and
    # runfile closure, SDK sources, stdlib map, and the direct dependency's
    # report/surface artifacts — the producer edge R8 needs (AC 4).
    inputs = [f.basename for f in raw.inputs.to_list()]
    env.expect.that_collection(inputs).contains("api_component.component.textproto")
    env.expect.that_collection(inputs).contains("api_component.package-layout.json")
    for src in info.closure.to_list():
        for s in src.srcs:
            env.expect.that_collection(inputs).contains(s.basename)
    env.expect.that_collection(inputs).contains(_stdlib_map_path(env, raw).rsplit("/", 1)[-1])
    dep_info = None
    for dep_manifest in info.transitive_manifests.to_list():
        if dep_manifest.basename == "shared_component.component.textproto":
            dep_info = dep_manifest
    env.expect.that_str(dep_info != None).equals(True)
    env.expect.that_collection(inputs).contains("shared_component.report.json")
    env.expect.that_collection(inputs).contains("shared_component.surface.json")

    # Outputs: exactly the two structural artifacts (task req 1).
    env.expect.that_collection([f.basename for f in raw.outputs.to_list()]).contains_exactly([
        "api_component.report.json",
        "api_component.surface.json",
    ])

    # The transition input set is exact (AC 5), asserted in both directions
    # from a specification-derived expectation, never from the action under
    # test: the expected non-SDK set is built from the provider's closure,
    # the component's own manifest/layout (declared outputs), the dependency
    # manifest/layout/producer artifacts (named by the manifest convention),
    # and the default seam's map artifact — then every action input outside
    # the SDK must be in it, and every expected file must be an input.
    sdk_frame_prefix = _sdk_repo_prefix(env, target)
    non_sdk_inputs = [
        f.basename
        for f in raw.inputs.to_list()
        if not f.short_path.startswith("../" + sdk_frame_prefix)
    ]

    expected_basenames = {}
    for src in info.closure.to_list():
        for s in src.srcs:
            expected_basenames[s.basename] = True
    expected_basenames[info.manifest.basename] = True
    expected_basenames[info.layout.basename] = True
    expected_basenames["arcc_stdlib_map.stdlib-map.json"] = True
    for manifest in info.transitive_manifests.to_list():
        if manifest == info.manifest:
            continue
        dep_stem = manifest.basename[:-len(".component.textproto")]
        expected_basenames[manifest.basename] = True
        expected_basenames[dep_stem + ".package-layout.json"] = True
        expected_basenames[dep_stem + ".report.json"] = True
        expected_basenames[dep_stem + ".surface.json"] = True

    unexpected = [
        basename
        for basename in non_sdk_inputs
        if not expected_basenames.get(basename)
        # The arcc tool and its runfiles (the argv[1] executable, declared as
        # a tool), the generated frame wrapper (the action's executable), and
        # covered dependency sources — today's source/runfile closure, which
        # AC 5 explicitly keeps in the transition set until Step 8.
        and basename != argv[1].rsplit("/", 1)[-1]
        and basename != "arcc.runfiles"
        and basename != argv[0].rsplit("/", 1)[-1]
        and not basename.endswith(".go")
    ]
    env.expect.that_collection(unexpected).contains_exactly([])
    input_set = {basename: True for basename in non_sdk_inputs}
    missing = [
        basename
        for basename in sorted(expected_basenames.keys())
        if not input_set.get(basename)
    ]
    env.expect.that_collection(missing).contains_exactly([])

    # Explicit negatives over the non-SDK inputs (AC 5): no export data, no
    # host/toolchain caches, no toolchain binary in the declared inputs.
    env.expect.that_collection([b for b in non_sdk_inputs if ".export" in b]).contains_exactly([])
    env.expect.that_collection([b for b in non_sdk_inputs if "cache" in b]).contains_exactly([])
    env.expect.that_collection([b for b in non_sdk_inputs if b == "go" or b.endswith(".a")]).contains_exactly([])

def _sdk_repo_prefix(env, target):
    """The runfiles-frame prefix of the SDK sources, from the emitted layout."""
    layout_action = env.expect.that_target(target).action_generating(
        "bazel_rules/go/tests/testdata/api/api_component.package-layout.json",
    ).actual
    content = layout_action.content
    marker = "\"go_sdk_root\": \""
    start = content.find(marker)
    if start == -1:
        env.fail("layout names no go_sdk_root")
    root = content[start + len(marker):].split("\"")[0]
    return root.split("/")[0] + "/"

def _migrated_fixture_stays_checked_test(name):
    # AC 4 (Step 5 task 05): a fixture whose `manual` tag was only a
    # check-suppression workaround keeps the checked producer after the
    # migration to `check_tags` — it must never receive an asserted surface.
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests/testdata/violation:violation_component",
        impl = _migrated_fixture_stays_checked_impl,
        attr_values = {"size": "small"},
    )

def _migrated_fixture_stays_checked_impl(env, target):
    info = target[ArccComponentInfo]
    env.expect.that_str(info.provenance).equals("checked")
    if info.report == None:
        env.fail("migrated negative fixture must keep a checked report")
    if info.surface == None:
        env.fail("migrated negative fixture must keep a checked surface")

def _negative_rules_stage_the_checked_report_test(name):
    # AC 4 (Step 5 task 06): the grep assertion rule stages the component's
    # canonical report — building the test builds the component's ArccCheck
    # producer action, so the negative assertions read a report the producer
    # action actually generated. The staged set is minimal: the surface is no
    # longer carried, and no source closure rides along with it.
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests:broken_checker_component_fails_test",
        impl = _negative_rules_stage_the_checked_report_impl,
        attr_values = {"size": "small"},
    )

def _negative_rules_stage_the_checked_report_impl(env, target):
    runfiles = [
        f.basename
        for f in target[DefaultInfo].default_runfiles.files.to_list()
    ]
    env.expect.that_collection(runfiles).contains("broken_checker_component.report.json")
    env.expect.that_collection([b for b in runfiles if b.endswith(".surface.json")]).contains_exactly([])
    env.expect.that_collection([b for b in runfiles if b.endswith(".go")]).contains_exactly([])

    # The golden rule is asserted through a second analysis test below; this
    # target's impl runs only for the grep rule.

def _golden_rule_stages_the_checked_report_test(name):
    # AC 4 (Step 5 task 06): the golden rule diffs the provider's canonical
    # report directly; only the report and the golden are staged.
    analysis_test(
        name = name,
        target = "//bazel_rules/go/tests:api_component_text_report_golden_test",
        impl = _golden_rule_stages_the_checked_report_impl,
        attr_values = {"size": "small"},
    )

def _golden_rule_stages_the_checked_report_impl(env, target):
    runfiles = [
        f.basename
        for f in target[DefaultInfo].default_runfiles.files.to_list()
    ]
    env.expect.that_collection(runfiles).contains("api_component.report.json")
    env.expect.that_collection([b for b in runfiles if b.endswith(".surface.json")]).contains_exactly([])

def go_component_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _membership_classification_test,
            _generated_files_test,
            _unimported_member_closure_test,
            _reordered_member_closure_test,
            _package_surface_closure_test,
            _transitive_files_test,
            _member_covered_conflict_fails_test,
            _nested_component_root_allowed_test,
            _auto_attached_infra_test,
            _declining_infra_test,
            _closure_attached_infra_test,
            _root_collection_attachment_test,
            _never_infra_test,
            _authored_auto_conflict_test,
            _deterministic_multi_infra_test,
            _dependency_artifact_bindings_test,
            _dependency_artifact_inputs_test,
            _missing_surface_provider_test,
            _checked_without_report_provider_test,
            _asserted_with_report_provider_test,
            _unknown_provenance_provider_test,
            _duplicate_dependency_provider_test,
            _conflicting_edge_provider_test,
            _checked_component_outputs_test,
            _checked_action_command_test,
            _checked_action_inputs_test,
            _migrated_fixture_stays_checked_test,
            _negative_rules_stage_the_checked_report_test,
            _golden_rule_stages_the_checked_report_test,
        ],
    )
