# Idea Honing — Architectural Contracts MVP (Go)

Q&A log for requirements clarification. Appended as we go.

---

## Round 1 — core scoping decisions (after Capslock research)

### Q1. How should our tool consume Capslock?
**A (confirmed): As a Go library.** Import `analyzer.GetCapabilityInfo` directly.
We get structured protos, access to the call-graph hook (`CapabilityGraph`) for
future pillar-1 checks, and can share a single `packages.Load`. Accepted cost:
a direct dependency on Capslock's `analyzer`/`interesting`/`proto` API.
- Considered: subprocess+JSON (looser coupling, loses call-graph hook); abstracting
  behind our own analyzer port. See note under Q1-followup.

*Q1 follow-up (provisional, to confirm):* even choosing the library, we will still
wrap it behind a small internal **CapabilityAnalyzer port** so the checker core
stays decoupled and testable, with Capslock as the (only) adapter. This is cheap
and preserves the future-Rust option without committing to a second impl now.

### Q2. What granularity does one manifest (component) cover?
**A (confirmed): A set of Go packages named by import-path pattern/list.** The
manifest identifies its packages via patterns (e.g. `example.com/csvtool` or
`example.com/csvtool/...`) or an explicit list. Examples will exercise the
single-package case first, but the model is not hard-coded to one package.

### Q3. Which Capslock findings FAIL the ambient-authority-free check?
**A (CONFIRMED — compromise): Parameterize the checker by a capability policy;
default to STRICT (any capability fails).** The checker's internal API exposes the
sets of *allowed* and *warn* capabilities. The MVP default is option (2): the
allowed and warn sets are **empty**, so **any** capability finding (true-authority
*or* analysis-defeating) fails conformance. Rationale: simple examples that don't
use reflect/unsafe/etc. will "just work" under strict; if a legitimate example
needs to relax it, we revisit the default. Crucially, exposing allowed/warn sets in
the API is the seam that makes future **configurable-per-manifest** cheap — and the
*allowed* set is naturally sourced from the manifest's `declared_authority`
(empty in the MVP ⇒ strict), so per-manifest configuration is already half-built.
- Considered/rejected as the *default*: "true fails / analysis-defeating warns"
  (kept available as a non-default policy the API can express).

### Q4. How are manifests authored / schema'd?
**A (CONFIRMED): Proto schema + textproto manifests.** A shared `.proto` defines
the manifest message; each component ships a hand-authored `.textproto` (working
name `COMPONENT.textproto`). Matches the "we use protobuf" constraint and stays
diff-friendly.

### Q5. Go module boundary for the tool?
**A (CONFIRMED): `./go` is its own Go module** (its own `go.mod`). Cleanest
separation for the multi-language repo. Examples may be a nested module or use
`testdata`, TBD in implementation.

---

## Round 3 — component→component dependency boundary (user directive)

**User directive:** when a component depends on another *manifested* component, two
things must hold, both anchored on the dependency's **declared interface symbols**
(the exported symbols in the dependency's interface files):

1. **Interface-boundary call check (Pillar 1, strong form).** From our component we
   must NOT call anything of the dependency that is *language-level public but not
   in the dependency's declared manifest interface*. Only declared-interface
   symbols may be called across the component boundary.

2. **Capability-attribution pruning (Pillar 3).** When analyzing our component for
   ambient authority, **prune the call-graph traversal at the dependency's declared
   interface methods.** Authority used *inside* the dependency's implementation is
   attributed to the **dependency**, not to us. Mechanism: enumerate the
   dependency's public-interface symbols and stop traversal there.

**Decision / how it lands in the design:**
- This is now **core MVP design**, not deferred. It resolves the previously-open
  "component-dependency authority re-absorption" question and makes the
  *component-dep vs absorbed-dep* distinction **mechanically precise**:
  - **Component dependency** → traversal **pruned** at its declared interface →
    authority NOT absorbed (it belongs to the dependency's own contract).
  - **Absorbed dependency** → traversal **not pruned** → authority absorbed and
    surfaced by us (Capslock's default transitivity).
- **Mechanism (feasibility confirmed in Capslock):** generate a per-analysis custom
  capability map that marks each direct component dependency's declared interface
  symbols `CAPABILITY_SAFE` (which "terminates further analysis"), then run the
  Capslock analysis on our component. Func-key format:
  `func <path>.<Name>` / `func (*<path>.<Type>).<Method>`.
- **The interface-file-strictness question (my open item #3) is answered by this.**
  Interface files define the *declared interface set*. A symbol that is Go-exported
  but NOT in an interface file is **architecture-private**: internal cross-package
  exports within the component are fine, but **no code outside the component may
  call them** (this merges the old FR4 "leaked symbol" and FR5 "private-impl
  protection" into one cross-component call-boundary graph property — the same
  property seen from the dependent's side vs the dependency's side).
- **Soundness is compositional (modular reasoning, concept §4):** pruning trusts the
  dependency's *declared* contract; if the dependency actually exceeds its declared
  authority, *its own* conformance check fails. The whole graph is sound iff every
  component conforms to its own manifest.
- **Cost:** needs an SSA **call graph** (not just imports) to (a) find cross-
  component call edges and (b) drive the pruned capability analysis. Both come from
  one call-graph build; Capslock's `CapabilityGraph`/callgraph is the likely source.
  **Precision caveat:** dynamic dispatch through Go interfaces makes call-graph
  edges conservative (over-approximate); documented as a limitation.

---

## Provisional requirements derived from the rough idea (not yet user-confirmed)

These are being carried into the draft design and are open for revision:

- **Scope:** MVP is a **CLI conformance checker** run against a component manifest.
  Pillars **1 (architecture as code)** and **3 (ambient authority)** only.
  Contracts are **informal prose in comments** (pillar 2 not enforced). Pillar 4
  (data-flow) out of scope. Bazel rule generation is a **later** step, but the
  manifest's dependency + interface-file fields stay mechanically-generatable.
- **Ambient authority for MVP:** effectively forced **empty** (ambient-authority-
  free). A `declared_authority` field may exist in the schema but default-empty;
  MVP asserts the component uses none (per Q3 strictness).
- **Two kinds of dependencies:** manifest distinguishes **component dependencies**
  (another manifest, first-class architecture edge) from **absorbed impl-detail
  dependencies** (allowlisted import path, no manifest, authority absorbed &
  surfaced by the absorbing component — Capslock does this transitively for free).
- **Pillar-1 conformance checks:** (a) every import of the component's packages ∈
  {component deps ∪ absorbed deps ∪ allowed stdlib}; (b) exported symbols live only
  in declared interface files; (c) private-impl protection — free within one
  package via Go visibility; cross-package variant deferred past single-package MVP.
- **Repo layout:** language-specific tooling under `./go`; shared manifest
  `.proto` (+ generated code / parsing helpers) in a shared location so Rust can
  reuse it later.
- **Self-hosting:** the tool is itself decomposed into components with manifests.
  A **shell/loader** component holds ambient authority (reads manifest, loads
  packages, runs Capslock; declares FILES/EXEC/READ_SYSTEM_STATE); a **pure
  checker core** takes injected loaded data and is genuinely ambient-authority-free
  — this pure core is the self-hosting showcase.
- **Examples:** a small multi-component Go project, e.g. a CSV tool that returns the
  top row sorted by a column, split into an ambient-authority-free "logic"
  component and a "shell" that holds the file I/O — to demonstrate both a passing
  ambient-authority-free component and a failing (undeclared-authority) one.
