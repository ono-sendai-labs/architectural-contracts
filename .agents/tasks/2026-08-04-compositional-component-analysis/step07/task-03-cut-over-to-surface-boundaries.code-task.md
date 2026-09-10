# Task: Cut Over to Surface Boundaries

## Description
Switch native and Bazel checks from source-derived dependency interfaces to the validated surface resolver, publish all three boundary status axes, warn on failed dependency checks, reject direct surface overlap before lookup construction, and delete the legacy per-dependency source-loading path.

## Background
Tasks 1-2 establish the build binding and a fully tested artifact consumer without changing production. This task is the atomic behavior cutover: `Runner.runCheck` must resolve the target SDK key before dependency artifacts, pass only surface-derived `DependencyInterface` values to the pure checker, and render their provenance/freshness/authority without parsing each dependency a second time.

The component package loader still retains `NeedDeps` and Bazel still stages closure source until Step 8 replaces non-member source with export data. This task removes the dedicated `ResolveDependencyInterface` `./...` load and every dependency-interface source parse; it must not claim the Step 8 member-only/export-data input contract early.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R7, R14, DR-03, DR-06, DR-08, DR-12, §Edge classification, §Provenance, freshness and authority, §Error Handling, acceptance matrix)

**Additional References:**
- Plan Step 7: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Baseline and remaining per-dependency load cost: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Prior cutover boundary: `.agents/tasks/2026-08-04-compositional-component-analysis/step06/task-05-cut-over-check-to-reference-analysis.code-task.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Reorder `Runner.runCheck` so it validates the stdlib map/target SDK key before resolving dependencies, then resolves every declared and auto-attached dependency through Task 2. Layout mode uses Task 1 bindings; native mode uses sibling artifact conventions.
2. Delete the source-backed `goanalysis.ResolveDependencyInterface`, its `packages.Load ./...`/foreign-member loading branches, interface-file source extraction, implements-era caches/helpers, and tests that exist only for source derivation. No replacement may parse or type-check a dependency to reconstruct its architectural surface.
3. Preserve Step 6 verdict parity on all existing boundary/reference/import fixtures. Declared-interface checks use exact persisted `SymbolID`s; package surfaces authorize owned packages; import use still marks authored and auto-attached dependencies used.
4. Make direct dependency overlap fail before any package-to-dependency map is usable. The deterministic `DEPENDENCY_OVERLAP` tool error must name the canonical package and both component names independent of dependency order; remove or bypass no existing insert-or-error guard.
5. Extend `report.DependencyBoundary` and canonical report encoding with provenance, freshness, and structural authority. Deterministically render every applicable text word: `certified` for checked-pass non-stale boundaries, `asserted`, `check failed`, `stale`, and `untrusted` for unknown authority. Preserve all axes in JSON rather than collapsing combinations into one label.
6. Add `DEPENDENCY_CHECK_FAILED` as a warning when a consumed checked report has verdict fail. The dependent must continue analysis against the surface and must not duplicate the dependency's violations or call the boundary certified.
7. Emit a stale warning for native `STALE` surfaces while allowing the check to continue; `UNKNOWN` freshness is visible in JSON but is not itself a tool error. A missing surface, format/namespace/SDK mismatch, invalid report, or overlap remains a tool error and publishes neither report nor surface.
8. Prove native `VERIFIED`, `STALE`, and `UNKNOWN` behavior through the real command, with file-open/load instrumentation showing that the dependency-surface resolution and freshness path reads bytes only and never parses dependency source. Keep the main member/closure loader's `NeedDeps` transition explicitly documented as Step 8 work.
9. Update the Bazel action from ordering-only artifact inputs to semantic surface/report consumption. Do not remove closure source, SDK source, transitive manifest/layout, or `NeedDeps` inputs yet; Step 8 owns export-data cutover and final N1/N2 action-input shape.
10. Update native selfcheck/integration setup so dependency surfaces are produced or staged in deterministic topological order without relying on stale developer cache state. Do not weaken missing-surface errors or check generated artifacts into an uncontrolled host-specific state.
11. Add end-to-end tests for: pass/fail/asserted dependencies, all text labels and JSON axes, missing/mismatched artifacts exiting 2 with no outputs, direct overlap, namespace cross-rejection, native freshness, and parity with every existing Step 6 fixture.
12. Remove all remaining `OwnCheckRuns`/`CertificationReference` code or documentation residue; verification status must now come only from artifacts/provider structure. Update Component Contract blocks, README/layout docs, build metadata, and run `just ci` before committing.

## Dependencies
- Task 1: direct Bazel artifact bindings in the layout/action.
- Task 2: validated surface/report resolver and pure status vocabulary.
- Plan Step 6: production typed reference/import classifier and deterministic overlap index.
- Tasks 4-6 build on this cutover to migrate foreign ownership and remove the temporary Step 6 exemptions.

## Implementation Approach
1. Add production-runner integration tests around Task 2's resolver, status reporting, and no-publication error ordering.
2. Reorder authority/dependency resolution, switch both operating modes, and route the status-rich interfaces through checker/report.
3. Delete the legacy source resolver and its obsolete seams in the same change so no dual production path remains.
4. Update Bazel and native integration harnesses, run parity/no-parse/overlap tests, then update documentation and run the full gate.

## Acceptance Criteria

1. **Dependency decisions consume artifacts, not reconstructed source surfaces**
   - Given a native or layout-mode check with valid direct dependency artifacts
   - When the production runner resolves boundaries
   - Then packages and symbols come only from decoded surfaces and the dedicated dependency `packages.Load`/interface extraction path is absent

2. **Step 6 verdicts remain semantically identical**
   - Given every existing reference-kind, import-table, declared-interface, and package-surface fixture
   - When it is rerun with equivalent persisted dependency surfaces
   - Then its verdict and boundary findings match the Step 6 source-backed result

3. **Status axes remain orthogonal and visible**
   - Given checked-pass, checked-fail, asserted, stale, and unknown-authority boundaries in valid combinations
   - When text and canonical JSON reports are emitted
   - Then JSON preserves all three axes and text renders every applicable status word without calling failed, stale, asserted, or untrusted boundaries certified

4. **Failed dependencies warn downstream**
   - Given a dependency surface paired with a structurally checked fail report
   - When a dependent is checked
   - Then the boundary is `CHECKED_FAIL`, one deterministic `DEPENDENCY_CHECK_FAILED` warning is emitted, the dependency's own violations are not copied, and the dependent's verdict depends only on its own findings

5. **Native freshness never parses dependency code**
   - Given unchanged, changed, and unreadable dependency inputs
   - When the real native command consumes the surface
   - Then it reports `VERIFIED`, `STALE`, and `UNKNOWN` respectively, and parser/load counters prove the resolver and freshness computation parse no dependency file

6. **Overlap and artifact faults fail before publication**
   - Given two direct surfaces claiming one package, or a missing/invalid/mismatched surface or report
   - When a production check runs with fresh output paths
   - Then it exits 2 deterministically, names the fault, and writes neither output artifact

7. **The Step 8 boundary is honest**
   - Given the completed surface cutover
   - When the action graph and member loader are inspected
   - Then dependency surfaces/reports are semantically consumed and no dedicated dependency-source resolution occurs, while closure sources and `NeedDeps` remain explicitly staged for the later export-data cutover

8. **Repository integration passes**
   - Given native selfcheck, csvtool, self-hosting components, Bazel checked actions, asserted producers, and report assertion rules
   - When `just ci` runs
   - Then they pass except only the five explicitly recorded Step 6 AC8b exemptions, which Tasks 4-6 remove without broadening

9. **The Step 7 status demo is reproducible**
   - Given `examples/csvtool/app` and fixtures for checked-pass, checked-fail, and stale dependency artifacts
   - When dependency source is made unreadable and the checks are run in native and Bazel modes as applicable
   - Then csvtool still resolves the surface with native freshness `UNKNOWN`, and the reports visibly demonstrate `certified`, `stale`, and `check failed` boundary renderings

## Metadata
- **Complexity**: High
- **Labels**: go, cli, checker, reports, surfaces, cutover, provenance, freshness, overlap, bazel
- **Required Skills**: Go orchestration and dependency injection, artifact-driven architecture, deterministic reporting, Bazel action integration, integration-test design
