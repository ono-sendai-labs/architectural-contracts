# Implementation plan — compositional component analysis

**Date:** 2026-08-04 · **Revised:** 2026-09-02 (post design review); 2026-09-07 (Step 4:
layout-driver map generation and cgo scoping, design I5); 2026-09-08 (Step 5: asserted
surfaces are package-level, design I6; Step 6/7: foreign-member wrappers and the residual
`UNANALYZED` decision); 2026-09-09 (Step 6: bounded routine stdlib-map generation and CI
feedback time, design N5)
**Design:** [`../design/detailed-design.md`](../design/detailed-design.md)
**Decision record:** [`../idea-honing.md`](../idea-honing.md),
[`../2026-09-02-design-review-response.md`](../2026-09-02-design-review-response.md)
**Baseline:** `dev-exp-go-bazel-mvp` @ `5011b726`

Sequencing follows idea-honing **Q17**: the monorepo PoC will not re-import until this
work lands, so the plan optimises for internal simplicity rather than incremental host
benefit. The load-deduplication step stays dropped and `absorbed_dependencies` removal
stays first.

Three ordering constraints drive the middle of the plan, two of them new since the
review:

1. **Schemas and the symbol grammar come before anything that persists or compares
   symbols** (DR-04, DR-15). Steps 3 precedes 4–7.
2. **The stdlib map must be a declared Bazel artifact before the check depends on it**
   (DR-07): Bazel sandboxes have no `go` binary and cannot share a native cache, and
   `just ci` runs the Bazel checks. Step 4 delivers generator, port, Bazel rule and
   native cache together.
3. **The reference scan lands before export-data loading** (unchanged reasoning, one
   more reason): the implements-closure workaround compensates for VTA, so surfaces
   must not replace the symbol source until VTA is gone; and check-time Capslock needs
   closure syntax, so closure sources cannot leave the runfiles until Capslock leaves
   the check path. Steps 6 → 7 → 8.

Core end-to-end functionality — a check with no call graph — lands at **Step 6**.
Member-only inputs (N1/N2) land at **Step 8**.

**Release constraint (DR-19.6).** Between Step 2 and Step 11 no revision satisfies I1.
None of them is tagged, released, or imported; the series is consumed at its end.

---

## Progress checklist

- [x] **Step 1** — Baseline measurement and low-risk groundwork
- [x] **Step 2** — Remove `absorbed_dependencies` and pattern membership
- [x] **Step 3** — Persisted schemas, symbol grammar, and the authority lattice
- [x] **Step 4** — Standard-library authority map: generator, port, Bazel artifact, native cache
- [x] **Step 5** — Build topology: analysis action, providers, surface emission, CLI outputs
- [ ] **Step 6** — Reference scan replaces the call graph *(keystone)*
- [ ] **Step 7** — Surface consumption, status axes, overlap, namespace
- [ ] **Step 8** — Export-data type loading and member-only inputs
- [ ] **Step 9** — Golden restructure: verdicts vs layout shape
- [ ] **Step 10** — `UnusedAuthority`
- [ ] **Step 11** — `authority: UNKNOWN`
- [ ] **Step 12** — Host adapter hooks
- [ ] **Step 13** — Cross-cutting acceptance: scaling, hermeticity, determinism

---

## Step 1: Baseline measurement and low-risk groundwork

**Objective.** Establish where the time actually goes before restructuring anything, and
land three orthogonal fixes that reduce churn during the rest of the work.

**Guidance.**
- Profile `arcc check` on a large component, attributing wall/CPU between (a)
  `packages.Load` and type-checking of member packages, (b) type-checking of the
  closure, (c) SSA construction, (d) VTA, (e) Capslock's second load and analysis. The
  design eliminates (b)–(e) and keeps (a); record findings in
  `../research/current-analysis-pipeline.md` under "measured attribution".
- Fix friction report §1: `aspect.bzl:28-29` returns `[]` for non-Go targets; return
  `[ArccPackageInfo(packages = depset())]`.
- Apply friction report §6(2): normalise both sides through `hostpolicy.CanonicalizePath`
  before diffing in the golden comparison helper.
- Make `discoverStdlibWithContext` (`packagelayout.go:191`) fail loudly when
  `os.Stat(sdkRoot)` fails instead of inviting a filesystem-walk fallback (moved up from
  the old Step 11; this is the third fix).

**Tests.**
- A golden whose expected file is written in a rewritten path namespace compares equal
  after normalisation.
- An aspect test asserting a non-Go dependency yields an empty `ArccPackageInfo`.
- A layout with a missing SDK root fails with an actionable error.

**Integration.** Nothing structural changes; `just ci` stays green.

**Demo.** A cost-attribution note showing which phase dominates on a real component.

---

## Step 2: Remove `absorbed_dependencies` and pattern membership

**Objective.** Delete both concepts and everything that exists to support them (R9, Q4,
Q7, DR-16). Pure deletion.

**Guidance.**
- Remove proto field 4 and `AbsorbedDependency`; add `reserved 4; reserved
  "absorbed_dependencies";`. Remove `Manifest.AbsorbedDependencies` and its
  parse/validation path.
- Remove `checker.go:100-112`, `:189-205`, the unused-absorbed warning, and
  `ABSORBED_FUNC_VALUE_ESCAPE`.
- Remove `facts.FuncValueEscapes`, `facts.BodilessAbsorbedPackages`,
  `goanalysis.scanFuncValueEscapes`, `collectBodilessAbsorbedPackages`, and
  `capanalyzer.AnalyzeRequest.PruneAtPackages` with its classifier branch.
- Remove pattern membership: `isPatternMembership`/`isAnyPatternMember`
  (`goanalysis.go:1748-1762`), `resolvePatternMembershipDependencyInterface`
  (`:1792-1879`), `facts.MatchesMember`, and the direct-load rejection at `:64-80`,
  which becomes a parse-time rejection of glob metacharacters in `members`.
- Rewrite the README walkthrough (`README.md:296-340`) to teach `UNDECLARED_AUTHORITY`
  directly: a component calling `os.ReadFile` without declaring FILES.
- Update self-components, `manifestparity`, and the Bazel `absorbed` attribute.
- Verify no example or self-component manifest uses pattern members before deleting;
  record the result in the change description.

**Tests.** Existing absorbed and pattern fixtures are deleted, not weakened. A manifest
still carrying `absorbed_dependencies` or a glob member is rejected with an actionable
error.

**Integration.** Self-check and `manifestparity` stay green.

**Demo.** `just ci` green with both concepts gone; the README walkthrough produces the
same verdict by a simpler route; `arcc` rejects a stale manifest.

---

## Step 3: Persisted schemas, symbol grammar, and the authority lattice

**Objective.** Define every artifact and identifier that crosses a process or action
boundary before anything produces one (DR-04, DR-06, DR-15). Data and pure code only;
the check path is untouched.

**Guidance.**
- `proto/archcontracts/v1/surface.proto` and `stdlibmap.proto` per the design's Data
  Models: `format_version` first, sorted repeated entries, no `map` fields. Extend
  `just gen` and `gen-is-clean`.
- `component.proto`: add `authority` (enum `DECLARED`/`UNKNOWN`, default `DECLARED`);
  remove fields 8 and 9 with `reserved` numbers and names. Parse rejects `UNKNOWN`
  with non-empty `declared_authority`.
- Core-side `SymbolID` type with the versioned textual grammar, a normaliser from
  Capslock names, and a `go/types`-object-to-`SymbolID` function implementing the
  declaring-object rule (aliases, promoted members, generic origins, pointer/value
  receivers).
- `AuthorityDeclaration` with `Join`; parse and serialise forbid the empty-set spelling of
  `UNKNOWN`.
- Canonical JSON encoder/decoder (`protojson` + compact/indent), SHA-256 digest helper,
  atomic write helper, size caps.
- `hostpolicy.NamespaceID` and `IsCanonicalPath` (friction report §7).

**Tests.**
- Table-driven `SymbolID` tests for every object kind in the design's grammar table,
  including alias expansion, embedding/promotion, generics, and duplicate observations.
- `Join` lattice tests: `UNKNOWN` absorbs; `∅ ⊔ ∅ = ∅`; parse rejects the forbidden
  spelling; round-trip through JSON preserves `UNKNOWN`.
- Encoding: two marshals of the same message are byte-identical; a message with
  reordered repeated entries canonicalises to the same bytes; unknown major version is
  rejected; unknown fields are ignored; oversize input is rejected.
- Atomic write: a simulated crash mid-write leaves the previous file intact.

**Integration.** Additive; nothing consumes the new types yet.

**Demo.** `just gen-is-clean` green with the new schemas; a unit-level walkthrough of the
grammar table.

---

## Step 4: Standard-library authority map — generator, port, Bazel artifact, native cache

**Objective.** Produce the map as a total, keyed, fail-closed artifact available in both
modes before anything depends on it (R5, R6, N3, Q13, DR-05, DR-07, DR-09).

**Guidance.**
- `StdlibAuthority` port core-side per the design (fail-closed `SymbolAuthority`,
  `PackageInitAuthority`, `Evidence`, `Key`).
- `stdlibmap` shell package: inventory with `go/types` (every exported object, every
  exported method of exported named/alias types, `init`); Capslock
  `GranularityFunction` in an isolated run for each importable package, grouped by
  `Path[0]` within that package. Step 6 proved that one whole-stdlib program conflates
  unrelated packages under VTA (10,399 findings instead of the isolated 8,182), so the
  isolation is a correctness property despite its roughly 146-second generation cost;
  a **generation classifier** that keeps the minting-site reclassification
  but does **not** wrap with `ClassifierExcludingUnanalyzed` (I4); var rule with the
  handle minting-authority clause; consts/types/interface-specs `SAFE`; `init` keyed on
  the aggregate; `unsafe.*` hardcoded; curated-safe provenance per entry. Generation
  fails if any inventoried symbol lacks a classification.
- `SDKKey` from the layout `platform` block plus toolchain version and GOEXPERIMENT;
  `classifier_hash` over the generation classifier text and rule version.
- Oracle: `go list std` natively; toolchain package list in Bazel. Enumerate `internal/…`
  with `importable: false`; a listed package whose files are all excluded by the target's
  build constraints is `importable: false`, any other oracle/loader discrepancy fails.
- Loading (design I5): natively through `go/packages`' `go list` driver; in Bazel through
  `packagelayout`'s `GOPACKAGESDRIVER` self-exec driver over a whole-stdlib layout the
  generator computes from the declared SDK sources — the mechanism the Bazel check
  already uses for its closure. No Bazel action executes a toolchain binary. Extend
  `packagelayout` first: `goexperiment` on `platform`; release tags from the toolchain
  version and tool tags from GOEXPERIMENT in `BuildContextForLayout` instead of
  `build.Default`; the target GOARCH in the driver response.
- Bazel: `arcc_stdlib_map` rule taking SDK sources (via the existing `go_sdk_srcs`
  seam), the toolchain's package list and the target configuration — no toolchain
  binary — producing the map for the target configuration; it fails at analysis time,
  naming the target, for a cgo-enabled target configuration (design §Out of scope); wire
  it as the default `_stdlib_map` attr of the analysis action (Step 5). Native: on-demand
  generate-and-cache under the user cache dir keyed by `SDKKey`, with atomic writes and
  corrupt-cache regeneration.
- `arcc stdlibmap generate|inspect` subcommands.
- Carry the FR10 Component Contract block including the Ambient Authority line.

**Tests.**
- `os.ReadFile` → FILES with non-empty evidence; `strings.TrimSpace` → `SAFE`;
  `sort.Slice` → `UNANALYZED` (the spike's laundering case); `os.Stdin` → FILES;
  `io.EOF` → `SAFE`; `http.DefaultClient` → NETWORK; `unsafe.Pointer` → `UNANALYZED`;
  an init-bearing fixture has an `inits` entry.
- Totality: every `go list std` package is enumerated; every inventoried symbol of
  every importable package has an entry; deleting one entry from a map makes lookup
  return the inventory-gap error.
- Determinism: two generations are byte-identical.
- Keying: two target configurations under one host (cgo on vs off at the key level, and
  in Bazel a cross-compile to another GOOS plus a build-tag variant) produce distinct keys
  and maps; a cgo-enabled target configuration fails `arcc_stdlib_map` analysis with an
  error naming the target; lookup with a mismatched `classifier_hash` or format version
  fails closed.
- Bazel: the map builds in a sandbox with no network and no `go` binary at all; an
  analysis test asserts the action's inputs contain no toolchain binary or build cache.

**Integration.** Nothing consumes the port yet.

**Demo.** Generate for the local toolchain and query it; build the Bazel artifact; show
the laundering case classified `UNANALYZED` rather than absent.

---

## Step 5: Build topology — analysis action, providers, surface emission, CLI outputs

**Objective.** Give surfaces a producer and consumers a provider edge before the
analysis model changes (R7, R8, DR-01, DR-06, DR-19.3). Emission only; nothing reads a
surface yet.

**Guidance.**
- CLI: `--report-out`, `--surface-out`, `--report-verdict-only` (exit 0 when analysis
  ran; verdict in the report). Surface derived from manifest, layout and member facts
  via the Step 3 extractor; symbols are the exact declared interface with no
  implements-closure injection. *(Resolved in Step 6: the workaround is deleted, and
  check and surface share the same exact declaring-object interface.)*
- `go_component`: add the analysis action with declared inputs per N2 (export data and
  dependency surfaces are wired in Steps 7–8; until then the action's inputs are today's
  runfiles). Outputs `<name>.report.json`, `<name>.surface.json`; `OutputGroupInfo(arcc)`;
  `ArccComponentInfo` gains `surface`, `report`, `provenance`.
- `manual`-tagged components (transitional stand-in for `UNKNOWN`) get an analysis-time
  `ctx.actions.write` of an asserted surface and no action; `provenance = "asserted"`. Per
  design **I6** that surface is package-level: `Packages` from the layout, namespace, SDK
  key, format and producer version, no symbols and the empty digest. A declared-interface
  component cannot be asserted and fails at analysis time naming the target, so the one
  genuine declared-interface `manual` fixture migrates to `PACKAGE_SURFACE`. This
  asserted branch lands with the asserted-surface producer, together with the audit of every
  current `manual` tag and the migration of the fixture-only uses; until it does, such
  components stay on the checked path. The analysis action and the asserted branch are
  deliberately separate pieces of work within this step.
- `check.bzl`: the three test rules assert over the report artifact (`arcc verdict
  --expect pass|fail`, grep, golden) instead of running `arcc check` themselves.
- Native dependency-surface location by convention (`<manifest>.surface.json`).

**Tests.**
- Byte-identical surface across repeated runs of an unchanged component.
- A declared-interface component's surface contains exactly its interface symbols; a
  `PACKAGE_SURFACE` component's contains its member packages and no symbols; both carry
  namespace, SDK key, format version.
- Digest test enumerates inputs: changes when a member source or the manifest changes;
  unchanged when a dependency's source, a test file, or an unrelated file changes.
- Bazel analysis test: `bazel test` of a dependent's `.check` builds the dependency's
  analysis action; `bazel build //...` without the output group builds none.
- `expect_violation` and golden tests pass via the report artifact.

**Integration.** Check behaviour unchanged; only where it runs and what it writes.

**Demo.** Build csvtool's components with `--output_groups=+arcc`; show surfaces and
reports; a `manual`-tagged component yields an asserted surface and no action.

---

## Step 6: Reference scan replaces the call graph *(keystone)*

**Objective.** Replace SSA/VTA and check-time Capslock with an AST + `types.Info` scan
over member packages (R1–R4, DR-10, DR-11). Dependency surfaces still come from the
existing source-loading `ResolveDependencyInterface` in this step.

**Guidance.**
- Implement `ScanReferences` producing `ReferenceEdge`s (Uses/Selections, deduplicated
  per site) and `ImportEdge`s (every import declaration).
- Classify per the design's two tables/diagrams, resolving stdlib membership and
  authority through the Step 4 port; `UNANALYZED` becomes an `AnalysisDefeating`
  finding under the existing policy model (violation by default).
- Apply the declaring-object rule against the dependency's symbol set; delete the
  implements-closure computation (`goanalysis.go:1619-1714`) in the same change.
- Delete `goanalysis.go:274-279` (SSA/VTA), `:281-326` (edge extraction), the five
  stdlib predicates, and Capslock from the check path (`app.go:200-229`, the
  `capslockadapter` check entry point; the generation entry point stays).
- Detect `//go:linkname`, assembly files, and cgo in member packages and emit an
  `AnalysisDefeating` finding.
- Findings per `(capability, class)` with sorted sites and map evidence (DR-17).
- Load mode is unchanged in this step (`NeedDeps` stays until Step 8).
- Restore bounded feedback after the correctness-mandated per-package map generation:
  check in the canonical map for the exact pinned Linux/amd64, cgo-disabled toolchain as
  a native test artifact; validate its complete SDK key against the current classifier;
  remove whole-SDK regeneration from routine Go integration tests and selfcheck; run the
  Bazel map action through a generation-only binary; and move replica/cross-configuration
  actions to an explicit full lane. Routine Bazel wildcard build/test retains one real
  default map generation and compares it byte-for-byte with the checked artifact.
- Keep production native on-demand generation and the hermetic Bazel artifact contract
  unchanged. Generator optimisations and a multi-architecture CI matrix remain separate
  work; the immediate supported routine-CI envelope is Linux/amd64.

**Tests.** Design fixtures 1–6 and 9, in full, **before** the cutover commit is
described: the reference-kind table and the import table are written against the new
scanner and must pass with the workaround already deleted. Plus: an `AnalysisDefeating`
fixture is a violation by default and a warning with policy; a finding with three sites
renders one line with a count in text and three sites in JSON. The checked map decodes and
matches every field of the key derived from the pinned target and current classifier;
routine Go integration performs no whole-SDK generation; Bazel wildcard build/test contains
exactly one **default-configuration** `ArccStdlibMap` action, with additional non-default
actions permitted where analysis-time-failure fixtures that must stay in the routine lane pull
the map in under a transition; the displaced replica, tagged, cross-platform and
transitioned-surface checks remain runnable through the explicit full lane.

**Integration.** The analysis model changes here. Expect golden churn; Step 9 cleans up
structure, but goldens must be *correct* here. On the pinned Linux/amd64 reference
configuration, a routine `just ci` with a warm Bazel cache completes within five minutes
wall clock (N5); a cold routine run performs no native whole-SDK generation and at most one
Bazel whole-SDK generation.

**Demo.** `arcc check` end to end with no call graph and no Capslock, on csvtool and
arcc's own components; wall-clock delta against Step 1; the two FR5 fixtures showing the
precision change; the surface emitted in Step 5 now agrees with the check. Show before/after
stdlib-map action counts and a timed warm-cache `just ci` within the N5 bound.

---

## Step 7: Surface consumption, status axes, overlap, namespace

**Objective.** Resolve dependency surfaces from artifacts instead of loading dependency
source, with structural provenance (R7, R14, Q16, DR-03, DR-06, DR-08, DR-12).

**Guidance.**
- `ResolveDependencyInterface` reads the dependency's surface and, when present, report.
  Delete the `./...` source load (`goanalysis.go:1441-1470`).
- After surface-based resolution is operational, introduce one in-tree
  `protobuf-runtime` `PACKAGE_SURFACE` component owning the explicitly enumerated
  protobuf package closure. Migrate `schema`, `manifest`, `artifactio`, and other
  self-hosting consumers away from declaring those packages as their own members before
  enabling the overlap checks below. This ordering removes the native resolver limitation
  that prevents a foreign-member wrapper from working during Steps 5 and 6.
- Do the same for `x/tools`: one `PACKAGE_SURFACE` wrapper owning the retained `x/tools`
  packages, with `goanalysis` migrated off declaring them as its own members. Step 6's
  cutover exempted `goanalysis` from the gate for exactly these members (`sort.Slice`,
  `unsafe.Pointer` inside `x/tools` source), so this removes the exemption rather than
  merely tidying ownership.
- **Decide the residual `UNANALYZED` question here**, once the foreign members above are
  gone and the remaining findings are the honest ones. The open cases are
  `csv.Reader.ReadAll` in `examples/csvtool`'s `parsecsv`, `UNANALYZED` through
  interface-parameter indirection, and `internal/capslockadapter`'s native-only selfcheck of
  Capslock's own member source, which currently contains 1042 `UNANALYZED` sites not owned
  by either foreign-member wrapper. The two candidate mechanisms, both deferred from Step 6:
  (a) a recorded, I2-consistent *capability-use indirection* curation in the generation
  classifier — interface-method and func-parameter invocation wrappers such as
  `(sync.Once).Do`, `(sync.Pool).Get` and `csv.(Reader).ReadAll` count as capability use,
  since the indirect target's own edges are visible as member references; or (b) the
  user-facing policy carrier DR-11 already presumes ("a violation unless policy allows or
  warns") — the checker half exists and is tested (`Warn[""]` downgrades to
  `ANALYSIS_LIMITATION`), but no manifest field or CLI flag sets it. Step 6's cutover
  exempted `parsecsv` and the native `capslockadapter` selfcheck pending this decision.
  Step 7 must remove both exemptions: capability-use curation is sufficient only if tests
  prove that no residual sites remain; otherwise the policy carrier (or another explicit
  design correction) must consciously account for the remainder.
- Keep dependencies explicit for hand-authored protobuf imports. Step 12 may auto-attach
  the same runtime component for generated or host-injected imports; auto-attachment
  changes edge provenance, not runtime ownership or the component's surface.
- Bazel: the analysis action takes direct dependencies' `surface` and `report` from
  `ArccComponentInfo` (declared and auto-attached) as inputs; the layout carries their
  paths. Provenance derived: `CHECKED_PASS` iff a report is present with verdict pass;
  `CHECKED_FAIL` → `DEPENDENCY_CHECK_FAILED` warning; asserted provider → `ASSERTED`.
- Native: surfaces by convention; provenance `ASSERTED`; freshness by hashing readable
  dependency source bytes without parsing, else `UNKNOWN`.
- Verify `FormatVersion`, `Namespace`, `SDKKey` before use; each mismatch its own tool
  error.
- `DEPENDENCY_OVERLAP` before building any package-to-dependency map; the checker's
  last-wins maps (`checker.go:65-70`, `213-227`) become insert-or-error.
- `report.DependencyBoundary` carries the three axes; text summary words per the design.
- Remove `OwnCheckRuns`/`CertificationReference` from facts, report and `defs.bzl:124`.

**Tests.**
- Parity: consuming a surface produces the same verdicts as Step 6's source path on
  every existing fixture.
- Failed dependency → `CHECKED_FAIL` + warning, never `certified`.
- Native: `VERIFIED` when unchanged, `STALE` after touching a dependency source,
  `UNKNOWN` when the dependency root is unreadable — and in all three cases no
  dependency file is parsed (assert via a load hook or file-open counter).
- Namespace: two distinct idempotent canonicalisers; each rejects the other's surface.
- SDK key or format mismatch → exit 2. Missing surface → exit 2.
- Two direct dependencies claiming one package → `DEPENDENCY_OVERLAP`.

**Integration.** After this step no check *parses* a dependency's source. Closure
sources are still staged in Bazel until Step 8.

**Demo.** Check `examples/csvtool/app` with its dependencies' sources made unreadable —
it succeeds natively with freshness `UNKNOWN`; show `certified`, `stale`, and
`check failed` renderings.

---

## Step 8: Export-data type loading and member-only inputs

**Objective.** Complete N1/N2: members from source, everything else from compiled export
data, no dependency source in the action (DR-02, DR-18).

**Guidance.**
- Layout schema: per-package `export_file` for every non-member package (deps and
  stdlib); keep the full transitive import graph (`goexperiment` on `platform` landed in
  Step 4).
  Validate closure completeness and export-file presence before `packages.Load`,
  returning a tool error rather than letting `go/packages` panic.
- Driver: return `GoFiles`/`CompiledGoFiles` only for members; `ExportFile` for all
  others.
- Loader: drop `NeedDeps` (`goanalysis.go:104-109`); assert dependency `Syntax` is nil
  post-load as a guard.
- Aspect: collect `GoArchive.data.export_file` per package (verify the field name
  against the pinned rules_go 0.61.1); stdlib export data from the toolchain's
  `GoStdLib` via a new adapter seam (Step 12 formalises it; land the upstream default
  here). Remove closure sources from `go_component` runfiles (`component.bzl:385-409`)
  and from the analysis action's inputs; keep member sources only.
- Native: obtain export data through `go list -export` for the closure; document that
  native mode may shell out to the toolchain here.
- Record non-member input count and bytes in the report's diagnostics section.

**Tests.**
- Bazel analysis test: the dependent's action inputs contain member `.go` files, export
  files, surfaces, reports and the map, and no dependency `.go` file.
- Hermetic integration test: dependency sources absent, export data present → check
  succeeds with correct `Uses`/`Selections` for the reference-kind table.
- A layout missing one transitive node or export file fails with a tool error naming it.
- Export data produced by a mismatched toolchain fails at load with a tool error.

**Integration.** Runfiles shrink; `just ci` and the self-check must stay green in both
modes.

**Demo.** The dependent's action inputs listed; a deep synthetic closure checked with no
dependency source present; load time versus Step 6.

---

## Step 9: Golden restructure — verdicts vs layout shape

**Objective.** Split host-independent verdict assertions from host-dependent layout-shape
assertions, now that layouts have reached their final shape (friction report §6(1)).

**Guidance.**
- Separate each affected golden into a verdict golden and, where genuinely needed, a
  layout-shape golden. Most tests assert only verdicts.
- Surfaces, reports and maps are golden-able; add their shape coverage here.

**Tests.** Verdict goldens contain no absolute paths, no closure package lists, and no
host-specific import prefixes.

**Integration.** With Step 1's normalisation, this is what makes the next import cheap.

**Demo.** Count of host-patchable goldens before and after; verdict goldens compare equal
under a simulated rewritten-prefix namespace.

---

## Step 10: `UnusedAuthority`

**Objective.** Report authority declared but never exercised (R12, Q15).

**Guidance.** Diff exercised authority (from the scan) against `DeclaredAuthority`
(`checker.go:311` only ever widens). Warning, matching `UNUSED_DEPENDENCY`'s shape. Audit
arcc's own components.

**Tests.** Declaring FILES without touching the filesystem warns; exact declarations do
not; an `AnalysisDefeating` finding does not mask a genuinely unused declaration.

**Integration.** Pure addition to `checker.Check`.

**Demo.** Over-declarations found in arcc's own components, or a clean result plus a
deliberately over-declaring fixture.

---

## Step 11: `authority: UNKNOWN`

**Objective.** Land the verification-status axis so unowned code can be adopted without
being falsely certified, and retire the transitional `manual` path (R10, R11, Q7–Q9,
DR-13).

**Guidance.**
- The schema field and lattice type exist since Step 3; this step wires them: the Bazel
  attribute selects the asserted-surface path (replacing the `manual` stand-in from
  Step 5 and the host's package-name string match); the consumer renders `untrusted`.
- `declared_authority` must be empty with `UNKNOWN` (already rejected at parse).
- Document I1 as enforced-half plus governance assumption; the Q7 predicate remains
  deferred.

**Tests.** An `UNKNOWN` surface round-trips without becoming empty; a dependent renders
`untrusted` and its JSON shows `authority: UNKNOWN`; an `UNKNOWN` component has no
analysis action; the report never shows `certified` for such a boundary.

**Integration.** Completes the tool-enforced half of I1. The release constraint lifts
here.

**Demo.** A `PACKAGE_SURFACE` wrapper over an unowned library with `authority: UNKNOWN`;
the dependent passes with the boundary rendered `untrusted`.

---

## Step 12: Host adapter hooks

**Objective.** Land the additive adapter hooks that take the host's remaining production
patches to zero, shaped for the post-redesign architecture (friction report §3, §4, §5;
Q14; DR-14).

**Guidance.**
- `runtime_injection_attrs(deps_aspect)` and `extra_runtime_packages(ctx, root_packages)`,
  empty upstream; injected packages become visible to the existing infra
  auto-attachment.
- Three SDK seams with upstream defaults: source enumeration on `arcc_stdlib_map` only;
  export-data enumeration on the analysis action (formalising Step 8's default); target
  platform and key discovery (toolchain version, GOEXPERIMENT).
- Derive infra self-exemption from the component's own label rather than a package-name
  literal (§5).

**Tests.** With empty defaults behaviour is identical. A fake adapter injecting a
synthetic runtime package produces a component whose layout includes it and whose check
passes without a declaration. A fake adapter supplying an alternative export-data set is
honoured by the analysis action's inputs.

**Integration.** Additive and inert upstream.

**Demo.** A test host adapter injecting a synthetic runtime; the component type-checks,
auto-attaches the infra component, and passes.

---

## Step 13: Cross-cutting acceptance — scaling, hermeticity, determinism

**Objective.** Turn N1, N4 and the design's acceptance matrix rows not owned by an
earlier step into regression tests (DR-18).

**Guidance.**
- Synthetic scaling benchmark: fixed member source, dependency depth 1/4/16 and width
  variants; record package-load, scan and total time and non-member input count; assert
  no dependency parse/SSA/Capslock by construction (counters) and that scan time is flat.
- A second benchmark with growing directly imported type surface, documenting the
  residual export-data cost.
- Hermeticity: run the Bazel suite with `--sandbox_writable_path` restrictions and no
  host `go` on `PATH`; assert no undeclared cache directory is touched.
- Determinism: repeated full builds produce identical map, surface and report bytes.
- Record the numbers in `../research/current-analysis-pipeline.md` next to Step 1's
  baseline.

**Tests.** The benchmarks and hermeticity run are `-tags=integration` tests wired into
`just ci`.

**Integration.** Closes the plan; the next-host-import checklist in
[`../research/host-import-friction.md`](../research/host-import-friction.md) is walked
once here.

**Demo.** Baseline versus final on the same component; the scaling table.

---

## Notes on what is deliberately absent

- **No load-deduplication step.** Superseded by Steps 6–8 (Q17).
- **No friction-report §2 step.** The five stdlib predicates delete in Step 6 (Q12, Q17).
- **No transitive/tree-predicate step and no `UNKNOWN` approval predicate.** Deferred
  (Q10, Q7, DR-13); I1's governance half is documented, not enforced.
- **No bootstrap-CLI step.** Likely unnecessary given Step 11 (Q7).
- **No dual VTA/typed-edge path.** The reference table is pinned against the new scanner
  inside Step 6 instead (review's proposed Step 5, not adopted).
- **No multi-architecture full-CI matrix yet.** Step 6 keeps the existing cross-target map
  assertions in an explicit full lane and supports routine CI on Linux/amd64. A follow-up
  must run that lane across execution and target architecture combinations.
