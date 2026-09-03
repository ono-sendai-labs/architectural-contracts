# Task: Retire Legacy Verification Fields

## Description
Remove the obsolete `own_check_runs` and `certification_reference` manifest fields and the self-declared verification model built around them. Reserve their persisted field numbers and names, reject stale authored manifests clearly, and remove their downstream Go, Starlark, report, fixture, and documentation residue while keeping the repository buildable.

## Background
The compositional design derives verification provenance structurally from the producer and report artifacts introduced in later steps. The current fields are author claims and cannot establish that provenance. Their persisted numbers and names must never be reused, and textproto parsing must fail loudly because the parser otherwise discards unknown fields. This implementation series is intentionally unreleasable until Step 11, so temporary loss of the old report annotation is expected; do not pre-implement the Step 7 provenance/freshness axes here.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Plan Step 3: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-06 and DR-15: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Delete fields 8 and 9 from `proto/archcontracts/v1/component.proto`; reserve both numbers and the names `own_check_runs` and `certification_reference`, preserving the existing reservations for field 4 and `absorbed_dependencies`.
2. Extend the manifest parser's removed-field detection so either retired spelling produces an actionable `RemovedFieldError` before lenient textproto unmarshalling can discard it. Preserve the existing comment/string scrubbing behavior and stale `absorbed_dependencies` rejection.
3. Regenerate the checked-in component binding and update schema tests to assert that fields 8 and 9 are absent and reserved rather than accessible.
4. Remove `OwnCheckRuns` and `CertificationReference` from the native manifest model, dependency-interface facts, report dependency data, checker mapping, dependency resolution, application plumbing, parity comparisons, and tests. Do not replace them with booleans or another self-declared certification shortcut.
5. Remove `own_check_runs` emission and private/public rule attributes from the Bazel component rule and test helpers. Keep existing `manual` tags as ordinary Bazel tags until Step 5 gives them asserted-surface semantics.
6. Remove the retired fields from checked-in component manifests, testdata, generated-manifest goldens, report goldens, README/API documentation, and comments. Update verdict expectations only where deletion of the obsolete display metadata necessarily changes output.
7. Preserve current component checking, dependency pruning, call-graph analysis, and verdict behavior; this task must not introduce surface provenance, freshness, or analysis actions scheduled for Steps 5 and 7.
8. Run schema generation cleanliness and the full CI gate, and verify that the retired spellings remain only in their proto reservations, removed-field compatibility checks/tests, and historical planning artifacts.

## Dependencies
- Step 2 must be complete so field 4 and `absorbed_dependencies` are already retired.
- No dependency on later Step 3 tasks; this contracts the current persisted manifest before new fields and artifact schemas are added.

## Implementation Approach
1. Inventory every live use of the two fields in proto, generated Go, native models, facts, reports, application code, Bazel rules, manifests, fixtures, goldens, and documentation.
2. Reserve the schema identifiers, add parse-time stale-field diagnostics, regenerate bindings, and update schema/parser tests.
3. Contract the Go DTO flow from manifest parsing through dependency resolution and report rendering, deleting assertions that exist only for the obsolete model.
4. Contract the Bazel macro/rule/test helper API and refresh only outputs affected by removal of the two fields.
5. Search outside `.agents/` for unintended live references, run focused parser/report/Bazel tests, then run `just gen-is-clean` and `just ci`.

## Acceptance Criteria

1. **Persisted identifiers are retired safely**
   - Given the regenerated component schema
   - When its descriptor is inspected
   - Then field numbers 8 and 9 and names `own_check_runs` and `certification_reference` are reserved, neither field exists, and the prior field-4 reservations remain intact.

2. **Stale verification declarations fail loudly**
   - Given a textproto naming either retired field
   - When `manifest.Parse` reads it
   - Then parsing returns an actionable removed-field error naming that field rather than silently ignoring it; occurrences in comments and quoted values do not trigger the check.

3. **Self-declared verification state is absent**
   - Given the native manifest, dependency facts, report DTOs, checker, resolver, and CLI path
   - When their public fields and emitted JSON/text are inspected
   - Then no verification state is derived from `own_check_runs` or `certification_reference`, and no replacement author-asserted certification flag was introduced.

4. **Bazel and repository inputs expose no obsolete API**
   - Given `go_component`, its implementation and testing helpers, checked-in manifests, fixtures, and goldens
   - When they are searched and built
   - Then they neither accept nor emit the retired fields, while ordinary `manual` tagging remains valid Bazel behavior.

5. **Existing analysis semantics remain intact**
   - Given conforming and violating components that do not use the retired fields
   - When their checks run
   - Then call-graph analysis, boundary pruning, findings, and verdicts are unchanged apart from removal of obsolete verification annotations.

6. **Generation and integration remain green**
   - Given all cleanup and regenerated bindings
   - When `just gen-is-clean` and `just ci` run
   - Then both pass, and non-planning searches find the retired names only in compatibility reservations and rejection tests.

## Metadata
- **Complexity**: High
- **Labels**: protobuf, go, bazel, manifest, report, schema-evolution, deletion
- **Required Skills**: Go, Protocol Buffers, Starlark, Bazel testing, persisted-schema evolution, deterministic golden maintenance
