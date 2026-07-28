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
    absorbed_deps = ["//csvtool/internal/parsecsv"],  # implementation details this component owns
    declared_authority = [FILES],                 # ambient authority it is permitted to use
    visibility = ["//visibility:public"],
)
```

The attributes mirror the manifest schema below:

- **`interface`** — the single `go_library` holding the component's public API. Passing a list is a load-time error. Required for the default (declared) style; rejected under `PACKAGE_SURFACE`.
- **`members`** — additional `go_library` labels the component owns. The rule resolves each to its import path and writes the **fully expanded literal list** into the manifest's `members`, which is also the layout's `roots`. Member packages are analysis roots, so their authority is charged to this component whoever calls them, and their imports are checked against the component's declarations.
- **`component_deps`** — other `go_component` targets this one depends on. Their packages are covered by them, so arcc prunes authority at their interfaces (the `app` example checks authority-free this way).
- **`absorbed_deps`** — libraries absorbed as implementation details, whose ambient authority this component takes responsibility for. (Use a BUILD comment where a reason is worth noting; the rule records none.)
- **`interface_style`** — omit for the declared style, or pass `PACKAGE_SURFACE` (exported by `defs.bzl`) to wrap a library that has no architectural interface. Under `PACKAGE_SURFACE`, `interface` must be absent and `members` non-empty, and members may be import-path patterns rather than labels — which is how a component covers packages that visibility rules make impossible to name as targets.
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
    absorbed_deps = ["@org_golang_x_crypto//sha3"],
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
  `auto_attached`. Returning `True` unconditionally is conforming: pruning at a
  package is a no-op unless that package is reached, and a component's own
  authority is charged regardless because its packages are roots. A host whose
  analysis-phase closure does not show toolchain-injected packages cannot
  evaluate a closure test at all.

The layout the rules emit is documented for emitter authors in
[the package-layout schema guide](docs/package-layout-schema.md) — required
reading before writing a second emitter, since two of its fields (`is_stdlib`
and the platform block) have an obvious implementation that is wrong.

`go_component` expands to two targets:

| Target | What it is |
|---|---|
| `csvfile_component` | generates `csvfile_component.component.textproto` + `csvfile_component.package-layout.json`, and forwards the interface library's Go providers, so it can be used as a `deps` entry |
| `csvfile_component.check` | a hermetic test that runs `arcc check` on the generated manifest |

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
`parsecsv` (no authority), `toprow` (no authority), `csvfile` (`[FILES]`), and
`app` (authority pruned by its component dependencies).

---

## CLI Reference and Exit Codes

The `arcc` tool provides a streamlined command-line interface for checking manifests:

```
arcc checks Go architectural component contracts.

Usage:
  arcc check <manifest>
  arcc --version
```

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

This example demonstrates how `arcc` tracks ambient authority, allows legitimate capabilities, and prunes analysis boundaries:

1. **`parsecsv`** (Shared Utility): A small in-memory CSV parser, declared as a component of its own so that both `toprow` and `csvfile` can depend on it across a real boundary. No declared authority.
2. **`toprow`** (Pure Logic): This component sorts and extracts top rows from in-memory CSV data, depending on the `parsecsv` component. It performs no file or network I/O; its manifest declares no authority, and checking it succeeds with `0`.
3. **`csvfile`** (High Authority): This component legitimately accesses the real filesystem to read files using `os.ReadFile`. It explicitly declares its requirement for `FILES` authority in its manifest. Checked on its own, it conforms.
4. **`app`** (Composition Root): This component calls `csvfile` to load data and `toprow` to sort it. 
   - **Boundary Pruning (FR5b) in action:** Because `app` depends on `csvfile` as a first-class **component dependency**, `arcc`'s capability analysis is pruned at `csvfile`'s declared public interface. Even though `app` orchestrates a file-reading component, the filesystem authority is owned by `csvfile` and does not bleed into `app`'s contract. Therefore, `app` checks as fully conformant and ambient-authority-free!

### Running the Conforming Examples
To verify these behaviors, execute the following checks from within the `go` directory:

```bash
cd go

# 1. Check the shared parser (conforms, authority-free)
../bin/arcc check examples/csvtool/internal/parsecsv/component.textproto

# 2. Check the pure sorting logic (conforms, authority-free)
../bin/arcc check examples/csvtool/toprow/component.textproto

# 3. Check the file reader (conforms, uses declared FILES authority)
../bin/arcc check examples/csvtool/csvfile/component.textproto

# 4. Check the composition root (conforms, authority-free via pruning)
../bin/arcc check examples/csvtool/app/component.textproto
```

For all four commands, you will see a conforming output and an exit code of `0`.
A component with dependencies also lists each pruned boundary, annotated
`certified` when the dependency declares that its own check runs and `asserted`
when it does not:
```
Component "app" conforms; does not exceed declared authority

Dependencies:
- csvfile: asserted
- toprow: asserted
```

The annotation is not a finding: the depending component did nothing wrong by
pruning at a boundary whose owner has not declared a check. It exists so that
what is being trusted is visible in every report that rests on it.

### Reproducing a Conformance Violation
To see what a contract violation looks like, you can easily create a temporary failing component.

While `app` successfully prunes authority by declaring `csvfile` as a `component_dependency`, we can see what happens when we *absorb* it instead. Absorbing a package transitively pulls its code and capabilities into the absorbing component's own contract boundaries.

1. Create a temporary folder and files inside the `go/examples/csvtool/` directory:
```bash
mkdir -p go/examples/csvtool/absorbapp
```

2. Save the following manifest as `go/examples/csvtool/absorbapp/component.textproto`. Notice we list `csvfile` under `absorbed_dependencies` instead of `component_dependencies`:
```textproto
name: "absorbapp"
interface_files: "main.go"
absorbed_dependencies {
  import_path: "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile"
  reason: "Absorbing csvfile instead of declaring it as a component dependency"
}
```

3. Save the following code as `go/examples/csvtool/absorbapp/main.go`:
```go
package main

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile"
)

func Run() {
	// Calling csvfile.Read, which internally uses the FILES capability.
	// Since csvfile is absorbed, its authority requirements bleed into ours.
	_, _ = csvfile.Read("test.csv")
}
```

4. Now, check this temporary component from inside the `go` directory:
```bash
cd go
../bin/arcc check examples/csvtool/absorbapp/component.textproto
```

The output will clearly list the `UNDECLARED_AUTHORITY` violation, show the exact call path where the capability was transitively minted, and return an exit code of `1`:

```
Component: absorbapp

Violations:
- [UNDECLARED_AUTHORITY] use of undeclared authority "FILES" in package "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/absorbapp"
  Evidence:
    - github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/absorbapp.Run at :0
    - github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile.Read at main.go:10
    - os.ReadFile at csvfile.go:22
```

Using the JSON format:
```bash
../bin/arcc check examples/csvtool/absorbapp/component.textproto --format=json
```

Yields:
```json
{
  "component": "absorbapp",
  "violations": [
    {
      "kind": "UNDECLARED_AUTHORITY",
      "message": "use of undeclared authority \"FILES\" in package \"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/absorbapp\"",
      "location": {
        "file": "",
        "line": 0
      },
      "evidence": [
        "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/absorbapp.Run at :0",
        "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile.Read at main.go:10",
        "os.ReadFile at csvfile.go:22"
      ]
    }
  ],
  "warnings": null
}
```

*Note: Be sure to delete the temporary `absorbapp` directory when finished checking.*

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

A member may not also be covered by a `component_dependency` or listed in
`absorbed_dependencies`; that contradiction is reported as `MEMBER_OVERLAP`.
Overlap with an *unrelated* component's membership is not detected — see
[Limitations](#limitations-and-scope).

Declared-style components must list **literal import paths**. Import-path
patterns (`example.com/logger/*`) are accepted only under
`interface_style: INTERFACE_STYLE_PACKAGE_SURFACE`, where membership only
derives prune keys and an exported-symbol set. Everywhere else the point of
`members` is that a reader can see exactly what the component owns, which a
pattern defeats.

### Schema Fields
The manifest structure is defined by the following fields:
- **`name`** (string): Sibling-unique logical name of the component.
- **`interface_files`** (repeated string): Paths to Go files relative to the component root that declare the public surface (functions, types, vars, constants, receiver types). Exported methods whose receiver type is declared in an interface file must also be defined in an interface file.
- **`members`** (repeated string, optional): Import paths of the packages this component owns and analyzes as roots. Empty means directory-based membership (above). Patterns are allowed only under `INTERFACE_STYLE_PACKAGE_SURFACE`.
- **`interface_style`** (enum, optional): `INTERFACE_STYLE_UNSPECIFIED` (the default) means `interface_files` declare the surface. `INTERFACE_STYLE_PACKAGE_SURFACE` means the interface is *every exported symbol of every member* — see below.
- **`component_dependencies`** (repeated): Dependencies on other first-class components.
  - `name` (string): Logical name of the dependent component.
  - `manifest` (string): Path to the dependent component's manifest, relative to the declaring manifest's folder.
  - `auto_attached` (bool, optional): The edge was injected by an emitter rather than written by an author, so an unused edge is nobody's mistake and never produces `UNUSED_DEPENDENCY`. See [Auto-attached dependencies](#auto-attached-infrastructure-dependencies).
- **`absorbed_dependencies`** (repeated): Third-party or internal implementation-detail Go packages whose capabilities are transitively absorbed into this component's policy scope.
  - `import_path` (string): Fully qualified import path (e.g. `github.com/foo/bar`).
  - `reason` (string, optional): Prose explanation for why this is treated as an implementation detail.
- **`declared_authority`** (repeated string): Capabilities from Capslock's classified set that this component is permitted to exercise. Leaving this empty makes the component ambient-authority-free.
  - Known Capabilities: `FILES`, `NETWORK`, `READ_SYSTEM_STATE`, `MODIFY_SYSTEM_STATE`, `OPERATING_SYSTEM`, `SYSTEM_CALLS`, `EXEC`, `RUNTIME`, `ARBITRARY_EXECUTION`, `CGO`, `UNSAFE_POINTER`, `REFLECT`, `UNANALYZED`.
- **`own_check_runs`** (bool, optional) and **`certification_reference`** (string, optional): Whether this component's own conformance check runs as part of the build, and — when it does not — where its conformance is established instead (a scheduled job, a run record, a document). Both are **self-declarations at the same trust level as `declared_authority`**: arcc does not verify them. They drive the `certified` / `asserted` annotation each pruned boundary gets in the report's dependency listing, so that trusting a dependency's contract is visible rather than invisible.

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
- Dependents prune at **package** granularity rather than at declared interface
  symbols, and `CALLS_UNDECLARED_INTERFACE` becomes vacuous for them — the
  honest reading of "the whole surface is the interface". `UNUSED_DEPENDENCY`
  still applies, computed against the full exported surface.
- FR4 placement rules (`METHOD_OUTSIDE_INTERFACE`) do not apply.
- The component's own check still runs and still surfaces its authority. That is
  the point of certifying a wrapper rather than trusting a list.

Package-granularity pruning is coarser than symbol pruning and will hide
authority reached through unexported entry points — see
[Limitations](#limitations-and-scope).

### Auto-attached infrastructure dependencies

A `component_dependency` marked `auto_attached: true` was injected by an emitter
(a build-system toolchain that puts a runtime into every target's closure), not
written by an author. It is exempt from `UNUSED_DEPENDENCY`, because an author
who never asked for the edge should not be told to remove it. Everything else
about the boundary is normal, including the certified/asserted annotation.

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

# First-class dependent components that have their own manifests and prune authority
component_dependencies {
  name: "db_driver"
  manifest: "../db_driver/component.textproto"
}

# Low-level package dependencies that are absorbed into our scope
absorbed_dependencies {
  import_path: "golang.org/x/crypto/sha3"
  reason: "Cryptographic hashing implementation detail"
}

# Permitted ambient capabilities (validated at parse time)
declared_authority: "FILES"
declared_authority: "SYSTEM_CALLS"
```

---

## Limitations and Scope

The MVP implementation makes several engineering trade-offs and has known boundary conditions. They are listed here rather than left in the design notes, because most of them are costs an author feels while using the tool.

### Analysis scope and precision

1. **Call-Graph Precision (VTA Over-approximation):** Boundary analysis relies on static call graph analysis (Variable Type Analysis - VTA) matching Capslock. Because dynamic dispatch and reflection cause over-approximation of call edges, the analyzer can occasionally register call paths that are unreachable at runtime, potentially leading to false-positive violations.
2. **Absorbed Function-Value Escapes (`ABSORBED_FUNC_VALUE_ESCAPE`):** Owned member code is analyzed in full as analysis roots, so callback bodies defined in member packages attribute their authority directly to the component. However, when member code takes the value of a function defined in an *absorbed* package and passes it across a boundary without calling it directly, no analysis covers what that absorbed function body does when invoked elsewhere. The tool detects this shape and reports an `ABSORBED_FUNC_VALUE_ESCAPE` warning to make the residual gap visible, rather than proving the absorbed function body safe.
3. **Call-Edge-Only Enforcement:** Only call edges are checked at boundaries. Reading exported struct fields, types, or accessing package-level variables across boundaries is outside the scope of MVP check coverage.
4. **Compositional / Single-Component Checking:** Pruning depends on the honesty of dependencies' manifests. Therefore, security guarantees only hold if *every* component in the system is independently checked and conforms.
5. **Generic Symbol Matching:** Generic SSA type parameter brackets are simplified for matching, which is conservative but can sometimes lead to loose checks (failing open on complex edge cases).
6. **Go-only Scope:** The current implementation supports Go codebases only (layout is split under `go/` and schemas under `proto/` to permit future language extensions).
7. **One platform per check.** `declared_authority` is platform-agnostic, but a check verifies exactly one platform (the layout's `platform` block, or `build.Default` natively). Authority exercised only in a `_windows.go` file is invisible on a Linux check.
8. **Bodiless packages cannot be analyzed.** A package whose function bodies are unavailable cannot be a member — the loader errors, naming it — and absorbing it attributes nothing, so an `ANALYSIS_LIMITATION` warning names it instead of letting silence look like cleanliness. Nothing forces its authority to be declared.

### Membership and boundaries

9. **Absorbed code remains use-attributed.** Authority inside an absorbed dependency is charged to the absorbing component only where owned code reaches it. Authority in absorbed code that nothing owned reaches is not charged at all. Limitation 2 warns where that is most likely to matter; it does not close it.
10. **Cross-component membership overlap is undetected.** `MEMBER_OVERLAP` catches a member that is also covered or absorbed *by the same component's own declarations*. Two unrelated components both claiming the same package is not detected — that needs a repo-wide uniqueness check, which does not exist yet.
11. **A component's BUILD file changes when its implementation is restructured.** A declared-style component's `members` are explicit labels, so adding, removing or renaming an internal package edits the component declaration. This works against a goal the component model exists to serve — implementation changes should be reviewable with little attention *because* interface changes are the ones that surface — since an internal-only refactor now shows up as a diff to the component declaration. Bazel offers no mechanism that closes this: a package cannot enumerate packages below its immediate children, so "everything under here" is not expressible in one place (which is also why there is no wildcard helper). Partly mitigated: a member you *forget* to declare, but that owned code actually imports, is fail-closed — it lands in the closure, matches nothing, and is reported as `UNDECLARED_DEPENDENCY` naming the package to add. The silent residue is a nested package nothing imports, i.e. dead code, which stays unowned.
12. **`PACKAGE_SURFACE` prunes at package granularity,** which is coarser than symbol pruning and will hide authority reached through unexported entry points. This was accepted knowingly for toolchain-injected runtimes; note that it now applies wherever an author chooses that style to wrap an existing library, which is the common case.
13. **A `PACKAGE_SURFACE` component can launder authority.** Wrapping a large library and declaring the union of what it needs stops charging that authority to every caller — which is the value of drawing the boundary, and also the risk. The safeguard is that the wrapper's own check reports its actual authority; nothing prevents an author from declaring it and moving on. Review of `declared_authority` is the control, as it is for any component.
14. **An asserted boundary is not a verified one.** A component whose membership is patterns rather than targets has no check of its own, so its `declared_authority` is a reviewable assertion rather than a maintained claim. The `certified`/`asserted` annotation makes this visible in every report that depends on it rather than closing it; repo-wide enforcement is deferred with the uniqueness check above.

### Bazel-specific

15. **A component whose closure contains a cgo package cannot be checked under Bazel.** A cgo package compiles from preprocessed sources that do not exist at analysis time, so the rule fails closed rather than emitting a layout naming files that will not be in the sandbox. **The exclusion propagates upward through importers:** a cgo package anywhere in a closure excludes every component above it, not merely the one that names it. Native mode is unaffected, because the go tool preprocesses cgo before `go/packages` sees it, so such a component keeps full native coverage and loses only its Bazel leg. This is why `bazel test //...` runs **six** self-checks while `just selfcheck` runs **eight**: `capslockadapter` absorbs capslock, whose closure contains `golang.org/x/sys/unix` built with cgo, and `cli` imports `capslockadapter`. One root cause, two components. The workaround of patching a third-party build file to claim the package is not cgo is deliberately not taken: buying a green check by falsifying build metadata is the exact failure mode these checks exist to remove.

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
This compiles the local `arcc` binary and runs it against each of its own eight components' manifests (`checker`, `facts`, `report`, `capanalyzer`, `manifest`, `goanalysis`, `capslockadapter`, and `cli`). This confirms the pure checking core remains entirely ambient-authority-free.

The same components are also declared as `go_component` targets, so `bazel test //...` checks them hermetically — but only **six** of the eight: `capslockadapter` and `cli` are excluded because their closures contain a cgo package, which the Bazel rule refuses (limitation 15 above). Keeping both legs is deliberate rather than redundant: the native leg derives membership from directories (FR1) and the Bazel leg from declared `members`, so the two checking the same components cross-checks the membership model itself. A `self_manifest_parity_test` additionally asserts that the generated manifests and the checked-in ones agree, so the two forms cannot drift.
