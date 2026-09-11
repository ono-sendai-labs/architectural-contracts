# Task: Formalize SDK Adapter Seams

## Description
Separate and formalize the three host-replaceable SDK contracts used by the post-redesign Bazel topology: SDK source enumeration for stdlib-map generation, SDK export-data enumeration for component analysis, and exact target platform/SDK-key discovery. Preserve the current rules_go behavior as the upstream default and prove that a fake export-data adapter controls the checked action's inputs.

## Background
Earlier steps implemented the required mechanics directly in `go_adapter.bzl`: source-backed map generation reads the resolved SDK source depset and package list; checked components consume `GoStdLib` metadata and compiled exports; layouts and map providers derive target GOOS/GOARCH, cgo, build tags, toolchain version, and GOEXPERIMENT. Step 12 turns those implementations into an explicit host adapter contract so a host can replace its provider-specific discovery in one file without patching `component.bzl` or `stdlib_map.bzl`.

The three roles must remain separate. Standard-library source is an input only to `ArccStdlibMap`; it must never return to `ArccCheck`. Compiled stdlib metadata/export files are analysis inputs and must describe the same target configuration as the selected authority map. Target/key discovery is shared identity, not a heuristic assembled differently at each call site. Design I5 continues to forbid toolchain binaries, host caches, and network access in both actions.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N2–N3, I3–I5, §Build topology, §Type loading, §Host adapter contract)

**Additional References:**
- Plan Steps 8 and 12: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-14: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Host import friction: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md` (§4 SDK source hook)
- Existing adapter/rule implementations: `bazel_rules/go/private/go_adapter.bzl`, `bazel_rules/go/private/stdlib_map.bzl`, `bazel_rules/go/private/component.bzl`, and `bazel_rules/go/tests/stdlib_export_data_tests.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Define and document three distinct host-neutral adapter contracts in `go_adapter.bzl`: (a) SDK source/oracle material for `arcc_stdlib_map`, (b) SDK package-graph metadata and compiled export artifacts for `ArccCheck`, and (c) exact target platform/SDK-key fields including toolchain version and GOEXPERIMENT.
2. Preserve rules_go as the upstream implementation. All rules_go provider and private-field access (`GoConfigInfo`, `GoStdLib`, SDK provider fields, export-stdlib transition) must remain confined to `go_adapter.bzl`; upper layers may consume only generic structs, files, depsets, and adapter-supplied rule attributes.
3. Route `stdlib_map.bzl` exclusively through the SDK-source contract for source files, package-list/oracle data, and SDK-root location. The `ArccStdlibMap` action may declare those sources, but it must receive no Go/toolchain executable or undeclared cache and must retain target-key stamping and cgo fail-closed behavior.
4. Route `component.bzl` exclusively through the SDK-export-data contract for stdlib graph metadata and compiled export files. `ArccCheck` must honor the complete descriptor returned by the adapter, including an alternative host-supplied set, while SDK `.go` sources remain absent from its direct and transitive inputs.
5. Make target identity a single adapter contract used consistently by map configuration/provider fields, component layouts, stdlib export-data validation, and asserted-surface SDK keys. It must include exact toolchain version, target GOOS/GOARCH, cgo state, sorted build tags, and GOEXPERIMENT; do not derive any of these from execution-host values.
6. Fail during Bazel analysis, naming the target and missing/mismatched fields, if any adapter descriptor is incomplete or if source, export-data, layout, and selected-map identities disagree. Never substitute a host default or cgo-off artifact for an unknown target configuration.
7. Keep adapter rule-attribute dictionaries composable and collision-checked when they are merged into `arcc_stdlib_map_rule` and `go_component_rule`. A host must be able to add private discovery attrs without editing generic rule implementations or exposing new author-facing attributes.
8. Extend the test-only rule implementation seams so analysis tests can supply a fake SDK export descriptor with sentinel metadata/export files. Assert that these sentinel files, and not the upstream defaults they replace, appear in the registered `ArccCheck` inputs and emitted layout descriptor.
9. Add or update analysis tests for source/export role isolation, complete target/key propagation, transitioned target identity, missing material, mismatch diagnostics, and absence of SDK source/toolchain binaries from component analysis. Retain existing pinned-map and asserted-surface key equality tests.
10. Update `go_adapter.bzl`, public `arcc_stdlib_map`/`go_component` documentation, and any topology comments to teach the three post-redesign seams and their input ownership. Remove stale wording that presents SDK source as a general check-rule attribute hook.

## Dependencies
- Step 4 supplies hermetic source-backed stdlib-map generation and the target SDK key.
- Step 8 supplies the upstream `GoStdLib` export-data implementation and member-only `ArccCheck` action-input topology that this task formalizes rather than replaces.
- Step 11 supplies asserted-surface key emission, which must consume the same target identity without gaining an analysis action.
- Task 2 may extend the component rule's adapter-attribute/function injection pattern; reuse that generic test seam where practical, but the SDK contracts remain functionally independent of runtime injection.

## Implementation Approach
1. Inventory the provider-specific values currently returned by `go_stdlib_toolchain`, `go_stdlib_export_data`, `go_target_mode`, and `go_build_platform`, then reshape them behind three explicitly documented contracts without changing emitted identity.
2. Move generic rule consumers to those contracts and add collision-checked adapter-attribute composition at each rule boundary.
3. Add a fake export-data provider/function to the test-only component implementation and inspect both the layout metadata and `ArccCheck` action inputs for sentinel replacement files.
4. Re-run target-transition, stdlib-map, asserted-surface, export-data, action-input, hermeticity, and full repository tests to prove the refactor is inert upstream.

## Acceptance Criteria

1. **SDK sources are scoped to map generation**
   - Given the default upstream adapter
   - When `ArccStdlibMap` and `ArccCheck` actions are inspected
   - Then only the map action receives SDK source/oracle material, and no component check action directly or transitively receives SDK `.go` files.

2. **Alternative export data is honored**
   - Given a fake adapter descriptor containing sentinel stdlib metadata and compiled export artifacts
   - When a checked component is analyzed
   - Then its layout names the alternative descriptor, its `ArccCheck` inputs contain the sentinel set, and replaced upstream export artifacts are not selected merely through hard-coded rules_go access in `component.bzl`.

3. **Target and SDK-key identity have one source**
   - Given default and transitioned target configurations
   - When map providers, layouts, export descriptors, checked surfaces, and asserted surfaces are inspected
   - Then toolchain version, GOOS, GOARCH, cgo, sorted tags, and GOEXPERIMENT agree exactly for each target and differ where the transition changes them.

4. **Incomplete or mismatched adapter data fails early**
   - Given a source descriptor without an oracle/root, an export descriptor without graph metadata/artifacts, or target identity that disagrees with the selected map
   - When the relevant rule is analyzed
   - Then analysis fails before action execution with an actionable diagnostic naming the target and fields/material at fault.

5. **Ruleset-specific knowledge stays in the adapter**
   - Given the finalized Bazel implementation
   - When loads and provider accesses are audited
   - Then only `go_adapter.bzl` names rules_go's `GoStdLib`, `GoConfigInfo`, SDK/provider internals, or export-stdlib transition; generic component/map rules consume host-neutral contracts.

6. **Hermetic action boundaries remain intact**
   - Given both SDK-backed actions
   - When their inputs, tools, environment, and execution requirements are inspected
   - Then they use declared files only, run only the arcc executable appropriate to the action, block network, use no inherited shell environment, and receive no Go/compiler tool or host cache.

7. **Upstream behavior is unchanged**
   - Given the default rules_go adapter and all repository components
   - When canonical layouts/maps/surfaces/reports are rebuilt and `just ci` runs
   - Then existing bytes and verdicts remain stable apart from deliberate test fixtures, and all self-check, determinism, and target-transition coverage passes.

## Metadata
- **Complexity**: High
- **Labels**: bazel, starlark, adapter, sdk, stdlib-map, export-data, toolchain, hermeticity
- **Required Skills**: Bazel toolchains and transitions, rules_go provider internals, Starlark adapter design, action-input analysis testing, deterministic SDK-key modeling
