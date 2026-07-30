# Task: Correct the adapter seam contract and the macro documentation

## Description
`go_attach_infra` is declared and documented as taking a single Go target; its
only call site passes a list of root targets. Fix the contract in all three
places that state it — the parameter name, the seam docstring, and design §4.7 —
and pin it with a test whose predicate actually reads the argument. While in the
same area, correct the `go_component` macro doc, which still describes every
component as wrapping a Go interface library.

## Background
Addresses findings **F4** and **F7** from the implementation review.

The seam is declared `def go_attach_infra(target, infra)`
(`bazel_rules/go/private/go_adapter.bzl:70`) and its docstring says "`target` is
the Go target being wrapped and `infra` is one entry from `INFRA_COMPONENTS`. A
host may inspect either input when its analysis-phase graph exposes the relevant
injected packages." Design §4.7 declares the same signature.

The call chain passes a list:

```python
# component.bzl:204, 206
roots = ([ctx.attr.interface] if ctx.attr.interface else []) + ctx.attr.members
attached_infra = go_attached_infra(ctx, roots, ctx.attr.infra_deps)
# go_adapter.bzl:200
should_attach = go_attach_infra(roots, entry)
```

The list is correct: M9 requires layout inputs and, by extension, attachment to
derive from the union of the interface and the members rather than one
distinguished target. So the code is right and three pieces of documentation are
wrong.

This matters more than an ordinary stale comment. The seam exists to be
reimplemented by someone who has only the documentation. A host that writes
`target[ArccPackageInfo]` per the docstring fails at analysis time; a host that
takes the documented licence to return `True` unconditionally never finds out
the signature was different. Upstream's own implementation ignores both
arguments, so nothing currently catches the divergence.

The second finding is the open `suggestion` from the Step 6 task 03 review,
verified still present at `defs.bzl:186-192`: the public macro doc says a
component is declared "around a Go interface library" and that the target
"forwards the interface library's Go providers", neither of which is true for a
`PACKAGE_SURFACE` target.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§4.7 `go_attach_infra`; M9; A7; §4.8 the shape rule table)
- Implementation Review: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/review.yaml` (findings F4, F7)

**Additional References (if relevant to this task):**
- `.agents/scratchpad/2026-07-25-component-membership-and-authority/step06/task-03-enforce-component-shape-rules/review.yaml` — the original macro-doc finding.
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` F3 — why a host may be unable to evaluate a closure predicate at all.

## Technical Requirements
1. Rename `go_attach_infra`'s first parameter to reflect what it receives (for
   example `targets` or `roots`) and rewrite the docstring to say it is given
   every root target of the component being wrapped — the interface, if any,
   plus the declared members — and why (M9).
2. Keep the docstring's existing statement that returning `True` unconditionally
   is conforming, and its reason. That part is accurate and load-bearing.
3. Correct design §4.7's declaration and prose to match. Do not amend around it;
   the signature line itself is what a host copies.
4. Add an analysis test whose attachment predicate inspects its first argument
   and would fail if a single target were passed instead of the root list — for
   example, one that attaches only when more than one root is present, or that
   reads a provider off each element. The contract must be pinned by executable
   behavior, not only by prose.
5. Rewrite the `go_component` macro doc (`defs.bzl:186-206`) to distinguish
   declared-style from `PACKAGE_SURFACE` targets, and to state that only
   declared style forwards the interface library's Go providers. Keep the
   example, and consider adding the `PACKAGE_SURFACE` form from design §4.8.
6. Check the neighbouring seam docstrings while there: `go_build_platform` and
   `go_target_info` are accurate today and should stay untouched unless this
   task's rename makes one of them inconsistent.

## Dependencies
- Task 03 removes the test-only branch from the same function's neighbourhood.
  Land task 03 first to avoid editing code that is about to move.

## Implementation Approach
1. Write the pinning analysis test first against the current signature and watch
   it pass for the wrong reason (the upstream predicate ignores its arguments),
   then make it discriminate.
2. Rename the parameter and rewrite the docstring.
3. Edit design §4.7, checking that no other design section repeats the
   single-target framing.
4. Rewrite the macro doc.
5. Run `just ci`.

## Acceptance Criteria

1. **The seam signature says what it receives**
   - Given `go_attach_infra`
   - When its declaration and docstring are read
   - Then the first parameter is named for a collection and the docstring states
     it is the component's root targets — the interface if present plus the
     declared members — citing M9 as the reason.

2. **The design matches the code**
   - Given design §4.7
   - When its `go_attach_infra` declaration is read
   - Then it agrees byte-for-byte in shape with the implementation's signature,
     and no other design section still describes a single wrapped target.

3. **The contract is pinned by a test**
   - Given the new analysis test
   - When the call site is changed to pass a single target instead of the root
     list
   - Then the test fails.

4. **The macro doc describes both authoring forms**
   - Given `go_component`'s `doc` string
   - When an author reads it to choose a style
   - Then it distinguishes declared style from `PACKAGE_SURFACE`, and states
     that provider forwarding happens only for declared style.

5. **The conforming-constant licence survives**
   - Given the rewritten docstring
   - When it is read
   - Then it still says that returning `True` unconditionally is conforming, and
     still gives the reason (pruning at a package is a no-op until the package is
     reached, and a component's own authority is charged regardless).

6. **Repository checks pass**
   - Given the change
   - When `just ci` runs
   - Then every leg is green.

## Metadata
- **Complexity**: Low
- **Labels**: Bazel, host-seam, documentation, design-sync, remediation
- **Required Skills**: Starlark, Bazel analysis tests, technical writing
