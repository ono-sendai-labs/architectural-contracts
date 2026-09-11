# Task: Use Label Identity for Infra Self-Exemption

## Description
Make infrastructure-component self-exemption a structural Bazel-label decision. The component that provides an infra dependency must not auto-attach itself, while an unrelated component that merely has the same short component name must remain eligible for attachment.

## Background
The existing infra registry and `go_attached_infra` seam auto-attach component providers before the generated manifest and dependency-artifact bindings are written. Its self-exemption currently compares `ArccComponentInfo.component_name` with `ctx.label.name`. That short-name comparison is not a stable identity: two targets in different packages can share a name, and a host currently has to preserve or patch name-based assumptions when importing the rules.

Step 12 removes the host's remaining production patches. The component's own canonical target label is already available from `ctx.label`, and every `infra_deps` entry is a target with its own label. Comparing those labels makes the exemption a general property of the infra mechanism, independent of repository package names or component naming conventions. This task changes only self-exemption; registry matching, attachment predicates, package-surface ownership, and checked/asserted provenance remain unchanged.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R13, §Host adapter contract, adherence requirement for host-dependent ports)

**Additional References:**
- Plan Step 12: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Host import friction: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md` (§5 infra-registry self-exemption)
- Current adapter seam and test harness: `bazel_rules/go/private/go_adapter.bzl`, `bazel_rules/go/tests/testing.bzl`, and `bazel_rules/go/tests/component_tests.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Replace the component-name-based self-exemption in the infra attachment path with an exact comparison between the consuming component's canonical `ctx.label` and the candidate infra component target's label.
2. Keep label normalization and comparison inside the Bazel adapter/attachment seam. Do not introduce a repository package-name literal, component-name allowlist, substring/pattern match, or host-specific target label into the generic component rule.
3. Exempt only the exact component target. A distinct target in another Bazel package with the same `component_name` or target basename must not be skipped solely because the short names collide.
4. Preserve the existing registry lookup and fail-closed direct-dependency validation. Label-based self-exemption must happen without weakening duplicate dependency-name, shared-surface, provenance, or overlap checks for dependencies that remain attached.
5. Preserve testability through the existing injected infra-registry/attachment harness; do not add production-only test attributes to `go_component`.
6. Add focused Starlark analysis tests for exact-self exclusion and same-short-name/non-self eligibility, including a regression fixture whose package path distinguishes two identically named targets.
7. Update adapter documentation and stale comments so they describe structural target identity rather than a package or component-name convention.

## Dependencies
- Step 7 supplies the auto-attached infra dependency records, surface/report bindings, and overlap checks whose behavior must remain intact.
- Step 11 supplies explicit `authority = UNKNOWN` for unowned infra wrappers; the self-exemption must be independent of checked/asserted authority.
- No Step 12 task dependency. This identity correction lands first so the runtime-injection hook in Task 2 can safely expose an infra component's injected packages without making the component attach itself.

## Implementation Approach
1. Isolate the self-identity test in `go_attached_infra` (or its smallest adapter-local helper) and compare canonical Bazel labels rather than provider display names.
2. Extend the analysis-test harness with two label-distinct candidates that share a component/target name, then assert that only the exact consumer target is exempted.
3. Re-run the existing infra attachment, malformed-provider, and dependency-binding suites to ensure the identity change does not bypass other validation.

## Acceptance Criteria

1. **The exact infra component does not attach itself**
   - Given a component whose own target also appears in the host infra registry/dependency set
   - When its infra attachments are evaluated
   - Then the exact same Bazel label is excluded and no self dependency, self surface binding, or attachment cycle is produced.

2. **Short-name collisions do not create false exemptions**
   - Given a consumer and an infra component in different Bazel packages whose target basenames or published component names are equal
   - When the infra predicate selects the candidate
   - Then the candidate remains attached because its canonical label differs from the consumer's label.

3. **No host-specific identity literal remains**
   - Given the production adapter and component rule
   - When their self-exemption logic and documentation are inspected
   - Then identity derives only from the current component label and candidate target label, with no package-name string match or repository-specific allowlist.

4. **Dependency validation remains fail closed**
   - Given attached dependencies with duplicate names, conflicting authored identity, shared surface paths, or invalid provenance
   - When the component is analyzed
   - Then the existing actionable analysis failures still occur for every non-self candidate.

5. **Existing behavior remains green**
   - Given the upstream empty infra registry and the repository's current infra fixtures
   - When `just ci` runs
   - Then component manifests, layouts, reports, surfaces, and check verdicts remain unchanged except for tests that deliberately exercise label identity.

## Metadata
- **Complexity**: Low
- **Labels**: bazel, starlark, adapter, infrastructure, identity
- **Required Skills**: Bazel label semantics, Starlark rule implementation, rules_testing analysis tests
