# Task: Prove Full-Build Artifact Determinism

## Description
Add full-pipeline determinism coverage for standard-library maps, checked and asserted surfaces, and reports, then consolidate the final acceptance evidence and next-host-import checklist that close the compositional-analysis plan.

## Background
Unit and focused integration tests already pin canonical encoding and repeated emission for individual artifact types. Step 13 requires a stronger boundary: repeated complete builds from identical declared inputs must publish byte-identical artifacts even when actions execute in separate output trees and in different scheduling orders. The check must include dependency-produced artifacts, not only direct serializer calls, while respecting N5's rule that expensive independent whole-SDK map generation lives in the explicit full lane.

The plan closes only after the final measurements are compared with the Step 1 baseline and the host-import friction checklist is walked. Those records must distinguish routine cached/pinned-map determinism from the explicit full lane's two real map generations.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N4–N5, §Persisted formats, §Routine stdlib-map validation topology, §Determinism and self-check)

**Additional References:**
- Plan Steps 3–6 and 13: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Baseline/final measurement record: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Next-import checklist source: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md`
- Stdlib-map cost/lane rationale: `.agents/planning/2026-08-04-compositional-component-analysis/research/2026-09-08-stdlib-map-generation-cost.md`
- Existing artifact and full-lane coverage: `go/internal/artifactio/report_test.go`, `go/internal/surface/derive_test.go`, `go/internal/stdlibmap/generate_integration_test.go`, and `justfile`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a deterministic build harness that materializes the canonical stdlib map, checked dependency surfaces/reports, a checked dependent surface/report, and an asserted `authority: UNKNOWN` surface through their real Bazel producers.
2. Execute the artifact pipeline at least twice from identical source/configuration inputs in separate isolated output trees and with deliberately different target/request ordering. Compare exact bytes and SHA-256 digests for every corresponding `.stdlib-map.json`, `.surface.json`, and `.report.json` output.
3. Ensure reports exercise sorted multi-site findings and dependency-boundary axes, checked surfaces exercise sorted packages/symbols and non-empty digest, and the asserted surface exercises package-level UNKNOWN authority with empty digest. The comparison must be sensitive to ordering drift rather than covering only trivial empty artifacts.
4. Keep elapsed/profile data from Tasks 1–3 out of all compared artifacts. Normalize nothing during byte comparison: canonical outputs for identical declared inputs must already match exactly.
5. In routine `just ci`, reuse the one permitted default-configuration map generation (or its checked canonical artifact) while independently rebuilding the remaining artifact graph in isolated trees. Assert zero native and at most one Bazel whole-SDK generation for the routine run.
6. Preserve and strengthen the explicit full lane so it performs two genuinely independent stdlib-map generations and compares their exact bytes. Keep replica/cross-configuration generation outside the five-minute routine lane, and make `just test-integration-full`/`just bazel-test-full` retain their documented purpose.
7. Run the routine determinism harness as an `integration`-tagged test reached by `just test-integration`/`just ci`; ensure temporary output roots and copied artifacts are removed on success and failure without touching developer-owned outputs.
8. Run `just ci` on a warm Bazel cache, record its wall time and stdlib-map action count, and verify it remains within the N5 five-minute budget. Record cold-lane generation behavior separately without encoding a flaky wall-clock timeout.
9. Consolidate Tasks 1–3 results in `research/current-analysis-pipeline.md`: Step 1 versus final timings on the same component, native and Bazel scaling tables, hermeticity invocation/result, determinism digests, routine CI timing/action counts, environment, revision, and limitations.
10. Walk `research/host-import-friction.md`'s next-import requirements against the final tree. Record concrete evidence that runtime injection and SDK adapter hooks have landed and that removed/tagging/golden/namespace concerns have their planned resolution; identify any genuinely deferred item without silently marking it implemented.
11. Keep `just ci`, self-check, `manifestparity`, canonical artifact readers, and existing corruption/concurrent-cache recovery tests green.

## Dependencies
- Task 1: Add Member-Analysis Scaling Benchmarks.
- Task 2: Measure Bazel Producer-Chain Scaling.
- Task 3: Enforce Restricted-Sandbox Hermeticity.
- Steps 3–12 provide all artifact producers, canonical encoders, adapter hooks, and self-check coverage under acceptance.

## Implementation Approach
1. Select a nontrivial checked dependency chain plus one asserted wrapper and expose all produced artifacts through a deterministic test target/output group.
2. Build the same graph in isolated output roots with reversed request order, collect outputs by logical identity, and compare bytes/digests with focused mismatch diagnostics.
3. Split map coverage by lane: one bounded canonical/default generation in routine CI and two independent real generations plus cross-configuration assertions in the explicit full lane.
4. Run the final supported CI workflow, capture the acceptance tables/checklists, and audit documentation for claims that predate the auxiliary import-projection correction.

## Acceptance Criteria

1. **Repeated full builds are byte-identical**
   - Given identical complete source, manifest, namespace, target SDK key, map classifier, dependency artifacts, and producer version inputs
   - When the artifact graph is built in separate output trees and different request orders
   - Then every corresponding map, checked/asserted surface, and report has identical bytes and SHA-256 digest.

2. **The comparison covers meaningful canonical ordering**
   - Given findings with multiple sites, dependency axes, multi-package/symbol surfaces, and an UNKNOWN asserted wrapper
   - When artifacts are compared
   - Then any ordering, duplicate, digest, authority, or provenance drift fails with the logical artifact name and first differing digest/path.

3. **Timing observations never alter artifacts**
   - Given two runs with different measured phase/action durations
   - When their persisted outputs are compared without normalization
   - Then map, surface, and report bytes remain identical and contain no benchmark timing field.

4. **Routine and full map lanes retain distinct guarantees**
   - Given routine `just ci` and the explicit full integration/Bazel lanes
   - When generation actions are counted
   - Then routine validation performs zero native and at most one default Bazel whole-SDK generation, while the full lane performs and byte-compares two independent real generations plus retained cross-configuration coverage.

5. **Final performance and CI evidence is recorded**
   - Given the pinned Linux/amd64, cgo-disabled environment
   - When the final benchmark and warm-cache CI runs complete
   - Then the research note contains baseline-versus-final results on the same component, both scaling tables, CI wall time below five minutes, generation counts, and clearly labeled machine-dependent observations.

6. **Hermeticity and determinism evidence is reproducible**
   - Given a reader following the research note from a clean checkout
   - When they execute the documented routine and full commands
   - Then they can reproduce restricted-sandbox checks and exact artifact comparisons without developer caches or untracked workspace outputs.

7. **The next-host-import checklist is closed honestly**
   - Given every item in `research/host-import-friction.md`
   - When it is checked against the final implementation
   - Then required runtime/SDK hooks and related redesign resolutions cite concrete files/tests, while explicitly deferred items remain labeled deferred.

8. **Repository-wide validation stays green**
   - Given all Step 13 additions
   - When `just ci`, self-check, manifest parity, cache-recovery tests, and the explicit full lane run
   - Then they pass without weakening existing assertions or adding a bookmark-only dependency.

## Metadata
- **Complexity**: High
- **Labels**: bazel, integration, determinism, artifacts, ci, acceptance
- **Required Skills**: Bazel output isolation, canonical artifact testing, Go integration harnesses, deterministic serialization, CI performance analysis, technical documentation
