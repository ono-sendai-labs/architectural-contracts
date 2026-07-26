# Implementation plan — component membership and authority attribution

**Design:** `../design/detailed-design.md`
**Requirements:** `../idea-honing.md` (Q1–Q14)
**Research:** `../research/`

Requirement IDs (M*, A*, T*, B*) refer to §2 of the design.

## Progress checklist

- [ ] **Step 1** — Declared membership: `members` + `interface_style` in the schema, honored end to end in native mode (M1, M3, M5)
- [ ] **Step 2** — Attribution follows ownership: members are roots, `INIT_OUTSIDE_INTERFACE` removed (A1, A2, A3)
- [ ] **Step 3** — The analysis platform becomes declared data (T1, loader half of T2)
- [ ] **Step 4** — Interface-file constraint reporting and report wording (T3, T7)
- [ ] **Step 5** — Classification and canonicalization made fail-closed (T4, T5, T6)
- [ ] **Step 6** — Bazel: adapter seam additions, `members` and `interface_style` attributes (B1, emitter half of T2)
- [ ] **Step 7** — Bazel: declared classification, emission, and example migration (M4, M6, M7)
- [ ] **Step 8** — The absorbed func-value escape warning (A5)
- [ ] **Step 9** — Package-surface components and auto-attached edges (A6, A7)
- [ ] **Step 10** — Bazelified self-check (B3)
- [ ] **Step 11** — Documentation and design-record sync

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
- Add `members` (repeated string) and the `InterfaceStyle` enum with
  `interface_style` to `proto/archcontracts/v1/component.proto`, plus
  `auto_attached` on `ComponentDependency`, per design §4.1; regenerate with
  `just gen` and keep `just gen-is-clean` green.
- `manifest`: parse and validate all three. Reject duplicate members, a member
  that is also an `absorbed_dependencies` import path (M7 contradiction), and
  malformed patterns. Under `interface_style = PACKAGE_SURFACE`, require non-empty
  `members` and **empty** `interface_files`; both directions are errors, so the
  surface has one source of truth.
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

## Step 3: The analysis platform becomes declared data

**Objective.** Stop the analysis depending on what the arcc binary itself was
built for and on ambient environment variables.

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

**Tests.**
- A package with a `//go:build purego` file is **kept** when the layout declares
  `build_tags: ["purego"]` and **dropped** when it does not. This is the test
  that would have caught the current unsoundness, where a host tag the real build
  compiles with is invisible to `build.Default`.
- Cross-platform: a declared `goos` other than the host's selects that platform's
  `_GOOS.go` variants.
- Empty-package error.

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
- Standard library is the **AND** of module metadata and
  `hostpolicy.IsStdlibPath` (T4). This fixes both directions: a driver-loaded
  dependency with no `Module` is no longer classified stdlib and silently skipped;
  a host-rewritten dotless path that has a `Module` is no longer misread as
  stdlib.
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
  `GoSDK.goos`, which is the exec platform) and an `INFRA_COMPONENTS` list (used
  in Step 9). **No** package→target naming hook: B2 is deferred because nothing in
  rules_arcc maps a package path to a target any more (Q16).
- **No `arcc_subpackages()` helper.** `members` takes concrete labels;
  `native.subpackages()` returns only the frontier of nearest descendant packages
  and cannot address anything past a package boundary, so a helper named for
  subtrees would not deliver subtrees (`../research/members-glob-expansion.md`,
  findings 2-3). An author wanting the frontier calls bazel-skylib's
  `subpackages.all()` at BUILD top level themselves.
- `members = attr.label_list(providers = GO_PROVIDERS, aspects = [arcc_deps_aspect])`
  on the component rule, so member targets are pulled into the closure by the
  aspect — a member the interface does not import must still be analyzed.
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
- The rule attaches an `INFRA_COMPONENTS` entry as a `component_dep` **iff** the
  component's closure contains one of its packages, marking that edge
  `auto_attached` (A7).

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
- `go_component` targets for arcc's own eight components. `cli` spans `cmd/arcc`
  and its `app` subpackage and is the repo's own multi-package case, so it
  exercises `members`.
- Follow the existing `manifest_parity_test` precedent from the csvtool example
  so the hand-written manifest and the generated one cannot drift.
- Keep `just selfcheck` as well: the two paths (native FR1, Bazel declared
  members) checking the same components is a genuine cross-check of this whole
  design.

**Tests.** The checks themselves are the tests. Add a negative: removing a
declared dependency from a self-manifest must fail `bazel test`.

**Integration.** Last, so it exercises the finished model.

**Demo.** `bazel test //...` runs all eight self-checks. Delete an
`absorbed_dependencies` entry and watch it fail — the regression that motivated
this requirement.

---

## Step 11: Documentation and design-record sync

**Objective.** Leave the design records true, since they are the project's
memory.

**Guidance.**
- Rewrite **R4** and **§5.3** of
  `.agents/planning/2026-07-16-bazel-arcc-rules/design/detailed-design.md`, and
  reverse **§4.7**'s "membership is not encoded in the manifest". Rewrite rather
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
- Record Appendix C's six limitations where a user will find them, not only in the
  planning tree — C.4 especially, since "the component's BUILD file changes when
  the implementation is restructured" is a live cost an author feels.

**Tests.** `just ci` green, including `gen-is-clean`.

**Demo.** A reader following the Bazel design document arrives at the model the
code actually implements.
