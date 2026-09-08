# Task: Enforce Declaring-Object Boundaries

## Description
Implement the AST + `types.Info` scanner, exact dependency-interface classification, and delete the implements-closure workaround that currently authorizes concrete implementors in this same atomic change. This establishes the new FR5 semantics before the production pipeline is cut over.

## Background
The legacy `ResolveDependencyInterface` expands declared interface types through `types.Implements`, then injects matching concrete methods into the allowed symbol set. That workaround compensated for VTA call edges, which name dynamically resolved implementations rather than the object written in source. This task lands the scanner that names the `go/types` declaring object and deletes the compensating workaround in the same commit, so no revision has the new scanner paired with closure-expanded dependency symbols.

Dependency surfaces are not consumed yet. This task must continue using the existing source-loading `ResolveDependencyInterface`; only its symbol semantics change. `PACKAGE_SURFACE` dependencies still authorize every external object in their owned packages.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R4, Reference semantics, Object references, regression fixtures 3-5)

**Additional References:**
- `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md` (the implements-closure workaround and why the typed scan makes it unnecessary)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Implement the design API `ScanReferences(pkgs []*packages.Package, members MemberSet)` in `goanalysis`, or an equivalently small API consistent with existing style. Inspect only member packages even though `NeedDeps` still populates the closure.
2. Walk both `types.Info.Uses` and `types.Info.Selections`, converting every external object through the Step 3 declaring-object rule. Deduplicate overlapping Uses/Selections only per semantic site, preserve distinct sites, and sort output deterministically.
3. Emit one `ImportEdge` for every import declaration in member syntax, including named, dot, and blank imports. Handle builtins, labels, nil-package objects, incomplete type info, and malformed imports deliberately; errors must not become partial facts.
4. Change resolved dependency symbol facts from the legacy `capanalyzer.InterfaceSymbol`/dual-receiver convention to shared `symbol.SymbolID`. Declared-interface extraction returns exactly the exported declaring objects in surviving `interface_files`: functions, methods, types, fields, interface method specs, variables, constants, aliases, and all other design grammar objects.
5. Delete the complete implements-closure computation from `ResolveDependencyInterface` (the plan identifies the former `goanalysis.go:1619-1714` region), including helpers, caches, comments, fixtures, and imports that exist only for `types.Implements` expansion. This deletion and scanner implementation must be one commit.
6. Add the pure boundary decision for `ReferenceEdge`: ignore members; locate a uniquely owning direct dependency by package; authorize all symbols for `PACKAGE_SURFACE`; for declared interfaces compare exact declaring-object `SymbolID`; otherwise produce site-specific `CALLS_UNDECLARED_INTERFACE` or `UNDECLARED_DEPENDENCY`.
7. Treat direct-package ownership as validated. If two resolved direct dependencies own the same package, return a deterministic `DEPENDENCY_OVERLAP` tool error before building a lookup; do not preserve last-wins behavior.
8. Count declared and auto-attached dependency edges as uses while retaining edge provenance. Step 7 status/surface consumption remains out of scope.
9. Keep compatibility scaffolding narrowly additive if the legacy capability path still needs `CallEdges` before Task 05, but use the typed scanner for the new FR5 fixture path and never repopulate or consult an implements closure.
10. Remove Step 5's temporary divergence comments from the resolver and surface package once the exact-source symbol set is shared; defer CLI/README cleanup to Task 05.

## Dependencies
- Task 01: typed `ReferenceEdge`, source sites, and deterministic scanner output.
- Plan Step 3: `symbol.SymbolID` declaring-object conversion.
- Existing Step 5 surface derivation, whose exact symbol set is the reference behavior.

## Implementation Approach
1. Build the member-only scanner over loaded syntax/type info and the Task 01 facts.
2. Convert `facts.DependencyInterface.Symbols` and resolution helpers to canonical `SymbolID` values.
3. Remove implementor discovery in that same change and prove the resolved source interface equals Step 5 surface extraction.
4. Add an isolated reference-boundary classifier consumed by tests now and by `Runner` at Task 05.
5. Build real source fixtures for interface dispatch, concrete implementation access, and the full reference-kind table across declared and package-surface dependencies.

## Acceptance Criteria

1. **Every typed reference and import is observed**
   - Given member source containing function values, all design reference kinds, normal imports, and blank imports plus loaded transitive non-member syntax
   - When `ScanReferences` runs
   - Then it emits canonical site-bearing edges only from member source, sees non-call references and every written import, and excludes closure-originating observations

2. **Uses and selections deduplicate per site**
   - Given a selector represented in both `Uses` and `Selections`
   - When the scanner runs repeatedly over nondeterministic type-info map order
   - Then the declaring object appears once at that site, remains distinct at another site, and complete output order is deterministic

3. **Interface dispatch uses the named declaration**
   - Given design fixture 3, where member code invokes `dep.Greeter.Greet()` and the runtime implementation is unexported
   - When the typed reference is classified against a declared-interface dependency listing `Greeter`
   - Then the check passes because the source names the declared interface object, with no dynamically discovered implementor in the allowed set

4. **Concrete implementation access is rejected**
   - Given design fixture 4, where source directly reaches an implementation method that is not in the dependency interface
   - When the typed reference is classified
   - Then it produces `CALLS_UNDECLARED_INTERFACE` at the exact site even if the concrete type implements a listed interface; this fixture must demonstrate the precision correction from today's false pass

5. **The reference-kind matrix is exact**
   - Given design fixture 5 covering field, var, const, type, alias, embedded/promoted member, generic instantiation, pointer/value selection, and duplicate `Uses`/`Selections`
   - When each reference is checked against a declared-interface dependency and a `PACKAGE_SURFACE` dependency
   - Then every declared-interface outcome matches the declaring-object table, every package-surface reference is authorized, and duplicate observations do not duplicate findings

6. **Source resolution and emitted surfaces agree**
   - Given one dependency loaded by the existing `ResolveDependencyInterface` and derived by the Step 5 surface package
   - When their package and symbol sets are compared
   - Then the exact sets are equal and neither contains a symbol admitted only through `types.Implements`

7. **Scanner and workaround deletion are atomic**
   - Given the single implementation commit produced for this task
   - When its diff and resulting code are inspected
   - Then it both introduces the typed scanner and removes all implementor-closure helpers/`types.Implements` expansion, while scanner/boundary fixtures pass before Task 05 changes the full production capability path

8. **Overlapping owners fail deterministically**
   - Given two resolved direct dependencies that claim the same package
   - When the boundary index is constructed
   - Then classification stops with a `DEPENDENCY_OVERLAP` tool error naming the package and both components, independent of dependency order

9. **Each commit remains green**
   - Given the temporary coexistence of legacy execution and the new classifier
   - When `just ci` runs
   - Then all tests pass without restoring the workaround or consuming persisted surfaces

## Metadata
- **Complexity**: High
- **Labels**: go, ast, go-types, reference-scan, symbol-id, dependency-boundary, fr5, precision, keystone
- **Required Skills**: Go AST traversal, `go/types`, `go/packages`, interface/method-set semantics, pure-core design, regression fixture construction
