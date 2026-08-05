# Implementation plan — compositional component analysis

**Date:** 2026-08-04
**Design:** [`../design/detailed-design.md`](../design/detailed-design.md)
**Decision record:** [`../idea-honing.md`](../idea-honing.md)
**Baseline:** `dev-exp-go-bazel-mvp` @ `5011b726`

Sequencing follows idea-honing **Q17**: the monorepo PoC will not re-import until this
work lands, so the plan optimises for internal simplicity rather than incremental host
benefit. Two consequences are already baked in — the load-deduplication step is dropped
as superseded, and `absorbed_dependencies` removal moves from last to first.

One ordering constraint is not obvious and drives steps 4–6: the implements-closure
workaround (`goanalysis.go:1620-1730`) exists to compensate for VTA. Replacing the
dependency symbol source with a surface manifest **before** removing VTA would strip
those concrete methods while dynamic resolution was still producing edges to them,
breaking FR5. So the reference scan lands first, the workaround dies with it, and only
then does the symbol source swap to a manifest.

Core end-to-end functionality — a check that runs with no call graph — is available at
**Step 4**. Full composability (N1/N2: cost independent of the closure) lands at
**Step 6**.

---

## Progress checklist

- [ ] **Step 1** — Baseline measurement and low-risk groundwork
- [ ] **Step 2** — Remove `absorbed_dependencies`
- [ ] **Step 3** — Standard-library authority map: generator and lookup port
- [ ] **Step 4** — Reference scan replaces the call graph *(keystone)*
- [ ] **Step 5** — Surface manifest emission
- [ ] **Step 6** — Surface manifest consumption and boundary vocabulary
- [ ] **Step 7** — Golden restructure: verdicts vs layout shape
- [ ] **Step 8** — Map distribution, keying, and fail-closed behaviour
- [ ] **Step 9** — `UnusedAuthority`
- [ ] **Step 10** — `authority: UNKNOWN`
- [ ] **Step 11** — Host adapter hooks

---

## Step 1: Baseline measurement and low-risk groundwork

**Objective.** Establish where the time actually goes before restructuring anything, and
land the three orthogonal fixes that reduce churn during the rest of the work.

**Guidance.**
- Profile `arcc check` on a large component, attributing wall/CPU between (a)
  `packages.Load` and type-checking of member packages, (b) SSA construction over the
  closure, (c) VTA, (d) Capslock's second load and analysis. The design eliminates
  (b)–(d) and keeps (a); this attribution determines the real size of the win and may
  reorder later steps. Record findings in
  `../research/current-analysis-pipeline.md` under a new "measured attribution" section.
- Fix friction report §1: `aspect.bzl:28-29` returns `[]` for non-Go targets; return
  `[ArccPackageInfo(packages = depset())]` so consumers need no `in target` guard.
- Apply friction report §6(2): normalise both sides through
  `hostpolicy.CanonicalizePath` before diffing in the golden comparison helper.

**Tests.**
- A golden test whose expected file is written in a rewritten path namespace compares
  equal after normalisation.
- An aspect test asserting a non-Go dependency yields an empty `ArccPackageInfo` rather
  than none.

**Integration.** Nothing structural changes; `just ci` must stay green.

**Demo.** A cost-attribution note showing which phase dominates on a real component;
goldens that previously required host patches now compare equal under normalisation; the
aspect no longer forces defensive guards.

---

## Step 2: Remove `absorbed_dependencies`

**Objective.** Delete the concept and everything that exists to support it, so every
later step has fewer cases to handle. Pure deletion (R9, Q4).

**Guidance.**
- Remove the schema field and its parse/validation path; remove
  `Manifest.AbsorbedDependencies`.
- Remove `checker.go:100-112` (absorbed branch of the import sweep), `:189-205`
  (absorbed overlap), and the unused-absorbed-dependency warning.
- Remove `facts.FuncValueEscapes`, `facts.BodilessAbsorbedPackages`,
  `goanalysis.scanFuncValueEscapes`, `collectBodilessAbsorbedPackages`, and
  `capanalyzer.AnalyzeRequest.PruneAtPackages` with its classifier branch.
- Rewrite the README walkthrough (`README.md:296-340`): it currently teaches
  `UNDECLARED_AUTHORITY` via absorption. Teach the same lesson directly — a component
  that calls `os.ReadFile` without declaring FILES.
- Update self-components, `manifestparity`, and the Bazel `absorbed` attribute.

**Tests.** Existing absorbed fixtures (`goanalysis/testdata/escapes/`) are deleted, not
weakened. Add a check that a manifest still carrying `absorbed_dependencies` is rejected
with an actionable error rather than silently ignored.

**Integration.** Self-check and `manifestparity` must stay green; they are the signal
that the deletion is complete rather than partial.

**Demo.** `just ci` green with the concept gone; the README walkthrough produces the
same `UNDECLARED_AUTHORITY` verdict by a simpler route; `arcc` rejects a stale manifest
that still declares absorbed dependencies.

---

## Step 3: Standard-library authority map — generator and lookup port

**Objective.** Produce the map and the interface the scan will consume, backed by an
on-demand cache so distribution can wait (R5, R6, Q13).

**Guidance.**
- Define the `StdlibAuthority` port core-side and the data model per the design's Data
  Models section: total package enumeration, sparse symbol table, per-package init
  entries, evidence paths, and `SDKKey` including `classifier_hash`.
- Implement generation in a new `stdlibmap` shell package: run Capslock over the SDK with
  every exported symbol as a root, reusing the existing classifier construction so the
  `fileHandleUseMethods` reclassification (I2) is identical to today's.
- Back lookup with an on-demand generate-and-cache implementation keyed by `SDKKey`. No
  artifact distribution yet — that is Step 8.
- Carry the FR10 Component Contract doc block on the new package, including its Ambient
  Authority line.

**Tests.**
- `os.ReadFile` resolves to FILES with a non-empty evidence path; a pure symbol
  (`strings.TrimSpace`) resolves to empty authority; both are *in* the package
  enumeration.
- Totality: every package reported by the SDK enumeration is present in the map.
- Determinism: two generations over the same SDK and classifier are byte-identical.
- A package whose `init` exercises authority has an `inits` entry (needed by Step 4).

**Integration.** Nothing consumes the port yet; the check path is untouched.

**Demo.** Generate a map for the local toolchain and query it: `os.ReadFile` → FILES with
evidence, `strings.TrimSpace` → empty, `net/http` present in the enumeration, and a
regenerated map identical byte-for-byte.

---

## Step 4: Reference scan replaces the call graph *(keystone)*

**Objective.** Replace SSA/VTA and check-time Capslock with an AST + `types.Info` scan
over member packages, treating references and imports as edges (R1–R4).

**Guidance.**
- Implement `ScanReferences` over `types.Info.Uses` / `Selections` plus each member
  package's imports. Every reference to a symbol outside the member set is an edge; every
  import is an edge.
- Classify per the design's edge-classification diagram, resolving stdlib membership and
  authority through the Step 3 port. Dependency surfaces still come from the existing
  `ResolveDependencyInterface` in this step — swapping that is Step 6.
- Delete `goanalysis.go:274-279` (SSA/VTA), `:281-326` (edge extraction), the
  implements-closure computation at `:1620-1730`, and the five stdlib predicates
  (`packagelayout.go:79`, `goanalysis.go:533,550,563`, and `hostpolicy.IsStdlibPath`'s
  classification role). Remove `capslockadapter` from the check path, retaining it for
  Step 3's generation.
- Detect `//go:linkname`, assembly, and cgo in member code and emit `AnalysisLimitation`.

**Tests.** The behaviour-pinning fixtures from the design's Testing Strategy:
1. `f := os.ReadFile` never called → attributed FILES.
2. Import-only init authority → attributed.
3. `dep.Greeter.Greet()` with unexported implementation → **passes** (it passes today for
   the wrong reason; it must still pass once the workaround is gone).
4. Direct concrete-method call on a method implementing a declared interface → **fails**.
   This is the deliberate behaviour change; today it wrongly passes.

**Integration.** This is where the analysis model actually changes. Expect golden churn;
Step 7 cleans up the structure, but goldens must be *correct* here, not deferred.

**Demo.** `arcc check` runs end to end with no call graph and no Capslock in the check
path, on the csvtool example and on arcc's own components. Show the wall-clock delta
against the Step 1 baseline, and the two FR5 fixtures demonstrating the precision change.

---

## Step 5: Surface manifest emission

**Objective.** Every check emits a deterministic surface manifest describing its public
surface, with provenance (R7, R8, Q16).

**Guidance.**
- Define `SurfaceManifest` and `Provenance` per the design. Declared-interface components
  enumerate symbols; `PACKAGE_SURFACE` components record member packages and leave symbols
  empty.
- Emit as an output of the check action, with `ProducedByCheck`, an `InputHash` over the
  component's sources and manifest, and the canonical `Namespace` the symbols are written
  in.
- Deterministic serialisation: sorted symbols and packages, stable keys.
- Now that Step 4 has made edges syntactic, the emitted symbol set is exactly the declared
  interface — no implements-closure injection.

**Tests.**
- Byte-identical output across repeated runs of an unchanged component.
- A declared-interface component's manifest contains exactly its interface symbols; a
  `PACKAGE_SURFACE` component's contains its member packages and no symbols.
- `InputHash` changes when a member source changes and not otherwise.

**Integration.** Emission only — nothing reads these yet, so the check's behaviour is
unchanged.

**Demo.** Check each csvtool component and show the emitted manifests; modify a member
source and show the input hash move; re-run unchanged and show byte-identical output.

---

## Step 6: Surface manifest consumption and boundary vocabulary

**Objective.** Resolve dependency surfaces from manifests instead of loading dependency
source, completing N1/N2 (R7, Q16).

**Guidance.**
- Rewrite `ResolveDependencyInterface` to read a dependency's surface manifest. Delete the
  `./...` source load at `goanalysis.go:1440-1470`.
- Implement the four-way boundary vocabulary: `certified` (produced by a check action),
  `asserted` (present, provenance unverifiable), `stale` (input hash mismatch),
  `untrusted` (reserved for Step 10).
- Enforce namespace agreement: a manifest written in a foreign namespace is a tool error,
  not a best-effort comparison.
- Add `hostpolicy.IsCanonicalPath` (friction report §7) as the override-once predicate that
  makes idempotent canonicalisation expressible.

**Tests.**
- A check consuming a dependency manifest produces the same verdicts as the Step 4
  source-loading path (regression parity).
- A manifest with a mismatched input hash yields `stale` plus a warning.
- A manifest in a foreign namespace is rejected with exit 2.
- A missing manifest is a tool error, not a silent pass.

**Integration.** After this step no check reads a transitive dependency's source. Verify
by measuring a component whose dependency tree is deep and showing cost no longer tracks
the closure.

**Demo.** Check `examples/csvtool/app` with its dependencies' sources deliberately made
unreadable — it still succeeds, reading only manifests. Show the report's dependency
section rendering `certified`, and `stale` after touching a dependency without re-checking.

---

## Step 7: Golden restructure — verdicts vs layout shape

**Objective.** Split host-independent verdict assertions from host-dependent layout-shape
assertions, now that layouts have reached their final shape (friction report §6(1)).

**Guidance.**
- Separate each affected golden into a verdict golden ("component X reports
  `UNDECLARED_AUTHORITY` FILES at `foo.go:20`") and, where genuinely needed, a
  layout-shape golden.
- Most tests should assert only verdicts. Confine layout-shape assertions to the small
  number of tests actually about layout.
- Surface manifests are now a golden-able artifact; add coverage for their shape here
  rather than scattering it through earlier steps.

**Tests.** The restructure *is* test work, but it is not a testing-only step: it changes
what the suite asserts and what a host must patch. Verify by confirming the verdict
goldens contain no absolute paths, no closure package lists, and no host-specific import
prefixes.

**Integration.** Combined with Step 1's normalisation, this is what makes the next import
cheap.

**Demo.** Count of host-patchable golden files before and after; the verdict goldens
compare equal under a simulated rewritten-prefix namespace.

---

## Step 8: Map distribution, keying, and fail-closed behaviour

**Objective.** Turn the on-demand cache from Step 3 into a pinned, keyed, verifiable
artifact (N3, Q13).

**Guidance.**
- Key maps on `(go_version, GOOS, GOARCH, build_tags, classifier_hash)` and discover the
  active toolchain's key at check time.
- Fail closed: no map for the active key, or a `classifier_hash` mismatch, exits 2 with an
  actionable message. Never fall back to a near-miss map.
- Wire generation as a Bazel artifact of the pinned SDK, keeping the on-demand path for
  native mode.
- Express SDK enumeration as an explicit host seam — a total package set, not a path
  heuristic (the point of Q12).

**Tests.**
- A check against a toolchain with no map exits 2 without analysing.
- A map whose `classifier_hash` disagrees with the running classifier is rejected.
- Two hosts with different `GOOS` resolve to different maps.

**Integration.** The lookup port is unchanged from Step 3, so only its backing swaps.

**Demo.** Check under the pinned SDK and succeed; point at a toolchain with no map and get
a clear exit-2 refusal; corrupt the classifier hash and get a distinct refusal.

---

## Step 9: `UnusedAuthority`

**Objective.** Report authority declared but never exercised (R12, Q15).

**Guidance.**
- The scan already yields exercised authority per component; diff it against
  `DeclaredAuthority`, which today only ever widens the policy (`checker.go:311`).
- Report as a warning, matching the existing `UnusedDependency` shape.
- Audit arcc's own components and tighten any declarations this surfaces — the self-check
  is the first consumer.

**Tests.** A component declaring FILES without touching the filesystem warns; a component
declaring exactly what it uses does not; an `AnalysisDefeating` finding does not mask a
genuinely unused declaration.

**Integration.** Pure addition to `checker.Check`; no new inputs.

**Demo.** Run against arcc's own components and show the over-declarations it finds (or a
clean result plus a deliberately over-declaring fixture).

---

## Step 10: `authority: UNKNOWN`

**Objective.** Add the verification-status axis so unowned code can be adopted without
being falsely certified (R10, R11, Q7–Q9).

**Guidance.**
- Add `authority` to the schema as a separate axis from `interface_style`:
  `DECLARED` (default) or `UNKNOWN`. Do **not** add a third `interface_style`.
- Represent as `AuthorityDeclaration{Known bool; Set []Capability}`. `UNKNOWN` must not be
  expressible as an empty set (R11); any join with an unknown operand is unknown.
- An `UNKNOWN` component runs no check action — it contributes a surface manifest and
  nothing else — and renders `untrusted` at every boundary that rests on it.
- Replaces the host's `manual`-tagging patch (friction report §5), so the tagging
  behaviour should become an attribute rather than a package-name string match.

**Tests.**
- A component depending on an `UNKNOWN` component does not compute as having empty
  authority in any bound (design fixture 5).
- `UNKNOWN` round-trips through the manifest and the surface manifest without becoming
  empty.
- The report renders `untrusted` for such a boundary.

**Integration.** Completes invariant I1: every component is checked, or explicitly
unanalysed and visible as such.

**Demo.** Declare a `PACKAGE_SURFACE` wrapper over an unowned library with
`authority: UNKNOWN`, depend on it, and show the check passing with the boundary rendered
`untrusted` — and the dependent's authority bound refusing to read as pure.

---

## Step 11: Host adapter hooks

**Objective.** Land the two additive adapter hooks that take the host's remaining
production patches to zero (friction report §3, §4; Q14).

**Guidance.**
- `runtime_injection_attrs(deps_aspect)` and `extra_runtime_packages(ctx, root_packages)`,
  both returning empty upstream. Parameterise by the aspect to avoid the load cycle.
  Injected packages become visible to the **existing** infra auto-attachment
  (`component.bzl:217-230`), which `checker.go:289-291` already exempts from the
  unused-dependency warning — no new classification concept (Q14).
- `GO_SDK_SRCS_ATTRS` merged into the three check rules' attribute dicts, returning empty
  upstream.
- Make `discoverStdlibWithContext` fail loudly when `os.Stat(sdkRoot)` fails rather than
  inviting a filesystem-walk fallback.
- Address friction report §5's self-exemption: derive infra self-exemption from the
  component's own label rather than requiring a package-name literal.

**Tests.** With empty defaults, behaviour is identical to before (the regression bar). A
fake adapter returning a synthetic injected package produces a component whose layout
includes it and whose check passes without the author declaring it.

**Integration.** Additive and inert upstream, so no existing host is affected.

**Demo.** A test host adapter injecting a synthetic runtime: the component type-checks,
auto-attaches the infra component, and passes with no declaration written by the author.

---

## Notes on what is deliberately absent

- **No load-deduplication step.** Superseded by Steps 4 and 6 (Q17).
- **No friction-report §2 step.** The five stdlib predicates delete in Step 4 rather than
  being consolidated first (Q12, Q17).
- **No transitive/tree-predicate step.** Deferred (Q10); the design records a barrier-marking
  hint for when it is picked up.
- **No bootstrap-CLI step.** Likely unnecessary given Step 10 (Q7).
