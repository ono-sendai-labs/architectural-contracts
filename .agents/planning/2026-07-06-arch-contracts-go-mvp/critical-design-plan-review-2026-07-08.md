# Critical Design/Plan Review — 2026-07-08

Scope: senior-engineer review of `design/detailed-design.md`,
`implementation/plan.md`, and supporting planning artifacts after the Step-0
Capslock spike and PR-review revisions. (Codex gpt-5.5/high)

## Overall verdict

The design is directionally sound. The pure-core/authority-shell split, the
component-dependency vs absorbed-dependency pruning model, whole-package
authority scope, and the Step-0 Capslock spike materially reduce implementation
risk.

The main remaining risk was document drift: the main design and summary still
contained superseded decisions (`archcheck`, package-pattern membership, silent
interface closure, open port choice) after the plan had moved to the current MVP
decisions (`arcc`, directory-root membership, interface-file well-formedness,
Capslock behind a port). That drift would confuse task generation and
implementation.

## Findings and resolutions

### 1. Pillar-1 boundary scope was overclaimed

The MVP boundary check is intentionally call-edge based. That is acceptable for
this MVP, but the main requirements text previously used broader wording such as
"no code outside a component may call/use private implementation" while the
caveats admitted that type uses, field access, and exported vars are not covered.

Resolution: the main design now states that FR5 is a call-boundary rule for the
MVP. Non-call communication is documented as an explicit post-MVP extension in
the caveats/appendix rather than contradicting the body.

### 2. Superseded choices remained in implementer-facing text

Stale references included:

- CLI name `archcheck` and layout `cmd/archcheck`.
- Component membership by package list/pattern.
- Declared-interface "closure" as the current rule.
- `CapabilityAnalyzer` port as still open.
- The summary's implementation status and step count.

Resolution: implementer-facing body text is updated to current decisions only.
Superseded choices are retained as historical rationale in appendix/decision-log
sections where useful.

### 3. Dependency-resolution API lacked required context

`ResolveDependencyInterface(dep)` was too small for the path semantics it must
enforce. Dependency manifest paths are relative to the declaring component root,
and overlap checks require both the analyzed root and dependency root.

Resolution: the design and plan now make the shell API take explicit
`declaringRoot` and `analyzedRoot` context.

### 4. "Policy" overloaded checker policy and Capslock classifier config

The docs used `StrictPolicy` language both for the checker allow/warn decision
and the Capslock classifier configuration. These are separate concepts:

- `CapabilityPolicy`: checker decision over emitted findings.
- `ClassifierConfig`: adapter-side Capslock classifier construction
  (`excludeUnanalyzed`, minting-not-use overrides, prune map).

Resolution: wording now separates the two.

### 5. Shared package loading claim was stronger than the port shape

The design claimed "one packages.Load feeds both pillars," while the analyzer
port carries package names/configuration rather than already-loaded package
objects. That claim is not guaranteed by the interface.

Resolution: the design now says the shell uses equivalent package-load settings
for both pillars in the MVP, and a shared-loaded-package adapter is a future
optimization, not an implementer promise.

