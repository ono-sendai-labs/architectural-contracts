# Task: Declare the cli self-component across `cmd/arcc` and `app`

> ## ⛔ VOID — do not implement
>
> **This task is superseded and has no work.** `cli` is excluded from the Bazel
> self-check leg, for the same reason `capslockadapter` is: `go/cmd/arcc/main.go`
> imports `//go/internal/capslockadapter`, whose closure reaches
> `golang.org/x/sys/unix` built with cgo, and `component.bzl` fails closed on cgo
> closures because a cgo package's preprocessed sources do not exist at analysis
> time. A `cli_component` therefore cannot pass its check, and the rule is right
> to refuse.
>
> Established by an escalation on the first attempt at this task and decided by
> the user; recorded in the interposed spec commit alongside the parallel change
> to tasks 04 and 05. The Bazel leg covers **six** components; `just selfcheck`
> still covers all **eight**, so `cli` keeps full native coverage and only its
> Bazel leg is lost.
>
> The `members` coverage this task was meant to provide under Bazel is not lost
> either: `manifest` is a second multi-package component (its `gen` package) and
> is already declared and checked.
>
> The migration of `go/cmd/arcc/component.textproto` off cross-package
> `interface_files` is **not** performed — it was only required to satisfy the
> Bazel model, which no longer applies to this component. Native FR1 behavior is
> unchanged.
>
> Everything below is retained for the record and describes work that will not
> happen.

## Description
Declare the eighth and last arcc self-component, `cli`, which spans
`//go/cmd/arcc` and its `app` subpackage. This is the repository's own
multi-package component and the one that exercises `members` end to end: `app`
stops being an interface file of a foreign package and becomes a declared member
of the component whose interface is `main.go`.

## Background
`go/cmd/arcc/component.textproto` currently declares two interface files in two
different Go packages — `main.go` and `app/app.go`. That is the pre-membership
idiom for "this package is mine too", and it has no Bazel expression: the
generated interface set is exactly the srcs of the one interface `go_library`.
Declared membership is the model that does express it (M1, M2, M5), and the plan
names `cli` as the case that proves it on real code.

The migration is therefore not cosmetic. After it, `app`'s exported symbols are
no longer part of `cli`'s declared interface — they are member code, analyzed as
roots (A1) with their authority charged to `cli` regardless of who calls them.
`main.go` calling into `app` is an intra-component reference and unaffected.
Nothing outside the component imports `go/cmd/arcc/app`.

`cli` also depends on all seven other components and absorbs the two in-repo
seams, so its check is the composition-root check: it is the one that would have
failed loudly on the `hostpolicy` manifest breakage.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M1/M2/M4/M5/M6, A1, B3; §7.3 "The bazelified self-check" — `cli` named as the multi-package case)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 10)

**Additional References (if relevant to this task):**
- `bazel_rules/go/tests/testdata/membercomponent/BUILD.bazel` — the `members` authoring form (literal labels; the interface package is implicit and not listed).
- `bazel_rules/go/tests/goldens/member_component.component.textproto` — what the emitter writes for a multi-member component: fully expanded import paths, interface package included.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a `go_component` named `cli_component` to `go/cmd/arcc/BUILD.bazel` with
   `interface = ":arcc_lib"` and `members = ["//go/cmd/arcc/app"]`. Name it
   `cli_component`, not `arcc_component`: the parity comparison normalizes only
   the `_component` suffix, and the manifest's `name` is `cli`.
2. Declare its edges to mirror the checked-in manifest: `component_deps` on the
   `capanalyzer`, `capslockadapter`, `checker`, `facts`, `goanalysis`,
   `manifest` and `report` components; `absorbed_deps` on
   `//go/internal/hostpolicy` and `//go/internal/packagelayout`;
   `declared_authority = [FILES, REFLECT, READ_SYSTEM_STATE, UNSAFE_POINTER,
   MODIFY_SYSTEM_STATE, EXEC]`.
3. Give it `visibility = ["//go:__subpackages__"]` and add
   `exports_files(["component.textproto"])` to `go/cmd/arcc/BUILD.bazel`.
4. Migrate `go/cmd/arcc/component.textproto`:
   - remove `interface_files: "app/app.go"`, leaving `main.go` as the sole
     interface file;
   - add `members:` entries for
     `github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc` and
     `github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app`,
     matching the emitter's fully expanded form.
5. Verify the generated layout's roots equal that member set — `cli` is the
   component where an M6 mismatch is most likely, because it is the only one
   with a member outside the interface package.
6. Keep the native leg green: `just selfcheck`'s `cmd/arcc` check must still
   conform with the migrated manifest, and `cd go && go test ./...` — including
   `go/cmd/arcc/cli_integration_test.go` and `go/cmd/arcc/app/app_test.go` —
   must pass.
7. If dropping `app/app.go` from the interface produces a new native finding, do
   not restore it. Report what the finding is and escalate: an FR4-family rule
   that fires on member code would mean the rule is still interface-scoped in
   name only, which is a design question, not a manifest question.
8. Comment the declaration with what makes it different from the other seven —
   that it is the repo's own multi-package component and its `app` member is why
   `members` exists.
9. Do not modify `justfile`, `README.md` or the Bazel rules.

## Dependencies
- `task-01-declare-core-self-components` and
  `task-02-declare-analysis-self-components` provide the seven component targets
  `cli_component` names in `component_deps`.
- `task-04-self-manifest-parity-test` consumes `cli_component` and the migrated
  checked-in manifest; the `_component`-suffix and member normalizations it
  needs are settled here.

## Implementation Approach
1. Add `cli_component` mirroring the current checked-in manifest, and build it
   before checking it: confirm the generated manifest lists both member import
   paths and that its interface files are `main.go` only.
2. Run `bazel test //go/cmd/arcc:cli_component.check`; resolve findings by
   correcting the declaration and the checked-in manifest together.
3. Migrate `go/cmd/arcc/component.textproto` and run `just selfcheck` to confirm
   the native leg agrees with the Bazel leg about what `cli` owns.
4. Run the Go test suite, then `just ci`.

## Acceptance Criteria

1. **The cli component is declared**
   - Given `go/cmd/arcc/BUILD.bazel`
   - When it is inspected
   - Then `cli_component` declares `//go/cmd/arcc/app` as a member,
     `:arcc_lib` as its interface, seven component dependencies and two absorbed
     dependencies.

2. **Membership spans both packages**
   - Given the generated `cli_component.component.textproto`
   - When it is parsed
   - Then its members are exactly the `go/cmd/arcc` and `go/cmd/arcc/app` import
     paths, and its interface files are exactly the srcs of `:arcc_lib`.

3. **Roots equal members**
   - Given the generated manifest and package layout
   - When `arcc check` loads them
   - Then the layout roots equal the declared member set and no mismatch error
     is raised.

4. **The check passes under Bazel**
   - Given `cli_component`
   - When `bazel test //...` runs
   - Then `cli_component.check` passes and is not tagged `manual`, making all
     eight self-checks part of the Bazel leg.

5. **The checked-in manifest is migrated**
   - Given `go/cmd/arcc/component.textproto`
   - When it is read
   - Then `app/app.go` is no longer an interface file and both member import
     paths are declared.

6. **The native leg still conforms**
   - Given the migrated manifest
   - When `just selfcheck` runs
   - Then `cmd/arcc/component.textproto` conforms with no new findings, and the
     other seven checks are unaffected.

7. **Repository checks pass**
   - Given the declaration and the migration
   - When `just ci` runs
   - Then every leg is green, including the CLI integration tests.

## Metadata
- **Complexity**: Medium
- **Labels**: Bazel, dogfooding, self-check, members, multi-package, migration
- **Required Skills**: Starlark, Bazel rules, arcc component modelling, Go
