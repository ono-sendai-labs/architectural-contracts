# Task: `checker` scaffold + dependency rule (FR3)

## Description
Stand up the pure `internal/checker` package — the heart of the tool — with its
`Inputs` struct and `Check(Inputs) → report.ConformanceReport` entry point, then
implement the **first Pillar-1 rule: the dependency allowlist rule (FR3)**. This
produces the first real (partial) conformance report: an import that no declared
dependency covers surfaces as an `UNDECLARED_DEPENDENCY` violation, and a declared
dependency that matches no import surfaces as an `UNUSED_DEPENDENCY` warning.

`checker` is a **pure function of injected data** — no I/O, no globals, no
filesystem, no `go/packages`. The authority-holding loaders that produce its
`facts`/`DepIfaces` inputs arrive in the shell (Steps 6/9); here every test runs on
hand-built inputs (design §8). This task establishes the compositional skeleton that
task-02 (FR4) plugs its findings into.

## Background
Step 4 begins the pure core. Of the checker's rule families, this task covers the two
that need only import/symbol facts and the manifest — no call graph and no capability
findings (those are Step 5). The `Inputs` struct and `Check` signature defined here
are the seam every later rule (FR4 in task-02; FR5/FR6 in Step 5) extends by appending
to the returned report's `Violations`/`Warnings`.

FR3 mechanics (design §5.1):
- **Allowed import set** = union of (a) every direct **component dependency's**
  interface packages — supplied already-resolved as `DepIface.Packages` on the
  injected `[]facts.DependencyInterface` (the shell derives these from each
  dependency's own manifest in Step 9; hand-built here) — and (b) every **absorbed
  dependency's** import-path pattern, glob-matched against literal import strings
  (pure string matching, no filesystem).
- **Membership** = the packages under the component root = the import paths present in
  `Facts.Packages` (FR1). An import that is itself a component package is intra-component
  and never a violation.
- For each component package, for each **non-stdlib** direct import that is **not**
  a component package: if it is not in the allowed set → `UNDECLARED_DEPENDENCY`
  violation (carrying the importing package + the import path). **Stdlib imports are
  auto-allowed at Pillar 1** (design §5.2 — `PackageFact.IsStdlib` gates this; ambient
  authority in stdlib is governed by Pillar 3, not here).
- A declared component or absorbed dependency matching **no** import of the component's
  packages → `UNUSED_DEPENDENCY` **warning** (review C13).

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§4.1 `checker.Inputs`/`Check` signature; §5.1 dependency rule FR3; §5.2 why stdlib is auto-allowed at Pillar 1)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 4)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Create package `internal/checker` — pure: no I/O, no globals, no filesystem,
   no `go/packages`/`go/ast` imports. Imports only the project's pure core packages
   (`internal/manifest`, `internal/facts`, `internal/capanalyzer`, `internal/report`).
2. Define `Inputs` per design §4.1:
   - `Manifest manifest.Manifest`
   - `Facts facts.PackageFacts`
   - `DepIfaces []facts.DependencyInterface` (resolved direct component deps)
   - `Caps []capanalyzer.CapabilityFinding` (unused by this task; consumed in Step 5)
   - `Policy capanalyzer.CapabilityPolicy` (unused by this task; consumed in Step 5)
3. Define `Check(in Inputs) report.ConformanceReport`. It initializes the report
   (`Component` from `Manifest.Name`, empty `Violations`/`Warnings` slices) and, in
   this task, applies only the FR3 rule. Later rule families append to the same report.
4. Implement FR3 (design §5.1):
   - Build the component-membership set from `Facts.Packages[].ImportPath`.
   - Build the allowed-import set: union of every `DepIfaces[].Packages` entry plus a
     glob match of each `Manifest.AbsorbedDependencies[].ImportPath` pattern against
     literal import strings. Use `path.Match` (or equivalent pure glob) — no filesystem.
   - For each package in `Facts.Packages`, for each entry in its `Imports`: skip if the
     package `IsStdlib` **or** the import is a component member; otherwise, if the import
     is not allowed → append an `UNDECLARED_DEPENDENCY` violation
     (`report.UndeclaredDependency`) whose message names the importing package and the
     offending import, with `Location.File` set to the importing package where practical.
   - Track which declared component/absorbed dependencies matched at least one import;
     any that matched none → append an `UNUSED_DEPENDENCY` warning
     (`report.UnusedDependency`).
5. Emit findings **deterministically** (stable ordering — e.g. sorted by package then
   import) so golden text renderings are reproducible.
6. Document, in package/func doc comments, that `checker.Check` is a pure function of
   its inputs (the basis for the `checker` component being ambient-authority-free) and
   that this task implements FR3 only; FR4/FR5/FR6 extend the same `Check`.

## Dependencies
- Step 2 `internal/manifest` (`Manifest`, `ComponentDependency`, `AbsorbedDependency`).
- Step 3 `internal/facts` (`PackageFacts`, `PackageFact`, `DependencyInterface`),
  `internal/capanalyzer` (`CapabilityFinding`, `CapabilityPolicy`), `internal/report`
  (`ConformanceReport`, `Finding`, `Kind`, `Location`).

## Implementation Approach
1. Define `Inputs` and the `Check` skeleton returning a report seeded from `Manifest.Name`.
2. Add a small pure helper that classifies an import: stdlib (skip), component member
   (skip), allowed (ok), else violation.
3. Build the allowed set from `DepIfaces` packages ∪ absorbed-dependency glob patterns.
4. Iterate packages/imports, collect `UNDECLARED_DEPENDENCY` violations, and record
   dependency match hits.
5. After the sweep, emit `UNUSED_DEPENDENCY` warnings for unmatched declared deps.
6. Sort findings for determinism.
7. Table-driven unit tests on hand-built `Manifest` + `facts.PackageFacts` + `DepIfaces`
   (no build, no Capslock — design §8) covering the cases below.

## Acceptance Criteria

1. **Pure and inward-only**
   - Given `internal/checker`
   - When built and its imports inspected
   - Then it compiles and imports only project pure-core packages (`manifest`, `facts`,
     `capanalyzer`, `report`) and stdlib — no `go/packages`, no `go/ast`, no I/O.

2. **Conforming imports → clean**
   - Given a component whose every non-stdlib import is covered by a `DepIface` package
     or an absorbed-dependency pattern (and stdlib imports present)
   - When `Check` runs
   - Then the report has no `UNDECLARED_DEPENDENCY` violations.

3. **Undeclared non-stdlib import → one violation**
   - Given a component package importing a non-stdlib path in neither the component nor
     the allowed set
   - When `Check` runs
   - Then exactly one `UNDECLARED_DEPENDENCY` violation is emitted, naming the importing
     package and the import path.

4. **Stdlib import → no violation**
   - Given a component importing a stdlib package (its `PackageFact.IsStdlib` true or the
     import classified as stdlib) that is not declared
   - When `Check` runs
   - Then no `UNDECLARED_DEPENDENCY` is emitted (Pillar-1 auto-allow, §5.2).

5. **Intra-component import → no violation**
   - Given a package importing another package that is itself a member of `Facts.Packages`
   - When `Check` runs
   - Then no `UNDECLARED_DEPENDENCY` is emitted for that import.

6. **Absorbed dependency glob match → allowed**
   - Given an absorbed dependency pattern that glob-matches an imported path
   - When `Check` runs
   - Then that import is allowed (no violation) and the absorbed dependency counts as used.

7. **Unused declared dependency → warning**
   - Given a declared component or absorbed dependency that matches no import
   - When `Check` runs
   - Then exactly one `UNUSED_DEPENDENCY` warning is emitted for it, and the report's
     warnings-only state does not include any violation.

8. **Deterministic output**
   - Given the same inputs
   - When `Check` runs repeatedly
   - Then the findings order is stable (golden text render reproducible).

## Metadata
- **Complexity**: Medium
- **Labels**: pure-core, checker, FR3, dependency-rule
- **Required Skills**: Go
