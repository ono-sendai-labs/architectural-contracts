# Task: Cut Over Check to Reference Analysis

## Description
Switch `arcc check` end to end to the already-proven typed reference/import classifier, load and validate the Step 4 stdlib authority artifact for decisions, and delete the obsolete SSA/VTA and check-time Capslock pipeline. Update integration artifacts, documentation, and measurements in the same atomic cutover.

## Background
Tasks 01-04 establish the replacement path and fixtures without destabilizing production. The production runner still constructs a VTA call graph, expands dependency interfaces, builds a Capslock prune set, and invokes a `CapabilityAnalyzer`. This task removes that entire behavioral path. Capslock remains only behind stdlib-map generation. Dependency interfaces deliberately remain source-loaded by `ResolveDependencyInterface`, and `NeedDeps` deliberately remains enabled; persisted surface consumption is Step 7 and export-data/member-only loading is Step 8.

Step 5 documented a temporary semantic gap: emitted surfaces contain the exact declared interface, while checking admitted the implements closure. Task 02 already removed the workaround, and this cutover must remove every user-facing statement that the gap still exists.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R1-R6, DR-10, DR-11, changed components, deletion inventory, testing strategy)

**Additional References:**
- `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md` (baseline pipeline and Step 1 wall/CPU measurements)
- `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md` (authority map semantics used at check time)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Rewire `Runner.runCheck` so one analysis invocation loads member facts, scans AST + `types.Info`, resolves current source-backed dependency interfaces, classifies references/imports through `StdlibAuthority`, applies policy in the pure checker, and publishes report/surface artifacts from that result.
2. In layout/Bazel mode, open the declared `--stdlib-map` artifact through `artifactio.NewStdlibMapReader` and validate its full target SDK key before any verdict. In native mode, use the Step 4 native on-demand cache/generator path. Add a narrow injected authority-resolver seam for unit tests; do not retain `Runner.Analyzer` as a production seam.
3. Make the stdlib map mandatory whenever a check decision needs it, not merely a surface SDK-key source. Map absence, corruption, format/classifier mismatch, key mismatch, or inventory gap must exit 2 and publish no artifacts.
4. Replace checker consumption of `Facts.CallEdges`, `StdlibImports`, unresolved legacy import warnings, and externally injected Capslock findings with the new reference/import/classification facts. Preserve FR3/FR4/member-overlap/unused-dependency behavior where the new edge model does not intentionally change semantics.
5. Delete SSA construction and VTA (`ssautil.AllPackages`, program build, `vta.CallGraph`), legacy call-edge extraction, callgraph-only DTOs/helpers/tests, and their Go/module/BUILD/component dependencies. A repository search outside generation code must find no check-time SSA/VTA path.
6. Delete the Capslock check adapter entry point, `CapabilityAnalyzer`/`AnalyzeRequest` prune contracts and boundary-prune classifier code that is no longer used. Preserve the separate Capslock generation API, generation classifier, stdlib-map tests, and map-generation dependency.
7. Delete all five stdlib predicates and loader-produced stdlib classification used by the old check. Every production stdlib decision must call `StdlibAuthority.IsStdlibPackage`, `SymbolAuthority`, or `PackageInitAuthority`.
8. Keep `ResolveDependencyInterface` source-loading in place and keep `packages.NeedDeps` in the check load mode. Do not consume dependency surfaces, remove dependency sources, introduce export-data loading, or claim N1/N2 completion; those changes belong to Steps 7 and 8.
9. Update every affected report golden and Bazel/native integration expectation to the new correct semantics now; do not defer incorrect goldens to Step 9. Prove `arcc check` and the checked Bazel analysis action both use the new path.
10. Remove the temporary Step 5 divergence language from CLI help, README, `app`, `surface`, resolver comments, and the plan-adjacent documentation it changed. State that check and emitted surface now share the exact declaring-object interface.
11. Run end-to-end checks for csvtool and arcc's self-components. Re-measure the same representative components/procedure from Step 1 and append an explicitly comparable post-cutover wall/CPU delta to `research/current-analysis-pipeline.md`; distinguish this Step 6 result from later Step 8 export-data savings.
12. Update all Component Contract blocks, manifests, BUILD files, and dependency declarations affected by removing the legacy analyzer path. Run `just ci` before committing.

## Dependencies
- Task 01: production-ready reference/import scanner.
- Task 02: exact dependency symbol resolution and deleted implements closure.
- Task 03: complete object/import/stdlib classification and design fixtures 1-6/9.
- Task 04: analysis-defeating detection and DR-17 report aggregation.
- Plan Steps 4-5: stdlib authority map reader/cache, report/surface outputs, Bazel action, and report assertions.

## Implementation Approach
1. Add the production authority resolver and fail-closed map validation, then assemble the new scan/classify/check pipeline behind the existing command contract.
2. Switch unit and integration seams from capability analyzer outputs to typed classification inputs.
3. Remove the obsolete code and dependencies in the same change so there is no dual production path.
4. Regenerate/update correct report goldens, run native and Bazel self-checks, update documentation, and record comparable timing results.

## Acceptance Criteria

1. **Production check uses only reference analysis**
   - Given a native or layout-mode component check
   - When `arcc check` analyzes it
   - Then boundary and authority decisions originate from member AST/`types.Info`, resolved dependency interfaces, and `StdlibAuthority`, with no SSA, VTA, call graph, or check-time Capslock invocation

2. **Stdlib artifacts fail closed before publication**
   - Given a missing, corrupt, key-mismatched, classifier-mismatched, format-mismatched, or inventory-incomplete stdlib map
   - When a check runs
   - Then it exits 2 with an actionable error and writes neither report nor surface artifact

3. **Legacy analysis is deleted, generation survives**
   - Given the repository after cutover
   - When code/dependency searches and all generation tests run
   - Then no check path, DTO, prune set, or adapter entry point for SSA/VTA/Capslock remains, while stdlib-map generation still performs its one intended Capslock batch successfully

4. **Fixtures remain the cutover gate**
   - Given design fixtures 1-6 and 9 plus the analysis-defeating fixture
   - When they are run through the real `arcc check` orchestration
   - Then they retain the outcomes proven before cutover, including function values, blank-import init, exact interface dispatch, rejected concrete access, the full reference/import tables, and `os.Stdin` versus `io.EOF`

5. **Surfaces and checks agree**
   - Given a declared-interface dependency with a concrete implementor omitted from its interface files
   - When its surface is emitted and a dependent is checked
   - Then both use the same exact declaring-object symbol set and no implements-closure disagreement is documented or observed

6. **Reports preserve complete evidence**
   - Given three sites reaching one capability and an `UNANALYZED` reference
   - When the real command emits text and canonical JSON reports
   - Then text contains one counted finding, JSON retains all sorted sites/evidence/class/SDK key, and default policy fails the unanalyzed case

7. **Current loading boundary is explicit**
   - Given a check after this task
   - When its package and dependency loading are inspected
   - Then `NeedDeps` and source-backed `ResolveDependencyInterface` remain, with comments pointing to Steps 7-8 rather than prematurely consuming surfaces/export data

8. **Native, Bazel, and self-hosting integrations pass**
   - Given csvtool, arcc's own components, report assertion rules, and checked Bazel actions
   - When `just ci` and the step's integration checks run
   - Then all pass with correctly updated goldens and no undeclared build/component dependencies,
     **except** the four components listed in AC 8b, whose checks are explicitly exempted there.

8b. **The honest post-cutover failures are exempted, not hidden**
   - Given that deleting the implements-closure laundering makes `UNANALYZED` stdlib records
     referenced by member source into `AnalysisDefeating` findings — the designed behaviour
     (DR-11, I4), for which no user-facing policy carrier exists yet
   - When the four affected components are removed from the gate
   - Then each is exempted by an explicit, individually justified TODO rather than by weakening
     the analysis, the map, or any test's meaning:
     - `internal/artifactio` and `internal/manifest` — transitional protobuf-runtime members;
       TODO cites Step 7's protobuf-runtime `PACKAGE_SURFACE` migration.
     - `internal/goanalysis` — retained `x/tools` members; TODO cites Step 7's `x/tools`
       wrapper.
     - `examples/csvtool`'s `parsecsv` — `csv.Reader.ReadAll` is honestly `UNANALYZED`; TODO
       cites Step 7's residual-`UNANALYZED` decision.
   - And the native exemptions are comments in `just selfcheck`, the Bazel ones use the
     existing `check_tags = ["manual"]` seam from Step 5 task 05 — no new mechanism is
     invented — and the exempted findings are enumerated in the change description so the
     set cannot silently grow.

9. **The performance delta is recorded honestly**
   - Given the Step 1 measurement procedure and representative components
   - When post-cutover checks are measured
   - Then `research/current-analysis-pipeline.md` records wall/CPU results and deltas attributable to removing SSA/VTA/check-time Capslock, while noting that dependency source loading and `NeedDeps` remain until later steps

## Metadata
- **Complexity**: High
- **Labels**: go, cli, checker, cutover, ssa-removal, capslock-removal, bazel, documentation, performance, keystone
- **Required Skills**: Go orchestration and dependency injection, static-analysis migration, Bazel integration, golden maintenance, performance measurement
