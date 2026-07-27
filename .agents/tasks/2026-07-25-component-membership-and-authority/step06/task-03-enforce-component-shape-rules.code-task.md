# Task: Enforce component shape rules

## Description
Add the `interface_style` authoring surface, enforce the declared-style and `PACKAGE_SURFACE` attribute shapes in the symbolic macro, make the private interface attribute optional, and remove the obsolete directory-root nesting restriction.

## Background
Declared-style components continue to expose one Go interface library and may optionally list explicit members. A `PACKAGE_SURFACE` component has no distinguished interface target: its complete exported surface will come from mandatory members. The symbolic macro must reject invalid combinations early with messages naming the component, while the private rule must tolerate the absent interface needed by the valid package-surface shape.

The existing `_check_component_roots` restriction predates declared membership. It rejects a component dependency rooted inside its consumer's directory, but directory nesting no longer describes ownership when members may live anywhere. It is also inputless for a package-surface component, which has no interface-derived logical root. Local member overlap replaces this constraint in Step 7, so the root check and its failure fixture must be removed now.

This task adds and validates the authoring shape but does not yet emit `interface_style` or declared members into the manifest. Step 7 completes artifact emission and end-to-end checking.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M2, A6, B1; §4.8–4.9; §6; §7.3; and §8)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 6)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/members-glob-expansion.md` (why members remain concrete labels at this stage)
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` (F4 and F7 constraints on future pattern membership)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a nonconfigurable `interface_style` string attribute to the public `go_component` macro and the corresponding private rule.
2. Expose or document a stable `PACKAGE_SURFACE` authoring value consistent with the generated protobuf enum spelling `INTERFACE_STYLE_PACKAGE_SURFACE`; avoid an ambiguous free-form convention that Step 7 would have to replace.
3. Treat an unset/default `interface_style` as declared style. Require `interface` and allow `members` to be empty or non-empty.
4. For `PACKAGE_SURFACE`, require a non-empty `members` list and reject any supplied `interface`.
5. Reject unknown `interface_style` values with a `fail()` that names the component and the accepted values.
6. Enforce all shape rules in the symbolic macro implementation before invoking `go_component_rule`, so users get direct authoring errors rather than failures deep in artifact generation.
7. Make the private rule's `interface` label `mandatory = False` while retaining `GO_PROVIDERS` and `arcc_deps_aspect` validation whenever it is supplied.
8. Refactor implementation paths that assume `ctx.attr.interface` exists. For a valid package-surface target, derive the merged package set from members alone, emit no interface files, and do not forward interface Go providers.
9. Preserve interface-provider forwarding unchanged for declared-style components.
10. Delete `_check_component_roots` and its call from `component.bzl`.
11. Delete or rewrite the analysis fixture that expects nested component roots to fail. Add a positive analysis test proving a component can depend on a component rooted inside the same package subtree.
12. Add negative analysis/load tests for all three required shape failures:
    a. `PACKAGE_SURFACE` with `interface` set;
    b. `PACKAGE_SURFACE` with empty or omitted `members`;
    c. default/declared style with no `interface`.
13. Ensure every shape failure names the component and states the conflicting or missing attributes.
14. Add a positive analysis fixture for each valid shape: declared style with interface and optional members, and package-surface style with members and no interface.
15. Do not emit `interface_style` or `members` in the manifest, change classification order, or run a package-surface `.check` end to end in this task. Step 7 owns emission and checker integration; Step 6 only establishes analyzable rule targets and validation.
16. Do not add pattern membership to the label-list attribute in this task. Step 9 handles unexpanded import-path patterns for package-surface components.

## Dependencies
- `task-01-extend-go-adapter-seams` is complete and keeps host-specific platform/infra decisions out of the component rule.
- `task-02-collect-member-layout-inputs` is complete and provides the member attributes and interface-plus-members root merge that package-surface analysis reuses.
- Steps 1–5 are complete; the protobuf/manifest model already defines `PACKAGE_SURFACE`, though Bazel emission is deferred to Step 7.

## Implementation Approach
1. Define the public style value and macro attribute, then validate style and attribute presence in `_go_component_impl` before filtering unset kwargs.
2. Relax the private interface attribute and restructure `_go_component_impl` around the root union introduced by Task 2, using an empty interface-file set and provider list when no interface exists.
3. Remove the directory-root validator and convert the old nested-root negative fixture into a positive co-location/nesting case.
4. Add three focused failure fixtures whose expected messages include the component name and precise shape rule.
5. Add a valid package-surface analysis fixture that builds the component action graph without executing its not-yet-complete Step 7 `.check`.
6. Verify declared-style forwarding and existing goldens remain unchanged, then run the full repository checks.

## Acceptance Criteria

1. **Declared style requires an interface**
   - Given a component with default or declared `interface_style` and no `interface`
   - When its macro is expanded
   - Then analysis fails with a message naming the component and requiring `interface`.

2. **Declared style permits explicit members**
   - Given a component with an interface and zero or more concrete member labels
   - When it is analyzed
   - Then the shape is accepted and the interface continues to provide files and forwarded Go providers.

3. **Package surface rejects an interface**
   - Given a component with `interface_style = PACKAGE_SURFACE`, non-empty members, and `interface` set
   - When its macro is expanded
   - Then analysis fails with a message naming the component and stating that `interface` is not allowed.

4. **Package surface requires members**
   - Given a component with `interface_style = PACKAGE_SURFACE` and empty or omitted `members`
   - When its macro is expanded
   - Then analysis fails with a message naming the component and requiring non-empty `members`.

5. **Valid package surface has no interface assumptions**
   - Given a component with `interface_style = PACKAGE_SURFACE`, concrete members, and no interface
   - When its private rule is analyzed
   - Then its closure and layout actions are derived from members, its interface-file set is empty, and no absent-interface provider access occurs.

6. **Unknown styles fail clearly**
   - Given an unsupported `interface_style` string
   - When the macro is expanded
   - Then it fails before rule invocation and lists the supported style values.

7. **Nested component roots are allowed**
   - Given a component dependency whose target package is rooted at or below the consumer's Bazel package subtree
   - When the consumer component is analyzed
   - Then no directory-root nesting check rejects it.

8. **Declared-style provider forwarding is unchanged**
   - Given an existing Go target depends on a declared-style component target
   - When Bazel analyzes and builds it
   - Then it receives the same forwarded Go providers as before.

9. **Step 7 emission remains deferred**
   - Given the valid package-surface analysis fixture
   - When Step 6 tests run
   - Then they validate rule analysis only; this task does not claim an end-to-end generated manifest check before style and member emission land.

10. **Step 6 demo shapes are reproducible**
    - Given one valid member-declaring component and the three invalid shape fixtures
    - When their targets are analyzed
    - Then the valid target exposes the member-only closure and each invalid target fails for its intended shape rule with no unrelated error.

11. **Repository checks remain green**
    - Given the new component shapes and removed nesting restriction
    - When `just ci` runs
    - Then unit, integration, self-check, generation-cleanliness, and Bazel suites pass, with existing declared-style artifacts unchanged.

## Metadata
- **Complexity**: High
- **Labels**: Bazel, Starlark, symbolic-macro, interface-style, package-surface, validation
- **Required Skills**: Starlark symbolic macros, Bazel rule attributes, provider forwarding, analysis-failure testing, API evolution
