# The package-layout schema (for build-system emitters)

`arcc check <manifest> --package-layout=<layout.json>` runs **hermetically**: no
`go list`, no `go.mod`, no Go toolchain on the host. Everything arcc needs about
the Go package graph comes from the layout file, which the build system emits.
`bazel_rules/` is one emitter (`go_component` writes
`<name>.package-layout.json`); a monorepo with its own Go rules writes its own.
Pre-load layout diagnostics identify the owning component from the generated
layout artifact basename (`<name>.package-layout.json`), so the semantic layout
bytes remain independent of the component target name.

This document is aimed at whoever writes that emitter. Two of its rules —
`is_stdlib` and the platform block — are places where the *natural*
implementation is the wrong one, so they are spelled out at length rather than
left to be inferred from the field names.

## 1. Shape

```json
{
  "go_sdk_root": "rules_go++go_sdk+main___download_0_linux_amd64/src",
  "platform": {
    "goos": "linux",
    "goarch": "amd64",
    "build_tags": [],
    "cgo_enabled": false
  },
  "roots": ["example.com/svc", "example.com/svc/impl"],
  "ordinary_import_data": {
    "metadata": "_main/components/svc/svc_component.package-imports.json"
  },
  "stdlib_export_data": {
    "metadata": "rules_go+/stdlib_/stdlib.pkg.json",
    "export_roots": [
      {
        "runfiles_path": "rules_go+/stdlib_/gocache",
        "exec_path": "bazel-out/<config>/bin/external/rules_go+/stdlib_/gocache"
      },
      {
        "runfiles_path": "rules_go+/stdlib_/pkg",
        "exec_path": "bazel-out/<config>/bin/external/rules_go+/stdlib_/pkg"
      }
    ],
    "target": {
      "toolchain_version": "go1.26.4",
      "goos": "linux",
      "goarch": "amd64",
      "build_tags": [],
      "cgo_enabled": false,
      "goexperiment": ""
    }
  },
  "dependency_artifact_bindings": [
    {
      "dependency": "logger",
      "surface": "_main/components/logger.surface.json",
      "report": "_main/components/logger.report.json",
      "auto_attached": false,
      "provenance": "checked"
    }
  ],
  "packages": [
    {
      "ID": "example.com/svc",
      "Name": "svc",
      "PkgPath": "example.com/svc",
      "GoFiles": ["_main/svc/api.go"],
      "CompiledGoFiles": ["_main/svc/api.go"],
      "is_stdlib": false
    },
    {
      "ID": "example.com/dep",
      "Name": "dep",
      "PkgPath": "example.com/dep",
      "GoFiles": ["_main/dep/dep.go"],
      "CompiledGoFiles": ["_main/dep/dep.go"],
      "ExportFile": "external/dep/pkg.a",
      "Imports": {},
      "is_stdlib": false
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `go_sdk_root` | Path to the Go SDK's `src` directory. arcc enumerates and type-checks the standard library from here itself; the emitter does **not** list stdlib packages. |
| `platform` | The target the analysis is for (§3). Optional; absent means `build.Default`. |
| `roots` | The component's member packages. Must equal the manifest's `members`, or the load fails (§2). |
| `stdlib_export_data` | The target-configured host-adapter descriptor. `metadata` is a newline-delimited rules_go package graph; `export_roots` maps its generated execroot paths to runfiles paths; `target` is checked against `platform` before records are merged. Its compiled export trees are declared action inputs, not embedded in the package records. |
| `dependency_artifact_bindings` | Sorted direct dependency artifact bindings. Each names the manifest dependency and its runfiles-frame surface, optional report, edge origin, and structural producer provenance (§6). |
| `packages` | Every package in the closure, in `go/packages`' own driver "flat" encoding, plus the layout-only `is_stdlib` bit (§4). Member roots carry source fields; effective non-members carry `ExportFile` for member-only loading. |
| `ordinary_import_data` | Stable logical token for the deterministic, target-configured graph descriptor generated from the ordinary package sources. The driver resolves it to the sibling declared output using the layout artifact path; it supplies exact direct imports, including implicit standard-library edges that rules_go's archive dependency provider does not expose. |

Per-package fields are exactly `packages.Package`'s JSON, so `ID`, `Name`,
`PkgPath`, `GoFiles`, `CompiledGoFiles`, `ExportFile` and `Imports` mean what
that type means. The driver JSON field is spelled `ExportFile` (not
`export_file`). Package IDs are import paths — fold the closure by import path
so they are unique.

**Path frames.** Emitter source paths and export artifacts use arcc's
workspace/runfiles working-directory frame. Standard-library *source* paths in
a source-backed SDK layout are the exception: they resolve below
`go_sdk_root`. Export artifacts are build outputs, never SDK-source paths, so
`ExportFile` is always resolved in the workspace/runfiles frame independently
of `go_sdk_root`. Export paths must be non-empty, relative, slash-separated,
normalized, and free of parent escapes; the validator also checks that the
resolved artifact is a non-empty readable regular file. Dependency surface and
report paths use the same workspace frame and are validated as relative,
normalized slash paths before the driver runs.

## 2. `roots` and members

`roots` is the set of packages the component owns and analyzes as roots. arcc
compares it against the manifest's `members` field and fails closed on any
difference — the two artifacts are written by the same emitter from the same
list, so a mismatch means one of them was hand-edited or the emitter is
inconsistent.

A declared member with **no source files** is a load error naming the package: a
component cannot own code the analysis cannot see, and the layout makes that
checkable rather than merely stated.

## 3. `platform` — and the two conforming shapes

The platform block is what the loader builds its `build.Context` from, instead of
using whatever platform the arcc binary was compiled for. Getting this wrong is
silent: type-checking a wrong-platform file makes a package ill-typed, and the
analysis degrades without saying so.

The emitter must not assume it can filter. A host whose Go rules expose the
declared source set — rules_go's `GoInfo.srcs` lists every `_GOOS.go` variant,
because its *compiler*, not its provider, applies build constraints — cannot
produce a per-platform file list at analysis time, and should not have to.
So **two shapes conform**:

1. **Filtered.** The emitter declares files *and* `Imports` for the declared
   platform, consistently.
2. **Unfiltered, `Imports` omitted.** The emitter declares the full source set
   and **no** `Imports` at all; the loader recovers the edges by parsing the
   files that survive filtering. This is what `bazel_rules/` does.

What does **not** conform is the accidental middle: an unfiltered file set with
`Imports` derived from it. That layout claims edges only a
non-compiled file contributes, and it is what a naive emitter produces.

**The loader validates rather than trusts.** It is already parsing sources for
import recovery, so after filtering it compares the declared imports against the
imports of the surviving files and errors on a mismatch **in either direction** —
a declared import no surviving file contributes, or an import a surviving file
contributes that was never declared (an edge FR2 would otherwise never see).

Two details worth knowing before you debug an error from this:

- **Equality is over *resolvable* imports.** An import resolving to a layout
  package or to the standard library participates. One that resolves to
  **nothing** does not: an emitter may legitimately drop an edge with no node
  behind it rather than emit a dangling reference. Such imports are not silently
  dropped either — the loader collects them and `arcc check` reports each as an
  `ANALYSIS_LIMITATION` warning naming the package, file and import path. Post
  filtering these should be rare (the usual cause, a `_windows.go` importing
  something the build never compiled, is exactly what filtering removes), so one
  usually means a wrong platform block or a genuinely incomplete closure.
- **Omitting `Imports` is not the same as `"Imports": {}`.** An absent (or
  `null`) key selects shape 2. An empty object is shape 1 declaring zero imports,
  and any recovered resolvable import will then fail the equality check.

A package left with **no** Go sources after filtering is a load error, not an
empty package.

## 3a. Source and export-data package roles

The package graph has two intentional layouts. A component/member-only layout
has source ownership only at its roots and carries compiler export artifacts for
the complete reachable non-root closure. A source-backed whole-SDK layout used
by `arcc stdlibmap generate` has source fields for every SDK package and does
not require export artifacts.

For member-only validation:

- **Member roots** retain their selected `GoFiles` and `CompiledGoFiles`,
  resolved in the workspace frame. If `Imports` is omitted, arcc may recover
  the root's edges from those member sources; once present, the map is checked
  against the surviving source imports.
- **Reachable non-members** retain their package identity and every direct
  `Imports` edge, but their effective type-loading input is `ExportFile`. The
  path is a workspace-frame build output, not a path under `go_sdk_root`.
  Every such package except the builtin `unsafe` must have an export artifact
  and an explicit `Imports` map. A leaf must use `"Imports": {}`; an omitted
  or `null` field is an incomplete graph, not an empty leaf.
- The layout may retain non-member source fields for inspection, but the checked
  analysis action does not declare or stage them. Its driver response selects
  source for roots and `ExportFile` for non-roots; the post-load guard fails if
  dependency syntax, type info, or source lists reappear. The emitted ordinary
  non-member records already carry their export artifact; the build-time
  ordinary-import descriptor supplies the exact direct graph, while the stdlib
  descriptor supplies the target-configured SDK graph.
- **The graph is not an API-surface projection.** Packages and edges are kept
  even when an imported package does not appear to be referenced by the
  exporting package's public API. The validator walks sorted roots and sorted
  import paths and rejects a dangling package ID before `go/packages` sees the
  layout. It never infers a non-member graph from non-member source files at
  load time: the emitter's separate `ArccImportGraph` metadata action computes
  that graph before the check action and the check consumes only its declared
  descriptor.
- **`unsafe`** is the one reachable non-root builtin that has no compiler
  export artifact. It remains a graph node when the emitter declares the edge.

`StdlibLayout` is deliberately a different contract. It discovers the target
SDK's whole standard-library source tree, marks those packages as SDK-provided,
and validates them through the ordinary source-backed `ValidateAndResolve`
path. That generation layout may have no `ExportFile` values and must not be
passed through the member-only export validator.

### 3b. The Bazel ordinary-import and stdlib export-data handoff

The ordinary-package graph has a parallel handoff. rules_go's
`GoArchive.direct` provider represents declared archive dependencies and does
not include implicit SDK imports such as `strings`. The component emitter runs
a small `ArccImportGraph` action with arcc's lexical source scanner and the
declared target build context. Its output is the `ordinary_import_data.metadata`
file; `ArccLayout` applies it to the emitted non-member `Imports` maps, and the
checked action declares the descriptor and recreates its runfiles frame.
This is metadata projection, not component analysis, and it executes no
toolchain binary. The runtime resolver validates the descriptor's target
identity, maps each direct import path to the effective layout package ID, and
applies it to non-member packages before any source-backed transition or
`packages.Load`. The layout artifact therefore visibly retains an edge such as
`lowlevel -> strings`; the runtime pass also repairs host-specific stdlib IDs.

The Bazel emitter obtains member-only standard-library material through its host
adapter. The adapter returns a host-neutral descriptor with four values:

| Descriptor value | Package-layout use |
|---|---|
| `metadata` (`File`) | Generated package JSON containing every target-configured stdlib package identity, direct `Imports` edge, and `ExportFile` path. |
| `export_files` (`depset[File]`) | Generated cache/archive trees containing the compiled export artifacts named by `metadata`. |
| `inputs` (`depset[File]`) | The exact union of `metadata` and `export_files` for declaration on the component action. |
| `target` (platform record) | The GOOS, GOARCH, cgo, tags, toolchain version, and GOEXPERIMENT identity checked against the attached stdlib authority map. |

The package-layout emitter consumes this descriptor in Task 4: it stages the
metadata and export trees, rewrites the generated paths into the workspace
frame, and lets the runtime validator populate `ExportFile` for every reachable
stdlib node while preserving the metadata's complete import graph. It does not
read or infer the graph from import-path spelling. The descriptor's `inputs`
contain no SDK `.go` source, `go` binary, compiler/linker tools, undeclared host
cache, or network dependency; source-backed `StdlibLayout` generation remains a
separate path.

The concrete upstream metadata uses `__BAZEL_EXECROOT__/` as a placeholder in
each `ExportFile`. Runtime resolution strips only that leading marker, verifies
the resulting relative path and artifact, and resolves it against the action
execroot. A marker in any other position, an absolute path, or a parent escape
is rejected. The checked `ArccCheck` action declares exactly this input set: the
selected member `.go` files; the manifest and package layout; ordinary
non-member `ExportFile` artifacts and the ordinary-import graph descriptor; the
stdlib metadata file and its generated `gocache`/`pkg` export trees; each direct
dependency's surface and applicable report; and the target stdlib map. It does
not declare dependency or SDK `.go` source, a Go/toolchain binary, an undeclared
cache, or a network input. The auxiliary `ArccImportGraph` action may read
ordinary source to project direct imports, but that source is not an input to
the component analysis action.

**Native loading.** Without a package layout, `go/packages` uses the host Go
driver and its `go list -export` results. Native checks may therefore execute
the host toolchain and read its build cache; an unsupported or mismatched export
artifact is a tool error naming the package and artifact. Bazel/layout checks
use arcc's self-exec driver instead and consume only declared export files.

The optional `export_roots` records are the same two spellings the host adapter
knows for each generated tree: `exec_path` is the relative path beneath the
action execroot, while `runfiles_path` is the path staged for the replay/test
runfiles frame. Runtime replaces a metadata export path beneath the execroot
root with the corresponding runfiles path before checking the artifact. This
keeps the layout usable both from the action wrapper's execroot and from a
checked-analysis test's runfiles root without guessing a repository name.

**Pinning the toolchain.** The platform block may carry two optional
identity fields, `toolchain_version` (`go1.N.M`) and `goexperiment`. When
present they pin the loader's release tags and tool (GOEXPERIMENT) tags to
the named toolchain instead of the host arcc runs under, which is what makes
the emitted surface's SDK key reproducible from the layout alone. Omit a
field when it is empty — a present-but-empty `toolchain_version` is a
validation error, and a present-but-empty `goexperiment` pins
`GOEXPERIMENT=none`. An emitter whose check stamps surfaces against a
declared stdlib-map artifact (the Bazel rules' analysis action) must pin
both fields to exactly the target configuration the map was generated for;
the loader fails closed on any key mismatch between the pinned platform and
the declared map.

## 4. `is_stdlib` — provenance, never a heuristic

Each emitter-listed package carries an `is_stdlib` boolean. It must record
**where the package came from in the build graph** — the SDK or toolchain, versus
a target the emitter enumerated. It must **not** be computed by re-evaluating an
import-path heuristic in the emitter.

This is the load-bearing sentence in the whole schema, because the wrong
implementation is the obvious one and it is silently self-defeating. arcc uses
the declared bit and SDK discovery as structural provenance for resource
resolution; it never reconstructs standard-library membership from the import
path spelling. A dotless host or third-party path therefore remains non-stdlib
when the emitter identifies it as non-SDK. Check-time standard-library membership
comes only from the total authority map, while missing SDK or package data still
fails closed during layout validation.

Practical consequences:

- If every package your emitter lists comes from an enumerated build target — the
  SDK not being in its metadata set at all — then `is_stdlib` is structurally
  `false` for all of them, and `true` never needs computing. That is the
  `bazel_rules/` case.
- Standard-library packages are **not** listed by the emitter. arcc discovers
  them from `go_sdk_root` and treats SDK provenance as structural, in layout mode
  and native mode alike.
- Omitting the field decodes as `false`. That is deliberate compatibility with
  older hand-written layouts; no path-based fallback changes the decoded
  provenance.

## 5. cgo

A cgo package compiles from preprocessed sources that do not exist at analysis
time, so `CompiledGoFiles` cannot be named honestly in a layout emitted from
build-graph metadata. `bazel_rules/` therefore refuses a closure containing one:
the component's declaration fails at analysis time rather than producing a layout
pointing at files that will not be in the sandbox. Native mode is unaffected,
because the go tool preprocesses cgo before `go/packages` sees it. See the
README's limitations for what that means for a component whose closure includes
cgo code.

## 6. Direct dependency artifact bindings

`dependency_artifact_bindings` is build metadata emitted from the direct
`ArccComponentInfo` provider edges. It is not an authored trust claim and does
not carry authority, a report verdict, or freshness. The collection is sorted
by `dependency` and contains at most one record for each direct dependency.

Each record has the following fields:

| Field | Meaning |
|---|---|
| `dependency` | The logical name from the manifest's direct component dependency. |
| `surface` | Required runfiles-frame path to the dependency's surface artifact. |
| `report` | Optional runfiles-frame path to the dependency's report artifact; present for checked providers and absent for asserted providers. |
| `auto_attached` | Whether the edge was attached by the build-system infrastructure mechanism rather than authored in the manifest. |
| `provenance` | Structural producer kind, exactly `checked` or `asserted`; it is derived from the provider and is not read from the surface file. |

The emitter must fail analysis for a missing surface, a checked provider without
a report, an asserted provider with a report, unknown provider provenance,
duplicate dependency names, or an authored/auto-attached name collision. A
surface path may not be empty, absolute, parent-escaping, or non-normalized;
duplicate surface paths across dependency names are rejected as well. The
checked action declares every bound surface and report as a direct input and
constructs its path frame from those explicit files; transitive dependency
runfiles are not the source of binding discovery.

Native mode remains convention-based and does not consume this build-graph
collection or synthesize Bazel provenance. It locates the required surface next
to the dependency manifest by replacing the manifest extension with
`.surface.json`, and may read the optional sibling `.report.json`; the binding
metadata described here is emitted and consumed by layout/Bazel mode.

The resulting dependency report keeps status axes independent. A checked
provider with a passing report is `CHECKED_PASS`, a checked provider with a
failing report is `CHECKED_FAIL`, and an asserted provider is `ASSERTED`.
Layout/Bazel freshness is `BUILD_GRAPH`; native freshness is `VERIFIED` or
`STALE` when the named member source bytes can be audited and `UNKNOWN` when
they cannot. Consumers render these axes as `certified`, `check failed`,
`asserted`, `stale`, and `untrusted` words without collapsing them into one
field. Native integration harnesses must stage both sibling artifacts in
topological order and must replace/restore pre-existing files rather than
accepting a stale developer cache hit.
