# Task: Emit Check Artifacts from the CLI

## Description
Extend `arcc check` with `--report-out`, `--surface-out`, and `--report-verdict-only`, and make one analysis invocation atomically publish the canonical report and exact surface artifacts required by Bazel and native workflows.

## Background
The current runner prints a report and maps policy violations to exit 1. Tasks 1 and 2 provide the two artifact producers. The ordinary Bazel analysis action needs policy violations to be successful executions so downstream artifacts exist, while usage, loading, analysis, key-validation, and write failures must still fail as tool errors. Surface creation also needs the exact target SDK key. Under Bazel the Step 4 stdlib map is the declared, configuration-keyed source of that identity; native mode may use Step 4 target discovery/cache services. The map is used only for identity in this step—stdlib authority classification remains on the old check path until Step 6.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` — N2–N4, §Build topology, §Changed: Bazel rules and CLI, §Error Handling

**Additional References:**
- Plan Step 5: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-01: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- CLI orchestration: `go/cmd/arcc/app/app.go`, `go/cmd/arcc/main.go`
- Step 4 map reader/cache: `go/internal/artifactio/stdlibmapreader.go`, `go/internal/stdlibmap/cache.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Parse `--report-out=<path>`, `--surface-out=<path>`, and `--report-verdict-only` as single-occurrence `check` options with precise empty, duplicate, and missing-value diagnostics. Keep the existing manifest, layout, and output-format validation.
2. `--report-out` writes Task 1's canonical report JSON atomically. `--surface-out` writes Task 2's canonical surface JSON atomically after the manifest/interface validation and member fact load have succeeded.
3. Establish one injected SDK-key resolver for check orchestration. In Bazel/layout mode it must validate and use the declared Step 4 stdlib-map artifact (or an equivalent exact-key input contract); in native mode it may use Step 4 target discovery/cache. A missing, corrupt, format-mismatched, classifier-mismatched, or target-mismatched key is exit 2. Do not use map symbol classifications in checker decisions until Step 6.
4. With `--report-verdict-only`, require a report artifact destination, return 0 for both pass and fail after analysis and artifact publication, and retain exit 2 for every tool/usage/write error. Without it, preserve the existing exit 0/1/2 contract.
5. Preserve human text and `--format=json` stdout behavior for ordinary native invocations. The artifact output is canonical and independent of display format.
6. Ensure report and surface bytes are deterministic across repeated unchanged invocations. Write failures must be contextualized with the target path and must never expose a partial file.
7. Update CLI usage, README/user-facing CLI documentation, BUILD/component metadata, and FR10 Component Contracts. Explicitly document that emitted surfaces omit implements-closure injection while the Step 5 check still applies the workaround until Step 6.

## Dependencies
- Task 1 provides canonical report encoding and verdict semantics.
- Task 2 provides exact surface derivation, digesting, and native companion-path conventions.
- Step 4 provides map artifacts, readers, target identity, native cache/discovery, and the Bazel default map seam.
- Task 4 consumes this command in the ordinary Bazel analysis action.

## Implementation Approach
1. Refactor check option parsing and execution results so display, persisted output, and exit policy are separate decisions.
2. RED: parser/error matrix and pass/fail/tool-error exit matrix with injected loader, analyzer, key resolver, and artifact writer seams.
3. GREEN: generate both artifacts from the single existing analysis result and publish via canonical atomic I/O.
4. Add subprocess/integration coverage for layout-mode key validation, deterministic bytes, and native output paths.
5. Run the full CI gate and self-check.

## Acceptance Criteria

1. **One check emits both artifacts**
   - Given a valid component, package layout, and matching target SDK identity
   - When `arcc check` runs with `--report-out` and `--surface-out`
   - Then it writes a valid canonical report and exact canonical surface from that same analysis invocation.

2. **Verdict-only mode separates policy from execution**
   - Given one conforming and one violating component
   - When each runs with `--report-verdict-only` and a report destination
   - Then both executions exit 0, their reports respectively record `pass` and `fail`, and any usage, analysis, key, or write error exits 2.

3. **Ordinary CLI compatibility is preserved**
   - Given existing text and JSON invocations without `--report-verdict-only`
   - When they run
   - Then stdout formatting and pass/violation/tool-error exit codes remain 0/1/2.

4. **SDK identity fails closed without changing Step 5 verdicts**
   - Given missing, corrupt, classifier-mismatched, format-mismatched, and target-mismatched map/key inputs
   - When surface emission is requested
   - Then each fails with exit 2 and actionable context; a valid map contributes only the surface SDK key and does not replace Capslock or alter the current check findings.

5. **Artifact writes are deterministic and atomic**
   - Given two unchanged invocations and an injected mid-write failure
   - When outputs are compared
   - Then successful bytes are identical and the failed write leaves any previous target intact.

6. **Option validation is actionable**
   - Given duplicate, empty, partial, or incompatible artifact options
   - When parsed
   - Then the command exits 2, names the offending option, and does not run analysis.

7. **The temporary surface/check disagreement is documented**
   - Given generated CLI help and developer documentation
   - When Step 5 behavior is described
   - Then it states that surfaces are exact while the check retains implements-closure widening only until Step 6.

## Metadata
- **Complexity**: High
- **Labels**: go, cli, report, surface, sdk-key, atomic-io
- **Required Skills**: Go CLI orchestration, dependency injection, artifact I/O, error semantics, integration testing
