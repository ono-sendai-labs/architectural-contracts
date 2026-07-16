# Research: arcc Current State (relevant to Bazel integration)

Sources: `proto/archcontracts/v1/component.proto`, `README.md`, `go/internal/goanalysis/goanalysis.go`, `go/internal/capslockadapter/capslockadapter.go` (as of 2026-07-16).

## Manifest format (component.textproto)

- `name` — logical component name.
- `interface_files` — repeated string, paths **relative to the component root** (the manifest's directory). Multi-package components use subdir paths (`store/api.go`).
- `component_dependencies` — `{name, manifest}` where `manifest` is a path **relative to the declaring manifest's directory**. The resolved manifest's own directory is the dependency's component root.
- `absorbed_dependencies` — `{import_path, reason?}`; import path may be a pattern.
- `declared_authority` — capability names from Capslock's classified set, validated at parse time.

Key structural assumptions that Bazel integration challenges:

1. **Directory-based membership (FR1):** the component consists of *all Go packages under the manifest's directory*. There is no explicit implementation-file/package list.
2. **Colocation:** the manifest must sit at the component root; all paths are relative to it.
3. **Disjoint roots:** component roots must not nest.
4. **Interface packages are derived** (packages containing interface files, C10).

## How arcc loads code (the critical fact for Bazel)

Both analysis entry points use `golang.org/x/tools/go/packages`:

- `goanalysis.go:39-46` — `packages.Load(cfg, "./...")` with `Dir: componentRoot`, and again at `:504-511` with `Dir: cleanDepRoot` for component dependencies.
- `capslockadapter.go:128-132` — `packages.Load(cfg, req.Packages...)` with Capslock's required load mode (full syntax + types + deps, needed to build SSA/VTA call graphs).

`go/packages` in its default configuration **shells out to `go list`**, which requires:
- a resolvable Go module context (go.mod, module cache or vendored deps),
- network or pre-populated `GOMODCACHE` for third-party deps,
- the real workspace layout on disk (`./...` patterns are directory-relative).

This is fundamentally at odds with Bazel's sandboxed, hermetic actions (see `bazel-rules-landscape.md`). Any Bazel integration must either (a) reconstruct a `go list`-able world inside the action, (b) bypass `go list` via a custom `go/packages` driver / loader fed by Bazel-provided metadata, or (c) run non-hermetically against the source workspace.

## Capslock dependency

- `github.com/google/capslock v0.3.2` is used as a library (`capslockadapter`).
- Capslock builds a whole-program SSA + VTA call graph, so it needs **full source (syntax + types) of the entire transitive dependency closure**, including the Go standard library sources. Under Bazel, all of these must be declared action inputs for a hermetic check.

## Existing CLI surface

- `arcc check <manifest>` with `--format=json`; exit codes 0 (conforms), 1 (violations), 2 (tool error). This maps cleanly onto a Bazel test action (test passes on exit 0).

## Implications for the Bazel design

1. The user's instinct is right: the manifest (or an alternative Bazel-generated manifest) needs **explicit membership** (implementation files/packages) instead of directory-based membership, since Bazel enumerates everything explicitly and sandbox layouts don't preserve "everything under this directory" semantics.
2. Lifting **manifest colocation** is near-mandatory: a Bazel rule generates the manifest into `bazel-out/`, not into the source tree, so paths can't be manifest-directory-relative in the current sense. Options: keep relative-path semantics but generate a matching layout in the action sandbox, or extend the schema/CLI to accept explicit roots.
3. `component_dependencies.manifest` path chaining (manifest → manifest, relative paths) maps naturally onto Bazel providers: each `go_architectural_component` target's generated manifest is an artifact its dependents receive via a provider, and the rule can compute correct relative (or root-relative) paths at analysis time.
4. `declared_authority` validation already happens at arcc parse time; Bazel-side label-typed authority is purely a UX/type-checking layer on top.
