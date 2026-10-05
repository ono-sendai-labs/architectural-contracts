# Component structure conformance — summary

**Date:** 2026-10-05 · **Revised:** 2026-10-05 (R1) · **Status:** design + plan complete,
no implementation started

## What this is

arcc checks itself: every Go package under `go/internal` and `go/cmd` is a component.
Several of those self-hosting declarations do not follow the structure we want adopting
projects to follow. This planning set restructures them so that:

1. each component's **interface is small** — an explicit `interface_files` list drawn
   from its member packages (identical in Bazel and textproto); implementation lives in
   non-interface files of the same package;
2. components that need **authority** confine every environment interaction to a small
   sub-component (`<component>/internal/host*`) with its own narrow API and measured
   declaration, preferring capability passing (`fs.FS`, `io.Reader`, bytes) wherever the
   code merely *processes* data;
3. **`PACKAGE_SURFACE` is used only for foreign code** (`protobuf-runtime`, `x-tools`,
   new `capslock-runtime`).

## Files

| File | Purpose |
|------|---------|
| [`design/detailed-design.md`](design/detailed-design.md) | Constraining facts F1–F11, conventions C1–C12, three design questions (sub-component vs injection; the `encoding/json` codec exception and the `reportio` split; file-list interfaces vs packages), per-component targets §5, risks, deferred items, Appendix D = `go_component.interface_files` rule spec. |
| [`implementation/plan.md`](implementation/plan.md) | 16 ordered steps with Objective / Guidance / Tests / Integration / Demo, a standard verification gate, and the inventory touch list per step. Written for a less capable model. |
| [`research/`](research/) | Six measured analyses (authority sites, exported-symbol usage, test coupling, split proposals) + the report template. |

## Headline decisions (R1)

- **File-list interfaces.** The native leg already defines an interface as a file set;
  the Bazel rule gets an `interface_files` attribute (plan Step 2, validated first on
  `goanalysis`/`checker`, whose textprotos already list subsets — fixing a latent
  two-leg divergence). Interface shrinking becomes intra-package file splits: no
  import-path churn, no test relocation, no cross-package type translation.
- **Packages are still required for authority** (F6 charges per component):
  `atomicfile`, `goanalysis-hostfs`, `packagelayout-hostenv`, `layout-driver-hook`,
  `stdlibmap-hosttoolchain`; and for ownership: `capslocksymbol`, `reportio`,
  `schema-gen`, `capslock-runtime`.
- `artifactio` becomes **authority-free**; its JSON report/golden codecs move to
  `reportio` (`[REFLECT, UNSAFE_POINTER]`, documented codec exception). Retiring those
  capabilities needs a protojson report — separate follow-up plan.
- `packagelayout` → `[REFLECT, UNSAFE_POINTER]` with `fs.FS`/bytes passing, `hostenv`,
  the driver `init()` moved to `layout-driver-hook`, and a project-owned `BuildTags`
  replacing `build.Context` (mandatory if spike S2 shows `go/build` is charged).
- `goanalysis` → three interface files (≈ 13 symbols), empty authority; `RUNTIME` and
  the `report` dep were dead.
- `schema-gen`: declare `UNSAFE_POINTER` + `analysis_defeating_policy: WARN`; the
  expected-fail gate becomes a real green gate with a 15-finding growth assertion.
- Counts after the work: Bazel gates 16 → 24, native analyses 20 → 29, asserted
  surfaces 2 → 3, expected-fail analyses 1 → 0.

## Remaining open points

1. Spike S2 (Step 1) outcome decides whether `BuildTags` is mandatory and whether
   `hostenv` declares `RUNTIME`.
2. Spike S4 (Step 2): if the rules/aspect assume an interface *library* somewhere not
   yet seen, the fallback is the previous (package-per-interface) revision of the plan.
3. `goanalysis/fixture_test.go` coupling to `checker` internals: rewrite in Step 6 if
   cheap, otherwise recorded follow-up.
