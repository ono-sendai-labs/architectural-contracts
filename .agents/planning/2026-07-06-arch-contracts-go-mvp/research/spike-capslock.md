# Spike — Capslock validation (plan Step 0)

**Goal.** *Execute* — not just read — the load-bearing Capslock assumptions the
whole design stands on, before building five steps of pure core on them (review
B8). All six assumptions were driven through Capslock **as a library**
(`github.com/google/capslock/analyzer`) over purpose-built probe packages plus
the draft `csvtool` example.

**Verdict: all six assumptions hold** — with **two material corrections** to the
design (both improvements). See "Design feedback" at the end.

## How to reproduce

The spike is preserved as a self-contained Go module next to this note:

```
research/spike/
  go.mod                 # module .../spike; replace github.com/google/capslock => ../../external/capslock
  harness.go             # drives analyzer.GetCapabilityInfo the way capslockadapter will
  spike_test.go          # one test per assumption — asserts + prints a transcript
  probes/                # tiny probe packages (targets of the analysis)
    stdlibenv/ keyforms/ initauth/ initimporter/
    readerattr/ readercleanshell/ readerdirtyshell/ scope/
```

The `csvtool` example packages live in their final destination,
`go/examples/csvtool/{toprow, internal/parsecsv, csvfile, app}` (drafts, refined
in Steps 8/9). The spike loads them from the `go/` module.

```
cd research/spike && go test -v ./       # runs on this machine via the local capslock checkout
```

Environment: Go 1.26.4; Capslock pinned to the local checkout at
`../../external/capslock` (module `github.com/google/capslock`, go 1.25; VTA call
graph, `x/tools` v0.43.0).

---

## Finding 0 (the big one): use `GetClassifier(excludeUnanalyzed=true)`

Capslock classifies a set of stdlib functions that take a caller-supplied
callback/interface as **`UNANALYZED`** (a deliberate leaf, so it doesn't assume
"any callback flows to any call site"). The set includes ubiquitous functions:
**`io.ReadAll`, `io.Copy*`, `io.ReadFull`, `errors.Is/As/Unwrap`, the `bufio`
readers, `sort.Search/Find/Reverse/Stable`, `(*sync.Once).Do`, `log.Printf`**
(`interesting/interesting.cm`, 39 entries).

Two classifier modes behave very differently, and the choice is a **design
decision the spike forces**:

| | `GetClassifier(false)` — UNANALYZED **visible** (builtin default) | `GetClassifier(true)` — UNANALYZED **excluded** ⟵ **RECOMMENDED** |
|---|---|---|
| `io.ReadAll`, `errors.Is` in clean code | reported as spurious `UNANALYZED` findings | descended-through → **no finding** |
| `manifest.Parse(io.Reader)` fed an `*os.File` | FILES **masked** (Parse stops at the `io.ReadAll` leaf; only `UNANALYZED`) | **FILES attributed to Parse** — §11 works |
| genuinely-clean component | *not* empty (UNANALYZED noise) | **empty capability set** |

**Excluding UNANALYZED makes Capslock descend into these functions**, so (a)
truly-clean code yields the empty set the design equates with
"ambient-authority-free", and (b) the `io.Reader → *os.File` VTA attribution the
design §11 depends on actually fires. **Adopt `excludeUnanalyzed=true` for the
MVP `StrictPolicy` classifier in `capslockadapter`** (and when generating the
pruning map, wrap the merged classifier in `interesting.ClassifierExcludingUnanalyzed`).

Trade-off to record: we lose UNANALYZED as an explicit "analysis-defeating"
signal. For the MVP that is the right call — the alternative floods every real
component with UNANALYZED. A post-MVP policy could re-surface UNANALYZED as a
*warning* (`ANALYSIS_LIMITATION`) without failing the check.

---

## Finding 0b (model correction): ambient authority = capability *minting*, not *use*

Capslock's builtin map conflates two object-capability notions that our model
must keep distinct:

- **Ambient authority** — *minting* a capability out of an ambient designator:
  `os.Open`/`os.OpenFile`/`os.Create`/`os.ReadFile`/`os.NewFile`/… turn a
  caller-invented *path* (or raw fd) into an `*os.File`. This is the authority we
  care about.
- **Capability use** — exercising a capability you were *granted*: the `*os.File`
  *methods* `(*os.File).Read/Write/Close/Seek/…`, and helpers like `io.ReadAll`
  that operate on a handed-in `io.Reader`. No ambient authority is exercised here;
  the authority was spent by whoever minted the handle.

Capslock classifies **both** groups `CAPABILITY_FILES` (see `interesting.cm`:
`func os.Open … FILES` *and* `func (*os.File).Read … FILES`). Consequences of the
conflation, both bad for our model:

1. A **deprivileged consumer**'s attributed authority depends on **what its caller
   passes in**. `manifest.Parse(io.Reader)` "becomes" FILES-holding iff some caller
   hands it a file-backed reader — even though `Parse`'s code is unchanged and only
   ever reads a stream. That breaks **modular reasoning**: a component's authority
   should be a property of its own code + its dependencies' contracts, not of who
   calls it. Design §11's "shell must wrap the manifest in a `bytes.Reader`" rule is
   a *workaround* for this — disciplining callers to avoid a mis-attribution.
2. Authority is attributed **far from where it is exercised** — at every stream
   read, rather than at the single `os.Open`.

**Fix (config-only, validated):** reclassify the file-handle *use* methods
`CAPABILITY_SAFE` while leaving the *minting* functions `FILES`. Because Capslock's
`FunctionCategory` gives a function-level entry precedence over the `package os …
OPERATING_SYSTEM` fallback, a per-run capability map with
`func (*os.File).Read CAPABILITY_SAFE` (× the 22 FILES-classified handle methods)
does exactly this. Deliberately **excludes** `(*os.File).Chdir` (Capslock:
`MODIFY_SYSTEM_STATE` — it mutates process cwd, genuine ambient authority).

`TestOcapMintingNotUse` proves it. The **same** dirty-reader scenario as §11:

```
ocap: reader dirty (*os.File flows into Parse)  — 1 finding
  FILES  DIRECT  readerdirtyshell.Run  ->  os.Open        # opener holds FILES
                                                          # readerattr.Parse: CLEAN
ocap: filehandle (Mint vs Consume)  — 1 finding
  FILES  DIRECT  filehandle.Mint     ->  os.Open          # mint = authority
                                                          # filehandle.Consume (f.Read): CLEAN
ocap: csvfile (path-based)  — 1 finding
  FILES  DIRECT  csvfile.Read        ->  os.ReadFile      # path-based mint+read stays FILES
```

**Design impact — this is better than §11 and supersedes it:**
- `manifest.Parse(io.Reader)` is authority-free **by construction**, regardless of
  what reader flows in. **Drop the "wrap in `bytes.Reader`" rule** (§11); it becomes
  unnecessary. The shell that calls `os.Open` holds FILES, which it declares anyway.
- Attribution lands at the minting site — the component "closest to where ambient
  authority is exercised," exactly as intended.
- Modular/compositional reasoning is restored: a component's authority no longer
  depends on its callers.

**Costs / caveats to record:**
- We now curate a small capability-map override (the 22 handle methods). It is a
  stable, reviewable set; pin Capslock and re-check on upgrade (already required).
  Combine with `excludeUnanalyzed=true` and the boundary-prune map — all three are
  merged into the one classifier the adapter builds per run.
- **Symmetry (follow-up):** the same minting-vs-use split applies to
  **network** (`net.Dial`/`Listen` mint; `(net.Conn).Read/Write` use) and
  **exec/env**. MVP needs only the filesystem set (that's what the examples and
  `manifest.Parse` touch); curate the rest when those capabilities appear.
- **Known loosening — stdio globals.** With handle methods SAFE, reading the
  process-global ambient handles `os.Stdin/Stdout/Stderr` (package *variables*, not
  minted) becomes invisible, where Capslock's default would flag it FILES.
  Defensible (stdio fds are granted by the parent process — capability use), but a
  genuine policy choice. Post-MVP mitigation: specifically re-flag references to
  those three variables if strict stdio accounting is wanted.

---

## Assumption 1 — strict-safe stdlib envelope ✔ (with a B8 correction)

Under the recommended strict classifier, **`stdlibenv` produces zero findings** —
every entry point the pure core and the examples need is authority-free:

- `fmt.Sprintf`, `strconv.Atoi/Itoa`, `errors.New`, `fmt.Errorf`, `errors.Is`
- `io.ReadAll` over a `bytes.Reader` (**needed by `manifest.Parse(io.Reader)`**) — clean
- `sort.Ints` (SAFE), `slices.SortFunc`
- **`sort.Sort` *and* `sort.Slice`** — both clean

### Correction to review B8: `sort.Slice` is fine; the sort split is unnecessary

The design (review B8) told the `toprow` example to use `sort.Sort` with a
concrete `sort.Interface` rather than `sort.Slice`, believing `sort.Slice` is
`unanalyzed` and would fail strict. **Empirically both are clean.** Capslock's
`buildGraph` runs `rewriteCallsToSort`/`rewriteCallsToOnceDoEtc` **before**
building SSA: it rewrites `sort.Sort`, `sort.Slice`, `sort.SliceStable`,
`(*sync.Once).Do`, `(*sync.WaitGroup).Go`, `errgroup.Group.Go` call sites to call
the caller's comparator/closure **directly**, bypassing the sort machinery. So
the comparator (not the sort internals) is what gets analyzed.

`TestSortCallSiteRewrite` proves this: the *raw* VTA graph contains the edge
`UseSortSort → sort.Sort` (category `UNANALYZED`), yet Capslock reports nothing.

**Design impact:** `toprow` may use `sort.Slice` freely. We keep the concrete
`sort.Interface` in the example anyway (it reads fine and documents the pattern),
but the *requirement* is dropped — note it in the design so we don't over-constrain.

---

## Assumption 3 — key formats & normalization (review A4) ✔

`ssa.Function.String()` (the exact string the classifier matches, and the exact
form we must emit into prune maps) for the `keyforms` probe:

| Construct | Emitted key |
|---|---|
| value-receiver method | `(<pkg>.Widget).Name` |
| pointer-receiver method | `(*<pkg>.Widget).SetName` |
| interface impl (value recv) | `(<pkg>.Circle).Area` |
| promoted (embedded) method | `(<pkg>.Boxed).Name` and `(*<pkg>.Boxed).SetName` |
| generic function (origin) | `<pkg>.MapInts` — **bracket-free** |
| generic instantiation (SSA node) | `<pkg>.MapInts[string]` — but categorized via the bracket-free **origin** (`getNodeCapabilities` uses `origin.String()`), so the classifier key is `<pkg>.MapInts` |
| explicit/synthetic init | `<pkg>.init` (and a wrapper `<pkg>.init#1`, see A2) |

The `.cm` line form is `func <key> CAPABILITY_SAFE` where `<key>` is exactly the
string above (no leading `func` inside the key). Confirmed the whole-package load
emits both receiver forms and that bracket-stripping is the correct normalization
for generics.

---

## Assumption 4 — init attribution (review A2) + init pruning ✔

`initauth` has `func init(){ os.Getenv(...) }` (READ_SYSTEM_STATE); `initimporter`
imports it and touches no authority itself. Capslock attributes the capability to
the **importer**:

```
READ_SYSTEM_STATE  TRANSITIVE  initimporter.init
    -> initimporter.init
    -> initauth.init
    -> initauth.init#1
    -> os.Getenv
```

Note the two init nodes: `<pkg>.init` (the runtime-ordering wrapper) and
`<pkg>.init#1` (the body). Pruning **`func <pkg>.init CAPABILITY_SAFE`** on the
dependency terminates traversal at the wrapper → `initimporter` comes out empty.
This confirms the Step-9 rule: for each component-dependency package, add
`func <deppkg>.init CAPABILITY_SAFE` to the prune map (precedent:
`func encoding/json.init CAPABILITY_SAFE` in the builtin map).

---

## Assumption 5 — whole-package scope + `_test.go` exclusion ✔

`scope` has an exported helper `Unreached()` that calls `os.ReadFile` and is
**not reachable from the package's "interface"** (`Clean`), plus authority-using
helpers in an in-package `scope_test.go` and an external `scope_external_test.go`.

```
FILES  DIRECT  scope.Unreached  ->  os.ReadFile
```

- **Whole-package scope confirmed:** `Unreached` is reported purely by package
  membership, not reachability from any interface — the empirical basis for
  rejecting the interface-rooted alternative (design Appendix A).
- **`_test.go` excluded confirmed:** neither `inTestAuthority` nor
  `externalTestAuthority` appears — Capslock's `go/packages` load (no `Tests:true`)
  drops test files from the analyzed build, so authority in `_test.go` is invisible.

---

## Assumption 6 — Reader attribution (design §11) ✔ *characterized, then superseded*

`readerattr.Parse(io.Reader)` holds no authority itself; under the plain strict
classifier what flows in decides. Each shell is analyzed in its own package load so
VTA sees only one concrete type.

- **clean** (`readercleanshell` feeds a `bytes.Reader`) → `Parse` **FILES-free**.
- **dirty** (`readerdirtyshell` feeds an `*os.File`) →

```
FILES  DIRECT  readerattr.Parse   ->  io.ReadAll  ->  (*os.File).Read
```

VTA flows the `*os.File` through the `io.Reader` and (because UNANALYZED is
excluded) *through* `io.ReadAll` down to `(*os.File).Read`. So §11's mechanism is
real: under the plain strict classifier, handing `Parse` an `*os.File` **would**
attribute FILES to `manifest`, which is why §11 has the shell wrap in a
`bytes.Reader`.

**But Finding 0b shows we shouldn't want that attribution at all.** Under the
ocap classifier (`(*os.File).Read` etc. → SAFE), the *same* dirty scenario leaves
`Parse` clean and attributes FILES to `Run` (the `os.Open` caller). So we **adopt
the ocap classifier and drop the §11 wrapping rule**: `manifest.Parse` is
authority-free by construction. The probes still stand as evidence of both
behaviors; the recommendation is Finding 0b.

---

## Assumption 2 — `CAPABILITY_SAFE` pruning end-to-end ✔

The draft `csvtool/app` composes `csvfile` (holds FILES via `os.ReadFile`) and
`toprow` (pure).

- **unpruned** → `app.Run` is attributed FILES transitively:

```
FILES  TRANSITIVE  app.Run  ->  csvfile.Read  ->  os.ReadFile
```

- **pruned** at `csvfile`'s interface via a per-run map
  `func <.../csvfile>.Read CAPABILITY_SAFE` (loaded with
  `interesting.LoadClassifier(..., excludeBuiltin=false)` so it merges with
  builtins) → `app` comes out **ambient-authority-free (empty)**.

This is FR5b in miniature: a component may compose a FILES-holding component and
remain authority-free, because traversal is pruned at the dependency's declared
interface. Absorbed deps (not in the map) stay absorbed — exactly the
component-dep-vs-absorbed-dep distinction.

---

## Design feedback (fold into the design before Step 3)

1. **Classifier mode is now settled: `excludeUnanalyzed=true`.** Add to design
   §5.4/§10 and wire it as the `StrictPolicy` classifier in Step 7's adapter.
   Record the trade-off (UNANALYZED no longer a signal; candidate post-MVP
   warning).
1b. **Adopt the "minting, not use" classifier and drop the §11 wrapping rule
   (Finding 0b).** Reclassify the `(*os.File)` handle-use methods `SAFE` (keeping
   `os.Open`/`os.ReadFile`/… as FILES) so ambient authority is attributed at the
   minting site. `manifest.Parse(io.Reader)` then holds no authority by
   construction, independent of its callers — restoring modular reasoning and
   removing the "wrap the manifest in a `bytes.Reader`" footgun from §11/§4.1.
   Wire the handle-method SAFE set into the adapter's per-run classifier (merged
   with the builtins, `excludeUnanalyzed`, and the boundary-prune map). Curate the
   symmetric network/exec/env use-methods when those capabilities first appear;
   document the stdio-globals loosening.
2. **Drop the review-B8 `sort.Slice` prohibition.** `sort.Slice` is clean because
   of Capslock's sort-call rewrite; `toprow` needn't avoid it. Keep the concrete
   `sort.Interface` only as a style choice, not a requirement.
3. **Confirmations (no change needed):** A4 key forms (both receiver forms;
   generic origin bracket-stripping), A2 init attribution + `func <pkg>.init`
   pruning (and the `init#1` wrapper form), whole-package scope, `_test.go`
   exclusion, and `CAPABILITY_SAFE` boundary pruning all behave exactly as the
   design assumes.
4. **Adapter mechanics validated for Step 7/9:** load via
   `analyzer.PackagesLoadModeNeeded`; `GetQueriedPackages` restricts reporting to
   the component's packages; `GetCapabilityInfo` with `GranularityFunction`;
   findings map cleanly to `{package, capability, DIRECT|TRANSITIVE, []Frame}`;
   prune maps are generated from `ssa.Function.String()` keys.
