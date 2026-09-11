# Task: Prune Source Inputs and Report Export-Data Diagnostics

## Description
Complete the Step 8 input contract by removing dependency and SDK source files from component analysis actions, retaining member sources only, and recording deterministic non-member export-data input counts and bytes in the report diagnostics. Add final Bazel/self-check coverage and capture the step demo evidence.

## Background
After Task 5, `go/packages` no longer parses or type-checks non-member source, but the Bazel action may still stage those files for migration safety. N2 requires the declared action inputs to be exactly member sources, manifest/layout, the closure's export data, direct dependency surfaces/reports, and the stdlib map. Removing unused source inputs is what makes the build graph enforce that contract rather than merely relying on loader behavior.

The plan also requires reports to expose the size of the remaining closure-shaped cost. The diagnostic count and byte total make the export-data input footprint observable without claiming zero closure dependence; Step 13 will use the same metrics for depth-scaling acceptance.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N1-N2, N4, I5, DR-02, §Build topology, acceptance matrix “Action inputs”)

**Additional References:**
- Export-data spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-export-data-loading.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Change `_checked_analysis_action` and its callers so direct/transitive source inputs contain only the effective member packages' selected `.go` files. Remove dependency closure sources and `go_sdk_srcs(ctx)` from the action and wrapper frame.
2. Retain as declared inputs: member sources, generated manifest and package layout, every reachable non-member export artifact and required graph descriptor, each direct dependency surface and applicable report, and the selected stdlib authority map. Do not add a Go binary, tool binary, undeclared cache, network, or transitive dependency source through runfiles.
3. Preserve source availability for member type-checking, interface-file validation, analysis-defeating scans, and surface digest generation. Selection must follow the layout's target build constraints and canonical member roots, not a raw unfiltered closure.
4. Add a report `diagnostics` section with stable fields for the number of unique non-member export artifacts consumed and their total bytes. Define the byte total as the sum of resolved artifact file sizes after deduplication; exclude member sources, layout metadata, surfaces/reports, and the stdlib authority map.
5. Populate diagnostics from the actual validated load inputs in both Bazel/layout and native modes. Missing stat data or an overflow must fail as a tool error rather than silently record a partial total.
6. Extend canonical report encoding/decoding and text rendering deterministically, preserving backward-compatible decoding of reports that predate diagnostics and preserving verdict derivation solely from violations.
7. Add a Bazel analysis test that inspects a dependent's `ArccCheck` inputs and proves they contain member `.go` files, export files, dependency surfaces/reports, layout/manifest, and map, with no dependency `.go` or SDK `.go` file.
8. Add end-to-end tests for a deep synthetic closure with dependency sources absent, deterministic diagnostics across repeated runs, and accurate deduplicated count/bytes. Keep native and Bazel self-check green.
9. Update package-layout/adapter documentation and the Step 1 measurement note (or a directly linked Step 8 research note) with the final action-input listing, a representative deep-closure load-time comparison against Step 6, and confirmation that non-member source is absent. Do not turn the Step 13 scaling benchmark into a flaky timeout here.

## Dependencies
- Task 5: Cut Over to Member-Only Type Loading.

## Implementation Approach
1. Derive member-source and non-member-export depsets separately from the already merged closure and pass them explicitly into action registration and frame construction.
2. Compute metrics at the validated loader boundary, carry them through the shell/core DTO boundary, and attach them to the completed report before artifact publication/rendering.
3. Extend report canonicalization and tests, then tighten Bazel action-input assertions to the final N2 allowlist.
4. Run the native/Bazel self-check and record the non-flaky demo measurements and input inventory.

## Acceptance Criteria

1. **Bazel action contains only allowed inputs**
   - Given a checked component with a direct dependency and stdlib imports
   - When an analysis test inspects its `ArccCheck` action
   - Then inputs include member sources, export data/graph metadata, direct dependency surfaces/reports, manifest/layout, and map, and include no dependency or SDK `.go` source and no toolchain binary/cache.

2. **Member analyses still have required source**
   - Given interface files, build-tagged member files, assembly/cgo/linkname detection fixtures, and surface emission
   - When the action runs after pruning
   - Then every selected member input remains available and existing findings/digests are unchanged.

3. **Diagnostics are accurate and deterministic**
   - Given a closure where multiple package records may reference the same physical export artifact
   - When the report is emitted twice
   - Then diagnostics record the unique artifact count and exact deduplicated byte sum, and canonical report bytes are identical.

4. **Older reports remain readable**
   - Given a valid persisted report without a diagnostics field
   - When it is decoded
   - Then decoding succeeds with zero-value diagnostics and its verdict is unchanged.

5. **Deep closure works without dependency source**
   - Given the Step 8 synthetic deep closure with every dependency `.go` file absent from the sandbox
   - When the check runs
   - Then it succeeds with correct `Uses`/`Selections`, import facts, report, surface, and non-member metrics.

6. **Step demo evidence is recorded**
   - Given the final implementation
   - When the checked action inputs and representative load timing are captured
   - Then the research note shows no dependency source, lists the export-data inputs, and compares load time with Step 6 without imposing a flaky timeout.

7. **Integration remains green**
   - Given the final Step 8 state
   - When `just ci` and self-check run in both supported modes
   - Then all checks pass and the action remains network-blocked and toolchain-exec-free under Bazel.

## Metadata
- **Complexity**: High
- **Labels**: bazel, go, action-inputs, report, diagnostics, hermeticity
- **Required Skills**: Starlark, Bazel action analysis, Go report modeling, deterministic artifact encoding, filesystem metrics, integration testing
