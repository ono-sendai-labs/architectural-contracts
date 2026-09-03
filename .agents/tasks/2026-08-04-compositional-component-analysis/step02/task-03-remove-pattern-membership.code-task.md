# Task: Remove Pattern Membership

## Description
Remove wildcard/pattern component membership from manifest validation, package loading, dependency-interface resolution, checker membership, facts helpers, and Bazel macros. Every `members` entry becomes a literal package identity, and glob metacharacters are rejected while the manifest is parsed.

## Background
Pattern membership is a dependency-side exception today: directly checking a pattern-member component fails in `goanalysis`, while resolving it as a dependency expands patterns against the depender's loaded closure. The design rejects this asymmetry because wildcard membership hides newly introduced transitive packages. Explicit membership preserves the upgrade-review signal and is required before compositional surfaces are introduced.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Design review response DR-16 and verified pattern-membership inventory: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Decision record Q7: `.agents/planning/2026-08-04-compositional-component-analysis/idea-honing.md`
- Plan Step 2: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Make `manifest.Parse` reject any `members` entry containing glob metacharacters supported by the old matcher, for every interface style. Return an actionable `InvalidMemberError` explaining that members must be literal import paths; malformed and well-formed glob spellings must both fail at this boundary.
2. Remove the direct-load pattern rejection from `goanalysis.LoadPackageFacts`, because invalid membership can no longer pass parsing into the loader.
3. Delete `isPatternMembership`, `isAnyPatternMember`, `resolvePatternMembershipDependencyInterface`, and their tests/fixtures. `ResolveDependencyInterface` must follow only the literal-member path.
4. Remove `facts.MatchesMember` and its dedicated tests. Replace remaining legitimate membership checks in `goanalysis` and `checker` with exact comparisons or canonical literal-member sets; do not retain `path.Match` as an implicit membership mechanism.
5. Remove Bazel's private `member_patterns` attribute and all production/test macro logic that splits string patterns from label members. Public package-surface component APIs must accept explicit target labels only and must not serialize unexpanded patterns into manifests.
6. Delete existing pattern-membership Bazel components, consumers, analysis tests, check targets, tags, and fixtures rather than weakening them into literal-membership tests. Preserve independent coverage for normal explicit `PACKAGE_SURFACE` components.
7. Verify before deletion that no non-test example or self-component manifest relies on pattern members, and record the result in the implementation change description as required by the plan.
8. Update schema comments, provider/API documentation, and source comments to describe literal members only. Remove all non-historical pattern-membership wording and obsolete imports used solely for glob matching.
9. Keep stale-manifest rejection, explicit membership, dependency interface resolution, self-check, `manifestparity`, and Bazel validation green under `just ci`.

## Dependencies
- Task 2 must be complete so `facts.MatchesMember` no longer has absorbed-dependency callers and can be deleted cleanly.
- This is the final task in Step 2.

## Implementation Approach
1. Audit all example, self-component, and test manifests for wildcard members and record which are production inputs versus pattern-only fixtures.
2. Tighten manifest validation first and add table-driven cases for `*`, `?`, bracket expressions, escapes, and ordinary literal import paths.
3. Simplify Go membership construction and dependency resolution to canonical exact-set membership; remove pattern helpers and pattern-only fixtures/tests.
4. Contract the Starlark public/private APIs and test wrappers to label-only members, deleting pattern-only analysis and check fixtures.
5. Update comments and schema docs, search the non-planning tree for stale feature names and matcher calls, then run focused tests and `just ci`.
6. Include the example/self-component audit result in the final `jj` change description.

## Acceptance Criteria

1. **Glob members fail during parsing**
   - Given a declared-interface or `PACKAGE_SURFACE` textproto whose `members` contains `*`, `?`, `[ ... ]`, or an escape used by the former glob grammar
   - When `manifest.Parse` reads it
   - Then it returns an actionable invalid-member error stating that members must be literal import paths.

2. **Literal members still work**
   - Given a manifest with canonical literal member import paths
   - When it is parsed, loaded, checked directly, or resolved as a component dependency
   - Then membership uses exact canonical identity and existing explicit-member behavior remains green.

3. **Runtime pattern machinery is absent**
   - Given the facts, checker, and `goanalysis` packages
   - When their APIs and implementations are searched
   - Then `MatchesMember`, pattern-detection helpers, pattern dependency resolution, direct-load pattern rejection, and membership uses of `path.Match` are gone.

4. **Bazel accepts only explicit members**
   - Given the public component macros and private component rule
   - When a package-surface component is declared
   - Then members flow as typed target labels, there is no `member_patterns` attribute or pattern-splitting test shim, and no unexpanded import-path pattern is emitted.

5. **Pattern fixtures are deleted, not weakened**
   - Given the existing pattern-membership producer/consumer components and their analysis/check tests
   - When the task is complete
   - Then those fixtures and tests are removed while unrelated explicit package-surface coverage remains.

6. **Audit is recorded**
   - Given the repository's non-test examples and self-components
   - When wildcard membership is audited before feature deletion
   - Then the result is stated in the implementation change description and every real user is either confirmed absent or migrated explicitly.

7. **Integration remains green**
   - Given only literal membership remains
   - When `just ci` runs
   - Then Go tests, self-check, manifest parity, and Bazel tests all pass.

## Metadata
- **Complexity**: High
- **Labels**: go, manifest, bazel, membership, validation, deletion
- **Required Skills**: Go, Starlark, Bazel analysis tests, table-driven validation testing, Jujutsu
