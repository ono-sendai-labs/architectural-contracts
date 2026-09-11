# Task: Expose Standard-Library Export Data

## Description
Add the upstream Bazel adapter seam that makes target-configured standard-library export data and its complete package graph available to component checks as declared inputs. Keep toolchain-specific `GoStdLib` details confined to the adapter and do not execute a Go toolchain binary in the analysis action.

## Background
Non-member loading requires export data for standard-library packages as well as ordinary dependencies. The SDK source tree used by stdlib-map generation is not sufficient and must leave the check action after the cutover. Pinned `rules_go` exposes target-configured stdlib metadata through `GoStdLib`, including its package-list JSON/cache artifacts; this data is generated for the target mode and is the upstream source for the complete stdlib import graph and export files.

This task establishes the host adapter contract and an execution-time representation that `packagelayout` can consume in Task 4. It must preserve design I5: arcc's check action reads declared files and self-executes only its package driver; it never receives or runs `go`, compiler tools, a host cache, or network access.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1-N3, I5, DR-02, §Type loading, §Host adapter contract)

**Additional References:**
- Export-data spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-export-data-loading.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Inspect pinned `rules_go` 0.61.1 and identify the `GoStdLib` fields and generated metadata that provide the target configuration's complete package identities, direct import edges, and export artifacts.
2. Add an adapter function and any private attribute/provider plumbing needed to return a host-neutral stdlib export-data descriptor plus its ordinary `File`/depset inputs. Only `go_adapter.bzl` may name `GoStdLib` or its private fields.
3. Ensure the selected stdlib data is for the same target GOOS, GOARCH, cgo mode, tags, toolchain version, and GOEXPERIMENT used by the component layout and stdlib authority map; fail analysis with a target-naming diagnostic when the adapter cannot provide it.
4. Define the execution-time handoff to `packagelayout` so the effective package graph can populate `ExportFile` for every reachable stdlib package and retain complete `Imports` edges. Do not infer stdlib membership from import-path spelling.
5. Declare only the generated stdlib metadata and compiled export artifacts needed by checking. The SDK source depset, `go` binary, compiler/linker tools, undeclared build cache, and network must not become check inputs.
6. Keep the seam host-replaceable and document it in `go_adapter.bzl` and the package-layout emitter documentation. Step 12 may replace the upstream implementation, but upper layers must not require rules_go-specific shapes.
7. Add Bazel analysis tests that prove the seam is target-configured, exposes metadata/export artifacts, rejects absent material, and excludes toolchain binaries and SDK sources.

## Dependencies
- Task 1: Add Export-Data Layout Contract defines the effective `ExportFile` and graph requirements.
- Task 2: Collect Go Archive Export Files establishes the analogous host-neutral package export-file projection.

## Implementation Approach
1. Follow the resolved `GoConfigInfo`/toolchain path already used by `go_target_mode` so stdlib exports and SDK keys cannot describe different target configurations.
2. Project the minimum rules_go-specific `GoStdLib` material into a generic descriptor struct returned by the adapter.
3. Add focused analysis-test seams for missing and mismatched material, and document exactly which declared files upper layers receive.

## Acceptance Criteria

1. **Target stdlib exports are available**
   - Given a pure target configuration under the pinned rules_go toolchain
   - When the adapter resolves standard-library type data
   - Then it returns declared metadata and export artifacts sufficient to identify every reachable stdlib package, its imports, and its `ExportFile`.

2. **Configuration identity is consistent**
   - Given a transitioned GOOS/GOARCH or build-tag configuration
   - When stdlib export data and the stdlib map are selected
   - Then both describe the same target identity, and absent or mismatched metadata fails with an actionable target-specific error.

3. **No source or toolchain execution leaks in**
   - Given the adapter's returned inputs
   - When an analysis test inspects them
   - Then they contain compiled stdlib type data and metadata but no SDK `.go` sources, `go` binary, compiler/linker tools, or host cache paths.

4. **Host boundary stays isolated**
   - Given the completed implementation
   - When Starlark loads are inspected
   - Then only `go_adapter.bzl` references `GoStdLib`; component/aspect code sees a host-neutral contract.

5. **Failure is early and named**
   - Given a toolchain that exposes no usable stdlib export-data descriptor
   - When a component is analyzed
   - Then Bazel analysis fails before action registration and names the component and missing adapter material.

6. **Repository checks pass**
   - Given the additive seam
   - When `just ci` runs
   - Then existing component checks, stdlib-map generation, and self-check remain green.

## Metadata
- **Complexity**: High
- **Labels**: bazel, starlark, rules-go, stdlib, export-data, adapter
- **Required Skills**: Starlark, Bazel toolchains/providers, rules_go `GoStdLib`, target transitions, hermetic build design
