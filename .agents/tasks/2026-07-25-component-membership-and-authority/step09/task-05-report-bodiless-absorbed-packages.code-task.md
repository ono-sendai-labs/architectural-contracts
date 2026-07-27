# Task: Report bodiless absorbed packages as an analysis limitation

## Description
Observe, in the loader, absorbed packages the layout shows with no source bodies,
and report each from the pure checker as `ANALYSIS_LIMITATION` naming the
package, so a report cannot look clean where authority was simply unanalyzable.

## Background
An absorbed dependency is code the component takes responsibility for: its
authority is charged to the component through use-attribution, which requires the
analysis to actually see its bodies. When the layout carries an absorbed package
with no surviving source files, nothing is analyzed and nothing is charged — and
today the report says nothing at all. That is a clean-looking report over an
unanalyzed hole, which is exactly the fail-open shape this whole design exists to
close.

The layout loader already anticipates this. Its root check rejects a root with no
surviving source, deliberately "leaving bodiless transitive packages available for
later reporting" — this task is that later reporting. Note the asymmetry and keep
it: a bodiless **member** is a hard error (M10), because a component must be able
to analyze its own code; a bodiless **absorbed** package is a warning, because the
component may legitimately be built against something the analysis cannot see and
the honest response is to say so.

**Reuse `ANALYSIS_LIMITATION`; do not reach for the `UNANALYZED` authority
constant.** The word fits, but the constant is taken and its handling is
load-bearing: `UNANALYZED` is a Capslock capability the adapter deliberately
excludes from the classifier, because with it visible, functions like
`io.ReadAll` become capability leaves and mask the real `FILES` flow behind them.
Giving the constant a second meaning would entangle a report concept with a
classifier setting whose whole purpose is to stay suppressed. `ANALYSIS_LIMITATION`
already means "the analysis could not see through this", which is the claim.

This follows the same loader-collects / checker-reports split `UnresolvedImports`
uses: `goanalysis` owns the observation, the pure checker owns the wording.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A9 and M10, §4.3 table, §4.4, §5.2, §5.3 — including "Why A9 reuses ANALYSIS_LIMITATION")
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 9)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a pure observation to `facts.PackageFacts` recording absorbed packages
   with no analyzable bodies — the package path, as canonical data with a doc
   comment stating the loader owns the observation and the checker owns the
   wording. `facts` gains no logic.
2. Produce it in `goanalysis` for both loading modes, using the absorbed
   declarations already carried on `LoadRequest` (added in Step 8) and the same
   `path.Match` glob semantics the checker and the escape scan use, over
   canonicalized paths, so all three agree on what "absorbed" means.
3. "Bodiless" means no surviving source files after normalization and build-
   constraint filtering — the same notion the layout's root check applies, not
   merely an absent `GoFiles` list before filtering. Reuse the existing helper
   rather than reimplementing the predicate.
4. Report only packages that are actually present in the analyzed closure. An
   absorbed declaration matching nothing is the existing `UNUSED_DEPENDENCY`
   case and must not become an analysis limitation as well.
5. Do not weaken M10: a declared **member** with no source files stays a hard
   load error with its existing message. Add a test pinning both behaviors side
   by side so the two are not conflated later.
6. Deduplicate and sort deterministically; the field is always initialized,
   empty rather than nil when there is nothing to report.
7. In the checker, emit one `ANALYSIS_LIMITATION` warning per observed package,
   naming the package and saying plainly that its authority could not be
   analyzed. No new report kind. The checker stays pure.
8. Message wording must be distinguishable from the existing
   `ANALYSIS_LIMITATION` producers (an unresolved import, and a capability
   classified as analysis-defeating), since all three share a kind and a reader
   needs to tell them apart.
9. Warnings only: no violation, no exit-code change.
10. Update any golden affected; a component with no bodiless absorbed packages
    must render exactly as before.

## Dependencies
- Step 8 put the component's absorbed declarations on `goanalysis.LoadRequest`
  and wired them from the application; this task reuses that input rather than
  adding another.
- Step 1 supplies the M10 bodiless-member error this must not duplicate or
  weaken.
- Independent of Tasks 1-4 and 6; it can be implemented and reviewed on its own.

## Implementation Approach
1. Add the pure field and its doc comment.
2. Add the loader-side collection next to the existing absorbed matching, taking
   the surviving-source predicate from the layout code path so the two cannot
   drift.
3. Build layout fixtures with a bodiless absorbed package, a bodied absorbed
   package, a bodiless non-absorbed transitive package, and a bodiless member,
   and assert the produced facts (and, for the member, the error) exactly.
4. Add the checker warning and its message, then a checker test asserting the
   three `ANALYSIS_LIMITATION` producers are individually identifiable.
5. Run focused `facts`, `packagelayout`, `goanalysis`, and `checker` suites, then
   `just ci`.

## Acceptance Criteria

1. **A bodiless absorbed package is reported**
   - Given a layout whose absorbed package has no surviving source files
   - When `arcc check` runs
   - Then exactly one `ANALYSIS_LIMITATION` warning names that package.

2. **A bodied absorbed package is silent**
   - Given an absorbed package with source files
   - When facts are loaded
   - Then no observation is produced for it.

3. **Only closure packages count**
   - Given an absorbed declaration matching no package in the closure
   - When `arcc check` runs
   - Then the existing `UNUSED_DEPENDENCY` warning is produced and no analysis limitation is.

4. **A bodiless member is still a hard error**
   - Given a declared member with no source files
   - When facts are loaded
   - Then loading fails with the existing M10 error, not a warning.

5. **A bodiless non-absorbed package is not reported**
   - Given a bodiless transitive package that is neither member nor absorbed
   - When facts are loaded
   - Then no observation is produced, preserving today's behavior.

6. **Glob-declared absorption matches**
   - Given an absorbed declaration written as a pattern that matches a bodiless package
   - When facts are loaded
   - Then the observation is produced, using the same match semantics the checker applies.

7. **The three analysis limitations are distinguishable**
   - Given a component with a bodiless absorbed package, an unresolved import, and an analysis-defeating capability
   - When the report is rendered
   - Then all three appear as `ANALYSIS_LIMITATION` with messages a reader can tell apart.

8. **Facts are deterministic and always initialized**
   - Given repeated loads, and a component with nothing to report
   - When facts are returned
   - Then the field is duplicate-free, sorted, byte-identical across runs, and non-nil when empty.

9. **Unaffected components are byte-identical**
   - Given every existing fixture and self-manifest
   - When reports are rendered
   - Then no output changes.

10. **Repository checks remain green**
    - Given the new observation and warning
    - When `just ci` runs
    - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, goanalysis, packagelayout, facts, checker, analysis-limitation
- **Required Skills**: Go, package-layout fixtures, loader/checker split discipline
