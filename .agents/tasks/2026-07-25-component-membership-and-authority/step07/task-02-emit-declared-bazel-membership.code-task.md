# Task: Emit declared Bazel membership

## Description
Change `go_component` classification to covered → declared member → absorbed → undeclared, emit the literal member set into the manifest and layout roots, and record the target build platform in the generated layout.

## Background
Step 6 made `interface` and concrete `members` labels independent aspect roots, but the rule still classifies every package left after coverage and absorption as a member. That fallback recreates the old implicit-ownership model: transitive utilities are silently swept into the component. Step 7 must classify membership from the labels the author actually supplied, with the interface package implicitly included, and leave unclassified packages available for the existing checker to report as `UNDECLARED_DEPENDENCY`.

The two generated artifacts are a single contract. Their literal member/root sets must be identical so Task 1's load-time equality check passes. The layout must also carry the target settings already exposed by `go_build_platform`, allowing the platform-aware loader to analyze the same source selection Bazel built.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M4/M6/M7 and T2, §3.1, §4.7–4.8, §5.1, §6, §7.1 fixture 3, and §8)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 7)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/build-platform-and-tags.md` (rules_go target-mode fields and cgo approximation)
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` (F1–F2 emitter/platform portability constraints)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Import and call `go_build_platform` only through `go_adapter.bzl`; do not read rules_go providers directly from `component.bzl`.
2. Derive the declared member import paths from the concrete `members` targets themselves, plus the interface package as the implicit declared-style member. Do not treat each member target's transitive closure as declared membership.
3. Validate that every member label has a non-empty import path and fail analysis with the component name and offending label when it does not.
4. Classify every package in the merged interface/member closure in this order:
   a. covered by a declared `component_dep`;
   b. an exact declared member;
   c. inside an explicitly absorbed label's transitive closure;
   d. otherwise unclassified.
5. Keep all closure packages in the layout for type checking even when unclassified. Do not add unclassified packages to the component provider's owned/absorbed closure and do not generate blanket absorbed declarations for them.
6. Let the existing Go checker surface an import from a member to an unclassified non-stdlib package as `UNDECLARED_DEPENDENCY`; do not introduce a new Starlark finding mechanism or analysis-time failure for this remainder.
7. Fail analysis when an authored member label is also directly listed in `absorbed_deps`, or its import path is covered by a `component_dep`. Messages must name the component and conflicting label/dependency.
8. Preserve coverage precedence for packages reached transitively through an absorbed closure. Only contradictions involving the authored member or absorbed labels themselves are analysis-time failures; checker-time `MEMBER_OVERLAP` remains the defense for resolved/pattern cases.
9. Emit one deterministic `members:` field per expanded literal member import path in the generated manifest. Include the implicit interface package so the generated manifest and layout express the same effective ownership set without requiring the author to repeat the interface label.
10. Emit exactly that sorted member set as the layout's `roots`. No absorbed, covered, or unclassified package may appear as a root.
11. Emit `interface_style` consistently with the Bazel authoring value and existing protobuf spelling. Preserve the default declared-style representation expected by manifest parsing; package-surface execution remains Step 9 unless already required for artifact validity.
12. Emit a `platform` object containing `goos`, `goarch`, `build_tags`, and `cgo_enabled` from `go_build_platform` using the JSON field names accepted by `packagelayout`.
13. Preserve the layout's conforming platform shape: rules_go's source set may be unfiltered, so omit package imports and allow the loader to recover imports after platform filtering rather than emitting an unfiltered union.
14. Keep deterministic output across label ordering by sorting member paths, roots, platform tags, interface files, absorbed paths, and packages where applicable.
15. Extend analysis fixtures to pin each classification bucket and both authored conflicts. Add the design §7.1 fixture 3: a member importing a package that is neither member, covered, absorbed, nor stdlib must reach an end-to-end `.check` and report `UNDECLARED_DEPENDENCY`.
16. Update manifest and package-layout goldens for literal members, equal roots, and platform emission. Extend golden normalization only for genuinely machine-specific SDK/runfiles values; do not normalize away membership or platform semantics.
17. Preserve components that omit explicit `members`: their interface package remains an effective emitted member/root, and transitive non-members now deliberately surface instead of being silently owned.

## Dependencies
- `task-01-enforce-member-layout-consistency` must be committed first so generated roots and members are checked by the consumer.
- Step 6 supplies the member attributes, merged interface/member closures, optional interface shape, and `go_build_platform` adapter seam.
- Task 3 migrates real csvtool declarations after this task establishes the final artifact and classification semantics.

## Implementation Approach
1. Split `_classify` into explicit covered, declared-member, absorbed, and remainder sets keyed by import path, retaining the merged closure separately for layout generation.
2. Validate authored member conflicts before transitive classification and construct one canonical effective-member list containing the interface package plus declared labels.
3. Extend manifest and layout serializers to accept the effective members, interface style, and adapter-derived platform.
4. Keep unclassified nodes in layout packages/runfiles but exclude them from emitted ownership and provider closure, allowing Go-side FR2 logic to diagnose imports.
5. Expand analysis fixtures and add an executable check fixture for the undeclared transitive utility case.
6. Regenerate intentional goldens, verify deterministic output under reversed member order, then run focused Bazel and Go integration suites followed by `just ci`.

## Acceptance Criteria

1. **Declared labels define membership**
   - Given a component with an interface, one explicit member, and transitive utility packages
   - When the rule classifies its merged closure
   - Then only the interface and explicit member are members; utilities are neither silently members nor automatically absorbed.

2. **Classification order is deterministic**
   - Given closure packages spanning covered, declared-member, absorbed, and unclassified cases
   - When the component is analyzed with member-label order varied
   - Then each package lands in the same first matching bucket and emitted artifacts are byte-identical.

3. **Unclassified imports fail through FR2**
   - Given a member imports a non-stdlib package that is not a member, covered, or absorbed
   - When the generated `.check` runs
   - Then it reports `UNDECLARED_DEPENDENCY` naming the member and imported package.

4. **Authored conflicts fail during analysis**
   - Given a member label that is also an absorbed label or whose import path is covered by a component dependency
   - When Bazel analyzes the component
   - Then analysis fails with the component name, offending label, and conflicting declaration.

5. **Manifest and roots are identical**
   - Given a valid declared-style component
   - When its manifest and layout are generated
   - Then the sorted manifest `members` values equal the sorted layout `roots`, including the implicit interface package exactly once.

6. **Layout retains type-checking closure**
   - Given an unclassified transitive package
   - When the layout is generated
   - Then its package/source data remains present for type checking even though it is absent from members, roots, absorbed declarations, and the component's responsibility provider.

7. **Target platform is emitted**
   - Given a component configured for a non-host target mode with tags and pure/cgo state
   - When its layout is generated
   - Then `platform.goos`, `platform.goarch`, `platform.build_tags`, and `platform.cgo_enabled` match `go_build_platform` and round-trip through the Go layout parser.

8. **Unfiltered source shape remains valid**
   - Given rules_go exposes the declared source set across platforms
   - When the layout is loaded for its emitted target platform
   - Then imports are recovered from surviving sources and T8 validation does not reject an unfiltered import union.

9. **Goldens pin the new contract**
   - Given the Bazel golden fixtures
   - When the golden test runs
   - Then it compares literal members, equal roots, interface style where relevant, and platform fields rather than normalizing them away.

10. **Repository checks remain green**
   - Given final Bazel membership classification and emission
   - When `just ci` runs
   - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass with only intentional contract changes.

## Metadata
- **Complexity**: High
- **Labels**: Bazel, Starlark, membership, classification, manifest, package-layout, platform
- **Required Skills**: Starlark, Bazel aspects/providers, deterministic serialization, Go package layouts, end-to-end rule testing
