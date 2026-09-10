# Task: Declare Dependency Artifact Bindings

## Description
Extend the package-layout and Bazel component contracts so every direct declared or auto-attached dependency is bound to its surface, optional report, and structural producer provenance. Keep the bindings additive in this task: the existing source-backed dependency resolver remains active until Task 3.

## Background
Step 5 placed direct dependency surfaces and reports in the checked action's inputs only to establish producer ordering. The layout does not identify those artifacts, so `arcc check` cannot yet distinguish which surface/report pair belongs to which manifest dependency or derive checked versus asserted provenance from the build graph. Step 7 needs an explicit, deterministic binding before the consumer can replace `ResolveDependencyInterface`.

The binding is build metadata, not an authored trust claim. Bazel obtains it from `ArccComponentInfo`; native mode continues to locate artifacts by the existing sibling-path convention and does not synthesize build-graph provenance.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R7-R8, N2, DR-01, DR-03, §Build topology, §Provenance, freshness and authority)

**Additional References:**
- Plan Step 7: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Build-topology context: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md`
- Existing provider/action wiring: `bazel_rules/providers.bzl`, `bazel_rules/go/private/component.bzl`, `go/internal/packagelayout/packagelayout.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend the package-layout wire model with a sorted collection of direct dependency artifact bindings. Each record must identify the manifest dependency name, surface path, optional report path, whether the edge is auto-attached, and structural producer provenance (`checked` or `asserted`). Do not put authority, verdict, or freshness claims in the binding.
2. Emit exactly one binding for every direct authored and auto-attached `ArccComponentInfo` dependency. A checked provider must contribute both surface and report and provenance `checked`; an asserted provider must contribute a surface, no report, and provenance `asserted`.
3. Fail Bazel analysis early, naming the component and dependency, for impossible provider combinations: missing surface, checked provenance without a report, asserted provenance with a report, unknown provenance, duplicate component names, or conflicting authored/auto-attached identity.
4. Use runfiles-frame paths consistently with the manifest and package layout. Ensure the checked action declares every bound surface/report as a direct input and that frame-symlink construction covers every bound artifact without relying on transitive dependency runfiles to discover it.
5. Parse, validate, clone, and deterministically marshal the new bindings in `packagelayout`. Reject empty names/paths, duplicate names, duplicate surface paths assigned to different dependencies, unknown provenance, and non-normalized or unsafe runfiles paths before the driver runs.
6. Preserve the Step 7/8 transition: dependency source/runfile closure and `packages.NeedDeps` remain present, the existing `goanalysis.ResolveDependencyInterface` remains the production resolver, and no binding is semantically trusted by the check in this task.
7. Update `docs/package-layout-schema.md`, Starlark/provider comments, component contracts, and analysis tests to describe the binding as structural build metadata and to state that native artifact lookup remains convention-based.
8. Run `just ci` before committing.

## Dependencies
- Plan Step 5 supplies `ArccComponentInfo.surface`, `.report`, and `.provenance`, plus the ordinary checked analysis action and asserted producer.
- Plan Step 6 remains the active reference-analysis path while dependency interfaces are still source-backed.
- Task 2 consumes the binding through a validated surface resolver; Task 3 switches production to it.

## Implementation Approach
1. Define the smallest package-layout DTO and validation rules for dependency artifact bindings.
2. Add parser/marshal round-trip and invalid-shape tests before changing the Starlark emitter.
3. Emit bindings from direct provider edges, declare their files explicitly on the action, and add `rules_testing` assertions for checked, asserted, and auto-attached dependencies.
4. Update schema documentation and run the full gate while verifying the source-backed resolver is unchanged.

## Acceptance Criteria

1. **Every direct dependency has one deterministic binding**
   - Given a component with authored checked, authored asserted, and auto-attached checked dependencies
   - When its package layout is emitted twice
   - Then the bindings are sorted, byte-identical, and contain the correct surface/report paths, edge provenance, and auto-attached flags exactly once

2. **Provider provenance is structurally constrained**
   - Given a dependency provider with a missing surface, a checked provider without a report, an asserted provider with a report, or an unknown provenance value
   - When the consuming component is analyzed
   - Then Bazel fails before registering the check action and names the invalid dependency and component

3. **The action declares the bound artifacts**
   - Given a dependent checked component
   - When its `ArccCheck` action inputs are inspected
   - Then every direct dependency surface and available report named by the layout is a declared direct input and the dependency producer chain is present

4. **Layout validation fails closed**
   - Given empty, duplicate, conflicting, non-normalized, unsafe, or unknown-provenance dependency bindings
   - When the layout is parsed and validated
   - Then a deterministic tool error identifies the invalid record before package loading

5. **This task is additive**
   - Given the repository after the binding lands
   - When the production check path is inspected and `just ci` runs
   - Then `ResolveDependencyInterface`, dependency source inputs, and `NeedDeps` remain active, all checks pass, and no code yet derives a boundary decision from the new binding

## Metadata
- **Complexity**: Medium
- **Labels**: bazel, starlark, package-layout, surfaces, reports, provenance, build-graph
- **Required Skills**: Starlark provider/action design, Go JSON schema validation, deterministic serialization, rules_testing
