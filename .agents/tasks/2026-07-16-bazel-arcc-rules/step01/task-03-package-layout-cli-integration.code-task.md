# Task: Package layout CLI integration

## Description
Expose package-layout mode through `arcc check`, wire hidden self-exec driver dispatch in the production binary, and prove the complete hermetic workflow with command-level fixtures and exit-code coverage.

## Background
Tasks 1 and 2 establish the driver and analysis seams. This task makes them a supported CLI mode: `arcc check <manifest> --package-layout=<layout>`. The same binary must detect driver invocations before normal argument parsing, while ordinary checks, selfcheck usage, version output, and colocated manifests continue to behave exactly as before. The integration harness must demonstrate correct checks from a directory with no `go.mod`, including a real `FILES` call path.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-16-bazel-arcc-rules/design/detailed-design.md` (§4.6, §6.2, §8.4, and §9)
- Plan: `.agents/planning/2026-07-16-bazel-arcc-rules/implementation/plan.md` (Step 1 demo and tests)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver-findings.md` (validated self-exec behavior and negative control)
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver/testdata/` (seed fixtures)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `--package-layout=<file>` to `arcc check`, accept it in a documented deterministic argument position, reject missing/duplicate/empty values, and update usage output.
2. Resolve the layout path and the workspace-relative source base needed by the layout before changing analysis behavior; report open, parse, validation, and path failures as tool errors with exit 2.
3. Wire the production runner so the option selects Task 2's layout-backed structural loader, capability adapter, interface validation, and dependency resolution; absence of the option retains the current injected defaults.
4. Add hidden startup dispatch in `cmd/arcc` for `GOPACKAGESDRIVER` self-execution, reading the driver request from stdin and writing only the protocol response to stdout; diagnostics go to stderr with nonzero exit.
5. Ensure driver mode is selected only by the private marker environment and cannot be confused with a user-facing subcommand.
6. Add hermetic fixtures derived from the spike for a clean component, a component that mints `FILES`, and an inconsistent layout naming a missing source file.
7. Extend the existing compiled-binary integration harness to launch arcc with a working directory outside every Go module and confirm no `go list`/module fallback is possible.
8. Cover exit 0 for a conforming component, exit 1 plus a `FILES` finding and call path for the violating component, and exit 2 plus an actionable diagnostic for a missing layout source.
9. Exercise the driver's `std` query through real Capslock analysis, not only through a protocol unit test, and retain existing CLI/integration coverage for `--version`, JSON output, colocated manifests, and invalid options.

## Dependencies
- Task 1's package-layout parser and driver protocol handler.
- Task 2's layout-backed analysis configuration and workspace-relative path semantics.
- Existing `cmd/arcc/app` unit tests and `cmd/arcc` integration harness.

## Implementation Approach
1. Extend argument parsing and runner configuration with an optional layout path while keeping the established exit-code and output boundaries.
2. Dispatch driver mode at the earliest main-package entry point, before constructing or invoking the normal CLI runner.
3. Adapt spike fixtures into stable repository testdata and generate any machine-specific SDK paths during test setup rather than checking them into golden JSON.
4. Run the built binary from a fresh module-less directory and assert user-visible verdicts, diagnostics, and call paths.

## Acceptance Criteria

1. **The new CLI mode is usable**
   - Given a valid manifest and package layout
   - When `arcc check <manifest> --package-layout=<layout>` runs
   - Then arcc selects layout-backed loading and emits the normal text or JSON conformance report.

2. **A clean hermetic component passes**
   - Given the clean fixture and a working directory with no `go.mod`
   - When the compiled arcc binary checks it
   - Then the command exits 0 and reports conformance without invoking `go list`.

3. **A `FILES` violation is reported with evidence**
   - Given a layout-backed component whose public function reaches `os.Open` without declaring `FILES`
   - When the compiled arcc binary checks it from the module-less directory
   - Then the command exits 1 and its report contains the `FILES` violation and call path.

4. **Missing layout sources are tool errors**
   - Given a layout naming a source file absent from disk
   - When the check runs
   - Then the command exits 2 and stderr identifies the missing source and its package.

5. **Capslock's `std` query works end to end**
   - Given either hermetic fixture
   - When capability analysis executes
   - Then the self-exec driver serves the nested `std` query from SDK sources and analysis completes without a module or Go tool fallback.

6. **Existing CLI behavior is preserved**
   - Given no `--package-layout` option
   - When existing colocated checks, version/help commands, JSON formatting, and invalid-option cases run
   - Then their outputs and exit-code semantics remain unchanged.

7. **Step 1 demo is reproducible**
   - Given the documented clean and violating fixtures
   - When the demo command is run from a directory with no `go.mod`
   - Then swapping only the fixture inputs produces the expected passing verdict or `FILES` finding with its call path.

8. **Repository checks remain green**
   - Given the completed task
   - When `just ci` runs
   - Then all checks pass.

## Metadata
- **Complexity**: High
- **Labels**: Go, CLI, integration-test, GOPACKAGESDRIVER, hermetic-loading
- **Required Skills**: Go, CLI design, subprocess integration testing, `go/packages`, Capslock
