# Task: Add Stdlib Authority Port and Reader

## Description
Define the core-side standard-library authority lookup contract and implement a fail-closed reader over the canonical Step 3 map artifact. This gives later reference scanning one stable API for package membership, symbol and package-init classifications, evidence, and SDK identity without coupling the core to protobuf or Capslock.

## Background
Step 3 established the persisted stdlib-map schema, canonical artifact I/O, symbol grammar, and validation rules. Step 4 must now expose those records as a total lookup: membership in the package inventory is the definition of standard library, and a missing symbol or init in an enumerated package is a tool error rather than an empty authority result. The port belongs on the core side while the artifact-backed adapter belongs in a shell package, preserving the repository's dependency direction and Component Contract convention.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Plan Step 4: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Canonical map I/O: `go/internal/artifactio/stdlibmap.go`
- Persisted schema: `proto/archcontracts/v1/stdlibmap.proto`
- Step 3 map-semantics task: `.agents/tasks/2026-08-04-compositional-component-analysis/step03/task-08-preserve-and-validate-stdlib-map-semantics.code-task.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Define the core-side `StdlibAuthority` port with `IsStdlibPackage`, fail-closed `SymbolAuthority`, fail-closed `PackageInitAuthority`, `Evidence`, and `Key` operations matching the design.
2. Define a terminal classification model in which exactly one state is valid: `SAFE`, a sorted non-empty capability set, or `UNANALYZED`; do not represent missing inventory as `SAFE` or as an empty capability set.
3. Define an inspectable `SDKKey` value containing toolchain version, GOOS, GOARCH, cgo state, sorted build tags, GOEXPERIMENT, classifier hash, and map format version, with deterministic equality/string behavior as needed by consumers.
4. Implement an artifact-backed adapter that decodes through `artifactio`, indexes packages, symbols, inits, and evidence deterministically, and satisfies the core port via a compile-time interface assertion.
5. Return typed or sentinel inventory-gap and key-mismatch errors with the absent package/symbol or mismatched key fields available for diagnostics; reject invalid or duplicate terminal states even if handed an in-memory message outside the normal decoder.
6. Treat all enumerated packages, including `internal/...` entries marked non-importable, as standard-library membership; only importable packages may have symbol and init lookup records, per the schema validator.
7. Return defensive copies of capability and evidence slices so callers cannot mutate shared lookup state.
8. Give every new non-test package the full FR10 Component Contract block, including its Ambient Authority line; keep the core package standard-library-only and authority-free.

## Dependencies
- Step 3 persisted stdlib-map schema, canonical artifact I/O, map validation, and canonical `SymbolID` implementation must be complete.
- No generator, cache, CLI, Bazel rule, or check-path consumption is part of this task.

## Implementation Approach
1. Place the minimal lookup types and interface with the core decision-side code, using existing capability and symbol vocabulary without importing generated protobuf types.
2. Add explicit conversion from validated persisted enum/message records to terminal core classifications and SDK keys.
3. Build immutable indexes once when opening the artifact, validating cross-record assumptions at that boundary.
4. Add table-driven unit tests for every classification, lookup, evidence, package-membership, defensive-copy, inventory-gap, and key-mismatch case.
5. Run focused package tests and the full CI gate.

## Acceptance Criteria

1. **Package membership comes only from the map**
   - Given importable and non-importable package inventory entries
   - When `IsStdlibPackage` is queried
   - Then it returns true for both enumerated packages and false for an absent path without applying a path heuristic.

2. **Symbol and init lookup are total and fail closed**
   - Given an enumerated importable package with validated symbol and init records
   - When known keys are queried
   - Then their exact terminal classifications are returned; querying an absent symbol or init returns an actionable inventory-gap error and never `SAFE`.

3. **Terminal states cannot be ambiguous**
   - Given `SAFE`, capability-bearing, and `UNANALYZED` records plus malformed in-memory combinations
   - When the adapter is constructed
   - Then each valid record maps to exactly one core state and every empty, overlapping, unknown, or otherwise invalid state is rejected.

4. **Evidence and SDK identity are stable**
   - Given capability evidence and a complete persisted SDK key
   - When they are read through the port
   - Then frame order and every target-configuration field are preserved, returned collections cannot mutate adapter state, and a required-key mismatch fails closed.

5. **Architectural boundaries remain clean**
   - Given the new port and adapter packages
   - When dependency and self-component checks run
   - Then the core port has no protobuf/Capslock dependency, the shell adapter owns decoding, and each new package carries the FR10 Component Contract including Ambient Authority.

6. **Integration remains green**
   - Given the additive lookup implementation
   - When `just ci` runs
   - Then all generation, lint, unit, Bazel, and self-check gates pass and the existing check path is unchanged.

## Metadata
- **Complexity**: Medium
- **Labels**: go, core-port, stdlib-map, artifact-reader, fail-closed, determinism
- **Required Skills**: Go API design, immutable indexing, protobuf-to-domain conversion, error modeling, table-driven testing
