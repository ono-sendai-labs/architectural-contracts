# Task: Add Checked Bazel Analysis Action

## Description
Move checked-component analysis into one ordinary Bazel action owned by `go_component`, publish its report and surface through `ArccComponentInfo`, and expose both only through `OutputGroupInfo(arcc)` rather than default outputs.

## Background
Currently `go_component` writes only a manifest and layout, while each `.check` test reruns `arcc check`. Tests cannot publish outputs to dependent actions, so no build-graph provenance exists. Step 4 delivered `stdlib_map_default_attr()` specifically for this action. In Step 5 the action still runs today's source/SSA/VTA/Capslock analysis: export data arrives in Step 8, and dependency surfaces are not semantically consumed until Step 7. Direct dependency report/surface artifacts must nevertheless be declared as ordering inputs now so testing a dependent establishes the producer chain required by R8.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` — R8, N2, I5, §Build topology, §Composition across components, acceptance-matrix Build graph row

**Additional References:**
- Plan Step 5: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-01: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Build-topology friction context: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md`
- Current rules: `bazel_rules/go/private/component.bzl`, `command.bzl`, `providers.bzl`
- Step 4 default map seam: `bazel_rules/go/private/stdlib_map.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add one ordinary action per checked component invoking Task 3's artifact-producing `arcc check` command with report-verdict-only semantics. Its outputs are exactly `<name>.report.json` and `<name>.surface.json`; violations do not fail the action, tool errors do.
2. Attach `_stdlib_map = stdlib_map_default_attr()` to the component rule and pass/declare the map so the surface receives the exact target SDK key. Extend the component layout's platform block with the already-supported `toolchain_version` and `goexperiment` fields needed for the same target identity.
3. For this transitional step, declare the manifest, layout, today's source/runfile closure, SDK inputs required by today's loader, arcc executable, and stdlib map. Also declare direct checked dependencies' report/surface outputs as build-order inputs, but do not parse them or replace `ResolveDependencyInterface` until Step 7. Do not introduce export data or remove closure sources before Step 8.
4. Extend language-neutral `ArccComponentInfo` with `surface`, `report`, and `provenance`; regular producers set `provenance = "checked"`. Keep existing manifest/layout/closure/contracts fields working for current callers. `manual`-tagged components are **out of scope for this task** and deliberately remain on the checked path with `provenance = "checked"` until Task 5 introduces the asserted branch: Task 5 owns detecting the tag, auditing every current use of it, and migrating the fixture-only uses. Do not implement, partially implement, or anticipate that branch here — doing so would decide Task 5's migration and fixture classification inside this task. This transitional state is expected and is not a defect of this task.
5. Return `OutputGroupInfo(arcc = depset([report, surface]))`. Do not put report/surface in `DefaultInfo.files`: `bazel build //...` must not execute component analysis unless the `arcc` group or a consumer/test requests it.
6. Keep the action hermetic under I5: declared inputs only, no host cache/network, no toolchain binary execution, deterministic argv/environment, and the existing layout-driver working-directory contract.
7. Factor command construction in `command.bzl` so the action and assertion rules cannot drift. Task 6 will remove legacy check execution from the tests.
8. Add rules_testing coverage for output names/provider fields/provenance, exact action argv/inputs/outputs, stdlib-map attachment, no default outputs, and dependency producer edges. Add execution coverage showing unchanged components produce byte-identical reports and surfaces.
9. Update Starlark docs, public macro docs, BUILD metadata, and any generated-layout goldens affected by the newly complete platform block.

## Dependencies
- Task 3 supplies the artifact-producing check command and always-green-on-verdict mode.
- Step 4 Task 6 supplies `ArccStdlibMapInfo` and `stdlib_map_default_attr()`.
- Existing `go_component` manifest/layout actions and `ArccPackageInfo` supply today's source closure.
- Task 5 adds the asserted alternative; Task 6 turns the check rules into pure report assertions.

## Implementation Approach
1. Extend the provider and command helper contracts with analysis-action data while preserving all existing fields/callers.
2. RED: analysis tests for outputs, output group, default-output laziness, map input, and direct-dependency producer edges.
3. GREEN: create the checked action and wire its inputs through `go_component_impl`.
4. RED/GREEN: execution tests for pass/fail reports, deterministic artifacts, and no action under an ordinary default build.
5. Update docs/goldens and run the full CI gate.

## Acceptance Criteria

1. **Checked components publish structural outputs**
   - Given a regular `go_component` (a `manual`-tagged component is out of scope here and stays on the checked path until Task 5)
   - When analyzed
   - Then its provider carries `<name>.surface.json`, `<name>.report.json`, and `provenance = "checked"`, produced together by one ordinary analysis action.

2. **Violations are data, tool errors fail the action**
   - Given conforming, violating, and malformed fixtures
   - When their analysis outputs are requested
   - Then conforming and violating actions succeed with pass/fail reports, while the malformed/tool-error action fails.

3. **Analysis outputs are lazy**
   - Given wildcard/default builds and an explicit `--output_groups=+arcc` build
   - When action execution is observed
   - Then the default build runs no component analysis, while the output-group build produces both artifacts.

4. **Dependent checks establish producer order**
   - Given checked component B depends directly on checked component A
   - When B's report or `.check` is requested
   - Then A's analysis action is in the build graph and builds first, even though B does not semantically read A's artifacts until Step 7.

5. **Inputs match the Step 5 transition**
   - Given a checked component action
   - When its action graph is inspected
   - Then it contains the manifest, layout, today's source/runfile closure, required SDK source inputs, Step 4 stdlib map, arcc tool, and direct dependency artifacts; it contains no undeclared host cache/network input and does not yet claim export-data/member-only semantics.

6. **Target identity is coherent**
   - Given a target configuration with toolchain version, GOOS/GOARCH, tags, cgo-off, and GOEXPERIMENT
   - When the action emits its surface
   - Then the surface SDK key matches the attached stdlib map exactly and the generated layout carries the target platform fields required to reproduce it.

7. **Artifacts are deterministic**
   - Given two unchanged executions for the same component inputs
   - When their output files are compared
   - Then report and surface bytes are identical.

## Metadata
- **Complexity**: High
- **Labels**: bazel, starlark, analysis-action, providers, output-group, hermeticity
- **Required Skills**: Bazel action authoring, Starlark providers, rules_go toolchains, rules_testing, hermetic build design
