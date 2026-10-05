# Pure & Mostly-Pure Components Architectural Analysis Report

## Executive Summary & Cross-Component Synthesis

This report analyzes the 10 pure and mostly-pure components of the `arcc` codebase:
`symbol`, `facts`, `checker`, `manifest`, `surface`, `stdlibauthority`, `report`, `capanalyzer`, `hostpolicy`, and `schema`.

### Synthesis Table: Interface Size vs Code Volume

| Component | Non-test Files | Non-test LOC | Total Exported Symbols | Used Ext (Non-Test) | Used Tests Only | Internal Only | % Symbols Used Ext | Reachable Only Internally (LOC) | % LOC Internal / Tooling Only | Authority Verdict |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **`symbol`** | 4 | 1,704 | 15 | 11 | 1 | 3 | 73.3% | 1,049 LOC (`capslock.go` tooling-only) + ~150 LOC unused | 70.4% | `pass` (0 auth) |
| **`facts`** | 5 | 703 | 73 | 59 | 1 | 13 | 80.8% | ~95 LOC (helpers / unused comparators) | 13.5% | `pass` (0 auth) |
| **`checker`** | 4 | 1,098 | 20 | 2 | 10 | 8 | 10.0% | 712 LOC (`boundary.go`, `classify.go`, `aggregate.go`) | 64.8% | `pass` (0 auth) |
| **`manifest`** | 2 | 558 | 37 | 17 | 4 | 16 | 45.9% | ~180 LOC (internal parser / error types) | 32.3% | `pass` (0 auth) |
| **`surface`** | 2 | 332 | 4 | 4 | 0 | 0 | 100.0% | 0 LOC | 0.0% | `pass` (0 auth) |
| **`stdlibauthority`** | 1 | 270 | 16 | 14 | 2 | 0 | 87.5% | ~15 LOC | 5.5% | `pass` (0 auth) |
| **`report`** | 1 | 271 | 39 | 31 | 7 | 1 | 79.5% | ~10 LOC | 3.7% | `pass` (0 auth) |
| **`capanalyzer`** | 1 | 71 | 12 | 11 | 0 | 1 | 91.7% | 0 LOC | 0.0% | `pass` (0 auth) |
| **`hostpolicy`** | 1 | 107 | 5 | 5 | 0 | 0 | 100.0% | 0 LOC | 0.0% | `pass` (0 auth) |
| **`schema`** | 1 (handwritten) + 3 (gen) | 38 (pkg) + 1,626 (gen) | 1 (pkg) + 169 (gen) | 1 (pkg) + 61 (gen) | 0 (pkg) + 49 (gen) | 0 (pkg) + 59 (gen) | 36.5% overall | ~580 LOC (descriptor/reflection methods) | 34.8% | `fail` (15 UNANALYZED in gen) |

---

### Key Investigation Answers

#### 1. Interface Relative to Code Size
- **`checker`** represents the most drastic asymmetry: 1,098 LOC across 4 files, declaring 20 exported symbols, but **only 2 symbols** (`checker.Inputs` and `checker.Check`, checker.go:28, 52) are used outside the package by production code (`go/cmd/arcc/app`). 64.8% of the codebase (712 LOC across `aggregate.go`, `boundary.go`, and `classify.go`) consists of internal pipeline stages that do not need to be in the component's public interface package.
- **`symbol`** contains 1,704 LOC, but 1,049 LOC (61.6%) lives in `capslock.go`, which is consumed solely by `go/internal/stdlibmap` to generate the stdlib authority map. The core checking pipeline relies only on `symbol.go` (234 LOC) and `typesconv.go`/`extract.go` (421 LOC).
- **`surface`**, **`hostpolicy`**, and **`capanalyzer`** are already lean, with 90-100% of their exported symbols actively consumed across package boundaries.

#### 2. `symbol`: `capslock.go` (1,049 LOC) Examination
- **What is exported**:
  - `ParseCapslock(name string) (SymbolID, error)` ([capslock.go:44](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L44))
  - `type CapslockFunction struct` ([capslock.go:52](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L52))
  - `type CapslockInventory interface` ([capslock.go:65](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L65))
  - `ParseCapslockFunction(fn CapslockFunction, inv CapslockInventory) (SymbolID, error)` ([capslock.go:90](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L90))
  - `ParseCapslockWithPackages(name string, knownPackage func(importPath string) bool) (SymbolID, error)` ([capslock.go:125](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L125))
- **Who uses it**:
  - `ParseCapslock`, `CapslockFunction`, and `ParseCapslockFunction` are used **only** by `go/internal/stdlibmap` (`stdlibmap/inventory.go`).
  - `CapslockInventory` is used only by `stdlibmap/inventory_test.go` and as an argument to `ParseCapslockFunction`.
  - `ParseCapslockWithPackages` is completely unused outside `symbol` (internal-only).
  - No core checking or derivation component (`checker`, `facts`, `manifest`, `surface`, `report`, `stdlibauthority`) touches anything in `capslock.go`.
- **Architectural implication**: `capslock.go` accounts for **61.6%** of `symbol`'s code. It is a specialized normalizer for Capslock display strings, needed only when compiling standard library capability maps. Isolating it into `symbol/capslock` (or a dedicated `capslocknorm` member/component) removes over 1,000 LOC from the core vocabulary component interface.

#### 3. `schema`: Architecture Assessment
- **Who uses `schema.KnownCapabilities`**:
  - `go/internal/manifest` ([authority.go:105](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L105), [manifest.go:370](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go#L370))
  - `go/internal/artifactio` ([surface.go:143](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surface.go#L143), [stdlibmap.go:268, 364](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmap.go#L268))
  - `go/internal/stdlibmap` ([generate.go:541](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L541))
- **Who uses `gen.*` protobuf types directly**:
  - `go/internal/manifest` (`gen.Component`, `gen.ComponentDependency`, `gen.InterfaceStyle`, `gen.AnalysisDefeatingPolicy`, `gen.Authority`)
  - `go/internal/artifactio` (`gen.SurfaceManifest`, `gen.StdlibMap`, `gen.Component`, `gen.AuthorityDeclaration`)
  - `go/internal/surface` (`gen.SurfaceManifest`, `gen.AuthorityDeclaration`, `gen.SymbolCapability`)
  - `go/internal/goanalysis` (`gen.SurfaceManifest`, `gen.AuthorityDeclaration`)
  - `go/internal/stdlibmap` (`gen.StdlibMap`, `gen.StdlibPackage`, `gen.SymbolCapability`, `gen.Frame`)
  - `go/cmd/arcc/app` (`gen.SurfaceManifest`, `gen.StdlibMap`)
- **Assessment of component style**:
  - In Bazel `go_component`, `interface` must be exactly **one** `go_library`.
  - If `schema` were an interface-style component with `interface = "//go/internal/schema/gen:gen"` and `members = ["//go/internal/schema"]`, then consumers would only be allowed to reference symbols from `gen`. Calling `schema.KnownCapabilities` across the component boundary would violate `CALLS_UNDECLARED_INTERFACE`.
  - Conversely, if `interface = "//go/internal/schema"` and `members = ["//go/internal/schema/gen"]`, consumers could not import `gen.*` types directly.
  - Furthermore, `gen` contains 15 `UNDECLARED_AUTHORITY` / `AnalysisDefeating` findings (`unsafe.Slice`, `unsafe.StringData`, `(sync.Once).Do`) generated by `protoc-gen-go`.
  - **Recommendation**: `schema` should **remain `PACKAGE_SURFACE`** (or be split into two distinct components: `schema_gen` as asserted/package-surface generated code, and `schema` as pure handwritten capability vocabulary). Because `gen` consists entirely of generated protobuf stubs, `PACKAGE_SURFACE` is appropriate under the project's rule that PACKAGE_SURFACE is reserved for external / toolchain-generated artifacts.

#### 4. `manifest`: Authority Dependencies
- **Finding**: `manifest`'s **only** authority-relevant dependency is `prototext` (`google.golang.org/protobuf/encoding/prototext`), which is encapsulated behind the asserted `protobuf-runtime` component boundary.
- Non-test code in `manifest` imports only pure stdlib packages (`fmt`, `slices`, `sort`, `strings`, `errors`, `io`, `regexp`), plus `schema` and `schema/gen`.
- The `manifest` component declares no authority, and its `report.json` passes with zero violations (`violations: null`).

#### 5. `hostpolicy`: Manifest Declaration
- In `go/internal/hostpolicy/component.textproto`, there is **no `members:` line** (only `name: "hostpolicy"` and `interface_files: "hostpolicy.go"`).
- Per the component manifest specification, when `members:` is omitted, membership defaults to directory-based membership (the directory containing `component.textproto`).

---


## Component: symbol (dir: go/internal/symbol, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "symbol_component"`
  - `interface = ":symbol"` (library containing `capslock.go`, `extract.go`, `symbol.go`, `typesconv.go`)
  - `members = []`
  - `interface_style = ""` (default: interface library)
  - `declared_authority = []` (none)
  - `component_deps = ["//go/internal/hostpolicy:hostpolicy_component"]`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "symbol"`
  - `interface_files: "symbol.go"`
  - `interface_files: "typesconv.go"`
  - `interface_files: "extract.go"`
  - `interface_files: "capslock.go"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"`
  - `component_dependencies { name: "hostpolicy", manifest: "../hostpolicy/component.textproto" }`
  - **Divergence**: None; both declare all 4 `.go` files as interface files.
- **Files (non-test)**:
  - [`symbol.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go) (234 LOC): Defines `SymbolID` value type, v1 textual grammar parser, comparison, and formatting.
  - [`typesconv.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/typesconv.go) (289 LOC): Converts `go/types.Object` and `types.Selection` to canonical `SymbolID` under the declaring-object rule.
  - [`extract.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/extract.go) (132 LOC): Emits exact declared surface (sorted duplicate-free SymbolIDs) of a type-checked Go AST package.
  - [`capslock.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go) (1,049 LOC): Normalizes Capslock classifier function names into canonical v1 SymbolID syntax.
  - **Total non-test LOC**: 1,704 LOC.

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `ParseCapslock` | func | [`capslock.go:44`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L44) | `go/internal/stdlibmap` | - | false |
| `CapslockFunction` | type | [`capslock.go:52`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L52) | `go/internal/stdlibmap` | `stdlibmap:inventory_test.go` | false |
| `CapslockInventory` | type | [`capslock.go:65`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L65) | - | `stdlibmap:inventory_test.go` | false |
| `ParseCapslockFunction` | func | [`capslock.go:90`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L90) | `go/internal/stdlibmap` | `stdlibmap:inventory_test.go` | false |
| `ParseCapslockWithPackages` | func | [`capslock.go:125`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go#L125) | - | - | true |
| `ExtractSurface` | func | [`extract.go:30`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/extract.go#L30) | `go/internal/surface` | - | false |
| `GrammarVersion` | const | [`symbol.go:41`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go#L41) | - | - | true |
| `SymbolID` | type | [`symbol.go:66`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go#L66) | `artifactio`, `facts`, `stdlibauthority`, `stdlibmap`, `surface`, `teststdlibmap` | `cmd/arcc/app`, `checker`, `goanalysis`, `stdlibmap` tests | false |
| `Compare` | func | [`symbol.go:70`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go#L70) | `go/internal/checker`, `go/internal/facts`, `go/internal/stdlibmap` | - | false |
| `(SymbolID).Format` | method | [`symbol.go:78`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go#L78) | `artifactio`, `checker`, `goanalysis`, `teststdlibmap` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `(SymbolID).String` | method | [`symbol.go:83`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go#L83) | `artifactio`, `capslockadapter`, `checker`, `goanalysis`, `report`, `stdlibauthority`, `stdlibmap` | widespread test use | false |
| `ValidateCanonicalPath` | func | [`symbol.go:94`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go#L94) | `go/internal/facts` | - | false |
| `Parse` | func | [`symbol.go:102`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go#L102) | `artifactio`, `checker`, `facts`, `goanalysis`, `stdlibmap`, `teststdlibmap` | `stdlibmap` tests | false |
| `FromObject` | func | [`typesconv.go:25`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/typesconv.go#L25) | `go/internal/goanalysis`, `go/internal/stdlibmap` | - | false |
| `FromSelection` | func | [`typesconv.go:50`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/typesconv.go#L50) | - | - | true |

- **Summary**: Total exported symbols: **15**; count used externally (non-test): **11**; count test-only: **1**; count internal-only: **3**.

### 3. Authority inventory (measured)
- **Measured report**: `symbol.report.json` gives verdict `pass` with `violations: null`.
- **Declared authority**: None. The component is guaranteed pure computation.
- **Findings**: 0 capability findings. Does not use filesystem, network, reflection, unsafe, or OS APIs.

### 4. Internal structure and seams
- **Clusters**:
  1. *Core Grammar & Identity* ([`symbol.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go), 234 LOC): Pure string grammar parser, lexer, validator.
  2. *AST & Type Conversion* ([`typesconv.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/typesconv.go) 289 LOC, [`extract.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/extract.go) 132 LOC): Operates on `go/ast` and `go/types`.
  3. *Capslock String Normalization* ([`capslock.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go), 1,049 LOC): Complex text parsing of legacy/Capslock function descriptors.
- **Cross-file dependencies**:
  - `extract.go` -> calls `typesconv.FromObject`, `symbol.SymbolID`, `symbol.Compare`, `hostpolicy.CanonicalizePath`.
  - `typesconv.go` -> calls `symbol.SymbolID`, `symbol.Parse`.
  - `capslock.go` -> calls `symbol.SymbolID`, `hostpolicy.CanonicalizePath`.
- **Pure computation vs. environment interaction**: 100% pure computation across all files.
- **Package-level mutable state / init / build tags**: None.

### 5. Consumers' view
- `go/internal/surface`: uses `SymbolID`, `ExtractSurface`
- `go/internal/facts`: uses `SymbolID`, `Compare`, `Parse`, `ValidateCanonicalPath`
- `go/internal/checker`: uses `SymbolID`, `Compare`, `Parse`, `(SymbolID).Format`, `(SymbolID).String`
- `go/internal/stdlibauthority`: uses `SymbolID`, `(SymbolID).String`
- `go/internal/report`: uses `(SymbolID).String`
- `go/internal/artifactio`: uses `SymbolID`, `Parse`, `(SymbolID).Format`, `(SymbolID).String`
- `go/internal/goanalysis`: uses `SymbolID`, `Parse`, `FromObject`, `(SymbolID).Format`, `(SymbolID).String`
- `go/internal/stdlibmap`: uses `SymbolID`, `Compare`, `Parse`, `FromObject`, `(SymbolID).String`, and uniquely `ParseCapslock`, `CapslockFunction`, `ParseCapslockFunction`.

### 6. Tests
- 6 test files, **all external** (`package symbol_test`):
  `capslock_corpus_test.go`, `capslock_inventory_test.go`, `capslock_test.go`, `extract_test.go`, `symbol_test.go`, `typesconv_test.go`.
- Reliance on unexported identifiers: None.
- Testdata dirs: None.

### 7. Draft split proposal
- **Proposed interface package (`symbol`)**: Retain only [`symbol.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/symbol.go) (234 LOC). Exports `SymbolID`, `Parse`, `Compare`, `Format`, `String`, `ValidateCanonicalPath`.
- **Proposed member package 1 (`symbol/typesconv` or `symbol/extract`)**: [`extract.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/extract.go) + [`typesconv.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/typesconv.go) (421 LOC). Consumed only by `surface`, `goanalysis`, and `stdlibmap`.
- **Proposed member package 2 / sub-component (`symbol/capslock`)**: Move [`capslock.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/capslock.go) (1,049 LOC) to a dedicated member package or a standalone tooling component `capslocknorm`. It is used exclusively by `go/internal/stdlibmap`.
- **Resulting declared authority**: Stays empty (pure).
- **Risks**: Test split in `capslock_corpus_test.go` and `capslock_test.go` will need to import the new package path. No circular dependencies.


## Component: facts (dir: go/internal/facts, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/facts`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "facts_component"`
  - `interface = ":facts"` (contains `bypass.go`, `dependency_status.go`, `facts.go`, `member.go`, `refs.go`)
  - `members = []`
  - `interface_style = ""` (default: interface library)
  - `declared_authority = []` (none)
  - `component_deps = ["//go/internal/manifest:manifest_component", "//go/internal/symbol:symbol_component"]`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "facts"`
  - `interface_files: "bypass.go"`, `interface_files: "dependency_status.go"`, `interface_files: "facts.go"`, `interface_files: "member.go"`, `interface_files: "refs.go"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"`
  - `component_dependencies`: `manifest`, `symbol`
  - **Divergence**: None.
- **Files (non-test)**:
  - [`bypass.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go) (95 LOC): Classifies analysis-defeating constructs (`//go:linkname`, assembly, cgo) and represents bypass observations.
  - [`dependency_status.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go) (71 LOC): Structural authority lattice and provenance states carried by resolved dependencies.
  - [`facts.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go) (113 LOC): Pure data models for findings, package summaries, and resolved dependency interfaces.
  - [`member.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/member.go) (64 LOC): Component membership representation (`MemberSet`), validation, and lookup.
  - [`refs.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go) (360 LOC): Step 6 typed reference and import edge vocabulary (`ReferenceEdge`, `ImportEdge`), sorting, and deduplication.
  - **Total non-test LOC**: 703 LOC.

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `BypassKind` | type | [`bypass.go:13`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L13) | `go/internal/goanalysis` | - | false |
| `BypassLinkname` | const | [`bypass.go:17`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L17) | `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `BypassAssembly` | const | [`bypass.go:19`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L19) | `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `BypassCgo` | const | [`bypass.go:21`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L21) | `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `BypassObservation` | type | [`bypass.go:31`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L31) | `go/internal/checker`, `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `(BypassObservation).Validate` | method | [`bypass.go:38`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L38) | `go/internal/checker`, `go/internal/goanalysis` | `artifactio`, `stdlibauthority` tests | false |
| `CompareBypassObservations` | func | [`bypass.go:52`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L52) | - | - | true |
| `SortBypassObservations` | func | [`bypass.go:60`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L60) | `go/internal/goanalysis` | `goanalysis` tests | false |
| `DedupBypassObservations` | func | [`bypass.go:69`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L69) | `go/internal/goanalysis` | - | false |
| `BypassKey` | type | [`bypass.go:92`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go#L92) | `go/internal/goanalysis` | - | false |
| `DependencyAuthority` | type | [`dependency_status.go:12`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L12) | - | - | true |
| `DependencyProvenance` | type | [`dependency_status.go:19`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L19) | `go/internal/goanalysis` | `goanalysis` tests | false |
| `DependencyProvenanceCheckedPass` | const | [`dependency_status.go:22`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L22) | `go/internal/goanalysis` | `goanalysis` tests | false |
| `DependencyProvenanceCheckedFail` | const | [`dependency_status.go:23`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L23) | `go/internal/checker`, `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `DependencyProvenanceAsserted` | const | [`dependency_status.go:24`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L24) | `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `ProvenanceCheckedPass` | const | [`dependency_status.go:25`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L25) | - | - | true |
| `ProvenanceCheckedFail` | const | [`dependency_status.go:26`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L26) | - | - | true |
| `AuthorityDeclared` | const | [`dependency_status.go:29`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L29) | - | - | true |
| `AuthorityUnknown` | const | [`dependency_status.go:30`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L30) | - | - | true |
| `AuthorityFree` | const | [`dependency_status.go:31`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L31) | - | - | true |
| `AuthorityDerived` | const | [`dependency_status.go:32`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L32) | - | - | true |
| `TopAuthority` | func | [`dependency_status.go:35`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L35) | - | - | true |
| `BottomAuthority` | func | [`dependency_status.go:40`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L40) | - | - | true |
| `AuthorityLattice` | type | [`dependency_status.go:45`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L45) | - | - | true |
| `DependencyStatus` | type | [`dependency_status.go:50`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go#L50) | `go/internal/goanalysis` | - | false |
| `SymbolID` | type | [`facts.go:17`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go#L17) | `go/internal/checker`, `go/internal/goanalysis` | `checker` tests | false |
| `DependencyInterface` | type | [`facts.go:30`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go#L30) | `go/internal/checker`, `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `(DependencyInterface).IsPackageSurface` | method | [`facts.go:41`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go#L41) | `go/internal/checker`, `go/internal/goanalysis` | `checker` tests | false |
| `(DependencyInterface).OwnsPackage` | method | [`facts.go:47`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go#L47) | `go/internal/checker` | `checker` tests | false |
| `(DependencyInterface).ContainsSymbol` | method | [`facts.go:57`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go#L57) | `go/internal/checker` | `checker` tests | false |
| `PackageSummary` | type | [`facts.go:76`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go#L76) | `go/internal/checker`, `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `(PackageSummary).Validate` | method | [`facts.go:94`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go#L94) | `go/internal/checker`, `go/internal/goanalysis` | `artifactio`, `stdlibauthority` tests | false |
| `MemberSet` | type | [`member.go:12`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/member.go#L12) | `go/internal/checker`, `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `NewMemberSet` | func | [`member.go:17`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/member.go#L17) | `go/internal/checker`, `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `(MemberSet).Contains` | method | [`member.go:37`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/member.go#L37) | `go/internal/checker`, `go/internal/goanalysis` | `checker` tests | false |
| `(MemberSet).Len` | method | [`member.go:42`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/member.go#L42) | `go/internal/checker`, `go/internal/goanalysis` | `checker` tests | false |
| `(MemberSet).Slice` | method | [`member.go:48`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/member.go#L48) | `go/internal/checker`, `go/internal/goanalysis` | `checker` tests | false |
| `SourceSite` | type | [`refs.go:19`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L19) | `go/internal/checker`, `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `CompareSourceSite` | func | [`refs.go:30`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L30) | `go/internal/checker` | - | false |
| `(SourceSite).Validate` | method | [`refs.go:52`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L52) | `go/internal/checker`, `go/internal/goanalysis` | `artifactio`, `stdlibauthority` tests | false |
| `ReferenceKind` | type | [`refs.go:94`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L94) | `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `RefFunc` | const | [`refs.go:97`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L97) | `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `RefMethod` | const | [`refs.go:98`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L98) | `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `RefType` | const | [`refs.go:99`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L99) | `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `RefField` | const | [`refs.go:100`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L100) | `go/internal/goanalysis` | `goanalysis` tests | false |
| `RefVar` | const | [`refs.go:101`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L101) | `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `RefConst` | const | [`refs.go:102`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L102) | `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `ReferenceEdge` | type | [`refs.go:112`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L112) | `go/internal/checker`, `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `ReferenceKey` | type | [`refs.go:122`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L122) | `go/internal/goanalysis` | `goanalysis` tests | false |
| `(ReferenceEdge).Key` | method | [`refs.go:131`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L131) | `go/internal/goanalysis` | `goanalysis` tests | false |
| `(ReferenceEdge).Validate` | method | [`refs.go:144`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L144) | `go/internal/checker`, `go/internal/goanalysis` | `artifactio`, `stdlibauthority` tests | false |
| `SymbolIDPackage` | func | [`refs.go:175`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L175) | `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `CompareSymbolIDs` | func | [`refs.go:196`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L196) | - | - | true |
| `CompareReferenceEdges` | func | [`refs.go:201`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L201) | - | - | true |
| `SortReferenceEdges` | func | [`refs.go:220`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L220) | `go/internal/checker`, `go/internal/goanalysis` | `goanalysis` tests | false |
| `CloneReferenceEdges` | func | [`refs.go:228`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L228) | - | - | true |
| `DedupReferenceEdges` | func | [`refs.go:237`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L237) | `go/internal/goanalysis` | - | false |
| `ImportResolution` | type | [`refs.go:253`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L253) | - | `checker`, `goanalysis` tests | false |
| `ImportResolved` | const | [`refs.go:258`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L258) | `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `ImportMissingTypeData` | const | [`refs.go:263`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L263) | `go/internal/checker`, `go/internal/goanalysis` | `checker`, `goanalysis` tests | false |
| `ImportUnresolved` | const | [`refs.go:267`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L267) | `go/internal/checker`, `go/internal/goanalysis` | `checker` tests | false |
| `ImportEdge` | type | [`refs.go:275`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L275) | `go/internal/checker`, `go/internal/goanalysis` | `cmd/arcc/app`, `checker`, `goanalysis` tests | false |
| `ImportKey` | type | [`refs.go:283`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L283) | `go/internal/goanalysis` | `goanalysis` tests | false |
| `(ImportEdge).Key` | method | [`refs.go:291`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L291) | `go/internal/goanalysis` | `goanalysis` tests | false |
| `(ImportEdge).Validate` | method | [`refs.go:302`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L302) | `go/internal/checker`, `go/internal/goanalysis` | `artifactio`, `stdlibauthority` tests | false |
| `CompareImportEdges` | func | [`refs.go:327`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L327) | - | - | true |
| `SortImportEdges` | func | [`refs.go:342`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L342) | `go/internal/checker`, `go/internal/goanalysis` | `goanalysis` tests | false |
| `DedupImportEdges` | func | [`refs.go:351`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go#L351) | `go/internal/goanalysis` | - | false |

- **Summary**: Total exported symbols: **73**; count used externally (non-test): **59**; count test-only: **1**; count internal-only: **13**.

### 3. Authority inventory (measured)
- **Measured report**: `facts.report.json` passes with `violations: null`.
- **Declared authority**: None.
- **Findings**: 0 capability findings. Pure data models and canonicalization.

### 4. Internal structure and seams
- **Clusters**:
  1. *Reference & Import Edges* ([`refs.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go), 360 LOC - 51.2%): Edge models and deterministic ordering/validation.
  2. *Membership* ([`member.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/member.go), 64 LOC): Member set lookup.
  3. *Bypass Observations* ([`bypass.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/bypass.go), 95 LOC): Compiler bypass classification.
  4. *Dependency Interfaces & Summaries* ([`facts.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go) 113 LOC, [`dependency_status.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/dependency_status.go) 71 LOC): Inter-component dependency facts.
- **Cross-file references**:
  - `facts.go` uses `symbol.SymbolID`, `manifest.InterfaceStyle`.
  - `refs.go` uses `symbol.SymbolID`, `symbol.Compare`, `symbol.ValidateCanonicalPath`.
  - `member.go` uses `manifest.ValidateMemberPackage`.
- **Package-level mutable state / init / build tags**: None.

### 5. Consumers' view
- `go/internal/checker`: consumes `BypassObservation`, `MemberSet`, `DependencyInterface`, `PackageSummary`, `ReferenceEdge`, `ImportEdge`, `SourceSite`, and their sorting/validation methods.
- `go/internal/goanalysis`: the primary producer; uses virtually all types and sorting/dedup helpers.

### 6. Tests
- 5 external test files (`bypass_test.go`, `dependency_status_test.go`, `facts_test.go`, `member_test.go`, `refs_test.go`), all `package facts_test`.
- No unexported identifier access; no testdata directory.

### 7. Draft split proposal
- `facts` is a cohesive shared vocabulary package with high external utilization (80.8%).
- Potential seam: split [`refs.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/refs.go) into `facts/edges` (360 LOC) to isolate the typed edge model from component membership (`member.go`) and dependency definitions (`facts.go`).
- Declared authority remains empty.


## Component: checker (dir: go/internal/checker, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/checker`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "checker_component"`
  - `interface = ":checker"` (srcs: `aggregate.go`, `boundary.go`, `checker.go`, `classify.go`)
  - `members = []`
  - `component_deps = [capanalyzer, facts, manifest, report, stdlibauthority, symbol]`
  - `declared_authority = []`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "checker"`
  - `interface_files: "aggregate.go"`
  - `interface_files: "checker.go"`
  - `interface_files: "classify.go"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"`
  - `component_dependencies`: `capanalyzer`, `facts`, `manifest`, `report`, `stdlibauthority`, `symbol`
  - **Divergence**: In `component.textproto`, `boundary.go` is **omitted** from `interface_files` (only aggregate.go, checker.go, classify.go are listed). In Bazel `BUILD.bazel`, however, `boundary.go` is part of `:checker` which is assigned to `interface`, making `boundary.go` an interface file in Bazel.
- **Files (non-test)**:
  - [`checker.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go) (386 LOC): Top-level component checker; orchestrates classification, aggregation, boundary checks, and report generation via `Check(inputs Inputs) (*report.Report, error)`.
  - [`classify.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go) (330 LOC): Dispatches typed reference edges and import edges to stdlib authority and dependency boundaries.
  - [`aggregate.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/aggregate.go) (196 LOC): Aggregates authority findings, applies capability policy (Strict vs Warn), and evaluates unused declared authority.
  - [`boundary.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go) (186 LOC): Classifies reference edges against resolved direct dependencies and detects package overlaps.
  - **Total non-test LOC**: 1,098 LOC.

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `AuthoritySite` | type | [`aggregate.go:41`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/aggregate.go#L41) | - | - | true |
| `AuthorityFinding` | type | [`aggregate.go:51`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/aggregate.go#L51) | - | - | true |
| `AggregateAuthority` | func | [`aggregate.go:65`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/aggregate.go#L65) | - | - | true |
| `ApplyAuthorityPolicy` | func | [`aggregate.go:146`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/aggregate.go#L146) | - | - | true |
| `ReferenceStatus` | type | [`boundary.go:41`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L41) | - | `goanalysis:fixture_test.go` | false |
| `ReferenceIntraComponent` | const | [`boundary.go:46`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L46) | - | - | true |
| `ReferenceDeclaredDependency` | const | [`boundary.go:51`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L51) | - | `goanalysis:fixture_test.go` | false |
| `ReferenceCallsUndeclaredInterface` | const | [`boundary.go:54`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L54) | - | `goanalysis:fixture_test.go` | false |
| `ReferenceUndeclaredDependency` | const | [`boundary.go:57`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L57) | - | `goanalysis:fixture_test.go` | false |
| `ReferenceDecision` | type | [`boundary.go:64`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L64) | - | - | true |
| `(ReferenceDecision).ViolationKind` | method | [`boundary.go:71`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L71) | - | `goanalysis:fixture_test.go` | false |
| `BoundaryIndex` | type | [`boundary.go:85`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L85) | - | - | true |
| `NewBoundaryIndex` | func | [`boundary.go:96`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L96) | - | `goanalysis:fixture_test.go` | false |
| `(BoundaryIndex).Classify` | method | [`boundary.go:153`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L153) | - | `capanalyzer_test.go`, `goanalysis:fixture_test.go` | false |
| `Inputs` | type | [`checker.go:28`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go#L28) | `go/cmd/arcc/app` | `cmd/arcc`, `goanalysis` tests | false |
| `Check` | func | [`checker.go:52`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go#L52) | `go/cmd/arcc/app` | `cmd/arcc`, `goanalysis` tests | false |
| `AuthorityObservation` | type | [`classify.go:52`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go#L52) | - | `goanalysis` fixture tests | false |
| `BoundaryObservation` | type | [`classify.go:67`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go#L67) | - | - | true |
| `Classified` | type | [`classify.go:82`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go#L82) | - | `goanalysis` fixture tests | false |
| `ClassifyEdges` | func | [`classify.go:94`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go#L94) | - | `goanalysis` fixture tests | false |

- **Summary**: Total exported symbols: **20**; count used externally (non-test): **2** (10.0%); count test-only: **10** (50.0%); count internal-only: **8** (40.0%).

### 3. Authority inventory (measured)
- **Measured report**: `checker.report.json` passes with `violations: null`.
- **Declared authority**: None.
- **Findings**: 0 capability findings. Pure pipeline.

### 4. Internal structure and seams
- **Pipeline Structure**:
  - `checker.go` (386 LOC) is the orchestration coordinator.
  - `classify.go` (330 LOC) invokes `boundary.go` and `stdlibauthority` to classify edges.
  - `boundary.go` (186 LOC) builds `BoundaryIndex` and checks whether referents fall within declared interfaces or owned packages.
  - `aggregate.go` (196 LOC) filters observations into violations and warnings according to `manifest.AnalysisDefeatingPolicy` and unused authority.
  - Total internal pipeline: 712 LOC (64.8% of the package).
- **Seams**: The entire execution path is internal to `checker.Check(inputs)`. None of `ClassifyEdges`, `NewBoundaryIndex`, `AggregateAuthority`, or `ApplyAuthorityPolicy` are called by external production code.
- **Package-level mutable state / init / build tags**: None.

### 5. Consumers' view
- **Single consumer**: `go/cmd/arcc/app`.
- **Symbols consumed**: Only [`checker.Inputs`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go#L28) and [`checker.Check`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go#L52).

### 6. Tests
- 5 external test files (`aggregate_test.go`, `boundary_test.go`, `checker_test.go`, `classify_test.go`, `unused_authority_test.go`), all `package checker_test`.
- They test sub-pipeline stages via their exported names.
- No unexported identifier access; no testdata dir.

### 7. Draft split proposal
- **Proposed interface package (`checker`)**: Contains only [`checker.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go) (386 LOC). Exports solely `Inputs` and `Check`.
- **Proposed member package (`checker/internal/engine` or `checker/internal/boundary`)**: Move `boundary.go` (186 LOC), `classify.go` (330 LOC), and `aggregate.go` (196 LOC) to internal member packages.
- **Resolving divergence**: Aligns Bazel declaration with `component.textproto` where `boundary.go` was already excluded from `interface_files`.
- **Authority**: Stays empty (pure).


## Component: manifest (dir: go/internal/manifest, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "manifest_component"`
  - `interface = ":manifest"` (`authority.go`, `manifest.go`)
  - `members = []`
  - `component_deps = ["//go/internal/protobufruntime:protobuf-runtime", "//go/internal/schema:schema_component"]`
  - `declared_authority = []`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "manifest"`
  - `interface_files: "authority.go"`, `interface_files: "manifest.go"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"`
  - `component_dependencies`: `schema`, `protobuf-runtime`
  - **Divergence**: None.
- **Files (non-test)**:
  - [`authority.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go) (182 LOC): Ambient authority capability representation, lattice algebra (Union, Intersects, Difference, SubsetOf), and `AnalysisDefeatingPolicy`.
  - [`manifest.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go) (376 LOC): Component manifest parser (`prototext` decoding, member normalization, pattern validation, error formatting).
  - **Total non-test LOC**: 558 LOC.

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `Capability` | type | [`authority.go:14`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L14) | - | - | true |
| `AnalysisDefeatingPolicy` | type | [`authority.go:20`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L20) | - | - | true |
| `AnalysisDefeatingPolicyStrict` | const | [`authority.go:24`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L24) | - | `manifestparity_test.go` | false |
| `AnalysisDefeatingPolicyWarn` | const | [`authority.go:26`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L26) | `go/cmd/arcc/app` | `manifestparity_test.go` | false |
| `AuthorityDeclaration` | type | [`authority.go:34`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L34) | - | `manifestparity_test.go` | false |
| `UnknownAuthority` | func | [`authority.go:43`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L43) | `go/cmd/arcc/app`, `go/internal/goanalysis` | `surface` tests | false |
| `DeclaredAuthority` | func | [`authority.go:50`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L50) | `go/cmd/arcc/app`, `go/internal/goanalysis` | `manifestparity`, `surface` tests | false |
| `(AuthorityDeclaration).IsUnknown` | method | [`authority.go:57`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L57) | `checker`, `goanalysis`, `surface` | tests | false |
| `(AuthorityDeclaration).IsDeclared` | method | [`authority.go:59`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L59) | `goanalysis` | tests | false |
| `(AuthorityDeclaration).HasCapability` | method | [`authority.go:61`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L61) | `checker` | - | false |
| `(AuthorityDeclaration).Capabilities` | method | [`authority.go:73`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L73) | `checker`, `surface` | tests | false |
| `(AuthorityDeclaration).Equals` | method | [`authority.go:82`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L82) | `checker` | - | false |
| `(AuthorityDeclaration).Union` | method | [`authority.go:96`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L96) | `checker` | tests | false |
| `(AuthorityDeclaration).Intersects` | method | [`authority.go:111`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L111) | `checker` | - | false |
| `(AuthorityDeclaration).Difference` | method | [`authority.go:124`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L124) | `checker` | tests | false |
| `(AuthorityDeclaration).SubsetOf` | method | [`authority.go:137`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L137) | `checker` | tests | false |
| `(AuthorityDeclaration).IsDisjoint` | method | [`authority.go:150`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L150) | `checker` | - | false |
| `(AuthorityDeclaration).String` | method | [`authority.go:163`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L163) | `checker`, `goanalysis` | tests | false |
| `InterfaceStyle` | type | [`manifest.go:30`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go#L30) | `checker`, `goanalysis`, `surface` | tests | false |
| `InterfaceStyleDeclared` | const | [`manifest.go:34`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go#L34) | - | `manifestparity_test.go` | false |
| `InterfaceStylePackageSurface` | const | [`manifest.go:36`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go#L36) | `checker`, `goanalysis`, `surface` | `cmd/arcc/app`, `checker` tests | false |
| `Manifest` | type | [`manifest.go:172`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go#L172) | `cmd/arcc/app`, `checker`, `manifestparity` | tests | false |
| `ComponentDependency` | type | [`manifest.go:191`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go#L191) | `cmd/arcc/app`, `goanalysis`, `manifestparity` | tests | false |
| `Parse` | func | [`manifest.go:199`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go#L199) | `go/cmd/arcc/app`, `go/internal/manifestparity` | tests | false |
| Error sentinels & types (13 items) | type/var | `manifest.go:23-162` | - | - | true |

- **Summary**: Total exported symbols: **37**; count used externally (non-test): **17**; count test-only: **4**; count internal-only: **16**.

### 3. Authority inventory (measured)
- **Measured report**: `manifest.report.json` passes with `violations: null`.
- **Declared authority**: None.
- **Authority dependency check**: Non-test source files import only pure stdlib (`fmt`, `slices`, `sort`, `strings`, `errors`, `io`, `regexp`), plus `schema` and `google.golang.org/protobuf/encoding/prototext`. `prototext` is encapsulated behind the asserted `protobuf-runtime` boundary. Hence, `prototext` is `manifest`'s **sole authority-relevant dependency**, and all authority used inside it terminates structurally at the `protobuf-runtime` boundary.

### 4. Internal structure and seams
- **Clusters**:
  1. *Authority Lattice* ([`authority.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go), 182 LOC): Set theory operations on Capslock capability sets. Validated against `schema.KnownCapabilities`.
  2. *Manifest Parser & Model* ([`manifest.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go), 376 LOC): Reads protobuf text format using `prototext.Unmarshal` and builds `Manifest`.
- **Package-level mutable state**: `removedFieldHints` map and `removedFields` slice (read-only error hint lookup tables). No `init()`; no build tags.

### 5. Consumers' view
- `go/cmd/arcc/app`: `Parse`, `Manifest`, `ComponentDependency`, `DeclaredAuthority`, `UnknownAuthority`, `AnalysisDefeatingPolicyWarn`.
- `go/internal/checker`: `Manifest`, `InterfaceStylePackageSurface`, `AuthorityDeclaration` methods.
- `go/internal/goanalysis`: `ComponentDependency`, `InterfaceStylePackageSurface`, `AuthorityDeclaration` methods.
- `go/internal/surface`: `InterfaceStylePackageSurface`, `AuthorityDeclaration` methods.

### 6. Tests
- 4 external test files (`authority_test.go`, `manifest_test.go`, `pattern_match_test.go`, `schema_test.go`), all `package manifest_test`.
- No reliance on unexported identifiers; no testdata dir.

### 7. Draft split proposal
- The package is compact (558 LOC) and pure.
- Could cleanly split into:
  - `manifest/authority`: [`authority.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go) (authority lattice used by checker and analysis).
  - `manifest`: [`manifest.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go) (manifest parsing model).
- Resulting authority: Remains pure.


## Component: surface (dir: go/internal/surface, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/surface`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "surface_component"`
  - `interface = ":surface"` (`derive.go`, `digest.go`)
  - `members = []`
  - `component_deps = [manifest, schema, symbol, stdlibauthority]`
  - `declared_authority = []`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "surface"`
  - `interface_files: "derive.go"`, `interface_files: "digest.go"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"`
  - `component_dependencies`: `manifest`, `schema`, `symbol`, `stdlibauthority`
  - **Divergence**: None.
- **Files (non-test)**:
  - [`derive.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/derive.go) (248 LOC): Derives persisted `gen.SurfaceManifest` from type-checked package ASTs, manifest, and stdlib authority map.
  - [`digest.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/digest.go) (84 LOC): Computes sha256 producer digest for surface files and manifest.
  - **Total non-test LOC**: 332 LOC.

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `SourceFile` | type | [`derive.go:51`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/derive.go#L51) | `cmd/arcc/app`, `artifactio`, `goanalysis` | tests | false |
| `Input` | type | [`derive.go:59`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/derive.go#L59) | `go/cmd/arcc/app` | tests | false |
| `Derive` | func | [`derive.go:93`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/derive.go#L93) | `go/cmd/arcc/app` | tests | false |
| `DigestForSources` | func | [`digest.go:17`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/digest.go#L17) | `go/internal/goanalysis` | - | false |

- **Summary**: Total exported symbols: **4**; count used externally (non-test): **4** (100%); count test-only: **0**; count internal-only: **0**.

### 3. Authority inventory (measured)
- **Measured report**: `surface.report.json` passes with `violations: null`.
- **Declared authority**: None.
- **Findings**: 0 capability findings. Uses `crypto/sha256` for hashing in-memory bytes, which carries no ambient authority.

### 4. Internal structure and seams
- 332 LOC across 2 cohesive files: `derive.go` performs surface extraction and protobuf message construction; `digest.go` computes deterministic hash of input sources.
- No package-level mutable state, no `init()`, no build tags.

### 5. Consumers' view
- `go/cmd/arcc/app`: `Derive`, `Input`, `SourceFile`
- `go/internal/goanalysis`: `SourceFile`, `DigestForSources`
- `go/internal/artifactio`: `SourceFile`

### 6. Tests
- 2 external test files (`derive_test.go`, `digest_test.go`), `package surface_test`.
- No unexported identifier access; no testdata dir.

### 7. Draft split proposal
- The component is small (332 LOC) and 100% of its API is externally utilized. It can remain as a single interface library.


## Component: stdlibauthority (dir: go/internal/stdlibauthority, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "stdlibauthority_component"`
  - `interface = ":stdlibauthority"` (`stdlibauthority.go`)
  - `members = []`
  - `component_deps = ["//go/internal/symbol:symbol_component"]`
  - `declared_authority = []`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "stdlibauthority"`
  - `interface_files: "stdlibauthority.go"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"`
  - `component_dependencies`: `symbol`
  - **Divergence**: None.
- **Files (non-test)**:
  - [`stdlibauthority.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go) (270 LOC): Core-side port for querying standard-library package membership, symbol capabilities, and package init authority from an in-memory stdlib map.
  - **Total non-test LOC**: 270 LOC (< 300 LOC; sections 3 and 4 abbreviated).

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `Capability` | type | [`stdlibauthority.go:38`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L38) | `checker` | - | false |
| `StdlibAuthority` | type | [`stdlibauthority.go:42`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L42) | `checker`, `surface` | tests | false |
| `NewFromMap` | func | [`stdlibauthority.go:73`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L73) | `cmd/arcc/app`, `goanalysis` | tests | false |
| `(StdlibAuthority).KnownPackage` | method | [`stdlibauthority.go:121`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L121) | `checker`, `goanalysis` | tests | false |
| `(StdlibAuthority).SymbolAuthority` | method | [`stdlibauthority.go:138`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L138) | `checker`, `surface` | tests | false |
| `(StdlibAuthority).PackageAuthority` | method | [`stdlibauthority.go:158`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L158) | `checker` | - | false |
| `(StdlibAuthority).InitAuthority` | method | [`stdlibauthority.go:167`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L167) | `checker` | - | false |
| `(StdlibAuthority).PackageList` | method | [`stdlibauthority.go:175`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L175) | `goanalysis` | tests | false |
| `(StdlibAuthority).ValidateTargetSDK` | method | [`stdlibauthority.go:193`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L193) | `cmd/arcc/app`, `goanalysis` | tests | false |
| `(StdlibAuthority).HasInventoryGap` | method | [`stdlibauthority.go:216`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L216) | - | `stdlibauthority_test.go` | false |
| `(StdlibAuthority).SDKKey` | method | [`stdlibauthority.go:221`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L221) | - | `stdlibauthority_test.go` | false |
| `ErrInventoryGap` | var | [`stdlibauthority.go:227`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L227) | `cmd/arcc/app` | - | false |
| `ErrKeyMismatch` | var | [`stdlibauthority.go:254`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L254) | `cmd/arcc/app` | - | false |
| `KeyMismatchError` | type | [`stdlibauthority.go:258`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L258) | `cmd/arcc/app` | - | false |
| `(KeyMismatchError).Error` | method | [`stdlibauthority.go:264`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L264) | `cmd/arcc/app` | - | false |
| `(KeyMismatchError).Unwrap` | method | [`stdlibauthority.go:270`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority.go#L270) | `cmd/arcc/app` | - | false |

- **Summary**: Total exported symbols: **16**; count used externally (non-test): **14**; count test-only: **2**; count internal-only: **0**.

### 3. Authority inventory (measured)
- Report `stdlibauthority.report.json` passes (`violations: null`). 0 capability findings. Pure in-memory lookup.

### 4. Internal structure and seams
- Single file, 270 LOC (< 300 LOC). Pure in-memory lookup index. No mutable state except error sentinels.

### 5. Consumers' view
- `cmd/arcc/app`: `NewFromMap`, `ValidateTargetSDK`, error sentinels.
- `checker`: `StdlibAuthority`, `SymbolAuthority`, `PackageAuthority`, `InitAuthority`, `KnownPackage`.
- `surface`: `StdlibAuthority`, `SymbolAuthority`.
- `goanalysis`: `NewFromMap`, `KnownPackage`, `PackageList`, `ValidateTargetSDK`.

### 6. Tests
- 1 external test file [`stdlibauthority_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibauthority/stdlibauthority_test.go) (`package stdlibauthority_test`).

### 7. Draft split proposal
- Maintain as single small interface package (270 LOC).


## Component: report (dir: go/internal/report, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/report`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "report_component"`
  - `interface = ":report"` (`report.go`)
  - `members = []`
  - `declared_authority = []`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "report"`
  - `interface_files: "report.go"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/report"`
  - **Divergence**: None.
- **Files (non-test)**:
  - [`report.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go) (271 LOC): Conformance report data models (`Report`, `Finding`, `Kind`, `Location`, `AuthoritySite`) and JSON rendering logic.
  - **Total non-test LOC**: 271 LOC (< 300 LOC; sections 3 and 4 abbreviated).

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `Kind` | type | [`report.go:16`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L16) | `checker` | tests | false |
| `UndeclaredDependency` | const | [`report.go:20`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L20) | `checker` | widespread test use | false |
| `CallsUndeclaredInterface` | const | [`report.go:21`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L21) | `checker` | tests | false |
| `UndeclaredAuthority` | const | [`report.go:22`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L22) | `checker` | widespread test use | false |
| `MethodOutsideInterface` | const | [`report.go:23`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L23) | `checker` | tests | false |
| `MemberOverlap` | const | [`report.go:24`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L24) | `checker` | tests | false |
| `AnalysisLimitation` | const | [`report.go:27`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L27) | `checker` | tests | false |
| `AllowedWithWarning` | const | [`report.go:28`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L28) | `checker` | tests | false |
| `UnusedDependency` | const | [`report.go:29`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L29) | `checker` | tests | false |
| `UnusedAuthority` | const | [`report.go:30`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L30) | `checker` | tests | false |
| `InterfaceFileExcluded` | const | [`report.go:31`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L31) | `cmd/arcc/app` | tests | false |
| `DependencyCheckFailed` | const | [`report.go:32`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L32) | `checker` | tests | false |
| `DependencySurfaceStale` | const | [`report.go:33`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L33) | `checker` | tests | false |
| `(Kind).String` | method | [`report.go:37`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L37) | `checker` | widespread test use | false |
| `Location` | type | [`report.go:42`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L42) | `cmd/arcc/app`, `checker` | tests | false |
| `AuthoritySite` | type | [`report.go:51`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L51) | `artifactio`, `checker` | tests | false |
| `Finding` | type | [`report.go:64`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L64) | `cmd/arcc/app`, `artifactio`, `checker` | tests | false |
| `DependencyProvenance` | type | [`report.go:77`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L77) | `checker` | - | false |
| `DependencyProvenanceCheckedPass` | const | [`report.go:80`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L80) | - | `artifactio`, `goanalysis` tests | false |
| `DependencyProvenanceCheckedFail` | const | [`report.go:81`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L81) | - | `checker`, `goanalysis` tests | false |
| `DependencyProvenanceAsserted` | const | [`report.go:82`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L82) | - | `artifactio`, `goanalysis` tests | false |
| `DependencyFreshness` | type | [`report.go:87`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L87) | `checker` | - | false |
| `DependencyFreshnessBuildGraph` | const | [`report.go:90`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L90) | - | `artifactio`, `goanalysis` tests | false |
| `DependencyFreshnessVerified` | const | [`report.go:91`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L91) | - | `cmd/arcc/app` tests | false |
| `DependencyFreshnessStale` | const | [`report.go:92`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L92) | - | `cmd/arcc/app` tests | false |
| `DependencyFreshnessUnknown` | const | [`report.go:93`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L93) | - | `cmd/arcc/app` tests | false |
| `DependencyAuthority` | type | [`report.go:99`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L99) | - | - | true |
| `DependencyAuthorityDeclared` | const | [`report.go:102`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L102) | `checker` | tests | false |
| `DependencyAuthorityUnknown` | const | [`report.go:103`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L103) | `checker` | tests | false |
| `Dependency` | type | [`report.go:108`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L108) | `checker` | tests | false |
| `Report` | type | [`report.go:119`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L119) | `cmd/arcc/app`, `artifactio`, `checker` | tests | false |
| `(Report).Conforms` | method | [`report.go:142`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L142) | `cmd/arcc/app` | tests | false |
| `(Report).Verdict` | method | [`report.go:146`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L146) | `cmd/arcc/app` | tests | false |
| `RenderJSON` | func | [`report.go:153`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L153) | `cmd/arcc/app`, `artifactio` | tests | false |
| JSON format types (FormatVersion1, VerdictPass, VerdictFail, Artifact, etc.) | types/consts | `report.go:175-236` | `cmd/arcc/app`, `artifactio` | tests | false |

- **Summary**: Total exported symbols: **39**; count used externally (non-test): **31**; count test-only: **7**; count internal-only: **1**.

### 3. Authority inventory (measured)
- Report `report.report.json` passes (`violations: null`). 0 capability findings. Uses `encoding/json` in memory without OS or reflection authority.

### 4. Internal structure and seams
- 271 LOC in 1 file. Pure data models and JSON serialization.

### 5. Consumers' view
- `cmd/arcc/app`: `Report`, `Finding`, `VerdictPass`, `VerdictFail`, `RenderJSON`, `(Report).Conforms`.
- `checker`: generates `Report`, using `Finding`, `Kind` constants, `AuthoritySite`, `Dependency`.
- `artifactio`: encodes and decodes `Report`, `Finding`, `AuthoritySite`.

### 6. Tests
- 2 external test files ([`artifact_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/artifact_test.go), [`report_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report_test.go)), `package report_test`.

### 7. Draft split proposal
- Retain as small cohesive interface package (271 LOC).


## Component: capanalyzer (dir: go/internal/capanalyzer, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "capanalyzer_component"`
  - `interface = ":capanalyzer"` (`capanalyzer.go`)
  - `members = []`
  - `declared_authority = []`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "capanalyzer"`
  - `interface_files: "capanalyzer.go"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"`
  - **Divergence**: None.
- **Files (non-test)**:
  - [`capanalyzer.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go) (71 LOC): Capability policy classes (`TrueAuthority`, `AnalysisDefeating`), strict policy evaluation, and capability classification.
  - **Total non-test LOC**: 71 LOC (< 300 LOC; sections 3 and 4 abbreviated).

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `Class` | type | [`capanalyzer.go:12`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L12) | `checker` | tests | false |
| `TrueAuthority` | const | [`capanalyzer.go:15`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L15) | `checker` | tests | false |
| `AnalysisDefeating` | const | [`capanalyzer.go:16`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L16) | `checker` | tests | false |
| `(Class).String` | method | [`capanalyzer.go:20`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L20) | `checker` | tests | false |
| `CapabilityPolicy` | type | [`capanalyzer.go:25`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L25) | `cmd/arcc/app`, `checker` | tests | false |
| `StrictPolicy` | func | [`capanalyzer.go:31`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L31) | `cmd/arcc/app` | tests | false |
| `Decision` | type | [`capanalyzer.go:36`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L36) | - | - | true |
| `DecisionViolation` | const | [`capanalyzer.go:39`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L39) | `checker` | - | false |
| `DecisionWarn` | const | [`capanalyzer.go:40`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L40) | `checker` | - | false |
| `DecisionAllowed` | const | [`capanalyzer.go:41`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L41) | `checker` | - | false |
| `(Decision).String` | method | [`capanalyzer.go:45`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L45) | `checker` | tests | false |
| `Classify` | func | [`capanalyzer.go:63`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer.go#L63) | `checker` | - | false |

- **Summary**: Total exported symbols: **12**; count used externally (non-test): **11**; count test-only: **0**; count internal-only: **1**.

### 3. Authority inventory (measured)
- Report `capanalyzer.report.json` passes (`violations: null`). 0 capability findings. Pure logic.

### 4. Internal structure and seams
- 71 LOC in 1 file (< 300 LOC). Pure classification functions and constants.

### 5. Consumers' view
- `go/internal/checker`: consumes `Classify`, `Class`, `TrueAuthority`, `AnalysisDefeating`, `CapabilityPolicy`, `DecisionViolation`, etc.
- `go/cmd/arcc/app`: consumes `StrictPolicy`, `CapabilityPolicy`.

### 6. Tests
- 1 external test file ([`capanalyzer_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capanalyzer/capanalyzer_test.go)), `package capanalyzer_test`.

### 7. Draft split proposal
- Retain as-is; it is an exemplary small interface package (71 LOC).


## Component: hostpolicy (dir: go/internal/hostpolicy, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "hostpolicy_component"`
  - `interface = ":hostpolicy"` (`hostpolicy.go`)
  - `members = []`
  - `declared_authority = []`
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "hostpolicy"`
  - `interface_files: "hostpolicy.go"`
  - **Special Note on Membership**: In `component.textproto`, there is **no `members:` line**. Membership defaults to directory-based membership (`github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy`).
  - **Divergence**: None.
- **Files (non-test)**:
  - [`hostpolicy.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/hostpolicy.go) (107 LOC): Declares host-configurable extension hooks for import-path canonicalization and namespace validation (`CanonicalizePath`, `NamespaceID`, `IsCanonicalPath`, `ValidateCanonicalPath`, `ValidateStdlibPaths`).
  - **Total non-test LOC**: 107 LOC (< 300 LOC; sections 3 and 4 abbreviated).

### 2. Exported API inventory
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `CanonicalizePath` | var | [`hostpolicy.go:40`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/hostpolicy.go#L40) | `cmd/arcc/app`, `goanalysis`, `manifestparity`, `symbol` | tests | false |
| `NamespaceID` | var | [`hostpolicy.go:52`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/hostpolicy.go#L52) | `goanalysis` | tests | false |
| `IsCanonicalPath` | var | [`hostpolicy.go:72`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/hostpolicy.go#L72) | `goanalysis` | tests | false |
| `ValidateCanonicalPath` | func | [`hostpolicy.go:81`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/hostpolicy.go#L81) | `stdlibmap`, `symbol` | - | false |
| `ValidateStdlibPaths` | func | [`hostpolicy.go:100`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/hostpolicy.go#L100) | `stdlibmap` | - | false |

- **Summary**: Total exported symbols: **5**; count used externally (non-test): **5** (100%); count test-only: **0**; count internal-only: **0**.

### 3. Authority inventory (measured)
- Report `hostpolicy.report.json` passes (`violations: null`). 0 capability findings. Pure string validation hooks.

### 4. Internal structure and seams
- 107 LOC in 1 file (< 300 LOC).
- Contains package-level mutable variables: `CanonicalizePath`, `NamespaceID`, and `IsCanonicalPath`. These are intentional extension points allowing host environments (e.g. Google-internal module paths) to substitute custom canonicalization routines without modifying the core algorithms.

### 5. Consumers' view
- `cmd/arcc/app`: sets `CanonicalizePath`.
- `symbol`: uses `CanonicalizePath`, `ValidateCanonicalPath`.
- `stdlibmap`: uses `ValidateCanonicalPath`, `ValidateStdlibPaths`.
- `goanalysis`: uses `CanonicalizePath`, `NamespaceID`, `IsCanonicalPath`.
- `manifestparity`: uses `CanonicalizePath`.

### 6. Tests
- 2 external test files ([`hostpolicy_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/hostpolicy_test.go), [`namespace_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/namespace_test.go)), `package hostpolicy_test`.

### 7. Draft split proposal
- Retain as-is (107 LOC, 100% consumed).


## Component: schema (dir: go/internal/schema, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/schema`)

### 1. Current declaration
- **Bazel `go_component` attrs**:
  - `name = "schema_component"`
  - `interface_style = "PACKAGE_SURFACE"`
  - `component_deps = ["//go/internal/protobufruntime:protobuf-runtime"]`
  - `members = ["//go/internal/schema", "//go/internal/schema/gen"]`
  - `check_tags = ["manual"]` (tagged manual because of documented UNANALYZED findings in generated code)
  - `visibility = ["//go:__subpackages__"]`
- **`component.textproto`**:
  - `name: "schema"`
  - `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/schema"`
  - `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"`
  - `component_dependencies`: `protobuf-runtime`
  - **Divergence**: None.
- **Files (non-test)**:
  - [`capabilities.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/capabilities.go) (38 LOC): Handwritten package `schema` defining the single capability taxonomy map `KnownCapabilities`.
  - [`gen/component.pb.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/component.pb.go) (470 LOC): Generated protobuf bindings for component manifest schema (`Component`, `ComponentDependency`, `InterfaceStyle`, `AnalysisDefeatingPolicy`, `Authority`).
  - [`gen/stdlibmap.pb.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/stdlibmap.pb.go) (818 LOC): Generated protobuf bindings for stdlib authority map (`StdlibMap`, `StdlibPackage`, `SymbolCapability`, `Frame`, `SDKKey`).
  - [`gen/surface.pb.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/surface.pb.go) (338 LOC): Generated protobuf bindings for surface manifests (`SurfaceManifest`, `AuthorityDeclaration`).
  - **Total non-test LOC**: 38 LOC handwritten + 1,626 LOC generated protobuf = 1,664 LOC.

### 2. Exported API inventory
- **Package `schema`** ([`capabilities.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/capabilities.go)):
| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `KnownCapabilities` | var | [`capabilities.go:24`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/capabilities.go#L24) | `artifactio`, `manifest`, `stdlibmap` | - | false |

- **Package `schema/gen`** ([`gen/*.pb.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen)):
  - Declares **169** exported symbols (types, protobuf enum values, methods, and descriptor vars).
  - **61 used externally (non-test)**:
    - In `manifest`: `Component`, `ComponentDependency`, `InterfaceStyle`, `AnalysisDefeatingPolicy`, `Authority`, `Authority_DECLARED`, `Authority_UNKNOWN`.
    - In `artifactio`: `Component`, `SurfaceManifest`, `StdlibMap`, `AuthorityDeclaration`, `SymbolCapability`.
    - In `surface`: `SurfaceManifest`, `AuthorityDeclaration`, `SymbolCapability`.
    - In `goanalysis`: `SurfaceManifest`, `AuthorityDeclaration`.
    - In `stdlibmap`: `StdlibMap`, `StdlibPackage`, `SymbolCapability`, `Frame`.
    - In `cmd/arcc/app`: `SurfaceManifest`, `StdlibMap`.
  - **49 used by tests only**: Protobuf getters, resetters, and schema reflection descriptors.
  - **59 internal-only**: Unused protobuf getters (`GetFile`, `GetLine`, `GetFormatVersion`, `GetNamespace`, etc.) and descriptor byte slice vars.

### 3. Authority inventory (measured)
- **Measured report**: `schema.report.json` gives verdict **`fail`** due to **`UNDECLARED_AUTHORITY`**.
- **Root Cause & Findings**:
  - Exactly **15 sites** of `AnalysisDefeating` constructs in generated `.pb.go` files:
    1. [`gen/component.pb.go:420`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/component.pb.go#L420): `(sync.Once).Do`
    2. [`gen/component.pb.go:421`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/component.pb.go#L421): `unsafe.Slice`
    3. [`gen/component.pb.go:421`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/component.pb.go#L421): `unsafe.StringData`
    4. [`gen/component.pb.go:456`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/component.pb.go#L456): `unsafe.Slice`
    5. [`gen/component.pb.go:456`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/component.pb.go#L456): `unsafe.StringData`
    6. [`gen/stdlibmap.pb.go:761`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/stdlibmap.pb.go#L761): `(sync.Once).Do`
    7. [`gen/stdlibmap.pb.go:762`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/stdlibmap.pb.go#L762): `unsafe.Slice`
    8. [`gen/stdlibmap.pb.go:762`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/stdlibmap.pb.go#L762): `unsafe.StringData`
    9. [`gen/stdlibmap.pb.go:804`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/stdlibmap.pb.go#L804): `unsafe.Slice`
    10. [`gen/stdlibmap.pb.go:804`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/stdlibmap.pb.go#L804): `unsafe.StringData`
    11. [`gen/surface.pb.go:288`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/surface.pb.go#L288): `(sync.Once).Do`
    12. [`gen/surface.pb.go:289`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/surface.pb.go#L289): `unsafe.Slice`
    13. [`gen/surface.pb.go:289`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/surface.pb.go#L289): `unsafe.StringData`
    14. [`gen/surface.pb.go:325`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/surface.pb.go#L325): `unsafe.Slice`
    15. [`gen/surface.pb.go:325`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen/surface.pb.go#L325): `unsafe.StringData`
  - These are generated by standard `protoc-gen-go` for compressed descriptor initialization. This explains why `schema_component` is excluded from default test targets via `check_tags = ["manual"]`.

### 4. Internal structure and seams
- Handwritten code ([`capabilities.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/capabilities.go), 38 LOC) contains only `KnownCapabilities` (a static lookup map of 12 capability string keys).
- Generated code ([`gen/`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/gen), 1,626 LOC) is pure struct definitions and protobuf runtime hooks.

### 5. Consumers' view
- Users of `schema.KnownCapabilities`:
  - `go/internal/manifest` ([authority.go:105](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/authority.go#L105), [manifest.go:370](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go#L370))
  - `go/internal/artifactio` ([surface.go:143](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surface.go#L143), [stdlibmap.go:268, 364](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmap.go#L268))
  - `go/internal/stdlibmap` ([generate.go:541](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L541))
- Users of `gen.*`:
  - `manifest`, `artifactio`, `surface`, `goanalysis`, `stdlibmap`, and `cmd/arcc/app`.

### 6. Tests
- No tests inside `schema`. Tested by `archcontracts/schema_test.go` and `manifest/schema_test.go`.

### 7. Draft split proposal & component style assessment
- **Could `schema` become an interface-style component?**
  - **If `interface = //go/internal/schema/gen:gen` and `members = [//go/internal/schema]`**:
    `gen` would be the sole interface. Consumers could call `gen.*`, but any reference to `schema.KnownCapabilities` would be classified as `CALLS_UNDECLARED_INTERFACE` because `schema` is a member, not the interface!
  - **If `interface = //go/internal/schema` and `members = [//go/internal/schema/gen]`**:
    `schema` would be the sole interface. Consumers could access `KnownCapabilities`, but all direct imports of `gen.SurfaceManifest`, `gen.Component`, etc. would fail boundary checks.
  - **Could `gen` and `schema` be separate components?**
    Yes: `schema_gen` (asserted/package-surface component for generated code) and `schema_caps` (pure interface component for the capability taxonomy).
  - **Verdict on `PACKAGE_SURFACE`**:
    Because `gen` is 100% machine-generated code from `.proto` definitions containing unavoidable `unsafe.Slice` / `unsafe.StringData` constructs, **`schema` should remain `PACKAGE_SURFACE`** (or be split into an asserted `schema_proto` component and a pure `schema` taxonomy component). `PACKAGE_SURFACE` accurately models generated vocabulary packages where every generated type is intentionally public.
