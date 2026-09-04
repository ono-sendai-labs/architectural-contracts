# Task: Build Hermetic Bazel Stdlib Map Artifact

## Description
Add the public `arcc_stdlib_map` Bazel rule and toolchain adapter plumbing that build a target-configuration-specific standard-library authority map from declared SDK inputs in a sandbox with no network or host `go` binary. Expose the artifact through the private default seam that Step 5's analysis action will consume.

## Background
`just ci` executes Bazel checks, so the stdlib map cannot depend on native cache discovery or a host toolchain. The rule must receive SDK sources through the existing `go_sdk_srcs` seam, a complete toolchain-owned stdlib package list, and target platform/key data. Its action invokes the Step 4 generator using only declared inputs and produces one canonical map for the target configuration. Step 5 has not yet introduced the component analysis action, so this task establishes the default `_stdlib_map` attribute/helper contract without prematurely changing check execution.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Plan Step 4: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response, DR-07: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Go host/toolchain seams: `bazel_rules/go/private/go_adapter.bzl`
- Component rule structure: `bazel_rules/go/private/component.bzl`
- Public Go rule surface: `bazel_rules/go/defs.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend only `go_adapter.bzl`'s rules_go-facing seam to expose the pinned SDK source depset, complete stdlib package enumeration, target toolchain version, target GOOS/GOARCH, cgo state, sorted tags, and GOEXPERIMENT needed by generation; upper layers must not load `@rules_go` directly.
2. Implement `arcc_stdlib_map` as an ordinary hermetic action that declares the generator executable, SDK sources, package-list/config files, and all other toolchain material it reads as inputs and declares exactly one canonical map output.
3. Invoke the explicit-input generation path from Task 4 so the action never consults user cache directories, runs `go list`, resolves a host `go` binary, accesses the network, or reads undeclared host files.
4. Stamp the output key from the target configuration, classifier fingerprint, and map format version. Cross-compilation and cgo toggles must build distinct correctly stamped maps even when executed on one host.
5. Export the rule from `bazel_rules/go/defs.bzl` with deterministic, non-configurable public attributes and fail-fast validation that names the target.
6. Establish one private default-label/attribute helper for the stdlib map that Step 5's analysis action can attach as `_stdlib_map`; do not add the Step 5 analysis action or change current check verdict execution in this task.
7. Add Starlark analysis tests for action inputs, mnemonic/argv/output shape, provider/default availability, and configuration key propagation, plus an execution test that removes the host `go` binary from `PATH` and denies network access.
8. Ensure generated/source artifacts and rule action order are deterministic and that ordinary `bazel build //...` behavior does not eagerly run unrelated component analysis.
9. Update Bazel BUILD/component metadata and FR10 Component Contract Ambient Authority declarations for any Go packages whose responsibilities change.

## Dependencies
- Task 3 provides the deterministic explicit-input generator.
- Task 4 provides the stable `arcc stdlibmap generate` executable contract.
- The existing `go_sdk_srcs` and target platform seams are the only allowed rules_go boundary.
- Step 5 will consume the private default `_stdlib_map` seam; no analysis action exists yet to wire in this step.

## Implementation Approach
1. Inventory the exact rules_go toolchain providers needed for SDK package enumeration, sources, version, target platform, cgo, tags, and GOEXPERIMENT inside `go_adapter.bzl`.
2. Build deterministic package-list and configuration parameter files at analysis time and pass them with SDK sources to one generator action.
3. Re-export a small public macro/rule while keeping implementation and default-label machinery private.
4. Use rules_testing analysis assertions to pin declared inputs and argv, then execute the artifact in a sandbox fixture with no host toolchain/network.
5. Build multiple target configurations, inspect keys and representative classifications, and run the full CI gate.

## Acceptance Criteria

1. **The Bazel artifact is hermetic**
   - Given a clean Bazel sandbox with network disabled, no host `go` binary in `PATH`, and empty user caches
   - When `arcc_stdlib_map` builds
   - Then generation succeeds using only declared pinned toolchain inputs and produces a valid canonical map.

2. **Action inputs are complete and minimal**
   - Given the configured stdlib-map target
   - When its action graph is inspected
   - Then SDK sources, toolchain package enumeration/configuration, and generator executable are declared, while home/cache paths, dependency component sources, and unrelated workspace files are absent.

3. **Target configurations produce distinct keyed artifacts**
   - Given linux/amd64 cgo on and off plus a supported cross-compile configuration
   - When all maps build on the same execution host
   - Then each output validates against its own distinct target SDK key and rejects lookup under either other key.

4. **The map remains total and semantically pinned**
   - Given a built Bazel map
   - When it is opened through the Task 1 reader
   - Then every toolchain-enumerated package and inventoried importable symbol/init has a terminal classification, and representative `os.ReadFile`, `strings.TrimSpace`, `sort.Slice`, `os.Stdin`, `io.EOF`, `http.DefaultClient`, and `unsafe.Pointer` results match the design.

5. **The public rule and future analysis seam are stable**
   - Given a BUILD file loading `arcc_stdlib_map` and the existing component implementation preparing for Step 5
   - When Bazel analysis runs
   - Then the public rule has validated non-configurable inputs, and one private default `_stdlib_map` contract is available without introducing or running the Step 5 component analysis action.

6. **Repeated Bazel builds are deterministic**
   - Given unchanged toolchain and classifier inputs
   - When the output is rebuilt without remote/local cache reuse
   - Then canonical bytes and digest are identical.

7. **Integration remains green**
   - Given the Bazel rule, adapters, tests, and metadata
   - When `just ci` runs
   - Then Go and Bazel tests, generation cleanliness, self-check, and existing component checks all pass.

## Metadata
- **Complexity**: High
- **Labels**: bazel, starlark, stdlib-map, hermeticity, rules-go, toolchain, cross-compile
- **Required Skills**: Bazel rule authoring, rules_go toolchains, hermetic actions, Starlark analysis testing, Go CLI integration, configuration transitions
