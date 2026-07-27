"""The `_go_component` rule: manifest and package-layout generation.

Everything here happens at analysis time; the only actions are the two writes.
The rule classifies the union of the interface and declared member package
closures into
component-dep-covered, absorbed, and member packages (design §5.3), emits the
manifest arcc checks and the layout arcc loads through, and forwards the
interface library's Go providers so the component target is usable as a
`deps` entry.
"""

load("//bazel_rules:authority.bzl", "ALL_AUTHORITIES")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load(":aspect.bzl", "arcc_deps_aspect", "merge_by_importpath")
load(
    ":go_adapter.bzl",
    "GO_PROVIDERS",
    "GO_TOOLCHAINS",
    "forward_go_providers",
    "go_build_platform",
    "go_importpath",
    "go_library_srcs",
    "go_sdk_root",
)
load(":paths.bzl", "runfiles_path")

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

def _manifest_content(ctx, interface_files, component_deps, absorbed, declared_authority, manifest_dir, interface_style, members):
    lines = ["name: " + _textproto_string(ctx.label.name)]

    if interface_style == "PACKAGE_SURFACE":
        lines.append("interface_style: INTERFACE_STYLE_PACKAGE_SURFACE")

    for path in interface_files:
        lines.append("interface_files: " + _textproto_string(path))

    for dep in component_deps:
        info = dep[ArccComponentInfo]
        lines.append("component_dependencies {")
        lines.append("  name: " + _textproto_string(info.component_name))

        # arcc resolves a dependency's manifest relative to the declaring
        # manifest's own directory, and derives the dependency's layout from
        # that path by convention. Both files sit in the same output tree, so
        # the relative path is well defined.
        manifest_path = _relativize(runfiles_path(ctx, info.manifest), manifest_dir)
        lines.append("  manifest: " + _textproto_string(manifest_path))
        lines.append("}")

    for importpath in absorbed:
        lines.append("absorbed_dependencies {")
        lines.append("  import_path: " + _textproto_string(importpath))

        # No `reason`: an absorbed dependency is an implementation detail the
        # component takes responsibility for, and owes its consumers no
        # justification (design §7.3).
        lines.append("}")

    for m in members:
        lines.append("members: " + _textproto_string(m))

    for authority in declared_authority:
        lines.append("declared_authority: " + _textproto_string(authority))

    return "\n".join(lines) + "\n"

def _layout_content(ctx, merged, roots, go_sdk_root, platform):
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
        packages.append({
            "ID": importpath,
            "Name": _package_name(importpath),
            "PkgPath": importpath,
            "GoFiles": go_files,
            # Identical until cgo is supported; a cgo package compiles from
            # preprocessed sources, which is why the rule rejects one below.
            "CompiledGoFiles": go_files,
            "is_stdlib": False,
        })

    layout_data = {
        "go_sdk_root": go_sdk_root,
        "roots": roots,
        "packages": packages,
    }
    if platform:
        layout_data["platform"] = {
            "goos": platform.goos,
            "goarch": platform.goarch,
            "build_tags": sorted(platform.tags),
            "cgo_enabled": platform.cgo_enabled,
        }

    return json.encode_indent(
        layout_data,
        indent = "  ",
    ) + "\n"

def _classify(ctx, merged, effective_members):
    """Splits the closure into covered, member, absorbed, and unclassified (design §5.3)."""

    # 1. Covered: a listed component dependency already accounts for it.
    covered = {}
    for dep in ctx.attr.component_deps:
        info = dep[ArccComponentInfo]
        for pkg in info.closure.to_list():
            covered[pkg.importpath] = info.component_name

    # 2. Absorbed: inside an explicitly absorbed label's transitive closure.
    absorbed_closure = {}
    for dep in ctx.attr.absorbed_deps:
        for pkg in dep[ArccPackageInfo].packages.to_list():
            absorbed_closure[pkg.importpath] = True

    members = []
    absorbed = []
    unclassified = []

    for importpath in sorted(merged.keys()):
        if importpath in covered:
            continue
        elif importpath in effective_members:
            members.append(importpath)
        elif importpath in absorbed_closure:
            # Coverage precedence has priority, but covered is checked first so we're good.
            absorbed.append(importpath)
        else:
            unclassified.append(importpath)

    return sorted(members), sorted(absorbed), sorted(unclassified)

def _go_component_impl(ctx):
    for authority in ctx.attr.declared_authority:
        if authority not in ALL_AUTHORITIES:
            fail("component %s: unknown declared_authority %r; known: %s" % (
                ctx.label.name,
                authority,
                ", ".join(ALL_AUTHORITIES),
            ))

    # 1. Gather all covered import paths to allow direct member conflict checking.
    covered = {}
    for dep in ctx.attr.component_deps:
        info = dep[ArccComponentInfo]
        for pkg in info.closure.to_list():
            covered[pkg.importpath] = info.component_name

    # 2. Gather exact import paths of authored absorbed dependencies labels.
    absorbed_label_importpaths = {}
    for dep in ctx.attr.absorbed_deps:
        importpath = go_importpath(dep)
        if not importpath:
            fail("component %s: absorbed_dep %s has no importpath; only importable Go libraries can be absorbed." % (
                ctx.label.name,
                dep.label,
            ))
        absorbed_label_importpaths[importpath] = dep.label
        if importpath in covered:
            fail("component %s: %s is already covered by component_dep %s; remove it from absorbed_deps." % (
                ctx.label.name,
                dep.label,
                covered[importpath],
            ))

    # 3. Get exact import paths of authored members and validate they are non-empty.
    member_importpaths = []
    for m in ctx.attr.members:
        m_path = go_importpath(m)
        if not m_path:
            fail("component %s: member %s has no importpath" % (ctx.label.name, m.label))
        member_importpaths.append(m_path)

    # 4. Fail analysis when an authored member label is also directly listed in absorbed_deps,
    #    or its import path is covered by a component_dep.
    for m in ctx.attr.members:
        m_path = go_importpath(m)
        if m_path in covered:
            fail("component %s: member %s is already covered by component_dep %s" % (
                ctx.label.name,
                m.label,
                covered[m_path],
            ))
        if m_path in absorbed_label_importpaths:
            fail("component %s: member %s is also listed in absorbed_deps (%s)" % (
                ctx.label.name,
                m.label,
                absorbed_label_importpaths[m_path],
            ))

    interface = ctx.attr.interface
    interface_importpath = go_importpath(interface) if interface else ""

    # Validate interface conflicts
    if interface:
        if interface_importpath in covered:
            fail("component %s: its own interface package %s is covered by component_dep %s" % (
                ctx.label.name,
                interface_importpath,
                covered[interface_importpath],
            ))
        if interface_importpath in absorbed_label_importpaths:
            fail("component %s: its own interface package %s is listed in absorbed_deps (%s)" % (
                ctx.label.name,
                interface_importpath,
                absorbed_label_importpaths[interface_importpath],
            ))

    # Construct the effective members set (implicit interface + authored members)
    effective_members = {}
    if interface_importpath:
        effective_members[interface_importpath] = True
    for m_path in member_importpaths:
        effective_members[m_path] = True

    roots = ([interface] if interface else []) + ctx.attr.members
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

    members, absorbed, unclassified = _classify(ctx, merged, effective_members)

    # Validate roots == members check on interface
    if interface and interface_importpath not in members:
        fail(("component %s: its own interface package %s is covered by a component_dep or listed in " +
              "absorbed_deps, which would leave the component with nothing to check.") % (
            ctx.label.name,
            interface_importpath,
        ))

    manifest = ctx.actions.declare_file(ctx.label.name + ".component.textproto")
    layout = ctx.actions.declare_file(ctx.label.name + ".package-layout.json")

    # Get target platform settings using go_build_platform
    platform = go_build_platform(roots[0]) if roots else None

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
            absorbed = absorbed,
            declared_authority = ctx.attr.declared_authority,
            manifest_dir = _dirname(runfiles_path(ctx, manifest)),
            interface_style = ctx.attr.interface_style,
            members = members,
        ),
    )

    ctx.actions.write(
        output = layout,
        content = _layout_content(
            ctx,
            merged = merged,
            roots = members,
            go_sdk_root = go_sdk_root(ctx),
            platform = platform,
        ),
    )

    # The layout names every package in the closure, not just this component's
    # own: a member package cannot be type-checked without the sources of what
    # it imports, whoever else covers them.
    closure_srcs = []
    for importpath in merged:
        closure_srcs.extend(merged[importpath].srcs)

    transitive_manifests = depset(
        direct = [manifest],
        transitive = [dep[ArccComponentInfo].transitive_manifests for dep in ctx.attr.component_deps],
    )
    transitive_layouts = depset(
        direct = [layout],
        transitive = [dep[ArccComponentInfo].transitive_layouts for dep in ctx.attr.component_deps],
    )
    contracts = depset(
        direct = ctx.files.contract,
        transitive = [dep[ArccComponentInfo].contracts for dep in ctx.attr.component_deps],
    )

    covered_or_member = {importpath: True for importpath in members}
    for importpath in absorbed:
        covered_or_member[importpath] = True

    providers = [
        DefaultInfo(
            files = depset([manifest, layout]),
            # Enough to run the check: the manifests and layouts of this
            # component and everything it depends on, plus every source the
            # layouts name.
            runfiles = ctx.runfiles(
                files = closure_srcs,
                transitive_files = depset(transitive = [transitive_manifests, transitive_layouts]),
            ),
        ),
        ArccComponentInfo(
            component_name = ctx.label.name,
            component_root = ctx.label.package,
            manifest = manifest,
            layout = layout,
            transitive_manifests = transitive_manifests,
            transitive_layouts = transitive_layouts,
            closure = depset([
                struct(
                    importpath = importpath,
                    srcs = tuple(merged[importpath].srcs),
                    deps = tuple(merged[importpath].deps),
                )
                for importpath in sorted(covered_or_member.keys())
            ]),
            contracts = contracts,
        ),
    ]
    if interface:
        # Declared-style components remain usable as Go deps by forwarding the
        # interface providers verbatim. Package-surface components have no
        # distinguished interface target to forward.
        providers.extend(forward_go_providers(interface))
    return providers
go_component_rule = rule(
    implementation = _go_component_impl,
    attrs = {
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
        "absorbed_deps": attr.label_list(
            providers = GO_PROVIDERS,
            aspects = [arcc_deps_aspect],
            doc = "Libraries this component absorbs as implementation details.",
        ),
        "contract": attr.label_list(
            allow_files = True,
            doc = "Contract documents; Bazel-only metadata, not part of the manifest.",
        ),
        "declared_authority": attr.string_list(
            doc = "Ambient authority the component declares, from //bazel_rules:authority.bzl.",
        ),
    },
    toolchains = GO_TOOLCHAINS,
    provides = [ArccComponentInfo],
    doc = "Generates an arcc manifest and package layout for a Go component.",
)
