# Task: Add Persisted Artifact Shape Goldens

## Description
Add explicit, deterministic shape-golden coverage for the three persisted analysis artifacts: reports, component surfaces, and standard-library authority maps. Keep these schema/serialization assertions distinct from Task 1's host-independent verdict goldens and avoid adding a second whole-SDK map generation or a duplicate full-size map fixture.

## Background
The compositional design made reports, surfaces, and stdlib maps versioned canonical JSON artifacts, but current coverage is uneven. Reports are compared as complete artifacts even when a test means only “pass,” checked surfaces are compared only against reruns of themselves, asserted surfaces have targeted field tests, and the default map is compared with the pinned artifact without a concise golden that communicates its public shape.

Step 9 requires these artifacts to be golden-able while keeping host-dependent details out of ordinary verdict assertions. Shape goldens should pin the persisted contract and meaningful semantic distinctions, with narrowly defined normalization only for values that legitimately vary by target or input bytes. They must use the production bounded decoders/canonicalizers (or a schema-aware test adapter over them), not regex deletion that could let a malformed artifact pass.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N4, §Data Models, §Persisted formats, §Golden restructure)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Provide reusable schema-aware golden comparison support for report, surface, and stdlib-map artifacts. It must validate the input with the production bounded decoder before producing or comparing a deterministic snapshot.
2. Define normalization explicitly per artifact and keep it minimal. Stable field names, enum values, collection order, package/symbol identities, authority/provenance/freshness axes, and format versions must remain visible; only documented host/target- or content-derived scalar values may be replaced with named placeholders.
3. Add report shape goldens that cover the persisted envelope, verdict location, dependency boundary axes, diagnostics field shape, and null/empty finding collections. At least one non-empty finding fixture must prove finding kind/class/site ordering is represented without requiring host-absolute source paths.
4. Add checked-surface shape coverage for both `DECLARED_INTERFACE` and `PACKAGE_SURFACE` behavior as available in the fixture graph: component identity, concrete sorted packages, exact sorted symbols where applicable, namespace, SDK key structure, producer version, authority declaration, and non-empty digest semantics.
5. Add asserted-surface shape coverage that pins `PACKAGE_SURFACE`, `authority: UNKNOWN`, concrete sorted packages, empty symbols, the empty digest, namespace, SDK key structure, and producer version, without invoking an analysis action.
6. Add stdlib-map shape coverage for its format version, complete SDK key structure, sorted package/symbol/init/evidence collections, and representative `SAFE`, capability-bearing, and `UNANALYZED` records. Reuse a bounded synthetic/canonical fixture or the already pinned default artifact; do not check in a second full map and do not regenerate the whole SDK in a routine test.
7. Prove that reordered logical inputs normalize to byte-identical golden snapshots and that a stable schema or semantic field change causes an intelligible diff. Invalid, oversized, wrong-version, or non-canonical artifacts must fail rather than being normalized into an acceptable snapshot.
8. Keep artifact-shape tests separate and clearly named so downstream hosts can identify the small set of intentionally layout/target-sensitive fixtures instead of patching semantic verdict goldens.

## Dependencies
- Task 1: Separate Semantic Verdict Goldens, so complete report comparisons can be retained only where they intentionally cover artifact shape.

## Implementation Approach
1. Build a small typed snapshot/comparison seam around `artifactio` and the existing report, surface, and stdlib-map schemas, with per-field placeholder policy expressed in code and tested.
2. Exercise the seam against actual checked and asserted component artifacts and against a bounded representative map input or the checked-in pinned map.
3. Replace the temporary complete-report goldens with explicitly named report-shape goldens and add the new surface/map snapshots without duplicating expensive producers.

## Acceptance Criteria

1. **Report shape is pinned independently of verdict-only tests**
   - Given passing and finding-bearing canonical reports
   - When shape snapshots are compared
   - Then the persisted envelope, dependency axes, diagnostics, and ordered findings are covered while host-absolute paths and machine-dependent metric values are represented only by documented placeholders.

2. **Checked surface contracts are visible**
   - Given checked declared-interface and package-surface fixtures
   - When their artifacts are shape-compared
   - Then concrete packages, exact symbol behavior, declared authority, namespace, SDK-key structure, producer identity, and non-empty digest semantics match the goldens.

3. **Asserted surface semantics are visible**
   - Given an asserted component
   - When its surface is shape-compared
   - Then the golden records `PACKAGE_SURFACE`, unknown authority, no symbols, and an empty digest, and no `ArccCheck` action is introduced.

4. **Map shape is covered without routine regeneration**
   - Given the bounded map fixture or existing pinned default map
   - When the map shape test runs in routine CI
   - Then SDK-key, package, symbol, init, evidence, ordering, and terminal-classification shapes are checked without generating a new whole-SDK map or storing a duplicate full-size artifact.

5. **Normalization cannot hide malformed artifacts**
   - Given malformed, oversized, unsupported-version, unsorted, or otherwise non-canonical artifacts
   - When the shape helper processes them
   - Then it fails through the production validation boundary rather than emitting a passing normalized snapshot.

6. **Artifact snapshots are deterministic**
   - Given logically identical artifact messages assembled in different input orders
   - When snapshots are produced twice
   - Then their bytes are identical; changing a stable schema or semantic value yields a focused golden diff.

7. **Repository checks pass**
   - Given all shape goldens and targets
   - When `just ci` runs
   - Then routine validation remains green and does not add a native whole-SDK generation or more than the already permitted Bazel generation.

## Metadata
- **Complexity**: High
- **Labels**: go, bazel, testing, persisted-artifacts, goldens, determinism
- **Required Skills**: Go protobuf/JSON artifact handling, Starlark, Bazel test fixtures, deterministic normalization, golden-test design
