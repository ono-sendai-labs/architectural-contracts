## Component: packagelayout  (dir: go/internal/packagelayout, import path github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout)

### 1. Current declaration
- **Bazel `go_component` attrs (`//go/internal/packagelayout:packagelayout_component` in [go/internal/packagelayout/BUILD.bazel:31-46](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/BUILD.bazel#L31-L46)):**
  - `interface_style`: `"PACKAGE_SURFACE"`
  - `interface`: None (package-surface boundary)
  - `members`: `["//go/internal/packagelayout"]`
  - `component_deps`: `["//go/internal/xtools:x-tools"]`
  - `declared_authority`: `["FILES", "READ_SYSTEM_STATE", "OPERATING_SYSTEM", "MODIFY_SYSTEM_STATE", "REFLECT", "RUNTIME", "UNSAFE_POINTER"]`
  - `visibility`: `["//go:__subpackages__"]`
- **Checked-in manifest (`component.textproto` in [go/internal/packagelayout/component.textproto:1-18](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/component.textproto#L1-L18)):**
  - `name`: `"packagelayout"`
  - `interface_style`: `INTERFACE_STYLE_PACKAGE_SURFACE`
  - `members`: `"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"`
  - `component_dependencies`: `[name: "x-tools", manifest: "../xtools/component.textproto"]`
  - `declared_authority`: `["FILES", "READ_SYSTEM_STATE", "OPERATING_SYSTEM", "MODIFY_SYSTEM_STATE", "REFLECT", "RUNTIME", "UNSAFE_POINTER"]`
- **Divergence:** None. Both files define a `PACKAGE_SURFACE` boundary with identical members, dependencies, and declared authorities.
- **Files (non-test):**
  - [cgo_disabled.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/cgo_disabled.go): 5 LOC. Defines `nativeCgoEnabled = false` under `//go:build !cgo`.
  - [cgo_enabled.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/cgo_enabled.go): 5 LOC. Defines `nativeCgoEnabled = true` under `//go:build cgo`.
  - [shape_imports.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/shape_imports.go): 35 LOC. Pure projection helper `ShapeImports` mapping package import tables to canonical import-to-package ID strings.
  - [ordinary_import_data.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go): 587 LOC. Types and file routines for reading, validating, merging, and writing ordinary non-member import metadata descriptors.
  - [stdlib_export_data.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go): 498 LOC. Types and stream decoder for target-configured standard library export metadata and runfiles/execroot path mapping.
  - [packagelayout.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go): 2853 LOC. Core package layout schema, JSON codecs, build constraint evaluation, stdlib discovery from SDK tree, layout validation (source-backed and member-only), GOPACKAGESDRIVER protocol handler, driver execution, active layout state, and process environment lifecycle (`WithDriverEnv`).
  - **Total non-test LOC:** 3983 LOC (~4000 LOC).

---

### 2. Exported API inventory

| symbol | kind | file:line | used by (outside the package, non-test) | used by tests only | internal-only |
|---|---|---|---|---|---|
| `OrdinaryImportData` | type | [ordinary_import_data.go:30](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L30) | - | `go/cmd/arcc/app` | no |
| `ImportGraphRequest` | type | [ordinary_import_data.go:38](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L38) | - | - | yes |
| `ImportGraphPackageInput` | type | [ordinary_import_data.go:45](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L45) | - | - | yes |
| `ImportGraph` | type | [ordinary_import_data.go:54](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L54) | - | - | yes |
| `ImportGraphPackage` | type | [ordinary_import_data.go:62](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L62) | - | - | yes |
| `ResolveOrdinaryImportData` | func | [ordinary_import_data.go:144](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L144) | - | - | yes |
| `MergeImportGraphLayout` | func | [ordinary_import_data.go:309](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L309) | `go/cmd/arcc/app` | - | no |
| `WriteImportGraph` | func | [ordinary_import_data.go:469](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L469) | `go/cmd/arcc/app` | - | no |
| `Layout` | type | [packagelayout.go:86](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L86) | `go/cmd/arcc/app`, `go/internal/goanalysis`, `go/internal/stdlibmap` | - | no |
| `IsMemberOnlyLayout` | func | [packagelayout.go:134](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L134) | `go/internal/goanalysis` | - | no |
| `DependencyArtifactProvenance` | type | [packagelayout.go:153](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L153) | - (type used via field) | - | yes |
| `DependencyArtifactProvenanceChecked` | const | [packagelayout.go:156](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L156) | `go/internal/goanalysis` | `go/cmd/arcc/app` | no |
| `DependencyArtifactProvenanceAsserted` | const | [packagelayout.go:157](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L157) | `go/internal/goanalysis` | - | no |
| `DependencyArtifactBinding` | type | [packagelayout.go:165](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L165) | `go/cmd/arcc/app`, `go/internal/goanalysis` | - | no |
| `UnresolvedImport` | type | [packagelayout.go:175](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L175) | - (accessed as `[]UnresolvedImport`) | - | yes |
| `Platform` | type | [packagelayout.go:188](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L188) | `go/cmd/arcc/app`, `go/internal/stdlibmap` | - | no |
| `(Layout).IsStdlibPackage` | method | [packagelayout.go:215](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L215) | `go/cmd/arcc/app`, `go/internal/checker` | `go/internal/artifactio`, `go/internal/goanalysis`, `go/internal/teststdlibmap` | no |
| `BuildContextForLayout` | func | [packagelayout.go:303](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L303) | `go/internal/goanalysis` | - | no |
| `Parse` | func | [packagelayout.go:1043](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1043) | `go/cmd/arcc/app` | `bazel_rules/go/tests` | no |
| `(Layout).MarshalJSON` | method | [packagelayout.go:1158](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1158) | `go/cmd/arcc/app` (via `json.MarshalIndent`) | - | no |
| `(Layout).UnmarshalJSON` | method | [packagelayout.go:1247](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1247) | - (via `json.Unmarshal` in `Parse`) | - | yes |
| `ValidateAndResolve` | func | [packagelayout.go:1410](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1410) | - | - | yes |
| `ValidateAndResolveForMemberOnly` | func | [packagelayout.go:1737](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1737) | - | - | yes |
| `FileMatchesBuildConstraints` | func | [packagelayout.go:2004](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2004) | - | - | yes |
| `FileMatchesBuildConstraintsWithContext` | func | [packagelayout.go:2123](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2123) | `go/internal/goanalysis` | - | no |
| `SurvivingSourceFiles` | func | [packagelayout.go:2151](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2151) | - | - | yes |
| `HandleDriverRequest` | func | [packagelayout.go:2429](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2429) | - | - | yes |
| `StdlibLayout` | func | [packagelayout.go:2521](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2521) | `go/internal/stdlibmap` | - | no |
| `RunDriver` | func | [packagelayout.go:2593](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2593) | - | - | yes |
| `CheckMu` | var | [packagelayout.go:2646](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2646) | `go/internal/goanalysis` | - | no |
| `IsLayoutMode` | func | [packagelayout.go:2650](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2650) | `go/internal/goanalysis` | `go/cmd/arcc` | no |
| `GetActiveLayout` | func | [packagelayout.go:2657](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2657) | `go/internal/goanalysis` | - | no |
| `GetActiveLayoutPath` | func | [packagelayout.go:2664](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2664) | `go/internal/goanalysis` | - | no |
| `GetActiveWorkspaceDir` | func | [packagelayout.go:2671](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2671) | `go/internal/goanalysis` | - | no |
| `WithTemporaryLayout` | func | [packagelayout.go:2679](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2679) | - | - | yes |
| `WithDriverEnv` | func | [packagelayout.go:2741](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2741) | `go/internal/goanalysis`, `go/internal/stdlibmap` | `go/cmd/arcc` | no |
| `ShapeImports` | func | [shape_imports.go:15](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/shape_imports.go#L15) | `go/cmd/arcc/app` | - | no |
| `StdlibExportData` | type | [stdlib_export_data.go:25](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L25) | - | `go/cmd/arcc/app` | no |
| `StdlibExportRoot` | type | [stdlib_export_data.go:35](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L35) | - | `go/cmd/arcc/app` | no |
| `StdlibExportTarget` | type | [stdlib_export_data.go:44](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L44) | `go/cmd/arcc/app` | - | no |
| `ResolveStdlibExportData` | func | [stdlib_export_data.go:134](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L134) | - | - | yes |

- **Total exported symbols:** 41
- **Count used externally (non-test):** 21
- **Count test-only:** 3 (`OrdinaryImportData`, `StdlibExportData`, `StdlibExportRoot`)
- **Count internal-only:** 17 (`ImportGraphRequest`, `ImportGraphPackageInput`, `ImportGraph`, `ImportGraphPackage`, `ResolveOrdinaryImportData`, `DependencyArtifactProvenance`, `UnresolvedImport`, `(Layout).UnmarshalJSON`, `ValidateAndResolve`, `ValidateAndResolveForMemberOnly`, `FileMatchesBuildConstraints`, `SurvivingSourceFiles`, `HandleDriverRequest`, `RunDriver`, `WithTemporaryLayout`, `ResolveStdlibExportData`)

---

### 3. Authority inventory (measured)

- **Measurement Methodology:** Executed `arcc check` against the component manifest with stripped `declared_authority` in an isolated staging tree under the pinned Go 1.26.4 toolchain and target standard library authority map (`go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json`), and validated via Bazel action graph. When declared authority is empty, `arcc check` emits every exercised capability site as an `UNDECLARED_AUTHORITY` violation with exact file, line, referenced symbol, and evidence path. When declared authority matches `component.textproto`, zero unused authority warnings are emitted (`warnings: null`), proving that **all 7 declared capabilities are exercised**.

#### Measured Authority Call Sites Table

| capability | stdlib symbol referenced | member symbol (func/method) referencing it | file:line |
|---|---|---|---|
| `FILES` | `os.Stat` | `ResolveOrdinaryImportData` | [ordinary_import_data.go:157](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L157) |
| `FILES` | `os.Open` | `readImportGraph` | [ordinary_import_data.go:280](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L280) |
| `FILES` | `os.ReadFile` | `MergeImportGraphLayout` | [ordinary_import_data.go:310](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L310) |
| `FILES` | `os.WriteFile` | `MergeImportGraphLayout` | [ordinary_import_data.go:329](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L329) |
| `FILES` | `os.WriteFile` | `MergeImportGraphLayout` | [ordinary_import_data.go:405](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L405) |
| `FILES` | `os.ReadFile` | `WriteImportGraph` | [ordinary_import_data.go:470](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L470) |
| `FILES` | `os.ReadFile` | `WriteImportGraph` | [ordinary_import_data.go:521](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L521) |
| `FILES` | `os.WriteFile` | `WriteImportGraph` | [ordinary_import_data.go:562](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L562) |
| `FILES` | `os.Stat` | `<package-level>` (`var osStat = os.Stat`) | [packagelayout.go:79](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L79) |
| `FILES` | `os.Open` | `<package-level>` (`var osOpen = os.Open`) | [packagelayout.go:83](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L83) |
| `FILES` | `os.Stat` | `discoverStdlibTree` | [packagelayout.go:516](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L516) |
| `FILES` | `path/filepath.Walk` | `discoverStdlibTree` | [packagelayout.go:526](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L526) |
| `FILES` | `os.Stat` | `discoverStdlibTree` | [packagelayout.go:553](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L553) |
| `FILES` | `os.ReadDir` | `scanSDKPackageDir` | [packagelayout.go:701](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L701) |
| `FILES` | `os.ReadFile` | `scanSDKPackageDir` | [packagelayout.go:718](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L718) |
| `FILES` | `os.ReadDir` | `noGoPackageName` | [packagelayout.go:961](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L961) |
| `FILES` | `os.ReadFile` | `noGoPackageName` | [packagelayout.go:970](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L970) |
| `FILES` | `os.ReadDir` | `hasNonTestGoSource` | [packagelayout.go:988](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L988) |
| `FILES` | `os.ReadFile` | `hasNonTestGoSource` | [packagelayout.go:1000](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1000) |
| `FILES` | `os.ReadDir` | `hasBuildableTestSource` | [packagelayout.go:1017](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1017) |
| `FILES` | `os.ReadFile` | `hasBuildableTestSource` | [packagelayout.go:1030](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1030) |
| `FILES` | `os.ReadFile` | `ValidateAndResolve` | [packagelayout.go:1575](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1575) |
| `FILES` | `os.ReadFile` | `sourceImportsForPackage` | [packagelayout.go:1914](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1914) |
| `FILES` | `os.Open` | `leadingBuildConstraint` | [packagelayout.go:2009](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2009) |
| `FILES` | `os.Open` | `RunDriver` | [packagelayout.go:2606](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2606) |
| `FILES` | `os.Open` | `WithTemporaryLayout` | [packagelayout.go:2686](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2686) |
| `FILES` | `os.Open` | `WithDriverEnv` | [packagelayout.go:2745](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2745) |
| `FILES` | `os.Stdin` | `init` | [packagelayout.go:2847](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2847) |
| `FILES` | `os.Stdout` | `init` | [packagelayout.go:2847](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2847) |
| `FILES` | `os.Stderr` | `init` | [packagelayout.go:2848](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2848) |
| `FILES` | `os.Stat` | `ResolveStdlibExportData` | [stdlib_export_data.go:152](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L152) |
| `FILES` | `os.Open` | `readStdlibExportPackages` | [stdlib_export_data.go:227](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L227) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithTemporaryLayout` | [packagelayout.go:2717](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2717) |
| `MODIFY_SYSTEM_STATE` | `os.Unsetenv` | `WithTemporaryLayout` | [packagelayout.go:2729](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2729) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithTemporaryLayout` | [packagelayout.go:2731](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2731) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithDriverEnv` | [packagelayout.go:2781](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2781) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithDriverEnv` | [packagelayout.go:2782](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2782) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithDriverEnv` | [packagelayout.go:2783](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2783) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithDriverEnv` | [packagelayout.go:2784](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2784) |
| `MODIFY_SYSTEM_STATE` | `os.Unsetenv` | `WithDriverEnv` | [packagelayout.go:2797](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2797) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithDriverEnv` | [packagelayout.go:2799](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2799) |
| `MODIFY_SYSTEM_STATE` | `os.Unsetenv` | `WithDriverEnv` | [packagelayout.go:2803](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2803) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithDriverEnv` | [packagelayout.go:2805](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2805) |
| `MODIFY_SYSTEM_STATE` | `os.Unsetenv` | `WithDriverEnv` | [packagelayout.go:2809](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2809) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithDriverEnv` | [packagelayout.go:2811](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2811) |
| `MODIFY_SYSTEM_STATE` | `os.Unsetenv` | `WithDriverEnv` | [packagelayout.go:2815](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2815) |
| `MODIFY_SYSTEM_STATE` | `os.Setenv` | `WithDriverEnv` | [packagelayout.go:2817](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2817) |
| `MODIFY_SYSTEM_STATE` | `os.Stdin` | `init` | [packagelayout.go:2847](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2847) |
| `MODIFY_SYSTEM_STATE` | `os.Stdout` | `init` | [packagelayout.go:2847](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2847) |
| `MODIFY_SYSTEM_STATE` | `os.Stderr` | `init` | [packagelayout.go:2848](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2848) |
| `OPERATING_SYSTEM` | `os.Stdin` | `init` | [packagelayout.go:2847](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2847) |
| `OPERATING_SYSTEM` | `os.Stdout` | `init` | [packagelayout.go:2847](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2847) |
| `OPERATING_SYSTEM` | `os.Stderr` | `init` | [packagelayout.go:2848](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2848) |
| `READ_SYSTEM_STATE` | `go/build.init` | `<package-level>` (`import "go/build"`) | [packagelayout.go:58](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L58) |
| `READ_SYSTEM_STATE` | `runtime.GOROOT` | `nativeBuildContext` | [packagelayout.go:369](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L369) |
| `READ_SYSTEM_STATE` | `os.Getenv` | `nativeBuildContext` | [packagelayout.go:370](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L370) |
| `READ_SYSTEM_STATE` | `path/filepath.Abs` | `resolveSDKSourcePath` | [packagelayout.go:2224](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2224) |
| `READ_SYSTEM_STATE` | `path/filepath.Abs` | `resolveSDKSourcePath` | [packagelayout.go:2228](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2228) |
| `READ_SYSTEM_STATE` | `os.LookupEnv` | `WithTemporaryLayout` | [packagelayout.go:2712](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2712) |
| `READ_SYSTEM_STATE` | `os.LookupEnv` | `WithDriverEnv` | [packagelayout.go:2764](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2764) |
| `READ_SYSTEM_STATE` | `os.LookupEnv` | `WithDriverEnv` | [packagelayout.go:2765](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2765) |
| `READ_SYSTEM_STATE` | `os.LookupEnv` | `WithDriverEnv` | [packagelayout.go:2766](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2766) |
| `READ_SYSTEM_STATE` | `os.LookupEnv` | `WithDriverEnv` | [packagelayout.go:2767](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2767) |
| `READ_SYSTEM_STATE` | `os.Executable` | `WithDriverEnv` | [packagelayout.go:2774](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2774) |
| `READ_SYSTEM_STATE` | `os.Getenv` | `init` | [packagelayout.go:2843](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2843) |
| `READ_SYSTEM_STATE` | `os.Getenv` | `init` | [packagelayout.go:2844](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2844) |
| `READ_SYSTEM_STATE` | `os.Getenv` | `init` | [packagelayout.go:2845](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2845) |
| `REFLECT` | `(encoding/json.Decoder).Decode` | `readImportGraph` | [ordinary_import_data.go:288](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L288) |
| `REFLECT` | `(encoding/json.Decoder).Decode` | `readImportGraph` | [ordinary_import_data.go:292](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L292) |
| `REFLECT` | `encoding/json.MarshalIndent` | `MergeImportGraphLayout` | [ordinary_import_data.go:400](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L400) |
| `REFLECT` | `encoding/json.Unmarshal` | `WriteImportGraph` | [ordinary_import_data.go:475](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L475) |
| `REFLECT` | `encoding/json.MarshalIndent` | `WriteImportGraph` | [ordinary_import_data.go:557](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L557) |
| `REFLECT` | `(encoding/json.Decoder).Decode` | `Parse` | [packagelayout.go:1046](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1046) |
| `REFLECT` | `(encoding/json.Decoder).Token` | `Parse` | [packagelayout.go:1049](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1049) |
| `REFLECT` | `encoding/json.Marshal` | `(l *Layout).MarshalJSON` | [packagelayout.go:1197](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1197) |
| `REFLECT` | `encoding/json.Unmarshal` | `(l *Layout).MarshalJSON` | [packagelayout.go:1202](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1202) |
| `REFLECT` | `encoding/json.Marshal` | `(l *Layout).MarshalJSON` | [packagelayout.go:1218](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1218) |
| `REFLECT` | `encoding/json.Marshal` | `(l *Layout).MarshalJSON` | [packagelayout.go:1224](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1224) |
| `REFLECT` | `encoding/json.Unmarshal` | `(l *Layout).UnmarshalJSON` | [packagelayout.go:1257](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1257) |
| `REFLECT` | `encoding/json.Unmarshal` | `(l *Layout).UnmarshalJSON` | [packagelayout.go:1274](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1274) |
| `REFLECT` | `encoding/json.Unmarshal` | `(l *Layout).UnmarshalJSON` | [packagelayout.go:1280](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1280) |
| `REFLECT` | `encoding/json.Unmarshal` | `(l *Layout).UnmarshalJSON` | [packagelayout.go:1284](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1284) |
| `REFLECT` | `(encoding/json.Decoder).Decode` | `RunDriver` | [packagelayout.go:2596](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2596) |
| `REFLECT` | `(encoding/json.Encoder).Encode` | `RunDriver` | [packagelayout.go:2631](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2631) |
| `REFLECT` | `(encoding/json.Decoder).Decode` | `readStdlibExportPackages` | [stdlib_export_data.go:237](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L237) |
| `RUNTIME` | `runtime.init` | `<package-level>` (`import "runtime"`) | [packagelayout.go:67](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L67) |
| `UNSAFE_POINTER` | `(encoding/json.Decoder).Decode` | `readImportGraph` | [ordinary_import_data.go:288](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L288) |
| `UNSAFE_POINTER` | `(encoding/json.Decoder).Decode` | `readImportGraph` | [ordinary_import_data.go:292](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L292) |
| `UNSAFE_POINTER` | `encoding/json.MarshalIndent` | `MergeImportGraphLayout` | [ordinary_import_data.go:400](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L400) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `WriteImportGraph` | [ordinary_import_data.go:475](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L475) |
| `UNSAFE_POINTER` | `encoding/json.MarshalIndent` | `WriteImportGraph` | [ordinary_import_data.go:557](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L557) |
| `UNSAFE_POINTER` | `(encoding/json.Decoder).Decode` | `Parse` | [packagelayout.go:1046](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1046) |
| `UNSAFE_POINTER` | `(encoding/json.Decoder).Token` | `Parse` | [packagelayout.go:1049](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1049) |
| `UNSAFE_POINTER` | `encoding/json.Marshal` | `(l *Layout).MarshalJSON` | [packagelayout.go:1197](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1197) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `(l *Layout).MarshalJSON` | [packagelayout.go:1202](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1202) |
| `UNSAFE_POINTER` | `encoding/json.Marshal` | `(l *Layout).MarshalJSON` | [packagelayout.go:1218](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1218) |
| `UNSAFE_POINTER` | `encoding/json.Marshal` | `(l *Layout).MarshalJSON` | [packagelayout.go:1224](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1224) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `(l *Layout).UnmarshalJSON` | [packagelayout.go:1257](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1257) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `(l *Layout).UnmarshalJSON` | [packagelayout.go:1274](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1274) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `(l *Layout).UnmarshalJSON` | [packagelayout.go:1280](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1280) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `(l *Layout).UnmarshalJSON` | [packagelayout.go:1284](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1284) |
| `UNSAFE_POINTER` | `(encoding/json.Decoder).Decode` | `RunDriver` | [packagelayout.go:2596](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2596) |
| `UNSAFE_POINTER` | `(encoding/json.Encoder).Encode` | `RunDriver` | [packagelayout.go:2631](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2631) |
| `UNSAFE_POINTER` | `(encoding/json.Decoder).Decode` | `readStdlibExportPackages` | [stdlib_export_data.go:237](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L237) |

#### Functional Clusters of Authority-Bearing Call Sites

1. **Process Environment & Self-exec Driver Lifecycle (`driverenv`)**
   - **LOC estimate:** ~210 LOC ([packagelayout.go:2645-2853](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2645-L2853))
   - **Capabilities:** `MODIFY_SYSTEM_STATE` (18 sites: `os.Setenv`, `os.Unsetenv`, stdio), `OPERATING_SYSTEM` (3 sites: stdio writes in `init`), `READ_SYSTEM_STATE` (8 sites: `os.LookupEnv`, `os.Executable`, `os.Getenv`), `FILES` (4 sites: `os.Open`, stdio files).
   - **Is leaf?** No: `init()` calls `RunDriver` which calls `HandleDriverRequest`. However, `WithDriverEnv` is a lifecycle wrapper around an arbitrary `func() error`.
   - **Minimal API to isolate in sub-package:**
     ```go
     func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error
     func IsDriverMode() bool
     func DriverParams() (layoutPath, workspaceDir string, patterns []string)
     var CheckMu sync.Mutex
     ```

2. **JSON Serialization & Deserialization (`encoding/json`)**
   - **LOC estimate:** ~350 LOC ([packagelayout.go:1041-1330, 2596, 2631](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1041-L1330); [ordinary_import_data.go:280-305, 400, 475, 557](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L280-L305); [stdlib_export_data.go:230-245](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L230-L245))
   - **Capabilities:** `REFLECT` (18 sites), `UNSAFE_POINTER` (18 sites).
   - **Is leaf?** Yes. Pure encoding and decoding of memory structures; does not call back into driver or validation logic.
   - **Minimal API to isolate in sub-package:**
     ```go
     func Parse(r io.Reader) (*Layout, error)
     func MarshalLayout(l *Layout) ([]byte, error)
     func UnmarshalLayout(data []byte, l *Layout) error
     ```

3. **Standard Library Tree Walk & Discovery (`stdlibdiscover`)**
   - **LOC estimate:** ~540 LOC ([packagelayout.go:503-1040](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L503-L1040))
   - **Capabilities:** `FILES` (10 sites: `os.Stat`, `filepath.Walk`, `os.ReadDir`, `os.ReadFile`).
   - **Is leaf?** No, calls source constraint checking and lexical scanner (`scanGoSource`).
   - **Minimal API to isolate in sub-package:**
     ```go
     func DiscoverStdlib(sdkRoot string, bctx build.Context) ([]*packages.Package, []string, error)
     ```

4. **Target Platform Defaults & Runtime Inspection (`targetcontext`)**
   - **LOC estimate:** ~180 LOC ([packagelayout.go:180-502, 2220-2240](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L180-L502))
   - **Capabilities:** `READ_SYSTEM_STATE` (4 sites: `go/build.init`, `runtime.GOROOT()`, `os.Getenv("GOPATH")`, `filepath.Abs` -> `os.Getwd`), `RUNTIME` (1 site: `runtime.init`).
   - **Is leaf?** Yes. Computes and returns a pure `build.Context` struct.
   - **Minimal API to isolate in sub-package:**
     ```go
     func BuildContextForLayout(l *Layout) (build.Context, error)
     ```

5. **Descriptor and Layout File I/O (`fsio`)**
   - **LOC estimate:** ~600 LOC ([ordinary_import_data.go:141-587](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go#L141-L587); [stdlib_export_data.go:131-498](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go#L131-L498); [packagelayout.go:1575, 1914, 2009](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L1575))
   - **Capabilities:** `FILES` (18 sites: `os.Stat`, `os.Open`, `os.ReadFile`, `os.WriteFile`).
   - **Is leaf?** Leaf file system read/write utilities.
   - **Minimal API to isolate in sub-package:**
     ```go
     func ReadFile(path string) ([]byte, error)
     func WriteFile(path string, data []byte) error
     func Stat(path string) (os.FileInfo, error)
     ```

#### Root Causes of Declared Capabilities

- **`OPERATING_SYSTEM`:** Caused exclusively by referencing standard I/O file handles `os.Stdin`, `os.Stdout`, `os.Stderr` in `packagelayout.go:2847-2848` inside `init()`, which transitively invokes `(*os.File).WriteTo`.
- **`MODIFY_SYSTEM_STATE`:** Caused by environment variable manipulation: `os.Setenv` and `os.Unsetenv` in `WithDriverEnv` ([packagelayout.go:2781-2817](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2781-L2817)) and `WithTemporaryLayout` ([packagelayout.go:2717-2731](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2717-L2731)), plus `os.Stdin/Stdout/Stderr` in `init()`.
- **`READ_SYSTEM_STATE`:** Caused by: (1) `import "go/build"` which executes package init `go/build.init` reading host environment variables via `os.Getenv` ([packagelayout.go:58](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L58)); (2) reading `runtime.GOROOT()` and `os.Getenv("GOPATH")` in `nativeBuildContext` ([packagelayout.go:369-370](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L369-L370)); (3) calling `filepath.Abs` which inspects `os.Getwd()` ([packagelayout.go:2224, 2228](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2224)); (4) inspecting process environment with `os.LookupEnv` and executable path with `os.Executable` in `WithDriverEnv` ([packagelayout.go:2764-2774](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2764-L2774)); and (5) `os.Getenv` in `init()` ([packagelayout.go:2843-2845](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2843-L2845)).
- **`REFLECT`:** Caused entirely by `encoding/json` (`json.Marshal`, `json.Unmarshal`, `json.NewDecoder`, `json.NewEncoder`, `json.MarshalIndent`). The stdlib map traces `(*encoding/json.Decoder).Decode` -> `unmarshal` -> `value` -> `object` -> `reflect.MakeMap` / `reflect.ValueOf`. `packagelayout` does not import `reflect` directly.
- **`UNSAFE_POINTER`:** Caused entirely by `encoding/json`. The stdlib map traces `(*encoding/json.Decoder).Decode` -> `literalStore` -> `strconv.ParseFloat` -> `internal/strconv.parseFloatPrefix` -> `atof32` -> `float32frombits` -> `unsafe.Pointer`. `packagelayout` does not import `unsafe` directly.
- **`RUNTIME`:** Caused by `import "runtime"` in [packagelayout.go:67](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L67), which triggers `runtime.init` (evidence `(*runtime.addrRanges).init`). Used to read `runtime.Version()`, `runtime.GOOS`, `runtime.GOARCH`, `runtime.GOROOT()`, and `runtime.Compiler`.
- **`FILES`:** Caused by 32 direct filesystem call sites across `ordinary_import_data.go` (8 sites: `os.Stat`, `os.Open`, `os.ReadFile`, `os.WriteFile`), `packagelayout.go` (22 sites: `os.Stat`, `os.Open`, `filepath.Walk`, `os.ReadDir`, `os.ReadFile`, stdio), and `stdlib_export_data.go` (2 sites: `os.Stat`, `os.Open`).
- **Exercised Capabilities:** All 7 declared capabilities are exercised; none are unused.

---

### 4. Internal structure and seams

#### Functional Sub-clusters

1. **Schema & Models (pure data types, ~200 LOC):**
   - Structs: `Layout`, `Platform`, `DependencyArtifactBinding`, `DependencyArtifactProvenance`, `UnresolvedImport`, `OrdinaryImportData`, `StdlibExportData`, `StdlibExportRoot`, `StdlibExportTarget`.
   - Pure helpers: `IsMemberOnlyLayout`, `(Layout).IsStdlibPackage`.
2. **Platform & Build Context Configuration (pure computation + runtime inspection, ~320 LOC):**
   - Functions: `BuildContextForLayout`, `nativeBuildContext`, `supportedGoTargets`, `supportedGOOS`, `supportedGOARCH`, `supportedGoTarget`, `baselineExperimentTags`, `effectiveExperimentTags`, `releaseTagsForVersion`, `defaultArchToolTags`.
3. **Build Constraints & File Filtering (pure lexical evaluation, ~220 LOC):**
   - Functions: `filterByBuildConstraints`, `filterByBuildConstraintsWithContext`, `splitGoFilesByBuildConstraints`, `mergeFileRoles`, `FileMatchesBuildConstraints`, `FileMatchesBuildConstraintsWithContext`, `leadingBuildConstraint`, `filenameMatchesBuildContext`, `buildContextHasTag`, `SurvivingSourceFiles`.
4. **Lexical Source Scanner (pure token analysis, ~120 LOC):**
   - Functions: `scanGoSource` (uses `go/scanner` and `go/token` to extract package declaration, import paths, and cgo comments without AST parsing).
5. **Import Shaping (pure helper, 35 LOC in [shape_imports.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/shape_imports.go)):**
   - `ShapeImports`: pure map conversion with duplicate canonical path rejection.
6. **JSON Codecs (data transformation + `encoding/json`, ~350 LOC):**
   - Functions: `Parse`, `(Layout).MarshalJSON`, `(Layout).UnmarshalJSON`, `cloneDependencyArtifactBindings`, `validateDependencyArtifactBindings`, `validateDependencyArtifactPath`.
7. **Validation & Resolution Engine (validation logic + file checks, ~600 LOC):**
   - Functions: `indexLayoutPackages`, `layoutRootPackages`, `ValidateAndResolve`, `ValidateAndResolveForMemberOnly`, `sourceImportsForPackage`, `(Layout).resolveAndCheckFiles`, `validateReachableExportClosure`, `validateReachableSDKSourcePackages`.
8. **Ordinary Import Metadata Processor (file I/O + parsing + layout mutation, 587 LOC in [ordinary_import_data.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/ordinary_import_data.go)):**
   - Functions: `ResolveOrdinaryImportData`, `readImportGraph`, `MergeImportGraphLayout`, `WriteImportGraph`.
9. **Stdlib Export Metadata Processor (file I/O + stream decoding + path mapping, 498 LOC in [stdlib_export_data.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/stdlib_export_data.go)):**
   - Functions: `ResolveStdlibExportData`, `readStdlibExportPackages`.
10. **Stdlib Source Discovery (filesystem tree walk + package classification, ~540 LOC):**
    - Functions: `discoverStdlib`, `discoverStdlibWithContext`, `discoverStdlibTree`, `scanSDKPackageDir`, `noGoPackageName`, `hasNonTestGoSource`, `hasBuildableTestSource`, `StdlibLayout`.
11. **Driver Protocol & Self-Execution Subsystem (GOPACKAGESDRIVER protocol + OS environment, ~420 LOC):**
    - Functions: `clonePackageForDriver`, `HandleDriverRequest`, `resolveQueryPattern`, `RunDriver`, `WithDriverEnv`, `WithTemporaryLayout`, `IsLayoutMode`, `GetActiveLayout`, `GetActiveLayoutPath`, `GetActiveWorkspaceDir`, `init()`.

#### Pure Computation vs. Environment Interaction Clusters

- **Pure Computation Clusters:**
  - Schema types & models
  - Build constraint evaluation (`filterByBuildConstraintsWithContext`, `FileMatchesBuildConstraintsWithContext`)
  - Lexical scanner (`scanGoSource`)
  - Import shaping (`ShapeImports`)
  - Driver query matcher (`clonePackageForDriver`, `resolveQueryPattern`, `HandleDriverRequest`)
  - Layout graph indexing & root verification (`indexLayoutPackages`, `layoutRootPackages`)
- **Environment Interaction Clusters:**
  - Driver lifecycle & process environment (`WithDriverEnv`, `WithTemporaryLayout`, `init`, `CheckMu`) — touches `os.Setenv`, `os.Unsetenv`, `os.Getenv`, `os.LookupEnv`, `os.Executable`, stdio.
  - File reading/writing (`WriteImportGraph`, `MergeImportGraphLayout`, `ResolveOrdinaryImportData`, `ResolveStdlibExportData`, `os.ReadFile`, `os.Stat`) — touches `os.Stat`, `os.Open`, `os.ReadFile`, `os.WriteFile`.
  - SDK filesystem walk (`discoverStdlibTree`, `scanSDKPackageDir`) — touches `filepath.Walk`, `os.ReadDir`, `os.ReadFile`, `os.Stat`.
  - Target baseline discovery (`nativeBuildContext`) — touches `runtime.GOROOT()`, `os.Getenv("GOPATH")`, `runtime.Compiler`, `runtime.Version()`, and triggers `go/build.init`.
  - JSON serialization/deserialization — touches `encoding/json` (which brings `reflect` and `unsafe.Pointer`).

#### Cross-File Unexported Dependencies

| Source File | Target File | Unexported Identifier | Kind |
|---|---|---|---|
| `cgo_enabled.go` | `cgo_disabled.go` | `nativeCgoEnabled` | const |
| `ordinary_import_data.go` | `packagelayout.go` | `indexLayoutPackages` | func |
| `ordinary_import_data.go` | `packagelayout.go` | `layoutRootPackages` | func |
| `ordinary_import_data.go` | `packagelayout.go` | `scanGoSource` | func |
| `ordinary_import_data.go` | `packagelayout.go` | `validateExportFilePath` | func |
| `packagelayout.go` | `cgo_disabled.go` | `nativeCgoEnabled` | const |
| `packagelayout.go` | `ordinary_import_data.go` | `attachLayoutPathContext` | func |
| `packagelayout.go` | `ordinary_import_data.go` | `cloneOrdinaryImportData` | func |
| `packagelayout.go` | `ordinary_import_data.go` | `withLayoutComponent` | func |
| `packagelayout.go` | `stdlib_export_data.go` | `cloneStdlibExportData` | func |
| `packagelayout.go` | `stdlib_export_data.go` | `validateReachableExportClosure` | func |
| `packagelayout.go` | `stdlib_export_data.go` | `validateStdlibPackagePath` | func |
| `stdlib_export_data.go` | `packagelayout.go` | `hasWindowsVolumePrefix` | func |
| `stdlib_export_data.go` | `packagelayout.go` | `layoutPackageIndex` | type |
| `stdlib_export_data.go` | `packagelayout.go` | `validateExportFilePath` | func |

#### Package-Level Mutable State, `init()`, and Build Tags

- **Package-level mutable state:**
  - `CheckMu sync.Mutex` ([packagelayout.go:2646](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2646)): public mutex used across packages to synchronize process environment modification during layout execution.
  - `activeLayout *Layout` ([packagelayout.go:2647](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2647)): unexported pointer storing the layout currently active in the process.
  - `activeLayoutPath string` ([packagelayout.go:2648](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2648)): unexported string storing the path to the active layout file.
  - `activeWorkspaceDir string` ([packagelayout.go:2649](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2649)): unexported string storing the active workspace directory.
  - `osStat = os.Stat` ([packagelayout.go:79](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L79)): unexported variable used as a test mock seam.
  - `osOpen = os.Open` ([packagelayout.go:83](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L83)): unexported variable used as a test mock seam.
- **`init()` function ([packagelayout.go:2842-2852](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2842-L2852)):**
  - Intercepts process startup before `main()`: if `os.Getenv("ARCC_DRIVER_MODE") == "1"`, reads `ARCC_PACKAGE_LAYOUT` and `ARCC_WORKSPACE_DIR`, extracts `os.Args[1:]`, invokes `RunDriver(layoutPath, workspaceDir, patterns, os.Stdin, os.Stdout)`, and terminates the process immediately via `os.Exit(0)` (or `os.Exit(1)` on error).
- **Build-tagged files:**
  - [cgo_disabled.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/cgo_disabled.go): `//go:build !cgo`
  - [cgo_enabled.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/cgo_enabled.go): `//go:build cgo`
  - Together they declare the single boolean constant `nativeCgoEnabled`.

---

### 5. Consumers' view

#### 1. `go/internal/goanalysis`
- **Schema & Identity:**
  - `packagelayout.Layout`
  - `packagelayout.IsMemberOnlyLayout`
  - `packagelayout.DependencyArtifactBinding`
  - `packagelayout.DependencyArtifactProvenanceChecked`
  - `packagelayout.DependencyArtifactProvenanceAsserted`
  - `packagelayout.UnresolvedImport` (via `Layout.UnresolvedImports`)
- **Build Constraints & Context:**
  - `packagelayout.BuildContextForLayout`
  - `packagelayout.FileMatchesBuildConstraintsWithContext`
- **Environment & State Execution:**
  - `packagelayout.CheckMu`
  - `packagelayout.WithDriverEnv`
  - `packagelayout.IsLayoutMode`
  - `packagelayout.GetActiveLayout`
  - `packagelayout.GetActiveLayoutPath`
  - `packagelayout.GetActiveWorkspaceDir`

#### 2. `go/internal/stdlibmap`
- **Schema & Generation:**
  - `packagelayout.Layout`
  - `packagelayout.Platform`
  - `packagelayout.StdlibLayout`
  - `packagelayout.WithDriverEnv`

#### 3. `go/cmd/arcc/app`
- **Schema & Codecs:**
  - `packagelayout.Layout`
  - `packagelayout.Platform`
  - `packagelayout.Parse`
  - `(Layout).MarshalJSON` (via `json.MarshalIndent`)
  - `(Layout).IsStdlibPackage`
  - `packagelayout.ShapeImports`
  - `packagelayout.DependencyArtifactBinding`
  - `packagelayout.StdlibExportTarget`
  - `packagelayout.StdlibExportData` (in test fixtures)
  - `packagelayout.StdlibExportRoot` (in test fixtures)
  - `packagelayout.OrdinaryImportData` (in test fixtures)
  - `packagelayout.DependencyArtifactProvenanceChecked` (in test fixtures)
- **Import Graph Emitter Actions:**
  - `packagelayout.MergeImportGraphLayout`
  - `packagelayout.WriteImportGraph`

#### 4. `go/internal/checker`
- `(Layout).IsStdlibPackage`

#### 5. `go/cmd/arcc` (integration tests)
- `packagelayout.WithDriverEnv`
- `packagelayout.IsLayoutMode`

#### 6. `bazel_rules/go/tests`
- `packagelayout.Parse`

---

### 6. Tests

#### Test Files Inventory

| Test File | LOC | Package | Relies on Unexported Identifiers? |
|---|---|---|---|
| [dependency_artifacts_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/dependency_artifacts_test.go) | 129 | `packagelayout` | No (0 unexported identifiers) |
| [export_data_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/export_data_test.go) | 828 | `packagelayout` | Yes (4 unexported: `attachLayoutPathContext`, `ordinaryImportDataFormatVersion`, `ordinaryImportDataLogicalPath`, `osOpen`) |
| [member_file_roles_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/member_file_roles_test.go) | 114 | `packagelayout` | No (0 unexported identifiers) |
| [member_only_driver_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/member_only_driver_test.go) | 125 | `packagelayout` | No (0 unexported identifiers) |
| [packagelayout_integration_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout_integration_test.go) | 103 | `packagelayout` | No (0 unexported identifiers) |
| [packagelayout_test.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout_test.go) | 2949 | `packagelayout` | Yes (5 unexported: `discoverStdlib`, `discoverStdlibWithContext`, `filterByBuildConstraintsWithContext`, `nativeCgoEnabled`, `osStat`; plus unexported struct field `stdlibByID`) |

- **In-package vs. External:** All 6 test files are declared in-package (`package packagelayout`). There are no `package packagelayout_test` external test files.
- **Unexported identifiers constraints on splitting:**
  - `packagelayout_test.go` exercises internal discovery (`discoverStdlib`, `discoverStdlibWithContext`), unexported constraint helpers (`filterByBuildConstraintsWithContext`), internal build flags (`nativeCgoEnabled`), and mocks filesystem existence via `osStat`.
  - `export_data_test.go` exercises unexported descriptor internals (`attachLayoutPathContext`, `ordinaryImportDataFormatVersion`, `ordinaryImportDataLogicalPath`) and mocks export file readability via `osOpen`.
- **Testdata directories:** None (`find go/internal/packagelayout -type d` yields no subdirectories). All fixtures are either synthesized programmatically in memory or created within `t.TempDir()`.

---

### 7. Draft split proposal

#### Proposed Architecture

The current component violates the target architecture by using `interface_style = PACKAGE_SURFACE` for in-tree project code and concentrating 7 capabilities (`FILES`, `READ_SYSTEM_STATE`, `OPERATING_SYSTEM`, `MODIFY_SYSTEM_STATE`, `REFLECT`, `RUNTIME`, `UNSAFE_POINTER`) across 3983 LOC in one package.

The natural decomposition splits the component into:
1. **Interface Package (`go/internal/packagelayout`):** Narrow interface declaring the schema data structures, query methods, and loading entry points.
2. **Member Packages (`go/internal/packagelayout/internal/...`):**
   - `internal/layoutmodel`: JSON schema data types and JSON marshal/unmarshal codecs (`REFLECT`, `UNSAFE_POINTER`).
   - `internal/layoutvalidate`: Graph validation, reachable export closure checking, build constraint filtering, and lexical source scanning.
   - `internal/driverhandler`: `HandleDriverRequest` query resolution logic.
3. **Isolated Authority Sub-component (`go/internal/packagelayout/driverenv`):**
   - Encapsulates all process environment mutation, `GOPACKAGESDRIVER` setup, binary self-execution, `init()` dispatch, and active process-level layout state.
   - Carries declared authority: `["MODIFY_SYSTEM_STATE", "OPERATING_SYSTEM", "READ_SYSTEM_STATE", "FILES"]`.

#### Proposed Interface Package Contents (`go/internal/packagelayout`)

- **Schema Types:**
  - `type Layout struct`
  - `type Platform struct`
  - `type DependencyArtifactBinding struct`
  - `type DependencyArtifactProvenance string`
  - `const DependencyArtifactProvenanceChecked, DependencyArtifactProvenanceAsserted`
  - `type UnresolvedImport struct`
  - `type OrdinaryImportData struct`
  - `type StdlibExportData struct`, `type StdlibExportRoot struct`, `type StdlibExportTarget struct`
- **Core Methods & Functions:**
  - `func Parse(r io.Reader) (*Layout, error)`
  - `func (l *Layout) MarshalJSON() ([]byte, error)`
  - `func (l *Layout) UnmarshalJSON(data []byte) error`
  - `func (l *Layout) IsStdlibPackage(p *packages.Package) bool`
  - `func IsMemberOnlyLayout(l *Layout) bool`
  - `func BuildContextForLayout(l *Layout) (build.Context, error)`
  - `func FileMatchesBuildConstraintsWithContext(path string, bctx build.Context) bool`
  - `func ShapeImports(imports map[string]*packages.Package, canonicalize func(string) string) (map[string]string, error)`
  - `func StdlibLayout(sdkRoot string, platform *Platform) (*Layout, error)`
  - `func MergeImportGraphLayout(baseLayoutPath, graphPath, outputPath string) error`
  - `func WriteImportGraph(configPath, outputPath string) error`
  - Re-exported driver environment entry points (delegating to sub-component `driverenv`):
    - `func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error`
    - `func IsLayoutMode() bool`
    - `func GetActiveLayout() *Layout`
    - `func GetActiveLayoutPath() string`
    - `func GetActiveWorkspaceDir() string`
    - `var CheckMu sync.Mutex`

#### Proposed Authority Sub-component (`packagelayout/driverenv`)

- **Location:** `go/internal/packagelayout/driverenv`
- **Declared Authority:**
  - `MODIFY_SYSTEM_STATE` (`os.Setenv`, `os.Unsetenv`)
  - `OPERATING_SYSTEM` (`os.Stdin`, `os.Stdout`, `os.Stderr`)
  - `READ_SYSTEM_STATE` (`os.Getenv`, `os.LookupEnv`, `os.Executable`)
  - `FILES` (`os.Open`, stdio handles)
- **Exact API Signature:**
  ```go
  package driverenv

  import (
      "sync"
      "github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
  )

  var CheckMu sync.Mutex

  func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error
  func IsLayoutMode() bool
  func ActiveLayout() *packagelayout.Layout
  func ActiveLayoutPath() string
  func ActiveWorkspaceDir() string
  func RunDriverMain(patterns []string) error
  ```

#### Resulting Declared Authority of Parent Component (`packagelayout`)

- By delegating environment manipulation to `driverenv`, the parent `packagelayout` component drops:
  - `OPERATING_SYSTEM` -> eliminated from parent!
  - `MODIFY_SYSTEM_STATE` -> eliminated from parent!
- If descriptor file reading and SDK tree walking remain in member packages under `packagelayout`:
  - `FILES`: retained in member package `internal/fs` for reading layout files, metadata descriptors, and SDK Go sources.
  - `READ_SYSTEM_STATE`: retained for `go/build.init` and `nativeBuildContext`.
  - `REFLECT`: retained in member package `internal/layoutmodel` for `encoding/json`.
  - `UNSAFE_POINTER`: retained in member package `internal/layoutmodel` for `encoding/json`.
  - `RUNTIME`: retained for `runtime.Version()`, `runtime.GOOS`, `runtime.GOARCH`.

#### Flagged Risks & Constraints

1. **`init()` Dispatch Ordering:**
   - [packagelayout.go:2842-2852](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/packagelayout/packagelayout.go#L2842-L2852) uses `func init()` so that when `cmd/arcc` is invoked by `go/packages` as a subprocess with `ARCC_DRIVER_MODE=1`, it intercepts execution before `main()` runs. Moving `init()` into a sub-component requires ensuring that `cmd/arcc` imports that sub-component (either directly or transitively) so the hook remains active during driver execution.
2. **`CheckMu` Mutex Sharing:**
   - `CheckMu` is currently public in `packagelayout` and locked directly by `goanalysis.go:96` around `go/packages.Load`. If moved to `driverenv`, `goanalysis` must reference the new location or call a wrapper.
3. **In-Package Unit Tests:**
   - `packagelayout_test.go` and `export_data_test.go` access unexported functions (`discoverStdlib`, `attachLayoutPathContext`, `osStat`, `osOpen`). Splitting into internal member packages requires either exporting these helpers with `internal/` package visibility or colocating tests with their respective member packages.
4. **Third-party / Foreign Boundaries:**
   - Upstream `golang.org/x/tools/go/packages` remains the sole asserted foreign component dependency (`x-tools`). Splitting does not introduce any foreign dependency cycle.
