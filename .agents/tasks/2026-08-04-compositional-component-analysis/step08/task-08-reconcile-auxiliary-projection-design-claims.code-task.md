# Task: Reconcile Auxiliary Projection Design Claims

## Description
Reconcile the detailed design's component-only work and closure-independence claims with the approved `ArccImportGraph` topology, documenting the residual per-component lexical scan over ordinary non-member source and ensuring future performance acceptance measures it.

## Background
Addresses finding F2 from the Step 8 implementation review. The repaired build-topology bullet correctly permits a declared-input metadata projection to read non-member source, and the implementation keeps that source out of `ArccCheck`. Other load-bearing design passages still say that a check scans only its own source, that all closure work is eliminated, and that nothing re-reads or parses dependency source. Those statements are no longer literally true for the complete Bazel producer chain because each checked component registers an `ArccImportGraph` action over its ordinary non-member closure.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (§Overview, N1-N2, §Build topology, §Composition across components, acceptance matrix “Performance”)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step08.yaml`

**Additional References:**
- Package layout schema: `docs/package-layout-schema.md` (§3b The Bazel ordinary-import and stdlib export-data handoff)
- Step 8 evidence: `.agents/planning/2026-08-04-compositional-component-analysis/research/step08-input-pruning.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Preserve the adjudicated topology decision: one `ArccCheck` component-analysis action, plus hermetic declared-input producer actions such as `ArccImportGraph` and `ArccLayout`; do not redesign or remove the approved projection in this documentation remediation.
2. Update the detailed design's overview and composition text to distinguish the component analysis action from its producer chain. State explicitly that parsing/type-checking, typed reference scanning, SSA, VTA, and Capslock over non-members are eliminated, while a lexical ordinary-import projection currently reads target-selected non-member source because pinned rules_go does not expose the exact graph.
3. Clarify N1/N2 and the build-topology discussion without weakening the enforced asymmetry: only the auxiliary metadata action may receive ordinary non-member source, and `ArccCheck` itself may receive only the final Step 8 allowlist.
4. Correct claims that the remaining closure-shaped cost consists only of export files. Document that the auxiliary import projection also scales with the ordinary source closure and is cached as a declared build artifact.
5. Update the future performance/scaling acceptance language so measurements cover the whole producer chain, including `ArccImportGraph` action count, input bytes, and wall time, rather than measuring only the Go loader inside `ArccCheck`.
6. Keep `docs/package-layout-schema.md` and the Step 8 research note consistent with the detailed design; remove or qualify any remaining absolute claim that no dependency source is read anywhere in the Bazel producer chain.
7. Preserve the specification repair's audit trail and date/reference; the documentation must explain the accepted tradeoff rather than erasing it.

## Dependencies
- Step 8 Task 4's specification repair `xsswztkmrnrkzmumxosypwuqrqmnuquy` and the implemented `ArccImportGraph`/`ArccLayout` topology.
- No dependency on Task 07; the tasks may be implemented independently, but this documentation task should be sequenced after the higher-severity code remediation.

## Implementation Approach
1. Enumerate every absolute source-independence and closure-cost statement in the detailed design and linked Step 8 documentation.
2. Rewrite them around the precise action boundary established by the repaired topology bullet, keeping the correctness, hermeticity, and fail-closed claims intact.
3. Extend the performance acceptance description with producer-chain measurements that will make the projection's residual scaling visible in Step 13.
4. Run link/format checks and a targeted terminology search to prove no contradictory absolute statement remains.

## Acceptance Criteria

1. **Topology decision is described consistently**
   - Given the repaired build-topology bullet and the current auxiliary actions
   - When the detailed design is read end to end
   - Then every source-independence statement distinguishes `ArccCheck` from `ArccImportGraph` and no passage contradicts the permitted non-member lexical projection.

2. **Residual closure work is explicit**
   - Given an ordinary dependency closure
   - When the overview, composition, and type-loading sections describe its cost
   - Then they identify the cached lexical import projection as residual closure-shaped work alongside export-data inputs.

3. **Performance acceptance covers the producer chain**
   - Given the deferred Step 13 depth/width benchmark
   - When its design requirements are followed
   - Then it records import-projection action count, source input size, and elapsed contribution in addition to loader/export metrics.

4. **The action asymmetry remains unambiguous**
   - Given the updated N1/N2 and topology text
   - When a future implementer derives action inputs from it
   - Then ordinary non-member source is permitted only on the metadata producer and forbidden from `ArccCheck` itself.

5. **Documentation checks pass**
   - Given the completed remediation
   - When repository documentation checks and a search for the superseded absolute claims run
   - Then all checks pass and the remaining statements agree with the approved topology.

## Metadata
- **Complexity**: Medium
- **Labels**: architecture, documentation, bazel, performance, hermeticity
- **Required Skills**: Technical writing, Bazel action-graph reasoning, architecture review, performance-test design
