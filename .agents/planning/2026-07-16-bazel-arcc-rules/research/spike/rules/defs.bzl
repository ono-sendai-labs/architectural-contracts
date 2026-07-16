"""Spike: go_component rule + symbolic macro.

Validates:
  A. Forwarding GoInfo/GoArchive so the component target works in go_* deps.
  B. Enumerating transitive package closure from GoArchive at analysis time
     and generating a manifest textproto.
  C. Symbolic macro expansion into component rule + check test (Bazel 8+).
  D. A tags=["local"] test resolving real workspace paths from runfiles.
"""

load("@rules_go//go:def.bzl", "GoArchive", "GoInfo")

ArccComponentInfo = provider(
    doc = "Architectural component metadata.",
    fields = {
        "component_name": "logical component name",
        "manifest": "generated manifest File",
        "transitive_manifests": "depset of manifest Files of this and all component deps",
        "contracts": "depset of contract doc Files",
    },
)

def _manifest_content(ctx, interface_infos, interface_archives, dep_components):
    lines = ['name: "%s"' % ctx.label.name]

    # Interface files: all srcs of the interface libraries (Q3: package-level).
    iface_srcs = []
    for info in interface_infos:
        iface_srcs += [s for s in info.srcs]
    for s in iface_srcs:
        lines.append('interface_files: "%s"' % s.short_path)

    # Component deps: name + generated manifest path.
    covered = {}
    for dc in dep_components:
        info = dc[ArccComponentInfo]
        lines.append("component_dependencies {")
        lines.append('  name: "%s"' % info.component_name)
        lines.append('  manifest: "%s"' % info.manifest.short_path)
        lines.append("}")

    # Transitive closure from GoArchive: candidate absorbed deps.
    seen = {}
    for arch in interface_archives:
        for data in arch.transitive.to_list():
            if data.importpath and data.importpath not in seen:
                seen[data.importpath] = [s.short_path for s in data.srcs]
    for ip, srcs in seen.items():
        lines.append('# closure: %s (%s)' % (ip, ", ".join(srcs)))

    for cap in ctx.attr.declared_authority:
        lines.append('declared_authority: "%s"' % cap)
    return "\n".join(lines) + "\n"

def _go_component_impl(ctx):
    interface_infos = [t[GoInfo] for t in ctx.attr.interface]
    interface_archives = [t[GoArchive] for t in ctx.attr.interface]

    manifest = ctx.actions.declare_file(ctx.label.name + ".component.textproto")
    ctx.actions.write(
        output = manifest,
        content = _manifest_content(ctx, interface_infos, interface_archives, ctx.attr.component_deps),
    )

    providers = [
        ArccComponentInfo(
            component_name = ctx.label.name,
            manifest = manifest,
            transitive_manifests = depset(
                [manifest],
                transitive = [d[ArccComponentInfo].transitive_manifests for d in ctx.attr.component_deps],
            ),
            contracts = depset(ctx.files.contract),
        ),
        DefaultInfo(files = depset([manifest])),
    ]

    # (A) Forward the interface library's Go providers so this target is a
    # drop-in dep for go_library/go_test. Single-interface assumption for spike.
    if len(ctx.attr.interface) == 1:
        providers.append(interface_infos[0])
        providers.append(interface_archives[0])
    return providers

_go_component_rule = rule(
    implementation = _go_component_impl,
    attrs = {
        "interface": attr.label_list(providers = [GoInfo, GoArchive], mandatory = True),
        "component_deps": attr.label_list(providers = [ArccComponentInfo]),
        "contract": attr.label_list(allow_files = True),
        "declared_authority": attr.string_list(),
    },
)

def _check_test_impl(ctx):
    info = ctx.attr.component[ArccComponentInfo]
    iface_src = ctx.attr.component[GoInfo].srcs[0]
    script = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.write(
        output = script,
        content = """#!/bin/bash
set -euo pipefail
echo "=== generated manifest ==="
cat "{manifest}"
echo "=== (D) realpath of interface src in runfiles ==="
real=$(realpath "{src}")
echo "$real"
case "$real" in
  */spike/*) echo "OK: resolves into real workspace" ;;
  *) echo "NOTE: does not resolve into workspace (sandboxed?)" ;;
esac
""".format(manifest = info.manifest.short_path, src = iface_src.short_path),
        is_executable = True,
    )
    runfiles = ctx.runfiles(files = [info.manifest, iface_src] + list(info.transitive_manifests.to_list()))
    return [DefaultInfo(executable = script, runfiles = runfiles)]

_check_test = rule(
    implementation = _check_test_impl,
    test = True,
    attrs = {
        "component": attr.label(providers = [ArccComponentInfo]),
    },
)

def _go_component_macro_impl(name, visibility, interface, component_deps = [], contract = [], declared_authority = [], **kwargs):
    _go_component_rule(
        name = name,
        visibility = visibility,
        interface = interface,
        component_deps = component_deps,
        contract = contract,
        declared_authority = declared_authority,
        **kwargs
    )
    _check_test(
        name = name + ".check",
        component = ":" + name,
        tags = ["local"],
        visibility = visibility,
    )

go_component = macro(
    implementation = _go_component_macro_impl,
    attrs = {
        "interface": attr.label_list(configurable = False, mandatory = True),
        "component_deps": attr.label_list(configurable = False),
        "contract": attr.label_list(allow_files = True, configurable = False),
        "declared_authority": attr.string_list(configurable = False),
    },
)
