# Task: Manifest protobuf schema & codegen wiring

## Description
Define the shared, language-neutral manifest schema as a protobuf file and wire the
real `gen` codegen target so the generated Go code is committed and the CI
`gen`-is-clean check becomes meaningful. This is the data-model foundation that the
parser (task-02) and the checker (Step 4) consume.

## Background
The manifest is the on-disk `component.textproto` at each component's root. Its fields
must stay plain/mechanical so a future Bazel rule can populate them (NFR3), and the
schema lives in the **top-level `proto/`** dir (outside `go/`) so a future Rust
toolchain can share it (design §9). Step 1 stubbed `gen` as a no-op placeholder in the
`justfile`; this task makes it real.

Per the PR review the schema is deliberately trimmed: **no `packages` field**
(membership = all packages under the manifest's directory, FR1), **no `contract_note`**
(the contract is doc-comment prose in interface files, FR10), **no `interface_packages`**
(a dependency's interface packages are derived from its own manifest, review C10), and
`AbsorbedDependency.reason` is `optional`.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§6.1 Manifest schema, §9 Repository layout)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 2)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Create `proto/archcontracts/v1/component.proto` (`syntax = "proto3"`, `package archcontracts.v1`) with exactly the messages/fields from design §6.1:
   - `Component`: `string name = 1`; `repeated string interface_files = 2`; `repeated ComponentDependency component_dependencies = 3`; `repeated AbsorbedDependency absorbed_dependencies = 4`; `repeated string declared_authority = 5`.
   - `ComponentDependency`: `string name = 1`; `string manifest = 2`.
   - `AbsorbedDependency`: `string import_path = 1`; `optional string reason = 2`.
   - Carry the field-semantics comments from §6.1 (paths relative to the component root; `manifest` relative to the declaring manifest's directory; no `packages`/`contract_note`/`interface_packages`).
   - Set an appropriate `option go_package` pointing at the committed gen package (e.g. `.../go/internal/manifest/gen;gen`).
2. Wire the real **`gen`** justfile target to protobuf codegen (`protoc` / `buf` with `protoc-gen-go`), replacing the Step-1 no-op placeholder. The generated Go must be **committed** under `go/internal/manifest/gen/` (design §9 — chosen so the build needs no `protoc`).
3. The CI `gen`-is-clean check (already part of `just ci` from Step 1) must verify the committed generated code matches the proto (regenerate + `git`/`jj` diff is empty).
4. Keep `proto/` **outside** the `go/` module.

## Dependencies
- Step 1 scaffold: existing `justfile` (with placeholder `gen`), `go/` module, CI workflow.

## Implementation Approach
1. Author `proto/archcontracts/v1/component.proto` per §6.1, including the trims and the `go_package` option.
2. Choose the codegen toolchain and pin plugin versions; document the required tools (the CI workflow installs `protoc`/plugins as noted in Step 1).
3. Implement the `gen` recipe so it regenerates into `go/internal/manifest/gen/` deterministically; commit the generated output.
4. Ensure the `gen`-is-clean check in `just ci` fails when the committed code drifts from the proto.
5. Confirm `just build` / `just gen` succeed and the generated package compiles.

## Acceptance Criteria

1. **Schema matches the design**
   - Given the design §6.1 schema
   - When `proto/archcontracts/v1/component.proto` is inspected
   - Then it declares `Component`, `ComponentDependency`, `AbsorbedDependency` with exactly the listed fields/numbers, `optional reason`, and no `packages`/`contract_note`/`interface_packages` fields.

2. **Codegen is real and committed**
   - Given a clean checkout
   - When `just gen` runs
   - Then generated Go is produced under `go/internal/manifest/gen/`, the package compiles under `just build`, and re-running `gen` produces no diff.

3. **gen-is-clean guards drift**
   - Given a manual edit to the proto without regenerating
   - When `just ci` (or its gen-clean check) runs
   - Then it fails because the committed generated code no longer matches the proto.

4. **Proto stays module-neutral**
   - Given the repo layout
   - When `proto/` is located
   - Then it sits at the top level, outside the `go/` module, sharable by a future toolchain.

## Metadata
- **Complexity**: Medium
- **Labels**: proto, codegen, tooling, data-model, ci
- **Required Skills**: Protobuf, Go, build tooling (just/protoc)
