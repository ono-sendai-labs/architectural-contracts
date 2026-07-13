# Research — Mapping the coco discipline onto the arcc go-mvp design

Sources:
- coco skills: `.claude/skills/coco-component-design/SKILL.md`,
  `.claude/skills/coco-references/concepts.md` (component model, two contract
  tiers, contract algebra, rely-guarantee reasoning).
- go-mvp design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md`
  (twice-reviewed; Step-0 Capslock spike executed). Its research files
  (`research/capslock.md`, `research/go-component-model.md`,
  `research/spike-capslock.md`) remain the technical ground truth for Capslock
  behavior and are **reused, not repeated**, here.
- Manifest proto basis:
  `../architectural-contracts/proto/archcontracts/v1/component.proto`.

## 1. What the go-mvp design already does coco-style

The go-mvp design is *structurally* coco-aligned:

- Components with designated interface files, directory-based membership,
  declared dependencies (two kinds), declared authority. This **is** the coco
  component model — unsurprisingly, since arcc mechanizes it.
- Pure-core / authority-shell split; ports defined in the core
  (`capanalyzer.CapabilityAnalyzer`), adapters in the shell; fact data model in
  the core (dependencies point inward).
- Compositional soundness is already the stated basis of FR5b pruning ("the
  graph is sound iff every component independently conforms").

## 2. What is missing (the coco deltas)

1. **Contracts are unstructured prose.** FR10 asks for "informal contract" doc
   comments — one tier, no labels, no designated Tier-2 home, nothing a test or
   review can anchor to. Coco wants **two tiers**: lean caller-facing clauses in
   interface doc comments (Tier 1) and a full contract file per component
   (Tier 2) with exhaustive edge behavior, rely-set, clause→test map, fake
   notes, authority rationale.
2. **The manifest cannot designate a contract home.** The proto comment says
   explicitly "There is no contract field." Coco expects the manifest to name
   the contract file(s). *Decision (Q1): designation-only —
   `contract_files` field; arcc checks existence, never content.*
3. **Rely-sets are implicit.** The go-mvp design contains load-bearing
   inter-component assumptions that are stated only in passing prose or code
   comments. Extracted (this is the raw material for the design's rely-sets):
   - `checker.Check` assumes `Caps` arrive **already pruned** at the
     component-dependency boundaries ("ALREADY pruned at DepIfaces" — a comment
     on a struct field!). That is a rely on the orchestrator (cli) honoring the
     adapter's `PruneAt` contract.
   - `checker` assumes symbol keys in `Facts`/`DepIfaces`/`Caps` are in **one
     shared normal form** (Capslock key form, generic brackets stripped, both
     receiver forms). Three producers (goanalysis × 2 paths, capslockadapter)
     must agree; today the normal form is described inside a Go doc comment on
     `InterfaceSymbol`.
   - `checker` assumes the `Manifest` it receives is **syntactically valid**
     (post-`Parse`), and that `DepIfaces` really are the FR4 symbol sets of the
     *declared* dependencies (resolution done by goanalysis per §5.5).
   - `capslockadapter` relies on **pinned Capslock behavior** validated by the
     Step-0 spike (CAPABILITY_SAFE terminates traversal; key formats;
     whole-package scope; `_test.go` exclusion) — an *external* rely that
     belongs in its Tier-2 contract so version bumps trigger re-validation.
   - `manifest.Parse`'s authority-freedom **by construction** relies on the
     adapter's minting-not-use classifier config — a cross-component rely from
     a core component on a shell component's configuration.
4. **No contract algebra discipline.** Nothing marks a contract change as
   strengthening vs breaking; nothing says a fake must be a behavioral subtype.
5. **Tests are organized by module, not by clause.** The go-mvp test strategy
   is good but has no clause→test map, so there is no way to see which contract
   clauses lack evidence.
6. **No verified fakes.** The go-mvp plan uses hand-built inputs for the pure
   checker (fine — plain data) but has no fake for the `CapabilityAnalyzer`
   port, so orchestration (cli/app) can only be tested end-to-end.

## 3. Design levers chosen (feed §Design)

- **Proto delta:** add `repeated string contract_files` to `Component`
  (field 6), paths relative to component root, any format; arcc's only rule is
  that each designated file exists. Empty is legal (arcc imposes no contract
  discipline on other projects); *this* project's convention requires ≥1.
- **Project convention doc** (`docs/coco-conventions.md` in-repo): Tier-2 file
  is `component-contract.md` beside the manifest; clause labels
  `PRE:`/`POST:`/`INV:`/`HIST:` + kebab slug; Tier-1 states caller-facing
  clauses once, Tier-2 references by label and elaborates; Tier-2 sections:
  Purpose, Clause elaborations, Rely-set, Declared authority rationale,
  Clause→test map, Fake fidelity (where applicable).
- **The symbol-key normal form becomes a named contract clause** owned by the
  `facts` component (`INV:symbol-key-form`), relied on by name from `checker`,
  `goanalysis`, and `capslockadapter` — turning today's fuzziest implicit
  coupling into an explicit, single-home clause.
- **Checker preconditions become real PRE clauses** discharged by `cli`'s
  contract (its Tier-2 shows how each PRE of `checker.Check` is met), replacing
  the "already pruned" field comment.
- **Verified fake for `CapabilityAnalyzer`** per `coco-contract-testing`: an
  in-memory fake driven by a declarative capability table that applies the same
  pruning semantics; a **shared contract suite** runs against both the fake
  (table describing a fixture) and the real adapter (the fixture itself) —
  scenarios from the port's contract clauses.
- **Orchestration ports are consumer-defined** (Go idiom): the cli's `app`
  package defines small interfaces for facts-loading and dependency-resolution
  that `goanalysis` satisfies; `capanalyzer` keeps the analyzer port + finding
  types (it is also the future-Rust seam). This keeps `app` hermetically
  testable with the fake + hand-built facts.
- **Dependency shape:** the component graph is a DAG with benign diamonds
  (everyone shares the pure data-model components `facts`/`capanalyzer`);
  no cycles. Called out per coco §6; acceptable because the shared nodes are
  leaf-like pure vocabulary components whose contracts are stable.

## 4. Carried-over technical ground truth (not re-researched)

All Capslock mechanics from the go-mvp design carry over unchanged: library
behind a port; classifier config (exclude `UNANALYZED`; minting-not-use SAFE
reclassification of `(*os.File)` handle methods; per-run merged capability map
with `CAPABILITY_SAFE` prune entries incl. `func <pkg>.init`); VTA call graph
for FR5 edges; whole-package analysis scope; `_test.go` exclusion; known
limitations (higher-order boundary leak, generic normalization, use-beyond-
calls, compositional trust). See the go-mvp design §5.4a, §11 and
`research/spike-capslock.md`.
