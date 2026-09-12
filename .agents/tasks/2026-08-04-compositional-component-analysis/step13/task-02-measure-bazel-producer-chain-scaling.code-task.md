# Task: Measure Bazel Producer-Chain Scaling

## Description
Extend scaling acceptance across the complete Bazel producer chain. Measure and structurally verify `ArccImportGraph`, `ArccLayout`, and `ArccCheck` as dependency depth and width grow, including the ordinary non-member source projection that remains outside the member-only check action.

## Background
The corrected design permits one source-reading exception to N1: pinned rules_go does not expose the exact ordinary import graph, so a hermetic `ArccImportGraph` action lexically scans declared non-member source and `ArccLayout` merges its descriptor before `ArccCheck`. Existing analysis tests pin the three-action topology and input allowlists, but Step 13 must expose the projection's closure-shaped source count/bytes and elapsed contribution alongside `ArccCheck` loader time. This prevents the final scaling claim from measuring only the leaf action while hiding residual producer work.

Timing data is acceptance evidence, not a persisted component artifact. The benchmark path must preserve production action arguments, inputs, and hermeticity and must not add elapsed fields to reports, surfaces, layouts, or maps.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1–N2, N4, §Build topology, §Type loading, acceptance matrix “Performance”)

**Additional References:**
- Plan Step 13: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Baseline measurements: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Step 8 input-pruning research: `.agents/planning/2026-08-04-compositional-component-analysis/research/step08-input-pruning.md`
- Existing topology/input assertions: `bazel_rules/go/tests/component_tests.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add deterministic Bazel testdata representing the same fixed-member depth 1, 4, and 16 graphs and bounded width variants used by Task 1. Every variant must produce a checked component through the real aspect, ordinary-import projection, layout merge, export-data staging, dependency surfaces/reports, and stdlib-map edge.
2. Extend analysis-time coverage to assert exactly one `ArccImportGraph`, one `ArccLayout`, and one `ArccCheck` action per measured component and to classify every declared input by role.
3. For each variant, record `ArccImportGraph` action count, target-selected ordinary non-member source file count/bytes, import-projection elapsed time, `ArccLayout` elapsed time, `ArccCheck` package-loader time, total `ArccCheck` time, export artifact count/bytes, and overall producer-chain elapsed time.
4. Obtain action timings from a reproducible Bazel profile/execution log or a benchmark-only observation channel. Reuse Task 1's loader observation where needed, but do not change canonical artifacts or register a different production analysis topology merely to measure it.
5. Assert structurally that non-member source is accepted only by `ArccImportGraph`; its request/source inputs and the base layout remain private to the auxiliary actions; `ArccLayout` consumes only the base layout and projection descriptor; and `ArccCheck` consumes the final descriptor, member source, exports, dependency artifacts, and map with no non-member `.go` file.
6. Assert that the number of `ArccCheck` actions and its member scan workload remain constant while projection source inputs and export artifacts scale with the graph. Treat projection and export decoding as measured residual closure costs, not violations of the corrected N1/N2 contract.
7. Provide an integration-tagged Go driver or equivalent test entry point that launches Bazel with an isolated output root, collects machine-readable profiles, validates the metrics, and prints a stable table. The existing `just test-integration`/`just ci` path must execute the bounded acceptance series without depending on developer caches or files.
8. Keep costly stdlib-map generation out of every variant: select the same pinned/default map artifact or shared Bazel action so the routine series performs at most the one generation N5 permits.
9. Append the producer-chain table and an explanation of the source-projection exception to `research/current-analysis-pipeline.md`, aligned row-for-row with Task 1's member-analysis results.

## Dependencies
- Task 1: Add Member-Analysis Scaling Benchmarks supplies the shared synthetic graph parameters and package-loader observation boundary.
- Step 8 Task 08 established the accepted `ArccImportGraph`/`ArccLayout` correction to the original N1/N2 topology.

## Implementation Approach
1. Materialize the synthetic graph family as small rules_go targets and one checked component per parameter set, sharing source templates where Bazel permits without changing effective graph shape.
2. Add analysis tests for exact action/input roles before adding runtime measurement, so semantic-boundary regressions fail independently of timing noise.
3. Drive bounded cold/warm builds in an isolated Bazel output root, parse profiles and benchmark-only loader observations, and aggregate repeated samples consistently with Task 1.
4. Join action metrics with report export diagnostics and record the complete producer-chain table in the research note.

## Acceptance Criteria

1. **The complete producer topology is measured**
   - Given each depth/width scaling target
   - When the integration driver builds its `arcc` output group and reads the profile
   - Then the result includes projection, layout, check-loader, check-total, and whole-chain timing plus source/export input counts and bytes.

2. **Auxiliary source projection remains isolated**
   - Given a closure containing ordinary dependency source
   - When all three actions' inputs are inspected
   - Then that source appears only in `ArccImportGraph`, no source or base-layout input leaks through `ArccLayout` into `ArccCheck`, and the leaf check still receives only the N2 allowlist.

3. **Typed member work stays constant**
   - Given fixed member source across depth and width variants
   - When action counts and Task 1 scan observations are compared
   - Then there is one `ArccCheck` and identical member scan work per variant while projection/export volumes grow with the synthetic closure.

4. **Residual closure costs are explicit**
   - Given the completed scaling table
   - When depth and width increase
   - Then ordinary source projection count/bytes and export artifact count/bytes are visible with their elapsed contributions and are not folded into a claim that the entire producer chain is flat.

5. **Routine map-generation bounds are preserved**
   - Given the full bounded scaling series in one CI run
   - When Bazel action mnemonics are counted
   - Then there are zero native whole-SDK generations and at most one default-configuration `ArccStdlibMap` action.

6. **The suite is automated and reproducible**
   - Given a clean checkout on the pinned Linux/amd64 environment
   - When `just test-integration` or `just ci` runs
   - Then the Bazel scaling driver executes in an isolated output root, validates structural invariants, and emits a table suitable for comparison with the recorded research result.

## Metadata
- **Complexity**: High
- **Labels**: bazel, integration, performance, scaling, import-projection, action-topology
- **Required Skills**: Bazel profiling and execution logs, Starlark analysis testing, rules_go dependency graphs, Go integration harnesses, deterministic metrics aggregation
