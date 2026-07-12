# Task: Declared interface + well-formedness (FR4)

## Description
Extend the pure `checker` (task-01) with the second Pillar-1 rule family: compute the
component's **declared interface** from `interface_files`, and enforce the two **FR4
well-formedness rules** that guarantee the interface is fully visible in the interface
files — `METHOD_OUTSIDE_INTERFACE` and `INIT_OUTSIDE_INTERFACE`. Exported symbols not
in interface files are *architecture-private*: they are **not** a violation of the
component's own manifest (their misuse by outside code is the boundary rule's job in
Step 5).

## Background
FR4 (design §5.3) makes the interface files an honest, complete picture of a
component's exposed surface, so that any interface-affecting change necessarily edits a
declared interface file.

- **Declared interface** = every `facts.ExportedSymbol` whose `File` ∈
  `Manifest.InterfaceFiles`.
- **Method rule:** an exported method whose `Receiver` is an exported **non-interface**
  type that is *declared in an interface file*, but whose **own** declaration
  (`ExportedSymbol.File`) is **not** in an interface file → `METHOD_OUTSIDE_INTERFACE`
  violation. Any interface file satisfies the rule — a `types.go` holding the receiver
  type plus an `api.go` holding its methods is clean (Go allows methods in a different
  file than their type).
- **Explicit `init` rule (review A2):** an `ExportedSymbol` of `Kind == "init"`
  (an explicit `func init()`) declared in a **non-interface** file →
  `INIT_OUTSIDE_INTERFACE` violation. Importing a package runs its inits, so init
  behavior is de-facto interface. (Synthetic inits from var initializers are not
  `Kind=="init"` symbols and are governed by the component's own capability check, not
  here.)
- **Exemption:** concrete implementation methods of interface-file **interface types**
  are exempt from the method rule (contract-bound by the interface). In the MVP the
  interface-type implementation symbol set is computed by `goanalysis` via `go/types`
  and arrives with the facts in Step 9; this task keys the exemption off the available
  fact fields and must **not** flag an implementation method as
  `METHOD_OUTSIDE_INTERFACE` when its receiver type is an exported **interface** type
  (as opposed to a concrete type). Where the current `facts` model cannot yet
  distinguish an interface-type receiver, scope the method rule to receivers that are
  concrete exported types declared in interface files, matching the design's intent.

**Report-model note.** `report.ConformanceReport` has only `Violations` and `Warnings`
channels (no informational/notes section). The design says the checker *may* emit an
informational note listing architecture-private exported symbols; since there is no
notes channel, the **load-bearing behavior is that such symbols produce no violation
and no warning**. An informational listing is optional and, if added, must not go into
`Violations`/`Warnings` — defer it unless the report model gains a notes channel.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§5.3 declared interface + well-formedness FR4, incl. the method rule, the `types.go`/`api.go` split, the explicit-init rule, the interface-type implementation exemption, and architecture-private symbols)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 4)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. In `internal/checker`, add an FR4 rule that appends to the report produced by
   `Check` (do **not** change the `Inputs`/`Check` signatures from task-01).
2. Compute the declared-interface symbol set: the `ExportedSymbol`s across
   `Facts.Packages` whose `File` is one of `Manifest.InterfaceFiles`. Interface-file
   matching uses the same path convention the facts loader emits (relative to component
   root) — compare as the design specifies; keep it pure string matching.
3. Method rule → `report.MethodOutsideInterface`:
   - Identify exported symbols of method kind (`Kind == "method"`) whose `Receiver`
     names an exported **non-interface** type that is itself declared in an interface
     file.
   - If such a method's own `File` is **not** an interface file → emit
     `METHOD_OUTSIDE_INTERFACE` (message naming the method, its receiver type, and the
     file), with `Location` set to the method's file.
   - A method whose receiver type is declared in interface file A and whose own
     declaration is in interface file B (both in `InterfaceFiles`) → **no** violation.
   - Do **not** flag concrete implementation methods of interface-file **interface**
     types (the exemption above).
4. Explicit-init rule → `report.InitOutsideInterface`:
   - For each `ExportedSymbol` with `Kind == "init"` whose `File` is not an interface
     file → emit `INIT_OUTSIDE_INTERFACE` (message naming the package/file), `Location`
     at that file.
5. Architecture-private symbols (exported, not in any interface file, not caught by the
   rules above) → **no** violation and **no** warning. Optionally track them for a
   future informational listing, but do not add report entries.
6. Emit findings deterministically (stable ordering) for reproducible golden renders.
7. Keep the package pure (no I/O, no `go/ast`/`go/packages`).

## Dependencies
- task-01 (`checker` package scaffold, `Inputs`, `Check`, the FR3 rule and the shared
  report-assembly/determinism helpers).
- Step 3 `internal/facts` (`ExportedSymbol` with `Kind`/`Receiver`/`File`),
  `internal/report` (`MethodOutsideInterface`, `InitOutsideInterface`, `Finding`,
  `Location`).

## Implementation Approach
1. Build the interface-file set from `Manifest.InterfaceFiles` and an index of exported
   symbols (by name/kind/receiver/file) from `Facts.Packages`.
2. Determine which exported types declared in interface files are non-interface vs
   interface (using available `facts` fields; scope conservatively per requirement 3).
3. Apply the method rule and the explicit-init rule, appending violations.
4. Leave architecture-private symbols unreported (no violation/warning).
5. Wire the FR4 pass into `Check` after the FR3 pass, sharing the deterministic ordering.
6. Table-driven unit tests on hand-built `Manifest` + `facts.PackageFacts` (design §8)
   covering the cases below.

## Acceptance Criteria

1. **Method of interface-file type declared elsewhere → violation**
   - Given an exported method whose receiver is an interface-file-declared exported
     non-interface type, but whose own declaration file is not an interface file
   - When `Check` runs
   - Then exactly one `METHOD_OUTSIDE_INTERFACE` violation is emitted, naming the method
     and its receiver.

2. **`types.go`/`api.go` split → clean**
   - Given the receiver type declared in interface file `types.go` and its method
     declared in interface file `api.go` (both in `InterfaceFiles`)
   - When `Check` runs
   - Then no `METHOD_OUTSIDE_INTERFACE` violation is emitted.

3. **Interface-type implementation method → exempt**
   - Given a concrete method that implements an interface-file **interface** type,
     declared outside the interface files
   - When `Check` runs
   - Then no `METHOD_OUTSIDE_INTERFACE` violation is emitted for it.

4. **Explicit init outside interface files → violation**
   - Given an `ExportedSymbol` of `Kind == "init"` declared in a non-interface file
   - When `Check` runs
   - Then exactly one `INIT_OUTSIDE_INTERFACE` violation is emitted.

5. **Explicit init in an interface file → clean**
   - Given an explicit `init` declared in an interface file
   - When `Check` runs
   - Then no `INIT_OUTSIDE_INTERFACE` violation is emitted.

6. **Architecture-private symbol → non-violating**
   - Given an exported symbol declared in no interface file that is neither an
     offending method nor an explicit init
   - When `Check` runs
   - Then it produces no `Violations` and no `Warnings` entry.

7. **Composes with FR3**
   - Given inputs that trigger both an `UNDECLARED_DEPENDENCY` (task-01) and a
     `METHOD_OUTSIDE_INTERFACE`
   - When `Check` runs
   - Then both findings appear in the one report, deterministically ordered, and the
     rendered text shows both.

## Metadata
- **Complexity**: Medium
- **Labels**: pure-core, checker, FR4, well-formedness
- **Required Skills**: Go
