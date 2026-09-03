# Response to the 2026-09-02 design review

**Date:** 2026-09-02
**Review:** [`2026-09-02-design-review.md`](2026-09-02-design-review.md) (gpt-5.6-sol)
**Reviewed by:** Claude (Fable 5.1), with code fact-checking and two feasibility spikes
delegated to Sonnet subagents.
**Disposition:** The review is substantially correct. Of nineteen findings, sixteen are
valid as stated, two are valid with corrections to their premises (DR-11, DR-14), and
one is partly misconstrued (DR-13). No finding is wholly misconstrued. Both feasibility
spikes the review asked for were run; both confirmed the concern and one (DR-02) showed
the situation is *worse* than the review claimed. The design and plan have been revised
accordingly; this document records the assessment and the decisions taken.

Every code claim in the review was checked against the working copy (`cca66212` plus
this change). Where the review's line numbers drifted, the response cites current lines.

---

## 1. Evidence gathered

Two read-only fact-checks and two spikes were run before assessing anything.

### 1.1 Fact-check results that matter

| Claim in review | Verified? | Note |
| --- | --- | --- |
| `.check` is a test rule producing only a launcher; no downstream-consumable output | **Yes** | `check.bzl:66-90`, `rule(test = True)`; no `OutputGroupInfo` |
| Layout JSON carries source paths only, no export data; driver never sets `ExportFile` | **Yes** | `component.bzl:121-150`, `packagelayout.go:932-995`; zero hits for `export_file`/`GoArchiveData` in `bazel_rules/` |
| Closure sources are staged in check runfiles | **Yes** | `component.bzl:385-409` → `DefaultInfo.runfiles` → `check.bzl:85-88` |
| `LoadPackageFacts` requests `NeedDeps|NeedSyntax|NeedTypes|NeedTypesInfo` for roots and deps alike | **Yes** | `goanalysis.go:104-109`; same literal at `1441-1446`, `1460-1465` |
| `extractSymbols` omits interface method specs and struct fields | **Yes** | `goanalysis.go:594-712` visits only top-level `FuncDecl`/`GenDecl` |
| `InterfaceSymbol` contract is call-oriented (pointer+value method forms, generic brackets stripped) | **Yes** | `capanalyzer.go:38-47` |
| Package→dependency maps are last-assignment-wins | **Yes** | `checker.go:65-70`, `213-227` |
| Analysis-defeating findings are violations by default | **Yes, with nuance** | `capanalyzer.go:71-118`: `StrictPolicy` is empty, so *every* capability is a violation by default; the `ANALYSIS_LIMITATION` warning kind appears only when policy has `Warn` for that capability (`checker.go:313-344`) |
| `GO_SDK_SRCS_ATTRS` exists and is used by check rules | **No** | It does not exist anywhere in the repo. Stdlib sources enter via `go_sdk_srcs(ctx)`/`go_sdk_root(ctx)` (`go_adapter.bzl:249-267`). `GO_SDK_SRCS_ATTRS` is a *proposed* hook from friction report §4. The review's point about the plan still stands |
| Manifest message is `ComponentManifest` | **No** | It is `Component` (`component.proto`); encoding is textproto (`manifest.go:131-139`); no `reserved` statements exist |
| Pattern membership exists and is dependency-side only | **Yes** | Direct load rejects patterns (`goanalysis.go:64-80`); resolution matches globs against the *depender's* closure (`1792-1879`); the Bazel path never emits patterns |
| `own_check_runs`/`certification_reference` are unverified self-declarations | **Yes** | `facts.go:110-114`, `report.go:54-55`, `defs.bzl:124` |
| Capslock adapter runs a second, independent `packages.Load` | **Yes** | `capslockadapter.go:164-168` |

### 1.2 Spike: member-only syntax with export-data type loading

Note: [`research/spike-export-data-loading.md`](research/spike-export-data-loading.md).

- **Works.** `NeedName|NeedFiles|NeedCompiledGoFiles|NeedImports|NeedTypes|NeedSyntax|NeedTypesInfo`
  (no `NeedDeps`) type-checks only the root from source and yields complete
  `Uses`/`Selections` for interface methods, fields, vars, consts, func values and stdlib
  references. Dependency `.go` files were physically deleted and the load still succeeded.
- **Worse than the review said.** `go/packages` requires a real `ExportFile` for **every
  package in the transitive closure** (58/58 in the fixture, not the 3 directly imported),
  because a missing export file sets `needsrc` on that node and `needsrc` bubbles upward
  (`packages.go:802-811`, `872-877`). And the driver's `Imports` graph must be
  transitively complete: a missing node causes `log.Panicf` (`packages.go:1571`), not an
  error.
- Build-cache `.a` archives with `__.PKGDEF` are accepted as `ExportFile` verbatim.
  `$GOROOT/pkg` ships no precompiled stdlib; export data must be produced by the build.

### 1.3 Spike: total per-symbol stdlib map generation

Note: [`research/spike-stdlib-map-generation.md`](research/spike-stdlib-map-generation.md).

- **Per-root attribution works.** `GranularityFunction` yields one entry per
  `(capability, root)` with `Path[0]` the root; the generator groups by `Path[0].Name`.
  Name format matches the adapter's existing key convention.
- **"Absent" conflates three things:** not-a-root (const/type/interface spec), curated
  `CAPABILITY_SAFE` (`os.Exit`, 26 `runtime.*`), and analyzed-pure. All must be
  explicit entries.
- **Critical:** the adapter's `ClassifierExcludingUnanalyzed` silently turns 1078
  `UNANALYZED` stdlib roots (`sort.Slice`, `io.Copy`, `errors.As`, …) into zero-entry
  "pure" results. Reusing the check-time classifier for generation, as plan Step 3
  said, would ship exactly the fail-open bug the review predicted.
- Vars: "union of the dereferenced type's method authority under the map's own
  classifier" works for `net/http.Default*`; for `os.Stdin` it yields `CHDIR` only, so
  pre-minted handles need an explicit minting-authority rule (§2, DR-05).
- `init` is reported as a synthesized aggregate `pkg.init`; key on it alone.
- `unsafe.*` are `*types.Builtin`, never SSA functions; must be hardcoded.
- Cost: full non-internal `std` (178 pkgs) in ~2.2 s wall, ~1.35 GB RSS. `go list std`
  = 360 packages; trivially regenerable per SDK.

---

## 2. Finding-by-finding assessment

Verdict key: **Valid** — accepted as stated; **Valid, corrected** — accepted, premise
adjusted; **Partly misconstrued** — the discrepancy is real but the recommended remedy
misreads a recorded decision.

### DR-01 — No producer/consumer topology for surfaces — **Valid (Blocking)**

The `.check` target is a test rule with no declared outputs; R8 and Steps 5–6 have no
mechanism. The "certified means produced by a *successful* check" point is also right.

**Decision.** `go_component` gains an ordinary **analysis action** running
`arcc check --report-out --surface-out` that always exits 0 when analysis *ran*
(violations are recorded in the report; tool errors still fail the action). It produces
`<name>.report.json` and `<name>.surface.json`, carried on `ArccComponentInfo` and an
`arcc` output group. The `.check` test becomes an assertion over the report artifact
(verdict == pass, or == fail for `expect_violation`). Consumers receive the dependency's
surface *and* report through the provider; provenance is `CHECKED_PASS` only when the
report verdict is pass, otherwise `CHECKED_FAIL` (warning `DEPENDENCY_CHECK_FAILED`, not
a violation — the dependency's own test already fails). Trust derives from the provider
edge, not from a boolean in the file. `authority: UNKNOWN` components get an
analysis-time `ctx.actions.write` of an **asserted** surface (packages are known from the
layout) and no analysis action. Native mode: `--surface-out` on the CLI; dependency
surfaces are located by convention next to the dependency manifest; the authored manifest
never names a surface path. This differs from the review's "fail the action on
violation" suggestion: keeping the action green keeps `bazel build` semantics unchanged
and avoids a duplicate action for negative/golden tests, while still making provenance
structural. Applied: design §Build topology; plan Step 5.

### DR-02 — Surfaces do not supply type data — **Valid (Blocking); spike confirms and strengthens**

The design's own Appendix A said "export data rather than source" but nothing in the plan
made it happen, and the layout has no export data at all. The spike shows the residual
cost is larger than the review estimated: export data for the **whole closure** is a
required action input. In Bazel those are existing compile outputs (`GoArchive.data.export_file`)
so the *work* eliminated is parsing, type-checking and SSA over the closure; the *inputs*
still scale with it.

**Decision.** N1/N2 are rewritten as measurable statements: a check parses, type-checks
and scans only member source; dependency and stdlib types come from compiled export data;
no SSA, VTA, or Capslock at check time; the count and size of non-member inputs is
measured. Layout schema gains per-package `export_file` and the driver drops `GoFiles` for
non-members; the loader drops `NeedDeps`; the layout is validated for transitive closure
before loading (to convert the `go/packages` panic into a tool error). Applied: design
§Type loading, Appendix A; plan Step 8.

### DR-03 — Native staleness contradicts source-independence — **Valid (Blocking)**

Q16's "write an input hash so native mode can detect staleness" cannot be honoured
without reading dependency sources.

**Decision.** Bazel freshness is structural (the action graph); the digest in a surface is
audit-only. Native mode consumes existing surfaces as `ASSERTED`; freshness is
best-effort: if the dependency's sources are readable, native mode hashes the file bytes
(no parse) and reports `STALE` on mismatch; if unreadable, freshness is `UNKNOWN`. The
Step 6 demo of "unreadable sources" is kept but now demonstrates `UNKNOWN` freshness
rather than a contradiction. The digest binds member source bytes, manifest bytes, format
version, namespace ID, SDK key and producer version. Applied: design §Provenance,
freshness and authority; plan Step 7.

### DR-04 — Reference grammar undefined — **Valid (Blocking)**

`extractSymbols` records no interface method specs or fields; the design's fixture 3
cannot pass once the workaround is deleted without a new rule.

**Decision.** FR5 expands to all externally declared objects, with a **declaring-object
rule**: a reference is authorised by the top-level declaration that owns the resolved
object. Interface method specs and struct fields are authorised by their declaring type
being listed; declared methods (`FuncDecl`) are separate entries; promoted members
resolve via `Selection.Obj()` to their declaring type; generic instantiations resolve to
their origin; an alias entry authorises the alias name and expands to its target's
members when the target is in the same package. One versioned textual symbol grammar is
used by surfaces and the stdlib map (`pkg.F`, `pkg.T`, `pkg.V`, `(pkg.T).M`, `pkg.init`;
receiver written without `*` or type arguments). Table-driven fixtures cover every kind.
Applied: design §Reference semantics; plan Step 3.

### DR-05 — Stdlib map fails open — **Valid (Blocking); spike confirms**

Absent-means-pure was unsound in principle and the spike found a concrete instance
(the adapter's classifier launders 1078 `UNANALYZED` roots into silence).

**Decision.** Total inventory per package: every exported package-level object, every
exported method of every exported type, and `init`, each with a terminal classification
(`SAFE`, capabilities, or `UNANALYZED`). Lookup of an inventoried package's unknown symbol
is a tool error. Generation uses a classifier that *preserves* `UNANALYZED` (unlike the
check-time adapter). Rules: consts/types → `SAFE`; interface method specs → `SAFE`
(capability use, per I2); vars → union of the dereferenced type's method authority, plus
the **minting authority** of the handle when the type owns a reclassified use-method
(so `os.Stdin` is FILES, not merely CHDIR); `unsafe.*` hardcoded `AnalysisDefeating`;
`init` keyed on the aggregate `pkg.init`. Evidence keyed by `(symbol, capability)`.
Enumeration oracle: `go list std` for the target toolchain natively; the toolchain's
package list in Bazel; totality test compares the two. Applied: design §Stdlib map;
plan Step 4.

### DR-06 — `UNKNOWN` semantics contradictory — **Valid (Blocking)**

R8 says all surfaces are check outputs; Step 10 says `UNKNOWN` runs no check yet emits a
surface; R11's "poisons any bound" names a computation nothing performs; the four-way
vocabulary mixes three axes.

**Decision.** Three orthogonal axes, all present in JSON, summarised in text:
provenance `CHECKED_PASS | CHECKED_FAIL | ASSERTED`; freshness
`BUILD_GRAPH | VERIFIED | STALE | UNKNOWN`; authority `DECLARED{set} | UNKNOWN`. Asserted
surfaces come from a distinct producer (DR-01). R11 is restated as a representational
constraint on `AuthorityDeclaration` and its join, unit-tested; the only consumer in this
design is the report annotation, and no bound is computed (Q10 stands). `own_check_runs`
and `certification_reference` are removed from the manifest (fields reserved): they were
the self-declarations Q16 set out to replace. `UNKNOWN` is allowed with either interface
style; `declared_authority` must be empty when authority is unknown. Applied: design
§Provenance, freshness and authority; plan Steps 5, 11.

### DR-07 — Map consumed before a hermetic artifact exists — **Valid (High)**

Bazel sandboxes have no `go` binary and cannot share an undeclared cache; `just ci` runs
Bazel checks, so Steps 4–7 would have been broken or non-hermetic.

**Decision.** Generator, lookup port, Bazel artifact rule and native cache land together
in one step before the cutover (old Steps 3 and 8 merged). Applied: plan Step 4.

### DR-08 — Namespace has no identity — **Valid (High)**

A fixed-point predicate does not name a canonicaliser.

**Decision.** `hostpolicy.NamespaceID` (string, default `"upstream"`), set once by the
host alongside `CanonicalizePath`; persisted in every surface; mismatch is a tool error.
Stdlib paths are never canonicalised (the map is namespace-free); `CanonicalizePath` must
be the identity on map packages. Applied: design §Canonical namespace; plan Step 7.

### DR-09 — `SDKKey` incomplete — **Valid (High)**

**Decision.** Key = (toolchain version incl. patch, GOOS, GOARCH, cgo_enabled, sorted
build tags, GOEXPERIMENT, classifier hash, map format version), describing the **target**
configuration used to compile member code. The layout's existing `platform` block
supplies most of it (`go_build_platform`, noting its `cgo = not mode.pure`
approximation); GOEXPERIMENT is added. Test replaced by two target configurations under
one host including a cross-compile. Applied: design §Stdlib map; plan Step 4.

### DR-10 — Import edges don't fit the state machine — **Valid (High)**

**Decision.** The review's six import rules adopted verbatim, including "unresolved import
of a declared/stdlib path with missing type data is a tool error, not
`UNDECLARED_DEPENDENCY`". Separate classification diagram for imports vs object
references. Applied: design §Edge classification; plan Step 6.

### DR-11 — Analysis-defeating severity contradictory — **Valid, corrected (High)**

The error table row was wrong. Correction to the review's premise: there is no built-in
default that downgrades `AnalysisDefeating`; with `StrictPolicy` every finding is a
violation, and `ANALYSIS_LIMITATION` is the report kind used when policy explicitly warns.
The fail-closed choice the review recommends is therefore the *existing* behaviour.

**Decision.** Member-code `linkname`/asm/cgo emit an `AnalysisDefeating` capability
finding, a violation unless policy allows or warns. Map-generation gaps are `UNANALYZED`
entries, not component-local warnings. Applied: design §Error handling.

### DR-12 — Direct dependency overlap order-dependent — **Valid (High)**

Confirmed last-wins maps. Q11 deferred *dep-vs-dep divergence*; the review correctly
notes that a *direct* overlap already needs a unique answer, which is local and cheap.

**Decision.** Two direct dependency surfaces (declared or auto-attached) claiming one
package is a tool error `DEPENDENCY_OVERLAP`, raised before any lookup map is built.
Tree-level divergence remains deferred. Applied: design §Edge classification; plan Step 7.

### DR-13 — I1's "approved" not implemented — **Partly misconstrued (High)**

The discrepancy is real: I1 says "marked unanalyzed **and approved**" while Step 10 claims
to complete I1 with marking only. But Q7 already records approval as a governance
predicate deferred with the tree predicates (Q10), so the plan does not silently drop a
requirement; the design text overstated what the tool enforces.

**Decision.** Take the review's first option: I1 is split into the tool-enforced half
(*every component is checked, or explicitly marked `UNKNOWN` and visibly `untrusted` at
every boundary that rests on it*) and an explicit external governance assumption
(approval of `UNKNOWN` components is a review/ownership process until the Q7 predicate
lands). No allowlist mechanism is added. Applied: design §Constraints; plan Step 11.

### DR-14 — SDK source hook contradicts the redesign — **Valid, corrected (Medium)**

Correction: `GO_SDK_SRCS_ATTRS` does not exist; it is friction report §4's proposal, and
today's check rules stage SDK sources through `go_sdk_srcs(ctx)`. The review's point
holds against the plan: after the export-data step, check rules need SDK **export data**,
and only map generation needs SDK **sources**.

**Decision.** Three adapter hooks: SDK source enumeration (map rule), SDK export-data
enumeration (analysis action), target-platform/key discovery. Applied: design §Host
adapter contract; plan Step 12.

### DR-15 — Persisted formats not designed — **Valid (High)**

**Decision.** Both artifacts are protobuf-defined (`surface.proto`, `stdlibmap.proto`
under `proto/archcontracts/v1/`), serialised as JSON via `protojson` then canonicalised
(compact → indent) so bytes are stable; no proto `map` fields (sorted repeated entries);
`format_version` first; unknown version or namespace or key mismatch is a tool error;
SHA-256 over canonical bytes; temp-file-and-rename writes; corrupt native cache is
discarded and regenerated, corrupt Bazel input is an action failure; `component.proto`
reserves 4, 8, 9 and the removed names. Applied: design §Persisted formats; plan Step 3.

### DR-16 — Surface packages underspecified; patterns — **Valid (High)**

Confirmed: pattern membership is glob-based, `PACKAGE_SURFACE`-only, resolved against
the *depender's* closure, rejected for direct checks, and never produced by Bazel. Under
I1 such a component can never be checked, and Q7 already rejected wildcard membership for
`PACKAGE_SURFACE` as hiding exactly the change worth reviewing.

**Decision.** Surfaces carry concrete package paths only. **Pattern membership is
removed** in the deletion step alongside `absorbed_dependencies`, consistent with Q7.
This is a scope addition the user should confirm; it is flagged in §4 below. Applied:
design §Surface manifest; plan Step 2.

### DR-17 — Evidence model unspecified — **Valid (Medium)**

**Decision.** One finding per `(capability, class)` per component carrying every member
source site sorted; the text report prints the first site and a count; JSON carries all
sites, the canonical referenced symbol, the map's evidence path for
`(symbol, capability)`, and the SDK key. Applied: design §Findings and evidence.

### DR-18 — Acceptance tests too weak — **Valid (Medium)**

**Decision.** The review's acceptance matrix (§7) is adopted; each row is assigned to a
plan step, and a final step runs the cross-cutting ones (scaling benchmark, hermeticity,
action-input inspection). Applied: plan Steps 8, 13.

### DR-19 — Smaller discrepancies — **Valid (Medium)**, all six

1. Step 1 now names three fixes: aspect provider, golden normalisation, and the
   SDK-root failure hardening moved up from old Step 11.
2. Digest test enumerates included and excluded inputs.
3. Surface lookup rule defined (DR-01).
4. "Report kinds unchanged" corrected: `UNUSED_AUTHORITY`, `DEPENDENCY_CHECK_FAILED`,
   `DEPENDENCY_OVERLAP` and the status axes are additions.
5. Corruption, concurrent-write and atomic-write tests added to Steps 3–4.
6. Absorption removal stays first (Q17), with an explicit release constraint: no
   intermediate revision satisfies I1 and none is released independently.

---

## 3. Review recommendations not adopted

- **Fail the analysis action on violation.** Rejected in favour of an always-green
  analysis action plus verdict-in-report (DR-01). The property the review wants — a
  violating component cannot be consumed as certified — holds because provenance is
  derived from the report the consumer receives via the provider.
- **Keep VTA alive alongside typed edges (review's proposed Step 5).** Rejected: two
  verdict paths double the maintenance surface. Instead the reference-semantics fixture
  table (DR-04) is written and passing against the new scanner *before* the cutover
  commit, in the same step.
- **Move absorption removal to the end (review's proposed Step 8).** Rejected; the Q17
  rationale stands (no consumer sees intermediate revisions). A release constraint is
  recorded instead.
- **Approval allowlist for `UNKNOWN`.** Not adopted (DR-13); I1 weakened instead.

## 4. Decisions the user should confirm

These were taken to keep the design implementable; each is reversible at the design
level and none has been implemented.

1. **Pattern membership is removed** (DR-16). Bazel never emits it and it cannot be
   checked under I1, but it is an existing native-mode feature.
2. **Analysis action stays green on violations**; the `.check` test asserts the verdict
   (DR-01). The alternative (action fails) makes `bazel build` of a dependent fail when a
   leaf violates.
3. **`own_check_runs` and `certification_reference` are removed** from the manifest
   (DR-06).
4. **I1's "approved" becomes an external governance assumption** (DR-13).
5. **Pre-minted stdlib handle vars inherit minting authority** (`os.Stdin` → FILES),
   going beyond the spike's method-union rule (DR-05).
6. **JSON-over-protobuf encoding** for surfaces and the map, canonicalised for byte
   stability (DR-15), rather than the manifest's textproto.

## 5. Where things landed

| Artifact | Change |
| --- | --- |
| `design/detailed-design.md` | Rewritten: measurable N1/N2, build topology, type loading, reference grammar, total map inventory, status axes, persisted formats, import rules, overlap, evidence, corrected error table, I1 split |
| `implementation/plan.md` | Restructured to 13 steps: schemas and grammar first, map with Bazel artifact before cutover, topology and emission before cutover, export-data loading after, acceptance matrix mapped to steps |
| `research/spike-export-data-loading.md` | New |
| `research/spike-stdlib-map-generation.md` | New |
| `idea-honing.md` | Pointer to this document added |
| `summary.md` | Status and plan shape updated |
