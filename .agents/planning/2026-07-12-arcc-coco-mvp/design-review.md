# Design & Plan Review — arcc coco-first MVP (2026-07-12)

Self-review pass per Round-1 Q4: draft design + plan were committed
(`zvxptlnp` — "docs(planning): draft coco-first arcc MVP design +
implementation plan") **before** these findings were folded in, so the diff of
the follow-up change records exactly what the review changed.

Reviewed with `coco-contract-review` (design-stage mode: proposed contracts,
no code yet) plus a general critical pass over the plan. Dimensions: adherence
(tier coherence, responsibility split, testability), compositional soundness
(rely-set completeness/strength), changeset discipline, and plan consistency.

## Findings

### F1 — `manifest` lists its own `gen/` subpackage as an absorbed dependency
*(important · adherence/structural · design §5.3)*
Directory-based membership (FR1) makes `internal/manifest/gen/` **part of the
`manifest` component** — absorbed dependencies are for *external* packages. As
written, self-hosting would model a component absorbing itself.
**Fix:** membership covers `gen/`; the absorbed dependency is the protobuf
runtime (`google.golang.org/protobuf/...`) only. Applied.

### F2 — `goanalysis` declares a dependency on `capanalyzer` it no longer uses
*(important · soundness · design §4 graph, §5.6)*
A leftover from the go-mvp, where `InterfaceSymbol` lived in `capanalyzer`.
After ADR-2 moved the symbol vocabulary to `facts`, `goanalysis` consumes only
`facts` and `manifest`. The stale edge would trip arcc's own R2
(`UNUSED_DEPENDENCY`) on self-hosting — the design failing its own checker.
**Fix:** edge removed (component table, mermaid graph, §7 walk). Applied.

### F3 — `DependencyInterface` cannot support R1's allowed-import derivation
*(critical · soundness/model completeness · design §8.2, §5.6, §6)*
R1 permits imports of a component dependency's **interface packages** (the
dep's packages containing ≥1 interface file). `DependencyInterface{Component,
Packages, Symbols}` carries *all* packages and the symbol set — the checker
cannot derive the interface-package subset from it, so R1 as modeled is
unimplementable (or would over-allow every dep package). This gap is inherited
from the go-mvp data model and surfaced by walking the rely-set: `checker`'s
rely on `goanalysis POST:depiface-fr4` was **present but too weak**.
**Fix:** add `InterfacePackages []string` to `DependencyInterface`; extend
`POST:depiface-fr4` to promise it; checker R1 consumes it. Applied.

### F4 — `PRE:keys-normal-form` overreaches to `Caps`
*(important · precision/responsibility split · design §5.5, §5.2)*
Capability findings carry no symbol keys the checker matches on
(`Capability`/`Class` drive R7/R8; `CallPath` is evidence prose). The
normal-form obligation actually sits on the **request side of the port**:
`PruneAt` must be normal-form for the adapter's translation to Capslock map
keys (bracket-free origins, spike-verified) to be exact.
**Fix:** rescope `PRE:keys-normal-form` to `Facts`/`DepIfaces`; add
`PRE:pruneat-normal-form` to `CapabilityAnalyzer.Analyze`; cli discharge table
updated. Applied.

### F5 — `capfake`'s API is architecture-private, but other components' tests must call it
*(important · boundary discipline · design §5.2, §11)*
`capfake` lives in `capanalyzer`'s subtree with only `capanalyzer.go` as an
interface file, making the fake's exported API architecture-private — whose
declared semantics ("component-internal testing") exactly *forbid* the
intended cross-component use (cli and adapter test suites). Mechanically
invisible (call edges from `_test.go` are outside the analyzed build) but a
contradiction of the design's own architecture-private semantics.
**Fix:** `capanalyzer`'s manifest lists `capfake/capfake.go` as an interface
file — the fake is *deliberately* part of the port component's declared
surface, which also matches its Tier-1 status as a contracted behavioral
subtype. Applied.

### F6 — "Four example manifests" is wrong: `parsecsv` has none
*(minor · consistency · plan Steps 11–12, summary)*
The absorbed `internal/parsecsv` deliberately has no manifest; the example has
**three** manifested components (`toprow`, `csvfile`, `app`).
**Fix:** counts corrected. Applied.

### F7 — Step 7's demo overclaims a runnable CLI against a synthetic world
*(minor · plan sequencing)*
At Step 7 the production `main` cannot be wired (real loaders arrive in
Step 8); the demo is the hermetic `app` test suite plus a binary that fails
gracefully.
**Fix:** demo reworded; graceful-failure behavior made explicit. Applied.

### F8 — Step 8's intermediate wiring can produce a misleadingly clean pass
*(important · plan/honest reporting)*
Wiring the real `FactsLoader` while the analyzer is still an empty-table fake
makes authority checks *vacuously* pass — a silent false "conforms" for any
component with undeclared authority, exactly the failure mode the tool exists
to prevent.
**Fix:** Step 8 must surface an explicit `ANALYSIS_LIMITATION`-style warning
("capability analysis stubbed") in every report produced by the intermediate
wiring, removed in Step 10. Applied to plan.

### F9 — "HIST-style maintenance note" mislabels a process obligation
*(suggestion · vocabulary hygiene · design §5.7)*
`HIST:` clauses constrain a component's state trajectory across calls
(concepts §4); the Capslock-pin re-validation duty is an **external-rely
maintenance obligation**, not a history property. Sloppy vocabulary in the
discipline's own flagship would teach the wrong pattern.
**Fix:** renamed to an "external-rely re-validation obligation" in the Tier-2
outline. Applied.

## Changeset-discipline check (all-new contracts)

All contracts are v1 (no callers to break), so strengthening/breaking
classification is trivially satisfied; the ripple introduced by F3
(checker's stronger demand → `goanalysis` promises `InterfacePackages`) is
**closed within this changeset** — no dangling demands. Post-fix, the §7
soundness walk holds: no unmet relies; external relies (Capslock, x/tools)
remain flagged with re-validation obligations.

## What the review did *not* find (checked and passed)

Tier-1/Tier-2 single-home discipline consistent throughout; checker PREs
correctly on the caller side of the Meyer split with a complete discharge
table in `cli`; `REFLECT` on `manifest` honestly declared and consistently
excluded from the authority-free showcase set; R10-as-violation mechanism
preserves checker purity; dependency DAG cycle-free after F2; fake-fidelity
story (shared suite, table/fixture forms) matches the concepts doc's
"necessary but not sufficient" stance; plan steps satisfy dependency closure
(each component built after the contracts it relies on).
