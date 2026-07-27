# Task: Migrate csvtool membership

## Description
Migrate the csvtool Bazel examples and their checked-in manifest parity fixtures from implicit transitive absorption to explicit component membership and boundaries, then add the note's four-package/three-utility regression scenario as an end-to-end demonstration.

## Background
The csvtool components are the repository's executable Bazel examples and manifest-parity reference. They currently use `absorbed_deps` to account for `parsecsv`, relying on transitive absorption semantics that Step 7 is designed to make exceptional rather than the default ownership model. Their BUILD declarations and checked-in manifests must demonstrate the final explicit model without assigning the same package ambiguously or weakening the existing FILES and cross-component pruning checks.

The source note also calls out a concrete reasoning hazard: four implementation packages are authored, while three utility packages are pulled in transitively and silently become owned. A durable regression fixture should prove those utilities now surface individually, and that declaring the correct memberships or component boundaries makes the check pass.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M4/M6/M7, §3.1, §7.1 fixture 3, §7.3, §8, and §10)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 7)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-24-callback-authority-attribution/callback-authority-attribution.md` (Part 2, the four authored implementation packages and three transitive utilities)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Update every csvtool `go_component` declaration to state its owned package labels explicitly through `members` while preserving the interface package's implicit inclusion.
2. Remove blanket `absorbed_deps` declarations used only to make ordinary in-repository implementation or utility packages disappear into a transitive closure.
3. Give shared packages such as `internal/parsecsv` one unambiguous architectural home. If they are shared across components, represent that ownership with a component boundary rather than declaring overlapping membership in unrelated components.
4. Preserve csvtool behavior and its architectural intent:
   a. `csvfile` still declares and reports FILES for `os.ReadFile`;
   b. `app` still composes `csvfile` and `toprow` through pruned component boundaries and does not inherit FILES;
   c. `toprow` remains authority-free.
5. Update checked-in `component.textproto` files to the same explicit member/dependency model as the Bazel-generated manifests.
6. Extend `manifest_parity_test` normalization and comparisons so `members`, component dependencies, absorbed dependencies, interface style, and declared authority are compared semantically. Do not normalize away the newly declared membership.
7. Ensure each generated csvtool manifest's member set equals its generated layout roots and each `.check` target succeeds under Task 1's fail-closed validation.
8. Add or extend an end-to-end Bazel fixture reproducing the source note's classification hazard: four declared implementation/member packages reach three utility packages that are neither members, covered, absorbed, nor stdlib.
9. In the fixture's failing form, assert that the check reports all three utility import paths as `UNDECLARED_DEPENDENCY`; do not accept an analysis-time Starlark failure or a single generic diagnostic.
10. Add a passing form that accounts for the utilities through the architecturally correct explicit membership or component dependencies, proving the reported diagnostics are actionable.
11. Keep the fixture hermetic and deterministic. Use synthetic packages with no incidental ambient authority so membership diagnostics are isolated from capability findings.
12. Update comments and example documentation adjacent to the BUILD declarations so they describe declared membership and narrow absorption accurately.
13. Run the csvtool Go tests, manifest parity test, all csvtool `.check` targets, the focused regression fixture, and the full repository checks.

## Dependencies
- `task-01-enforce-member-layout-consistency` provides roots/member equality and `MEMBER_OVERLAP`.
- `task-02-emit-declared-bazel-membership` provides final Bazel classification, manifest/layout/platform emission, and undeclared-remainder behavior.
- No later Step 7 task depends on this task.

## Implementation Approach
1. Map csvtool packages to a single ownership graph, introducing a small explicit component for a genuinely shared utility if necessary rather than duplicating membership.
2. Update BUILD declarations and checked-in manifests together, preserving the existing interface files and authority declarations.
3. Strengthen the parity test's semantic normalization to compare members and the revised dependency graph.
4. Build a compact Bazel testdata graph with four authored members and three transitive utilities, plus failing and repaired component/check targets.
5. Assert exact undeclared-package diagnostics in a shell or integration test and verify the repaired target conforms.
6. Run focused tests, then `just ci`, and leave the implementation-plan checklist unchanged.

## Acceptance Criteria

1. **Csvtool ownership is explicit**
   - Given the csvtool BUILD files
   - When their component declarations are inspected
   - Then owned non-interface packages are listed through `members`, and ordinary in-repository packages are not hidden by blanket absorption.

2. **Shared packages have one owner**
   - Given `internal/parsecsv` is used by more than one csvtool package
   - When the component graph is resolved
   - Then it is owned by exactly one component and consumers cross an explicit component boundary without `MEMBER_OVERLAP`.

3. **Csvfile retains FILES**
   - Given the migrated `csvfile` component
   - When its generated check runs
   - Then its FILES declaration still covers the `os.ReadFile` capability and the component conforms.

4. **App pruning remains correct**
   - Given the migrated app depends on csvfile and toprow components
   - When the app check runs
   - Then dependency pruning prevents csvfile's FILES authority from being re-attributed to app.

5. **Manifest parity includes membership**
   - Given generated and checked-in csvtool manifests
   - When `manifest_parity_test` runs
   - Then it fails on a member or dependency mismatch and passes only when the explicit ownership models agree.

6. **Generated roots match members**
   - Given each migrated csvtool component
   - When its manifest and package layout are loaded
   - Then the literal member set equals the layout roots and no tool-error mismatch occurs.

7. **Three utilities surface**
   - Given the four-member regression component in its uncorrected form
   - When its Bazel check runs
   - Then it reports `UNDECLARED_DEPENDENCY` for each of the three utility import paths that old transitive absorption would have swept in.

8. **The scenario has a passing repair**
   - Given the same fixture with correct explicit ownership or component boundaries
   - When its check runs
   - Then all three utility imports are accounted for and the component conforms.

9. **Demo is reproducible end to end**
   - Given the failing and repaired fixture targets
   - When run through `bazel test`
   - Then the first expected-output test proves the three named failures and the repaired component's `.check` passes.

10. **Repository checks remain green**
   - Given migrated examples, strengthened parity, and the regression scenario
   - When `just ci` runs
   - Then unit, integration, generation-cleanliness, self-check, csvtool, and Bazel suites pass.

## Metadata
- **Complexity**: High
- **Labels**: Bazel, examples, migration, members, component-boundaries, regression-fixture
- **Required Skills**: Starlark, Go, architectural decomposition, Bazel integration testing, manifest parity testing
