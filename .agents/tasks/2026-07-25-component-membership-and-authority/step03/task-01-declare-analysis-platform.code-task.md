# Task: Declare the analysis platform

## Description
Make the package layout's target platform explicit and use it for every build-constraint decision, so analysis no longer depends on the platform or ambient environment of the `arcc` binary. Reject packages whose declared source set is entirely removed by that filtering.

## Background
`packagelayout.ValidateAndResolve` currently filters `GoFiles` and `CompiledGoFiles` with `build.Default`, and `FileMatchesBuildConstraints` does the same for individual files. That context reflects the running binary and its environment, not necessarily the target being analyzed. A layout may instead declare `goos`, `goarch`, build tags, and cgo state. Existing layouts without that block retain current behavior by using a copy of `build.Default`.

The Bazel adapter is the host-specific seam for extracting this data. Under rules_go, target settings come from `GoInfo.mode`; `GoSDK.goos` and `GoSDK.goarch` describe the execution host and must not be used. Some conforming hosts expose no target-platform metadata, so their adapter may return a fixed constant. The Starlark emitter does not consume the seam until Step 6.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 T1–T2, §4.4, §4.7, §5.1, §6, and §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 3)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/build-platform-and-tags.md` (rules_go platform sources and cgo approximation)
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` (F1–F2)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add an optional `platform` object to `packagelayout.Layout` with JSON fields `goos`, `goarch`, `build_tags`, and `cgo_enabled`, using a named Go type whose field comments define the layout contract.
2. Preserve deterministic layout JSON behavior: marshal the platform block when present, omit it when absent, continue sorting roots and packages without mutating caller-owned data, and cover parsing and round-trip serialization.
3. Build one `build.Context` per validated layout. Start from a copy of `build.Default`; when `platform` is present, replace `GOOS`, `GOARCH`, `BuildTags`, and `CgoEnabled` with the declared values while retaining defaults such as `GOROOT`, compiler, tool tags, and release tags.
4. Define and validate the required platform shape. At minimum, a present block must not silently fall back for an empty or unsupported `goos` or `goarch`; errors must identify the invalid field and value. Preserve declared build tags deterministically and avoid mutating `build.Default`.
5. Pass the derived context through standard-library discovery, `filterByBuildConstraints`, and the single-file `FileMatchesBuildConstraints` path so suffix constraints, `//go:build` expressions, custom tags, and cgo all use the same target platform.
6. Keep the existing fail-open behavior for a file whose constraints cannot be evaluated because the file is unreadable, and keep non-Go paths unchanged.
7. When a non-stdlib package had one or more `.go` source files before constraint filtering and has none afterward, return a load error naming the package and explaining that the declared platform excluded every Go source. Do not reject a package that was already bodiless before filtering; later work reports that distinct limitation.
8. Apply the empty-after-filter check to every emitter-listed package, not only roots, because vacuous dependency type-checking is also unsound. Continue enforcing the existing stronger no-source rule for member/root packages.
9. Add `go_build_platform(target)` to `bazel_rules/go/private/go_adapter.bzl` as the target-platform extraction seam. The rules_go implementation must use `target[GoInfo].mode` for GOOS, GOARCH, tags, and `pure`; it must document that `cgo_enabled = not pure` is an approximation.
10. Document on the adapter seam that a host may return a fixed constant when its Go providers expose no target-platform metadata, and that this is conforming rather than degraded behavior. Do not wire the returned value into layout emission in this task; that emitter half belongs to Step 6.
11. Update stale comments that claim filtering follows the platform for which the `arcc` binary was compiled.
12. Do not implement import-set consistency, unresolved-import reporting, interface-file exclusion reporting, or standard-library provenance changes in this task.

## Dependencies
- Steps 1–2 (complete): declared member roots and ownership-based authority attribution.
- No task within Step 3 precedes this task.
- `task-02-validate-platform-imports` depends on the platform-derived filtered source set established here.

## Implementation Approach
1. Introduce the platform schema and deterministic JSON coverage before changing filtering behavior.
2. Add a small helper that derives a fresh `build.Context` from a layout, and thread that value explicitly through validation, SDK discovery, bulk filtering, and single-file matching rather than mutating global state.
3. Capture whether each non-stdlib package had Go sources before filtering, filter both `GoFiles` and `CompiledGoFiles`, and issue the package-specific error only for a transition from non-empty to empty.
4. Implement the rules_go adapter seam from `GoInfo.mode`, including the cgo approximation and fixed-constant portability contract, while leaving `component.bzl` emission unchanged.
5. Add focused table tests for absence/default behavior, build tags, cross-platform suffixes, cgo, empty-after-filter errors, JSON determinism, and adapter contract coverage before running the full repository checks.

## Acceptance Criteria

1. **Absent platform preserves compatibility**
   - Given a hand-written layout without a `platform` block
   - When it is parsed and validated
   - Then constraint filtering uses an unmodified copy of `build.Default` and existing native/layout behavior remains unchanged.

2. **Declared build tags select files**
   - Given a package with a source guarded by `//go:build purego`
   - When one layout declares `build_tags: ["purego"]` and an otherwise identical layout does not
   - Then the tagged layout keeps the file and the untagged layout excludes it.

3. **Declared target overrides the host**
   - Given mutually exclusive `_GOOS.go` source variants and a layout whose `goos` differs from the machine running the test
   - When the layout is validated
   - Then only the source for the declared target remains, independent of the host GOOS and ambient environment.

4. **Cgo constraints use declared data**
   - Given files selected by the `cgo` build constraint
   - When layouts differ only in `cgo_enabled`
   - Then file matching follows the declared value.

5. **All constraint APIs share one context**
   - Given the same file and declared platform
   - When bulk package filtering and the single-file matching API evaluate it
   - Then both return the same result, and SDK discovery uses the same GOOS, GOARCH, tags, and cgo settings.

6. **Constraint-empty package fails closed**
   - Given any non-stdlib layout package initially contains Go sources but all are excluded by the declared platform
   - When `ValidateAndResolve` runs
   - Then it fails with the package path and an all-sources-excluded explanation rather than leaving a vacuous package.

7. **Pre-existing bodiless package remains distinct**
   - Given a non-root package declares no Go source before filtering
   - When the layout is validated
   - Then this task does not newly reject it as constraint-empty, while the existing root/member no-source rule remains enforced.

8. **Platform JSON is stable**
   - Given layouts with and without platform data
   - When they are parsed, marshaled, and marshaled repeatedly
   - Then the optional block round-trips correctly, output remains deterministic, and caller-owned slices are not mutated.

9. **Host seam states the full contract**
   - Given the upstream rules_go adapter
   - When `go_build_platform` is inspected and exercised by Starlark tests where practical
   - Then it reads the target's `GoInfo.mode`, maps tags and `pure` correctly, avoids `GoSDK` host fields, and documents both the cgo approximation and conforming fixed-constant implementation.

10. **Repository checks remain green**
    - Given the declared-platform implementation
    - When `just ci` runs
    - Then all Go tests, Bazel tests, generation-cleanliness checks, and self-checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, packagelayout, build-constraints, Bazel, host-seam
- **Required Skills**: Go, `go/build`, JSON schema design, rules_go providers, Starlark testing
