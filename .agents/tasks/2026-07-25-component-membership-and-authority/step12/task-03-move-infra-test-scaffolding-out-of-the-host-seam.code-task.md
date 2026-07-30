# Task: Move the infra-attachment test harness out of the host seam

## Description
`go_adapter.bzl` is documented as the one file a host replaces to port arcc.
It now contains a test-only attachment harness with hardcoded fixture patterns,
and the production component rule declares two attributes documented as
"Undocumented testing attribute". Move the harness somewhere a porting host will
never have to read or delete, and drop the unreachable registry branch.

## Background
Addresses finding **F3** from the implementation review.

The seam file opens by promising that a host "ports arcc by replacing just this
file — the rules above it stay byte-identical". Against that promise,
`go_infra_components` (`bazel_rules/go/private/go_adapter.bzl:120-158`) begins
with a 35-line branch that exists only for arcc's own analysis tests:

```python
if ctx != None and hasattr(ctx.attr, "test_infra_attach") and ctx.attr.test_infra_attach:
    ...
    def _test_predicate(roots, entry):
        if attach_mode == "ALWAYS": return True
        if attach_mode == "NEVER":  return False
        if attach_mode == "CLOSURE":
            search_patterns = getattr(entry, "import_path_patterns", [])
            if not search_patterns:
                search_patterns = ["*runtime*", "*injected*", "*member*"]
```

and `component.bzl:470-475` correspondingly declares `test_infra_patterns` and
`test_infra_attach` on the production rule. Six BUILD files under
`bazel_rules/go/tests/testdata/` set them.

Two costs. Because the file is the swap unit, the harness is not separable by
construction: a porting host must read past it to find the contract and then
decide whether removing it breaks anything. And the attachment behavior arcc
tests is largely the test predicate rather than `go_attach_infra`, so the seam
that actually ships is less exercised than the fixture count suggests.

A dead branch sits immediately below: line 156 reads `ctx.attr.infra_components`,
an attribute no rule in the tree declares, so its `hasattr` guard is always
false.

The encapsulation the Step 9 task 06 rework achieved — attachment behind the
adapter seam rather than open-coded in the rule — is the right shape and must
survive this change. This task moves the *test* half out, not the seam.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§4.7 the three seam additions; §9 "Host seams as single files"; A7)
- Implementation Review: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/review.yaml` (finding F3)

**Additional References (if relevant to this task):**
- `bazel_rules/go/tests/testdata/infra/consumer/BUILD.bazel` and `.../unused_consumer/BUILD.bazel`, `bazel_rules/go/tests/testdata/membercomponent/BUILD.bazel` — the six fixtures that set the test attributes.
- `bazel_rules/go/tests/testing.bzl` — the existing precedent for a test-only wrapper over a public symbol.

## Technical Requirements
1. Remove the `test_infra_attach` branch and its `_test_predicate` from
   `go_adapter.bzl`, along with the hardcoded fallback patterns
   `["*runtime*", "*injected*", "*member*"]`.
2. Remove the unreachable `ctx.attr.infra_components` branch
   (`go_adapter.bzl:156-157`).
3. Remove `test_infra_patterns` and `test_infra_attach` from
   `go_component_rule`'s attrs, or move them onto a clearly test-scoped rule
   that is not the one hosts ship.
4. Keep every existing attachment fixture behaving as it does today. The ALWAYS,
   NEVER and CLOSURE cases each pin a real property — unconditional attachment
   still emitting the manifest entry, no attachment when the closure lacks the
   runtime, and pattern matching — and none may be dropped to simplify the move.
5. Do not undo the Step 9 encapsulation: attachment must still be reached
   through the adapter, so a host still overrides one file to change it.
6. Leave `go_attach_infra`, `go_infra_deps` and `INFRA_COMPONENTS` in the seam,
   including the commented worked examples, which are contract documentation.
7. After the move, `go_adapter.bzl` must contain no branch predicated on an
   attribute whose only setters are test fixtures.

## Dependencies
- Task 04 rewrites `go_attach_infra`'s signature and docstring in the same file.
  Land this one first so task 04 edits a seam that no longer has a test branch
  competing with the real predicate.
- Touches the same Bazel goldens as task 02 if attachment output changes; it
  should not, and a golden diff here is a signal to stop and check.

## Implementation Approach
1. Enumerate the fixtures and, for each, write down which property it pins.
   That list is the regression contract for the move.
2. Introduce the test-only seam — a test adapter, or a thin rule in
   `bazel_rules/go/tests` that supplies the registry — and repoint the fixtures
   at it.
3. Delete the branch, the two attributes and the dead `infra_components` guard.
4. Run the Bazel analysis tests and confirm each previously-enumerated property
   still has a test that fails when the corresponding behavior is broken; spot
   check by inverting one predicate.
5. Run `just ci`.

## Acceptance Criteria

1. **The seam carries no test-only code**
   - Given `bazel_rules/go/private/go_adapter.bzl`
   - When it is read end to end
   - Then it contains no `test_infra_*` handling, no hardcoded fixture patterns,
     and no branch on an attribute set only by test fixtures.

2. **The production rule carries no undocumented test attributes**
   - Given `go_component_rule`'s attrs
   - When they are read
   - Then `test_infra_patterns` and `test_infra_attach` are absent from the rule
     hosts ship.

3. **The dead registry branch is gone**
   - Given `go_adapter.bzl`
   - When it is searched for `infra_components`
   - Then only the real `INFRA_COMPONENTS` registry and its accessor remain.

4. **Attachment coverage is preserved, not thinned**
   - Given the infra and membercomponent fixtures
   - When the Bazel analysis tests run
   - Then unconditional attachment still emits the manifest entry, attachment
     still does not happen when the closure lacks the runtime, and pattern
     matching is still asserted — each by a test that fails if that behavior is
     broken.

5. **Attachment stays behind the adapter**
   - Given `component.bzl`
   - When its attachment path is read
   - Then it still reaches attachment through the adapter seam rather than
     open-coding the predicate.

6. **Repository checks pass**
   - Given the change
   - When `just ci` runs
   - Then every leg is green, with no Bazel golden changed by this task.

## Metadata
- **Complexity**: Medium
- **Labels**: Bazel, host-seam, portability, test-scaffolding, remediation
- **Required Skills**: Starlark, Bazel rules and analysis tests
