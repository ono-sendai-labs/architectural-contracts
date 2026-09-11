# Task: Reconcile Member File-Role Documentation

## Description
Align the public package-layout schema and Step 8 evidence with the complete member analysis-file vocabulary restored by Task 07: selected Go source, build-excluded Go source retained as ignored metadata, and assembly retained as other-file metadata and as declared `ArccCheck` inputs.

## Background
Addresses finding F6 from the Step 8 implementation re-review. The fail-closed code and executing Bazel fixture now retain assembly and build-excluded Go files, but `docs/package-layout-schema.md` and the Step 8 research note still describe the exact checked-action inventory as selected member `.go` files only. That stale contract omits the `IgnoredFiles` and `OtherFiles` roles required by `ScanAnalysisDefeats` and can guide a future emitter back to the defect fixed by F1.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1-N2, DR-11, §Error Handling, acceptance matrix “Action inputs” and “Analysis defeating”)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step08-r2.yaml`

**Additional References:**
- Original implementation review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step08.yaml` (F1)
- Package-layout schema: `docs/package-layout-schema.md` (§3a and §3b)
- Step 8 evidence: `.agents/planning/2026-08-04-compositional-component-analysis/research/step08-input-pruning.md`
- Member-role implementation: `bazel_rules/go/private/component.bzl`, `go/internal/packagelayout/packagelayout.go`, and `go/internal/goanalysis/defeatscan.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Update `docs/package-layout-schema.md` to define the complete member package vocabulary: selected Go files remain `GoFiles`/`CompiledGoFiles`, target-excluded declared Go files are retained in `IgnoredFiles`, and assembly/non-Go analysis files are retained in `OtherFiles`.
2. State that only selected member Go files are parsed and type-checked, while ignored Go and assembly remain declared inputs consumed by the fail-closed analysis-defeating scan.
3. Correct the schema's exact `ArccCheck` input allowlist so it includes every declared member analysis file while continuing to exclude every ordinary non-member and SDK source file.
4. Correct the final input inventory and action-test description in `research/step08-input-pruning.md` to use the same role-aware wording.
5. Preserve the F2 distinction between `ArccCheck` and the cached `ArccImportGraph`/`ArccLayout` producer chain; do not weaken the disclosure or measurement requirements for the auxiliary non-member lexical projection.
6. Keep the design's source asymmetry and fail-closed policy explicit: non-member source may enter only `ArccImportGraph`, never `ArccCheck`, while all declared member bypass-capable files remain visible to `ScanAnalysisDefeats`.

## Dependencies
- Step 8 Task 07, which implements and tests the member `IgnoredFiles`/`OtherFiles` roles.
- Step 8 Task 08, whose auxiliary-projection wording and performance requirements must remain intact.

## Implementation Approach
1. Trace the member file roles from Starlark layout emission through member-only validation, driver projection, and defeat scanning.
2. Amend the schema's per-package role section and exact action allowlist with that vocabulary.
3. Amend the Step 8 research inventory and test description, then search both documents for remaining selected-`.go`-only claims.
4. Run documentation hygiene checks and `just ci` to ensure the update introduces no broken links, formatting defects, or generated-file drift.

## Acceptance Criteria

1. **Member package roles are complete**
   - Given a member package with selected Go, target-excluded Go, and assembly files
   - When an emitter author reads the package-layout schema
   - Then the document assigns them to `GoFiles`/`CompiledGoFiles`, `IgnoredFiles`, and `OtherFiles` respectively and explains which roles are type-checked versus defeat-scanned.

2. **The ArccCheck allowlist matches the implementation**
   - Given the final Step 8 action boundary
   - When the schema and research inventory enumerate its inputs
   - Then both include the complete declared member analysis-file set and still forbid every non-member and SDK source input.

3. **F1 and F2 guarantees remain explicit together**
   - Given the restored member-role implementation and the accepted auxiliary import projection
   - When both documents are read end to end
   - Then member assembly/excluded Go remain visible only as member analysis inputs, ordinary non-member source remains confined to `ArccImportGraph`, and the producer-chain performance measurements remain required.

4. **Contradictory inventory wording is absent**
   - Given the updated documentation
   - When targeted searches inspect claims about selected member `.go` inputs, `IgnoredFiles`, `OtherFiles`, and exact action inputs
   - Then no passage still presents selected member `.go` files as the whole member input vocabulary.

5. **Repository checks pass**
   - Given the completed documentation change
   - When link/whitespace/conflict-marker checks and `just ci` run
   - Then all checks pass without behavior or generated-file changes.

## Metadata
- **Complexity**: Low
- **Labels**: documentation, packagelayout, bazel, analysis-defeating, hermeticity
- **Required Skills**: Technical writing, Go package metadata, Bazel action-input contracts
