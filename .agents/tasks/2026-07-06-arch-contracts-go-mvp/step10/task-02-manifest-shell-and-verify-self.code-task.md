# Task: Manifest the shell and verify the tool

## Description
Complete the self-hosting graph with manifests and informal contracts for the parser and authority-holding shell, then run `arcc` across every tool component and minimize declarations to the authority actually observed.

## Background
FR8 is compositional: the pure core remains authority-free because loading, process execution, system-state access, and JSON reflection belong to explicit shell components. The `manifest` parser has a known `prototext` reflection caveat, so the MVP guarantees the narrower four-component pure showcase while declaring any authority the parser actually needs. Every dependency must independently conform for pruning to be trustworthy.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§2 FR8/FR10, §3, §4, §5.4)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 10)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/capslock.md` (self-hosting shell/core split and analysis limits)
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (validated classifier and minting semantics)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add colocated manifests for `go/internal/manifest`, `go/internal/goanalysis`, `go/internal/capslockadapter`, and `go/cmd/arcc`; the last root is one CLI component containing its `app` subpackage.
2. Declare exact component and absorbed dependencies from the real import graph, using relative manifest paths that resolve from each declaring root.
3. Start from the designed authority sets: `goanalysis` and `capslockadapter` may require `FILES`, `EXEC`, and `READ_SYSTEM_STATE`; CLI may require `FILES` and `REFLECT`; confirm and minimize every declaration against analyzer findings.
4. Keep the guaranteed authority-free claim scoped to `facts`, `checker`, `report`, and `capanalyzer`; declare `REFLECT` for `manifest` only if observed, and document the outcome accurately.
5. Complete FR10 contract comments in all newly selected interface files, covering behavior, requirements, guarantees, held authority, and caller obligations.
6. Run the built `arcc` against all eight tool manifests. All must exit 0; the four pure reports must be ambient-authority-free and shell/parser authority must be explicitly declared.
7. Do not weaken the strict capability policy or classifier to make self-hosting pass.

## Dependencies
- Task 1, which establishes the four authority-free core manifests and contracts.
- Completed Steps 6–9, which provide the final shell implementations and boundary pruning.

## Implementation Approach
1. Inventory package membership, exported declarations, imports, and authority findings for each remaining component.
2. Add exact manifests and complete their interface-file contract prose.
3. Build `arcc` and check every tool manifest, changing only truthful declarations or genuine implementation defects.
4. Record the parser reflection outcome and run the full repository CI suite.

## Acceptance Criteria

1. **The complete tool graph is manifested**
   - Given production packages under `go/internal` and `go/cmd/arcc`
   - When colocated manifests are enumerated
   - Then all eight designed components exist with correct disjoint roots and dependency edges.

2. **The pure-core guarantee is demonstrated**
   - Given the `facts`, `checker`, `report`, and `capanalyzer` manifests
   - When each is checked by `arcc`
   - Then each exits 0 and reports conformance as ambient-authority-free.

3. **Shell authority is explicit and least-privilege**
   - Given the `goanalysis`, `capslockadapter`, and CLI manifests
   - When each is checked
   - Then each exits 0 using only declarations justified by observed findings.

4. **The parser reflection caveat is resolved honestly**
   - Given strict analysis of the `manifest` component
   - When it is checked
   - Then it conforms with its exact observed authority and documentation scopes the guaranteed-pure showcase correctly.

5. **All tool interfaces carry informal contracts**
   - Given every interface file named by the tool manifests
   - When its comments are reviewed
   - Then the prose covers what the component does, requires, provides, and what authority it holds.

6. **The complete manual self-check stays green**
   - Given all eight manifests
   - When their checks and `just ci` run
   - Then every component conforms and the existing suite passes.

## Metadata
- **Complexity**: High
- **Labels**: self-hosting, shell, manifests, authority, FR8, FR10
- **Required Skills**: Go, Capslock capability analysis, component architecture, textproto, technical documentation
