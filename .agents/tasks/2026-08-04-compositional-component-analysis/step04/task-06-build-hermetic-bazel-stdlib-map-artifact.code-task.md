# Task: Build Hermetic Bazel Stdlib Map Artifact

## Description
Add the public `arcc_stdlib_map` Bazel rule and the toolchain adapter plumbing that build a target-configuration-specific standard-library authority map from declared SDK sources and the toolchain's package list, in a sandbox that contains no `go` binary at all (design I5). Expose the artifact through the private default seam that Step 5's analysis action will consume.

## Background
`just ci` executes Bazel checks, so the stdlib map cannot depend on native cache discovery or a host toolchain (DR-07). Task 05 gives `arcc stdlibmap generate` an explicit-input mode that loads the standard library through the `packagelayout` driver from an SDK root, a package-list file and a configuration file, executing no toolchain binary. This task wires that mode into one ordinary hermetic action. Step 5 has not yet introduced the component analysis action, so this task establishes the default `_stdlib_map` attribute/helper contract without changing check execution.

cgo-enabled target configurations are out of scope for hermetic generation (design §Explicitly out of scope): the rule rejects them at analysis time rather than producing, or silently substituting, a map. This repository builds with `--@rules_go//go/config:pure`, so its own default configuration is cgo-off.

A first attempt at this task (jj changes `rpskorutppyt` … `rnvkqvqqzsxs`, kept temporarily for reference and to be abandoned) executed the pinned toolchain's `go list` inside the action and declared the toolchain `go` binary and tool binaries as inputs. That approach is superseded (design Appendix C). Reusable from it, by reading rather than rebasing: the adapter seam's shape, the rules_testing analysis-test structure, the transition probes, and the execution test's key/digest assertions.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` — I5, §Explicitly out of scope, §The stdlib authority map, §Error Handling, §Acceptance matrix

**Additional References:**
- Plan Step 4: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response, DR-07 and DR-09: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Task 05: `task-05-add-layout-backed-stdlib-loader.code-task.md` (the CLI contract this action invokes)
- Go host/toolchain seams: `bazel_rules/go/private/go_adapter.bzl`
- Component rule structure: `bazel_rules/go/private/component.bzl`
- Public Go rule surface: `bazel_rules/go/defs.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend only `go_adapter.bzl`'s rules_go-facing seam to expose the pinned SDK source depset, a file locating the SDK root, the toolchain-owned stdlib package-list file, the exact toolchain version, and the target GOOS/GOARCH, cgo state, sorted tags and GOEXPERIMENT. It must **not** expose the toolchain `go` binary or tool binaries to this rule; upper layers must not load `@rules_go` directly.
2. Implement `arcc_stdlib_map` as an ordinary hermetic action whose declared inputs are exactly the SDK sources, the package-list file, the analysis-time configuration file, and the arcc generator as an exec-configuration tool, and whose single output is the canonical map. The action carries `execution_requirements = {"block-network": "1"}` and `use_default_shell_env = False`, and sets no GOROOT, GOCACHE or PATH — the generator needs none.
3. Invoke task 05's explicit-input mode (`--package-list`, `--config-file`, `--sdk-root`, `--output`). The configuration file is written at analysis time from the adapter seam in the format task 05 defines.
4. Fail at analysis time, with an error naming the target and `--@rules_go//go/config:pure`, when the target configuration is cgo-enabled. Also fail fast, naming the target, when the toolchain exposes no version, no package list, or no target platform.
5. Stamp the output key from the target configuration, classifier fingerprint and map format version; cross-compilation and build-tag transitions must build distinct, correctly stamped maps on one execution host.
6. Export the rule from `bazel_rules/go/defs.bzl` with deterministic, non-configurable public attributes.
7. Establish one private default-label/attribute helper for the stdlib map that Step 5's analysis action can attach as `_stdlib_map`; do not add the Step 5 analysis action or change current check verdict execution.
8. Starlark analysis tests (rules_testing) for: action inputs complete and minimal — SDK sources, package list and config present; no file under the SDK's `bin/` or `pkg/tool/`, no cache, home, dependency-component or unrelated workspace path; mnemonic, argv and output shape; `block-network`; provider and default-seam availability; key propagation under a cross-compile transition and a build-tag transition; and an `expect_failure` test for a cgo-enabled transition.
9. An execution test over built artifacts: each map accepts `inspect --expect-key` for its own configuration and rejects the other configurations' keys (including a build-tag difference); the representative classifications from the design (`os.ReadFile`, `strings.TrimSpace`, `sort.Slice`, `os.Stdin`, `io.EOF`, `http.DefaultClient`, `unsafe.Pointer`) hold; a second target with identical inputs produces byte-identical bytes and digest. Document in the test why two distinct-output actions with identical inputs cannot share an action-cache entry, so the replica comparison is a genuine two-execution check.
10. Ordinary `bazel build //...` and `bazel test //...` must not run unrelated component analysis and must not attempt a cgo-enabled map build.
11. Update Bazel BUILD/component metadata and FR10 Component Contract Ambient Authority declarations for any Go package whose responsibilities change.

## Dependencies
- Task 05 provides the explicit-input CLI mode and the layout-backed loader.
- The existing `go_sdk_srcs` and target platform seams are the only allowed rules_go boundary.
- Step 5 will consume the private default `_stdlib_map` seam; no analysis action exists yet.

## Implementation Approach
1. Inventory the rules_go toolchain providers needed for SDK sources, package list, version, and target platform/cgo/tags/GOEXPERIMENT inside `go_adapter.bzl`.
2. RED: analysis tests for inputs, argv, `block-network`, the cgo failure and key propagation; then GREEN with the rule and the configuration-file writer.
3. Re-export a small public rule while keeping implementation and default-label machinery private.
4. RED: the execution test; then GREEN by building the default, replica, cross-compile and tagged variants.
5. Run the full CI gate.

## Acceptance Criteria

1. **The Bazel artifact is hermetic**
   - Given a clean Bazel sandbox with no `go` binary on `PATH` and empty user caches
   - When `arcc_stdlib_map` builds
   - Then generation succeeds using only the declared SDK sources, package list, configuration file and generator, and produces a valid canonical map.

2. **Action inputs are complete and minimal**
   - Given the configured stdlib-map target
   - When its action graph is inspected
   - Then SDK sources, the package list, the configuration file and the generator are declared; no toolchain `go` or tool binary, cache path, home path, dependency-component source or unrelated workspace file is; and the action blocks network access.

3. **Target configurations produce distinct keyed artifacts, and cgo is rejected**
   - Given the default configuration, a cross-compile to another GOOS/GOARCH, and a build-tag variant, all on one execution host, plus a cgo-enabled transition
   - When all analyse and the first three build
   - Then each built map validates against its own SDK key and rejects the others' keys, and the cgo-enabled transition fails analysis with an error naming the target and the pure-mode flag.

4. **The map remains total and semantically pinned**
   - Given a built Bazel map
   - When it is opened through the task 01 reader
   - Then every listed package and every inventoried importable symbol and init has a terminal classification, and the representative classifications match the design.

5. **The public rule and future analysis seam are stable**
   - Given a BUILD file loading `arcc_stdlib_map` and the existing component implementation preparing for Step 5
   - When Bazel analysis runs
   - Then the public rule has validated non-configurable inputs, and one private default `_stdlib_map` contract is available without introducing or running the Step 5 analysis action.

6. **Repeated Bazel builds are deterministic**
   - Given unchanged inputs
   - When two distinct targets with identical inputs are built
   - Then their canonical bytes and digests are identical, and the test explains why the comparison cannot be satisfied by action-cache reuse.

7. **Integration remains green**
   - Given the rule, adapter, tests and metadata
   - When `just ci` runs
   - Then Go and Bazel tests, generation cleanness, self-check and the existing component checks all pass, and wildcard builds attempt no cgo-enabled map.

## Metadata
- **Complexity**: Medium
- **Labels**: bazel, starlark, stdlib-map, hermeticity, rules-go, toolchain, cross-compile
- **Required Skills**: Bazel rule authoring, rules_go toolchains, hermetic actions, Starlark analysis testing, configuration transitions
