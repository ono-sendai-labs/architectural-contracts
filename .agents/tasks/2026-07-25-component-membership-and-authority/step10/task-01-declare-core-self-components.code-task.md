# Task: Declare the core arcc self-components in Bazel

## Description
Give arcc's four lower components — `capanalyzer`, `report`, `facts` and
`manifest` — `go_component` declarations, so `bazel test //...` runs their
`.check` targets, and bring their checked-in manifests onto the declared-
membership model the Bazel emitter now produces. `manifest` is the first of the
repo's two multi-package components: its generated protobuf package
`//go/internal/manifest/gen` becomes a declared **member**, not an interface
file.

## Background
arcc's own dogfooding currently exists only as `just selfcheck`, which shells
out to a natively built `bin/arcc` and checks the eight colocated
`component.textproto` files. A Bazel-only environment runs none of it. That is
the exact gap that let the `hostpolicy` manifest breakage through: a manifest
that no longer described the code, with no test in the Bazel leg to say so
(design §2 B3, plan Step 10).

This step closes it by declaring arcc's components with the same
`go_component` macro the csvtool example uses, one dependency-closed batch at a
time. This task takes the batch that depends on nothing else in the repo:

- `capanalyzer` and `report` are leaves.
- `facts` depends on `capanalyzer` and `manifest`.
- `manifest` absorbs three protobuf packages and owns the generated `gen`
  package.

`manifest`'s checked-in manifest lists `gen/component.pb.go` as an *interface
file* even though `gen` is a separate Go package. That was the pre-membership
way of saying "this package is mine too". Under the declared model the honest
statement is membership: the Bazel interface is exactly the srcs of the
interface `go_library` (`manifest.go`), and `gen` is a member. Nothing outside
`//go/internal/manifest` imports `gen` (only `manifest.go` and
`internal/manifest/schema_test.go` do), so this loses no one's access to a
declared surface.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M1/M4/M5/M6/M7, B3; §7.3 "The bazelified self-check")
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 10)

**Additional References (if relevant to this task):**
- `.agents/tasks/2026-07-25-component-membership-and-authority/step07/task-03-migrate-csvtool-membership.code-task.md` — the migration this one mirrors, one repo inward.
- `go/examples/csvtool/csvfile/BUILD.bazel` and `go/examples/csvtool/csvfile/component.textproto` — the paired declaration/manifest idiom to copy, including the explanatory comment above the `go_component`.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a `go_component` target to each of `go/internal/capanalyzer/BUILD.bazel`,
   `go/internal/report/BUILD.bazel`, `go/internal/facts/BUILD.bazel` and
   `go/internal/manifest/BUILD.bazel`, loaded from
   `@rules_arcc//bazel_rules/go:defs.bzl`. Name each target
   `<component>_component`, so the `_component` suffix the parity comparison
   normalizes away is the only difference from the manifest `name`.
2. Declare each one to mirror its checked-in manifest exactly:
   - `capanalyzer_component`: `interface = ":capanalyzer"`, nothing else.
   - `report_component`: `interface = ":report"`, nothing else.
   - `facts_component`: `interface = ":facts"`, `component_deps` on
     `//go/internal/capanalyzer:capanalyzer_component` and
     `//go/internal/manifest:manifest_component`.
   - `manifest_component`: `interface = ":manifest"`,
     `members = ["//go/internal/manifest/gen"]`, `absorbed_deps` on
     `@org_golang_google_protobuf//encoding/prototext`,
     `@org_golang_google_protobuf//reflect/protoreflect` and
     `@org_golang_google_protobuf//runtime/protoimpl`, and
     `declared_authority = [READ_SYSTEM_STATE, REFLECT, RUNTIME, SYSTEM_CALLS,
     UNSAFE_POINTER]`.
3. Give each component target `visibility = ["//go:__subpackages__"]`, matching
   the visibility of the library it wraps. Later tasks in this step consume them
   from `//go/cmd/arcc` and `//go/cmd/arcc/app`, both inside `//go`.
4. Add `exports_files(["component.textproto"])` to each of the four packages, so
   Task 4's parity test can name the checked-in manifest as data.
5. Update the four checked-in manifests to the declared-membership model, using
   the csvtool manifests as the template:
   - each gains a `members:` entry naming its own package import path
     (`github.com/ono-sendai-labs/architectural-contracts/go/internal/<name>`);
   - `go/internal/manifest/component.textproto` additionally gains
     `members: ".../go/internal/manifest/gen"` and **loses**
     `interface_files: "gen/component.pb.go"`.
6. Keep the native leg green and unchanged in meaning: `just selfcheck` must
   still pass for all eight components, and
   `TestSelfHostingManifests` in `go/internal/manifest/manifest_test.go` — which
   asserts the name, interface files and dependencies of `capanalyzer`, `facts`,
   `report` and `checker` — must still pass.
7. Write a short comment above each `go_component` explaining what it mirrors,
   in the style of the csvtool declarations. For `manifest_component`, say why
   `gen` is a member rather than an interface file.
8. Do not add `own_check_runs` or `certification_reference` to any manifest in
   this step; the certification self-declarations are out of scope.
9. Do not modify `justfile`, `README.md`, or the Bazel rules themselves. This
   task only declares components with the existing macro.

## Dependencies
- Steps 1–9 are complete: `members`, `interface_style`, the emitter's expanded
  membership, `roots == members` validation and package-granularity pruning are
  all in place.
- Tasks 2 and 3 of this step add the remaining components and depend on the
  targets introduced here; Task 4's parity test depends on all eight plus the
  `exports_files` added here.

## Implementation Approach
1. Start with `capanalyzer` and `report`, the two leaves; run
   `bazel test //go/internal/capanalyzer:all //go/internal/report:all` and
   confirm the generated `.check` targets pass.
2. Add `manifest_component` with its `gen` member and protobuf absorption, then
   `facts_component` on top of it.
3. Update the four checked-in manifests in the same commit as the declarations
   they mirror, then run `just selfcheck` and `cd go && go test ./...`.
4. If a `.check` reports authority beyond the declared set, read the finding
   before touching a declaration: it is either a genuine difference in the
   analyzed package set (Bazel analyzes real dependency sources from the layout;
   the native leg loads via `go list`) or a real gap in the hand-written
   manifest. Fix both the BUILD declaration and the checked-in manifest
   together so they keep saying the same thing. Escalate rather than silence a
   finding the native leg does not report — that disagreement is precisely the
   cross-check this step exists to create.
5. Run `just ci` last; it runs both legs.

## Acceptance Criteria

1. **Four components are declared**
   - Given `go/internal/{capanalyzer,report,facts,manifest}/BUILD.bazel`
   - When `bazel query 'kind(rule, //go/internal/...)'` is inspected
   - Then each package has a `<name>_component` target and the macro-generated
     `<name>_component.check` test.

2. **Their checks pass under Bazel**
   - Given the four declarations
   - When `bazel test //go/internal/capanalyzer/... //go/internal/report/...
     //go/internal/facts/... //go/internal/manifest/...` runs
   - Then every `.check` target passes without being tagged `manual`, so a plain
     `bazel test //...` runs them.

3. **`gen` is a member of the manifest component**
   - Given `manifest_component`
   - When its generated manifest is read
   - Then it lists both `.../internal/manifest` and `.../internal/manifest/gen`
     as members, its interface files are exactly the srcs of
     `//go/internal/manifest:manifest`, and the layout's roots equal that member
     set with no `roots`/`members` mismatch error.

4. **The checked-in manifests agree**
   - Given the four updated `component.textproto` files
   - When each is compared with its generated counterpart by hand
   - Then member sets, absorbed import paths, declared authority and
     component-dependency names match, and `gen/component.pb.go` no longer
     appears as an interface file.

5. **The native leg is unchanged**
   - Given the updated checked-in manifests
   - When `just selfcheck` runs
   - Then all eight components still conform, with no new findings.

6. **Repository checks pass**
   - Given the declarations and manifest edits
   - When `just ci` runs
   - Then generation-cleanliness, lint, unit, integration, selfcheck and Bazel
     legs are all green.

## Metadata
- **Complexity**: Medium
- **Labels**: Bazel, dogfooding, self-check, members, manifests
- **Required Skills**: Starlark, Bazel rules, arcc component modelling
