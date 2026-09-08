# Task: Derive Exact Component Surfaces

## Description
Implement deterministic surface derivation from a parsed manifest, package layout/member facts, exact Step 3 `SymbolID` extraction, target SDK identity, namespace, and source bytes. The result is a canonical `SurfaceManifest` ready for either a checked producer or the later asserted producer.

## Background
The persisted surface schema and exact `symbol.ExtractSurface` algorithm already exist, but the current load path retains only legacy `ExportedSymbol` values used by SSA/VTA checking. Step 5 must emit the exact declared interface without the `ResolveDependencyInterface` implements-closure workaround. That workaround intentionally continues to affect the check until Step 6, so a direct concrete implementing method may be accepted by the check while being absent from the emitted surface for this one step. This temporary disagreement is required and must be documented and tested, not repaired by widening the surface.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` — R7, N4, §New: surface, §Reference semantics, §The surface manifest, §Provenance/freshness/authority

**Additional References:**
- Plan Step 5: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Current pipeline and implements-closure rationale: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Existing extractor and artifact codec: `go/internal/symbol/extract.go`, `go/internal/artifactio/surface.go`
- Current loader and dependency resolver: `go/internal/goanalysis/goanalysis.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Introduce a small surface-emission API that accepts all required inputs explicitly and returns a schema `SurfaceManifest`; keep data-model work pure and isolate filesystem reads in a shell adapter.
2. For declared-interface components, emit exactly the `SymbolID`s declared by surviving interface files using `symbol.ExtractSurface`: exported functions, methods, types, vars, consts, alias expansion, and aggregate init under the declaring-object rule. Do not call or copy `ResolveDependencyInterface`'s implements-closure logic.
3. For `PACKAGE_SURFACE`, emit the component's concrete canonical member packages and no symbols. For both styles, packages are canonical, sorted, duplicate-free, and reflect component ownership rather than the whole dependency closure.
4. Populate format version, component, interface style, `manifest.AuthorityDeclaration`, `hostpolicy.NamespaceID`, complete target `SDKKey`, producer version, and digest. Reject a missing/incomplete target key rather than emitting an artifact a Step 7 consumer cannot validate.
5. Compute `Digest` over member source bytes sorted by canonical path, manifest bytes, format version, namespace, SDK key, and producer version. A dependency source, test file, unrelated file, file enumeration order, or repeated run must not affect it.
6. Use the existing canonical surface encoder and atomic writer. Add the native location convention helper that replaces a dependency manifest's extension with `.surface.json` (and reserves the alongside `.report.json` path) without reading dependency artifacts yet.
7. Preserve the current SSA/VTA, Capslock check path, dependency source resolver, and implements-closure workaround. Add a prominent design/plan comment explaining their deliberate one-step disagreement with emitted surfaces.
8. Update BUILD/component metadata and FR10 Component Contract blocks for changed package responsibilities.

## Dependencies
- Step 3 supplies `surface.proto`, canonical artifact I/O, authority conversion, namespace policy, `SymbolID`, and `symbol.ExtractSurface`.
- Step 4 supplies the complete `SDKKey` definition and classifier fingerprint used to identify the target map configuration.
- Task 3 wires this derivation into `arcc check`; Tasks 4 and 5 provide checked and asserted Bazel producers.

## Implementation Approach
1. Define the emission input boundary and digest-input representation before coupling it to `Runner` or Bazel.
2. RED: declared-interface and `PACKAGE_SURFACE` fixtures, including aliases, canonical namespace, sorted duplicates, and an implementation method admitted only by the legacy check workaround.
3. GREEN: retain exact surface symbols during member loading and assemble/encode the schema without widening it.
4. RED/GREEN: digest sensitivity/exclusion matrix and native path convention.
5. Run focused tests, self-check, manifest parity, and the full CI gate.

## Acceptance Criteria

1. **Declared surfaces are exact**
   - Given a declared-interface component with interface functions, types, fields/interface specs, aliases, generic methods, and a concrete method admitted only by the legacy implements workaround
   - When its surface is derived
   - Then it contains exactly the Step 3 extractor's declared `SymbolID`s, excludes the workaround-only concrete method, and is sorted and duplicate-free.

2. **Package surfaces contain packages, not symbols**
   - Given a `PACKAGE_SURFACE` component with multiple concrete members and a dependency closure
   - When its surface is derived
   - Then `packages` contains exactly its owned canonical member packages, `symbols` is empty, and dependency packages are absent.

3. **Every emitted surface is fully identified**
   - Given either interface style and a complete target configuration
   - When the surface is encoded
   - Then it carries the current format version, component, structural authority, namespace, complete SDK key, producer version, and a valid digest; incomplete target identity fails closed.

4. **Digest tracks only semantic producer inputs**
   - Given an unchanged component
   - When the manifest or a member source changes
   - Then the digest changes; changing a dependency source, a test file, an unrelated file, or only enumeration order leaves it unchanged.

5. **Emission is deterministic**
   - Given identical complete inputs in repeated runs and different map iteration orders
   - When surfaces are derived and marshaled
   - Then the bytes are identical.

6. **The Step 5 semantic gap is explicit and contained**
   - Given the implements-closure fixture
   - When both the current check and surface emission run
   - Then the check retains its pre-Step-6 behavior, the surface remains exact, and code/docs identify this as the intentional Step 5-only disagreement.

7. **Native paths are conventional**
   - Given a dependency manifest path
   - When its companion artifact paths are derived
   - Then the extension is deterministically replaced by `.surface.json` and `.report.json`, with no manifest field or caller-selected surface path introduced.

## Metadata
- **Complexity**: High
- **Labels**: go, surface, symbol-id, digest, determinism
- **Required Skills**: Go AST/types, artifact modeling, hashing, filesystem boundary design, regression testing
