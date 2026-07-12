# Task: Syntactic manifest validation in `Parse`

## Description
Add the **syntactic** (contents-only) validation layer to `manifest.Parse`: reject
manifests that are structurally invalid without touching the filesystem. This completes
the pure `manifest` package for Step 2.

## Background
Validation is deliberately **split** (design §7, review C9): pure `Parse` sees only the
manifest's contents, not the filesystem around it, so it performs only syntactic checks.
Checks that need the filesystem (interface file exists on disk / belongs to a package
under the component root; dependency-manifest resolution & name match) are the **shell's**
job in Steps 6/9 and are explicitly **out of scope** here.

The syntactic checks (design §7, plan Step 2):
- empty `name`
- empty `interface_files`
- duplicate declarations
- unknown capability name in `declared_authority`, validated against the **known
  capability set** (review C11)

Each failure returns a parse error (which the shell maps to exit code 2). An unknown
capability name such as `"FILE"` (typo of `FILES`) must be rejected with a specific
error.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§7 Error Handling — syntactic vs resolved validation; §5.4a for the capability vocabulary)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 2)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Define the **known capability set** used to validate `declared_authority` names
   (the capability vocabulary — e.g. `FILES`, `NETWORK`, `EXEC`, `READ_SYSTEM_STATE`,
   `MODIFY_SYSTEM_STATE`, `REFLECT`, `UNSAFE_POINTER`, `CGO`, `ARBITRARY_EXECUTION`, …;
   derive the exact set from Capslock's capability names as used across the design/§5.4a
   and research). Expose it in a form the rest of the codebase can reuse (the checker's
   authority rule references the same vocabulary).
2. Extend `Parse` to validate the mapped `Manifest` **after** unmarshalling and return a
   parse error on any of:
   - empty `name`
   - empty `interface_files`
   - duplicate declarations (e.g. duplicate `interface_files`, duplicate component
     dependency names, duplicate absorbed import paths, duplicate `declared_authority`
     entries)
   - a `declared_authority` name not in the known capability set
3. Errors must be specific enough that a test can assert *which* rule fired (distinct
   sentinel errors or messages per rule).
4. Do **not** add filesystem or dependency-resolution checks — those are the shell's
   (Steps 6/9). Keep `Parse` pure.

## Dependencies
- task-02 (`Manifest` model + `Parse` mapping — validation hooks into the seam it left).

## Implementation Approach
1. Add the known-capability-set definition and a membership check.
2. Implement a `validate(Manifest) error` invoked at the end of `Parse` (after mapping),
   returning on the first failed rule (or aggregating — implementer's choice, but each
   rule must be individually assertable).
3. Table-driven unit tests, one case per validation error, fed via `bytes.Reader`:
   empty name; empty `interface_files`; a duplicate declaration of each kind; unknown
   capability name (`"FILE"`) → the specific unknown-capability error. Plus a
   fully-valid manifest → no error (guards against false positives).

## Acceptance Criteria

1. **Empty name rejected**
   - Given a manifest with empty `name`
   - When `Parse` is called
   - Then it returns the empty-name parse error.

2. **Empty interface_files rejected**
   - Given a manifest with no `interface_files`
   - When `Parse` is called
   - Then it returns the empty-interface-files parse error.

3. **Duplicates rejected**
   - Given a manifest with a duplicated declaration (interface file, dependency name,
     absorbed import path, or authority entry)
   - When `Parse` is called
   - Then it returns the duplicate-declaration parse error.

4. **Unknown capability rejected**
   - Given `declared_authority` containing `"FILE"` (not in the known capability set)
   - When `Parse` is called
   - Then it returns the specific unknown-capability parse error naming the offending value.

5. **Valid manifest passes**
   - Given a well-formed manifest (non-empty name & interface_files, no duplicates, only
     known capabilities)
   - When `Parse` is called
   - Then it returns the mapped `Manifest` and no error.

6. **No filesystem checks**
   - Given `Parse`'s validation
   - When reviewed
   - Then it performs only contents-only checks and never touches the filesystem or
     resolves dependency manifests (those remain the shell's job).

## Metadata
- **Complexity**: Low
- **Labels**: validation, parser, pure-core, manifest, error-handling
- **Required Skills**: Go
