# Task: Emit Export-Data Package Layouts

## Description
Wire the ordinary-package and standard-library export metadata into Bazel-produced package layouts and stage the complete export-data closure for the checked action. Preserve the current source-backed loader during this task so the commit remains independently buildable; Task 5 performs the behavioral cutover.

## Background
Tasks 1-3 establish the Go-side layout contract and the two Bazel sources of export data. The component emitter must now combine them into one effective package graph: member packages retain source, non-member packages carry `ExportFile`, and every reachable package retains exact direct import edges. The action may temporarily retain closure source inputs because `NeedDeps` is removed only in Task 5, but the newly emitted export layout must already validate completely and deterministically.

Standard-library metadata may be generated rather than analysis-time-readable. The integration must still present `packagelayout` with the design's effective per-package `ExportFile` and full graph before `packages.Load`, without adding a second component-analysis action or executing a toolchain binary.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1-N2, I5, DR-02, §Build topology, §Type loading)

**Additional References:**
- Export-data spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-export-data-loading.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend `_layout_content` and the runtime layout-resolution path so each component root retains only its selected source role and each reachable non-member package obtains the exact export file collected in Tasks 2-3.
2. Preserve the complete transitive import graph for ordinary and stdlib packages. Export-backed package `Imports` must be explicit, deterministic, and resolvable before loading; source import recovery remains a member-only facility.
3. Merge the adapter-provided stdlib descriptor into the effective layout before the Task 1 export-data validator runs. Standard-library provenance must come from the toolchain descriptor, never an import-path predicate.
4. Stage every referenced export artifact and graph descriptor as a declared checked-action input and in the wrapper's runfiles path frame. Continue staging closure sources temporarily in this task so the still-active `NeedDeps` loader remains green.
5. Reject a missing package export, duplicate/conflicting package identity, incomplete graph, unsafe artifact path, or target-configuration mismatch before invoking `packages.Load`, with diagnostics naming the component and package.
6. Keep dependency surfaces/reports, the manifest/layout, and stdlib map wiring unchanged. Do not introduce an extra analysis action or rely on a dependency's `.check` test having run.
7. Ensure layout bytes and the effective merged graph are stable across dependency/aspect iteration order and repeated builds.
8. Add Go integration coverage for parsing/resolving the emitted shape and Bazel analysis tests proving all ordinary and stdlib export artifacts are declared while the transitional source inputs remain intentionally present.

## Dependencies
- Task 1: Add Export-Data Layout Contract.
- Task 2: Collect Go Archive Export Files.
- Task 3: Expose Standard-Library Export Data.

## Implementation Approach
1. Extend the component emitter's generic package records with export-file paths and exact import edges.
2. Connect the stdlib adapter descriptor to the active layout resolution path, materializing one validated effective graph in Go before loading.
3. Update action inputs and symlink-frame construction so every emitted path resolves inside the sandbox.
4. Add determinism and failure-path tests while leaving the legacy load mode active until Task 5.

## Acceptance Criteria

1. **Every non-member has export data**
   - Given a Bazel component with ordinary and stdlib packages in its transitive closure
   - When its package layout is resolved in the check sandbox
   - Then every reachable non-member other than `unsafe` has a real `ExportFile` and an explicit import map.

2. **Members retain source ownership**
   - Given the same component
   - When the effective layout is inspected
   - Then component roots retain their member Go files and are not replaced by their own archives.

3. **Full graph is preserved**
   - Given a deep dependency whose exported API does not mention one of its own imports
   - When the layout is emitted and validated
   - Then that import and its transitive node remain in the graph with export data.

4. **All staged paths resolve hermetically**
   - Given external-repository ordinary archives and target stdlib exports
   - When the checked action enters its runfiles frame
   - Then every layout export path resolves to a declared input without an absolute or parent-escaping path.

5. **Invalid metadata fails before load**
   - Given a missing export file, incomplete graph, conflicting package record, or configuration mismatch
   - When the component check starts
   - Then it exits as a tool error naming the component and offending package before `packages.Load` executes.

6. **Transitional load remains green**
   - Given export layouts are now emitted but `NeedDeps` has not yet been removed
   - When `just ci` runs
   - Then existing checks continue to pass using the temporarily retained closure sources.

## Metadata
- **Complexity**: High
- **Labels**: bazel, go, packagelayout, export-data, hermeticity
- **Required Skills**: Starlark, Go, Bazel action inputs/runfiles, `go/packages` driver protocol, deterministic graph processing
