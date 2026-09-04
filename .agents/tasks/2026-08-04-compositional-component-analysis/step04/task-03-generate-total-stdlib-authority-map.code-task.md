# Task: Generate Total Stdlib Authority Map

## Description
Implement the Capslock-backed generation classifier and reconcile its per-root results with the independent SDK inventory to emit a complete canonical standard-library authority map with evidence and no silent gaps.

## Background
Capslock function-granularity output must be grouped by `Path[0]` and then completed against the typed inventory because absence can mean structural non-root, curated safe, or analyzed pure. Generation deliberately preserves `CAPABILITY_UNANALYZED`; reusing the live check adapter's `ClassifierExcludingUnanalyzed` wrapper would launder cases such as `sort.Slice` into apparent purity. Variables, aggregate init, compiler builtins, minting-site reclassification, and curated-safe provenance each require explicit rules pinned by the design and spike.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Stdlib generation spike and proposed rules: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md`
- Design review response, DR-05: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Existing Capslock adapter and minting-site policy: `go/internal/capslockadapter/capslockadapter.go`
- Canonical map I/O: `go/internal/artifactio/stdlibmap.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Run Capslock once at `GranularityFunction` over the complete batch of importable stdlib packages and group findings by the root name in `Path[0]`; do not run one analysis per package or symbol.
2. Build a dedicated generation classifier that includes Capslock builtins and the existing object-capability minting-site reclassification but never applies `ClassifierExcludingUnanalyzed`; keep its source/rule descriptor identical to the material hashed in Task 2.
3. Normalize Capslock roots through the Step 3 structured package/inventory API, discard unexported helpers, closures, instantiations, and source-level `init#N`, and fail on any ambiguous or unresolvable exported root rather than guessing.
4. Assign functions and declared methods the union of their grouped findings, make zero-finding inventoried roots explicitly `SAFE`, retain `UNANALYZED` as its terminal class, and keep capability lists sorted and duplicate-free.
5. Classify consts, plain types, and interface method specs as `SAFE`. Classify variables from the pointer-dereferenced static type's exported method classifications plus the handle minting-authority clause; interface-typed and methodless variables are `SAFE`.
6. Classify each package's compiler-synthesized aggregate `pkg.init` from the aggregate root, unioning all source init paths; emit one init record for every importable package including explicit `SAFE`.
7. Hardcode every exported `unsafe.*` compiler builtin as `UNANALYZED`, since these are `*types.Builtin` values with no SSA roots.
8. Record deterministic evidence for each `(symbol or init, capability)` and retain evidence frame order. Record `capslock-curated` provenance on entries whose `SAFE` result comes from Capslock's curated safe rules, distinguishing it from proved-pure and project override provenance.
9. Reconcile classifications against the independent inventory before serialization. Refuse to emit if any inventoried symbol/init is missing, any classification lacks required evidence, an analyzer root cannot be accounted for, or any record violates the Step 3 canonical validator.
10. Emit through canonical artifact I/O only, with sorted repeated records and the Task 2 SDK key; two generations over identical complete inputs must produce identical bytes.

## Dependencies
- Task 1 provides domain classifications and artifact-reader semantics.
- Task 2 provides target SDK discovery, typed inventory, normalization context, classifier fingerprint, and SDK key.
- This task produces the generator API consumed by native caching/CLI and the Bazel rule in later tasks.

## Implementation Approach
1. Extract/share the minting-site classifier rules without changing the live check-time wrapper, and pin separate constructors with tests proving generation retains `UNANALYZED`.
2. Convert one batched Capslock response into normalized per-root capability/evidence aggregates.
3. Apply object-kind completion rules over the typed inventory, handling variables, aggregate init, curated-safe provenance, and unsafe builtins explicitly.
4. Validate exact inventory equality and round-trip the result through canonical artifact I/O before returning bytes.
5. Add small typed SDK fixtures for every rule, then an integration test over the real local stdlib for totality and named semantic cases.

## Acceptance Criteria

1. **Pinned semantic examples classify correctly**
   - Given the target standard library inventory
   - When generation completes
   - Then `os.ReadFile` is `FILES` with non-empty evidence, `strings.TrimSpace` is `SAFE`, `sort.Slice` is `UNANALYZED`, `os.Stdin` includes `FILES`, `io.EOF` is `SAFE`, `http.DefaultClient` includes `NETWORK`, and `unsafe.Pointer` is `UNANALYZED`.

2. **Generation preserves unanalyzed findings**
   - Given a root curated by Capslock as `CAPABILITY_UNANALYZED`
   - When the generation classifier runs
   - Then the emitted record is `UNANALYZED`; a regression test proves the live check-time exclusion wrapper is absent from the generation path.

3. **Aggregate init is classified once**
   - Given a fixture package with multiple source-level init functions reaching different capabilities
   - When generation runs
   - Then one `pkg.init` record contains their terminal union and evidence, while no `init#N` record is emitted.

4. **Variable and minting rules are explicit**
   - Given pre-minted handle, network client, interface-typed, and methodless variables
   - When they are classified
   - Then method authority and moved minting authority are combined as designed, without treating every handle use as ambient authority.

5. **Inventory completeness is enforced**
   - Given the discovered importable package inventory
   - When generation finishes or a symbol/init classification is deliberately deleted
   - Then the complete map passes validation, while the damaged map fails with an inventory-gap error and can never be read as pure.

6. **Provenance and evidence remain auditable**
   - Given capability-bearing and curated-safe entries
   - When the map is inspected
   - Then every capability has a non-empty ordered evidence path and curated `SAFE` trust decisions are visibly distinguished from proved-pure and project overrides.

7. **Generation is deterministic**
   - Given identical SDK sources, package list, target configuration, classifier rules, and generator version
   - When generation runs twice
   - Then canonical map bytes and digests are identical regardless of analyzer/map iteration order.

8. **Full stdlib generation remains practical**
   - Given the local target SDK
   - When the integration generation test runs
   - Then every oracle package and every inventoried symbol/init has a terminal entry, with execution kept within the repository's integration-test conventions.

9. **Integration remains green**
   - Given the complete producer library
   - When `just ci` runs
   - Then all existing checks pass and the component check path still uses its pre-Step-6 analysis model.

## Metadata
- **Complexity**: High
- **Labels**: go, capslock, stdlib-map, totality, ambient-authority, evidence, determinism
- **Required Skills**: Capslock/SSA analysis, go/types, capability modeling, deterministic reconciliation, fail-closed validation, integration testing
