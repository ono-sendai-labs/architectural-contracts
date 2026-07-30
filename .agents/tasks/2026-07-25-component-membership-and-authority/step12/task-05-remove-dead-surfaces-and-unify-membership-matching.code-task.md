# Task: Remove dead surfaces and unify membership matching

## Description
Delete three surfaces the batch left behind with no consumer, correct the design
sentence that still cites one of them as a live consumer, and give the Go core a
single membership-matching helper so "is this package a member?" has one answer
rather than two.

## Background
Addresses findings **F5** and **F6** from the implementation review.

**Dead surfaces.**

- `bazel_rules/go/private/component.bzl:313` binds `unclassified` from
  `_classify` and never uses it. This is behaviorally correct — design §3.1 says
  the error branch needs no emitter-side rule, because `UNDECLARED_DEPENDENCY`
  already reports it at check time — but `_classify` builds and sorts a list
  nobody reads, and the discarded binding reads like an unfinished error path.
  That misreading is not hypothetical: the orchestrator flagged it during the
  Step 11 doc sync for exactly that reason.
- `packagelayout.IsStdlib` (`go/internal/packagelayout/packagelayout.go:82`) has
  no caller outside its own unit test, and the free function `IsStdlibPackage`
  (`:107`) has none at all — every real call goes through the `*Layout` method.
  Design §4.6 still justifies the T4 agreement check by naming
  `packagelayout.IsStdlib` and "the checker's nil-facts fallback" as other
  consumers of `hostpolicy.IsStdlibPath` that a wrong policy would corrupt.
  Neither exists any more; the checker reads only `Facts.StdlibImports`. The
  check is still worth keeping — `hostpolicy.IsStdlibPath` remains the heuristic
  half of the layout-mode verdict, and `goanalysis` case 3 uses it natively — but
  the stated reason has gone stale and should be replaced by the real one.
- `go_adapter.bzl:156` reads `ctx.attr.infra_components`, an attribute no rule
  declares. (Task 03 removes this one; it is listed here so the sweep is
  complete, not to be done twice.)

**Membership matching.** The question is answered three times with two meanings:

- `checker.buildMembership` (`checker.go:400-408`) matches each declared member
  against each package path with `path.Match` — a member entry is a *pattern*.
- `goanalysis.packageIsDeclaredMember` (`goanalysis.go:458-466`) and
  `validateDeclaredPackages` (`:420-438`) compare canonicalized strings for
  equality — a member entry is a *literal*.
- The Bazel emitter uses `match_path` for `member_patterns` and label identity
  for `members`.

No supported configuration currently reaches both Go paths with a pattern:
manifest validation forbids patterns under the declared style, and task 01
settles what happens under `PACKAGE_SURFACE`. So this is latent rather than
live — but it is the kind of divergence that surfaces as an unexplained
difference between what the loader analyzed and what the checker swept, and it
is cheap to close once task 01 has fixed the semantics.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§3.1 the classification buckets and the E branch; §4.6 the T4 rationale paragraph; M1, M3, M5, M8)
- Implementation Review: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/review.yaml` (findings F5, F6)

**Additional References (if relevant to this task):**
- `go/internal/hostpolicy/hostpolicy.go` — the `IsStdlibPath` and `CanonicalizePath` contracts the helper must keep honoring.

## Technical Requirements
1. Stop `_classify` from returning a value the caller discards: either drop the
   third return, or consume it at the call site with a comment saying why
   nothing is emitted for it. Do not turn it into a `fail()` — §3.1 is explicit
   that the check-time rule is the reporting mechanism.
2. Delete `packagelayout.IsStdlib` and the package-level `IsStdlibPackage`,
   together with the test that exists only to call the first. Keep the `*Layout`
   method, which is the live path.
3. Rewrite design §4.6's justification for the stdlib agreement check to name
   the consumers that actually exist, without weakening the requirement — the
   check is still fail-closed and provenance is still authoritative.
4. Give `checker` and `goanalysis` one membership-matching helper with one
   documented semantics, and state on `goanalysis.LoadRequest.Members` which
   form entries may take in each mode.
5. The purity constraint is not negotiable: `checker` is a guaranteed-pure
   component and its manifest declares no authority. A shared helper must live
   somewhere `checker` may import — `facts` and `manifest` are already in its
   allowlist — or be duplicated deliberately with a comment rather than
   introduced as a new dependency edge. If a new edge is unavoidable, the
   checker's own manifest and `checker_component`'s `component_deps` must be
   updated together, and the self-check must stay green.
6. Behavior must not change for any currently supported configuration. This is a
   consolidation, not a semantics change; task 01 owns the semantics.

## Dependencies
- **Task 01** settles what a pattern member means when the component is the
  subject of a check. Land it first; this task adopts whatever it decided.
- **Task 03** removes the `infra_components` dead branch listed above.

## Implementation Approach
1. Confirm the dead surfaces are dead with a fresh search rather than trusting
   this task file, since tasks 01–04 will have moved code.
2. Remove them one at a time, running the package tests after each.
3. Extract the membership helper, choosing its home by what `checker` is allowed
   to import, and route both call sites through it.
4. Re-run the checker and goanalysis suites, then `just selfcheck`, then
   `just ci`.

## Acceptance Criteria

1. **No discarded classification result**
   - Given `component.bzl`
   - When `_classify`'s call site is read
   - Then no return value is bound and ignored, and if a value is bound it is
     used or explained.

2. **The unused packagelayout exports are gone**
   - Given `go/internal/packagelayout`
   - When it is searched for `func IsStdlib` and the package-level
     `IsStdlibPackage`
   - Then neither exists, the `*Layout` method remains, and no test was left
     behind that only existed to call a deleted function.

3. **The design cites live consumers**
   - Given design §4.6's paragraph on why the agreement check exists
   - When it is read
   - Then every code path it names exists in the tree, and the requirement is
     no weaker than before.

4. **One membership semantics**
   - Given `checker` and `goanalysis`
   - When each decides whether a package is a member
   - Then both call the same helper, and its documented semantics matches what
     `LoadRequest.Members` says entries may contain.

5. **The pure core stays pure**
   - Given `checker`'s manifest and the arcc self-checks
   - When `just selfcheck` and the Bazel `checker_component.check` run
   - Then `checker` still declares no ambient authority, and any new dependency
     edge is declared in both the checked-in and the generated manifest.

6. **No behavior change**
   - Given the full test suite before and after
   - When it runs
   - Then no test expectation was modified to accommodate this task.

7. **Repository checks pass**
   - Given the change
   - When `just ci` runs
   - Then every leg is green.

## Metadata
- **Complexity**: Medium
- **Labels**: dead-code, consistency, checker, goanalysis, design-sync, remediation
- **Required Skills**: Go, Starlark, arcc component model
