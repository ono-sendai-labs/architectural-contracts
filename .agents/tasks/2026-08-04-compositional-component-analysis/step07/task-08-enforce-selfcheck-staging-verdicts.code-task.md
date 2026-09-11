# Task: Enforce Selfcheck Staging Verdicts

## Description
Make the topological native selfcheck stage a real verdict gate instead of an artifact-only producer. Assert a passing verdict for every analyzed component expected to conform, preserve the two report-free asserted wrappers and the one explicitly expected schema failure, and align the README with the executable coverage.

## Background
Addresses finding F1 from the Step 7 implementation review. `stage_component` currently invokes `arcc check --report-verdict-only`, so analysis failures are written to reports but return zero. Only schema's expected failure is inspected; all other staging verdicts are discarded, and the later nine direct checks do not cover the full staged set. Because native surface reports never self-certify and consumers derive `ASSERTED` provenance, a failed staged dependency also cannot fail a downstream gate. The review reproduced this false-green behavior with the current failing `stdlibmap` component.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step07.yaml`

**Additional References:**
- Plan Step 7: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Native recipe: `justfile`
- User-facing selfcheck description: `README.md` (§Self-Hosting Verification)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Keep artifact production in one temporary copied Go tree and in component-dependency topological order so every consumer sees only artifacts created by the same run.
2. Make the staging helper decode or otherwise robustly inspect each canonical report and require verdict `pass` by default. A tool error, missing report, malformed report, or `fail` verdict must fail the recipe and name the component.
3. Model expected outcomes explicitly: `schema` remains the sole analyzed expected-fail stage for its documented generated-protobuf findings, while `protobuf-runtime` and `x-tools` remain asserted UNKNOWN surfaces with no report or analysis action.
4. Ensure every analyzed production component included in the selfcheck topology has its verdict asserted exactly once, either during staging or by an ordinary non-verdict-only final check. Avoid a second analysis when the staged canonical report already provides the same gate.
5. Preserve the semantic distinction that native sibling reports are audit-only and do not certify dependency surfaces; fix the harness rather than changing native provenance behavior.
6. Add a regression that substitutes or produces a failing report for an otherwise stage-only component and proves the selfcheck verdict gate fails. Also prove the expected schema failure and asserted-wrapper paths still succeed.
7. Update `justfile` comments and README counts/component lists so they describe the exact post-Step-7 native and Bazel coverage, including any intentionally non-gating component with its reason.
8. Run `just ci` before committing.

## Dependencies
- Task 07 must first make the real `stdlibmap` component pass, otherwise the corrected selfcheck gate will fail by design.

## Implementation Approach
1. Extract or centralize canonical report-verdict assertion in the recipe/test harness and pin its pass/fail/error behavior.
2. Apply explicit expected verdicts to the topological component list, retaining special handling only for schema and asserted wrappers.
3. Remove redundant final analyses where the staged report is now the authoritative gate, or document and test any intentionally retained second run.
4. Align the README and execute the full selfcheck and CI lanes.

## Acceptance Criteria

1. **Every analyzed stage has an asserted verdict**
   - Given the complete native selfcheck topology
   - When staging finishes
   - Then every analyzed component's canonical report is checked against an explicit expected verdict, with pass as the default

2. **A stage-only failure makes selfcheck fail**
   - Given a component that is not repeated in the final command list and whose staged report has verdict fail
   - When the selfcheck harness runs
   - Then the recipe exits nonzero and names that component instead of proceeding to a green result

3. **Intentional non-pass paths remain explicit**
   - Given the schema component and the protobuf-runtime/x-tools wrappers
   - When the selfcheck runs
   - Then schema must produce its documented fail verdict, each wrapper must provide an asserted UNKNOWN surface and no report, and no other component is silently exempted

4. **Documentation matches executable coverage**
   - Given the final `justfile` and README self-hosting section
   - When their component lists, counts, and semantics are compared
   - Then they agree exactly on which checks are native, which are Bazel, and which exceptions remain

5. **The real gate passes**
   - Given all Step 7 remediation in place
   - When `just selfcheck` and `just ci` run
   - Then both pass while observing every expected verdict and consuming only temporary staged artifacts

## Metadata
- **Complexity**: Medium
- **Labels**: ci, selfcheck, integration, reports, documentation
- **Required Skills**: Shell/just recipes, artifact verdict validation, integration-test design, documentation maintenance
