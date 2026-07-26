# Idea honing — component membership and authority attribution

## Q1 — How should the interface-file well-formedness rules be scoped once member implementation packages become analysis roots?

**Framing correction established before asking.** The source note names two rules
as needing rescoping. Only one does:

- **Method placement (`METHOD_OUTSIDE_INTERFACE`, `checker.go:152-165`) is already
  interface-scoped.** It fires only when the receiver's type is itself declared in
  an interface file (`exists && interfaceFiles[declFile]`). A member
  implementation package's own types and methods never trigger it. No change
  needed.
- **The init rule (`INIT_OUTSIDE_INTERFACE`, `checker.go:166-171`) is the crux.**
  It iterates every package in `Facts.Packages` unconditionally, so making member
  implementation packages roots would flag every ordinary `func init()` in them.

Review A2's rationale for the init rule was that importing a package runs its
init, so init behavior is de facto part of the exposed interface — which stays
transitively true for members, since the interface imports them. Options
considered: (a) treat the placement rule as redundant for members and rely on
attribution; (b) interface-scoped violation plus a new member-wide warning;
(c) interface-scoped silently (the note's literal proposal); (d) keep it
member-wide (purist A2).

**Answer: (a) — the placement rule is redundant for members; drop it there.**

A2's *soundness* concern was import-time authority re-absorbing with no prune
point. That is already handled by capability attribution: once members are
analysis roots, their init authority **is** charged to the component. The
placement rule then serves only documentation, so it stays a violation for the
interface package and members are unconstrained. Soundness rests on attribution
rather than on file placement.

## Q2 — What is the authoritative definition of component membership?

**Framing established before asking.** "The remainder becomes an error" comes
free: `checker.go` already reports `UNDECLARED_DEPENDENCY` for any member import
that is not covered, absorbed, standard library, or in-component. That rule only
stays quiet today because the Bazel rule emits blanket absorbed entries for each
label's whole transitive tree. So Part 2 is largely a rules-layer change plus a
membership declaration.

Three definitions were on the table: unify on location and demote R4 (FR1's
"packages in the component's own tree" as the single model, with the Bazel glob
constrained to a subtree of the component root); keep `members` as a Bazel-only
attribute with two coexisting mechanisms; or make membership first-class in the
manifest.

**Answer: membership becomes first-class in the manifest.**

Driving requirement: in Bazel, `members` must be able to name packages **anywhere,
including outside the component root** — in practice often "all packages in a
tree" via a subpackages wildcard. That kills the location-unification option:
once membership is not derivable from geography, the checker can only learn it
from a declaration, and today that declaration exists only in the generated
layout's `roots` (machine-facing). Declaring it in the manifest is the direct fix
for the note's actual complaint, since a reviewer then reads membership in the
same file as the interface and the declared authority, and a membership change
shows up in a manifest diff.

Schema churn is a non-issue: the project is experimental, unpublished, with a
single user and no stability guarantees.

Two refinements agreed alongside:

- **The manifest is authoritative and the layout must agree.** The layout's
  `roots` must equal the manifest's members; a mismatch is an error. Emitter bugs
  — the silent membership drift that motivated Part 2 — become a hard failure.
- **It gives Q1 a crisp boundary.** With membership declared rather than derived,
  "member" has one meaning in both modes, which the interface-only init rule keys
  on.

Cost accepted, addressed in Q3b: FR1 guaranteed disjoint membership structurally
(`PACKAGE_OVERLAP` fell out of disjoint roots). With members anywhere, two
components can claim the same package.

## Q3a — What form should the members declaration take in the manifest?

Options: patterns in the schema with emitters writing the expansion; patterns
carried as written and expanded by the checker; or exact import paths only.

**Answer: patterns in the schema, expanded by emitters.**

The schema accepts import-path patterns (`example.com/comp/...`) and exact paths,
reusing the idiom `absorbed_dependencies` already has, so hand-written native
manifests stay terse. The Bazel emitter always writes the **fully expanded
literal list**, making a generated manifest a diffable record — so a
`:__subpackages__` glob that swept in something unexpected is visible in review,
which is the failure mode Part 2 exists to prevent.

## Q3b — How is membership overlap between components detected?

Options: local checking only, local now with a repo-wide check as follow-on, or
local permanently.

**Answer: local now; repo-wide uniqueness check designed later.**

The checker errors when a member overlaps a resolved `component_dep`'s packages
or the component's own absorbed entries — both catchable from what a single check
already loads. Overlap with an unrelated component elsewhere in the repo is a
**documented limitation**, with a repo-wide membership-uniqueness check (natural
in Bazel as one test over all `go_component`s) deferred to a follow-on. Note that
double-claiming over-attributes, which is fail-closed.

## Q4 — What does `absorbed_dependencies` become, and what happens to the residual Part 1 gap?

With `members` able to name packages anywhere and patterns expressing whole
trees, `absorbed_dependencies` loses its "transitive shorthand" justification.
What remains is an **attribution difference**: members are roots (every function
analyzed — *ownership*), absorbed packages are not (analyzed only where reachable
— *use*). A second asymmetry makes that distinction load-bearing: in layout mode
`Facts.Packages` is the root set, so only members get their import edges
FR2-checked; promoting a vendored tree to member also demands declaring every
external dependency of that tree.

Options: keep both kinds and warn on the residual; collapse absorbed into members
entirely; keep both with a per-entry attribution knob; keep both and document the
residual.

**Answer: keep both kinds, and close the residual with a warning.**

Rationale from the user: in an ideal world there are no `absorbed_deps` and
everything is charged to a component, but that is burdensome on existing code.
`absorbed_deps` are what make it cheap to introduce components as **boundaries**
into an existing architecture — to draw a boundary and deliberately ignore what
is inside a big blob behind it — and the contents can then be refined
incrementally into real components over time. That adoption path is worth keeping
the second attribution mode for.

## Q5 — What triggers the residual warning?

**Findings established before asking.**

- **Capslock never traverses forward from entry points.** `forEachPath`
  (`capslock/analyzer/analyzer.go:714-780`) runs a **backwards BFS from the
  capability-bearing node** (e.g. `os.Open`) over *incoming* call edges, and
  reports any function whose package is in `queriedPackages`. Membership of the
  **defining package** in the query set is the entire criterion: a func value
  whose body lives in a member package is charged even if nothing in the
  component ever calls it, and regardless of how or where it escapes. This
  confirms the user's expectation about the plugin-struct pattern — no
  special-casing or suppressor is needed for it.
- **Anonymous closures are covered.** Running the `funcvalue` check yields
  evidence `example.com/aspect/funcvalue.init$1 → os.Open`, i.e. a closure
  reported via its enclosing package.
- **Calling into absorbed code attributes correctly.** The backwards BFS walks
  *through* the (unqueried) absorbed function to its member caller, which is
  reported. Only taking the **value** escapes.
- **The existing `HigherOrderBoundaryCall` warning has the wrong polarity**
  (`checker.go:240-246`): it fires when a *declared* boundary call passes a func
  value — precisely the intentional plugin case the user does not want warnings
  for.

The user's proposed refinement (do not warn when the crossing happens via a
declared component interface, since that is what makes it intentional) turns out
to be unnecessary under the chosen rule, because bodies living in members are
already analyzed.

**Answer: warn when member code references an absorbed package's function as a
value rather than calling it.**

Purely local — no boundary dataflow, no tracking through structs, returns or
assignments — because where the value goes is irrelevant: taking it at all is
what leaves the body unanalyzed. Catches the note's Part 1 example exactly, and
replaces `HigherOrderBoundaryCall`.

## Q6a — How is the analysis platform determined?

Options: declare it in the layout and thread a `build.Context`; that plus
multi-platform verification; or keep `build.Default` and merely fail loudly.

**Answer: declare it in the layout and thread a `build.Context`.**

The layout JSON carries GOOS, GOARCH, build tags and cgo, and the loader builds a
`build.Context` from it instead of using `build.Default`. This makes the layout a
complete description of the analysis, removes ambient environment from a check
advertised as hermetic and cacheable, and gives hosts with custom `gotags` a
declared way to pass them.

Multi-platform verification (so authority in a `_windows.go` file is not
invisible on Linux) was considered and **not** taken in this pass: it needs a
per-platform check story in the rules and per-platform reporting. It stays a
known limitation.

**Added later:** the mechanism that *populates* the platform description belongs
in the **adapter seam** (`go_adapter.bzl`), not in the shared rules — it will
almost certainly work differently in the monorepo than under rules_go. The
layout field and the loader's `build.Context` construction stay host-agnostic;
only the extraction of GOOS/GOARCH/tags/cgo from the host's Go rules is
swappable.

## Q6b — What should `ValidateInterfaceFiles` do with build-constraint-excluded interface files?

Options: report and error on an empty interface; error on any skipped file;
report only.

**Answer: report skipped files, and error when the interface ends up empty.**

Skipping stays — it is right for a genuinely cross-platform interface — but each
skipped file is surfaced as a warning naming the file and the constraint that
excluded it, and a component whose entire interface is excluded on the analysis
platform is a hard error rather than a vacuous pass.

## Q7a — How should standard-library membership be classified?

**Answer: a package is standard library only when module metadata and the host
path policy both agree.**

Fixes both directions: a driver-loaded dependency with no `Module` is no longer
stdlib (the heuristic disagrees), and a host-rewritten dotless path that has a
`Module` is no longer stdlib (the metadata disagrees). Disagreement fails closed
— a visible, fixable `UNDECLARED_DEPENDENCY` rather than a silent pass.

## Q7b — How should the `CanonicalizePath` identity requirement be handled?

**Answer: state it in the exported contract and assert it at runtime.**

Document the identity-on-loader-paths requirement in `hostpolicy`'s contract, and
check it at load time (`canonicalize(p) == p` for loader-reported package paths),
failing closed with a clear message. A host that picks the wrong canonical form
learns immediately instead of getting an empty load or silently unmatched prune
keys. (The more principled alternative — never feed canonical paths back into
`packages.Load`, keeping an explicit raw↔canonical mapping — was considered and
deferred as a larger change to the load path.)

## Q7c — Which smaller review findings are in scope?

**Answer: all four.**

- `canonicalizeSymbol` rewrites only the first occurrence, so a generic symbol's
  type-argument packages keep the host namespace and keys can miss silently.
- The `vendor/`-prefix relaxation in `ValidateAndResolve` phase 3 is restricted to
  standard-library packages instead of applying to every package.
- The facts stdlib representation is cleaned up: `PackageFact.IsStdlib` has no
  consumer, while `StdlibImports` encodes the same notion in a second shape with
  policy hidden in a slice's nil-ness.
- `ResolveDependencyInterface`'s local `factsPkgs` canonicalizes `ImportPath` and
  `Imports` like the main loader does.

## Q8a — Is the interface package itself a member?

**Answer: implicitly a member, excluded from the members list.**

`interface` stays its own declaration and the checker treats the interface
package as a member for attribution without it appearing in `members`. A glob
that matches it is silently fine, since a subtree pattern naturally covers it.

## Q8b — Does `absorbed_dependencies` keep transitive-closure semantics?

**Answer: yes, absorbed stays transitive.**

Absorbing a label means absorbing its whole tree — defensible for genuine
vendoring, and exactly what makes it cheap to draw a boundary around an existing
blob, which is the adoption path from Q4. Membership legibility is now carried by
`members`, so the implicitness costs much less than it did.

## Q9 — How are toolchain-injected runtimes handled?

**Findings established before asking.**

- **Declaring a package changes nothing about authority.** Attribution is
  Capslock's backwards BFS, so a member calling a proto runtime that uses
  `reflect` is charged `REFLECT` whether that runtime is undeclared, absorbed, or
  exempt. Only a **prune point** (a `component_dep`) stops authority flowing.
- **An undeclared capability is a violation regardless of class**
  (`checker.go:275-290`). `AnalysisDefeating` only selects the *warning* kind, so
  an undeclared `REFLECT` does fail an otherwise-pure component.

The requirement that follows: it must be possible to have "pure" components that
only compute and copy data around, and using protos must not make that
impossible. So a pruning mechanism is needed, not just an FR2 exemption.

Options weighed: emitter declares them as absorbed (rejected — does not prune);
a trusted-infrastructure list that prunes (rejected as stated — see below);
certifying the runtime as a real component; narrowing the capability
classification the way `fileHandleUseMethods` does.

**Answer: pre-declared *implicit infra components*, maintained in the Bazel
rules — a combination of certification and a trusted list.**

> **Superseded in form by Q17.** The reasoning below stands unchanged; only the
> encoding moved. The single `implicit` marker split into
> `interface_style = PACKAGE_SURFACE` on the component (what it is) and
> `auto_attached` on the dependency edge (how it got there), once it became clear
> that an author wrapping an ordinary library wants the first without the second.

The user's formulation: maintain pre-declared infra components somewhere in the
Bazel rules; they are themselves **checked**, so their authority is surfaced;
and the emitter adds them automatically as a `component_dep`, with an `implicit`
marker so no unused-dependency warning fires.

Why a bare trust list does not close the hole, and this does: a list asserts
"trust this" without enumerating **what** is trusted — the authority is never
stated, never verified, and silently changes as the runtime evolves (a runtime
that starts touching files keeps passing). A checked infra component states the
authority explicitly and re-verifies it every build; if the runtime's authority
grows, that component's own check fails and someone must consciously widen the
declaration. A list is a static assertion; a component is a maintained claim.

Two refinements agreed:

- **Attach only where actually injected** — the emitter adds the infra
  `component_dep` iff the component's closure contains one of its packages.
  Blanket attachment would put unreal dependencies in manifests, the same
  legibility problem Part 2 exists to fix.
- **`implicit` lives on the infra component's own manifest**, not on each
  dependency edge. The checker already resolves the dep's manifest for its
  interface symbols, so it reads the flag there — one declaration at the source
  instead of one per depender.

## Q10a — How does an infra component designate its interface?

Pruning happens at declared interface symbols, so an infra component needs
`interface_files` — but it wraps third-party code whose file layout nobody
controls.

**Answer: `implicit` components get relaxed well-formedness.**

The `implicit` marker also means: the interface is the whole exported surface of
the named packages, and FR4 placement rules (init and method placement) do not
apply. The check still runs and still surfaces the runtime's authority — the
whole point — without demanding that vendored code be organized to arcc's
conventions.

## Q10b — Where does the set of pre-declared infra components live?

**Answer: a host-replaceable seam, like `go_adapter.bzl`.**

Upstream `rules_arcc` ships the mechanism plus any runtimes it knows about; a
host swaps one file listing its own injected runtimes and their component labels.
Consistent with the seam pattern already established, and keeps a monorepo's
specifics out of upstream.

## Q11 — What does membership mean in native (non-Bazel) mode?

**Answer: `members` is optional; FR1 remains the default.**

An absent `members` field means "packages under the component root", so
hand-written manifests keep working untouched and the repo's own eight
self-manifests need no change. The field is how a component says something other
than the default — which is precisely the Bazel case. The one piece of
implicitness this keeps is the long-standing, documented FR1 rule, not a derived
closure.

## Q12 — Should the report's success wording be reworded here?

**Answer: yes.**

"Component X conforms / ambient-authority-free" is misleading — a component that
declares authority and stays within it prints the same line. Reword to
declared-authority terms ("conforms; does not exceed declared authority"). This
design already adds findings (skipped interface files, the absorbed-func-value
warning), so the report surface is in scope regardless.

## Q13 — What is the `members` authoring surface, given expansion cannot happen inside a symbolic macro?

Research finding (`research/members-glob-expansion.md`): `native.subpackages()`
is rejected inside a symbolic macro implementation — "can only be used while
evaluating a BUILD file or a legacy macro" — and it returns *packages*, not
targets.

**Answer: a helper called at the BUILD call site, with the package→target naming
convention as a hook in the adapter.**

```python
go_component(
    name = "component",
    interface = ":iface",
    members = arcc_subpackages(),   # expands here, before the macro runs
)
```

Keeps `go_component` a symbolic macro with typed attributes, matches the
"emitters write the expansion" decision from Q3a, and puts the naming convention
where the other host-specific knowledge already lives (`go_adapter.bzl`).

## Q14 — Is the bazelified self-check part of this work?

**Answer: yes, in scope for this batch.**

Introduce `go_component` targets for arcc's own components so `bazel test //...`
runs the self-component checks. This is what would have caught the `hostpolicy`
manifest breakage (fixed separately in change `oxvznunp`), which went unnoticed
precisely because the monorepo environment is Bazel-only and never ran
`just selfcheck`. It also dogfoods `members`: the `cli` component spans
`cmd/arcc` plus its `app` subpackage, so it is the repo's own multi-package case.


---

*Q15–Q17 came out of the user's review of the completed design and plan
(2026-07-26). They revise A2, B1/B2 and A6/A7 respectively.*

## Q15 — What is `INIT_OUTSIDE_INTERFACE` still for once A2 scopes it to the interface package?

**The user's observation.** Under A2 the rule only fires where an init is *not*
declared in an interface file of a package that has one — so what is left? The
suspicion: the rule was introduced under the assumption that only member code
reachable *from the interface* would be analyzed, which would leave member inits
unanalyzed unless reachable through an interface-exposed init. With every member
package now an analysis root that assumption is gone. And conceptually an init is
an implementation detail, not an interface-exposed contract.

**Answer: correct — remove the rule entirely.**

Verified before answering: under A2 the rule's entire remaining surface is *an
explicit `func init()` in a non-interface file of a package that has at least one
interface file*, e.g. the init in `svc/helpers.go` when only `svc/api.go` is
declared. Three reasons that is not worth keeping:

1. **The soundness half of review A2 is elsewhere and unaffected.** A2 had two
   parts. Part 1 — emit `func <B-pkg>.init CAPABILITY_SAFE` when pruning
   dependency B — is what prevents import-time authority from re-absorbing with no
   prune point. It is independent of file placement and stays. Part 2 (this rule)
   rested on "importing a package runs its init, so init behavior is de facto
   interface"; once members are roots, an init's authority is charged to the
   component wherever it is declared. Attribution carries it.
2. **The remaining scope is arbitrary.** If import-time behavior is de facto
   interface, it is equally so for a member package's init, since the interface
   imports it transitively. Interface-package-only catches the case where the
   interface package happens to be multi-file and misses the general case.
3. **An init is an implementation detail.** No signature, not callable, not
   referenceable; what it changes is behavior, which arcc does not check. The
   style concern is `gochecknoinits` territory.

Removed: `report.InitOutsideInterface`, the `sym.Kind == "init"` branch at
`checker.go:166-176`, `TestCheck_FR4_ExplicitInitOutsideInterface`, the assertion
at `checker_test.go:910`, and the FR4 prose in the 2026-07-06 design. A2 becomes
"delete" rather than "rescope".

Side effect worth noting: with the init rule gone, `METHOD_OUTSIDE_INTERFACE` is
already vacuous when `interface_files` is empty (it is conditional on the
receiver's type being declared in an interface file), so the "relaxed
well-formedness" that Q10a wanted for interface-less components costs nothing in
the checker — only a manifest-validation relaxation.

## Q16 — Should `arcc_subpackages()` ship?

**Correction that forced the question.** `research/members-glob-expansion.md`
finding 2 originally read `native.subpackages()` as returning transitive
subpackages. Re-verified with a fixture where a package is nested *under* a
package (`comp/a/deep`), it returns only the **frontier** of nearest descendant
packages and cannot address anything past a package boundary —
`include = ["a/deep"]` returns `[]` for an existing `//comp/a/deep`. The original
fixture could not tell the difference because `comp/b` was a plain directory.
bazel-skylib's `subpackages.all()` (which
[bazel.build recommends](https://bazel.build/rules/lib/toplevel/native#subpackages)
over the native call) is a thin wrapper with the same limit, though its
`fully_qualified = True` output makes the B2 naming hook a no-op under the Gazelle
basename convention.

**The user's actual goal, which the helper was standing in for.** Components are
meant to enable a style of work where implementation changes need little or no
human review *because* interface changes are the ones that surface. For that, the
BUILD file declaring a component should not have to change when the
implementation does — internal packages added, removed, or restructured. A
top-level "all packages underneath" declaration would give that.

**Answer: table it — ship no helper at all.**

`members` takes concrete labels. An author who wants the frontier calls skylib
directly at BUILD top level; otherwise labels are listed, and whoever changes the
implementation adjusts the BUILD files under the component. A helper named for
subtrees that delivers a frontier is worse than no helper: it reads as "everything
below here" and is wrong the moment a nested BUILD file appears.

Consequences: **B1** becomes "concrete labels only"; **B2** is deferred, since
nothing in rules_arcc maps a package path to a target any more; no
`bazel_dep` on bazel-skylib is needed. The unmet goal is recorded as Appendix C.4
rather than dropped, along with the mitigating fact that a *missed* member which
is actually imported is fail-closed (`UNDECLARED_DEPENDENCY` names it) and only
dead nested code stays silently unowned. The aggregate-target-per-package pattern
in Appendix B is the likeliest eventual answer, since it keeps the component's own
BUILD file stable — at the cost of a BUILD edit in every subpackage.

## Q17 — Does an author-declared "implicit interface" component collapse with the implicit infra component?

**The user's proposal.** A common adoption scenario is wrapping a commonly used
library — a logger, say — that was never structured to have an architecturally
exposed interface. Such a component would prune at package granularity like an
infra component, its interface being implicitly every exported symbol of its
members, but it *should* still warn when declared and unused, since an author
wrote it.

**Answer: the component kind collapses to one; the difference is real but belongs
on a different axis.**

Comparing the two cases across every property in play, they agree on: interface =
full exported surface of members; package-granularity pruning; `interface_files`
may be empty; FR4 placement rules not applying; own check running so authority is
certified rather than trusted. They differ on exactly two, which are the same
thing twice: the edge is created by the emitter rather than the author, and it is
exempt from `UNUSED_DEPENDENCY`.

So: one component kind, one edge property.

```protobuf
enum InterfaceStyle {
  INTERFACE_STYLE_UNSPECIFIED = 0;      // interface_files enumerate the surface
  INTERFACE_STYLE_PACKAGE_SURFACE = 1;  // every exported symbol of every member
}
message Component { ... InterfaceStyle interface_style = 7; }
message ComponentDependency { ... bool auto_attached = N; }
```

`bool implicit` on `Component` disappears. Two fields rather than one three-value
enum, for two reasons: `auto_attached` sits on the **depender's own** edge, so the
checker needs no cross-manifest lookup to decide whether to warn (simpler than the
original design, which resolved the dep's manifest to read `implicit`); and it
permits the combination a three-value enum forbids — an injected component that
does declare real interface files.

### Q17a — Is `interface` still needed under `PACKAGE_SURFACE`?

**The user's question.** The logger example still carried an `interface`
declaration; if the style is package-surface, is that necessary, or is
`interface_style` plus `members` sufficient?

**Answer: not necessary — and it should be rejected outright.**

`interface` does four jobs today: supplies `interface_files`, is the aspect root,
defines the FR1 component root, and forwards Go providers so the component target
works as a `deps` entry. Under `PACKAGE_SURFACE` the first three are subsumed by
`members` (already aspect roots; FR1 moot because membership is explicit). Only
provider forwarding survives, and it is close to worthless in exactly this
scenario: callers of a library you do not own already depend on it directly, and
you cannot edit their BUILD files to point at your component target. An author who
wants it writes an `alias`. Allowing `interface` as optional-but-permitted would
leave two sources of truth for the surface.

So the shape rule is: **declared style** — `interface` mandatory, `members`
optional; **`PACKAGE_SURFACE`** — `members` mandatory, `interface` rejected. M5
("the interface package is implicitly a member") is vacuous under
`PACKAGE_SURFACE`, and layout `roots` equals `members` exactly.

### Q17b — Consequences accepted with the decision

1. `ResolveDependencyInterface` needs a `PACKAGE_SURFACE` branch returning every
   exported symbol of every member, so `UNUSED_DEPENDENCY` stays meaningful
   (`logger.Info(...)` resolves to a used symbol) while
   `CALLS_UNDECLARED_INTERFACE` becomes vacuous.
2. The authority hiding is deliberate and is the point: absorbing
   `//common/logger/impl:network_logger` charges NETWORK to every component that
   logs, whereas making it a component stops that. The load-bearing safeguard is
   the wrapper's own check — the certified-not-trusted argument from Q9, now
   applying to author-written components too (Appendix C.6).
3. Appendix C's package-pruning limitation widens: it was scoped to injected
   runtimes and now applies wherever an author writes `PACKAGE_SURFACE`, which
   will be common (Appendix C.5).
