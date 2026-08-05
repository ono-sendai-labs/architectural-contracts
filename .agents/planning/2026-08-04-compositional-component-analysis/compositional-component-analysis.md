# Compositional component analysis

**Date:** 2026-08-04
**Status:** open design — not implemented. Written from a design conversation to
scope a replacement for the whole-program analysis model used by `arcc check`.

> Today a component check builds SSA and a VTA call graph over the component's
> entire transitive dependency closure — twice — and separately type-checks every
> declared dependency from source. Cost scales with the closure, not the
> component.
>
> Two changes recently landed make that unnecessary. Membership is now explicit and
> **every member function is an analysis root**, which collapses intra-component
> transitivity. And capability findings are already **pruned at declared dependency
> boundaries**, so authority behind a boundary is already not attributed to the
> caller.
>
> What remains is to classify the edges that *leave* a component. That needs no
> call graph: a reference scan over member sources, a per-SDK precomputed map of
> the standard library's authority surface, and a small surface manifest published
> by each dependency's own check.
>
> The result is faster, but the reason to do it is that component checks become
> **independent, cacheable, and composable**.

## Part 0 — Why one hop is enough

The soundness argument has two halves, both already true in the codebase.

**Intra-component transitivity is already collapsed.** Every member function is a
root. If member `A` calls member `B` which calls `os.ReadFile`, the call site is in
`B`'s body, and `B` is a root. A one-hop scan of every member body finds it. Building
a call graph to discover that `A` transitively reaches `os.ReadFile` adds nothing —
the finding is already attributed to the component either way.

**Boundary trust is already the model.** `cmd/arcc/app/app.go:200-229` builds
`PruneAt`/`PruneAtPackages` from resolved dependency symbols and hands them to
Capslock as a `CAPABILITY_SAFE` classifier. Authority behind a declared component
dependency is *already* not attributed to the caller; the report annotates the
boundary `certified` or `asserted` and moves on. Looking a dependency's surface up
in a published manifest instead of walking into its source is therefore not a new
trust assumption — it is the same assumption, discharged cheaply.

The framing that follows from this, and that should be stated in user-facing docs:

> **The component boundary is where ambient authority becomes designated
> capability.** A parser depending on a logging component does not acquire
> filesystem authority; it acquires the ability to log.

Pruning is not an approximation of a transitive analysis. It is the semantic
content of the model. `certified`/`asserted` annotate *the claim that the boundary
is a real abstraction*, not the claim that code was scanned.

### Measured cost of the status quo

```
arcc check examples/csvtool/app       →  2.1s wall,  6.4s CPU   (4 trivial packages)
arcc check internal/goanalysis        →  3.8s wall, 13.5s CPU   (1 component)
```

Nearly all of that is fixed overhead proportional to the dependency closure. Three
distinct sources:

1. **Double SSA build.** `goanalysis.go:274-279` runs `ssautil.AllPackages` +
   `prog.Build()` + `vta.CallGraph`. `capslockadapter.go:168` then calls
   `packages.Load` again and Capslock builds its own SSA program and call graph over
   the same closure.
2. **Per-dependency source loads.** `ResolveDependencyInterface`
   (`goanalysis.go:1440-1470`) loads each dependency with
   `NeedDeps|NeedSyntax|NeedTypes|NeedTypesInfo` over `./...` — a full type-check of
   every dependency's tree, on every run of every dependent.
3. **Stdlib SSA**, rebuilt on every invocation.

Parts 1–3 below remove all three.

## Part 1 — A reference scan replaces the call graph

### What is scanned

For each member package, walk the AST and resolve every reference to a symbol via
`types.Info`. Classify the referent's package:

| Referent | Verdict |
| --- | --- |
| A member package | ignore (intra-component) |
| Standard library | look up the authority map (Part 2); check against declared authority |
| A declared component dependency | check the symbol against that dependency's surface manifest (Part 3) |
| Anything else | `UNDECLARED_DEPENDENCY` violation |

Two things must be edges, not just call sites:

- **References, not calls.** `f := os.ReadFile; f(path)` has no statically resolvable
  call site, but it does have a reference to `os.ReadFile`. Scanning every
  identifier and selector resolution (`types.Info.Uses`, `types.Info.Selections`) is
  conservative, cheap, and subsumes the existing `FuncValueEscapes` machinery.
- **Imports.** Importing a package runs its `init`, with no call site anywhere.
  Today Capslock's rooting catches init-time authority transitively; a scan will not
  unless imports are edges and the authority map carries per-package init entries.

### What is no longer needed

No SSA. No VTA. No Capslock at check time. No source for anything outside the
component — dependencies and the standard library resolve from compiled export
data. Note that full type-checking of *member* packages is still required: resolving
`f.Read` to `(*os.File).Read` needs `types.Info`.

### Syntactic is not just cheaper here — it is more correct

`ResolveDependencyInterface` steps 9–11 (`goanalysis.go:1620-1730`) spend ~110 lines
computing the implements-closure of every declared interface type and injecting the
concrete implementing methods into the dependency's allowed symbol set. That
machinery exists solely to suppress false `CALLS_UNDECLARED_INTERFACE` findings:
source says `dep.Greeter.Greet()`, VTA resolves it to `(*dep.greeterImpl).Greet`,
which is not a declared symbol.

The compensation over-corrects. It whitelists those concrete methods even when
source names them *directly*, which is exactly the violation FR5 exists to catch.

The underlying error is using a reachability analysis to answer a syntactic
question. "Does the source name a symbol outside the declared interface?" is about
what is written, not about what executes. A reference scan answers it directly, and
those 110 lines plus their fixtures delete.

**The general rule this establishes: boundaries are syntactic; authority is
semantic.** Precompute the semantic part once (Part 2); do the syntactic part per
component.

### The approximation boundary, stated honestly

A reference scan does not resolve dynamic dispatch. This is tolerable *because of a
decision already made*, and that dependency should be recorded: the
`fileHandleUseMethods` reclassification (`capslockadapter.go:32-40`) attributes
authority to the **minting** site rather than to handle use. Minting sites —
`os.Open`, `net.Dial`, `exec.Command`, `os.Getenv` — are essentially always
statically resolved package-level functions. `w.Write(...)` through an `io.Writer`
therefore needs no resolution.

The ocap attribution rule is load-bearing for the soundness of the syntactic
approximation. If handle-use methods were ever reclassified back to
capability-bearing, this design would break.

Remaining gaps needing an explicit policy rather than an omission:

- **Stdlib interface methods** (`io.Writer.Write` and friends) have no single
  authority. The map needs a decision — most likely "contributes nothing", given
  the minting rule — recorded as a decision.
- **`//go:linkname` and assembly** bypass the surface map entirely. Detect the
  pragma and classify `AnalysisDefeating`.

## Part 2 — The precomputed standard-library authority map

Generate once per SDK: for every exported standard-library symbol, the set of
ambient authorities its implementation transitively reaches. Capslock is still the
right tool for *producing* this — it is the only place a call graph is still needed.

Requirements:

- **Keyed on (Go version, GOOS, GOARCH, build tags).** `net`'s resolver, `os/user`,
  and `syscall` differ across all of these. `netgo`, `purego`, and cgo on/off change
  the answer.
- **Fail closed.** Running against a toolchain with no map must be an error.
  Silently analyzing Go 1.26 sources against a 1.25 map is a soundness failure with
  no symptom.
- **Stamped with the classifier hash.** The map must be generated with the same
  classifier the check assumes, including the `fileHandleUseMethods` reclassification.
  Drift between generation and use is silent otherwise.
- **Per-package init entries**, so imports-as-edges (Part 1) has something to
  resolve against.
- **A canned explanation path per symbol**, so `os.ReadFile → FILES` is not an
  unexplained oracle. Today findings carry a full `CallPath` rendered as evidence
  (`checker.go:317-320`) and the README advertises it; the replacement is shorter
  and more readable but must not be evidence-free.

This fits the hermetic direction well: the SDK is pinned, so the map becomes a
build artifact of the SDK repo rather than a vendored blob with a staleness problem.

## Part 3 — Surface manifests make checks composable

Each component publishes a **surface manifest**: for a declared-interface component,
its interface symbols; for a `PACKAGE_SURFACE` component, its member package list
(the surface is implicitly everything those packages export). Dependents read that
file instead of loading the dependency's source.

This is the change that matters most, and speed is only its symptom:

- Component checks become **independent actions** whose inputs are the component's
  own sources plus N small files. Check cost stops scaling with the dependency
  closure entirely.
- It maps directly onto the Bazel action graph, and results become cacheable.

### Certification becomes structural

`facts.go:108` is explicit about the status quo: `OwnCheckRuns` is *"read from the
dependency's own manifest and is not verified by arcc."* A component can claim
`own_check_runs: true` and be wrong.

**Emit the surface manifest as an output of the check action.** Then consuming a
dependency's surface manifest means the check ran, because the file would not exist
otherwise. Bazel supplies version-correctness on top: the artifact is keyed to the
sources that produced it, so "checked at the version it is used at" becomes a build
graph invariant rather than a presubmit convention. (`defs.bzl:124` already derives
`own_check_runs` from whether the target is tagged `manual`, so this is a
continuation, not a new idea.)

Native (non-Bazel) mode cannot offer this guarantee, and that limitation is
accepted. Mitigate cheaply: write an input hash into the emitted surface manifest so
native mode can *detect* staleness even though it cannot prevent it. The boundary
annotation vocabulary then covers both modes uniformly:

```
certified  — surface manifest produced by a check action
asserted   — surface manifest present, provenance unverifiable
stale      — input hash does not match the dependency's current sources
untrusted  — dependency is marked unanalyzed (Part 5)
```

## Part 4 — Absorbed dependencies are removed

`absorbed_dependencies` means *analyze through it*: the absorber inherits the
absorbed code's authority. A component dependency means *prune and trust its
declaration*. These are not interchangeable in general — converting one to the other
moves authority from a checked property to an unchecked assertion.

They **are** interchangeable under a stated invariant:

> Every component is either checked against its manifest, or explicitly marked
> unanalyzed and approved.

In a monorepo with presubmit CI — and structurally, under Part 3, in Bazel — the
first branch holds by construction. An absorbed package becomes a `PACKAGE_SURFACE`
component whose own check verifies its declared authority. Nothing becomes
unverified; attribution simply moves to the component that actually contains the
code, which is the component whose sources determine the answer. Absorption made the
absorber accountable for a fact about someone else's code.

Authority in this model is a join over the closure, and the join is invariant under
where boundaries are drawn. Absorption and infra components partition the same total
differently. What changes is who declares it.

Deleted along with the concept: `FuncValueEscapes` (subsumed by reference scanning),
`BodilessAbsorbedPackages`, `PruneAtPackages`, the absorbed-overlap check
(`checker.go:189-205`), and the absorbed-specific fixtures.

The README walkthrough (`README.md:296-340`) teaches absorption as its
`UNDECLARED_AUTHORITY` example and must be rewritten.

### `MEMBER_OVERLAP` stays, for `PACKAGE_SURFACE` too

Worth recording because it is narrower than it looks: `checker.go:173-187` intersects
the *analyzed component's own members* against each resolved dependency's packages.
It is a self-vs-dep check, not dep-vs-dep. Two different wrapper components both
claiming `//base/go:log` is **already** not flagged.

What does fire is layered wrappers — wrapper A lists `log` as a member *and* depends
on wrapper B which also lists it. That is a genuine contradiction: A claims `log` is
simultaneously inside and outside its boundary, and the call-boundary check cannot
decide whether a call to it is intra-component or cross-boundary. Keep the check.

Dep-vs-dep overlap is mostly benign — authority is a union, so double counting is
idempotent, and symbol lookup succeeding via either dependency passes either way.
The one real hazard is **divergent declarations** (A says `log` is FILES, B says
FILES+NETWORK), where the applicable authority depends on lookup order. That is a
tree-level consistency check, deferred.

## Part 5 — Unowned code

### Wrapper components are verified, not trusted

A `PACKAGE_SURFACE` component declared in your own tree can name foreign packages as
members. In Bazel, membership is by label, so the manifest's *location* is already
decoupled from its members' location — the "cannot write a manifest into the module
cache" problem does not arise:

```starlark
go_component(
    name = "flag_component",
    interface_style = PACKAGE_SURFACE,
    members = [
        "//base/go:flag",
        "//base/go/importpath",
        "//base/go/taskstatus",
        "//base/go/gccgodemangle",
    ],
    declared_authority = [REFLECT, UNSAFE_POINTER],
)
```

Because `PACKAGE_SURFACE` derives the surface from the member packages, there is no
symbol enumeration to maintain: adding a method to the wrapped library causes no
churn. `declared_authority` churns only when the library's *authority* changes,
which is precisely the signal worth reviewing.

Critically, a wrapper's completeness is **forced by the check, not by discipline**.
Declare `members = [orm]` and omit its 500 transitive dependencies, and the check
fails immediately on undeclared imports from `orm`. There is no way to pass by
omission, so a passing wrapper has a complete member set and a complete authority
declaration by construction.

That enumeration is also what buys the **upgrade signal**: version-bump the library,
new authority appears, the check fails, someone reviews it.

Native mode needs a story here. `ResolveDependencyInterface` derives the component
root from the manifest's directory (`goanalysis.go:1372`) and rejects root overlap
(`:1401-1405`), so out-of-tree membership may not work outside Bazel. The
pattern-membership path is separate and may already handle it — verify rather than
assume.

### `authority: UNKNOWN` — the adoption escape hatch

Enumerating a 500-package closure to adopt arcc for new code in an existing
ecosystem is sound but prohibitive. Add a component that is **not analyzed**: its
surface is the exported symbols of its named member packages, and nothing else about
it is checked.

This also dissolves the enumeration problem, and not by fiat: **you only ever need
to name the packages your own code references.** Your members are scanned; their
imports and references are the edges; the wrapped library's transitive closure is
never referenced *by you*, so it never appears as an edge target and never needs
covering. The closure only mattered because an *analyzed* wrapper must attribute its
closure's authority. An unanalyzed one attributes nothing, so it needs no closure. A
wildcard-membership primitive would have hidden the same problem instead of removing
it.

It is also free in the build graph: an unanalyzed component runs no arcc action,
contributing only a surface manifest.

**Model it as a separate axis, not a third `interface_style`.** Surface computation
and verification status are orthogonal, and the second axis already exists
(`own_check_runs`, `certification_reference`, `certified`/`asserted`). Keep
`interface_style: PACKAGE_SURFACE` and add `authority: UNKNOWN` (vs `DECLARED`).
This keeps surface computation on one code path and permits an unanalyzed component
with a declared interface later.

**`UNKNOWN` must not be spelled `declared_authority: []`.** Unknown and empty are
different lattice elements. An unanalyzed component must contribute ⊤ — or a
distinguished poisoning value — to any bound it flows into, or a subtree containing
an unanalyzed ORM computes as pure. This is the same failure as "an unverified leaf
looks clean", which has now appeared twice; the schema should make authority
provenance explicit everywhere rather than inferring it from absence.

**What to leave unanalyzed.** The trade is the upgrade signal for adoption cost.
Leave unanalyzed the libraries whose authority you have decided not to track;
enumerate the ones where you want an upgrade to stop you.

**Governance makes it a ratchet, not a leak.** A manifest-only predicate — "no
unanalyzed components in this subtree except these approved ones" — turns blind trust
into a reviewable list with a name on it, and gives adoption a burn-down: wrap
everything unanalyzed on day one, add the predicate, drive the exception list toward
zero as manifests arrive upstream.

## Part 6 — Unused authority

`DeclaredAuthority` only ever *widens* the policy (`checker.go:311`,
`deriveEffectivePolicy`). Nothing flags authority declared but never used. There is
an `UnusedDependency` warning; there is no `UnusedAuthority`.

Harmless today. Not harmless once wrapper components and tree predicates exist:
over-declaration is contagious, one component defensively declaring FILES poisons
every bound above it, and the natural response to a noisy predicate is to declare
more rather than less. Tight declarations are what make Part 5's wrapper manifests
meaningful instead of defensive.

## Deferred

**Transitive / whole-tree analysis.** Considered and tabled. The concern that
motivated it — "reading one manifest no longer tells you the truth about the
closure" — was overstated: that has never been true for component dependencies.
`csvtool`'s `app` already declares no authority while depending on `csvfile`, which
uses FILES. Absorption was the only place the property held, and removing it is
deliberate. A component declaring no authority while depending on components that
have it is the *intended* reading — the dependency is a safe abstraction at the
component level.

When it is picked back up, one design hint: "no ambient authority except via these
three trusted leaves" reads more naturally as a property *of those leaves* than as
an exception list repeated in every predicate. Marking a component as an authority
barrier puts the trust decision where the trusted thing is, and lets every predicate
above it compute uniformly with no lists to maintain as the tree changes. If that is
the eventual shape, the schema affordance is a single field, nearly free to reserve
now. The inputs already exist on the Bazel side: `ArccComponentInfo` propagates
`transitive_manifests` (`component.bzl:391`).

**A bootstrap CLI** that seeds a `PACKAGE_SURFACE` component from `bazel query` plus
an authority pass. Likely unnecessary given `authority: UNKNOWN`. If revisited, note
that it cannot compute closures one component at a time: closures of different
wrappers overlap, so it must be aware of every already-declared component and stop
at their boundaries, emitting a component dependency instead of a member. That
closure-partitioning behavior, not the query, is what determines whether such a tool
is usable.

**Dep-vs-dep declaration divergence** (Part 4).

## Design decisions to nail

- **Stdlib interface methods** in the authority map: contribute nothing, or ⊤?
- **`authority: UNKNOWN` propagation.** Exact lattice element and how it renders in
  a report that has no tree analysis yet.
- **Native-mode out-of-tree membership** — does the pattern-membership path already
  permit a manifest outside its members' tree?
- **Surface manifest format and provenance stamp.** What the input hash covers, and
  whether it is a separate file or a field.
- **Map distribution.** Vendored artifact vs generated from the pinned SDK, and how
  the version/tag key is discovered at check time.
- **Well-formedness rules** (`init` and method placement) are unaffected in
  principle but should be re-confirmed against a scan that no longer builds SSA.

## Implementation notes and sequencing

Ordering is constrained:

1. **Deduplicate the load and cache dependency surfaces.** Sources (1) and (2) of the
   measured cost. Large, independently bankable, no semantic change. Do this first —
   it also tells you how much of the remaining time the stdlib map is actually worth.
2. **Reference scan + imports-as-edges**, replacing VTA call edges. Deletes the
   implements-closure workaround. FR5 changes behavior here (correctly); expect
   fixture churn.
3. **Surface manifest emission from the check action**, plus the provenance stamp
   and the `certified`/`asserted`/`stale` vocabulary.
4. **The stdlib authority map**, with fail-closed version keying.
5. **`UnusedAuthority`** — gates the wrapper story in Part 5.
6. **`authority: UNKNOWN`**, with the lattice handled explicitly from the start.
7. **Remove `absorbed_dependencies`** and rewrite the README walkthrough.

Regression fixtures worth writing early:

- A member taking a func value of a stdlib authority function without calling it
  (`f := os.ReadFile`) must be attributed — the case a call-site-only scan misses.
- A member importing a package with init-time authority and never calling into it
  must be attributed — the imports-as-edges case.
- A member calling a dependency's declared interface method where the concrete
  implementation is unexported must **pass** — today's implements-closure workaround
  makes it pass for the wrong reason; it must still pass once that code is gone.
- A member calling a dependency's concrete method *directly*, where that method
  implements a declared interface, must **fail** — today it wrongly passes.
- A component depending on an `authority: UNKNOWN` component must not compute as
  pure in any bound.
