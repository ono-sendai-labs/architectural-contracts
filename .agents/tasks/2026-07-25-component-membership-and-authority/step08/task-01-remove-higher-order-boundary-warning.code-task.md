# Task: Remove the higher-order boundary warning

## Description
Delete the `HIGHER_ORDER_BOUNDARY_CALL` report kind, the `facts.CallEdge.PassesFuncValue` fact that drives it, and the loader computation that produces that fact, so the analysis stops warning on the one shape it should stay silent about.

## Background
`HIGHER_ORDER_BOUNDARY_CALL` fires when a member calls a *declared* interface symbol of a component dependency and any argument at that call site has a function type. Its polarity is wrong. Passing a callback whose body is the component's own owned code is the intentional plugin-struct pattern: the body is a member, so its authority is charged to the component by attribution, and nothing has escaped. The warning fires there and stays quiet in the case that actually leaks — a function *value* whose body lives in absorbed code, which no one analyzes.

The replacement (A5) is `ABSORBED_FUNC_VALUE_ESCAPE`, built in Tasks 2 and 3 of this step from a new loader-produced fact. This task is the removal half, kept separate so the deletion of a public report kind and a public facts field is a single reviewable, atomic change that leaves the repository green.

The removal is deliberately not a rename: the new warning has a different subject (the referenced function, not the call site), a different trigger (an operand scan, not an argument-type test), and a different suppression rule. Nothing in the old branch survives into the new one.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A5, §3.3, §5.2, and §5.3)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 8)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/capability-analysis-mechanics.md` (§1 — why calling *into* an unqueried package still attributes, which is the reason the old warning's premise does not hold)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Remove the `HigherOrderBoundaryCall` constant from `go/internal/report/report.go`. No synonym or deprecated alias may remain; the kind string `HIGHER_ORDER_BOUNDARY_CALL` must not appear anywhere in `go/`.
2. Remove `PassesFuncValue` from `facts.CallEdge` along with its doc comment. `CallEdge` keeps `Caller` and `Callee` only.
3. Remove the `passesFuncValue` helper in `go/internal/goanalysis/goanalysis.go` and the per-edge computation and merge (`oldPasses || passes`) in the call-graph walk. Edge collection becomes plain deduplication.
4. Preserve call-edge behavior otherwise: the same caller/callee pairs, the same intra-package edge exclusion, the same canonicalization, and the same deterministic sort by caller then callee.
5. Remove the `else if edge.PassesFuncValue` branch from the boundary-call loop in `go/internal/checker/checker.go`. A declared boundary call now produces no finding at all; the `CALLS_UNDECLARED_INTERFACE` violation for an undeclared callee is unchanged, as is the `init` skip that precedes it.
6. Verify the checker no longer imports anything only that branch needed, and that removing the branch does not change which dependencies are marked used.
7. Update every test that constructs or asserts the removed field or kind: `go/internal/facts/facts_test.go`, `go/internal/goanalysis/goanalysis_test.go` (the expected `CallEdge` table), and `go/internal/checker/checker_test.go`.
8. Convert, do not delete, the two end-to-end higher-order tests. `TestIntegration_...HigherOrder...` in `go/cmd/arcc/cli_integration_test.go` and the higher-order assertions in `go/cmd/arcc/app/app_test.go` must keep their fixtures and assert the new behavior: the run still succeeds or still reports its unrelated findings, and the output contains no `HIGHER_ORDER_BOUNDARY_CALL` and no "passes function value" text. These become the pinning tests for the plugin-struct pattern staying silent.
9. Keep the `TriggerHigherOrder`/`TriggerHigherOrderNamed` fixtures in `go/internal/goanalysis/testdata/success/a/b/b.go`; they still exercise call-edge extraction through function arguments, which remains correct behavior.
10. Update the README's limitation 2 ("Higher-Order Boundary Warnings"): it must not claim a mitigation by a kind that no longer exists. State the residual gap plainly; the replacement warning is documented in Task 3.
11. Update golden and rendered expectations that enumerate report kinds, and confirm the Bazel goldens under `bazel_rules/go/tests/goldens/` and the `arcc_check_test` expectations are unaffected or regenerated. `just bazel-test` must pass.
12. Exit-code behavior is unchanged: the removed kind was a warning, so no fixture may change its exit code as a result of this task.

## Dependencies
- Steps 1–7 are complete.
- This is the first task in Step 8. Tasks 2 and 3 add the replacement warning and do not depend on this task's mechanism, only on it having cleared the vocabulary.

## Implementation Approach
1. Delete the facts field first and let the compiler enumerate every producer, consumer, and fixture.
2. Simplify the call-graph walk to a deterministic edge set, confirming the edge table in `goanalysis_test.go` is otherwise byte-identical.
3. Remove the checker branch and the report constant, then repair rendering, JSON, and CLI tests.
4. Rewrite the two end-to-end higher-order tests as silence assertions over their existing fixtures.
5. Correct the README limitation, then run focused Go suites followed by `just ci`.

## Acceptance Criteria

1. **The kind is gone**
   - Given the repository after this task
   - When `HIGHER_ORDER_BOUNDARY_CALL` is searched for across `go/`, `bazel_rules/`, and rendered fixtures
   - Then there are no matches and no deprecated alias remains.

2. **The fact is gone**
   - Given `facts.CallEdge`
   - When its fields are inspected and the repository compiles
   - Then it carries only `Caller` and `Callee`, and no producer computes a func-value flag.

3. **Call edges are otherwise unchanged**
   - Given the `testdata/success` fixture used by the goanalysis loader tests
   - When package facts are loaded
   - Then the extracted caller/callee pairs and their order are identical to before the removal.

4. **A declared boundary call passing a callback is silent**
   - Given a member calling a declared interface symbol of a component dependency with a function-typed argument
   - When the checker runs
   - Then no warning is produced for that call site and the check still exits 0.

5. **Undeclared boundary calls still fail**
   - Given a member calling an undeclared symbol of a component dependency, with or without a func-valued argument
   - When the checker runs
   - Then `CALLS_UNDECLARED_INTERFACE` is still reported and the exit code is still 1.

6. **End-to-end tests pin the silence**
   - Given the converted CLI and application higher-order tests
   - When they run
   - Then they assert the absence of the removed kind and its message text rather than being deleted.

7. **Documentation makes no false claim**
   - Given the README's limitations section
   - When limitation 2 is read
   - Then it does not attribute a mitigation to a report kind the tool no longer emits.

8. **Repository checks remain green**
   - Given the removal
   - When `just ci` runs
   - Then generation-cleanliness, lint, unit, integration, self-check, and Bazel suites pass, including the Bazel golden tests.

## Metadata
- **Complexity**: Low
- **Labels**: Go, facts, checker, report, goanalysis, removal, goldens
- **Required Skills**: Go, pure data-model evolution, test migration, golden maintenance
