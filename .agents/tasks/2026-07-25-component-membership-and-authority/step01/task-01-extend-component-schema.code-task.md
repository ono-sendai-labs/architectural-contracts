# Task: Extend component schema

## Description
Extend the component manifest protobuf with declared membership, interface style, certification metadata, and auto-attached dependency-edge metadata, then regenerate the checked-in Go bindings.

## Background
Step 1 establishes the additive wire contract used by later membership and package-surface work. Existing manifests must continue to decode with declared-style defaults and no explicit members. The certification fields are reviewable self-declarations, not facts verified by arcc, so their schema comments are part of the trust contract.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M1/M3, §4.1, and §10)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 1)

**Additional References (if relevant to this task):**
- None.

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `members` as repeated string field 6 on `Component`.
2. Add `InterfaceStyle` with `INTERFACE_STYLE_UNSPECIFIED = 0` and `INTERFACE_STYLE_PACKAGE_SURFACE = 1`, and add `interface_style` as field 7 on `Component`.
3. Add `own_check_runs` as field 8 and `certification_reference` as field 9 on `Component`.
4. Add `auto_attached` as the next available field on `ComponentDependency`.
5. Document member semantics, including empty-as-FR1 and the implicit interface member.
6. State explicitly in both certification-field comments that the values are self-declarations at the same trust level as `declared_authority` and are not verified by arcc.
7. Replace stale schema prose claiming that component membership is never explicit.
8. Regenerate `go/internal/manifest/gen/component.pb.go` with `just gen`; do not hand-edit generated code.
9. Add or update schema/binding tests only where needed to pin field numbers, enum values, defaults, and textproto compatibility.

## Dependencies
- None; this is the foundational task for Step 1.

## Implementation Approach
1. Update `proto/archcontracts/v1/component.proto` with additive, stable field numbers and trust-level comments.
2. Run `just gen` and inspect the generated descriptors and accessors.
3. Exercise an old-style textproto and a textproto containing every new field to verify backward-compatible decoding and enum names.
4. Run generation-cleanliness and repository checks before committing.

## Acceptance Criteria

1. **New wire fields are available**
   - Given the regenerated Go protobuf API
   - When callers construct or decode a component
   - Then `members`, `interface_style`, `own_check_runs`, `certification_reference`, and dependency `auto_attached` are available at the specified stable field numbers.

2. **Interface style has a backward-compatible default**
   - Given an existing manifest that omits `interface_style`
   - When it is decoded
   - Then its style is `INTERFACE_STYLE_UNSPECIFIED`.

3. **Certification trust is unambiguous**
   - Given a reader inspecting the protobuf schema
   - When they read either certification-field comment
   - Then the comment states that it is an unverified self-declaration at the same trust level as `declared_authority`.

4. **Existing textprotos remain compatible**
   - Given the repository's current manifests and examples
   - When they are decoded with the regenerated bindings
   - Then no migration is required and their existing field values are unchanged.

5. **Generated code is reproducible**
   - Given the completed schema change
   - When `just gen-is-clean` and `just ci` run
   - Then regeneration produces no diff and all checks pass.

## Metadata
- **Complexity**: Low
- **Labels**: protobuf, schema, manifest, code-generation
- **Required Skills**: Protocol Buffers, Go code generation, compatibility testing
