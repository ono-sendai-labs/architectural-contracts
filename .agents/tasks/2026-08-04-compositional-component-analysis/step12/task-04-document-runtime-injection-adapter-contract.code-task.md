# Task: Document the Runtime-Injection Adapter Contract

## Description
Update the public Bazel porting documentation so it accurately teaches the
runtime-injection hooks delivered by Step 12 and the current infra attachment
callback shape.

## Background
Addresses finding F1 from the Step 12 implementation review. The runtime hook
implementation and tests are complete, but README.md currently announces
runtime hooks without naming `runtime_injection_attrs(deps_aspect)` or
`extra_runtime_packages(ctx, root_packages)`, and it still describes
`go_attach_infra` as a two-argument callback over one target. A host porter needs
the complete contract to replace hidden runtime wiring without patching generic
component rules.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step12.yaml`

**Additional References:**
- Plan Step 12: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Host import friction: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md`
- Implemented contract: `bazel_rules/go/private/go_adapter.bzl`
- Generic consumer: `bazel_rules/go/private/component.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `runtime_injection_attrs(deps_aspect)` to README.md's host-adapter porting surface, including its empty `{}` upstream default, private-attribute requirement, dependency-aspect purpose, and collision-checked composition into the component rule.
2. Add `extra_runtime_packages(ctx, root_packages)` to the same section, including its empty `[]` upstream default, host-neutral package-record shape, read-only ordinary-package input, and deterministic-output requirement.
3. Explain that ordinary and injected records are merged once before cgo validation, infra attachment, membership classification, layout/import projection, and export staging.
4. Correct the documented `go_attach_infra` contract to use the component roots collection and optional merged `package_view`; explain how that view lets the existing infra boundary select a hidden runtime.
5. State explicitly that empty defaults add no author-facing attributes, package records, analysis actions, or action inputs upstream.
6. Remove or correct stale step references in the adjacent adapter comments or public documentation when they describe the same runtime-injection and infra-attachment contract.

## Dependencies
- Step 12 Tasks 1-3 provide the finalized labels, signatures, ordering, and SDK boundaries that the documentation must describe.
- No code behavior changes are required or permitted by this documentation task.

## Implementation Approach
1. Reconcile README.md's porting section with the function contracts and call ordering in `go_adapter.bzl` and `component.bzl`.
2. Add compact bullets for the two runtime hooks and revise the infra predicate text without duplicating implementation details unrelated to host porting.
3. Search the adjacent public and adapter documentation for the superseded callback spelling and stale step references, correcting only statements about this contract.
4. Run documentation-oriented searches and the repository validation gate to ensure the edit introduces no stale symbol names or formatting regressions.

## Acceptance Criteria

1. **Both runtime-injection hooks are discoverable**
   - Given README.md's Go-ruleset porting section
   - When a host integrator reads the documented adapter surface
   - Then `runtime_injection_attrs(deps_aspect)` and `extra_runtime_packages(ctx, root_packages)` are named with their inputs, outputs, and empty upstream defaults.

2. **The ordering and boundary reuse are accurate**
   - Given the public description of injected package handling
   - When it is compared with `component.bzl`
   - Then it states that ordinary and injected records share one deterministic merge before validation, infra selection, membership/layout construction, and export staging, and that the existing auto-attached component boundary is reused.

3. **The attachment callback matches the implementation**
   - Given the README and adapter documentation
   - When `go_attach_infra` is described
   - Then the roots collection and optional merged package view match the implemented callback, with no remaining two-argument single-target description.

4. **Upstream inertness is explicit**
   - Given the default upstream adapter
   - When the documentation describes the empty runtime hooks
   - Then it makes clear that they add no attributes, package records, actions, action inputs, artifacts, or verdict changes.

5. **The remediation is documentation-only and internally consistent**
   - Given the completed change
   - When its diff and repository validation are inspected
   - Then no implementation or test behavior changed, the relevant symbol/signature searches find no stale public wording, and `just ci` passes.

## Metadata
- **Complexity**: Low
- **Labels**: documentation, bazel, starlark, adapter, runtime
- **Required Skills**: Technical documentation, Bazel/Starlark API reading, repository validation
