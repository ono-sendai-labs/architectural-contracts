# Task: Report excluded interface files

## Description
Make build-constraint exclusions from a component's declared interface visible without rejecting a valid cross-platform interface. Emit one deterministic `INTERFACE_FILE_EXCLUDED` warning per excluded file, and fail validation when no declared interface file survives for the analysis platform.

## Background
`goanalysis.ValidateInterfaceFiles` currently treats a declared file that exists but is absent from the loaded source set as valid when the active `build.Context` excludes it. That skip is necessary for platform-specific interfaces, but it silently shrinks the contract being checked. The validator must return structured exclusion information to the application, which adds warnings to the final conformance report after the pure checker runs.

The exclusion decision must use the same active build context as package loading: `build.Default` in native or legacy layout use, and the layout-declared platform when present. A partly gated interface remains checkable and passes with warnings; an entirely gated declared interface is a load/validation error rather than a vacuous conformance result.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 T3, §4.5, §5.3, §6, and §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 4)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/build-platform-and-tags.md` (active `go/build.Context` inputs and constraint semantics)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Change `goanalysis.ValidateInterfaceFiles` to return deterministic structured information for every declared interface file excluded by the active build constraints, in addition to an error. Keep the result independent of the `report` package so the loader/validator does not own presentation.
2. Include enough data in each exclusion to render the declared file path and the constraint that excluded it. Report explicit `//go:build` or legacy build expressions and filename/platform constraints in a stable, human-readable form as applicable; do not emit a generic warning that omits the cause.
3. Continue using the layout-derived `build.Context` in layout mode and `build.Default` otherwise, matching the source-membership decision already used by package loading and `FileMatchesBuildConstraintsWithContext`.
4. Preserve all existing validation failures for absolute paths, root escapes, missing or non-regular files, and files that should compile for the active platform but do not belong to any loaded package.
5. Preserve the current fail-open behavior when a file's constraints cannot be evaluated because the file is unreadable; do not mislabel an I/O failure as a constraint exclusion.
6. After validating all declared files, return an error when every declared interface file was excluded. The error must identify that no interface file survives the analysis platform and provide useful file/constraint context.
7. Do not treat a partially excluded interface as an error. Return all exclusions while allowing the check to continue using the surviving files.
8. Add `report.InterfaceFileExcluded` with serialized value `INTERFACE_FILE_EXCLUDED` as a warning kind.
9. In the application flow, translate every returned exclusion into a `report.Finding` warning after `checker.Check`, naming the declared file and excluding constraint and setting a useful file location. Preserve deterministic warning order relative to checker-produced warnings.
10. Ensure the warning is present in both text rendering and `--format=json`, and that warnings alone retain exit status 0.
11. Cover native-mode and layout-mode validation, including a declared Linux analysis platform tested independently of the host platform. Avoid tests whose expected result changes when run on Windows.
12. Add focused report tests for the new kind and end-to-end CLI/layout tests for text and JSON output. Update affected deterministic and Bazel-facing expected output without weakening unrelated assertions.
13. Do not reword the clean success line in this task; that is Task 2. Do not change package source filtering, import consistency, or standard-library classification.

## Dependencies
- Step 3 (complete): supplies a layout-declared build context and consistent post-filter source membership.
- No task within Step 4 precedes this task.
- `task-02-reword-conformance-success` follows this task and refreshes the final Step 4 report expectations.

## Implementation Approach
1. Introduce a small exclusion value type in `goanalysis`, and refactor the existing skip branch to recover a deterministic description of the build expression or filename constraint while retaining the active-context match as the authoritative verdict.
2. Track surviving and excluded declared interface files during the single validation pass. Return collected exclusions for a partial interface and a precise error for an all-excluded interface.
3. Add the report kind, then translate exclusions at the application boundary into sorted warning findings before selecting text or JSON rendering.
4. Build fixtures with one portable and one platform-gated interface file, plus an all-gated variant. Exercise both an explicitly declared Linux layout and native mode where useful.
5. Pin the exact warning kind, file, constraint, exit status, JSON fields, text rendering, and hard-error message before running the full repository checks.

## Acceptance Criteria

1. **Partial platform interface warns and passes**
   - Given a component declares one surviving interface file and one `//go:build windows` interface file while the analysis platform is Linux
   - When `arcc check` runs
   - Then it exits successfully and reports one `INTERFACE_FILE_EXCLUDED` warning naming the Windows file and its constraint.

2. **Every exclusion is retained deterministically**
   - Given multiple declared interface files are excluded by build expressions or filename platform suffixes
   - When validation and checking are repeated
   - Then every exclusion appears once with stable ordering and a stable, specific constraint description.

3. **Fully gated interface fails closed**
   - Given every declared interface file is excluded for the active analysis platform
   - When interface validation runs
   - Then the check returns a tool error identifying that no interface file survives and does not emit a vacuous conformance report.

4. **Declared platform controls the verdict**
   - Given the same source tree is checked with otherwise identical layouts declaring Linux and Windows
   - When interface files use platform build constraints
   - Then each run reports exclusions according to the layout platform rather than the host running `arcc`.

5. **Non-constraint validation remains strict**
   - Given an interface path is absolute, escapes the root, is missing or non-regular, or should compile but is not in a loaded package
   - When validation runs
   - Then the existing precise validation error remains and is not converted into an exclusion warning.

6. **Warnings reach both report formats**
   - Given a partially excluded interface
   - When the component is checked in text mode and JSON mode
   - Then both outputs contain `INTERFACE_FILE_EXCLUDED`, the file, and the excluding constraint, and JSON classifies it under `warnings`.

7. **Warning-only reports do not fail conformance**
   - Given interface exclusion is the only finding
   - When `arcc check` completes
   - Then the process exits 0 because the report contains no violations.

8. **Report kind is pinned**
   - Given a finding constructed with `report.InterfaceFileExcluded`
   - When it is rendered and JSON-marshaled
   - Then its stable external value is exactly `INTERFACE_FILE_EXCLUDED`.

9. **Step 4 exclusion demo is reproducible**
   - Given a component with a platform-gated interface file
   - When it is checked in text and JSON modes, then changed so the whole interface is gated
   - Then the first two runs visibly warn and the final run fails with the exclusion reason.

10. **Repository checks remain green**
    - Given the interface-exclusion diagnostic implementation
    - When `just ci` runs
    - Then unit, integration, self-check, generation-cleanliness, and Bazel suites pass with only intentional expectation changes.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, goanalysis, build-constraints, report, CLI, diagnostics
- **Required Skills**: Go, `go/build`, build-expression parsing, deterministic diagnostics, JSON and integration testing
