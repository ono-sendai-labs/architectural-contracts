# Task: Add Canonical Namespace Policy

## Description
Extend `hostpolicy` with a stable namespace identifier and an override-once canonical-path predicate. Define and test the contract tying `NamespaceID`, `CanonicalizePath`, and standard-library identity together so later surface emitters and consumers can reject cross-namespace artifacts rather than silently comparing incompatible symbol paths.

## Background
A host may rewrite import prefixes, while surface symbols persist package paths. Canonicalization alone cannot reveal which rewrite policy produced an artifact. The design therefore records a host-selected namespace ID on surfaces and requires canonicalization to be idempotent. `IsCanonicalPath` makes that invariant testable at the shell boundary; the standard-library map remains namespace-free, so hosts must never rewrite its package paths.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Host import friction report §7: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md`
- Plan Step 3: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-08: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add an override-once `hostpolicy.NamespaceID` seam with the upstream default exactly `"upstream"`. Document that it is stable process configuration, is recorded by future surface emitters, and is compared exactly by future consumers.
2. Add an override-once `hostpolicy.IsCanonicalPath(string) bool` predicate. The upstream default must agree with identity `CanonicalizePath`; host overrides must identify precisely the fixed points of their canonicalizer.
3. Document and test the contract `IsCanonicalPath(p) == (CanonicalizePath(p) == p)` for accepted inputs and `CanonicalizePath(CanonicalizePath(p)) == CanonicalizePath(p)`. Do not silently repair a host override whose hooks disagree.
4. Add a validation helper suitable for shell adapters to check a candidate path and return a contextual error when loader/build-graph output is not canonical. Keep the pure checker free of host-policy imports.
5. Define and test the namespace-free standard-library invariant: for every package path supplied by a caller as belonging to a stdlib map, canonicalization must be identity and `IsCanonicalPath` must be true. The helper may validate a supplied list; it must not discover or classify stdlib packages itself.
6. Keep current `IsStdlibPath` behavior and all existing check-path classification in place until the authority-map cutover. Do not use `NamespaceID` to accept/rewrite mismatched artifacts or add surface consumption from Step 7.
7. Add isolated tests for defaults and host overrides. Restore all package-global seams with `t.Cleanup`, avoid parallel tests that mutate them, and cover rewritten, already-canonical, invalid, and stdlib paths.

## Dependencies
- No functional dependency on Tasks 1-5; it is sequenced last so all Step 3 data contracts are available for its documentation and future use.
- Future surface emission and consumption depend on this task, but this task must not wire those paths yet.

## Implementation Approach
1. Add the two small seams and contract comments alongside `CanonicalizePath`, retaining upstream identity defaults.
2. Implement narrow validation helpers for canonical fixed points and caller-supplied stdlib package lists.
3. Add non-parallel table-driven tests with cleanup-backed host overrides that prove idempotence, predicate agreement, namespace defaulting, and stdlib identity.
4. Run focused host-policy tests, self-check to confirm dependency boundaries, and the full CI gate.

## Acceptance Criteria

1. **Upstream defaults are stable**
   - Given an unmodified host policy
   - When namespace and canonical-path hooks are queried
   - Then `NamespaceID` is `"upstream"`, canonicalization is identity, and every identity-fixed path is reported canonical.

2. **Host rewrites are expressibly idempotent**
   - Given a test host override that maps external spellings into one canonical prefix
   - When raw and canonical spellings are evaluated
   - Then raw paths are not canonical, canonical paths are fixed points, and applying the canonicalizer twice yields the same value as once.

3. **Hook disagreement fails explicitly**
   - Given a host predicate and canonicalizer that disagree for a candidate loader path
   - When the validation helper checks it
   - Then it returns a contextual error rather than accepting or rewriting the path silently.

4. **Standard-library paths remain namespace-free**
   - Given caller-supplied stdlib map package paths
   - When the invariant helper validates them under identity and rewriting host policies
   - Then unchanged canonical paths pass and any path the host would rewrite fails clearly; the helper performs no heuristic stdlib discovery.

5. **Mutable seams are test-isolated**
   - Given tests override `NamespaceID`, `CanonicalizePath`, or `IsCanonicalPath`
   - When each test completes
   - Then `t.Cleanup` restores the prior values and subsequent tests observe upstream defaults.

6. **Current check path is unchanged**
   - Given existing standard-library and host-canonicalization fixtures
   - When `just ci` runs
   - Then they retain current behavior, `IsStdlibPath` still exists, and no surface namespace comparison has been wired prematurely.

## Metadata
- **Complexity**: Low
- **Labels**: go, host-policy, namespace, canonicalization, compatibility
- **Required Skills**: Go, host adapter design, global test-seam isolation, invariant testing
