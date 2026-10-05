# Component analysis report template

## Component: artifactio  (dir: go/internal/artifactio, import path github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio)

### 1. Current declaration
- **Bazel `go_component` attrs** (defined in [`go/internal/artifactio/BUILD.bazel:66-84`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/BUILD.bazel#L66-L84)):
  - `name`: `"artifactio_component"`
  - `interface_style`: `"PACKAGE_SURFACE"` (no discrete `interface` library; all exported symbols of member packages form the component interface)
  - `members`: `["//go/internal/artifactio"]`
  - `component_deps`:
    - `"//go/internal/report:report_component"`
    - `"//go/internal/schema:schema_component"`
    - `"//go/internal/symbol:symbol_component"`
    - `"//go/internal/stdlibauthority:stdlibauthority_component"`
    - `"//go/internal/surface:surface_component"`
    - `"//go/internal/protobufruntime:protobuf-runtime"`
  - `declared_authority`: `["FILES", "REFLECT", "UNSAFE_POINTER"]`
  - `visibility`: `["//go:__subpackages__"]`

- **Checked-in manifest** ([`go/internal/artifactio/component.textproto:1-34`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/component.textproto#L1-L34)):
  - `name`: `"artifactio"`
  - `interface_style`: `INTERFACE_STYLE_PACKAGE_SURFACE`
  - `component_dependencies`: `report`, `schema`, `stdlibauthority`, `surface`, `symbol`, `protobuf-runtime`
  - `members`: `"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"`
  - `declared_authority`: `"FILES"`, `"REFLECT"`, `"UNSAFE_POINTER"`

- **Divergence**:
  - The Bazel rule and the textproto manifest agree exactly on interface style (`PACKAGE_SURFACE`), member packages, dependencies, and declared capabilities (`FILES`, `REFLECT`, `UNSAFE_POINTER`).
  - **Docstring discrepancy**: The package-level documentation in [`go/internal/artifactio/artifactio.go:30-32`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go#L30-L32) claims: *"This is a shell component holding FILES, READ_SYSTEM_STATE, MODIFY_SYSTEM_STATE, REFLECT, RUNTIME, SYSTEM_CALLS, and UNSAFE_POINTER for filesystem writes and protobuf unmarshaling."* However, only `FILES`, `REFLECT`, and `UNSAFE_POINTER` are declared in both manifests, and the measured report confirms that `READ_SYSTEM_STATE`, `MODIFY_SYSTEM_STATE`, `RUNTIME`, and `SYSTEM_CALLS` are neither declared nor exercised.

- **Non-test files (9 files, 2,116 LOC)**:

| File | LOC | One-line Purpose |
| :--- | :--- | :--- |
| [`artifactio.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go) | 87 | Component contract documentation, artifact format versions (`SurfaceFormatVersion`, `MapFormatVersion`), size caps (`MaxSurfaceBytes`, `MaxMapBytes`), `ErrUnsupportedVersion`, and canonical JSON compaction/indentation helper (`canonicalJSON`). |
| [`digest.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/digest.go) | 14 | Canonical lowercase hexadecimal SHA-256 digest computation (`digest`). |
| [`golden.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go) | 614 | Shape snapshotting (`Snapshot`), golden comparison (`CompareShape`), and error definitions (`ShapeMismatchError`) for shape testing of report, surface, and stdlib-map artifacts. |
| [`report.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go) | 417 | Canonical JSON serialization/deserialization for persisted conformance reports (`PersistedReport`, `MarshalReport`, `DecodeReport`), golden verdict codec (`MarshalVerdict`, `ReadVerdictGoldenFile`), and bounded report file reader (`ReadReportFile`). |
| [`stdlibmap.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmap.go) | 372 | Canonical serialization (`MarshalMap`), bounded deserialization (`DecodeMap`), semantic invariant validator (`validateMap`), and digest computation (`MapDigest`) for stdlib authority maps. |
| [`stdlibmapreader.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmapreader.go) | 196 | In-memory read-only index view (`stdlibMapReader`) implementing the `stdlibauthority.StdlibAuthority` port over a validated `gen.StdlibMap` artifact. |
| [`surface.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surface.go) | 256 | Canonical serialization (`MarshalSurface`), bounded deserialization (`DecodeSurface`), semantic invariant validation (`validateSurface`), and digest calculation (`SurfaceDigest`) for component surface manifests. |
| [`surfacepaths.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go) | 65 | Companion artifact naming conventions (`SurfacePath`, `ReportPath`), atomic surface writer (`WriteSurface`), and bounded source file reader (`ReadSources`). |
| [`write.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go) | 95 | Atomic file replacement via temporary sibling file, sync, chmod, and rename (`WriteFileAtomic`), with injectable filesystem operation seams (`WriteSeams`). |

---

### 2. Exported API inventory

| Symbol | Kind | File:Line | Used by (outside package, non-test) | Used by tests only | Internal-only |
| :--- | :--- | :--- | :--- | :--- | :--- |
| [`SurfaceFormatVersion`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go#L56) | const | `artifactio.go:56` | — *(cited in comment in `go/internal/surface/derive.go:151`)* | `cmd/arcc/app` (`surface_cutover_test.go:26, 91, 205`) | No |
| [`MapFormatVersion`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go#L59) | const | `artifactio.go:59` | `internal/teststdlibmap` (`pinned.go:65, 115, 162`), `internal/stdlibmap` (`generate.go:158, 317`), `cmd/arcc/app` (`check.go:671`) | — | No |
| [`MaxSurfaceBytes`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go#L61) | const | `artifactio.go:61` | — | — | **Yes** *(used in `surface.go:36`, `golden.go:144`)* |
| [`MaxMapBytes`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go#L63) | const | `artifactio.go:63` | `internal/stdlibmap` (`cache.go:86, 119`) | `internal/stdlibmap` (`cache_test.go:454`), `cmd/arcc/app` (`stdlibmap_test.go:26`) | No |
| [`ErrUnsupportedVersion`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go#L68) | var | `artifactio.go:68` | — | — *(in-pkg `decode_test.go:27, 49`)* | **Yes** *(used in `surface.go:99`, `stdlibmap.go:145`)* |
| [`ShapeKind`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L19) | type | `golden.go:19` | `cmd/arcc/app` (`artifactshape.go:22, 137, 141`) | — | No |
| [`ShapeReport`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L22) | const | `golden.go:22` | — | `cmd/arcc/app` (`artifactshape_test.go:22`) | No |
| [`ShapeSurface`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L23) | const | `golden.go:23` | — | — | **Yes** *(used in `golden.go:84`)* |
| [`ShapeMap`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L24) | const | `golden.go:24` | — | — | **Yes** *(used in `golden.go:90`)* |
| [`ShapeMismatchError`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L30) | type | `golden.go:30` | — | — *(in-pkg `golden_test.go:121, 240, 375`)* | **Yes** *(checked via `IsShapeMismatch`)* |
| [`(*ShapeMismatchError).Error`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L34) | method | `golden.go:34` | — | — | **Yes** *(satisfies `error` interface)* |
| [`IsShapeMismatch`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L42) | func | `golden.go:42` | `cmd/arcc/app` (`artifactshape.go:94`) | — | No |
| [`Snapshot`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L76) | func | `golden.go:76` | `cmd/arcc/app` (`artifactshape.go:138`) | `cmd/arcc/app` (`artifactshape_test.go:22`) | No |
| [`CompareShape`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L105) | func | `golden.go:105` | `cmd/arcc/app` (`artifactshape.go:142`) | — | No |
| [`ReportFormatVersion`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L23) | const | `report.go:23` | — | — | **Yes** *(used in `report.go:58, 69, 116`)* |
| [`MaxReportBytes`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L24) | const | `report.go:24` | — | — | **Yes** *(used in `report.go:101, 165, 366`, `golden.go:125`)* |
| [`MaxVerdictGoldenBytes`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L29) | const | `report.go:29` | — | — | **Yes** *(used in `report.go:390`)* |
| [`ErrMalformedReport`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L36) | var | `report.go:36` | — | `artifactio_test` (`report_test.go:44, 49, 54, 59, 64, 69, 74, 113, 118, 123`) | No |
| [`ErrUnknownVerdict`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L37) | var | `report.go:37` | — | `artifactio_test` (`report_test.go:87, 92`) | No |
| [`ErrVerdictMismatch`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L38) | var | `report.go:38` | — | `artifactio_test` (`report_test.go:97, 102`) | No |
| [`ErrUnsupportedReportVersion`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L39) | var | `report.go:39` | — | `artifactio_test` (`report_test.go:77, 82`) | No |
| [`ErrInvalidVerdictGolden`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L40) | var | `report.go:40` | — | `artifactio_test` (`report_test.go:238, 244, 250, 256, 263`) | No |
| [`PersistedReport`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L49) | type | `report.go:49` | `cmd/arcc/app` (`verdict.go:101`) | `artifactio_test` (`report_test.go`), `cmd/arcc/app` (`check_test.go`, `check_integration_test.go`), `cmd/arcc` (`cli_integration_test.go:1474`) | No |
| [`(PersistedReport).MarshalJSON`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L67) | method | `report.go:67` | `json.Marshal(wire)` inside `json.MarshalIndent` in `MarshalReport` | — | No |
| [`(*PersistedReport).UnmarshalJSON`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L78) | method | `report.go:78` | `json.Unmarshal` | `artifactio_test` (`report_test.go:107, 128`) | No |
| [`MarshalReport`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L151) | func | `report.go:151` | `cmd/arcc/app` (`check.go:442`) | `artifactio_test` (`report_test.go`), `cmd/arcc/app` (`verdict_test.go:16, 234`, `app_test.go:1250`, `surface_cutover_test.go:232, 236`, `artifactshape_test.go:18`), `cmd/arcc` (`cli_integration_test.go:1461`) | No |
| [`DecodeReport`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L164) | func | `report.go:164` | `internal/goanalysis` (`surface_resolver.go:604`) | `artifactio_test` (`report_test.go`), `cmd/arcc/app` (`check_test.go`, `check_integration_test.go`), `cmd/arcc` (`cli_integration_test.go:239, 1474`) | No |
| [`ReadReportFile`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L357) | func | `report.go:357` | `cmd/arcc/app` (`verdict.go:101`) | `artifactio_test` (`report_test.go:183, 203, 213, 222`), `cmd/arcc/app` (`verdict_test.go:257, 261`) | No |
| [`ReadVerdictGoldenFile`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L383) | func | `report.go:383` | `cmd/arcc/app` (`verdict.go:94`) | `artifactio_test` (`report_test.go:231, 238`) | No |
| [`MarshalVerdict`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L408) | func | `report.go:408` | — | `artifactio_test` (`report_test.go:269, 276, 283`), `cmd/arcc/app` (`verdict_test.go:265, 269`) | No |
| [`MarshalMap`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmap.go#L20) | func | `stdlibmap.go:20` | `internal/teststdlibmap` (`pinned.go:136`), `internal/stdlibmap` (`generate.go:164`) | `internal/stdlibmap` (`cache_test.go:581`), `cmd/arcc/app` (`check_internal_test.go:52, 129`) | No |
| [`DecodeMap`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmap.go#L33) | func | `stdlibmap.go:33` | `internal/stdlibmap` (`cache.go:339`, `generate.go:170`), `cmd/arcc/app` (`check.go:561`) | `internal/stdlibmap` (`generate_integration_test.go:193`) | No |
| [`MapDigest`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmap.go#L51) | func | `stdlibmap.go:51` | `internal/stdlibmap` (`generate.go:178`) | — | No |
| [`NewStdlibMapReader`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmapreader.go#L29) | func | `stdlibmapreader.go:29` | `cmd/arcc/app` (`check.go:589`) | — | No |
| [`OpenStdlibMap`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmapreader.go#L74) | func | `stdlibmapreader.go:74` | `internal/teststdlibmap` (`pinned.go:77, 182`), `internal/stdlibmap` (`cache.go:239, 261`) | — | No |
| [`MarshalSurface`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surface.go#L22) | func | `surface.go:22` | `cmd/arcc/app` (`check.go:434`) | `cmd/arcc/app` (`app_test.go:1333`, `surface_cutover_test.go:45, 90, 204, 323`), `cmd/arcc` (`cli_integration_test.go:406`) | No |
| [`DecodeSurface`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surface.go#L35) | func | `surface.go:35` | `internal/goanalysis` (`surface_resolver.go:139`) | `cmd/arcc/app` (`check_test.go:291`, `check_integration_test.go:376, 694, 1109`, `app_test.go:1328`), `cmd/arcc` (`cli_integration_test.go:234, 391`) | No |
| [`SurfaceDigest`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surface.go#L53) | func | `surface.go:53` | — | — *(in-pkg `decode_test.go:281, 290, 298, 308, 319`)* | **Yes** |
| [`SurfacePath`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go#L18) | func | `surfacepaths.go:18` | `internal/goanalysis` (`surface_resolver.go:395`) | `cmd/arcc` (`layout_integration_test.go:197`, `cli_integration_test.go:170, 211, 372, 1396, 1449`), `cmd/arcc/app` (`check_integration_test.go:974`), `artifactio_test` (`surfacepath_test.go`) | No |
| [`ReportPath`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go#L25) | func | `surfacepaths.go:25` | `internal/goanalysis` (`surface_resolver.go:396`) | `cmd/arcc` (`layout_integration_test.go:198`, `cli_integration_test.go:171, 212, 371, 1448`), `cmd/arcc/app` (`check_integration_test.go:975`), `artifactio_test` (`surfacepath_test.go`) | No |
| [`WriteSurface`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go#L38) | func | `surfacepaths.go:38` | — | — *(in-pkg `rework_test.go:257, 281, 314`)* | **Yes** |
| [`ReadSources`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go#L53) | func | `surfacepaths.go:53` | — *(mentioned in comment `app/check.go:462`)* | `artifactio_test` (`surfacepath_test.go:57, 85, 101`) | No |
| [`WriteSeams`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L13) | type | `write.go:13` | — *(parameter to `WriteFileAtomic`)* | `artifactio` (`write_test.go:13`) | No |
| [`WriteFileAtomic`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L29) | func | `write.go:29` | `internal/stdlibmap` (`cache.go:183, 332`), `cmd/arcc/app` (`check.go:528`) | `internal/stdlibmap` (`cache_test.go:538`), `cmd/arcc/app` (`app_test.go:1291`) | No |

**Exported symbol counts**:
- **Total exported symbols**: 44
- **Used externally (non-test)**: 24
- **Used by tests only**: 9
- **Internal-only**: 11

---

### 3. Authority inventory (measured)

- **Measurement from Bazel and native report**:
  - `bazel build //go/internal/artifactio:artifactio_component --output_groups=+arcc` was built successfully.
  - The resulting artifact [`bazel-bin/go/internal/artifactio/artifactio_component.report.json`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel-bin/go/internal/artifactio/artifactio_component.report.json) records:
    ```json
    {
      "format_version": 1,
      "verdict": "pass",
      "report": {
        "component": "artifactio_component",
        "dependencies": [
          { "component": "protobuf-runtime", "provenance": "ASSERTED", "freshness": "BUILD_GRAPH", "authority": "UNKNOWN" },
          { "component": "report_component", "provenance": "CHECKED_PASS", "freshness": "BUILD_GRAPH", "authority": "DECLARED" },
          { "component": "schema_component", "provenance": "CHECKED_FAIL", "freshness": "BUILD_GRAPH", "authority": "DECLARED" },
          { "component": "stdlibauthority_component", "provenance": "CHECKED_PASS", "freshness": "BUILD_GRAPH", "authority": "DECLARED" },
          { "component": "surface_component", "provenance": "CHECKED_PASS", "freshness": "BUILD_GRAPH", "authority": "DECLARED" },
          { "component": "symbol_component", "provenance": "CHECKED_PASS", "freshness": "BUILD_GRAPH", "authority": "DECLARED" }
        ],
        "violations": null,
        "warnings": [
          { "kind": "DEPENDENCY_CHECK_FAILED", "message": "dependency \"schema_component\" failed its conformance check" }
        ]
      }
    }
    ```
  - When measuring actual authority sites by probing with undeclared authority, the checker isolates precisely **18 authority sites** across 3 capabilities (6 `FILES`, 6 `REFLECT`, 6 `UNSAFE_POINTER`).

- **Authority Sites Table**:

| Capability | Stdlib / 3rd-party Symbol Referenced | Member Symbol (func/method) | File:Line |
| :--- | :--- | :--- | :--- |
| `FILES` | `os.Open` | `ReadReportFile` | [`go/internal/artifactio/report.go:358`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L358) |
| `FILES` | `os.Open` | `ReadVerdictGoldenFile` | [`go/internal/artifactio/report.go:384`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L384) |
| `FILES` | `os.CreateTemp` | `WriteFileAtomic` | [`go/internal/artifactio/write.go:37`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L37) |
| `FILES` | `os.Chmod` | `WriteFileAtomic` | [`go/internal/artifactio/write.go:56`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L56) |
| `FILES` | `os.Rename` | `WriteFileAtomic` | [`go/internal/artifactio/write.go:60`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L60) |
| `FILES` | `os.Remove` | `WriteFileAtomic` | [`go/internal/artifactio/write.go:64`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L64) |
| `REFLECT` | `(encoding/json.Encoder).Encode` | `marshalShape` | [`go/internal/artifactio/golden.go:570`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L570) |
| `REFLECT` | `encoding/json.Unmarshal` | `canonicalShapeJSON` | [`go/internal/artifactio/golden.go:579`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L579) |
| `REFLECT` | `encoding/json.Marshal` | `(PersistedReport).MarshalJSON` | [`go/internal/artifactio/report.go:73`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L73) |
| `REFLECT` | `encoding/json.Unmarshal` | `decodePersistedReport` | [`go/internal/artifactio/report.go:109`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L109) |
| `REFLECT` | `encoding/json.Unmarshal` | `decodePersistedReport` | [`go/internal/artifactio/report.go:134`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L134) |
| `REFLECT` | `encoding/json.MarshalIndent` | `MarshalReport` | [`go/internal/artifactio/report.go:152`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L152) |
| `UNSAFE_POINTER` | `(encoding/json.Encoder).Encode` | `marshalShape` | [`go/internal/artifactio/golden.go:570`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L570) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `canonicalShapeJSON` | [`go/internal/artifactio/golden.go:579`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L579) |
| `UNSAFE_POINTER` | `encoding/json.Marshal` | `(PersistedReport).MarshalJSON` | [`go/internal/artifactio/report.go:73`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L73) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `decodePersistedReport` | [`go/internal/artifactio/report.go:109`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L109) |
| `UNSAFE_POINTER` | `encoding/json.Unmarshal` | `decodePersistedReport` | [`go/internal/artifactio/report.go:134`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L134) |
| `UNSAFE_POINTER` | `encoding/json.MarshalIndent` | `MarshalReport` | [`go/internal/artifactio/report.go:152`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L152) |

- **Comprehensive call-site enumeration**:
  - **`encoding/json` call sites (8 total)**:
    1. [`artifactio.go:78`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go#L78): `json.Compact(&compact, raw)` — classified **SAFE** in stdlib map.
    2. [`artifactio.go:82`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go#L82): `json.Indent(&indented, compact.Bytes(), "", "  ")` — classified **SAFE** in stdlib map.
    3. [`report.go:73`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L73): `json.Marshal(wire)` — causes **`REFLECT`**, **`UNSAFE_POINTER`**.
    4. [`report.go:109`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L109): `json.Unmarshal(data, &raw)` — causes **`REFLECT`**, **`UNSAFE_POINTER`**.
    5. [`report.go:134`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L134): `json.Unmarshal(*raw.Report, &parsed)` — causes **`REFLECT`**, **`UNSAFE_POINTER`**.
    6. [`report.go:152`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L152): `json.MarshalIndent(...)` — causes **`REFLECT`**, **`UNSAFE_POINTER`**.
    7. [`golden.go:570`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L570): `encoder.Encode(value)` — causes **`REFLECT`**, **`UNSAFE_POINTER`**.
    8. [`golden.go:579`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L579): `json.Unmarshal(data, &object)` — causes **`REFLECT`**, **`UNSAFE_POINTER`**.
    *(Note: [`golden.go:586, 590`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L586) call `json.Compact` and `json.Indent`, which are SAFE).*
  - **`os`, `io`, `io/fs`, `path/filepath` call sites**:
    - `os.Open`: [`report.go:358, 384`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L358) — causes **`FILES`**.
    - `os.CreateTemp`: [`write.go:37`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L37) — causes **`FILES`**.
    - `os.Chmod`: [`write.go:56`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L56) — causes **`FILES`**.
    - `os.Rename`: [`write.go:60`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L60) — causes **`FILES`**.
    - `os.Remove`: [`write.go:64`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L64) — causes **`FILES`**.
    - `(*os.File).Write`, `Sync`, `Close`, `Name`: [`write.go:42, 48, 52, 80, 89`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L42), [`report.go:362, 388`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L362) — all classified **SAFE** with `provenance: "project-override"` in stdlib map.
    - `fs.ReadFile`: [`surfacepaths.go:58`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go#L58) — classified **SAFE** (`provenance: "proved-pure"`) because `fs.FS` is an interface abstraction.
    - `filepath.Ext`, `filepath.Dir`, `filepath.Base`: [`surfacepaths.go:32`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go#L32), [`write.go:33, 67`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go#L33) — all classified **SAFE** (`provenance: "proved-pure"`).
    - `io.Reader` read loop (`r.Read`, `io.EOF`, `io.ErrNoProgress`): [`surface.go:175, 185, 191`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surface.go#L175) — **SAFE**.

- **Protobuf treatment**:
  - `artifactio` imports `google.golang.org/protobuf/proto` and `google.golang.org/protobuf/encoding/protojson` directly in `artifactio.go`, `surface.go`, and `stdlibmap.go`.
  - In `BUILD.bazel` and `component.textproto`, `artifactio` declares a dependency on `//go/internal/protobufruntime:protobuf-runtime`.
  - `protobuf-runtime` is an asserted component declared with `authority: UNKNOWN` and `interface_style: PACKAGE_SURFACE` covering all `google.golang.org/protobuf/...` packages.
  - In arcc's compositional contract model, references across a declared component dependency boundary *terminate authority structurally*: the authority used inside `protobuf` is charged to `protobuf-runtime`, not to `artifactio`.
  - **Confirmation**: Neither `protojson.Marshal`, `(protojson.UnmarshalOptions).Unmarshal`, nor `proto.Clone` is charged to `artifactio`. Zero capability violations are incurred from protobuf runtime usage.

- **JSON type confinement assessment**:
  - **Could `encoding/json` be confined to a sub-package with a type-specific API?**
    **Yes, absolutely.** No public API needs to accept `any`.
  - **Concrete Go types JSON-marshalled / unmarshalled**:
    1. [`report.ConformanceReport`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L125) and its constituent types:
       - [`report.Location`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L42)
       - [`report.AuthoritySite`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L51)
       - [`report.Finding`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L64)
       - [`report.DependencyBoundary`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L109)
       - [`report.Diagnostics`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L121)
       - *Tag location*: All `json:` tags for these types live directly in package `report` ([`go/internal/report/report.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L43-L122)).
    2. [`wirePersistedReport`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L57): private wire wrapper struct.
       - *Tag location*: Defined in `artifactio` ([`report.go:57-61`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L57-L61)).
    3. Anonymous decoding envelope: `struct { FormatVersion *int; Verdict *string; Report *json.RawMessage }`.
       - *Tag location*: Defined in `artifactio` ([`report.go:104-108`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L104-L108)).
    4. Shape snapshots: `reportShapeSnapshot`, `surfaceShapeSnapshot`, `mapShapeSnapshot`, and nested helper structs.
       - *Tag location*: Defined in `artifactio` ([`golden.go:181-227, 334-361, 452-491`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go#L181)).
    5. `map[string]json.RawMessage`: in `golden.go:578`.
    *(Note: `SurfaceManifest` and `StdlibMap` are protobuf messages generated by `protoc-gen-go` in `go/internal/schema/gen/`. They are encoded/decoded exclusively via `protojson`, not `encoding/json`!)*

- **Functional authority clusters**:
  1. **Atomic File Write (`write.go`)**:
     - LOC: ~95 lines
     - Capabilities: `FILES` (4 sites: `os.CreateTemp`, `os.Chmod`, `os.Rename`, `os.Remove`)
     - Leaf: **Yes** (depends only on stdlib `os`, `path/filepath`, `errors`, `fmt`).
     - Minimal API for sub-package:
       ```go
       type WriteSeams struct { ... }
       func WriteFileAtomic(path string, data []byte, mode os.FileMode, seams *WriteSeams) error
       ```
  2. **File Reading (`report.go`)**:
     - LOC: ~55 lines (`ReadReportFile`, `ReadVerdictGoldenFile`)
     - Capabilities: `FILES` (2 sites: `os.Open`)
     - Leaf: Calls `boundedRead` and `DecodeReport`.
     - Minimal API for sub-package (combining with atomic write into an fs sub-component):
       ```go
       func ReadFileBounded(path string, maxBytes int64) ([]byte, error)
       ```
  3. **Report JSON Codec (`report.go`)**:
     - LOC: ~135 lines (`PersistedReport`, `MarshalReport`, `DecodeReport`, `decodePersistedReport`)
     - Capabilities: `REFLECT`, `UNSAFE_POINTER` (4 sites each)
     - Leaf: Depends on `go/internal/report` model and unexported sorting helpers.
     - Minimal API for sub-package:
       ```go
       func MarshalReport(r report.ConformanceReport) ([]byte, error)
       func DecodeReport(data []byte) (PersistedReport, error)
       ```
  4. **Shape Golden Comparison (`golden.go`)**:
     - LOC: ~614 lines (`Snapshot`, `CompareShape`, `canonicalShapeJSON`)
     - Capabilities: `REFLECT`, `UNSAFE_POINTER` (2 sites each)
     - Leaf: Depends on artifact decoding functions.
     - Minimal API for sub-package:
       ```go
       func Snapshot(kind ShapeKind, r io.Reader) ([]byte, error)
       func CompareShape(kind ShapeKind, artifact io.Reader, golden io.Reader) error
       ```

- **Root causes of declared capabilities**:
  - `FILES`: Caused strictly by `os.Open` (in `report.go`), and `os.CreateTemp`, `os.Chmod`, `os.Rename`, `os.Remove` (in `write.go`).
  - `REFLECT`: Caused strictly by `encoding/json.Marshal`, `encoding/json.MarshalIndent`, `encoding/json.Unmarshal`, and `(encoding/json.Encoder).Encode` in `report.go` and `golden.go`.
  - `UNSAFE_POINTER`: Root-caused by the identical 6 `encoding/json` call sites as `REFLECT` (in the Go stdlib authority map, `encoding/json`'s internal number formatting and reflect value traversal paths reference `unsafe.Pointer`).
  - **Unexercised docstring capabilities**: `READ_SYSTEM_STATE`, `MODIFY_SYSTEM_STATE`, `RUNTIME`, and `SYSTEM_CALLS` mentioned in `artifactio.go:30-32` comments are not declared and not exercised.

---

### 4. Internal structure and seams

- **Cohesive clusters**:
  1. *Pure Surface Manifest Handling*: [`surface.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surface.go), [`surfacepaths.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go) (paths only), [`digest.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/digest.go), [`artifactio.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio.go).
  2. *Pure Stdlib Map Handling & Indexing*: [`stdlibmap.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmap.go), [`stdlibmapreader.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmapreader.go).
  3. *Report Codec & Verdicts*: [`report.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go).
  4. *Shape Snapshot Testing*: [`golden.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden.go).
  5. *Filesystem I/O*: [`write.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write.go), [`surfacepaths.go:38`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepaths.go#L38) (`WriteSurface`), [`report.go:357, 383`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report.go#L357) (`ReadReportFile`, `ReadVerdictGoldenFile`).

- **Shared unexported identifiers across files**:
  - `canonicalJSON` (defined in `artifactio.go:72`): called in `surface.go:27, 58`, `stdlibmap.go:25, 56`.
  - `digest` (defined in `digest.go:11`): called in `surface.go:62`, `stdlibmap.go:60`.
  - `boundedRead` (defined in `surface.go:157`): called in `stdlibmap.go:34`, `report.go:366, 390`, `golden.go:110, 125, 144, 163`.
  - `decodeInto` (defined in `surface.go:202`): called in `stdlibmap.go:39`.
  - `sortStrings` (defined in `surface.go:211`): called in `stdlibmap.go:89, 93`.
  - `rejectDuplicates` (defined in `surface.go:216`): called in `stdlibmap.go:163, 360`.
  - `validPackagePath` (defined in `surface.go:227`): called in `stdlibmap.go:179`.
  - `replaceManifestExt` (defined in `surfacepaths.go:31`): called in `surfacepaths.go:19, 26`.
  - `readCanonicalReport`, `readCanonicalSurface`, `readCanonicalMap` (defined in `golden.go:124, 143, 162`): called in `golden.go:79, 85, 91`.

- **Cross-file dependency table**:

| File / Cluster | References (unexported / local symbols) |
| :--- | :--- |
| `surface.go` | `artifactio.go` (`SurfaceFormatVersion`, `canonicalJSON`), `digest.go` (`digest`) |
| `stdlibmap.go` | `artifactio.go` (`MapFormatVersion`, `canonicalJSON`), `digest.go` (`digest`), `surface.go` (`boundedRead`, `decodeInto`, `sortStrings`, `rejectDuplicates`, `validPackagePath`) |
| `stdlibmapreader.go` | `stdlibmap.go` (`validateMap`, `symPackage`), `surface.go` (`sortStrings`) |
| `surfacepaths.go` | `surface.go` (`MarshalSurface`), `write.go` (`WriteFileAtomic`) |
| `report.go` | `surface.go` (`boundedRead`) |
| `golden.go` | `artifactio.go` (`MaxReportBytes`, `MaxSurfaceBytes`, `MaxMapBytes`), `surface.go` (`boundedRead`, `DecodeSurface`, `MarshalSurface`), `stdlibmap.go` (`DecodeMap`, `MarshalMap`), `report.go` (`DecodeReport`, `MarshalReport`, `PersistedReport`) |
| `write.go` | *(none — completely self-contained leaf)* |
| `digest.go` | *(none — completely self-contained leaf)* |

- **Pure computation vs. environment interaction**:
  - **Pure computation**: `digest.go`, `surface.go`, `stdlibmap.go`, `stdlibmapreader.go`, `artifactio.go`, `surfacepaths.go` (`SurfacePath`, `ReportPath`), `golden.go` (snapshot generation and diffing).
  - **Environment interaction**: `write.go` (filesystem atomic write), `report.go` (`ReadReportFile`, `ReadVerdictGoldenFile`), `surfacepaths.go` (`WriteSurface`, `ReadSources`).

- **Mutable state, init(), build tags**:
  - **Package-level mutable state**: None. All top-level variables are immutable error sentinels (`ErrUnsupportedVersion`, `ErrMalformedReport`, `ErrUnknownVerdict`, `ErrVerdictMismatch`, `ErrUnsupportedReportVersion`, `ErrInvalidVerdictGolden`).
  - **`init()` functions**: None.
  - **Build-tagged files**: None.

---

### 5. Consumers' view

| External Consumer Package | Symbols Used | Role / Purpose |
| :--- | :--- | :--- |
| `go/internal/teststdlibmap` | `MapFormatVersion`, `MarshalMap`, `OpenStdlibMap` | Constructs pinned test stdlib authority maps in tests/fixtures. |
| `go/internal/stdlibmap` | `MapFormatVersion`, `MaxMapBytes`, `MarshalMap`, `DecodeMap`, `MapDigest`, `OpenStdlibMap`, `WriteFileAtomic` | Serializes, normalizes, caches, and digests generated stdlib authority maps; writes cache files atomically. |
| `go/internal/goanalysis` | `DecodeSurface`, `DecodeReport`, `SurfacePath`, `ReportPath` | Resolves dependency companion surface/report artifacts by path convention and parses them for analysis. |
| `go/cmd/arcc/app` | `MapFormatVersion`, `DecodeMap`, `NewStdlibMapReader`, `MarshalSurface`, `MarshalReport`, `ReadReportFile`, `ReadVerdictGoldenFile`, `WriteFileAtomic`, `ShapeKind`, `IsShapeMismatch`, `Snapshot`, `CompareShape` | CLI execution commands: `arcc check` (reading maps, writing surfaces/reports), `arcc verdict` (reading reports/goldens), and `arcc shape` (snapshotting/comparing goldens). |

---

### 6. Tests

- **Test Files Inventory (9 files, 2,610 LOC)**:
  - **In-package (`package artifactio`)** — 7 files:
    1. [`artifactio_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/artifactio_test.go) (241 LOC): Tests round-tripping of surfaces and stdlib maps, canonical JSON indentation, and format version gates.
    2. [`decode_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/decode_test.go) (334 LOC): Tests bounded reader limits, duplicate item rejection, and malformed inputs.
    3. [`golden_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/golden_test.go) (384 LOC): Tests shape snapshot generation and canonical comparisons across report, surface, and map artifacts.
    4. [`rework_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/rework_test.go) (347 LOC): Tests crash resilience, atomic writes, and filesystem fault tolerance.
    5. [`stdlibmap_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmap_test.go) (147 LOC): Tests stdlib map normalization, semantic invariants, and hashing.
    6. [`stdlibmapreader_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/stdlibmapreader_test.go) (348 LOC): Tests `stdlibMapReader` conformance with the `stdlibauthority.StdlibAuthority` port.
    7. [`write_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/write_test.go) (118 LOC): Tests `WriteFileAtomic` error handling and cleanup using injected `WriteSeams`.
  - **External package (`package artifactio_test`)** — 2 files:
    1. [`report_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/report_test.go) (576 LOC): Black-box tests for `PersistedReport`, `MarshalReport`, `DecodeReport`, `ReadReportFile`, `ReadVerdictGoldenFile`, and all report error sentinels.
    2. [`surfacepath_test.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/surfacepath_test.go) (115 LOC): Black-box tests for `SurfacePath`, `ReportPath`, and `ReadSources`.

- **Reliance on unexported identifiers**:
  - [`decode_test.go:236`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/decode_test.go#L236) directly invokes the unexported function `boundedRead(reader, 3)` via a test probe `boundedReadProbe`.
  - All other tests across the 7 in-package test files test exported identifiers.
- **Testdata directories**: None (fixtures are dynamically instantiated in memory or written to `t.TempDir()`).

---

### 7. Draft split proposal (brief; the architect will decide)

- **Proposed Interface Package**:
  - **Package**: `github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio` (converted from `PACKAGE_SURFACE` to a minimal interface library).
  - **Exported Symbols in Interface**:
    - Constants: `MapFormatVersion`, `SurfaceFormatVersion`, `MaxMapBytes`, `MaxSurfaceBytes`, `MaxReportBytes`
    - Formats/Codecs: `MarshalSurface`, `DecodeSurface`, `SurfaceDigest`, `SurfacePath`, `ReportPath`
    - Stdlib Map Codecs: `MarshalMap`, `DecodeMap`, `MapDigest`, `OpenStdlibMap`, `NewStdlibMapReader`
    - Report Codecs: `PersistedReport`, `MarshalReport`, `DecodeReport`, `ReadReportFile`, `ReadVerdictGoldenFile`, `MarshalVerdict`
    - Shape Golden: `ShapeKind`, `ShapeReport`, `ShapeSurface`, `ShapeMap`, `IsShapeMismatch`, `Snapshot`, `CompareShape`
    - File Writing: `WriteFileAtomic`, `WriteSeams`
  - **Declared Authority**: **`NONE`** (pure coordinator).

- **Proposed Authority Sub-components & Member Packages**:
  1. **`fsutil` / `fsio` Sub-component** (e.g. `go/internal/artifactio/internal/fsutil`):
     - **Purpose**: Encapsulates all ambient filesystem operations (`os.CreateTemp`, `os.Chmod`, `os.Rename`, `os.Remove`, `os.Open`).
     - **Files**: `write.go` + file-reading helper.
     - **Declared Authority**: `["FILES"]`
     - **Exact Minimal API**:
       ```go
       package fsutil

       type WriteSeams struct {
           CreateTemp func(dir, pattern string) (*os.File, error)
           Write      func(f *os.File, data []byte) error
           Sync       func(f *os.File) error
           Close      func(f *os.File) error
           Chmod      func(path string, mode os.FileMode) error
           Rename     func(oldPath, newPath string) error
           Remove     func(path string) error
       }

       func WriteFileAtomic(path string, data []byte, mode os.FileMode, seams *WriteSeams) error
       func ReadFileBounded(path string, maxBytes int64) ([]byte, error)
       ```
     - *Parent integration*: Parent `ReadReportFile` and `ReadVerdictGoldenFile` call `fsutil.ReadFileBounded` and pass bytes to pure `DecodeReport`. The parent holds **zero** filesystem authority.

  2. **`reportjson` Sub-component** (e.g. `go/internal/artifactio/internal/reportjson`):
     - **Purpose**: Encapsulates all `encoding/json` reflection-bearing serialization for reports.
     - **Files**: `report.go` JSON marshaling/unmarshaling and canonical sorting logic (`wirePersistedReport`, `decodePersistedReport`).
     - **Declared Authority**: `["REFLECT", "UNSAFE_POINTER"]`
     - **Exact Minimal API (Type-Specific, No `any`)**:
       ```go
       package reportjson

       type PersistedReport struct {
           FormatVersion int
           Verdict       report.Verdict
           Report        report.ConformanceReport
       }

       func MarshalReport(r report.ConformanceReport) ([]byte, error)
       func DecodeReport(data []byte) (PersistedReport, error)
       ```

  3. **`shape` Member / Sub-component** (e.g. `go/internal/artifactio/internal/shape`):
     - **Purpose**: Isolates `golden.go` test-golden snapshotting and comparison.
     - **Declared Authority**: `["REFLECT", "UNSAFE_POINTER"]` (if declared as a sub-component, or grouped under `reportjson`).
     - **Exact API**:
       ```go
       package shape

       type ShapeKind string
       const (
           ShapeReport  ShapeKind = "report"
           ShapeSurface ShapeKind = "surface"
           ShapeMap     ShapeKind = "stdlib-map"
       )
       type ShapeMismatchError struct { Detail string }
       func (e *ShapeMismatchError) Error() string
       func IsShapeMismatch(err error) bool
       func Snapshot(kind ShapeKind, r io.Reader) ([]byte, error)
       func CompareShape(kind ShapeKind, artifact io.Reader, golden io.Reader) error
       ```

  4. **`artifactutil` Internal Shared Package** (e.g. `go/internal/artifactio/internal/artifactutil`):
     - **Purpose**: Pure shared helpers currently living in `surface.go` and `digest.go` (`boundedRead`, `sortStrings`, `rejectDuplicates`, `validPackagePath`, `digest`, `canonicalJSON`).
     - **Declared Authority**: `NONE` (pure computation).

- **Flagged Risks & Mitigations**:
  - **`decode_test.go:236` calling `boundedRead`**: Moving `boundedRead` to `artifactutil` allows in-package tests or an internal test package to continue testing the bounded reader probe without keeping it in the parent interface.
  - **Import Cycles**: None. `fsutil` is a leaf (depends only on stdlib `os`, `path/filepath`, `errors`, `fmt`). `reportjson` depends on `go/internal/report` and `artifactutil`. `artifactio` interface coordinates between `fsutil`, `reportjson`, and `shape`.
  - **Protobuf Boundary**: `protobuf-runtime` remains an external asserted component with `authority: UNKNOWN`. The pure protobuf marshaling (`protojson`, `proto.Clone`) in `surface.go` and `stdlibmap.go` stays authority-free.
