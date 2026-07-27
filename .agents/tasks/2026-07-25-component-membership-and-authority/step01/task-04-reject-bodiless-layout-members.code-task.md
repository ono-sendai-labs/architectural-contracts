# Task: Reject bodiless layout members

## Description
Make package-layout validation reject a declared member/root that has no source files, so member ownership cannot silently point at an unanalyzable package.

## Background
Layout roots are the layout-mode representation of source-loaded component members. The current layout validator checks that roots resolve to package records but permits a root package with no source bodies, which would make ownership and authority attribution vacuous. Step 1 requires this condition to fail closed and name the member. Broader platform filtering and the empty-after-constraints rule land in Step 3; this task covers the directly observable bodiless-member contract without pulling later platform work forward.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M10, §4.4, §6, and Appendix C.7)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 1)

**Additional References (if relevant to this task):**
- None.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. During `ValidateAndResolve`, resolve each root by package ID or import path and verify that the root package has at least one source file available for analysis.
2. Treat `GoFiles` as the source-loaded membership signal and account for the validator's existing path resolution and build-constraint filtering order.
3. Return a deterministic error naming the bodiless member/root package.
4. Do not reject a non-root transitive package solely for being bodiless in this step; absorbed bodiless reporting is scheduled for Step 9.
5. Preserve all existing layout validation, standard-library discovery, canonical marshaling, and driver behavior.
6. Add tests for bodiless roots addressed by both ID and `PkgPath`, a sourced root, and a bodiless non-root.

## Dependencies
- Task 1 establishes the Step 1 membership contract; implementation is otherwise isolated in `packagelayout`.

## Implementation Approach
1. Reuse the layout's existing `byID`/`byPath` indexes to retain the resolved root package records.
2. Perform the source assertion after file normalization and the current constraint filtering so the check reflects files actually available to analysis.
3. Keep the helper narrow enough that Step 3 can later supply an explicit platform context without rewriting the M10 assertion.
4. Add small filesystem-backed fixtures rather than mocking successful nonexistent files.

## Acceptance Criteria

1. **Bodiless member fails closed**
   - Given a layout root whose package record has no Go source files
   - When `ValidateAndResolve` runs
   - Then it returns an error naming that root/member and explaining that it has no source files.

2. **Both root addressing forms are covered**
   - Given roots resolved once by package ID and once by package import path
   - When either resolved package is bodiless
   - Then both layouts fail with the same semantic error.

3. **Sourced member remains valid**
   - Given a root with at least one existing, constraint-matching Go source file
   - When validation runs
   - Then the M10 check succeeds and existing validation continues.

4. **Non-member bodies are deferred**
   - Given a sourced root and a bodiless non-root package in its layout closure
   - When validation runs
   - Then this task does not reject the layout solely because the non-root is bodiless.

5. **Repository checks remain green**
   - Given the completed layout validation
   - When `just ci` runs
   - Then all package-layout, driver, integration, and Bazel checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, package-layout, validation, fail-closed, M10
- **Required Skills**: Go, `go/packages` data modeling, filesystem-backed testing
