# Task: Collect Go Archive Export Files

## Description
Extend the Bazel Go adapter and closure aspect so every non-standard-library package node carries the compiler export artifact produced by its pinned `rules_go` archive. Keep this task additive: expose and validate the metadata without changing the emitted layout or analysis action inputs yet.

## Background
The component aspect already projects each Go target into an import-path-keyed closure containing sources, direct imports, and cgo state. Member-only type loading also needs the corresponding compiled export file for every reachable non-member package. In pinned `rules_go` 0.61.1 the field is `GoArchive.data.export_file`; the adapter is the only repository layer permitted to depend on that ruleset-specific shape.

Embedded libraries can contribute duplicate nodes for one import path. Their metadata must merge deterministically, and conflicting export artifacts for one effective package must fail during analysis rather than being selected by traversal order.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1-N2, DR-02, §Type loading, §Changed: Bazel rules and CLI)

**Additional References:**
- Export-data spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-export-data-loading.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Verify against the pinned `rules_go` 0.61.1 provider implementation that the package export artifact is `GoArchive.data.export_file`, and isolate that access in `bazel_rules/go/private/go_adapter.bzl`.
2. Extend the adapter's generic Go package projection with an `export_file` `File` while preserving the existing import path, source, dependency, and cgo fields.
3. Propagate export artifacts through `_arcc_deps` for the complete `deps`/`embed` closure; non-Go targets must continue to contribute an empty `ArccPackageInfo` rather than failing.
4. Extend `merge_by_importpath` so one effective package has exactly one deterministic export artifact. Identical duplicates from embedding are accepted; absent or conflicting artifacts fail with a diagnostic naming the import path and contributing labels/files.
5. Keep ruleset-specific providers and field names below `go_adapter.bzl`; `aspect.bzl` and `component.bzl` must consume only the adapter's generic node contract.
6. Do not add export files to layouts, runfiles, or analysis actions in this task. The repository must continue using the Step 7 source-backed load path until Tasks 3-5 complete the consumer side.
7. Add Starlark analysis tests covering a normal transitive closure, embed-folded duplicates, a non-Go edge, deterministic merge ordering, and the fail-closed missing/conflicting-artifact cases available through test seams.

## Dependencies
- Task 1: Add Export-Data Layout Contract defines the eventual consumer contract and terminology.

## Implementation Approach
1. Add one rules_go-specific accessor/projection field in `go_adapter.bzl` and document the pinned provider assumption beside it.
2. Carry the generic `File` through the aspect node and import-path merge without changing public component outputs.
3. Extend existing aspect/component analysis-test fixtures rather than adding a second closure walker.

## Acceptance Criteria

1. **Every Go closure node carries export data**
   - Given a component root with direct and transitive Go dependencies
   - When `_arcc_deps` projects the closure
   - Then each importable non-stdlib package node exposes the exact `GoArchive.data.export_file` through the generic adapter contract.

2. **Embedding merges deterministically**
   - Given embedder and embedded targets that fold to one import path and share an export artifact
   - When `merge_by_importpath` runs in either traversal order
   - Then the merged node contains one stable export file and otherwise identical metadata.

3. **Conflicts fail closed**
   - Given two nodes for one import path with missing or distinct export artifacts
   - When the closure is merged
   - Then analysis fails with a diagnostic naming the package and conflicting metadata rather than choosing one by iteration order.

4. **Non-Go traversal stays compatible**
   - Given a filegroup or other non-Go target on a traversed edge
   - When the aspect visits it
   - Then it yields an empty package depset and does not attempt to read `GoArchive`.

5. **Layering is preserved**
   - Given the completed Starlark change
   - When imports are inspected and Bazel tests run
   - Then only `go_adapter.bzl` loads rules_go providers and upper layers use the generic projection.

6. **Repository checks pass**
   - Given the completed change
   - When `just ci` runs
   - Then all existing checks pass while the runtime load path remains source-backed.

## Metadata
- **Complexity**: Medium
- **Labels**: bazel, starlark, rules-go, aspect, export-data
- **Required Skills**: Starlark, Bazel providers/aspects, rules_go provider internals, analysis testing
