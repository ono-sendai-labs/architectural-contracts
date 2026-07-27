# Task: Reword conformance success

## Description
Replace the clean report's claim that a conforming component is ambient-authority-free with wording that accurately says the component stays within its declared authority.

## Background
A component may conform while legitimately using authority listed in `declared_authority`. The current text, `Component %q conforms / ambient-authority-free`, therefore overclaims what the checker established. The report contract should instead render exactly `Component %q conforms; does not exceed declared authority`.

This is a report-surface migration: renderer tests, application and CLI integrations, examples or user documentation that quote the output, self-check expectations, and Bazel-facing tests must agree on the new sentence. Internal prose that accurately describes a package or function as free of ambient authority is not stale solely because it uses the same phrase.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 T7 and §5.3)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 4)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Change the no-findings text rendering to exactly `Component %q conforms; does not exceed declared authority` followed by the existing newline.
2. Apply the wording only to a clean report with no violations and no warnings. Reports containing the new `INTERFACE_FILE_EXCLUDED` warning or any existing finding continue using the detailed component/findings format.
3. Update exact and substring expectations in `report`, application, CLI, layout-mode, self-check, and Bazel-facing tests to the new wording.
4. Update checked-in golden outputs if any Bazel or integration fixture captures the clean report. Do not change generated manifest or package-layout goldens that do not contain report text.
5. Update README or other user-facing documentation that quotes the old success output so documented behavior matches the executable.
6. Review remaining uses of `ambient-authority-free` individually. Preserve uses that describe an actual architectural or implementation property rather than the old report sentence.
7. Preserve text and JSON schemas for findings, sorting, exit-code behavior, and all non-clean rendering.
8. Do not change authority evaluation, warning/violation classification, manifests, or interface-file validation in this task.

## Dependencies
- `task-01-report-excluded-interface-files`: establishes the final Step 4 warning surface whose reports must continue using detailed rendering.
- Steps 1–3 (complete): the existing conformance and report behavior being reworded.
- No later Step 4 task depends on this task; it completes Step 4.

## Implementation Approach
1. Update the report renderer and its exact clean-output unit test first.
2. Search every repository occurrence of the old sentence and classify it as an output expectation, quoted user documentation, or a still-valid conceptual statement.
3. Update only output expectations and quoted documentation, retaining meaningful architectural claims and comments.
4. Run focused report, app, CLI, layout, self-check, and Bazel tests, then run the full repository CI to catch hidden golden or substring dependencies.

## Acceptance Criteria

1. **Clean report uses declared-authority wording**
   - Given a conformance report with no violations and no warnings
   - When text rendering runs
   - Then the output is exactly `Component "<name>" conforms; does not exceed declared authority` plus a newline.

2. **Declared authority is not described as absent**
   - Given a component declares and uses allowed authority without producing findings
   - When its result is rendered
   - Then the success text says it did not exceed declared authority and does not call it ambient-authority-free.

3. **Finding reports retain their format**
   - Given a report contains a violation, an existing warning, or `INTERFACE_FILE_EXCLUDED`
   - When text rendering runs
   - Then it uses the existing detailed `Component:`, `Violations:`, and/or `Warnings:` sections with unchanged finding serialization.

4. **All executable expectations migrate**
   - Given unit, CLI, layout, self-check, and Bazel-facing suites
   - When they assert clean output
   - Then they expect the new sentence, with no stale exact or substring expectation for the old success line.

5. **User documentation matches**
   - Given documentation that quotes the command's clean success output
   - When it is read alongside current behavior
   - Then it shows the new declared-authority wording.

6. **Valid conceptual prose is preserved**
   - Given comments or examples use `ambient-authority-free` to describe an actual pure component or code property rather than report output
   - When the migration is reviewed
   - Then those accurate statements are not mechanically rewritten.

7. **Step 4 report demo is reproducible**
   - Given a clean component and a component with a platform-excluded interface file
   - When both are checked
   - Then the clean component shows the new success sentence and the warned component shows the detailed warning report.

8. **Repository checks remain green**
   - Given the completed report wording migration
   - When `just ci` runs
   - Then all unit, integration, self-check, generation-cleanliness, and Bazel suites pass.

## Metadata
- **Complexity**: Low
- **Labels**: Go, report, CLI, documentation, golden-tests
- **Required Skills**: Go, deterministic text rendering, integration and golden testing, documentation maintenance
