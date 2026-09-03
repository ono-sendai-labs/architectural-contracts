# Spike — loading member packages from source and dependencies from gc export data

**Date:** 2026-09-02
**Method:** direct reading of `golang.org/x/tools@v0.48.0/go/packages` and
`go/gcexportdata` (via `$GOMODCACHE`), plus a working prototype: a fixture module
(`dep`, `third`, `member`), a fake `GOPACKAGESDRIVER` executable, and a harness that
calls `packages.Load` and asserts on the resulting `types.Info`. All code lives under
the scratchpad spike directory (not committed); this note captures the findings.

## Question

Can `go/packages`, talking to an external `GOPACKAGESDRIVER`, parse and type-check
only a component's own ("member") packages from source while obtaining types for every
imported package — stdlib and non-stdlib — from compiled gc export data, with no
dependency source files present at all? If so, exactly which `LoadMode` and driver
response shape make that happen, and what does the driver have to supply?

**Short answer: yes**, with
`NeedName|NeedFiles|NeedCompiledGoFiles|NeedImports|NeedTypes|NeedSyntax|NeedTypesInfo`
(no `NeedDeps`) — but only if the driver's `Packages` response enumerates the **entire**
transitive import graph (identity + `Imports` edges) reachable from the root, and
supplies a real `ExportFile` for **every non-root package in that graph**, not just the
ones the member's code actually references. Omitting either is fatal: a missing graph
node causes `go/packages` to **panic** (not return an error); a missing `ExportFile` on
a reachable node causes a reported, non-panicking `Error` on that package and
`IllTyped` on everything importing it, transitively.

## Setup

- Fixture module `fixture` (`go 1.26`) with three packages: `dep` (exported func
  `Hello`, exported interface `Greeter` with method `Greet`, exported struct `Config`
  with exported field `Name`, exported var `DefaultConfig`, exported const `Version`,
  an unexported `stdout = os.Stdout`, and an `init` calling `os.Getenv`); `third`
  (trivial, blank-imported by `member`); `member` (imports `dep` and `os`; takes
  `dep.Hello` as a func value without calling it, calls a `Greeter` obtained from
  `dep.NewGreeter()` — an interface method call, reads `dep.DefaultConfig.Name` — a
  field access, reads `dep.Version` — a const, and calls `os.Stdout.WriteString`).
- Export data for `dep`, `third`, and every stdlib package `member` transitively pulls
  in (58 packages total, `os` down through `runtime`, `internal/*`, etc.) was produced
  with `go list -export -f '{{.Export}}' <pattern>` — this forces a compile into the
  local build cache and prints the resulting object/archive path. No `go build` of
  `member` itself was ever run.
- A fake driver (`driver/main.go`) ignores the request's patterns/mode and always
  replies with the contents of a pre-built `DriverResponse` JSON file (built by a small
  Python generator that shells out to `go list -json -deps ./member` for graph shape
  and `go list -export` per package for `ExportFile`, filtering which packages get an
  `ExportFile` per experiment). `GOPACKAGESDRIVER` pointed at the compiled driver
  binary; `packages.Config.Dir` pointed at the fixture module.
- The harness (`harness/main.go`) calls `packages.Load` and walks
  `TypesInfo.Uses`/`Selections` to confirm every reference above resolved to an object
  whose `Pkg().Path()` is `dep`/`os`, that the `Greet` call resolved to a `*types.Func`
  whose receiver's underlying type is `*types.Interface`, and that `Config.Name`
  resolved to a `*types.Var` with `IsField()==true`. It also asserts, for `dep`/`os`/
  `third`, that `Syntax`/`GoFiles` are empty and `TypesInfo` is `nil` (never parsed)
  while `Types.Complete()==true` (full type information still obtained).

## Findings

### 1. How `refine()` decides export-data vs. source (`go/packages/packages.go`)

Two booleans, computed per package in `refine()` (`packages.go:785`), drive everything:

```go
// packages.go:802
exportDataInvalid := len(ld.Overlay) > 0 || pkg.ExportFile == "" && pkg.PkgPath != "unsafe"
// packages.go:805
needtypes := (ld.Mode&(NeedTypes|NeedTypesInfo) != 0 && (rootIndex >= 0 || ld.Mode&NeedDeps != 0))
// packages.go:808-811
needsrc := ((ld.Mode&(NeedSyntax|NeedTypesInfo) != 0 && (rootIndex >= 0 || ld.Mode&NeedDeps != 0)) ||
    (ld.Mode&(NeedTypes|NeedTypesInfo) != 0 && exportDataInvalid)) && pkg.PkgPath != "unsafe"
```

The `NeedDeps` gate only decides whether a *non-root* package additionally gets full
source treatment because the caller wants deep results; it is **not** what forces
source-loading in general. The clause that forces it for *any* package, root or not,
whenever `NeedTypes`/`NeedTypesInfo` is requested, is `exportDataInvalid`: if a
package's `ExportFile` is empty, `go/packages` unconditionally sets `needsrc = true` as
a fallback, since there's no other way to obtain its types. The surprise: this applies
uniformly to every package in the graph, not just ones actually referenced — see
finding 3.

A second, separate propagation pass runs during the postorder DFS that materializes the
import graph (`packages.go:895`):

```go
// Complete type information is required for the immediate dependencies
// of each source package.
if lpkg.needsrc && ld.Mode&NeedTypes != 0 {
    for _, ipkg := range lpkg.Imports {
        ld.pkgs[ipkg.ID].needtypes = true
    }
}
```

This is what makes `dep`/`third`/`os` get `needtypes=true` even without `NeedDeps`:
`member` is a root with `needsrc=true` (because `NeedSyntax`/`NeedTypesInfo` was
requested for it), so its **direct** imports get `needtypes` forced on, regardless of
`NeedDeps`. `needsrc` itself is *not* propagated by this clause — only `needtypes` is.
`needsrc` instead propagates **upward** through the `visit()` DFS return value
(`packages.go:872-877`, `visit(lpkg, imp)` returning `imp.needsrc`): if anything
reachable below a package needs source, that need bubbles up to every ancestor. That
upward bubbling is the mechanism behind finding 3.

In `loadPackage()` (`packages.go:1046`), the actual dispatch is:

```go
if !lpkg.needsrc {
    ld.loadFromExportData(lpkg)   // packages.go:1082-1090
    return
}
...
if ld.Config.Mode&NeedTypes != 0 && len(lpkg.CompiledGoFiles) == 0 && lpkg.ExportFile != "" {
    // "sources missing" — falls back to export data anyway (packages.go:1183)
    appendError(Error{"-", fmt.Sprintf("sources missing for package %s", lpkg.ID), ParseError})
    _ = ld.loadFromExportData(lpkg)
    return
}
```

So: `needsrc=false` → clean export-data load, no error. `needsrc=true` with no
`CompiledGoFiles` but a non-empty `ExportFile` → a **reported** error
(`"sources missing for package %s"`) plus a best-effort export-data fallback (this is
why the "min" load mode below still worked once the graph had `needsrc=false` for
`dep`/`os`; it is also why the harness fails with this message the moment any
transitively-reachable package lacks `ExportFile`, per finding 3).

### 2. `NeedTypesInfo` without `NeedDeps`: complete `Uses`/`Selections`/`Defs` for roots only

Confirmed by the prototype. With
`LoadMode = NeedName|NeedFiles|NeedCompiledGoFiles|NeedImports|NeedTypes|NeedSyntax|NeedTypesInfo`
(no `NeedDeps`), `packages.Load(cfg, "fixture/member")` returns one root package,
`member`, whose `TypesInfo` is non-nil and whose `Uses`/`Selections` fully resolve
every reference in the fixture (func value, interface method, struct field, const, and
a direct stdlib reference) to objects in the correct package. `dep`, `third`, and `os`
all come back with `TypesInfo == nil`, `Syntax == nil`, `GoFiles == nil`, and
`Types.Complete() == true` — i.e., `newTypesInfo()` (`packages.go:1330`) is only ever
populated on the source-parsing path (`loadPackage`, after the `needsrc` branch), which
export-data packages never reach. This matches `NeedTypesInfo`'s doc comment ("adds
TypesInfo") applying per-package, gated by the same `needsrc` flag as syntax.

`go/packages` sent the driver `mode: 8655`, which decodes to the seven bits above plus
`NeedModule` (`8192`), auto-added by `impliedLoadMode()` (`packages.go:1580`) because
`NeedTypes` was set (used to pick a `types.Config.GoVersion`; our driver never set
`Package.Module`, so `go/packages` fell back to `DriverResponse.GoVersion` via
`lpkg.goVersion`, since `ld.externalDriver == true` — see `packages.go:1301-1305`).
`NeedImports` was already explicit and unaffected.

### 3. A dependency with neither `ExportFile` nor source — and the graph-completeness surprise

Two distinct empirical results, both important for the design:

- **Missing `ExportFile` on a reachable package → reported error, not a crash.** If a
  package in the driver's `Packages` response has no `ExportFile` and no
  `CompiledGoFiles`, `needsrc` becomes `true` for it (finding 1's `exportDataInvalid`
  clause) and it hits the `"sources missing for package %s"` path, producing a
  `packages.Error` that `packages.PrintErrors`/caller-side error checks will surface,
  and it marks that package (and everything importing it) `IllTyped`. This propagates:
  a supplied graph of 58 packages (member's full transitive closure, computed by
  `go list -json -deps ./member`) with `ExportFile` on only the three packages `member`
  actually imports (`dep`, `third`, `os`) **failed** — `os` and `dep` themselves ended
  up with `needsrc=true` because `needsrc` bubbles upward from any needsrc-true
  descendant (finding 1), and every one of `os`'s ~54 further transitive stdlib
  dependencies lacked `ExportFile` in that experiment. Supplying `ExportFile` for the
  **entire** 58-package closure fixed it — the load then succeeded purely from export
  data, confirmed by deleting `dep`'s and `third`'s `.go` files entirely (`mv`'d aside)
  before the load and re-running: `packages.Load` still succeeded, `TypesInfo`
  resolution stayed correct, and no source-file read was attempted (verified by the
  files being physically absent).
- **A driver response that omits graph nodes for a package's own transitive imports
  causes a hard `log.Panicf`, not a recoverable error.** A second experiment supplied a
  minimal 4-package graph (`member`, `dep`, `third`, `os`) where `os` was listed with
  **no `Imports` field at all** (the driver didn't tell `go/packages` that `os` itself
  imports `internal/bytealg`, `syscall`, etc., even though `os`'s real export data
  internally names those packages by path). `loadFromExportData`'s `view` map
  (`packages.go:1475`, comment at 1518: *"the view must contain every existing package
  that might possibly be mentioned by the current package — its transitive closure"*)
  is built strictly from `lpkg.Imports`, which in turn was built strictly from what the
  driver reported. When `gcexportdata.Read` for `os` needed a `*types.Package` for a
  path not in the view, `go/packages` panicked:
  `log.Panicf("...unexpected new packages during load of %s", lpkg.PkgPath)`
  (`packages.go:1571`) — taking the whole process down (goroutine panic, not a returned
  `error`).

  Net implication: the driver's `Packages` list must enumerate the complete transitive
  import graph reachable from the root — every package identity and every `Imports`
  edge, all the way down to `unsafe` — and (per the bullet above) essentially all of
  those nodes currently need real `ExportFile` content too, not just identity. gc
  export data since Go 1.20 (unified IR) does *not* let a consumer skip supplying
  `ExportFile` for packages outside the exposed API surface, at least not through this
  `go/packages` version's `needsrc`/`exportDataInvalid` logic — export data files are
  self-describing/complete for what they reference, but `go/packages` itself doesn't
  distinguish "referenced by this package's exported API" from "just present in the
  graph" before deciding `needsrc`.
- **Contrast measurement:** the same 58-package graph loaded with `NeedDeps` added to
  the mode (`LoadAllSyntax`-style) required `CompiledGoFiles` for literally every one of
  the 58 packages — with `dep`'s and `third`'s source hidden, all 58 (down to
  `internal/goarch`) reported `"sources missing"`. This is the mechanism behind today's
  Bazel driver behavior (dependencies type-checked from source): `NeedDeps` turns every
  reachable dependency into a `needsrc=true` package regardless of `ExportFile`
  presence.

### 4. `ExportFile` format: `.a` archive with `__.PKGDEF` is accepted

`loadFromExportData` (`packages.go:1475`) opens `lpkg.ExportFile` and passes it straight
to `gcexportdata.NewReader`, which calls `gcimporter.FindExportData` to locate the
export-data section inside an archive/object file
(`gcexportdata/gcexportdata.go:108-121`, doc comment: *"In files written by the
compiler, the export data is not at the start of the file. Before calling Read, use
NewReader to locate the desired portion of the file."*). Empirically,
`go list -export -f '{{.Export}}' os` (and every other package used in this spike)
produced a build-cache file whose first bytes are `!<arch>\n__.PKGDEF ...` — a
classic `ar` archive with a `__.PKGDEF` member — and that file, used verbatim as
`ExportFile`, loaded correctly. So yes: the `.a`/archive-with-`__.PKGDEF` format
produced by the current `go` toolchain's build cache is exactly what `ExportFile` is
expected to contain; no unwrapping is needed before handing the path to the driver
response.

## Measurements (item 3 restated as numbers)

- Fixture transitive closure of `member`: **58 packages** (`go list -json -deps
  ./member`, including `unsafe`).
- Packages needing a real `ExportFile` for the load to succeed: **all 58** except
  `unsafe` (special-cased to `types.Unsafe`) — not just the 3 directly imported by
  `member`. Headline number for the layout-schema design: "member's package count"
  does not bound "packages that need export data"; "member's full transitive
  dependency closure" does.
- Packages whose **source** was required: **1** (`member` itself). Zero source files
  were read for the other 57 in the successful "min" run (proved by removing `dep`'s
  and `third`'s `.go` files and re-running successfully).

## Implications for the layout schema and driver

`packagelayout.HandleDriverRequest` (`go/internal/packagelayout/packagelayout.go:932`)
currently returns, for every package in `Layout.Packages` (stdlib discovered by
walking `GoSDKRoot` in `discoverStdlibWithContext`, non-stdlib presumably Bazel-declared
elsewhere in the layout), real `GoFiles`/`CompiledGoFiles` — the whole transitive
closure is type-checked from source today, matching the `NeedDeps` contrast
measurement above. To load only member packages from source, the layout schema needs,
per non-member package, an `ExportFile` path (compiled gc export data — the
`.a`/`__.PKGDEF` shape from finding 4 is fine, no transcoding needed) in place of
`GoFiles`/`CompiledGoFiles`, while still carrying accurate `PkgPath`/`ID`/`Imports`
edges for the **entire** transitive closure — finding 3 means the layout can neither
prune "irrelevant" leaves out of the graph (panic risk) nor skip export-data content
for them (reported-error risk). `HandleDriverRequest` itself would only need to keep
serving the full `l.Packages` slice as `DriverResponse.Packages` unchanged, and stop
requiring `GoFiles` for every package. Two facts bear on where stdlib export data comes
from, worth recording without speculating about which hermetic build system originates
the layout JSON: (1) the installed Go SDK's `$GOROOT/pkg` does not ship precompiled
`.a` files (confirmed empty here) — `go list -export std` instead compiles stdlib into
the local build cache on demand, so a layout emitter that shells out to the `go`
toolchain needs a place to keep those build-cache artifacts across invocations; (2) in
a hermetic Bazel workspace, a `go_sdk`/stdlib build target would be the natural
producer of that same export data as a build output, rather than invoking
`go list -export` from within the driver at query time.

## Residual risks

- **Graph completeness is load-bearing to the point of panicking.** A layout emitter
  that under-reports the transitive `Imports` graph for even one node (e.g. an SDK
  version skew where a stdlib package gained a new internal dependency) crashes the
  `go/packages` load with `log.Panicf` rather than degrading gracefully. A redesigned
  driver needs either its own validation that a layout's package graph is transitively
  closed and consistent with what the referenced export data imports, or a documented
  operational expectation that layout generation and Go SDK version stay in lockstep.
  This spike did not characterize exactly which mismatches trigger the panic versus a
  milder error.
- **All-or-nothing `ExportFile` requirement undercuts the "just the API surface"
  intuition.** This version of `go/packages` does not support supplying export data
  only for packages whose types are actually consulted — every reachable package needs
  it, or the load reports "sources missing" and marks dependents `IllTyped`. The
  redesign's cost model should assume export-data generation scales with "the member's
  entire transitive dependency closure," same as today's source-based load — the
  savings are in *avoiding parsing/type-checking that closure from source*, not in
  avoiding needing data for it. Worth re-checking against a newer `x/tools` release
  before the design finalizes: the comment at `packages.go:1084` ("TODO: I think it
  should be `lpkg.needtypes && !lpkg.needsrc`...") signals the maintainers consider
  today's `needsrc` gating imprecise and may change it.
- **Version skew on export data.** `gcexportdata`'s package doc states it supports data
  from "only the last two Go releases plus tip." A hermetic layout that pins a Go SDK
  version different from the `go/packages`/`x/tools` version compiled into `arcc` could
  fail to read export data even though graph and files are otherwise correct. Not
  exercised here (same toolchain produced the export data and ran the loader).
- **Driver protocol surface not otherwise stressed.** This spike used a single-pattern,
  single-root request; it did not test multi-root loads, `Tests: true`, overlays (which
  invalidate export data unconditionally, `packages.go:802`), or concurrent driver
  invocations sharing on-disk `ExportFile`s.
