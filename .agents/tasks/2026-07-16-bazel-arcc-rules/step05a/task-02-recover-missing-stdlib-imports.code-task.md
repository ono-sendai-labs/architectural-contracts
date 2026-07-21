# Task: Recover missing standard-library imports

## Description
Recover standard-library dependencies omitted by build-system package layouts by parsing each layout-provided package's selected Go sources, then prove that arcc can analyze a component from a minimal layout with no standard-library records or edges and no Go toolchain on `PATH`.

## Background
Task 1 makes SDK packages available to the driver, but a Bazel-emitted component package still has no `Imports` entry for imports such as `os` or `strings`: rules_go's direct archive metadata excludes the standard library. `go/packages` resolves imports through the driver's flat graph, so the driver must fill these missing edges before validation. The layout remains authoritative for every edge it does provide, and imports that are neither layout-known nor discoverable standard-library packages must continue to fail as graph errors rather than being guessed.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-16-bazel-arcc-rules/design/detailed-design.md` (§4.6, §5.4, §6.2, and §8.4)
- Plan: `.agents/planning/2026-07-16-bazel-arcc-rules/implementation/plan.md` (Step 5a)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver-findings.md` (validated driver loading and findings 1–2)
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver/testdata/` (fixture patterns for clean and `FILES`-minting packages)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Parse imports from each layout-provided package's build-selected source set with `go/parser` in `parser.ImportsOnly` mode after source paths are safely resolved and existence-checked.
2. Normalize and unquote import literals with actionable package-and-file diagnostics for invalid source or import syntax; process files and imports in sorted order.
3. For each parsed import missing from the package's existing `Imports` map, add an edge only when the import path resolves to a standard-library package discovered by Task 1 and satisfies the package-layout standard-library classification.
4. Preserve every layout-provided import mapping unchanged, including a provided mapping for a standard-library path; do not replace or reinterpret producer-supplied edges.
5. Leave non-standard missing imports for the existing graph validation and driver error behavior; do not synthesize third-party or component packages from source text.
6. Ensure import recovery applies to the layout-provided component/dependency records that need Bazel's omitted stdlib edges and composes deterministically with Task 1's discovered SDK graph.
7. Add unit tests for multiple files, duplicate imports, blank/dot/aliased imports, missing stdlib edges, already-provided stdlib edges, non-standard missing imports, malformed source/import literals, and stable recovered-map behavior.
8. Rework or extend `go/cmd/arcc/layout_integration_test.go` fixtures so the tested package layout names only member/dependency packages while their sources import at least `os` and `strings`; do not manually place standard-library packages or edges in the layout.
9. Run the end-to-end cases through the existing `runArccHermetic` harness from a directory with no `go.mod` and with no Go toolchain reachable on `PATH`: a conforming component exits 0, and a `FILES`-minting component exits 1 with the authority finding and call path.
10. Keep the change confined to `go/internal/packagelayout` and the existing layout-mode integration fixtures/tests; do not alter `goanalysis`, `capslockadapter`, CLI semantics, the layout schema, or colocated mode.

## Dependencies
- Task 1 in this step: SDK standard-library discovery, merge precedence, and exact/`std` driver serving.
- Step 1's `runArccHermetic` compiled-binary integration harness and package-layout fixtures.

## Implementation Approach
1. Add a focused helper that parses the resolved source files for layout-provided packages and returns sorted import paths with package/file context on errors.
2. Use Task 1's merged standard-library index as the allowlist for synthesized edges, inserting only absent mappings and preserving producer data.
3. Invoke recovery at a preparation stage where source paths have been validated and before final graph consistency checks or driver responses require the new edges.
4. Simplify the integration fixture layout so it contains only the component graph, points at an SDK source tree, and relies entirely on enumeration and import recovery for `os`, `strings`, and their transitive standard-library closure.
5. Assert both the clean result and the `os.Open` capability path under the no-module/no-toolchain harness, alongside focused unit error cases.

## Acceptance Criteria

1. **Missing standard-library edges are recovered**
   - Given a layout-provided package whose selected sources import `os` and `strings` but whose `Imports` map omits both
   - When the layout is prepared
   - Then deterministic edges to the discovered `os` and `strings` package IDs are added before the driver graph is served.

2. **Producer-provided edges remain authoritative**
   - Given a layout package with an existing import mapping, including an existing standard-library mapping
   - When source imports are recovered
   - Then the mapping is not overwritten, and duplicate source imports do not produce duplicate or unstable graph data.

3. **Only known standard-library imports are synthesized**
   - Given source imports that include a missing third-party path and a path classified as standard library but absent from the discovered SDK
   - When recovery runs
   - Then neither edge is guessed, and the established validation/query path returns a clear deterministic error for the incomplete graph.

4. **Parse failures identify their source**
   - Given a layout source with malformed Go or an invalid import literal
   - When imports are parsed in imports-only mode
   - Then preparation fails with a tool error identifying the package and source file.

5. **A stdlib-free layout checks cleanly without the Go toolchain**
   - Given a component whose member imports `os` and `strings`, a layout naming only its own package graph, a valid SDK source root, no `go.mod`, and no `go` executable on `PATH`
   - When `arcc check <manifest> --package-layout=<layout>` runs through `runArccHermetic`
   - Then the driver supplies the recovered standard-library graph and arcc exits 0 for a conforming declaration.

6. **A recovered `FILES` path is reported end to end**
   - Given the same minimal layout and a member function that calls `os.Open` without declaring `FILES`
   - When the hermetic check runs
   - Then arcc exits 1 and reports the `FILES` finding with a call path from the member function to `os.Open`.

7. **Existing modes and repository checks remain green**
   - Given the completed task
   - When package-layout unit/integration tests, colocated-mode tests, and `just ci` run
   - Then all checks pass without changes to non-layout behavior.

## Metadata
- **Complexity**: High
- **Labels**: Go, package-layout, import-recovery, standard-library, hermetic-integration
- **Required Skills**: Go, `go/parser`, `go/packages`, graph validation, subprocess integration testing, hermetic test design
