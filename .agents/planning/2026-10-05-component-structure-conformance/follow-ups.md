# Follow-ups from Component Structure Conformance (2026-10-06)

This file records the items explicitly deferred out of scope by [`design/detailed-design.md` §7](design/detailed-design.md) and [`implementation/plan.md` Step 16](implementation/plan.md).

## 1. Report artifact as protobuf (`proto/report.proto` + `protojson`)

- **Why deferred:** Retiring `REFLECT` and `UNSAFE_POINTER` from `reportio`, `packagelayout`, and `cli` (`go/cmd/arcc/app`) requires migrating the report, golden-report, and package-layout JSON schemas from `encoding/json` to `google.golang.org/protobuf/encoding/protojson` (whose `REFLECT` and `UNSAFE_POINTER` usage sits behind the asserted `protobuf-runtime` `PACKAGE_SURFACE` boundary; see design §4.2 and convention C9).
- **Current state:** `reportio` (`go/internal/reportio`) isolates report and golden JSON serialization from `artifactio` (which is now authority-free). `packagelayout` and `cli` carry explicit C9 comments documenting why `encoding/json` remains in their own schema codecs.
- **Next step:** Separate schema-migration plan introducing `proto/report.proto` (and optionally `proto/packagelayout.proto`) and switching `reportio` / `packagelayout` / `cli` to `protojson`.

## 2. Purity follow-ups (mutable package-level state)

Conventions C7–C11 and the authority analyzer track ambient capability reachability (`FILES`, `EXEC`, `READ_SYSTEM_STATE`, `MODIFY_SYSTEM_STATE`, `OPERATING_SYSTEM`, `RUNTIME`, `REFLECT`, `UNSAFE_POINTER`), not in-memory package-level variable mutation (design F6). The following purity cleanups remain as follow-ups:

1. **`go/internal/hostpolicy` package-level hooks:** `hostpolicy` exposes package-level hook variables used by tests and host policy configuration. Threading a `Policy` value explicitly through callers would eliminate the package-level state.
2. **`go/internal/packagelayout` active-layout globals and `CheckMu`:** `CheckMu`, `activeLayout`, `activeLayoutPath`, and `activeWorkspaceDir` (in `driver.go` / `api.go`) serialize process-wide `GOPACKAGESDRIVER` environment activation during self-exec driver loads. Passing an explicit driver context where `go/packages` permits it would narrow this global state.
3. **`go/internal/goanalysis` phase/scan observer seams:** Test-only observer hooks in `goanalysis` can be replaced by options on `Loader` / `LoadInput` if further decoupling is desired.

## 3. `go/internal/goanalysis/fixture_test.go` coupling to `checker` internals

- **Why deferred:** In Step 6, `checker`'s component interface was narrowed to `interface_files = ["checker.go"]` (80 LOC, 1 exported function `Check`). Eight internal symbols (`ClassifyCapability`, ` DeduplicateViolations`, `CheckUnusedAuthority`, `CheckTransitiveAuthority`, `SortViolations`, `SortWarnings`, `CheckNamespaceMismatch`, `CheckSymbolResolvability`) were unexported.
- **Current state:** Seven internal helper functions in `go/internal/checker/{boundary.go,classify.go}` remain exported at the Go language level solely because `go/internal/goanalysis/fixture_test.go` (a `go_test` target, which is not analyzed as a component consumer) calls them directly. Under C4, these functions live in non-interface files (`boundary.go`, `classify.go`), so no component consumer can call them without triggering `CALLS_UNDECLARED_INTERFACE`, and each carries a `// Exported for goanalysis/fixture_test.go only; not part of the checker component interface.` comment.
- **Next step:** Rewrite the sub-phase assertions in `go/internal/goanalysis/fixture_test.go` to assert through `checker.Check` (or move the unit-level phase tests into `go/internal/checker/checker_test.go`), then unexport the remaining 7 functions in `boundary.go` and `classify.go`.

## 4. `go/cmd/arcc-stdlibmap` component manifest

- **Why deferred:** `go/cmd/arcc-stdlibmap` is a 25-line thin Bazel tool binary entrypoint (`main.go` calling `stdlibmap.RunGenerateCommand` / `stdlibmap.RunProjectExportCommand`) and had no component manifest prior to this refactoring (design §7).
- **Next step:** Optionally add a native `component.textproto` for `go/cmd/arcc-stdlibmap` if tool-binary entrypoints are brought into `just selfcheck`.
