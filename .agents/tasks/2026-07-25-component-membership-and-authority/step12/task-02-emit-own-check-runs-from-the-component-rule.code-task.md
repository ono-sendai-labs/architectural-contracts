# Task: Emit `own_check_runs` so a certified boundary is reachable

## Description
A8's dependency-listing annotation can only ever print `asserted` for a
generated manifest, because no emitter writes `own_check_runs`. Make the
`go_component` rule record whether the component it emits has a check that
actually runs, so `certified` becomes reachable and the annotation carries
information. Emitter side only — the report side of A8 is complete and correct.

## Background
Addresses finding **F2** from the implementation review.

`_manifest_content` (`bazel_rules/go/private/component.bzl:68-112`) emits name,
`interface_style`, `interface_files`, `component_dependencies`,
`absorbed_dependencies`, `members` and `declared_authority`. It never emits
`own_check_runs` or `certification_reference`. A grep over `*.bzl`,
`*.textproto` and `BUILD.bazel` finds those two fields only in goanalysis unit
fixtures and one Bazel golden asserting the JSON default `false`.

The visible consequence is on arcc itself. `just ci` currently prints, for every
one of the eight self-checks:

```
Dependencies:
- capanalyzer: asserted
- facts: asserted
...
```

including the six components whose `.check` targets run under `bazel test //...`
and are therefore certified in the only sense A8 defines. Design §5.3 argues at
length against making an uncertified boundary a *finding*, on the grounds that
the dependency listing carries the signal instead. Right now the listing has one
reachable value, so that argument is not yet paid for.

The emitter already knows the answer without a lookup: the `go_component` macro
unconditionally expands to `name.check` (`defs.bzl:129-143`). The one case that
needs a decision is a component carrying `tags = ["manual"]` — the deliberate
analysis-failure fixtures — whose generated check is deliberately kept out of
`bazel test //...` and therefore does not run.

Design §4.1 is explicit that this is the emitter's job: "The dependency's own
emitter is the only party that knows whether a check target exists; a depender
cannot infer it."

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§4.1 the two certification fields and their trust level; A8 in §2; §5.3 "No new kind for an uncertified boundary")
- Implementation Review: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/review.yaml` (finding F2)

**Additional References (if relevant to this task):**
- `bazel_rules/go/defs.bzl:129-143` — the macro's unconditional `arcc_check_test` expansion and its `tags`/`testonly` inheritance.
- `go/internal/report/report.go:141-150` — `formatDependencyBoundary`, the consumer.
- `go/internal/manifestparity/manifestparity.go` — the parity test that must keep the generated and checked-in manifests in agreement.
- `bazel_rules/go/tests/goldens/` — the manifest goldens this will move.

## Technical Requirements
1. Emit `own_check_runs` from `_manifest_content` for every component the rule
   generates, with a value that reflects whether that component's generated
   `.check` participates in `bazel test`.
2. Decide the `manual`-tagged case explicitly and comment it at the emission
   site. A fixture whose check is suppressed does not have a check that runs,
   and claiming otherwise would make the annotation lie in the one place the
   repository can observe it.
3. Do not emit `certification_reference` from the rule. It is an out-of-band
   human reference (design §4.1) and has no build-graph source; leave it to
   hand-written manifests.
4. Keep the field a self-declaration. Do not add checker-side verification, and
   do not let a depender infer it — that is the design's stated non-goal.
5. Update every affected golden under `bazel_rules/go/tests/goldens/` and the
   checked-in self-manifests that the parity test compares, so
   `self_manifest_parity_test` and `manifest_parity_test` stay green without
   being loosened.
6. Add coverage that the annotation now discriminates: a depender on a
   check-running component renders `certified`, and a depender on one without a
   running check renders `asserted`. An assertion that only checks the emitted
   textproto field is not sufficient — the point is the rendered report.

## Dependencies
- Independent of the other step 12 tasks.
- Touches the same goldens as task 03; if both are in flight, land this one
  first so task 03 rebases onto the updated goldens.

## Implementation Approach
1. Start from the observable: run `just selfcheck` and capture the current
   all-`asserted` listings as the RED baseline.
2. Add the emission in `_manifest_content`, threading whatever signal the rule
   needs from the macro (the macro is where `tags` is known).
3. Regenerate the goldens and the checked-in self-manifests, and read the diff
   rather than accepting it — a self-manifest that flips to `own_check_runs:
   true` for `capslockadapter` or `cli` would be wrong, since neither has a
   Bazel check.
4. Add the discriminating report-level test.
5. Run `just ci` and re-read a self-check's dependency listing to confirm the
   values are now mixed and correct.

## Acceptance Criteria

1. **Generated manifests record the fact**
   - Given a `go_component` target whose `.check` runs under `bazel test`
   - When its manifest is generated
   - Then the manifest contains `own_check_runs: true`.

2. **A suppressed check is not claimed as running**
   - Given a `manual`-tagged component whose generated `.check` is excluded from
     `bazel test //...`
   - When its manifest is generated
   - Then `own_check_runs` reflects that the check does not run, and the
     emission site carries a comment saying why.

3. **The annotation discriminates in a rendered report**
   - Given two components, one with a running check and one without, each a
     dependency of a third
   - When the third component's report is rendered
   - Then its dependency listing shows `certified` for the first and `asserted`
     for the second.

4. **arcc's own reports stop being uniformly asserted**
   - Given `just selfcheck`
   - When the eight reports are read
   - Then dependencies on the six Bazel-checked components render `certified`,
     and the values are consistent with which components actually have checks.

5. **Parity is preserved without loosening it**
   - Given `self_manifest_parity_test` and the csvtool `manifest_parity_test`
   - When they run
   - Then they pass against regenerated goldens and checked-in manifests, with
     no comparison field removed or exempted to make them pass.

6. **Repository checks pass**
   - Given the change
   - When `just ci` runs
   - Then every leg is green.

## Metadata
- **Complexity**: Medium
- **Labels**: Bazel, emitter, A8, certification, goldens, remediation
- **Required Skills**: Starlark, Bazel rules and macros, arcc manifest schema
