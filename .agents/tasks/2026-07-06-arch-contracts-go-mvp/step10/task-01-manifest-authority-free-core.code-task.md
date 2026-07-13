# Task: Manifest the authority-free core

## Description
Add self-hosting manifests for the four guaranteed-pure core components and complete their informal interface contracts so the repository explicitly records the architecture that FR8 claims.

## Background
The MVP's central claim is that conformance policy can be isolated from ambient authority. `facts`, `checker`, `report`, and `capanalyzer` form that guaranteed authority-free showcase: shell code produces or consumes their plain values, while these components perform no filesystem, process, environment, or reflection work. Their component roots are separate package directories, and their manifests must follow the real inward dependency graph.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§2 FR8/FR10, §3, §4)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 10)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `component.textproto` at the roots of `go/internal/facts`, `go/internal/checker`, `go/internal/report`, and `go/internal/capanalyzer`.
2. Give each manifest the correct component name, interface files, component dependencies, absorbed dependencies, and an empty `declared_authority` set based on the actual package import graph.
3. Keep component roots disjoint and express links between core packages as component dependencies rather than absorbed dependencies.
4. Review every interface file named by these manifests and make its package/type/function doc comments state what the component does, what inputs or invariants it requires, what it provides, and that it holds no ambient authority.
5. Preserve existing public API behavior; this task records manifests and completes FR10 prose rather than redesigning the core.
6. Add focused parsing or structural tests if needed to catch malformed manifests or drift in their key declarations before the full self-hosting runner is introduced.

## Dependencies
- Completed Steps 2–5 and 9, which provide the manifest schema and the final pure-core interfaces/import graph.
- No Step 10 task dependency; this is the first task in the step.

## Implementation Approach
1. Inventory exported declarations and imports for each pure package and select the interface files that expose its architectural surface.
2. Author the four colocated manifests with empty authority and exact component edges.
3. Complete the informal contract prose in the selected interface files without changing behavior.
4. Parse the manifests and run the repository checks to establish a green atomic change.

## Acceptance Criteria

1. **Every guaranteed-pure component has a valid colocated manifest**
   - Given the `facts`, `checker`, `report`, and `capanalyzer` package roots
   - When their `component.textproto` files are parsed
   - Then all four are valid, name existing interface files, and have disjoint roots.

2. **The manifests encode the real core dependency graph**
   - Given the non-standard-library imports of the four components
   - When their dependency declarations are compared with the code
   - Then each cross-component import is represented by the correct component dependency and no stale grant is present.

3. **The showcase grants no ambient authority**
   - Given each of the four core manifests
   - When its authority declarations are inspected
   - Then `declared_authority` is empty and no shell authority is hidden in an absorbed dependency.

4. **Interface contracts satisfy FR10**
   - Given every interface file listed by the four manifests
   - When its documentation is reviewed
   - Then it states the component's behavior, requirements, provided guarantees, and absence of ambient authority.

5. **The atomic change remains green**
   - Given the new manifests and documentation
   - When `just ci` runs
   - Then all generation, formatting, build, unit, and integration checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: self-hosting, manifests, pure-core, FR8, FR10
- **Required Skills**: Go architecture, textproto, API documentation, manifest modeling
