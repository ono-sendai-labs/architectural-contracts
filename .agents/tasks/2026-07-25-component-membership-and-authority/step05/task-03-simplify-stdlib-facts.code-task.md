# Task: Simplify standard-library facts

## Description
Remove the unused per-package standard-library flag and make `PackageFacts.StdlibImports` the single, always-populated loader-authoritative representation consumed by the pure checker.

## Background
`facts.PackageFact.IsStdlib` has no checker consumer and duplicates classification state that is already expressed by `PackageFacts.StdlibImports`. Meanwhile the checker interprets a nil `StdlibImports` slice as permission to fall back to its own path-policy classification. That nil sentinel embeds policy in data shape and recreates the single-signal fail-open that Step 5 is removing.

After Task 1, both native and layout loaders have an authoritative provenance-backed classifier. They must always populate the canonical standard-library import set, including an explicit empty slice when there are none. The checker can then consume one pure fact without consulting host policy or guessing from nil-ness.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 T4 and T6, §4.3, §5.2, and §6)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 5)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Remove `IsStdlib` from `facts.PackageFact` and update all constructors, fixtures, equality assertions, serialization assumptions, and comments that reference it.
2. Keep `PackageFacts.StdlibImports` as the sole pure-facts representation of direct standard-library imports relevant to checker dependency classification.
3. Ensure every production loader path initializes `StdlibImports` to a non-nil slice, even when the authoritative result is empty. Preserve canonical paths, uniqueness, and deterministic sorting.
4. Update `ResolveDependencyInterface` and any other secondary fact producers to satisfy the same non-nil contract where they construct `PackageFacts`.
5. Remove checker behavior that treats `StdlibImports == nil` as a signal to classify imports with `hostpolicy.IsStdlibPath`. The checker must consume only the supplied authoritative set.
6. Remove the resulting pure-checker dependency on host standard-library policy if no other checker behavior requires it.
7. Update hand-built checker test inputs to declare their intended standard-library imports explicitly. A nil slice in a test must behave as an empty authoritative set, not activate a compatibility fallback.
8. Add a regression test in which a dotless or rewriting-host import is absent from `StdlibImports`; verify the checker treats it as non-stdlib and can report `UNDECLARED_DEPENDENCY`.
9. Preserve all checker behavior unrelated to the source of standard-library classification, including member, covered, absorbed, and declared dependency handling.
10. Update facts documentation to say `StdlibImports` is loader-authoritative, canonical, deterministic, and always populated.

## Dependencies
- `task-01-classify-stdlib-from-provenance`: supplies the authoritative classifier used to populate `StdlibImports`.
- `task-02-enforce-canonical-loader-paths`: guarantees loader and dependency facts use one canonical package namespace.
- This is the final task in Step 5.

## Implementation Approach
1. Change the facts model and compile to identify every producer and test fixture that still initializes or checks `PackageFact.IsStdlib`.
2. Centralize normalization of the standard-library import set so each loader returns a sorted, unique, non-nil slice.
3. Simplify the checker to build its standard-library set directly from facts without a nil-policy branch.
4. Migrate pure checker tests to explicit facts and add regressions for empty/nil authoritative sets under dotless and rewriting path policies.
5. Run focused facts, goanalysis, checker, application, self-check, and Bazel tests before the full repository CI.

## Acceptance Criteria

1. **Per-package stdlib flag is gone**
   - Given the public structs in `go/internal/facts`
   - When `PackageFact` is inspected and the repository compiles
   - Then it has no `IsStdlib` field and no constructor or test refers to one.

2. **Production facts always populate stdlib imports**
   - Given native and layout loads with zero or more standard-library imports
   - When `PackageFacts` is returned
   - Then `StdlibImports` is non-nil, canonical, duplicate-free, and deterministically sorted.

3. **Secondary fact producers honor the contract**
   - Given dependency-interface or other non-root fact construction
   - When `PackageFacts` values are created
   - Then their `StdlibImports` field is explicitly populated rather than relying on a nil default.

4. **Checker has no nil fallback**
   - Given `StdlibImports` is nil or empty in a hand-built checker input
   - When dependency classification runs
   - Then the checker treats the authoritative set as empty and does not call `hostpolicy.IsStdlibPath`.

5. **Rewriting-host path cannot silently skip**
   - Given a dotless or host-rewritten import absent from `StdlibImports`
   - When the checker evaluates a member's imports
   - Then the import is non-stdlib and can produce `UNDECLARED_DEPENDENCY`.

6. **Explicit stdlib facts still suppress boundary findings**
   - Given a member imports `fmt` and `StdlibImports` explicitly contains canonical `fmt`
   - When the checker runs
   - Then `fmt` is skipped as standard library and produces no undeclared dependency finding.

7. **Pure facts contract is documented**
   - Given a contributor reads `PackageFacts.StdlibImports`
   - When they inspect its comment
   - Then it states that the loader supplies the canonical authoritative set and always initializes it.

8. **Unrelated dependency classification is unchanged**
   - Given existing member, component dependency, absorbed dependency, and ordinary undeclared dependency fixtures
   - When the checker runs after the representation cleanup
   - Then their findings remain unchanged except where the removed stdlib fallback previously hid a real dependency.

9. **Step 5 demo remains reproducible**
   - Given the simulated rewriting-host fixture from Task 1
   - When the completed Step 5 pipeline runs
   - Then the real dependency remains visible through the pure facts/checker boundary and reports the expected finding.

10. **Repository checks remain green**
    - Given the simplified facts representation
    - When `just ci` runs
    - Then unit, integration, self-check, generation-cleanliness, and Bazel suites pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, facts, checker, goanalysis, standard-library, contract-cleanup
- **Required Skills**: Go, pure data-model evolution, deterministic collections, checker testing, integration migration
