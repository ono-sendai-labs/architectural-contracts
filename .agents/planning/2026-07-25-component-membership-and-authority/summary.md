# Summary — component membership and authority attribution

**Date:** 2026-07-25
**Status:** design and plan complete, revised after review (Q15–Q19); implementation not started
**Branch:** `dev/exp-go-bazel-mvp`

## Artifacts

```
.agents/planning/2026-07-25-component-membership-and-authority/
├── rough-idea.md          the three problem groups and where they came from
├── idea-honing.md         Q1–Q19, every decision with its rationale
├── research/
│   ├── members-glob-expansion.md          Bazel Starlark: what can expand a glob, and where
│   ├── capability-analysis-mechanics.md   how Capslock actually attributes; SSA detection shape
│   ├── build-platform-and-tags.md         where a host gets GOOS/GOARCH/tags/cgo
│   └── host-portability-findings.md      the design run against a second host's Go rules
├── design/detailed-design.md              the design
├── implementation/plan.md                 11 steps with a progress checklist
└── summary.md                             this file
```

Input: `.agents/planning/2026-07-24-callback-authority-attribution/callback-authority-attribution.md`,
plus the port records in `2026-07-23-host-portability-seams/` and
`2026-07-24-layout-completeness/`.

## The design in one page

**Membership becomes declared, not derived.** A `members` field in the manifest
lists the packages a component is responsible for. Members may live anywhere,
patterns are an authoring convenience that emitters expand literally, and the
layout's `roots` must equal them. Absent `members`, FR1 (packages under the
component root) still applies, so hand-written manifests are untouched.

**Attribution follows ownership.** Members become analysis roots, so a
component's own code is charged regardless of who calls it — closing the
callback-escape fail-open for owned code. `absorbed_dependencies` survives as the
cheap way to draw a boundary around existing code, keeping use-attribution; the
residual gap becomes a warning when member code takes an absorbed function as a
*value*.

**The analysis is made trustworthy enough to rest on.** The build platform
becomes declared data instead of an ambient property of the arcc binary;
standard-library classification requires two signals to agree; the
canonicalization contract hosts depend on is asserted rather than assumed.

**Two supporting pieces.** *Package-surface components* let code that was never
given an architectural interface be pruned while still being checked, so its
authority is certified rather than trusted — one kind covering both
toolchain-injected runtimes and libraries an author wraps to draw a boundary;
a *bazelified self-check* means a Bazel-only environment cannot silently skip
arcc's own dogfooding.

## Findings that shaped it

- **The "remainder is an error" rule already exists.** `UNDECLARED_DEPENDENCY`
  covers it; it only stays quiet because the Bazel rule emits blanket absorbed
  entries. Part 2 of the source note is largely a rules-layer change.
- **Neither well-formedness rule survives rescoping.**
  `METHOD_OUTSIDE_INTERFACE` is already conditional on the receiver type living in
  an interface file, so it needs no change; the init rule breaks when members
  become roots, and on review (Q15) turned out to have no remaining job at all —
  its soundness argument is carried by attribution, so it is removed rather than
  narrowed.
- **Capslock attributes by backwards BFS** from capability nodes, so the
  analyzed-*package* set is the entire criterion — there is no "reachable from the
  interface". This is why members-as-roots closes the gap for owned code, and why
  the note's Option B is not expressible through the current port.
- **Capslock supports package-level prune keys**, which is what makes
  package-surface components cheap and, in fact, more complete than symbol
  pruning.
- **`native.subpackages()` cannot express a subtree at all.** It is rejected
  inside a symbolic macro, and — corrected on re-verification (Q16) — returns only
  the *frontier* of nearest descendant packages, unable to address anything past a
  package boundary. bazel-skylib's wrapper has the same limit. So `members` takes
  concrete labels and no wildcard helper ships.
- **The infra and library-wrapper cases are one component kind** (Q17), differing
  only in who created the dependency edge — which is a property of the edge
  (`auto_attached`), not of the component (`interface_style`).
- **T4 as first written would have broken layout mode outright** (Q18a). Nothing
  carries a `*packages.Module` there, so flipping the nil-`Module` branch and
  applying the AND uniformly makes `os` and `fmt` non-stdlib. Layouts now carry a
  per-package stdlib bit that must be **provenance-derived** — an emitter that
  recomputes a path heuristic produces a copy of the signal it is meant to check.
- **An import that resolves to nothing is its own failure** (Q19). T8's equality
  runs over *resolvable* imports; an unresolvable post-filter import is reported as
  `ANALYSIS_LIMITATION` rather than failing the load, because folding it in would
  turn a consistency requirement into a completeness one — but it is not silent,
  since an edge FR2 cannot classify is the fail-open this batch exists to remove.
- **The platform block binds emitters, not just the loader** (Q18b). A layout can
  carry several platforms' files for one package at once, in which case its import
  list is the union across platforms and contradicts its own platform block. The
  loader now validates this bidirectionally.
- **Some packages cannot be named as targets at all** (Q18c) — visibility-gated
  toolchain runtimes — so `PACKAGE_SURFACE` membership accepts unexpanded patterns,
  at the cost of the component's own check. That loss is surfaced as a report
  annotation rather than a warning, because a warning there would fire on every
  component in a repository and be suppressed wholesale.
- **`GoInfo.mode` carries the target platform**; `GoSDK.goos` is the exec
  platform and would have reproduced the bug being fixed.

## Plan shape

Eleven steps. Owned-code attribution is closed and demoable in native mode at
**Step 2**; the full Bazel path is end-to-end at **Step 7**. Ordering is chosen so
every step leaves the tree green: schema first (behavior unchanged when `members`
is absent), then trustworthiness fixes, then the behavior change, then the rules,
then infra components and the residual warning, with the bazelified self-check
last so it exercises the finished model.

## Open for review

Choices made without an explicit decision from the user:

- Report kind names: `MEMBER_OVERLAP`, `ABSORBED_FUNC_VALUE_ESCAPE`,
  `INTERFACE_FILE_EXCLUDED`.
- `capanalyzer.AnalyzeRequest` gains `PruneAtPackages` rather than overloading
  `PruneAt`.
- `facts.PackageFact.IsStdlib` removed outright rather than deprecated.
- The layout's `platform` block is **optional** (absent ⇒ `build.Default`), which
  keeps native mode and hand-written layouts working but leaves the old ambient
  behavior reachable.
- Enum value naming: `INTERFACE_STYLE_PACKAGE_SURFACE` rather than the
  `IMPLICIT_INTERFACE` the review sketched, because "implicit" is already carrying
  M5 and auto-attachment.
- A8 as a dependency-listing annotation rather than an `UNCERTIFIED_PRUNE` warning,
  and A9 reusing `ANALYSIS_LIMITATION` rather than the `UNANALYZED` authority
  constant. Both decline a new report kind the port review proposed.

Reviewed and settled (Q15–Q17): removing `INIT_OUTSIDE_INTERFACE` outright;
shipping no `members` wildcard helper; splitting `implicit` into
`interface_style` + `auto_attached`; rejecting `interface` under
`PACKAGE_SURFACE`.

Eight limitations are stated deliberately in design Appendix C: one platform per
check; cross-component membership overlap undetected; absorbed code still
use-attributed; **a component's BUILD file changing when its internal package
structure changes** (C.4, tabled with the wildcard helper — the one that works
against the review-light-implementation-changes goal); package-granularity pruning
hiding unexported entry points, now with a wider blast radius; an **asserted
boundary not being a verified one** (a pattern-membership component has no check of
its own); **bodiless packages** being an unanalyzable authority category; and
`PACKAGE_SURFACE` components being able to launder authority.

## Next steps

1. Review `design/detailed-design.md` and `implementation/plan.md` together.
2. Resolve the remaining open items above (or accept them).
3. Run `plan-to-tasks` to generate code task files for Step 1.

## Related work not in this batch

- **Repo-wide membership uniqueness check** (Q3b) — a single Bazel test over all
  `go_component`s, closing the cross-component overlap limitation.
- **Multi-platform verification** (Q6a) — so authority in a `_windows.go` file is
  not invisible on a Linux check.
- **Never feeding canonical paths back into `packages.Load`** (Q7b) — the
  principled removal of the invariant that Step 5 merely asserts.
- **Repo-wide certification check** (Q18d) — "every `PACKAGE_SURFACE` dependency is
  certified or carries a reference", the one place an uncertified boundary should
  fail once rather than warn everywhere. Naturally lands with the
  membership-uniqueness check.
- **A stable way to say "all packages under here"** (Q16, Appendix C.4) — the
  aggregate-target-per-package pattern is the likeliest answer; it keeps the
  component's own BUILD file untouched by internal restructuring, at the cost of a
  small target in every subpackage.
