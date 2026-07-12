# Task: Build VTA call graph

## Description
Extend `goanalysis.LoadPackageFacts` to build a VTA call graph and emit deterministic, normalized call-edge facts, including whether a boundary call passes a function-typed value.

## Background
Step 6 deliberately left `PackageFacts.CallEdges` empty. Step 9 needs the same VTA algorithm Capslock uses so FR5 boundary enforcement and FR5b capability attribution reason over compatible call edges. The checker and fact types already support these facts.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§4.1, §5.3b, §11 and review A3/A4/A5 decisions)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 9)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/capslock.md` (Capslock call-graph construction and symbol keys)
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (validated VTA and key-normalization behavior)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Load the syntax, types, and dependency information needed to construct SSA for the component and use `vta.CallGraph`, matching Capslock's algorithm.
2. Populate `facts.PackageFacts.CallEdges` for relevant inter-package calls with caller and callee in `capanalyzer.InterfaceSymbol` key form.
3. Normalize generic instantiations by stripping type-argument brackets and produce method keys compatible with checker and Capslock matching.
4. Set `PassesFuncValue` when a call site passes any function-typed value, including named function types where applicable.
5. Deduplicate and deterministically sort emitted edges without changing the existing package/import/exported-symbol facts.
6. Add integration fixtures and tests for direct calls, dynamic dispatch, generic normalization, pointer/value receiver forms, and higher-order calls.

## Dependencies
- Step 6 `goanalysis` loader and Step 3 `facts.CallEdge` model.
- Pinned `golang.org/x/tools` SSA, callgraph, and VTA packages already used by Capslock.

## Implementation Approach
1. Extend package loading and build the SSA program for the loaded component graph.
2. Traverse VTA edges and their call sites, converting SSA functions to canonical interface-symbol keys.
3. Inspect call arguments for function-valued types, normalize, deduplicate, and sort facts.
4. Cover static, dynamic, generic, method, and callback cases with integration tests.

## Acceptance Criteria

1. **VTA edges populate package facts**
   - Given a component that calls functions and methods in another package
   - When `LoadPackageFacts` runs
   - Then `CallEdges` contains the expected caller-to-callee VTA edges.

2. **Dynamic calls resolve to concrete methods**
   - Given a call through an interface with an in-program concrete implementation
   - When the graph is built
   - Then the emitted callee identifies the concrete method VTA resolves.

3. **Keys match shared normalization rules**
   - Given generic functions and pointer/value receiver methods
   - When edges are emitted
   - Then generic brackets are stripped and method keys match the forms consumed by checker and Capslock.

4. **Higher-order calls are identified**
   - Given a call site that passes a function-typed value
   - When its edge is emitted
   - Then `PassesFuncValue` is true, while ordinary calls remain false.

5. **Output is stable and existing facts remain intact**
   - Given repeated loads of the same component
   - When results are compared
   - Then edges are deduplicated and deterministically ordered and all Step 6 facts remain correct.

6. **Repository checks remain green**
   - Given the completed task
   - When `just ci` runs
   - Then all checks pass.

## Metadata
- **Complexity**: High
- **Labels**: goanalysis, callgraph, VTA, FR5, higher-order, integration
- **Required Skills**: Go, go/packages, SSA, static call-graph analysis, integration testing
