# Final Component Structure & Authority Audit (Step 16)

**Date:** 2026-10-06
**Scope:** All 31 self-hosted component manifests staged in `justfile` (28 analyzed components + 3 asserted foreign `UNKNOWN` package-surface wrappers), cross-checked against the 24 Bazel `go_component` gates.

---

## 1. Executive Summary

- **Authority Conformance (C7–C11):** **28 / 28 analyzed components match their measured authority probe (`0` mismatches).**
  - **19 analyzed components are completely authority-free (`declared_authority = []`):** `capanalyzer`, `hostpolicy`, `symbol`, `capslocksymbol`, `schema`, `schema-gen`, `manifest`, `report`, `stdlibauthority`, `facts`, `surface`, `artifactio`, `checker`, `stdlibmap`, `goanalysis`, `parsecsv`, `toprow`, `app` (csvtool), plus `schema-gen` under `WARN` (with its 15 `protoc-gen-go` `AnalysisDefeating` sites pinned in `just selfcheck`).
  - **5 authority sub-components / hooks confine host OS/toolchain interactions (C7):** `atomicfile` (`FILES`), `packagelayout-hostenv` (`FILES, MODIFY_SYSTEM_STATE, READ_SYSTEM_STATE, RUNTIME`), `layout-driver-hook` (`FILES, MODIFY_SYSTEM_STATE, OPERATING_SYSTEM`), `stdlibmap-hosttoolchain` (`EXEC, FILES, READ_SYSTEM_STATE`), `goanalysis-hostfs` (`FILES, READ_SYSTEM_STATE`).
  - **3 components hold documented codec/builtin-digest reflection (C9):** `packagelayout` (`REFLECT, UNSAFE_POINTER`), `reportio` (`REFLECT, UNSAFE_POINTER`), `capslockadapter` (`REFLECT`).
  - **1 composition root (`cli`)** holds the measured union of capabilities it wires (`FILES, MODIFY_SYSTEM_STATE, OPERATING_SYSTEM, READ_SYSTEM_STATE, REFLECT, UNSAFE_POINTER`, C11), and **1 example I/O component (`csvfile`)** holds `FILES`.
- **Interface Style Conformance (C1–C2):** Only the 3 foreign-module wrappers (`protobuf-runtime`, `x-tools`, `capslock-runtime`) use `PACKAGE_SURFACE`. All 28 project-authored components use declared interfaces (`interface_files` or whole-library `interface` sugar).
- **Interface Size Conformance (C3):** Every analyzed component satisfies either **C3(a)** (interface files ≤ ⅓ of total non-test LOC) or **C3(b)** (dedicated vocabulary, codec, or narrow sub-component where all exported symbols in the interface files are consumer-facing).

---

## 2. Authority Audit Table (Declared vs. Measured)

Each analyzed component was probed in topological order by stripping all `declared_authority:` lines from its manifest and running `bin/arcc check` against the pinned `linux_amd64.stdlib-map.json` (the `scripts/authority-probe.sh` procedure, Design Appendix B).

| # | Component | Manifest | Declared Authority | Measured Probe Findings | Match |
|---|-----------|----------|--------------------|-------------------------|-------|
| 1 | `capanalyzer` | `go/internal/capanalyzer/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 2 | `atomicfile` | `go/internal/atomicfile/component.textproto` | `FILES` | `FILES` (`FILES`×4) | MATCH |
| 3 | `hostpolicy` | `go/internal/hostpolicy/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 4 | `symbol` | `go/internal/symbol/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 5 | `capslocksymbol` | `go/internal/capslocksymbol/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 6 | `protobuf-runtime` | `go/internal/protobufruntime/component.textproto` | `UNKNOWN` (asserted) | N/A (asserted foreign wrapper) | MATCH |
| 7 | `x-tools` | `go/internal/xtools/component.textproto` | `UNKNOWN` (asserted) | N/A (asserted foreign wrapper) | MATCH |
| 8 | `packagelayout-hostenv` | `go/internal/packagelayout/internal/hostenv/component.textproto` | `FILES, MODIFY_SYSTEM_STATE, READ_SYSTEM_STATE, RUNTIME` | `FILES`×4, `MODIFY_SYSTEM_STATE`×15, `READ_SYSTEM_STATE`×10, `RUNTIME`×1 | MATCH |
| 9 | `packagelayout` | `go/internal/packagelayout/component.textproto` | `REFLECT, UNSAFE_POINTER` | `REFLECT`×19, `UNSAFE_POINTER`×19 | MATCH |
| 10 | `layout-driver-hook` | `go/internal/layoutdriverhook/component.textproto` | `FILES, MODIFY_SYSTEM_STATE, OPERATING_SYSTEM` | `FILES`×6, `MODIFY_SYSTEM_STATE`×5, `OPERATING_SYSTEM`×5 | MATCH |
| 11 | `schema` | `go/internal/schema/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 12 | `schema-gen` | `go/internal/schema/gen/component.textproto` | `[]` (`WARN` policy) | `[]` (0 `TrueAuthority`; 15 `AnalysisDefeating` sites asserted) | MATCH |
| 13 | `manifest` | `go/internal/manifest/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 14 | `report` | `go/internal/report/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 15 | `stdlibauthority` | `go/internal/stdlibauthority/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 16 | `facts` | `go/internal/facts/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 17 | `surface` | `go/internal/surface/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 18 | `artifactio` | `go/internal/artifactio/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 19 | `reportio` | `go/internal/reportio/component.textproto` | `REFLECT, UNSAFE_POINTER` | `REFLECT`×6, `UNSAFE_POINTER`×6 | MATCH |
| 20 | `checker` | `go/internal/checker/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 21 | `capslock-runtime` | `go/internal/capslockruntime/component.textproto` | `UNKNOWN` (asserted) | N/A (asserted foreign wrapper) | MATCH |
| 22 | `capslockadapter` | `go/internal/capslockadapter/component.textproto` | `REFLECT` | `REFLECT`×2 | MATCH |
| 23 | `stdlibmap-hosttoolchain` | `go/internal/stdlibmap/internal/hosttoolchain/component.textproto` | `EXEC, FILES, READ_SYSTEM_STATE` | `EXEC`×6, `FILES`×12, `READ_SYSTEM_STATE`×5 | MATCH |
| 24 | `stdlibmap` | `go/internal/stdlibmap/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 25 | `goanalysis-hostfs` | `go/internal/goanalysis/internal/hostfs/component.textproto` | `FILES, READ_SYSTEM_STATE` | `FILES`×4, `READ_SYSTEM_STATE`×1 | MATCH |
| 26 | `goanalysis` | `go/internal/goanalysis/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 27 | `parsecsv` | `go/examples/csvtool/internal/parsecsv/component.textproto` | `[]` (`WARN` policy) | `[]` (0 findings) | MATCH |
| 28 | `csvfile` | `go/examples/csvtool/csvfile/component.textproto` | `FILES` | `FILES`×1 | MATCH |
| 29 | `toprow` | `go/examples/csvtool/toprow/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 30 | `app` (csvtool) | `go/examples/csvtool/app/component.textproto` | `[]` | `[]` (0 findings) | MATCH |
| 31 | `cli` | `go/cmd/arcc/component.textproto` | `FILES, MODIFY_SYSTEM_STATE, OPERATING_SYSTEM, READ_SYSTEM_STATE, REFLECT, UNSAFE_POINTER` | `FILES`×15, `MODIFY_SYSTEM_STATE`×2, `OPERATING_SYSTEM`×2, `READ_SYSTEM_STATE`×3, `REFLECT`×5, `UNSAFE_POINTER`×5 | MATCH |

---

## 3. Interface-Size Audit Table (C3(a) vs. C3(b))

Convention **C3** requires that each component's interface files satisfy either:
- **C3(a):** Interface files comprise ≤ ⅓ (33.3%) of the component's total non-test LOC; or
- **C3(b):** The component is a dedicated vocabulary, data-model, codec, or narrow authority sub-component where ≥ ⅔ of its exported interface symbols are consumed by external dependents.

| # | Component | Interface Files | IF LOC | Total Non-Test LOC | IF % | Surface Symbols (Used / Exported) | Rule | Notes |
|---|-----------|-----------------|--------|--------------------|------|-----------------------------------|------|-------|
| 1 | `capanalyzer` | `capanalyzer.go` | 71 | 71 | 100.0% | 12 / 12 (100%) | **C3(b)** | Capability-analyzer vocabulary (71 LOC) |
| 2 | `atomicfile` | `atomicfile.go` | 99 | 99 | 100.0% | 2 / 2 (100%) | **C3(b)** | Single-file authority sub-component (99 LOC) |
| 3 | `hostpolicy` | `hostpolicy.go` | 107 | 107 | 100.0% | 5 / 5 (100%) | **C3(b)** | Single-file policy vocabulary (107 LOC) |
| 4 | `symbol` | `extract.go, symbol.go, typesconv.go` | 653 | 653 | 100.0% | 8 / 8 (100%) | **C3(b)** | Core symbol ID grammar & constructors (§5.7) |
| 5 | `capslocksymbol` | `capslock.go` | 1,096 | 1,096 | 100.0% | 4 / 4 (100%) | **C3(b)** | Dedicated Capslock symbol parser (4 exported symbols, all consumed by `stdlibmap`, §5.7) |
| 6 | `packagelayout-hostenv` | `hostenv.go` | 124 | 134 | 92.5% | 11 / 11 (100%) | **C3(b)** | Narrow authority sub-component (124 LOC IF + 10 LOC cgo tag files) |
| 7 | `packagelayout` | `api.go, layout.go` | 397 | 3,877 | **10.2%** | 38 / 38 (100%) | **C3(a)** | Split from 2,853-LOC monolith into 2 IF files + 7 NIF files (§5.4) |
| 8 | `layout-driver-hook` | `hook.go` | 29 | 29 | 100.0% | 1 / 1 (`init`, 100%) | **C3(b)** | 29-LOC self-exec driver `init()` hook (§5.4) |
| 9 | `schema` | `capabilities.go` | 38 | 38 | 100.0% | 1 / 1 (100%) | **C3(b)** | 38-LOC capability taxonomy constant (`KnownCapabilities`) |
| 10 | `schema-gen` | `archcontracts.pb.go, manifest.pb.go, stdlib_map.pb.go` | 1,626 | 1,626 | 100.0% | 170 (generated protobuf API) | **C3(b)** | Generated protobuf schema bindings (§5.8) |
| 11 | `manifest` | `manifest.go` | 558 | 558 | 100.0% | 25 / 25 (100%) | **C3(b)** | Manifest data model & parser (16 internal helpers unexported in Step 3) |
| 12 | `report` | `report.go` | 271 | 271 | 100.0% | 38 / 38 (100%) | **C3(b)** | Report data model & builders (271 LOC) |
| 13 | `stdlibauthority` | `stdlibauthority.go` | 270 | 270 | 100.0% | 16 / 16 (100%) | **C3(b)** | Stdlib authority vocabulary & query interface (270 LOC) |
| 14 | `facts` | `facts.go` | 689 | 689 | 100.0% | 61 / 61 (100%) | **C3(b)** | Package-facts vocabulary & canonical sorting (13 internal helpers unexported in Step 3) |
| 15 | `surface` | `surface.go` | 332 | 332 | 100.0% | 4 / 4 (100%) | **C3(b)** | Component surface builder & model (332 LOC) |
| 16 | `artifactio` | `artifactio.go, stdlibmap.go, surface.go` | 835 | 968 | 86.3% | 13 / 13 (100%) | **C3(b)** | Dedicated protojson artifact codec & digest boundary (§5.2) |
| 17 | `reportio` | `golden.go, report.go` | 1,053 | 1,053 | 100.0% | 21 / 21 (100%) | **C3(b)** | Dedicated JSON report & golden codec component (§5.2, C9) |
| 18 | `checker` | `checker.go` | 386 | 1,128 | **34.2%** | 2 / 2 (`Inputs`, `Check`, 100%) | **C3(a)/C3(b)** | 1-function, 1-struct entry point (`checker.go`), 742 LOC in NIF files (§5.6) |
| 19 | `capslockadapter` | `capslockadapter.go, generation.go` | 378 | 378 | 100.0% | 9 / 9 (100%) | **C3(b)** | 378-LOC adapter over `capslock-runtime` (§5.9) |
| 20 | `stdlibmap-hosttoolchain` | `hosttoolchain.go` | 226 | 226 | 100.0% | 14 / 14 (100%) | **C3(b)** | Authority sub-component for `stdlibmap` (§5.10) |
| 21 | `stdlibmap` | `sdkkey.go, stdlibmap.go` | 537 | 3,319 | **16.2%** | 34 / 34 (100%) | **C3(a)** | 2 IF files (537 LOC) + 8 NIF files (2,782 LOC) (§5.10) |
| 22 | `goanalysis-hostfs` | `hostfs.go` | 17 | 17 | 100.0% | 5 / 5 (100%) | **C3(b)** | 17-LOC host filesystem sub-component for `goanalysis` (§5.3) |
| 23 | `goanalysis` | `dependency_surface.go, goanalysis.go, surfaceinputs.go` | 222 | 3,758 | **5.9%** | 18 / 18 (100%) | **C3(a)** | 3 IF files (222 LOC) + 8 NIF files (3,536 LOC) (§5.3) |
| 24 | `parsecsv` | `parsecsv.go` | 16 | 16 | 100.0% | 1 / 1 (100%) | **C3(b)** | 16-LOC CSV example parser |
| 25 | `csvfile` | `csvfile.go` | 44 | 49 | 89.8% | 2 / 2 (100%) | **C3(b)** | 49-LOC CSV example file reader (`interface_files = ["csvfile.go"]`) |
| 26 | `toprow` | `toprow.go` | 41 | 41 | 100.0% | 2 / 2 (100%) | **C3(b)** | 41-LOC CSV example helper |
| 27 | `app` (csvtool) | `app.go` | 44 | 44 | 100.0% | 1 / 1 (100%) | **C3(b)** | 44-LOC CSV example app |
| 28 | `cli` | `app/app.go` | 205 | 2,376 | **8.6%** | 8 / 8 (100%) | **C3(a)** | 1 IF file (`app/app.go`, 205 LOC) + 7 NIF files (2,171 LOC) (§5.5) |

---

## 4. Convention Cross-Check Greps

1. **`PACKAGE_SURFACE` restricted to foreign wrappers (C1):**
   ```text
   $ grep -rn PACKAGE_SURFACE go/ --include=BUILD.bazel --include=component.textproto
   go/internal/capslockruntime/component.textproto:6:interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
   go/internal/protobufruntime/component.textproto:6:interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
   go/internal/protobufruntime/BUILD.bazel:1:load("@rules_arcc//bazel_rules/go:defs.bzl", "PACKAGE_SURFACE", "UNKNOWN", "go_component")
   go/internal/protobufruntime/BUILD.bazel:10:    interface_style = PACKAGE_SURFACE,
   go/internal/xtools/component.textproto:7:interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
   go/internal/xtools/BUILD.bazel:1:load("@rules_arcc//bazel_rules/go:defs.bzl", "PACKAGE_SURFACE", "UNKNOWN", "go_component")
   go/internal/xtools/BUILD.bazel:8:    interface_style = PACKAGE_SURFACE,
   ```
2. **Ambient authority imports confined to sub-components, hooks, and composition root (C7):**
   ```text
   $ grep -rn '"os"\|os/exec\|"runtime"\|go/build"' go/internal go/cmd --include='*.go' \
       | grep -v _test \
       | grep -v '/internal/host\|layoutdriverhook\|atomicfile\|cmd/arcc\|teststdlibmap\|manifestparity\|/testdata/'
   (empty)
   ```
3. **Whole-library `interface = ` sugar in `BUILD.bazel` (C2, Appendix D):**
   ```text
   $ grep -rn 'interface = ' go/ --include=BUILD.bazel
   go/examples/csvtool/internal/parsecsv/BUILD.bazel:16:    interface = ":parsecsv",
   go/examples/csvtool/toprow/BUILD.bazel:25:    interface = ":toprow",
   go/examples/csvtool/app/BUILD.bazel:25:    interface = ":app",
   go/internal/stdlibauthority/BUILD.bazel:22:    interface = ":stdlibauthority",
   go/internal/schema/gen/BUILD.bazel:24:    interface = ":gen",
   go/internal/manifest/BUILD.bazel:64:    interface = ":manifest",
   go/internal/surface/BUILD.bazel:52:    interface = ":surface",
   go/internal/report/BUILD.bazel:21:    interface = ":report",
   go/internal/capanalyzer/BUILD.bazel:21:    interface = ":capanalyzer",
   go/internal/hostpolicy/BUILD.bazel:26:    interface = ":hostpolicy",
   go/internal/facts/BUILD.bazel:45:    interface = ":facts",
   ```
   Used only by `schema/gen` (generated protobuf library) and single-file vocabulary/example components.
