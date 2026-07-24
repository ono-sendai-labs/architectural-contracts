"""The `_arcc_deps` aspect: the Go package closure, with edges.

`GoArchiveData` exposes no public importpath-keyed edge data (its only edge
data is the private, label-based `_dep_labels`), but the package-layout JSON
arcc consumes is keyed by import path. So this aspect walks the graph itself
and projects each node's direct archives onto their import paths.

Reference implementation and the reasoning behind every non-obvious line:
`.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-aspect-findings.md`.
"""

load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load(":go_adapter.bzl", "go_target_info", "is_go_target")

# Attributes the closure propagates over.
#
# `embed` has to be traversed even though embedded libraries are *not* closure
# members: an embed-only transitive dependency (embedder -> embed -> embedded
# -> deps -> X) is reachable no other way, and a layout missing X cannot be
# type-checked. The cost is that the embedded library is itself visited, under
# the same import path as its embedder, which is what `merge_by_importpath`
# below exists to fold back together.
_ATTR_ASPECTS = ["deps", "embed"]

def _arcc_deps_impl(target, ctx):
    # A dependency edge can point at a filegroup, a proto target, or anything
    # else that is not a Go library.
    if not is_go_target(target):
        return []

    transitive = []
    for attr_name in _ATTR_ASPECTS:
        for dep in getattr(ctx.rule.attr, attr_name, None) or []:
            if ArccPackageInfo in dep:
                transitive.append(dep[ArccPackageInfo].packages)

    node = go_target_info(target)
    return [ArccPackageInfo(
        packages = depset(direct = [node] if node else [], transitive = transitive),
    )]

arcc_deps_aspect = aspect(
    implementation = _arcc_deps_impl,
    attr_aspects = _ATTR_ASPECTS,
    provides = [ArccPackageInfo],
    doc = "Collects the Go package closure of a target, with direct-dependency edges.",
)

def merge_by_importpath(nodes):
    """Folds nodes sharing an import path into one package.

    An embedder and its embedded libraries all carry the embedder's import
    path, so the closure can contain several nodes per package. They are
    unioned rather than overwritten: a keyed dict with last-write-wins is
    order-dependent, and depset iteration order is not something to rely on.
    The union is safe because rules_go makes the embedder's srcs and direct
    archives a superset of the embedded library's.

    Args:
      nodes: closure nodes, as emitted by `arcc_deps_aspect`.

    Returns:
      dict of importpath -> struct(importpath, srcs, deps, cgo), where srcs is
      a list of File sorted by path and deps a sorted list of importpaths.
    """
    by_importpath = {}
    for node in nodes:
        entry = by_importpath.get(node.importpath)
        if entry == None:
            entry = struct(srcs = {}, deps = {}, cgo = [node.cgo])
            by_importpath[node.importpath] = entry
        for src in node.srcs:
            entry.srcs[src.path] = src
        for dep in node.deps:
            if dep != node.importpath:
                entry.deps[dep] = True
        entry.cgo.append(node.cgo)

    merged = {}
    for importpath in sorted(by_importpath.keys()):
        entry = by_importpath[importpath]
        merged[importpath] = struct(
            importpath = importpath,
            srcs = [entry.srcs[path] for path in sorted(entry.srcs.keys())],
            deps = sorted(entry.deps.keys()),
            cgo = True in entry.cgo,
        )
    return merged
