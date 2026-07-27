# Task: Annotate certified and asserted boundaries in the report

## Description
Add a dependency listing to the conformance report that marks each pruned
boundary **certified** (the dependency's own conformance check runs) or
**asserted** (it does not), carrying the certification reference where one is
recorded. This is an annotation, deliberately not a finding.

## Background
Pruning at a dependency is a transfer of trust: the depender stops being charged
for authority behind the boundary. Today the report says nothing about who, if
anyone, is checking the other side. A8 makes that visible.

It is emphatically **not** a finding, for three reasons the design states and
which should shape the implementation:

1. It is not a defect in the component being checked. The depender did nothing
   wrong by pruning at a boundary someone else failed to certify.
2. It would fire with a 100% hit rate. Where a runtime is injected everywhere,
   every component prunes at the same uncertified dependency, identically,
   forever — and a warning that always fires gets suppressed wholesale in CI,
   which then hides the interesting instances too.
3. The subject is wrong. "This component is not certified" is a property of
   *that* component, not of the N that depend on it. Reporting it once is the
   fix, and once belongs in a repo-wide check, deferred with the repo-wide
   membership-uniqueness check.

So: always visible, never suppressible, no severity, no exit-code effect.

Both underlying fields are **self-declarations at the same trust level as
`declared_authority`** — reviewable, not proven. The rendering must not imply
arcc verified anything; "certified" here means the dependency declared that its
own check runs.

The report currently returns early on the clean path with a single success line.
The listing has to survive that path — a clean report is exactly where a reader
wants to see which boundaries were merely asserted.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A8, §4.1 — the two certification fields and their trust level, §4.3 table, §5.3 "No new kind for an uncertified boundary")
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 9)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a pure listing type to `report` — one entry per resolved component
   dependency — carrying at least the dependency name, whether its own check
   runs, and the certification reference when recorded. Add it as a field on
   `ConformanceReport` with a JSON tag consistent with the existing fields.
2. Document on the type that both values are the dependency's own
   self-declaration, not something arcc verified, so the first reader does not
   assume otherwise.
3. The checker populates the listing from `facts.DependencyInterface` (Task 2)
   and returns it as part of the report. No new checker input, no I/O; the
   checker stays pure.
4. Order the listing deterministically by dependency name.
5. Produce **no** finding, of any kind or level, from certification state. Exit
   codes are unchanged: a report whose only notable content is an asserted
   boundary still exits 0.
6. `RenderText` shows the listing in both shapes: on the clean path, after the
   success line rather than instead of it, and on the findings path alongside
   violations and warnings. A component with no component dependencies renders
   no listing section at all, so today's single-line clean output is unchanged
   for such components.
7. Render the reference when present and omit the trailing detail when absent,
   with wording that reads as an annotation rather than a complaint.
8. The JSON form carries the same information; a component with no component
   dependencies must not gain a noisy empty section (omit it).
9. Update the text and JSON goldens, including the Bazel golden test, for the
   components that do have dependencies. Do not normalize the annotation away —
   it is the thing under test.
10. Absorbed dependencies do not appear in the listing: the boundary this
    annotates is the pruned one.

## Dependencies
- `task-02-resolve-package-surface-dependency-surface` supplies `OwnCheckRuns`
  and `CertificationReference` on `facts.DependencyInterface`.
- Step 1 supplies both fields in the schema and in `manifest`.

## Implementation Approach
1. Add the pure type and the `ConformanceReport` field, then populate it in
   `Check` from the resolved dependency interfaces.
2. Restructure `RenderText`'s early return so the clean path emits the success
   line and then the listing, keeping the no-dependency output byte-identical.
3. Write report-level rendering tests for certified, asserted-with-reference,
   asserted-without-reference, and no-dependencies, in both text and JSON.
4. Add a checker test asserting that certification state produces no finding and
   does not change the exit-code-relevant violation set.
5. Regenerate the affected goldens deliberately, inspecting each diff.
6. Run focused `checker`, `report`, and `app` suites, then `just ci`.

## Acceptance Criteria

1. **A dependency whose own check runs is certified**
   - Given a dependency manifest declaring `own_check_runs: true`
   - When the report is rendered
   - Then its listing entry reads as certified, in both text and JSON.

2. **A dependency without its own check is asserted**
   - Given a dependency declaring `own_check_runs: false` and a `certification_reference`
   - When the report is rendered
   - Then its entry reads as asserted and shows the reference.

3. **A missing reference degrades cleanly**
   - Given a dependency declaring neither
   - When the report is rendered
   - Then its entry reads as asserted with no reference detail and no placeholder text.

4. **The annotation is not a finding**
   - Given a component whose every dependency is asserted and which has no violations or warnings
   - When `arcc check` runs
   - Then no violation or warning is produced, the exit code is 0, and the success line is still printed.

5. **The listing survives the clean path**
   - Given a conforming component with component dependencies
   - When the text report is rendered
   - Then it contains both the success line and the dependency listing.

6. **No dependencies means no section**
   - Given a component with no component dependencies
   - When text and JSON reports are rendered
   - Then the output is identical to before this task, with no empty listing section.

7. **Ordering is deterministic**
   - Given several dependencies supplied in varying order
   - When the report is rendered repeatedly
   - Then the listing is sorted by dependency name and byte-identical across runs.

8. **Absorbed dependencies are excluded**
   - Given a component with both component and absorbed dependencies
   - When the report is rendered
   - Then only the component dependencies appear in the listing.

9. **Goldens pin the annotation**
   - Given the text, JSON, and Bazel goldens
   - When the golden tests run
   - Then they compare the rendered certified/asserted state rather than normalizing it away.

10. **Repository checks remain green**
    - Given the new listing
    - When `just ci` runs
    - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass with only the intentional golden changes.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, report, checker, pure-core, certification, goldens
- **Required Skills**: Go, deterministic rendering, golden-test maintenance
