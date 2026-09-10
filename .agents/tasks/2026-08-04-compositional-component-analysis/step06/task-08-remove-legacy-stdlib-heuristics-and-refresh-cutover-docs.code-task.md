# Task: Remove Legacy Stdlib Heuristics and Refresh Cutover Docs

## Description
Finish the Step 6 stdlib-classification cutover by removing the remaining
host-policy path heuristic from the production layout-loading path and by
rewriting stale comments that still describe call edges or check-time Capslock
traversal pruning. Explicit layout/SDK provenance may continue to drive file
resolution, while the authority map remains the only source of standard-library
membership for check decisions.

## Background
Addresses findings F2 and F4 from the Step 6 implementation review. The checker
already dispatches stdlib references and imports exclusively through
`StdlibAuthority`, but `hostpolicy.IsStdlibPath` remains a public heuristic used
by `packagelayout.Parse`. This preserves a second stdlib identity source despite
the design's deletion inventory. Several user-facing example and component
comments also describe the removed Capslock/prune implementation; three are
residue explicitly deferred from task 05, while task 06 already corrected the
two generation-batching comments it touched.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R5-R6, Edge classification, Canonical namespace, Changed components, What is deleted)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step06.yaml`

**Additional References:**
- `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Remove `hostpolicy.IsStdlibPath` as a production API and remove its use from
   package-layout parsing and validation. Do not replace it with another import-
   path spelling heuristic.
2. Preserve the layout loader's resource-resolution needs through explicit
   emitter provenance (`is_stdlib`), SDK discovery results, and validated layout
   identities. `Layout.IsStdlibPackage` may remain only as an accessor for that
   structural provenance; it must not infer membership from path spelling.
3. Preserve fail-closed behavior for missing SDK inputs and missing type/layout
   data. A declared/map-known stdlib import with missing data must still become
   a tool error rather than SAFE or `UNDECLARED_DEPENDENCY`.
4. Add regressions proving a dotless non-stdlib import path is not treated as
   stdlib and an explicitly identified stdlib package is resolved correctly.
   Include a host canonicalization case so removal of the override cannot
   reintroduce rewritten-path ambiguity.
5. Audit production code outside stdlib-map generation: every authority or
   boundary decision about stdlib membership must flow through
   `StdlibAuthority.IsStdlibPackage`, `SymbolAuthority`, or
   `PackageInitAuthority`.
6. Update the hostpolicy package contract and affected component manifests,
   BUILD declarations, and tests after removing the API.
7. Replace stale cutover terminology in at least these locations:
   `go/internal/goanalysis/goanalysis.go`'s LoadPackageFacts comment,
   `go/internal/goanalysis/fixture_test.go`'s fixture-2 comment,
   `go/internal/stdlibmap/component.textproto`,
   `go/examples/csvtool/app/app.go`,
   `go/examples/csvtool/csvfile/csvfile.go`,
   `go/examples/csvtool/app/BUILD.bazel`, and README.md's
   `declared_authority` description.
8. Describe the current mechanism consistently: member typed references and
   imports are classified through the total map; declared component boundaries
   terminate authority structurally; Capslock is generation-only.

## Dependencies
- Step 6 tasks 01-06.
- May be implemented after task 07 so searches and documentation reflect the
  corrected complete reference scanner, but it does not depend on task 09.

## Implementation Approach
1. Pin current layout behavior with structural-provenance tests, including the
   dotless and rewritten-path cases.
2. Remove the heuristic API and route each loader use to explicit provenance or
   a fail-closed missing-data path.
3. Audit stdlib membership call sites and update package/component metadata.
4. Rewrite the stale comments as one documentation pass and run repository-wide
   searches for the deleted traversal/pruning claims.

## Acceptance Criteria

1. **No path heuristic decides standard-library identity**
   - Given the repository after remediation
   - When production Go and Starlark sources are searched
   - Then `hostpolicy.IsStdlibPath` and equivalent dot-in-first-segment stdlib
     predicates are absent, and checker membership decisions use the authority
     map port

2. **Layout resolution uses explicit provenance**
   - Given layouts containing an explicitly identified stdlib package and a
     dotless host/non-stdlib package
   - When each layout is parsed and served through the driver
   - Then the stdlib package resolves from its declared SDK provenance, the
     dotless package remains non-stdlib, and missing required data fails closed

3. **Host canonicalization remains unambiguous**
   - Given a non-identity but idempotent canonical path policy
   - When a layout containing rewritten non-stdlib packages is validated
   - Then structural provenance is honored without a second path-based stdlib
     verdict or a false standard-library classification

4. **Cutover documentation describes the current architecture**
   - Given the README, csvtool contracts/BUILD comment, goanalysis loader and
     fixture comments, stdlibmap component contract, and hostpolicy contract
   - When they are read after the change
   - Then none claims that a check invokes Capslock, prunes call-graph traversal,
     emits loader-side standard-library facts, or awaits an already-completed
     step

5. **Repository gates pass**
   - Given the completed removal and documentation repair
   - When `CGO_ENABLED=0 just ci` runs
   - Then all routine native, self-hosting, and Bazel checks pass without
     weakening explicit stdlib provenance or fail-closed behavior

## Metadata
- **Complexity**: High
- **Labels**: go, package-layout, host-policy, stdlib-map, architecture, documentation, remediation
- **Required Skills**: Go package loading, hermetic layout design, host-policy seams, architectural documentation
