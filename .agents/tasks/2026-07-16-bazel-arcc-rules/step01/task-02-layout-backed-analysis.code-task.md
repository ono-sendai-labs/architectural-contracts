# Task: Layout-backed analysis

## Description
Thread package-layout loading through arcc's structural and capability analyzers so explicit layout roots define component membership and all `packages.Load` calls use the hermetic driver environment while the existing colocated mode remains unchanged.

## Background
The package-layout driver is useful only when both analysis paths consistently use it. `goanalysis.LoadPackageFacts` currently discovers membership with `./...` under the manifest directory, while `capslockadapter` independently loads import paths and Capslock later issues its own `packages.Load(nil, "std")`. Layout mode must install one self-exec driver environment for the entire check, use layout roots instead of directory discovery, and interpret interface/source paths relative to the Bazel workspace. The actual `packages.Load`-based analysis algorithms should remain intact.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-16-bazel-arcc-rules/design/detailed-design.md` (§4.6, §5.1, §5.3, §5.4, and §9)
- Plan: `.agents/planning/2026-07-16-bazel-arcc-rules/implementation/plan.md` (Step 1)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/arcc-current-state.md` (current loading and colocation assumptions)
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver-findings.md` (Capslock `std`, SDK-source, and self-exec environment findings)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Introduce a loading context/configuration seam that can represent either the existing colocated directory mode or package-layout mode without changing the core `packages.Load` analysis algorithms.
2. In layout mode, configure `GOPACKAGESDRIVER` to self-exec arcc and pass the layout/driver marker environment through the initial `goanalysis` and `capslockadapter` loads and Capslock's nested `packages.Load(nil, "std")` call.
3. Avoid leaking or racing process-global driver state across independent checks/tests; if inherited environment must be scoped around Capslock's nested load, provide cleanup and serialize only the minimum critical section.
4. Load structural facts from the layout's explicit `roots` rather than `./...`; only member roots contribute component membership/facts, while their transitive graph remains available for type checking and call analysis.
5. Resolve and validate manifest `interface_files` against workspace-relative layout sources in layout mode, while preserving manifest-directory-relative validation in colocated mode.
6. Ensure dependency-interface resolution uses the same layout-backed source graph and workspace-relative path semantics when package-layout mode is active.
7. Preserve deterministic fact, symbol, edge, and finding ordering and provide contextual errors for empty roots, failed package loads, or layout/source mismatches.
8. Add unit/integration-level tests for explicit roots, transitive non-member packages, workspace-relative interfaces, dependency-interface resolution, Capslock `std` loading, and isolation between layout-backed and colocated invocations.

## Dependencies
- Task 1's validated package-layout model, query logic, and driver environment contract.
- Existing `goanalysis`, `capslockadapter`, `manifest`, and application dependency-injection seams.

## Implementation Approach
1. Model loading mode explicitly and adapt the structural loader and capability adapter constructors or requests to receive it.
2. Reuse the existing fact extraction, SSA/VTA, and Capslock code after selecting package patterns and configuring the driver environment.
3. Centralize layout-aware source normalization so fact extraction and interface validation compare the same workspace-relative names.
4. Exercise both loading modes in the same test suite to prove that the new behavior is additive.

## Acceptance Criteria

1. **Layout roots define membership**
   - Given a layout with member roots and additional transitive packages
   - When structural facts are loaded in layout mode
   - Then facts and component package requests are rooted in exactly the declared members, while dependencies remain available for types and call edges.

2. **Both analyzers use the self-exec driver**
   - Given layout mode from a working directory with no `go.mod`
   - When structural and capability analysis run
   - Then their `packages.Load` calls, including Capslock's `std` query, are satisfied by the configured driver and never fall back to `go list`.

3. **Workspace-relative interfaces validate**
   - Given a generated manifest outside the source tree whose `interface_files` identify layout sources workspace-relatively
   - When interface validation and dependency-interface resolution run
   - Then the correct exported symbols are selected without deriving paths from the manifest directory.

4. **Driver configuration is isolated**
   - Given a layout-backed check followed by a colocated check in the same test process
   - When both execute
   - Then the second check uses normal package loading and no stale driver environment or layout path remains.

5. **Colocated behavior is unchanged**
   - Given an existing manifest and no package-layout option
   - When facts, interfaces, and capabilities are analyzed
   - Then the existing `./...`, component-root, and `go list` behavior and results remain unchanged.

6. **Repository checks remain green**
   - Given the completed task
   - When `just ci` runs
   - Then all checks pass.

## Metadata
- **Complexity**: High
- **Labels**: Go, goanalysis, capslockadapter, explicit-membership, hermetic-loading
- **Required Skills**: Go, `go/packages`, Capslock, concurrency-safe environment handling, static-analysis testing
