# Task: Honor declared membership in checker

## Description
Make the pure checker use explicit manifest members when present, retain FR1 membership when absent, and always treat the declared interface package as a component member.

## Background
Today the checker treats every package fact supplied by the loader as component-owned. With explicit membership, the loaded graph may contain non-member packages for type checking and call analysis, so FR2 import checks and intra-component decisions must be scoped to the member set. Compatibility requires the empty-members case to remain bit-for-bit FR1. For declared-style components the interface package is implicitly owned even when omitted from `members`; a declaration or pattern that also names it is harmless.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M1/M3/M5, §4.3, §7.2, and §10)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 1)

**Additional References (if relevant to this task):**
- None.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Build checker membership from `Manifest.Members` when that list is non-empty; otherwise retain the current FR1 behavior over loaded component facts.
2. Match membership consistently with the validated manifest form and do not treat arbitrary transitive facts as owned merely because they were loaded.
3. Force every package containing a declared interface file into the member set for declared-style components.
4. Treat a member declaration or valid member pattern that also selects the interface package as idempotent, not an overlap error.
5. Run FR2 import-boundary checks only for member packages, while still using the full fact graph where required to classify their imports and call edges.
6. Ensure intra-component import suppression is based on membership, not on all loaded facts.
7. Preserve deterministic finding order and every current checker result when `Members` is empty.
8. Add focused tests for explicit membership, non-member transitive facts, FR1 fallback, the implicit interface member, and a member pattern also matching the interface package.

## Dependencies
- Task 2: The native manifest exposes validated members and interface style.

## Implementation Approach
1. Isolate membership construction in a pure helper that accepts the manifest and package facts.
2. Reuse the resulting set for both the package sweep and intra-component import classification.
3. Derive interface-package membership from the same normalized file/package information used by existing FR4 interface checks; avoid directory-prefix assumptions that would break members outside the component root.
4. Add table-driven checker fixtures that include member and non-member facts in one input to pin scoping behavior.

## Acceptance Criteria

1. **Explicit membership scopes checking**
   - Given facts for a declared member and a loaded non-member dependency
   - When the checker runs with non-empty `Manifest.Members`
   - Then only the declared member is swept as owned code, and the non-member is not accepted as intra-component merely because its facts are present.

2. **FR1 fallback is unchanged**
   - Given the same checker input with `Manifest.Members` absent
   - When the checker runs
   - Then all supplied component packages are members and existing findings are bit-for-bit unchanged.

3. **Interface package is implicitly owned**
   - Given a declared-style manifest whose literal members omit the package containing an interface file
   - When the checker runs
   - Then that interface package is treated as a member and its imports are checked.

4. **Duplicate interface selection is harmless**
   - Given a member declaration or accepted pattern that also selects the interface package
   - When membership is constructed
   - Then no error or duplicate behavior occurs and the package is checked once.

5. **Transitive packages do not become an ownership bypass**
   - Given a member importing a non-member package present in `Facts.Packages`
   - When that import is neither stdlib, covered, nor absorbed
   - Then the checker reports `UNDECLARED_DEPENDENCY`.

6. **Repository checks remain green**
   - Given the completed pure-core change
   - When `just ci` runs
   - Then checker tests, self-checks, and existing examples pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, checker, functional-core, membership, FR1, FR2
- **Required Skills**: Go, pure data transformations, architectural-boundary testing
