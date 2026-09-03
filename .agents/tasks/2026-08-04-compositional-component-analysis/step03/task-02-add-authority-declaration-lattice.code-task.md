# Task: Add Authority Declaration Lattice

## Description
Add the persisted `authority` axis to component manifests and introduce the structural `AuthorityDeclaration` model with a total join operation. Preserve `DECLARED` as the default, distinguish a verified empty declaration from `UNKNOWN`, and reject contradictory or non-canonical representations without changing the current analysis engine.

## Background
An unanalyzed component cannot safely be represented by an empty `declared_authority` list: empty means a checked component uses no ambient authority, while `UNKNOWN` means it could use any authority. The design models these as distinct lattice elements and requires unknown to absorb every join. Step 3 establishes that invariant in data and pure code; later build and report steps decide how unknown components are produced and consumed.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Decision record Q7-Q11: `.agents/planning/2026-08-04-compositional-component-analysis/idea-honing.md`
- Plan Step 3: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-06: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a component-proto authority enum with `DECLARED` as the zero/default value and `UNKNOWN` as the only other accepted value, and add the `authority` field without reusing any reserved field number. Regenerate the checked-in Go binding and assert exact enum/field numbers in schema tests.
2. Add a native `AuthorityDeclaration` value with structural known/unknown state and a sorted capability set. Its constructors and validation must make `DECLARED{}` and `UNKNOWN` distinct, reject `UNKNOWN` carrying any declared capability, reject unknown enum values, and retain existing known-capability and duplicate validation.
3. Parse omitted `authority` as `DECLARED`; parse `authority: UNKNOWN` only when `declared_authority` is empty. Return an actionable error for the contradictory spelling.
4. Implement `Join(a, b AuthorityDeclaration) AuthorityDeclaration` as the lattice join: unknown absorbs, two known declarations union their capability sets, empty joined with empty remains known-empty, and the result is sorted and duplicate-free without aliasing either input slice.
5. Provide explicit conversion/serialization helpers between the native declaration and persisted authority-plus-capabilities fields. They must reject, rather than normalize away, an unknown value paired with a non-empty capability list.
6. Thread the new native declaration through manifest equality/parity and copies while preserving the current checker behavior for declared manifests. Do not add authority-bound computation, asserted surfaces, manual-tag behavior, or the final Step 11 unknown-component check path.
7. Update checked-in manifests and generated goldens only where an explicit spelling adds test value; ordinary manifests should rely on the default `DECLARED` encoding to prove backward compatibility.
8. Add table-driven unit tests covering all join pairs, input immutability, deterministic ordering, parse defaults, unknown enum values, contradictory unknown input, duplicate/invalid capabilities, and persisted round trips for both `DECLARED{}` and `UNKNOWN`.

## Dependencies
- Task 1 must be complete so component field numbers 8 and 9 are reserved before the new field is allocated.
- No dependency on the new surface or standard-library map schemas; those schemas consume this authority representation in Task 3.

## Implementation Approach
1. Extend the component schema with a zero-safe enum and a non-conflicting field number, regenerate bindings, and update descriptor tests.
2. Introduce the native structural type and focused constructors/conversion helpers beside manifest parsing, maintaining a small pure API.
3. Convert manifest parsing and parity to the structural declaration while retaining declared-authority behavior needed by the existing checker during the unreleasable transition.
4. Add exhaustive table-driven lattice and parsing tests, including slice-alias checks and non-canonical serialization rejection.
5. Run focused manifest/parity tests, generation cleanliness, and the full CI gate.

## Acceptance Criteria

1. **Default declaration remains known**
   - Given an existing component textproto with no `authority` field and no declared capabilities
   - When it is parsed
   - Then its authority is `DECLARED{}` rather than `UNKNOWN`, and existing declared manifests retain their current checker behavior.

2. **Unknown is structurally distinct**
   - Given one manifest spelling `authority: UNKNOWN` and another using the default with an empty capability list
   - When both are parsed and round-tripped through the native model
   - Then the first remains unknown and the second remains known-empty; neither can be inferred solely from slice emptiness.

3. **Contradictory input fails**
   - Given `authority: UNKNOWN` with one or more `declared_authority` entries, or an unrecognized enum value
   - When parsing or serialization is attempted
   - Then it fails with an actionable error and does not emit a normalized but misleading declaration.

4. **Join obeys the lattice**
   - Given every pair among unknown, known-empty, and known non-empty declarations
   - When `Join` is evaluated
   - Then unknown absorbs, known sets union deterministically, `DECLARED{} ⊔ DECLARED{}` is known-empty, duplicates are removed, and inputs are unchanged.

5. **Persisted round trips preserve meaning**
   - Given valid declared and unknown native values
   - When converted to their persisted authority and capability representation and read back
   - Then equality, canonical ordering, and known/unknown state are preserved exactly.

6. **Integration remains green without premature consumers**
   - Given the new axis and lattice
   - When `just gen-is-clean` and `just ci` run
   - Then both pass, and no authority aggregation, surface provenance, or unknown-component execution semantics from later steps have been introduced.

## Metadata
- **Complexity**: Medium
- **Labels**: go, protobuf, manifest, authority, lattice, validation
- **Required Skills**: Go, Protocol Buffers, algebraic data modeling, table-driven testing, deterministic serialization
