# Task: Wire boundary pruning

## Description
Connect resolved dependency interfaces to checker boundary enforcement and Capslock capability pruning, making component dependencies account for their own authority while absorbed dependencies remain attributed to the analyzed component.

## Background
The checker already implements FR5 over injected edges and dependency interfaces, while the CLI currently supplies neither and the adapter rejects non-empty `PruneAt`. This task closes that integration seam using the same resolved symbol set for both architectural pillars.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§3.1, §5.3b, §5.4a, §5.5 and review A2/A3)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 9)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/capslock.md` (custom maps and classifier merging)
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (`CAPABILITY_SAFE` pruning proof and init key formats)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend the CLI application seams and production wiring to resolve every direct component dependency before analysis; failures remain tool errors with exit 2.
2. Pass resolved interfaces to `checker.Inputs.DepIfaces` so real VTA edges drive FR5 violations and higher-order warnings.
3. Build `AnalyzeRequest.PruneAt` from every dependency interface symbol plus `func <package>.init` for every derived dependency package; normalize and deduplicate deterministically.
4. Do not prune absorbed dependencies.
5. Remove the adapter's Step-9 guard and load the generated per-run `CAPABILITY_SAFE` entries through `interesting.LoadClassifier(..., excludeBuiltin=false)` so they merge with builtins and the minting-not-use overrides.
6. Safely encode/validate symbol keys in the generated classifier and return malformed-key or classifier-load failures explicitly.
7. Add adapter and orchestration tests proving interface and init pruning, checker inputs, name/resolution errors, deterministic requests, and absorbed-dependency non-pruning.

## Dependencies
- Task 1 VTA call edges and Task 2 dependency-interface resolver.
- Existing Step 5 checker boundary rule, Step 7 classifier configuration, and Step 8 injected CLI runner.

## Implementation Approach
1. Add a dependency resolver seam to the runner and resolve declarations after loading the analyzed component.
2. Derive one canonical prune set and feed it to both the analyzer request and checker inputs as appropriate.
3. Enable adapter custom-map pruning while preserving all existing classifier overrides.
4. Test the orchestration with fakes and the adapter with capability-bearing fixtures behind interface and init prune points.

## Acceptance Criteria

1. **One dependency primitive feeds both pillars**
   - Given resolved direct component dependencies and real call edges
   - When the CLI orchestrates a check
   - Then the same dependency interfaces reach `checker.Check` and form the analyzer prune set.

2. **Declared interfaces and init are pruned**
   - Given dependency interface symbols and packages
   - When the analyzer request is built
   - Then it includes every normalized symbol plus `func <pkg>.init` once in deterministic order.

3. **Capslock honors per-run pruning**
   - Given authority reachable only behind a pruned dependency interface or dependency init
   - When the adapter analyzes the caller
   - Then that authority is absent while builtin classification and adapter safety overrides still apply.

4. **Absorbed authority remains visible**
   - Given the same authority-bearing package declared as absorbed rather than as a component dependency
   - When the caller is analyzed
   - Then no prune entries are generated for it and its authority is attributed to the caller.

5. **Boundary findings use real facts**
   - Given a call into a dependency's undeclared exported function or a declared higher-order interface call
   - When the integrated checker runs
   - Then it emits `CALLS_UNDECLARED_INTERFACE` or `HIGHER_ORDER_BOUNDARY_CALL`, respectively.

6. **Resolution and classifier failures are tool errors**
   - Given an unresolvable dependency or invalid prune key
   - When the CLI check runs
   - Then it exits 2 with an actionable diagnostic and no conformance report.

7. **Repository checks remain green**
   - Given the completed task
   - When `just ci` runs
   - Then all checks pass.

## Metadata
- **Complexity**: High
- **Labels**: orchestration, capslockadapter, pruning, FR5, FR5b, FR7
- **Required Skills**: Go, dependency injection, Capslock classifiers, static analysis, unit and integration testing
