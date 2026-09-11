# Task: Report Unused Authority

## Description
Add the `UNUSED_AUTHORITY` warning to the pure checker so every authority capability declared by a component but not exercised by its scanned member code is reported deterministically. Audit arcc's own component manifests against the new warning and tighten declarations that the completed reference-analysis pipeline proves unnecessary.

## Background
The checker currently widens its effective capability policy with every `declared_authority` entry, then applies that policy to the aggregated authority findings. It does not compare the manifest declaration with the capabilities actually observed by the reference scan, so stale declarations remain invisible. Step 10 closes that gap as a warning, analogous to `UNUSED_DEPENDENCY`, without changing the component verdict.

Exercise must be derived from the scan's classified true-authority observations, including package-init authority, before policy application hides allowed declarations. An `AnalysisDefeating` observation carries no known capability and therefore cannot prove that any declared capability was exercised; it must neither suppress a genuine unused warning nor cause all declarations to be treated as used.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R12, DR-17, changed `checker`, and the `UNUSED_AUTHORITY` error-handling row)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `report.UnusedAuthority` with persisted value `UNUSED_AUTHORITY` to the warning kinds. It must remain a warning, so a report containing only unused-authority findings has a passing verdict and renders through the existing deterministic warning path.
2. In `checker.Check`, compute exercised authority from the capabilities present in the aggregated `TrueAuthority` findings produced from the typed reference/import scan. Perform this comparison independently of whether the effective policy allows, warns on, or rejects the observed capability.
3. Diff the manifest's `DeclaredAuthority` against the exercised set and emit exactly one `UNUSED_AUTHORITY` warning for each declared but unexercised capability. Match the concise shape of `UNUSED_DEPENDENCY`: name the unused declaration, carry no fabricated source location/evidence/site, and participate in the checker's normal deterministic warning ordering.
4. Do not treat `AnalysisDefeating` findings from `CAPABILITY_UNANALYZED`, `//go:linkname`, assembly, or cgo as evidence that any named declaration was exercised. Such findings must retain their existing policy result while genuine unused declarations are reported alongside them.
5. Preserve exact-declaration behavior for authority reached through either a symbol reference or package initialization: every actually exercised declared capability suppresses only its matching unused warning, including when one scan finding exposes multiple capabilities or multiple sites exercise the same capability.
6. Add focused pure-core tests covering an over-declared `FILES` capability, an exact declaration, multiple declarations with a deterministic unused subset, repeated observations, package-init authority, policy allow/warn interactions, and coexistence with an analysis-defeating finding. Assert warning kind/message, absence of synthetic location data, and passing verdict where there are no violations.
7. Update report rendering/artifact tests and any intentional report-shape goldens affected by the new warning kind. Do not weaken complete warning comparisons or normalize the semantic kind away.
8. Audit the checked manifests for arcc's own `go/cmd` and `go/internal` components using the production self-check path. Remove declarations shown to be unused, retain capabilities supported by scan evidence, and keep `authority: UNKNOWN` wrapper components unchanged. If an intentionally broad declaration must remain, document the concrete reason at the declaration site.
9. Preserve the pure-core boundary: no new shell inputs, filesystem access, logging, third-party dependencies, protobuf fields, or secondary analysis pass may be introduced. The feature must consume the already classified scan results from the existing single `checker.Check` execution.

## Dependencies
- Step 9 must be complete so warning output and any affected report snapshots use the final verdict-versus-shape golden structure.
- No other Step 10 task dependency; this task delivers the warning, its tests, and the required self-component audit as one atomic change.

## Implementation Approach
1. Extend the report warning vocabulary and add focused assertions that `UNUSED_AUTHORITY` remains non-failing and renders consistently.
2. After `AggregateAuthority` and before final sorting, collect capabilities from `TrueAuthority` findings, compare them with `Manifest.DeclaredAuthority`, and append one warning per missing exercise without coupling the comparison to policy output.
3. Exercise the logic with table-driven checker tests for exact, over-declared, repeated, init-time, policy, and analysis-defeating cases.
4. Run arcc's production self-check over its component manifests, remove only declarations the new scan reports as unused, update intentional shape fixtures if necessary, and verify the full repository gate.

## Acceptance Criteria

1. **Over-declared authority is visible without failing conformance**
   - Given a component that declares `FILES` and has no `FILES` authority observation
   - When `checker.Check` evaluates it
   - Then the report contains exactly one location-free `UNUSED_AUTHORITY` warning naming `FILES`, and the warning alone leaves the verdict as `pass`.

2. **Exact declarations produce no unused warning**
   - Given a component whose declared capabilities exactly match true authority exercised by symbol references or package initialization
   - When the component is checked under strict, allow, or warn policy as applicable
   - Then none of those declarations produces `UNUSED_AUTHORITY`, regardless of how policy classifies the already exercised capability.

3. **The diff is per capability and deterministic**
   - Given several sorted or unsorted declarations, repeated authority observations, multiple sites, and a finding that carries multiple capabilities
   - When the same inputs are checked repeatedly
   - Then each unexercised declaration appears once, each exercised declaration is absent, and warning order and serialized output are byte-identical across runs.

4. **Analysis-defeating findings do not mask stale declarations**
   - Given a component that declares `FILES` but has only an analysis-defeating observation from an unanalyzed stdlib symbol or member bypass
   - When the component is checked under strict or explicitly downgraded policy
   - Then its existing analysis-defeating violation or warning is preserved and `FILES` is also reported as unused.

5. **Existing warning behavior remains intact**
   - Given a component with unused dependencies, dependency-status warnings, authority-policy warnings, and unused authority
   - When the report is rendered and persisted
   - Then every warning kind remains present, `UNUSED_AUTHORITY` uses the standard warning representation, and the final ordering is deterministic.

6. **Arcc's own declarations are audited**
   - Given the checked component manifests under `go/cmd` and `go/internal`
   - When the production self-check runs with the new rule
   - Then no unexplained `UNUSED_AUTHORITY` warning remains; obsolete declarations have been removed and unknown-authority wrappers are unchanged.

7. **Repository checks pass**
   - Given the implementation, tests, manifest audit, and any intentional shape-golden updates
   - When `just ci` runs
   - Then all Go, Bazel, self-check, manifest-parity, generation-cleanliness, and golden checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: go, checker, authority, warnings, self-check
- **Required Skills**: Go, pure-core rule implementation, table-driven testing, deterministic report handling, component-manifest auditing
