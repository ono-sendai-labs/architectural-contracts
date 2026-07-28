# Task: Declare the analysis arcc self-components in Bazel

## Description
Declare `go_component` targets for `checker`, `goanalysis` and
`capslockadapter` — the three components that carry arcc's real absorption and
authority declarations — and bring their checked-in manifests onto the declared-
membership model. These are the components whose Bazel `.check` would have
caught the `hostpolicy` manifest breakage. `cli` is deliberately not here: it is
the repo's multi-package case and gets its own task.

## Background
Task 1 declared the four lower components. This task adds the layer above them.
Unlike the leaves, these three have substance to get right:

- `goanalysis` absorbs two in-repo seams (`hostpolicy`, `packagelayout`) and
  five `golang.org/x/tools` packages, and declares seven authorities.
- `capslockadapter` absorbs the two capslock packages plus `packagelayout` and
  `x/tools/go/packages`, and declares eight authorities.
- `checker` is authority-free and depends on four sibling components — which is
  what makes it the right fixture for Task 5's negative test.

`hostpolicy` and `packagelayout` are absorbed by more than one component. That
is allowed: M7 checks membership overlap *locally* (a member of this component
may not also be covered by one of its resolved `component_dep`s or listed in its
`absorbed_dependencies`), and overlap between unrelated components is a
documented limitation, not an error. Absorption keeps transitive-closure
semantics (A4), so absorbing `//go/internal/packagelayout` also accounts for
what it pulls in.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M1/M4/M5/M6/M7, A4, B3; §7.3 "The bazelified self-check")
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 10)

**Additional References (if relevant to this task):**
- `go/examples/csvtool/csvfile/BUILD.bazel` — the paired declaration/manifest idiom, including the explanatory comment above the `go_component`.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a `go_component` target to `go/internal/checker/BUILD.bazel`,
   `go/internal/goanalysis/BUILD.bazel` and
   `go/internal/capslockadapter/BUILD.bazel`, named `<component>_component`,
   loaded from `@rules_arcc//bazel_rules/go:defs.bzl`, with
   `visibility = ["//go:__subpackages__"]`.
2. Mirror each checked-in manifest exactly:
   - `checker_component`: `interface = ":checker"`, `component_deps` on the
     `capanalyzer`, `facts`, `manifest` and `report` components. No absorbed
     deps, no declared authority.
   - `goanalysis_component`: `interface = ":goanalysis"`, `component_deps` on
     the `capanalyzer`, `facts` and `manifest` components; `absorbed_deps` on
     `//go/internal/hostpolicy`, `//go/internal/packagelayout`,
     `@org_golang_x_tools//go/callgraph`,
     `@org_golang_x_tools//go/callgraph/vta`,
     `@org_golang_x_tools//go/packages`, `@org_golang_x_tools//go/ssa` and
     `@org_golang_x_tools//go/ssa/ssautil`; `declared_authority = [FILES, EXEC,
     READ_SYSTEM_STATE, OPERATING_SYSTEM, REFLECT, UNSAFE_POINTER,
     MODIFY_SYSTEM_STATE]`.
   - `capslockadapter_component`: `interface = ":capslockadapter"`,
     `component_deps` on the `capanalyzer` component; `absorbed_deps` on
     `@com_github_google_capslock//analyzer`,
     `@com_github_google_capslock//interesting`,
     `//go/internal/packagelayout` and `@org_golang_x_tools//go/packages`;
     `declared_authority = [FILES, EXEC, READ_SYSTEM_STATE, OPERATING_SYSTEM,
     REFLECT, RUNTIME, SYSTEM_CALLS, UNSAFE_POINTER]`.
3. Add `exports_files(["component.textproto"])` to each of the three packages.
4. Add a `members:` entry naming the component's own package import path to each
   of the three checked-in `component.textproto` files, matching what the
   emitter produces for a single-package component.
5. Keep the absorbed sets in the BUILD declaration and the checked-in manifest
   in one-to-one correspondence, including the two in-repo seams. The
   `hostpolicy` regression is the reason this step exists; a Bazel declaration
   that quietly omits it would recreate the gap on the other side.
6. Keep `just selfcheck` green and `TestSelfHostingManifests` (which pins
   `checker`'s name, interface files and dependency set) passing.
7. Write a short comment above each `go_component` saying what it mirrors and
   why it absorbs what it absorbs — the `reason` text in the checked-in manifest
   is the source for that comment, since the rule emits no reasons.
8. Do not modify `justfile`, `README.md` or the Bazel rules.

## Dependencies
- `task-01-declare-core-self-components` provides the `capanalyzer`, `report`,
  `facts` and `manifest` component targets that these three name in
  `component_deps`.
- `task-03-declare-cli-self-component` depends on all three targets added here.
- `task-05-self-check-dependency-regression` uses `checker_component` as the
  model for its deliberately broken fixture.

## Implementation Approach
1. Declare `checker_component` first — no absorption, no authority — and get its
   `.check` green; it validates that the four Task 1 components are visible and
   correctly covered.
2. Add `capslockadapter_component`, then `goanalysis_component`, which has the
   largest closure and the slowest check.
3. Update the three checked-in manifests alongside the declarations.
4. When a check reports an undeclared dependency or unexpected authority, treat
   the finding as data: confirm whether the closure genuinely reaches it, then
   correct both the declaration and the checked-in manifest. Escalate rather
   than paper over a finding the native leg does not report.
5. Run `bazel test //go/internal/...`, then `just selfcheck`, then `just ci`.

## Acceptance Criteria

1. **Three components are declared**
   - Given `go/internal/{checker,goanalysis,capslockadapter}/BUILD.bazel`
   - When they are inspected
   - Then each declares a `<name>_component` with the component dependencies,
     absorbed dependencies and declared authority of its checked-in manifest.

2. **Their checks pass unmarked**
   - Given the three declarations
   - When `bazel test //go/internal/...` runs
   - Then `checker_component.check`, `goanalysis_component.check` and
     `capslockadapter_component.check` all pass and none is tagged `manual`.

3. **In-repo seams stay declared**
   - Given `goanalysis_component` and `capslockadapter_component`
   - When their generated manifests are read
   - Then `//go/internal/hostpolicy` and `//go/internal/packagelayout` appear as
     absorbed import paths exactly where the checked-in manifests list them.

4. **Membership is stated**
   - Given the three checked-in manifests
   - When they are parsed
   - Then each names its own package import path in `members`, and that set
     equals the member set of the corresponding generated manifest.

5. **The native leg is unchanged**
   - Given the updated manifests
   - When `just selfcheck` and `cd go && go test ./...` run
   - Then all eight components still conform and the Go tests pass.

6. **Repository checks pass**
   - Given the declarations and manifest edits
   - When `just ci` runs
   - Then every leg is green.

## Metadata
- **Complexity**: Medium
- **Labels**: Bazel, dogfooding, self-check, absorption, authority
- **Required Skills**: Starlark, Bazel rules, arcc component modelling
