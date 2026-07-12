# Task: CLI end-to-end golden tests

## Description
Complete Step 8 with process-level golden coverage for conforming CSV components,
undeclared dependencies and authority, malformed manifests, text/JSON rendering,
and exact exit-code semantics.

## Background
Unit tests establish individual seams, but the Step 8 milestone is the first real
shell-to-core verdict. These tests must invoke the compiled CLI (or an equivalent
subprocess entry point) with the real loader and Capslock adapter. The fixtures
include `absorbapp` only to show authority absorption; the conforming composition
root and pruning contrast remain Step 9 work.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§7 error outcomes, §8 end-to-end tests, §10 CSV example)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 8 tests, integration, and demo)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (expected clean and FILES analysis behavior)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add integration/golden infrastructure that executes `arcc check` with real filesystem, `goanalysis`, Capslock, stdout/stderr separation, and process exit status. Keep slower cases under the repository's integration convention.
2. Assert the canonical `toprow` manifest exits `0` and reports conformance plus ambient-authority-free status.
3. Assert the canonical `csvfile` manifest exits `0` despite a real `FILES` finding because its manifest declares that authority.
4. Create an isolated failing `toprow` fixture whose manifest omits its `parsecsv` absorbed dependency; assert exit `1` and an actionable `UNDECLARED_DEPENDENCY` finding.
5. Create an isolated `absorbapp` fixture that absorbs `csvfile` or directly opens a file while declaring no authority; assert exit `1`, `UNDECLARED_AUTHORITY`, and preserved Capslock path evidence.
6. Add malformed-manifest cases, including an unknown capability name, and assert exit `2`, stderr diagnostics, and no conformance output.
7. Run a representative report with `--format=json`, decode it into `report.ConformanceReport`, and compare semantic content with text-mode expectations rather than relying on unstable whitespace.
8. Use stable golden normalization for unavoidable absolute paths/line details while retaining finding kinds, component names, dependency/capability names, and evidence order.
9. Wire the suite into `just test-integration` and ensure `just ci` exercises it.

## Dependencies
- Step 8 Task 1's production CLI and error/output contract.
- Step 8 Task 2's canonical `toprow` and `csvfile` examples.
- Existing integration build-tag and `just` conventions.

## Implementation Approach
1. Build a reusable subprocess harness that captures stdout, stderr, and exit codes.
2. Add canonical passing cases, then small self-contained failure fixtures.
3. Normalize environmental path details and check semantic golden content.
4. Exercise both output formats and run the entire CI aggregate.

## Acceptance Criteria

1. **Ambient-authority-free component passes end to end**
   - Given the canonical `toprow` manifest
   - When the real CLI checks it from the shell
   - Then it exits `0` and prints a conforming ambient-authority-free report.

2. **Declared authority passes end to end**
   - Given the canonical `csvfile` manifest
   - When the real CLI checks it
   - Then it exits `0` and does not report its declared `FILES` use as a violation.

3. **Undeclared imports are conformance failures**
   - Given a `toprow` fixture that imports `parsecsv` without declaring absorption
   - When checked
   - Then it exits `1` with `UNDECLARED_DEPENDENCY` naming the relevant import.

4. **Absorbed filesystem authority is surfaced**
   - Given `absorbapp` with no authority declaration and an absorbed/direct filesystem path
   - When checked
   - Then it exits `1` with `UNDECLARED_AUTHORITY`, `FILES`, and actionable call-path evidence.

5. **Malformed manifests are tool errors**
   - Given syntactically invalid input or `declared_authority: "FILE"`
   - When checked
   - Then it exits `2`, writes the cause to stderr, and never emits a passing report.

6. **Machine-readable output round-trips**
   - Given `--format=json` for a representative check
   - When stdout is decoded into `report.ConformanceReport`
   - Then component, violations, warnings, locations, and evidence retain their expected semantic values.

7. **Golden output is deterministic across environments**
   - Given repeated runs from the supported repository checkout
   - When outputs are compared after narrowly scoped path normalization
   - Then finding order and all architecture-relevant content are stable.

8. **Step 8 demo and CI pass**
   - Given the finished step
   - When `just ci` and the documented shell demo are run
   - Then all suites pass; `toprow` demonstrates exit `0`, while undeclared-dependency and undeclared-authority fixtures demonstrate exit `1`.

## Metadata
- **Complexity**: High
- **Labels**: cli, end-to-end, golden-test, FR2, FR7, FR9, integration-test
- **Required Skills**: Go, subprocess testing, Capslock integration, golden-test maintenance
