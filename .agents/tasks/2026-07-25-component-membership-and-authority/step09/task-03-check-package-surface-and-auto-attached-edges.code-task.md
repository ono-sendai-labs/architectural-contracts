# Task: Check package-surface and auto-attached dependency edges

## Description
Teach the pure checker the two rules that make package-surface and injected
boundaries behave: `CALLS_UNDECLARED_INTERFACE` is skipped for a
`PACKAGE_SURFACE` dependency, and a `component_dependency` marked `auto_attached`
never produces `UNUSED_DEPENDENCY`.

## Background
These are two independent axes that the design deliberately keeps apart.
`interface_style` is a property of the *depended-on component*: with the whole
exported surface as the interface, "you called something undeclared" has no
content, so the rule is vacuous rather than merely quiet. `auto_attached` is a
property of the *edge*: an emitter injected it, so an unused edge is not the
author's mistake and warning about it would fire on every component in a
repository where a runtime is injected everywhere.

Keeping them separate is what lets the four combinations behave sensibly. In
particular an author-written `PACKAGE_SURFACE` wrapper — a logger component whose
`members` are the library it wraps — is *not* auto-attached, so declaring it and
never calling it still warns. That is the axis `auto_attached` separates from the
component kind, and it is the test that pins the distinction.

The exemption is read straight off the **depender's own manifest**. No
dep-manifest lookup: the checker is pure, the flag is on the edge it already has,
and a well-structured injected component may still declare real interface files.

FR4 placement needs no special case here. With `INIT_OUTSIDE_INTERFACE` removed
in Step 2, `METHOD_OUTSIDE_INTERFACE` is already conditional on the receiver's
type being declared in an interface file, so it is vacuous when `interface_files`
is empty — which manifest validation guarantees under `PACKAGE_SURFACE`. Adding a
style check to that rule would be dead code; do not add one.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A6/A7, §4.1 — why two fields rather than one enum, §4.3 table and the paragraph on FR4 vacuity, §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 9)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. In the FR5 call-boundary sweep, skip `CALLS_UNDECLARED_INTERFACE` entirely for
   a call whose callee package belongs to a dependency whose resolved interface
   style is `PACKAGE_SURFACE`. Decide from `facts.DependencyInterface` (Task 2),
   not by re-reading a manifest.
2. Keep the used/unused bookkeeping running for such a dependency: a call into it
   still marks the edge used, so `UNUSED_DEPENDENCY` stays meaningful for a
   wrapped library. Skipping the violation must not short-circuit the loop before
   the match is recorded.
3. Keep the existing package-initializer skip and symbol normalization for
   declared-style dependencies exactly as they are.
4. Suppress `UNUSED_DEPENDENCY` for any `component_dependency` whose manifest
   entry has `AutoAttached` set, whether or not it was matched, and whether or not
   the dependency is package-surface. Read it from `in.Manifest`; do not consult
   the dependency's manifest or its `DependencyInterface`.
5. `absorbed_dependencies` are untouched: they have no `auto_attached` concept and
   their unused warning is unchanged.
6. Do **not** add an `interface_style` condition to `METHOD_OUTSIDE_INTERFACE` or
   any other FR4 rule. Instead add a pinning test showing the rule is already
   silent for a component with empty `interface_files`, so a later reader does not
   "fix" the missing special case.
7. The checker stays pure: no new inputs beyond manifest data and facts, no I/O,
   no reflection.
8. Finding messages, ordering, and the deterministic sort are otherwise unchanged;
   this task removes findings in specific cases and adds none.

## Dependencies
- `task-02-resolve-package-surface-dependency-surface` supplies the interface
  style on `facts.DependencyInterface`.
- Step 1 supplies `auto_attached` on `ComponentDependency` and its parsing into
  `manifest.ComponentDependency.AutoAttached`.
- Step 2 removed `INIT_OUTSIDE_INTERFACE`, which is what makes the FR4 relaxation
  free rather than a special case.
- `task-06-attach-infra-components` produces the first real `auto_attached` edges;
  this task makes them behave.

## Implementation Approach
1. Extend the `depInfo` lookup the FR5 sweep already builds with the dependency's
   interface style, so the callee-package lookup answers both questions at once.
2. Add the style guard at the point the undeclared-callee violation is appended,
   after the match is recorded, and assert the ordering with a test that a
   package-surface dependency called into is not reported unused.
3. Add the `auto_attached` guard in the unused-dependency loop.
4. Write table-driven checker tests covering all four combinations of
   {package-surface, declared-style} × {auto-attached, author-written}, each
   asserting both rules.
5. Add the FR4 pinning test for empty `interface_files`.
6. Run the focused `checker` and `report` suites, then `just ci`.

## Acceptance Criteria

1. **Calls into a package-surface dependency are never undeclared**
   - Given call edges into a `PACKAGE_SURFACE` dependency, including to a symbol absent from its resolved surface
   - When the checker runs
   - Then no `CALLS_UNDECLARED_INTERFACE` violation is produced for that dependency.

2. **A declared-style dependency is unaffected**
   - Given a call to an undeclared symbol of a declared-style dependency
   - When the checker runs
   - Then `CALLS_UNDECLARED_INTERFACE` is reported exactly as before, with the same message.

3. **A used package-surface wrapper does not warn**
   - Given an author-written `PACKAGE_SURFACE` wrapper that a member calls into
   - When the checker runs
   - Then no `UNUSED_DEPENDENCY` warning is produced, because the call still marks the edge used.

4. **An unused package-surface wrapper does warn**
   - Given the same wrapper declared but never called
   - When the checker runs
   - Then `UNUSED_DEPENDENCY` is reported — the axis `auto_attached` separates from the component kind.

5. **An auto-attached edge never warns**
   - Given a `component_dependency` marked `auto_attached`, both used and unused, and both package-surface and declared-style
   - When the checker runs
   - Then no `UNUSED_DEPENDENCY` is produced in any of those cases.

6. **The exemption reads only the depender's manifest**
   - Given an auto-attached edge whose dependency manifest is not otherwise inspectable
   - When the checker runs
   - Then the exemption applies with no dependency-manifest access, and the checker remains pure.

7. **Absorbed dependencies are untouched**
   - Given an unused absorbed dependency in a component that also has auto-attached edges
   - When the checker runs
   - Then its `UNUSED_DEPENDENCY` warning is still produced.

8. **FR4 is already vacuous without a special case**
   - Given a component with empty `interface_files` and exported methods in member packages
   - When the checker runs
   - Then no `METHOD_OUTSIDE_INTERFACE` violation is produced, and the code contains no interface-style condition in that rule.

9. **Repository checks remain green**
   - Given the two new suppressions
   - When `just ci` runs
   - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass, with no change to any existing component's findings.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, checker, pure-core, package-surface, auto-attached, FR4, FR5
- **Required Skills**: Go, functional-core reasoning, table-driven testing
