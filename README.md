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

- **`interface`** — the single `go_library` holding the component's public API. Passing a list is a load-time error.
- **`component_deps`** — other `go_component` targets this one depends on. Their packages are covered by them, so arcc prunes authority at their interfaces (the `app` example checks authority-free this way).
- **`absorbed_deps`** — libraries absorbed as implementation details, whose ambient authority this component takes responsibility for. (Use a BUILD comment where a reason is worth noting; the rule records none.)
- **`declared_authority`** — authority constants from `defs.bzl` (`FILES`, `NETWORK`, …). Empty means the component claims to be authority-free.
- **`contract`** — optional contract documents; Bazel-only metadata arcc never reads.

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
`toprow` (no authority), `csvfile` (`[FILES]`), and `app` (authority pruned by
its component dependencies).

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
- **`0`**: The component is fully conformant with its architectural contract and is either ambient-authority-free or only uses declared capabilities.
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

1. **`toprow`** (Pure Logic): This component parses and sorts in-memory CSV data. It depends on an absorbed parser package, but performs no file or network I/O. Its manifest declares no authority, and checking it succeeds with `0` (ambient-authority-free).
2. **`csvfile`** (High Authority): This component legitimately accesses the real filesystem to read files using `os.ReadFile`. It explicitly declares its requirement for `FILES` authority in its manifest. Checked on its own, it conforms.
3. **`app`** (Composition Root): This component calls `csvfile` to load data and `toprow` to sort it. 
   - **Boundary Pruning (FR5b) in action:** Because `app` depends on `csvfile` as a first-class **component dependency**, `arcc`'s capability analysis is pruned at `csvfile`'s declared public interface. Even though `app` orchestrates a file-reading component, the filesystem authority is owned by `csvfile` and does not bleed into `app`'s contract. Therefore, `app` checks as fully conformant and ambient-authority-free!

### Running the Conforming Examples
To verify these behaviors, execute the following checks from within the `go` directory:

```bash
cd go

# 1. Check the pure sorting logic (conforms, authority-free)
../bin/arcc check examples/csvtool/toprow/component.textproto

# 2. Check the file reader (conforms, uses declared FILES authority)
../bin/arcc check examples/csvtool/csvfile/component.textproto

# 3. Check the composition root (conforms, authority-free via pruning)
../bin/arcc check examples/csvtool/app/component.textproto
```

For all three commands, you will see a conforming output and an exit code of `0`:
```
Component "<name>" conforms / ambient-authority-free
```

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

### Directory-Based Membership
All Go packages located in directories recursively under the manifest file's directory automatically belong to the component. Component roots must be disjoint: you cannot nest a component's root inside another's.

### Schema Fields
The manifest structure is defined by the following fields:
- **`name`** (string): Sibling-unique logical name of the component.
- **`interface_files`** (repeated string): Paths to Go files relative to the component root that declare the public surface (functions, types, vars, constants, receiver types). Exported methods whose receiver type is declared in an interface file must also be defined in an interface file.
- **`component_dependencies`** (repeated): Dependencies on other first-class components.
  - `name` (string): Logical name of the dependent component.
  - `manifest` (string): Path to the dependent component's manifest, relative to the declaring manifest's folder.
- **`absorbed_dependencies`** (repeated): Third-party or internal implementation-detail Go packages whose capabilities are transitively absorbed into this component's policy scope.
  - `import_path` (string): Fully qualified import path (e.g. `github.com/foo/bar`).
  - `reason` (string, optional): Prose explanation for why this is treated as an implementation detail.
- **`declared_authority`** (repeated string): Capabilities from Capslock's classified set that this component is permitted to exercise. Leaving this empty makes the component ambient-authority-free.
  - Known Capabilities: `FILES`, `NETWORK`, `READ_SYSTEM_STATE`, `MODIFY_SYSTEM_STATE`, `OPERATING_SYSTEM`, `SYSTEM_CALLS`, `EXEC`, `RUNTIME`, `ARBITRARY_EXECUTION`, `CGO`, `UNSAFE_POINTER`, `REFLECT`, `UNANALYZED`.

### Complete Example Manifest
Below is a valid, comprehensive `component.textproto` example matching the schema:

```textproto
name: "my_component"

# Relative paths to Go files containing public API declarations
interface_files: "api.go"
interface_files: "types.go"

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

The MVP implementation makes several engineering trade-offs and has known boundary conditions:

1. **Call-Graph Precision (VTA Over-approximation):** Boundary analysis relies on static call graph analysis (Variable Type Analysis - VTA) matching Capslock. Because dynamic dispatch and reflection cause over-approximation of call edges, the analyzer can occasionally register call paths that are unreachable at runtime, potentially leading to false-positive violations.
2. **Higher-Order Boundary Warnings (Leak):** When a component passes a function value or callback across a pruned boundary, the authority exercised when that callback is invoked might not attribute correctly to its source. The tool mitigates this by flagging a `HIGHER_ORDER_BOUNDARY_CALL` warning on func-valued arguments.
3. **Call-Edge-Only Enforcement:** Only call edges are checked at boundaries. Reading exported struct fields, types, or accessing package-level variables across boundaries is outside the scope of MVP check coverage.
4. **Compositional / Single-Component Checking:** Pruning depends on the honesty of dependencies' manifests. Therefore, security guarantees only hold if *every* component in the system is independently checked and conforms.
5. **Generic Symbol Matching:** Generic SSA type parameter brackets are simplified for matching, which is conservative but can sometimes lead to loose checks (failing open on complex edge cases).
6. **Go-only Scope:** The current implementation supports Go codebases only (layout is split under `go/` and schemas under `proto/` to permit future language extensions).

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
This compiles the local `arcc` binary and runs it against each of its own components' manifests (`checker`, `facts`, `report`, `capanalyzer`, `manifest`, `goanalysis`, `capslockadapter`, and `cli`). This confirms the pure checking core remains entirely ambient-authority-free.
