# Task: Pin Acceptance Fixture Semantics

## Description
Close the two remaining acceptance-fixture gaps by asserting the user-visible
`untrusted` rendering of an asserted `UNKNOWN` dependency and the intended
failing verdict/kind of the full-build determinism consumer.

## Background
Addresses findings F4 and F6 from the plan-scoped implementation review. The
report-boundary fixture currently verifies structural `ASSERTED` provenance and
`UNKNOWN` authority in persisted JSON, while separate Go tests verify renderer
vocabulary. It does not prove that this actual end-to-end dependency edge is
rendered `untrusted` and never `certified` or `declared`.

The full-build determinism fixture compares exact bytes correctly, but its
consumer is documented as intentionally failing with three `os.ReadFile`
authority sites and the semantic audit never requires a FAIL verdict or the
intended violation kind. A future policy change could therefore weaken the
fixture while leaving the byte-equality assertion green.

These are test hardening changes, not production behavior changes. Preserve the
single `ArccCheck` producer and the separation between report production and
verdict/report assertion rules.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (I1, I6, N4, acceptance matrix “Unknown dependency” and “Determinism”)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review.yaml`

**Additional References:**
- `bazel_rules/go/tests/testdata/reportboundary/consumer/BUILD.bazel`
- `bazel_rules/go/private/check.bzl`
- `go/internal/report/report.go`
- `go/internal/report/report_test.go`
- `go/internal/goanalysis/full_build_determinism_integration_test.go`
- `bazel_rules/go/tests/testdata/determinism/BUILD.bazel`
- `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add an end-to-end assertion using the production text renderer for the `consumer_component` edge to `manual_component`: the boundary must include `asserted` and `untrusted` and must not call that edge `certified` or give it declared-authority wording.
2. Bind the text assertion to the existing persisted report/producer chain. Do not introduce a second `ArccCheck` action, make `.check` rerun analysis, or add a production CLI/API solely for test convenience.
3. Keep the existing JSON assertions for `provenance: ASSERTED` and `authority: UNKNOWN`; the text test complements rather than replaces structural coverage.
4. In `assertFullBuildArtifactSemantics`, require `consumer_component`'s report verdict to be FAIL.
5. Require its multi-site finding to be the intended `UNDECLARED_AUTHORITY` violation for `FILES`, with at least the three sorted `os.ReadFile` sites, rather than accepting any finding or warning with three sites.
6. Retain the exact-byte/digest comparisons across isolated builds, the checked/asserted dependency-axis assertions, the pinned-map topology, and the rule that timing fields never enter persisted artifacts.
7. Update fixture comments or research evidence only where needed to state the newly enforced semantics accurately.

## Dependencies
- Task 01 should land first so the full validation includes lint.
- Task 02 may land first or in parallel at the specification level; this task must preserve its clarified asserted-surface semantics.

## Implementation Approach
1. Add a failing assertion at the closest existing seam that renders the persisted report through production code; prefer extending an existing test helper over adding a new production command.
2. Tighten the determinism semantic audit to select the expected violation by kind/capability and verify verdict, cardinality, and sorted sites before the cross-build byte comparison.
3. Run the focused Go/Bazel targets for each fixture, then the full repository validation gate.

## Acceptance Criteria

1. **The real UNKNOWN edge renders untrusted**
   - Given the checked reportboundary consumer depending on the asserted UNKNOWN manual component
   - When its persisted report is rendered through production text-rendering code
   - Then that dependency line contains `asserted` and `untrusted` and does not contain `certified` or declared-authority wording.

2. **Structural UNKNOWN coverage remains intact**
   - Given the same reportboundary fixture
   - When its canonical JSON report is inspected
   - Then it still asserts `provenance: ASSERTED` and `authority: UNKNOWN` through the existing producer chain.

3. **No duplicate analysis action is introduced**
   - Given the reportboundary component and its assertion targets
   - When Bazel actions are inspected
   - Then the component has exactly one `ArccCheck` producer and report rendering/assertion does not rerun component analysis.

4. **The determinism consumer remains intentionally failing**
   - Given either isolated full-build artifact set
   - When `consumer_component`'s canonical report is decoded
   - Then its verdict is FAIL and it contains an `UNDECLARED_AUTHORITY` violation for `FILES` with at least the three intended sorted `os.ReadFile` sites.

5. **Determinism remains an exact-byte property**
   - Given the tightened semantic assertions
   - When the two isolated producer graphs build with identical complete inputs in reverse label order
   - Then map, checked/asserted surface, and report bytes/digests remain identical and no timing field is persisted.

6. **Focused and aggregate validation pass**
   - Given the completed fixture changes
   - When the focused reportboundary Bazel target, the producer-chain integration test, and `just ci` run
   - Then all pass with no production checker, provider, or artifact behavior change.

## Metadata
- **Complexity**: Medium
- **Labels**: remediation, tests, bazel, go, unknown-authority, determinism
- **Required Skills**: Go integration testing, Bazel analysis/test topology, deterministic artifact assertions, report rendering semantics
