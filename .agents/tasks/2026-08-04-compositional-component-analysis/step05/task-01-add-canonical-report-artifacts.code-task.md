# Task: Add Canonical Report Artifacts

## Description
Define the persisted report contract used by the new build topology: a deterministic JSON artifact with an explicit pass/fail verdict, plus a reader and `arcc verdict --expect pass|fail` assertion command. This separates analysis results from process exit status so later Bazel actions can stay green on policy violations while tests assert the recorded verdict.

## Background
Today `go/internal/report.ConformanceReport` is marshaled directly to stdout and the three Bazel check rules infer the verdict from `arcc check`'s exit code. Step 5 needs one `<name>.report.json` output that ordinary actions can publish and all check tests can consume. A policy violation is a successful analysis with verdict `fail`; malformed or unreadable reports remain tool errors. The dependency provenance axes are Step 7 work and must not be introduced here.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` — R8, N4, §Build topology, §Persisted formats, Appendix A

**Additional References:**
- Plan Step 5: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-01: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Current CLI/report path: `go/cmd/arcc/app/app.go`, `go/internal/report/report.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Give the persisted JSON report an explicit, validated `pass` or `fail` verdict derived from whether the completed report has violations. There must be one authoritative derivation; callers cannot stamp a contradictory verdict.
2. Add deterministic report marshal/decode helpers. The same logical report must produce byte-identical JSON, collection ordering must remain stable, malformed input and unknown verdicts must fail with actionable errors, and decoding must enforce a bounded input size.
3. Keep the pure `report` package free of filesystem and third-party dependencies. Put filesystem-backed reading/writing or CLI adapter behavior in the shell layer and reuse `artifactio.WriteFileAtomic` for eventual file output rather than duplicating atomic-write code.
4. Add `arcc verdict <report> --expect=pass|fail`. It exits 0 only when the report is valid and its verdict matches, exits 1 for a valid opposite verdict, and exits 2 for usage, open, decode, or validation errors. Diagnostics name both expected and actual verdicts on mismatch.
5. Preserve current text rendering and existing `arcc check` exit behavior in this task. `--report-out` and `--report-verdict-only` are integrated in Task 3.
6. Do not add Step 7 dependency provenance/freshness/authority axes or change checker policy decisions.
7. Update BUILD/component metadata and FR10 Component Contract blocks for changed package responsibilities.

## Dependencies
- Step 3 provides canonical artifact conventions and atomic file replacement in `go/internal/artifactio`.
- The existing checker produces the complete `ConformanceReport`; this task changes only how that result is persisted and asserted.
- Task 3 consumes this artifact API from `arcc check`; Task 6 consumes `arcc verdict` from Bazel tests.

## Implementation Approach
1. RED: pin verdict derivation, canonical byte stability, bounded decoding, malformed/contradictory input, and command exit semantics.
2. Add the smallest report artifact model/codec that keeps report rendering pure and gives consumers an explicit verdict.
3. Wire a dedicated `verdict` command parser into the CLI without coupling it to check execution.
4. Run focused Go tests, self-component parity, and the full CI gate.

## Acceptance Criteria

1. **Reports carry one trustworthy verdict**
   - Given completed reports with zero and non-zero violations
   - When each is encoded and decoded
   - Then its persisted verdict is respectively `pass` and `fail`, and no API permits a contradictory verdict to survive validation.

2. **Report bytes are deterministic and bounded**
   - Given logically identical reports with inputs observed in different orders
   - When they are canonicalized repeatedly
   - Then the output bytes are identical; malformed, oversized, or unknown-verdict input returns a specific tool error.

3. **The verdict command asserts report artifacts**
   - Given valid passing and failing report files
   - When `arcc verdict --expect=pass|fail` is run against matching and opposite expectations
   - Then matching exits 0, opposite exits 1 with expected/actual context, and invalid input or invalid usage exits 2.

4. **Existing check behavior is unchanged**
   - Given the existing CLI and Bazel check fixtures
   - When they run before Task 3's output flags are added
   - Then text/JSON rendering and exit 0/1/2 behavior are unchanged apart from the explicit verdict field in persisted JSON expectations.

5. **Architectural boundaries remain valid**
   - Given the new codec and command
   - When self-check and manifest parity run
   - Then the pure report package has gained no ambient authority and all changed Component Contracts and BUILD metadata are accurate.

## Metadata
- **Complexity**: Medium
- **Labels**: go, report, artifact, cli, determinism
- **Required Skills**: Go API design, deterministic JSON encoding, CLI exit-code design, table-driven testing
