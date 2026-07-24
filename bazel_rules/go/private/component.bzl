"""The `_go_component` rule: manifest and package-layout generation.

Everything here happens at analysis time; the only actions are the two writes.
The rule classifies the interface library's package closure into
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

def _manifest_content(ctx, interface_files, component_deps, absorbed, declared_authority, manifest_dir):
    lines = ["name: " + _textproto_string(ctx.label.name)]

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

    for authority in declared_authority:
        lines.append("declared_authority: " + _textproto_string(authority))

    return "\n".join(lines) + "\n"

def _layout_content(ctx, merged, roots, go_sdk_root):
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
        imports = {dep: dep for dep in pkg.deps}

        packages.append({
            "ID": importpath,
            "Name": _package_name(importpath),
            "PkgPath": importpath,
            "GoFiles": go_files,
            # Identical until cgo is supported; a cgo package compiles from
            # preprocessed sources, which is why the rule rejects one below.
            "CompiledGoFiles": go_files,
            "Imports": imports,
        })

    return json.encode_indent(
        {
            "go_sdk_root": go_sdk_root,
            "roots": roots,
            "packages": packages,
        },
        indent = "  ",
    ) + "\n"

def _classify(ctx, merged):
    """Splits the closure into covered / absorbed / member (design §5.3)."""

    # 1. Covered: a listed component dependency already accounts for it.
    covered = {}
    for dep in ctx.attr.component_deps:
        info = dep[ArccComponentInfo]
        for pkg in info.closure.to_list():
            covered[pkg.importpath] = info.component_name

    # 2. Absorbed: a listed label, or anything in its closure.
    absorbed = {}
    for dep in ctx.attr.absorbed_deps:
        importpath = go_importpath(dep)
        if not importpath:
            fail("component %s: absorbed_dep %s has no importpath; only importable Go libraries can be absorbed." % (
                ctx.label.name,
                dep.label,
            ))

        # Coverage wins over absorption, so absorbing a package a component
        # dependency already covers is a contradiction rather than a silent
        # no-op. Only the label the author wrote down is an error; a package
        # that merely happens to be inside an absorbed library's closure is
        # left to the coverage rule.
        if importpath in covered:
            fail("component %s: %s is already covered by component_dep %s; remove it from absorbed_deps." % (
                ctx.label.name,
                dep.label,
                covered[importpath],
            ))

        for pkg in dep[ArccPackageInfo].packages.to_list():
            absorbed[pkg.importpath] = True

    # 3. Member: everything else. With explicit membership there is no
    #    unaccounted fall-through — every closure package lands in one bucket.
    members = []
    absorbed_members = []
    for importpath in sorted(merged.keys()):
        if importpath in covered:
            continue
        if importpath in absorbed:
            absorbed_members.append(importpath)
        else:
            members.append(importpath)

    return members, absorbed_members

def _check_component_roots(ctx):
    """No component dependency may be rooted inside this component."""
    root = ctx.label.package
    for dep in ctx.attr.component_deps:
        dep_root = dep[ArccComponentInfo].component_root
        if dep_root == root or dep_root.startswith(root + "/") or root == "":
            fail("component %s: component_dep %s has root %s, nested inside this component's root %s." % (
                ctx.label.name,
                dep[ArccComponentInfo].component_name,
                dep_root if dep_root else "(repository root)",
                root if root else "(repository root)",
            ))

def _go_component_impl(ctx):
    for authority in ctx.attr.declared_authority:
        if authority not in ALL_AUTHORITIES:
            fail("component %s: unknown declared_authority %r; known: %s" % (
                ctx.label.name,
                authority,
                ", ".join(ALL_AUTHORITIES),
            ))

    _check_component_roots(ctx)

    interface = ctx.attr.interface
    merged = merge_by_importpath(interface[ArccPackageInfo].packages.to_list())

    for importpath in sorted(merged.keys()):
        if merged[importpath].cgo:
            fail(("component %s: package %s is built with cgo, whose preprocessed sources are not " +
                  "available at analysis time; cgo closures are not supported yet.") % (
                ctx.label.name,
                importpath,
            ))

    members, absorbed = _classify(ctx, merged)
    interface_importpath = go_importpath(interface)
    if interface_importpath not in members:
        fail(("component %s: its own interface package %s is covered by a component_dep or listed in " +
              "absorbed_deps, which would leave the component with nothing to check.") % (
            ctx.label.name,
            interface_importpath,
        ))

    manifest = ctx.actions.declare_file(ctx.label.name + ".component.textproto")
    layout = ctx.actions.declare_file(ctx.label.name + ".package-layout.json")

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
        ),
    )

    ctx.actions.write(
        output = layout,
        content = _layout_content(
            ctx,
            merged = merged,
            roots = members,
            go_sdk_root = go_sdk_root(ctx),
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

    return [
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
    ] + forward_go_providers(interface)
    # The interface library's Go providers are forwarded verbatim so
    # `deps = [":some_component"]` works from any Go rule: the component target
    # stands in for its interface library.

go_component_rule = rule(
    implementation = _go_component_impl,
    attrs = {
        "interface": attr.label(
            mandatory = True,
            providers = GO_PROVIDERS,
            aspects = [arcc_deps_aspect],
            doc = "The single go_library holding the component's public surface.",
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
