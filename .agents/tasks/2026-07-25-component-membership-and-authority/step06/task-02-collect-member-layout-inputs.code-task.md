# Task: Collect member layout inputs

## Description
Add concrete `members` labels to `go_component`, pull each member into the aspect closure even when the interface does not import it, and generate layout inputs from the union of the interface and all declared members.

## Background
The current component rule starts its package closure solely from `interface`. That makes an implementation package invisible when the public interface does not import it, even if the author intends to declare the package as owned code. Explicit member labels must therefore carry `arcc_deps_aspect` themselves, and their package closures must be merged with the interface closure before layout generation, cgo validation, runfiles collection, and provider coverage are computed.

This task establishes reachability and layout completeness only. Step 7 changes classification to covered → declared member → absorbed → error, emits literal `members` in the manifest, and makes layout roots equal that emitted set. Until then, the existing classification and artifact schema remain in force so components that omit `members` behave exactly as before.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M2, M4, M9, B1; §4.8–4.9; §7.3; and §8)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 6)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/members-glob-expansion.md` (label-list constraints, frontier semantics, and symbolic-macro restrictions)
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` (F7 portability boundary)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `members` as a nonconfigurable label-list attribute to the public `go_component` symbolic macro, documented as concrete Go library labels for declared-style components.
2. Add the corresponding private-rule attribute as `attr.label_list(providers = GO_PROVIDERS, aspects = [arcc_deps_aspect])`.
3. Preserve `members` as optional for the default interface style. An omitted or empty list must retain the existing interface-rooted closure, classification, manifest, layout, provider, and check behavior.
4. Treat the interface target, when present, and every member target as independent aspect roots. Merge all resulting `ArccPackageInfo` package sets by import path before any classification or artifact generation.
5. Include each declared member's transitive Go closure in layout packages, source runfiles, cgo rejection, and all other checks currently applied to the interface-derived closure.
6. Deduplicate packages reached from multiple roots by import path using the existing deterministic merge behavior. Do not duplicate layout nodes or source runfiles.
7. Ensure a member target the interface does not import is present in the merged closure and contributes package data to the generated layout.
8. Ensure dependencies imported only by that member are also present as layout/type-checking inputs; this task must not emit an incomplete isolated member node.
9. Preserve deterministic ordering of roots, layout packages, closure provider data, and generated files regardless of label-list ordering.
10. Keep the interface package included through the interface root even when it is not repeated in `members`.
11. Do not change package classification to use the declared member set yet, do not emit the manifest's `members` field, and do not enforce `roots == members`; those are Step 7 concerns.
12. Do not add wildcard or target-pattern expansion. Reject invalid non-label forms through Bazel's typed attribute handling and direct authors who want the nearest-package frontier to bazel-skylib at BUILD top level.
13. Add an analysis/golden fixture with a member package that the interface does not import. Pin both its presence in the component closure and its package/source data in the emitted layout.
14. Keep all existing components and goldens that omit `members` unchanged.

## Dependencies
- `task-01-extend-go-adapter-seams` establishes the Step 6 host-seam contract and must be committed first.
- Steps 1–5 are complete; in particular, native manifests already understand declared members, but this task deliberately does not emit them.
- `task-03-enforce-component-shape-rules` builds on this task's public and private `members` attributes.

## Implementation Approach
1. Thread a nonconfigurable `members` attribute from the symbolic macro into the private rule with the same Go provider and aspect constraints as other Go roots.
2. Refactor `_go_component_impl` to construct a root list from the optional interface plus member targets, merge every root's aspect packages by import path, and apply existing closure-wide validation once.
3. Preserve the existing fallback classification and roots calculation for this step while broadening only the input closure.
4. Add a fixture package outside the interface's import closure, declare it in `members`, and inspect both `ArccComponentInfo.closure` and generated layout content.
5. Add a transitive dependency beneath the unimported member if needed to prove layout generation uses the complete union rather than only the member label itself.
6. Run focused analysis and golden tests, verify legacy goldens are byte-identical, then run the full repository checks.

## Acceptance Criteria

1. **Unimported member enters the closure**
   - Given a component whose `members` includes a Go library that its interface never imports
   - When the component target is analyzed
   - Then that member package is present in the aspect-derived merged closure.

2. **Member package data enters the layout**
   - Given the same unimported member has Go source files
   - When the package-layout artifact is built
   - Then the layout contains the member's import path and source data exactly once.

3. **Member dependencies remain loadable**
   - Given an unimported member imports another Go package
   - When layout inputs and runfiles are assembled
   - Then the imported package and required sources are available for arcc type checking.

4. **Multiple roots merge deterministically**
   - Given the interface and two member labels reach overlapping package closures
   - When the rule merges aspect results with different member-label orderings
   - Then each import path occurs once and generated/provider ordering is stable.

5. **Interface remains an implicit root**
   - Given a declared-style component does not repeat its interface target in `members`
   - When the component is analyzed
   - Then the interface package remains present in the merged closure and existing provider forwarding still works.

6. **Empty members preserve existing behavior**
   - Given an existing component omits `members`
   - When its manifest, layout, closure provider, and `.check` target are built
   - Then behavior and golden output remain unchanged.

7. **Step 7 semantics are not pulled forward**
   - Given a component declares a member label during Step 6
   - When its generated manifest is inspected
   - Then this task has not added literal member emission, new classification errors, or roots-equality validation ahead of Step 7.

8. **Only concrete labels are accepted**
   - Given an author attempts to place a command-line target pattern such as `//pkg/...` in a declared-style `members` list
   - When Bazel loads or analyzes the target
   - Then the typed label-list API rejects it; rules_arcc does not silently expand it.

9. **Repository checks remain green**
   - Given union-based member layout inputs
   - When `just ci` runs
   - Then unit, integration, self-check, generation-cleanliness, and Bazel suites pass, with only the intentional new fixture output.

## Metadata
- **Complexity**: High
- **Labels**: Bazel, Starlark, members, aspects, package-layout, runfiles
- **Required Skills**: Starlark, Bazel aspects and providers, depset/closure handling, deterministic artifact generation, analysis and golden testing
