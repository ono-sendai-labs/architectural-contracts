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

## Final next-import checklist

Walked against the Step 13 tree on 2026-09-12. The two items identified above
as mandatory before the next host import are present; the remaining entries
are either implemented by this redesign, deliberately dropped, or explicitly
deferred.

| source item | final disposition and concrete evidence |
| --- | --- |
| §1 non-Go aspect provider | Landed. The aspect returns an empty ArccPackageInfo for non-Go targets at bazel_rules/go/private/aspect.bzl:27-33; bazel_rules/go/tests/aspect_tests.bzl:302-315 asserts the empty provider. |
| §2 stdlib through hostpolicy | Dropped as designed. Standard-library membership/classification is owned by the keyed stdlib map and its canonical readers; hostpolicy now supplies namespace/canonical-path hooks, while no IsStdlibPath classifier remains. |
| §3 runtime-injection attributes | Landed with empty upstream defaults at bazel_rules/go/private/go_adapter.bzl:124-143. The consumer hook at :145-159 is merged into the component rule at bazel_rules/go/private/component.bzl:830-841; runtime injection tests are wired by bazel_rules/go/tests/runtime_injection_tests.bzl:129-170. |
| §3 injected-runtime classification | Resolved in this design. Existing infra attachment is preserved and receives the merged package view; the injected-runtime analysis/report tests cover the asserted UNKNOWN boundary and checked shared dependency in bazel_rules/go/tests/BUILD.bazel:242-272. |
| §4 SDK source/oracle attrs | Landed as the source-only adapter contract at bazel_rules/go/private/go_adapter.bzl:371-426 and consumed by bazel_rules/go/private/stdlib_map.bzl:77-169. The map action input tests in bazel_rules/go/tests/stdlib_map_tests.bzl:50-170 keep the contract explicit. |
| §4 missing SDK root | Landed fail-closed. bazel_rules/go/private/go_adapter.bzl:366-369 rejects an absent adapter root, and go/internal/packagelayout/packagelayout.go:503-522 rejects a missing/non-directory SDK root; the missing-root fixture is go/internal/packagelayout/packagelayout_test.go:508-525. |
| §5 infra-registry self-exemption | Landed by target identity rather than a package-name literal at bazel_rules/go/private/go_adapter.bzl:236-261; exact-self coverage is wired in bazel_rules/go/tests/BUILD.bazel:131-138 and bazel_rules/go/tests/aspect_tests.bzl. |
| §5 check tagging | Resolved by the explicit authority axis. bazel_rules/go/defs.bzl:277-291 documents scheduling-only check_tags, while authority UNKNOWN selects the package-level asserted producer; the UNKNOWN shape and no-checked-report tests remain in bazel_rules/go/tests/asserted_surface_tests.bzl and bazel_rules/go/tests/BUILD.bazel:470-481. |
| §6(1) verdict/layout golden split | Landed. Typed report/surface shape rules live in bazel_rules/go/private/check.bzl:435-560, and layout shapes are a separate rule family at :564-617; the corresponding goldens are under bazel_rules/go/tests/goldens. |
| §6(2) canonical-path normalization | Landed as the host canonicalization contract. go/internal/hostpolicy/hostpolicy.go:23-98 and go/internal/hostpolicy/namespace_test.go:44-76 pin idempotence and the IsCanonicalPath predicate; surface consumers reject non-canonical paths in go/internal/goanalysis/surface_resolver.go:500-515. |
| §6(3) host-supplied testdata BUILD convention | Deferred/orthogonal. No upstream host-specific BUILD injection is claimed by this tree; the generic repository testdata BUILD files remain the in-repo fixture convention. |
| §7 IsCanonicalPath seam | Landed at go/internal/hostpolicy/hostpolicy.go:56-74 with default and override tests in go/internal/hostpolicy/namespace_test.go:44-176. |
| §7 canonical namespace | Resolved in this design. hostpolicy.NamespaceID and the surface namespace are validated end-to-end; the deterministic fixture compares surfaces under the upstream namespace, and go/internal/goanalysis/surface_resolver.go rejects a mismatched namespace before consumption. |

The next import can therefore consume the runtime-injection and SDK-source
adapter seams without carrying the old host patches. The only explicitly
deferred checklist item is the host-supplied testdata BUILD convention; it is
not silently counted as implemented. Separately, the design's broader
multi-architecture CI matrix and UNKNOWN approval/tree predicate remain
deferred and are documented in the final acceptance record.
