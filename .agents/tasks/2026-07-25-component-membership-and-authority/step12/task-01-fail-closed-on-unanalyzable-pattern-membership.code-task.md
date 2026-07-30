# Task: Fail closed when a component's membership cannot be analyzed

## Description
`arcc check` on a component whose `members` are all import-path patterns
currently prints `Component "x" conforms; does not exceed declared authority`
and exits 0 without loading a single package. Make that outcome impossible: the
run must either fail with a message naming what could not be analyzed, or
report an `ANALYSIS_LIMITATION` so the report cannot read clean. Cover the
mixed literal/pattern case at the same time.

## Background
Addresses finding **F1** from the implementation review.

`LoadPackageFacts` short-circuits at `go/internal/goanalysis/goanalysis.go:60`:

```go
if len(req.Members) > 0 && isAllPatternMembers(req.Members) {
    return facts.PackageFacts{}, nil
}
```

Nothing downstream treats that empty result as unusual. `app.go:237` guards the
capability analysis with `if len(pkgs) > 0`, so no analyzer runs at all;
`checker.Check` over empty facts produces no violations and no warnings; and
`report.RenderText` takes its clean branch. Reproduced against the shipped
binary with a four-line manifest:

```
name: "pattern_surface_comp"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "example.com/runtime/*"
declared_authority: "FILES"

$ ./bin/arcc check component.textproto
Component "pattern_surface_comp" conforms; does not exceed declared authority
$ echo $?
0
```

The short-circuit is not itself a mistake. Design Appendix C.6 says a
pattern-membership component "has no check of its own", because its packages
cannot be named as targets, and §4.5 explains that such a component's surface is
resolved from the *depender's* layout — the dependency direction, which is
implemented and correct. What is missing is that nothing enforces the
assumption, and the cost of violating it is the exact shape this batch exists to
remove. A9 spends a whole report producer on making one bodiless absorbed
package visible; a whole unanalyzed component should not be quieter than that.

There is a second, narrower hole in the same place. `isAllPatternMembers`
requires *every* entry to contain a metacharacter, so a `PACKAGE_SURFACE`
component with one literal and one pattern member skips the short-circuit and
hands the raw pattern to `packages.Load`, which does not understand `path.Match`
syntax. Whatever shape the fix takes, that case must be decided rather than left
to fall out.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§4.5 pattern-membership resolution; §6 "Load-time (arcc, fail closed)"; §5.3 on A9's reuse of `ANALYSIS_LIMITATION`; Appendix C.6)
- Implementation Review: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/review.yaml` (finding F1)

**Additional References (if relevant to this task):**
- `go/internal/goanalysis/goanalysis.go:1727-1753` — `isPatternMembership`, `isAllPatternMembers`, `isAnyPatternMember`.
- `go/internal/checker/checker.go:240-245` — how A9's bodiless-package observation is turned into a warning; the same loader-collects/checker-reports split applies here.
- `README.md` limitation 14 — the user-facing statement that an asserted boundary is not a verified one.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Decide between the two fail-closed shapes and record the reasoning in the
   commit message: (a) a load error naming the manifest and the unanalyzable
   members, or (b) a report-level `ANALYSIS_LIMITATION` naming them. Prefer (a)
   if a pattern-membership component is never legitimately the *subject* of a
   check, and (b) if there is a case where it is. Do not implement both.
2. Whichever shape is chosen, `arcc check` on the reproduction manifest above
   MUST NOT print the conformance line with exit 0.
3. Handle the mixed literal/pattern membership explicitly. A `PACKAGE_SURFACE`
   component with both forms must not reach `packages.Load` with a `path.Match`
   pattern as a load pattern; it must take the same decided path as the
   all-pattern case, or load its literal members and account for the patterns.
4. Do not change the dependency direction. `ResolveDependencyInterface`'s
   `PACKAGE_SURFACE` and pattern-membership branches
   (`goanalysis.go:1376-1378`, `1783+`) resolve a *dependency's* surface from
   the depender's layout and must keep working exactly as they do — the fixtures
   under `go/internal/goanalysis/testdata/dep_resolve/dep/` pin that behavior.
5. If shape (b) is chosen, the message must name the component and the
   unanalyzable member entries, not merely state that something was skipped.
6. Update `README.md` limitation 14 and design Appendix C.6 to state the
   enforced behavior rather than the unenforced assumption.

## Dependencies
- Independent of the other step 12 tasks. Task 05 unifies membership matching
  and will build on whichever semantics this task settles, so land this first.

## Implementation Approach
1. Write the failing test first, at the CLI level: the reproduction manifest
   above, asserting the current output, then inverting it to the chosen shape.
2. Add a unit test at the `goanalysis` boundary for both the all-pattern and the
   mixed cases.
3. Implement the guard at `goanalysis.go:60` (or wherever the decision belongs
   once the mixed case is handled), keeping the dependency-side helpers
   untouched.
4. Re-run the dep_resolve fixture tests specifically to confirm the dependency
   direction is unaffected.
5. Update the two documents, then run `just ci`.

## Acceptance Criteria

1. **The vacuous pass is gone**
   - Given a manifest with `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE`
     and only pattern members
   - When `arcc check` runs on it
   - Then it does not emit the conformance line with exit 0; it either fails
     with a message naming the manifest and the unanalyzable members, or exits
     with a report containing an `ANALYSIS_LIMITATION` that names them.

2. **The mixed case is decided, not accidental**
   - Given a `PACKAGE_SURFACE` component with one literal and one pattern member
   - When `arcc check` runs on it
   - Then the outcome is the one the implementation deliberately chose, pinned
     by a test, and no `path.Match` pattern is passed to `packages.Load`.

3. **The dependency direction is untouched**
   - Given the `dep_resolve` fixtures, including `pattern_surface.textproto`
   - When the goanalysis dependency-resolution tests run
   - Then they pass unchanged, and a depender on a pattern-membership component
     still resolves that component's surface from its own layout.

4. **A regression test would catch a re-introduction**
   - Given the new test
   - When the guard is removed from the loader
   - Then the test fails.

5. **Docs state the enforced behavior**
   - Given `README.md` limitation 14 and design Appendix C.6
   - When they are read
   - Then they describe what arcc now does when such a component is checked,
     not an assumption about what will not happen.

6. **Repository checks pass**
   - Given the change
   - When `just ci` runs
   - Then every leg is green, including the eight native self-checks.

## Metadata
- **Complexity**: Medium
- **Labels**: fail-closed, goanalysis, package-surface, pattern-membership, remediation
- **Required Skills**: Go, go/packages loading, arcc report model
