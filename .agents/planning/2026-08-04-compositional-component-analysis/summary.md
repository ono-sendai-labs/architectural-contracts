# Summary — compositional component analysis

**Date:** 2026-08-04
**Status:** design and plan complete; implementation not started
**Baseline:** `dev-exp-go-bazel-mvp` @ `5011b726`

## Artifacts

```
.agents/planning/2026-08-04-compositional-component-analysis/
├── rough-idea.md                          the spoken notes that started it, plus what they did not settle
├── idea-honing.md                         Q1–Q17, every decision with rationale and rejected alternatives
├── research/
│   ├── current-analysis-pipeline.md       what a check does today, measured costs, findings that shaped the design
│   └── host-import-friction.md            triage of the second-monorepo friction report
├── design/detailed-design.md              the design
├── implementation/plan.md                 11 steps with a progress checklist
└── summary.md                             this file
```

Inputs: the user's spoken notes (2026-08-04) and *Upstream recommendations from the
second monorepo import* (2026-08-03).

## The design in brief

`arcc check` answers every question by building a whole-program call graph — loading the
component's transitive closure, building SSA, running VTA, then doing it again inside
Capslock, while separately type-checking each dependency from source. Cost scales with
the closure: 2.1s for four trivial packages, ~50s for some packages in the monorepo PoC.

Two already-landed changes make that unnecessary: membership is explicit with every
member function as a root (so intra-component transitivity is already collapsed), and
findings are already pruned at declared dependency boundaries (so authority behind a
boundary is already not attributed to the caller). What remains is classifying the edges
that *leave* a component, which needs no call graph.

The replacement is three pieces:

1. a **reference scan** over member sources — references and imports as edges, AST plus
   `types.Info`, no SSA;
2. a **precomputed standard-library authority map**, generated once per SDK, whose package
   enumeration doubles as the definition of "standard library" — which is why the five
   existing stdlib predicates delete rather than consolidate;
3. a **surface manifest** emitted by each component's check action and read by its
   dependents in place of their source.

Two consequences follow. `absorbed_dependencies` is removed, its role taken by ordinary
components under the invariant *every component is either checked, or explicitly marked
unanalyzed and approved*. And an `authority: UNKNOWN` axis is added so unowned code can be
adopted without being falsely certified.

The governing principle, which belongs in user-facing docs: **the component boundary is
where ambient authority becomes designated capability.** A parser depending on a logging
component does not acquire filesystem authority; it acquires the ability to log. The
corollary that organises the design: **boundaries are syntactic; authority is semantic.**

Speed is the symptom. The reason to do it is that checks become independent, cacheable,
and composable — a check's inputs become its own sources plus a few small files.

## Plan shape

11 steps. Sequencing optimises for internal simplicity, since the PoC will not re-import
until this lands (Q17):

- **Steps 1–2** — baseline measurement, three orthogonal fixes, then `absorbed_dependencies`
  removed *first* (pure deletion that shrinks every later step)
- **Step 3** — stdlib map generator and lookup port, on-demand cached
- **Step 4** — the reference scan; **core end-to-end functionality lands here** (a check
  runs with no call graph)
- **Steps 5–6** — surface manifest emission then consumption; **full composability lands at
  Step 6** (cost stops tracking the closure)
- **Steps 7–8** — golden restructure, then map distribution and fail-closed keying
- **Steps 9–11** — `UnusedAuthority`, `authority: UNKNOWN`, host adapter hooks

The non-obvious ordering constraint driving Steps 4–6: the implements-closure workaround
compensates for VTA, so swapping to manifests before removing VTA would break FR5. The
scan lands first, the workaround dies with it, and only then does the symbol source swap.

## Areas that may need further refinement

- **The unmeasured split.** How much of the PoC's ~50s is closure SSA (eliminated) versus
  member-package type-checking (kept). Step 1 answers it, and the answer could reorder
  later steps.
- **Native-mode out-of-tree membership.** `ResolveDependencyInterface` derives the component
  root from the manifest's directory and rejects root overlap, so wrapper components for
  unowned code may not work outside Bazel. The pattern-membership path may already handle
  it — to be verified, not assumed (Q5).
- **Stdlib interface methods.** What `io.Writer.Write` and friends contribute to the map.
  Most likely nothing, given the minting-site rule, but it is a decision not yet made.
- **Manifest namespace.** Recorded as a format requirement; the concrete encoding and the
  rejection path for foreign namespaces are not yet specified in detail.
- **No codebase summary exists.** `.agents/summary/` is absent. Running the
  `codebase-summary` workflow would give later phases (and reviewers) a shared map of the
  Go module's structure; the design was written from direct code reading instead.

## Deferred by decision

Transitive/whole-tree authority predicates (Q10, with a barrier-marking design hint
recorded); a bootstrap CLI for wrapper components (Q7); dep-vs-dep declaration divergence
checking (Q11). Friction-report items triaged as orthogonal are listed in
[`research/host-import-friction.md`](research/host-import-friction.md).

## Next steps

1. Review [`design/detailed-design.md`](design/detailed-design.md) and
   [`implementation/plan.md`](implementation/plan.md).
2. Run `plan-to-tasks` to generate code task files for Step 1.
3. Optionally run `codebase-summary` first, so later phases have an up-to-date structural
   map to work against.
