# Task: Report the absorbed func-value escape warning

## Description
Turn the loader-produced `facts.FuncValueEscape` into the `ABSORBED_FUNC_VALUE_ESCAPE` warning in the pure checker, rendered in text and JSON, and pin the behavior with the design's fixture-2 regression pair.

## Background
Task 2 makes the escape observable as a pure fact. This task gives it a name, a level, and a message a reader can act on, completing A5 and the residual-gap half of this design's soundness story: owned code is charged by attribution (Step 2), declared boundaries are pruned and checked, and the one remaining hole — a function value whose body is absorbed and which nothing in the component calls — is now visible instead of silent.

It is a **warning**, not a violation. The component has not exceeded its declared authority; the tool cannot see what the absorbed body does when someone else invokes it. Saying so is the honest report, and the exit-code contract must not change.

The checker stays pure: it does no matching, no path resolution, and no SSA reasoning. It reads the facts the loader collected and words them, exactly as it does for `UnresolvedImports` and `ANALYSIS_LIMITATION`.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A5, §4.5, §5.2, §5.3, §6, and §7.1 fixture 2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 8)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/capability-analysis-mechanics.md` (§1 — why this is the only escaping shape, which the message should reflect)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `AbsorbedFuncValueEscape Kind = "ABSORBED_FUNC_VALUE_ESCAPE"` to `go/internal/report/report.go`, grouped with the warning kinds.
2. In `go/internal/checker/checker.go`, emit one warning per `facts.FuncValueEscape`, next to the existing `UnresolvedImports` loop so the loader-collects/checker-reports split stays visible in one place.
3. The message must name the absorbed function symbol and the referencing member package, and say why it matters: the component takes the value of a function it never calls, whose body no analysis covers. Populate `Location` with the fact's file and line.
4. Findings must be deterministic in order and content for identical facts, and must not depend on map iteration.
5. The checker performs no classification: it must not consult `hostpolicy`, absorbed patterns, or package paths to decide whether to warn. The fact's presence is the decision.
6. Exit codes are unchanged: a component whose only finding is `ABSORBED_FUNC_VALUE_ESCAPE` exits 0; violations still exit 1 and tool errors 2.
7. The warning renders in both output paths — `RenderText` with its location line, and the JSON report with `kind`, `message`, and `location`.
8. Add the design's §7.1 fixture-2 regression pair as end-to-end tests: the note's Part 1 example with `backend` **absorbed** warns, and the same shape with the func value's body in a **member** produces no warning and a clean conforming report.
9. Add pure checker unit tests over hand-built facts: one escape produces one warning with the expected kind, message, file, and line; an empty or nil `FuncValueEscapes` produces none; multiple escapes render deterministically.
10. Update the README so the limitations section describes the warning that now exists — replacing the claim Task 1 removed — and states the residual gap it makes visible rather than fixes.
11. Update rendered fixtures, JSON expectations, and any enumeration of report kinds. Confirm the Bazel goldens under `bazel_rules/go/tests/goldens/` and the `arcc_check_test` expectations still hold; regenerate them if the added kind changes any generated artifact.
12. Do not add a Bazel rule attribute, manifest field, or suppression mechanism for this warning. Scope is the checker, the report, and their tests.

## Dependencies
- `task-02-detect-absorbed-func-value-escapes`: supplies `facts.FuncValueEscape` and its production; this task is the pure consumer.
- `task-01-remove-higher-order-boundary-warning`: the removal that this warning replaces; both must be complete for the report's kind set to be correct.
- This is the final task in Step 8.

## Implementation Approach
1. Add the report kind, then the checker loop over the new facts, keeping the message construction adjacent to the unresolved-import warning for symmetry.
2. Add pure checker tests over hand-built facts before touching any end-to-end fixture, so the wording and location are pinned independently of the loader.
3. Build the fixture-2 pair as two near-identical components differing only in whether the callback's body is absorbed or a member, and assert the warning in one and a clean report in the other.
4. Extend text and JSON rendering tests, then refresh goldens.
5. Run focused checker, report, application, and CLI suites, then `just ci` including the Bazel leg.

## Acceptance Criteria

1. **The warning exists with the right level**
   - Given the report kind set
   - When `ABSORBED_FUNC_VALUE_ESCAPE` is inspected
   - Then it is a warning kind and appears in the report's warning list, never among violations.

2. **A collected escape is reported**
   - Given checker input containing one `FuncValueEscape`
   - When the checker runs
   - Then exactly one `ABSORBED_FUNC_VALUE_ESCAPE` warning names the absorbed symbol and the referencing member package and carries the fact's file and line.

3. **No facts, no warning**
   - Given checker input whose `FuncValueEscapes` is empty or nil
   - When the checker runs
   - Then no warning of this kind is produced.

4. **Absorbed callback warns end to end**
   - Given the design §7.1 fixture-2 component with `backend` absorbed and its function value handed across a boundary
   - When `arcc check` runs
   - Then the output contains `ABSORBED_FUNC_VALUE_ESCAPE` naming the absorbed function, and the exit code is 0.

5. **Member callback stays silent end to end**
   - Given the same component with the callback's body in a member package
   - When `arcc check` runs
   - Then no warning of this kind appears and the report is the conforming success line.

6. **Both renderings carry the finding**
   - Given a report containing the warning
   - When it is rendered as text and as JSON
   - Then the text output shows the kind, message, and `at file:line`, and the JSON output carries `kind`, `message`, and `location`.

7. **The checker stays pure**
   - Given the checker package after this task
   - When its imports and the new code path are inspected
   - Then the warning is derived only from injected facts, with no host policy, glob matching, or filesystem access.

8. **Exit-code contract is unchanged**
   - Given one component whose only finding is this warning and one with a real violation
   - When each is checked
   - Then the first exits 0 and the second still exits 1.

9. **Output is deterministic**
   - Given facts containing several escapes
   - When the checker runs repeatedly
   - Then the warnings appear in the same order with identical text every time.

10. **Documentation matches the tool**
    - Given the README's limitations section
    - When the higher-order/callback limitation is read
    - Then it describes `ABSORBED_FUNC_VALUE_ESCAPE` accurately, including that it warns rather than proves the absorbed body safe.

11. **Repository checks remain green**
    - Given the new warning and refreshed goldens
    - When `just ci` runs
    - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass, including the Bazel golden tests.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, checker, report, facts, warnings, goldens
- **Required Skills**: Go, pure core design, deterministic reporting, end-to-end fixture construction
