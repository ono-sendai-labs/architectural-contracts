# Task: Implement capability analysis adapter

## Description
Implement `internal/capslockadapter` as the Capslock-backed realization of
`capanalyzer.CapabilityAnalyzer`. Load the requested packages, run Capslock with
the strict object-capability classifier from Task 1, map structured results into
deterministic core finding values, and verify whole-package analysis behavior with
integration fixtures.

After this task the shell can inject real capability findings into the checker,
completing Step 7's fact-production seam for the Step 8 CLI.

## Background
The pure core owns the analyzer port and knows nothing about Capslock. This shell
adapter may exercise `FILES`, `EXEC`, and `READ_SYSTEM_STATE` while loading Go
packages and constructing SSA/VTA analysis, but it returns only
`capanalyzer.CapabilityFinding` values.

Capslock reports every queried-package function on a path to a capability. That
native whole-package scope is intentional: architecture-private or otherwise
unreachable shipped code still counts, while `_test.go` files are absent from the
ordinary analyzed build. Raw transitivity also provides absorbed-dependency
authority attribution. Step 7 accepts `PruneAt` only as forward-compatible input;
actual `CAPABILITY_SAFE` boundary pruning belongs to Step 9.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§3 shell→core data flow; §4.1 analyzer port; §5.4 whole-package authority scope; §5.4a classifier; §8 integration tests)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 7)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/capslock.md` (`LoadPackages`, `GetQueriedPackages`, `GetCapabilityInfo`, proto fields, taxonomy, and absorption)
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (whole-package and `_test.go` scope evidence; expected probe behavior)
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike/harness.go` (validated library invocation and result-flattening example)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add an adapter type with a compile-time assertion that it implements
   `capanalyzer.CapabilityAnalyzer`, and implement
   `Analyze(capanalyzer.AnalyzeRequest) ([]capanalyzer.CapabilityFinding, error)`.
2. Honor every import path/pattern in `AnalyzeRequest.Packages`. Reject an empty
   package request with a descriptive error rather than accidentally analyzing an
   implicit working-directory package.
3. Load packages with Capslock's required load mode and configuration, collect
   errors across the loaded graph, and return diagnostics without partial-success
   findings. Use Capslock as an in-process library.
4. Define queried packages from the request's top-level loads, configure function
   granularity, use Task 1's strict object-capability classifier, and call
   `analyzer.GetCapabilityInfo` once per request. Preserve Capslock's native
   transitivity and whole-package scope; do not add entry-point filtering.
5. Accept `AnalyzeRequest.PruneAt`; an empty value means no boundary pruning. If a
   non-empty value is supplied before Step 9, return a clear unsupported-input
   error rather than silently claiming that pruning occurred.
6. Map each Capslock `CapabilityInfo` into `capanalyzer.CapabilityFinding`:
   package identifier, normalized capability name, `Class`, and every example
   path frame's function, file, and line. Preserve evidence for both direct and
   transitive findings.
7. Map true ambient-authority capabilities to `capanalyzer.TrueAuthority` and
   analysis-defeating capabilities (`ARBITRARY_EXECUTION`, `CGO`,
   `UNSAFE_POINTER`, `REFLECT`, and any surfaced `UNANALYZED`) to
   `capanalyzer.AnalysisDefeating`. Reject unknown/control capability values with
   a descriptive compatibility error so Capslock upgrades cannot silently weaken
   classification.
8. Return an empty, non-error finding slice for an ambient-authority-free package.
   Sort findings and call-path representation deterministically without discarding
   distinct Capslock findings.
9. Add integration-tagged fixtures and tests under
   `internal/capslockadapter/testdata`: file minting yields a `FILES` finding with
   a plausible path; pure arithmetic yields none; authority in an exported helper
   unreachable from the component interface is reported; equivalent authority
   only in `_test.go` is not reported. Include direct assertions for the mapped
   class and frame fields.
10. Ensure the adapter tests run under the existing `just test-integration`
    target. Log the Step 7 demo findings in verbose test output without making
    output text itself brittle.

## Dependencies
- Step 7 Task 1's pinned Capslock dependency and classifier helper.
- Step 3 `internal/capanalyzer` request, finding, frame, class, and interface types.
- Existing integration-test build-tag convention and repository `just` targets.
- No dependency on `checker`, `manifest`, `goanalysis`, or the future CLI is
  required.

## Implementation Approach
1. Introduce the adapter type and small helpers for validated package loading,
   capability-class mapping, frame mapping, and stable sorting.
2. Invoke Capslock with top-level queried packages, function granularity, and the
   Task 1 classifier, then flatten its protobuf output into core-owned values.
3. Build compact testdata packages for minting, pure arithmetic, private exported
   authority, and test-only authority scenarios.
4. Add integration tests for behavior, error propagation, mapping completeness,
   scope, determinism, and the verbose demo.

## Acceptance Criteria

1. **Adapter satisfies the core port**
   - Given the `capslockadapter` implementation
   - When the Go compiler checks its interface assertion
   - Then it implements `capanalyzer.CapabilityAnalyzer` without exposing Capslock
     types through the port.

2. **Requested packages are analyzed in process**
   - Given one or more valid package paths in `AnalyzeRequest.Packages`
   - When `Analyze` runs
   - Then it loads and queries those packages through Capslock's library API using
     function granularity and the configured classifier.

3. **Filesystem minting maps to evidence-rich findings**
   - Given a fixture function that reads a path using `os.ReadFile` or opens one
   - When the adapter analyzes the fixture
   - Then at least one `FILES` finding is returned as `TrueAuthority` with the
     fixture package and a plausible ordered call path containing function, file,
     and positive source-line evidence.

4. **Pure packages are ambient-authority-free**
   - Given a fixture containing only pure arithmetic
   - When the adapter analyzes it
   - Then it returns no findings and no error.

5. **Whole-package scope includes architecture-private code**
   - Given a regular `.go` fixture containing an authority-using exported helper
     that is unreachable from the declared interface
   - When its package is analyzed
   - Then the helper's authority is reported without any adapter entry-point filter.

6. **Test files are excluded by construction**
   - Given equivalent authority-using code that exists only in a fixture's
     `_test.go` file
   - When the ordinary package build is analyzed
   - Then that code contributes no capability finding.

7. **Capability taxonomy is mapped safely**
   - Given true-authority, analysis-defeating, and unknown/control capability enum
     cases at the mapping seam
   - When mapping occurs
   - Then known values receive the correct `capanalyzer.Class`, while unsupported
     values fail explicitly.

8. **Unsupported pruning is not silently ignored**
   - Given a non-empty `AnalyzeRequest.PruneAt` before Step 9
   - When `Analyze` runs
   - Then it returns a clear error indicating that boundary pruning is not yet
     implemented.

9. **Failures and ordering are deterministic**
   - Given broken package requests or repeated successful analyses
   - When `Analyze` runs
   - Then loader/compatibility failures retain useful diagnostics and successful
     findings have stable ordering with no partial-success result.

10. **Step 7 integration demo passes**
    - Given the repository checkout
    - When `just test-integration` runs (or the adapter suite runs verbosely)
    - Then the file-reading fixture logs a `FILES` finding and call path, the pure
      fixture logs `ambient-authority-free (no findings)`, and all assertions pass.

## Metadata
- **Complexity**: High
- **Labels**: capslockadapter, capslock, shell, capability-analysis, FR6, FR7, integration-test
- **Required Skills**: Go, Capslock analyzer/protobuf APIs, `go/packages`, static analysis testing
