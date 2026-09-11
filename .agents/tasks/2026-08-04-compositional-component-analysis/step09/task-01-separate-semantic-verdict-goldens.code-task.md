# Task: Separate Semantic Verdict Goldens

## Description
Replace full-report golden assertions that are intended only to pin pass/fail behavior with a dedicated, host-independent verdict-golden path. The assertion must consume the canonical report produced by the component's existing `ArccCheck` action and must not rerun analysis or accidentally pin layout, toolchain, diagnostic-size, or path details.

## Background
Step 5 temporarily migrated the old text and JSON report goldens to byte-for-byte comparisons of persisted reports so the build topology could cut over without retaining a second analysis path. Those reports now contain build-graph provenance, export-data diagnostics, SDK-derived details, and potentially source/evidence locations. Treating the complete artifact as a verdict golden couples semantic expectations to host-dependent layout shape and recreates the downstream patch burden identified by friction report section 6(1).

Most existing checks need to assert only the recorded verdict. Full report schema coverage belongs to Task 2, and package-layout shape coverage belongs to Task 3. The verdict path must remain structural: it reads `ArccComponentInfo.report`, so building the test builds the producer action exactly once.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R8, N4, §Build topology, §Golden restructure)

**Additional References:**
- Host import friction: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md` (§6)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a dedicated verdict-golden assertion surface for checked components. It must decode or query the canonical persisted report and compare a deterministic representation of its recorded `pass`/`fail` verdict with a checked-in golden; it must not compare the complete report bytes.
2. Construct the assertion from `ArccComponentInfo.report` and the shared report-verdict behavior. Do not run `arcc check`, stage component source/layout/SDK inputs, or introduce a second verdict derivation that can disagree with `arcc verdict`.
3. Keep verdict-golden content deliberately host-independent. It must contain no absolute paths, source/evidence locations, package-layout package lists, export artifact counts or byte totals, SDK roots or versions, build-graph paths, or host-specific import prefixes.
4. Reject asserted components at analysis time with the same fail-closed rule used by the existing check, grep, and report-golden assertions: an asserted surface has no checked verdict.
5. Migrate the existing API and report-boundary goldens whose purpose is semantic conformance to the verdict-golden path. Remove duplicate text-vs-JSON cases when they now assert the same persisted verdict, while retaining separate tests only where they cover a distinct behavior.
6. Add at least one failing-component verdict golden so both stable values are pinned and a mismatch produces an actionable diagnostic containing expected and actual verdicts.
7. Preserve shell safety, minimal runfiles, default-build laziness, and exactly-once producer behavior in Starlark analysis tests. Golden contents and paths must remain inert data.
8. Add a regression test that applies a simulated host import-prefix rewrite to the fixture graph or comparison inputs and proves the verdict-golden bytes and result are unchanged.

## Dependencies
- Step 8 must be complete so reports and layouts have their final export-data-era shape.
- No Step 9 task dependency; this establishes the semantic half of the golden split used by Tasks 2 and 3.

## Implementation Approach
1. Factor the existing report-verdict assertion seam so an ordinary `.check` and a verdict golden share decoding and verdict semantics without sharing full-report comparison behavior.
2. Add the smallest Starlark rule or test helper needed to materialize and compare the stable verdict representation, with analysis tests for its action/runfiles topology.
3. Migrate semantic golden targets, delete superseded complete-report goldens, and add pass, fail, asserted-provider, mismatch, and rewritten-prefix coverage.

## Acceptance Criteria

1. **Passing and failing verdicts are pinned**
   - Given one conforming checked component and one violating checked component
   - When their verdict-golden tests run
   - Then they compare equal to stable `pass` and `fail` expectations derived from the producer reports.

2. **Verdict goldens are host-independent**
   - Given all files designated as verdict goldens
   - When their contents are inspected
   - Then none contains an absolute path, package closure, SDK/build metadata, diagnostic metrics, source location, or host-specific import prefix.

3. **Prefix rewrites do not churn verdicts**
   - Given an idempotent simulated host rewrite of an import-path prefix
   - When the same semantic component verdict is asserted under the rewritten namespace
   - Then the generated verdict representation is byte-identical and the same golden passes.

4. **The producer remains authoritative**
   - Given a verdict-golden target
   - When its analysis action and runfiles are inspected
   - Then it consumes only the checked component's existing report plus assertion tooling/golden data and never invokes a second `arcc check`.

5. **Asserted components fail closed**
   - Given a component whose provider carries an asserted surface and no report
   - When a verdict-golden target is declared for it
   - Then Bazel analysis fails with a diagnostic naming the assertion target and component and explaining that there is no checked verdict.

6. **Mismatches are actionable and shell-safe**
   - Given a golden with the wrong verdict or hostile text supplied as golden data
   - When the assertion runs
   - Then it fails without executing the data and reports the expected and actual verdicts.

7. **Repository checks pass**
   - Given the completed migration
   - When `just ci` runs
   - Then all Go, Bazel, self-check, manifest-parity, and generated-file checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: bazel, testing, goldens, reports, portability
- **Required Skills**: Starlark, Bazel runfiles and analysis testing, Go CLI/report decoding, shell-safe test generation
