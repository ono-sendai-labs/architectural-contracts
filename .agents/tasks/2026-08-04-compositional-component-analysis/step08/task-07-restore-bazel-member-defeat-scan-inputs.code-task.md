# Task: Restore Bazel Member Defeat-Scan Inputs

## Description
Restore the complete declared member-file vocabulary to Bazel component checks so assembly files and build-excluded Go files remain visible to `ScanAnalysisDefeats`, while preserving Step 8's prohibition on dependency and SDK source in `ArccCheck`.

## Background
Addresses finding F1 from the Step 8 implementation review. The final input-pruning path currently stages only member files whose extension is `.go`, emits only `GoFiles`/`CompiledGoFiles`, and drops build-constraint-excluded Go files instead of retaining them as `IgnoredFiles`. `ScanAnalysisDefeats` deliberately scans `GoFiles`, `IgnoredFiles`, and `OtherFiles`, so Bazel checks can silently miss assembly and linkname/cgo constructs hidden behind inactive build constraints.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1-N2, DR-11, §Error Handling, acceptance matrix “Analysis defeating”)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step08.yaml`

**Additional References:**
- Package layout schema: `docs/package-layout-schema.md` (§3a Source and export-data package roles)
- Defeat scanner: `go/internal/goanalysis/defeatscan.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Preserve every declared member source needed by analysis-defeating detection in the Bazel package-layout contract: selected Go files remain source roots, build-constraint-excluded Go files remain identifiable as ignored member files, and assembly files remain identifiable as member `OtherFiles`.
2. Stage those member files as declared `ArccCheck` inputs and recreate their workspace/runfiles frame without reintroducing any direct or transitive non-member source, SDK source, provider runfiles, Go binary, compiler tool, host cache, or network input.
3. Keep the member-only driver projection strict: member roots retain the relevant `GoFiles`, `CompiledGoFiles`, `IgnoredFiles`, and `OtherFiles`, while every non-member has all source-shaped fields cleared and continues to load only from `ExportFile`.
4. Make build-constraint filtering deterministic and lossless for the defeat scan: files excluded from member type-checking must be moved to or retained in `IgnoredFiles`, not discarded, and must not be parsed or type-checked by `go/packages`.
5. Add a Bazel execution fixture containing assembly plus build-excluded Go files with `//go:linkname` and cgo syntax. Prove the ordinary checked action reports the corresponding `AnalysisDefeating` observations under strict policy and downgrades them only under the explicit WARN carrier.
6. Extend the action-input analysis test with non-Go and build-excluded member files and assert in both directions that all declared member analysis inputs are present while every dependency and SDK source remains absent.
7. Preserve deterministic layout, report, and surface bytes and keep native member-only behavior unchanged.

## Dependencies
- Step 8 Tasks 1-6, especially the member-only driver projection and final action-input pruning.

## Implementation Approach
1. Extend the host-neutral Go package projection and layout emission with the member file roles the defeat scanner consumes, without exposing additional rules_go provider details above `go_adapter.bzl`.
2. Split selected and excluded member Go files during target-context validation, retain excluded files as ignored metadata, and carry member assembly through the driver response.
3. Change action-input derivation from “member `.go` files” to the exact declared member analysis-file set, leaving export-only non-members untouched.
4. Add focused Go layout/driver tests and a Bazel fixture that executes the real `ArccCheck` path.

## Acceptance Criteria

1. **Assembly remains analysis-defeating after input pruning**
   - Given a Bazel component member with a declared `.s` file
   - When its checked analysis action runs
   - Then the file is a declared member input, reaches `packages.Package.OtherFiles`, and produces an assembly `AnalysisDefeating` finding.

2. **Excluded Go files remain visible without being type-checked**
   - Given member Go files excluded by the target build context that contain `//go:linkname` and cgo syntax
   - When the member-only layout is validated and loaded
   - Then the files are absent from selected `GoFiles`/`CompiledGoFiles`, present in `IgnoredFiles`, and both bypass kinds are reported.

3. **Non-member source remains pruned**
   - Given the same component with ordinary and standard-library dependencies
   - When an analysis test inspects `ArccCheck`
   - Then every required declared member analysis file is present and no dependency or SDK source file appears directly, transitively, or through provider runfiles.

4. **Policy behavior stays fail-closed**
   - Given the Bazel defeat fixture under strict policy and under the explicit WARN carrier
   - When both reports are decoded
   - Then strict produces violations, WARN produces `ANALYSIS_LIMITATION` warnings, and neither path silently drops any site.

5. **Repository checks pass**
   - Given the completed remediation
   - When focused Go/Bazel tests and `just ci` run
   - Then all checks pass with deterministic artifacts.

## Metadata
- **Complexity**: High
- **Labels**: go, bazel, starlark, packagelayout, analysis-defeating, hermeticity
- **Required Skills**: Go, Starlark, rules_go providers, `go/packages` driver protocol, Bazel analysis and execution testing
