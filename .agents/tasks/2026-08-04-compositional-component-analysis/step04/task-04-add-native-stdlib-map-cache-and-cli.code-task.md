# Task: Add Native Stdlib Map Cache and CLI

## Description
Add native on-demand generation with a target-keyed atomic cache, plus `arcc stdlibmap generate` and `arcc stdlibmap inspect` commands for producing and querying canonical map artifacts.

## Background
Native use needs the same fail-closed artifact as Bazel without regenerating roughly 1.4 GB of analysis work on every invocation. Cache identity must include the complete `SDKKey`; stale or corrupt content is regenerated atomically, while an explicitly supplied mismatched artifact is rejected. The CLI provides the user-visible generation and inspection path required by the Step 4 demo and a stable executable interface for the later Bazel rule.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Plan Step 4: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Stdlib generation spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md`
- CLI orchestration conventions: `go/cmd/arcc/app/app.go`
- Atomic artifact writes: `go/internal/artifactio/write.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a native cache service keyed by a collision-resistant deterministic encoding/digest of the complete `SDKKey`, rooted under the platform user cache directory in an `arcc`-specific subtree.
2. On a cache miss, generate through Task 3 and write canonical bytes with Step 3 atomic-write semantics. Concurrent generators must never expose partial bytes; a failed generation must leave any previous valid artifact intact.
3. Decode and validate every cache hit against the requested key, classifier hash, and format version. Treat truncated, malformed, invalid, mismatched, or otherwise corrupt cache content as a miss and regenerate it; do not accept it or silently downgrade classifications.
4. Provide injectable cache-root, discovery, generator, and filesystem seams so unit tests do not mutate the real user cache or require a full SDK analysis.
5. Add `arcc stdlibmap generate` with explicit output and target-configuration inputs sufficient for native use and later Bazel invocation; default native discovery may use the current toolchain and `go list std`.
6. Add `arcc stdlibmap inspect` to validate an artifact and print deterministic human-readable key/package/inventory summaries plus exact symbol or package-init classification and ordered evidence queries. Unknown packages/symbols and mismatched expected keys must exit as tool errors.
7. Preserve the CLI's exit-code contract: usage, discovery, generation, cache, decode, validation, and lookup errors exit 2; successful generation/inspection exits 0. Keep stdout for requested results and stderr for actionable errors.
8. Update usage documentation and package Component Contracts, including the `stdlibmap` shell package's Ambient Authority line; do not wire the map into `arcc check` until Step 6.

## Dependencies
- Task 1 provides fail-closed artifact lookup and key verification.
- Task 2 provides target discovery and SDK-key derivation.
- Task 3 provides deterministic total-map generation.
- Bazel action/rule wiring is deferred to Task 5.

## Implementation Approach
1. Define a cache API returning an opened, key-verified authority map and isolate path selection from generation.
2. Implement validate-on-read, regenerate-on-corruption, and atomic replacement using temporary test directories and generator spies.
3. Extend CLI dispatch with a dedicated `stdlibmap` command parser rather than coupling it to `check` argument parsing.
4. Provide concise inspect output for summary, symbol, and init queries and pin ordering/exit behavior with subprocess-style CLI tests.
5. Exercise real local generation and cache reuse in integration tests, then run the full CI gate.

## Acceptance Criteria

1. **Native generation is cached by the full SDK key**
   - Given repeated requests for one target configuration
   - When the first request generates a valid map and the second runs
   - Then the second reuses byte-identical cached content without invoking the generator; changing cgo, GOOS/GOARCH, tags, GOEXPERIMENT, classifier hash, or format version selects a distinct entry.

2. **Corrupt caches regenerate safely**
   - Given truncated JSON, invalid inventory, or content stamped with another SDK key at the expected cache path
   - When the cache is opened
   - Then it is treated as corrupt, regenerated, validated, and atomically replaced; a generation failure leaves the prior bytes intact and returns an error.

3. **Concurrent readers never observe partial output**
   - Given overlapping cache requests and a delayed writer
   - When readers access the cache
   - Then every observed artifact is either the previous complete valid map or the new complete valid map, never partial data.

4. **Generate command emits a canonical artifact**
   - Given a valid native toolchain and target configuration
   - When `arcc stdlibmap generate` runs with an output path
   - Then it exits 0, writes a canonical total map atomically, and a second identical run produces identical bytes.

5. **Inspect command queries semantic cases**
   - Given a generated local map
   - When summary, `os.ReadFile`, `strings.TrimSpace`, `sort.Slice`, and package-init queries run
   - Then output deterministically shows the full SDK key, exact terminal classification, provenance, and ordered evidence where applicable; absent inventory exits 2 with an actionable message.

6. **Existing checking remains unchanged**
   - Given the new CLI commands and cache
   - When ordinary `arcc check` fixtures run
   - Then their analysis inputs and verdicts are unchanged and no native map is generated implicitly before the Step 6 cutover.

7. **Integration remains green**
   - Given cache and CLI behavior are complete
   - When `just ci` runs
   - Then all generation, lint, unit, Bazel, and self-check gates pass.

## Metadata
- **Complexity**: High
- **Labels**: go, cli, cache, atomic-write, stdlib-map, sdk-key, fail-closed
- **Required Skills**: Go CLI design, cache correctness, filesystem concurrency, atomic I/O, deterministic rendering, integration testing
