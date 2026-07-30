# Implementation plan — component membership and authority attribution

**Design:** `../design/detailed-design.md`
**Requirements:** `../idea-honing.md` (Q1–Q14)
**Research:** `../research/`

Requirement IDs (M*, A*, T*, B*) refer to §2 of the design.

## Progress checklist

- [x] **Step 1** — Declared membership: `members`, `interface_style` and the certification fields in the schema, honored end to end in native mode (M1, M3, M5, M8, M10)
- [x] **Step 2** — Attribution follows ownership: members are roots, `INIT_OUTSIDE_INTERFACE` removed (A1, A2, A3)
- [x] **Step 3** — The analysis platform becomes declared data, and layouts are held to it (T1, T2, T8, T8a)
- [x] **Step 4** — Interface-file constraint reporting and report wording (T3, T7)
- [x] **Step 5** — Classification and canonicalization made fail-closed (T4, T4a, T5, T6)
- [x] **Step 6** — Bazel: adapter seam additions, `members` and `interface_style` attributes (B1, emitter half of T2)
- [x] **Step 7** — Bazel: declared classification, emission, and example migration (M4, M6, M7)
- [x] **Step 8** — The absorbed func-value escape warning (A5)
- [x] **Step 9** — Package-surface components, auto-attached edges, pattern membership (A6, A7, A8, A9, M8)
- [x] **Step 10** — Bazelified self-check (B3)
- [x] **Step 11** — Documentation and design-record sync
- [x] **Step 12** — Remediation from implementation review 2026-07-29

**Core functionality milestones.** Owned-code attribution — the source note's
Part 1 fail-open — is closed and demoable in **native mode at Step 2**. The full
Bazel path (declared members, expanded manifests, `roots == members`) is
end-to-end at **Step 7**. Everything after that is the residual warning, infra
components, and dogfooding.

---

## Step 1: Declared membership in the schema, honored in native mode

**Objective.** A component can declare its members explicitly; when it does not,
FR1 behavior is bit-for-bit what it is today.

**Guidance.**
- Add `members` (repeated string), the `InterfaceStyle` enum with
  `interface_style`, and the two certification fields (`own_check_runs`,
  `certification_reference`) to `proto/archcontracts/v1/component.proto`, plus
  `auto_attached` on `ComponentDependency`, per design §4.1; regenerate with
  `just gen` and keep `just gen-is-clean` green. The certification fields' comments
  must say they are self-declarations at the same trust level as
  `declared_authority`, or the first reader assumes arcc verified them.
- `manifest`: parse and validate all three. Reject duplicate members, a member
  that is also an `absorbed_dependencies` import path (M7 contradiction), and
  malformed patterns. Under `interface_style = PACKAGE_SURFACE`, require non-empty
  `members` and **empty** `interface_files`; both directions are errors, so the
  surface has one source of truth. Accept unexpanded patterns in `members` for a
  `PACKAGE_SURFACE` component (M8); require literal paths for a declared-style one
  (M4).
- `packagelayout`: error when a declared member has no source files (M10). Checkable
  rather than merely stated — a bodiless package is visible on its face in the
  layout.
- `checker`: membership comes from `Manifest.Members` when non-empty, else FR1.
  Force the interface package into the member set (M5); a pattern that also
  matches it is not an error.
- `goanalysis`: when `members` is non-empty, load exactly those patterns instead
  of `./...` from the component root. This is what makes M2 (members anywhere)
  real in native mode rather than Bazel-only — without it a declared member
  outside the root would never be loaded.

**Tests.**
- `manifest`: members parsing, pattern forms, duplicate rejection, the
  absorbed-contradiction rejection; `PACKAGE_SURFACE` requiring members and
  rejecting non-empty `interface_files`, and the default style still requiring
  `interface_files`.
- `checker`: membership derived from `Members` when present; FR1 when absent;
  interface package always a member.
- `packagelayout`: a member with no source files errors (M10).
- `goanalysis`: a manifest whose members name a package outside the component
  root loads that package.

**Integration.** Purely additive. All eight self-manifests and both example
components omit `members`, so `just selfcheck` must stay green unchanged — that
is the primary regression signal for this step.

**Demo.** Take an existing component, add an explicit `members` list naming a
package outside its root, and show `arcc check` analyzing it. Remove the list and
show identical behavior to today. Show the three new validation errors.

---

## Step 2: Attribution follows ownership

**Objective.** Close the source note's Part 1 fail-open for a component's own
code: member packages are analysis roots, so their authority is charged no matter
who calls them.

**Guidance.**
- Roots derive from declared membership (Step 1) rather than from layout-mode
  filtering alone; `LoadPackageFacts` filters facts to members.
- **Remove `INIT_OUTSIDE_INTERFACE`** (A2): the `report.Kind` constant, the
  `sym.Kind == "init"` branch at `checker.go:166-176`, and its tests
  (`TestCheck_FR4_ExplicitInitOutsideInterface`, the index-2 assertion at
  `checker_test.go:910`). Without some change here, every member implementation
  package with an ordinary `func init()` becomes a violation; scoping it to the
  interface package would leave a rule whose rationale no longer holds (Q15).
  A2's *other* half — emitting `func <B-pkg>.init CAPABILITY_SAFE` when pruning
  dependency B — is untouched and is what actually carries the soundness.
- This is the one non-additive edit in the batch and it touches the golden text
  renders, so it lands here with the roots change that makes it necessary.
- `METHOD_OUTSIDE_INTERFACE` needs **no change** — it is already conditional on
  the receiver type being declared in an interface file (`checker.go:152-165`).
  Add a test that pins this, so a later refactor cannot silently widen it.

**Tests.**
- Design §7.1 fixture 1: the note's Part 1 example with `backend` as a *member*
  must report `UNDECLARED_AUTHORITY` for FILES, where it passes today.
- `checker`: a member implementation package with `func init()` produces no
  finding, and no `INIT_OUTSIDE_INTERFACE` survives anywhere (pinned by the
  golden renders).
- Pinning test for the method rule's existing interface scoping.

**Integration.** First behavior change of the batch. Self-checks must stay green;
if any component's own impl package trips something here, that is a real finding
to look at rather than a test to relax.

**Demo.** Run the Part 1 fixture before and after: authority reachable only
through a member implementation package now fails the check. Show a package with
`func init()` no longer flagged, and its authority charged to the component
instead — attribution doing the job the placement rule was standing in for.

---

## Step 3: The analysis platform becomes declared data, and layouts are held to it

**Objective.** Stop the analysis depending on what the arcc binary itself was
built for and on ambient environment variables — and stop a layout from silently
contradicting the platform it declares.

**Guidance.**
- Add the optional `platform` block to the layout (design §5.1); build a
  `build.Context` from it in `packagelayout` and use it for
  `filterByBuildConstraints` and `FileMatchesBuildConstraints`. Absent block →
  `build.Default`, preserving today's behavior for hand-written layouts and the
  native path.
- Error when constraint filtering leaves a package with **no** `.go` files —
  today that silently yields an empty package that type-checks vacuously.
- `cgo_enabled` maps to `build.Context.CgoEnabled`; note in a comment that
  rules_go's `pure` is an approximation of it
  (`../research/build-platform-and-tags.md` §3).
- **T8 validation.** The platform block governs emitters too. Accept both
  conforming shapes — filtered files *with* imports, or unfiltered files with
  imports **omitted** — and reject the accidental middle by comparing declared
  imports against the imports of the post-filter source set, **bidirectionally**:
  error on a declared import no surviving file contributes, and on an import a
  surviving file contributes that was never declared. The loader already parses
  sources for import recovery, so this is nearly free, and the second direction
  catches an edge FR2 would otherwise never see.
- **T8a — the equality runs over *resolvable* imports only.** An import resolving to
  no layout package and no stdlib package is a different failure with a different
  cause; folding it into T8 would make T8 a completeness requirement on layouts
  rather than a consistency one, and would turn an emitter's deliberate drop of a
  dangling edge into a hard failure. Collect them instead and report as
  `ANALYSIS_LIMITATION` naming file and import — same loader-collects /
  checker-reports split T3 uses. Needed in **both** shapes: under shape 2 the loader
  faces the same decision and would otherwise make it silently.
- Document T2's contract where the seam is defined: the platform extractor returns
  the target platform **or a fixed constant** where the host's rules expose none —
  a constant is conforming, not a degradation
  (`../research/host-portability-findings.md` F2).

**Tests.**
- A package with a `//go:build purego` file is **kept** when the layout declares
  `build_tags: ["purego"]` and **dropped** when it does not. This is the test
  that would have caught the current unsoundness, where a host tag the real build
  compiles with is invisible to `build.Default`.
- Cross-platform: a declared `goos` other than the host's selects that platform's
  `_GOOS.go` variants.
- Empty-package error.
- T8: a layout listing several platforms' variants of one package *with* their
  union of imports fails validation; the same layout with imports omitted passes and
  the loader recovers the filtered set; a surviving-file import missing from the
  declared list fails. The first of these is the shape observed in practice
  (`../research/host-portability-findings.md` F1).
- T8a: an unresolvable post-filter import does **not** fail the load and **does**
  produce `ANALYSIS_LIMITATION`, in both conforming shapes.

**Integration.** The Starlark half (emitting the block) lands in Step 6; until
then the block is exercised by hand-written layouts in tests, which is exactly
how a non-Bazel host would use it.

**Demo.** Two runs over the same sources with the same arcc binary, differing
only in the layout's `platform` block, producing different (correct) file sets.

---

## Step 4: Interface-file constraint reporting and report wording

**Objective.** Stop silently shrinking the declared interface, and stop the
success line overclaiming.

**Guidance.**
- `ValidateInterfaceFiles` returns the set of build-constraint-excluded files
  rather than skipping them silently; each becomes an `INTERFACE_FILE_EXCLUDED`
  warning naming the file and the constraint that excluded it.
- An interface with **no** surviving files is an error, not a vacuous pass.
- Reword the success line to "Component %q conforms; does not exceed declared
  authority" (T7).

**Tests.**
- A `//go:build windows` interface file on Linux produces the warning and the
  check still passes.
- A component whose entire interface is excluded fails.
- Report tests for the new kind and the reworded success line.

**Integration.** Touches `report`'s kind set; update golden outputs including the
Bazel golden test.

**Demo.** A component with a platform-gated interface file: warning visible in
both text and JSON output. A fully-gated interface: hard failure with the reason.

---

## Step 5: Classification and canonicalization made fail-closed

**Objective.** Remove the two silent-pass paths a rewriting or driver-backed host
can fall into, and clear the residue the port left behind.

**Guidance.**
- **Standard-library classification takes its verdict from provenance (T4).** Three
  cases, and they are not symmetric:
  - **Native mode:** three ordered cases. An **SDK (`GOROOT`) package is stdlib
    structurally**, mirroring `discoverStdlib` in layout mode — `packages.Package`
    has no `Goroot` field, so determine it loader-side via `go/build`'s
    `Context.Import(..., build.FindOnly)` or `go list -json`. Otherwise a package
    with `p.Module == nil` is **not** stdlib, which is what stops a driver-loaded
    dependency being silently skipped. Otherwise module metadata ANDed with
    `hostpolicy.IsStdlibPath`. **Do not** flip the nil-`Module` branch and then AND
    uniformly: `go/packages` reports `Module == nil` for *every* stdlib package, so
    that yields `false && true` for `fmt` and drowns the self-check in
    `UNDECLARED_DEPENDENCY` — the native twin of the Q18a layout trap below, and
    the reason Step 5 task 01 escalated on its first attempt.
  - **Layout mode:** the verdict comes from the layout's per-package `is_stdlib`
    bit, ANDed with the path policy; SDK-discovered packages (from
    `discoverStdlib`) are stdlib structurally and need no bit. **Do not** simply
    apply the AND on top of `stdlibClassifier`'s existing layout branch: nothing
    has a `*packages.Module` in layout mode, so the nil-`Module` flip plus a
    uniform AND makes `os` and `fmt` non-stdlib and drowns every member package in
    `UNDECLARED_DEPENDENCY` (Q18a — this would have shipped).
  - A declared bit disagreeing with the path policy is a load error naming the path
    and both verdicts. **The two are not peers:** provenance is authoritative, the
    path policy is a heuristic, and the check exists because
    `hostpolicy.IsStdlibPath` is consulted by other code paths that a wrong policy
    would also corrupt.
- **T4a: document that the bit must be provenance-derived**, in the layout schema
  next to the field. An emitter that computes it by re-evaluating a path heuristic
  produces a copy of the signal it is meant to check, making the disagreement error
  unreachable — and that is the natural implementation, already observed in a host
  emitter (`../research/host-portability-findings.md` F6). This is documentation
  doing load-bearing work; it is not decoration.
- State the identity-on-loader-paths requirement in `hostpolicy`'s exported
  contract and assert it at load time, failing closed with the offending path
  named (T5).
- `canonicalizeSymbol` rewrites the package path in every position, including
  inside generic type arguments (T6).
- Restrict the `vendor/`-prefix relaxation in `ValidateAndResolve` phase 3 to
  standard-library packages (T6).
- Remove `facts.PackageFact.IsStdlib` (no consumer) and stop encoding policy in
  `StdlibImports`' nil-ness — the loader always populates it (T6).
- Canonicalize `ResolveDependencyInterface`'s local `factsPkgs` (T6).

**Tests.**
- With a `hostpolicy` override simulating a rewriting host, a dependency that
  would previously have been skipped as stdlib now reports
  `UNDECLARED_DEPENDENCY`.
- A canonicalizer that is not identity on loader paths fails the load with a
  clear message rather than producing an empty analysis.
- `canonicalizeSymbol` on `(*a/b.T[c/d.U]).M`.
- The vendor relaxation no longer applies to a non-stdlib package.
- Layout mode: `os` and `fmt` stay stdlib (the regression Q18a identifies), a
  declared bit disagreeing with the path policy errors, and an SDK-discovered
  package needs no bit.

**Migration.** Three failures here will look like regressions and are correct: a
hand-written layout that *lists* an SDK package explicitly now needs the bit; a
fixture whose synthetic module path has a dotless first segment starts failing the
agreement check; and a layout in the unfiltered-with-imports shape starts failing
T8 (Step 3).

**Integration.** `facts` is the shared pure contract; removing a field touches
its consumers. Self-checks and the full Bazel suite are the regression signal.

**Demo.** Under a simulated rewriting host, show the before/after: previously all
boundary checks silently skipped, now a real finding.

---

## Step 6: Bazel — adapter seam additions and the `members` attribute

**Objective.** Give the rules everything they need to express declared
membership, with all host-specific knowledge in the one swappable file.

**Guidance.**
- `go_adapter.bzl` gains `go_build_platform(target)` (reads `GoInfo.mode`, **not**
  `GoSDK.goos`, which is the exec platform, and documented as permitted to return a
  fixed constant — T2), `go_attach_infra(target, infra)` (documented as permitted to
  return True unconditionally — A7), and an `INFRA_COMPONENTS` list accepting
  patterns as well as labels, with a worked example rather than a bare `[]`.
  **No** package→target naming hook: B2 is deferred because nothing in rules_arcc
  maps a package path to a target any more, and the convention it would have relied
  on does not generalize (Q16, Q18f).
- **No `arcc_subpackages()` helper.** `members` takes concrete labels;
  `native.subpackages()` returns only the frontier of nearest descendant packages
  and cannot address anything past a package boundary, so a helper named for
  subtrees would not deliver subtrees (`../research/members-glob-expansion.md`,
  findings 2-3). An author wanting the frontier calls bazel-skylib's
  `subpackages.all()` at BUILD top level themselves.
- `members = attr.label_list(providers = GO_PROVIDERS, aspects = [arcc_deps_aspect])`
  on the component rule, so member targets are pulled into the closure by the
  aspect — a member the interface does not import must still be analyzed.
- Delete `_check_component_roots` (`component.bzl:181-192`). It is *inputless*
  under `PACKAGE_SURFACE` — no interface target means no `component_root` — and
  jobless under declared membership, where M7 states the real constraint over
  package sets. Its removal unblocks co-locating a wrapper with the library it wraps.
- Layout-generation inputs derive from the union of the interface and the members
  (M9), not from the interface target alone. Required by M2 in general, not by
  `PACKAGE_SURFACE`: a declared-style component with members the interface does not
  import has the same need.
- `interface_style` attribute, with the shape rule enforced in the macro
  implementation: default style ⇒ `interface` mandatory and `members` optional;
  `PACKAGE_SURFACE` ⇒ `members` mandatory and `interface` **rejected** with a
  `fail()` naming the component (Q17a). `interface` therefore becomes
  `mandatory = False` plus a validation check rather than a mandatory attr.

**Tests.**
- Analysis test: a component declaring members has those packages in its closure
  even when the interface does not import them.
- Analysis test for `go_build_platform` returning the target (not exec) platform.
- Analysis tests for the shape rule: `PACKAGE_SURFACE` with `interface` set
  fails; `PACKAGE_SURFACE` with empty `members` fails; the default style with no
  `interface` fails.
- A component whose members include a package the interface does not import gets
  that package's layout data (M9) — the test that pins the union rule.
- The `_check_component_roots` fixtures are deleted with the rule; a component whose
  `component_dep` is rooted inside it now builds.

**Integration.** Additive to the rules; no manifest or layout change yet, so the
existing Bazel suite must stay green untouched.

**Demo.** `bazel build` a component declaring explicit `members` and print the
resolved closure, showing member packages present that the interface never
imports. Show the three shape-rule failures.

---

## Step 7: Bazel — declared classification, emission, and example migration

**Objective.** The full Bazel path end to end: membership is declared, the
manifest records it literally, and the leftover packages that used to be
swept into absorption now surface.

**Guidance.**
- Classification order becomes covered → member → absorbed → error (design §3.1).
  The error branch needs **no new checker rule**: `UNDECLARED_DEPENDENCY` already
  covers it and only stays quiet today because the rule emits blanket absorbed
  entries.
- **Classify the FR2 frontier, not the whole closure** (design §3.1). `_classify`
  must iterate the packages directly imported by a member or interface package —
  available as `merged[p].deps` from the aspect — rather than every key of
  `merged`. Packages deeper in the closure stay layout-only: emitted into the
  layout's `packages` for type-checking, with no `absorbed_dependencies` entry and
  no trip through the unclassified/error branch. Iterating the whole closure makes
  the emitter declare packages no checker rule consults, and the checker then
  reports each as `UNUSED_DEPENDENCY` — which is the checker being right and the
  emitter being wrong.
- Emit the expanded member import paths into the manifest (M4) and the identical
  set as the layout's `roots`; enforce `roots == members` at load time as a hard
  error (M6).
- `MEMBER_OVERLAP` violation, plus the analysis-time `fail()`s for a member that
  is also an `absorbed_dep` label or covered by a `component_dep` (M7).
- Emit the `platform` block from `go_build_platform` (T2 emitter half).
- Migrate the csvtool example components off blanket absorption onto `members`.

**Tests.**
- Design §7.1 fixture 3: a member importing a package that is neither member,
  covered, absorbed nor stdlib reports `UNDECLARED_DEPENDENCY`.
- A fixture reproducing the note's scenario — four declared implementation
  packages plus three utility packages previously swept in transitively — showing
  the three now surface.
- `roots != members` rejected at load.
- Golden test updated for the new manifest fields.

**Integration.** This is the step that supersedes R4 and §5.3 of the Bazel
design; note it in the commit and carry the doc rewrite in Step 11.

**Demo.** The note's scenario end to end: `bazel test` on a component whose
absorbed closure silently included three utility packages now fails, naming them,
and passes once they are declared where they belong.

---

## Step 8: The absorbed func-value escape warning

**Objective.** Make the residual gap visible: a function value whose body lives
in absorbed code is analyzed by nobody.

**Guidance.**
- In `goanalysis`, scan SSA instruction operands in member functions for
  `*ssa.Function` values whose defining package is absorbed. Exclude the static
  callee position of a call (`CallCommon.Operands` includes the callee), and
  resolve the defining package via `Object()` rather than `Pkg`, which is nil for
  bound-method thunks and synthetic wrappers
  (`../research/capability-analysis-mechanics.md` §3).
- Suppress when the call graph already reaches that function from member code —
  `f := absorbed.F; f(x)` is attributed via VTA and would be noise.
- Emit `facts.FuncValueEscape`; `checker` turns it into
  `ABSORBED_FUNC_VALUE_ESCAPE`.
- Remove `HIGHER_ORDER_BOUNDARY_CALL` and the `PassesFuncValue` branch: its
  polarity is wrong, firing precisely on the intentional plugin-struct pattern.

**Tests.**
- Design §7.1 fixture 2: the Part 1 example with `backend` **absorbed** warns.
- The same shape with the func value's body in a **member** produces no warning —
  this is the plugin-struct pattern and it must stay silent.
- `f := absorbed.F; f(x)` produces no warning.
- A bound method value (`x.M`) from an absorbed package is detected.

**Integration.** Removing a warning kind touches goldens and the Bazel golden
test.

**Demo.** Two nearly identical components — callback body absorbed vs. member —
showing a warning in the first and silence in the second.

---

## Step 9: Package-surface components and auto-attached edges

**Objective.** Let code that was never given an architectural interface be pruned
without its authority simply vanishing — whether it is a toolchain-injected
runtime (so a pure component can use protos) or an existing library an author
wraps to draw a boundary around it. These are one kind of component (Q17).

**Guidance.**
- `capanalyzer.AnalyzeRequest` gains `PruneAtPackages []string` beside `PruneAt`,
  keeping granularity explicit in the port.
- `capslockadapter.buildClassifier` emits `package <path> CAPABILITY_SAFE` for
  the packages of any dependency whose manifest declares
  `interface_style = PACKAGE_SURFACE`. Capslock resolves per-function keys before
  the package fallback, so both forms coexist
  (`../research/capability-analysis-mechanics.md` §2).
- `goanalysis.ResolveDependencyInterface` gains a `PACKAGE_SURFACE` branch: the
  dependency's interface symbol set is every exported symbol of every member,
  taken from facts already loaded. This is what keeps `UNUSED_DEPENDENCY`
  meaningful for a wrapped library while making `CALLS_UNDECLARED_INTERFACE`
  vacuous.
- `checker`: a `PACKAGE_SURFACE` dependency prunes at package granularity and
  skips `CALLS_UNDECLARED_INTERFACE`; a dependency edge marked `auto_attached`
  never produces `UNUSED_DEPENDENCY`. The exemption is read off the depender's own
  manifest — no dep-manifest lookup. FR4 placement rules need **no** special case:
  with the init rule gone (Step 2), `METHOD_OUTSIDE_INTERFACE` is already vacuous
  when `interface_files` is empty.
- The rule attaches an `INFRA_COMPONENTS` entry as a `component_dep` per
  `go_attach_infra`, marking that edge `auto_attached` (A7). Unconditional
  attachment is conforming: pruning at a package is a no-op unless that package is
  reached, and own-code authority is charged regardless because members are roots.
  The injected edge **appears in the emitted manifest even when unconditional** —
  otherwise the legibility that justified `auto_attached` over a hidden allowlist is
  given back.
- **Pattern membership (M8).** A `PACKAGE_SURFACE` component may declare
  import-path patterns rather than labels, unexpanded, for packages that cannot be
  named as targets. Its surface then resolves from the *depender's* layout, which
  contains those packages by construction. Narrower than it sounds: prune keys need
  only package paths, and an `auto_attached` edge is exempt from
  `UNUSED_DEPENDENCY`, so local symbol extraction is needed only for pattern
  membership *without* auto-attachment.
- **A8 — the certified/asserted annotation.** The report's dependency listing marks
  each pruned boundary certified (the dependency's own check runs) or asserted, with
  the certification reference where recorded. Deliberately **not** a finding: the
  depending component is not at fault, and where a runtime is injected everywhere a
  warning would fire on every component in the repository and be suppressed
  wholesale. Enforcement is one repo-wide check, deferred with the
  membership-uniqueness one.
- **A9 — bodiless absorbed packages** yield `ANALYSIS_LIMITATION` naming the
  package. Reuses the existing kind rather than the `UNANALYZED` authority constant,
  which is a Capslock capability the adapter deliberately suppresses.

**Tests.**
- A component reaching authority only through an injected infra dependency
  declares nothing and passes.
- The infra component's **own** check still reports its authority — the property
  that distinguishes this from a bare trust list.
- A per-function prune key still overrides the package key.
- Attachment does not happen when the closure lacks the runtime's packages.
- The author-written case: a `PACKAGE_SURFACE` wrapper that a member calls into
  produces no `UNUSED_DEPENDENCY`, and the same wrapper left unused **does** warn
  — the axis that `auto_attached` separates from the component kind.
- Unconditional attachment still emits the manifest entry.
- A pattern-membership dependency resolves its surface from the depender's layout.
- The dependency listing shows certified for a component with its own check and
  asserted for one without, with the reference rendered when present.
- A bodiless absorbed package yields `ANALYSIS_LIMITATION`.

**Integration.** Depends on Step 1 (`interface_style` and `auto_attached` in the
schema), Step 2 (the init-rule removal that makes relaxed well-formedness free),
and Step 6 (`INFRA_COMPONENTS` and the `interface_style` attribute).

**Demo.** Two demos, one per case. A component whose generated code reaches a
runtime using `REFLECT`: fails before, passes after, while the infra component's
own check shows the `REFLECT` declaration doing the work. And a logger-shaped
wrapper — `interface_style = PACKAGE_SURFACE`, `members = [//common/logger]`,
`declared_authority = [FILES, NETWORK]` — where callers stop being charged NETWORK
but still get an unused warning if they declare it and never log.

---

## Step 10: Bazelified self-check

**Objective.** arcc's own dogfooding runs under Bazel, so a Bazel-only
environment cannot silently skip it — the exact gap that let the `hostpolicy`
manifest breakage through.

**Guidance.**
- `go_component` targets for **six** of arcc's own eight components:
  `capanalyzer`, `report`, `facts`, `manifest`, `checker`, `goanalysis`.
  `manifest` is a multi-package component (its `gen` package) and is what
  exercises `members` under Bazel; its hand-written manifest migrates off
  cross-package `interface_files`.
- **`capslockadapter` and `cli` are excluded from the Bazel leg**, for one shared
  reason. `capslockadapter` absorbs capslock, whose closure contains
  `golang.org/x/sys/unix` built with cgo, and the rule fails closed on cgo closures
  — a cgo package's preprocessed sources do not exist at analysis time. `cli`
  inherits it because `cmd/arcc/main.go` imports `capslockadapter`: a cgo package
  anywhere in a closure excludes every component above it, not just the one that
  names it. Native mode routes around this because the go tool preprocesses cgo
  before `go/packages` sees it, so `just selfcheck` keeps covering both and only
  the Bazel leg is lost. `cli` therefore keeps its cross-package
  `interface_files` — that migration was only needed to satisfy the Bazel model.
  Record it as a known limitation in Step 11. Do **not**
  patch the dependency to claim it is not cgo: an attempted
  `go_deps.module_override` running `sed -i 's/cgo = True/cgo = False/g'` over
  `x/sys` was rejected, because buying a green check by falsifying build metadata
  is the exact failure mode this batch exists to remove.
- Follow the existing `manifest_parity_test` precedent from the csvtool example
  so the hand-written manifest and the generated one cannot drift.
- Keep `just selfcheck` as well: the two paths (native FR1, Bazel declared
  members) checking the same components is a genuine cross-check of this whole
  design.

**Tests.** The checks themselves are the tests. Add a negative: removing a
declared dependency from a self-manifest must fail `bazel test`.

**Integration.** Last, so it exercises the finished model.

**Demo.** `bazel test //...` runs six self-checks, and `just selfcheck` still
runs all eight. Delete an `absorbed_dependencies` entry and watch the Bazel leg
fail — the regression that motivated this requirement.

---

## Step 11: Documentation and design-record sync

**Objective.** Leave the design records true, since they are the project's
memory.

**Guidance.**
- Rewrite **R4** and **§5.3** of
  `.agents/planning/2026-07-16-bazel-arcc-rules/design/detailed-design.md`, and
  reverse **§5.1**'s "membership is not encoded in the manifest" (cited as §4.7
  in earlier drafts; that document has no §4.7). Rewrite rather
  than amend — the classification order and the membership source both change.
- Correct §4.3/§4.6's statement that `GoInfo.srcs` is the compiled set; the
  loader filters, and now does so against a declared platform.
- Remove the `INIT_OUTSIDE_INTERFACE` prose from the 2026-07-06 design (§FR4 rule
  (c)) and from review A2, keeping A2's `init CAPABILITY_SAFE` half — that record
  should say the rule was retired because attribution supersedes it, not that it
  never existed.
- README: `members` (concrete labels, and why there is no wildcard helper),
  `interface_style = PACKAGE_SURFACE` for wrapping interface-less libraries,
  `auto_attached` edges, and the adapter's new hooks.
- Record Appendix C's eight limitations where a user will find them, not only in
  the planning tree — C.4 especially, since "the component's BUILD file changes when
  the implementation is restructured" is a live cost an author feels.
- **Add the cgo-closure limitation** discovered in Step 10: a component whose
  closure contains a cgo package cannot be checked under Bazel, because a cgo
  package's preprocessed sources do not exist at analysis time. Native mode is
  unaffected. arcc has **two** instances — `capslockadapter`, which absorbs
  capslock, and `cli`, which imports `capslockadapter` — which is why the Bazel
  self-check covers six components and `just selfcheck` covers eight. State the
  asymmetry plainly, and state that exclusion propagates upward through importers:
  a reader who sees six Bazel checks and eight native ones should find the reason
  next to the count, not have to infer it.
- Document the layout schema's `is_stdlib` field with T4a's provenance requirement
  adjacent to it, and the two conforming platform shapes (T8). Both are places where
  the natural implementation is the wrong one, so the doc is load-bearing.

**Tests.** `just ci` green, including `gen-is-clean`.

**Demo.** A reader following the Bazel design document arrives at the model the
code actually implements.

---

## Step 12: Remediation from implementation review 2026-07-29

**Report:** `review.yaml` (verdict `remediation_recommended`; 4 important, 5
suggestion, 1 nit). Tasks in
`.agents/tasks/2026-07-25-component-membership-and-authority/step12/`.

**Objective.** Close the three substantive gaps the whole-implementation review
found — all of them at the edges the design cared most about — plus the residue
of dead surfaces, stale contracts and weak tests that no single task review was
positioned to see.

**Guidance.**
- **A component whose membership cannot be analyzed must not report conformance
  (F1).** `arcc check` on a manifest whose `members` are all patterns currently
  prints the conformance line and exits 0 having loaded nothing:
  `LoadPackageFacts` short-circuits, `app.go` skips capability analysis on an
  empty package set, and the clean render branch does the rest. Appendix C.6
  assumes such a component is never the subject of a check; nothing enforces
  that, and the cost of the assumption failing is the silent-clean shape this
  whole batch exists to remove. Fail closed, and decide the mixed
  literal/pattern case rather than letting it fall out.
- **Emit `own_check_runs` (F2).** No emitter writes it, so A8's `certified`
  state is unreachable for every generated manifest — visible on arcc itself,
  whose eight self-check reports list every dependency as `asserted` including
  the six components whose checks run under `bazel test`. §5.3 argues against a
  finding *because* the dependency listing carries the signal; the listing has
  to be able to carry it.
- **Get the test harness out of the host seam (F3).** `go_adapter.bzl` promises
  it is the one file a host replaces, and now contains a test-only attachment
  predicate with hardcoded fixture patterns, backed by two "Undocumented testing
  attribute" attrs on the production rule. Keep the Step 9 encapsulation; move
  the fixtures' half out.
- **Correct the seam contract (F4, F7).** `go_attach_infra` is declared and
  documented as taking one target and is called with the list of root targets —
  which is what M9 requires, so the code is right and the design §4.7
  declaration, the docstring and the parameter name are all wrong. Pin it with a
  predicate that reads its argument. Fold in the still-open Step 6 macro-doc
  finding while in `defs.bzl`.
- **Sweep the residue (F5, F6, F8, F9, F10).** A discarded `_classify` return,
  two unused `packagelayout` exports the design still cites as live consumers,
  two membership-matching semantics in the Go core, two tests that pass under
  mutation of the properties they name, two converted tests whose names assert
  the warning they now assert is absent, and a handful of comments pointing at
  removed rules and superseded sections.

**Tests.** Each task carries its own; the cross-cutting requirement is that the
two mutation-sensitive tests (F8) are proven RED under mutation and GREEN after,
with both runs recorded, since the finding is precisely that they are green
today under mutation. No existing assertion may be weakened to accommodate a
remediation task.

**Integration.** Tasks 01–04 are independent of each other; 05 depends on 01's
membership semantics and 03's dead-branch removal; 06 lands last so its comment
sweep falls on final text. `just ci` green after each.

**Demo.** `arcc check` on a pattern-membership manifest no longer claims
conformance. A self-check's dependency listing shows `certified` for the
components whose checks actually run and `asserted` for the two that cannot run
under Bazel. `go_adapter.bzl` reads as a contract a second host can implement
from, with no fixture code in it.
