# Idea honing — compositional component analysis

**Date:** 2026-08-04
**Status:** complete; design written.

> Recorded after the fact. The clarification was conducted as an ad-hoc design
> conversation rather than through the interactive-design question loop, so the
> questions below are reconstructed from that conversation in the order they were
> actually settled. Every answer is the decision the user made or confirmed;
> alternatives are recorded where a real choice existed.

---

## Q1 — Is one-hop analysis actually sound, given member-as-root?

**Question.** The rough idea claims a full call graph is unnecessary because members
are all roots. Does that hold?

**Answer.** Yes, and for two independent reasons, both already true in the codebase.

*Intra-component transitivity is already collapsed.* Every member function is a root,
so if member `A` calls member `B` which calls `os.ReadFile`, the call site is in `B`'s
body and `B` is a root. A one-hop scan of every member body finds it. A call graph
adds nothing.

*Boundary trust is already the model.* `cmd/arcc/app/app.go:200-229` builds
`PruneAt`/`PruneAtPackages` from resolved dependency symbols and hands them to Capslock
as a `CAPABILITY_SAFE` classifier. Authority behind a declared dependency is already
not attributed to the caller. Reading a dependency's surface from a published manifest
is therefore the same trust assumption discharged more cheaply — not a new one.

**Consequence.** The framing adopted for user-facing docs: *the component boundary is
where ambient authority becomes designated capability.* Pruning is not an
approximation of a transitive analysis; it is the semantic content of the model.

---

## Q2 — Scan call sites, or scan all references?

**Question.** The rough idea proposes finding "all the call sites". Is that enough?

**Answer.** No. Scan **all references**, and treat **imports as edges**.

**Rationale.** A `CallExpr`-only scan misses:

- **Func values.** `f := os.ReadFile; f(path)` has no statically resolvable call site,
  but it does have a reference to `os.ReadFile`.
- **Package init.** Importing a package runs its `init` with no call site anywhere.
  Capslock's rooting catches this transitively today; a scan will not unless imports
  are edges and the authority map carries per-package init entries.

Scanning `types.Info.Uses` / `types.Info.Selections` over all identifiers and selectors
is conservative, cheap, and additionally subsumes the existing `FuncValueEscapes`
machinery.

**Decided by the user explicitly:** "definitely scan the references and treat imports
as edges."

---

## Q3 — Is a syntactic scan acceptable for the FR5 call-boundary rule?

**Question.** FR5 currently uses VTA call edges. Does replacing them with a syntactic
scan lose precision?

**Answer.** It *gains* precision. The syntactic scan is more correct here, not merely
cheaper.

**Rationale.** `ResolveDependencyInterface` steps 9–11 (`goanalysis.go:1620-1730`) spend
~110 lines computing the implements-closure of every declared interface type and
injecting the concrete implementing methods into the dependency's allowed symbol set.
That exists solely to suppress false `CALLS_UNDECLARED_INTERFACE` findings: source says
`dep.Greeter.Greet()`, VTA resolves it to `(*dep.greeterImpl).Greet`, which is not a
declared symbol. The compensation over-corrects — it whitelists those concrete methods
even when source names them *directly*, which is exactly the violation FR5 exists to
catch.

"Does the source name a symbol outside the declared interface?" is a question about
what is written, not about what executes. Using reachability to answer it was the
original error.

**General rule adopted:** *boundaries are syntactic; authority is semantic.* Precompute
the semantic part once; do the syntactic part per component.

---

## Q4 — Can `absorbed_dependencies` be removed "without loss of generality"?

**Question.** The rough idea asserts absorbed dependencies can be replaced by
package-style components with no loss. Is that true?

**Answer.** Not unconditionally. It is true under an invariant that must be stated
explicitly:

> **Every component is either checked against its manifest, or explicitly marked
> unanalyzed and approved.**

**Rationale.** Absorbed means *analyze through it* — the absorber inherits the
authority. A component dependency means *prune and trust its declaration*. Converting
one to the other moves a checked property to an unchecked assertion **unless something
actually checks the new component**. In a monorepo with presubmit CI — and structurally
in Bazel, per Q16 — the first branch holds by construction, so the conversion is safe.

The user confirmed this assumption is reasonable to state for the target environment.

**Consequence accepted.** Authority attribution moves from the absorber to the
component that contains the code. This is finer-grained and arguably more honest:
absorption made the absorber accountable for a fact about someone else's code.
Authority is a join over the closure, and the join is invariant under where boundaries
are drawn.

**Alternative considered and rejected:** keeping an "authority-inheriting boundary" for
unowned code (a component dependency whose declared authority the depender inherits).
Rejected because Q7's `authority: UNKNOWN` covers the unowned case more directly, and
the invariant above makes inheritance unnecessary for owned code.

---

## Q5 — How is unowned (third-party) code modelled?

**Question.** Absorbed dependencies were typically unowned code. What replaces that?

**Answer.** A `PACKAGE_SURFACE` component declared **in your own tree**, naming foreign
packages as members.

**Rationale.** In Bazel, membership is by label, so the manifest's *location* is already
decoupled from its members' location — the "cannot write a manifest into the module
cache" problem does not arise. Because `PACKAGE_SURFACE` derives the surface from the
member packages, there is no symbol enumeration to maintain: adding a method to the
wrapped library causes no churn. `declared_authority` churns only when the library's
*authority* changes, which is exactly the signal worth reviewing.

**Alternative considered and rejected:** having arcc *generate* and check in a full
surface manifest for unowned code. Rejected by the user: it would cause churn on every
upstream API addition, and the component-level concerns are only (a) that my code
depends on the library and (b) what its authority is.

**Open item carried to the design:** native (non-Bazel) mode derives the component root
from the manifest's directory (`goanalysis.go:1372`) and rejects root overlap
(`:1401-1405`), so out-of-tree membership may not work there. The pattern-membership
path is separate and may already handle it — to be verified, not assumed.

---

## Q6 — Is a hand-written wrapper for unowned code sound, or just a guess?

**Question.** If a human writes `declared_authority` for a library they do not own, is
that a verified fact or a fiction?

**Answer.** Verified — **provided the wrapper is checked**, which Q4's invariant
guarantees. An initial framing that hand-written wrappers amount to fiction was wrong
and was withdrawn.

**Rationale.** Completeness is *forced by the check, not by discipline*. Declare
`members = [orm]` and omit its transitive dependencies, and the check fails immediately
on undeclared imports from `orm`. There is no way to pass by omission, so a passing
wrapper has a complete member set and a complete authority declaration by construction.

**Consequence that shapes Q7.** That enumeration is also what buys the **upgrade
signal**: version-bump the library, new authority appears, the check fails, someone
reviews it. So the wrapper is not merely more cumbersome than the alternative — it is
what makes upgrades visible.

---

## Q7 — Do we need an escape hatch for large unowned closures?

**Question.** Enumerating a 500-package closure to adopt arcc for new code in an
existing ecosystem is sound but prohibitive. Is an escape hatch needed?

**Answer.** Yes. A component that is **not analyzed**: its surface is the exported
symbols of its named member packages, and nothing else about it is checked.

**Rationale.** Beyond adoption cost, it dissolves the enumeration problem for a
principled reason: **you only ever need to name the packages your own code
references.** Your members are scanned; their imports and references are the edges; the
wrapped library's transitive closure is never referenced *by you*, so it never appears
as an edge target and never needs covering. The closure only mattered because an
*analyzed* wrapper must attribute its closure's authority.

**Alternative considered and rejected:** an `absorbed_deps`-like wildcard membership
primitive for `PACKAGE_SURFACE`. Rejected because it hides precisely the change worth
reviewing — a wildcard conceals a newly pulled-in transitive dependency, where
enumeration surfaces it as a diff.

**Guidance recorded.** The trade is the upgrade signal (Q6) for adoption cost. Leave
unanalyzed the libraries whose authority you have decided not to track; enumerate the
ones where you want an upgrade to stop you.

**Governance.** A manifest-only predicate — "no unanalyzed components in this subtree
except these approved ones" — makes it a ratchet rather than a leak: wrap everything
unanalyzed on day one, add the predicate, drive the exception list toward zero.

---

## Q8 — Is "unanalyzed" a new `interface_style`, or a separate axis?

**Question.** Should this be `UNANALYZED_PACKAGE_SURFACE`, or an orthogonal field?

**Answer.** A **separate axis**: keep `interface_style: PACKAGE_SURFACE` and add
`authority: UNKNOWN` (vs `DECLARED`).

**Rationale.** Surface computation and verification status are orthogonal, and the
second axis already exists (`own_check_runs`, `certification_reference`, and the
`certified`/`asserted` report vocabulary). Keeping them separate leaves surface
computation on one code path and permits an unanalyzed component with a declared
interface later.

**Counterargument weighed.** A distinct enum value fails loudly in a `switch` where a
forgotten boolean check fails silently — and failing silently here means treating
unanalyzed as analyzed. Judged acceptable because `UNKNOWN` must be threaded through
the authority lattice explicitly regardless (Q9), which removes the reliance on
exhaustiveness checking.

**Corroboration from real use.** The monorepo host had independently hand-rolled this
concept: it string-matches on package name in `defs.bzl` to tag `PACKAGE_SURFACE`
components' `.check` targets `manual`, which `defs.bzl:124` turns into
`own_check_runs = false`, rendering the boundary `asserted`. That is "this component is
not analyzed, trust its surface anyway" reached by hack because no spelling existed.

---

## Q9 — How is "no authority declaration" represented?

**Question.** Does an unanalyzed component declare empty authority?

**Answer.** No. `UNKNOWN` and empty are **different lattice elements** and must not be
conflated. An unanalyzed component contributes ⊤ — or a distinguished poisoning value —
to any bound it flows into.

**Rationale.** If `UNKNOWN` were spelled `declared_authority: []`, a subtree containing
an unanalyzed ORM would compute as pure. This is the same failure as "an unverified
leaf looks clean", which surfaced twice independently in this conversation; the schema
should therefore make authority provenance explicit everywhere rather than inferring it
from absence.

---

## Q10 — Do we need transitive / whole-tree analysis now?

**Question.** After removing absorption, reading one manifest no longer tells you the
truth about a component's closure. Does that need fixing before landing?

**Answer.** No — **tabled**. An initial claim that this property would be *lost* was
overstated and was withdrawn.

**Rationale.** The property never held for component dependencies. `csvtool`'s `app`
already declares no authority while depending on `csvfile`, which uses FILES.
Absorption was the only place it held, and removing it is deliberate. A component
declaring no authority while depending on components that have it is the *intended*
reading — the dependency is a safe abstraction at the component level. As the user put
it: a parser depending on a logging component does not acquire filesystem authority; it
acquires the ability to log.

**Design hint recorded for when it is picked up.** "No ambient authority except via
these three trusted leaves" reads more naturally as a property *of those leaves* than
as an exception list repeated in every predicate. Marking a component as an authority
barrier puts the trust decision where the trusted thing is. The schema affordance is a
single field, nearly free to reserve now. Inputs already exist on the Bazel side:
`ArccComponentInfo` propagates `transitive_manifests` (`component.bzl:391`).

---

## Q11 — Should `MEMBER_OVERLAP` be relaxed for `PACKAGE_SURFACE` components?

**Question.** Do we care if two wrapper components both attribute a library's authority
to themselves?

**Answer.** Keep the check unchanged, for `PACKAGE_SURFACE` too.

**Rationale.** The check is narrower than it appears: `checker.go:173-187` intersects
the *analyzed component's own members* against each resolved dependency's packages. It
is self-vs-dep, not dep-vs-dep. Two different wrappers both claiming `//base/go:log` is
**already** not flagged.

What does fire is layered wrappers — wrapper A lists `log` as a member *and* depends on
wrapper B which also lists it. That is a genuine contradiction: A claims `log` is
simultaneously inside and outside its boundary, and the call-boundary check cannot
decide whether a call to it is intra-component or cross-boundary.

**Residual risk noted, deferred.** Dep-vs-dep overlap is mostly benign (authority is a
union, so double counting is idempotent). The one real hazard is **divergent
declarations** — A says `log` is FILES, B says FILES+NETWORK — where the applicable
authority depends on lookup order. That is a tree-level consistency check, deferred
with Q10.

---

## Q12 — Does the stdlib classification predicate survive the authority map?

**Question.** Five stdlib predicates exist today (`hostpolicy.IsStdlibPath`,
`packagelayout.Layout.IsStdlibPackage:79`, `goanalysis.isStdlibPackage:533`,
`classifyStdlibPackage:550`, `stdlibClassifier:563`). Should they be consolidated onto
the host seam?

**Answer.** They are **deleted, not consolidated**. If the map is total, "is this
stdlib?" *is* "is this in the map?". Classification stops being a heuristic and becomes
data.

**Rationale.** What survives is a different shape: **enumerating the SDK** so the map
can be generated. That is a set-construction problem ("what packages does this SDK
contain"), not a path-shape predicate ("does this look stdlib-ish"). It runs once per
SDK rather than once per referent per check, it is total by construction, and it cannot
disagree with itself.

**Invariant this creates.** The map must be **total at package granularity**. Storing
only non-empty entries makes "not in the map" ambiguous between "not stdlib" and
"stdlib but safe". Symbol-level entries may be sparse:

- `pkg ∈ enumeration` → it is stdlib
- `symbol ∈ table` → that authority; otherwise empty

**Non-issues confirmed.** Stdlib `internal/...` packages are unimportable from outside,
so the exported surface is the whole reachable surface. Blank imports for side effects
resolve against the per-package init entries required by Q2.

---

## Q13 — Does that change when the stdlib map is built?

**Question.** The map was sequenced late. But the reference scan needs a
stdlib/non-stdlib dispatch on day one.

**Answer.** Split the map in two and move the first half early.

- **(a) Generator + lookup interface.** Run Capslock over the SDK with every exported
  symbol as a root. The machinery already exists — approximately what happens on every
  check today. No dependency on surface manifests. Initially generate on demand and
  cache, which sidesteps distribution entirely.
- **(b) Distribution, keying, pinning, fail-closed.** `(version, GOOS, GOARCH, tags)`,
  classifier-hash stamping, hermetic artifact. All the unknowns live here, and none of
  them are analysis concerns — they are packaging.

**Rationale.** Without (a) early, the scan gets built against the legacy predicates and
rewired later. With it, the predicates delete in the same change that introduces the
scan.

---

## Q14 — How are host-injected runtime packages classified?

**Question.** A host toolchain injects packages below the visible dep graph (the
concrete case: a generated-proto runtime). Under imports-as-edges, those become edges
to packages nobody declared. Trusted infrastructure, or stdlib-like?

**Answer.** It depends on the runtime, and **the default is an infra component**. Most
such runtimes are conceptually safe abstractions; a runtime that genuinely leaks
authority to its caller can instead be given stdlib-like treatment via the map.

**Rationale.** This is not two mechanisms — it is the Q1 boundary rule applied to a new
case. "Safe abstraction" = there is a boundary, authority is attributed to the runtime
and does not propagate. "Leaks authority" = no boundary, it flows to the caller.

**Consequence: no new mechanism is needed.** The infra-component path already works end
to end — `component.bzl:217-230` auto-attaches infra components based on the package
closure, and `checker.go:289-291` exempts `AutoAttached` deps from the
unused-dependency warning, which is exactly what a dependency nobody wrote requires.
Since injected runtimes are unowned code, the component describing one is a Q5 wrapper,
`authority: UNKNOWN` if its closure is not worth enumerating.

The friction report's §3 therefore reduces to its **adapter hook**: make the injected
packages *visible* so the existing attachment logic can see them.

**UX note.** The default matters because an injected import is one the author never
wrote. Failing with "undeclared dependency on a package you never named" is the wrong
failure; auto-attachment means the common case needs no declaration.

---

## Q15 — Is an unused-authority check needed?

**Question.** Nothing currently flags authority that is declared but never used —
`DeclaredAuthority` only ever *widens* the policy (`checker.go:311`,
`deriveEffectivePolicy`). There is an `UnusedDependency` warning but no
`UnusedAuthority`.

**Answer.** Yes, add it.

**Rationale.** Harmless today; not harmless once wrapper components (Q5) and tree
predicates (Q10) exist. Over-declaration is contagious — one component defensively
declaring FILES poisons every bound above it, and the natural response to a noisy
predicate is to declare more rather than less. Tight declarations are what make wrapper
manifests meaningful instead of defensive.

---

## Q16 — Can certification be made structural rather than self-declared?

**Question.** `facts.go:108` states plainly that `OwnCheckRuns` is *"read from the
dependency's own manifest and is not verified by arcc"*. Can the redesign fix that?

**Answer.** Yes. **Emit the surface manifest as an output of the check action.**
Consuming a dependency's surface manifest then means the check ran, because the file
would not exist otherwise.

**Rationale.** Bazel supplies version-correctness on top: the artifact is keyed to the
sources that produced it, so "checked at the version it is used at" becomes a build
graph invariant rather than a presubmit convention. This is a continuation of existing
behaviour — `defs.bzl:124` already derives `own_check_runs` from whether the target is
tagged `manual`.

**Native-mode limitation accepted by the user.** No action graph means no guarantee.
Mitigated cheaply: write an input hash into the emitted surface manifest so native mode
can *detect* staleness even though it cannot prevent it.

**Report vocabulary this yields:**

```
certified  — surface manifest produced by a check action
asserted   — surface manifest present, provenance unverifiable
stale      — input hash does not match the dependency's current sources
untrusted  — dependency is marked authority: UNKNOWN
```

---

## Q17 — What sequencing follows from "no re-import until this lands"?

**Question.** The monorepo PoC has landed on the current version with a larger-than-ideal
patch set, and no updated import will happen until the compositional work lands. Does
optimising for our own simplicity — rather than for incremental host benefit — change
the plan?

**Answer.** Yes, in four ways.

1. **Drop the dedup-load step.** It was a hedge whose entire justification was banking a
   speedup before the redesign landed. The reference scan removes goanalysis's SSA/VTA
   and takes Capslock out of the check path; surface manifests remove the per-dependency
   source loads. The dedup work is superseded by both. Keep the *measurement*, discard
   the refactor.
2. **Drop friction-report §2 entirely.** Patching five stdlib predicates upstream so
   they can be deleted two steps later (Q12) is pure churn with no re-import deadline in
   between.
3. **Invert absorbed-removal from last to first.** Originally sequenced last out of
   caution about breaking users; there are none before the re-import, and in this repo
   absorption appears only in test fixtures and an ad-hoc README walkthrough. Removing it
   first is pure deletion that shrinks every subsequent step.
4. **Add a profiling step first.** The PoC's ~50s must be attributed between closure SSA
   (which the redesign eliminates) and member-package type-checking (which it keeps). If
   the latter dominates, the map's priority drops relative to whatever does. Cheap
   information that could reorder everything below it.

**Requirement retained.** The next import must be at least incrementally easier. This is
satisfied by the redesign itself — the predicates deleting removes §2's patches, the
`authority: UNKNOWN` axis removes §5's tagging patch, the golden restructure attacks the
45-file problem against the final layout shape — plus two additive adapter hooks (§3
runtime injection, §4 SDK attrs) with `{}` defaults.

---

## Explicitly out of scope

- Transitive / whole-tree authority predicates (Q10).
- A bootstrap CLI that seeds a `PACKAGE_SURFACE` component from `bazel query` plus an
  authority pass — likely unnecessary given Q7. If revisited, note it cannot compute
  closures one component at a time: closures of different wrappers overlap, so it must
  know every already-declared component and stop at their boundaries.
- Dep-vs-dep declaration divergence checking (Q11).
- Friction-report items triaged as orthogonal: §1 (aspect provider bug), §5
  self-exemption, §6 golden work beyond the restructure, §7's `IsCanonicalPath` seam.
  See [`research/host-import-friction.md`](research/host-import-friction.md).

---

## Post-review decisions (2026-09-02)

An independent design review
([`2026-09-02-design-review.md`](2026-09-02-design-review.md)) raised nineteen
findings. The assessment, the decisions taken for each, and six decisions flagged for
user confirmation are recorded in
[`2026-09-02-design-review-response.md`](2026-09-02-design-review-response.md). Where a
decision there refines one above, the response document is authoritative; in particular
Q12's "symbol entries may be sparse" is superseded by a total symbol inventory (DR-05),
Q16's four-way vocabulary by three orthogonal status axes (DR-06), and Q7's rejection of
wildcard membership now also removes the existing pattern-membership feature (DR-16).
