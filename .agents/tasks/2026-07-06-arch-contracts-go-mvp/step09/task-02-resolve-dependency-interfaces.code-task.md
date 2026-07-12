# Task: Resolve dependency interfaces

## Description
Add `goanalysis.ResolveDependencyInterface` to turn each declared component dependency into its derived package membership and complete FR4 boundary symbol set.

## Background
Both FR5 and FR5b consume one shared `facts.DependencyInterface` primitive. It must be derived from the dependency's own manifest and Go types, rather than trusting package or symbol lists supplied by the declaring component.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§4.1, §5.3, §5.3b, §5.5, §7)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 9)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (receiver, generic, promoted-method, and init key formats)
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/go-component-model.md` (component-root and dependency model context)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Implement `ResolveDependencyInterface(declaringRoot, analyzedRoot, dep)` in `goanalysis`.
2. Resolve `dep.Manifest` relative to the declaring root, parse it with `manifest.Parse`, and treat its containing directory as the dependency component root.
3. Reject unreadable/invalid manifests, dependency name mismatches, invalid interface files, and roots overlapping the analyzed component as actionable tool errors.
4. Enumerate every Go package under the dependency root for `DependencyInterface.Packages`; do not accept declared package lists.
5. Derive symbols from declarations in the dependency's interface files, including exported top-level declarations, exported methods, and explicit init declarations in canonical normalized form.
6. For exported interface types declared in interface files, use `go/types` satisfaction and method sets to add methods of in-component concrete implementations; emit compatible pointer/value receiver forms and normalize generics.
7. Return deduplicated, deterministic packages and symbols. Keep per-package synthetic init prune keys an orchestration concern for Task 3.
8. Add integration fixtures for interface files split across `types.go`/`api.go`, interface/concrete implementations, generics, methods, init, mismatch, invalid paths, and root overlap.

## Dependencies
- Task 1's package-loading/SSA-compatible fact infrastructure.
- Manifest parsing, interface-file validation, and `facts.DependencyInterface` from earlier steps.

## Implementation Approach
1. Resolve and validate the dependency manifest and roots with explicit path-containment checks.
2. Reuse package loading and interface-file validation to derive membership and declared symbols.
3. Walk `go/types` named types and method sets to augment interface contracts with concrete implementation method keys.
4. Normalize and sort the result, then add success and error integration tests.

## Acceptance Criteria

1. **Dependency paths and identity resolve correctly**
   - Given a component dependency whose manifest path is relative to the declaring root and whose name matches
   - When it is resolved
   - Then its manifest directory becomes the dependency root; missing, malformed, or mismatched manifests return tool errors.

2. **Package membership is derived**
   - Given a dependency with multiple packages below its root
   - When resolution completes
   - Then all and only those loaded packages appear in `DependencyInterface.Packages` in stable order.

3. **Declared symbols come from interface files**
   - Given top-level declarations, methods split between `types.go` and `api.go`, generics, and explicit init declarations
   - When resolution completes
   - Then the normalized declared symbols are present and architecture-private declarations are absent.

4. **Interface implementations support dynamic dispatch**
   - Given an exported interface type in an interface file and in-component concrete implementations
   - When symbols are derived
   - Then the implementations' applicable concrete method keys, including compatible receiver forms, are in the boundary set.

5. **Invalid component topology is rejected**
   - Given a dependency root nested in or containing the analyzed component root
   - When resolution is attempted
   - Then an overlap error suitable for the CLI's exit-2 path is returned.

6. **Repository checks remain green**
   - Given the completed task
   - When `just ci` runs
   - Then all checks pass.

## Metadata
- **Complexity**: High
- **Labels**: goanalysis, dependency-interface, FR4, FR5, FR5b, go-types, integration
- **Required Skills**: Go, go/types, AST analysis, filesystem path safety, integration testing
