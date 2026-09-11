# Task: Add Export-Data Layout Contract

## Description
Extend `packagelayout` with a deterministic, fail-closed contract for loading component roots from source and every reachable non-member package from compiler export data. Add the schema, path handling, and pre-load graph validation without changing the active `goanalysis` load mode yet, so this remains an atomic compatibility-safe foundation for the later cutover.

## Background
Step 8 completes design requirements N1 and N2. The pinned `go/packages` version can type-check member roots from source without parsing dependencies, but only when every reachable non-root package has an `ExportFile` and the driver supplies the complete transitive `Imports` graph. A missing graph node can panic inside `go/packages`, so the layout boundary must reject incomplete data before invoking the loader.

The effective package model uses `packages.Package.ExportFile` (the driver JSON field is `ExportFile`). Export artifacts are build outputs in the runfiles/workspace frame, not files beneath `go_sdk_root`; source-path and export-path resolution must therefore remain distinct. Whole-stdlib layouts used by map generation continue to be source-rooted and must not be broken by this additive task.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1-N2, DR-02, §Type loading, §Error Handling)

**Additional References:**
- Export-data spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-export-data-loading.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend the documented and in-memory package-layout contract so each effective non-member package can carry a compiler export artifact through `packages.Package.ExportFile`, while member roots continue to carry their selected `GoFiles` and `CompiledGoFiles`.
2. Resolve export-file paths in the emitter's workspace/runfiles frame, independently of `go_sdk_root`; reject empty, absolute, parent-escaping, non-normalized, missing, directory, or otherwise unreadable export artifacts with an error naming the package and path.
3. Add a pre-load validator that walks deterministically from every layout root and proves that each import edge resolves to a declared package node, each reachable non-root other than the `unsafe` builtin has an export file, and every export-backed package supplies an explicit import map (including an explicit empty map for a leaf).
4. Preserve package identity and the full transitive graph. Do not prune packages based on whether their exported API appears to reference them, and do not infer a missing import graph from non-member source.
5. Keep source-backed whole-stdlib layouts used by `arcc stdlibmap generate` valid. The export-data validator must be selected for component/member-only loading rather than globally requiring export files from generation roots.
6. Preserve deterministic layout JSON: roots, packages, import identities, dependency bindings, and any export-data metadata must round-trip without mutating caller-owned values.
7. Update `docs/package-layout-schema.md` and the package Component Contract to distinguish member source, non-member export data, SDK-source generation layouts, and their path frames.
8. Add focused unit tests for valid deep closures, `unsafe`, missing transitive nodes, nil/omitted import maps, missing export files, unsafe paths, deterministic round trips, and compatibility with source-backed stdlib generation layouts.

## Dependencies
- Step 7 must be complete so dependency artifact bindings and the final package-layout ownership model are already present.
- No Step 8 task dependency; this is the schema and validation foundation for Tasks 2-6.

## Implementation Approach
1. Introduce a small role-aware validation API in `packagelayout` that can distinguish roots from reachable non-roots without duplicating package identity logic.
2. Extend layout resolution and canonical marshaling for export artifacts, keeping existing source resolution behavior intact.
3. Validate graph closure with a sorted traversal and actionable errors before any `packages.Load` call can observe malformed export-backed data.
4. Update the schema documentation and exercise both component and stdlib-generation layout shapes in tests.

## Acceptance Criteria

1. **Valid export-backed closure is accepted**
   - Given a layout with one source-backed member and a multi-level non-member import closure
   - When export-data validation and resolution run
   - Then every non-member export file is resolved, every graph node remains present, and validation succeeds without reading non-member Go source.

2. **Incomplete graph fails before loading**
   - Given a reachable package whose import points to an absent package ID
   - When the layout is validated for member-only loading
   - Then validation returns a tool error naming the importing package, import path, and absent target before `packages.Load` is called.

3. **Missing export data fails closed**
   - Given a reachable non-root package other than `unsafe` with an empty, missing, or invalid export file
   - When the layout is validated
   - Then validation returns an actionable package-specific error and never falls back to dependency source.

4. **Explicit non-member graph shape is enforced**
   - Given an export-backed non-member package with an omitted `Imports` field
   - When validation runs
   - Then it is rejected as an incomplete graph, while an explicit empty import map is accepted for a leaf.

5. **Generation layout remains supported**
   - Given a whole-stdlib source layout used by the map generator
   - When its existing validation path runs
   - Then it remains valid without export files and existing stdlib-map tests stay green.

6. **Serialization is deterministic**
   - Given logically identical export-backed layouts whose collections were supplied in different orders
   - When they are marshaled and parsed
   - Then they produce byte-identical canonical JSON and preserve every export-file binding.

7. **Repository checks pass**
   - Given the completed change
   - When `just ci` runs
   - Then all lint, unit, integration, Bazel, self-check, and generated-file checks pass.

## Metadata
- **Complexity**: High
- **Labels**: go, packagelayout, export-data, validation, documentation
- **Required Skills**: Go, `go/packages` driver protocol, deterministic serialization, filesystem-safe path validation, table-driven testing
