# Task: Enforce Restricted-Sandbox Hermeticity

## Description
Add an automated end-to-end hermeticity run that builds and tests component analysis and stdlib-map generation in a restricted Bazel sandbox with no host `go` command, no usable native Go caches, no network, and no writable undeclared cache location.

## Background
Earlier steps established hermetic action shapes and analysis-time input assertions, and layout-mode Go tests already run arcc without a toolchain on `PATH`. Step 13 closes the cross-cutting acceptance row by exercising the actual Bazel actions under hostile host settings rather than relying only on declared-input inspection. The test must cover both the component producer chain (`ArccImportGraph`, `ArccLayout`, `ArccCheck`) and `ArccStdlibMap`, preserve the cgo-disabled target contract, and fail if any action consults or mutates a native cache.

The repository's routine lane may perform one real default-configuration map generation. The hermeticity run must share or deliberately account for that action rather than multiplying the roughly 146-second generation across fixtures.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N2, N5, I5, §Routine stdlib-map validation topology, acceptance matrix “Hermeticity”)

**Additional References:**
- Plan Steps 4, 6, and 13: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Stdlib-map generation cost: `.agents/planning/2026-08-04-compositional-component-analysis/research/2026-09-08-stdlib-map-generation-cost.md`
- Existing hermetic CLI tests: `go/cmd/arcc/layout_integration_test.go`, `go/cmd/arcc/layout_realsdk_integration_test.go`, and `go/cmd/arcc/app/stdlibmap_explicit_integration_test.go`
- Existing action-input assertions: `bazel_rules/go/tests/component_tests.bzl` and `bazel_rules/go/tests/stdlib_map_tests.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add an integration-tagged test driver that invokes the Bazel suite through an absolute Bazel path with sandboxed spawn strategy, network denial, explicit restricted `--sandbox_writable_path` configuration, and an isolated output/user root.
2. Run actions with `PATH` unable to resolve `go` or compiler tools and with `GOROOT`, `GOCACHE`, `GOMODCACHE`, `GOPATH`, `HOME`, and common XDG cache variables either absent or pointing at read-only poison directories outside declared action inputs. Do not repurpose process-global shell variables unsafely in repository scripts.
3. Seed each poison cache/directory with sentinels and snapshot its directory tree and metadata before and after the run. Fail if an arcc action reads successfully from, creates files in, or mutates any undeclared cache location; keep Bazel's own explicitly isolated output/cache directories distinguishable from forbidden native Go caches.
4. Build/test at least one checked component with a direct dependency and the canonical default `arcc_stdlib_map` artifact. Exercise `ArccImportGraph`, `ArccLayout`, `ArccCheck`, report/surface assertion, and real map generation in the same restricted environment.
5. Verify through execution log/action inspection that all four arcc mnemonics declare only their documented files/tools, have empty or explicitly controlled environments, execute the arcc binaries rather than a Go/toolchain binary, and carry network-blocking execution requirements.
6. Ensure the restricted run does not fall back to native on-demand stdlib-map generation or a developer cache. A missing/mismatched declared map must remain a tool error rather than triggering discovery.
7. Reuse the routine default map action across the selected suite and assert the N5 action-count bounds: zero native whole-SDK generation and at most one Bazel whole-SDK generation.
8. Wire the driver into the existing `integration`-tagged Go suite and `just ci`. Fail loudly when the pinned CI environment lacks Bazel or required sandbox support; do not silently skip the acceptance check.
9. Document the exact invocation, sandbox/cache restrictions, action counts, and result in `research/current-analysis-pipeline.md` so the hermeticity evidence is reproducible.

## Dependencies
- Task 2: Measure Bazel Producer-Chain Scaling may provide the isolated Bazel driver and execution-log parsing utilities; reuse them rather than starting a competing harness.
- Step 12 formalized the SDK adapter seams whose source/export/key roles this task exercises.

## Implementation Approach
1. Factor a test-only Bazel invocation helper that creates explicit temporary output, writable, and poison-cache directories and records filesystem snapshots.
2. Select a compact checked dependency chain and canonical map target, then run the real sandboxed build/test with hostile environment settings.
3. Parse the execution log for action command, environment, declared inputs, execution requirements, and mnemonic counts; pair this with before/after sentinel checks.
4. Add a focused failure fixture only if needed to prove no native fallback, keeping it manual outside wildcard builds when failure is intentional.

## Acceptance Criteria

1. **Checks run without a host toolchain**
   - Given a sandbox environment whose `PATH` cannot resolve `go` and whose `GOROOT`/Go cache variables do not expose a toolchain
   - When a dependent component's analysis and assertion targets run
   - Then `ArccImportGraph`, `ArccLayout`, and `ArccCheck` succeed using declared metadata, export data, surfaces/reports, and map inputs only.

2. **Map generation is hermetic**
   - Given the same restricted environment and cgo-disabled target configuration
   - When the default `ArccStdlibMap` action executes
   - Then it succeeds through the layout driver using declared SDK source/oracle files, invokes no Go/toolchain executable, uses no native cache, and performs no network access.

3. **Undeclared caches remain untouched**
   - Given read-only poison native cache directories with recorded sentinels
   - When the restricted suite completes
   - Then their trees and metadata are unchanged and no undeclared cache path appears in an arcc action's inputs, environment, or command line.

4. **Action contracts are observed at execution time**
   - Given the restricted run's machine-readable execution log
   - When commands and inputs are classified
   - Then each arcc mnemonic executes only its intended arcc tool with controlled environment/network settings and its documented declared-input role set.

5. **Generation remains bounded**
   - Given all selected hermeticity targets in one run
   - When stdlib-map actions are counted
   - Then the run performs zero native whole-SDK generations and no more than one default Bazel whole-SDK generation.

6. **Hermeticity is a routine regression test**
   - Given the pinned CI host with Bazel sandbox support
   - When `just test-integration` and `just ci` run
   - Then the restricted-sandbox test executes automatically, fails on unsupported/missing prerequisites, and records no state in the developer workspace.

## Metadata
- **Complexity**: High
- **Labels**: bazel, integration, hermeticity, sandbox, stdlib-map, caches
- **Required Skills**: Bazel sandboxing and execution logs, hermetic build testing, Go integration harnesses, filesystem mutation detection, toolchain isolation
