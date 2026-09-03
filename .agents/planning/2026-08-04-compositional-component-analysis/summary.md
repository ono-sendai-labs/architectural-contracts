# Summary — compositional component analysis

**Date:** 2026-08-04 · **Revised:** 2026-09-02
**Status:** design and plan revised after independent review; implementation not started
**Baseline:** `dev-exp-go-bazel-mvp` @ `5011b726` (code unchanged through `cca66212`)

## Artifacts

```
.agents/planning/2026-08-04-compositional-component-analysis/
├── rough-idea.md                          the spoken notes that started it, plus what they did not settle
├── idea-honing.md                         Q1–Q17, every decision with rationale and rejected alternatives
├── research/
│   ├── current-analysis-pipeline.md       what a check does today, measured costs, findings that shaped the design
│   ├── host-import-friction.md            triage of the second-monorepo friction report
│   ├── spike-export-data-loading.md       member-only syntax + export-data types via go/packages (works; full-closure caveat)
│   └── spike-stdlib-map-generation.md     per-symbol Capslock attribution, inventory, vars, init, cost
├── design/detailed-design.md              the design (revised 2026-09-02)
├── implementation/plan.md                 13 steps with a progress checklist (revised 2026-09-02)
├── 2026-09-02-design-review.md            independent review (gpt-5.6-sol), 19 findings
├── 2026-09-02-design-review-response.md   per-finding assessment, decisions, and what changed
└── summary.md                             this file
```

Inputs: the user's spoken notes (2026-08-04), *Upstream recommendations from the second
monorepo import* (2026-08-03), and the 2026-09-02 design review.

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
   `types.Info`, no SSA — with a declaring-object rule and one versioned symbol grammar;
2. a **precomputed standard-library authority map**, total at package *and* symbol
   granularity, generated once per target SDK configuration; its enumeration is the
   definition of "standard library", so the five existing stdlib predicates delete;
3. a **surface manifest** per component, produced by an ordinary build action (checked)
   or written at analysis time (asserted), consumed by dependents in place of source,
   with provenance, freshness and authority as three separate axes.

Two consequences follow. `absorbed_dependencies` and pattern membership are removed,
their role taken by ordinary components under the invariant *every component is either
checked, or explicitly marked `UNKNOWN` and visibly untrusted* (approval is governance,
not tool-enforced). And an `authority: UNKNOWN` axis is added so unowned code can be
adopted without being falsely certified.

The governing principle: **the component boundary is where ambient authority becomes
designated capability.** The corollary that organises the design: **boundaries are
syntactic; authority is semantic.**

What the review sharpened: the *work* over the closure disappears, but the type checker
still reads compiled export data for the whole closure — a measured, cached input rather
than zero dependence. That is now stated as N1/N2 and tested by a scaling benchmark.

## Plan shape

13 steps; sequencing still optimises for internal simplicity (Q17), with three ordering
constraints:

- **Steps 1–2** — baseline measurement, three fixes, then `absorbed_dependencies` and
  pattern membership removed *first*
- **Step 3** — persisted schemas, symbol grammar, authority lattice (before anything
  persists or compares symbols)
- **Step 4** — stdlib map: generator, port, **Bazel artifact and native cache together**
  (before the check depends on it)
- **Step 5** — build topology: analysis action, providers, surface emission, CLI outputs
- **Step 6** — the reference scan; **core end-to-end functionality lands here**
- **Step 7** — surface consumption, status axes, overlap, namespace
- **Step 8** — export-data type loading; **member-only inputs (N1/N2) land here**
- **Steps 9–13** — golden restructure, `UnusedAuthority`, `authority: UNKNOWN`, host
  adapter hooks, cross-cutting acceptance

Release constraint: between Steps 2 and 11 no revision satisfies I1; none is released.

## Areas that may need further refinement

- **The unmeasured split.** How much of the PoC's ~50s is closure SSA (eliminated) versus
  member-package type-checking (kept). Step 1 answers it.
- **rules_go export-data plumbing.** `GoArchive.data.export_file` and `GoStdLib` access
  from the aspect are assumed from rules_go 0.61.1 docs, not yet exercised (Step 8).
- **Var rule at scale.** The handle minting-authority rule was validated on `os` and
  `net/http` only; Step 4's totality test will surface surprises across all 178
  importable packages.
- **Six decisions flagged for user confirmation** in the review response §4 (pattern
  membership removal, always-green analysis action, removal of `own_check_runs` and
  `certification_reference`, I1 governance split, handle-var minting rule,
  JSON-over-protobuf encoding).

## Deferred by decision

Transitive/whole-tree authority predicates and the `UNKNOWN` approval predicate (Q10, Q7,
DR-13); a bootstrap CLI for wrapper components (Q7); tree-level dep-vs-dep declaration
divergence (Q11 — direct overlap is now in scope). Friction-report items triaged as
orthogonal are listed in [`research/host-import-friction.md`](research/host-import-friction.md).

## Next steps

1. Confirm or overturn the six flagged decisions in
   [`2026-09-02-design-review-response.md`](2026-09-02-design-review-response.md) §4.
2. Run `plan-to-tasks` to generate code task files for Step 1.
