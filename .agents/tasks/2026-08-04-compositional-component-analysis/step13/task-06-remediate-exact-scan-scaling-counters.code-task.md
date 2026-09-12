# Task: Add Exact Member-Only Scan Work Counters

## Description
Remediate implementation-review finding F2 by making the scaling suites observe exact work performed inside the typed reference and analysis-defeat scanners, so closure-shaped scan regressions fail structurally instead of relying on a permissive timing threshold.

## Background
Addresses finding F2 from the Step 13 implementation review. The existing depth/width fixtures correctly hold member source and emitted edges constant, remove non-member source, and verify export-backed loaded-package roles. Their exact workload snapshots are derived from the `packages.Load` result, however, while the only direct scan assertion is a factor-20-plus-20ms elapsed ceiling. That can miss substantial closure-correlated bookkeeping which leaves package data and output edges unchanged.

The remediation should preserve timing as useful machine-dependent evidence while adding a nil-by-default, package-private observation seam at the actual scan boundary. Production APIs and persisted artifacts must remain unchanged.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1, N4, §Type loading, acceptance matrix “Performance”)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step13.yaml`

**Additional References:**
- `go/internal/goanalysis/refscan.go`
- `go/internal/goanalysis/defeat.go`
- `go/internal/goanalysis/member_analysis_scaling_fixture_test.go`
- `go/internal/goanalysis/member_analysis_scaling_integration_test.go`
- `go/internal/goanalysis/producer_chain_scaling_integration_test.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a narrowly scoped package-private scan observer, nil by default, which records actual package consideration/entry and member syntax/type-info work inside `ScanReferences`; include `ScanAnalysisDefeats` if it independently traverses package inputs.
2. Record enough identity and counts to prove that every entered package is a declared member and that no non-member syntax, type-info, AST, import, `Uses`, or `Selections` work is performed.
3. Keep the observer deterministic and test-only in effect: it must not alter scan ordering/results, public signatures, report/facts/surface/map schemas, or production artifact bytes.
4. Capture and restore observer state safely in the integration harness, following the existing loader/phase-observer pattern without introducing parallel-test races.
5. Assert exact scan counters for every native depth/width and direct-type-surface case and for both checked subchains of every Bazel producer-chain row.
6. Require the exact scan counters and emitted typed edges to be identical across graph sizes, while source/export projection counts and bytes continue to grow as expected.
7. Retain separated loader/scan/total timing observations and a noise-tolerant timing check, but do not use elapsed time as the proof of member-only work.
8. Update the research note to distinguish loaded-package role counters from scanner-internal work counters and record the exact invariant values.

## Dependencies
- Task 01's synthetic graph builder and Task 02's Bazel loader-frame harness provide the cases to instrument.
- No dependency on Task 05.

## Implementation Approach
1. Add a focused failing unit test that presents member and non-member packages and requires the observer to report only member scan entry/work.
2. Instrument the production scanners at their actual traversal points with a nil-check and immutable observation values.
3. Thread captured counters into `scalingSample`, `producerChainLoaderObservation`, and the row comparison helpers, then assert exact equality and zero non-member work.
4. Re-run native and Bazel scaling suites and refresh the documented workload tokens/table interpretation.

## Acceptance Criteria

1. **Typed scan entry is member-only by exact count**
   - Given a loaded graph containing one fixed member and growing non-member closures
   - When `ScanReferences` runs
   - Then its internal observer records exactly one member package entry, the fixed member syntax/type-info/Uses/Selections/import workload, and zero non-member package entries or syntax/type-info work for every case.

2. **Analysis-defeat scanning is also bounded**
   - Given the same graph family
   - When analysis-defeating constructs are scanned
   - Then its internal work counters remain fixed to member source and record no non-member traversal that performs AST/source analysis.

3. **Native scaling rows pin actual scan work**
   - Given depth 1/4/16, width variants, and growing direct type surfaces
   - When the member-analysis integration suite runs
   - Then exact scanner-internal counters and typed edges are identical across rows while export artifact counts/bytes grow monotonically.

4. **Bazel scaling rows pin both checked scans**
   - Given every producer-chain variant
   - When root and checked-dependency loader observations are replayed from declared Bazel artifacts
   - Then both subchains have identical exact scanner-internal work tokens and zero non-member scan entry, while projection/export volumes retain their expected growth.

5. **Observation cannot affect persisted output**
   - Given otherwise identical runs with the scan observer disabled and enabled
   - When reports, surfaces and facts are emitted
   - Then their bytes are identical and contain no scan-counter or timing field.

6. **Timing remains evidence, not the invariant**
   - Given machine-dependent duration noise
   - When the suites evaluate scaling
   - Then timing tables and a broad regression guard remain available, but pass/fail for member-only typed work is determined by exact scanner counters.

## Metadata
- **Complexity**: Medium
- **Labels**: go, integration, scaling, reference-scan, instrumentation, remediation
- **Required Skills**: Go package-private test seams, go/packages graph semantics, typed AST scanning, deterministic integration tests, concurrency-safe test instrumentation
