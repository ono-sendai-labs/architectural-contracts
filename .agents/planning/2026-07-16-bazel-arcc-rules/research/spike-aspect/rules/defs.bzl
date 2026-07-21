"""Spike: the real _arcc_deps aspect + a layout-emitting rule.

Validates the PRODUCER side of the hermetic design (design §4.3, §5.4):
  - an aspect that reconstructs the package closure WITH per-package edges
  - correct handling of embed (src+dep merge, duplicate-importpath dedup),
    diamonds (shared reached by two paths), and stdlib exclusion
  - emission of the §5.4 package-layout JSON (roots, packages, go_sdk_root)
  - the full SDK stdlib set (finding 1) + stdlib source (finding 2) sourced
    from the rules_go toolchain, as declared inputs (hermetic)
"""
load("@rules_go//go:def.bzl", "GoInfo", "GoArchive")

GO_TOOLCHAIN = "@rules_go//go:toolchain"

# design §4.3: one struct per package, deps = tuple of direct-dep importpaths.
ArccPackageInfo = provider(
    doc = "Go package closure with edges, for arcc layout emission.",
    fields = {"packages": "depset of struct(importpath, dir, srcs, deps)"},
)

def _node_for(target, ctx):
    gi = target[GoInfo]
    ga = target[GoArchive]
    ip = gi.importpath
    if not ip:
        return None  # main packages / no-importpath libs are not closure members
    # gi.srcs already includes embed-merged srcs (validated); keep File objects.
    srcs = tuple(gi.srcs)
    pkg_dir = srcs[0].dirname if srcs else target.label.package
    # Direct package edges from the archive; exclude self (embed shares importpath),
    # and stdlib never appears here (validated). No go list, no stdlib noise.
    deps = tuple(sorted([a.data.importpath for a in ga.direct if a.data.importpath != ip]))
    return struct(importpath = ip, dir = pkg_dir, srcs = srcs, deps = deps)

def _arcc_deps_impl(target, ctx):
    if GoInfo not in target or GoArchive not in target:
        return []
    transitive = []
    for attr_name in ("deps", "embed"):
        for d in getattr(ctx.rule.attr, attr_name, []) or []:
            if ArccPackageInfo in d:
                transitive.append(d[ArccPackageInfo].packages)
    node = _node_for(target, ctx)
    direct = [node] if node else []
    return [ArccPackageInfo(packages = depset(direct = direct, transitive = transitive))]

_arcc_deps = aspect(
    implementation = _arcc_deps_impl,
    attr_aspects = ["deps", "embed"],
    provides = [ArccPackageInfo],
)

def _merge_by_importpath(nodes):
    """Fold duplicate-importpath nodes (embedder + embedded) into one package."""
    by_ip = {}
    for n in nodes:
        cur = by_ip.get(n.importpath)
        if cur == None:
            by_ip[n.importpath] = struct(
                importpath = n.importpath, dir = n.dir,
                srcs = {s.short_path: s for s in n.srcs},
                deps = {d: True for d in n.deps if d != n.importpath},
            )
        else:
            for s in n.srcs:
                cur.srcs[s.short_path] = s
            for d in n.deps:
                if d != n.importpath:
                    cur.deps[d] = True
    return by_ip

def _layout_json(interface_ip, merged, go_sdk_root):
    pkgs = []
    for ip in sorted(merged.keys()):
        m = merged[ip]
        pkgs.append('    {"importpath": "%s", "dir": "%s", "srcs": [%s], "deps": [%s]}' % (
            ip, m.dir,
            ", ".join(['"%s"' % p for p in sorted(m.srcs.keys())]),
            ", ".join(['"%s"' % d for d in sorted(m.deps.keys())]),
        ))
    return '{\n  "roots": ["%s"],\n  "go_sdk_root": "%s",\n  "packages": [\n%s\n  ]\n}\n' % (
        interface_ip, go_sdk_root, ",\n".join(pkgs))

def _arcc_component_impl(ctx):
    iface = ctx.attr.interface
    interface_ip = iface[GoInfo].importpath
    nodes = iface[ArccPackageInfo].packages.to_list()
    merged = _merge_by_importpath(nodes)

    sdk = ctx.toolchains[GO_TOOLCHAIN].sdk
    go_sdk_root = sdk.root_file.dirname + "/src"
    layout = ctx.actions.declare_file(ctx.label.name + ".package-layout.json")
    ctx.actions.write(output = layout, content = _layout_json(interface_ip, merged, go_sdk_root))

    # Inputs a hermetic check would need: every member/absorbed src File +
    # the full SDK stdlib source tree + the SDK stdlib package list.
    src_files = []
    for ip in merged:
        for f in merged[ip].srcs.values():
            src_files.append(f)
    all_inputs = depset(src_files, transitive = [sdk.srcs, depset([sdk.package_list])])

    # Emit a debug closure summary for the spike's assertions.
    summary = ctx.actions.declare_file(ctx.label.name + ".closure.txt")
    lines = ["interface_root=%s" % interface_ip, "package_count=%d" % len(merged)]
    for ip in sorted(merged.keys()):
        m = merged[ip]
        lines.append("%s | dir=%s | srcs=%s | deps=%s" % (
            ip, m.dir, ",".join(sorted(m.srcs.keys())), ",".join(sorted(m.deps.keys()))))
    lines.append("sdk_stdlib_src_count=%d" % len(sdk.srcs.to_list()))
    lines.append("go_sdk_root=%s" % go_sdk_root)
    # Every member src + full stdlib tree + package_list are real Files, so a
    # hermetic check declaring `all_inputs` gets a sandbox with no host GOROOT.
    lines.append("declared_input_count=%d" % len(all_inputs.to_list()))
    ctx.actions.write(output = summary, content = "\n".join(lines) + "\n")

    return [
        DefaultInfo(files = depset([layout, summary])),
        ArccPackageInfo(packages = iface[ArccPackageInfo].packages),
    ]

arcc_deps_probe = rule(
    implementation = _arcc_component_impl,
    attrs = {"interface": attr.label(aspects = [_arcc_deps], providers = [GoInfo])},
    toolchains = [GO_TOOLCHAIN],
)
