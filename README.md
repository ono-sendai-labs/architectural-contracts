# Architectural Contracts MVP (Go)

Architectural Contracts is a Go-based tool (`arcc`) for declaring, checking, and enforcing package structures, dependency boundaries, and ambient authority limits (capabilities) in software systems. 

By defining declarative boundaries on top of your existing code, you gain durable intellectual control over your architecture. For more detailed discussion about the background, design space, and core rationales, please consult the [Rationale and Concepts Guide](docs/rationale-and-concepts.md).

---

## Table of Contents
- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Building and Installation](#building-and-installation)
- [CLI Reference and Exit Codes](#cli-reference-and-exit-codes)
- [The CSV Tool Walkthrough](#the-csv-tool-walkthrough)
- [Authoring a Component Manifest](#authoring-a-component-manifest)
- [Limitations and Scope](#limitations-and-scope)
- [Development and Contributing](#development-and-contributing)

---

## Overview

Architectural Contracts is organized around three primary pillars:
1. **Architecture as Code (Pillar 1):** Explicitly declaring how your codebase is partitioned into distinct components, specifying what files constitute their public interfaces, and verifying that internal package imports conform to declared component and implementation-detail dependencies.
2. **Informal Contracts (Pillar 2):** Documenting in plain-prose comments in interface files what each component does, requires, and provides, facilitating modular human reasoning.
3. **Ambient Authority and Capabilities (Pillar 3):** Tracking and restricting what system capabilities (such as filesystem access, network sockets, or binary execution) each component can touch, allowing for self-sandboxing.

---

## Prerequisites

To compile and use the `arcc` tool, you need:
- **Go 1.26 or later** (compatible with modern Go toolchains).
- **`just`** (optional, recommended command-runner for building and linting).
- **Bazel 8 or later** (optional; only for the Bazel build described below — it downloads its own Go SDK, so no system Go is required for that path).
- **Protocol Buffer compiler (`protoc`)** (required to run the complete CI pipeline via `just ci`, or if you intend to modify the protobuf schema).
- **`protoc-gen-go` Go plugin** (required by `protoc` for Go code generation during `just ci`). You can install it using:
  ```bash
  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
  ```
  Ensure your Go binary installation directory (typically `$GOPATH/bin` or `$HOME/go/bin`) is in your system's `PATH`.

*Note: If you do not have `protoc` or `protoc-gen-go` installed, you can still build and run tests using `just` or Go directly without running the protobuf verification steps (see below).*

---

## Building and Installation

From a fresh clone of the repository, you can build the CLI binary and run all tests immediately.

### Using `just` (Recommended)
Compile the `arcc` binary into the `./bin` directory:
```bash
just build
```

Verify that everything builds, lints, and passes tests (including selfcheck):
```bash
# Runs the full CI pipeline, which includes verifying that generated Go files
# match the protobuf schema. Requires `protoc` and `protoc-gen-go`.
just ci
```

If you do not have `protoc` or `protoc-gen-go` installed, you can run all other verification checks (lint, build, unit/integration tests, and self-hosting checks) using:
```bash
just lint build test test-integration selfcheck
```

### Using raw Go commands
Alternatively, you can build directly using the Go toolchain:
```bash
# Navigate to the Go module directory
cd go

# Build the arcc CLI
go build -o ../bin/arcc ./cmd/arcc
```

### Using Bazel

The repository is also a Bazel module named `rules_arcc`, which builds arcc from
source and exposes it under a stable label:

```bash
bazel build @rules_arcc//:arcc
bazel run   @rules_arcc//:arcc -- --version
```

To consume it from another workspace, add it as a `bazel_dep` (a
`git_override`/`archive_override` is needed until `rules_arcc` is published to
the Bazel Central Registry):

```python
bazel_dep(name = "rules_arcc", version = "0.0.0")
```

arcc is pure Go, so nothing here needs a C compiler — but rules_go's default
mode does. Build with `--@rules_go//go/config:pure` (this repo sets it in
`.bazelrc`) or make sure a CC toolchain is configured.

The Bazel build is additive: `just`, `go build` and `go test` are unaffected,
and the generated `BUILD.bazel` files sit alongside the Go sources. Regenerate
them with `bazel run //:gazelle` after adding or moving packages.

### Declaring and checking components with `go_component`

`rules_arcc` exposes a `go_component` rule that generates a component's manifest
from its Go build graph and checks it hermetically — no `component.textproto`
to hand-write and keep in sync, and no `go.mod` or Go toolchain needed at check
time. Load it, and the authority constants, from one path:

```python
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component", "FILES")

go_library(
    name = "csvfile",
    srcs = ["csvfile.go", "private.go"],
    importpath = "example.com/csvtool/csvfile",
    deps = ["//csvtool/internal/parsecsv"],
)

go_component(
    name = "csvfile_component",
    interface = ":csvfile",                       # exactly one go_library — the public surface
    members = ["//csvtool/internal/parsecsv"],    # implementation packages this component owns
    declared_authority = [FILES],                 # ambient authority it is permitted to use
    visibility = ["//visibility:public"],
)
```

The attributes mirror the manifest schema below:

- **`interface`** — the single `go_library` holding the component's public API. Passing a list is a load-time error. Required for the default (declared) style; rejected under `PACKAGE_SURFACE`.
- **`members`** — additional `go_library` labels the component owns. The rule resolves each to its import path and writes the **fully expanded literal list** into the manifest's `members`, which is also the layout's `roots`. Member packages are analysis roots, so their authority is charged to this component whoever calls them, and their imports are checked against the component's declarations.
- **`component_deps`** — other `go_component` targets this one depends on. Their packages are covered by them, so arcc checks references through their exact declared interfaces (the `app` example stays authority-free this way).
- **`interface_style`** — omit for the declared style, or pass `PACKAGE_SURFACE` (exported by `defs.bzl`) to wrap a library that has no architectural interface. Under `PACKAGE_SURFACE`, `interface` must be absent and `members` non-empty. Members are always literal target labels; import-path patterns are rejected.
- **`declared_authority`** — authority constants from `defs.bzl` (`FILES`, `NETWORK`, …). Empty means the component claims to be authority-free.
- **`contract`** — optional contract documents; Bazel-only metadata arcc never reads.

```python
go_component(
    name = "svc_component",
    interface = ":svc_api",
    members = [
        ":svc_impl",              # concrete labels — no wildcards
        "//svc/internal/store",
    ],
    declared_authority = [FILES],
)
```

**Why `members` takes concrete labels and there is no wildcard helper.**
`rules_arcc` deliberately ships no `arcc_subpackages()`-style expander. Bazel's
`native.subpackages()` (and bazel-skylib's `subpackages.all()` wrapper over it)
returns only the *frontier* of nearest descendant packages and cannot see past a
package boundary at all, so a helper named after subtrees would not deliver
subtrees — `include = ["a/deep"]` returns `[]` when `//comp/a/deep` exists. It is
also rejected inside a symbolic macro, so it could not live inside
`go_component` in any case. An author who wants that frontier, knowing what it
is, calls skylib directly at BUILD top level; everyone else lists labels. A
misleading helper is worse than no helper, and the cost of listing labels is
recorded honestly under [Limitations](#limitations-and-scope).

### Porting the rules to another Go ruleset

Everything the rules need from the host's Go rules is funnelled through one file,
`bazel_rules/go/private/go_adapter.bzl`; the rules above it stay byte-identical
across hosts. Alongside the provider/importpath/srcs accessors, it carries three
hooks worth knowing about:

- **`go_build_platform(target)`** — returns the GOOS/GOARCH/build tags/cgo of the
  *target* being analyzed, which the rule writes into the layout's `platform`
  block so arcc filters sources for the right platform instead of for whatever
  platform the arcc binary was built for. Under rules_go this reads
  `GoInfo.mode`; `GoSDK.goos` is the **execution** platform and would reproduce
  the bug this exists to fix. A host whose Go providers expose no platform
  metadata may return a fixed constant — that is a conforming implementation,
  not a degraded fallback.
- **`INFRA_COMPONENTS`** — a registry of components a toolchain injects into
  every target (an RPC or proto runtime, say), each entry naming a component
  target and optionally the import-path patterns of packages that cannot be named
  as labels. Empty upstream, because upstream injects nothing.
- **`go_attach_infra(target, infra)`** — whether to attach a registry entry to a
  given target. The rule adds each attached entry as a `component_dep` marked
  `auto_attached`. Returning `True` unconditionally is conforming: checking at a
  package boundary is a no-op unless that package is reached, and a component's own
  authority is charged regardless because its packages are roots. A host whose
  analysis-phase closure does not show toolchain-injected packages cannot
  evaluate a closure test at all.

The layout the rules emit is documented for emitter authors in
[the package-layout schema guide](docs/package-layout-schema.md) — required
reading before writing a second emitter, since two of its fields (`is_stdlib`
and the platform block) have an obvious implementation that is wrong.

`go_component` expands to two targets, plus a lazy analysis output group:

| Target | What it is |
|---|---|
| `csvfile_component` | generates `csvfile_component.component.textproto` + `csvfile_component.package-layout.json`, runs the checked analysis action, and forwards the interface library's Go providers, so it can be used as a `deps` entry |
| `csvfile_component.check` | a hermetic test that asserts the component's report verdict |
| `arcc` output group | `bazel build //csvtool/csvfile:csvfile_component --output_groups=+arcc` additionally produces `csvfile_component.report.json` (the check report, verdict included) and `csvfile_component.surface.json` (the canonical surface manifest) |

Ordinary builds never run component analysis: the report and surface ride in
the `arcc` output group, so `bazel build //...` stays analysis-free until a
consumer or test requests the artifacts. Testing a `.check` builds its
component's analysis action — and, through the action's declared inputs, the
direct dependencies' producer chain.

Enforce a component's contract by testing its `.check`:

```bash
bazel test //csvtool/csvfile:csvfile_component.check   # fails (exit ≠ 0) if the contract is violated
bazel test //csvtool/...                               # every component's .check at once
```

Because the check is sandboxed and cacheable, a green `bazel test` means the
contract holds with no reliance on the host Go toolchain. The authority
taxonomy is equally available from its language-neutral home,
`@rules_arcc//bazel_rules:authority.bzl`.

Worked BUILD files live under [`go/examples/csvtool/`](go/examples/csvtool/):
`parsecsv` (no declared authority), `toprow` (no authority), `csvfile` (`[FILES]`),
and `app` (its component dependencies provide the checked boundaries).

---

## CLI Reference and Exit Codes

The `arcc` tool provides a streamlined command-line interface for checking manifests:

```
arcc checks Go architectural component contracts.

Usage:
  arcc check <manifest> [--package-layout=<layout>] [--format=json]
        [--report-out=<path>] [--surface-out=<path>] [--stdlib-map=<artifact>]
        [--report-verdict-only]
  arcc verdict <report> --expect=pass|fail
  arcc stdlibmap generate --output=<path> [--toolchain=<version>] [--goos=<os>] [--goarch=<arch>] [--cgo] [--tags=<t1,t2>] [--goexperiment=<exp>]
  arcc stdlibmap inspect <artifact> [--expect-key=<field=value,...>] [summary | symbol <id> | init <pkg>]...
  arcc --version
```

### Check artifact emission

`arcc check` can publish the canonical artifacts Bazel and native workflows
consume, from one analysis invocation:

- `--report-out=<path>` writes the canonical report JSON
  (`<name>.report.json` shape): the completed report plus its explicitly
  derived verdict. Written atomically; identical inputs produce byte-identical
  artifacts.
- `--surface-out=<path>` writes the exact canonical surface JSON after the
  manifest/interface validation and member fact load have succeeded. The
  surface needs the target SDK identity, which comes from the declared
  Step 4 stdlib-map artifact:
  - **Layout/Bazel mode**: pass `--stdlib-map=<artifact>`; the declared map
    is validated (decode, format version, classifier hash, and — when the
    layout pins a platform — the target key) and its key becomes the
    surface's SDK identity. A missing, corrupt, format-mismatched,
    classifier-mismatched, or target-mismatched map fails closed with exit 2.
  - **Native mode**: without `--stdlib-map`, the target is discovered via the
    Step 4 services (`go env`) and the map is resolved from the on-demand
    cache (generated on a miss). Passing `--stdlib-map` in native mode uses
    the declared artifact instead, validated against the discovered native
    target — a map for another toolchain, GOOS, GOARCH, cgo state, or
    build-tag set is rejected with exit 2.
- `--report-verdict-only` requires `--report-out` and separates policy from
  execution for the Bazel analysis action: after analysis and artifact
  publication both a passing and a violating component exit 0 (the verdict is
  recorded in the report), while every usage, loading, analysis, key, or
  write error still exits 2. Without it, the exit codes below are unchanged.
- Artifact output is canonical and independent of `--format`: stdout display
  is preserved as usual.

Every check decision reads the map: standard-library membership and each
referenced symbol's classification come from the authority map, `UNANALYZED`
records surface as analysis-defeating findings, and dependency boundaries are
decided against the dependency's exact declaring-object interface. Check and
emitted surface share that same exact interface — there is no
implements-closure injection on either side. `--stdlib-map` is mandatory in
layout/Bazel mode; native mode resolves the map from the on-demand cache
whenever a check runs, not only when a surface is emitted.

Dependency status is deliberately three-dimensional. Canonical JSON preserves
`provenance` (`CHECKED_PASS`, `CHECKED_FAIL`, or `ASSERTED`), `freshness`
(`BUILD_GRAPH`, `VERIFIED`, `STALE`, or `UNKNOWN`), and `authority`
(`DECLARED` with its set or `UNKNOWN`) independently. Text reports render
`certified` only for a checked-pass, non-stale boundary; `check failed`,
`asserted`, `stale`, and `untrusted` remain visible when their individual axes
apply. A native dependency surface is conventionally staged with its report in
dependency order; do not copy generated artifacts into the repository or rely
on a developer's existing sibling files.


`arcc stdlibmap generate` produces the canonical standard-library authority
map for a target configuration: by default the current toolchain (`go env`
and `go list std`), or an explicit target (`--toolchain`, `--goos`,
`--goarch`, `--cgo`, `--tags`, `--goexperiment`) for cross-compilation and
the later Bazel rule. The output is written atomically; two runs over the
same configuration produce byte-identical artifacts. Native users can also
rely on the on-demand cache: it is keyed by a digest of the complete SDK key
under the user cache directory's `arcc` subtree, revalidates every hit
against the requested key, and regenerates corrupt or mismatched entries.

`arcc stdlibmap inspect` validates an artifact and answers deterministic
queries: the summary (with the full SDK key) when no query is given,
`symbol <id>` for a symbol's exact classification and ordered evidence, and
`init <pkg>` for a package's aggregate init classification. Unknown
packages/symbols and a `--expect-key` mismatch are tool errors.

### Exit Codes
The `arcc` command adheres to a deterministic three-way exit code structure:
- **`0`**: The component conforms with its architectural contract and does not exceed its declared authority (a component that declares none is ambient-authority-free). Warnings do not change the exit code.
- **`1`**: Architectural violations or non-conformance detected (e.g., undeclared imports, calls to non-interface boundary symbols, or undeclared ambient capabilities).
- **`2`**: Tool or execution error (e.g., manifest syntax error, Go build failure, or missing files).

### JSON Output
For programmatic consumption and integration into continuous integration pipelines, pass the `--format=json` flag:
```bash
arcc check path/to/component.textproto --format=json
```

---

## The CSV Tool Walkthrough

The repository includes a complete, realistic example of a multi-package command-line application (a tool to sort and extract top rows from a CSV) located under `go/examples/csvtool/`.

This example demonstrates how `arcc` tracks ambient authority, allows legitimate capabilities, and checks component boundaries:

1. **`parsecsv`** (Shared Utility): A small in-memory CSV parser, declared as a component of its own so that both `toprow` and `csvfile` can depend on it across a real boundary. It has no declared authority, and its use of `csv.Reader.ReadAll` is honestly reported as `UNANALYZED` by the current stdlib map.
2. **`toprow`** (Pure Logic): This component sorts and extracts top rows from in-memory CSV data, depending on the `parsecsv` component. It performs no file or network I/O; its manifest declares no authority, and checking it succeeds with `0`.
3. **`csvfile`** (High Authority): This component legitimately accesses the real filesystem to read files using `os.ReadFile`. It explicitly declares its requirement for `FILES` authority in its manifest. Checked on its own, it conforms.
4. **`app`** (Composition Root): This component calls `csvfile` to load data and `toprow` to sort it. Because both are first-class **component dependencies**, references to their exact declared interfaces are checked as component-boundary edges. The filesystem authority remains owned by `csvfile` and does not become part of `app`'s contract, so `app` checks as conformant and ambient-authority-free.

### Running the Examples
To verify these behaviors, execute the following checks from within the `go`
directory. The commands use a temporary copy of the CSV workspace because each
check stages its sibling surface/report artifacts beside the copied manifests;
the repository checkout remains free of generated files.

```bash
cd go

csv_demo_root="$(mktemp -d "${TMPDIR:-/tmp}/arcc-csvtool.XXXXXX")"
trap 'rm -rf -- "$csv_demo_root"' EXIT
cp go.mod "$csv_demo_root/go.mod"
mkdir -p "$csv_demo_root/examples"
cp -a examples/csvtool "$csv_demo_root/examples/csvtool"
arcc_bin="$(pwd)/../bin/arcc"

# Produce every sibling pair in dependency order before running consumers.
for component in internal/parsecsv csvfile toprow app; do
  manifest="$csv_demo_root/examples/csvtool/$component/component.textproto"
  base="${manifest%.textproto}"
  "$arcc_bin" check "$manifest" \
    --report-out="$base.report.json" \
    --surface-out="$base.surface.json" \
    --report-verdict-only >/dev/null
done

# 1. Check the shared parser (expected exit 1: its UNANALYZED use is reported)
"$arcc_bin" check "$csv_demo_root/examples/csvtool/internal/parsecsv/component.textproto" || test "$?" -eq 1

# 2. Check the pure sorting logic (conforms, authority-free)
"$arcc_bin" check "$csv_demo_root/examples/csvtool/toprow/component.textproto"

# 3. Check the file reader (conforms, uses declared FILES authority)
"$arcc_bin" check "$csv_demo_root/examples/csvtool/csvfile/component.textproto"

# 4. Check the composition root (conforms, authority-free through component boundaries)
"$arcc_bin" check "$csv_demo_root/examples/csvtool/app/component.textproto"
```

The first command currently exits `1`: `csv.Reader.ReadAll` is an honest
`UNANALYZED` result and remains a non-gating AC8b case until a later residual
policy task supplies its explicit treatment. The other three commands exit `0`. A component with dependencies
also lists each checked boundary:
```
Component "app" conforms; does not exceed declared authority

Dependencies:
- csvfile (asserted)
- toprow (asserted)
```

The listing is not a finding: it makes the component boundaries visible in every
report that rests on them.

### Reproducing the status demo

The native demo below uses its own temporary copy and stages all four CSV
components in topological order. The `--report-verdict-only` flag makes a
failing dependency report usable as an artifact while preserving its `fail`
verdict for downstream warning/status handling. Generated artifacts and the
source mutation are removed with the temporary workspace:

```bash
cd go
csv_demo_root="$(mktemp -d "${TMPDIR:-/tmp}/arcc-csvtool-status.XXXXXX")"
trap 'rm -rf -- "$csv_demo_root"' EXIT
cp go.mod "$csv_demo_root/go.mod"
mkdir -p "$csv_demo_root/examples"
cp -a examples/csvtool "$csv_demo_root/examples/csvtool"
arcc_bin="$(pwd)/../bin/arcc"

for component in \
  internal/parsecsv \
  csvfile \
  toprow \
  app; do
  manifest="$csv_demo_root/examples/csvtool/$component/component.textproto"
  base="${manifest%.textproto}"
  "$arcc_bin" check "$manifest" \
    --report-out="$base.report.json" \
    --surface-out="$base.surface.json" \
    --report-verdict-only >/dev/null
done

# Native convention lookup shows ASSERTED + VERIFIED for unchanged sources.
"$arcc_bin" check "$csv_demo_root/examples/csvtool/app/component.textproto"

# A readable source-byte change is reported as ASSERTED + STALE.
printf '\n// status-demo source change\n' >> "$csv_demo_root/examples/csvtool/csvfile/private.go"
"$arcc_bin" check "$csv_demo_root/examples/csvtool/app/component.textproto" --format=json
```

For build-graph status, build the same checked artifacts with Bazel:

```bash
bazel build //go/examples/csvtool/app:app_component --output_groups=+arcc
bazel build //go/examples/csvtool/csvfile:csvfile_component --output_groups=+arcc
cat bazel-bin/go/examples/csvtool/app/app_component.report.json
cat bazel-bin/go/examples/csvtool/csvfile/csvfile_component.report.json
```

The app report contains `CHECKED_PASS` boundaries (the `certified` text case).
`parsecsv` is tagged `manual` in the checked-in Bazel graph, so its provider is
an asserted package-surface dependency; the csvfile/toprow reports therefore
show the actual asserted parsecsv boundary rather than claiming `CHECKED_FAIL`.
The native JSON run supplies the `STALE` case. The reproducible `UNKNOWN` and
checked-fail cases use the same test-owned workspace strategy in
`TestIntegration_CSVTool_StatusDemo` (the latter uses a layout checked binding
to consume its exact report/surface pair), and the complete text rendering is
also asserted by `TestRunner_Check_CSVToolStatusLabelsInLayoutReport`.

### Reproducing a Conformance Violation
To see what a contract violation looks like, you can easily create a temporary failing component.

A component that uses ambient authority must declare it. The most direct failure is a component that calls `os.ReadFile` — which needs `FILES` — without declaring `FILES` in `declared_authority`.

1. Create a temporary folder and files inside the `go/examples/csvtool/` directory:
```bash
mkdir -p go/examples/csvtool/authorityapp
```

2. Save the following manifest as `go/examples/csvtool/authorityapp/component.textproto`. Notice that `declared_authority` is absent:
```textproto
name: "authorityapp"
interface_files: "main.go"
```

3. Save the following code as `go/examples/csvtool/authorityapp/main.go`:
```go
package main

import "os"

func Run() {
	// Calling os.ReadFile, which uses the FILES capability.
	// Since the manifest declares no authority, this is a violation.
	_, _ = os.ReadFile("test.csv")
}
```

4. Now, check this temporary component from inside the `go` directory:
```bash
cd go
../bin/arcc check examples/csvtool/authorityapp/component.textproto
```

The output will clearly list the `UNDECLARED_AUTHORITY` violation, show the exact call path where the capability was exercised, and return an exit code of `1`:

```
Component: authorityapp

Violations:
- [UNDECLARED_AUTHORITY] use of undeclared authority "FILES" in package "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/authorityapp"
  Evidence:
    - github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/authorityapp.Run at :0
    - os.ReadFile at main.go:8
```

Using the JSON format:
```bash
../bin/arcc check examples/csvtool/authorityapp/component.textproto --format=json
```

Yields:
```json
{
  "component": "authorityapp",
  "violations": [
    {
      "kind": "UNDECLARED_AUTHORITY",
      "message": "use of undeclared authority \"FILES\" in package \"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/authorityapp\"",
      "location": {
        "file": "",
        "line": 0
      },
      "evidence": [
        "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/authorityapp.Run at :0",
        "os.ReadFile at main.go:8"
      ]
    }
  ],
  "warnings": null
}
```

*Note: Be sure to delete the temporary `authorityapp` directory when finished checking.*

---

## Authoring a Component Manifest

A component's boundaries are described in a `component.textproto` manifest file, which must sit at the **component root**.

### Membership: declared, or directory-based by default
A component's **members** are the packages it owns: they are analyzed as roots
(every function in them is a capability start point) and their imports are
checked against the component's declarations.

There are two ways membership is determined:

- **`members` is empty (the default).** All Go packages located in directories
  recursively under the manifest file's directory belong to the component.
  Component roots must be disjoint: you cannot nest a component's root inside
  another's. This is what every hand-written manifest in this repository used
  before `members` existed, and it still works unchanged.
- **`members` is declared.** The listed import paths are the membership, and
  they may name packages **anywhere** — including outside the component root.
  The package holding the interface files is implicitly a member and need not be
  listed. Generated manifests always declare `members`.

A member may not also be covered by a `component_dependency`; that
contradiction is reported as `MEMBER_OVERLAP`. Overlap with an *unrelated*
component's membership is not detected — see
[Limitations](#limitations-and-scope).

Every member entry is a **literal import path**. Import-path patterns
(`example.com/logger/*`) are rejected at parse time for every interface style:
wildcard membership hides newly introduced transitive packages from review,
and the point of `members` is that a reader can see exactly what the component
owns.

### Schema Fields
The manifest structure is defined by the following fields:
- **`name`** (string): Sibling-unique logical name of the component.
- **`interface_files`** (repeated string): Paths to Go files relative to the component root that declare the public surface (functions, types, vars, constants, receiver types). Exported methods whose receiver type is declared in an interface file must also be defined in an interface file.
- **`members`** (repeated string, optional): Import paths of the packages this component owns and analyzes as roots. Empty means directory-based membership (above). Every entry must be a literal import path; glob metacharacters are rejected at parse time.
- **`interface_style`** (enum, optional): `INTERFACE_STYLE_UNSPECIFIED` (the default) means `interface_files` declare the surface. `INTERFACE_STYLE_PACKAGE_SURFACE` means the interface is *every exported symbol of every member* — see below.
- **`component_dependencies`** (repeated): Dependencies on other first-class components.
  - `name` (string): Logical name of the dependent component.
  - `manifest` (string): Path to the dependent component's manifest, relative to the declaring manifest's folder.
  - `auto_attached` (bool, optional): The edge was injected by an emitter rather than written by an author, so an unused edge is nobody's mistake and never produces `UNUSED_DEPENDENCY`. See [Auto-attached dependencies](#auto-attached-infrastructure-dependencies).
- **`declared_authority`** (repeated string): Capabilities that the component is permitted to exercise. Member typed references and imports are checked against the total standard-library authority map, and declared component boundaries terminate authority structurally. Leaving this empty means the component claims no ambient authority.
  - Known Capabilities: `FILES`, `NETWORK`, `READ_SYSTEM_STATE`, `MODIFY_SYSTEM_STATE`, `OPERATING_SYSTEM`, `SYSTEM_CALLS`, `EXEC`, `RUNTIME`, `ARBITRARY_EXECUTION`, `CGO`, `UNSAFE_POINTER`, `REFLECT`, `UNANALYZED`.
- **`authority`** (enum, optional): `DECLARED` (the default) means the
  component's `declared_authority` is checked; `UNKNOWN` marks a package-level
  adopted surface whose authority has not been analysed. `UNKNOWN` must have an
  empty `declared_authority` and is rendered `untrusted` at dependent boundaries.

### Wrapping a library that has no interface: `PACKAGE_SURFACE`

Some code was never written to have an architectural interface — a
toolchain-injected runtime, or an existing library you want to draw a boundary
around without editing it. Declaring such a component with
`interface_style: INTERFACE_STYLE_PACKAGE_SURFACE` says "the interface is the
whole exported surface of my members":

```textproto
name: "logger"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/common/logger"
members: "example.com/common/logger/backends"
declared_authority: "FILES"
declared_authority: "NETWORK"
```

Consequences, all local to the component:

- `interface_files` must be **empty**, and `members` must be non-empty. Both
  directions are validation errors, so the surface has one source of truth.
  - Dependents accept references at **package** granularity rather than against
    declared interface symbols, and `CALLS_UNDECLARED_INTERFACE` becomes vacuous
    for them — the honest reading of "the whole surface is the interface".
    `UNUSED_DEPENDENCY` still applies, computed against the full exported surface.
- FR4 placement rules (`METHOD_OUTSIDE_INTERFACE`) do not apply.
- The component's own check still runs and still surfaces its authority. That is
  the point of certifying a wrapper rather than trusting a list.

Package-granularity boundaries are coarser than symbol boundaries and can hide
authority reached through unexported entry points — see
[Limitations](#limitations-and-scope).

### Auto-attached infrastructure dependencies

A `component_dependency` marked `auto_attached: true` was injected by an emitter
(a build-system toolchain that puts a runtime into every target's closure), not
written by an author. It is exempt from `UNUSED_DEPENDENCY`, because an author
who never asked for the edge should not be told to remove it. Everything else
about the boundary is normal.

The edge appears in the emitted manifest even when the emitter attaches it
unconditionally: the whole argument for a flag over a hidden allowlist is that a
reader can see which boundaries were injected.

### Complete Example Manifest
Below is a valid, comprehensive `component.textproto` example matching the schema:

```textproto
name: "my_component"

# Relative paths to Go files containing public API declarations
interface_files: "api.go"
interface_files: "types.go"

# Optional: the packages this component owns. Omit the field entirely to fall
# back to directory-based membership. The interface package is implicit.
members: "example.com/my_component/internal/store"

# First-class dependent components that have their own manifests and define boundaries
component_dependencies {
  name: "db_driver"
  manifest: "../db_driver/component.textproto"
}

# Permitted ambient capabilities (validated at parse time)
declared_authority: "FILES"
declared_authority: "SYSTEM_CALLS"
```

---

## Limitations and Scope

The MVP implementation makes several engineering trade-offs and has known boundary conditions. They are listed here rather than left in the design notes, because most of them are costs an author feels while using the tool.

### Analysis scope and precision

1. **Reference precision:** Boundary analysis follows the objects resolved by the
   Go AST and `types.Info` reference scanner. Dynamic dispatch and reflection that
   do not produce a resolvable typed reference can remain outside the checked
   vocabulary; analysis-defeating source constructs fail closed instead.
2. **Compositional / single-component checking:** Component boundaries depend on
   the honesty of dependencies' manifests and their checked surfaces. Security
   guarantees therefore hold only if *every* component in the system is
   independently checked and conforms.
3. **Generic symbol matching:** Generic type-parameter brackets are simplified
   when canonical symbol identities are matched, which is conservative but can
   be loose for complex edge cases.
4. **Go-only scope:** The current implementation supports Go codebases only
   (layout is split under `go/` and schemas under `proto/` to permit future
   language extensions).
5. **One platform per check.** `declared_authority` is platform-agnostic, but a
   check verifies exactly one platform (the layout's `platform` block, or
   `build.Default` natively). Authority exercised only in a `_windows.go` file is
   invisible on a Linux check.
6. **Bodiless packages cannot be analyzed.** A package whose function bodies are
   unavailable cannot be a member — the loader errors, naming it rather than
   letting silence look like cleanliness. Nothing forces its authority to be
   declared.

### Membership and boundaries

8. **Cross-component membership overlap is undetected.** `MEMBER_OVERLAP` catches a member that is also covered *by the same component's own declarations*. Two unrelated components both claiming the same package is not detected — that needs a repo-wide uniqueness check, which does not exist yet.
9. **A component's BUILD file changes when its implementation is restructured.** A declared-style component's `members` are explicit labels, so adding, removing or renaming an internal package edits the component declaration. This works against a goal the component model exists to serve — implementation changes should be reviewable with little attention *because* interface changes are the ones that surface — since an internal-only refactor now shows up as a diff to the component declaration. Bazel offers no mechanism that closes this: a package cannot enumerate packages below its immediate children, so "everything under here" is not expressible in one place (which is also why there is no wildcard helper). Partly mitigated: a member you *forget* to declare, but that owned code actually imports, is fail-closed — it lands in the closure, matches nothing, and is reported as `UNDECLARED_DEPENDENCY` naming the package to add. The silent residue is a nested package nothing imports, i.e. dead code, which stays unowned.
10. **`PACKAGE_SURFACE` uses package-granularity boundaries,** which are coarser than symbol boundaries and can hide authority reached through unexported entry points. This was accepted knowingly for toolchain-injected runtimes; note that it now applies wherever an author chooses that style to wrap an existing library, which is the common case.
11. **A `PACKAGE_SURFACE` component can launder authority.** Wrapping a large library and declaring the union of what it needs stops charging that authority to every caller — which is the value of drawing the boundary, and also the risk. The safeguard is that the wrapper's own check reports its actual authority; nothing prevents an author from declaring it and moving on. Review of `declared_authority` is the control, as it is for any component.

### Bazel-specific

12. **A component whose closure contains a cgo package cannot be checked under Bazel.** A cgo package compiles from preprocessed sources that do not exist at analysis time, so the rule fails closed rather than emitting a layout naming files that will not be in the sandbox. **The exclusion propagates upward through importers:** a cgo package anywhere in a closure excludes every component above it, not merely the one that names it. Native mode is unaffected, because the go tool preprocesses cgo before `go/packages` sees it, so such a component keeps full native coverage and loses only its Bazel leg. This is why `bazel test //...` runs **six** self-checks while `just selfcheck` runs **eight**: `capslockadapter` owns capslock as member code, whose closure contains `golang.org/x/sys/unix` built with cgo, and `cli` imports `capslockadapter`. One root cause, two components. The workaround of patching a third-party build file to claim the package is not cgo is deliberately not taken: buying a green check by falsifying build metadata is the exact failure mode these checks exist to remove.

---

## Development and Contributing

Developers contributing to the Architectural Contracts project can use the provided tooling to ensure clean commits.

### Run the pipeline
Make sure everything remains green before committing changes. If you have `protoc` and `protoc-gen-go` installed, run:
```bash
just ci
```
This runs lints, verifies that generated protobuf files are clean (using `protoc` and `protoc-gen-go`), compiles the binaries, executes unit and integration tests, and runs the selfcheck.

If you do not have `protoc` or `protoc-gen-go` installed, you can run all verification checks except the protobuf generation check using:
```bash
just lint build test test-integration selfcheck
```

### Self-Hosting Verification
The tool is built recursively out of components and is checked against itself:
```bash
just selfcheck
```
This compiles the local `arcc` binary and runs it against each of its own eight components' manifests. The two groups prove different things, which is why the recipe separates them:

- **The authority-free core** — `checker`, `facts`, `report`, `capanalyzer` — declares *no* ambient authority at all. Their checks confirm the pure checking core genuinely remains ambient-authority-free.
- **The remaining components** — `manifest`, `goanalysis`, `capslockadapter`, `cli` — legitimately declare authority (`goanalysis` declares seven kinds, `capslockadapter` eight). Their checks confirm something weaker and equally important: that each *does not exceed* what it declares.

Conflating the two would overclaim. A conforming component is not an authority-free one; it is one that stayed inside its declaration.

The same components are also declared as `go_component` targets, so `bazel test //...` checks them hermetically — but only **six** of the eight: `capslockadapter` and `cli` are excluded because their closures contain a cgo package, which the Bazel rule refuses (limitation 13 above). Keeping both legs is deliberate rather than redundant: the native leg derives membership from directories (FR1) and the Bazel leg from declared `members`, so the two checking the same components cross-checks the membership model itself. A `self_manifest_parity_test` additionally asserts that the generated manifests and the checked-in ones agree, so the two forms cannot drift.
