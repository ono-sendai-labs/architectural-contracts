# Summary — Architectural Contracts MVP (Go)

Interactive design complete. This document indexes every artifact produced and
gives a one-page overview of the design and next steps.

## Artifacts created

```
.agents/planning/2026-07-06-arch-contracts-go-mvp/
├── rough-idea.md                     # the original concept
├── idea-honing.md                    # requirements Q&A + decision log (Rounds 1–3)
├── research/
│   ├── capslock.md                   # Capslock API, capabilities, absorption,
│   │                                 #   verified SAFE-based boundary-pruning mechanism
│   └── go-component-model.md         # mapping "component" onto Go; pillar-1 tooling
├── design/
│   └── detailed-design.md            # the design (standalone; revised per review)
├── design-review.md                  # senior review (verified against Capslock
│                                     #   source) + resolutions, folded into design/plan
├── critical-design-plan-review-2026-07-08.md
│                                     # follow-up critical review + cleanup record
├── implementation/
│   └── plan.md                       # incremental implementation plan (Steps 0–11)
└── summary.md                        # this file
```

## What we're building

A **CLI conformance checker** (`arcc`) that verifies a Go
component's source against a machine-readable **manifest**, implementing two pillars
of the architecture-as-code concept:

- **Pillar 1 — Architecture as code:** the manifest declares the component's
  interface files, dependencies, and ambient authority. Component membership is
  directory-based: the manifest sits at the component root, and every Go package
  beneath that directory belongs to the component. The checker enforces declared
  imports and, for MVP boundary enforcement, that cross-component **call edges**
  land only on declared-interface symbols.
- **Pillar 3 — Ambient authority:** using Capslock's transitive call-graph
  analysis, the component provably uses no undeclared ambient authority. The MVP
  default is **strict** (any capability fails), via an injected `CapabilityPolicy`
  whose allow-set is sourced from the manifest's `declared_authority` — the seam to
  future per-manifest configuration.

Contracts (Pillar 2) are informal prose in interface-file comments; data-flow
(Pillar 4) and the Bazel rule are out of scope for the MVP.

## Key design decisions

- **Pure core / authority shell.** A `checker` that is a pure function of
  `(Manifest, PackageFacts, DepInterfaces, CapabilityFindings, Policy)`, wrapped by
  a shell that holds all file/`go list`/Capslock authority. The pure core is itself
  a genuine **ambient-authority-free component** — the self-hosting showcase.
- **Capslock as a Go library** behind a small `CapabilityAnalyzer` port.
- **Component = manifest-directory subtree.** There is no package list in the
  manifest; the manifest's directory defines the component root.
- **Two dependency kinds made mechanically precise:** *component* dependencies are
  **pruned** at their declared interface (authority stays theirs); *absorbed*
  impl-detail dependencies are **not pruned** (authority absorbed and surfaced by
  the absorber). Pruning uses Capslock's `CAPABILITY_SAFE` directive — feasibility
  and key-format verified against `interesting/interesting.cm`.
- **One call-boundary property, two views:** "only call a dependency's declared
  interface" and "checked consumers do not call a component's architecture-private
  callable symbols" are the same call-graph property. The same declared-interface
  symbol set drives both the FR5 call-boundary check and the FR5b prune set.
- **Declared interface = interface-file declarations plus well-formedness.**
  Interface-surface methods and explicit `init`s must be declared in interface
  files. Concrete implementations of interface-file interface types are added to
  the boundary/prune set because VTA resolves dispatch to concrete methods.
- **Compositional soundness:** pruning trusts each dependency's declared contract;
  the graph is sound iff every component independently conforms (modular reasoning).
- **Pure `facts` component (review B7):** the fact data model lives in the core;
  the shell produces core-defined types — dependencies point inward.
- **Layout:** `./go` is its own module; shared `.proto` lives in `./proto` for a
  future Rust toolchain; `.textproto` manifests.

## Known caveats (see design §11)

Call-graph precision (VTA; dynamic dispatch over-approximates edges), MVP
call-only boundary coverage (type use, field access, exported vars, and other
non-call communication are post-MVP), the higher-order boundary leak (func values
crossing a pruned boundary can escape attribution — warned on, principled fix is
future work), generic-symbol matching (MVP bracket-stripping normalization),
compositional trust (needs every component checked), and Capslock's
analysis-soundness limits (reflection/unsafe/cgo, which under strict policy fail
rather than pass silently).

## Remaining open items (non-blocking — design Appendix D)

1. Single-component check vs a whole-graph check mode.
2. Post-MVP research: callbacks as capabilities (review A3); robust
   generic-symbol matching (review A4).

## Next steps

1. ~~Checkpoint & review~~ **Done.** Design reviewed (see `design-review.md`,
   verified against the Capslock source); resolutions folded into
   `design/detailed-design.md` and `implementation/plan.md`.
2. **Review refreshed.** A critical follow-up review is recorded in
   `critical-design-plan-review-2026-07-08.md`; the design and plan have been
   cleaned so their bodies reflect current decisions only.
3. **Implement.** Enter the **`plan-to-tasks`** workflow against
   `implementation/plan.md`, starting with **Step 1 — Project scaffold &
   developer tooling**. Step 0, the Capslock validation spike, is complete.
