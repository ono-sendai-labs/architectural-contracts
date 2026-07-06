# Detailed Design — Architectural Contracts MVP (Go)

> **Status: revised after design review.** All major requirements decisions are
> user-confirmed: Capslock-as-library (Q1), component = package-pattern set (Q2),
> **strict capability policy by default, parameterized** (Q3), **textproto**
> manifests (Q4), and **`./go` as its own Go module** (Q5). Component→component
> boundary enforcement and capability pruning (FR5/FR5b) are designed in per the
> Round-3 directive. A senior design review (see `../design-review.md`) was
> verified against the Capslock source and its resolutions are folded in:
> the interface **closure rule** (A1), **init pruning + explicit-init rule** (A2),
> the **higher-order boundary warning** (A3), key **normalization** (A4),
> **VTA for both graphs** (A5), the pure **`facts`** component (B7), JSON
> rendering moved to the shell (B6), and assorted spec-gap fixes (C9–C14).
> This document is standalone; it does not require reading the other project files.

## 1. Overview

This project prototypes the "architecture as code" idea from
`rationale-and-concepts.md`: a component's **structure** (its interface, its
private implementation, and its dependencies) and its **ambient authority** are
recorded in a small, machine-readable **manifest**, and a **conformance checker**
mechanically verifies that the Go code matches the manifest. When the structure
of the code changes in a way that violates the manifest, the check fails — forcing
the architectural change to surface as an explicit edit to the manifest.

The MVP implements **Pillar 1 (Architecture as code)** and **Pillar 3 (Ambient
authority)** for **Go**, with authority constrained to the strongest case:
**ambient-authority-free** components (they declare — and provably use — no ambient
authority). Contracts (Pillar 2) are informal prose in interface-file comments and
are *not* mechanically checked. Data-flow (Pillar 4) is out of scope.

The deliverable is a **CLI** you run against a component manifest; it reports
whether the code conforms. The tool is itself decomposed into components with
manifests (self-hosting), and ships with example Go projects that demonstrate both
conforming and non-conforming components.

## 2. Detailed Requirements

Consolidated from `idea-honing.md` and the rough idea.

### 2.1 Functional requirements

- **FR1 — Manifest.** A component is described by a machine-readable manifest that
  declares: the component's **name**, the **Go packages** that constitute it (by
  import-path pattern/list), the **interface files** (the `.go` files that hold its
  public surface), its **component dependencies** and **absorbed (impl-detail)
  dependencies**, and its **declared ambient authority** (MVP: empty).
- **FR2 — Conformance CLI.** A CLI takes a manifest and reports pass/fail with
  actionable violations. Exit code `0` = conforms, `1` = violations found, `2` =
  tool error. Human-readable output by default; `--format=json` for machine use.
- **FR3 — Pillar 1: dependency conformance.** Every non-stdlib import of the
  component's packages must be permitted by the manifest, i.e. belong to a declared
  component dependency's interface packages or a declared absorbed dependency.
  Undeclared imports fail conformance. (Stdlib imports are governed by Pillar 3,
  not this check — see §5.2.)
- **FR4 — Declared interface = closure over interface-file declarations.** A
  component's **declared interface** is the *closure*:
  - **(a)** every exported top-level symbol (funcs, types, vars, consts) declared
    in files listed as `interface_files`; **plus**
  - **(b)** the full **method set** (within the component's packages) of every
    exported type — including *interface* types — declared in an interface file,
    **regardless of which file defines the method bodies**. For an exported
    interface type declared in an interface file, the in-component concrete
    implementations' methods enter the boundary/prune **symbol set**, because the
    call graph (VTA) resolves dynamic dispatch to concrete methods (review A1).
  - **(c)** package **`init` functions are implicitly part of the interface**
    (importing a package runs its init). To make that explicit, any explicit
    `func init()` in a component's packages must be declared in an interface
    file; otherwise → `INIT_OUTSIDE_INTERFACE` violation (review A2).

  A symbol that is Go-exported but outside this closure is
  **architecture-private**: it may be used for cross-package composition *within*
  the component, but it is not part of the component's contract.
- **FR5 — Cross-component interface boundary (Pillar 1, strong form).** No code
  **outside** a component may call that component's architecture-private symbols;
  equivalently, when component A depends on component B, every call edge from A into
  B must land on one of **B's declared interface symbols** — never on a symbol that
  is merely language-level public. This one graph property, seen from B's side, is
  "private-implementation protection"; seen from A's side, it is "only call declared
  interfaces." (Within a single package, Go visibility already enforces the
  unexported part for free; this FR adds the exported-but-not-declared part and the
  cross-package/cross-component part.)
- **FR5b — Pillar 3: capability-attribution pruning at component boundaries.** When
  analyzing component A's ambient authority, the call-graph traversal is **pruned at
  each direct component dependency's declared interface symbols** (the FR4 closure
  set) **and at each dependency package's `init` function** (`func <pkg>.init` —
  the synthetic root init, which covers explicit `init#N` funcs and package-level
  var initializers; without this, a dependency's import-time authority would
  re-absorb into A with no prune point — review A2). Authority exercised *inside*
  a dependency's implementation is attributed to the **dependency** (and governed
  by *its* manifest), not to A. This is what makes a **component dependency**
  differ from an **absorbed dependency**: component deps are pruned (authority not
  absorbed); absorbed deps are not pruned (authority absorbed and surfaced by A).
  Soundness is **compositional**: pruning trusts each dependency's declared
  contract, and the whole graph is sound iff every component independently
  conforms. (One known exception — function values passed across the pruned
  boundary — is documented in §11 and warned on, review A3.)
- **FR6 — Pillar 3: ambient-authority-free enforcement.** Using Capslock's
  transitive call-graph capability analysis, the component's packages must exercise
  no ambient authority beyond what a **capability policy** allows. The checker is
  parameterized by that policy (`allowed` + `warn` capability sets); the policy's
  `allowed` set is sourced from the manifest's `declared_authority`. **MVP default
  is strict:** `declared_authority` is empty and `warn` is empty, so **any**
  capability finding — true-authority *or* analysis-defeating — fails conformance.
  Non-default policies (e.g. "true fails, analysis-defeating warns", or a per-
  manifest allow/warn list) are expressible through the same parameter and are the
  extension path to configurable-per-manifest.
- **FR7 — Absorption.** A third-party/internal package used purely as an
  implementation detail is declared as an **absorbed dependency**: it needs no
  manifest, and its transitive use of ambient authority is **absorbed into and
  surfaced by** the absorbing component (this is automatic — Capslock attributes a
  dependency's capabilities to the caller). A **component dependency**, by
  contrast, is a first-class architecture edge to another manifested component that
  accounts for its own authority behind its contract.
- **FR8 — Self-hosting.** The tool's own Go code is decomposed into components,
  each with a manifest, and the checker can be run on itself. At least the pure
  checking core is ambient-authority-free.
- **FR9 — Examples.** A small multi-component example Go project (a CSV "top row by
  column" tool) demonstrates a passing ambient-authority-free component and a
  failing (undeclared-authority or undeclared-dependency) component.

### 2.2 Non-functional / structural requirements

- **NFR1 — Multi-language-ready layout.** Go-specific tooling lives under `./go`;
  the manifest schema and shared assets live in a shared location so a future Rust
  toolchain can reuse them.
- **NFR2 — Protobuf.** The manifest schema is defined in Protocol Buffers.
  Manifests are authored as `.textproto` files (Q4).
- **NFR3 — Bazel-ready.** The manifest's dependency and interface-file fields must
  be plain lists a future `go_architectural_component` Bazel rule can populate
  mechanically. (Bazel integration itself is out of scope for the MVP.)
- **NFR4 — Capslock as a library.** Capslock is consumed as a Go library behind a
  small internal `CapabilityAnalyzer` port (Q1), keeping the checker core
  decoupled and testable.

### 2.3 Out of scope (MVP)

Verified/formal contracts (Pillar 2); data-flow & privacy (Pillar 4); the Bazel
rule; non-empty declared authority and the capability box (§5.4(b) of the concept
doc); cross-package private-impl enforcement; languages other than Go.

## 3. Architecture Overview

The tool is split into a **pure core** (ambient-authority-free) and an
authority-holding **shell**. The shell loads all facts from the world (files,
`go list`, Capslock) and hands plain data to the core, which decides conformance.
This split is both good design and the centerpiece of the self-hosting demo.

```mermaid
flowchart TD
    subgraph shell["Shell (holds ambient authority: FILES, EXEC, READ_SYSTEM_STATE)"]
        CLI["cli\n(arg parsing, read manifest file, print report)"]
        LOAD["goanalysis\n(go/packages load, AST exported-symbol map,\nimport extraction)"]
        CAPADAPT["capslock-adapter\n(implements CapabilityAnalyzer port)"]
    end
    subgraph core["Pure core (ambient-authority-free)"]
        MAN["manifest\n(textproto bytes -> Manifest model, validation)"]
        FACTS["facts\n(pure data model: PackageFacts,\nCallEdge, DependencyInterface)"]
        CHK["checker\n(all conformance rules -> ConformanceReport)"]
        REP["report\n(report model + text rendering;\nJSON marshaling is the shell's job)"]
    end
    PORT{{"CapabilityAnalyzer (port/interface)"}}

    CLI -->|manifest bytes| MAN
    CLI -->|orchestrates| LOAD
    CLI -->|orchestrates| CAPADAPT
    LOAD -.produces types defined in.-> FACTS
    LOAD -->|"facts.PackageFacts\n(imports, exported symbols,\ncall edges, dep interface symbols)"| CHK
    LOAD -->|"PruneAt = dep interface symbols"| CAPADAPT
    CAPADAPT -.implements.-> PORT
    PORT -->|"CapabilityFinding[]\n(pruned at component-dep boundaries)"| CHK
    MAN -->|Manifest model| CHK
    CHK -->|ConformanceReport| REP
    REP -->|rendered| CLI

    capslock["Capslock analyzer\n(github.com/google/capslock)"]
    CAPADAPT --> capslock
```

Data flows one way into the core; the core calls nothing that touches the world.
The `CapabilityAnalyzer` port is defined in the core; its Capslock-backed adapter
lives in the shell. Likewise, the **fact data model lives in the pure core**
(`facts`), and the shell's `goanalysis` produces values of those core-defined
types — dependencies point inward, never core→shell (review B7).

### 3.1 End-to-end sequence

```mermaid
sequenceDiagram
    participant U as User
    participant CLI as cli (shell)
    participant M as manifest (core)
    participant L as goanalysis (shell)
    participant C as capslock-adapter (shell)
    participant K as checker (core)

    U->>CLI: archcheck check COMPONENT.textproto
    CLI->>CLI: read manifest file bytes (FILES)
    CLI->>M: Parse(bytes) -> Manifest
    M-->>CLI: Manifest (validated)
    CLI->>L: ResolveDependencyInterface(each component dep) (FILES)
    L-->>CLI: DepIfaces (declared interface symbols per dep)
    CLI->>L: LoadPackageFacts(manifest.packages) (go list + SSA callgraph, FILES/EXEC)
    L-->>CLI: PackageFacts (imports, exported symbols->file, call edges)
    CLI->>C: Analyze({Packages, PruneAt: DepIfaces.Symbols})
    C-->>CLI: CapabilityFinding[] (pruned at component-dep boundaries)
    CLI->>K: Check({Manifest, Facts, DepIfaces, Caps, Policy})
    K-->>CLI: ConformanceReport
    CLI->>U: rendered report + exit code (0/1)
```

## 4. Components and Interfaces

Each of the tool's own components gets a manifest (self-hosting). Working import
root: `github.com/xtofian/architectural-contracts/go` (adjust to the real module
path). Component boundaries and their authority:

| Component | Package(s) | Role | Ambient authority |
|---|---|---|---|
| `manifest` | `.../internal/manifest` | Parse+validate manifest bytes → model | none (pure)* |
| `facts` | `.../internal/facts` | Pure data model: `PackageFacts`, `CallEdge`, `DependencyInterface` (produced by the shell, consumed by the checker) | none (pure) |
| `checker` | `.../internal/checker` | All conformance rules → report | none (pure) |
| `report` | `.../internal/report` | Report model + text rendering (JSON marshaling is the shell's) | none (pure) |
| `capanalyzer` (port) | `.../internal/capanalyzer` | `CapabilityAnalyzer` interface + finding types | none (pure) |
| `goanalysis` | `.../internal/goanalysis` | `go/packages` load; AST/imports extraction; VTA call-graph → cross-component call edges; resolve component-dependency manifests → declared-interface symbol sets (all as `facts.*` values) | FILES, EXEC, READ_SYSTEM_STATE |
| `capslockadapter` | `.../internal/capslockadapter` | Capslock-backed `CapabilityAnalyzer` | FILES, EXEC, READ_SYSTEM_STATE |
| `cli` | `.../cmd/archcheck` + `.../internal/app` | Orchestration, I/O, exit codes, JSON marshaling of the report | FILES (read manifest, stdout), REFLECT (encoding/json) |

Core components (`manifest`, `facts`, `checker`, `report`, `capanalyzer`) are
**ambient-authority-free** and depend only on each other and stdlib-that-is-safe.
They are the self-hosting showcase. (*`manifest` carries a known reflection
caveat from `prototext` — see the plan's Step 2 risk note; the guaranteed
showcase is `facts`/`checker`/`report`/`capanalyzer`.)

### 4.1 Key interfaces (Go signatures, illustrative)

```go
// package capanalyzer  (pure port; defined in the core)

// CapabilityFinding is one capability reached transitively by a component's code.
type CapabilityFinding struct {
    Package    string      // import path where the capability is incurred
    Capability string      // e.g. "FILES", "NETWORK", "REFLECT"
    Class      Class       // TrueAuthority | AnalysisDefeating
    CallPath   []Frame     // example path: caller -> ... -> privileged callee
}
type Frame struct{ Func, File string; Line int }

// InterfaceSymbol identifies one declared-interface symbol of a component, in the
// key form Capslock/go-types use, e.g. "example.com/store.Read" or
// "(*example.com/store.DB).Get".  It is the shared primitive behind both the
// boundary check (Pillar 1) and capability pruning (Pillar 3).
//
// Matching is NORMALIZED (review A4): generic type-argument brackets are
// stripped from SSA names before comparison, and methods are emitted in BOTH
// pointer- and value-receiver key forms. Symbol sets are the FR4 *closure*
// (interface-file decls + method sets of interface-file types, incl. concrete
// implementations of interface-file interface types).
type InterfaceSymbol string

// AnalyzeRequest asks the analyzer for the capabilities of a component's packages,
// with the call-graph traversal PRUNED at the given symbols (the declared
// interface symbols of the component's direct *component* dependencies, plus
// "func <pkg>.init" for each dependency package — review A2). Authority
// reached only *through* a pruned symbol is attributed to the dependency, not to
// the analyzed component. Absorbed deps are simply absent from PruneAt.
type AnalyzeRequest struct {
    Packages []string
    PruneAt  []InterfaceSymbol
}

// CapabilityAnalyzer is the port the checker depends on; the Capslock adapter
// (in the shell) implements it. Injecting it keeps the checker pure & testable.
// The Capslock adapter implements PruneAt by emitting a custom capability map that
// marks each pruned symbol CAPABILITY_SAFE (which terminates traversal).
type CapabilityAnalyzer interface {
    Analyze(req AnalyzeRequest) ([]CapabilityFinding, error)
}

// CapabilityPolicy decides, per capability, whether a finding is allowed (no
// report entry), a warning, or a violation. A capability in neither set is a
// violation. The MVP default (StrictPolicy) has both sets empty, so ANY
// capability fails.  Allowed is normally sourced from the manifest's
// declared_authority, which is the seam to configurable-per-manifest policy.
type CapabilityPolicy struct {
    Allowed map[string]bool // permitted capabilities (e.g. from declared_authority)
    Warn    map[string]bool // capabilities downgraded to a non-fatal warning
}

func StrictPolicy() CapabilityPolicy { return CapabilityPolicy{} } // MVP default
```

```go
// package facts  (PURE core) — the data model for facts about loaded code.
// Defined in the core so the checker never imports the shell (review B7);
// the shell's goanalysis produces values of these types.
type PackageFacts struct {
    Packages   []PackageFact
    CallEdges  []CallEdge          // inter-package call edges (from VTA call graph)
}
type PackageFact struct {
    ImportPath      string
    IsStdlib        bool
    Imports         []string             // direct imports of this package
    ExportedSymbols []ExportedSymbol     // top-level exported decls (+ init decls)
}
type ExportedSymbol struct {
    Name     string       // Capslock/go-types key form (see InterfaceSymbol)
    File     string       // file (relative to component root) where declared
    Kind     string       // func | type | var | const | method | init
    Receiver string       // for methods: the declaring type's key (drives the
                          // FR4 closure: method ∈ interface iff its receiver
                          // type is declared in an interface file)
}
// CallEdge is one static call edge; used for the FR5 cross-component boundary check.
type CallEdge struct {
    Caller InterfaceSymbol   // calling function/method (in some package)
    Callee InterfaceSymbol   // called function/method
    PassesFuncValue bool     // call site passes function-typed value(s) —
                             // drives the HIGHER_ORDER_BOUNDARY_CALL warning (A3)
}

// DependencyInterface is a *component* dependency's manifest + interface files
// resolved into its declared-interface symbol set (the FR4 closure, incl.
// concrete methods of interface-file interface types, plus per-package init
// keys). Used to (a) build the FR5b PruneAt set and (b) validate FR5 boundary
// calls.
type DependencyInterface struct {
    Component string
    Packages  []string
    Symbols   []capanalyzer.InterfaceSymbol
}
```

```go
// package goanalysis  (shell) — produces the facts the checker needs, as
// values of the core-defined facts types.
func LoadPackageFacts(pkgPatterns []string) (facts.PackageFacts, error) // FILES/EXEC
func ResolveDependencyInterface(dep manifest.ComponentDependency) (facts.DependencyInterface, error) // FILES
```

```go
// package checker  (pure) — the heart of the tool.
type Inputs struct {
    Manifest    manifest.Manifest
    Facts       facts.PackageFacts
    DepIfaces   []facts.DependencyInterface        // resolved direct component deps
    Caps        []capanalyzer.CapabilityFinding    // ALREADY pruned at DepIfaces (FR5b)
    Policy      capanalyzer.CapabilityPolicy       // default StrictPolicy()
}
func Check(in Inputs) report.ConformanceReport
```
Everything the checker needs is injected data — no I/O, no globals. `Caps` arrive
**already pruned** at the component-dependency boundaries (the analyzer applied
`PruneAt`), so the authority rule simply checks what's left. `DepIfaces` also feed
the FR5 boundary check over `Facts.CallEdges`.

The `checker.Check` function is a **pure function of its inputs** — no I/O, no
globals — which is exactly what makes the `checker` component ambient-authority-
free and trivially unit-testable with hand-built inputs.

## 5. Conformance rules (the checker)

Given `Manifest`, `PackageFacts`, and `[]CapabilityFinding`, the checker emits a
`ConformanceReport` of violations + warnings.

### 5.1 Dependency rule (FR3)
Build the **allowed import set** = union of:
- every component dependency's **interface packages** — defined as *the packages
  of the dependency that contain at least one of its interface files*, derived
  from the dependency's own manifest (review C10), and
- every absorbed dependency's import path(s) (pattern-matched: the checker
  glob-matches literal import strings against the declared patterns — pure).

For each package in the component, for each of its **non-stdlib** direct imports
that is **not itself part of the component**: if the import ∉ allowed set →
`UNDECLARED_DEPENDENCY` violation (with the importing package + the import path).
Types-only imports follow the same allowlist (type *use* across the boundary is
governed by FR5's call-edge scope; see the §11 "use beyond calls" caveat).

Additionally, a declared component or absorbed dependency that matches **no**
import of the component's packages → `UNUSED_DEPENDENCY` **warning** (review
C13 — keeps manifests from accumulating stale grants, in the least-privilege
spirit).

### 5.2 Why stdlib imports are auto-allowed at Pillar 1
"Dependencies are declared at the granularity of *components*" (concept §3.1); the
Go standard library is not a component. The *risk* stdlib carries is **ambient
authority**, which Pillar 3 (Capslock) governs directly and transitively. So a
component may import `os` without a Pillar-1 violation, but importing `os` in a way
that reaches a privileged call will fail the **Pillar-3** check. This keeps the two
pillars cleanly separated: Pillar 1 governs *edges to other code units*, Pillar 3
governs *system powers reached*.

### 5.3 Declared interface (FR4 closure)
From `PackageFacts`, the component's **declared interface** is computed as the
FR4 closure:
- every `ExportedSymbol` whose `File` ∈ `interface_files`;
- every method whose `Receiver` type is declared in an interface file — even if
  the method body lives in a non-interface file (review A1; requiring
  colocation of a type and all its methods in one file is not idiomatic Go);
- for exported **interface types** declared in interface files, the
  in-component concrete implementations' method keys (computed via `go/types`
  method-set/satisfaction analysis in `goanalysis`) join the *boundary/prune
  symbol set*, since VTA edges land on concrete methods, not abstract ones.

Exported symbols outside this closure are **architecture-private** (usable for
intra-component composition, not part of the contract). An
exported-but-non-interface symbol is *not itself* a violation of the component's
own manifest — it is governed by the boundary rule below, which forbids *outside*
code from calling it. (We may still emit an informational note when such symbols
exist, to help authors notice unintended public surface.)

**Explicit `init` rule (review A2):** an explicit `func init()` declared in a
non-interface file → `INIT_OUTSIDE_INTERFACE` violation. Rationale: importing a
package runs its inits, so init behavior is *de facto* part of the component's
exposed interface; the manifest should make that visible rather than implicit.
(Synthetic inits from package-level var initializers are governed by the
component's own capability check.)

### 5.3b Cross-component interface boundary (FR5 + FR5b linkage)
Using `Facts.CallEdges` and the resolved `DepIfaces` (symbol sets are the FR4
closure; matching uses the A4 normalization — generic brackets stripped, both
receiver forms):
- For each call edge whose **callee** belongs to a direct component dependency
  `B`'s packages: if the callee ∉ `B`'s declared interface symbol set →
  `CALLS_UNDECLARED_INTERFACE` violation (with caller, callee, and `B`). This is
  the "don't call B's language-public-but-not-declared symbols" rule.
- For each call edge **into** a declared interface symbol of `B` whose call site
  passes function-typed values (`CallEdge.PassesFuncValue`) →
  `HIGHER_ORDER_BOUNDARY_CALL` **warning** (review A3): authority exercised by
  the passed function when `B` invokes it may escape pruned attribution — see
  §11.
- The **same** `DepIfaces` symbol sets (plus per-package `init` keys, FR5b) are
  handed to the analyzer as `PruneAt`, so the capability findings the checker
  receives are already pruned at those boundaries — authority behind `B`'s
  interface is attributed to `B`.

Note the consistency between the two pillars: if A calls a *non-declared* public
symbol of B, that symbol is (correctly) **not** in `PruneAt`, so any authority
behind it leaks into A's capability findings — but A has already failed the
boundary check, so the outcome (A is non-conformant) is the same either way.

**Scope honesty (review D18):** seen from B's side, "nothing outside B calls
B's architecture-private symbols" is enforced only over the set of components
that are actually *checked*. An unchecked consumer can call anything Go lets
it call; the guarantee accrues as coverage does (see §11 compositional trust
and Appendix D whole-graph mode).

### 5.4 Authority rule (FR6)
Derive the effective policy: `policy.Allowed ⊇ manifest.declared_authority`
(empty in MVP). For each `CapabilityFinding`:
- capability ∈ `policy.Allowed` → no report entry.
- capability ∈ `policy.Warn` → `ANALYSIS_LIMITATION`/`ALLOWED_WITH_WARNING`
  **warning** (non-fatal).
- otherwise → `UNDECLARED_AUTHORITY` **violation**, carrying the Capslock example
  call path as evidence.

Under the **MVP default (`StrictPolicy`)** both sets are empty, so every finding —
true-authority or analysis-defeating — is a violation. The `Class` field on a
finding is retained so a non-strict policy (e.g. "warn on analysis-defeating") can
be expressed without re-running analysis. Because findings are **already pruned**
at component-dependency boundaries (§5.3b / FR5b), authority owned by a dependency
does not appear here — only authority the analyzed component reaches on its own or
through its **absorbed** dependencies.

### 5.5 Component-dependency integrity
For each declared component dependency (review C12):
- its `manifest` path (relative to the **declaring manifest's directory**)
  resolves and loads — failure is a tool error (exit 2);
- the resolved manifest's `name` matches `ComponentDependency.name` — mismatch
  is a tool error;
- the dependency's package set does **not overlap** the analyzed component's
  package set → overlap is a conformance violation (`PACKAGE_OVERLAP`) —
  overlapping membership breaks attribution;
- imports/edges into it target only its declared interface packages/symbols
  (§5.3b).

This is a *first-class* MVP check, not a deferred one — it is the load-bearing
mechanism for compositional soundness. (Overlap among components not on the
current check's dependency path is caught only by whole-graph mode, Appendix D.)

## 6. Data Models

### 6.1 Manifest schema (proto; logical fields) `[PROVISIONAL Q4 = textproto on disk]`

```proto
// component.proto  (shared, language-neutral where possible)
syntax = "proto3";
package archcontracts.v1;

message Component {
  string name = 1;                          // logical component name
  repeated string packages = 2;             // import-path patterns/list defining membership
  repeated string interface_files = 3;      // .go files holding the public surface, relative to the
                                            // COMPONENT ROOT := the manifest file's directory (C9);
                                            // multi-package components use subdir paths (store/api.go)
  repeated ComponentDependency component_dependencies = 4;
  repeated AbsorbedDependency  absorbed_dependencies  = 5;
  repeated string declared_authority = 6;   // capability names, validated at parse time against the
                                            // known capability set (C11); MVP: empty (authority-free)
  string contract_note = 7;                 // optional free-text pointer; real contract is prose in interface files
}

message ComponentDependency {
  string name = 1;                          // referenced component's name; must match the `name`
                                            // in the resolved manifest (C12)
  string manifest = 2;                      // path to that component's manifest, relative to the
                                            // DECLARING manifest's directory (source of truth for
                                            // the dependency's declared interface symbols)
  reserved 3;                               // was interface_packages; dropped — interface packages
                                            // are DERIVED (packages containing interface files, C10)
}

message AbsorbedDependency {
  string import_path = 1;                   // third-party/internal impl-detail package (may be a pattern)
  string reason = 2;                        // optional human note (why it's an impl detail)
}
```

Example (`examples/csvtool/toprow/COMPONENT.textproto`):
```textproto
name: "toprow"
packages: "example.com/csvtool/toprow"
interface_files: "toprow.go"
absorbed_dependencies { import_path: "example.com/csvtool/internal/parsecsv" reason: "CSV parsing impl detail" }
# declared_authority intentionally empty -> ambient-authority-free
```

### 6.2 Report model

```go
type ConformanceReport struct {
    Component  string
    Violations []Finding
    Warnings   []Finding
}
type Finding struct {
    Kind     Kind        // violations: UNDECLARED_DEPENDENCY | CALLS_UNDECLARED_INTERFACE |
                         //   UNDECLARED_AUTHORITY | INIT_OUTSIDE_INTERFACE | PACKAGE_OVERLAP
                         // warnings:   ANALYSIS_LIMITATION | ALLOWED_WITH_WARNING |
                         //   HIGHER_ORDER_BOUNDARY_CALL | UNUSED_DEPENDENCY
    Message  string
    Location Location    // file:line where relevant
    Evidence []string    // e.g. Capslock example call path frames
}
```

`report` renders **text** purely (returns strings; the shell prints). For
`--format=json`, the shell (`cli`) marshals the `ConformanceReport` struct with
`encoding/json` — kept out of the pure core because `encoding/json` reaches
`reflect` and would break the core's own authority-free claim (review B6);
`cli`'s manifest declares `REFLECT` accordingly.

## 7. Error Handling

Three-way outcome, mapped to exit codes (mirrors Capslock's own convention):
- **Conforms** → report with no violations, exit `0`. **Warnings alone do not
  change the exit code** — a warnings-only report exits `0` (review C14).
- **Non-conformance** → report lists violations (and any warnings), exit `1`.
- **Tool error** (manifest parse failure, package load failure, Capslock error,
  interface file not found on disk, dependency-manifest resolution/name-mismatch
  failure) → error to stderr, exit `2`. Tool errors are distinct from
  conformance failures and never masquerade as a "pass."

Specifics (validation is split — review C9 — because pure `Parse` cannot
resolve package patterns to directories):
- **Syntactic manifest validation** (pure, in `manifest.Parse`): empty `name`,
  empty `packages`, duplicate declarations, unknown capability name in
  `declared_authority` (C11) → parse error (exit 2).
- **Resolved validation** (shell, after package load): interface file missing
  on disk or not belonging to any of the component's packages; dependency
  manifest unresolvable or name-mismatched (§5.5) → tool error (exit 2).
- **Package load**: `go/packages` load errors (e.g. package not found, build
  errors) are surfaced verbatim and abort with exit 2 (we do not analyze a broken
  build).
- **Capslock**: analysis-defeating findings are *warnings* not errors under a
  non-strict policy (violations under strict); an actual Capslock failure is a
  tool error (exit 2).

## 8. Testing Strategy

- **Checker (pure core) — table-driven unit tests.** Because `checker.Check` is a
  pure function of `(Manifest, PackageFacts, []CapabilityFinding)`, we test every
  rule with hand-constructed inputs and golden `ConformanceReport`s — no Go build,
  no Capslock, fully deterministic. This is the bulk of the test suite.
- **manifest.Parse — unit tests** for valid/invalid textproto, including each
  validation error.
- **goanalysis — integration tests** against small fixture packages under
  `testdata/`: assert extracted imports and exported-symbol→file mappings.
  Fixtures must cover the FR4-closure corner cases (review A1/A4): a type
  declared in an interface file with methods defined elsewhere; an exported
  interface type + concrete implementation; a generic function/method
  (normalization); pointer- vs value-receiver keys; promoted methods from
  embedding; an explicit `func init()`.
- **capslockadapter — integration tests** against fixture packages with known
  capabilities (e.g. a package that reads a file → expect a FILES finding; a pure
  arithmetic package → expect none). Guards our mapping of Capslock's proto onto
  `CapabilityFinding`/`Class`.
- **End-to-end golden tests** on the `examples/` projects: run the CLI, assert exit
  code + rendered report. Includes at least one **conforming** and one
  **intentionally non-conforming** example (undeclared dep, leaked symbol,
  undeclared authority).
- **Self-hosting test.** Run `archcheck` on the tool's own core-component manifests
  in CI; the pure core must verify as ambient-authority-free.

## 9. Repository layout

```
architectural-contracts/
├── proto/                         # shared, language-neutral schema
│   └── archcontracts/v1/component.proto
├── go/                            # Go toolchain (this MVP)
│   ├── go.mod
│   ├── cmd/archcheck/             # CLI entry (shell)
│   ├── internal/
│   │   ├── manifest/              # pure: parse+validate  (+ generated proto or gen/)
│   │   ├── facts/                 # pure: fact data model (PackageFacts, CallEdge, DependencyInterface)
│   │   ├── checker/               # pure: conformance rules
│   │   ├── report/                # pure: report model + text rendering (JSON = shell)
│   │   ├── capanalyzer/           # pure: CapabilityAnalyzer port + finding types
│   │   ├── goanalysis/            # shell: go/packages + AST facts (produces facts.* values)
│   │   ├── capslockadapter/       # shell: Capslock-backed adapter
│   │   └── app/                   # shell: orchestration used by cmd/archcheck (+ JSON marshal)
│   ├── components/                # this tool's OWN manifests (self-hosting)
│   │   ├── checker.COMPONENT.textproto
│   │   ├── manifest.COMPONENT.textproto
│   │   └── ...
│   └── examples/
│       └── csvtool/               # demo multi-component Go project
│           ├── toprow/            # ambient-authority-free logic component
│           ├── internal/parsecsv/ # absorbed impl-detail dependency
│           ├── shell/             # component that holds file I/O (declares FILES) or fails if it claims none
│           └── *.COMPONENT.textproto
└── (rust/ later)
```

Note (Q5, confirmed): **`./go` is its own Go module** (`go/go.mod`). The `proto/`
dir is deliberately outside `go/` so a future Rust toolchain can share it (NFR1);
Go accesses the generated code either via a committed `go/internal/manifest/gen/`
or a module-local generation step (TBD in implementation). Examples under
`go/examples/` are either a nested module or `testdata`-style packages so they
don't pollute the tool's own dependency graph.

## 10. The CSV example (FR9) — what it demonstrates

A tiny "print the top row of a CSV sorted by column N" tool, split to make **all
the concepts** — including the two dependency kinds and boundary pruning — visible:

- **`toprow`** — pure logic: given already-parsed rows, sort and pick. Imports only
  an absorbed CSV-parsing helper + stdlib-safe (`sort`, `strconv`). Its manifest
  declares **no authority**; `archcheck` confirms it is **ambient-authority-free**.
  **Stdlib pitfall (review B8):** it must use `sort.Sort` with a concrete
  `sort.Interface` (or another SAFE-classified entry point) — **not `sort.Slice`**,
  which Capslock classifies `unanalyzed` (its `less` callback is precisely the
  A3 higher-order phenomenon) and which would fail the strict policy. The Step-0
  spike establishes the strict-safe stdlib envelope the examples may use.
- **`internal/parsecsv`** — **absorbed** impl-detail dependency (wraps
  `encoding/csv`; no manifest). Declared as an `absorbed_dependency` of `toprow`;
  its behavior — and any authority it used — is **absorbed** into `toprow`'s
  contract. (In this example it touches no FS, so `toprow` stays authority-free.)
- **`csvfile`** — a **component** dependency *with its own manifest* that legitimately
  declares `declared_authority: "FILES"` and exposes `Read(path) ([][]string, err)`.
  Checked on its own, it conforms (FILES is declared).
- **`app`** — the composition root. It depends on `csvfile` as a **component
  dependency** and on `toprow`. **This is the pruning showcase:** because the
  traversal is pruned at `csvfile`'s declared interface, `app` is **not** attributed
  `FILES` even though it composes a file-reading component — so `app` can itself be
  **ambient-authority-free** while orchestrating authority it never holds. That is
  the capability model working as intended.

Failing variants (golden non-conformance tests):
- `app` **absorbs** `csvfile` instead of depending on it as a component (or calls
  `os.Open` directly) while claiming no authority → `UNDECLARED_AUTHORITY` (evidence:
  Capslock path to `os.Open`), showing absorb-vs-prune is what differs.
- `app` calls a `csvfile` symbol that is Go-exported but **not** in `csvfile`'s
  interface files → `CALLS_UNDECLARED_INTERFACE`.
- `toprow` omits the `parsecsv` `absorbed_dependency` it imports →
  `UNDECLARED_DEPENDENCY`.

## 11. Known limitations / caveats (MVP)

Now *designed in* (were deferred in the earlier draft): the cross-component
interface boundary check (FR5), "no call edge into a dependency's non-interface
symbols" (concept Appendix A.1), and component-dependency authority pruning /
non-re-absorption (FR5b). See §5.3b, §5.4, §5.5.

Remaining caveats:
- **Call-graph precision.** FR5 and FR5b rely on a static call graph — **VTA**,
  matching Capslock exactly (review A5; two graph constructions per run is an
  accepted MVP cost, since Capslock's internal graph is not injectable). Dynamic
  dispatch (and reflection) makes edges **over-approximate**: the analyzer may
  see call edges that can't happen at runtime, yielding false-positive boundary
  violations. Documented, not eliminated.
- **Higher-order boundary leak (review A3) — the one known way pruning can hide
  authority.** Pruning cuts every path segment through a SAFE frame. If A passes
  a function value into B's pruned interface and that function comes from A's
  *absorbed dependency*, authority it exercises when B invokes it is attributed
  to nobody (A's path is pruned at B; B's own check never sees A's absorbed
  dep). Authority in **A's own functions** invoked via callback *is* still
  caught (Capslock reports every queried-package function with its own path to
  a capability). Mitigation: `HIGHER_ORDER_BOUNDARY_CALL` warning on func-valued
  arguments crossing a pruned boundary (§5.3b). A principled treatment —
  functions passed to higher-order functions are themselves capabilities whose
  authority should attribute to the supplier — is future work (Appendix D).
- **Generic symbols (review A4).** MVP matching strips generic type-argument
  brackets from SSA names and emits both receiver key forms; this is believed
  conservative (a missed match fails *open*: authority leaks in and surfaces as
  a violation rather than being hidden). Robust handling is an open question
  (Appendix D).
- **Compositional trust.** FR5b pruning trusts each dependency's *declared* contract;
  the guarantee holds only if **every** component in the graph is itself checked and
  conformant. A partial check (some components unchecked) weakens the guarantee —
  and the FR5 "two views" equivalence likewise only covers checked consumers
  (§5.3b scope note).
- **Use beyond calls.** The boundary check targets *call* edges. Using a
  dependency's types, struct fields, or exported **vars** (a mutable-state channel
  invisible to both FR5 and FR5b) without a call is not covered; an extension.
  (Concept §3.1: "calls into, uses types from, or otherwise communicates with".)
- **Interface strictness (resolved):** exported symbols outside the FR4 closure are
  *architecture-private*, not errors (§5.3) — the boundary rule, not a blanket
  "everything exported must be in an interface file," is what bites.
- **Analysis soundness** is otherwise bounded by Capslock (reflection/unsafe/cgo),
  which under `StrictPolicy` fail conformance rather than pass silently.

## 12. Appendices

### Appendix A — Key design decisions & rationale

- **Pure-core / authority-shell split.** Chosen so the checker is (a) trivially
  testable as a pure function and (b) a genuine ambient-authority-free component we
  can dogfood. It also cleanly resolves the "Capslock needs authority, so how can
  the tool be authority-free?" tension: only the shell touches the world.
- **Capslock as a library behind a port (Q1).** Gives structured protos + the
  future call-graph hook, while the port keeps the core decoupled and swappable
  (future `cargo-geiger`-style Rust adapter).
- **Two dependency kinds = pruned vs not pruned (FR7 + FR5b).** The distinction is
  made *mechanically precise* by call-graph pruning: a **component dependency** is
  pruned at its declared interface (its authority is *its own*, behind its
  contract); an **absorbed dependency** is not pruned (its authority is absorbed and
  surfaced by the absorber, which Capslock's transitivity does for free). This is
  the resolution of the rough idea's central open question.
- **One boundary property, two views (FR5).** "A only calls B's declared interface"
  (dependent's view) and "nothing outside B calls B's architecture-private symbols"
  (dependency's view) are the *same* call-graph property. Interface files define the
  declared set; the same set drives the FR5 check and the FR5b prune set — one
  primitive, both pillars.
- **Pruning mechanism = Capslock `CAPABILITY_SAFE`.** We generate a per-analysis
  custom capability map marking each pruned symbol `CAPABILITY_SAFE`, which
  "terminates further analysis" — no fork of Capslock needed. Feasibility and the
  `func <path>.<Name>` / `func (*<path>.<Type>).<Method>` key format were verified
  against `interesting/interesting.cm`; the prune-skipping behavior was verified
  in `analyzer.searchBackwardsFromCapabilities` (review). The prune set also
  includes `func <pkg>.init` per dependency package (review A2 — precedent:
  `func encoding/json.init CAPABILITY_SAFE` in the builtin map).
- **Interface = closure, not file-literal (review A1).** Call graphs (VTA)
  resolve dynamic dispatch to *concrete* methods, so a file-literal reading of
  "declared interface" breaks on idiomatic Go (interface-returning constructors,
  type in `api.go` / methods in `impl.go`). The FR4 closure — method sets of
  interface-file types, incl. concrete implementations of interface-file
  interface types — is what makes FR5/FR5b match real edges.
- **Fact model in the core (review B7).** `facts` is a pure core component so the
  checker never imports the shell; `goanalysis` produces core-defined types.
  Same inward-pointing pattern as the `capanalyzer` port.
- **Stdlib auto-allowed at Pillar 1 (§5.2).** Keeps Pillar 1 = "edges between code
  units" and Pillar 3 = "system powers," avoiding a redundant per-stdlib-package
  declaration burden.

### Appendix B — Technology choices (pros/cons)

| Choice | Pros | Cons |
|---|---|---|
| Capslock as **library** | structured protos; call-graph hook; single shared `packages.Load` | tighter coupling to a pre-1.0 API |
| Capslock as **subprocess** (rejected) | loose coupling; API-churn-proof | no call-graph hook; JSON parsing; still shells out |
| **textproto** manifests `[PROVISIONAL]` | matches protobuf constraint; diff-friendly; Bazel-generatable | less familiar than YAML to some authors |
| Component = **package-pattern set** | flexible; matches "module or packages" | needs care mapping patterns→membership |

### Appendix C — Research findings (summary)

From `research/capslock.md` and `research/go-component-model.md`:
- Capslock exposes `analyzer.GetCapabilityInfo(pkgs, queried, config) →
  CapabilityInfoList`; **empty list ⇒ ambient-authority-free.** 14 capabilities; 8
  true-authority, 5 analysis-defeating, 1 explicit-safe.
- Capslock's transitivity **implements authority absorption for free** (for
  absorbed deps), and its **custom capability map** with `CAPABILITY_SAFE` (which
  "terminates further analysis") gives us **boundary pruning for component deps** —
  verified against `interesting/interesting.cm`; key format
  `func <path>.<Name>` / `func (*<path>.<Type>).<Method>`.
- Capslock **itself requires ambient authority** (`go list` via `os/exec`) → drives
  the pure-core/shell split.
- Pillar 1 is **not** Capslock's job; it comes from `go/packages` imports +
  `go/ast` exported-symbol→file mapping. **One `packages.Load` feeds both pillars.**

### Appendix D — Remaining open items
Resolved: Q1 (Capslock as library), Q2 (package-pattern component), Q3 (strict
parameterized policy), Q4 (textproto), Q5 (`./go` own module), call-graph
algorithm (**VTA**, matching Capslock — review A5), interface-file strictness
(exported-but-non-interface symbols are architecture-private, not errors, §5.3).

Still open, non-blocking for the MVP:
1. **`CapabilityAnalyzer` port** — confirm wrapping Capslock behind our own port
   (vs the checker importing the library directly). Design assumes the port.
2. **CLI name** — working name `archcheck`; confirm or replace.
3. **Whole-graph vs single-component invocation.** Because FR5b's guarantee is
   compositional (§11), do we check one manifest at a time (trusting dependency
   manifests) or offer a "check the whole graph" mode that verifies every reachable
   component (which would also catch package-membership overlap globally, §5.5)?
   MVP does single-component + resolve-direct-deps; whole-graph is a natural
   extension.

Post-MVP research questions (from the design review):
4. **Callbacks as capabilities (review A3).** A function value passed across a
   component boundary is itself a capability grant (analogous to a file handle);
   a principled model would attribute the authority such a function exercises —
   when invoked by the callee — back to the *supplier*. The MVP's
   `HIGHER_ORDER_BOUNDARY_CALL` warning is a stopgap. Note `sort.Slice`'s
   `unanalyzed` classification in Capslock is the same phenomenon (its `less`
   argument is a meta-capability).
5. **Robust generic-symbol matching (review A4)** beyond the MVP's
   bracket-stripping normalization (instantiation-aware matching, synthetic
   wrapper functions for promoted methods).
```
