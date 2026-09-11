# Task: Add Runtime Injection Adapter Hooks

## Description
Add inert upstream hooks through which a host can expose toolchain-injected runtime packages to component analysis. Feed those packages into the existing package closure and infra auto-attachment path so a synthetic injected runtime is type-loadable, owned by its infra component, and accepted without an authored component dependency.

## Background
Imports are analysis edges, including imports introduced by generated or toolchain-transformed code. A host-injected runtime may therefore appear in the compiled package graph even though the component author never named it. Reporting an undeclared dependency on such a hidden package is the wrong authoring contract. Design R13 and Q14 resolve the common case by using the existing infra-component boundary: the runtime has one package-surface wrapper (typically `authority: UNKNOWN` when it is unowned), and the edge is recorded as auto-attached rather than authored.

The missing piece is host visibility. Step 12 specifies two adapter APIs: `runtime_injection_attrs(deps_aspect)`, which lets the adapter declare whatever hidden dependencies its Go ruleset exposes, and `extra_runtime_packages(ctx, root_packages)`, which projects those dependencies into arcc's host-neutral package records. Both defaults are empty upstream. The generic aspect/component layers must consume only this contract and continue to avoid direct `@rules_go` provider access.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R1, R13, R14, N1–N2, §Build topology, §Host adapter contract)

**Additional References:**
- Plan Step 12: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Runtime classification decision Q14: `.agents/planning/2026-08-04-compositional-component-analysis/idea-honing.md`
- Host import friction: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md` (§3 injected runtime)
- Existing seams: `bazel_rules/go/private/go_adapter.bzl`, `bazel_rules/go/private/aspect.bzl`, `bazel_rules/go/private/component.bzl`, and `bazel_rules/go/tests/testing.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Define `runtime_injection_attrs(deps_aspect)` in `go_adapter.bzl`. It must return a dictionary of adapter-owned private rule attributes needed to expose injected runtime targets, with `{}` as the upstream rules_go default. The supplied dependency aspect must be applicable by a host implementation without requiring upper layers to load host-specific providers.
2. Define `extra_runtime_packages(ctx, root_packages)` in `go_adapter.bzl`. It must return host-neutral package records compatible with the `ArccPackageInfo`/`merge_by_importpath` contract, with `[]` as the upstream default. Document whether the input is mutated (it must not be) and require deterministic output or deterministic merging.
3. Merge `runtime_injection_attrs(arcc_deps_aspect)` into the component rule's attributes with explicit collision protection. Keep the new attributes private/non-author-facing upstream and retain `go_adapter.bzl` as the only file that a host must replace or patch for its Go provider shape.
4. Invoke `extra_runtime_packages` after collecting the roots' ordinary package records and include its result in the same import-path merge, cgo validation, closure/layout construction, member/frontier classification, export-data staging, and determinism rules as other non-member packages. Do not create a second component-analysis action or pass undeclared files to `ArccCheck`.
5. Make the augmented package view visible to infra attachment selection. A matching injected package must cause the existing infra component to be auto-attached before membership classification and dependency-artifact binding, so its packages are covered rather than absorbed as members or reported as undeclared.
6. Preserve edge provenance: the generated manifest/layout must record the runtime wrapper as `auto_attached: true`; its surface/report inputs must come from `ArccComponentInfo`; it remains exempt from `UNUSED_DEPENDENCY` only through the existing auto-attached fact, not through a new checker special case.
7. Keep the upstream defaults behaviorally inert. With `{}` and `[]`, rule attributes, package closure, action graph, action inputs, emitted manifests/layouts, reports, surfaces, and verdicts must match the pre-hook behavior.
8. Extend the test-only component-rule injection seam to supply fake runtime attributes/packages without adding test controls to the public production macro. Construct a synthetic runtime package absent from the ordinary root package projection and a matching infra package-surface component.
9. Add analysis and end-to-end tests proving that the synthetic runtime appears in the final layout/type-loading graph, selects and auto-attaches the infra component, contributes the required export/metadata inputs, and lets the component check pass without an authored `component_deps` entry or authority declaration for the runtime package.
10. Add negative/fail-closed coverage for malformed or conflicting injected records (including duplicate import paths with different export artifacts and unsupported cgo) using the existing merge and validation paths rather than silently selecting one record.

## Dependencies
- Task 1: Use Label Identity for Infra Self-Exemption ensures exposing injected packages cannot make an infra wrapper auto-attach itself through a short-name workaround.
- Step 7 supplies infra auto-attachment, dependency surface/report bindings, overlap detection, and auto-attached provenance.
- Step 8 supplies export-backed non-member type loading and the exact `ArccCheck` action-input allowlist; injected packages must participate without reintroducing dependency source into that action.
- Step 11 supplies package-level `UNKNOWN` wrappers and visible untrusted boundaries for unowned runtimes.

## Implementation Approach
1. Add and document the two empty-default adapter functions, then merge their attribute contract into `GO_COMPONENT_ATTRS` with collision checks.
2. Refactor component construction so the ordinary root package records and adapter-injected records form one deterministic effective package view before infra selection and ownership classification.
3. Extend the existing test-only dependency-injection functions to return one synthetic package record and registry match while leaving the production rule's public authoring surface unchanged.
4. Assert the generated manifest, effective layout, action inputs, report, and surface boundary end to end, then run the full repository gate with the upstream empty defaults.

## Acceptance Criteria

1. **Empty upstream hooks are inert**
   - Given the default rules_go adapter returns no runtime attributes or extra packages
   - When every existing component is analyzed and checked
   - Then its rule shape, declared actions/inputs, canonical artifacts, and verdict are unchanged.

2. **Injected runtime packages enter the effective layout**
   - Given a fake host adapter exposing a synthetic runtime package that is absent from the ordinary root package records
   - When a component is analyzed
   - Then the package is present once in the merged layout/closure with deterministic metadata and the export material needed for non-member type loading.

3. **The existing infra boundary claims the runtime**
   - Given the fake injected package matches a registered infra package-surface component
   - When attachment and component membership are classified
   - Then that component is auto-attached, the runtime package is covered by its surface, and it is neither made a member nor reported as an undeclared dependency.

4. **No author declaration is required for hidden runtime wiring**
   - Given member code that type-checks only when the synthetic injected runtime is available
   - When the component is checked without naming that runtime in `component_deps`, members, or declared authority
   - Then the check passes, the manifest/layout record an auto-attached dependency edge, and the report records the infra boundary with its actual checked/asserted authority and provenance axes.

5. **Injected inputs obey the analysis-action allowlist**
   - Given the synthetic runtime check action
   - When its declared inputs are inspected
   - Then it contains member sources, the final manifest/layout, required export data, dependency surfaces/reports, and the stdlib map, but no runtime/dependency Go source or host toolchain binary.

6. **Invalid injected metadata fails closed**
   - Given an injected cgo package or injected/ordinary records that conflict for one import path
   - When the component is analyzed
   - Then analysis fails with an actionable component/package diagnostic instead of selecting data by iteration order.

7. **Determinism and integration remain green**
   - Given reordered injected package and registry inputs
   - When artifacts are produced repeatedly and `just ci` runs
   - Then attachment, layout records, report/surface bytes, and verdicts are identical and all existing self-checks pass.

## Metadata
- **Complexity**: High
- **Labels**: bazel, starlark, adapter, runtime, infrastructure, export-data, hermeticity
- **Required Skills**: Bazel aspects and rule attributes, Starlark dependency injection, provider projection, infra boundary modeling, hermetic action-input testing
