# Task: Extend Go adapter seams

## Description
Complete the Bazel host-adapter contract needed by declared membership and future infrastructure-component attachment. Pin platform extraction to the Go target's build mode, add the infrastructure attachment predicate and registry, and keep all host-specific knowledge confined to `go_adapter.bzl`.

## Background
`bazel_rules/go/private/go_adapter.bzl` is the single replaceable boundary between rules_arcc and a host's Go rules. It already exposes `go_build_platform` for rules_go and correctly reads `GoInfo.mode`; this task must preserve that implementation and add an analysis test proving it reports the target platform rather than the Go SDK's execution platform.

Step 9 will use an infrastructure registry and attachment predicate to add package-surface component dependencies. Those hooks must land now so the later component-rule work does not introduce host knowledge outside the adapter. Some hosts cannot observe injected runtime packages during analysis, so `go_attach_infra` is explicitly allowed to return `True` unconditionally. Registry entries must be able to describe concrete labels or import-path patterns because visibility-gated toolchain packages cannot always be named as Bazel targets.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 T2, A7, B1–B2; §4.7; §7.3; and §9)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 6)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/build-platform-and-tags.md` (rules_go target-platform fields and cgo approximation)
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` (F2–F5 and F7)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Keep `go_build_platform(target)` in `bazel_rules/go/private/go_adapter.bzl` as the sole host seam for GOOS, GOARCH, build tags, and cgo state.
2. For rules_go, derive the returned values from `target[GoInfo].mode`: `goos`, `goarch`, `tags`, and `cgo_enabled = not mode.pure`. Do not read `GoSDK.goos` or `GoSDK.goarch`, which describe the execution platform.
3. Document that a host whose Go rules expose no target-platform metadata may return a fixed constant and that this is conforming behavior, not a degraded fallback.
4. Retain a comment that rules_go's `pure` flag is only an approximation of cgo availability because it does not prove a working C toolchain exists.
5. Add `go_attach_infra(target, infra)` to the adapter. Document its target and registry-entry inputs and that returning `True` unconditionally is conforming when a host cannot inspect toolchain-injected packages during analysis.
6. Add an `INFRA_COMPONENTS` registry representation that can express both concrete Bazel labels and import-path patterns without forcing inaccessible packages to become labels.
7. Document the registry with a worked label entry and a worked pattern entry, including the fields Step 9 will need to attach the corresponding component dependency. Do not leave only an unexplained empty list.
8. Keep the upstream registry safe for existing users: illustrative entries must be comments or otherwise non-attaching examples, so this task does not inject dependencies into current components.
9. Do not add a package-path-to-target naming hook and do not add `arcc_subpackages()` or any other membership expansion helper.
10. Add focused Bazel analysis coverage for `go_build_platform` that distinguishes target settings from execution-platform SDK settings. The assertion must fail if the implementation is changed to read `GoSDK`.
11. Add direct coverage for the default upstream `go_attach_infra` behavior and registry shape where practical without coupling component emission to Step 9.
12. Do not emit a platform block or auto-attached manifest edge in this task; those consumers belong to Steps 7 and 9 respectively.

## Dependencies
- Steps 1–5 are complete and supply the manifest, loader, and platform data model that later Bazel emission will consume.
- No task within Step 6 precedes this task.
- `task-02-collect-member-layout-inputs` may rely on the adapter remaining the only rules_go dependency seam, but does not consume infrastructure attachment yet.

## Implementation Approach
1. Review every direct `@rules_go` use in the Bazel rules and preserve the existing adapter boundary while adding the two new declarations and documented registry representation.
2. Reuse or extend the existing analysis-test probe infrastructure to expose `go_build_platform` results from a target configured for a platform distinguishable from the execution SDK.
3. Add small assertions for GOOS, GOARCH, tags, and cgo mapping, with the target-versus-exec distinction explicit in fixture names and failure messages.
4. Represent registry examples in a form Step 9 can consume without redesign, while keeping the live upstream registry empty and behavior unchanged.
5. Run the focused Bazel analysis suite, then the repository-wide checks.

## Acceptance Criteria

1. **Target platform is extracted**
   - Given a rules_go target configured for a target platform that differs from the execution platform
   - When `go_build_platform` is evaluated
   - Then its GOOS and GOARCH match `GoInfo.mode`, not the Go SDK host fields.

2. **Tags and cgo mapping are preserved**
   - Given a target mode with explicit build tags and `pure` state
   - When `go_build_platform` returns its struct
   - Then tags are stable and `cgo_enabled` is the inverse of `pure`, with the approximation documented.

3. **Fixed constants remain a conforming host option**
   - Given a host adapter author reads the platform seam contract
   - When their Go rules expose no target-platform metadata
   - Then the documentation explicitly permits a fixed constant rather than implying ambient host detection or a best-effort fallback.

4. **Infrastructure attachment is host-swappable**
   - Given a future component-rule caller supplies a Go target and an infrastructure registry entry
   - When it invokes `go_attach_infra`
   - Then the decision is made entirely in `go_adapter.bzl`, and the contract explicitly permits unconditional `True`.

5. **Registry supports both identifier forms**
   - Given the documented `INFRA_COMPONENTS` examples
   - When an adapter author models a target-addressable component and a visibility-gated runtime
   - Then one can be represented by a concrete label and the other by an import-path pattern.

6. **Examples have no current side effects**
   - Given the upstream adapter after this task
   - When existing Bazel components are analyzed
   - Then no new component dependency is attached and existing manifest/layout goldens remain unchanged.

7. **No unsupported expansion API is introduced**
   - Given the public and private Bazel rule APIs
   - When searched after this task
   - Then there is no `arcc_subpackages` helper and no package-path-to-target naming hook.

8. **Repository checks remain green**
   - Given the extended adapter contract and its analysis tests
   - When `just ci` runs
   - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Bazel, Starlark, rules_go, host-adapter, platform, infrastructure
- **Required Skills**: Starlark, Bazel analysis testing, rules_go providers, platform transitions, API contract design
