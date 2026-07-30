# Task: Close the open test findings and the stale comments

## Description
Two tests assert properties their fixtures cannot falsify, two keep names that
say the opposite of what they now check, and a scattering of comments still
point at removed rules, superseded design sections and task numbers. Fix them
together.

## Background
Addresses findings **F8**, **F9** and **F10** from the implementation review.

**Tests that cannot fail (F8).** The Step 9 task 05 orchestrator re-review
approved the task with four findings open, two of which are worth closing and
were deferred to this review by name:

- `TestCollectBodilessAbsorbedPackages_DeduplicationAndSorting`
  (`go/internal/goanalysis/canonicalization_test.go:730`) cannot fail on either
  property in its name. The reviewer removed the `seen` guard
  (`goanalysis.go:2107-2110`) and then `sort.Strings` (`:2114`) in a scratch copy
  of the module, and the test passed both times: `packages.Visit` already dedupes
  by pointer and sorts each package's import paths, so the fixture arrives sorted
  and deduped. The criterion's other claims — non-nil when empty, byte-identical
  across runs — *are* pinned and did fail under mutation.
- `TestCheck_ThreeAnalysisLimitationsAreDistinguishable`
  (`go/internal/checker/checker_test.go:2057`) asserts the rendered report
  contains `"is an analysis limitation"`, evidently to identify the capability
  limitation. The unresolved-import message ends in the same words
  (`checker.go:233`), so the assertion holds even if the capability warning is
  missing entirely. The count-is-3 and distinct-messages assertions carry the
  test, so this is a weak assertion rather than a false one.

**Names that mislead (F9).** Step 8 required the two end-to-end
`HIGHER_ORDER_BOUNDARY_CALL` tests to be *converted* to silence assertions
rather than deleted, so they become the pinning tests for the plugin-struct
pattern — the polarity problem that justified removing the kind. The conversion
was done correctly and the names were not touched:
`TestIntegration_HigherOrderBoundaryCall_Warning`
(`go/cmd/arcc/cli_integration_test.go:868`) now asserts exit 0 and the absence of
the old string, and `TestCheck_FR5_HigherOrderBoundaryCall`
(`go/internal/checker/checker_test.go:954`) asserts zero violations and zero
warnings. A reader scanning test names concludes the warning still exists. The
same staleness sits in `checker.go:41`, whose doc comment still lists "FR5
(cross-component call-boundary rule and higher-order boundary-call warning)".

**Stale comments (F10).**

- `go/internal/manifest/manifest.go:207` — "validate is a seam for task-03 to
  add syntactic validation rules"; the rules were added in that task.
- `bazel_rules/go/defs.bzl:49-51` — the exported `validate_component_shape`
  wrapper is wedged between the authority constants and `PACKAGE_SURFACE` with no
  blank line and nothing saying it exists for `tests/testing.bzl`.
- `go/internal/goanalysis/goanalysis.go:1731` —
  `isAllPatternMembers(m.Members) || (len(m.Members) > 0 && isAnyPatternMember(m.Members))`;
  the first disjunct is subsumed by the second.
- `bazel_rules/go/private/component.bzl:307` —
  `getattr(ctx.attr, "member_patterns", [])` defends against the absence of an
  attribute the same rule declares.
- `go/cmd/arcc/app/app.go:277,289` — a second `// 7.` and `// 8.` after the first
  pair.
- `bazel_rules/go/private/component.bzl` module docstring and `_classify`'s
  docstring both cite design §5.3, which Step 7 superseded.
- `bazel_rules/go/private/component.bzl:134` hardcodes `"is_stdlib": False` with
  no comment, at exactly the spot where `docs/package-layout-schema.md` §4 tells
  a reader to be suspicious of a heuristic. It is structurally correct here — the
  aspect closure enumerates build targets and the SDK is not among them, which
  §5.1a of the design calls out as the case where the bit is structurally false —
  and should say so.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (A5 and §4.3 on why `HIGHER_ORDER_BOUNDARY_CALL` was removed; §5.1a `is_stdlib` provenance; §3.1)
- Implementation Review: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/review.yaml` (findings F8, F9, F10)

**Additional References (if relevant to this task):**
- `.agents/scratchpad/2026-07-25-component-membership-and-authority/step09/task-05-report-bodiless-absorbed-packages/orchestrator-review.yaml` — the two open findings, with the reviewer's mutation-testing evidence and its suggested fixture shapes.
- `docs/package-layout-schema.md` §4 — the provenance requirement the emitter comment should reference.

## Technical Requirements
1. Make `TestCollectBodilessAbsorbedPackages_DeduplicationAndSorting` able to
   fail on both named properties, or rename it to what it verifies and drop the
   unsupported claims from its name and comment. If keeping the name: give the
   root two import keys whose lexicographic order is the reverse of the absorbed
   packages' traversal order so a missing `sort.Strings` fails, and use two
   distinct `*packages.Package` values whose paths canonicalize to the same
   absorbed path so a missing `seen` guard yields a duplicate.
2. Prove the strengthened assertions by mutation: remove the `seen` guard, run
   the test, confirm it fails; restore; remove `sort.Strings`, run, confirm it
   fails; restore. Record both results in the work log.
3. Change `TestCheck_ThreeAnalysisLimitationsAreDistinguishable`'s capability
   assertion to a substring unique to the capability producer — for example
   `capability "FILES"` — so each of the three checks identifies its own
   limitation.
4. Rename `TestIntegration_HigherOrderBoundaryCall_Warning` and
   `TestCheck_FR5_HigherOrderBoundaryCall` to say what they pin: that the
   plugin-struct pattern produces no finding. Keep their bodies and assertions
   as they are — the conversion was the point of Step 8 task 01 and must not be
   undone.
5. Drop the removed warning from `checker.go`'s `Check` doc comment, leaving the
   FR5 call-boundary rule described accurately.
6. Sweep the F10 comment list. Each is a one-line change; none justifies a
   separate commit.
7. Add the missing rationale comment at `component.bzl:134`, saying that the bit
   is structurally `false` because the emitter's closure contains only
   enumerated build targets, and pointing at `docs/package-layout-schema.md` §4.
8. Do not weaken or delete any existing assertion to make this task simpler.

## Dependencies
- Tasks 01–05 touch `goanalysis.go`, `component.bzl`, `defs.bzl` and `app.go`.
  Land this last so the comment sweep lands on final text.

## Implementation Approach
1. Start with the two test fixtures, using mutation to establish RED before and
   GREEN after, since the whole finding is that the current tests are green
   under mutation.
2. Rename the two converted tests and grep for any reference to their old names
   (task files, work logs and the README are out of scope; code references are
   not).
3. Fix the `Check` doc comment.
4. Sweep the comment list in one pass, re-checking each line number against the
   post-task-05 tree.
5. Run `just ci`.

## Acceptance Criteria

1. **The dedup and sort properties can fail**
   - Given the strengthened collector test
   - When the `seen` guard is removed, and separately when `sort.Strings` is
     removed
   - Then the test fails in each case, and both runs are recorded in the work
     log. (Or: the test is renamed to what it verifies and no longer claims
     either property.)

2. **Each analysis limitation is identified by its own producer**
   - Given `TestCheck_ThreeAnalysisLimitationsAreDistinguishable`
   - When the capability warning is suppressed but the other two remain
   - Then the test fails.

3. **Test names describe what they assert**
   - Given the two converted higher-order tests
   - When their names are read
   - Then they say they pin the plugin-struct pattern's silence, and their
     bodies still assert exit 0 / zero findings and the absence of the removed
     kind's string.

4. **No stale reference to the removed warning**
   - Given `go/` and `bazel_rules/`
   - When searched for the higher-order boundary-call warning in comments, doc
     strings and test names
   - Then nothing implies the kind still exists.

5. **The comment sweep is complete**
   - Given the F10 list in the implementation review
   - When each cited location is read
   - Then it has been corrected, including the `is_stdlib` rationale comment at
     the emission site and the two superseded §5.3 citations.

6. **Nothing was weakened**
   - Given the diff
   - When it is reviewed
   - Then no assertion was deleted or loosened, and no test was skipped.

7. **Repository checks pass**
   - Given the change
   - When `just ci` runs
   - Then every leg is green.

## Metadata
- **Complexity**: Low
- **Labels**: tests, mutation-testing, comments, hygiene, remediation
- **Required Skills**: Go testing, Starlark, arcc report model
