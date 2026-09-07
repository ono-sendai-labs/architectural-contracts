# Task: Add Layout-Backed Stdlib Loader for Hermetic Map Generation

## Description
Give `arcc stdlibmap generate` an explicit-input mode that loads the standard library through the `packagelayout` `GOPACKAGESDRIVER` driver from declared SDK sources, executing no toolchain binary (design I5). Extend `packagelayout` so a layout computed for a pinned target configuration is faithful to that configuration: `goexperiment` and `toolchain_version` on `platform`, release/tool tags derived from them instead of `build.Default`, and the target GOARCH in the driver response. This is the loader the Bazel `arcc_stdlib_map` rule (task 06) invokes.

## Background
The Bazel check already loads its whole closure — standard library included — with full syntax and types through the self-exec `GOPACKAGESDRIVER` driver in `go/internal/packagelayout`, and Capslock's own load goes through the same driver. `discoverStdlibWithContext` already enumerates an SDK tree with `go/build`, mirrors `go list std`'s exclusions, resolves the GOROOT `vendor/` tree and synthesises `unsafe`; `HandleDriverRequest` already serves a `std` pattern. Map generation (task 03) needs exactly that: a `go/types` inventory and a Capslock run over every importable stdlib package. Task 04's `Loader` seam is an injectable interface, so a layout-backed loader is a new implementation, not a contract change.

Three fidelity gaps in the driver must close for a layout to describe a *pinned* target rather than the running binary: `BuildContextForLayout` copies `ReleaseTags` and `ToolTags` from `build.Default` (arcc's own build toolchain), the layout `platform` block has no GOEXPERIMENT, and `HandleDriverRequest` reports `runtime.GOARCH` as `Arch`, which `go/packages` turns into `types.Sizes`. All three also affect the existing check's driver, so fixing them here fixes both consumers.

A first attempt at the Bazel artifact (jj changes `rpskorutppyt` … `rnvkqvqqzsxs`, kept temporarily for reference and to be abandoned) executed the pinned toolchain's `go list` inside the action instead. That approach is superseded (design Appendix C, I5). Reusable from it, by reading rather than rebasing: the key=value target-configuration file format and its parser, the CLI usage tests, and the `--expect-key tag=<v>` fix.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` — I3, I5, §Explicitly out of scope, §The stdlib authority map (*Oracle*, *Loading*), §Error Handling, Appendix C

**Additional References:**
- Plan Step 4: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response, DR-07 and DR-09: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Driver: `go/internal/packagelayout/packagelayout.go` (`discoverStdlibWithContext`, `BuildContextForLayout`, `HandleDriverRequest`, `WithTemporaryLayout`, `WithDriverEnv`)
- Generator seams: `go/internal/stdlibmap/inventory.go` (`Loader`), `go/internal/stdlibmap/generate.go`, `go/internal/capslockadapter/generation.go`
- CLI: `go/cmd/arcc/app/stdlibmap.go`
- Earlier driver design: `.agents/planning/2026-07-16-bazel-arcc-rules/design/detailed-design.md`, `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver-findings.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend the layout `platform` block with `toolchain_version` (the `go1.N.M` spelling the SDK key uses) and `goexperiment` (the enabled experiments, deterministically serialised and round-tripping into the SDK key's GOEXPERIMENT field). Both are optional for hand-written layouts; when absent, behaviour is unchanged.
2. When `platform.toolchain_version` is present, `BuildContextForLayout` derives `ReleaseTags` from it (`go1.1` … `go1.N`) instead of copying `build.Default`; when `platform.goexperiment` is present, it derives `goexperiment.<name>` tool tags from it. Arch feature tags are the toolchain's defaults for the target GOARCH; a non-default level (GOAMD64, GOARM, …) is out of scope and must not be silently taken from the host.
3. `HandleDriverRequest` reports the layout platform's GOARCH as `Arch` when a platform block is present, falling back to `runtime.GOARCH` only without one.
4. Add a whole-stdlib layout builder in `packagelayout` that, given an SDK root and a platform, produces a validated layout whose packages are the discovered standard library (stdlib provenance set, files resolved under the SDK root, import graph transitively complete, `vendor/` resolved) and whose roots are the importable packages. Reuse `discoverStdlibWithContext`; do not duplicate it.
5. Add a `stdlibmap` loader implementing the task 04 `Loader` seam that serves such a layout through the driver (`WithTemporaryLayout`/`WithDriverEnv`), and route the Capslock generation load through the same driver environment, so explicit-input generation performs no `exec` of any binary other than arcc's own self-exec driver. `go/packages` must never fall back to the `go list` driver in this mode; assert that (e.g. by running with an empty `PATH` and no `GOROOT`).
6. Explicit-input oracle: the toolchain package-list file is the enumeration. A listed package that the layout discovers is enumerated with the existing `internal/…` rule; a listed package whose directory exists but whose files are all excluded by the target's build constraints is enumerated `importable: false`; a listed package with no directory, or a discovered package the list omits, fails generation with an error naming the package (I3).
7. CLI: `arcc stdlibmap generate --package-list=<file> --config-file=<file> --sdk-root=<dir> --output=<path>` selects explicit-input mode. The three explicit flags are all-or-nothing and single-occurrence; a partial set, a duplicate, or any native-discovery flag (`--toolchain`, `--goos`, `--goarch`, `--cgo`, `--tags`, `--goexperiment`) alongside them is a usage error, exit 2. Explicit mode never calls `NativeTargetConfig`, `NativeStdPackageList`, `go env` or `go list`. The config file is deterministic `key=value` lines: `toolchain_version`, `goos`, `goarch`, `cgo_enabled`, `build_tags`, `goexperiment`; a missing, duplicate or malformed key is an error naming it.
8. Explicit mode rejects `cgo_enabled=true` before loading, with an error that names the design's out-of-scope decision, rather than failing inside type-checking on `import "C"`. Native mode is unchanged.
9. Fix `arcc stdlibmap inspect --expect-key tag=<v>`, which currently verifies nothing because the checked field is spelled `tag` while the mismatch report names `build_tags`; cover it with a test that fails before the fix.
10. Update FR10 Component Contract blocks and `component.textproto` declarations for `packagelayout`, `stdlibmap` and `cli`: explicit-input mode adds no EXEC beyond arcc's self-exec driver, and the layout builder reads only the SDK root.

## Dependencies
- Task 03 provides the generator and its classifier; task 04 the `Loader` seam, `TargetEnv`, and the `generate|inspect` commands.
- `packagelayout` is the existing Bazel driver; its tests (`packagelayout_test.go`) must keep passing unchanged except where a requirement above changes documented behaviour.
- Task 06 consumes the CLI contract defined in requirement 7 from a Bazel action.

## Implementation Approach
1. RED: layout tests for `toolchain_version`/`goexperiment` round-trip, release/tool-tag derivation, and the target `Arch`; then GREEN in `packagelayout`.
2. RED: a whole-stdlib layout builder test against a fixture SDK tree (existing testdata) and, under `-tags=integration`, against the host SDK; then GREEN.
3. RED: a `Loader` test that loads a fixture package through the driver with an empty `PATH`; then GREEN with the layout loader, and route Capslock's generation load through the same environment.
4. RED: CLI usage tests for the all-or-nothing flag set, the cgo rejection, the oracle discrepancies, and the `--expect-key tag=` fix; then GREEN.
5. Integration: generate the host configuration's map both natively and through explicit mode and compare.

## Acceptance Criteria

1. **The layout describes the pinned target, not the running binary**
   - Given a layout whose platform names `toolchain_version: go1.26.4` and one enabled experiment, and a fixture package with files constrained by `//go:build go1.27`, `//go:build goexperiment.<name>`, and a GOARCH suffix for a non-host architecture
   - When the layout is validated and served through the driver
   - Then the `go1.27` file is excluded, the experiment file is included, the driver response reports the platform's GOARCH, and a layout without a platform block behaves exactly as before.

2. **The whole-stdlib layout matches the native oracle**
   - Given the host SDK root and the host configuration (integration test)
   - When the layout builder runs and `go list std` runs for the same configuration
   - Then the set of importable packages is identical and every package's file list equals `go list`'s `GoFiles`.

3. **Explicit-input generation executes no toolchain binary**
   - Given a package-list file, a config file, and an SDK root, with `PATH` set to an empty directory and `GOROOT`, `GOCACHE` and `GOPACKAGESDRIVER` unset
   - When `arcc stdlibmap generate` runs in explicit mode
   - Then it succeeds, the map's key matches the config file, and every symbol of every importable package has a terminal classification.

4. **Explicit and native generation agree**
   - Given the host toolchain's own configuration (integration test)
   - When a map is generated natively and one is generated in explicit mode from the same SDK root with a package list produced from `go list std`
   - Then the two maps are byte-identical; any difference is a task failure, not a documented deviation.

5. **The oracle cross-check is total**
   - Given a package list with (a) a package whose files are all build-excluded for the target, (b) a package with no directory, and (c) a list omitting one discovered package
   - When explicit generation runs
   - Then (a) is enumerated `importable: false`, and (b) and (c) each fail with an error naming the package.

6. **Explicit mode fails closed on its inputs**
   - Given `--config-file` without `--package-list`, a duplicate `--sdk-root`, `--goos` alongside the explicit flags, a config file with a malformed `cgo_enabled`, and a config file with `cgo_enabled=true`
   - When `arcc stdlibmap generate` runs
   - Then each exits 2 with a usage or actionable error, and nothing is discovered from the host in any of them.

7. **`--expect-key tag=<v>` verifies the build tags**
   - Given a map whose key carries build tag `probe`
   - When `inspect --expect-key tag=other` runs
   - Then it fails, and `tag=probe` passes; the test fails on the code before the fix.

8. **Integration remains green**
   - Given the driver changes, loader, CLI and metadata
   - When `just ci` runs
   - Then Go and Bazel tests, generation cleanness, self-check and the existing component checks all pass.

## Metadata
- **Complexity**: High
- **Labels**: stdlib-map, packagelayout, gopackagesdriver, hermeticity, cli, go-build
- **Required Skills**: go/build constraint semantics, go/packages driver protocol, go/types, Capslock, Go CLI design, TDD
