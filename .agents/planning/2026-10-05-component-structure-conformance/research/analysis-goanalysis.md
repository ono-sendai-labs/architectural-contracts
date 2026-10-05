# Component Analysis: goanalysis

## Component: goanalysis (dir: go/internal/goanalysis, import path `github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis`)

### 1. Current declaration

- **Bazel `go_component` declaration** ([BUILD.bazel:46-68](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/BUILD.bazel#L46-L68)):
  - `name`: `"goanalysis_component"`
  - `interface`: `":goanalysis"` ([BUILD.bazel:10-35](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/BUILD.bazel#L10-L35)), containing all 6 `.go` files: `defeatscan.go`, `goanalysis.go`, `refscan.go`, `scanobserver.go`, `surface_resolver.go`, `surfaceinputs.go`.
  - `members`: omitted (the interface library `:goanalysis` is the sole member).
  - `interface_style`: omitted (defaults to `DECLARED`).
  - `declared_authority`: `[FILES, READ_SYSTEM_STATE, RUNTIME]`
  - `component_deps`: 11 components:
    - `//go/internal/artifactio:artifactio_component`
    - `//go/internal/facts:facts_component`
    - `//go/internal/manifest:manifest_component`
    - `//go/internal/schema:schema_component`
    - `//go/internal/stdlibauthority:stdlibauthority_component`
    - `//go/internal/surface:surface_component`
    - `//go/internal/symbol:symbol_component`
    - `//go/internal/xtools:x-tools`
    - `//go/internal/report:report_component`
    - `//go/internal/hostpolicy:hostpolicy_component`
    - `//go/internal/packagelayout:packagelayout_component`

- **Checked-in manifest** ([component.textproto:1-54](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/component.textproto#L1-L54)):
  - `name`: `"goanalysis"`
  - `interface_files`: `"defeatscan.go"`, `"goanalysis.go"`, `"surface_resolver.go"`, `"surfaceinputs.go"`
  - `members`: `"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"`
  - `component_dependencies`: same 11 components
  - `declared_authority`: `"FILES"`, `"READ_SYSTEM_STATE"`, `"RUNTIME"`

- **Divergence between Bazel and Manifest**:
  1. *Interface files divergence*: `component.textproto` lists 4 files as `interface_files` (`defeatscan.go`, `goanalysis.go`, `surface_resolver.go`, `surfaceinputs.go`), omitting `refscan.go` and `scanobserver.go`. In contrast, Bazel's `go_component` specifies `interface = ":goanalysis"`, whose `srcs` contains all 6 `.go` files. Under rules_arcc, all sources of the interface library become interface files. As a result, Bazel considers `refscan.go` an interface file (surfacing 29 symbols in `bazel-bin/.../goanalysis_component.surface.json`, including `ScanReferences`), whereas the textproto excludes it (surfacing 28 symbols in `component.surface.json`, omitting `ScanReferences`).
  2. *Members divergence*: `component.textproto` explicitly lists `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"`, whereas Bazel relies on implicit membership from `interface = ":goanalysis"`.

- **Source files (non-test)**:
| File | LOC | Purpose |
| :--- | :--- | :--- |
| [defeatscan.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/defeatscan.go) | 509 | Scans member packages for analysis-defeating constructs (DR-11: `//go:linkname` directives, assembly `.s` files, and cgo use). |
| [goanalysis.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go) | 1,549 | Main analysis driver (`LoadPackageFacts`), package facts construction, export data type completion, interface file validation, and symbol canonicalization/parsing. |
| [refscan.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/refscan.go) | 423 | AST and `types.Info` traversal to extract typed reference edges and import edges from declared member packages. |
| [scanobserver.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/scanobserver.go) | 67 | Package-private observation hook and event definitions enabling integration tests to observe scanner loop progress without modifying public APIs. |
| [surface_resolver.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go) | 890 | Resolves dependency surfaces in layout and native modes, validates artifact hashes and SDK compatibility, derives provenance and freshness, and decodes surfaces. |
| [surfaceinputs.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go) | 350 | Loads surface emission inputs (`SurfaceInputs`), extracts surviving interface ASTs and types, and forwards layout/platform identity helpers. |
| **Total non-test** | **3,788** | |

---

### 2. Exported API inventory

| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `ActiveDependencyArtifactBindings` | func | [surfaceinputs.go:333](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L333) | `go/cmd/arcc/app` | — | no |
| `ActivePlatformIdentity` | func | [surfaceinputs.go:296](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L296) | `go/cmd/arcc/app` | — | no |
| `CanonicalNamespace` | func | [surfaceinputs.go:348](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L348) | `go/cmd/arcc/app` | — | no |
| `DependencyArtifactBinding` | type | [surfaceinputs.go:41](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L41) | — (type alias) | — | yes |
| `DependencySurfaceLayout` | const | [surface_resolver.go:35](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L35) | `go/cmd/arcc/app` | — | no |
| `DependencySurfaceMode` | type | [surface_resolver.go:31](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L31) | — | — | yes |
| `DependencySurfaceModeLayout` | const | [surface_resolver.go:39](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L39) | — | — | yes |
| `DependencySurfaceModeNative` | const | [surface_resolver.go:38](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L38) | — | — | yes |
| `DependencySurfaceNative` | const | [surface_resolver.go:34](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L34) | `go/cmd/arcc/app` | — | no |
| `DependencySurfaceReadDir` | type | [surface_resolver.go:50](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L50) | — | — | yes |
| `DependencySurfaceReadFile` | type | [surface_resolver.go:45](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L45) | — | — | yes |
| `DependencySurfaceReadSources` | type | [surface_resolver.go:55](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L55) | — | — | yes |
| `DependencySurfaceRequest` | type | [surface_resolver.go:63](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L63) | `go/cmd/arcc/app` | `go/cmd/arcc/app` | no |
| `InterfaceFileExclusion` | type | [goanalysis.go:956](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L956) | `go/cmd/arcc/app` | `go/cmd/arcc/app` | no |
| `LayoutModeActive` | func | [surfaceinputs.go:326](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L326) | `go/cmd/arcc/app` | — | no |
| `LoadPackageFacts` | func | [goanalysis.go:107](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L107) | `go/cmd/arcc`, `go/cmd/arcc/app` | `go/cmd/arcc`, `go/cmd/arcc/app` | no |
| `LoadRequest` | type | [goanalysis.go:77](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L77) | `go/cmd/arcc/app` | `go/cmd/arcc`, `go/cmd/arcc/app` | no |
| `LoadSurfaceInputs` | func | [surfaceinputs.go:54](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L54) | `go/cmd/arcc/app` | — | no |
| `PlatformIdentity` | type | [surfaceinputs.go:279](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L279) | `go/cmd/arcc/app` | `go/cmd/arcc/app` | no |
| `ResolveDependencySurface` | func | [surface_resolver.go:103](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L103) | — | `go/cmd/arcc/app` | test-only |
| `ResolveDependencySurfaceForDependency` | func | [surface_resolver.go:177](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L177) | `go/cmd/arcc/app` | — | no |
| `ResolveValidatedDependencySurface` | func | [surface_resolver.go:251](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L251) | — | — | yes |
| `ScanAnalysisDefeats` | func | [defeatscan.go:37](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/defeatscan.go#L37) | — | `goanalysis_test` | yes |
| `ScanReferences` | func | [refscan.go:51](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/refscan.go#L51) | — | `goanalysis_test` | yes |
| `SerializeChecks` | func | [goanalysis.go:95](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L95) | `go/cmd/arcc/app` | — | no |
| `SurfaceInputs` | type | [surfaceinputs.go:24](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L24) | `go/cmd/arcc/app` | `go/cmd/arcc/app` | no |
| `SurfaceResolutionRequest` | type | [surface_resolver.go:92](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L92) | — | — | yes |
| `ValidateInterfaceFiles` | func | [goanalysis.go:965](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L965) | `go/cmd/arcc/app` | — | no |
| `WithDriverEnv` | func | [goanalysis.go:88](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L88) | `go/cmd/arcc/app` | — | no |

- **Summary counts**:
  - Total exported symbols: **29**
  - Count used externally (non-test): **17**
  - Count test-only: **1** (`ResolveDependencySurface`, used in `go/cmd/arcc/app/surface_cutover_test.go:372`)
  - Count internal-only: **11** (candidates for unexporting or moving to internal member packages: `ScanAnalysisDefeats`, `ScanReferences`, `ResolveValidatedDependencySurface`, `SurfaceResolutionRequest`, `DependencyArtifactBinding`, `DependencySurfaceMode`, `DependencySurfaceModeLayout`, `DependencySurfaceModeNative`, `DependencySurfaceReadDir`, `DependencySurfaceReadFile`, `DependencySurfaceReadSources`).

---

### 3. Authority inventory (measured)

- **Measurement**:
  Measured via `timeout 900 bazel build //go/internal/goanalysis:goanalysis_component --output_groups=+arcc` producing [goanalysis_component.report.json](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel-bin/go/internal/goanalysis/goanalysis_component.report.json):
  - Verdict: **pass**
  - Violations: `null`
  - Diagnostics: `non_member_export_artifact_count: 196`, `non_member_export_bytes: 63967918`
  - Warnings:
    - `UNUSED_DEPENDENCY` on `report_component` (declared component dependency `report` is unused by member code; no imports of `internal/report` exist).
    - `DEPENDENCY_CHECK_FAILED` on `schema_component` (known in-tree fixture/conformance state).

- **Authority Call Sites Table**:
| capability | stdlib/third-party symbol referenced | member symbol (func/method) that references it | file:line |
| :--- | :--- | :--- | :--- |
| `FILES` | `os.Open` | `completeLoadedExportTypes` | [goanalysis.go:658](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L658) |
| `FILES` | `os.Open` | `validateExportProducerVersion` | [goanalysis.go:689](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L689) |
| `FILES` | `os.Open` | `leadingBuildConstraint` | [goanalysis.go:1063](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L1063) |
| `FILES` | `os.ReadDir` | `deriveStatuses` | [surface_resolver.go:725](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L725) |
| `FILES` | `os.ReadFile` | `ResolveDependencySurface` | [surface_resolver.go:110](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L110) |
| `FILES` | `os.ReadFile` | `scanBypassSourceFile` | [defeatscan.go:228](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/defeatscan.go#L228) |
| `FILES` | `os.ReadFile` | `deriveStatuses` | [surface_resolver.go:714](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L714) |
| `FILES` | `os.Stat` | `exportDataStat` (package var) | [goanalysis.go:42](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L42) |
| `FILES` | `os.Stat` | `ValidateInterfaceFiles` | [goanalysis.go:1003](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L1003) |
| `READ_SYSTEM_STATE` | `path/filepath.Abs` | `collectExportDataDiagnostics` | [goanalysis.go:557](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L557) |

- **Functional Clusters**:
  1. **Interface File System Inspection** ([goanalysis.go:955-1145](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L955-L1145)):
     - *LOC estimate*: ~100 LOC
     - *Capabilities*: `FILES` (`os.Stat` to check existence/mode; `os.Open` to parse leading `//go:build` constraints).
     - *Leaf status*: Leaf. It inspects local path metadata and reads file headers.
     - *Minimal API to isolate*: `type FileInspector interface { Stat(path string) (fs.FileInfo, error); ReadLeadingLines(path string, maxLines int) ([]string, error) }`.
  2. **Export Data Archive Inspection** ([goanalysis.go:521-753](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L521-L753)):
     - *LOC estimate*: ~230 LOC
     - *Capabilities*: `FILES` (`os.Open`, `exportDataStat`), `READ_SYSTEM_STATE` (`filepath.Abs` calling `os.Getwd()`).
     - *Leaf status*: Leaf operation over `.a` files on disk.
     - *Minimal API to isolate*: `ReadExportTypes(exportFile string, fset *token.FileSet, imports map[string]*types.Package) (*types.Package, error)`.
  3. **Defeat / Bypass Source File Reading** ([defeatscan.go:220-250](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/defeatscan.go#L220-L250)):
     - *LOC estimate*: ~40 LOC
     - *Capabilities*: `FILES` (`os.ReadFile`).
     - *Leaf status*: Leaf byte reading fallback for unparsed/ignored Go files.
     - *Minimal API to isolate*: Pass a `ReadFile` reader or pre-read byte provider.
  4. **Dependency Surface Artifact and Source Reading** ([surface_resolver.go:103-125, 710-745](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L103-L125)):
     - *LOC estimate*: ~60 LOC
     - *Capabilities*: `FILES` (`os.ReadFile`, `os.ReadDir`).
     - *Leaf status*: Leaf. Note that `ResolveDependencySurface` already defines `req.ReadFile` and `req.ReadDir` func signatures; it only calls `os.ReadFile` and `os.ReadDir` as fallback defaults when those request fields are nil.
     - *Minimal API to isolate*: Make `req.ReadFile` and `req.ReadDir` mandatory or isolate default fallback into an ambient host adapter.

- **Root Cause Analysis of Declared Capabilities**:
  - `FILES`: Directly caused by `os.Open` (3 sites), `os.ReadDir` (1 site), `os.ReadFile` (3 sites), and `os.Stat` (2 sites).
  - `READ_SYSTEM_STATE`: Directly caused by the single call to `path/filepath.Abs` at [goanalysis.go:557](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L557). In Go's stdlib, `filepath.Abs` invokes `os.Getwd()`, which queries process state and is categorized under `READ_SYSTEM_STATE`.
  - `RUNTIME`: **NOT EXERCISED**. While declared in both `BUILD.bazel` and `component.textproto`, `goanalysis` only references `runtime.Version()` at [goanalysis.go:730](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L730). In the stdlib map, `runtime.Version` is classified as `SAFE` (provenance `capslock-curated`). The `RUNTIME` capability declaration is completely redundant and unexercised.
  - `REFLECT`: **NOT CHARGED**. `goanalysis.go` imports `reflect` and calls `reflect.ValueOf(res.Packages).Pointer()` at lines 344 and 968 to obtain a slice pointer address. In the stdlib authority map, `reflect.ValueOf` and `(reflect.Value).Pointer` are classified as `SAFE` (`capslock-curated`). Thus, no `REFLECT` capability is attributed, and none was declared.
  - `go/build`: Only the struct type `build.Context` is referenced as a type declaration in function signatures ([goanalysis.go:1044, 1146](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L1044)); `build.Default` is never referenced. `build.Context` carries no capabilities.
  - `io/fs` and `os.IsNotExist`: `io/fs.DirEntry`, `io/fs.ErrNotExist`, and `os.IsNotExist` are all classified as `SAFE`.

---

### 4. Internal structure and seams

- **Cohesive Clusters**:
  - **Cluster 1: Analysis Driver & Package Facts Loader** ([goanalysis.go:1-520, 754-823](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go)): `LoadPackageFacts`, `LoadRequest`, `WithDriverEnv`, `SerializeChecks`, graph validation helpers. Uses `x/tools/go/packages`.
  - **Cluster 2: Export Data Completion** ([goanalysis.go:521-753](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L521-L753)): `completeLoadedExportTypes`, `collectExportDataDiagnostics`, `validateExportProducerVersion`. Uses `x/tools/go/gcexportdata`.
  - **Cluster 3: Interface File Validation** ([goanalysis.go:955-1175](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L955-L1175)): `ValidateInterfaceFiles`, `InterfaceFileExclusion`, `leadingBuildConstraint`, `describeInterfaceFileConstraintWithContext`.
  - **Cluster 4: Symbol Canonicalization & Type Expression Parser** ([goanalysis.go:1176-1549](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L1176-L1549)): `canonicalizeSymbol`, `rewriteSymbolTypeExpression`, `symbolTypeParser`. Pure syntax manipulation.
  - **Cluster 5: Defeat / Bypass Scanner** ([defeatscan.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/defeatscan.go)): `ScanAnalysisDefeats`, `scanBypassPackage`, `scanBypassSourceFile`, `scanBypassSyntax`.
  - **Cluster 6: Reference Scanner** ([refscan.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/refscan.go)): `ScanReferences`, `scanPackage`, `memberSourceSite`.
  - **Cluster 7: Scan Observer Hook** ([scanobserver.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/scanobserver.go)): `scanObserver`, `scanObserverEvent`, `notifyScanObserver`.
  - **Cluster 8: Surface Inputs & Platform Helpers** ([surfaceinputs.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go)): `LoadSurfaceInputs`, `SurfaceInputs`, `PlatformIdentity`, `ActivePlatformIdentity`, `LayoutModeActive`, `ActiveDependencyArtifactBindings`, `CanonicalNamespace`.
  - **Cluster 9: Dependency Surface Resolver** ([surface_resolver.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go)): `ResolveDependencySurface`, `ResolveDependencySurfaceForDependency`, `ResolveValidatedDependencySurface`, `DependencySurfaceRequest`.

- **Cross-File Unexported Dependencies**:
| Source File | Target File | Shared Unexported Symbols |
| :--- | :--- | :--- |
| `defeatscan.go` | `refscan.go` | `memberSiteRoot` (func) |
| `defeatscan.go` | `scanobserver.go` | `notifyScanObserver`, `scanObservationObserver`, `scanObserver`, `scanObserverEvent`, and 16 observation constants |
| `refscan.go` | `scanobserver.go` | `notifyScanObserver`, `scanObservationObserver`, `scanObserver`, `scanObserverEvent`, and 12 observation constants |
| `surfaceinputs.go` | `goanalysis.go` | `loadPackages`, `componentLoadMode`, `completeLoadedExportTypes`, `hasExportBackedNonMembers`, `describePackageLoadError`, `interfacePackagePatterns`, `isExpectedUnresolvedLayoutImport`, `layoutRequestMembers`, `packageContainsInterfaceFile`, `packageIsDeclaredMember`, `validateDeclaredPackages`, `validateLayoutMembership`, `validateLoadedPackageGraph`, `validateLoaderPackagePaths` |
| `surface_resolver.go` | *(none)* | **0 symbols** (completely self-contained from other files in `goanalysis`) |
| `goanalysis.go` | *(none)* | Calls `ScanAnalysisDefeats` and `ScanReferences` which are currently exported; 0 unexported symbols called from other files. |

- **Cross-File Cluster Reference Graph**:
```
+-------------------------------------------------------+
| surfaceinputs.go                                      |
|   (loads ASTs & type info for surface emission)       |
+--------------------------+----------------------------+
                           | calls 14 unexported loader
                           | & validation functions
                           v
+-------------------------------------------------------+
| goanalysis.go                                         |
|   - Driver (LoadPackageFacts)                         |
|   - Export data completion                            |
|   - Symbol canonicalization parser                    |
|   - Interface file validator                          |
+------------+-----------------------------+------------+
             | calls ScanReferences        | calls ScanAnalysisDefeats
             v                             v
+---------------------------+  +------------------------+
| refscan.go                |  | defeatscan.go          |
|   (reference edges)       |  |   (linkname/asm/cgo)   |
+-------------+-------------+  +----+-------------+-----+
              |                     |             |
              | calls memberSiteRoot|             |
              | <-------------------+             |
              |                                   |
              | calls notifyScanObserver          | calls notifyScanObserver
              v                                   v
+-------------------------------------------------------+
| scanobserver.go (package observer hook)               |
+-------------------------------------------------------+

+-------------------------------------------------------+
| surface_resolver.go (ISOLATED / DECOUPLED)            |
|   (resolves dependency surface manifests & artifacts) |
+-------------------------------------------------------+
```

- **Pure Computation vs Environment Interaction**:
  - *Pure computation*:
    - Symbol canonicalization & type expression parser ([goanalysis.go:1176-1549](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L1176-L1549))
    - Typed reference and import edge extraction ([refscan.go:51-370](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/refscan.go#L51-L370))
    - AST directive, comment, and token lexing ([defeatscan.go:250-470](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/defeatscan.go#L250-L470))
    - Decoded surface schema validation & status derivation ([surface_resolver.go:251-370, 640-710](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver.go#L251-L370))
  - *Environment interaction*:
    - Package loading via `x/tools/go/packages.Load` (external process invocation and toolchain probing)
    - Export data archive reading via `os.Open`, `os.Stat`, and `gcexportdata.Read`
    - Source file reading via `os.ReadFile` in `defeatscan.go` and `surface_resolver.go`
    - Interface file validation via `os.Stat` and `os.Open` in `goanalysis.go`
    - Directory enumeration via `os.ReadDir` in `surface_resolver.go`
    - Working directory query via `filepath.Abs` in `goanalysis.go`

- **Package-Level Mutable State, `init()`, and Build Tags**:
  - `checkMu sync.Mutex` ([goanalysis.go:85](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L85)): Serializes execution in `SerializeChecks` and `WithDriverEnv`.
  - `membershipMu sync.RWMutex` ([goanalysis.go:38](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L38)): Guards `loadedFiles`.
  - `loadedFiles = make(map[uintptr]map[string]bool)` ([goanalysis.go:39](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L39)): Caches the set of loaded package files using a slice header pointer key (`reflect.ValueOf(packages).Pointer()`), populated during `LoadPackageFacts` and retrieved during `ValidateInterfaceFiles`.
  - `exportDataStat = os.Stat` ([goanalysis.go:42](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L42)): Mutable test hook for sizing export files.
  - `loadPackages = packages.Load` ([goanalysis.go:46](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L46)): Mutable test hook for mocking package loader.
  - `phaseObserver analysisPhaseObserver` ([goanalysis.go:50](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L50)): Mutable test hook for loader phase timings.
  - `scanObserver scanObservationObserver` ([scanobserver.go:60](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/scanobserver.go#L60)): Mutable test hook for scanner loop observations.
  - `init()` functions: **None** in non-test files.
  - Build tags: **None** in non-test files.

- **Role of Third-Party and Boundary Dependencies**:
  - `golang.org/x/tools/go/packages`: Primary Go package loader. Invoked via `packages.Load` with config flags (`NeedCompiledGoFiles`, `NeedFiles`, `NeedName`, `NeedSyntax`, `NeedImports`, `NeedTypes`, `NeedTypesInfo`, `NeedModule`, `NeedExportFile`). Handled across the `//go/internal/xtools:x-tools` package-surface boundary.
  - `golang.org/x/tools/go/gcexportdata`: Used by `completeLoadedExportTypes` to read compiled export archives (`.a`) when non-member packages in layout mode lack full syntax. Also sits behind the `x-tools` boundary.
  - `packagelayout`: Mediates the Bazel layout driver mode. Provides `IsLayoutMode()`, `GetActiveLayout()`, `GetActiveWorkspaceDir()`, `BuildContextForLayout()`, and `DependencyArtifactBinding`.

---

### 5. Consumers' view

- **Consumer 1: `go/cmd/arcc/app`** (CLI application logic)
  - Imported at [go/cmd/arcc/app/app.go:18](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L18) and [go/cmd/arcc/app/check.go:18](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L18).
  - Used symbols:
    - `LoadPackageFacts` ([check.go:506](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L506))
    - `LoadRequest` ([app.go:24](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L24), [check.go:55, 219, 399](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L55))
    - `ValidateInterfaceFiles` ([check.go:231](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L231))
    - `InterfaceFileExclusion` ([check.go:389, 480](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L389))
    - `LoadSurfaceInputs` ([check.go:520](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L520))
    - `SurfaceInputs` ([check.go:55](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L55))
    - `ResolveDependencySurfaceForDependency` ([check.go:374](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L374))
    - `DependencySurfaceRequest` ([app.go:30](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L30), [check.go:354](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L354))
    - `DependencySurfaceNative` ([check.go:342](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L342))
    - `DependencySurfaceLayout` ([check.go:340](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L340))
    - `ActivePlatformIdentity` ([check.go:204](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L204))
    - `PlatformIdentity` ([check.go:37, 204, 644](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L37))
    - `LayoutModeActive` ([check.go:210](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L210))
    - `ActiveDependencyArtifactBindings` ([check.go:200](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L200))
    - `CanonicalNamespace` ([check.go:622](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L622))
    - `SerializeChecks` ([app.go:52](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L52))
    - `WithDriverEnv` ([app.go:126](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L126))

- **Consumer 2: `go/cmd/arcc`** (`main.go`)
  - Imported at [go/cmd/arcc/main.go:19](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go#L19).
  - Used symbols:
    - `LoadPackageFacts` ([main.go:28](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go#L28)) (used to register the default loader implementation on `app.CheckOptions`).

- **Consumer 3: `go/internal/stdlibmap`**
  - **None**. `stdlibmap` does not import `goanalysis`.

- **Consumer 4: External Tests**:
  - `go/cmd/arcc`: `layout_integration_test.go` uses `LoadPackageFacts`, `LoadRequest`.
  - `go/cmd/arcc/app`: `app_test.go`, `check_test.go`, `check_integration_test.go`, `check_internal_test.go`, `surface_cutover_test.go` use `LoadPackageFacts`, `LoadRequest`, `SurfaceInputs`, `InterfaceFileExclusion`, `PlatformIdentity`, `DependencySurfaceRequest`, and `ResolveDependencySurface`.

---

### 6. Tests

- **Test Files Breakdown** (18 test files, 8,926 test LOC):
  - **External Test Files (`package goanalysis_test`)** — 7 files (test only exported API):
    1. [defeat_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/defeat_test.go) (125 LOC): Tests `ScanAnalysisDefeats` on bypass fixture.
    2. [fixture_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/fixture_test.go) (716 LOC): Helper harness providing shared loader setup for `refscan_test.go` and `defeat_test.go`.
    3. [goanalysis_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis_test.go) (767 LOC): Tests `LoadPackageFacts`, `ValidateInterfaceFiles`.
    4. [member_only_integration_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/member_only_integration_test.go) (538 LOC): Tests member-only load behaviors.
    5. [refscan_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/refscan_test.go) (426 LOC): Tests `ScanReferences` on various language features.
    6. [surface_resolver_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver_test.go) (830 LOC): Tests `ResolveDependencySurface`.
    7. [surfaceinputs_integration_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs_integration_test.go) (94 LOC): Tests `LoadSurfaceInputs`.

  - **Internal Test Files (`package goanalysis`)** — 11 files (access unexported identifiers):
    1. [canonicalization_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/canonicalization_test.go) (375 LOC): Uses unexported `canonicalizeSymbol`, `loadPackages`, `validateLayoutMembership`, `validateLoaderPackagePaths`.
    2. [export_diagnostics_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/export_diagnostics_test.go) (121 LOC): Uses unexported `collectExportDataDiagnostics`, `exportDataStat`.
    3. [full_build_determinism_integration_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/full_build_determinism_integration_test.go) (480 LOC): End-to-end determinism checks.
    4. [hermeticity_integration_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/hermeticity_integration_test.go) (1,329 LOC): End-to-end hermeticity tests.
    5. [member_analysis_scaling_fixture_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/member_analysis_scaling_fixture_test.go) (639 LOC): Uses `analysisPhase*`, `loadPackages`, `phaseObserver`, `scanObserver*`.
    6. [member_analysis_scaling_integration_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/member_analysis_scaling_integration_test.go) (249 LOC): Uses `analysisPhase*`.
    7. [member_only_load_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/member_only_load_test.go) (221 LOC): Uses `loadPackages`, `validateLoadedPackageGraph`.
    8. [native_member_only_integration_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/native_member_only_integration_test.go) (59 LOC): Uses `componentLoadMode`.
    9. [producer_chain_scaling_integration_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/producer_chain_scaling_integration_test.go) (1,372 LOC): Uses `analysisPhase*`, `loadPackages`, `phaseObserver`, `scanObserver*`.
    10. [scanobserver_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/scanobserver_test.go) (457 LOC): Uses 26 unexported symbols in `scanobserver.go`.
    11. [surface_resolver_internal_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surface_resolver_internal_test.go) (128 LOC): Uses `loadPackages`.

- **Testdata Directories**:
  - `go/internal/goanalysis/testdata/`:
    - `success/`: Go packages `a` and `a/b` with types, init funcs, and non-Go files for basic load testing.
    - `refscan/`: Fixtures for reference scanning (`dep`, `member/kinds`, `member/concrete`, `member/dispatch`, `member/builtins`). Contains nested component manifests:
      - `testdata/refscan/dep/component.textproto`
      - `testdata/refscan/dep/pkg_surface.textproto`
    - `byp/`: Fixtures for bypass detection (`cgo.go`, `link.go`, `stub.s`, `stub_plan9.s`).
    - `broken_syntax/`: Contains `broken.go` for syntax error handling.
    - `classify/`: Fixtures for reference edge classification (`member/uses`, `member/rows`, `member/globals`, `infra/infra`).
    - `dep_resolve/`: Fixtures for dependency surface resolution testing with 7 nested textproto files:
      - `testdata/dep_resolve/dep/component.textproto`
      - `testdata/dep_resolve/dep/pkg_surface.textproto`
      - `testdata/dep_resolve/dep/pkg_surface_nocert.textproto`
      - `testdata/dep_resolve/dep/escaping_interface.textproto`
      - `testdata/dep_resolve/dep/missing_interface.textproto`
      - `testdata/dep_resolve/dep/malformed.textproto`
      - `testdata/dep_resolve/dep/declared_with_members.textproto`
      - `testdata/dep_resolve/declaring/component.textproto`
    - `unsafe/`: Fixtures for `unsafe` package references (`member/unsafe.go`).

---

### 7. Draft split proposal

#### 1. Proposed Target Structure

```
go/internal/goanalysis/                 (Root component: authority-free coordinator)
├── goanalysis.go                       (Small interface: LoadPackageFacts, ValidateInterfaceFiles, SurfaceInputs)
├── BUILD.bazel                         (go_component: declared_authority = [])
├── component.textproto
├── internal/
│   ├── refscan/                        (Member package: AST/Type reference & defeat scanner)
│   │   ├── refscan.go                  (ScanReferences)
│   │   ├── defeatscan.go               (ScanAnalysisDefeats)
│   │   └── scanobserver.go             (Observer hook & events)
│   ├── canonicalize/                   (Member package: Pure type expression parser & symbol canonicalizer)
│   │   └── canonicalize.go             (canonicalizeSymbol, symbolTypeParser)
│   ├── surfaceresolve/                 (Sub-component: Dependency surface resolution)
│   │   ├── surface_resolver.go         (ResolveDependencySurface, ResolveDependencySurfaceForDependency)
│   │   └── BUILD.bazel                 (declared_authority = [FILES])
│   ├── exportdata/                     (Sub-component: Export data archive reader)
│   │   ├── exportdata.go               (completeLoadedExportTypes, collectExportDataDiagnostics)
│   │   └── BUILD.bazel                 (declared_authority = [FILES, READ_SYSTEM_STATE])
│   └── fileprobe/                      (Sub-component: Interface file & build constraint reader)
│       ├── fileprobe.go                (ValidateInterfaceFiles, leadingBuildConstraint)
│       └── BUILD.bazel                 (declared_authority = [FILES])
```

#### 2. Component Authority and API Allocation

1. **`goanalysis` (Parent Component)**
   - **Declared Authority**: `[]` (Authority-free!).
   - **Interface API**:
     ```go
     type LoadRequest struct {
         ComponentName  string
         ComponentRoot  string
         Members        []string
         InterfaceFiles []string
     }
     func LoadPackageFacts(req LoadRequest) (facts.PackageFacts, error)
     func ValidateInterfaceFiles(componentRoot string, interfaceFiles []string, loaded facts.PackageFacts) ([]InterfaceFileExclusion, error)
     func LoadSurfaceInputs(req LoadRequest) (SurfaceInputs, error)
     type SurfaceInputs struct {
         InterfaceFiles []*ast.File
         InterfaceInfo  *types.Info
         MemberPackages []string
         SourcePaths    []string
     }
     type InterfaceFileExclusion struct {
         File       string
         Constraint string
     }
     func SerializeChecks(fn func())
     func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error
     ```
   - **Elimination of Global State & Reflection**:
     Remove the global map `loadedFiles = make(map[uintptr]map[string]bool)` and `reflect.ValueOf(res.Packages).Pointer()`. Store the loaded file set directly on `facts.PackageFacts` (or pass a loader context struct), making `ValidateInterfaceFiles` purely functional and removing all reflection and package-level mutex locking.
   - **Removal of Forwarding Shims**:
     Directly import `packagelayout` and `hostpolicy` in `go/cmd/arcc/app`, eliminating the redundant forwarding functions `ActivePlatformIdentity`, `LayoutModeActive`, `ActiveDependencyArtifactBindings`, and `CanonicalNamespace` from `goanalysis`.

2. **Sub-Component `surfaceresolve` (`go/internal/goanalysis/internal/surfaceresolve`)**
   - **Declared Authority**: `[FILES]`
   - **Rationale**: Isolates `os.ReadFile` and `os.ReadDir` during surface artifact reading and native directory inspection. Already 100% decoupled from the rest of `goanalysis`.
   - **API**:
     ```go
     func ResolveDependencySurfaceForDependency(req DependencySurfaceRequest) (facts.DependencySurface, error)
     func ResolveDependencySurface(req DependencySurfaceRequest) (facts.DependencySurface, error)
     ```

3. **Sub-Component `exportdata` (`go/internal/goanalysis/internal/exportdata`)**
   - **Declared Authority**: `[FILES, READ_SYSTEM_STATE]`
   - **Rationale**: Isolates `os.Open` (reading `.a` archives), `os.Stat` (checking archive sizes), and `filepath.Abs` (evaluating paths against workspace root, which invokes `os.Getwd()`).
   - **API**:
     ```go
     func CompleteLoadedExportTypes(pkgs []*packages.Package, memberPaths map[string]bool) error
     func CollectExportDataDiagnostics(pkgs []*packages.Package, memberPaths map[string]bool, baseDir string) (facts.ExportDataDiagnostics, error)
     ```

4. **Sub-Component `fileprobe` (`go/internal/goanalysis/internal/fileprobe`)**
   - **Declared Authority**: `[FILES]`
   - **Rationale**: Isolates `os.Stat` (checking that interface files exist and are regular files) and `os.Open` (parsing leading `//go:build` constraints).
   - **API**:
     ```go
     func CheckInterfaceFile(componentRoot string, path string, bctx build.Context) (excluded bool, constraint string, err error)
     ```

#### 3. Risks and Mitigations

- **Risk: Breaking Internal Tests**:
  11 test files in `goanalysis` currently import unexported symbols (e.g. `scanobserver_test.go` accesses 26 unexported symbols in `scanobserver.go`).
  *Mitigation*: When moving `scanobserver.go`, `refscan.go`, and `defeatscan.go` into `go/internal/goanalysis/internal/refscan`, move their corresponding tests (`scanobserver_test.go`, `refscan_test.go`, `defeat_test.go`) into `refscan` as well.
- **Risk: `RUNTIME` Capability Drop**:
  Dropping `RUNTIME` from `goanalysis`'s declared authority is completely safe because `runtime.Version()` is `SAFE` in the stdlib map and no other `runtime` symbols are called.
- **Risk: Removal of CLI Forwarding Wrappers**:
  Removing `CanonicalNamespace`, `LayoutModeActive`, `ActivePlatformIdentity`, etc., touches `go/cmd/arcc/app`.
  *Mitigation*: The CLI `cmd/arcc/app` already imports `hostpolicy` and `packagelayout` in parts of its implementation; moving those call sites directly to those packages simplifies the boundary cleanly.
