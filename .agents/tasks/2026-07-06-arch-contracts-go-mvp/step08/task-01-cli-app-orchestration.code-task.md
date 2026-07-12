# Task: CLI app orchestration

## Description
Replace the `arcc` stub with an injected application layer that runs the first full
manifest-to-report check: read and parse a manifest, load package facts, analyze
ambient authority, invoke the pure checker, render text or JSON, and return the
documented three-way exit status.

## Background
Steps 2–7 provide the pure core and authority-holding fact producers. Step 8 joins
them under `cmd/arcc/app`, keeping all filesystem access, Capslock execution,
terminal I/O, and JSON reflection in the CLI shell. Call-graph facts, dependency
interface resolution, and boundary pruning remain Step 9 work, so this task sends
an empty `PruneAt` and no resolved component interfaces.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§3–§4.1 orchestration, §5.4 authority policy, §6.2 JSON ownership, §7 errors, §8 tests)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 8)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (minting-not-use classifier and whole-package scope)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add an orchestration package under `cmd/arcc/app` with injected seams for package loading and `capanalyzer.CapabilityAnalyzer`; production wiring uses `goanalysis.LoadPackageFacts` and `capslockadapter.NewAdapter`.
2. Parse `arcc check <component.textproto>` and `--format=json` in supported positions, retain `--version` and useful usage output, and reject unknown commands/options or wrong arity as tool errors.
3. Open/read the manifest in the CLI shell, pass an `io.Reader` to `manifest.Parse`, and derive the component root from the cleaned manifest path's directory.
4. Load facts for that root, validate every declared interface file with `goanalysis.ValidateInterfaceFiles`, and pass all loaded component package import paths to the analyzer with empty `PruneAt`.
5. Start from `capanalyzer.StrictPolicy`; rely on `checker.Check` to merge `Manifest.DeclaredAuthority` into the allowed set. Pass no `DepIfaces` until Step 9.
6. Render text through `report.RenderText`. Marshal the report with `encoding/json` only in the shell, using stable readable output and a trailing newline.
7. Return exit `0` for no violations (warnings remain successful), `1` for violations, and `2` for argument, manifest, filesystem, package-load, interface-file, analyzer, or JSON errors. Tool errors go only to stderr and must not print a conforming report.
8. Keep `main` thin and unit-test orchestration using injected fakes, including call order/request values, declared-authority policy behavior, formatting, and all three outcomes.

## Dependencies
- Completed Steps 2–7: `manifest`, `facts`, `checker`, `report`, `capanalyzer`, `goanalysis`, and `capslockadapter`.
- Step 1 CLI stub and test conventions.
- Step 9 owns dependency-interface resolution, call graphs, and non-empty pruning.

## Implementation Approach
1. Define a small app runner/configuration with injected loader and analyzer seams.
2. Implement manifest acquisition, fact validation/loading, analyzer request construction, checker invocation, and output selection.
3. Adapt `main.run` to argument parsing and production dependency wiring.
4. Add focused unit tests with temporary manifests and deterministic fakes.

## Acceptance Criteria

1. **A component check is fully orchestrated**
   - Given a valid manifest and successful injected shell dependencies
   - When `arcc check` runs
   - Then its root-derived facts and whole-package capability findings reach `checker.Check` and the resulting report is printed.

2. **Authority declarations affect conformance**
   - Given an analyzer `FILES` finding
   - When the manifest declares `FILES`
   - Then the report conforms; when it omits `FILES`, the report contains `UNDECLARED_AUTHORITY`.

3. **Analyzer scope is correct for Step 8**
   - Given loaded facts for multiple component packages
   - When analysis is requested
   - Then every component package is included and `PruneAt` is empty.

4. **Output formats are owned by the correct layers**
   - Given the same report in text and JSON modes
   - When each mode runs
   - Then text uses the pure report renderer and JSON round-trips to `report.ConformanceReport` from shell-side marshaling.

5. **Exit statuses distinguish outcomes**
   - Given conforming, violating, and operational-error scenarios
   - When each check completes
   - Then statuses are respectively `0`, `1`, and `2`, warnings alone remain `0`, and tool errors appear on stderr without a success report.

6. **Resolved manifest validation is enforced**
   - Given a missing or out-of-component interface file
   - When the CLI checks the manifest
   - Then it exits `2` with an actionable diagnostic before capability analysis.

7. **CLI compatibility and invalid input are tested**
   - Given version, help/usage, unknown option, unknown command, and bad-arity invocations
   - When argument parsing runs
   - Then existing version behavior remains valid and invalid invocations produce deterministic usage diagnostics and exit `2`.

8. **Repository checks remain green**
   - Given the completed task
   - When `just ci` runs
   - Then all build, unit, integration, generation, formatting, and vet checks pass.

## Metadata
- **Complexity**: High
- **Labels**: cli, orchestration, shell, FR2, FR6, FR9, json
- **Required Skills**: Go, dependency injection, CLI design, error handling, unit testing
