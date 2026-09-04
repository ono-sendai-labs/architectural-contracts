# Task: Preserve and Validate Stdlib Map Semantics

## Description
Correct canonical stdlib-map I/O so evidence remains an ordered call path and structurally incomplete maps are rejected. Canonicalization must sort keyed sets while preserving order in sequence-valued fields.

## Background
Addresses finding F2 from the Step 3 implementation review. The schema defines `Evidence.frames` as an ordered path from a symbol to a capability use, but `normalizeMap` sorts those frames by content and the tests treat reordered paths as equivalent. The same validator claims total-inventory consistency while accepting maps missing an init for an importable package or missing evidence for a capability-bearing symbol.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step03.yaml`

**Additional References:**
- Schema contract: `proto/archcontracts/v1/stdlibmap.proto`
- Canonical map I/O: `go/internal/artifactio/stdlibmap.go`
- Plan Step 3: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Preserve `Evidence.frames` exactly in producer order during normalization, marshal, decode, and digest computation; frame order is semantic and must not be sorted.
2. Continue sorting only collections whose schema declares a canonical key: packages, symbols, inits, evidence records, SDK build tags, and capability sets.
3. Validate that every importable package has exactly one terminal init record and every non-importable package has none.
4. Validate that each capability on a `CAPABILITIES` symbol has exactly one matching evidence entry, that no unmatched evidence exists, and that every required evidence path is non-empty and contains no nil frame.
5. Keep terminal-classification, package/ID agreement, duplicate-key, size-cap, unknown-field, and SDK format-version checks intact.
6. Update comments and tests that currently describe reordered frames as semantically equivalent.
7. Do not add stdlib generation, cache policy, CLI commands, or lookup behavior from Step 4.

## Dependencies
- Task 7's component-boundary repair must be complete so BUILD and component changes target the final ownership graph.

## Implementation Approach
1. Separate keyed-set normalization from ordered-sequence preservation in `normalizeMap`.
2. Build expected init and evidence key sets while validating the package and symbol inventories, then compare them with the persisted records.
3. Replace the frame-reordering equivalence test with order-preservation and order-sensitivity tests.
4. Add malformed-map cases for missing/extra init and evidence records and empty evidence paths.
5. Run focused artifact I/O tests, generation cleanliness, self-checks, and the full CI gate.

## Acceptance Criteria

1. **Evidence path order is preserved**
   - Given an evidence path with frames in caller-to-capability order
   - When it is marshaled, decoded, and marshaled again
   - Then the frame sequence is unchanged, while reversing the input frames produces different canonical bytes and a different digest.

2. **Init inventory is complete**
   - Given a map package inventory
   - When an importable package lacks its single terminal init record, a package has duplicate init records, or a non-importable package has an init record
   - Then marshal and decode fail with an actionable inventory error.

3. **Evidence inventory is complete**
   - Given a capability-bearing symbol
   - When any listed capability lacks exactly one non-empty evidence path, or evidence names an absent/non-capability symbol or capability
   - Then marshal and decode fail closed with an actionable evidence error.

4. **Keyed collections remain deterministic**
   - Given semantically equivalent maps with keyed collections and capability sets in different orders
   - When marshaled
   - Then their bytes remain identical without treating ordered frame paths as sets.

5. **Integration remains green**
   - Given the strengthened validator and corrected canonicalizer
   - When `just ci` runs
   - Then all tests and self-checks pass and no Step 4 producer/cache/lookup behavior has been introduced.

## Metadata
- **Complexity**: Medium
- **Labels**: go, protobuf-json, determinism, validation, stdlib-map, evidence
- **Required Skills**: Go, protobuf data modeling, canonical serialization, fail-closed validation, table-driven testing
