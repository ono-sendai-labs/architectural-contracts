# Task: Remove Toolchain Binaries from ArccCheck Stdlib Inputs

## Description
Remediate implementation-review finding F1 by narrowing the standard-library type-data contract so `ArccCheck` receives only package metadata and compiler export artifacts, never Go/toolchain executables hidden inside an opaque rules_go tree input.

## Background
Addresses finding F1 from the Step 13 implementation review. Design invariant I5 says Go toolchain binaries are never Bazel action inputs, and N2 limits `ArccCheck` to its explicit semantic allowlist. The restricted-sandbox audit currently acknowledges that rules_go's expanded `stdlib_` input tree contains `pkg/tool` entries and then exempts that entire tree from its forbidden-tool predicate, proving only that the binaries are not selected as Bazel tools or placed in argv.

The fix must make the declared input shape satisfy the design rather than restating the exemption. If pinned rules_go cannot expose individual export archives without its mixed-content tree artifact, introduce one shared, target-configured, hermetic projection of the metadata-referenced export files; do not add a per-component semantic-analysis action or execute a Go/toolchain binary.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N2, I5, §Type loading, acceptance matrix “Action inputs” and “Hermeticity”)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step13.yaml`

**Additional References:**
- `bazel_rules/go/private/go_adapter.bzl`
- `bazel_rules/go/private/component.bzl`
- `go/internal/goanalysis/hermeticity_integration_test.go`
- `bazel_rules/go/tests/stdlib_export_data_tests.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Change the upstream `go_stdlib_export_data` adapter contract so its `inputs` resolve, including expanded tree contents visible to execution logs, to package metadata and compiler export data only.
2. Do not pass an SDK source tree, `bin/go`, any `pkg/tool` executable, a native Go cache, or unrelated rules_go build material to `ArccCheck`.
3. If an export-only projection is necessary, make it a single shared artifact per target SDK configuration, generated hermetically from declared inputs without invoking Go/toolchain executables; retain one `ArccCheck` action per checked component.
4. Keep layout `ExportFile` paths and the in-sandbox runfiles frame consistent with the narrowed artifacts, and preserve fail-closed diagnostics for missing/mismatched export data.
5. Remove the `/stdlib_/` blanket exemption from `hermeticityForbiddenToolPath`. Inspect expanded execution-log inputs and reject `/bin/go`, `/pkg/tool/`, compiler/linker executables, native cache paths, SDK sources, and unclassified stdlib-tree content regardless of parent path.
6. Preserve the empty action environment, `block-network=1`, poisoned-cache isolation, source-free non-member load, one default `ArccStdlibMap` generation, and pass-verdict assertions.
7. Update the recorded hermeticity evidence only after a fresh restricted run demonstrates the narrowed input set.

## Dependencies
- Step 8's export-data loader and layout schema define the `ExportFile` paths that must continue to resolve.
- Step 12's SDK export-data adapter seam is the integration point to narrow.
- No dependency on Task 06.

## Implementation Approach
1. Inspect rules_go's target-configured metadata and mixed tree contents, then choose the smallest export-only representation compatible with Bazel's declared-file model.
2. Add failing analysis/execution-log tests for expanded `pkg/tool`, `bin/go`, SDK source, cache, and unrelated input descendants before changing the adapter.
3. Implement the narrowed descriptor or shared projection and update layout/frame construction without changing component verdict semantics.
4. Run the restricted producer-chain suite and repository validation, then refresh the reproducible evidence table.

## Acceptance Criteria

1. **ArccCheck receives export-only stdlib material**
   - Given the real pinned rules_go target configuration
   - When an `ArccCheck` action's declared inputs are expanded from the execution log
   - Then they contain only stdlib package metadata and compiler export artifacts, with no `bin/go`, `pkg/tool`, SDK source, native cache, or unrelated tree content.

2. **Toolchain-free loading remains functional**
   - Given the narrowed stdlib inputs and a PATH with no Go/compiler tools
   - When a checked component with a checked direct dependency runs in linux-sandbox
   - Then both reports decode with PASS verdicts and all non-member types load from declared export data.

3. **Hermeticity assertion has no path exemption**
   - Given any forbidden tool/cache/source path beneath a directory named `stdlib_`
   - When the hermeticity input classifier evaluates it
   - Then the test rejects it exactly as it would reject the same path elsewhere.

4. **Producer topology remains bounded**
   - Given all routine scaling and hermeticity targets
   - When their actions are counted
   - Then each component still has one `ArccCheck`, no per-component toolchain/export-analysis action is introduced, and at most one default `ArccStdlibMap` action executes.

5. **Fail-closed export validation is preserved**
   - Given missing, incomplete, or target-mismatched projected stdlib export data
   - When component analysis is requested
   - Then Bazel analysis or `ArccCheck` fails with an actionable error rather than falling back to native discovery.

## Metadata
- **Complexity**: High
- **Labels**: bazel, go, hermeticity, stdlib, export-data, remediation
- **Required Skills**: Bazel tree artifacts and execution logs, Starlark providers/actions, rules_go GoStdLib integration, Go export-data loading, hermetic integration testing
