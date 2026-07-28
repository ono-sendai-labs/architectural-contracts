# Design Review — Architectural Contracts MVP (Go)

Date: 2026-07-06. Senior-engineer review of `design/detailed-design.md` and
`implementation/plan.md`. Load-bearing claims were verified against the actual
Capslock source (`../../external/capslock`, checked out at `/home/xtof/git/external/capslock`),
not just the research notes. All findings below were discussed and resolved with
the user; resolutions have been folded back into the design and plan.

**Overall verdict:** strong design — the pure-core/shell split, the
absorb-vs-prune resolution of the two dependency kinds, and the plan's
vertical-slice sequencing are all right. But several findings were in the
"will not survive contact with real Go code" category, chiefly because the
design's model of "call edges land on symbols declared in interface files"
did not match how Go dispatch and SSA actually behave.

Severity groups: **A** — correctness of the core FR5/FR5b mechanism;
**B** — self-hosting showcase risks; **C** — spec gaps; **D** — plan-level notes.

---

## A. Correctness of the core mechanism (FR5/FR5b)

### A1. Interface-typed APIs break both FR5 and FR5b — the biggest gap ⛔ blocker

**Finding.** Capslock builds its call graph with **VTA**
(`analyzer/util.go:209`), which resolves dynamic dispatch to **concrete
methods**. The design assumed call edges from A into B land on symbols
*declared in B's interface files*. For the most idiomatic Go API shape — an
interface file declaring `type Store interface{...}` and a constructor
returning it, with the concrete `(*store).Get` defined in a non-interface
file — the call edge's callee is the concrete method: not in the declared
set (false `CALLS_UNDECLARED_INTERFACE`) and not in `PruneAt` (B's authority
leaks into A; the pruning showcase silently fails). Even concrete-type APIs
break when the type is declared in `api.go` but its methods are defined in
`impl.go` (very common). The CSV example dodged this only because
`csvfile.Read(path)` is a top-level func.

**Resolution (user: option 1 — closure rule).** The declared interface is a
*closure*: (a) exported top-level symbols declared in interface files, plus
(b) the full method set (within B's packages) of every exported type —
including interface types — declared in an interface file, **regardless of
which file defines the method bodies**. For exported *interface* types
declared in interface files, the in-component concrete implementations'
method keys enter the boundary/prune symbol set so VTA-resolved edges match.
Landed in design FR4/§5.3/§5.3b; implemented in `goanalysis`'s
`ResolveDependencyInterface` (needs `go/types` method-set computation).

### A2. Package `init` functions bypass pruning

**Finding.** Verified: Capslock's graph covers all functions including each
package's synthetic `init` (`analyzer/analyzer.go:628`), and SSA `init` of an
importing package calls the `init` of every imported package. So when A
imports component B, B's init-time authority (package-level
`var db = mustOpen(...)`, env reads in `func init()`) is reachable via A's
`init`, and B's `init` is never in `PruneAt` — the exact re-absorption FR5b
exists to prevent, with no prune point.

**Resolution (user).** Two parts:
1. **Mechanism:** when pruning component dep B, also emit
   `func <each-B-package>.init CAPABILITY_SAFE` into the custom map. The
   builtin `interesting.cm` has exact precedent
   (`func encoding/json.init CAPABILITY_SAFE`). Marking the synthetic root
   `init` prunes explicit `init#N` funcs and var initializers alike; B's own
   conformance run covers them (compositional trust).
2. **Explicitness — retired 2026-07-27.** This half required any explicit
   `func init()` in a component's packages to be declared in an interface file,
   under a new violation kind `INIT_OUTSIDE_INTERFACE`, on the reasoning that a
   package's `init` is implicitly part of its exposed interface (importing ⇒
   running it). It was **removed**, kind and all, by the 2026-07-25
   component-membership and authority-attribution design (its A2). The reason is
   supersession, not repudiation: once member packages became analysis roots,
   an init's authority is charged to the component whatever file it sits in, so
   the rule's only remaining job was documentation — and scoped to the interface
   package it would have fired in one narrow shape while an equally
   import-time-live init in a member implementation package went unmentioned.
   Synthetic inits from var initializers were, and remain, covered by the
   component's own capability check.

   **Part 1 above is untouched and still load-bearing.** The prune key
   `func <B-pkg>.init CAPABILITY_SAFE` is what actually prevents a dependency's
   import-time authority from re-absorbing with no prune point; it is
   independent of file placement, and removing part 2 does not weaken it.

### A3. Callbacks across the pruned boundary — "pruning never hides authority" was false as stated

**Finding.** Verified against `searchBackwardsFromCapabilities`
(`analyzer/analyzer.go:276`): the backward BFS skips edges whose **caller**
is SAFE. If A passes a function value into B's pruned interface
(`B.Process(f)`) and `f` comes from A's **absorbed dependency** and exercises
authority, the only path to the capability runs through B's SAFE frame — it
is reported against *nobody* (A's check is pruned; B's check never sees A's
absorbed dep). Mitigating fact (also verified): authority in **A's own
functions** invoked via callback *is* still caught, because Capslock reports
every queried-package function with its own path to a capability. The hole is
narrow but real, and the design's "pruning stays conservative so it never
hides real authority" claim was false as stated.

**Resolution (user: both).**
1. Document the limitation honestly (design §11 rewritten).
2. The checker emits a **warning** (`HIGHER_ORDER_BOUNDARY_CALL`) when a call
   edge into a pruned interface symbol passes function-typed values
   (detectable from `go/types` signatures at the call site). **Replaced
   2026-07-27:** this kind was removed by the 2026-07-25 design (its A5) because
   its polarity was wrong — it fired on the intentional plugin-struct pattern,
   where the callback body is the component's own member code and is therefore
   already attributed, and stayed silent on the case that actually escapes. The
   replacement, `ABSORBED_FUNC_VALUE_ESCAPE`, warns when member code takes the
   value of a function defined in an **absorbed** package without calling it —
   the body nobody analyzes.
3. **Future-work open question captured** (design Appendix D): functions/
   callbacks passed to higher-order functions are themselves a capability
   (analogous to a file handle); a proper treatment would attribute the
   callee-side authority of a passed function back to the caller that
   supplied it. Note `sort.Slice` is an instance of the same phenomenon —
   its `less` argument is a meta-capability — which is presumably why
   Capslock classifies it `unanalyzed` (see B8).

### A4. Generics and method-key normalization

**Finding.** `ssa.Function.String()` for generic instantiations includes type
arguments (`pkg.F[int]`), so a prune entry `func pkg.F` won't match — pruning
fails **open** (conservative direction: spurious violation rather than hidden
authority) and FR5 comparison fails the same way. Also: pointer- vs
value-receiver keys (`(pkg.T).M` vs `(*pkg.T).M`) are distinct nodes, and
promoted methods from embedded types have no declaration in the interface
files at all. The design specified the key format but not the normalization
rule; `goanalysis` generates keys from AST/types while Capslock matches SSA
strings — exactly where mismatches hide.

**Resolution (user: capture as open question; MVP as reviewer judges best).**
MVP behavior: normalize SSA function names by **stripping generic
type-argument brackets** before matching prune/boundary sets; emit **both**
pointer- and value-receiver key forms for every interface method; promoted
methods are covered by the A1 closure rule (method-set computation via
`go/types` includes promotions — verify the SSA wrapper-function key form
against fixtures in the Step 0 spike). Robust generic handling recorded as an
open question (design Appendix D).

### A5. Two call graphs per check — consistency and cost

**Finding.** Capslock's `buildGraph` is unexported and `CapabilityGraph` only
walks the capability-relevant subgraph, so `goanalysis` must build its own
call graph for FR5 — two expensive graph constructions per run. Worse than
cost: if the FR5 graph and Capslock's internal graph used different
algorithms, the two pillars could disagree about which edges exist.

**Resolution (user: agreed).** Settle Appendix D#3 now: `goanalysis` uses
**`vta.CallGraph`**, matching Capslock exactly; the double build is an
accepted MVP cost (documented). "One `packages.Load` feeds both" still holds
for loading — just not for graph construction.

## B. Self-hosting showcase risks

### B6. `report`'s JSON rendering triggers REFLECT

**Finding.** Plan Step 2 flagged the reflection risk for `manifest`
(prototext) but Step 3 put JSON rendering in the pure `report` component and
Step 10 names `report` in the guaranteed authority-free showcase. Verified in
`interesting.cm`: only `encoding/json.init` is SAFE; the package reaches
`reflect` (`package reflect CAPABILITY_REFLECT`). `report` as planned fails
its own `StrictPolicy` check.

**Resolution (user: structured value, either fine).** `report` renders
**text** purely and returns the report as a **structured value**; JSON
serialization moves to the **shell** (`cli` marshals the report struct with
`encoding/json`), and `cli`'s manifest honestly declares `REFLECT` alongside
its other authority.

### B7. Fact types living in `goanalysis` invert the architecture

**Finding.** The design defined `PackageFacts`/`CallEdge`/
`DependencyInterface` in `goanalysis` — a FILES/EXEC shell component — and
had the pure `checker` import it: the core depending on the shell, the
classic ports-and-adapters inversion, and an ugly headline for a tool whose
thesis is clean dependency structure (the flagship pure component would
declare a component dependency on an authority-holding component).

**Resolution (user: agreed).** New pure core component **`facts`**
(`internal/facts`) holds the data model; `goanalysis` (shell) *implements
against* core-defined types. Dependencies now point inward, mirroring how the
`capanalyzer` port already works. Plan Step 4's awkward "define shell types
but not shell loaders" becomes the natural "define the core's fact model."

### B8. Strict policy vs stdlib reality — never executed, only read

**Finding.** The MVP's demo-ability rests on "simple examples just work under
strict." Verified in `interesting.cm`: `package fmt` and `sort.Ints/Strings`
are SAFE, but **`sort.Slice` is `unanalyzed`** → a finding → strict failure.
The design's `toprow` ("imports only `sort`, `strconv`") passes or fails
depending on which sort function the author reaches for. The plan's first
Capslock contact was Step 7 — after the entire pure core was built on
researched-but-never-executed assumptions.

**Resolution (user: agreed, plus pull examples earlier).** New **Step 0
spike**: create the `examples/csvtool` packages early in rough form and use
them (plus tiny probe packages) to validate (a) the strict-safe stdlib
envelope, (b) `CAPABILITY_SAFE` pruning end-to-end, (c) key formats incl. a
method, a generic, and an `init`. Examples are then refined, not recreated,
in Step 8/9. User's observation, recorded: `sort.Slice` is an instance of A3
(the `less` callback is a meta-capability) — which is likely *why* it is
classified `unanalyzed`.

## C. Spec gaps (resolved per reviewer judgment, user delegated)

### C9. Pure `Parse` claimed a validation it cannot perform; "component root" undefined
Design §7 listed "interface file not under a component package's directory"
as a *parse* error, but resolving package patterns to directories takes
`go list`/FS. **Resolution:** validation split — `manifest.Parse` does
**syntactic** checks only; membership/existence checks move to the **shell**
after package resolution. "Component root" is defined as **the directory of
the manifest file** (manifests conventionally live at the component root);
`interface_files` are paths relative to it (multi-package components use
subdir paths like `store/api.go`).

### C10. FR3 allowed-import derivation unstated
**Resolution:** a component dependency B's **interface packages** := the
packages of B that contain at least one of B's interface files, derived from
B's own manifest. A may import exactly those. Types-only imports follow the
same allowlist (type *use* is governed by FR5's call-edge scope and the §11
"use beyond calls" caveat). The redundant `ComponentDependency.
interface_packages` cross-check field is **dropped** from the schema
(field number reserved).

### C11. `declared_authority` values unvalidated
A typo (`"FILE"`) would silently never match. **Resolution:** `manifest.Parse`
validates each entry against the known capability-name set (parse error).

### C12. Manifest graph consistency
**Resolution:** `ComponentDependency.manifest` paths are relative to the
**declaring manifest's directory**; the resolved manifest's `name` must match
`ComponentDependency.name` (tool error otherwise); package-membership overlap
between the analyzed component and any resolved dependency is a conformance
**error** (whole-graph overlap checking deferred with whole-graph mode).

### C13. Unused declared dependencies never rot away
**Resolution:** new **warning** `UNUSED_DEPENDENCY` when a declared component
or absorbed dependency matches no import of the component's packages.

### C14. Warnings and exit codes
**Resolution:** stated explicitly in design §7 — a report with warnings but
no violations exits `0`.

## D. Plan-level notes

### D15. Release workflow premature in Step 1
**Resolution (user):** release workflow **deferred to Step 10**; CI stays in
Step 1 and lands as early as possible (user has been bitten by
local-vs-GH-Actions divergence before).

### D16. Capslock is pre-1.0
**Resolution:** pin the Capslock version in `go/go.mod`; upgrades are
deliberate, reviewed events (note added to plan Step 7).

### D17. Step 8's failing variant referenced Step 9's `app`
**Resolution:** the Step 8 failing fixture is renamed (`absorbapp`, a
throwaway variant); the real `app` composition root arrives in Step 9.

### D18. "One property, two views" oversells single-component mode
"Nothing outside B calls B's architecture-private symbols" holds only over
the set of *checked* components; unchecked consumers are the common case
during adoption. **Resolution:** one honest scoping sentence added where the
claim is made (design §5.3b).

---

## Follow-ups captured for post-MVP (Appendix D of the design)

- **Callbacks as capabilities (A3):** attribute authority of passed function
  values to the supplier; needs a model — a func value crossing a component
  boundary is a capability grant.
- **Robust generic-symbol matching (A4)** beyond bracket-stripping.
- Whole-graph check mode (pre-existing open item, unchanged).
