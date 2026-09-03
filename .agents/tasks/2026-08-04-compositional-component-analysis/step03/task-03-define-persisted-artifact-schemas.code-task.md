# Task: Define Persisted Artifact Schemas

## Description
Define the versioned protobuf schemas for component surfaces and standard-library authority maps, including SDK identity, terminal classifications, evidence, and all audit metadata required by the design. Extend repository generation and cleanliness checks so these process-boundary contracts are checked in and reproducible before any producer or consumer is implemented.

## Background
Surfaces and standard-library maps cross process and Bazel action boundaries. Their schemas must land before generators, analysis actions, caches, or comparisons can depend on an accidental in-memory shape. Determinism is designed into the schema: `format_version` is first, keyed collections are repeated entries rather than proto maps, and later canonical encoders sort them explicitly.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Plan Step 3: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Standard-library generation spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md`
- Design review response DR-05, DR-06, DR-09, and DR-15: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `proto/archcontracts/v1/surface.proto` with a top-level surface message whose field 1 is `format_version` and whose fields represent component name, interface style, structural authority, concrete package paths, symbol IDs, namespace, target SDK key, producer version, and digest as specified by the design.
2. Add `proto/archcontracts/v1/stdlibmap.proto` with a top-level map message whose field 1 is `format_version`; define the complete `SDKKey` (`toolchain_version`, target `GOOS`/`GOARCH`, cgo state, sorted build tags, GOEXPERIMENT, classifier hash, and map format version).
3. Model the map's total package inventory, symbol classifications, package-init classifications, and `(symbol, capability)` evidence paths. Terminal classification must distinguish exactly `SAFE`, non-empty capabilities, and `UNANALYZED`, and evidence frames must retain function, file, and line data needed later.
4. Reuse/import shared component enums and messages deliberately rather than duplicating authority or interface-style semantics. Keep generated-package ownership and Go import directions compatible with the pure-core/shell boundary.
5. Use no protobuf `map` fields. Represent every keyed collection with repeated entry messages, document its canonical sort key, and avoid fields whose truth is established by the build graph (especially checked/asserted provenance flags) inside a surface.
6. Assign stable, explicit field and enum numbers; include comments explaining target-versus-host semantics for SDK identity, totality for packages/symbols/inits, namespace requirements, and the fact that `format_version` is a major format version.
7. Extend `just gen` to generate all three schemas and extend `gen-is-clean` to cover every checked-in protobuf output without treating unrelated generated BUILD files as protoc outputs.
8. Add descriptor-level schema tests that verify required field numbers/types, absence of map fields, correct enum values, import relationships, and `format_version` at field 1. Add basic binary/JSON protobuf round trips without implementing canonical ordering or filesystem I/O from Task 5.
9. Add or update Gazelle/Bazel metadata and component ownership for the generated packages so native and Bazel builds remain green.

## Dependencies
- Task 2 must be complete because surfaces reuse the component interface style and authority declaration semantics.
- No producer, reader, cache, CLI, or Bazel analysis action may be added here; those belong to later steps and Task 5.

## Implementation Approach
1. Translate the design's surface, SDK key, inventory, classification, init, evidence, and frame models into explicit proto3 messages and enums with stable numbering.
2. Choose generated Go packages that keep shared schema dependencies acyclic, then update generator and Bazel metadata.
3. Add reflection-based descriptor tests for structural invariants and small protobuf round-trip tests for representative surface and map messages.
4. Run `just gen`, inspect generated diffs for stable package/import layout, then run `just gen-is-clean` and `just ci`.

## Acceptance Criteria

1. **Surface contract is complete**
   - Given `surface.proto`
   - When its top-level message is inspected
   - Then field 1 is `format_version`, all design-specified surface data is representable, packages and symbols are repeated sortable entries, authority distinguishes unknown from known-empty, and no provenance claim is stored in the file.

2. **SDK identity is target-complete**
   - Given the shared SDK key schema
   - When its fields are inspected
   - Then toolchain version, target OS/architecture, cgo setting, build tags, GOEXPERIMENT, classifier hash, and map format version are all represented without conflating host and target configuration.

3. **Map totality and classifications are representable**
   - Given `stdlibmap.proto`
   - When representative safe, capability-bearing, unanalyzed, non-importable, init, and evidence records are constructed
   - Then each state is explicit and unambiguous, capability classification cannot rely on an absent symbol meaning safe, and package inventory records importability.

4. **Schema supports canonical ordering**
   - Given every collection in both artifact schemas
   - When descriptors and comments are inspected
   - Then no proto map field exists and every repeated keyed entry has a defined deterministic sort key for Task 5.

5. **Generation is comprehensive and repeatable**
   - Given all component, surface, and map schemas
   - When `just gen` is run twice and `just gen-is-clean` is evaluated
   - Then all checked-in `.pb.go` files are reproduced identically and cleanliness covers every protoc output.

6. **Native and Bazel builds remain green**
   - Given the new generated packages and metadata
   - When `just ci` runs
   - Then all checks pass without an artifact producer, consumer, or analysis-path change.

## Metadata
- **Complexity**: High
- **Labels**: protobuf, schema, surface, stdlib-map, sdk-key, generation, bazel
- **Required Skills**: Protocol Buffers, Go code generation, schema evolution, Bazel/Gazelle, deterministic data modeling
