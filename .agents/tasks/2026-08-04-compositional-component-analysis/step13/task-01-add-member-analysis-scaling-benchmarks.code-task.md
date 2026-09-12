# Task: Add Member-Analysis Scaling Benchmarks

## Description
Add a reproducible, integration-tagged scaling suite for the member-only analysis path. Measure package loading, typed reference scanning, total analysis time, and export-data volume while dependency depth, width, and directly imported type surface grow, and prove that semantic analysis remains confined to the fixed member source.

## Background
Steps 6–8 replaced whole-closure source analysis with a typed scan of member packages backed by dependency export data. The existing deep-closure integration test proves correctness without dependency source and reports export artifact count/bytes, but it does not characterize depth/width scaling, split load time from scan time, or pin the absence of non-member parsing and scanning. Design N1 and the Step 13 acceptance row require that evidence without adding nondeterministic timing data to canonical reports, surfaces, or maps.

The primary series keeps member source and its typed references byte-identical while synthetic dependency graphs vary in depth (1, 4, and 16) and width. A second series grows the type information exported by a directly imported package so the residual export-data loading cost remains visible rather than being mistaken for a zero-closure-cost claim.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1, N4, §Type loading, §Testing Strategy, acceptance matrix “Performance”)

**Additional References:**
- Plan Steps 8 and 13: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Baseline and Step 6 measurements: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Export-data loading spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-export-data-loading.md`
- Existing member-only coverage: `go/internal/goanalysis/member_only_integration_test.go` and `go/internal/goanalysis/member_only_load_test.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add an integration-tagged scaling suite that constructs hermetic synthetic layouts/export artifacts for dependency depths 1, 4, and 16 plus bounded width variants. Keep the member package's source bytes, selected files, reference sites, and resulting typed edges identical across the primary series.
2. Measure package-load, `ScanReferences`, and end-to-end `LoadPackageFacts` durations separately. Collect multiple samples after warm-up and report a robust aggregate such as the median; keep timing observations in test output or a benchmark result, never in canonical reports or surfaces.
3. Reuse the production member-only loader and scanner. If observability requires a seam, make it narrowly scoped and deterministic, with a no-op production default; do not fork a second analysis implementation or weaken the public `LoadPackageFacts` contract.
4. Add exact structural counters/assertions proving that only member packages have syntax/type-info eligible for scanning, that non-members are export-backed with nil syntax/type-info, and that the scan sees the same member AST/reference workload at every depth/width. Audit the benchmark dependency graph so no SSA, VTA, or Capslock package is linked or invoked by the check path.
5. Record unique non-member export artifact count and deduplicated bytes for every case using the same validated-input definition as report diagnostics. Assert monotonic closure-volume growth and exact expected counts rather than inferring it from elapsed time.
6. Assert scan flatness using both the exact invariant work counters and a documented, noise-tolerant timing comparison over repeated samples. Fail on a material closure-correlated scan regression while avoiding a single-sample microsecond threshold.
7. Add a second series whose directly imported dependency exports progressively larger public type/signature data. Record export count/bytes, package-load time, scan time, and total time, and document that export decoding is the intended residual cost while member scanning remains bounded by member source.
8. Run the suite through `go test -tags=integration` so the existing `just test-integration` and `just ci` recipes execute it. Keep the fixture size and sample count compatible with the five-minute warm-cache CI feedback budget.
9. Append the resulting environment, fixture parameters, raw/aggregate table, and interpretation beside the Step 1/Step 6 measurements in `research/current-analysis-pipeline.md`. Clearly distinguish enforced structural invariants from machine-dependent timings.

## Dependencies
- Step 8 supplies member-only export-data loading and deterministic export artifact diagnostics.
- Step 6 supplies the typed reference scanner and removes SSA, VTA, and Capslock from the check path.

## Implementation Approach
1. Extract a reusable synthetic export-layout builder from the existing deep-closure integration fixture where practical, then generate depth, width, and type-surface cases from explicit deterministic parameters.
2. Add test-only phase observation around the production loader/scanner boundary and pair duration samples with exact package-role and scan-work counters.
3. Execute warm-up and repeated samples in a stable order, render a compact scaling table through test logs, and encode only robust structural and broad regression assertions.
4. Run the integration suite on the pinned Linux/amd64 toolchain and record the final measurements and comparison with the Step 1 baseline.

## Acceptance Criteria

1. **Depth and width do not expand typed scanning**
   - Given byte-identical member source with dependency depth 1, 4, and 16 and the specified width variants
   - When each case is loaded and scanned repeatedly
   - Then member package/syntax/reference work counters are identical, non-members have no syntax or type-info, and scan time remains flat within the documented noise-tolerant bound.

2. **Residual export-data cost is measured honestly**
   - Given successively larger dependency closures
   - When the integration suite records validated inputs and phase timings
   - Then export artifact count/bytes grow as expected, package-load and total timings are reported separately from scan timing, and no result claims that export-data loading is constant.

3. **Growing direct type surface has its own series**
   - Given a fixed member reference against directly imported packages with progressively larger exported type/signature data
   - When the second series runs
   - Then its type-surface size, export bytes, load/scan/total timings, and bounded member-scan workload are reported and the residual decoding trend is documented.

4. **Forbidden closure analysis is absent by construction**
   - Given every generated scaling case
   - When loaded-package roles and the invoked analysis path are inspected
   - Then only members are parsed/type-checked/scanned, non-members are export-backed, and no SSA, VTA, or Capslock path is invoked.

5. **Measurements do not contaminate deterministic artifacts**
   - Given two otherwise identical analysis runs with different observed durations
   - When their reports and surfaces are emitted
   - Then artifact bytes remain identical because elapsed values exist only in benchmark/test output.

6. **Routine integration coverage remains bounded**
   - Given the pinned supported CI configuration
   - When `just test-integration` and `just ci` run
   - Then the scaling suite executes without a manual benchmark flag and the warm-cache feedback lane remains within its documented budget.

## Metadata
- **Complexity**: High
- **Labels**: go, integration, performance, scaling, export-data, reference-scan
- **Required Skills**: Go integration testing, go/packages export-data loading, deterministic fixture generation, performance measurement, statistical regression-test design
