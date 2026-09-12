# Task: Align Final Implementation Documentation

## Description
Reconcile the canonical surface schema, detailed design, README limitations,
and stale source comments with the completed proof-of-concept implementation and
its accepted escalation decisions.

## Background
Addresses finding F3 from the plan-scoped implementation review. The
implementation behavior is coherent, but several authoritative or high-value
comments still describe an intermediate state. Most importantly,
`SurfaceManifest.digest` is documented as source-derived without saying that an
asserted `UNKNOWN` surface deliberately carries the empty digest under I6. The
design status still says implementation is in progress, and a handful of
source comments retain removed membership concepts, future tense, or completed
step placeholders.

The final handoff should also distinguish two accepted MVP decisions from
permanent production architecture: component-wide
`analysis_defeating_policy: WARN`, and the closure-shaped lexical
`ArccImportGraph` projection required by pinned rules_go. This is a
documentation-alignment task only; it must not redesign those mechanisms.

The repository owner has already added README sections identifying the
proof-of-concept status and AI coding practices. Preserve those disclosures.
Do not refresh `.agents/summary/`; the owner will do that separately with the
dedicated `codebase-summary` skill after Step 14.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (I5, I6, Step 7 residual policy, type loading, migration strategy)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review.yaml`

**Additional References:**
- `proto/archcontracts/v1/surface.proto`
- `bazel_rules/go/private/component.bzl` (asserted surface producer)
- `bazel_rules/go/private/stdlib_map.bzl`
- `go/internal/hostpolicy/hostpolicy.go`
- `go/internal/stdlibmap/stdlibmap.go`
- `docs/package-layout-schema.md`
- `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- `.agents/scratchpad/awo-acpx-report.2026-08-04-compositional-component-analysis.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Update `SurfaceManifest.digest` documentation in `surface.proto` to distinguish checked source-derived surfaces from asserted `UNKNOWN` surfaces, whose empty digest records that no component content was loaded, type-checked, or hashed.
2. Regenerate the Go protobuf output so the generated field comment matches the schema, and keep `just gen-is-clean` green.
3. Mark the detailed design's implementation status complete and rewrite baseline/proposed wording where it can now be mistaken for current behavior; retain historical reasoning where it is clearly framed as baseline or design rationale.
4. Correct the stale `hostpolicy` comments naming removed membership patterns or future surface emitters, the completed-Step-5 placeholder in `stdlib_map.bzl`, and `PackageEntry` documentation that omits target-build-constraint exclusions.
5. Repair README's limitation numbering and add concise production-handoff limitations for the component-wide WARN policy and the source-reading `ArccImportGraph` projection. State that both are accepted for this MVP, remain visible/measured, and require reconsideration rather than silent inheritance by a production implementation.
6. Preserve the README's Implementation Status and AI Coding Practices disclosures without softening their statement that neither implementation code nor workflow task descriptions received detailed human review.
7. Preserve the legacy SDK compatibility accessors in `go_adapter.bzl`; if they are mentioned, identify the canonical three seams while stating that the aliases remain for an external monorepo migration.
8. Do not edit any file under `.agents/summary/`.

## Dependencies
- Task 01 restores the complete documented CI gate and should land first so this task's validation includes lint.
- Steps 4, 5, 7, 8, 12, and 13 contain the final behavior and escalation decisions the documentation must describe.

## Implementation Approach
1. Compare the schema and comments against the checked and asserted surface producers, then correct the proto at the source of truth and regenerate its Go output.
2. Reconcile the detailed design's header and overview with the completed plan while preserving clearly historical measurements and alternatives.
3. Correct the enumerated stale source comments using the current call sites and target-selection behavior as evidence.
4. Add compact README handoff entries that link the WARN and import-projection limitations to their existing detailed documentation rather than duplicating it.
5. Search for the stale phrases and inspect the final diff for accidental behavior changes or `.agents/summary/` edits, then run generation, lint, and full CI validation.

## Acceptance Criteria

1. **Checked and asserted digest semantics are unambiguous**
   - Given the canonical surface proto and generated Go field comment
   - When a reader examines `SurfaceManifest.digest`
   - Then checked surfaces are described as source-derived and asserted `UNKNOWN` surfaces are explicitly described as carrying an empty non-content claim consistent with I6.

2. **The design reads as a completed implementation record**
   - Given the detailed design header and overview
   - When they are read after Step 13
   - Then the status is complete, current behavior is not described as a future proposal, and historical baseline reasoning remains identifiable as historical.

3. **Stale source comments match current behavior**
   - Given hostpolicy namespace hooks, the default stdlib-map attribute, and stdlib package entries
   - When their comments are compared with current call sites and generation behavior
   - Then they contain no removed membership-pattern/future-emitter/unused-Step-5 claims and correctly include build-constraint-based non-importability.

4. **Prototype debts are explicit production handoff items**
   - Given README's Limitations and Scope section
   - When a production implementer evaluates the MVP
   - Then the component-wide WARN carrier and lexical ArccImportGraph projection are named as accepted, visible MVP trade-offs that should be revisited rather than copied implicitly.

5. **Owner disclosures and compatibility policy are preserved**
   - Given README's implementation-status disclosure and the adapter contract
   - When the documentation change is complete
   - Then the AI coding-practices statement remains intact, legacy SDK aliases remain available for the external monorepo, and `.agents/summary/` has not changed.

6. **Documentation generation and validation remain clean**
   - Given the completed documentation changes
   - When `just gen-is-clean`, `just lint`, and `just ci` run
   - Then generated protobuf comments match the proto and every gate passes without implementation behavior changes.

## Metadata
- **Complexity**: Medium
- **Labels**: remediation, documentation, protobuf, design, readme, handoff
- **Required Skills**: Technical documentation, protobuf schema maintenance, Go/Starlark code reading, architectural decision reconciliation
