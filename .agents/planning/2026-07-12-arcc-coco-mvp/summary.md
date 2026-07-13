# Summary — arcc MVP, coco-first variant

## Artifacts

```
.agents/planning/2026-07-12-arcc-coco-mvp/
├── rough-idea.md            # the concept: coco-first redesign of the arcc go-mvp
├── idea-honing.md           # Round-1 Q&A (manifest scope, baseline, rigor, review process)
├── research/
│   └── coco-mapping.md      # delta analysis: what coco adds to the go-mvp design;
│                            #   extracted implicit relies; design levers chosen
├── design/
│   └── detailed-design.md   # the design (standalone; carries go-mvp mechanics by summary)
├── implementation/
│   └── plan.md              # incremental implementation plan (design-to-plan)
└── summary.md               # this file
```

## What this is

A redesign of the `arcc` conformance checker under the **coco
component-contract discipline**. Functional scope is inherited from the
2026-07-06 go-mvp design (Pillars 1 & 3, Capslock, CSV example, self-hosting);
the variant changes:

- **Manifest (designation-only contract awareness):** `contract_files` field
  added to the proto; arcc checks the named files exist (new violation
  `CONTRACT_FILE_MISSING`, R10) and never inspects content. Contract structure
  is a per-project convention (`docs/coco-conventions.md`), not tool policy.
- **Two-tier contracts everywhere:** every arcc and example component gets
  Tier-1 labeled clauses (`PRE:`/`POST:`/`INV:`/`HIST:`) in interface doc
  comments and a Tier-2 `component-contract.md` (clause elaborations,
  rely-set, authority rationale, clause→test map).
- **Implicit relies made explicit:** the go-mvp's load-bearing field comments
  ("Caps arrive already pruned", the shared symbol-key form) are promoted to
  named clauses; `checker.Check` gains four PREs discharged by a table in
  `cli`'s Tier-2; `facts` now owns the symbol vocabulary (ADR-2); a full
  compositional-soundness walk (design §7) shows no unmet relies, with the two
  external relies (Capslock, x/tools) flagged and tied to re-validation
  checklists.
- **Contract-driven testing:** clause→test maps; a **verified fake** for the
  `CapabilityAnalyzer` port certified by a shared contract suite run against
  both fake and real adapter; hermetic orchestration tests via
  consumer-defined ports.

## Process record (test-driving the coco skills)

Per the user's instructions: design + plan drafted autonomously → **jj commit
of the draft** → self-review pass (coco-contract-review-style soundness checks
+ critical review) recorded → fixes folded in → second commit. See
`design-review.md` (created during the review pass) and the jj log.

## Next steps

1. User reviews `design/detailed-design.md` and `implementation/plan.md`
   together, plus the review record.
2. Enter `plan-to-tasks` at Step 1 of the plan.
