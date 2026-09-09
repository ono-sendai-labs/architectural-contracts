# Task: Bound Routine Stdlib-Map Generation Cost

## Description
Reshape native integration tests and the Bazel stdlib-map build graph so routine development
and CI perform at most one whole-SDK stdlib-map generation, while preserving the existing
totality, semantic, hermeticity, keying, and determinism guarantees.

Check in the canonical map for the repository's pinned Linux/amd64, cgo-disabled Go toolchain
as a test artifact. Routine Go integration tests and selfcheck consume and validate that
artifact rather than repeatedly regenerating it. Bazel continues to generate its default map
hermetically, but does so through a dedicated thin generator binary; extra determinism and
cross-configuration generations move to an explicit full test lane.

This task changes test and build-graph shape only. It must not change stdlib classification
semantics, production native cache behavior, or the fail-closed map-consumption contract.

## Background
Step 6 task 05 corrected a soundness defect by running Capslock once per importable package
rather than analysing the whole standard library as one program. The isolation is required
because whole-stdlib VTA conflates unrelated package closures, but it increased one generation
from approximately two seconds to approximately 146 seconds.

Routine CI currently multiplies that cost:

- Go integration tests contain approximately eleven whole-SDK generations across the
  `stdlibmap`, `app`, and command integration packages.
- Native selfcheck performs another cold generation in its independent CI job.
- Bazel wildcard build/test contains six `ArccStdlibMap` actions in five configurations.
- The Bazel action uses the full `//:arcc` binary as its tool, so unrelated Go changes
  invalidate the default map.

The objective is to eliminate repeated routine generation without weakening the map's
correctness boundary. The one regular Bazel generation remains the end-to-end exercise of the
real hermetic generator. On the pinned reference configuration, a warm-Bazel-cache routine
`just ci` must complete within five minutes wall clock (design N5).

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`
  (R5-R6, N3-N5, I2-I5, DR-05, DR-07, DR-09, testing strategy)
- Plan: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
  (Steps 4 and 6)

**Additional References:**
- `.agents/scratchpad/2026-09-08-stdlib-map-generation-cost.md`
- `.agents/scratchpad/awo-acpx-report.2026-08-04-compositional-component-analysis.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Establish one exact routine-CI target configuration, initially Go 1.26.4, Linux/amd64,
   cgo disabled, no build tags, and default `GOEXPERIMENT`. Make the native GitHub CI and
   selfcheck jobs use that exact patch version and configuration. Add a fast consistency
   check so drift between the GitHub pin, Bazel's `go_sdk.download` version, and the checked
   artifact fails with expected and actual values.
2. Check in one canonical full stdlib authority map for that configuration. Treat it as a
   versioned test artifact, not as a replacement for production native on-demand generation
   or Bazel's declared generated map.
3. Add a shared test helper that opens the checked artifact through the production bounded
   decoder and reader, derives the expected complete SDK key from the pinned target plus the
   current production classifier rules, `RuleVersion`, and map format, and rejects any
   mismatch. Checking only `go version`, or accepting the map's own key without comparison,
   is insufficient.
4. Remove every whole-SDK generation from routine Go integration tests. Preserve the
   displaced assertions as follows:
   - Run totality, terminal-classification, evidence, provenance, and pinned semantic
     assertions against the checked artifact.
   - Run `stdlibmap inspect` integration assertions against the checked artifact.
   - Use the checked artifact for Linux/amd64 check and real-SDK integration paths.
   - Use small canonical synthetic maps for Windows/Darwin platform-selection and
     key-mismatch tests that do not intend to test full generation.
   - Test native and explicit generation command plumbing with bounded synthetic SDK inputs
     or existing injected seams rather than a full host SDK.
5. Make `just selfcheck` fail fast if its active toolchain configuration differs from the
   pin, then pass the checked artifact explicitly to each native check. Do not seed or mutate
   the user's production stdlib-map cache as a testing shortcut.
6. Add a dedicated Bazel generator binary containing only the explicit stdlib-map generation
   path and the package-layout self-exec driver. Share implementation with `arcc stdlibmap
   generate`; do not duplicate parsing or generation semantics. Point `arcc_stdlib_map` at
   this target through the Go adapter seam.
7. Audit the thin binary's dependency graph. At minimum it must exclude `cmd/arcc/app`,
   `checker`, `facts`, `goanalysis`, and `capanalyzer`, so changes in the normal check path do
   not invalidate the map. Record any unavoidable remaining dependencies, particularly those
   pulled through the current `artifactio` package.
8. Move the replica, build-tag, cross-platform, and transitioned checked-surface map tests and
   their supporting targets out of wildcard `bazel build //...` and `bazel test //...` using
   `manual` tags. Preserve them behind an explicit `just bazel-test-full` or equivalently
   named command; do not delete or weaken their assertions.
9. Keep exactly one real `ArccStdlibMap` action in routine Bazel wildcard build/test. Add a
   routine test comparing that generated default artifact byte-for-byte with the checked-in
   pinned artifact. This must reuse the already-required default generation, not introduce a
   second action.
10. Update stale design, plan, code comments, and research text. Record that generation is
    isolated per importable package, why whole-batch analysis was unsound, the measured cost,
    and the split between routine Linux/amd64 coverage and the explicit full lane.
11. Record deferred work to run the full lane in a CI matrix covering additional execution
    and target architectures. Do not implement that matrix in this task.
12. Do not implement Bazel B2 or generator optimizations C1-C4 in this task. Run `just ci`
    before committing and record before/after integration and Bazel action counts and elapsed
    times without replacing N5's end-to-end bound with narrower or flaky per-test timeouts.

## Dependencies
- Step 6 task 05 must be reviewed and completed first.
- Step 4's stdlib-map generator, artifact reader, SDK key, native cache, and hermetic Bazel
  rule.
- Step 5's checked Bazel analysis action and report assertion tests.

## Implementation Approach
1. Pin and validate the routine Linux/amd64 toolchain configuration.
2. Produce and check in the canonical artifact, together with complete-key validation.
3. Replace all routine native whole-SDK generations while preserving each test's intent.
4. Introduce the thin Bazel generator target and verify its dependency closure.
5. Move extra Bazel configurations to the explicit full lane and assert the routine action
   count.
6. Repair documentation, record measurements and deferred matrix work, then run `just ci`.

## Acceptance Criteria

1. **Pinned artifact is trustworthy**
   - Given the checked-in stdlib map
   - When routine tests derive the expected key from the pinned target and current production
     classifier
   - Then decoding, semantic validation, and every SDK-key field match exactly, and any
     toolchain, classifier, format, cgo, tag, or experiment drift fails clearly

2. **Routine Go integration performs no full generation**
   - Given `go test -tags=integration ./... -count=1`
   - When all integration packages run
   - Then no test invokes whole-SDK generation, while the previous totality, semantics,
     inspect, check orchestration, platform, and fail-closed assertions remain covered

3. **Selfcheck reuses the pinned artifact**
   - Given the pinned Linux/amd64 CI environment
   - When `just selfcheck` runs on a cold machine
   - Then it validates and uses the checked artifact without generating or populating a
     native cached map
   - And a mismatched environment fails before checking components

4. **Unrelated Go edits do not invalidate Bazel generation**
   - Given the dedicated generator target
   - When its Bazel dependency graph and `ArccStdlibMap` action inputs are inspected
   - Then the full `//:arcc` binary and normal check-path packages are absent

5. **Routine Bazel builds one map**
   - Given wildcard `bazel build //...` and `bazel test //...`
   - When `ArccStdlibMap` actions are queried
   - Then exactly one default-configuration generation is present
   - And its output is byte-identical to the checked-in pinned artifact

6. **Full coverage remains explicitly runnable**
   - Given the full stdlib-map test command
   - When it is invoked explicitly
   - Then the replica determinism, build-tag keying, cross-platform keying, and transitioned
     checked-surface assertions all run with their original meaning

7. **Production behavior is unchanged**
   - Given a normal native `arcc check` without `--stdlib-map`
   - When no matching cache entry exists
   - Then it still generates and atomically caches the target's map on demand
   - And Bazel checks still consume a declared hermetically generated map

8. **Routine CI is acceptably fast**
   - Given the pinned Linux/amd64 reference configuration and a warm Bazel cache
   - When the complete routine `just ci` gate is timed
   - Then it completes within five minutes wall clock
   - And a cold routine run performs no native whole-SDK generation and at most one Bazel
     whole-SDK generation

9. **The cost and coverage split are documented**
   - Given the revised design, plan, code comments, and research note
   - When reviewed against the implementation
   - Then they accurately describe per-package isolation, routine Linux/amd64 coverage, the
     explicit full lane, measured action counts, and the deferred architecture matrix

10. **Repository gates pass**
    - Given the completed change
    - When `just ci` runs
    - Then every routine gate passes with no stdlib-map semantic or fail-closed behavior
      weakened

## Metadata
- **Complexity**: High
- **Labels**: go, bazel, ci, performance, stdlib-map, determinism, hermeticity
- **Required Skills**: Go integration testing, Bazel action analysis, rules_go toolchains,
  deterministic artifact handling, CI workflow maintenance
