# Spike: the `_arcc_deps` aspect (producer side)

Status: complete — 2026-07-20. Runnable spike in `./spike-aspect/` (`./spike-aspect/run.sh`).
Environment: Bazel 9.2.0, rules_go 0.61.1, Go SDK 1.24.5 (bzlmod-downloaded) — same as
the earlier mechanics spike (`spike-findings.md`).

## Question

The driver spike (`spike-driver-findings.md`) retired the **consumer** risk: arcc loads and
analyzes from a package-layout JSON with no `go.mod`/`go list`. It explicitly left the
**producer** unspiked — "a Bazel aspect that emits the package-layout JSON from Bazel's own
graph." This spike builds that aspect for real and answers: can an aspect reconstruct the
package closure **with per-package import edges**, correctly through the awkward cases
(`embed`, diamonds, stdlib), and source the SDK stdlib set + source tree the layout needs —
all at analysis time, hermetically?

Answer: **yes, on all counts.** The `_arcc_deps` aspect in `spike-aspect/rules/defs.bzl` is a
close-to-final reference implementation.

## Why an aspect at all (not just `GoArchive`)

`GoArchive.transitive` already gives the **flat** closure (5 packages in the fixture) with
each package's `importpath` and `srcs` — stdlib excluded. That is enough for a package *set*
but **not** for the driver's `Imports` map: the only edge data on `GoArchiveData` is the
**private, label-based** `_dep_labels`; there is no public importpath→imports mapping. The
go/packages driver form keys imports by import path, so the edges must be reconstructed. The
aspect does this by reading each node's `GoArchive.direct` (the direct dependency archives)
and projecting them to import paths. This is the aspect's whole reason to exist (design §4.3).

## The fixture graph (exercises every hard case)

Module `example.com/aspect` (no `go.mod` needed to build under Bazel):

```
api  ─deps→ core ─deps→ lowlevel  (imports stdlib "strings")
 │           │   ─deps→ shared ◄── diamond: also a direct dep of api
 │           └─embed→ core_extra ─deps→ extradep   ← embed-only transitive dep
 └─deps→ shared
```

- **`core` embeds `core_extra`** (both `importpath = example.com/aspect/core`): tests
  src-merge, dep-merge, and duplicate-importpath dedup.
- **`extradep`** is reachable **only** through the embedded library: tests that embed subtrees
  are traversed.
- **`shared`** is reached by two paths (`api`, `core`): tests diamond dedup.
- **`lowlevel`** imports `strings`: tests stdlib exclusion.

## Result

`bazel build //api:api_closure` emits a `package-layout.json` and a closure summary. The
closure is exactly **5 packages**, edges correct, stdlib absent:

```
example.com/aspect/api      srcs=api/api.go            deps=core, shared
example.com/aspect/core     srcs=core/core.go,         deps=extradep, lowlevel, shared
                                 core/core_extra.go,
                                 core/extra_impl.go
example.com/aspect/extradep srcs=extradep/extradep.go  deps=
example.com/aspect/lowlevel srcs=lowlevel/lowlevel.go  deps=
example.com/aspect/shared   srcs=shared/shared.go      deps=
```

Note `core`: the embedded lib's source (`extra_impl.go`) **and** its embed-only dependency
(`extradep`) both folded into the single `core` package. `core_extra` does **not** appear as a
separate closure member. `shared` appears once. `strings` (stdlib) appears nowhere.

## Findings (the load-bearing details for the implementer)

1. **Get edges from `GoArchive.direct`, projected to import paths, self excluded.**
   `deps = sorted([a.data.importpath for a in target[GoArchive].direct if a.data.importpath != ip])`.
   `direct` already excludes stdlib and excludes the embedded same-importpath lib, so no
   filtering beyond the `!= self` guard is needed. (Confirmed: `core.direct` =
   `{lowlevel, shared, extradep}`.)

2. **`GoInfo.srcs` is already embed-merged — use it directly for a package's sources.**
   rules_go merges an embedder's own srcs with all embedded libs' srcs into `GoInfo.srcs`
   (`core.srcs` = `{core.go, core_extra.go, extra_impl.go}`). Do **not** hand-walk `embed` to
   collect srcs; you would double-count.

3. **You MUST still traverse `embed` in `attr_aspects`, and you MUST merge duplicate-importpath
   nodes.** These two facts pull in opposite directions and are the subtlest part of the spec:
   - Traverse `embed` (`attr_aspects = ["deps", "embed"]`) because an embed-only transitive
     dependency (`extradep`, reached via `core → embed → core_extra → deps → extradep`) is
     otherwise never visited, and its srcs would be missing from the layout → the driver
     could not serve it → type-check failure.
   - But traversing `embed` also **visits the embedded lib** (`core_extra`) as its own node
     with the **same import path** as its embedder. Emit both and you get two closure entries
     for `example.com/aspect/core`. The rule therefore folds nodes **by import path**, unioning
     srcs and deps (`_merge_by_importpath`). Union is safe because the embedder node is always
     a superset (finding 2), but a naive "last write wins" keyed dict is an order-dependent
     **bug** — spell out the union merge for implementers.

4. **stdlib for the layout comes entirely from the rules_go Go SDK toolchain — three pieces:**
   `sdk = ctx.toolchains["@rules_go//go:toolchain"].sdk`
   - `sdk.package_list` — a `File` (`packages.txt`) enumerating **all 703** stdlib import paths.
     This is what lets the driver answer Capslock's `packages.Load(nil, "std")` (driver-spike
     finding 1): the layout's stdlib set is this file, verbatim.
   - `sdk.srcs` — a depset of **5329** `GOROOT/src` source `File`s (driver-spike finding 2):
     declare these as inputs and the stdlib is readable in the sandbox with no host GOROOT.
   - `sdk.root_file` — its `.dirname + "/src"` is `go_sdk_root` for the layout.
   All three are declared Bazel inputs, so the check is **hermetic** (spike's summary declared
   5337 input Files total: member srcs + full stdlib tree + package list).

5. **`dir` per package = `srcs[0].dirname`, not `label.package`.** For same-repo packages they
   coincide, but external-repo deps (`third_party`) have srcs under `../<repo>/...`; the src
   dirname stays consistent with the actual file locations the driver hands to go/packages.

6. **Guard non-Go and no-importpath nodes.** `if GoInfo not in target or GoArchive not in
   target: return []` (a `deps` edge could be a `filegroup`/proto target); `if not
   gi.importpath: return None` skips `main`/no-importpath libraries — they are never closure
   members. The interface library and all real deps always carry both providers.

7. **Determinism.** Every projection is `sorted()` and the merge is keyed by import path /
   short_path, so the emitted JSON is stable across builds regardless of depset iteration
   order — required for cache stability and golden tests.

## What this validates / what remains

**Validated (producer mechanics):** an aspect over `deps`+`embed` that yields the full package
closure **with importpath edges**, correctly handling embed (src+dep merge, dedup), diamonds,
and stdlib exclusion; plus SDK-sourced stdlib set, stdlib source tree, and `go_sdk_root`, all
as declared inputs. The reference aspect is ~25 lines; the merge + emission ~40 more.

**Deliberately out of scope (belongs to the rule/step, not the aspect):**
- **Membership classification** (design §5.3: component-dep-covered / absorbed / member) and
  the diamond-conflict error. This is depset algebra over the closure the aspect produces —
  the spike emits `roots = [interface importpath]` as a placeholder; the real rule subtracts
  each `component_deps[i].closure` and the `absorbed_deps` set. Mechanically straightforward
  given the validated closure; it is the `_go_component` rule's job (plan Step 4).
- **cgo-preprocessed sources** (driver-spike finding 3). The fixture is pure Go
  (`--pure`). A closure pulling a real cgo package must emit the rules_go-produced
  preprocessed `CompiledGoFiles` rather than `GoFiles`; the aspect would read those off
  `GoArchive`/`GoInfo` for cgo nodes. Flagged for the layout-schema work, not a mechanism gap.
  (Note: cgo needs a working C toolchain — clang is fine, gcc is not required; the
  `--pure` / `BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1` flags here only dodge C-toolchain
  autodetection in this sandbox and are **not** a consumer requirement.)
- **The exact layout wire format.** The spike emits the design §5.4 shape
  (`roots`/`go_sdk_root`/`packages[{importpath,dir,srcs,deps}]`). Whether arcc's driver
  consumes that directly or the go/packages "flatPackage" form (as in `spike-driver`) is
  finalized when Step 1 and Step 4 meet; the aspect produces all the underlying data either
  way.

## Design impact

The producer risk is retired: nothing about the aspect required an escape hatch or an
unavailable API. Two points are worth promoting into the design/plan (done in the same change):
- §4.3 should state the **embed traversal + importpath-merge** rule explicitly (finding 3) —
  it is the one non-obvious correctness hazard.
- §4.6/§5.4 should name the concrete toolchain sources for the stdlib set, stdlib source, and
  `go_sdk_root` (`sdk.package_list`, `sdk.srcs`, `sdk.root_file`) — finding 4 — since those
  are what make driver-spike findings 1–2 hermetically satisfiable.
