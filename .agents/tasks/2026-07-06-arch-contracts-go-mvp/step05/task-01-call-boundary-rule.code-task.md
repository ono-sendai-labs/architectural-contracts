# Task: Cross-component call-boundary rule (FR5)

## Description
Extend the pure `internal/checker` with the **cross-component call-boundary rule
(FR5, design §5.3b)** plus the two adjacent checks that share its inputs: the
higher-order boundary-call warning (review A3) and the package-overlap violation
(§5.5). After this task, `Check` inspects the injected `Facts.CallEdges` and the
resolved `DepIfaces` and reports:

- `CALLS_UNDECLARED_INTERFACE` — a call edge whose callee belongs to a component
  dependency `B`'s packages but is **not** in `B`'s declared-interface symbol set.
- `HIGHER_ORDER_BOUNDARY_CALL` **warning** — a call edge **into** a declared
  interface symbol of `B` whose call site passes a function-typed value
  (`CallEdge.PassesFuncValue`).
- `PACKAGE_OVERLAP` **violation** — a resolved dependency whose package set overlaps
  the component's own membership (one root nested inside the other).

`checker` stays a **pure function of injected data** — no I/O, no `go/packages`,
no call-graph construction. Real call edges and resolved interfaces arrive from the
shell in Step 9; here every test runs on hand-built `CallEdge`s and
`DependencyInterface`s (design §8). This task appends to the same
`report.ConformanceReport` that FR3/FR4 already populate; it must not alter their
existing findings.

## Background
Step 5 completes the pure core. The FR3 dependency rule and FR4 well-formedness
rules (Step 4) already live in `Check`. This task adds the first of Step 5's two
rule families — the boundary rule — leaving the policy-aware authority rule (FR6)
to task-02.

FR5 mechanics (design §5.3b, §5.5):
- A call edge is `facts.CallEdge{Caller, Callee capanalyzer.InterfaceSymbol,
  PassesFuncValue bool}`. The callee's owning package is determined by matching the
  callee symbol against each `DepIface.Packages` entry.
- A dependency `B` is `facts.DependencyInterface{Component string, Packages
  []string, Symbols []capanalyzer.InterfaceSymbol}`. `Symbols` is the already-resolved
  declared-interface symbol set per §5.3 (interface-file declarations plus
  interface-type implementation methods) — hand-built in this task, shell-resolved
  in Step 9.
- **Symbol matching uses the A4 normalization**: strip generic type-argument
  brackets and accept **both receiver forms** (`(T).M` and `(*T).M`) as equal. A
  callee that normalizes-equal to any symbol in `B.Symbols` is in the interface; a
  callee whose owning package is `B`'s but which is not in `B.Symbols` is a
  boundary violation.
- `PACKAGE_OVERLAP`: with directory-based membership (FR1), overlap means one
  component root lies inside the other's subtree, surfaced here as a
  component-membership package (from `Facts.Packages`) that also appears in a
  resolved `DepIface.Packages` set. That collision is a `PACKAGE_OVERLAP`
  violation (§5.5) because overlapping membership breaks authority attribution.
- MVP scope is intentionally **call-only**: type references, field access, and
  exported-variable use are documented limitations, not enforced here (§5.3b/§11).

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§5.3b cross-component interface boundary FR5 + higher-order warning; §5.5 component-dependency integrity / PACKAGE_OVERLAP; §5.3 declared-interface symbol set and A4 normalization)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 5)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend `internal/checker` only — no new package, no new imports beyond the
   existing pure-core set (`manifest`, `facts`, `capanalyzer`, `report`, stdlib).
   `checker` must remain pure (no I/O, no `go/packages`, no `go/ast`).
2. Add an A4-normalization helper for `capanalyzer.InterfaceSymbol` matching: strip
   generic type-argument brackets (`Foo[int]` → `Foo`) and treat pointer and value
   receiver forms as equal, so a callee matches a declared symbol under either form.
   Reuse/extend the existing `cleanReceiverType` logic where it applies.
3. Implement the **FR5 boundary rule** over `Facts.CallEdges`:
   - For each edge, find the dependency `B` (if any) whose `Packages` contains the
     callee's owning package. Edges to callees not owned by any resolved dependency
     package are out of scope (intra-component or stdlib) — no finding.
   - If the callee's owning package is `B`'s and the callee ∉ `B.Symbols` (under A4
     normalization) → `CALLS_UNDECLARED_INTERFACE` violation naming caller, callee,
     and `B.Component`.
   - If the callee ∈ `B.Symbols` **and** `edge.PassesFuncValue` →
     `HIGHER_ORDER_BOUNDARY_CALL` **warning** naming caller, callee, and `B.Component`.
4. Implement the **PACKAGE_OVERLAP** check (§5.5): if any package in the component's
   membership set (`Facts.Packages[].ImportPath`) also appears in a resolved
   `DepIface.Packages` set → one `PACKAGE_OVERLAP` violation per overlapping
   dependency, naming the dependency and the overlapping package(s).
5. Append findings to the same `report.ConformanceReport` FR3/FR4 build; do not
   change their behavior. Emit findings **deterministically** (stable sort) so
   golden text renderings stay reproducible.
6. Document, in doc comments, that this task implements FR5 (call-boundary +
   higher-order warning) and the §5.5 overlap check, that matching is call-only per
   MVP scope, and that real edges/resolved interfaces are injected by the shell
   (Step 9).

## Dependencies
- Step 4 `internal/checker` (`Inputs`, `Check`, FR3/FR4 findings) — extended here.
- Step 3 `internal/facts` (`CallEdge`, `DependencyInterface`), `internal/capanalyzer`
  (`InterfaceSymbol`), `internal/report` (`CallsUndeclaredInterface`,
  `HigherOrderBoundaryCall`, `PackageOverlap` kinds, `Finding`, `Location`).

## Implementation Approach
1. Add the A4-normalization matcher for `InterfaceSymbol` (strip generic brackets;
   both receiver forms equal); unit-test it directly on representative keys.
2. Build a `package → DepIface` lookup from the resolved `DepIfaces`, and a
   per-dependency normalized symbol set.
3. Iterate `Facts.CallEdges`: classify each callee (out-of-scope / declared-interface
   / undeclared-interface) and emit `CALLS_UNDECLARED_INTERFACE` or
   `HIGHER_ORDER_BOUNDARY_CALL` accordingly.
4. Compute package-set overlap between component membership and resolved dependencies;
   emit `PACKAGE_OVERLAP`.
5. Append to the existing violations/warnings slices; keep the existing deterministic
   sort covering the new findings.
6. Table-driven unit tests on hand-built `Facts.CallEdges` + `DepIfaces` (no build, no
   Capslock — design §8) covering the cases below.

## Acceptance Criteria

1. **Undeclared-interface call → violation**
   - Given a call edge whose callee is a Go-exported symbol of a component dependency
     `B` that is **not** in `B`'s declared-interface symbol set
   - When `Check` runs
   - Then exactly one `CALLS_UNDECLARED_INTERFACE` violation is emitted naming the
     caller, callee, and `B`.

2. **Declared-interface call → clean**
   - Given a call edge whose callee **is** in `B`'s declared-interface symbol set
   - When `Check` runs
   - Then no boundary violation is emitted for that edge.

3. **Normalization: generic + receiver forms → clean**
   - Given a callee that is a generic instantiation (bracketed) and/or the opposite
     receiver form of a symbol present in `B.Symbols`
   - When `Check` runs
   - Then it matches under A4 normalization and no violation is emitted.

4. **Higher-order boundary call → warning**
   - Given a call edge into a declared-interface symbol of `B` with
     `PassesFuncValue == true`
   - When `Check` runs
   - Then exactly one `HIGHER_ORDER_BOUNDARY_CALL` warning is emitted (exit-neutral)
     and no violation for that edge.

5. **Package overlap → violation**
   - Given a resolved dependency whose `Packages` set shares a package with the
     component's own membership
   - When `Check` runs
   - Then a `PACKAGE_OVERLAP` violation is emitted naming the dependency and the
     overlapping package.

6. **Out-of-scope edges → no finding**
   - Given call edges into intra-component or stdlib callees (no owning resolved
     dependency package)
   - When `Check` runs
   - Then no boundary finding is emitted for those edges.

7. **FR3/FR4 unaffected + deterministic**
   - Given inputs that also trigger FR3/FR4 findings
   - When `Check` runs repeatedly
   - Then FR3/FR4 findings are unchanged and the full findings order is stable
     (golden text render reproducible).

## Metadata
- **Complexity**: Medium
- **Labels**: pure-core, checker, FR5, call-boundary, package-overlap
- **Required Skills**: Go
