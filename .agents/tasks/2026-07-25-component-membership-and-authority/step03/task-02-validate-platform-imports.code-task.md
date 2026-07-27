# Task: Validate platform-consistent imports

## Description
Validate that each layout package's declared import graph matches the imports of its post-filter source set, while recovering imports for the supported imports-omitted shape and reporting unresolved surviving imports as analysis limitations.

## Background
A host may emit either a source and import set already filtered for the declared platform, or an unfiltered source set with imports omitted so the loader can recover them after filtering. The accidental middle—unfiltered files plus the union of every platform variant's imports—silently leaves the package graph inconsistent with the platform. The loader already parses surviving sources for standard-library import recovery, so it can compute and compare the complete post-filter import set.

The equality contract applies only to imports that resolve to an emitter-listed layout package or an SDK-discovered standard-library package. A surviving import with no resolvable package is a separate analysis gap: it must not make platform consistency into a layout-completeness requirement, but it also must not disappear. The loader collects these file/import observations as pure facts, and the checker renders deterministic `ANALYSIS_LIMITATION` warnings.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 T8–T8a, §4.4, §5.1b, §5.2–§5.3, §6, and §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 3)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` (F1 and the T8 consequence)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. After the platform filtering from Task 1, parse imports from every surviving non-stdlib Go source while retaining the source filename associated with each import. Keep parsing and output order deterministic.
2. Classify each recovered import as resolvable when it maps to an emitter-listed layout package or an SDK-discovered standard-library package. Treat cgo's pseudo-import `"C"` consistently with existing loader behavior and do not report it as an unresolved package.
3. Preserve enough pre-resolution information to distinguish an omitted `Imports` field from a declared non-empty import map. Do not let later standard-library recovery erase which emitter shape was supplied.
4. Support both conforming shapes:
   - For imports omitted, recover the full resolvable import map from surviving sources.
   - For declared imports, require bidirectional equality between declared resolvable paths and recovered resolvable paths.
5. Reject a declared resolvable import that no surviving file contributes, naming the package and extra import. This catches layouts carrying the union of mutually exclusive platform variants.
6. Reject a resolvable import contributed by a surviving file but absent from the declared map, naming the package, source file, and missing import so an FR2-relevant edge cannot vanish.
7. Compare import paths against their source-level keys while preserving existing ID-to-package resolution and standard-library vendor handling. Do not fold Step 5's standard-library provenance or vendor-policy changes into this task.
8. Define a pure fact representation for unresolved post-filter imports containing at least the importing package, source file, and import path. Thread it from the active validated layout through `goanalysis.LoadPackageFacts` into `facts.PackageFacts` without making the pure checker depend on `packagelayout`.
9. Deduplicate unresolved observations deterministically. If one unresolved import appears in multiple surviving files, preserve sufficient file-level evidence to satisfy the report contract without emitting unstable duplicates.
10. Make the checker emit a warning of the existing `report.AnalysisLimitation` kind for every collected unresolved import, with a message naming the file and import path. These warnings must not become violations or load errors.
11. Apply unresolved-import collection in both conforming shapes: declared imports may deliberately omit a dangling edge, and imports-omitted layouts encounter the same unresolved source import during recovery.
12. Ensure unresolved imports do not participate in the bidirectional T8 equality check. Their presence alone must not fail `ValidateAndResolve` or `LoadPackageFacts`.
13. Add an end-to-end layout-mode test through the application or CLI proving the diagnostic survives loader, fact, checker, and renderer boundaries.
14. Keep the existing `ANALYSIS_LIMITATION` behavior for analysis-defeating capabilities unchanged and preserve deterministic warning sorting.
15. Do not implement interface-file exclusion warnings, the success-line wording change, provenance-based standard-library classification, or canonicalization hardening in this task.

## Dependencies
- `task-01-declare-analysis-platform`: supplies the declared `build.Context` and authoritative post-filter source set used for import recovery and comparison.
- Steps 1–2 (complete): stable member/root loading and checker fact flow.
- No later Step 3 task depends on this task; it completes Step 3.

## Implementation Approach
1. Refactor the existing import-recovery pass into a deterministic source-import collector that records file provenance before it mutates package import maps.
2. Build indexes for emitter-listed and SDK-discovered packages, partition recovered imports into resolvable and unresolved sets, and validate the declared-import shape before normal package-reference resolution.
3. For omitted imports, synthesize references for every resolvable recovered import; for declared imports, compare sorted path sets in both directions and return precise mismatch errors.
4. Store unresolved observations on the validated active layout or another loader-owned result, translate them into a pure facts type in `goanalysis`, and append checker warnings from that fact field.
5. Cover the two conforming shapes, both mismatch directions, cross-platform union rejection, unresolved imports in both shapes, deduplication, and deterministic report rendering with focused tests and a layout-mode integration.

## Acceptance Criteria

1. **Filtered declared imports pass**
   - Given a package whose files and declared imports already match the layout's declared platform
   - When `ValidateAndResolve` runs
   - Then validation succeeds and the resolved package graph preserves those imports.

2. **Imports-omitted shape is recovered**
   - Given a package lists unfiltered platform variants and omits its imports
   - When platform filtering and validation run
   - Then only surviving files contribute imports, all resolvable imports are recovered, and the layout loads successfully.

3. **Cross-platform import union is rejected**
   - Given one package lists mutually exclusive platform variants and declares the union of their imports
   - When the declared platform leaves only one variant
   - Then validation fails naming each declared import that no surviving source contributes.

4. **Missing declared edge is rejected**
   - Given a surviving source imports a resolvable package but the non-omitted declared import map lacks that path
   - When validation runs
   - Then it fails naming the importing package, source file, and missing import.

5. **Equality is bidirectional and resolvable-only**
   - Given declared and recovered imports differ in either direction
   - When the differing paths resolve to layout or SDK packages
   - Then validation fails; unresolved recovered paths are excluded from that equality.

6. **Unresolved import does not fail loading**
   - Given a surviving source imports a path with no layout package and no SDK package
   - When the package uses either the declared-import shape or imports-omitted shape
   - Then validation and package loading do not fail solely because of that path.

7. **Unresolved import is visible**
   - Given the unresolved post-filter import from the previous criterion
   - When the component is checked
   - Then the report contains a deterministic `ANALYSIS_LIMITATION` warning naming the source file and import path.

8. **Excluded files create no limitation**
   - Given an unresolved import appears only in a source excluded by the declared platform
   - When validation and checking run
   - Then no warning is produced for that import.

9. **Existing standard-library recovery remains correct**
   - Given a surviving source imports an SDK-discovered standard-library package
   - When imports were omitted
   - Then the import resolves and participates in equality/recovery as a resolvable import without being reported as a limitation.

10. **Diagnostics are deterministic**
    - Given repeated and unordered unresolved imports across packages and files
    - When checks are run repeatedly
    - Then facts and warnings have stable deduplication, ordering, messages, and JSON/text rendering.

11. **Step 3 demo is reproducible**
    - Given identical sources and the same `arcc` binary with two layouts differing only in `platform`
    - When both are checked
    - Then each run selects its platform-correct file/import set, and an intentionally inconsistent layout fails with a precise T8 error.

12. **Repository checks remain green**
    - Given the complete Step 3 implementation
    - When `just ci` runs
    - Then all unit, integration, self-check, generation-cleanliness, and Bazel checks pass.

## Metadata
- **Complexity**: High
- **Labels**: Go, packagelayout, imports, facts, checker, diagnostics
- **Required Skills**: Go, `go/packages`, Go source parsing, graph validation, deterministic diagnostics, integration testing
