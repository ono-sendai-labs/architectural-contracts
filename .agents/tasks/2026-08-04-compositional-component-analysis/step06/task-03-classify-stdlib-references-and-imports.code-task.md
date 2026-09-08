# Task: Classify Stdlib References and Imports

## Description
Add the two independent classification dispatches for typed object references and imports, resolving standard-library membership, symbol authority, init authority, and evidence exclusively through the Step 4 `StdlibAuthority` port. Complete the design fixtures that prove the new scanner's semantics before production cutover.

## Background
The old checker treats imports as package strings, relies on loader-side stdlib predicates, and asks Capslock to infer ambient authority transitively. In the new model, an object reference performs an exact total-map lookup, while an import performs only the package-init lookup. Dependency boundaries terminate authority and are classified separately from stdlib. The map's package enumeration—not path spelling—is the definition of standard library.

This remains pre-cutover work: tests drive the scanner and new classifier directly. At the end of this task, design fixtures 1-6 and 9 must all pass against the new path with the implements-closure workaround already deleted by Task 02.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R1-R6, Edge classification, Error Handling, regression fixtures 1-6 and 9)

**Additional References:**
- `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md` (map terminal states, globals, init behavior, and laundering cases)
- `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md` (legacy stdlib and Capslock behavior being replaced)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Introduce a deterministic analysis/classification operation that accepts scanned references/imports, effective member packages, resolved direct dependency interfaces, a `stdlibauthority.StdlibAuthority`, and the policy-ready fact model. Keep shell I/O and artifact decoding outside the pure checker.
2. Implement object-reference dispatch exactly as designed: member → ignore; stdlib package → exact `SymbolAuthority(SymbolID)` lookup; uniquely owned dependency → package-surface pass or exact declared-symbol comparison; otherwise → site-specific `UNDECLARED_DEPENDENCY`.
3. Implement import dispatch separately: member → ignore; stdlib-map member → `PackageInitAuthority`; uniquely owned declared or auto-attached dependency → pass and mark used; unowned resolved package → `UNDECLARED_DEPENDENCY`; missing type/layout data for a written declared/stdlib import → actionable tool error.
4. Convert `Classification{Capabilities: ...}` into one `TrueAuthority` observation per capability and `Classification{Unanalyzed: true}` into an `AnalysisDefeating` observation. `Safe` contributes no capability. Preserve the referencing site and `SymbolID`; for imports, use the aggregate `pkg.init` identity.
5. Attach evidence only through `StdlibAuthority.Evidence` for the relevant symbol/init and capability, and carry the map's `SDKKey` with authority observations. Never invoke Capslock or inspect stdlib/dependency source in this operation.
6. Propagate inventory gaps, invalid terminal classifications, missing init entries, and key mismatches as tool errors. Do not reinterpret them as SAFE, `UNDECLARED_DEPENDENCY`, or an analysis warning.
7. Remove the five legacy stdlib path/predicate decisions from the new classifier. Their production callers remain until Task 05, but no new path predicate may influence a scanner fixture.
8. Create the full design fixtures 1-6 and 9 against real typed source and a deterministic fake/read map. Avoid test-only shortcuts that construct only legacy `CallEdge` values.

## Dependencies
- Task 01: reference/import scan and sites.
- Task 02: exact dependency symbol sets, declaring-object boundary classification, and deleted implements closure.
- Plan Step 4: `StdlibAuthority`, artifact reader, total inventory, evidence, and SDK key.

## Implementation Approach
1. Define a small result type separating reportable boundary/capability observations from fatal tool errors.
2. Build validated member, dependency-owner, and map-membership indexes before dispatch; sort every emitted collection.
3. Drive both dispatches through table tests corresponding one-for-one to the design diagrams/tables.
4. Add integration-style typed fixtures for semantic cases, using a fake authority port for targeted edge behavior and the real artifact reader for totality/error behavior.

## Acceptance Criteria

1. **Function values carry stdlib authority**
   - Given design fixture 1, `f := os.ReadFile` where `f` is never called
   - When the package is scanned and classified
   - Then the `os.ReadFile` reference contributes FILES authority at that source site

2. **Blank imports classify aggregate init**
   - Given design fixture 2, a blank import of a map-enumerated package whose aggregate init carries authority
   - When imports are classified
   - Then the package's init authority is attributed without a symbol lookup or call edge

3. **Every import-table row is pinned**
   - Given design fixture 6 with blank imports for member, stdlib, declared dependency, auto-attached dependency, overlapping direct dependencies, unowned resolved package, and declared/stdlib package with missing type data
   - When each case is classified
   - Then outcomes match all seven design rows exactly, including used-edge provenance, `DEPENDENCY_OVERLAP`, `UNDECLARED_DEPENDENCY`, and fail-closed layout/type-data errors

4. **Stdlib globals use symbol classifications**
   - Given design fixture 9
   - When member source references `os.Stdin` and `io.EOF`
   - Then `os.Stdin` contributes its non-SAFE authority and `io.EOF` contributes none, based solely on exact map records

5. **Inventory and key faults fail closed**
   - Given a reference to an enumerated stdlib package whose symbol or init record is absent, an invalid classification, or a map whose SDK key does not match the target
   - When classification runs
   - Then it returns an actionable tool error and emits no partial conformance verdict

6. **Reference and import rules stay separate**
   - Given a dependency package with an init and a referenced object outside its declared symbol surface
   - When both the import and reference are classified
   - Then the import is accepted behind the component boundary while the object reference independently produces `CALLS_UNDECLARED_INTERFACE`

7. **Pre-cutover fixture gate is complete**
   - Given the repository after Tasks 01-03
   - When the Step 6 scanner suites run
   - Then design fixtures 1, 2, 3, 4, 5, 6, and 9 pass in full against AST + `types.Info`, the implements-closure code is absent, and the production `Runner` has not yet been switched

8. **No legacy analysis leaks into classification**
   - Given the new classifier packages
   - When their imports and test seams are inspected and `just ci` runs
   - Then they contain no Capslock, SSA, VTA, callgraph, or stdlib path-predicate dependency

## Metadata
- **Complexity**: High
- **Labels**: go, stdlib-map, imports, capability-classification, fail-closed, fixtures, keystone
- **Required Skills**: Go architectural analysis, port/adaptor design, deterministic error handling, integration testing
