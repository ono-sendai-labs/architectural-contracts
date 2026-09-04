# Task: Inventory SDK and Derive Map Key

## Description
Build the deterministic SDK discovery and `go/types` inventory layer used by stdlib-map generation, and derive the complete target-specific SDK key and classifier fingerprint. The inventory is the independent oracle against which Capslock classifications will be reconciled.

## Background
The map is sound only if it is total independently of the analyzer. Native generation discovers packages with `go list std`; Bazel supplies the pinned toolchain's package list and SDK sources because its sandbox has no host `go` binary. Importable packages require every externally referencable exported object, every exported method of exported named or alias types, and the aggregate `init`; packages with an `internal` path segment remain in the package inventory with `importable: false` but have no symbol inventory.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Stdlib generation spike: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md`
- Design review response, DR-05 and DR-07: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Package-layout platform discovery: `go/internal/packagelayout/packagelayout.go`
- Capslock normalizer inventory contract: `go/internal/symbol/capslock.go`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Introduce a `stdlibmap` shell package with the exact FR10 Component Contract block and an explicit Ambient Authority declaration covering its filesystem, process/toolchain discovery, reflection/type-loading, and Capslock generation responsibilities.
2. Model package discovery behind an injectable oracle that supports both native `go list std` and an explicit Bazel-supplied toolchain package list; normalize, deduplicate, and sort package paths deterministically.
3. Mark a package non-importable when any import-path segment is `internal`; retain it in the total package list and skip its exported symbol and init inventory.
4. Load each importable package for the requested target configuration and inventory, via `go/types`, every exported package-scope object and every exported method declared on exported named or alias types. Use the Step 3 declaring-object rules and canonical `SymbolID` constructors, including generic origins and alias behavior.
5. Inventory exactly one aggregate `pkg.init` entry for every importable package. Keep interface method specs and fields governed by their declaring type rather than inventing independent persisted IDs.
6. Reject package load/type errors, duplicate canonical IDs, non-canonical paths, or unresolved Capslock normalization context with actionable errors; generation must not continue with a partial inventory.
7. Derive `SDKKey` from toolchain version, target GOOS/GOARCH, cgo state, sorted build tags, GOEXPERIMENT, map format version, and a deterministic `classifier_hash` over the complete generation classifier text plus an explicit rule-version string.
8. Keep host and target configuration distinct: the key and loaded inventory must describe the target even for cross-compilation, and toggling cgo or target platform must change the key.
9. Provide test seams for the package oracle, toolchain version, environment/configuration, and package loader without global logging or network access.

## Dependencies
- Task 1 provides the SDK key domain type consumed by the eventual reader and generator.
- Step 3 provides canonical symbols and the inventory-backed Capslock-name normalizer.
- Full Capslock classification and map emission are deferred to Task 3.

## Implementation Approach
1. Define target-configuration, package-oracle, and typed-inventory inputs independently of the analyzer.
2. Implement and test native `go list std` parsing, plus an explicit package-list path suitable for a Bazel action.
3. Walk typed package scopes and method sets into a sorted, duplicate-free canonical symbol inventory.
4. Centralize generation classifier/rule source material in a hashable descriptor so the future classifier implementation cannot drift from its stamped hash.
5. Add hermetic unit fixtures for aliases, methods, generics, non-importable packages, errors, and cross-target keys; gate real-toolchain coverage as an integration test when appropriate.

## Acceptance Criteria

1. **The package oracle is total and deterministic**
   - Given unsorted or repeated native/Bazel package-list output containing public and `internal` paths
   - When discovery runs
   - Then every unique path is emitted in canonical order, internal-segment packages are retained as non-importable, and malformed paths fail generation.

2. **Every externally referencable declaration is inventoried**
   - Given typed fixture packages containing exported funcs, vars, consts, named and alias types, exported methods, fields, interface specs, and generics
   - When inventory runs
   - Then canonical IDs cover exactly the design's persisted symbol set plus one aggregate init, with no duplicate, promoted, instantiated, field, or interface-method-only IDs.

3. **Inventory failure cannot become partial output**
   - Given a package load error, type error, ambiguous symbol normalization, or canonical-ID collision
   - When inventory runs
   - Then it returns an actionable error and no usable partial inventory.

4. **SDK keys describe the target configuration**
   - Given linux/amd64 with cgo on and off and a cross-compiled target with another GOOS
   - When keys are derived on the same host
   - Then all configurations have distinct deterministic keys, build tags are sorted, and repeated derivation is byte-for-byte stable.

5. **Classifier drift changes the key**
   - Given unchanged SDK fields but a changed classifier text, minting rule, variable rule version, or map format version
   - When `classifier_hash` and the key are derived
   - Then the hash/key changes; non-semantic iteration order does not.

6. **Real SDK enumeration is covered**
   - Given the local Go toolchain
   - When the integration inventory test runs
   - Then every `go list std` package is represented and all inventoried IDs parse canonically without relying on network access.

7. **Integration remains green**
   - Given SDK discovery and inventory are additive
   - When `just ci` runs
   - Then all existing checks pass and no map has yet been consumed by component analysis.

## Metadata
- **Complexity**: High
- **Labels**: go, stdlib-map, sdk-inventory, go-types, sdk-key, classifier-hash, cross-compile
- **Required Skills**: Go toolchain discovery, go/types, canonical identifiers, dependency injection, deterministic hashing, integration testing
