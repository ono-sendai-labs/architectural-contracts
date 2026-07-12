# Task: Policy-aware ambient-authority rule (FR6)

## Description
Complete the pure `internal/checker` with the **ambient-authority rule (FR6,
design §5.4)**. `Check` consumes the injected `Caps []capanalyzer.CapabilityFinding`
and `Policy capanalyzer.CapabilityPolicy` (unused until now), derives the effective
policy by merging the manifest's `declared_authority` into `Policy.Allowed`, and
classifies each capability finding:

- capability ∈ effective `Allowed` → **no report entry**;
- capability ∈ `Warn` → non-fatal `ALLOWED_WITH_WARNING` / `ANALYSIS_LIMITATION`
  **warning**;
- otherwise → `UNDECLARED_AUTHORITY` **violation**, carrying the Capslock example
  call path (`CapabilityFinding.CallPath`) as `Evidence`.

Under the MVP default `StrictPolicy()` both policy sets start empty, so any residual
capability not covered by `declared_authority` fails. After this task,
`checker.Check` is the **complete, fully unit-tested pure function** — every rule
family (FR3/FR4/FR5/FR6) composed in one call, with no Go build and no Capslock in
the loop (the self-hosting showcase in miniature).

`checker` remains a **pure function of injected data**; real capability findings
arrive from the `capslockadapter` shell in Steps 7/8, and here every test runs on
hand-built `CapabilityFinding`s and policies (design §8).

## Background
This is the second and final rule family of Step 5, building on task-01's
call-boundary rule. It activates the two `Inputs` fields (`Caps`, `Policy`) that
prior steps carried but did not consume. The **policy seam** (`declared_authority`
→ `Allowed`) is what lets the example's `csvfile` legitimately declare and hold
`FILES`; the richer "capability box" (design §2.3) is explicitly out of scope.

FR6 mechanics (design §5.4):
- Effective policy: `Allowed ⊇ Policy.Allowed ∪ Manifest.DeclaredAuthority`, with
  `Warn` taken from `Policy.Warn`. (`capanalyzer.Classify(cap, policy)` already
  encodes the allow-wins/warn/violation precedence — reuse it against the effective
  policy rather than re-implementing precedence.)
- Findings are **already pruned** at component-dependency boundaries (FR5b, wired in
  Step 9), so authority owned by a dependency does not appear in `Caps` here — only
  authority the component reaches on its own or through **absorbed** dependencies.
- The `Class` field (`TrueAuthority | AnalysisDefeating`) is preserved on findings so
  a non-strict policy can distinguish them without re-running analysis; the MVP rule
  treats both identically (a non-allowed capability is a violation).
- Warnings alone keep the report in a passing state (exit 0, design §7 / review C14);
  only `UNDECLARED_AUTHORITY` (and the other violation kinds) fail.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§5.4 authority rule FR6 — analysis scope, policy derivation, per-finding classification; §5.4a classifier context for why findings are attributed at the minting site — background, not implemented here)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 5)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend `internal/checker` only — no new package, no new imports beyond the
   existing pure-core set. `checker` must remain pure (no I/O, no Capslock import;
   the port `capanalyzer` types only).
2. Derive the **effective policy**: copy `Policy.Allowed`, then add every name in
   `Manifest.DeclaredAuthority`; carry `Policy.Warn` through unchanged. Do not mutate
   the caller's `Policy` maps (build a fresh effective policy so `Check` stays pure /
   side-effect-free).
3. For each `Caps` finding, decide via `capanalyzer.Classify(finding.Capability,
   effectivePolicy)`:
   - `DecisionAllowed` → no entry;
   - `DecisionWarn` → append an `ALLOWED_WITH_WARNING` (or `ANALYSIS_LIMITATION`)
     **warning** naming the capability and package;
   - `DecisionViolation` → append an `UNDECLARED_AUTHORITY` **violation** naming the
     capability and package, with `Evidence` populated from the finding's `CallPath`
     frames (a readable rendering of caller → … → privileged callee).
4. Append to the same `report.ConformanceReport` the earlier rules build; do not
   change FR3/FR4/FR5 behavior. Keep findings **deterministic** (stable sort) so
   golden renderings are reproducible.
5. Document, in doc comments, that this task implements FR6, that the MVP default is
   `StrictPolicy()` merged with `declared_authority`, that findings are pre-pruned at
   dependency boundaries (Step 9), and that `Check` is now feature-complete for
   FR3/FR4/FR5/FR6.

## Dependencies
- Task-01 (FR5) and Step 4 `internal/checker` — extended here.
- Step 3 `internal/capanalyzer` (`CapabilityFinding`, `Frame`, `Class`,
  `CapabilityPolicy`, `StrictPolicy`, `Classify`, `Decision*`), `internal/report`
  (`UndeclaredAuthority`, `AllowedWithWarning`, `AnalysisLimitation` kinds, `Finding`,
  `Location`, `Evidence`), `internal/manifest` (`DeclaredAuthority`).

## Implementation Approach
1. Add a helper that builds the effective policy (`Policy.Allowed` ∪
   `Manifest.DeclaredAuthority`; `Policy.Warn` passthrough) without mutating inputs.
2. Add a helper rendering `[]capanalyzer.Frame` into `[]string` evidence lines.
3. Iterate `Caps`, classify via `capanalyzer.Classify`, and append the matching
   violation/warning.
4. Fold the new findings into the existing deterministic sort.
5. Table-driven unit tests on hand-built `Caps` + policies (no build, no Capslock —
   design §8) covering the cases below, plus at least one composite golden report.

## Acceptance Criteria

1. **Undeclared authority under StrictPolicy → violation with evidence**
   - Given a `FILES` `CapabilityFinding` with a non-empty `CallPath`, an empty
     `declared_authority`, and `StrictPolicy()`
   - When `Check` runs
   - Then exactly one `UNDECLARED_AUTHORITY` violation is emitted for `FILES` with the
     call path rendered into `Evidence`.

2. **Declared authority → no violation**
   - Given the same `FILES` finding but `declared_authority: ["FILES"]`
   - When `Check` runs
   - Then no `UNDECLARED_AUTHORITY` entry is emitted for `FILES` (the policy seam:
     `csvfile` legitimately holds `FILES`).

3. **Warn-set capability → warning, exit-neutral**
   - Given a capability present in `Policy.Warn` (not in `Allowed`)
   - When `Check` runs
   - Then an `ALLOWED_WITH_WARNING`/`ANALYSIS_LIMITATION` warning is emitted and the
     report carries no violation from it (warnings-only → passing).

4. **Allow wins over warn**
   - Given a capability listed in both `Allowed` (or `declared_authority`) and `Warn`
   - When `Check` runs
   - Then it produces no entry (allow precedence, matching `capanalyzer.Classify`).

5. **Class preserved, both classes fail strict**
   - Given two findings, one `TrueAuthority` and one `AnalysisDefeating`, under
     `StrictPolicy()`
   - When `Check` runs
   - Then both yield `UNDECLARED_AUTHORITY` violations (MVP treats both as failures).

6. **Purity + no input mutation**
   - Given a shared `Policy` value reused across calls
   - When `Check` runs
   - Then the caller's `Policy.Allowed`/`Warn` maps are unmodified and repeated calls
     yield identical reports.

7. **Feature-complete composite report**
   - Given hand-built inputs producing a boundary violation (FR5), an
     authority-violation-with-evidence (FR6), and a clean-pass variant
   - When `Check` runs
   - Then the rendered reports match the expected golden text, and a fully-conforming
     input produces an empty report (no violations, no warnings).

## Metadata
- **Complexity**: Medium
- **Labels**: pure-core, checker, FR6, authority, policy
- **Required Skills**: Go
