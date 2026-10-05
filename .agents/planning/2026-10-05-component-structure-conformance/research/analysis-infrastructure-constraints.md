# Infrastructure Analysis: Constraints for Adding Sub-Components and Splitting Packages

This report documents the architectural, Starlark/Bazel, and Go-side infrastructure constraints for adding sub-components and splitting packages in the `arcc` repository.

---

## 1. Component Nesting

### Native and Bazel Support for Nested Components
Nested sub-components—such as `go/internal/foo/internal/bar/component.textproto` under `go/internal/foo`—are **supported natively and under Bazel**, provided both parent and child declare explicit `members`.

#### Bazel Rule Semantics
- **Explicit Test Fixture**: [`bazel_rules/go/tests/testdata/nested/BUILD.bazel:12-22`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/tests/testdata/nested/BUILD.bazel#L12-L22) explicitly documents and tests this architecture:
  > *"A component dependency may be rooted inside the consumer's package subtree; directory nesting no longer describes ownership."*
- **Analysis Test**: [`bazel_rules/go/tests/component_tests.bzl:246-259`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/tests/component_tests.bzl#L246-L259) runs `_nested_component_root_allowed_test`, verifying that `nested_component`'s closure excludes the child component's packages.
- **Exclusion of Covered Subtrees**: In [`bazel_rules/go/private/component.bzl:871-880`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/component.bzl#L871-L880), any package belonging to a target in `component_deps` is registered in `covered[pkg.importpath]`. During member classification ([`bazel_rules/go/private/component.bzl:910-916`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/component.bzl#L910-L916)), covered packages are excluded from the parent's member set.

#### Go Native Loader Semantics
- **Pattern Construction with Explicit Members**: In [`go/internal/goanalysis/goanalysis.go:130-137`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L130-L137) and [`go/internal/goanalysis/surfaceinputs.go:67-74`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/surfaceinputs.go#L67-L74):
  - When `len(req.Members) > 0`, `patterns` consists only of the explicitly declared `Members` plus interface package patterns. The loader does **not** use `./...`.
  - Only when `len(req.Members) == 0` does the loader fall back to `patterns = []string{"./..."}`, which would recursively traverse subdirectories.
  - Member verification in [`go/internal/goanalysis/goanalysis.go:208-214`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/goanalysis.go#L208-L214) checks `packageIsDeclaredMember(p, req.Members)`.
- **Root Disjointness Checks in Tests vs Production**:
  - In [`go/internal/manifest/manifest_test.go:607-626`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest_test.go#L607-L626), `TestSelfHostingManifests` asserts pairwise disjointness via `isSubdirOrEqual` across 4 hardcoded self-hosting components (`capanalyzer`, `facts`, `report`, `checker`).
  - This check exists **only** in that unit test. Neither [`go/internal/manifest/manifest.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest.go) nor the production loaders enforce root disjointness.
- **Inventory Discovery**:
  - In [`go/internal/manifestparity/ownership_inventory.go:67-109`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory.go#L67-L109), `DiscoverProductionManifests` walks `productionInventoryRoots` (`go/internal`, `go/cmd`, `go/examples`).
  - It only excludes `testdata` directories ([`ownership_inventory.go:421-425`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory.go#L421-L425)).
  - It enforces unique component names ([`ownership_inventory.go:93-100`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory.go#L93-L100)), but does not prohibit directory nesting. Nested components are explicitly exercised in [`go/internal/manifestparity/ownership_inventory_test.go:17-21`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory_test.go#L17-L21) (`go/cmd/tool/deep/nested/...`, `go/examples/example/deeper/...`).

### Go `internal/` Restrictions and Bazel Visibility
1. **Go Language Rules**:
   - Under standard Go toolchain rules (`cmd/compile`), packages under `.../foo/internal/bar` are importable only by code rooted at `.../foo`.
   - Parent package `go/internal/foo` can import `go/internal/foo/internal/bar`.
   - Sibling internal packages outside `go/internal/foo` (e.g., `go/internal/checker`) are rejected by the Go compiler if they attempt to import `go/internal/foo/internal/bar`.
2. **Bazel Visibility Requirements**:
   - As documented in [`go/internal/artifactio/BUILD.bazel:18-23`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/artifactio/BUILD.bazel#L18-L23), [`go/internal/hostpolicy/BUILD.bazel:8-13`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/hostpolicy/BUILD.bazel#L8-L13), [`go/internal/symbol/BUILD.bazel:14-17`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/symbol/BUILD.bazel#L14-L17), [`go/internal/manifest/BUILD.bazel:24-27`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/BUILD.bazel#L24-L27), [`go/internal/surface/BUILD.bazel:12-15`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/BUILD.bazel#L12-L15), and [`go/internal/schema/BUILD.bazel:17-21`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/schema/BUILD.bazel#L17-L21):
     The macro `go_component` instantiates `go_component_rule` from [`bazel_rules/go/private/component.bzl`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/component.bzl#L1180-L1185). Member and interface labels are resolved against typed attributes (`attr.label_list` and `attr.label`).
   - Therefore, any `go_library` passed to `go_component` must declare:
     ```python
     visibility = [
         "//bazel_rules:__subpackages__",
         "//go:__subpackages__",
     ]
     ```
     (or the appropriate restricted scope under `//go/...` plus `//bazel_rules:__subpackages__`).

---

## 2. Interface Granularity Under Bazel

### Single Library and No File-Subset Selection
- **Single Target Requirement**:
  - In [`bazel_rules/go/defs.bzl:246-251`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/defs.bzl#L246-L251), `interface` is defined as `attr.label(...)` (scalar, mandatory for declared interface style).
  - [`bazel_rules/go/defs.bzl:160-167`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/defs.bzl#L160-L167) validates that `interface != None` for declared-style components.
- **Source Expansion**:
  - In [`bazel_rules/go/private/component.bzl:996-1002`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/component.bzl#L996-L1002):
    ```python
    interface_files = []
    if interface:
        interface_files = sorted([
            runfiles_path(ctx, src)
            for src in go_library_srcs(interface)
            if src.extension == "go"
        ])
    ```
  - **No Subset Selection**: Every `.go` source file belonging to the `interface` target is unconditionally added to `interface_files`. Neither `go_component` nor `component.bzl` provides any attribute to filter or select a subset of files from `interface`. To expose only a subset of files as the interface, they must be split into their own separate `go_library`.

### Can `package main` Be an Interface?
**No.** A `go_library` whose package is `main` cannot be an interface:
1. **Empty Import Path in Adapter**: In [`bazel_rules/go/private/go_adapter.bzl:86-88`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/go_adapter.bzl#L86-L88) and [`bazel_rules/go/private/go_adapter.bzl:317-320`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/go_adapter.bzl#L317-L320):
   ```python
   if not importpath:
       # `main` packages and other unimportable libraries are never closure
       # members: nothing can depend on them by import path.
       return None
   ```
2. **Omission from Package Closure**: In [`bazel_rules/go/private/aspect.bzl:41-44`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/aspect.bzl#L41-L44), `go_target_info` returns `None`, so `ArccPackageInfo` receives no direct package node.
3. **Analysis-Time Failure**: In [`bazel_rules/go/private/component.bzl:894-896`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/component.bzl#L894-L896) and [`bazel_rules/go/private/component.bzl:918-922`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/bazel_rules/go/private/component.bzl#L918-L922):
   `interface_importpath` evaluates to `""`, is omitted from `members`, and triggers:
   ```python
   fail(("component %s: its own interface package %s is covered by a component_dep, " +
         "which would leave the component with nothing to check.") % ...)
   ```
4. **Go Semantic Constraint**: In Go, `package main` cannot be imported by any other package. The repo documents this limitation explicitly in [`go/cmd/arcc/component.textproto:2-6`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/component.textproto#L2-L6), which requires `cli` to use `INTERFACE_STYLE_PACKAGE_SURFACE` because `main` cannot be an interface.

---

## 3. Repositories and Inventories to Update When Adding or Splitting Components

When a new `go_component` is added or an existing one is split, the following locations across the repository must be updated:

| Location | File & Line Citations | Invariants & Requirements |
| :--- | :--- | :--- |
| **`justfile` Selfcheck Recipe** | [`justfile:125-147`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/justfile#L125-L147) | New component must be added via `selfcheck_stage_component "$arcc_bin" "$map_path" <path>/component.textproto [fail]` in **topological dependency order**. |
| **`justfile` Component Counts** | [`justfile:63-71`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/justfile#L63-L71), [`justfile:73-87`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/justfile#L73-L87) | Comments documenting "20 analyzed production components" and "16 checked component gates" must be incremented. |
| **Parity Test Arguments** | [`go/cmd/arcc/app/BUILD.bazel:71-94`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/BUILD.bazel#L71-L94) | `self_manifest_parity_test` `args`: add `$(rootpaths //path:component)` and `$(rootpath //path:component.textproto)`. |
| **Parity Test Runfiles** | [`go/cmd/arcc/app/BUILD.bazel:95-118`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/app/BUILD.bazel#L95-L118) | `self_manifest_parity_test` `data`: add `//path:component` and `//path:component.textproto`. |
| **Parity Runner Matcher** | [`go/internal/manifestparity/manifestparity.go:32-59`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/manifestparity.go#L32-L59) | Pairs generated `<name>.component.textproto` with checked-in `component.textproto` by package directory name (`filepath.Base(filepath.Dir(f))`). |
| **Manifest Inventory Discovery** | [`go/internal/manifestparity/ownership_inventory.go:26-30`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory.go#L26-L30) | Components must reside beneath `productionInventoryRoots` (`go/internal`, `go/cmd`, `go/examples`). |
| **Unique Component Name** | [`go/internal/manifestparity/ownership_inventory.go:93-100`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory.go#L93-L100) | `seenNames` enforces that no two manifests in the repository declare the same component name (`DuplicateComponentNameError`). |
| **Foreign Wrapper Invariant** | [`go/internal/manifestparity/ownership_inventory.go:237-242`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory.go#L237-L242), [`ownership_inventory.go:253-277`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory.go#L253-L277), [`ownership_inventory.go:285-311`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_inventory.go#L285-L311) | Any package under `google.golang.org/protobuf/` must be owned by `protobuf-runtime`; any package under `golang.org/x/mod/`, `golang.org/x/sync/`, `golang.org/x/tools/` must be owned by `x-tools`. No component may claim multiple owners. |
| **Single Project Package Ownership** | [`go/internal/manifestparity/ownership_test.go:85-128`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_test.go#L85-L128) | `TestProjectPackagesSingleOwner`: if a split touches `artifactio`, `manifest`, `schema`, `stdlibauthority`, or `symbol`, the `wantOwner` map must be updated. |
| **Artifactio Dependency Inventory** | [`go/internal/manifestparity/ownership_test.go:140-194`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_test.go#L140-L194) | `TestArtifactioDeclaresCoreComponentDependencies`: `want` map defines exact declared component dependencies for `artifactio`. |
| **Acyclic Dependency Graph** | [`go/internal/manifestparity/ownership_test.go:468-493`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_test.go#L468-L493) | `TestPackageLayoutHasOneBoundaryAndAcyclicConsumers`: dynamically walks all checked-in manifests and enforces that the component dependency graph is **acyclic**. |
| **Pure Packages Layering Guard** | [`go/internal/manifestparity/ownership_test.go:529-570`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/ownership_test.go#L529-L570) | `TestPurePackagesDoNotImportArtifactIO`: `purePackages` must never import `internal/artifactio`. Any new pure sub-component should be added to `purePackages`. |
| **X-Tools Closure Parity** | [`go/internal/manifestparity/x_tools_ownership_test.go:17-49`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/x_tools_ownership_test.go#L17-L49), [`x_tools_ownership_test.go:93`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifestparity/x_tools_ownership_test.go#L93) | Exact retained 31 packages; consumers list (`goanalysis`, `capslockadapter`, `stdlibmap`). |
| **Legacy Self-Hosting Disjointness Test** | [`go/internal/manifest/manifest_test.go:574-626`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/manifest/manifest_test.go#L574-L626) | `TestSelfHostingManifests` tests `capanalyzer`, `facts`, `report`, `checker`. If any of these 4 are split or nested, update the test's `expected` table. |
| **README Documentation** | [`README.md:818`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/README.md#L818), [`README.md:851`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/README.md#L851), [`README.md:859-867`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/README.md#L859-L867) | Counts ("16 checked component gates", "20 analyzed production components") and explicit lists of component names. |
| **CLI Dependency List** | [`go/cmd/arcc/component.textproto:9-64`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/cmd/arcc/component.textproto#L9-L64) | If a sub-component is consumed by the CLI application, it must be declared in `component_dependencies`. |
| **Consumer BUILD/Manifest Files** | e.g. [`go/internal/goanalysis/BUILD.bazel:48-60`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/goanalysis/BUILD.bazel#L48-L60) | Any component importing the new package must add the component target to `component_deps` (Bazel) and `component.textproto` (native). |

---

## 4. Consumption of `PACKAGE_SURFACE` vs Interface-Style Dependencies

### Boundary Classification Findings
- **Implementation**: [`go/internal/checker/boundary.go:153-178`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L153-L178) and [`go/internal/checker/classify.go:131-145`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go#L131-L145).
- **`PACKAGE_SURFACE` Consumption**:
  In [`boundary.go:164-166`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L164-L166):
  ```go
  if dep.InterfaceStyle == manifest.InterfaceStylePackageSurface {
      return ReferenceDecision{Status: ReferenceDeclaredDependency, Dependency: dep.Component}, nil
  }
  ```
  Every exported symbol belonging to any member package owned by the dependency is permitted.
- **Interface-Style Consumption & Violations**:
  In [`boundary.go:167-176`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L167-L176):
  ```go
  symbols := make(map[facts.SymbolID]bool, len(dep.Symbols))
  for _, sym := range dep.Symbols {
      symbols[sym] = true
  }
  if !symbols[edge.Referent] {
      return ReferenceDecision{
          Status:     ReferenceCallsUndeclaredInterface,
          Dependency: dep.Component,
      }, nil
  }
  ```
  If a consumer references a symbol in a package owned by an interface-style dependency that is not present in `dep.Symbols`, the checker emits **`CALLS_UNDECLARED_INTERFACE`** ([`go/internal/report/report.go:21`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/report/report.go#L21)).

### `METHOD_OUTSIDE_INTERFACE` Constraint
- **Implementation**: [`go/internal/checker/checker.go:75-113`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go#L75-L113).
- **Mechanism**:
  ```go
  for _, sym := range pkg.ExportedSymbols {
      if sym.Kind == "method" {
          recvType := cleanReceiverType(sym.Receiver)
          declFile, exists := typeDeclFiles[recvType]
          if exists && interfaceFiles[declFile] {
              if !interfaceFiles[sym.File] {
                  violations = append(violations, report.Finding{
                      Kind:    report.MethodOutsideInterface,
                      Message: fmt.Sprintf("exported method %q with receiver %q declared in non-interface file %q", sym.Name, sym.Receiver, sym.File),
                      Location: report.Location{File: sym.File},
                  })
              }
          }
      }
  }
  ```
- **Architectural Constraint**:
  If a type `T` is declared in an interface file (`interfaceFiles[declFile] == true`), **every exported method** with receiver `T` or `*T` **must also be defined in an interface file**. Defining an exported method of `T` in an internal/implementation file in the same package or member package triggers `METHOD_OUTSIDE_INTERFACE`.
  *Rationale*: In Go, any caller receiving `T` can invoke all exported methods of `T`. Splitting methods across non-interface files would allow unadvertised implementation code to leak across the architectural interface.

### Exported Types Defined in Member Packages
- **Can Consumers Call Their Methods?**: **No.**
- **Mechanism**:
  - In [`go/internal/surface/derive.go:124-130`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/derive.go#L124-L130), a declared-interface component's surface manifest contains `symbols = symbolIDs(symbol.ExtractSurface(in.InterfaceFiles, in.InterfaceInfo))`.
  - The surface manifest includes all member packages in `Packages` ([`derive.go:137`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/surface/derive.go#L137)), but `Symbols` includes **only** symbols declared in `InterfaceFiles`.
  - If an interface function returns a concrete type `*impl.Foo` defined in a member package, and the consumer calls `foo.Method()`, the compiler creates a `ReferenceEdge` with `ReferentPackage: "path/to/impl"` and `Referent: "(*path/to/impl.Foo).Method"`.
  - In [`boundary.go:160-176`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/boundary.go#L160-L176), the dependency claims package `path/to/impl`, but `dep.Symbols` does not contain `(*path/to/impl.Foo).Method`.
  - The call is flagged as **`CALLS_UNDECLARED_INTERFACE`**.
- **Design Rule**: Interface functions must return Go `interface` types (e.g., standard interfaces or interfaces declared in interface files) rather than concrete types declared in internal member packages.

---

## 5. Authority Attribution

### Boundary Termination vs Member Authority
1. **Boundary Termination (Dependencies)**:
   - In [`go/internal/facts/facts.go:42-56`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/facts/facts.go#L42-L56), `References` and `Imports` are extracted **only from member packages** of the component being checked.
   - In [`go/internal/checker/classify.go:121-178`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go#L121-L178):
     - References into stdlib are checked against the stdlib authority map and classified into `out.Authority`.
     - Valid references into `component_dependencies` return `ReferenceDeclaredDependency`. They do **not** contribute to `out.Authority`.
     - Imports of dependency packages merely record the dependency as used ([`classify.go:177`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go#L177): `used[dep] = true`).
     - Authority exercised within a dependency is charged to that dependency's component and is **never** charged to the caller. Authority terminates structurally at the component boundary.
2. **Member Authority**:
   - Every member package declared in `members` (or within the interface package) has its source ASTs scanned.
   - Any stdlib authority exercised by any member package generates a `TrueAuthority` observation in `out.Authority`.
   - In [`go/internal/checker/aggregate.go`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/aggregate.go), all capability observations are evaluated against the component's `declared_authority`. Any capability used in a member package that is not in `declared_authority` produces an `AUTHORITY_VIOLATION`.

### Unused Dependency Rules
- **Implementation**: [`go/internal/checker/checker.go:134-150`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go#L134-L150).
- **Rule**:
  A dependency is marked used if any member package imports at least one package owned by that dependency ([`classify.go:173-178`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/classify.go#L173-L178)).
- **Sub-Component Used by Parent**:
  - If the parent component imports a package from the sub-component, the sub-component is marked used (`usedDeps[dep.Name] == true`).
  - If the parent declares the sub-component in `component_dependencies` but never imports any package from it, arcc emits a warning:
    `[UNUSED_DEPENDENCY] declared component dependency "<name>" is unused` ([`checker.go:145-149`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/go/internal/checker/checker.go#L145-L149)).

---

## 6. Native Staging Order (`justfile` / `scripts`)

### How Staging Order Is Defined
- In [`justfile:109-147`](file:///usr/local/google/home/xtof/git/ono-sendai-labs/architectural-contracts-compositional-next/justfile#L109-L147), `just selfcheck` creates an isolated staging directory in `/tmp` and executes a strictly ordered sequence of `selfcheck_stage_component` and `selfcheck_asserted_component` commands:
  ```makefile
  selfcheck_stage_component "$arcc_bin" "$map_path" internal/capanalyzer/component.textproto; \
  selfcheck_stage_component "$arcc_bin" "$map_path" internal/hostpolicy/component.textproto; \
  selfcheck_stage_component "$arcc_bin" "$map_path" internal/symbol/component.textproto; \
  selfcheck_asserted_component internal/protobufruntime/component.textproto; \
  selfcheck_asserted_component internal/xtools/component.textproto; \
  selfcheck_stage_component "$arcc_bin" "$map_path" internal/packagelayout/component.textproto; \
  selfcheck_stage_component "$arcc_bin" "$map_path" internal/schema/component.textproto fail; \
  selfcheck_stage_component "$arcc_bin" "$map_path" internal/manifest/component.textproto; \
  ...
  ```
- **Topological Ordering Requirement**:
  Under native execution, `arcc check` locates dependency surfaces directly on disk adjacent to their manifests (or via relative paths in `component_dependencies`). Because a consumer cannot be checked without the valid surface artifacts of its dependencies, **dependencies must be staged before their consumers**.
- **Adding a Component Requires Editing an Ordered List**:
  Adding or splitting any component **always requires adding a new staging line to `justfile:125-147` in exact topological order**.

---

## 7. Command Execution and Verification Report

The following commands were executed during this analysis:

1. `git status` — Exited 128 (repo uses Jujutsu / `jj`, not a git directory directly).
2. `pwd && ls -la` — Exited 0 (confirmed repo root layout).
3. `mkdir -p /usr/local/google/home/xtof/.../scratch/infra/` — Exited 0 (created scratch directory).
4. `find bazel_rules/go/tests/testdata/ -type d` — Exited 0 (identified testdata fixtures).
5. `find go/ -name "*surface*"` — Exited 0 (located surface handling modules).
6. `grep -rn ...` — Exited 0 (searched Starlark rules, Go modules, proto schemas, and docs).
7. `timeout 300 bazel query //go/cmd/arcc/app:self_manifest_parity_test` — Exited 0 (verified Bazel query execution; waited for lock on Bazel server, then succeeded).
8. `cd go && go test -v ./internal/manifestparity -run TestProjectPackagesSingleOwner` — Exited 0 (verified parity tests pass natively).
9. `bash scripts/selfcheck-staging-test.sh` — Exited 0 (`selfcheck staging verdict regression passed`).

**Sandbox Status**: No commands were blocked by the sandbox. All operations completed cleanly.
