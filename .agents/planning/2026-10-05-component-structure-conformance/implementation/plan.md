# Implementation plan — component structure conformance

**Date:** 2026-10-05 · **Revised:** 2026-10-05 (R1: file-list interfaces via a new
`go_component.interface_files` attribute; `reportio` split; `BuildTags` and `schema-gen`
WARN decided); 2026-10-05 (R2: Steps 1–2 done; spikes S2/S3/S4 resolved — `go/build`
and `runtime` imports are charged, so `hostenv` + `BuildTags` (Step 11) now precede the
`goanalysis` split (Step 12); `goanalysis`'s `RUNTIME` is real, not dead)
**Design:** [`../design/detailed-design.md`](../design/detailed-design.md) (§ references
below are to that document; F1–F11 are its constraining facts, C1–C12 its conventions,
Appendix D the rule change)
**Research:** [`../research/`](../research/) — per-component measured analyses with
`file:line` citations
**Baseline:** working copy of `architectural-contracts-compositional-next`, 2026-10-05
(`bazel test //...` green; `just selfcheck` green; 16 Bazel gates, 20 native analyses)

This plan is written to be executed by a less capable model, one step at a time. Every
step is self-contained: it says what to move, what the resulting API is, which
inventories to touch, and which commands must be green before the step is done. **Do
not reinterpret the design**; where a step says "implementer's choice" the choice is
local and must be recorded in the step's checklist entry.

Two kinds of change recur. Learn the difference before starting:

- **Intra-package file split** (interface shrinking, §4.3). Pure code motion between
  files of *one* package: `git diff --color-moved=dimmed-zebra` must show only moves plus
  export/unexport renames. No import paths change, no tests relocate, no BUILD `deps`
  change. The result is an `interface_files` list naming the API files.
- **New component** (authority confinement or ownership, §4.3 table). A new package with
  its own `BUILD.bazel` + `component.textproto`, and the Appendix A inventory checklist.

## Ordering rationale

1. **Conventions and the measuring tool first** (Step 1): every later acceptance is
   "declared authority equals the probe's output".
2. **The `interface_files` rule change second** (Step 2) — everything after it depends
   on it, and it is validated on the two components (`goanalysis`, `checker`) whose
   textprotos already list file subsets, which also fixes the leg divergence.
3. **Hygiene before structure** (Step 3): dead declarations and unexports shrink every
   later diff.
4. **`atomicfile` before `artifactio`/`reportio`** (Step 4 → 5).
5. **`checker`, `capslocksymbol`, `schema`, `capslock-runtime` before `goanalysis`**
   (Steps 6–9 → 10–12): `goanalysis` gains `schema-gen` as a dep and `checker` settles
   first.
6. **`goanalysis-hostfs` is its own step** (Step 10, spike S1): nested-component
   plumbing is proven on a 60-line package before the 3,800-line split.
7. **`hostenv` + `BuildTags` before the `goanalysis` split** (Step 11 → 12). Spike S2
   measured that importing `go/build` or `runtime` is itself charged (F12), and
   `goanalysis` does both (`goanalysis.go:15, :22`). Its remaining authority can only
   leave through `packagelayout`'s new pure `BuildTags` API and the `hostenv`-forwarded
   Go version, so those land first and `goanalysis` ends Step 12 with
   `declared_authority = []`. The rest of the `packagelayout` split (Step 13) follows.
8. **`cli` and `stdlibmap` last** (Steps 14–15): native-only, least automated coverage.
9. **Final audit** (Step 16).

---

## Standard gate (referenced by every step as "the gate")

Run from the repo root unless noted. All must pass before a step is checked off.

```sh
# 1. Formatting / vet / unit tests of both legs
just ci                                   # or its parts if too slow locally:
bazel test //...
(cd go && go test ./...)

# 2. Native self-check (stages every component in topological order)
just selfcheck

# 3. Parity between checked-in textprotos and Bazel-generated manifests (set equality after Step 2)
bazel test //go/cmd/arcc/app:self_manifest_parity_test
(cd go && go test ./internal/manifestparity/... ./internal/manifest/...)

# 4. Contract rules: no consumer references a non-interface symbol; no split methods; no dead deps.
bazel build //go/... --output_groups=+arcc
grep -l 'CALLS_UNDECLARED_INTERFACE\|METHOD_OUTSIDE_INTERFACE\|UNUSED_DEPENDENCY' \
     bazel-bin/go/**/*.report.json                                   # must print nothing

# 5. Authority is measured, not guessed (design Appendix B; scripted in Step 1):
scripts/authority-probe.sh <relative manifest path>                  # for each touched component

# 6. For intra-package splits only: the diff is motion.
git diff --color-moved=dimmed-zebra --stat -- go/internal/<pkg>      # review; no logic edits
```

The probe's `UNDECLARED_AUTHORITY` findings must be exactly the capabilities the step
says the component declares — no more and no fewer. Record the probe summary
(capability → file:line list) in the step's checklist entry.

---

## Progress checklist

- [x] **Step 1** — Conventions, authority probe script, spikes S2/S3 *(done 2026-10-05; S2: both imports charged; S3: `fs.FS` free)*
- [x] **Step 2** — `go_component.interface_files` attribute; parity set-equality; `goanalysis`/`checker` converted (spike S4) *(done 2026-10-05; `csvfile` also converted — real divergence found)*
- [x] **Step 3** — Hygiene: honest declarations, unexports, explicit members *(done 2026-10-05; `report` dropped from `goanalysis`, `x-tools` dropped from `cli`, `hostpolicy` explicit member, unexports across 8 packages)*
- [x] **Step 4** — New component `atomicfile` *(done 2026-10-05; lifted from `artifactio/write.go`, `FILES` ×4)*
- [x] **Step 5** — `artifactio` authority-free + new `reportio` *(done 2026-10-05; `artifactio` authority-free, `reportio` holds `REFLECT`+`UNSAFE_POINTER`)*
- [x] **Step 6** — `checker`: file-list interface = `checker.go` *(done 2026-10-05; `interface_files = ["checker.go"]`, 8 internal symbols unexported)*
- [x] **Step 7** — `symbol` → `symbol` + `capslocksymbol` *(done 2026-10-05; `symbol` 653 LOC, `capslocksymbol` 1,105 LOC, both pure)*
- [x] **Step 8** — `schema` → `schema` + `schema-gen` (WARN; last in-tree `PACKAGE_SURFACE` removed) *(done 2026-10-05; `schema` pure, `schema-gen` WARN with 15-site assertion; 0 `TrueAuthority` so `UNSAFE_POINTER` omitted)*
- [x] **Step 9** — `capslock-runtime` wrapper; `capslockadapter` shrinks *(done 2026-10-05; `capslock-runtime` native-only asserted UNKNOWN surface, `capslockadapter` strict with `REFLECT` ×2)*
- [x] **Step 10** — `goanalysis-hostfs` sub-component (spike S1) *(done 2026-10-05; S1 passed: `FILES` removed from `goanalysis`, `hostfs` holds `FILES` ×4 + `READ_SYSTEM_STATE` ×1)*
- [ ] **Step 11** — `packagelayout-hostenv` sub-component, `layout-driver-hook`, `BuildTags` (+ `goanalysis` drops `RUNTIME`/`READ_SYSTEM_STATE`)
- [ ] **Step 12** — `goanalysis`: file splits + three interface files
- [ ] **Step 13** — `packagelayout`: file splits, `fs.FS` passing, two interface files
- [ ] **Step 14** — `cli`: declared style with `app/app.go` as the interface
- [ ] **Step 15** — `stdlibmap`: `hosttoolchain` sub-component + two interface files
- [ ] **Step 16** — Final audit, README counts, conventions cross-check

---

## Step 1: Conventions, authority probe script, spikes S2/S3

**Objective.** Write down the rules we are about to enforce, build the tool that
measures conformance, and resolve the import-charge question before any code moves.

**Guidance.**
- **README section.** Add "Component structure conventions" to `README.md` (after the
  component-model section, before the limitations list): design §3 C1–C12 in prose,
  §4.1 (sub-component vs capability passing), §4.3's table (when a package is still
  required) and a three-sentence §4.2. Mention `interface_files` as the way to declare
  the interface (it lands in Step 2; write the text now).
- **Probe script** `scripts/authority-probe.sh <manifest-rel-path>` (design Appendix B):
  1. `cd go && go build -o ../bin/arcc ./cmd/arcc`.
  2. Create a temp stage as `justfile:117-124` does (copy `go/`, source
     `scripts/selfcheck-staging.sh`).
  3. Stage every component **preceding the target** in the `justfile:125-147` order —
     parse the list from the justfile (`grep -o 'selfcheck_[a-z_]*component.*textproto' justfile`)
     rather than duplicating it; stop at the target.
  4. Rewrite the target's staged `component.textproto` with all `declared_authority:`
     lines removed.
  5. Run `arcc check` with the pinned `go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json`,
     not failing on non-zero exit; print a table `capability | member symbol | stdlib
     symbol | file:line` from the report's `UNDECLARED_AUTHORITY` findings (`jq`; field
     names in `go/internal/report/report.go`).
  6. Exit 0 when the table is empty, 2 otherwise.
  Add `just probe component=<path>` and `scripts/authority-probe-test.sh` (runs it on
  `facts` → empty; on `artifactio` → `FILES`, `REFLECT`, `UNSAFE_POINTER` at baseline),
  wired into `just ci` next to `justfile:60`.
- **Spike S2 (import-time charges).** Throwaway `go/internal/zzspike/` (deleted at the
  end of the step, never committed): `a.go` = `import "go/build"; var _ build.Context`;
  `b.go` = `import "runtime"; var _ = runtime.Compiler`. Probe. Record in
  `../research/spike-s2-import-charges.md`.
  **Outcome (measured): both charged** — `go/build.init` → `READ_SYSTEM_STATE`,
  `runtime.init` → `RUNTIME` (F12). Consequences are already folded into Steps 3, 10, 11.
- **Spike S3.** Same throwaway: a function taking `fs.FS` that calls `fs.WalkDir`,
  `fs.ReadDir`, `fs.ReadFile`, `fs.Stat`. **Outcome: zero findings** — capability passing
  is free; Step 13's `stdlibdiscover`/`validate` design stands.

**Tests.** `scripts/authority-probe-test.sh` in `just ci` (recipe `authority-probe-test`,
wired into `ci-go`).

**Integration.** No component changes; `just ci` only.

**Demo (observed).** `scripts/authority-probe.sh goanalysis` prints `FILES` ×9
(`defeatscan.go:228`, `goanalysis.go:42,658,689,1003,1063`, `surface_resolver.go:110,714,725`),
`READ_SYSTEM_STATE` ×2 (`go/build.init` at `goanalysis.go:15`, `filepath.Abs` at `:557`)
and `RUNTIME` ×1 (`runtime.init` at `goanalysis.go:22`). The research report's claim
that `RUNTIME` was unexercised was wrong; the declaration stays until Step 11.

**Status: DONE 2026-10-05.** Files: `scripts/authority-probe.sh`,
`scripts/authority-probe-test.sh`, `justfile` (`probe`, `authority-probe-test`, `ci-go`),
`README.md` ("Component Structure Conventions"), `research/spike-s2-import-charges.md`.

---

## Step 2: `go_component.interface_files` attribute; parity set-equality (spike S4)

**Objective.** Let the Bazel leg declare an interface as a list of files inside the
member packages — what the native leg already does (F1) — and prove it on `goanalysis`
and `checker`, whose textprotos already list subsets. Specification: design Appendix D.

**Guidance.**
- **Macro** (`bazel_rules/go/defs.bzl:242-296`): add the `interface_files`
  `attr.label_list(allow_files = [".go"], configurable = False)` with the doc string
  from Appendix D. Thread it through `_go_component_impl` (`:184-222`) to the private
  rule; add the matching attr on `go_component_rule`.
- **Validation** (`_validate_component_shape`, `defs.bzl:143-182`): declared style ⇒
  exactly one of `interface` / `interface_files` (fail messages: *"declared
  interface_style requires exactly one of interface or interface_files"*);
  `interface_files` ⇒ non-empty `members`; `PACKAGE_SURFACE` ⇒ neither.
- **Private rule** (`bazel_rules/go/private/component.bzl`):
  - `:894-923`: when `interface` is unset, for each file in `ctx.files.interface_files`
    find the member `m` with the file in `go_library_srcs(m)`; fail *"component %s:
    interface file %s is not a src of any member"* if none; apply the existing
    "covered by component_dep" failure to `go_importpath(m)`; `effective_members` =
    members only.
  - `:996-1002`: `interface_files = sorted([runfiles_path(ctx, f) for f in
    ctx.files.interface_files])` when set; keep the `interface` branch.
  - Confirm nothing else dereferences `ctx.attr.interface` (grep `interface` in
    `component.bzl`, `aspect.bzl`, `go_adapter.bzl`); the asserted-surface path is
    unaffected.
- **Native containment check.** Write a textproto whose `interface_files` names a file
  outside `members`; if `arcc check` does not fail with an actionable error, add the
  validation in `go/internal/manifest/manifest.go` next to `:319-336` (or in the loader
  where interface files are resolved — `surface_resolver.go`/`surfaceinputs.go`) and a
  unit test.
- **Fixture** `bazel_rules/go/tests/testdata/filelist/`: a two-file package (`api.go`,
  `impl.go`) with `interface_files = ["api.go"]`; a consumer component that references
  an `impl.go` symbol → expected `CALLS_UNDECLARED_INTERFACE`; a variant where a type in
  `api.go` has an exported method in `impl.go` → expected `METHOD_OUTSIDE_INTERFACE`; a
  variant listing a file not in any member → expected analysis-time failure. Wire into
  the existing `bazel_rules/go/tests` test suite the same way `nested` is.
- **Parity set-equality.** In `go/internal/manifestparity` make `interface_files`
  comparison exact (the comparison lives after the basename pairing at
  `manifestparity.go:32-59`). Add a unit test with a textproto listing fewer files than
  the generated manifest → fails.
- **First real users.** `go/internal/goanalysis/BUILD.bazel`: replace `interface =
  ":goanalysis"` with `interface_files = ["defeatscan.go", "goanalysis.go",
  "surface_resolver.go", "surfaceinputs.go"], members = [":goanalysis"]` (exactly the
  textproto's current list — Step 12 shrinks it). `go/internal/checker/BUILD.bazel`:
  `interface_files = ["aggregate.go", "checker.go", "classify.go"]` (current textproto;
  Step 6 shrinks it). Both parity pairs must now be **equal**.
- Document the attribute where `go_component` attrs are documented (`bazel_rules/go/`
  README/g3doc) and in the README conventions section written in Step 1.

**Tests.** The fixture suite above; the parity equality test; `go_component` macro
validation tests if a Starlark unit-test file exists for `defs.bzl` (add cases: both
attrs set → fail; neither → fail; `interface_files` with empty members → fail).

**Integration.** Full gate. Gate item 4 is where a regression would show: any
consumer of `goanalysis`/`checker` that references a symbol outside the four/three
listed files now fails with `CALLS_UNDECLARED_INTERFACE` under Bazel exactly as it
already would natively. If one appears, the two legs *were* diverging — fix by adding
the file to both lists (temporarily; Step 6/12 redraws them) and record it.

**Demo.** `bazel build //go/internal/goanalysis:goanalysis_component --output_groups=+arcc`
produces a `goanalysis_component.component.textproto` whose `interface_files` equals the
checked-in list; `self_manifest_parity_test` passes with equality on.

**If S4 fails** (a structural assumption about an interface *library* elsewhere in the
rules or the aspect): stop, write `../research/spike-s4-interface-files.md`, and ask —
the fallback is the package-per-interface plan (previous revision of this file in VCS).

**Status: DONE 2026-10-05 (S4 passed).** Landed: `interface_files` attr + validation
(`defs.bzl`, `component.bzl`), fixture `bazel_rules/go/tests/testdata/filelist/` (three
variants + positive manifest assertion), `manifestparity` set-equality + test,
`goanalysis`/`checker`/`csvfile` converted (the last was a *real* divergence:
`private.go` was in the Bazel interface only), F9 visibility added to `:goanalysis` and
`:checker`. Native containment: an interface file in a non-root package requires that
package in `members` (`goanalysis.go:801-835`, F13) — the error message is the generic
"non-member package … retains source syntax" one; Step 14 should confirm it is
acceptable for `cli` or improve it. Gate item 4 at this point still matches
`goanalysis_component.report.json` on `UNUSED_DEPENDENCY: report_component` — the
pre-existing dead dependency removed in Step 3.

---

## Step 3: Hygiene — honest declarations, unexports, explicit members

**Objective.** Make every existing declaration true (G4) and remove internal-only
exports (C4) *before* restructuring.

**Guidance.**
- **Dead declarations** (probe before removing each):
  - `go/internal/goanalysis/{component.textproto,BUILD.bazel}`: remove the `report`
    dependency (both legs). **Keep `RUNTIME`** — the probe shows it is exercised through
    `import "runtime"` (`goanalysis.go:22`, F12); it leaves in Step 11.
  - `go/cmd/arcc/component.textproto`: remove the `x-tools` dependency.
  - `go/internal/artifactio/artifactio.go:30-32`: rewrite the package doc to name
    exactly `FILES`, `REFLECT`, `UNSAFE_POINTER` (rewritten again in Step 5).
- **Explicit members.** `hostpolicy/component.textproto`: add `members:` (the only
  directory-based manifest left).
- **Unexport internal-only symbols** using the research tables (symbol → external
  consumers): `artifactio` (11, `research/analysis-artifactio.md` §2), `manifest` (16),
  `facts` (13), `report` (1), `packagelayout` (17), `goanalysis` (11), `stdlibmap` (31),
  `symbol` (`FromSelection`, `GrammarVersion`). Rule: no reference outside the package
  (including other packages' `_test` files) ⇒ unexport (`gopls rename`). Symbols used
  only by another package's tests stay exported and are listed in the checklist entry.
  For file-list components (after Step 2), an exported symbol in a *non-interface* file
  that is used by nobody outside is unexported too.
- **Delete** `symbol.ParseCapslockWithPackages` (unused).

**Tests.** No behaviour change. `go vet ./...`.

**Integration.** Full gate.

**Demo.** Probe output for `goanalysis` shows no capability that is not declared and no
declared capability without a finding (`FILES`, `READ_SYSTEM_STATE`, `RUNTIME` — all
three measured); gate item 4 no longer matches `goanalysis_component.report.json`.

**Status: DONE 2026-10-05.**
- Removed dead `report` dependency from `go/internal/goanalysis/{component.textproto,BUILD.bazel}`; added `(PersistedReport).ComponentName()` in `artifactio/report.go` so `goanalysis/surface_resolver.go` does not select fields on `report.ConformanceReport` (F3).
- Removed `x-tools` dependency from `go/cmd/arcc/component.textproto`; added `(Layout).PackageViews()` (`PackageShapeView`) in `packagelayout/shape_imports.go` so `cmd/arcc/app/layoutshape.go` does not select fields on `packages.Package` (F3), eliminating both `UNUSED_DEPENDENCY` and `UNDECLARED_DEPENDENCY` on `cli`.
- Added explicit `members: "github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"` to `go/internal/hostpolicy/component.textproto`; rewrote `go/internal/artifactio/artifactio.go` package doc to list only `FILES`, `REFLECT`, `UNSAFE_POINTER`; deleted `symbol.ParseCapslockWithPackages`.
- Unexported internal-only symbols across `artifactio`, `manifest`, `facts`, `report`, `packagelayout`, `goanalysis`, `stdlibmap`, and `symbol` (`FromSelection`, `GrammarVersion`), using `export_test.go` bridges in `symbol`, `manifest`, and `stdlibmap` for black-box tests; kept external-test-referenced symbols exported (`SurfaceFormatVersion`, `ShapeReport`, `MarshalVerdict` in `artifactio`; `AnalysisDefeatingPolicyStrict`, `InterfaceStyleDeclared` in `manifest`; `ResolveDependencySurface` in `goanalysis`; `OrdinaryImportData`, `StdlibExportData`, `StdlibExportRoot` in `packagelayout`; `WriteArtifactAtomic`, `RenderTargetConfig`, `FindingSource`, `FindingSourceFunc`, `Generate`, `GeneratedMap`, `Loader`, `LoaderFunc` in `stdlibmap`).
- Probes: `goanalysis` → `FILES` ×9 (`defeatscan.go:228`, `goanalysis.go:42,658,689,1018,1078`, `surface_resolver.go:102,706,717`), `READ_SYSTEM_STATE` ×2 (`goanalysis.go:15,557`), `RUNTIME` ×1 (`goanalysis.go:22`); `cli` → `FILES` ×10, `MODIFY_SYSTEM_STATE` ×2, `OPERATING_SYSTEM` ×2, `READ_SYSTEM_STATE` ×3, `REFLECT` ×5, `UNSAFE_POINTER` ×5; `hostpolicy` → empty. Gate item 4 prints nothing.

---

## Step 4: New component `atomicfile`

**Objective.** Lift the atomic-replace primitive out of `artifactio/write.go` into the
single reviewed `FILES` component (§5.1).

**Guidance.**
- `go/internal/atomicfile/atomicfile.go`: `WriteFileAtomic`, `WriteSeams` moved
  **unchanged** from `artifactio/write.go` (`os` calls at `write.go:37,56,60,64` are the
  whole `FILES` surface); `write_test.go` moves with it. Package doc: "the one place that
  replaces a file on disk; callers hand it bytes".
- `BUILD.bazel`: `go_library` (F9 visibility), `go_test`, `go_component(name =
  "atomicfile_component", interface_files = ["atomicfile.go"], members =
  [":atomicfile"], declared_authority = ["FILES"])`, `exports_files`.
  `component.textproto`: `name: "atomicfile"`, `members: "./"`, `interface_files:
  "atomicfile.go"`, `declared_authority: FILES`.
- Consumers switch now: `go/cmd/arcc/app/check.go:528`, `go/internal/stdlibmap/cache.go:183,332`,
  tests (`grep -rn WriteFileAtomic go/`). Add `atomicfile` to `cli` and `stdlibmap`
  textproto deps and `app`'s BUILD deps. `artifactio` keeps `FILES` until Step 5.
- Appendix A: 4 (`justfile`: stage `internal/atomicfile` after `capanalyzer`; counts),
  5, 6 (`wantOwner`; **not** `purePackages`), 10 (batch into Step 16 but note), 11.

**Tests.** `write_test.go` as-is; add "failed rename leaves no temp file" if absent.

**Integration.** Full gate. Probe `atomicfile` → `FILES` at four sites; probe
`artifactio` → `FILES` only at `report.go:358,384`.

**Demo.** `bazel-bin/go/internal/atomicfile/atomicfile_component.report.json`: pass,
zero findings.

**Status: DONE 2026-10-05.**
- Extracted `go/internal/atomicfile/{atomicfile.go,write_test.go,BUILD.bazel,component.textproto}` (`interface_files = ["atomicfile.go"]`, `declared_authority = ["FILES"]`).
- Deleted `go/internal/artifactio/{write.go,write_test.go}` and removed test-only `WriteSurface` from `go/internal/artifactio/surfacepaths.go`.
- Switched consumers (`go/cmd/arcc/app/check.go`, `go/internal/stdlibmap/cache.go`, and tests across `go/cmd/arcc`, `go/internal/artifactio`, `go/internal/goanalysis`, `go/internal/stdlibmap`) to `atomicfile.WriteFileAtomic`.
- Updated `justfile` (21 staged components, 20 pass, 17 Bazel gates), `go/cmd/arcc/app/BUILD.bazel` (`self_manifest_parity_test`), and `go/internal/manifestparity/ownership_test.go`.
- Probes:
  - `atomicfile`: `FILES` ×4 (`atomicfile.go:41,60,64,68`).
  - `artifactio`: `FILES` ×2 (`report.go:363,389`), `REFLECT` ×6 (`golden.go:570,579`, `report.go:78,114,139,157`), `UNSAFE_POINTER` ×6 (same sites).

---

## Step 5: `artifactio` authority-free + new `reportio`

**Objective.** Split `artifactio` by artifact family (§4.2 decision R1, §5.2):
protojson codecs stay and become authority-free; the JSON report/golden codecs become
`reportio` with exactly `[REFLECT, UNSAFE_POINTER]`.

**Guidance.**
- **New `go/internal/reportio`** (package `reportio`): move `report.go` and `golden.go`
  with their tests. Keep names (`MarshalReport`, `DecodeReport`, `PersistedReport`,
  `Snapshot`, `CompareShape`, version constants). Add `DecodeVerdictGolden(io.Reader)`
  replacing `ReadVerdictGoldenFile`; delete `ReadReportFile` (callers in
  `cmd/arcc/app/verdict.go` and tests open the file and call `DecodeReport`). `golden.go`
  decodes surfaces/maps via `artifactio` (import direction `reportio → artifactio`). Any
  helper both packages need (`boundedRead`, canonical-JSON) is duplicated or moved to
  whichever side owns it — do **not** create a third package for ~20 lines.
  `BUILD.bazel`: `go_component(name = "reportio_component", interface_files =
  ["report.go", "golden.go"], members = [":reportio"], declared_authority = ["REFLECT",
  "UNSAFE_POINTER"], component_deps = [report, artifactio, schema(-gen)])` with the
  **exact** C9 comment from design §5.2 above `declared_authority` and in the textproto.
- **`artifactio`**: delete `write.go` (Step 4 already moved it), `report.go`,
  `golden.go`, `WriteSurface` (test-only; tests use `atomicfile` + `MarshalSurface`).
  `BUILD.bazel`: `interface_files = ["artifactio.go", "digest.go", "surface.go",
  "stdlibmap.go", "surfacepaths.go"], members = [":artifactio"]`, **no**
  `declared_authority`, **no** `interface_style`. `stdlibmapreader.go` is non-interface:
  its exported constructor is declared in `stdlibmap.go` (move the one-line declaration
  if needed); the reader type and its methods become unexported (returned as
  `stdlibauthority.StdlibAuthority`). Drop the `report` dep. Rewrite the package doc.
- **Consumers:** `cmd/arcc/app` (check/verdict/shape) imports `reportio`; `cli` textproto
  += `reportio`; `goanalysis`/`stdlibmap` keep `artifactio` only (gate item 4 confirms no
  `UNUSED_DEPENDENCY`). `ownership_test.go:140-194` (`artifactio` dep inventory) updated;
  `wantOwner` += `reportio`; `purePackages` += `artifactio`.
- Appendix A: 4 (`justfile`: `reportio` right after `artifactio`; counts), 5, 6, 10, 11.

**Tests.** Move with their files. `golden_test.go` fixtures stay under
`reportio/testdata`.

**Integration.** Full gate. Probe `artifactio` → **empty**; probe `reportio` → exactly
`REFLECT` + `UNSAFE_POINTER`, all sites in `report.go`/`golden.go`.

**Demo.** `grep -n 'declared_authority\|interface_style' go/internal/artifactio/BUILD.bazel`
is empty; `reportio_component.report.json` passes.

**Status: DONE 2026-10-05.**
- Created `go/internal/reportio/{report.go,golden.go,report_test.go,golden_test.go,BUILD.bazel,component.textproto}` (`interface_files = ["golden.go", "report.go"]`, `declared_authority = ["REFLECT", "UNSAFE_POINTER"]` with exact C9 comment).
- Replaced `ReadVerdictGoldenFile` with `DecodeVerdictGolden(io.Reader)` and deleted `ReadReportFile` (`cmd/arcc/app/verdict.go` opens files directly with `io.LimitReader(rf, reportio.MaxReportBytes+1)`).
- Converted `artifactio` to declared style (`interface_files = ["artifactio.go", "digest.go", "stdlibmap.go", "surface.go", "surfacepaths.go"]`, NIF `stdlibmapreader.go` with `NewStdlibMapReader`/`OpenStdlibMap` moved to `stdlibmap.go`) and zero `declared_authority`.
- Note on `goanalysis`: `go/internal/goanalysis/surface_resolver.go:596` (`readDependencyReport`) calls `reportio.DecodeReport` to inspect companion `.report.json` verdicts, so `goanalysis` declares a component dependency on `reportio` as well as `artifactio`. Per F6, `reportio`'s `REFLECT`/`UNSAFE_POINTER` are charged to `reportio`, never to `goanalysis`.
- Probes:
  - `artifactio`: **empty** (0 findings).
  - `reportio`: `REFLECT` ×6 (`golden.go:573,582`, `report.go:78,114,139,157`), `UNSAFE_POINTER` ×6 (same sites).
  - `goanalysis`: `FILES` ×6, `READ_SYSTEM_STATE` ×2, `RUNTIME` ×1 (no `REFLECT`/`UNSAFE_POINTER`).

---

## Step 6: `checker` — file-list interface = `checker.go`

**Objective.** Shrink the interface from 20 exported symbols to the 2 consumers use
(§5.6). Intra-package.

**Guidance.**
- `BUILD.bazel` + textproto: `interface_files = ["checker.go"]`. `boundary.go`,
  `classify.go`, `aggregate.go` are non-interface files of the same package.
- Unexport everything in the three NIF files that only `checker.go`/in-package tests
  use. Exceptions (external tests): `goanalysis/fixture_test.go` and
  `capanalyzer/capanalyzer_test.go` use `NewBoundaryIndex`, `(BoundaryIndex).Classify`,
  `ClassifyEdges`, `Classified`, `AuthorityObservation`, `ReferenceDecision.ViolationKind`.
  - `capanalyzer_test.go`: rewrite so a dependency's test does not import its dependent
    (local helper or move the case to `checker`'s own tests). Required.
  - `fixture_test.go`: rewrite to assert through `checker.Check` if ≤ ~1 day; otherwise
    leave those six symbols exported in NIF (allowed by C4 — tests are not analysed) and
    record the follow-up (design §7).
- **Method rule (F3′):** `Inputs` and any type declared in `checker.go` must have all
  exported methods in `checker.go`. Grep `func (.*Inputs)` etc.

**Tests.** In place.

**Integration.** Full gate; gate item 4 across `goanalysis` and `app`.

**Demo.** `go doc ./internal/checker` lists `Inputs`, `Check` (+ whatever the fixture
exception kept, clearly commented as test-only).

**Status: DONE 2026-10-05.**
- Set `interface_files = ["checker.go"]` in `go/internal/checker/{BUILD.bazel,component.textproto}` (including `broken_checker_component`) and updated `go/internal/manifest/manifest_test.go`.
- Verified `go/internal/capanalyzer/capanalyzer_test.go` does not import `checker` (the research table had conflated `capanalyzer.Classify` with `(BoundaryIndex).Classify`).
- Unexported 8 internal-only symbols across NIF files (`authoritySite`, `authorityFinding`, `aggregateAuthority`, `applyAuthorityPolicy`, `referenceIntraComponent`, `referenceDecision`, `boundaryIndex`, `boundaryObservation`), with `go/internal/checker/export_test.go` bridging `package checker_test`.
- Kept `ReferenceStatus`, `ReferenceDeclaredDependency`, `ReferenceCallsUndeclaredInterface`, `ReferenceUndeclaredDependency`, `(referenceDecision).ViolationKind`, `NewBoundaryIndex`, `(boundaryIndex).Classify`, `AuthorityObservation`, `Classified`, and `ClassifyEdges` exported in NIF (`boundary.go`/`classify.go`) with explicit test-only doc comments for `goanalysis/fixture_test.go` and `member_only_integration_test.go` (allowed by C4; full test rewrite tracked for `follow-ups.md`).
- Probe `checker`: **empty** (0 findings).

---

## Step 7: `symbol` → `symbol` + `capslocksymbol`

**Objective.** Move the 1,049-LOC Capslock parser, which has one consumer, out of the
vocabulary component (§5.7). New component (ownership).

**Guidance.**
- `go/internal/capslocksymbol/`: `capslock.go` + its three test files moved from
  `symbol/`; package `capslocksymbol`; exports `ParseCapslock`, `CapslockFunction`,
  `CapslockInventory`, `ParseCapslockFunction`. Imports `symbol`, `hostpolicy`.
- `BUILD.bazel`: `go_component(name = "capslocksymbol_component", interface_files =
  ["capslock.go"], members = [":capslocksymbol"], component_deps = [symbol,
  hostpolicy])`; textproto likewise.
- Consumer `go/internal/stdlibmap` (+ tests): update imports; textproto dep.
- `symbol` textproto/BUILD `interface_files` drop `capslock.go`.
- Appendix A: 4 (stage after `symbol`; counts), 5, 6 (`wantOwner`, `purePackages`), 10.

**Tests.** Moved unchanged.

**Integration.** Full gate; probe both → empty.

**Demo.** Non-test `wc -l go/internal/symbol/*.go` ≈ 655; `capslocksymbol_component`
report passes.

**Status: DONE 2026-10-05.**
- Extracted `go/internal/capslocksymbol/{capslock.go,capslock_test.go,capslock_corpus_test.go,capslock_inventory_test.go,BUILD.bazel,component.textproto}` (`interface_files = ["capslock.go"]`, `component_deps = [hostpolicy, symbol]`, 0 authority).
- Updated `go/internal/symbol/{BUILD.bazel,component.textproto,symbol.go}` (`interface_files = ["extract.go", "symbol.go", "typesconv.go"]`, non-test LOC = 653 across the 3 files).
- Updated sole consumer `go/internal/stdlibmap` (`generate.go`, `inventory.go`, `inventory_test.go`, `stdlibmap.go`, `BUILD.bazel`, `component.textproto`).
- Updated `justfile` (23 staged components, 22 pass + 1 fail, 19 Bazel gates), `go/cmd/arcc/app/BUILD.bazel`, and `go/internal/manifestparity/ownership_test.go`.
- Probes:
  - `symbol`: **empty** (0 findings).
  - `capslocksymbol`: **empty** (0 findings).

---

## Step 8: `schema` → `schema` + `schema-gen` (WARN)

**Objective.** Remove the last in-tree `PACKAGE_SURFACE` (G3) and turn the expected-fail
gate into a real green gate (§5.8, decided R1).

**Guidance.**
- `go/internal/schema/BUILD.bazel`: `go_component(name = "schema_component",
  interface_files = ["capabilities.go"], members = [":schema"])`; no style, no
  authority, no deps. Textproto: `members: "./"` only (drops `./gen`),
  `interface_files: "capabilities.go"`.
- `go/internal/schema/gen/BUILD.bazel`: `go_component(name = "schema_gen_component",
  interface = ":gen", component_deps = [protobuf-runtime], declared_authority =
  ["UNSAFE_POINTER"], analysis_defeating_policy = "warn")` (constant name per
  `defs.bzl:282-286`); `component.textproto`: `name: "schema-gen"`, `interface_files` =
  the three `.pb.go`, `members: "./"`, dep `protobuf-runtime`, `declared_authority:
  UNSAFE_POINTER`, `analysis_defeating_policy: WARN`. BUILD comment: *"generated
  protoc-gen-go code uses unsafe.Slice/StringData and sync.Once; WARN keeps the 15
  boilerplate sites visible without failing the gate; regenerate, never hand-edit."*
- Build with `--output_groups=+arcc`. **Expected:** pass with exactly the 15 known WARN
  findings. If other findings appear, stop and report — do not widen the declaration.
- Remove `check_tags = ["manual"]`; in `justfile:131-132` replace the `fail` stage and
  echo with ordinary stages for `internal/schema` and `internal/schema/gen`
  (both before `manifest`). Add to `scripts/selfcheck-staging.sh` (or the probe test) an
  assertion that `schema-gen`'s report contains exactly 15 `AnalysisDefeating` findings,
  so growth fails (design §6 last row).
- **Consumers of `gen.*`** add `schema-gen` in both legs: `manifest`, `artifactio`,
  `reportio`, `surface`, `goanalysis`, `stdlibmap`, `cli`. Consumers of only
  `schema.KnownCapabilities` keep `schema` (`manifest`, `artifactio`, `stdlibmap`).
  Gate item 4 prunes any now-unused `schema` dep.
- Appendix A: 4, 5, 6 (`wantOwner` `schema/gen` → `schema-gen`; `artifactio` inventory),
  9 (`manifest_test.go:574-626` if it enumerates `schema` members), 10.

**Tests.** The 15-finding count assertion; existing tests.

**Integration.** Full gate. `grep -rn PACKAGE_SURFACE go/ --include=BUILD.bazel
--include=component.textproto` → `protobufruntime`, `xtools` only (Step 9 adds
`capslockruntime`). `just selfcheck` has **no** expected-fail line.

**Demo.** `bazel test //go/internal/schema/...` runs and passes both `.check` targets.

**Status: DONE 2026-10-05.**
- Split `schema` into `go/internal/schema` (`interface_files = ["capabilities.go"]`, pure, `check_tags = ["manual"]` removed) and `go/internal/schema/gen` (`schema-gen`, `interface = ":gen"`, `analysis_defeating_policy = "warn"` / `WARN`, dep `protobuf-runtime`).
- Measured authority deviation (Principle 4 / C10): probing `schema-gen` showed that the 15 `protoc-gen-go` boilerplate sites (`3× (sync.Once).Do`, `6× unsafe.Slice`, `6× unsafe.StringData`) are classified as `AnalysisDefeating` (`UNANALYZED`), with **0 `TrueAuthority` capabilities**. Declaring `declared_authority = ["UNSAFE_POINTER"]` emitted `WARNING: declared authority "UNSAFE_POINTER" is unused`; omitting `declared_authority` leaves `schema-gen` with 0 `UNUSED_AUTHORITY` warnings and 1 `ANALYSIS_LIMITATION` warning containing the 15 sites.
- Added `selfcheck_assert_analysis_defeating_sites` in `scripts/selfcheck-staging.sh` (tested in `scripts/selfcheck-staging-test.sh`) and wired `justfile` to assert 15 `ANALYSIS_LIMITATION` sites on `internal/schema/gen/component.report.json`; all 24 staged components now pass with 0 expected-fail stages.
- Updated consumers: `manifest`, `artifactio`, `stdlibmap` depend on both `schema` and `schema-gen`; `reportio`, `surface`, `goanalysis`, `cli` replaced `schema` with `schema-gen`.
- Updated `go/internal/manifestparity/manifestparity.go` (`normalizeComponentName` mapping `_` to `-` after trimming `_component`) and `ownership_test.go`.
- Probes:
  - `schema`: **empty** (0 findings).
  - `schema-gen`: **empty** under `analysis_defeating_policy: WARN` (0 `TrueAuthority` findings; 15 `AnalysisDefeating` sites downgraded to `ANALYSIS_LIMITATION` warning).

---

## Step 9: `capslock-runtime` wrapper; `capslockadapter` shrinks

**Objective.** Give the foreign Capslock closure one `PACKAGE_SURFACE`/`UNKNOWN` wrapper
like `x-tools`, so `capslockadapter` declares only its own authority (§5.9).

**Guidance.**
- `go/internal/capslockruntime/` mirroring `go/internal/xtools/` file-for-file; `name:
  "capslock-runtime"`, `interface_style: PACKAGE_SURFACE`, `authority: UNKNOWN`, members
  = the seven foreign import paths currently in `capslockadapter`'s members. Generate the
  checked-in surface the way `xtools`'s was (recipe in `go/internal/xtools/BUILD.bazel`
  or the justfile).
- Try `go_component(authority = "UNKNOWN")` under Bazel; if layout emission fails closed
  on cgo, drop the rule, keep native-only, document in the BUILD comment.
- `capslockadapter/component.textproto`: `members: "./"`; deps += `capslock-runtime`;
  delete `analysis_defeating_policy: WARN`; `declared_authority` → `REFLECT`
  (`generation.go:180-183`). Then attempt to replace the reflective digest with a
  deterministic rendering capslock exposes; if achieved, `[]`. Record.
- `ownership_inventory.go:237-311`: capslock / `x/sys` / `fatih` / `mattn` prefixes →
  `capslock-runtime`.
- Appendix A: 4 (`selfcheck_asserted_component internal/capslockruntime/…` before
  `capslockadapter`; counts), 6, 10.

**Tests.** Inventory test: no component other than `capslock-runtime` lists a
`github.com/google/capslock` member.

**Integration.** Full gate; probe `capslockadapter` → `REFLECT` (one site) or empty.

**Demo.** `just selfcheck` shows `capslock-runtime` asserted, `capslockadapter` passing
with no `AnalysisDefeating` lines.

**Status: DONE 2026-10-05.**
- Created `go/internal/capslockruntime/{BUILD.bazel,component.textproto,component.surface.json}` (`name: "capslock-runtime"`, `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE`, `authority: UNKNOWN`, 7 foreign packages). Tested `go_component(authority = UNKNOWN)` under Bazel: failed closed on cgo (`@org_golang_x_sys//unix`); documented as native-only in `BUILD.bazel` with `exports_files(["component.surface.json", "component.textproto"])`.
- Updated `go/internal/capslockadapter/component.textproto`: `members` shrunk to `go/internal/capslockadapter` only; removed `analysis_defeating_policy: WARN`; pruned now-unused `protobuf-runtime` and `hostpolicy` dependencies; added `capslock-runtime`; kept `declared_authority: "REFLECT"` (`classifierContentDigest` in `generation.go:170-196` inspects Capslock's unexported `Classifier` struct fields to pin `classifierHash` into the SDK key).
- Updated `go/internal/manifestparity/ownership_inventory.go` (`foreignOwnershipRules` for `capslock-runtime`), `ownership_test.go`, `go/cmd/arcc/cli_integration_test.go` (`TestIntegration_CapslockAdapter_PassesStrictly` + `TestIntegration_AnalysisDefeatingPolicy_SchemaGen`), `justfile`, and `scripts/selfcheck-staging-test.sh`.
- Probe `capslockadapter`: `REFLECT` ×2 (`generation.go:180` `(reflect.Value).MapKeys`, `generation.go:183` `(reflect.Value).MapIndex`).

---

## Step 10: `goanalysis-hostfs` sub-component (spike S1)

**Objective.** Land the first nested sub-component end to end on a 60-line package,
proving parent/child plumbing in both legs before the large split (§5.3 **S**).

**Guidance.**
- `go/internal/goanalysis/internal/hostfs/hostfs.go` (+ test), package `hostfs`:
  ```go
  func ReadFile(path string) ([]byte, error)              // os.ReadFile
  func ReadDir(path string) ([]fs.DirEntry, error)        // os.ReadDir
  func Stat(path string) (fs.FileInfo, error)             // os.Stat
  func OpenExportData(path string) (io.ReadCloser, error) // os.Open
  func Abs(path string) (string, error)                   // filepath.Abs
  ```
  Package doc: not an `fs.FS` because sources and `bazel-out` export data live under
  different roots; the only file in `goanalysis` that names `os`.
- `BUILD.bazel`: `go_library` (F9), `go_test`, `go_component(name = "hostfs_component",
  interface_files = ["hostfs.go"], members = [":hostfs"], declared_authority = ["FILES",
  "READ_SYSTEM_STATE"])`, `exports_files`. Textproto `name: "goanalysis-hostfs"`.
- **Rewire the ten sites** (research `analysis-goanalysis.md` §3; known:
  `goanalysis.go:557` `filepath.Abs`; `defeatscan.go:228` `os.ReadFile`;
  `surface_resolver.go:110,714,725` defaults; `ValidateInterfaceFiles`'s `os.Stat`/header
  read; export-data `os.Open`). Afterwards `grep -n '"os"' go/internal/goanalysis/*.go`
  (non-test) is empty and `filepath` is used only for pure path arithmetic
  (`Join/Base/Clean/Rel/ToSlash`).
- Parent: `component_deps += "//go/internal/goanalysis/internal/hostfs:hostfs_component"`;
  textproto `component_dependencies` += `./internal/hostfs/component.textproto`;
  `members` stays explicit (F5). `declared_authority` → `["READ_SYSTEM_STATE", "RUNTIME"]`
  (`FILES` gone; the two that remain are the `go/build` and `runtime` import charges,
  F12, removed in Step 11 — say so in a comment).
- Appendix A: 4 (stage `internal/goanalysis/internal/hostfs` immediately before
  `internal/goanalysis`; counts), 5, 6 (`wantOwner`), 10.

**Tests.** `hostfs_test.go` against `t.TempDir()`. Existing tests unchanged.

**Integration.** Full gate + S1 pass criteria: parent report shows `goanalysis-hostfs`
`CHECKED_PASS`, no `UNUSED_DEPENDENCY`; parity pairs `hostfs` by basename (F10). Probe
`goanalysis` → exactly `READ_SYSTEM_STATE` (`go/build.init` only) + `RUNTIME`
(`runtime.init` only), no `FILES`, no `filepath.Abs`; probe `hostfs` → `FILES` +
`READ_SYSTEM_STATE`.

**Demo.** `bazel build //go/internal/goanalysis/... --output_groups=+arcc` → two reports;
parent `declared_authority` has lost `FILES`.

**If S1 fails:** stop, write `../research/spike-s1-nested.md`, ask. Fallback is sibling
components (`goanalysishostfs`), which changes later steps' paths.

**Status: DONE 2026-10-05 (S1 passed).**
- Created `go/internal/goanalysis/internal/hostfs/{hostfs.go,hostfs_test.go,BUILD.bazel,component.textproto}` (`name = "hostfs_component"` / `name: "goanalysis-hostfs"`, `interface_files = ["hostfs.go"]`, `declared_authority = ["FILES", "READ_SYSTEM_STATE"]`).
- Rewired all 10 filesystem/path authority sites across `goanalysis.go`, `defeatscan.go`, and `surface_resolver.go` to `hostfs`; zero production imports of `"os"` remain in `go/internal/goanalysis/*.go`.
- Updated `go/internal/goanalysis/{BUILD.bazel,component.textproto}` to depend on `goanalysis-hostfs` (`:hostfs_component`) and removed `FILES` from `declared_authority`.
- Updated `go/internal/manifestparity/manifestparity.go` (nested sub-component name/dependency identity matching, tested in `manifestparity_test.go`), `ownership_test.go`, `go/cmd/arcc/app/BUILD.bazel`, and `justfile` (25 analyzed components, 22 Bazel gates).
- Probes:
  - `goanalysis-hostfs`: `FILES` ×4 (`hostfs.go:13,14,15,16`), `READ_SYSTEM_STATE` ×1 (`hostfs.go:17` `path/filepath.Abs`).
  - `goanalysis`: `READ_SYSTEM_STATE` ×1 (`goanalysis.go:15` `go/build.init`), `RUNTIME` ×1 (`goanalysis.go:21` `runtime.init`); zero `FILES` findings.

---

## Step 11: `packagelayout-hostenv` sub-component, `layout-driver-hook`, `BuildTags`

**Objective.** Move every environment interaction of `packagelayout` (env vars, process
identity, host toolchain facts via `runtime`/`go/build`, stdio in `init()`) into one
≈120-line sub-component and one ≈40-line hook component, and give `packagelayout` a
pure `BuildTags` API — so that `goanalysis` (Step 12) and `packagelayout` (Step 13) can
each drop `go/build` and `runtime` entirely (F12). (§5.4 **S** / sibling / `BuildTags`.)

**Guidance.**
- **`internal/hostenv`** (`go/internal/packagelayout/internal/hostenv/hostenv.go`,
  component `packagelayout-hostenv`):
  ```go
  func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error // body of packagelayout.go:2741-2820
  func DriverRequestFromEnv() (ok bool, layoutPath, workspaceDir string)      // GOPACKAGESDRIVER-mode vars incl. ARCC_DRIVER_MODE
  type NativeDefaults struct { GOROOT, GOOS, GOARCH, Compiler, Version, GOPATH string; CgoEnabled bool; ReleaseTags, ToolTags []string }
  func NativeTargetDefaults() NativeDefaults                                  // runtime.* + build.Default + os.Getenv("GOPATH"); replaces :369-370
  ```
  plus the env half of `WithTemporaryLayout`. Move `cgo_enabled.go`/`cgo_disabled.go`
  here. `declared_authority = ["READ_SYSTEM_STATE", "MODIFY_SYSTEM_STATE", "RUNTIME"]`
  (measured: `runtime.init`, `go/build.init`, `os.Setenv/LookupEnv/Executable`) — probe
  and keep exactly what it shows. `ARCC_DRIVER_MODE` semantics unchanged (also read by
  `stdlibmap/layoutloader.go:59`, asserted in
  `goanalysis/hermeticity_integration_test.go:109`). This is the **only** package in
  the tree allowed to import `runtime` or `go/build` (F12); `grep -rlE '^\s*"(runtime|go/build)"$' go --include=*.go --exclude=*_test.go`
  must list only this file by the end of Step 13.
- **`BuildTags` in `packagelayout`** (pure): add to the package (it will be in
  `layout.go` after Step 13; for now put it in a new `buildtags.go` that Step 13 folds in)
  ```go
  type BuildTags struct { GOOS, GOARCH string; CgoEnabled bool; ReleaseTags, BuildTags, ToolTags []string }
  func BuildTagsForLayout(l *Layout) BuildTags                      // replaces BuildContextForLayout
  func FileMatchesBuildConstraints(tags BuildTags, path string, src []byte) (bool, error) // replaces ...WithContext; evaluates go/build/constraint
  func NativeGoVersion() string                                     // forwards hostenv.NativeTargetDefaults().Version
  ```
  Implement constraint evaluation with `go/build/constraint` (`constraint.Parse`,
  `Expr.Eval(func(tag string) bool)`) over the `//go:build` line and the `_GOOS_GOARCH`
  filename rules — port the logic from `go/build`'s `goodOSArchFile`/`shouldBuild` as
  pure code with a unit test table that compares against `build.Context.MatchFile` in a
  `_test.go` (tests may import `go/build`; non-test code may not). Delete
  `BuildContextForLayout` and `FileMatchesBuildConstraintsWithContext`; update their
  callers in `packagelayout` and `goanalysis` (`goanalysis.go:1044,1146`). Replace
  `runtime.Version()` at `goanalysis.go:730` with `packagelayout.NativeGoVersion()`.
  After this, `goanalysis` imports neither `runtime` nor `go/build`; its
  `declared_authority` loses `RUNTIME` and (together with Step 10's `hostfs`)
  `READ_SYSTEM_STATE` — verify with the probe and update both legs now.
- **`go/internal/layoutdriverhook`** (component `layout-driver-hook`, ≈ 40 LOC): the
  `init()` from `packagelayout.go:2842-2852`, now: `hostenv.DriverRequestFromEnv()` →
  `os.ReadFile(layoutPath)` → `packagelayout.RunDriver(layout, workspaceDir, patterns,
  os.Stdin, os.Stdout)` → exit. (In this step `RunDriver` can be a thin exported wrapper
  over today's internal entry taking bytes; Step 13 finishes it.) Declare what the probe
  shows (expected `FILES`, `OPERATING_SYSTEM`, `MODIFY_SYSTEM_STATE`). Deps:
  `packagelayout`, `packagelayout-hostenv`.
- **Blank imports.** Replace the comment at `packagelayout.go:2838-2841`. Add
  `_ ".../internal/layoutdriverhook"` to `go/cmd/arcc/main.go` and to every test package
  spawned as its own `GOPACKAGESDRIVER`: `grep -rln 'WithDriverEnv\|WithTemporaryLayout'
  go/ --include=*_test.go` plus `go/internal/stdlibmap`. One `driverhook_test.go` per
  package with a one-line comment. A missing one shows as a hang / "driver produced no
  output" — run each listed package's tests individually.
- `packagelayout` depends on `hostenv` and forwards `WithDriverEnv` (public name
  unchanged). `goanalysis` deps unchanged (F6).
- Appendix A: 4 (stage `internal/packagelayout/internal/hostenv` before
  `internal/packagelayout`, `internal/layoutdriverhook` right after; counts), 5, 6
  (`wantOwner`), 10, 11 (`cli` += `layout-driver-hook`).

**Tests.** `hostenv_test.go`: env restored on success and error; `DriverRequestFromEnv`
false when unset; `NativeTargetDefaults` fields non-empty. `buildtags_test.go`: table
comparing `FileMatchesBuildConstraints` with `build.Context.MatchFile` over the cgo
pair, `_linux.go`/`_windows_amd64.go` names, `//go:build` expressions with `!`, `&&`,
`||`, release tags. Hermeticity tests prove a spawned driver still answers.

**Integration.** Full gate; especially `go test ./internal/goanalysis/...
./internal/stdlibmap/... ./cmd/...` (driver spawning) and the Bazel hermeticity tests.
Probe `packagelayout` → no `OPERATING_SYSTEM`, no `RUNTIME`; `READ/MODIFY_SYSTEM_STATE`
only if a non-env site remains (Step 13 handles `FILES`). Probe `goanalysis` → `FILES`
gone (Step 10), `RUNTIME` gone, `READ_SYSTEM_STATE` gone ⇒ **empty**; set
`declared_authority = []` in both legs if not already.

**Demo.** `grep -rlE '^\s*"(runtime|go/build)"$' go --include=*.go --exclude=*_test.go`
lists only `go/internal/packagelayout/internal/hostenv/hostenv.go` (and
`packagelayout.go` until Step 13 finishes removing `build.Default` uses there, if any
remain — list them in the checklist entry); `just selfcheck` shows both new components
passing.

---

## Step 12: `goanalysis` — file splits + three interface files

**Objective.** Reduce the interface from 29 exported symbols across every file to ≈ 13
symbols in three small files (§5.3). Intra-package. Authority is already empty after
Steps 10–11; this step must keep it so.

**Guidance.** Sub-steps in order; `go build ./... && go test ./internal/goanalysis/...`
between each; every sub-step is pure motion (gate item 6).

- **12.1** Split `goanalysis.go` (1,549 LOC): clusters 1–2 (driver, graph validation,
  export-data completion, the 14 helpers `surfaceinputs.go` uses) → `loader.go`;
  cluster 4 (symbol canonicalization + type-expression parser) → `symcanon.go`;
  cluster 3 (`ValidateInterfaceFiles` body, `leadingBuildConstraint`) → `ifacefiles.go`.
  `goanalysis.go` keeps: `LoadRequest`, `LoadPackageFacts`, `SerializeChecks`,
  `WithDriverEnv`, `ValidateInterfaceFiles` (signature + call into `ifacefiles.go`),
  `InterfaceFileExclusion`.
- **12.2** New `dependency_surface.go`: move `DependencySurfaceRequest`,
  `DependencySurfaceMode` + consts, `ResolveDependencySurfaceForDependency`,
  `ResolveDependencySurface` (declarations + thin bodies) out of `surface_resolver.go`;
  the remaining `surface_resolver.go` API becomes unexported.
- **12.3** `surfaceinputs.go` keeps `LoadSurfaceInputs`, `SurfaceInputs`,
  `PlatformIdentity`; remove the four shims at `:292-350` (`ActivePlatformIdentity`,
  `LayoutModeActive`, `ActiveDependencyArtifactBindings`, `CanonicalNamespace`) — `app`
  calls `packagelayout`/`hostpolicy` directly and computes `PlatformIdentity` over
  `packagelayout.Platform`. Keep a shim only if >1 caller needs it; record.
- **12.4** `refscan.go`, `defeatscan.go`, `scanobserver.go`: unexport
  `ScanReferences`/`ScanAnalysisDefeats` if their only non-test callers are in-package;
  their external tests (`*_test` package in the same dir) switch to the unexported names
  by becoming internal tests, or go through `LoadPackageFacts`. No file moves.
- **12.5 (hygiene, optional)** Replace the `loadedFiles` global keyed by
  `reflect.ValueOf(pkgs).Pointer()` (`goanalysis.go:38-42,344,968`) with an explicit
  return (`LoadResult{Facts, Files}` or a field on `facts.PackageFacts`; record). Deletes
  `reflect` and `membershipMu`.
- **12.6** Method rule (F3′): every type declared in the three IF files has all exported
  methods in IF files (`grep -n '^func (' go/internal/goanalysis/{goanalysis,surfaceinputs,dependency_surface}.go`
  vs the others). Types-down (C5): every type `cmd/arcc/app`/`cmd/arcc` construct or
  read fields of is declared in an IF file — move declarations, do not alias.
- **12.7** Declarations: `BUILD.bazel` `interface_files = ["goanalysis.go",
  "surfaceinputs.go", "dependency_surface.go"]`; textproto identical. Deps = previous −
  `report` (Step 3) + `hostfs` (Step 10) + `schema-gen` (Step 8). `declared_authority`
  `[]`.
- No inventory changes (no new packages). `x_tools_ownership_test.go` unchanged.

**Tests.** All 18 files stay in the directory. Internal tests that referenced moved
unexported helpers keep compiling (same package).

**Integration.** Full gate; gate item 4 across `cmd/arcc/app`, `stdlibmap`, `cmd/arcc`.
Probe `goanalysis` → empty. `wc -l` of the three IF files ≤ ~450 non-test LOC combined.

**Demo.** `go doc ./internal/goanalysis` lists ≈ 13 symbols; Bazel report shows
`interface_files` = three files and empty `declared_authority`.

---

## Step 13: `packagelayout` — file splits, `fs.FS` passing, two interface files

**Objective.** Convert the widest-authority component to declared style with
`[REFLECT, UNSAFE_POINTER]` only (§5.4). Intra-package, plus signature changes for
in-tree consumers. `BuildTags` and `hostenv` already exist (Step 11).

**Guidance.** Sub-steps in order; `go test ./internal/packagelayout/...` green between
them.

- **13.1 Split `packagelayout.go`** (2,853 LOC) by cluster into `layout.go` (schema types
  + `Parse` + JSON codec + `IsMemberOnlyLayout`/`IsStdlibPackage` + the `BuildTags` type
  from Step 11's `buildtags.go`), `api.go` (exported entry points listed in §5.4 IF),
  `validate.go` + `scanner.go`, `stdlibdiscover.go`, `driver.go`, `active.go`
  (active-layout globals/accessors/`CheckMu` — may be folded into `api.go`). Pure motion;
  `packagelayout_test.go` (2.9k) split the same way.
- **13.2 `importdata`:** `ordinary_import_data.go` + `stdlib_export_data.go` lose their
  `os` calls; descriptors parse/validate/merge over `[]byte`/`io.Reader`. `api.go`
  exposes `MergeImportGraphLayout(base, graph []byte) ([]byte, error)` and
  `WriteImportGraph(config []byte) ([]byte, error)`; callers do I/O. Update
  `cmd/arcc/app` (reads via `os.ReadFile`, writes via `atomicfile`).
- **13.3 `stdlibdiscover.go`** over `fs.FS` (`fs.WalkDir/ReadDir/ReadFile/Stat` — free,
  S3); `StdlibLayout(sdk fs.FS, platform *Platform)`; callers pass `os.DirFS(sdkRoot)`
  (`stdlibmap` from its existing code now, from `hosttoolchain` in Step 15).
- **13.4 `validate.go`/`scanner.go`** take an `fs.FS` rooted at the workspace instead of
  `os.ReadFile` at (old) `:1575, 1914, 2009`; `osStat`/`osOpen` seams → `fstest.MapFS`.
- **13.5 `driver.go`:** `RunDriver(layout []byte, workspaceDir string, patterns []string,
  stdin io.Reader, stdout io.Writer) error`; `handleDriverRequest`,
  `clonePackageForDriver`, `resolveQueryPattern` unexported.
- **13.6 Remove the last `go/build`/`runtime` uses:** any remaining `build.Default` or
  `runtime.*` reference in `packagelayout` (e.g. `nativeBuildContext`) is replaced by
  `hostenv.NativeTargetDefaults()` fields; the two imports disappear from the package.
- **13.7 Declarations:** `BUILD.bazel` `interface_files = ["layout.go", "api.go"]`,
  `members = [":packagelayout"]`, `component_deps = [x-tools, hostenv]`,
  `declared_authority = ["REFLECT", "UNSAFE_POINTER"]` with the C9 comment adapted
  ("layout JSON codec and import-data descriptors are this component's own schema").
  Textproto identical; `PACKAGE_SURFACE` gone. Unexport everything in NIF files not used
  outside the package (consumers use 21 symbols — all must be declared in the two IF
  files; `OrdinaryImportData`/`StdlibExportData` stay exported in `layout.go` because
  `app` tests construct them).
- **13.8** Method rule and types-down check as in 12.6 for `layout.go`/`api.go`.
- Inventory: `purePackages` unchanged (component still declares REFLECT);
  `x_tools_ownership_test.go` unchanged.

**Tests.** Split along file boundaries; `fstest.MapFS` replaces OS fakes;
`docs/package-layout-schema.md` examples re-run against `Parse`.

**Integration.** Full gate. Probe `packagelayout` → exactly `REFLECT` + `UNSAFE_POINTER`
(sites in `layout.go` and the import-data files). Probe `hostenv`, `layout-driver-hook`
→ their declarations. Gate item 4 across `goanalysis`, `stdlibmap`, `app`, `checker`.

**Demo.** `grep -rn '"os"\|go/build"\|"runtime"' go/internal/packagelayout --include=*.go | grep -v _test | grep -v internal/hostenv`
prints nothing.

---

## Step 14: `cli` — declared style with `app/app.go` as the interface

**Objective.** Make the composition root follow C2/C11 with a ~250-LOC interface file and
unexported subcommand implementations (§5.5). Native-only (F2). Intra-package.

**Guidance.**
- `go/cmd/arcc/component.textproto`: remove `interface_style`; `interface_files:
  "app/app.go"`; `members: "./"`, `"./app"`. **First** verify the native loader accepts an
  interface file in a member subdirectory (`just selfcheck` on this component). If it
  rejects it, move the manifest to `go/cmd/arcc/app/component.textproto` with members
  `./`, `../`, and update `justfile:147` + `scripts/selfcheck-staging*.sh`. Record which.
- `app/app.go` keeps `Runner`, `Run`, seam types, usage, dispatch. In `check.go`,
  `artifactshape.go`, `layoutshape.go`, `stdlibmap.go`, `verdict.go` (+ the report
  reader from Step 5), `package_imports.go`: unexport entry points called only from
  `app.go` (`runCheck`, …). Method rule for `Runner` and the seam types.
- `declared_authority`: probe the whole component and declare exactly the result
  (expected `FILES`, `READ_SYSTEM_STATE`, `REFLECT`, `UNSAFE_POINTER`,
  `OPERATING_SYSTEM`, `MODIFY_SYSTEM_STATE`). Deps: drop `x-tools` (Step 3); add
  `atomicfile`, `reportio`, `layout-driver-hook`, `schema-gen`; gate item 4 prunes the
  rest.

**Tests.** In place; add one test per subcommand that an injected failing seam yields
the documented exit code if not already present.

**Integration.** Full gate; native leg authoritative: `(cd go && go test ./cmd/...)`,
`just selfcheck`.

**Demo.** `wc -l go/cmd/arcc/app/app.go` ≤ ~300; the staged `cli` report lists
`interface_files: app/app.go`.

---

## Step 15: `stdlibmap` — `hosttoolchain` sub-component + two interface files

**Objective.** Confine `go env`/`go list`/cache/env interactions to one 120-line
sub-component and declare the 20-symbol API as the interface (§5.10). Native-only.

**Guidance.**
- **`internal/hosttoolchain`** (component `stdlibmap-hosttoolchain`):
  ```go
  func GoEnv(ctx context.Context) (map[string]string, error)
  func GoListStd(ctx context.Context, env []string) ([]string, error)
  func GoVersion(ctx context.Context) (string, error)
  func UserCacheDir() (string, error)
  func Environ() []string
  func CacheFS(root string) fs.FS
  func SDKFS(root string) fs.FS
  func OpenCache(path string) (io.ReadCloser, error)
  ```
  `declared_authority = ["EXEC", "READ_SYSTEM_STATE", "FILES"]` — probe and trim. Rewire
  `cache.go`, `command.go`, `explicit.go`, `layoutloader.go`, `nativeloader.go`,
  `project_export.go`, `stdlibmap.go`; writes through `atomicfile`; `StdlibLayout` gets
  `SDKFS(root)`.
- **JSON out:** `layoutloader.go` synthetic layout → `packagelayout.(Layout).MarshalJSON`;
  `project_export.go` descriptor decode → `packagelayout` import-data codec.
- **Interface files:** `stdlibmap.go` + `sdkkey.go` hold the ≈ 20 consumer-facing
  symbols (design §5.10 list) — move declarations into them as needed; all other files
  NIF with the 31 internal-only symbols unexported (Step 3 did most). Method rule for
  `TargetConfig`, `GenerationDescriptor`, `GenerationInput`, etc.
- Textproto: `interface_files: "stdlibmap.go", "sdkkey.go"`; `members: "./"`;
  `declared_authority` empty; deps += `stdlibmap-hosttoolchain`, `atomicfile`,
  `capslocksymbol`, `schema-gen`.
- Appendix A: 4 (stage `internal/stdlibmap/internal/hosttoolchain` before
  `internal/stdlibmap`; counts), 6 (`wantOwner`), 11.

**Tests.** In place; `hosttoolchain` tests use the real toolchain (already required by
`just selfcheck`); map-generation tests (`justfile:80-81`) remain the engine coverage.

**Integration.** Full gate (native authoritative). Probe `stdlibmap` → empty; probe
`hosttoolchain` → its declaration.

**Demo.** `grep -rn 'os/exec\|os\.Environ\|os\.UserCacheDir\|os\.DirFS' go/internal/stdlibmap --include=*.go | grep -v _test`
lists only `internal/hosttoolchain/`.

---

## Step 16: Final audit, README counts, conventions cross-check

**Objective.** Prove every component satisfies C1–C12 and leave documentation consistent
with the tree (G4, G5).

**Guidance.**
- Probe **every** manifest in the `justfile` list; table (component → declared →
  measured) into `../research/final-audit.md`. Any mismatch is fixed in its step.
- Interface-size table: IF LOC vs total, exported-used vs exported; mark C3(a)/(b).
- `grep -rn PACKAGE_SURFACE go/ --include=BUILD.bazel --include=component.textproto` →
  `protobufruntime`, `xtools`, `capslockruntime` only.
- `grep -rn '"os"\|os/exec\|"runtime"\|go/build"' go/internal go/cmd --include=*.go |
  grep -v _test | grep -v '/internal/host\|layoutdriverhook\|atomicfile\|cmd/arcc'` →
  empty.
- `grep -rn 'interface = ' go/ --include=BUILD.bazel` → only `schema/gen` (whole-library
  sugar) and single-file vocabulary components, if left as is.
- README counts (`:818, :851, :859`), component lists, limitation 12; `justfile:63-97`
  coverage comment rewritten (no expected-fail analyses remain); `.agents/summary/
  components.md` if present. Recompute counts from the justfile (design says 24 Bazel /
  29 native / 3 asserted).
- `../follow-ups.md`: proto report, purity items, `fixture_test.go` if deferred.

**Tests.** None new.

**Integration.** Full gate after `bazel shutdown`, and `just ci` on a fresh clone of the
working copy.

**Demo.** `final-audit.md` shows zero declared-vs-measured mismatches and every
component tagged C3(a) or C3(b).

---

## Appendix — per-step inventory touch list (design Appendix A)

| Step | New components | `justfile` stage | parity args | `wantOwner` | `purePackages` | foreign prefixes | `cli` dep | README |
|------|----------------|------------------|-------------|-------------|----------------|------------------|-----------|--------|
| 2 | – (rule change) | – | equality on | – | – | – | – | attr doc |
| 4 | `atomicfile` | yes | yes | yes | no (FILES) | – | yes | counts |
| 5 | `reportio` | yes | yes | yes | `artifactio` added | – | yes | counts |
| 6 | – | – | – | – | – | – | – | – |
| 7 | `capslocksymbol` | yes | yes | yes | yes | – | – | counts |
| 8 | `schema-gen` | yes (fail removed) | yes | yes | – | – | yes | counts |
| 9 | `capslock-runtime` | asserted | maybe | – | – | **yes** | – | counts |
| 10 | `goanalysis-hostfs` | yes (before parent) | yes | yes | no | – | – | counts |
| 11 | `packagelayout-hostenv`, `layout-driver-hook` | yes ×2 | yes ×2 | yes | no | – | yes (hook) | counts |
| 12 | – | – | – | – | – | – | – | – |
| 13 | – | – | – | – | – | – | – | – |
| 14 | – (manifest may move) | path if moved | – | – | – | – | n/a | – |
| 15 | `stdlibmap-hosttoolchain` | yes (before parent) | – (native) | yes | – | – | – | counts |
