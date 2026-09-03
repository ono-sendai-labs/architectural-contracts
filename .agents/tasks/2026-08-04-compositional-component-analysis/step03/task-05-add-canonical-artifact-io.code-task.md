# Task: Add Canonical Artifact IO

## Description
Implement shared, type-aware encoding and decoding for surface and standard-library map protobufs, plus canonical digests, bounded reads, and atomic file replacement. The APIs must produce stable JSON independently of input ordering, reject unsupported major versions and non-canonical semantic states, ignore forward-compatible unknown fields, and protect existing files from interrupted writes.

## Background
The new artifacts are cache keys and declared action outputs, so semantically identical values must serialize to identical bytes. Raw `protojson` is insufficient because repeated entries retain caller order and output details can vary across protobuf releases. The design therefore normalizes type-specific collections, marshals with `protojson`, compacts and indents through `encoding/json`, hashes the canonical bytes, caps reads, and writes via a temporary sibling followed by rename.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Plan Step 3: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-03 and DR-15: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a shell-side artifact I/O package rather than importing protobuf reflection or filesystem authority into the pure checker packages. Give any new package the required FR10 Component Contract, BUILD metadata, and component ownership/dependencies.
2. Provide separate surface and map marshal functions that defensively copy and normalize their messages before encoding. Sort packages, symbols, inits, evidence records, evidence frames, build tags, capabilities, authority sets, and every other repeated field by the schema's declared canonical key; reject duplicate keys or contradictory terminal classifications rather than silently choosing one.
3. Encode by deterministic `protojson` marshaling followed by `encoding/json` compact/indent canonicalization. Ensure `format_version` appears first in final JSON and document the exact trailing-newline/indent contract so repeated marshals are byte-identical across supported protobuf versions.
4. Provide bounded decoders that accept unknown JSON fields for forward-compatible minor additions, reject malformed JSON/protobuf, reject an unsupported major `format_version`, validate required semantic invariants, and enforce maximum input sizes of 16 MiB for surfaces and 64 MiB for maps before unbounded allocation.
5. Provide a SHA-256 helper over canonical bytes with one documented lowercase digest spelling. Hashing a decoded artifact must operate on its re-canonicalized representation, not the source's whitespace or entry ordering.
6. Implement atomic file replacement using a uniquely named temporary file in the target directory, appropriate file close/sync/error handling, and rename. Clean up failed temporary files without removing or truncating the previous target.
7. Expose narrow test seams for write/sync/rename failure or an equivalent deterministic crash simulation. Do not add cache regeneration policy, CLI commands, Bazel actions, or dependency freshness computation from Steps 4-7.
8. Add tests proving deterministic repeated output, reordered repeated-entry equivalence, input immutability, stable digest equivalence, unknown-field tolerance, version rejection, semantic-validation errors, exact size-boundary behavior, and preservation of an old file after every simulated pre-rename failure.

## Dependencies
- Task 3 supplies the generated artifact messages and their canonical sort-key contracts.
- Task 4 supplies validated `SymbolID` parsing needed when artifact string fields are checked.
- Task 2 supplies structural authority validation; no producer or consumer from later plan steps is required.

## Implementation Approach
1. Define narrow type-specific normalization/validation functions that clone protobuf messages and sort or validate each repeated collection.
2. Build marshal/decode APIs around `protojson` plus standard JSON normalization, with explicit per-artifact version and size policies.
3. Add canonical SHA-256 helpers over the marshaled bytes.
4. Implement atomic sibling-temp writes with injected failure seams and exhaustive cleanup/preservation tests.
5. Run focused artifact tests, verify pure-core dependency boundaries through self-check, then run `just ci`.

## Acceptance Criteria

1. **Canonical bytes ignore caller ordering**
   - Given two semantically identical surface or map messages whose repeated entries and nested capabilities/frames are ordered differently
   - When each is marshaled
   - Then the resulting bytes are identical, `format_version` is first, formatting is stable, and neither input message was mutated.

2. **Digest follows canonical semantics**
   - Given equivalent artifacts encoded with different source whitespace or entry order
   - When decoded and hashed
   - Then they produce the same lowercase SHA-256 digest; a semantic field change produces a different digest.

3. **Readers are forward-compatible but fail closed**
   - Given an artifact with an unknown JSON field, an unsupported major version, malformed content, a duplicate key, an invalid symbol ID, contradictory authority, or a non-terminal map classification
   - When decoded
   - Then the unknown field alone is ignored, while every unsupported or semantically unsafe condition returns a specific error.

4. **Size caps are enforced before runaway reads**
   - Given surface inputs around 16 MiB and map inputs around 64 MiB
   - When decoded from a reader
   - Then inputs at or below the documented limit are processed normally and inputs above it fail with an actionable size error without reading or allocating the unbounded remainder.

5. **Atomic writes preserve the previous file**
   - Given an existing valid target and a simulated failure while creating, writing, syncing, closing, or renaming its replacement
   - When atomic write returns
   - Then the previous target bytes remain intact and temporary residue is cleaned up; only a successful rename makes the new bytes visible.

6. **Layering and integration remain green**
   - Given the new protobuf/filesystem package
   - When component self-checks, Bazel tests, and `just ci` run
   - Then pure core packages have gained no protobuf or filesystem dependency and all checks pass without cache/CLI/action behavior from later steps.

## Metadata
- **Complexity**: High
- **Labels**: go, protobuf-json, canonicalization, hashing, atomic-write, security
- **Required Skills**: Go, protobuf reflection, deterministic encoding, bounded I/O, filesystem durability, fault-injection testing
