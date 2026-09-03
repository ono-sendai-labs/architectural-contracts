# Spike: precomputed standard-library authority map via Capslock v0.3.2

## Question

Can we precompute, for every SDK package and every externally referencable
exported symbol (funcs, methods of exported types, exported vars, consts,
types, plus per-package `init`), a terminal ambient-authority classification,
such that a missing map entry can never be silently read as "pure"? Design
review raised five risks: (1) per-root attribution support in Capslock's API,
(2) exported vars are not call-graph roots, (3) `init` needs per-package
attribution, (4) asm/cgo/linkname bodies are unanalyzable, (5) an independent
enumeration oracle is needed.

## Setup

Scratch module `spike-stdlibmap/` (Go 1.26.4, `github.com/google/capslock
v0.3.2`, `golang.org/x/tools v0.48.0`, no `replace`, `GOFLAGS=-mod=mod`). Read
`go/internal/capslockadapter/capslockadapter.go` (production adapter) and the
Capslock source in the module cache (`analyzer/{analyzer,load,util}.go`,
`interesting/{interesting.go,interesting.cm}`, `proto/capability.pb.go`).
Built a CLI (`spike`) with subcommands `roots` (dump Capslock findings),
`inventory` (enumerate exported symbols via `go/types`), `compare`, `timing`.
All runs completed in seconds; nothing approached the 5-minute cap.

## Findings

### 1. Per-root attribution — yes, but grouping by `Path[0]` is on the caller

`analyzer.GetCapabilityInfo(pkgs, queriedPackages, &analyzer.Config{Granularity:
analyzer.GranularityFunction})` returns a flat `*cpb.CapabilityInfoList`.
`forEachPath` (analyzer.go:714) invokes `fn(cap, visited, v)` once for
**every** function `v` in a queried package that lies on the backward BFS from
a capability-bearing leaf — one `CapabilityInfo` per `(capability,
root-in-queried-package)` pair — and `GetCapabilityInfo`'s wrapper sets
`PackageDir`/`Capability` from `v` itself and reconstructs `Path` backward
from `v`, so `Path[0]` is always the root. Confirmed over `os,strings`, e.g.
`os.Mkdir  FILES  os.Mkdir` and `(*os.File).Chdir  MODIFY_SYSTEM_STATE/CHDIR
(*os.File).Chdir`. Name format (`ssa.Function.String()`, util.go:552): plain
func/var/const → `pkg.Name`; pointer-receiver method → `(*pkg.Type).Method`;
value-receiver → `(pkg.Type).Method` — identical to the adapter's own
`fileHandleUseMethods` key format.

**Generics**: an uninstantiated generic function gets its own direct entry
under its bare name (no `[T]`), matching the `go/types` key; instantiated
names (`Lookup[int]`) appear only as intermediate frames in callers' paths,
not as separate roots to precompute. Verified with a fixture: `genfix2.Lookup
READ_SYSTEM_STATE genfix2.Lookup -> os.Getenv`, plus per-instantiation frames
inside `genfix2.CallInt`'s and `genfix2.CallString`'s own paths.

**Caveat**: `queriedPackages` is the whole `*types.Package`, so Capslock also
reports unexported helpers/closures (`os.Mkdir$1`, `(*os.File).checkValid`) as
their own roots. The map generator must filter to exported package-level
objects + exported methods of exported/alias types + `init`; unexported/
closure entries are noise already folded into their exported callers.

### 2. Inventory — "missing" collapses three distinct, legitimate reasons

Enumerated `os` (188 exported symbols) and `strings` (83) via `go/types`.
Compared against Capslock's roots (adapter classifier): `os` 87/188 present,
101 missing; `strings` 0/83 present (genuinely 100% pure — no package-level
override in `interesting.cm` either). `os` missing breaks down as `const:29,
var:15, type:12, iface-method-spec:2, func:7, method:36`. Spot-checking every
non-var/const/type case showed all fall into one of three buckets, never a
fourth "silently unfound" bucket:

- **(a) Structurally never a root**: `const`, bare `type`, interface method
  specs (`os.Signal.Signal`/`.String` — no body, only implementers have one).
- **(b) Deliberately curated `CAPABILITY_SAFE`** in `interesting.cm`, so no
  entry appears despite being authority-adjacent: `os.Exit` (`func os.Exit
  CAPABILITY_SAFE`) and the 22 `(*os.File)` handle-use methods the adapter
  itself reclassifies SAFE. Same pattern for `runtime`: 26 "missing" funcs
  (`runtime.Caller`, `GC`, `NumGoroutine`, ...) are all explicit
  `CAPABILITY_SAFE` lines.
- **(c) Genuinely pure leaves**: trivial accessors
  (`(*os.LinkError).Error`, `(*os.ProcessState).Pid`), pure error inspection
  (`os.IsExist`/`IsNotExist`/...), and all of `strings`.

`ssautil.AllFunctions` (util.go:195) enumerates every function in the whole
program regardless of reachability from a `main`, so absence isn't a
dead-code-elimination artifact — every exported symbol's body really was
considered.

**Consequence**: an absent entry conflates (a) not-a-root, (b)
deliberately-safe, (c) analyzed-pure. The generator resolves (a) via its own
var/const/type rules and treats (b)/(c) as an explicit `SAFE` entry — but only
if the classifier can't also launder a genuinely *unanalyzable* function into
this same "absent" bucket. It can, and does — see Finding 5.

### 3. Variables — method-authority union works, if run against the *adapter's* classifier

Rule: a var's authority = union of `FunctionCategory` over its
(pointer-dereferenced) static named type's exported method set, using the
*same* classifier the map otherwise uses. Verified for `net/http`:
`DefaultClient`/`*http.Client` → `Do/Get/Head/Post/PostForm/
CloseIdleConnections` all `NETWORK` ⇒ `NETWORK`; `DefaultTransport`/
`*http.Transport` → `RoundTrip`/... all `NETWORK`; `DefaultServeMux`/
`*http.ServeMux` → `Handle`/`HandleFunc`/`ServeHTTP` all `NETWORK`.

For `os.Stdin` (`*os.File`): under the **raw** Capslock classifier,
`(*os.File).Read` is `CAPABILITY_FILES` (367 entries with `default` vs. 343
with `adapter` — exactly the 22 reclassified handle-use methods), so a naive
rule would overclassify `os.Stdin` as `FILES` — wrong, since Stdin is a
pre-minted handle. Rerun against the **adapter's** classifier:
`(*os.File).Read` is `SAFE` (excluded from the union), but
`(*os.File).Chdir` is *not* in `fileHandleUseMethods` and stays
`MODIFY_SYSTEM_STATE/CHDIR` — correctly, since `fchdir` is real ambient
authority regardless of which handle it came from. So `os.Stdin` resolves to
`MODIFY_SYSTEM_STATE`, the right answer, but only when the var rule runs
against the customized classifier, not raw Capslock.

### 4. Init — reported per-file and as a synthesized aggregate; key on the aggregate

Fixture `fixture/initpkg` (two `init` funcs, one `os.Getenv`, one
`os.ReadFile`), `GranularityFunction`, produces four entries: `initpkg.init`
(both capabilities, via `init#1`/`init#2` frames), plus `initpkg.init#1` and
`initpkg.init#2` individually. Go's compiler synthesizes one aggregate
`pkg.init` calling each source-level `init#N` in order; that synthetic
function is itself in the queried package, so its path-union already covers
every `init#N`. **The map should key on `pkg.init` alone**; `init#N` entries
are available but redundant.

### 5. Unanalyzable bodies — mostly pre-classified by package defaults; `UNANALYZED` is common and the adapter discards it

`unsafe`, `reflect`, `runtime`, `syscall` each carry a package-level default
in `interesting.cm` (`ARBITRARY_EXECUTION`/`REFLECT`/`RUNTIME`/
`SYSTEM_CALLS` respectively), with curated per-function overrides (34 for
`runtime`, 14 `reflect`, 3 `syscall`). Over `syscall,reflect,unsafe,runtime`:
2897 `RUNTIME`, 361 `SYSTEM_CALLS`, 327 `REFLECT`, 0 `CGO`, 0 raw
`ARBITRARY_EXECUTION` (asm leaves here already have explicit categories, so
the generic `f.Blocks == nil` → `ARBITRARY_EXECUTION` fallback in
`getExtraNodesByCapability`, analyzer.go:508, never fires for them — it
remains a safety net for asm in packages that lack curation).

`unsafe`'s exported names (`Add`, `Alignof`, `Offsetof`, `Sizeof`, `Slice`,
`SliceData`, `String`, `StringData`) are **all `*types.Builtin`**, not
`*types.Func` (verified with `go/types`) — compiler intrinsics inlined at
every call site, never an SSA function or Capslock root under any
configuration. These 8 symbols must be *hardcoded* in the map as
`ARBITRARY_EXECUTION`/`UNSAFE_POINTER`; no call-graph analysis will ever
discover them.

**Critical finding**: the adapter wraps its classifier in
`interesting.ClassifierExcludingUnanalyzed`, deleting the 39 `unanalyzed`
entries in `interesting.cm` (`sort.Slice`, `sort.Sort`, `io.Copy`,
`errors.As`, `(*bufio.Reader).Read`, ...). Under `default` these are
`CAPABILITY_UNANALYZED`; under the adapter's `no-unanalyzed` classifier they
produce **zero entries** — Capslock walks their bodies, can't see through a
caller-supplied closure/interface, and reports them as if pure:

```
$ ./spike roots default sort,io,errors | grep -E 'sort\.(Slice|Sort)|io\.Copy|errors\.As'
errors.As    UNANALYZED   errors.As
io.Copy      UNANALYZED   io.Copy
sort.Slice   UNANALYZED   sort.Slice
sort.Sort    UNANALYZED   sort.Sort
$ ./spike roots no-unanalyzed sort,io,errors | grep -E 'sort\.(Slice|Sort)|io\.Copy|errors\.As'
(no output)
```

This is a reproducible instance of the exact silent-pure failure mode design
review worried about, caused by reusing the adapter's classifier for map
generation. Over the full 178-package non-internal `std` set, `default`
reports **1078** distinct `UNANALYZED` findings — not a corner case. Full
breakdown (178 pkgs, `default`): `RUNTIME 3202, NETWORK 3049, UNANALYZED
1078, REFLECT 767, FILES 627, ARBITRARY_EXECUTION 377,
MODIFY_SYSTEM_STATE/SIGNALS 363, SYSTEM_CALLS 361, UNSAFE_POINTER 312,
OPERATING_SYSTEM 275, READ_SYSTEM_STATE 194, EXEC 67,
MODIFY_SYSTEM_STATE/{LOGGING,ENV} 3+3, /CHDIR 2` — every category the design
anticipated appears in practice, `UNANALYZED` and `ARBITRARY_EXECUTION` as
distinct entries.

### 6. Enumeration oracle and cost — cheap, well inside budget

`go list std` → **360** packages; excluding any path with an `internal`
segment → **178** remain (`go list std` never emits `cmd/...`, nothing to
exclude there). One `packages.Load` + `GetCapabilityInfo` call over all 178:

```
pkgs=178  load=503ms  analyze=1.69s  total=2.19s  entries=10681 (8257 distinct roots)
peak RSS ≈ 1.35 GB (VmHWM, sampled during the run)
```

Scaling (`default`): 30 pkgs → 1.09s, 60 → 1.40s, 120 → 2.13s, 178 → 2.19s
(dependency loading is shared within one process, so cost is sub-linear past
~120 packages once big transitive closures like `net/http`/`crypto/tls` are
paid once). A full-stdlib run is a single-digit-second, ~1.5 GB operation —
trivially inside budget, and cheap enough to regenerate in CI on every SDK
bump rather than needing incremental caching.

## Proposed classification rules

- **Funcs/methods** (incl. generic origins): one `GetCapabilityInfo` call
  (`GranularityFunction`) over all 178 non-internal `std` packages as one
  combined `queriedPackages` batch; group by `Path[0].GetName()`; keep only
  exported package-level funcs and exported methods of exported/alias types
  (drop unexported helpers/closures). A root with zero entries is recorded
  as `SAFE` explicitly (not left absent), cross-checked against the
  `go/types` inventory (Finding 2).
- **Vars**: resolve the (pointer-dereferenced) static type; if `*types.Named`
  with an exported method set, classification = union of the already-computed
  method classifications (Finding 3), using the map's own classifier;
  otherwise (basic types, `error`, method-less structs) → `SAFE`.
- **Consts and plain types**: always `SAFE` — never call-graph roots.
- **Interface method specs**: no map entry keyed by the interface itself
  (no body exists); a checker-time concern — a reference through an
  interface-typed value must resolve to known concrete implementations or
  fall back to `AnalysisDefeating`.
- **`init`**: one entry per package keyed `pkg.init` (the synthesized
  aggregate, Finding 4) — already unions every source-level `init#N`.
- **Unanalyzable bodies**: build the map with a classifier that does **not**
  call `ClassifierExcludingUnanalyzed` (opposite of the live user-code
  adapter), so `CAPABILITY_UNANALYZED` survives and maps to
  `AnalysisDefeating` — exactly what `capslockadapter.mapClass`'s existing
  `"UNANALYZED"` case already expects (today dead code for user scans, live
  for the stdlib map). Hardcode the 8 `unsafe.*` compiler-builtin symbols
  (never SSA functions, Finding 5) as `AnalysisDefeating`.

## Cost

Full non-internal `std` (178 packages), function granularity, one process:
~2.2s wall, ~1.35 GB peak RSS, 10681 `(root, capability)` rows / 8257 distinct
roots. Comfortably regenerable per SDK version, in CI, well under the
5-minute spike budget.

## Residual risks

- **Classifier drift between map-generation and check-time scanning.** The
  map needs `UNANALYZED` preserved while the live adapter strips it for user
  code; a future refactor that shares one classifier constructor for both
  would reintroduce Finding 5's silent-pure bug for the stdlib map.
- **Capslock's `CAPABILITY_SAFE` curation is an opaque trust boundary.**
  `os.Exit` and ~26 `runtime.*` functions are safe only because Capslock's
  maintainers said so, not because static analysis proved it. Worth
  surfacing provenance (Capslock builtin vs. our override vs.
  genuinely-proved-pure) per entry rather than collapsing all into one `SAFE`.
- **Interface-typed symbol references** (`os.Signal`, `io.Reader` held
  values) can't be resolved to a map entry by symbol name alone; this spike
  did not prototype checker-side resolution.
- **GOOS/GOARCH sensitivity** unexplored — default build environment used
  throughout; a map generated on linux/amd64 may classify differently
  elsewhere (e.g. syscall internals) — design should decide per-platform
  generation vs. accepting this as a known limitation.
- **Var rule validated on two hand-picked packages** (`os`, `net/http`), not
  run across all 178 — false-positive/negative rate at scale (vars of
  unexported types, interface-typed vars) is unverified.
