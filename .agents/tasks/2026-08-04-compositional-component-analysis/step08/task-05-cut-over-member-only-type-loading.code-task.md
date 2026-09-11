# Task: Cut Over to Member-Only Type Loading

## Description
Switch `goanalysis` and the package-layout driver to parse and type-check only component member packages, loading every non-member package from compiled export data. Add post-load guards and hermetic integration coverage that prove dependency source is not consulted.

## Background
The export-data spike established the exact load mode that retains complete `Uses`, `Selections`, and types for roots without loading dependency syntax: `NeedName|NeedFiles|NeedCompiledGoFiles|NeedImports|NeedTypes|NeedSyntax|NeedTypesInfo`, with no `NeedDeps`. Tasks 1-4 make the required full graph and export files available before this behavioral change.

This is the Step 8 keystone. Layout validation must run before `packages.Load` because the pinned loader can panic on an incomplete graph. After loading, dependency `Syntax`, `TypesInfo`, and source lists must remain absent; any violation is a tool error rather than an unnoticed regression to closure-wide work.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1, DR-02, §Type loading, acceptance matrix “Hermetic load”)

**Additional References:**
- Export-data spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-export-data-loading.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Remove `packages.NeedDeps` from the production component load mode while retaining `NeedName`, `NeedFiles`, `NeedCompiledGoFiles`, `NeedImports`, `NeedTypes`, `NeedSyntax`, `NeedTypesInfo`, and required module/version information.
2. Make the layout driver return `GoFiles`/`CompiledGoFiles` only for requested member roots and `ExportFile` with the complete `Imports` graph for every reachable non-member. Return defensive package copies so serving one request cannot mutate the validated active layout.
3. Invoke Task 1's closure/export validation before every export-backed component load. Do not allow `go/packages` source fallback for a missing non-member export artifact.
4. After loading, traverse the reachable graph and fail if any non-member has non-nil/non-empty `Syntax`, `TypesInfo`, `GoFiles`, or `CompiledGoFiles`; also verify its `Types` package is complete. Member packages must retain full syntax and type info for reference and defeat scans.
5. Preserve exact reference semantics: member `Uses` and `Selections`, interface method selections, promoted members, generics, imports, and declaring-object identities must match the Step 6 tables.
6. In native mode, rely on the Go driver/toolchain's `go list -export` behavior to obtain closure export data, document that native checks may execute the host toolchain, and surface unsupported or mismatched compiler export data as a tool error.
7. Add an integration fixture with a deep closure whose dependency source files are physically absent while export files and the complete graph are present. It must produce correct typed reference/import facts.
8. Add negative integration cases for one missing transitive node, one missing export file, and export data produced by an incompatible toolchain/version. Errors must be stable, actionable, and returned instead of panics.
9. Update stale Component Contract and design-reference comments that still describe the main loader as retaining `NeedDeps` until Step 8.

## Dependencies
- Task 1: Add Export-Data Layout Contract.
- Task 4: Emit Export-Data Package Layouts, which itself depends on Tasks 2-3.

## Implementation Approach
1. Centralize the production component load mode and switch it to the spike-proven bit set.
2. Project a request-specific driver response from validated layout package copies, stripping source from non-roots and export files from roots where appropriate.
3. Add a post-load invariant checker keyed by the canonical member set.
4. Exercise the real layout driver and `go/packages` with generated gc archives, including source-removal and failure fixtures; keep slow toolchain work behind integration build tags.

## Acceptance Criteria

1. **Only members have syntax**
   - Given a member package with a deep ordinary/stdlib closure
   - When `LoadPackageFacts` completes
   - Then members have syntax and `TypesInfo`, every non-member has neither, and all required non-member types are complete from export data.

2. **Typed references remain exact**
   - Given the reference-kind fixture using functions, interface methods, fields, constants, aliases, promoted members, generic instantiations, and pointer/value selections
   - When dependency sources are absent and the member is loaded
   - Then `Uses`/`Selections` and emitted `SymbolID`s match the pinned Step 6 outcomes.

3. **Import facts remain complete**
   - Given member source containing ordinary and blank imports
   - When it is loaded through export data
   - Then every member import edge is retained and classified from the same written path as before the cutover.

4. **Incomplete data cannot panic**
   - Given a layout missing one transitive node or one reachable package export file
   - When loading is requested
   - Then validation returns a package-specific tool error before `packages.Load`, and the process does not panic or fall back to source.

5. **Version skew is a tool error**
   - Given an export artifact incompatible with the loader's supported Go versions
   - When the load runs
   - Then the command exits 2 with a diagnostic naming the affected package/export artifact rather than emitting partial facts.

6. **Native mode uses the toolchain export path**
   - Given a native component check with no package layout
   - When `go/packages` invokes the host Go driver
   - Then the closure types are read from `go list -export` results, only member source is parsed, and the native-mode EXEC behavior is documented.

7. **End-to-end checks stay correct**
   - Given csvtool and arcc self-components
   - When checks run in native and Bazel modes
   - Then verdicts, surfaces, dependency boundaries, and typed findings remain correct and `just ci` passes.

## Metadata
- **Complexity**: High
- **Labels**: go, go-packages, export-data, type-loading, integration
- **Required Skills**: Go, `go/packages`, gc export data, package-driver protocols, hermetic integration testing
