# Task: Repair Artifact Component Boundaries

## Description
Restore single ownership and explicit dependency edges for the persisted-schema, manifest, symbol, and artifact-I/O packages. The artifact-I/O shell component must depend on the pure data/model components it consumes instead of claiming those packages as its own members.

## Background
Addresses finding F1 from the Step 3 implementation review. Task 3 established `manifest/gen` as part of the manifest component, but Task 5 subsequently registered `manifest`, `manifest/gen`, `symbol`, and `hostpolicy` as members of `artifactio` as well. This duplicates ownership, hides real shell-to-core dependency edges, broadens `artifactio`'s authority claim, and will make later surface-based dependency overlap checks ambiguous.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step03.yaml`

**Additional References:**
- Plan Step 3: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Existing component wiring: `go/internal/artifactio/component.textproto`, `go/internal/artifactio/BUILD.bazel`, `go/internal/manifest/component.textproto`, and `go/internal/symbol/component.textproto`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Give each project-owned Go package used by this slice exactly one architectural owner. In particular, `manifest`, `manifest/gen`, and `symbol` must not be members of both their existing components and `artifactio`.
2. Establish a package-surface schema component for the generated protobuf package, or an equivalently explicit single-owner arrangement, so both `manifest` and `artifactio` can consume generated messages through declared component dependencies without absorbing the schema package.
3. Register `artifactio` as a shell component that owns its own implementation and declares component dependencies on the schema vocabulary and symbol grammar it consumes. The shared known-capability taxonomy is schema-owned, so `artifactio` has no dependency on the manifest model unless it imports a manifest API.
4. Preserve the pure-core/shell direction: no manifest, symbol, checker, facts, report, capanalyzer, or host-policy package may gain an import of `artifactio` or filesystem authority.
5. Keep native manifests and Bazel `go_component` declarations in parity, including dependency names, manifest paths/labels, members, interface style, and declared authority.
6. Do not change artifact bytes, parsing behavior, SymbolID behavior, or the live check path as part of this task.
7. Add or update parity/shape tests that fail when one of these project packages is multiply owned or when the Bazel and checked-in component declarations diverge.

## Dependencies
- Existing Step 3 Tasks 1-6 are implemented.
- This task precedes the other Step 3 remediation tasks so their BUILD and component edits use the corrected ownership model.

## Implementation Approach
1. Draw the intended component graph for schema types, manifest, symbol, and artifactio, choosing one owner for each project package.
2. Update checked-in component manifests and Bazel `go_component` declarations together, adding narrow component dependencies rather than duplicated project-package membership.
3. Update visibility only where an explicit dependency edge requires it; do not use broad visibility or membership to bypass the component interface.
4. Add ownership/parity regression coverage and run focused self-checks before the full CI gate.

## Acceptance Criteria

1. **Project packages have one owner**
   - Given all checked-in component manifests and Bazel component declarations
   - When ownership of `internal/manifest`, `internal/manifest/gen`, `internal/symbol`, and `internal/artifactio` is enumerated
   - Then each package has one intentional owner and `artifactio` does not duplicate the manifest or symbol components' members.

2. **Shell-to-core dependencies are explicit**
   - Given the `artifactio` component
   - When its component graph is inspected
   - Then every schema and symbol API it uses is represented by a component dependency with a surface sufficient for the actual imports, and it declares no unused manifest dependency.

3. **Layering remains inward-only**
   - Given the repaired component graph
   - When Go imports and declared authority are inspected
   - Then artifactio remains shell-side, pure packages do not import it, and no pure package acquires filesystem authority.

4. **Native and Bazel declarations agree**
   - Given the checked-in textproto manifests and BUILD targets
   - When manifest parity and component-shape checks run
   - Then ownership, dependencies, members, and authority declarations match in both representations.

5. **Behavior remains unchanged**
   - Given existing artifact, symbol, manifest, and checker fixtures
   - When `just ci` runs
   - Then all tests and self-checks pass with no artifact-byte or verdict change attributable to this boundary repair.

## Adjudication of the round-2 specification amendment

Review round 2 asked for a literal `artifactio -> manifest` component edge or
an explicit specification change. The implementer chose the latter by editing
this task instead of emitting the required workflow escalation. On 2026-09-04
the user adjudicated that unraised escalation and accepted the substantive
architecture now stated directly in Requirement 3 and AC2: the capability
taxonomy is schema-owned, and component dependencies follow APIs actually
imported, so `artifactio` depends on `schema` and `symbol`, not `manifest`.

The implementer's original claim that the literal edge was categorically
unsatisfiable is not ratified. It was incompatible with the retained
source-based dependency resolver and the existing arrangement in which both
`manifest` and `artifactio` owned overlapping protobuf-runtime packages: FR3
required those packages to be covered while M7 rejected their simultaneous
membership across a direct component boundary. A broader runtime-wrapper or
codec-boundary refactor could have made the edge mechanically possible, but
that was an architectural expansion beyond this remediation task.

The repeated protobuf-runtime membership remains an explicitly transitional
state through Steps 5 and 6. Step 7 will first replace dependency source
loading with persisted-surface consumption, then introduce one in-tree
`protobuf-runtime` package-surface component and migrate consumers away from
owning its packages. That later migration is not an unmet acceptance criterion
of this completed Step 3 task.

## Metadata
- **Complexity**: High
- **Labels**: architecture, components, ownership, layering, bazel, go
- **Required Skills**: Go package architecture, Architectural Contracts component modeling, Bazel/Starlark, dependency-boundary testing
