# Task: Propagate Empty Provider for Non-Go Targets

## Description
Change the Go dependency aspect so every target it visits supplies `ArccPackageInfo`, including non-Go targets, with non-Go targets contributing an empty package depset. Add analysis coverage that proves consumers can rely on the declared provider contract across mixed-language dependency graphs.

## Background
`arcc_deps_aspect` declares `ArccPackageInfo` in `provides`, but its implementation currently returns no provider for a filegroup, proto target, or other non-Go dependency. That violates the aspect's provider contract and forces downstream traversal to reason about an avoidable absence. An empty provider preserves the fact that the target contributes no Go packages while keeping aspect composition total.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Host import friction report §1: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md`
- Plan Step 1: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. In `bazel_rules/go/private/aspect.bzl`, return `[ArccPackageInfo(packages = depset())]` when `is_go_target(target)` is false.
2. Preserve existing Go-target traversal over `deps` and `embed`, deterministic merging, and all existing provider contents.
3. Add a rules_testing analysis fixture with a non-Go target reached by the aspect and assert that `ArccPackageInfo` is present and its `packages` depset is empty.
4. Keep public API and provider schema unchanged; this is a behavior correction, not a new provider type.

## Dependencies
- No dependency on task 1; the change is independently implementable and testable.
- Uses the existing `ArccPackageInfo`, aspect probe helpers, and rules_testing suite.

## Implementation Approach
1. Add or reuse a minimal non-Go target in the Bazel testdata graph and expose its aspect-applied result to an analysis test.
2. Write the failing analysis assertion for provider presence and an empty collection.
3. Replace the non-Go early return in `_arcc_deps_impl` with an empty `ArccPackageInfo`.
4. Run the focused Bazel aspect suite, then the repository CI gate.

## Acceptance Criteria

1. **Provider exists on non-Go targets**
   - Given a filegroup, proto target, or equivalent non-Go target visited by `arcc_deps_aspect`
   - When an analysis consumer inspects that configured target
   - Then `ArccPackageInfo` is present.

2. **Non-Go closure is empty**
   - Given the provider emitted for a non-Go target
   - When its `packages` depset is converted to a list
   - Then the list is empty and contains no synthetic package node.

3. **Go traversal is unchanged**
   - Given the existing Go, embed, diamond, platform, and infra aspect fixtures
   - When the aspect test suite runs
   - Then their package closure, direct import edges, source merging, and platform assertions still pass unchanged.

4. **Integration remains green**
   - Given the corrected aspect implementation and new analysis fixture
   - When `just ci` runs
   - Then all Go, self-check, and Bazel checks pass.

## Metadata
- **Complexity**: Low
- **Labels**: bazel, starlark, aspect, regression
- **Required Skills**: Starlark, Bazel aspects, rules_testing analysis tests
