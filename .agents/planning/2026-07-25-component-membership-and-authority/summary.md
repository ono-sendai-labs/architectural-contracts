# Summary — component membership and authority attribution

**Date:** 2026-07-25
**Status:** design and plan complete; implementation not started
**Branch:** `dev/exp-go-bazel-mvp`

## Artifacts

```
.agents/planning/2026-07-25-component-membership-and-authority/
├── rough-idea.md          the three problem groups and where they came from
├── idea-honing.md         Q1–Q14, every decision with its rationale
├── research/
│   ├── members-glob-expansion.md          Bazel Starlark: what can expand a glob, and where
│   ├── capability-analysis-mechanics.md   how Capslock actually attributes; SSA detection shape
│   └── build-platform-and-tags.md         where a host gets GOOS/GOARCH/tags/cgo
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

**Two supporting pieces.** *Implicit infra components* let toolchain-injected
runtimes be pruned while still being checked, so their authority is certified
rather than trusted; a *bazelified self-check* means a Bazel-only environment
cannot silently skip arcc's own dogfooding.

## Findings that shaped it

- **The "remainder is an error" rule already exists.** `UNDECLARED_DEPENDENCY`
  covers it; it only stays quiet because the Bazel rule emits blanket absorbed
  entries. Part 2 of the source note is largely a rules-layer change.
- **Only one well-formedness rule needed rescoping.** `METHOD_OUTSIDE_INTERFACE`
  is already conditional on the receiver type living in an interface file; only
  the init rule breaks when members become roots.
- **Capslock attributes by backwards BFS** from capability nodes, so the
  analyzed-*package* set is the entire criterion — there is no "reachable from the
  interface". This is why members-as-roots closes the gap for owned code, and why
  the note's Option B is not expressible through the current port.
- **Capslock supports package-level prune keys**, which is what makes implicit
  infra components cheap and, in fact, more complete than symbol pruning.
- **`native.subpackages()` is rejected inside a symbolic macro**, so the
  `members` glob expands at the BUILD call site, not inside `go_component`.
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

Four limitations are stated deliberately in design Appendix C: one platform per
check; cross-component membership overlap undetected; absorbed code still
use-attributed; package-granularity pruning hiding a runtime's unexported entry
points.

## Next steps

1. Review `design/detailed-design.md` and `implementation/plan.md` together.
2. Resolve the four open items above (or accept them).
3. Run `plan-to-tasks` to generate code task files for Step 1.

## Related work not in this batch

- **Repo-wide membership uniqueness check** (Q3b) — a single Bazel test over all
  `go_component`s, closing the cross-component overlap limitation.
- **Multi-platform verification** (Q6a) — so authority in a `_windows.go` file is
  not invisible on a Linux check.
- **Never feeding canonical paths back into `packages.Load`** (Q7b) — the
  principled removal of the invariant that Step 5 merely asserts.
