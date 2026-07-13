# Idea Honing — arcc coco-first MVP

Requirements Q&A. Round 1 asked before the user stepped away for the evening;
the remainder of the workflow (research → design → plan → review) runs
autonomously on these answers.

## Round 1 (2026-07-12)

### Q1 — How coco-aware should the manifest schema and the arcc checker become?

Context: the existing proto deliberately has no contract field (contracts are
doc-comment prose only, FR10). The coco concepts doc expects the manifest to
name the contract file(s) that carry the Tier-2 contract.

Options considered: (a) designate + light checks (existence + rely-set
references validated), (b) designation only (existence check), (c) deeper
structural validation of contract files, (d) manifest unchanged.

**Answer: designation only (b).** The manifest is extended to allow naming
Tier-2 contract files. arcc checks nothing about their content. Rationale
(user): *"I don't want to impose a structure on contracts in arcc just yet. We
can make up a structure for **this** project specifically (and document it as a
project-specific convention), but I don't want arcc to impose contract
structure on **other** projects it will be used on."*

Consequences for the design:
- Proto gains a `contract_files` field (designation only).
- arcc verifies that designated contract files exist; nothing more.
- A **project-specific convention document** defines the contract structure
  (clause labels, rely-set section, clause→test map) used by arcc's own
  components and examples — convention, not tool policy.

### Q2 — Implementation baseline?

**Answer: greenfield in this workspace.** The design and plan target a fresh
implementation on this branch, structured coco-first from the start. The draft
implementation in `../architectural-contracts` is background only (its manifest
proto is the schema basis, per the rough idea).

### Q3 — How much coco discipline should the MVP implementation itself carry?

**Answer: full discipline.** Every component gets Tier-1 doc-comment contracts
with labeled clauses plus a Tier-2 `component-contract.md` with rely-set,
authority rationale, and clause→test map. Contract test suites per
`coco-contract-testing`, and a verified fake for the CapabilityAnalyzer port.

### Q4 — Autonomous self-review pass?

**Answer: yes, review + fold in fixes — but `jj commit` the draft design+plan
*before* making the fixes.** Part of the project's purpose is to test-drive the
coco skills; the commit boundary preserves a record of what the review skill
found and changed.

## Carried-over requirements (from the 2026-07-06 go-mvp design)

Functional scope is inherited rather than re-derived: Pillars 1 and 3 for Go
(manifest conformance, interface well-formedness, call-boundary check,
Capslock-based ambient-authority enforcement with component-dependency pruning
and absorption), the CSV example, self-hosting, textproto manifests, `./go` as
its own module, CLI name `arcc`. The coco variant changes *how the system is
designed and specified* (components with two-tier contracts, rely-sets,
contract-driven tests) and makes the *manifest contract-aware*
(designation-only `contract_files`); it does not change the checker's
enforcement scope except where noted in the design.
