# Task: Centralize x/tools Ownership

## Description
Create one in-tree asserted `PACKAGE_SURFACE` wrapper for the retained `golang.org/x/tools` package set, migrate self-hosting analyzers and generators away from declaring those packages as members, and remove the goanalysis Step 6 exemption without reclassifying stdlib findings.

## Background
`goanalysis` currently owns its `x/tools` closure as member code because the source-backed resolver could not consume a foreign-member wrapper. That makes honest `UNANALYZED` references inside x/tools fail the component's own check and required the Step 6 AC8b exemption. The surface cutover makes the intended boundary possible: x/tools authority and implementation stay behind one explicit package-surface component, while `goanalysis` owns and checks only its code plus deliberate local members.

Other native self-hosting components (`capslockadapter`, `stdlibmap`, and any additional owners found by audit) also enumerate overlapping x/tools/support packages. The migration must establish one owner repository-wide, not merely make the goanalysis verdict green.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R7-R8, I1, I6, §Changed: goanalysis, §Migration Strategy)

**Additional References:**
- Plan Step 7: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Post-cutover x/tools findings and load costs: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Step 6 exemption inventory: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step06.yaml`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add one in-tree `x-tools` component with `PACKAGE_SURFACE` style and a concrete, sorted, duplicate-free list of every retained `golang.org/x/tools/...` package used as member code by the repository. Include required `x/mod` or `x/sync` support packages in this boundary, or model them through another already-owned explicit boundary, so the wrapper itself has no undeclared package edge.
2. Produce the wrapper through the component-level asserted/manual path (`tags = ["manual"]`, not merely a tagged `.check` test) with no analysis action or report, provider provenance `asserted`, authority `UNKNOWN`, no symbols, and the empty digest. Provide a deterministic native convention surface for the pinned selfcheck target just as for the protobuf wrapper.
3. Migrate `goanalysis` off all external x/tools/support member declarations and add the wrapper as an explicit component dependency. Preserve intentional local ownership of `goanalysis`, `hostpolicy`, and `packagelayout` unless a separately declared in-tree component already owns one; do not accidentally move project packages into the foreign wrapper.
4. Audit and migrate overlapping ownership in `capslockadapter`, `stdlibmap`, and any other checked-in component manifests. Every x/tools package must have one component owner after the change, and component dependency direction must remain acyclic.
5. Keep BUILD and checked-in manifest declarations in parity. The wrapper's Bazel labels must map exactly to its import-path members, including internal packages needed by the retained public packages.
6. Remove goanalysis's `check_tags = ["manual"]` exemption and restore its native selfcheck command. Delete the Step 6 x/tools exemption comments/counts; do not add a warning policy or weaken `UNANALYZED` classification in this task.
7. Prove goanalysis's check no longer attributes x/tools stdlib references to goanalysis and that its boundary is resolved solely from the persisted wrapper surface with x/tools source unreadable or absent from the resolver.
8. Re-inventory `capslockadapter` after both foreign-wrapper migrations and record the exact residual `AnalysisDefeating` set owned by Capslock/other non-wrapper members. Keep its residual exemption until Task 6; do not assume protobuf or x/tools migration clears it.
9. Update Component Contract blocks, ownership/selfcheck comments, manifests, BUILD metadata, and run `just ci` before committing.

## Dependencies
- Task 3 supplies persisted surface consumption and overlap checks.
- Task 4 establishes the analogous protobuf wrapper first, simplifying the residual capslockadapter inventory.
- Task 6 consumes this task's residual inventory to make the explicit `UNANALYZED` policy decision.

## Implementation Approach
1. Index the union of x/tools, x/mod, and x/sync members currently claimed by self-hosting components and decide the exact wrapper boundary without patterns.
2. Add the asserted wrapper target, checked-in manifest/native surface, and parity/ownership tests.
3. Migrate goanalysis, then every other duplicate owner, maintaining explicit dependencies and acyclic component edges.
4. Restore goanalysis checks, rerun capslockadapter to capture the residual set, and update documentation without changing classifier semantics.

## Acceptance Criteria

1. **Retained x/tools packages have one owner**
   - Given all checked-in component manifests and Bazel declarations
   - When package ownership is indexed
   - Then every retained x/tools/support package belongs to exactly one explicit package-surface wrapper and no consumer duplicates that membership

2. **Goanalysis checks only its side of the boundary**
   - Given the migrated goanalysis component
   - When native and Bazel checks run
   - Then x/tools references resolve through the wrapper, x/tools stdlib findings are not attributed to goanalysis, and the component passes without a manual check exemption

3. **The wrapper is asserted, not falsely certified**
   - Given a report for a component consuming x-tools
   - When boundary status is inspected
   - Then the wrapper is `ASSERTED`, has `UNKNOWN` authority and renders untrusted, with no report or content-derived symbol/digest claim

4. **Dependency source remains unnecessary**
   - Given the x-tools surface and unavailable x/tools source in the dependency-resolver path
   - When goanalysis is checked
   - Then boundary resolution succeeds from exact package ownership without parsing x/tools files

5. **Residual findings remain honest**
   - Given capslockadapter after protobuf and x-tools packages move behind wrappers
   - When its strict check is run for inventory
   - Then every remaining `AnalysisDefeating` site is recorded as residual member-code evidence for Task 6 and none is silently made safe or dropped

6. **Repository gates pass**
   - Given manifest parity, selfcheck, generator code, and Bazel analysis actions
   - When `just ci` runs
   - Then all pass except the deliberately retained capslockadapter and parsecsv residual-policy exemptions owned by Task 6

## Metadata
- **Complexity**: High
- **Labels**: go, bazel, components, x-tools, package-surface, ownership, asserted-surface, selfcheck
- **Required Skills**: Go module dependency analysis, component ownership modeling, Starlark labels/providers, manifest parity, static-analysis integration
