# Task: Repair the Generation Classifier Fingerprint

## Description
Make the standard-library map's `classifier_hash` fingerprint the classifier
semantics that generation actually executes, so a Capslock builtin or project
rule change necessarily selects a different SDK key and native cache entry.

## Background
Addresses finding F1 from the Step 4 implementation review. Step 4 correctly
generates, validates, caches, and serves a total authority map, but
`sdkKeyProto` currently hashes `GenerationClassifierRules`, whose Capslock
builtin entry is a prose summary and whose minting entry is only a method-name
list. `NewGenerationClassifier` actually executes Capslock's embedded builtin
classifier plus `capslockadapter.GenerationClassifierText`. If those semantics
change without the prose or manually maintained `RuleVersion` changing, an old
native cache artifact retains the same `SDKKey` and can be accepted under the
new generator, contrary to design I2 and N3.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` — I2, I4, §The stdlib authority map (`classifier_hash`), §Error Handling
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step04.yaml`

**Additional References:**
- Plan Step 4: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Executed classifier: `go/internal/capslockadapter/generation.go`
- Current fingerprint assembly: `go/internal/stdlibmap/generate.go`, `go/internal/stdlibmap/sdkkey.go`
- Native cache identity: `go/internal/stdlibmap/cache.go`

**Note:** Read the detailed design document and implementation review before beginning implementation.

## Technical Requirements
1. Define one canonical fingerprint input that covers every classifier semantic used by map generation: Capslock's exact builtin classifications, the exact project overlay text consumed by `NewGenerationClassifier`, the variable/handle minting rule, the `unsafe.*` rule, and the explicit project rule version.
2. For Capslock builtins, use the classifier content itself when it can be obtained deterministically; otherwise use a cryptographically stable content pin or immutable dependency identity that changes whenever that embedded classifier content can change. A generic prose statement that builtins are included is insufficient.
3. Make `sdkKeyProto` derive `classifier_hash` from that canonical semantic input. There must be no independently maintained description that can drift from the classifier execution path.
4. Preserve deterministic hashing: ordering-only changes to sets do not change the hash, while any semantic change to the builtin pin, project overlay, variable/minting rule, unsafe rule, or explicit rule version does.
5. Add tests tying the production fingerprint assembly to the production classifier inputs. The tests must fail on the current implementation, where changing the actual overlay or builtin identity without editing the prose descriptor leaves the hash unchanged.
6. Add an end-to-end key/cache regression: maps produced with two different classifier semantic inputs have distinct `SDKKey.ClassifierHash` values and `CacheKeyDigest` paths, and a map from one input fails the reader/cache key check for the other rather than being reused.
7. Preserve the dedicated generation classifier's I4 behavior: Capslock builtins remain enabled, minting-site reclassification remains applied, and `ClassifierExcludingUnanalyzed` remains absent.

## Dependencies
- Step 4 Tasks 01–06 provide the SDK key, generator, cache, Capslock adapter, native CLI, and Bazel artifact paths being corrected.
- No Step 5 analysis-action or Step 6 check-path consumption belongs in this remediation.

## Implementation Approach
1. Identify the smallest stable representation of Capslock's embedded builtin classifier that can be owned or pinned at the capslockadapter boundary.
2. Expose a single canonical generation-classifier fingerprint input from that boundary, including the actual overlay source.
3. Fold the project variable/unsafe rules and explicit rule version into the same canonical hash input and route SDK-key derivation through it.
4. Add focused drift, determinism, reader-mismatch, and cache-selection tests, then run the full CI gate.

## Acceptance Criteria

1. **The fingerprint covers the executed classifier**
   - Given the production generation classifier
   - When its canonical fingerprint input is inspected
   - Then it includes or cryptographically pins Capslock's exact builtin classifier and includes the exact project overlay and every project-side completion rule used by generation, with no prose-only surrogate.

2. **Every semantic change invalidates the key**
   - Given otherwise identical target and map-format inputs
   - When the builtin pin, project overlay, variable/minting rule, unsafe rule, or explicit rule version changes independently
   - Then each change produces a different `classifier_hash` and `SDKKey`, while reordering an unordered rule set does not.

3. **Stale maps fail closed instead of hitting the native cache**
   - Given a valid map and cache entry generated with classifier input A
   - When the same target is requested with classifier input B
   - Then B selects a different cache path and the A artifact fails B's key validation rather than being returned.

4. **Generation semantics remain intact**
   - Given the remediated fingerprint plumbing
   - When the representative generation tests run
   - Then `sort.Slice` and `unsafe.Pointer` remain `UNANALYZED`, the minting-variable cases retain their capabilities, curated-safe provenance remains visible, and generation remains byte-deterministic.

5. **Integration remains green**
   - Given the classifier fingerprint repair
   - When `just ci` runs
   - Then generation, lint, Go unit/integration tests, Bazel map tests, self-check, and artifact determinism checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: go, stdlib-map, classifier-hash, sdk-key, cache, fail-closed, remediation
- **Required Skills**: Go hashing and API design, Capslock classifier integration, deterministic serialization, cache-key testing, TDD
