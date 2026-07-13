# Rough Idea — arcc MVP, coco-first variant

Create a **variant design for `arcc`** (the Go architectural-conformance CLI)
that incorporates the principles of the **coco component-contract discipline**
(`.claude/skills/coco-component-design` and `.claude/skills/coco-references/concepts.md`).

Basis:

- **Requirements and background** come from the existing Go-MVP design at
  `.agents/planning/2026-07-06-arch-contracts-go-mvp/` (design reviewed twice,
  Step-0 Capslock spike executed) and from `docs/rationale-and-concepts.md`.
- That design already uses components (checked with arcc on itself to conform to
  declared dependency and ambient-authority constraints), **but without rigorous
  use of interface contracts**. We want a stronger design that leans into the
  coco approach: two-tier contracts, labeled clauses, explicit rely-sets,
  contract-driven testing, verified fakes.
- The **manifest proto** from the sibling draft implementation
  (`../architectural-contracts/proto/archcontracts/v1/component.proto`) is the
  basis for this design's manifest proto. The draft *implementation* itself is
  deliberately **not** taken as a basis (it does not incorporate coco).
- Implementation will be **greenfield in this jj workspace** (this branch has
  only `go/go.mod` and an empty examples tree).

A secondary purpose of the project is to **test-drive the coco skills**
(design, testing, review) on a real system — arcc itself.
