# Component analysis report template

## Component: stdlibmap  (dir: go/internal/stdlibmap, import path github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap)

### 1. Current declaration
- **Bazel `go_component` attrs**: None. `stdlibmap` is not declared as a `go_component` in Bazel. As documented in [BUILD.bazel:3-8](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/BUILD.bazel#L3-L8), its closure (`golang.org/x/tools/go/packages`) pulls in cgo-built packages and `component.bzl` fails closed on cgo closures. It is declared only as a standard `go_library` in [BUILD.bazel:9-36](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/BUILD.bazel#L9-L36).
- **Checked-in manifest** ([component.textproto:1-61](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/component.textproto#L1-L61)):
  - `name`: `"stdlibmap"`
  - `interface_files`: lists ALL 10 non-test source files: `"cache.go"`, `"command.go"`, `"explicit.go"`, `"layoutloader.go"`, `"generate.go"`, `"inventory.go"`, `"nativeloader.go"`, `"project_export.go"`, `"sdkkey.go"`, `"stdlibmap.go"`
  - `members`: `["github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"]`
  - `component_dependencies`: `artifactio`, `capslockadapter`, `schema`, `stdlibauthority`, `symbol`, `x-tools`, `hostpolicy`, `packagelayout` (8 dependencies)
  - `declared_authority`: `FILES`, `EXEC`, `READ_SYSTEM_STATE`, `REFLECT`, `UNSAFE_POINTER`
- **Divergence**: `BUILD.bazel` does not define a `go_component` rule; `component.textproto` defines the native check configuration and declares every single non-test `.go` file in the package directory as an `interface_file`.
- **Files (non-test)**:

| File | LOC | Purpose |
| :--- | :--- | :--- |
| [cache.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go) | 372 | On-disk cache for generated SDK maps, digest keying, and native configuration detection |
| [command.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/command.go) | 216 | Standalone CLI entrypoint (`RunGenerateCommand`) for generating authority maps |
| [explicit.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/explicit.go) | 194 | Parses target configs and explicit package list text files, reconciling toolchain stdlib sets |
| [generate.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go) | 1234 | Core pure map generator: synthesizes inventory, Capslock findings, minting rules, and safe keys |
| [inventory.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go) | 495 | Pure `go/types` AST and type inventory builder for stdlib package and method declarations |
| [layoutloader.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go) | 190 | Loader adapter wrapping Bazel/hermetic package layouts for export-data based generation |
| [nativeloader.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/nativeloader.go) | 138 | Native package loader invoking `go/packages` with target environment overrides |
| [project_export.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go) | 227 | Projects export-data artifacts from Bazel execroots into hermetic staging roots |
| [sdkkey.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/sdkkey.go) | 119 | Target configuration model, classifier hashing, and SDK key derivation |
| [stdlibmap.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go) | 258 | Standard library package oracle, internal path classification, and native toolchain discovery |
| **Total** | **3243** | |

### 2. Exported API inventory

| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `RuleVersion` | const | [cache.go:26](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L26) | `teststdlibmap`, `cmd/arcc/app` | `cmd/arcc/app` | no |
| `CacheKeyDigest` | func | [cache.go:34](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L34) | none | none | yes |
| `DefaultCacheRoot` | func | [cache.go:65](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L65) | none | none | yes |
| `CacheSeams` | type | [cache.go:82](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L82) | none | none | yes |
| `CachedMapInput` | type | [cache.go:199](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L199) | `cmd/arcc/app` | none | no |
| `OpenCachedMap` | func | [cache.go:226](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L226) | `cmd/arcc/app` | none | no |
| `WriteArtifactAtomic` | func | [cache.go:331](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L331) | none | `cmd/arcc/app` | no |
| `DecodeMapArtifact` | func | [cache.go:338](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L338) | `cmd/arcc/app` | none | no |
| `NativeTargetConfig` | func | [cache.go:348](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L348) | `cmd/arcc/app` | `cmd/arcc`, `cmd/arcc/app` | no |
| `RunGenerateCommand` | func | [command.go:17](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/command.go#L17) | `cmd/arcc-stdlibmap`, `cmd/arcc/app` | none | no |
| `RenderTargetConfig` | func | [explicit.go:19](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/explicit.go#L19) | none | `cmd/arcc/app` | no |
| `ParseTargetConfig` | func | [explicit.go:40](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/explicit.go#L40) | none | none | yes |
| `ReadToolchainPackageList` | func | [explicit.go:108](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/explicit.go#L108) | none | none | yes |
| `ReconcilePackageList` | func | [explicit.go:152](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/explicit.go#L152) | none | none | yes |
| `ProvenanceCapslockCurated` | const | [generate.go:25](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L25) | none | none | yes |
| `ProvenanceProjectOverride` | const | [generate.go:26](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L26) | none | none | yes |
| `ProvenanceProvedPure` | const | [generate.go:27](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L27) | none | none | yes |
| `FindingSource` | type | [generate.go:39](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L39) | none | `cmd/arcc/app` | no |
| `FindingSourceFunc` | type | [generate.go:44](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L44) | none | `cmd/arcc/app` | no |
| `(FindingSourceFunc).Findings` | method | [generate.go:47](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L47) | none | none | yes |
| `GeneratedMap` | type | [generate.go:53](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L53) | none | none | yes |
| `GenerationInput` | type | [generate.go:62](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L62) | `cmd/arcc/app` | `cmd/arcc/app` | no |
| `Generate` | func | [generate.go:85](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L85) | none | `cmd/arcc/app` | no |
| `GenerationClassifierRules` | func | [generate.go:226](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L226) | `teststdlibmap`, `cmd/arcc/app` | none | no |
| `UnsafeRuleSemantics` | type | [generate.go:661](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L661) | none | none | yes |
| `VarRuleSemantics` | type | [generate.go:679](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L679) | none | none | yes |
| `BuildAuthorityMap` | func | [generate.go:964](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L964) | none | none | yes |
| `Loader` | type | [inventory.go:16](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L16) | none | `cmd/arcc/app` | no |
| `LoaderFunc` | type | [inventory.go:25](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L25) | none | `cmd/arcc/app` | no |
| `(LoaderFunc).Load` | method | [inventory.go:28](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L28) | none | `cmd/arcc/app` | no |
| `Inventory` | type | [inventory.go:34](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L34) | none | none | yes |
| `(*Inventory).KnownPackage` | method | [inventory.go:58](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L58) | none | none | yes |
| `(*Inventory).Declares` | method | [inventory.go:69](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L69) | none | none | yes |
| `(*Inventory).MethodOwner` | method | [inventory.go:82](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L82) | none | none | yes |
| `BuildInventory` | func | [inventory.go:177](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L177) | none | none | yes |
| `LayoutLoader` | type | [layoutloader.go:26](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L26) | none | none | yes |
| `(*LayoutLoader).Load` | method | [layoutloader.go:34](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L34) | none | `cmd/arcc/app` | no |
| `GenerateViaLayout` | func | [layoutloader.go:135](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L135) | none | none | yes |
| `GenerateExplicit` | func | [layoutloader.go:161](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L161) | none | none | yes |
| `TargetEnv` | func | [nativeloader.go:23](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/nativeloader.go#L23) | `cmd/arcc/app` | none | no |
| `NativeEnvironment` | func | [nativeloader.go:77](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/nativeloader.go#L77) | `cmd/arcc/app` | none | no |
| `NativeLoader` | type | [nativeloader.go:87](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/nativeloader.go#L87) | `cmd/arcc/app` | none | no |
| `(*NativeLoader).Environment` | method | [nativeloader.go:96](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/nativeloader.go#L96) | `cmd/arcc/app` | none | no |
| `(*NativeLoader).Load` | method | [nativeloader.go:106](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/nativeloader.go#L106) | none | `cmd/arcc/app` | no |
| `ProjectExportFiles` | func | [project_export.go:20](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L20) | none | none | yes |
| `RunProjectExportCommand` | func | [project_export.go:189](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L189) | `cmd/arcc-stdlibmap` | none | no |
| `TargetConfig` | type | [sdkkey.go:16](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/sdkkey.go#L16) | `teststdlibmap`, `cmd/arcc/app` | `cmd/arcc`, `cmd/arcc/app` | no |
| `ClassifierRule` | type | [sdkkey.go:39](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/sdkkey.go#L39) | none | none | yes |
| `CanonicalClassifierText` | func | [sdkkey.go:50](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/sdkkey.go#L50) | `teststdlibmap`, `cmd/arcc/app` | none | no |
| `GenerationDescriptor` | type | [sdkkey.go:70](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/sdkkey.go#L70) | `teststdlibmap`, `cmd/arcc/app` | none | no |
| `ClassifierHash` | func | [sdkkey.go:93](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/sdkkey.go#L93) | none | none | yes |
| `DeriveSDKKey` | func | [sdkkey.go:106](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/sdkkey.go#L106) | `teststdlibmap`, `cmd/arcc/app` | none | no |
| `PackageEntry` | type | [stdlibmap.go:90](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L90) | `cmd/arcc/app` | `cmd/arcc/app` | no |
| `PackageOracle` | type | [stdlibmap.go:102](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L102) | `cmd/arcc/app` | `cmd/arcc/app` | no |
| `PackageOracleFunc` | type | [stdlibmap.go:109](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L109) | `cmd/arcc/app` | `cmd/arcc/app` | no |
| `(PackageOracleFunc).Packages` | method | [stdlibmap.go:112](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L112) | none | none | yes |
| `IsInternalPath` | func | [stdlibmap.go:117](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L117) | none | none | yes |
| `ExplicitPackageList` | func | [stdlibmap.go:131](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L131) | none | none | yes |
| `NormalizePackageList` | func | [stdlibmap.go:148](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L148) | none | none | yes |
| `NativeStdPackageList` | func | [stdlibmap.go:221](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L221) | `cmd/arcc/app` | none | no |
| `NativeToolchainVersion` | func | [stdlibmap.go:245](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L245) | none | none | yes |

- **Symbol counts**:
  - Total exported symbols: **61**
  - Count used externally (non-test): **20**
  - Count used by tests only: **10**
  - Count internal-only (used only within `stdlibmap`): **31** (over 50% of the surface!)

### 3. Authority inventory (measured)
Measured using `arcc check` with an undeclared-authority probe during native selfcheck staging. All 5 declared capabilities are actively exercised; none is unused.

| Capability | Stdlib symbol referenced | Member symbol (func/method) referencing it | File:line |
| :--- | :--- | :--- | :--- |
| `EXEC` | `os/exec.Command` | `NativeTargetConfig` | [cache.go:353](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L353) |
| `EXEC` | `(*os/exec.Cmd).Output` | `NativeTargetConfig` | [cache.go:353](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L353) |
| `EXEC` | `os/exec.CommandContext` | `NativeStdPackageList` | [stdlibmap.go:222](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L222) |
| `EXEC` | `(*os/exec.Cmd).Output` | `NativeStdPackageList` | [stdlibmap.go:226](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L226) |
| `EXEC` | `os/exec.CommandContext` | `NativeToolchainVersion` | [stdlibmap.go:246](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L246) |
| `EXEC` | `(*os/exec.Cmd).Output` | `NativeToolchainVersion` | [stdlibmap.go:249](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L249) |
| `FILES` | `os.Open` | `openCachedArtifact` | [cache.go:114](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L114) |
| `FILES` | `os.MkdirAll` | `WriteArtifactAtomic` | [cache.go:175](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L175) |
| `FILES` | `os.Open` | `RunGenerateCommand` | [command.go:25](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/command.go#L25) |
| `FILES` | `os.Open` | `RunGenerateCommand` | [command.go:36](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/command.go#L36) |
| `FILES` | `os.Stat` | `targetMatchesPin` | [explicit.go:192](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/explicit.go#L192) |
| `FILES` | `os.CreateTemp` | `writeSyntheticLayout` | [layoutloader.go:66](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L66) |
| `FILES` | `os.Remove` | `writeSyntheticLayout` | [layoutloader.go:71](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L71) |
| `FILES` | `os.MkdirAll` | `ProjectExportFiles` | [project_export.go:27](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L27) |
| `FILES` | `os.Open` | `ProjectExportFiles` | [project_export.go:31](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L31) |
| `FILES` | `os.MkdirAll` | `materializeDir` | [project_export.go:69](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L69) |
| `FILES` | `path/filepath.EvalSymlinks` | `materializeDir` | [project_export.go:88](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L88) |
| `FILES` | `os.Lstat` | `materializeDir` | [project_export.go:93](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L93) |
| `FILES` | `path/filepath.EvalSymlinks` | `materializeDir` | [project_export.go:100](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L100) |
| `FILES` | `os.Stat` | `materializeDir` | [project_export.go:120](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L120) |
| `FILES` | `os.Open` | `copyFileAtomic` | [project_export.go:151](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L151) |
| `FILES` | `os.OpenFile` | `copyFileAtomic` | [project_export.go:157](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L157) |
| `READ_SYSTEM_STATE` | `os.UserCacheDir` | `defaultCacheDir` | [cache.go:66](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L66) |
| `READ_SYSTEM_STATE` | `os.UserCacheDir` | `DefaultCacheRoot` | [cache.go:102](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/cache.go#L102) |
| `READ_SYSTEM_STATE` | `os.Getenv` | `writeSyntheticLayout` | [layoutloader.go:59](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L59) |
| `READ_SYSTEM_STATE` | `os.Environ` | `(*NativeLoader).Environment` | [nativeloader.go:100](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/nativeloader.go#L100) |
| `READ_SYSTEM_STATE` | `path/filepath.Abs` | `materializeDir` | [project_export.go:84](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L84) |
| `READ_SYSTEM_STATE` | `path/filepath.Abs` | `materializeDir` | [project_export.go:104](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L104) |
| `REFLECT` | `encoding/json.Marshal` | `writeSyntheticLayout` | [layoutloader.go:62](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L62) |
| `REFLECT` | `(encoding/json.Decoder).Decode` | `ProjectExportFiles` | [project_export.go:44](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L44) |
| `UNSAFE_POINTER` | `encoding/json.Marshal` | `writeSyntheticLayout` | [layoutloader.go:62](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go#L62) |
| `UNSAFE_POINTER` | `(encoding/json.Decoder).Decode` | `ProjectExportFiles` | [project_export.go:44](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/project_export.go#L44) |

**Functional clusters**:
1. **Pure Map Synthesis and Inventory** (`generate.go` [1234 LOC], `inventory.go` [495 LOC], `sdkkey.go` [119 LOC]): **1848 LOC total** (57% of component LOC). **0 authority call sites!** Guaranteed pure computation.
2. **On-Disk Cache and Native Environment Discovery** (`cache.go` [372 LOC], `nativeloader.go` [138 LOC], `stdlibmap.go` [258 LOC]): ~768 LOC. Capabilities: `EXEC`, `FILES`, `READ_SYSTEM_STATE`. Leaf regarding map synthesis; executes `go env` / `go list` and writes `~/.cache/arcc/stdlibmap`.
3. **CLI Runner & Explicit Inputs** (`command.go` [216 LOC], `explicit.go` [194 LOC]): 410 LOC. Capabilities: `FILES`. CLI argument parsing and target verification.
4. **Layout Loader** (`layoutloader.go` [190 LOC]): 190 LOC. Capabilities: `FILES`, `READ_SYSTEM_STATE`, `REFLECT`, `UNSAFE_POINTER`. Writes temporary synthetic layout json.
5. **Project Export Staging** (`project_export.go` [227 LOC]): 227 LOC. Capabilities: `FILES`, `READ_SYSTEM_STATE`, `REFLECT`, `UNSAFE_POINTER`. Walks export manifests, evaluates symlinks, and copies files.

**Root causes**:
- `EXEC`: Running `go env` via `os/exec.Command` in `cache.go:353` and `stdlibmap.go:222, 246`.
- `FILES`: Cache file operations in `cache.go:114, 175`, CLI input reads in `command.go`, temp files in `layoutloader.go:66, 71`, and export staging in `project_export.go`.
- `READ_SYSTEM_STATE`: `os.UserCacheDir` (`cache.go:66, 102`), `os.Getenv` (`layoutloader.go:59`), `os.Environ` (`nativeloader.go:100`), and `filepath.Abs` (`project_export.go:84, 104`).
- `REFLECT` & `UNSAFE_POINTER`: Standard `encoding/json` encoding/decoding in `layoutloader.go:62` and `project_export.go:44`.

### 4. Internal structure and seams
- **Cluster structure**:
  - `generate.go`, `inventory.go`, and `sdkkey.go` form the pure algorithmic engine.
  - `cache.go`, `nativeloader.go`, and `stdlibmap.go` form the native host toolchain probe.
  - `layoutloader.go` and `project_export.go` form the Bazel package-layout hermetic bridge.
  - `command.go` and `explicit.go` form the command-line orchestration layer.
- **Cross-file unexported references**:
  Remarkably low coupling! Only two unexported identifiers cross file boundaries:
  - `validatePackagePath` ([stdlibmap.go:81](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/stdlibmap.go#L81)) is called from `explicit.go` and `inventory.go`.
  - `initID` ([inventory.go:48](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/inventory.go#L48)) is called from `generate.go`.
- **Cross-file dependency table**:
  - `cache.go` -> `generate.go`, `sdkkey.go`
  - `command.go` -> `cache.go`, `generate.go`, `layoutloader.go`, `sdkkey.go`, `stdlibmap.go`
  - `explicit.go` -> `sdkkey.go`, `stdlibmap.go`
  - `generate.go` -> `inventory.go`, `sdkkey.go`
  - `layoutloader.go` -> `generate.go`, `sdkkey.go`
  - `nativeloader.go` -> `sdkkey.go`
  - `project_export.go` -> `sdkkey.go`
  - `stdlibmap.go` -> `sdkkey.go`
- **Mutable state / init / build tags**:
  - Package-level variables: `initSubPattern` in [generate.go:345](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go#L345) is a compiled `*regexp.Regexp` (read-only). No mutable package state.
  - No `init()` functions anywhere in the package.
  - No build tags on any non-test files.

### 5. Consumers' view
1. `cmd/arcc/app`:
   - Cache & map loading: `OpenCachedMap`, `CachedMapInput`, `DecodeMapArtifact`
   - Target configuration: `NativeTargetConfig`, `TargetConfig`, `NativeEnvironment`, `TargetEnv`
   - Key derivation: `GenerationClassifierRules`, `CanonicalClassifierText`, `DeriveSDKKey`, `GenerationDescriptor`, `RuleVersion`
   - Package oracle: `NativeLoader`, `NativeStdPackageList`, `PackageOracleFunc`, `PackageOracle`, `PackageEntry`, `GenerationInput`
   - Commands: `RunGenerateCommand`
2. `cmd/arcc-stdlibmap`:
   - `RunGenerateCommand`, `RunProjectExportCommand`
3. `internal/teststdlibmap`:
   - `CanonicalClassifierText`, `DeriveSDKKey`, `GenerationClassifierRules`, `GenerationDescriptor`, `RuleVersion`, `TargetConfig`

### 6. Tests
- **In-package test files (`package stdlibmap`)**: 8 files, 2805 LOC
  - `cache_test.go` (586 LOC)
  - `explicit_test.go` (214 LOC)
  - `generate_test.go` (1004 LOC)
  - `inventory_test.go` (500 LOC)
  - `layoutloader_test.go` (77 LOC)
  - `project_export_test.go` (131 LOC)
  - `sdkkey_test.go` (189 LOC)
  - `stdlibmap_test.go` (104 LOC)
- **External test files (`package stdlibmap_test`)**: 2 files, 582 LOC
  - `generate_integration_test.go` (260 LOC, tag `integration && stdlibmap_full`)
  - `integration_test.go` (322 LOC, tag `integration`)
- **Unexported identifiers relied on by tests**:
  - `cache_test.go` tests `openCachedArtifact`, `defaultCacheDir`, and `CacheSeams`.
  - `generate_test.go` tests unexported classification helpers (`rootAggregate`, `dropReason`).
  - `inventory_test.go` tests `validatePackagePath` and unexported `Inventory` structure.
- **Testdata**: No local testdata directory; uses `go/internal/teststdlibmap/testdata`.

### 7. Draft split proposal
- **Proposed Interface Package** (`go/internal/stdlibmap`):
  Retain only the narrow public consumer API (currently 20 symbols, can be trimmed to 10):
  `OpenCachedMap(CachedMapInput) (*gen.StdlibMap, error)`, `DecodeMapArtifact(io.Reader) (*gen.StdlibMap, error)`, `DeriveSDKKey(GenerationDescriptor) (stdlibauthority.SDKKey, error)`, `TargetConfig`, `RuleVersion`, `GenerationClassifierRules() ([]ClassifierRule, error)`, `CanonicalClassifierText([]ClassifierRule) (string, error)`, `NativeTargetConfig() (TargetConfig, error)`.
  All 31 internal-only exported symbols unexported or moved to internal member packages.
- **Proposed Member Packages**:
  - `go/internal/stdlibmap/internal/engine`: Pure computation package owning `generate.go`, `inventory.go`, and `sdkkey.go` (1848 LOC). Declared authority: **NONE** (guaranteed pure).
  - `go/internal/stdlibmap/internal/layout`: Owns `layoutloader.go` (190 LOC).
- **Proposed Authority Sub-components**:
  - `stdlibmap/cache` (or `stdlibmap/hosttoolchain`):
    - Owns `cache.go`, `nativeloader.go`, and `stdlibmap.go` toolchain discovery.
    - Exported API:
      ```go
      func OpenCachedMap(in CachedMapInput) (*gen.StdlibMap, error)
      func NativeTargetConfig() (TargetConfig, error)
      func NativeStdPackageList(ctx context.Context, env []string) ([]PackageEntry, error)
      ```
    - Declared authority: `FILES`, `EXEC`, `READ_SYSTEM_STATE`.
  - `stdlibmap/projectexport`:
    - Owns `project_export.go`.
    - Declared authority: `FILES`, `READ_SYSTEM_STATE`, `REFLECT`, `UNSAFE_POINTER`.
- **Resulting Parent Authority**:
  The parent `stdlibmap` component becomes completely authority-free (or retains only `FILES` for decoding `DecodeMapArtifact` if kept in the parent).
- **Risks**:
  - `internal/teststdlibmap` and `cmd/arcc/app` import paths need updating.
  - In-package tests (`generate_test.go`) would move alongside `engine`.

---

## Component: capslockadapter  (dir: go/internal/capslockadapter, import path github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter)

### 1. Current declaration
- **Bazel `go_component` attrs**: None. Like `stdlibmap`, it is not declared as a `go_component` under Bazel. As stated in [BUILD.bazel:3-7](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/BUILD.bazel#L3-L7), its closure includes `golang.org/x/sys/unix` (via capslock) which uses cgo, failing closed under Bazel. It is declared as `go_library` in [BUILD.bazel:8-19](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/BUILD.bazel#L8-L19).
- **Checked-in manifest** ([component.textproto:1-39](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/component.textproto#L1-L39)):
  - `name`: `"capslockadapter"`
  - `interface_files`: `"generation.go"`
  - `analysis_defeating_policy`: `WARN`
  - `members`:
    - `github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter`
    - `github.com/google/capslock/analyzer`
    - `github.com/google/capslock/interesting`
    - `github.com/google/capslock/proto`
    - `golang.org/x/sys/unix`
    - `github.com/fatih/color`
    - `github.com/mattn/go-colorable`
    - `github.com/mattn/go-isatty`
  - `component_dependencies`: `stdlibauthority`, `protobuf-runtime`, `x-tools`, `hostpolicy` (4 dependencies)
  - `declared_authority`: `FILES`, `READ_SYSTEM_STATE`, `OPERATING_SYSTEM`, `REFLECT`, `RUNTIME`, `SYSTEM_CALLS`, `UNSAFE_POINTER`, `MODIFY_SYSTEM_STATE` (8 capabilities)
- **Files (non-test)**:

| File | LOC | Purpose |
| :--- | :--- | :--- |
| [generation.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go) | 378 | Classifier builder, handle-use reclassifications, builtin pin digester, and Capslock finding normalizer |
| **Total** | **378** | |

### 2. Exported API inventory

| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `GenerationClassifierText` | func | [generation.go:84](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L84) | `stdlibmap` | `stdlibmap` | no |
| `BuiltinClassifierPin` | func | [generation.go:104](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L104) | `stdlibmap` | `stdlibmap` | no |
| `NewGenerationClassifier` | func | [generation.go:206](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L206) | none | `capslockadapter` | yes |
| `MintingAuthority` | func | [generation.go:232](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L232) | `stdlibmap` | none | no |
| `GenerationFinding` | type | [generation.go:243](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L243) | `stdlibmap` | `stdlibmap`, `cmd/arcc/app` | no |
| `GenerationFindings` | func | [generation.go:257](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L257) | none | none | yes |
| `GenerationFindingsForEnv` | func | [generation.go:268](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L268) | `stdlibmap` | `stdlibmap` | no |
| `ReclassifiedHandleUseMethods` | func | [generation.go:356](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L356) | `stdlibmap` | `stdlibmap` | no |
| `CuratedSafeKeys` | func | [generation.go:366](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L366) | `stdlibmap` | none | no |

- **Symbol counts**:
  - Total exported symbols: **9**
  - Count used externally (non-test): **7** (all by `stdlibmap`)
  - Count used by tests only: **1** (`NewGenerationClassifier`)
  - Count internal-only: **1** (`GenerationFindings` — callers use `GenerationFindingsForEnv`)

### 3. Authority inventory (measured)
Measured using `arcc check` with an undeclared-authority probe during native selfcheck staging.

| Capability | Stdlib/third-party symbol referenced | Member symbol referencing it | File:line |
| :--- | :--- | :--- | :--- |
| `REFLECT` | `(reflect.Value).MapKeys` | `classifierContentDigest` | [generation.go:180](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L180) |
| `REFLECT` | `(reflect.Value).MapIndex` | `classifierContentDigest` | [generation.go:183](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L183) |
| `REFLECT` | `(encoding/json.Encoder).Encode` | member `capslock/analyzer` | `analyzer/scan.go:105` |
| `REFLECT` | `(text/template.Template).Execute` | member `capslock/analyzer` | `analyzer/scan.go:109, 115` |
| `FILES` | `os.ReadFile` | member `capslock/analyzer` | `analyzer/compare.go:51` |
| `FILES` | `os.Stdout` | member `capslock/analyzer`, `color` | `analyzer/compare.go:154`, `color.go:46` |
| `FILES` | `(text/template.Template).ParseFS` | member `capslock/analyzer` | `analyzer/scan.go:108, 114` |
| `MODIFY_SYSTEM_STATE` | `os.Stdout`, `os.Stderr` | member `capslock/analyzer`, `color` | `analyzer/compare.go:154`, `color.go:64` |
| `OPERATING_SYSTEM` | `os.Stdout`, `os.Stderr` | member `capslock/analyzer`, `color` | `analyzer/compare.go:154`, `color.go:64` |
| `READ_SYSTEM_STATE` | `os.Environ` | member `capslock/analyzer` | `analyzer/load.go:67` |
| `READ_SYSTEM_STATE` | `os.Getenv` | member `color` | `color.go:22, 40` |
| `READ_SYSTEM_STATE` | `syscall.Getenv` | member `x/sys/unix` | `unix/env_unix.go:14` |
| `RUNTIME` | `runtime.init` | member `x/sys/unix` | `unix/sockcmsg_unix_other.go:10`, `unix/sysvshm_linux.go:9` |
| `SYSTEM_CALLS` | `syscall.Setenv`, `syscall.Exec`, etc. | member `x/sys/unix` | `unix/env_unix.go:18`, `unix/syscall_unix.go:582` |
| `UNSAFE_POINTER` | `(encoding/json.Encoder).Encode`, `ParseFS` | member `capslock/analyzer` | `analyzer/scan.go:105, 108` |
| *Warning* | `AnalysisDefeating` (892 sites) | member `capslock/analyzer` | `analyzer/analyzer.go:136, 244, 291` |

**Root cause analysis**:
- **Critical finding**: In `generation.go` (the adapter itself), **the ONLY authority used is `REFLECT`** ([generation.go:180, 183](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L180)), used by `classifierContentDigest` to inspect unexported map fields of `interesting.Classifier`.
- **ALL OTHER 7 CAPABILITIES** (`FILES`, `MODIFY_SYSTEM_STATE`, `OPERATING_SYSTEM`, `READ_SYSTEM_STATE`, `RUNTIME`, `SYSTEM_CALLS`, `UNSAFE_POINTER`) and all **892 `AnalysisDefeating` warnings** originate exclusively from the third-party member packages (`analyzer`, `unix`, `color`, `colorable`).

### 4. Internal structure and seams
- `generation.go` is an adapter that provides a pure reflection-based digest of Capslock's embedded rules, builds the minting-site overlay text, and invokes `packages.Load` + `analyzer.GetCapabilityInfo`.
- **Package-level state**:
  - `fileHandleUseMethods` ([generation.go:46](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L46)) is an immutable list of `(*os.File)` method strings.
  - `expectedClassifierFields` ([generation.go:119](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/capslockadapter/generation.go#L119)) is an immutable schema descriptor.
  - No mutable state, no `init()`, no build tags.

### 5. Consumers' view
- Sole consumer: `stdlibmap` ([generate.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/generate.go) and [layoutloader.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/stdlibmap/layoutloader.go)).
- Symbols used: `BuiltinClassifierPin`, `CuratedSafeKeys`, `GenerationClassifierText`, `GenerationFinding`, `GenerationFindingsForEnv`, `MintingAuthority`, `ReclassifiedHandleUseMethods`.

### 6. Tests
- **Test files**:
  - `generation_test.go` (176 LOC, `package capslockadapter`)
  - `generation_integration_test.go` (22 LOC, `package capslockadapter`, `//go:build integration`)
- **Unexported identifiers relied on**: `fileHandleUseMethods`, `expectedClassifierFields`, `validateClassifierSchema`, `classifierContentDigest`.
- **Testdata directories**: `testdata/pure`, `testdata/scope`, `testdata/filereader`.

### 7. Draft split proposal
- **Foreign Member Extraction**:
  The third-party members (`github.com/google/capslock/...`, `golang.org/x/sys/unix`, `github.com/fatih/color`, `github.com/mattn/go-colorable`, `github.com/mattn/go-isatty`) should be removed from `capslockadapter` and packaged into an asserted `PACKAGE_SURFACE` wrapper component (e.g. `internal/capslock` or `thirdparty/capslock`), exactly matching `internal/protobufruntime` and `internal/xtools`.
- **Impact on capslockadapter**:
  1. `capslockadapter` would no longer require `analysis_defeating_policy: WARN` (its 892 AnalysisDefeating sites are eliminated).
  2. Its declared authority collapses from **8 capabilities** down to **1 capability (`REFLECT`)**.
  3. If Capslock exposes a public method for classifier content hashing or if a deterministic string serializer is used, even `REFLECT` can be dropped, making `capslockadapter` **authority-free**.
- **Risks**: None. `capslockadapter`'s public interface remains identical; only its component dependencies and member list change.

---

## Component: cli  (manifest: go/cmd/arcc/component.textproto, packages go/cmd/arcc and go/cmd/arcc/app)

### 1. Current declaration
- **Bazel `go_component` attrs**: None. Bazel declares `go_binary("arcc")` embedding `go_library("arcc_lib")` in [go/cmd/arcc/BUILD.bazel:1-18](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/BUILD.bazel#L1-L18) and `go_library("app")` in [go/cmd/arcc/app/BUILD.bazel:3-32](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/BUILD.bazel#L3-L32). No `go_component` is defined under Bazel because its closure contains cgo via `capslockadapter`.
- **Checked-in manifest** ([go/cmd/arcc/component.textproto:1-72](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/component.textproto#L1-L72)):
  - `name`: `"cli"`
  - `interface_style`: `INTERFACE_STYLE_PACKAGE_SURFACE` ("surface emission is package-scoped, so expose both concrete packages through one package-surface boundary")
  - `members`:
    - `github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc`
    - `github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app`
  - `component_dependencies`: `capanalyzer`, `checker`, `artifactio`, `facts`, `goanalysis`, `hostpolicy`, `x-tools`, `manifest`, `packagelayout`, `report`, `stdlibmap`, `stdlibauthority`, `schema`, `surface` (14 dependencies)
  - `declared_authority`: `FILES`, `OPERATING_SYSTEM`, `REFLECT`, `READ_SYSTEM_STATE`, `UNSAFE_POINTER`, `MODIFY_SYSTEM_STATE` (6 capabilities)
- **Warning from selfcheck analysis**:
  `declared component dependency "x-tools" is unused` (reported in `cli.report.json`).
- **Files (non-test)**:

| File | LOC | Purpose |
| :--- | :--- | :--- |
| [main.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go) | 31 | Binary entrypoint: extracts `os.Args`, `os.Stdout`, `os.Stderr`, runs `app.Runner`, and exits |
| [app/app.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go) | 167 | CLI command routing (`check`, `verdict`, `stdlibmap`, `package-imports`, `artifact-shape`) |
| [app/artifactshape.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/artifactshape.go) | 143 | Subcommand `artifact-shape`: opens artifacts, reads goldens, and executes shape comparison |
| [app/check.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go) | 673 | Composition root orchestrating `check`: manifest loading, authority resolution, running pure checker |
| [app/layoutshape.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/layoutshape.go) | 674 | Pure in-memory layout shape snapshotting, normalization, and mismatch comparison |
| [app/package_imports.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/package_imports.go) | 112 | CLI handler for `package-imports` and `package-layout-merge` (delegates to `packagelayout`) |
| [app/stdlibmap.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/stdlibmap.go) | 391 | CLI handler for `stdlibmap generate` and `stdlibmap inspect` |
| [app/verdict.go](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/verdict.go) | 114 | Subcommand `verdict`: validates expected pass/fail against recorded report artifact |
| **Total** | **2305** | (31 in `main`, 2274 in `app`) |

### 2. Exported API inventory
Package `cmd/arcc` (`main.go`) exports 0 symbols (both `main` and `run` are unexported).
Package `cmd/arcc/app` exports exactly 8 symbols:

| symbol | kind | file:line | used by (outside package, non-test) | used by tests only | internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `PackageLoader` | type | [app.go:24](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L24) | none | `cmd/arcc`, `cmd/arcc/app` | no |
| `DependencySurfaceResolver` | type | [app.go:30](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L30) | none | `cmd/arcc/app` | no |
| `Runner` | type | [app.go:39](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L39) | `cmd/arcc` | `cmd/arcc`, `cmd/arcc/app` | no |
| `(*Runner).Run` | method | [app.go:48](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L48) | `cmd/arcc` | `cmd/arcc`, `cmd/arcc/app` | no |
| `AuthorityRequest` | type | [check.go:33](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L33) | none | `cmd/arcc`, `cmd/arcc/app` | no |
| `AuthorityResolver` | type | [check.go:51](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L51) | none | `cmd/arcc`, `cmd/arcc/app` | no |
| `SurfaceInputsLoader` | type | [check.go:55](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L55) | none | `cmd/arcc/app` | no |
| `ArtifactWriter` | type | [check.go:61](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L61) | none | `cmd/arcc/app` | no |

- **Symbol counts**:
  - Total exported symbols: **8**
  - Count used externally (non-test): **2** (`Runner` and `(*Runner).Run`, used only by `main.go:27, 30`)
  - Count used by tests only: **6**
  - Count internal-only: **0**

### 3. Authority inventory (measured)
Measured using `arcc check` with an undeclared-authority probe during native selfcheck staging. All 6 declared capabilities are exercised; no `UNUSED_AUTHORITY` warning.

| Capability | Stdlib symbol referenced | Member symbol referencing it | File:line |
| :--- | :--- | :--- | :--- |
| `MODIFY_SYSTEM_STATE` | `os.Stderr` | `main` | [main.go:23](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go#L23) |
| `MODIFY_SYSTEM_STATE` | `os.Stdout` | `main` | [main.go:23](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go#L23) |
| `OPERATING_SYSTEM` | `os.Stderr` | `main` | [main.go:23](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go#L23) |
| `OPERATING_SYSTEM` | `os.Stdout` | `main` | [main.go:23](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go#L23) |
| `FILES` | `os.Stderr`, `os.Stdout` | `main` | [main.go:23](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go#L23) |
| `FILES` | `os.Open` | `(*Runner).runArtifactShape` | [app/artifactshape.go:64](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/artifactshape.go#L64) |
| `FILES` | `os.Open` | `(*Runner).runArtifactShape` | [app/artifactshape.go:83](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/artifactshape.go#L83) |
| `FILES` | `os.ReadFile` | `(*Runner).runLayoutArtifactShape` | [app/artifactshape.go:103](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/artifactshape.go#L103) |
| `FILES` | `os.ReadFile` | `(*Runner).runLayoutArtifactShape` | [app/artifactshape.go:121](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/artifactshape.go#L121) |
| `FILES` | `os.ReadFile` | `(*Runner).runCheck` | [app/check.go:182](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L182) |
| `FILES` | `os.ReadFile` | `readSurfaceSources` | [app/check.go:469](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L469) |
| `FILES` | `os.Open` | `authorityFromDeclaredMap` | [app/check.go:556](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L556) |
| `FILES` | `os.Open` | `runStdlibMapInspect` | [app/stdlibmap.go:88](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/stdlibmap.go#L88) |
| `READ_SYSTEM_STATE` | `path/filepath.Abs` | `(*Runner).Run` | [app/app.go:113](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L113) |
| `READ_SYSTEM_STATE` | `os.Getwd` | `(*Runner).Run` | [app/app.go:118](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L118) |
| `READ_SYSTEM_STATE` | `path/filepath.Abs` | `(*Runner).runCheck` | [app/check.go:194](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L194) |
| `REFLECT` | `encoding/json.MarshalIndent` | `(*Runner).runCheck` | [app/check.go:302](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L302) |
| `REFLECT` | `(encoding/json.Decoder).Decode` | `parseLayoutArtifact` | [app/layoutshape.go:106](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/layoutshape.go#L106) |
| `REFLECT` | `(encoding/json.Decoder).Token` | `parseLayoutArtifact` | [app/layoutshape.go:109](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/layoutshape.go#L109) |
| `REFLECT` | `encoding/json.MarshalIndent` | `parseLayoutArtifact` | [app/layoutshape.go:118](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/layoutshape.go#L118) |
| `REFLECT` | `(encoding/json.Encoder).Encode` | `canonicalizeLayout` | [app/layoutshape.go:198](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/layoutshape.go#L198) |
| `UNSAFE_POINTER` | `encoding/json.MarshalIndent` | `(*Runner).runCheck` | [app/check.go:302](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/check.go#L302) |
| `UNSAFE_POINTER` | `(encoding/json.Decoder).Decode`, `Token`, `MarshalIndent`, `Encode` | `parseLayoutArtifact`, `canonicalizeLayout` | [app/layoutshape.go:106, 109, 118, 198](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/layoutshape.go#L106) |

**Root cause analysis**:
- **`MODIFY_SYSTEM_STATE` and `OPERATING_SYSTEM`**: Occur **ONLY in `main.go:23`** by passing `os.Stdout` and `os.Stderr` to `run()`. Package `cmd/arcc/app` does NOT contain a single reference to `MODIFY_SYSTEM_STATE` or `OPERATING_SYSTEM`!
- **`FILES`**: Occurs at 8 sites in `app` where files are read directly via `os.Open` or `os.ReadFile` (`artifactshape.go`, `check.go`, `stdlibmap.go`).
- **`READ_SYSTEM_STATE`**: Occurs at 3 sites: resolving current working directory (`os.Getwd` in `app.go:118`) and converting paths (`filepath.Abs` in `app.go:113` and `check.go:194`).
- **`REFLECT` and `UNSAFE_POINTER`**: Induced solely by `encoding/json` formatting for human stdout output (`check.go:302`) and in-memory layout canonicalization (`layoutshape.go`).
- **Pure logic clusters**:
  - `layoutshape.go` (674 LOC): pure in-memory JSON processing. Zero file I/O!
  - `verdict.go` (114 LOC): reads via `artifactio.ReadVerdictGoldenFile` and `artifactio.ReadReportFile` (authority terminates structurally in `artifactio`!). Zero ambient authority!
  - `package_imports.go` (112 LOC): delegates to `packagelayout` (authority terminates structurally!). Zero ambient authority!

### 4. Internal structure and seams
- `app` is the composition root that binds CLI arguments to internal core libraries (`goanalysis`, `checker`, `artifactio`, `packagelayout`, `stdlibmap`, `report`).
- Seams on `Runner`:
  `Loader` ([app.go:40](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L40)), `AuthorityResolver` ([app.go:41](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L41)), `DependencySurfaceResolver` ([app.go:42](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L42)), `SurfaceInputsLoader` ([app.go:43](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L43)), `ArtifactWriter` ([app.go:44](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/app.go#L44)).
- Cross-file unexported references:
  - `parseCheckOptions` in `check.go` is called from `app.go`.
  - `compareLayoutShape`, `isLayoutShapeMismatch`, `snapshotLayoutShape` in `layoutshape.go` are called from `artifactshape.go`.
  - `printUsage` in `app.go` is called from `stdlibmap.go` and `verdict.go`.
  - `version` in `app.go` is called from `check.go`.
- No package-level mutable state; no `init()` functions; no build tags on non-test files.

### 5. Consumers' view
- Outside `cmd/arcc/app`, the **only non-test consumer** in the entire repository is `cmd/arcc/main.go` ([main.go:27, 30](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/main.go#L27)), which uses only:
  - `app.Runner`
  - `(*app.Runner).Run(args []string, stdout, stderr io.Writer) int`

### 6. Tests
- `cmd/arcc`:
  - 1 in-package test: `main_test.go` (55 LOC, `package main`). Tests `run()` directly.
  - 3 external integration tests (`package main_test`, tag `integration`): `cli_integration_test.go` (1984 LOC), `layout_integration_test.go` (1547 LOC), `layout_realsdk_integration_test.go` (164 LOC).
- `cmd/arcc/app`:
  - 3 in-package tests (`package app`): `artifactshape_test.go` (90 LOC), `check_internal_test.go` (210 LOC), `layoutshape_test.go` (288 LOC). Total: 588 LOC.
  - 9 external tests (`package app_test`): `app_test.go` (1426 LOC), `check_integration_test.go` (1509 LOC), `check_test.go` (592 LOC), `self_manifest_parity_test.go` (24 LOC), `stdlibmap_explicit_integration_test.go` (118 LOC), `stdlibmap_integration_test.go` (145 LOC), `stdlibmap_test.go` (295 LOC), `surface_cutover_test.go` (415 LOC), `verdict_test.go` (300 LOC). Total: 4824 LOC.
- No local testdata directory.

### 7. Draft split proposal
- **Retire `PACKAGE_SURFACE`**:
  `PACKAGE_SURFACE` is meant only for third-party wrappers, not first-party code. `cli` should declare an explicit interface package.
- **Proposed Architecture**:
  - `cmd/arcc`: Retain `main.go` as the CLI host runner holding `MODIFY_SYSTEM_STATE`, `OPERATING_SYSTEM`, and `FILES` (via stdout/stderr/args).
  - `cmd/arcc/app`: Declare explicit interface file `app.go` exposing only `Runner` and `Run`.
  - Isolate pure logic:
    - Move `layoutshape.go` (674 LOC) into a pure internal package `internal/layoutshape` (0 authority, or narrow `REFLECT`/`UNSAFE_POINTER` for JSON manipulation).
    - `verdict.go` and `package_imports.go` are already pure and structurally terminate authority in `artifactio` and `packagelayout`.
  - File reading isolation:
    - Direct `os.Open` / `os.ReadFile` in `artifactshape.go`, `check.go`, and `stdlibmap.go` should route through `artifactio` (e.g. `artifactio.ReadFileAtomic`, `artifactio.ReadSources`). Because `artifactio` is a component dependency, this terminates `FILES` authority structurally, leaving `app` authority-free for file operations!
  - Drop unused dependency:
    - Remove unused `x-tools` from `component_dependencies` in [component.textproto](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/component.textproto) (eliminating the selfcheck warning).
- **Resulting declared authority**:
  `app`'s authority drops significantly: `FILES`, `MODIFY_SYSTEM_STATE`, `OPERATING_SYSTEM` are eliminated from `app`; only `READ_SYSTEM_STATE` (`os.Getwd`) remains unless working directory is injected from `main.go`.
