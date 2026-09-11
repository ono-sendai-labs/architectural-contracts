# Task: Restore stdlibmap Component Conformance

## Description
Restore the real `stdlibmap` component to a passing strict check after the x/tools ownership migration. Give its `packagelayout` uses a singular, explicit, acyclic component boundary and remove the two currently analysis-defeating standard-library references without broadening the Step 7 warning policy.

## Background
Addresses finding F2 from the Step 7 implementation review. The topological native stage currently emits a failing `internal/stdlibmap/component.report.json`: `cache.go` references `io.ReadAll` and `errors.Is` under strict policy, while `explicit.go` and `layoutloader.go` use `packagelayout` without an architectural dependency. `packagelayout` is presently a member of `goanalysis`, so simply adding it to `stdlibmap` would create a second owner and violate the same one-owner model this step established.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step07.yaml`

**Additional References:**
- Plan Step 7: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Current component: `go/internal/stdlibmap/component.textproto`
- Current ownership declarations: `go/internal/goanalysis/component.textproto`, `go/internal/goanalysis/BUILD.bazel`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Reproduce the `stdlibmap` failure through the real native command and retain the failing report as RED evidence outside the checked-in source tree.
2. Establish exactly one architectural owner for `go/internal/packagelayout` and an explicit surface that authorizes both `goanalysis` and `stdlibmap` consumers. Preserve the shell-to-core dependency direction and prove the resulting component graph is acyclic.
3. Do not list `packagelayout` as a member of both components and do not hide its imports through a pattern, exemption, skipped check, or undeclared package edge.
4. Replace or narrowly refactor the `io.ReadAll` and `errors.Is` uses at the recorded `cache.go` sites so the strict stdlib map no longer produces AnalysisDefeating findings. Do not add `analysis_defeating_policy: WARN` to `stdlibmap` solely to make the check pass, and do not change stdlib-map classifications or classifier hashes.
5. Keep checked-in manifests, Bazel declarations, component surfaces, and manifest parity consistent for any component boundary introduced or changed.
6. Add regression coverage for singular `packagelayout` ownership, declared consumer edges, and the real strict `stdlibmap` verdict.
7. Update affected Component Contract comments and run `just ci` before committing.

## Dependencies
- Step 7 Tasks 3-6 provide surface-backed dependency resolution, the x-tools wrapper, and strict-by-default AnalysisDefeating policy behavior.
- This task must complete before Task 08 makes every staged pass verdict gating.

## Implementation Approach
1. Capture the current strict report and map each violation to its source or missing component edge.
2. Choose the smallest explicit `packagelayout` boundary consistent with singular ownership, Bazel/native parity, and an acyclic component graph; migrate both consumers atomically.
3. Refactor the two analysis-defeating helpers with focused behavioral tests that preserve their error semantics.
4. Add the real-manifest verdict regression, update contracts, and run the full gate.

## Acceptance Criteria

1. **stdlibmap passes under strict policy**
   - Given `go/internal/stdlibmap/component.textproto` and the pinned Linux/amd64 stdlib map
   - When the real native `arcc check` command runs without `--report-verdict-only`
   - Then it exits zero with no violations and without adding an AnalysisDefeating WARN policy

2. **packagelayout has one explicit owner**
   - Given every production component manifest and `go_component` declaration
   - When project-package ownership and dependency edges are indexed
   - Then `go/internal/packagelayout` has exactly one owner, both `goanalysis` and `stdlibmap` reach it through valid component semantics, and the component graph is acyclic

3. **AnalysisDefeating sites are removed without classifier changes**
   - Given the cache read and missing-file behaviors previously implemented with `io.ReadAll` and `errors.Is`
   - When focused tests exercise EOF, reader failure, oversize/no-progress where applicable, direct not-found, and wrapped not-found cases
   - Then behavior is preserved and the strict component report contains neither recorded site, with the pinned classifier hash unchanged

4. **Native and Bazel declarations remain coherent**
   - Given every changed component boundary
   - When manifest parity, ownership tests, native checks, and applicable Bazel checks run
   - Then they agree on members, interface, dependencies, and singular ownership

## Metadata
- **Complexity**: High
- **Labels**: go, components, stdlibmap, packagelayout, ownership, selfcheck
- **Required Skills**: Component-boundary design, Go error/reader semantics, Bazel manifest parity, integration testing
