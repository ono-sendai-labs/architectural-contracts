# Task: Parity-test the bazelified self-manifests against their generated form

## Description
Add a `manifest_parity_test` for the six arcc components that have
`go_component` targets, following the
csvtool precedent, so the hand-written `component.textproto` files and the
manifests the `go_component` rule generates cannot drift. Factor the comparison
out of the csvtool test so both tests apply one set of normalization rules.

## Background
Two descriptions of arcc's architecture now exist side by side: the eight
checked-in manifests that `just selfcheck` reads, and the six Bazel declarations
that Tasks 1–2 added (`capslockadapter` and `cli` have none — see requirement 1). Each is checkable on its own, and each check
passing proves only that *that* description matches the code. Nothing yet proves
the two descriptions match each other — and a silent divergence is exactly how
the `hostpolicy` breakage survived.

`go/examples/csvtool/app/manifest_parity_test.go` already solves this shape for
the example components. It is driven by Bazel, which passes every relevant
manifest as a positional argument; under plain `go test` it gets none and skips.
It compares meaning rather than bytes, normalizing away the `_component` target
suffix, absorbed-dependency `reason` text (the rule emits none) and the path
frame (interface files compared by basename), and it compares interface files as
checked-in ⊆ generated because the Bazel model's interface granularity is the
whole interface library.

The self-manifests need one normalization csvtool does not exercise, because the
csvtool manifests were migrated to list their interface package in `members`
explicitly: the emitter always includes the interface package in the generated
member set (M5 makes it an implicit member). Whichever convention the
self-manifests adopt in Tasks 1–2, the comparison must state it once, in code
both tests share, rather than growing a second dialect.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M4/M5, B3; §7.3 "Follow the existing `manifest_parity_test` precedent")
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 10)

**Additional References (if relevant to this task):**
- `go/examples/csvtool/app/manifest_parity_test.go` and
  `go/examples/csvtool/app/BUILD.bazel` — the test to generalize, including its
  `$(rootpaths)` argument wiring, `gazelle:exclude` comment and the reasoning in
  its header comment.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a Bazel-driven parity test covering the **six** arcc components that have
   `go_component` targets: `capanalyzer`, `report`, `facts`, `manifest`, `checker`
   and `goanalysis`. **`capslockadapter` and `cli` are excluded** — neither has a
   `go_component` target, so neither has a generated manifest to compare against.
   One root cause: capslock's closure contains `golang.org/x/sys/unix` built with
   cgo, the rule fails closed on cgo closures, and `cli`'s `main.go` imports
   `capslockadapter` and so inherits it. Both keep native coverage via
   `just selfcheck`.
2. Home it at `//go/cmd/arcc/app`, arcc's composition root, mirroring csvtool's
   choice to home its test at the composition root that pulls in every
   component. Name the target `self_manifest_parity_test` so gazelle's
   `app_test` convention is not shadowed, and exclude the new source file from
   gazelle with a `# gazelle:exclude` comment, as csvtool does.
3. Pass each component's generated artifacts with `$(rootpaths
   //pkg:<name>_component)` and each checked-in manifest with `$(rootpath
   //pkg:component.textproto)`, listing both in `data`. Ignore
   `*.package-layout.json` arguments, as the csvtool test does.
4. Do not copy the comparison logic. Extract the parsing, keying and field
   comparisons into a single Go package used by both tests — a `testonly` helper
   package (for example `go/internal/manifestparity`) with a BUILD file and
   visibility that reaches both `//go/cmd/arcc/app` and
   `//go/examples/csvtool/app`. The csvtool test must keep its current
   behavior and coverage after the extraction.
5. Compare, per component: manifest `name` (with the `_component` suffix
   normalized), members, interface style, absorbed import paths (reasons
   ignored), declared authority, and component-dependency names; and assert
   checked-in interface files ⊆ generated interface files, compared by basename.
6. Handle the interface package's implicit membership explicitly in the shared
   comparison: state in one place how a checked-in member list relates to the
   generated one, with a comment explaining the rule, so a reader can tell a
   convention from an accident.
7. Key generated and checked-in manifests together robustly. The csvtool test
   keys on the parent directory basename; verify that still discriminates for
   the arcc set (the six bazelified `internal/*` components) and, if
   it does not, key on something that does rather than renaming components.
8. The test must fail on a real divergence. Prove it during development by
   temporarily perturbing one checked-in manifest (an added absorbed path, a
   dropped member) and observing a failure that names the component and the
   field; leave no perturbation behind.
9. The test must skip cleanly under plain `go test ./...`, which passes it no
   manifest arguments.
10. Do not weaken any existing csvtool parity assertion, and do not modify the
    Bazel rules or the `justfile`.

## Dependencies
- Tasks 1–2 of this step provide the six `go_component` targets, their
  `exports_files(["component.textproto"])` declarations, and the migrated
  checked-in manifests.
- `task-05-self-check-dependency-regression` is independent of this task.

## Implementation Approach
1. Extract the comparison from `go/examples/csvtool/app/manifest_parity_test.go`
   into the shared helper package, leaving the csvtool test as a thin driver;
   confirm `bazel test //go/examples/csvtool/...` is still green before adding
   anything new.
2. Add `go/cmd/arcc/app/self_manifest_parity_test.go` as a second thin driver
   and wire its `go_test` target's `args` and `data` for the six components.
3. Run it, and expect the first run to disagree somewhere — the point of the
   test. Resolve each disagreement by correcting whichever side is wrong, not by
   widening the normalization; widening is warranted only where the two forms
   differ *by construction* (target-name suffix, reason text, path frame,
   interface granularity, implicit interface-package membership).
4. Perturb a manifest to confirm the failure message is specific, revert, then
   run `just ci`.

## Acceptance Criteria

1. **All six bazelified components are covered**
   - Given the new parity test target
   - When it runs under Bazel
   - Then it compares a generated and a checked-in manifest for each of the
     six components with `go_component` targets, and fails if any of them has no
     counterpart. `capslockadapter` and `cli` are absent by design and their
     absence is explained in a comment naming the shared cgo root cause, so a later
     reader does not read it as an oversight.

2. **One comparison, two callers**
   - Given the csvtool and self parity tests
   - When the sources are inspected
   - Then the field comparisons and normalization rules exist in exactly one Go
     package that both tests call.

3. **Csvtool parity is preserved**
   - Given the extraction
   - When `bazel test //go/examples/csvtool/...` runs
   - Then the csvtool parity test still passes and still asserts members,
     interface style, absorbed paths, declared authority, dependency names and
     the interface-file subset relation.

4. **Divergence is caught**
   - Given a checked-in self-manifest perturbed by adding an absorbed import
     path or dropping a member
   - When the parity test runs
   - Then it fails, naming the component and the field that diverged.

5. **Implicit interface membership is stated, not stumbled into**
   - Given a component whose generated member set includes its interface package
   - When the comparison runs
   - Then the relation between the two member sets is applied from a single
     documented rule, and a genuinely missing member still fails.

6. **The test is inert outside Bazel**
   - Given `cd go && go test ./...`
   - When the parity tests run without arguments
   - Then they skip rather than fail.

7. **Repository checks pass**
   - Given the new test and the extracted helper
   - When `just ci` runs
   - Then every leg is green.

## Metadata
- **Complexity**: Medium
- **Labels**: Bazel, dogfooding, parity, testing, refactor
- **Required Skills**: Go testing, Bazel test wiring, gazelle conventions
