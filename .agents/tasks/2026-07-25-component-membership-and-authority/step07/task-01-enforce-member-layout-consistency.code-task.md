# Task: Enforce member and layout consistency

## Description
Make declared membership fail closed at the Go boundary by requiring a Bazel package layout's roots to match the manifest's literal members, and report local membership overlap with the dedicated `MEMBER_OVERLAP` violation.

## Background
The Bazel emitter and the Go loader describe the same ownership set in two artifacts. If the manifest and layout disagree, capability analysis can use a different root set from the checker and silently omit owned code. The loader must therefore compare the two declarations before analysis begins.

The checker already detects packages shared with a resolved component dependency, but it reports the legacy `PACKAGE_OVERLAP` vocabulary. Declared membership makes the actual defect precise: a manifest member is also covered or absorbed. Exact member/absorbed contradictions are rejected during manifest parsing, while pattern matches and resolved component coverage still require checker-time detection.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M6–M7, §3.1, §4.3–4.5, §5.3, §6, and §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 7)

**Additional References (if relevant to this task):**
- None.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. In package-layout mode, compare the canonicalized, order-independent set of `LoadRequest.Members` with the active layout's canonicalized `roots` before loading or analyzing packages.
2. Treat any difference as a hard load error. The error must identify both sets and the members missing from each side so an emitter or hand-written fixture can be repaired directly.
3. Apply the equality requirement only when a manifest declares members and an active root layout is being used. Preserve native-mode loading and legacy manifests whose empty `members` list selects FR1 behavior.
4. Ensure the interface package participates consistently in Bazel-mode equality. Bazel emitters will include the implicit interface member in both generated sets; native manifests may continue to omit it and rely on interface-file discovery.
5. Compare sets rather than source ordering, reject duplicate or contradictory data through the existing validation layers, and do not mutate either the manifest or layout while validating.
6. Replace the legacy `PACKAGE_OVERLAP` report kind with `MEMBER_OVERLAP` for a member package also covered by a resolved component dependency.
7. Detect overlap between the effective member set and absorbed declarations, including valid import-path patterns that match a member even when no exact-string contradiction was caught by manifest parsing.
8. Emit deterministic overlap findings that identify the member package and the covering component or absorbed declaration responsible for the conflict.
9. Keep the checker pure: root equality belongs at the loader/application boundary, while overlap remains derived only from injected manifest, fact, and dependency-interface data.
10. Update checker, report-rendering, JSON, CLI, and golden expectations that currently name `PACKAGE_OVERLAP`; do not retain two synonymous public kinds.
11. Add tests for equal sets in different orders, missing manifest members, extra layout roots, canonical spelling, legacy empty members, native mode, component-dependency overlap, absorbed exact overlap, and absorbed-pattern overlap.
12. Preserve exit-code behavior: roots/member inconsistency is a tool error (2), while `MEMBER_OVERLAP` is a conformance violation (1).

## Dependencies
- Steps 1–6 are complete. Manifest members, layout roots, canonicalization, member-scoped checking, and member-root fact loading already exist.
- No task within Step 7 precedes this task.
- `task-02-emit-declared-bazel-membership` depends on this fail-closed consumer before it changes generated artifacts.

## Implementation Approach
1. Add a focused helper near layout-mode fact loading that canonicalizes, sorts, and compares the requested members and active roots without affecting native behavior.
2. Call the helper before package loading can select roots and return a diagnostic containing both one-sided differences.
3. Rename the overlap kind and refactor the checker overlap pass to inspect both resolved component coverage and absorbed-pattern matches against effective members.
4. Add table-driven unit tests and focused CLI/layout integration tests that pin error level, messages, and legacy compatibility.
5. Update affected report goldens, then run focused Go suites and the repository-wide checks.

## Acceptance Criteria

1. **Matching membership loads**
   - Given a declared manifest and active package layout containing the same canonical member set in different orders
   - When package facts are loaded
   - Then loading succeeds and the layout roots remain the analysis roots.

2. **Missing layout root fails closed**
   - Given a manifest member absent from the active layout's roots
   - When the check starts in package-layout mode
   - Then it exits as a tool error naming the missing member and both declared sets.

3. **Extra layout root fails closed**
   - Given an active layout root absent from the manifest's members
   - When the check starts
   - Then it exits as a tool error naming the extra root rather than analyzing a broader ownership set.

4. **Legacy and native behavior are preserved**
   - Given an empty-members legacy manifest or a native-mode declared manifest whose interface package is implicit
   - When package facts are loaded
   - Then the new Bazel layout equality check does not reject the valid existing workflow.

5. **Covered member reports the new violation**
   - Given an effective member package also present in a resolved `component_dep`
   - When the checker runs
   - Then it emits one deterministic `MEMBER_OVERLAP` violation naming the package and component.

6. **Absorbed member reports overlap**
   - Given an effective member matched by an exact or patterned absorbed declaration
   - When the checker runs
   - Then it emits `MEMBER_OVERLAP` and does not silently resolve the package as absorbed.

7. **Legacy overlap vocabulary is removed**
   - Given all report constants, rendered fixtures, JSON expectations, and checker tests
   - When searched after the task
   - Then `PACKAGE_OVERLAP` is absent and overlap is represented only as `MEMBER_OVERLAP`.

8. **Failure levels remain distinct**
   - Given one roots/member mismatch and one member-overlap fixture
   - When each check runs
   - Then the mismatch returns tool-error exit code 2 and the overlap returns violation exit code 1.

9. **Repository checks remain green**
   - Given fail-closed membership validation and renamed overlap reporting
   - When `just ci` runs
   - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, goanalysis, packagelayout, checker, membership, fail-closed
- **Required Skills**: Go, package-loader design, pure validation, deterministic diagnostics, integration testing
