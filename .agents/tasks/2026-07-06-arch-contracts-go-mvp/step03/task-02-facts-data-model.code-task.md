# Task: `facts` data model (core-defined, shell-produced)

## Description
Introduce the pure `internal/facts` package: the data model describing facts about
loaded Go code that the checker consumes — `PackageFacts`, `PackageFact`,
`ExportedSymbol`, `CallEdge`, and `DependencyInterface`. These are plain structs with
**no loaders**: the authority-holding producers (`goanalysis`) live in the shell and
arrive in Steps 6/9. Defining the model in the core (review B7) is what keeps the
checker from ever importing the shell — every dependency points inward.

## Background
`facts` is core-defined so that `checker` never imports `goanalysis` (the shell
produces values of these types; the core consumes them). The types mirror design §4.1
exactly. Two fields carry design weight:
- `ExportedSymbol.Receiver` — the declaring type's key for methods; this drives the FR4
  well-formedness rule (methods of interface-file types must themselves be declared in
  interface files). `Kind` includes `init` (review A2).
- `CallEdge.PassesFuncValue` — flags call sites passing function-typed values, driving
  the FR5 `HIGHER_ORDER_BOUNDARY_CALL` warning (review A3).

`CallEdge.Caller`/`Callee` and `DependencyInterface.Symbols` use
`capanalyzer.InterfaceSymbol`, so this task depends on task-01. `Symbols` and `Packages`
on `DependencyInterface` are **derived** by the shell from the dependency's own root/
manifest (not declared) — a doc-comment point, not code here.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§4.1 `facts` types — `PackageFacts`, `PackageFact`, `ExportedSymbol`, `CallEdge`, `DependencyInterface`; §5.3 for the Receiver/FR4 role; §5.3b for CallEdge/FR5 role)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 3)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Create package `internal/facts` — pure structs, no loaders, no I/O, no filesystem or
   `go/packages` imports.
2. Define per design §4.1:
   - `PackageFacts` (`Packages []PackageFact`, `CallEdges []CallEdge`).
   - `PackageFact` (`ImportPath string`, `IsStdlib bool`, `Imports []string`,
     `ExportedSymbols []ExportedSymbol`).
   - `ExportedSymbol` (`Name string` — go-types/Capslock key form; `File string` —
     relative to component root; `Kind string` — one of `func | type | var | const |
     method | init`; `Receiver string` — the declaring type's key for methods).
   - `CallEdge` (`Caller capanalyzer.InterfaceSymbol`, `Callee capanalyzer.InterfaceSymbol`,
     `PassesFuncValue bool`).
   - `DependencyInterface` (`Component string`, `Packages []string`,
     `Symbols []capanalyzer.InterfaceSymbol`).
3. Import `internal/capanalyzer` for `InterfaceSymbol` (task-01); import nothing from the
   shell.
4. Document, in doc comments, that these are core-defined/shell-produced (the loaders
   live in `goanalysis`, Steps 6/9), that `Receiver` drives FR4 and `PassesFuncValue`
   drives the FR5 higher-order warning, and that `DependencyInterface.Packages`/`Symbols`
   are derived by the shell (not declared).

## Dependencies
- task-01 (`internal/capanalyzer` for `InterfaceSymbol`).

## Implementation Approach
1. Add the five struct types with the exact fields above.
2. Wire the `capanalyzer` import for `InterfaceSymbol` on `CallEdge` and
   `DependencyInterface`.
3. Add the design-anchoring doc comments (core-defined/shell-produced; FR4/FR5 roles;
   derived fields).
4. Because these are pure data holders, tests are light: a compile/structure sanity test that
   constructs a fully-populated `PackageFacts` (a package with imports and exported
   symbols including a method with a `Receiver` and an `init` `Kind`, plus a `CallEdge`
   with `PassesFuncValue` and a `DependencyInterface`) and asserts the fields round-trip
   — this both guards the field set against drift and gives later steps a hand-built
   fixture pattern.

## Acceptance Criteria

1. **Package compiles, points inward only**
   - Given `internal/facts`
   - When built and its imports inspected
   - Then it compiles, imports only `internal/capanalyzer` from the project (no shell
     packages, no `go/packages`, no I/O).

2. **Model matches design §4.1**
   - Given the defined types
   - When compared to design §4.1
   - Then `PackageFacts`, `PackageFact`, `ExportedSymbol`, `CallEdge`, and
     `DependencyInterface` carry exactly the specified fields and types, with `Caller`/
     `Callee`/`Symbols` typed as `capanalyzer.InterfaceSymbol`.

3. **Fixture round-trips**
   - Given a hand-built `PackageFacts` populated with a method symbol (non-empty
     `Receiver`), an `init` symbol (`Kind == "init"`), a `CallEdge` with
     `PassesFuncValue == true`, and a `DependencyInterface`
   - When its fields are read back in a test
   - Then every field holds the constructed value.

## Metadata
- **Complexity**: Low
- **Labels**: pure-core, facts, data-model
- **Required Skills**: Go
