# Task: Validate declared membership

## Description
Map all new component fields into the hand-written manifest model and enforce declared-membership and interface-style validation without changing existing declared-style manifests.

## Background
The native `manifest.Manifest` shields the rest of arcc from protobuf types, so Step 1's wire additions must become explicit native values. Declared-style components require literal member import paths and continue to require interface files. `PACKAGE_SURFACE` components instead require non-empty members, forbid interface files, and may retain import-path patterns. Member declarations must also be internally consistent with absorbed declarations.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M1/M3/M4/M7/M8, §4.1–§4.2, and §6)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 1)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/members-glob-expansion.md` (why declared-style manifests contain expanded literals while package-surface membership may remain patterned)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend `manifest.Manifest` with `Members`, `InterfaceStyle`, `OwnCheckRuns`, and `CertificationReference`; extend `manifest.ComponentDependency` with `AutoAttached`.
2. Map all fields losslessly from the generated protobuf model while keeping protobuf-specific types out of downstream packages where practical.
3. Reject duplicate member entries with a typed, actionable validation error.
4. Validate every member using the same import-path pattern grammar used for absorbed dependencies and reject malformed patterns.
5. For declared style, reject members containing pattern metacharacters; emitters must supply expanded literal import paths.
6. For `PACKAGE_SURFACE`, allow valid unexpanded patterns, require at least one member, and reject every non-empty `interface_files` list.
7. For the default/unspecified style, preserve the current requirement for at least one interface file.
8. Reject any member string that is also exactly declared as an `absorbed_dependencies.import_path`; report the contradictory value.
9. Reject unknown future numeric interface-style values rather than silently treating them as declared style.
10. Cover parsing and validation of the certification and `auto_attached` fields even though their behavior lands in later plan steps.

## Dependencies
- Task 1: Extend component schema and regenerate Go bindings.

## Implementation Approach
1. Add a native interface-style representation with clear zero/default behavior and map generated values explicitly.
2. Expand the parser's field mapping before validation so errors can describe the complete declaration.
3. Factor member/pattern checks into focused helpers using Go's path-pattern semantics already used by the checker.
4. Add table-driven tests for valid literals and patterns, malformed syntax, duplicates, absorbed contradictions, both interface-style shape directions, unknown styles, and old manifests.

## Acceptance Criteria

1. **All new fields parse**
   - Given a textproto containing every Step 1 field
   - When `manifest.Parse` runs
   - Then the returned native model contains the same members, style, certification metadata, and auto-attached edge values.

2. **Declared style preserves current behavior**
   - Given a manifest with no `members` or `interface_style`
   - When it is parsed
   - Then it still requires `interface_files` and otherwise behaves exactly as before.

3. **Declared members are literal and unique**
   - Given a default-style manifest with duplicate members, malformed patterns, or valid pattern metacharacters
   - When it is parsed
   - Then each invalid form is rejected with an error naming the offending member.

4. **Package-surface shape has one source of truth**
   - Given `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE`
   - When `members` is empty or `interface_files` is non-empty
   - Then parsing fails with a specific shape error; valid non-empty literal or patterned members with no interface files parse successfully.

5. **Absorbed contradictions fail**
   - Given the same import-path declaration in `members` and `absorbed_dependencies`
   - When the manifest is parsed
   - Then parsing fails and names that path as both member and absorbed.

6. **Repository checks remain green**
   - Given the completed parser and validation changes
   - When `just ci` runs
   - Then all existing manifests remain valid and all checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, manifest, validation, membership
- **Required Skills**: Go, protobuf mapping, validation design, table-driven testing
