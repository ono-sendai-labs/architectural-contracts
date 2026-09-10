# Task: Resolve Validated Dependency Surfaces

## Description
Add an additive surface-backed dependency resolver that decodes dependency surfaces and reports into exact `facts.DependencyInterface` values with independent provenance, freshness, and authority axes. Validate every persisted and structural invariant fail-closed, but leave the production runner on the source-backed resolver until Task 3.

## Background
The Step 3/5 surface and report codecs already provide bounded, canonical artifact decoding, and Task 1 makes Bazel's direct artifact-to-dependency association explicit. The missing layer is a shell-side consumer that verifies artifact identity against the current component, namespace, and target SDK key before the pure checker sees the dependency's packages or symbols.

Native mode has a deliberately weaker contract: it finds `<manifest>.surface.json` and an optional sibling report by convention, always treats the surface as asserted, and computes freshness by reading source and manifest bytes without parsing dependency Go syntax. Bazel freshness and provenance instead come from the build graph.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R7-R8, DR-03, DR-06, DR-08, §New: surface, §Provenance, freshness and authority, §Canonical namespace, §Error Handling)

**Additional References:**
- Plan Step 7: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Source-loading cost and resolver description: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Namespace motivation: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md`
- Artifact codecs and conventions: `go/internal/artifactio/surface.go`, `report.go`, `surfacepaths.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Define typed, pure status vocabularies for dependency boundaries: provenance `CHECKED_PASS|CHECKED_FAIL|ASSERTED`, freshness `BUILD_GRAPH|VERIFIED|STALE|UNKNOWN`, and structural authority `DECLARED{set}|UNKNOWN`. Extend `facts.DependencyInterface` with these values without importing shell packages into the core.
2. Add a shell-side resolver whose inputs explicitly include the manifest dependency identity, optional Task 1 layout binding, current `hostpolicy.NamespaceID`, expected `stdlibauthority.SDKKey`, operating mode, and narrow filesystem/read seams. Its result must contain only decoded artifact data and derived status; it must never load or parse dependency Go packages.
3. Decode surfaces through the bounded canonical codec and validate component name, interface style, concrete canonical packages, canonical `SymbolID`s, authority spelling, format version, namespace equality, complete SDK key equality (including tags, GOEXPERIMENT, classifier hash, and map format), and checked/asserted structural provenance. Each mismatch must produce a distinct actionable tool error.
4. In Bazel/layout mode, require the Task 1 binding. Derive `CHECKED_PASS` only from `provenance=checked` plus a present, valid report whose component matches and verdict is pass; derive `CHECKED_FAIL` from a valid fail report; derive `ASSERTED` only from `provenance=asserted` with no report. Set freshness to `BUILD_GRAPH` in every valid Bazel case.
5. In native mode, locate the required surface and optional report with `artifactio.SurfacePath`/`ReportPath`, but always derive provenance `ASSERTED`; a sibling report is audit information and must never certify a standalone file. Missing surface is an error, while a missing report is allowed.
6. Implement native freshness as a byte-only best-effort check against the surface digest. Share producer/consumer digest framing, enumerate only the dependency member source set and manifest bytes when that set can be established without AST or `packages.Load`, return `VERIFIED` on equality, `STALE` on a mismatch, and `UNKNOWN` when the source root or complete byte set is unreadable/unavailable. Do not turn unreadability into a tool error and do not treat an asserted surface's empty digest as a content claim.
7. Ensure the resolver canonicalizes paths exactly once and validates fixed points. Add two distinct idempotent test canonicalizers and prove each accepts its own surface while rejecting the other's namespace; stdlib paths remain untouched.
8. Preserve exact interface semantics: declared-interface surfaces populate the persisted symbol set verbatim after validation; `PACKAGE_SURFACE` surfaces authorize their packages and carry no symbols. Do not reconstruct an implements closure or inspect dependency exports.
9. Add table-driven tests for every validation branch, all provenance/freshness combinations, report verdict/component mismatches, native path conventions, unreadable roots, and proof through parser/load/file-open seams that freshness reads bytes but no dependency syntax is parsed.
10. Update package contracts and run `just ci` before committing. Production `Runner.runCheck` must still invoke the legacy resolver at the end of this task.

## Dependencies
- Task 1 supplies validated Bazel dependency artifact bindings.
- Plan Steps 3 and 5 supply the authority lattice, surface/report schemas, codecs, path conventions, and producer provenance.
- Plan Step 6 supplies the exact declaring-object `DependencyInterface` semantics the resolver must preserve.
- Task 3 performs the production cutover and report rendering.

## Implementation Approach
1. Introduce the pure status types and artifact-to-facts conversion API with synthetic artifact tests.
2. Implement Bazel binding validation and checked/asserted report derivation.
3. Implement native convention lookup and a shared byte-only digest comparison seam, explicitly separating `UNKNOWN` freshness from hard artifact errors.
4. Prove namespace/SDK/format fail-closed behavior and exact symbol/package conversion, then leave the API unused by production for an atomic additive commit.

## Acceptance Criteria

1. **Valid artifacts become exact dependency facts**
   - Given valid declared-interface and `PACKAGE_SURFACE` artifacts for the current namespace and SDK key
   - When the new resolver consumes them
   - Then it returns their exact packages/symbols/authority plus the correctly derived provenance and freshness without loading dependency source

2. **Bazel provenance comes only from structure and verdict**
   - Given checked-pass, checked-fail, and asserted provider bindings
   - When their artifacts are resolved
   - Then statuses are respectively `CHECKED_PASS`, `CHECKED_FAIL`, and `ASSERTED`, all with `BUILD_GRAPH` freshness, and no file-internal flag can change that result

3. **Native files never self-certify**
   - Given a convention-located native surface with either a passing report, failing report, or no report
   - When it is resolved
   - Then provenance is always `ASSERTED` and the report cannot produce a certified status

4. **Native freshness is byte-only and best-effort**
   - Given unchanged readable dependency inputs, modified source bytes, and an unreadable or non-local dependency source set
   - When freshness is derived
   - Then the results are `VERIFIED`, `STALE`, and `UNKNOWN` respectively, and instrumentation observes no AST parse, type check, or dependency package load

5. **Artifact mismatches fail closed independently**
   - Given a missing surface or a surface with a wrong component, format, namespace, SDK field, interface style, package, symbol, or authority shape, or an invalid checked report
   - When resolution is attempted
   - Then a distinct exit-2-ready error identifies the failed invariant and no partial `DependencyInterface` is returned

6. **The additive boundary remains intact**
   - Given the new resolver and tests
   - When `Runner.runCheck` is inspected and `just ci` runs
   - Then production still uses the source-backed resolver, while the new resolver is fully unit-tested and ready for one atomic cutover

## Metadata
- **Complexity**: High
- **Labels**: go, surfaces, reports, provenance, freshness, namespace, sdk-key, artifact-validation
- **Required Skills**: Go API and enum design, deterministic hashing, filesystem seams, protobuf/JSON artifact validation, table-driven testing
