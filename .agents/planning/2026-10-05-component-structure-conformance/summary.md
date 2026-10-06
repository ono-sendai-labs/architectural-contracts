# Component structure conformance — summary

**Date:** 2026-10-05 · **Revised:** 2026-10-06 (R1, R2) · **Status:** Steps 1–14 of 16
done (probe script, conventions, spikes S1/S2/S3/S4 resolved, `interface_files` landed,
Step 3 hygiene/unexports/dead-dep removal complete, Step 4 `atomicfile` extracted,
Step 5 `artifactio` authority-free + `reportio` split, Step 6 `checker` interface
shrunk to `checker.go`, Step 7 `capslocksymbol` split from `symbol`, Step 8 `schema` +
`schema-gen` split and expected-fail gate retired, Step 9 `capslock-runtime` wrapper
extracted and `capslockadapter` shrunk to strict `REFLECT`, Step 10 `goanalysis-hostfs`
nested sub-component landed and `FILES` removed from `goanalysis`, Step 11
`packagelayout-hostenv`, `layout-driver-hook`, and `BuildTags` landed and `goanalysis`
now authority-free, Step 12 `goanalysis` split into 3 interface files totaling 222 LOC,
Step 13 `packagelayout` converted to declared style with `api.go` + `layout.go` and
`[REFLECT, UNSAFE_POINTER]` only, Step 14 `cli` converted to declared style with
`interface_files: "app/app.go"`); Steps 15–16 in progress

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
- `packagelayout` → `[REFLECT, UNSAFE_POINTER]` with `fs.FS`/bytes passing, `hostenv`
  (declares `RUNTIME`), the driver `init()` moved to `layout-driver-hook`, and a
  project-owned `BuildTags` replacing `build.Context` (**mandatory**: spike S2 measured
  that importing `go/build` or `runtime` is itself charged — F12).
- `goanalysis` → three interface files (≈ 13 symbols), empty authority. Its `RUNTIME`
  turned out to be *real* (import charge for one `runtime.Version()` call), so
  `hostenv` + `BuildTags` (Step 11) now land before the `goanalysis` split (Step 12).
- `schema-gen`: declare `analysis_defeating_policy: WARN` (and no `declared_authority` —
  the 15 `protoc-gen-go` sites are `AnalysisDefeating`, not `TrueAuthority`
  `UNSAFE_POINTER`); the expected-fail gate becomes a real green gate with a 15-site
  growth assertion.
- Counts after the work: Bazel gates 16 → 24, native analyses 20 → 29, asserted
  surfaces 2 → 3, expected-fail analyses 1 → 0.

## Spike outcomes (R2)

| Spike | Result |
|-------|--------|
| S1 nested sub-component | Landed on `goanalysis-hostfs` (`go/internal/goanalysis/internal/hostfs`); parent report shows `CHECKED_PASS`, zero `FILES` findings remain on `goanalysis`, parity pairs `hostfs` by basename. |
| S2 import charges | `import "go/build"` → `READ_SYSTEM_STATE`; `import "runtime"` → `RUNTIME`. Measured on `goanalysis` too (`goanalysis.go:15, :22`). |
| S3 capability passing | `fs.WalkDir/ReadDir/ReadFile/Stat` over an injected `fs.FS`: zero findings. |
| S4 `interface_files` | Landed; fixture covers `CALLS_UNDECLARED_INTERFACE`, `METHOD_OUTSIDE_INTERFACE`, unknown-file failure; parity is set-equality; `goanalysis`, `checker`, `csvfile` converted (`csvfile` was a real divergence). |

## Remaining open points

1. *(Resolved in Step 6)* `goanalysis/fixture_test.go` coupling to `checker` internals:
   8 strictly internal symbols unexported; the fixture-test symbols stay exported in
   non-interface files (`boundary.go`, `classify.go`) with explicit test-only doc
   comments (allowed under C4), and the full test rewrite will be recorded in
   `follow-ups.md` in Step 16.
2. *(Resolved in Step 14)* Native loader accepts `interface_files: "app/app.go"` in
   `go/cmd/arcc/component.textproto` with canonical Go import paths in `members:`, and
   `validateInterfaceFiles` (`go/internal/goanalysis/ifacefiles.go:85`) already emits the
   specific diagnostic `"interface file %q does not belong to any loaded Go package under
   component root"` when an interface file sits in a package missing from `members`.
