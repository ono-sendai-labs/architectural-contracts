# Task: Define Typed Reference Facts

## Description
Define the core-side reference, import, and source-site facts that will replace call edges, plus deterministic constructors and validation rules. This gives the scanner and pure checker one stable vocabulary without changing dependency resolution or production analysis yet.

## Background
Step 5 still feeds `facts.CallEdges` from SSA/VTA into the checker and runs Capslock at check time. Step 6 replaces that pipeline, but its observation vocabulary belongs in the pure `facts` core rather than in the shell scanner. The Step 3 `symbol.SymbolID` declaring-object grammar is the only identifier format edges may carry.

This data-model task is additive. Task 02 implements `ScanReferences` and deletes the implements-closure workaround in the same atomic change. This task does not change `Runner.runCheck`, dependency resolution, or legacy check behavior.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R1-R4, Reference semantics, Edge classification, and Findings and evidence)

**Additional References:**
- `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md` (current loader, SSA/VTA edge extraction, and measured cost)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend the core fact vocabulary with `ReferenceEdge`, `ImportEdge`, and a shared source-site value suitable for checker and report consumption. A reference edge must retain the referencing member package, the referenced object's `symbol.SymbolID`, and a component-relative file/line. An import edge must retain importing package, written canonical import path, source site, and enough resolution state to distinguish a real unowned package from missing type/layout data.
2. Define structural identity, validation, and total deterministic comparison for edges/sites. The same referent at the same site is an exact duplicate; a different site or object is distinct.
3. Make `symbol.SymbolID` the only reference identity. Do not add fields for AST spelling, instantiated generic brackets, SSA names, or dual pointer/value receiver keys.
4. Add a minimal effective-member set value/API, core-side if it contains no shell data, that supports exact canonical package membership and deterministic construction.
5. Keep legacy `CallEdge`, `StdlibImports`, and unresolved-import fields only as narrowly documented compatibility data until the Task 05 cutover; do not convert between call edges and reference edges.
6. Keep the production path, dependency resolution, implements closure, and `NeedDeps` load mode unchanged in this task. No new core type may import Capslock, `go/packages`, AST, SSA, or callgraph packages.
7. Update `facts` Component Contract comments, BUILD metadata, and component manifests for the additive pure-core vocabulary.

## Dependencies
- Plan Step 3: `symbol.SymbolID` and its `go/types` declaring-object conversion.
- Plan Step 5: the current member facts and layout-backed loading topology.
- No Step 6 task dependency; this is the first task and remains additive.

## Implementation Approach
1. Define the edge, site, and member-set DTOs core-side, with comments documenting provenance and ordering.
2. Add pure validation/comparison helpers needed by scanners and report canonicalization.
3. Add table-driven core tests for identity, sorting, invalid values, and defensive construction.

## Acceptance Criteria

1. **Reference identity is canonical**
   - Given reference facts for functions, methods, fields, variables, constants, types, aliases, and interface method specs
   - When they are validated
   - Then every referent is a grammar-valid `SymbolID` with a canonical component-relative source site

2. **Structural duplicate keys are precise**
   - Given two equal reference/import observations and observations differing only by referent or source position
   - When their identity keys are compared
   - Then only values equal in every semantic field deduplicate

3. **Import facts retain written-edge state**
   - Given resolved and missing-type-data import observations, including blank imports
   - When they are represented as `ImportEdge`
   - Then callers can distinguish those states without consulting legacy package-import slices

4. **Member identity is exact**
   - Given canonical member package paths and similar prefixes
   - When member-set membership is queried
   - Then only exact packages match and deterministic construction rejects invalid duplicates or noncanonical identities as designed

5. **Fact ordering is deterministic**
   - Given the same facts in different input orders
   - When the core comparison/sort helpers are applied
   - Then edge and site order is identical and exact duplicates can be removed without losing distinct sites

6. **The core stays shell-free**
   - Given the updated facts package
   - When its imports and component contract are inspected
   - Then it imports no AST, `go/packages`, SSA, callgraph, or Capslock package and holds no ambient authority

7. **Legacy execution is unchanged**
   - Given the repository before the production cutover
   - When `just ci` runs after this task
   - Then all existing checks and goldens pass, the implements closure and `NeedDeps` remain enabled, and the runtime check still follows the legacy path

## Metadata
- **Complexity**: High
- **Labels**: go, symbol-id, facts, data-model, determinism, keystone
- **Required Skills**: Go API/data-model design, architectural boundary modeling, deterministic table-driven testing
