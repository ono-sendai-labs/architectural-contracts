# Research — triage of the second-monorepo import friction report

**Date:** 2026-08-04
**Source:** *Upstream recommendations from the second monorepo import*, written
2026-08-03 against `dev-exp-go-bazel-mvp` @ `5011b726`.
**Purpose:** decide which of its recommendations belong to this design and which are
orthogonal work to be sequenced separately.

## Context that changes the calculus

The PoC import has landed and works. The local patch set is larger than ideal (6 Go
autopatches, 50 in `rules_arcc` of which 45 are test goldens and testdata), but **no
updated import will be taken until the compositional work lands**. Patch-relief work
with no re-import deadline before the affected code is deleted is therefore churn.

## Triage

| Item | Disposition | Reason |
| --- | --- | --- |
| §1 aspect returns no provider | **Ship, orthogonal** | Real one-line bug; unaffected by the redesign |
| §2 stdlib via `hostpolicy` | **Drop** | The five predicates delete outright (idea-honing Q12) |
| §3 injected runtime — hook | **Ship, orthogonal** | Additive adapter contract, `[]`/`{}` defaults |
| §3 injected runtime — classification | **In this design** | Resolved as Q14: infra component by default |
| §4 stdlib sources — attrs hook | **Ship, orthogonal** | Additive, inert upstream |
| §4 stdlib sources — contract statement | **In this design** | The requirement changes shape; see below |
| §4 fail loudly on missing SDK root | **Ship, orthogonal** | Robustness fix, independent of model |
| §5 infra-registry self-exemption | **Ship, orthogonal** | General property of the infra mechanism |
| §5 check tagging | **In this design** | This is `authority: UNKNOWN` hand-rolled (Q8) |
| §6(1) split verdict from layout goldens | **In this design** | Goldens churn during the redesign anyway |
| §6(2) normalize paths before diffing | **Ship, orthogonal** | Reduces our own churn during the work |
| §6(3) host-supplied testdata BUILD | **Ship, orthogonal** | Independent convention change |
| §7 `IsCanonicalPath` seam | **Ship, orthogonal** | Small predicate matching the existing pattern |
| §7 canonical namespace | **In this design** | Becomes a manifest *format* question; see below |

## Why §4's requirement changes shape

Today every check action type-checks the standard library, so the report's invariant —
"the resolved SDK root must be reachable at the path the layout names" — has to hold per
check. Under the compositional model that splits:

- **Check actions** need stdlib *type information* for member imports (signatures from
  export data, i.e. the compiled stdlib). No sources.
- **Map generation** needs stdlib *sources*, once per SDK, in a separate action that
  most likely lives in the SDK repo.

The per-check input is smaller and better-shaped, and the invariant becomes much less
load-bearing. Building `GO_SDK_SRCS_ATTRS` into all three check rules now would be
implementing a hook for a requirement about to move. The attribute hook itself is
harmless and can ship; the contract statement should wait.

The report's related recommendation — that `discoverStdlibWithContext` fail loudly when
`os.Stat(sdkRoot)` fails, rather than inviting a `filepath.Walk("/")` fallback ladder —
is orthogonal robustness and should ship regardless.

## Why §5's check tagging is `authority: UNKNOWN`

The host patches `defs.bzl` to string-match on package name and force
`PACKAGE_SURFACE` components' generated `.check` targets to be tagged `manual`.
`defs.bzl:124` turns that into `own_check_runs = false`, which renders the boundary
`asserted` in the report.

That is exactly "this component is not analyzed; trust its surface anyway" — reached by
hack because no spelling existed. It is independent corroboration that the axis is
needed, and it supports the decision to model verification status as a separate axis
rather than a third `interface_style`: the host wanted to express something about
*verification*, and the only lever available was tagging, which is why it ended up as a
package-name literal — the patch most likely to conflict on re-import.

## Why §7 becomes a format question

`IsCanonicalPath` as a third override-once predicate is a fine seam and should ship. But
under this design, canonicalization stops being internal: surface manifests are
persisted artifacts exchanged between components, and their symbol names embed import
paths. The design must therefore state which namespace manifests are written in —
host-canonical or upstream-canonical — and guarantee the canonical form is idempotent. A
host that rewrites prefixes, plus a cached manifest produced under a different
namespace, is a silent mismatch.

## Why §6 waits (mostly)

45 of the 50 `rules_arcc` patches are goldens and testdata. Recommendation (1) — split
verdict assertions from layout-shape assertions — is the right idea, but the redesign
*shrinks or removes the layout-shaped half*: no SSA over the closure means layouts
collapse toward member packages plus resolved dependencies, and surface manifests become
a new golden-able artifact that does not exist yet. Doing the split first means splitting
goldens that are about to change content, then redoing them. The redesign forces golden
churn on its own (at minimum the FR5 behaviour change flips a case from pass to fail).

Recommendation (2) inverts: normalizing both sides through `CanonicalizePath` before
diffing was proposed as host patch relief, but it now reduces *our own* churn while we
restructure. Ship it early for that reason.

## What must land before the next import

Only two things beyond the redesign itself:

- §3 runtime-injection hook
- §4 SDK-attrs hook

Both additive with empty defaults. Everything else is either deleted by the redesign
(§2's predicates, §5's tagging patch, a slice of §6) or independent of it.
