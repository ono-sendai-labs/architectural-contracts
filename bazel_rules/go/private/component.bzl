"""The `_go_component` rule: manifest and package-layout generation, plus the
component surface producer (Step 5 tasks 04 and 05).

Everything here happens at analysis time. The write actions produce the
manifest arcc checks and the layout arcc loads through. A checked component
runs the ArccCheck action — `command.bzl`'s analysis argv, in always-green
report-verdict-only mode — publishing
`<name>.report.json` and `<name>.surface.json` through `OutputGroupInfo(arcc)`
— never as default outputs, so `bazel build //...` runs no component analysis
unless asked. The `.check` assertion rules (check.bzl) consume the action's
persisted report; this action is the one analysis command. A manual-tagged
component takes the asserted producer instead
(design I6): its package-level asserted surface is written at analysis time,
with no symbols and the empty digest, `report = None` and
`provenance = "asserted"`; Step 11 replaces the tag-based selection with
`authority: UNKNOWN`. Direct provider edges are also written into the layout as
sorted, structural dependency artifact bindings; Task 2 consumes those bindings
later. The rule classifies the union of the interface and
declared member package closures into component-dep-covered and member
packages (design §3.1, §4.8), and forwards the interface library's Go
providers so the component target is usable as a `deps` entry.
"""

load("//bazel_rules:authority.bzl", "ALL_AUTHORITIES")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo", "ArccStdlibMapInfo")
load(":arcc_metadata.bzl", "ARCC_PRODUCER_VERSION", "DEFAULT_NAMESPACE", "SURFACE_FORMAT_VERSION", "arcc_sdk_key_fields")
load(":aspect.bzl", "arcc_deps_aspect", "merge_by_importpath")
load(":command.bzl", "arcc_check_argv")
load(
    ":go_adapter.bzl",
    "ARCC_TARGET",
    "GO_CONTEXT_DATA_ATTRS",
    "GO_PROVIDERS",
    "GO_TOOLCHAINS",
    "forward_go_providers",
    "go_attached_infra",
    "go_build_platform",
    "go_importpath",
    "go_library_srcs",
    "go_sdk_root",
    "go_sdk_srcs",
    "go_stdlib_toolchain",
    "go_stdlib_export_data",
    "go_target_mode",
)
load(":paths.bzl", "match_path", "runfiles_path")
load(":stdlib_map.bzl", "stdlib_map_default_attr")

def _relativize(target_path, base_dir):
    """`target_path` as seen from the directory `base_dir`."""
    target_parts = target_path.split("/")
    base_parts = [part for part in base_dir.split("/") if part]

    common = 0
    for i in range(min(len(base_parts), len(target_parts) - 1)):
        if base_parts[i] != target_parts[i]:
            break
        common = i + 1

    return "/".join([".."] * (len(base_parts) - common) + target_parts[common:])

def _dirname(path):
    if "/" not in path:
        return ""
    return path.rsplit("/", 1)[0]

def _package_name(importpath):
    """Best guess at a package's Go name, from its import path.

    The Go package clause is not on any provider rules_go exposes, so this
    reconstructs the usual case. arcc requires the field to be non-empty but
    type-checks from the package clause in the sources, so a package whose
    name differs from its directory is not misanalysed by a wrong guess here.
    """
    segments = [segment for segment in importpath.split("/") if segment]
    if not segments:
        return importpath
    last = segments[-1]

    # Module major-version suffixes are not part of the package name.
    if len(segments) > 1 and last.startswith("v") and last[1:].isdigit():
        return segments[-2]
    return last

def _textproto_string(value):
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'

def _manifest_content(ctx, interface_files, component_deps, auto_attached_deps, declared_authority, manifest_dir, interface_style, members, authority_unknown, analysis_defeating_policy):
    lines = ["name: " + _textproto_string(ctx.label.name)]

    if interface_style == "PACKAGE_SURFACE":
        lines.append("interface_style: INTERFACE_STYLE_PACKAGE_SURFACE")

    if authority_unknown:
        # The manual producer is an asserted UNKNOWN component. Keep its
        # generated manifest aligned with the checked-in declaration so
        # manifestparity cannot mistake the provider's structural authority
        # for the default DECLARED{} value (design I6/R10).
        lines.append("authority: UNKNOWN")

    if analysis_defeating_policy == "warn":
        # The schema's zero value is strict. Emit only the explicit non-default
        # policy so generated manifests and hand-authored manifests have the
        # same omission semantics.
        lines.append("analysis_defeating_policy: WARN")

    for path in interface_files:
        lines.append("interface_files: " + _textproto_string(path))

    all_deps = []
    for dep in component_deps:
        info = dep[ArccComponentInfo]
        manifest_path = _relativize(runfiles_path(ctx, info.manifest), manifest_dir)
        all_deps.append((info.component_name, manifest_path, False))

    for info in auto_attached_deps:
        manifest_path = _relativize(runfiles_path(ctx, info.manifest), manifest_dir)
        all_deps.append((info.component_name, manifest_path, True))

    all_deps = sorted(all_deps)

    for dep_name, manifest_path, auto_attached in all_deps:
        lines.append("component_dependencies {")
        lines.append("  name: " + _textproto_string(dep_name))
        lines.append("  manifest: " + _textproto_string(manifest_path))
        if auto_attached:
            lines.append("  auto_attached: true")
        lines.append("}")

    for m in members:
        lines.append("members: " + _textproto_string(m))

    for authority in declared_authority:
        lines.append("declared_authority: " + _textproto_string(authority))

    return "\n".join(lines) + "\n"

def _layout_content(ctx, merged, roots, go_sdk_root, platform, target, dependency_bindings, stdlib_export_data = None):
    root_set = {root: True for root in roots}
    packages = []
    for importpath in sorted(merged.keys()):
        pkg = merged[importpath]
        go_files = [
            runfiles_path(ctx, src)
            for src in pkg.srcs
            if src.extension == "go"
        ]

        # Package IDs are import paths: the closure is already folded by
        # import path, so they are unique, and it keeps the layout readable.
        package = {
            "ID": importpath,
            "Name": _package_name(importpath),
            "PkgPath": importpath,
            "GoFiles": go_files,
            # Identical until cgo is supported; a cgo package compiles from
            # preprocessed sources, which is why the rule rejects one below.
            "CompiledGoFiles": go_files,
            # The closure contains only enumerated build targets, not SDK packages,
            # so this provenance bit is structurally false (see docs/package-layout-schema.md §4).
            "is_stdlib": False,
        }
        if importpath not in root_set:
            if pkg.export_file == None:
                fail("package %s has no export file for non-member package %s" % (ctx.label.name, importpath))
            package["ExportFile"] = runfiles_path(ctx, pkg.export_file)
            # The aspect's generic dependency projection contains the
            # non-stdlib direct edges. Standard-library edges are completed by
            # the target-configured descriptor during the transitional
            # source-backed validation path; the map is explicit even for a
            # leaf so the later member-only validator can fail closed.
            package["Imports"] = {
                dep: dep
                for dep in sorted(pkg.deps)
            }
        packages.append(package)

    layout_data = {
        "go_sdk_root": go_sdk_root,
        "roots": roots,
        "packages": packages,
    }
    if dependency_bindings:
        # Binding order is part of the layout contract. The caller has already
        # rejected duplicate dependency names and supplied this list in the
        # canonical name order; the artifact paths remain ordinary runfiles
        # frame data, never semantic trust claims.
        layout_data["dependency_artifact_bindings"] = dependency_bindings
    if stdlib_export_data != None:
        if stdlib_export_data.target == None:
            fail("component %s: standard-library export descriptor has no target identity" % ctx.label.name)
        export_target = stdlib_export_data.target
        export_files_by_path = {
            export_file.path: export_file
            for export_file in stdlib_export_data.export_files.to_list()
        }
        layout_data["stdlib_export_data"] = {
            "metadata": runfiles_path(ctx, stdlib_export_data.metadata),
            "export_roots": [
                {
                    "runfiles_path": runfiles_path(ctx, export_files_by_path[path]),
                    "exec_path": path,
                }
                for path in sorted(export_files_by_path.keys())
            ],
            "target": {
                "toolchain_version": export_target.toolchain_version,
                "goos": export_target.goos,
                "goarch": export_target.goarch,
                "build_tags": sorted(export_target.tags),
                "cgo_enabled": export_target.cgo_enabled,
                "goexperiment": export_target.goexperiment,
            },
        }
    if platform:
        layout_platform = {
            "goos": platform.goos,
            "goarch": platform.goarch,
            "build_tags": sorted(platform.tags),
            "cgo_enabled": platform.cgo_enabled,
        }
        # Pinning fields (docs/package-layout-schema.md §3): they must be
        # omitted when empty — a present-but-empty toolchain_version is a
        # validation error and a present-but-empty goexperiment pins
        # GOEXPERIMENT=none. Non-empty, they pin the release/tool tags so the
        # layout describes exactly the target SDK configuration whose
        # stdlib map is attached to the analysis action (AC 6).
        if target.toolchain_version:
            layout_platform["toolchain_version"] = target.toolchain_version
        if target.goexperiment:
            layout_platform["goexperiment"] = target.goexperiment
        layout_data["platform"] = layout_platform

    return json.encode_indent(
        layout_data,
        indent = "  ",
    ) + "\n"

def _dependency_binding_record(ctx, info, auto_attached):
    """Validates one provider and projects it into layout metadata plus files."""
    dependency = info.component_name
    if not dependency:
        fail("component %s: direct dependency provider has an empty component name" % ctx.label.name)
    if info.surface == None:
        fail("component %s: dependency %s has no surface artifact" % (ctx.label.name, dependency))

    provenance = info.provenance
    if provenance == "checked":
        if info.report == None:
            fail("component %s: checked dependency %s has no report artifact" % (ctx.label.name, dependency))
    elif provenance == "asserted":
        if info.report != None:
            fail("component %s: asserted dependency %s unexpectedly has a report artifact" % (ctx.label.name, dependency))
    else:
        fail("component %s: dependency %s has unknown provider provenance %r" % (ctx.label.name, dependency, provenance))

    artifacts = [info.surface]
    if info.report != None:
        artifacts.append(info.report)
    return struct(
        info = info,
        binding = {
            "dependency": dependency,
            "surface": runfiles_path(ctx, info.surface),
            "auto_attached": auto_attached,
            "provenance": provenance,
        } | ({"report": runfiles_path(ctx, info.report)} if info.report != None else {}),
        artifacts = artifacts,
    )

def _direct_dependency_binding_records(ctx, authored_deps, attached_infra):
    """Returns canonical direct-edge records and rejects provider ambiguity."""
    authored_by_name = {}
    authored_records = []
    for dep in authored_deps:
        info = dep[ArccComponentInfo]
        record = _dependency_binding_record(ctx, info, False)
        if info.component_name in authored_by_name:
            fail("component %s: duplicate direct dependency name %s" % (ctx.label.name, info.component_name))
        authored_by_name[info.component_name] = info
        authored_records.append(record)

    auto_by_name = {}
    auto_records = []
    auto_deps = []
    auto_targets = []
    auto_patterns = []
    for item in attached_infra:
        info = item.info
        record = _dependency_binding_record(ctx, info, True)
        if info.component_name in authored_by_name:
            fail(("component %s: authored and auto-attached dependencies named %s " +
                  "have conflicting edge identity") % (ctx.label.name, info.component_name))
        if info.component_name in auto_by_name:
            fail("component %s: duplicate auto-attached dependency name %s" % (ctx.label.name, info.component_name))
        auto_by_name[info.component_name] = info
        auto_records.append(record)
        auto_deps.append(info)
        auto_targets.append(item.target)
        for pattern in item.patterns:
            auto_patterns.append((pattern, info.component_name))

    records_by_name = {}
    for record in authored_records + auto_records:
        records_by_name[record.binding["dependency"]] = record
    records = [records_by_name[name] for name in sorted(records_by_name.keys())]

    seen_surfaces = {}
    for record in records:
        surface = record.binding["surface"]
        dependency = record.binding["dependency"]
        if surface in seen_surfaces and seen_surfaces[surface] != dependency:
            fail(("component %s: direct dependencies %s and %s share surface path %s") % (
                ctx.label.name,
                seen_surfaces[surface],
                dependency,
                surface,
            ))
        seen_surfaces[surface] = dependency

    return records, auto_deps, auto_targets, auto_patterns

def _classify(ctx, merged, effective_members, covered):
    """Splits the FR2 frontier into covered and member (design §3.1, §4.8)."""
    frontier = {}
    for member_path in effective_members:
        frontier[member_path] = True
        pkg_struct = merged.get(member_path)
        if pkg_struct != None:
            for dep_path in pkg_struct.deps:
                frontier[dep_path] = True

    members = []
    for importpath in sorted(frontier.keys()):
        if importpath not in merged:
            continue
        if importpath in covered:
            continue
        elif importpath in effective_members:
            members.append(importpath)
        # The checker reports the remaining frontier as UNDECLARED_DEPENDENCY.

    return sorted(members)

def _asserted_surface_sdk_key(key):
    """The asserted surface's SDK-key object, in the schema's field order.

    Mirrors protojson's canonical zero-value omission: cgo off, no build tags
    and no GOEXPERIMENT are the zero states of their fields and are omitted,
    exactly as the checked emitter's canonical encoder omits them (DR-15).
    """
    sdk_key = {
        "toolchainVersion": key.toolchain_version,
        "goos": key.goos,
        "goarch": key.goarch,
    }
    if key.cgo_enabled:
        sdk_key["cgoEnabled"] = True
    if key.build_tags:
        sdk_key["buildTags"] = list(key.build_tags)
    if key.goexperiment:
        sdk_key["goexperiment"] = key.goexperiment
    sdk_key["classifierHash"] = key.classifier_hash
    sdk_key["mapFormatVersion"] = key.map_format_version
    return sdk_key

def _asserted_surface_content(component_name, packages, key):
    """The canonical asserted-surface JSON, written at analysis time (I6).

    An asserted surface is an assertion *about* the component, never a
    derivation *from* its code (design I6): it carries only what Bazel
    analysis already knows — the member packages from the layout, the
    canonical namespace, the complete target SDK key, the format and producer
    versions — and nothing derived. Its symbol list is empty and its digest is
    the empty string, both of which the canonical encoder omits as zero
    values: absence records that no content was derived, and consumers MUST
    NOT read it as a content claim (design §The surface manifest).

    The field order, indentation and trailing newline follow the canonical
    artifact contract (DR-15), so this write is byte-identical to what the
    checked emitter's encoder would produce for the same asserted content.
    The SDK key is assembled through arcc_metadata.bzl's single seam and is
    pinned against the checked emitter's key by asserted_surface_sdk_key_test
    (task req 5) — never duplicated here.
    """
    return json.encode_indent({
        "formatVersion": SURFACE_FORMAT_VERSION,
        "component": component_name,
        "interfaceStyle": "INTERFACE_STYLE_PACKAGE_SURFACE",
        "authority": {"authority": "UNKNOWN"},
        "packages": list(packages),
        "namespace": DEFAULT_NAMESPACE,
        "sdkKey": _asserted_surface_sdk_key(key),
        "producerVersion": ARCC_PRODUCER_VERSION,
    }, indent = "  ") + "\n"

def _shell_quote(arg):
    """Shell-quote one argv element for the generated frame wrapper (see check.bzl)."""
    return "'" + arg.replace("'", "'\\''") + "'"

def _frame_symlink_commands(files, workspace_name, preferred_files = []):
    """Shell commands recreating the runfiles path frame in an action sandbox.

    The manifest and layout name sources in the runfiles-root frame
    (paths.bzl): `<workspace_name>/<short_path>` for main-repo files and
    `<repo>/<stripped short_path>` for external-repo files. arcc resolves them
    against its working directory, which in a Bazel action is the sandbox
    execroot — a frame in which none of those names resolve as written.

    Two symlink families repair that with declared inputs alone (no host
    state, I5): `<workspace_name> -> .` recovers main-repo paths, and external
    repositories get either one root link or a deterministic per-file overlay.
    The overlay is needed when a repository contributes both source files and
    generated export outputs: Bazel's source and bazel-out trees have different
    physical roots but share one runfiles apparent name. Both forms are
    derived from the inputs themselves, so the command is deterministic.
    """
    preferred = {f.path: True for f in preferred_files}
    external_candidates = {}
    external_entries = {}
    generated_candidates = {}

    def add_external_candidate(apparent, priority, root, relative, target):
        external_candidates.setdefault(apparent, []).append((priority, root))
        external_entries.setdefault(apparent, {}).setdefault(relative, []).append((priority, target))

    for f in files:
        short_path = f.short_path
        # Tree and generated files produced under an external repository keep
        # a `../repo/...` runfiles short path even though their sandbox path is
        # a declared bazel-out path. Recreate that repository frame from the
        # declared output root; the stdlib metadata path uses this frame.
        if f.path.startswith("bazel-out/") and short_path.startswith("../"):
            marker = "/external/"
            marker_index = f.path.find(marker)
            if marker_index == -1:
                fail("component generated input %s has no external repository frame" % f.path)
            repository = f.path[marker_index + len(marker):].split("/")[0]
            external_root = f.path[:marker_index + len(marker)] + repository
            apparent = short_path[len("../"):].split("/")[0]
            relative = short_path[len("../") + len(apparent):].lstrip("/")
            add_external_candidate(
                apparent,
                0 if preferred.get(f.path, False) else 1,
                external_root,
                relative,
                f.path,
            )
            continue
        if not short_path.startswith("../"):
            # Generated outputs keep their package-relative runfiles spelling
            # in the layout, while Bazel stages the declared input under its
            # bazel-out path in the action sandbox. Recreate that frame link
            # just as the runfiles tree would; source files already have an
            # identical path and need no link.
            if f.path != short_path and f.path.startswith("bazel-out/"):
                generated_candidates.setdefault(short_path, []).append((
                    0 if preferred.get(f.path, False) else 1,
                    f.path,
                ))
            continue
        apparent = short_path[len("../"):].split("/")[0]
        path_parts = f.path.split("/")
        if len(path_parts) < 2 or path_parts[0] != "external":
            fail(("component input %s names an external repository frame the action " +
                  "sandbox does not stage; refusing to guess a symlink") % f.path)
        canonical = path_parts[1]
        external_root = "external/" + canonical
        relative = short_path[len("../") + len(apparent):].lstrip("/")
        add_external_candidate(
            apparent,
            0 if preferred.get(f.path, False) else 1,
            external_root,
            relative,
            f.path,
        )

    def select_frame_target(candidates, label):
        best_priority = min([candidate[0] for candidate in candidates])
        best = sorted({candidate[1]: True for candidate in candidates if candidate[0] == best_priority}.keys())
        if len(best) != 1:
            fail("component has ambiguous declared frame targets for %s: %s" % (label, ", ".join(best)))
        return best[0]

    external_repos = {}
    external_overlays = {}
    for apparent, candidates in external_candidates.items():
        best_priority = min([candidate[0] for candidate in candidates])
        best_roots = sorted({candidate[1]: True for candidate in candidates if candidate[0] == best_priority}.keys())
        if len(best_roots) == 1:
            external_repos[apparent] = best_roots[0]
            continue

        entries = {}
        for relative, entry_candidates in external_entries[apparent].items():
            best_entries = sorted({candidate[1]: True for candidate in entry_candidates if candidate[0] == best_priority}.keys())
            if not best_entries:
                # The entry belongs only to an incidental runfile under a
                # lower-priority configuration. It is not part of the frame
                # this action names and must not force an overlay entry.
                continue
            if len(best_entries) != 1:
                fail("component has ambiguous declared frame targets for %s/%s: %s" % (
                    apparent,
                    relative,
                    ", ".join(best_entries),
                ))
            entries[relative] = best_entries[0]
        external_overlays[apparent] = entries
    generated_files = {
        short_path: select_frame_target(candidates, short_path)
        for short_path, candidates in generated_candidates.items()
    }

    commands = ["ln -s . " + _shell_quote(workspace_name)]
    for apparent in sorted(external_repos.keys()):
        commands.append("ln -s %s %s" % (
            _shell_quote(external_repos[apparent]),
            _shell_quote(apparent),
        ))
    for apparent in sorted(external_overlays.keys()):
        for relative in sorted(external_overlays[apparent].keys()):
            frame_path = apparent + "/" + relative if relative else apparent
            parent = _dirname(frame_path)
            if parent:
                commands.append("mkdir -p " + _shell_quote(parent))
            commands.append("if [ ! -e %s ]; then ln -s %s %s; fi" % (
                _shell_quote(frame_path),
                _shell_quote(_relativize(external_overlays[apparent][relative], parent)),
                _shell_quote(frame_path),
            ))
    for short_path in sorted(generated_files.keys()):
        parent = _dirname(short_path)
        if parent:
            commands.append("mkdir -p " + _shell_quote(parent))
        commands.append("if [ ! -e %s ]; then ln -s %s %s; fi" % (
            _shell_quote(short_path),
            _shell_quote(_relativize(generated_files[short_path], parent)),
            _shell_quote(short_path),
        ))
    return commands

def _checked_analysis_action(ctx, manifest, layout, closure_srcs, export_files, stdlib_export_data, transitive_manifests, transitive_layouts, dep_artifacts, dep_runfiles, sdk_root_file):
    """The checked-component analysis action (design R8, task reqs 1/3/5/6).

    One ordinary action running `command.bzl`'s analysis argv — the exact
    command `arcc_checked_analysis_test` re-executes — in always-green
    report-verdict-only mode:
    violations are data in the report, tool errors (exit 2) fail the action.
    The `.check` assertion rules consume this action's persisted report
    (check.bzl); they never run a second analysis command.

    The executable is a generated frame-setting wrapper: it recreates the
    runfiles path frame (paths.bzl) inside the sandbox and then execs the
    argv below. arcc resolves the manifest's and layout's source paths
    against its working directory, and a Bazel action's working directory is
    the execroot — a frame in which those runfiles-root-relative names do not
    resolve as written. The wrapper's symlinks (`<workspace_name> -> .`, and
    one `<apparent repo name> -> external/<canonical repo name>` per external
    repository the frame names) are derived from the declared inputs alone,
    so the action stays deterministic and hermetic (I5) while keeping the
    existing layout-driver working-directory contract.

    Inputs retain the Step 7 source-backed transition set and add the Step 8
    export closure: ordinary non-member archive exports, plus the
    target-configured stdlib metadata and generated export trees. The SDK and
    closure sources remain deliberately staged until Task 5 cuts the loader
    over to member-only type loading. The descriptor's `inputs` is the complete
    host-neutral stdlib input set, so the action does not need to know any
    rules_go provider details.

    No environment at all: the argv and the frame symlinks fully determine
    the action; network access is blocked.
    """
    report = ctx.actions.declare_file(ctx.label.name + ".report.json")
    surface = ctx.actions.declare_file(ctx.label.name + ".surface.json")

    wrapper = ctx.actions.declare_file(ctx.label.name + ".arcc-check-wrapper.sh")
    map_file = ctx.attr._stdlib_map[ArccStdlibMapInfo].map
    argv = arcc_check_argv(
        arcc = ctx.executable._arcc.path,
        manifest = manifest.path,
        layout = layout.path,
        stdlib_map = map_file.path,
        report_out = report.path,
        surface_out = surface.path,
        verdict_only = True,
    )

    # The bound artifacts are explicit frame inputs as well as direct action
    # inputs. Keeping them in this list makes repository symlink construction
    # independent of transitive dependency runfiles, which may contain the
    # files today only as an incidental consequence of provider wiring.
    sdk_source_files = go_sdk_srcs(ctx).to_list()
    frame_files = list(closure_srcs) + list(export_files) + list(dep_artifacts) + [sdk_root_file]
    frame_files += sdk_source_files
    stdlib_export_inputs = stdlib_export_data.inputs.to_list()
    frame_files += stdlib_export_inputs
    frame_files += transitive_manifests.to_list() + transitive_layouts.to_list()
    frame_files += dep_runfiles.to_list()
    preferred_frame_files = list(closure_srcs) + list(export_files) + list(dep_artifacts) + [sdk_root_file]
    preferred_frame_files += sdk_source_files
    preferred_frame_files += stdlib_export_inputs
    preferred_frame_files += transitive_manifests.to_list() + transitive_layouts.to_list()

    ctx.actions.write(
        output = wrapper,
        # `set -eu` fails the action immediately if a frame symlink cannot be
        # created: the working-directory contract must fail closed, never
        # silently continue with unresolved paths.
        content = "\n".join(["#!/bin/bash", "set -eu"] + _frame_symlink_commands(frame_files, ctx.workspace_name, preferred_frame_files) + ["exec \"$@\"", ""]),
        is_executable = True,
    )

    ctx.actions.run(
        executable = wrapper,
        arguments = argv,
        inputs = depset(
            direct = [manifest, layout, map_file] + list(closure_srcs) + list(export_files) + list(dep_artifacts),
            transitive = [go_sdk_srcs(ctx), stdlib_export_data.inputs, transitive_manifests, transitive_layouts, dep_runfiles],
        ),
        tools = [ctx.executable._arcc],
        outputs = [report, surface],
        use_default_shell_env = False,
        execution_requirements = {"block-network": "1"},
        mnemonic = "ArccCheck",
        progress_message = "Checking component %s" % ctx.label,
    )
    return report, surface

def go_component_impl(ctx, attachment_fn = go_attached_infra):
    """Generates a component using the supplied adapter attachment function.

    The default is the production adapter seam. A test-only rule may inject a
    fixture registry without adding test attributes to the production rule.
    """
    for authority in ctx.attr.declared_authority:
        if authority not in ALL_AUTHORITIES:
            fail("component %s: unknown declared_authority %r; known: %s" % (
                ctx.label.name,
                authority,
                ", ".join(ALL_AUTHORITIES),
            ))

    if ctx.attr.analysis_defeating_policy not in ["strict", "warn"]:
        fail("component %s: unknown analysis_defeating_policy %r; accepted values are 'strict' and 'warn'" % (
            ctx.label.name,
            ctx.attr.analysis_defeating_policy,
        ))

    roots = ([ctx.attr.interface] if ctx.attr.interface else []) + ctx.attr.members

    attached_infra = attachment_fn(ctx, roots, ctx.attr.infra_deps)
    dependency_binding_records, auto_attached_deps, auto_attached_targets, auto_attached_patterns = _direct_dependency_binding_records(
        ctx,
        ctx.attr.component_deps,
        attached_infra,
    )
    dependency_bindings = [record.binding for record in dependency_binding_records]

    covered = {}
    for dep in ctx.attr.component_deps:
        info = dep[ArccComponentInfo]
        for pkg in info.closure.to_list():
            covered[pkg.importpath] = info.component_name

    for info in auto_attached_deps:
        for pkg in info.closure.to_list():
            covered[pkg.importpath] = info.component_name

    member_importpaths = []
    for m in ctx.attr.members:
        m_path = go_importpath(m)
        if not m_path:
            fail("component %s: member %s has no importpath" % (ctx.label.name, m.label))
        member_importpaths.append(m_path)
        if m_path in covered:
            fail("component %s: member %s is already covered by component_dep %s" % (
                ctx.label.name,
                m.label,
                covered[m_path],
            ))

    interface = ctx.attr.interface
    interface_importpath = go_importpath(interface) if interface else ""

    if interface:
        if interface_importpath in covered:
            fail("component %s: its own interface package %s is covered by component_dep %s" % (
                ctx.label.name,
                interface_importpath,
                covered[interface_importpath],
            ))

    root_packages = []
    for root in roots:
        root_packages.extend(root[ArccPackageInfo].packages.to_list())
    merged = merge_by_importpath(root_packages)

    for importpath in sorted(merged.keys()):
        if merged[importpath].cgo:
            fail(("component %s: package %s is built with cgo, whose preprocessed sources are not " +
                  "available at analysis time; cgo closures are not supported yet.") % (
                ctx.label.name,
                importpath,
            ))

    for pattern, comp_name in auto_attached_patterns:
        for importpath in merged.keys():
            if match_path(pattern, importpath):
                covered[importpath] = comp_name

    effective_members = {}
    if interface_importpath:
        effective_members[interface_importpath] = True
    for m_path in member_importpaths:
        effective_members[m_path] = True

    members = _classify(ctx, merged, effective_members, covered)

    if interface and interface_importpath not in members:
        fail(("component %s: its own interface package %s is covered by a component_dep, " +
              "which would leave the component with nothing to check.") % (
            ctx.label.name,
            interface_importpath,
        ))

    all_manifest_members = {}
    for m_path in members:
        all_manifest_members[m_path] = True
    manifest_members = sorted(list(all_manifest_members.keys()))

    manifest = ctx.actions.declare_file(ctx.label.name + ".component.textproto")

    map_info = ctx.attr._stdlib_map
    stdlib_export_data = None
    if roots and "manual" not in ctx.attr.tags:
        stdlib_export_data = go_stdlib_export_data(
            ctx,
            expected_mode = arcc_sdk_key_fields(map_info[ArccStdlibMapInfo]),
        )

    if roots:
        layout = ctx.actions.declare_file(ctx.label.name + ".package-layout.json")
        # Roots are the component's effective owned packages, not every
        # raw root before direct component dependencies/infra coverage is
        # removed. Keeping covered packages in the layout roots makes the
        # manifest/layout membership contract impossible to validate once
        # the surface consumer uses the manifest's exact member set.
        layout_roots = sorted(list(members))
        platform = go_build_platform(roots[0])
        target_mode = go_target_mode(ctx)
        ctx.actions.write(
            output = layout,
            content = _layout_content(
                ctx,
                merged = merged,
                roots = layout_roots,
                go_sdk_root = go_sdk_root(ctx),
                platform = platform,
                target = target_mode,
                dependency_bindings = dependency_bindings,
                stdlib_export_data = stdlib_export_data,
            ),
        )
        direct_layouts = [layout]
    else:
        layout = None
        direct_layouts = []

    interface_files = []
    if interface:
        interface_files = sorted([
            runfiles_path(ctx, src)
            for src in go_library_srcs(interface)
            if src.extension == "go"
        ])

    ctx.actions.write(
        output = manifest,
        content = _manifest_content(
            ctx,
            interface_files = interface_files,
            component_deps = ctx.attr.component_deps,
            auto_attached_deps = auto_attached_deps,
            declared_authority = ctx.attr.declared_authority,
            manifest_dir = _dirname(runfiles_path(ctx, manifest)),
            interface_style = ctx.attr.interface_style,
            members = manifest_members,
            authority_unknown = "manual" in ctx.attr.tags,
            analysis_defeating_policy = ctx.attr.analysis_defeating_policy,
        ),
    )

    closure_srcs = []
    for importpath in merged:
        closure_srcs.extend(merged[importpath].srcs)

    member_set = {importpath: True for importpath in members}
    export_files = [
        merged[importpath].export_file
        for importpath in sorted(merged.keys())
        if importpath not in member_set
    ]

    transitive_manifests = depset(
        direct = [manifest],
        transitive = [dep[ArccComponentInfo].transitive_manifests for dep in ctx.attr.component_deps] + [info.transitive_manifests for info in auto_attached_deps],
    )
    transitive_layouts = depset(
        direct = direct_layouts,
        transitive = [dep[ArccComponentInfo].transitive_layouts for dep in ctx.attr.component_deps] + [info.transitive_layouts for info in auto_attached_deps],
    )
    contracts = depset(
        direct = ctx.files.contract,
        transitive = [dep[ArccComponentInfo].contracts for dep in ctx.attr.component_deps] + [info.contracts for info in auto_attached_deps],
    )

    covered_or_member = {importpath: True for importpath in members}

    # Direct dependency artifacts are the same files named by the layout
    # bindings. They are direct semantic action inputs: the surface resolver
    # consumes their package/symbol data and report verdicts, while the
    # producer chain remains ordered by the build graph (R8).
    dep_artifacts = []
    for record in dependency_binding_records:
        dep_artifacts += record.artifacts

    report = None
    surface = None
    provenance = None
    if "manual" in ctx.attr.tags:
        # The asserted producer path (design I6, Step 5 task 05): a manual
        # component is not analysed. Its surface is asserted ABOUT it —
        # package-level, no symbols, empty digest — written here at analysis
        # time from data already known to the rule. Step 11 replaces this
        # tag-based selection with the `authority: UNKNOWN` attribute.
        if interface:
            fail(("component %s: a manual (asserted) component cannot have a declared interface. " +
                  "Asserted surfaces are package-level (design I6): Bazel analysis has no type " +
                  "information, so an asserted surface carries no symbols and a declared-interface " +
                  "component cannot be asserted. Check the component (remove the manual tag), or " +
                  "migrate it to interface_style = PACKAGE_SURFACE.") % ctx.label.name)
        if ctx.attr.declared_authority:
            fail(("component %s: a manual (asserted) component is not analysed, so its authority is " +
                  "UNKNOWN (design R10); declared_authority must be empty. Declared authority is " +
                  "only ever established by a checked analysis.") % ctx.label.name)
        if not layout:
            fail(("component %s: a manual (asserted) component must be PACKAGE_SURFACE with at " +
                  "least one member, so its layout exists and its packages are known.") % ctx.label.name)
        surface = ctx.actions.declare_file(ctx.label.name + ".surface.json")
        ctx.actions.write(
            output = surface,
            content = _asserted_surface_content(
                component_name = ctx.label.name,
                packages = members,
                key = arcc_sdk_key_fields(map_info[ArccStdlibMapInfo]),
            ),
        )
        provenance = "asserted"
    elif layout:
        # The checked producer path (design R8, task 04): one ordinary action
        # running `arcc check` in report-verdict-only mode.
        dep_runfiles = depset(transitive = [
            dep[DefaultInfo].default_runfiles.files
            for dep in ctx.attr.component_deps
        ] + [
            target[DefaultInfo].default_runfiles.files
            for target in auto_attached_targets
        ])
        report, surface = _checked_analysis_action(
            ctx,
            manifest = manifest,
            layout = layout,
            closure_srcs = closure_srcs,
            export_files = export_files,
            stdlib_export_data = stdlib_export_data,
            transitive_manifests = transitive_manifests,
            transitive_layouts = transitive_layouts,
            dep_artifacts = dep_artifacts,
            dep_runfiles = dep_runfiles,
            sdk_root_file = go_stdlib_toolchain(ctx).root_file,
        )
        provenance = "checked"

    analysis_export_inputs = depset()
    if provenance == "checked":
        analysis_export_inputs = depset(
            direct = export_files,
            transitive = [stdlib_export_data.inputs] if stdlib_export_data != None else [],
        )

    transitive_artifacts = depset(
        direct = [artifact for artifact in (report, surface) if artifact != None],
        transitive = [dep[ArccComponentInfo].transitive_artifacts for dep in ctx.attr.component_deps] + [info.transitive_artifacts for info in auto_attached_deps],
    )

    base_runfiles = ctx.runfiles(
        files = closure_srcs,
        transitive_files = depset(transitive = [transitive_manifests, transitive_layouts]),
    )
    dep_runfiles = [dep[DefaultInfo].default_runfiles for dep in ctx.attr.component_deps] + [target[DefaultInfo].default_runfiles for target in auto_attached_targets]
    runfiles = base_runfiles.merge_all(dep_runfiles)

    providers = [
        DefaultInfo(
            files = depset([manifest] + direct_layouts),
            runfiles = runfiles,
        ),
        ArccComponentInfo(
            component_name = ctx.label.name,
            component_root = ctx.label.package,
            manifest = manifest,
            layout = layout,
            transitive_manifests = transitive_manifests,
            transitive_layouts = transitive_layouts,
            transitive_artifacts = transitive_artifacts,
            analysis_export_inputs = analysis_export_inputs,
            closure = depset([
                struct(
                    importpath = importpath,
                    srcs = tuple(merged[importpath].srcs),
                    deps = tuple(merged[importpath].deps),
                )
                for importpath in sorted(covered_or_member.keys())
            ]),
            contracts = contracts,
            surface = surface,
            report = report,
            # Structural provenance (R8): the asserted producer publishes
            # "asserted" with report = None; the checked producer publishes
            # "checked" with its report. The distinction is the producer and
            # the build graph, never a flag inside the surface file (I6, DR-01).
            provenance = provenance,
        ),
    ]
    if surface != None:
        # Lazy analysis (design §Build topology): the report and surface ride
        # in the `arcc` output group, never in DefaultInfo, so `bazel build
        # //...` runs no component analysis unless the group — or a consumer
        # — requests it. An asserted component's output group carries only
        # the asserted surface: there is no report artifact at all (task
        # req 4).
        providers.append(OutputGroupInfo(arcc = depset([
            artifact
            for artifact in (report, surface)
            if artifact != None
        ])))
    if interface:
        providers.extend(forward_go_providers(interface))
    return providers

GO_COMPONENT_ATTRS = {
    "interface": attr.label(
        mandatory = False,
        providers = GO_PROVIDERS,
        aspects = [arcc_deps_aspect],
        doc = "The declared-style public surface; absent for PACKAGE_SURFACE.",
    ),
    "interface_style": attr.string(
        default = "",
        doc = "Unset for declared style, or PACKAGE_SURFACE for member-only components.",
    ),
    "members": attr.label_list(
        providers = GO_PROVIDERS,
        aspects = [arcc_deps_aspect],
        doc = "Concrete Go library labels whose transitive closures are analyzed " +
              "alongside the interface closure.",
    ),
    "component_deps": attr.label_list(
        providers = [ArccComponentInfo],
        doc = "Other components this one depends on; their packages are excluded from this one.",
    ),
    "infra_deps": attr.label_list(
        providers = [ArccComponentInfo],
        doc = "Auto-attached infrastructure component dependencies.",
    ),
    "contract": attr.label_list(
        allow_files = True,
        doc = "Contract documents; Bazel-only metadata, not part of the manifest.",
    ),
    "declared_authority": attr.string_list(
        doc = "Ambient authority the component declares, from //bazel_rules:authority.bzl.",
    ),
    "analysis_defeating_policy": attr.string(
        default = "strict",
        doc = "Policy for analysis-defeating findings: strict (default) or warn. Only warn is emitted into the manifest.",
    ),
} | GO_CONTEXT_DATA_ATTRS | {
    # The arcc binary the analysis action runs (exec configuration, like the
    # stdlib-map generator: the target is described by declared inputs).
    "_arcc": attr.label(
        default = ARCC_TARGET,
        executable = True,
        cfg = "exec",
        doc = "The arcc binary the checked analysis action runs.",
    ),
    # The stdlib authority map the analysis action passes to arcc, so the
    # emitted surface is stamped with the exact target SDK key (task req 2,
    # AC 6). The default seam is the adapter-overridable //:arcc_stdlib_map.
    "_stdlib_map": stdlib_map_default_attr(),
}

go_component_rule = rule(
    implementation = go_component_impl,
    attrs = GO_COMPONENT_ATTRS,
    toolchains = GO_TOOLCHAINS,
    provides = [ArccComponentInfo],
    doc = "Generates an arcc manifest and package layout for a Go component, " +
          "and produces its surface through the checked analysis action — or, " +
          "for manual-tagged components, the asserted package-level write " +
          "(design I6) — with the artifacts riding in the `arcc` output group.",
)
