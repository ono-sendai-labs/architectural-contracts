# Why `bazel build //:arcc_stdlib_map` takes ~150 s, and what can be done

**Date:** 2026-09-08 · **Author:** investigation session (no code changed; working copy clean)
**Context:** plan `2026-08-04-compositional-component-analysis`, Step 4 (stdlib map) and
Step 6 (reference-scan cutover, just landed). The orchestrator was interrupted after the
first implementation round of Step 6 because the integration suite now takes tens of
minutes.
**Audience:** an agent revising the design/plan. Everything below is measured on this
machine (16 cores, 31 GB RAM, heavily swapped during the run); reproduction commands are
given so the numbers can be re-taken.

---

## 1. Executive summary

Three independent multipliers stack up. Only the third is about the generator being slow.

| # | Cause | Cost |
|---|---|---|
| **M1** | The `ArccStdlibMap` action is invalidated by **any** change to **any** Go source in `go/`, because the action's tool input is the whole `//:arcc` binary. | every Go edit ⇒ full regeneration |
| **M2** | `bazel build //...` / `bazel test //...` contains **6 `ArccStdlibMap` actions in 5 distinct configurations** (default, byte-determinism replica, darwin/arm64, build-tag variant, and probe.bzl's darwin+pure+adapter_probe). | ×6 |
| **M3** | One generation runs Capslock **once per importable package** — 190 separate whole-closure `packages.Load` + SSA + VTA passes — instead of once. | ~146 s instead of ~2 s of analysis work |

M1 × M2 ≈ **15 minutes of stdlib-map regeneration on every Go source edit**, before any
test runs. Native `just test-integration` adds two more full generations
(`TestGenerateFullStdlibMap` calls `fullGeneration` twice) at ~140 s each.

The cheapest large wins are M1 and M2 (build-graph shape, no analysis semantics touched).
M3 has a safe ~40 % win and a ~5× win with a concurrency hazard; a principled fix is a
research-scale change.

---

## 2. Where the time actually goes

### 2.1 The action itself

```
bazel build //:arcc_stdlib_map --profile=p.gz
```

Profile attribution (`p2.gz`, replica target):

```
155.0 s  buildTargets
146.25 s   action 'Generating stdlib authority map //:arcc_stdlib_map_replica (linux/amd64)'
  6.11 s   GoStdlib …/stdlib_/pkg [for tool]
  <everything else < 2 s>
```

So ~95 % of the target's wall time is the single generation action. Building `//go/cmd/arcc`
from a warm-ish cache is ~12 s; from a genuinely cold cache the arcc build (protobuf,
x/tools, capslock, rules_go stdlib) is the rest of the user's ">300 s".

Action facts (from `bazel aquery` and a manual re-run inside the retained sandbox):

* declared inputs: the SDK `src/` tree, the toolchain package list, the config file, `arcc`
  as an exec-config tool. No toolchain binary — design I5 holds.
* one process spawns **204 `execve`s**: the generator plus ~190 self-exec
  `GOPACKAGESDRIVER` subprocesses.
* output: 383 packages (190 importable), 11 524 symbols, 190 inits.

Re-running the action's exact command line by hand inside the retained sandbox
(`--sandbox_debug`, sandbox `88`) reproduces it at **3 m 15 s** (6 m 42 s CPU) — the
sandbox is not the cost.

### 2.2 The 190× redundancy

`go/internal/stdlibmap/generate.go:120-127`:

```go
for _, pkgPath := range importable {
        batch, err := findingsSource.Findings([]string{pkgPath})
        ...
}
```

Each `Findings` call is `capslockadapter.GenerationFindingsForEnv`
(`go/internal/capslockadapter/generation.go:261`), which does a **fresh
`packages.Load(PackagesLoadModeNeeded, path)`** — full syntax + types over the whole
transitive closure — followed by `analyzer.GetCapabilityInfo`, which internally
(`capslock/analyzer/util.go:195-210`) does:

```go
ssautil.AllPackages(pkgs, ssa.InstantiateGenerics|ssa.GlobalDebug)  // whole closure
ssaProg.Build()
vta.CallGraph(ssautil.AllFunctions(ssaProg), nil)                   // whole closure
```

`net/http` has a 184-package closure; `net/rpc/jsonrpc` 196. Every one of the 190 queries
re-parses, re-type-checks, re-SSA-builds and re-VTAs its entire closure from scratch.

Measured natively (probe binary; go list driver with a warm build cache, so its load leg is
*cheaper* than the Bazel driver's):

```
per-package (current shape)   n=190   load=45.8 s   analyze=91.8 s   total 137.6 s wall / 6 m 15 s CPU
top offenders: net/rpc/jsonrpc deps=196 load=0.49 analyze=1.35
               net/http        deps=184 load=0.51 analyze=1.14
```

That 137.6 s native reproduces the 146 s Bazel action almost exactly, so the Bazel wall time
is essentially all this loop (the Bazel load leg is the ~190 driver subprocesses, ≈54 s of
the 146 s).

### 2.3 Why the loop is per-package (do not naively undo it)

The comment at `generate.go:112-119` records a Step 6 cutover finding: analysing all
importable packages as one program makes VTA conflate unrelated packages. Confirmed:

| shape | wall | findings |
|---|---|---|
| one whole-batch `Load` + one `GetCapabilityInfo` | **2.4 s** | **10 399** |
| 190 per-package runs (current) | 137.6 s | **8 182** |

2 217 extra findings — VTA propagating dynamic types across the whole stdlib. The isolation
is a real correctness property, not an accident. Any speed-up must preserve
"one program per queried package".

### 2.4 Documentation drift (worth fixing while here)

* `go/internal/stdlibmap/generate_integration_test.go:17-19` still documents
  `fullGeneration` as "one batched Capslock run at GranularityFunction … (~2 s, ~1.4 GB;
  spike 6)". That was true before the Step 6 cutover; it is now ~140 s.
* Plan Step 4 (`implementation/plan.md:170-172`) still specifies "Capslock
  `GranularityFunction` **in one batch** over all importable packages, grouped by `Path[0]`".
  The design and plan need to record the per-package change, its reason (§2.3) and its cost.

---

## 3. M1 — the map is invalidated by every Go edit

`bazel_rules/go/private/stdlib_map.bzl` passes `//:arcc` (→ `//go/cmd/arcc`) as
`ctx.executable._arcc` and lists it in `tools`. The action key therefore includes the whole
arcc binary, whose dependency closure is the entire Go module.

**Measured.** With `//:arcc_stdlib_map` fully cached, adding one comment line to
`go/internal/checker/checker.go` — a package the generator never uses — and rebuilding:

```
INFO: Elapsed time: 147.956s   (4 linux-sandbox processes)
```

Reverting the comment does **not** restore the cached artifact either: Bazel's local action
cache keeps one entry per action, so an A→B→A edit oscillation misses every time
(measured: 148.264 s on the revert build).

This is almost certainly the dominant contributor to "the integration test suite takes
several tens of minutes" during an implementer/reviewer rework loop, because every round
edits Go sources.

---

## 4. M2 — six generations per full build

```
$ bazel aquery 'mnemonic("ArccStdlibMap", //...)' --output=jsonproto
ArccStdlibMap actions: 6
  k8-fastbuild-ST-f20dd3651408  arcc_stdlib_map.stdlib-map.json
  k8-fastbuild-ST-7d8cff736ce3  arcc_stdlib_map.stdlib-map.json
  k8-fastbuild-ST-6cc2ce23ce3d  arcc_stdlib_map.stdlib-map.json
  k8-fastbuild-ST-2d2995ab1fe5  arcc_stdlib_map.stdlib-map.json
  k8-fastbuild                  arcc_stdlib_map.stdlib-map.json
  k8-fastbuild                  arcc_stdlib_map_replica.stdlib-map.json
```

Sources:

* `BUILD.bazel:31-40` — `//:arcc_stdlib_map` and `//:arcc_stdlib_map_replica`. The replica
  exists *specifically* to defeat the action cache so byte-determinism can be observed
  (`bazel_rules/go/tests/BUILD.bazel:78-95`, `stdlib_map_keys_test`). It is a deliberate
  second full generation.
* `bazel_rules/go/tests/BUILD.bazel:63-76` — `stdlib_map_darwin_arm64` (platforms
  transition) and `stdlib_map_tagged` (`@rules_go//go/config:tags` transition), both
  required by `stdlib_map_keys_test`.
* `bazel_rules/go/tests/probe.bzl:22-38, 104-125` — `transitioned_checked_map` under a
  *different* darwin/arm64 configuration (platforms + `pure` + `adapter_probe` tag), used by
  the component tests.

All five configurations run the analysis on the linux exec host, so each costs the same
~150 s. None is cached across configurations.

Native side: `just test-integration` runs `TestGenerateFullStdlibMap`, which calls
`fullGeneration` **twice** (`generate_integration_test.go:51` and `:197`, the second purely
for byte-determinism) — two more ~140 s generations.
`TestGenerateCrossTarget` is cheap (it generates for `syscall` only).
`just selfcheck` uses the native on-disk cache (`stdlibmap/cache.go`, keyed by
`CacheKeyDigest(SDKKey)` under `os.UserCacheDir()`), so repeated `arcc check` runs do not
regenerate — but a `classifier_hash` bump (as in commit `bda867e`) invalidates it once.

---

## 5. Options

Grouped by which multiplier they attack. A–B are build-graph shape only (no analysis
semantics); C changes the generator; D is a note.

### A. Stop invalidating the map on every Go edit (attacks M1)

**A1 — give the generator its own thin binary target.** Add
`//go/cmd/arcc-stdlibmap` whose dependency closure is only what generation needs
(`stdlibmap`, `capslockadapter`, `packagelayout`, `artifactio`, `schema`, `symbol`,
`stdlibauthority`, `hostpolicy`) and point `ARCC_TARGET` in `stdlib_map.bzl` at it. Edits to
`goanalysis`, `checker`, `report`, `facts`, `surface`, `cmd/arcc/app` — where essentially
all Step 6–13 churn lives — then stop invalidating the map.
*Caveat:* the binary must also serve `GOPACKAGESDRIVER` self-exec mode, since
`LayoutLoader` re-executes `os.Executable()`; `packagelayout`'s driver entry point has to be
wired into the thin binary too. Hermeticity and I5 are unaffected (the map action already
never touches the check path).
*Estimated effect:* removes the ×(every edit) factor for most of the remaining plan.

**A2 — check the default-configuration map in as a source artifact.** Treat it like
`just gen` / `gen-is-clean`: `arcc_stdlib_map` becomes a `filegroup` over a committed
`stdlibmap/linux_amd64.json`, plus a `map-is-clean` recipe that regenerates and diffs. The
normal build then runs no generation action at all.
*Caveat:* a large generated file in the tree; the freshness check must run somewhere
(pre-merge or nightly), and the SDK-key/classifier-hash mismatch check already fails closed
if it goes stale, so the failure mode is loud.
*Interacts with:* DR-07's "the stdlib map must be a declared Bazel artifact before the check
depends on it" — a committed artifact is still a declared artifact, but the design's intent
should be re-read before choosing this.

### B. Build fewer maps per run (attacks M2)

**B1 — move the multi-configuration map targets out of the default build.** Tag
`//:arcc_stdlib_map_replica`, `:stdlib_map_darwin_arm64`, `:stdlib_map_tagged` and
probe.bzl's `transitioned_checked_map` (and their tests) `manual`, and run them in a separate
`just ci-full` / nightly lane. Routine `bazel test //...` then builds **one** map.
*Effect:* ×6 → ×1. Largest single-line win available.
*Cost:* determinism and cross-configuration keying stop being checked on every run. Given
they are properties of the generator, not of the code under change, a nightly/pre-merge lane
is defensible — but it is a deliberate weakening and belongs in the design's acceptance
matrix, not in a silent tag.

**B2 — test key propagation on a synthetic SDK instead of the real stdlib.** What
`stdlib_map_keys_test` actually asserts is that distinct target configurations produce
distinct, self-validating SDK keys and distinct bytes. That property needs a handful of
packages, not 190. A fixture SDK (or restricting the transitioned maps to a small package
list via a rule attribute) keeps the assertions on every run at ~1 % of the cost.
*This is probably the best B option:* it preserves coverage rather than deferring it.

**B3 — native: stop generating the full map twice.** `TestGenerateFullStdlibMap`'s second
`fullGeneration` exists only for byte-determinism. Either scope the determinism assertion to
a subset, or drop it in favour of the Bazel replica (or vice versa — but not both).

### C. Make one generation cheaper (attacks M3)

All measurements below produced **identical findings** (8 182 findings, identical
per-package count signature) to the current shape.

**C1 — one shared `packages.Load`, then per-package analysis.** Load all 190 importable
packages in a single `packages.Load` (the dependency `*packages.Package` values are then
shared), and call `GetCapabilityInfo([]*packages.Package{p}, {p.Types}, cfg)` per package.
`ssautil.AllPackages` still visits only `p` and its closure, so the one-program-per-query
isolation of §2.3 is preserved exactly.

```
current shape:            load 45.8 s + analyze 91.8 s = 137.6 s
shared load, sequential:  load  0.5 s + analyze 84–90 s =  84–90 s      peak RSS 1.3 GB
```

In Bazel the saving is larger than native, because it also collapses ~190
`GOPACKAGESDRIVER` self-exec subprocesses into one (≈54 s of the 146 s).
*Risk:* low. One thing to know: capslock's `rewriteCallsToSort` /
`rewriteCallsToOnceDoEtc` mutate the loaded ASTs and `TypesInfo` in place, so shared
dependency packages get rewritten by the first query that reaches them. The rewrites are
idempotent (after rewriting, the call they match is gone), so sequential sharing is safe —
this should be asserted by a parity test against the current output.

**C2 — parallelise the per-package analysis.** On top of C1:

```
shared load, 8 workers:   load 0.6 s + analyze 26.7–27.7 s = ~27 s      peak RSS 4.85 GB
```

⇒ ~5× on the Bazel action (146 s → ~30 s).
**Hazard, measured:** with 4 workers the run died with

```
fatal error: concurrent map writes
  capslock/analyzer.statementCallingFunctionObject   rewrite.go:324
  capslock/analyzer.rewriteCallsToOnceDoEtc          rewrite.go:197
```

The in-place rewrites write into the *shared* dependency packages' `TypesInfo` maps, so
concurrent workers over a shared load race. (The 8-worker run happened to survive; it is a
race, not a threshold.) Mitigations, in order of preference:
  1. Shard at the process level instead (C3) — no in-process concurrency at all.
  2. Force the rewrites to happen once, single-threaded, before the parallel pass — e.g. one
     discarded whole-batch `GetCapabilityInfo` (~2 s) as a warm-up, after which the parallel
     workers find nothing to rewrite. Works, but depends on capslock internals and would need
     a regression test pinning the behaviour.
  3. Upstream a fix (or vendor a patched `analyzer`) making the rewrites idempotent under
     concurrency. Note the classifier/`BuiltinClassifierPin` machinery already fails closed on
     capslock structural changes, so a fork has a maintenance cost.
Also note memory: 4.85 GB peak at 8 workers. This machine already had the Bazel server
killed once during this investigation ("Server terminated abruptly"), so the worker count
must be bounded and ideally derived from the action's declared resources.

**C3 — shard the generation across Bazel actions.** Split into K `ArccStdlibMapShard`
actions (each doing C1 over its slice of importable packages) plus a merge action. Bazel
parallelises them, each shard caches independently, and there is no in-process concurrency
hazard. Cost: each shard reloads the closure of its own slice (cheap once C1 is in). Also
makes M1 less painful, since a shard that did not change can still hit the cache — though
with the tool binary in every shard's key, A1 is still needed for that to bite.

**C4 — replace whole-closure VTA-per-package with bottom-up summaries** (per-package
capability summaries computed in dependency order and propagated, instead of re-analysing
each closure). This is the only option that removes the asymptotic redundancy rather than
amortising it, and it would give the per-package isolation of §2.3 by construction. It is a
research-scale change to the generation model and is out of scope for this plan; record it
as a design note, not a task.

### D. Note for the design/plan text

Whatever is chosen, §2.4's drift must be repaired: the design's Step 4 "one batch over all
importable packages" is no longer what the code does, the reason (§2.3, with the
10 399-vs-8 182 evidence) is currently only a code comment, and the cost of the change was
never recorded anywhere.

---

## 6. Recommended combination

If the goal is "make the orchestrator's rework loop tolerable without weakening coverage":

1. **A1** (thin generator binary) — removes the every-edit invalidation, the single largest
   real-world factor.
2. **B2** (synthetic SDK for the key-propagation tests) — cuts 6 generations to 1–2 while
   *keeping* the assertions; fall back to **B1** if B2 is too much work right now.
3. **C1** (shared load) — safe ~40 % on the remaining generations, and it removes 190
   subprocess spawns from the Bazel action.
4. **B3** (native: one full generation, not two).

That is roughly 15 min → under 1 min for the map's contribution to a rework round, with no
change to what is analysed or asserted. **C2/C3** are the follow-up if the remaining ~90 s
still hurts; **C4** is a design note.

---

## 7. Reproduction

```sh
# action attribution
bazel build //:arcc_stdlib_map_replica --profile=/tmp/p.gz   # ~155 s, action ~146 s
python3 - <<'PY'
import gzip,json; d=json.load(gzip.open('/tmp/p.gz'))
ev=[e for e in d['traceEvents'] if e.get('ph')=='X' and e.get('dur',0)>1e6]
ev.sort(key=lambda e:-e['dur'])
[print(round(e['dur']/1e6,2), e.get('cat'), e['name'][:100]) for e in ev[:15]]
PY

# how many generations a full build contains
bazel aquery 'mnemonic("ArccStdlibMap", //...)' --output=jsonproto

# invalidation by an unrelated Go edit
bazel build //:arcc_stdlib_map                       # cached
printf '\n// probe\n' >> go/internal/checker/checker.go   # (inside the file, not at EOF)
bazel build //:arcc_stdlib_map                       # ~148 s
jj restore go/internal/checker/checker.go

# the action outside Bazel, in a retained sandbox
bazel build //:arcc_stdlib_map_replica --sandbox_debug
#   then run the aquery command line from
#   $(bazel info output_base)/sandbox/linux-sandbox/<N>/execroot/_main
```

The A/B measurements in §5 C were taken with a throwaway probe binary under
`go/cmd/xperfprobe` that calls `packages.Load` + `analyzer.GetCapabilityInfo` directly in
the three shapes (per-package / shared-sequential / shared-parallel / whole-batch). It was
**deleted**; the working copy is clean. Rebuild it from the shapes described in §2.2 and
§5 C if the numbers need re-taking.
