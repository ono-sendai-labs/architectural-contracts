# Task: Prove a broken self-manifest fails `bazel test`

## Description
Add the negative test Step 10 asks for: a self-component declaration with one
declared dependency removed must fail its check, reporting the now-undeclared
package by name. Then confirm the two dogfooding legs — native `just selfcheck`
and the Bazel `.check` targets — cover the same eight components, and say so
where a reader of the `justfile` will see it.

## Background
Tasks 1–4 make the Bazel leg check arcc against its own manifests and prove the
two manifest forms agree. What none of them proves is that the Bazel leg
*fails* when a manifest stops describing the code. Without that, eight green
checks are consistent with a check that cannot go red — and the regression that
motivated this whole requirement (a `hostpolicy` seam dropped from a manifest,
undetected because nothing in the Bazel leg looked) would still slip through.

The rules already provide the mechanism. `arcc_check_test(expect_violation =
True)` passes iff arcc exits 1, and `arcc_check_grep_test` additionally asserts
the diagnostic text, which is what distinguishes "failed" from "failed for the
right reason". Both live in `//bazel_rules/go/private:check.bzl`, whose
visibility is `//bazel_rules:__subpackages__`, so the test target belongs in
`//bazel_rules/go/tests` alongside `undeclared_member_dep_fails_test` and
`violation_component_check_fails_test`. The deliberately broken component
belongs next to the component it mirrors, tagged `manual` so its
macro-generated `.check` does not fail `bazel test //...` on its own — the
idiom `//bazel_rules/go/tests/testdata/violation` already uses.

`checker` is the right subject: it is authority-free, so the only thing its
check can complain about is the dependency graph, and it has four component
dependencies to remove one of.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 B3; §7.3 "The bazelified self-check")
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 10 — "Tests" and "Demo")

**Additional References (if relevant to this task):**
- `bazel_rules/go/tests/BUILD.bazel` — `undeclared_member_dep_fails_test` and `violation_component_check_fails_test`, the two negative-check precedents.
- `bazel_rules/go/tests/testdata/violation/BUILD.bazel` — the `manual`-tagged deliberately-failing fixture idiom, including its comment explaining why the auto `.check` is suppressed.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a deliberately incomplete fixture component to
   `go/internal/checker/BUILD.bazel` that mirrors `checker_component` with one
   `component_deps` entry removed — drop the `report` component — and is tagged
   `manual` so its generated `.check` stays out of `bazel test //...`. Give it
   visibility reaching `//bazel_rules:__subpackages__` so the rules' test
   package can drive it.
2. Drive it from `bazel_rules/go/tests/BUILD.bazel` with an
   `arcc_check_grep_test` that requires a violation (arcc exit 1) and asserts
   the diagnostic names both `UNDECLARED_DEPENDENCY` and the undeclared import
   path `github.com/ono-sendai-labs/architectural-contracts/go/internal/report`.
   An unqualified non-zero exit is not sufficient: a tool error (exit 2) or a
   different finding would otherwise pass the test.
3. Comment the fixture with what it is and what keeps it honest: it must stay a
   copy of `checker_component` minus exactly one edge, so that if `checker`'s
   real dependency set changes, the fixture is updated rather than left
   describing a component that no longer exists.
4. Confirm the two legs cover the same set: the eight `.check` targets that
   `bazel test //...` runs correspond one-to-one with the eight `arcc check`
   invocations in the `justfile`'s `selfcheck` recipe. Record that relationship
   in a comment on the `selfcheck` recipe — it explains why the apparently
   redundant native leg is kept: native FR1 membership and Bazel declared
   membership checking the same components is a cross-check of the whole
   membership model, not duplicated work.
5. Do not remove or weaken `just selfcheck`, and do not remove the `bazel-test`
   recipe's extra shell-driven validation tests.
6. Keep prose in `README.md` and the design records for Step 11; the `justfile`
   comment is the only documentation change in scope here.
7. Verify the demo the plan describes actually works and is reproducible from
   the task: `bazel test //...` runs all eight self-checks, and deleting an
   `absorbed_dependencies` entry — for example `//go/internal/hostpolicy` from
   `goanalysis_component` — makes the corresponding `.check` fail. Do not commit
   that deletion; the committed negative is the `checker` fixture.

## Dependencies
- `task-02-declare-analysis-self-components` provides `checker_component`, the
  component the fixture mirrors, and `goanalysis_component`, used in the manual
  demo verification.
- `task-01-declare-core-self-components` provides the `report` component whose
  edge the fixture omits.
- Independent of `task-04-self-manifest-parity-test`.

## Implementation Approach
1. Copy `checker_component` into a second, `manual`-tagged target in the same
   BUILD file with `report` dropped from `component_deps`.
2. Add the `arcc_check_grep_test` in `//bazel_rules/go/tests` and run it;
   confirm it fails when pointed at the *intact* `checker_component` (a
   conforming component exits 0) and passes against the fixture, so the test is
   known to discriminate.
3. Cross-read the `selfcheck` recipe against `bazel query 'kind("arcc_check_test
   rule", //...)'` (or the equivalent target listing) to confirm the eight-to-
   eight correspondence, then write the recipe comment.
4. Perform the plan's demo by hand — delete an absorbed entry, watch the check
   fail, restore it — and confirm the failure names the dropped package.
5. Run `just ci`.

## Acceptance Criteria

1. **A broken self-declaration fails**
   - Given the `manual`-tagged checker fixture with the `report` component
     dependency removed
   - When its check runs under `bazel test`
   - Then arcc exits 1 and the test passes only because of that violation.

2. **It fails for the stated reason**
   - Given the same fixture
   - When the diagnostic is inspected
   - Then it contains `UNDECLARED_DEPENDENCY` and the
     `.../go/internal/report` import path, and a tool error or unrelated finding
     would fail the test instead of satisfying it.

3. **The fixture does not pollute the green run**
   - Given `bazel test //...`
   - When it runs
   - Then the fixture's macro-generated `.check` does not run, the driving
     negative test does, and the eight real self-checks still pass.

4. **Both legs cover the same eight components**
   - Given the `justfile`'s `selfcheck` recipe and the Bazel check targets
   - When the two lists are compared
   - Then they name the same eight components, and the recipe carries a comment
     explaining why both legs are kept.

5. **The demo is reproducible**
   - Given the plan's demo instructions
   - When an `absorbed_dependencies` entry is deleted from a self-component
     declaration
   - Then the corresponding `.check` fails naming the dropped package, and
     restoring the entry makes it pass again.

6. **Repository checks pass**
   - Given the fixture, the negative test and the comment
   - When `just ci` runs
   - Then every leg is green.

## Metadata
- **Complexity**: Low
- **Labels**: Bazel, dogfooding, negative-test, regression, self-check
- **Required Skills**: Starlark, Bazel test rules, arcc diagnostics
