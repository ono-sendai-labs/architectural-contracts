# Task: `capanalyzer` capability port + finding & policy types

## Description
Introduce the pure `internal/capanalyzer` package: the `CapabilityAnalyzer` **port**
(the seam Capslock hides behind — NFR4/Q1), the finding/frame/class value types the
analyzer produces, the `InterfaceSymbol` normalized-key primitive shared with the
boundary check, the `AnalyzeRequest` scope descriptor, and the `CapabilityPolicy`
allow/warn model with its `StrictPolicy()` MVP default. This package is a
dependency-free leaf: it defines types and a policy, imports **no Capslock** (the
adapter arrives in Step 7), and is fully unit-testable in isolation. It is sequenced
first in Step 3 because `facts` (task-02) references `InterfaceSymbol`.

## Background
The checker is a pure function of injected data; `capanalyzer` is one of the two input
packages it consumes (findings + policy) and defines the `InterfaceSymbol` primitive
that both the FR5 boundary check and FR5b pruning use. Keeping the port here — with no
Capslock import — is what lets the checker be tested with hand-built findings and keeps
every core dependency pointing inward (never core→shell).

The A4 **normalization contract** on `InterfaceSymbol` is load-bearing: generic
type-argument brackets are stripped from SSA names before comparison, and methods are
emitted in **both** pointer- and value-receiver key forms (e.g.
`example.com/store.Read`, `(*example.com/store.DB).Get`). Document it on the type;
enforcement/consumption lands in later steps.

The classifier configuration that shapes findings (`UNANALYZED` exclusion,
minting-not-use, the prune map) is **adapter-side** (Step 7) and separate from the
checker-side `CapabilityPolicy` allow/warn decision defined here — keep the two
concerns distinct in the doc comments.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§4.1 `capanalyzer` interfaces — `CapabilityFinding`, `Frame`, `InterfaceSymbol`, `AnalyzeRequest`, `CapabilityAnalyzer`, `CapabilityPolicy`, `StrictPolicy`; §5.4 authority rule for allow/warn/violation semantics)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 3)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/capslock.md` (the true-authority vs analysis-defeating taxonomy behind `Class`; consumed by the Step-7 adapter, informs the `Class` values here)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Create package `internal/capanalyzer` with **no imports of Capslock** and no I/O.
2. Define value types per design §4.1:
   - `CapabilityFinding` (`Package string`, `Capability string`, `Class Class`,
     `CallPath []Frame`).
   - `Frame` (`Func string`, `File string`, `Line int`).
   - `Class` with the two members `TrueAuthority` and `AnalysisDefeating`.
   - `InterfaceSymbol` (a `string`-based named type) carrying the A4 normalization
     contract **in its doc comment**: generic brackets stripped, both receiver forms
     emitted, key forms like `pkg.Name` and `(*pkg.Type).Method`.
   - `AnalyzeRequest` (`Packages []string`, `PruneAt []InterfaceSymbol`) with the
     whole-package scope semantics documented (every function in `Packages`;
     `_test.go` excluded by construction; traversal pruned at `PruneAt`).
3. Define the port `CapabilityAnalyzer interface { Analyze(req AnalyzeRequest) ([]CapabilityFinding, error) }`.
4. Define `CapabilityPolicy` (`Allowed map[string]bool`, `Warn map[string]bool`) and
   `func StrictPolicy() CapabilityPolicy` returning the empty policy (both maps
   empty/nil — MVP default where every capability fails).
5. Provide a small **pure classification helper** that, given a capability name and a
   `CapabilityPolicy`, reports the decision (allowed / warn / violation) — the seam the
   Step-5 authority rule will use. (A tiny enum/result type is fine; the goal is a
   testable pure function encoding: in `Allowed` → allowed; else in `Warn` → warn; else
   → violation.)

## Dependencies
- None (leaf package; first task of Step 3).

## Implementation Approach
1. Add the value types and the `Class` enum (string- or int-based constants; give
   `Class` a `String()` if it aids test readability).
2. Add the `CapabilityAnalyzer` port and the `CapabilityPolicy` type + `StrictPolicy()`.
3. Add the pure classify-against-policy helper.
4. Document the A4 normalization contract on `InterfaceSymbol` and the adapter-vs-policy
   distinction on the relevant types.
5. Table-driven unit tests: `StrictPolicy()` has empty `Allowed`/`Warn`; the classify
   helper returns allowed for a capability in `Allowed`, warn for one only in `Warn`,
   and violation for one in neither; a capability present in both `Allowed` and `Warn`
   resolves to allowed (allow wins — matches §5.4: `∈ Allowed` short-circuits before the
   warn check).

## Acceptance Criteria

1. **Types compile and expose no Capslock**
   - Given the `internal/capanalyzer` package
   - When built and its imports inspected
   - Then it compiles, imports no Capslock and performs no I/O, and exposes
     `CapabilityFinding`, `Frame`, `Class`, `InterfaceSymbol`, `AnalyzeRequest`,
     `CapabilityAnalyzer`, `CapabilityPolicy`, and `StrictPolicy`.

2. **StrictPolicy is empty**
   - Given `StrictPolicy()`
   - When inspected
   - Then both `Allowed` and `Warn` are empty (len 0), so every capability classifies as
     a violation.

3. **Policy classification behaves**
   - Given a `CapabilityPolicy`
   - When a capability is classified against it
   - Then a capability in `Allowed` → allowed, one only in `Warn` → warn, one in neither
     → violation, and one in both → allowed.

4. **Normalization contract documented**
   - Given the `InterfaceSymbol` type
   - When its doc comment is read
   - Then it states the A4 rules (generic brackets stripped; both pointer- and
     value-receiver key forms) and gives the key-form examples.

## Metadata
- **Complexity**: Low
- **Labels**: pure-core, capanalyzer, port, policy
- **Required Skills**: Go
