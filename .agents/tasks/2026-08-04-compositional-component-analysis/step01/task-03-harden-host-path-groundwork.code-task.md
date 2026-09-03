# Task: Harden Host Path Groundwork

## Description
Make semantic manifest parity insensitive to host path-namespace rewrites by canonicalizing both generated and checked-in package paths before set comparison. Preserve and verify the baseline's fail-loud SDK-root discovery so invalid host layout paths cannot fall through into filesystem walking.

## Background
Host adapters may rewrite upstream import prefixes through `hostpolicy.CanonicalizePath`. The generated Bazel manifest and its checked-in mirror can therefore describe the same members or absorbed dependency paths in different namespaces. The parity helper currently diffs the raw strings and reports false mismatches. Separately, stdlib discovery must reject an absent SDK root before calling `filepath.Walk`; the current baseline already performs this check, so the task must retain it and ensure its public layout path has actionable regression coverage.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Host import friction report §4 and §6(2): `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md`
- Plan Step 1: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. In the Go `manifestparity` comparison path, canonicalize generated and checked-in member import paths through `hostpolicy.CanonicalizePath` before sorting, set differencing, and applying the implicit-interface-package allowance.
2. Canonicalize both sides of every other import-path comparison performed by the helper, including absorbed dependency import paths while that field still exists in Step 1.
3. Do not canonicalize component names, filesystem interface-file basenames, authority values, or other non-import-path fields.
4. Add a regression test that installs a temporary canonicalizer, presents semantically equal manifests written in rewritten and canonical namespaces, and observes no parity error. Restore the global host-policy seam with `t.Cleanup` so the test cannot leak state.
5. Preserve `discoverStdlibWithContext` validation that returns an error wrapping `os.Stat(sdkRoot)` before `filepath.Walk` when the SDK root is missing, and that rejects a non-directory root.
6. Verify or extend package-layout coverage through the normal layout stdlib-discovery entry point so a missing SDK root produces an actionable error containing the root context; do not add a fallback to another SDK or filesystem root.

## Dependencies
- No dependency on tasks 1 or 2; this task is independently implementable and testable.
- Uses the existing override-once `hostpolicy.CanonicalizePath` seam and package-layout test seams.

## Implementation Approach
1. Add a small path-normalization helper local to `manifestparity` and apply it only where manifest import-path sets are compared.
2. Add a focused `CompareManifests` test with distinct raw prefixes that converge under the test canonicalizer, including members and absorbed dependencies.
3. Inspect the existing SDK-root guard and failure-class tests; retain the implementation, adding only missing caller-level coverage needed to prove a layout with an absent root fails before walking.
4. Run focused manifest-parity and package-layout tests, then the repository CI gate.

## Acceptance Criteria

1. **Rewritten namespaces compare equal**
   - Given a generated manifest whose member and absorbed-dependency paths use a host-rewritten prefix and a checked-in manifest using the canonical prefix
   - When both spellings map to the same values through `hostpolicy.CanonicalizePath`
   - Then `CompareManifests` reports no member or absorbed-dependency mismatch.

2. **Real mismatches still fail**
   - Given generated and checked-in import paths that remain different after canonicalization
   - When semantic parity is checked
   - Then the helper reports the corresponding mismatch with the canonicalized sets.

3. **Canonicalizer isolation**
   - Given a test overrides the package-global canonicalization seam
   - When that test finishes
   - Then the original canonicalizer is restored and other tests are unaffected.

4. **Missing SDK root fails before walking**
   - Given a layout whose SDK root does not exist
   - When stdlib discovery runs through the layout path
   - Then it returns a non-nil, actionable error that identifies access to the SDK root and wraps the stat failure, without attempting a fallback filesystem walk.

5. **Non-directory SDK root fails clearly**
   - Given an SDK root path that names a regular file
   - When stdlib discovery runs
   - Then it returns an error stating that the SDK root is not a directory.

6. **Integration remains green**
   - Given the normalized comparison and preserved SDK validation
   - When `just ci` runs
   - Then all Go, self-check, and Bazel checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: go, host-policy, manifest-parity, package-layout, regression
- **Required Skills**: Go, table-driven testing, host path canonicalization, filesystem error handling
