# Task: Make Foreign Ownership Audits Repository-Wide

## Description
Strengthen the protobuf-runtime and x-tools ownership regressions so their claimed one-owner property is actually enforced across every production component manifest and `go_component` declaration, including examples and nested production components.

## Background
Addresses finding F3 from the Step 7 implementation review. Current-tree searches confirm singular ownership today, but `checkedInComponentManifests` scans only one directory level under `go/internal` and `go/cmd` while claiming to enumerate the repository. It omits `go/examples/csvtool` and any other deeper production manifest, and the foreign-owner checks rely on a partial Bazel parity target rather than an explicit inventory of all production `go_component` member declarations.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step07.yaml`

**Additional References:**
- Protobuf audit: `go/internal/manifestparity/ownership_test.go`
- x/tools audit: `go/internal/manifestparity/x_tools_ownership_test.go`
- Bazel parity driver: `go/cmd/arcc/app/self_manifest_parity_test.go`, `go/cmd/arcc/app/BUILD.bazel`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Define the production-manifest inventory explicitly and recursively. Include `go/internal`, `go/cmd`, and `go/examples`; exclude test fixtures through a named, documented rule rather than incidental scan depth.
2. Fail on duplicate component names instead of silently overwriting an earlier manifest in the name-keyed map.
3. Reuse the complete inventory for protobuf and x/tools ownership checks, preserving exact package-prefix rules, sorted/unique wrapper membership, asserted surface checks, and required consumer-edge assertions.
4. Extend Bazel-side parity or an equivalent structural audit to every production `go_component` declaration that can declare members. Prove that no non-wrapper declaration owns a protobuf or retained x/tools/x-mod/x-sync package.
5. Add negative tests placing a duplicate foreign owner under an example-like nested production path and in a non-wrapper `go_component`; both must fail the audit.
6. Keep the actual owner sets unchanged unless the expanded inventory exposes a genuine duplicate. Do not treat ordinary Go compile dependencies as architectural member ownership.
7. Update stale inventory comments/component lists and run `just ci` before committing.

## Dependencies
- Step 7 Tasks 4 and 5 define the canonical protobuf-runtime and x-tools wrapper sets.
- Task 07 may change packagelayout ownership, but this task's foreign protobuf/x-tools assertions remain independent.

## Implementation Approach
1. Extract a testable recursive production-manifest discovery helper with explicit fixture exclusions and duplicate-name errors.
2. Route both foreign ownership suites through that inventory and add nested negative fixtures in temporary directories.
3. Expand the Bazel parity input/audit set and add a deliberately conflicting analysis-test fixture.
4. Verify the live repository still has exactly one wrapper owner per foreign package set.

## Acceptance Criteria

1. **All production manifests are inventoried**
   - Given checked-in manifests under internal packages, commands, examples, and a nested production directory
   - When the ownership inventory runs
   - Then every production manifest is parsed exactly once and documented testdata fixtures are excluded

2. **Duplicate identities fail closed**
   - Given two production manifests with the same component name
   - When inventory is built
   - Then the audit fails and names both paths rather than overwriting either entry

3. **Foreign package ownership remains singular**
   - Given all production manifests and `go_component` declarations
   - When protobuf and retained x/tools/x-mod/x-sync members are indexed
   - Then only `protobuf-runtime` and `x-tools` respectively own those package sets, with no duplicate owner in examples or nested components

4. **Out-of-scope compile dependencies are not misclassified**
   - Given ordinary `go_library` dependencies on protobuf or x/tools packages
   - When the ownership audit runs
   - Then they are not treated as component membership unless they appear in a `go_component` members declaration

5. **Live parity and CI pass**
   - Given the expanded manifest and Bazel inventories
   - When ownership tests, manifest parity, and `just ci` run
   - Then the current singular wrapper ownership passes in both native and Bazel representations

## Metadata
- **Complexity**: Medium
- **Labels**: tests, ownership, protobuf, x-tools, bazel, manifest-parity
- **Required Skills**: Go filesystem test design, component ownership modeling, Starlark analysis testing, manifest parity
