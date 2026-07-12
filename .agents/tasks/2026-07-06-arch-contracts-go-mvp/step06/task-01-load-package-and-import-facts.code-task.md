# Task: Load package and import facts

## Description
Create the first increment of the authority-holding `internal/goanalysis` shell
package. Implement `LoadPackageFacts(componentRoot)` with `go/packages` so it
loads every Go package below the component root and returns deterministic,
core-defined package membership, direct-import, and standard-library facts.

This task establishes the real loader and its error contract. Exported-symbol
extraction and resolved interface-file validation are added by Task 2.

## Background
Step 6 begins the shell side of the pure-core/shell boundary. Component
membership is directory-based: the manifest directory is the component root and
all packages matched by `./...` beneath that directory belong to the component.
The shell may exercise `FILES`, `EXEC`, and `READ_SYSTEM_STATE`; it must return
plain `facts.PackageFacts` so the checker remains independent of `go/packages`.

The loader needs syntax, type, import, dependency, file, and module information
for this step and later call-graph work. Sharing a loaded graph with Capslock is
not required. `CallEdges` and `ExportedSymbols` remain empty in this increment.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§3 architecture and shell→core flow; §4.1 `goanalysis`/`facts` interfaces; §5.2 standard-library treatment; §8 integration-test strategy)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 6)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/go-component-model.md` (`go/packages` loading, component membership, and import extraction)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add the `golang.org/x/tools/go/packages` dependency and create
   `go/internal/goanalysis` with the public API
   `LoadPackageFacts(componentRoot string) (facts.PackageFacts, error)`.
2. Load pattern `./...` with `packages.Config.Dir` set to the supplied component
   root. Select a load mode that supplies names, files, compiled files, imports,
   dependencies, syntax, types, type information, and module metadata needed by
   Step 6 and the later Step 9 extension.
3. Treat every initially matched package as component membership, but do not add
   transitive dependency packages to `PackageFacts.Packages`.
4. For each member package, populate its import path, sorted direct-import paths,
   and `IsStdlib`. Determine standard-library status from package/module metadata,
   not from a dot-in-path heuristic. Keep `ExportedSymbols` empty and leave the
   aggregate `CallEdges` empty until subsequent work.
5. Detect all `packages.Load` and per-package errors. Return an error that retains
   the loader's diagnostic text; never return apparently successful partial facts
   for a broken package graph.
6. Normalize and sort all returned packages and imports for reproducible tests and
   reports. Do not expose `packages.Package` or any other shell type through the
   public API.
7. Add integration-tagged tests under `internal/goanalysis`, backed by compact
   fixture packages in `internal/goanalysis/testdata/`. Ensure they run through
   the existing `just test-integration` target and do not affect ordinary unit
   tests.

## Dependencies
- Step 3 `internal/facts` data model, especially `PackageFacts` and `PackageFact`.
- Existing `just test-integration` build-tag convention and Go module tooling.
- No dependency on `checker`, `manifest`, or Capslock is required in this task.

## Implementation Approach
1. Add a small loader helper that configures and invokes `packages.Load` at the
   component root and consolidates load diagnostics into an error.
2. Map only the root `./...` matches into `facts.PackageFact` values; derive and
   sort their direct imports and standard-library classification.
3. Create fixture packages containing an intra-component import, a standard-library
   import, and a non-stdlib dependency already available to the module.
4. Add integration tests for membership, direct imports, stdlib classification,
   determinism, invalid roots, and source/build errors.

## Acceptance Criteria

1. **Directory subtree defines membership**
   - Given a component root containing multiple nested fixture packages
   - When `LoadPackageFacts` loads it
   - Then each package under that root appears exactly once and dependency packages
     outside the root do not appear as component members.

2. **Direct imports are extracted**
   - Given fixture packages with standard-library, intra-component, and external
     direct imports
   - When facts are loaded
   - Then each `PackageFact.Imports` contains exactly that package's direct imports,
     in stable sorted order.

3. **Standard-library classification uses loader metadata**
   - Given loaded component and standard-library packages
   - When package facts are built
   - Then component packages have `IsStdlib == false`, and the implementation's
     classification helper correctly recognizes standard-library package metadata.

4. **Future fields remain empty**
   - Given a successful Step-6 Task-1 load
   - When the returned facts are inspected
   - Then `ExportedSymbols` and `CallEdges` are empty rather than containing
     placeholder or guessed data.

5. **Broken loads fail without partial success**
   - Given an invalid component root or a fixture with a package load/type error
   - When `LoadPackageFacts` runs
   - Then it returns a non-nil error containing the underlying loader diagnostic and
     does not present partial facts as a successful result.

6. **Integration suite is green and deterministic**
   - Given the repository checkout
   - When `just test-integration` is run repeatedly
   - Then the new fixture tests pass and return identically ordered facts.

## Metadata
- **Complexity**: Medium
- **Labels**: goanalysis, shell, go-packages, imports, FR1, FR3, integration-test
- **Required Skills**: Go, `golang.org/x/tools/go/packages`, Go integration testing
