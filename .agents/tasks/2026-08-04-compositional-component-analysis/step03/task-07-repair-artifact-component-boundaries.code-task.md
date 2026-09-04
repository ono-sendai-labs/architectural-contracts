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
3. Register `artifactio` as a shell component that owns its own implementation and necessary shell/runtime closure while declaring component dependencies on the manifest model, symbol grammar, and schema surface it consumes.
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
   - Then its uses of schema, manifest, and symbol APIs are represented as component dependencies with surfaces sufficient for the actual imports.

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

## Specification amendment (R2, recorded per review round 2)

Review round 2 asked for the literal `artifactio -> manifest` component edge
or an explicit specification change. This paragraph records that change.

The literal edge is unsatisfiable under the live checker's semantics, which
Requirement 6 forbids changing: the manifest component must own the protobuf
runtime closure because its `prototext` member imports it (checker FR3 sweeps
declared members' imports in the native leg), and the checker's member-overlap
rule (M7) rejects any package covered by a dependency also being a member in
the Bazel layout leg — while `artifactio` must be a member of the protobuf
runtime packages it imports (`proto`, `protojson`), which overlaps that
closure. Requirement 6 also forbids changing artifact bytes and parsing
behavior, ruling out runtime changes that would avoid the overlap.

Amended requirement 3 and AC2 therefore read: `artifactio` is a shell
component that owns its own implementation and its protobuf runtime closure,
declaring component dependencies on the schema surface and the symbol grammar
it consumes; the known-capability taxonomy is schema-owned
(`schema.KnownCapabilities`), shared by the manifest model and the artifact
shell through their declared schema dependency, so no component duplicates
another's source of truth. The schema component exposes its vocabulary
package (including the taxonomy) through its declared surface.

## Metadata
- **Complexity**: High
- **Labels**: architecture, components, ownership, layering, bazel, go
- **Required Skills**: Go package architecture, Architectural Contracts component modeling, Bazel/Starlark, dependency-boundary testing
