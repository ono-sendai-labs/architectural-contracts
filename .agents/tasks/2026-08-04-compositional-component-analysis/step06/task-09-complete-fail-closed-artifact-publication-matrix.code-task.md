# Task: Complete Fail-Closed Artifact Publication Matrix

## Description
Strengthen the real-command integration coverage for invalid standard-library
maps so every required failure mode independently proves that no report or
surface artifact is published. Preserve the current fail-closed implementation
and keep the tests bounded by mutating pinned or small synthetic artifacts rather
than generating the full SDK map.

## Background
Addresses finding F3 from the Step 6 implementation review and adjudicates the
highest-risk deferred finding from task 05. The corrupt and missing-map legs in
`TestCheck_EmitsArtifactsLayoutMode` currently reuse output paths populated by a
preceding successful check, so they prove exit status and diagnostics but cannot
prove artifact absence. A fresh incomplete-key case covers one branch, and the
Runner resolves the map before publication, but the complete AC2 matrix lacks an
end-to-end regression on that ordering invariant.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N3, Error Handling, Fail-closed on unknown SDK)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step06.yaml`

**Additional References:**
- `.agents/tasks/2026-08-04-compositional-component-analysis/step06/task-05-cut-over-check-to-reference-analysis.code-task.md` (AC2)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Build a table-driven production-Runner integration test covering at least a
   missing map, malformed/corrupt map, target SDK-key mismatch, classifier-hash
   mismatch, format-version mismatch, and an inventory-incomplete map.
2. Give every table row fresh report and surface output destinations that did
   not exist before invocation. Assert both remain absent after the failure.
3. Assert exit code 2 and an actionable diagnostic that identifies the relevant
   artifact or mismatched/incomplete field for every row.
4. Drive the real layout-mode check path, including production artifact decode,
   key validation, classification where applicable, and publication ordering.
   Unit-only calls to the reader or authority resolver are not sufficient.
5. Construct corrupt variants from the checked pinned artifact or small
   canonical synthetic maps. The routine integration suite must perform no
   whole-SDK generation and must not read or mutate the production native cache.
6. Retain the existing successful deterministic artifact-emission assertions;
   refactor shared fixture setup only where it makes each failure row independent
   and easier to audit.

## Dependencies
- Step 6 tasks 01-06.
- Independent of task 07; sequence after task 08 only to keep remediation task
  execution deterministic.

## Implementation Approach
1. Extract a narrow helper that invokes one invalid map with fresh artifact
   destinations and performs the common exit/no-publication assertions.
2. Generate each invalid artifact by a focused mutation that preserves all
   unrelated validity, ensuring the expected validator branch is reached.
3. Keep successful emission/determinism coverage separate, run the focused test,
   then run all routine gates.

## Acceptance Criteria

1. **Every required map fault is exercised end to end**
   - Given missing, corrupt, target-key-mismatched, classifier-mismatched,
     format-mismatched, and inventory-incomplete stdlib maps
   - When each is supplied to the production layout-mode Runner
   - Then each exits 2 with a diagnostic naming its fault

2. **No failing row publishes an artifact**
   - Given fresh report and surface output paths for every invalid-map row
   - When the check fails
   - Then neither output path exists afterward

3. **The matrix is independent and deterministic**
   - Given any table order or isolated row execution
   - When the test runs
   - Then no row relies on artifacts from a successful or preceding invocation,
     and every row reaches its intended validation branch

4. **Routine tests remain bounded**
   - Given the expanded integration matrix
   - When `CGO_ENABLED=0 just test-integration` runs
   - Then it uses only pinned/synthetic map data, performs no whole-SDK
     generation, and does not touch the native production cache

5. **Repository gates pass**
   - Given the completed coverage repair
   - When `CGO_ENABLED=0 just ci` runs
   - Then every routine gate passes and the successful artifact determinism
     coverage remains intact

## Metadata
- **Complexity**: Medium
- **Labels**: go, integration-tests, fail-closed, artifacts, stdlib-map, remediation
- **Required Skills**: Go table-driven testing, CLI integration testing, deterministic artifact mutation
