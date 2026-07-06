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
├── implementation/
│   └── plan.md                       # incremental implementation plan (Steps 0–10)
└── summary.md                        # this file
```

## What we're building

A **CLI conformance checker** (`archcheck`, working name) that verifies a Go
component's source against a machine-readable **manifest**, implementing two pillars
of the architecture-as-code concept:

- **Pillar 1 — Architecture as code:** the manifest declares the component's
  packages, interface files, dependencies, and (empty for the MVP) ambient
  authority. The checker enforces that imports/calls stay within declared
  dependencies and that only *declared-interface* symbols are called across
  component boundaries.
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
- **Component = a set of Go packages** named by import-path pattern/list.
- **Two dependency kinds made mechanically precise:** *component* dependencies are
  **pruned** at their declared interface (authority stays theirs); *absorbed*
  impl-detail dependencies are **not pruned** (authority absorbed and surfaced by
  the absorber). Pruning uses Capslock's `CAPABILITY_SAFE` directive — feasibility
  and key-format verified against `interesting/interesting.cm`.
- **One boundary property, two views:** "only call a dependency's declared
  interface" and "nothing outside a component calls its architecture-private
  symbols" are the same call-graph property; interface files define the declared
  set, which drives both the FR5 boundary check and the FR5b prune set. (Scope:
  the B-side view holds over *checked* consumers — see design §5.3b.)
- **Declared interface = closure (review A1):** interface-file declarations *plus*
  the method sets of interface-file types (incl. concrete implementations of
  interface-file interface types), because VTA resolves dispatch to concrete
  methods. Package `init`s are pruned per dependency and explicit `init`s must
  live in interface files (review A2).
- **Compositional soundness:** pruning trusts each dependency's declared contract;
  the graph is sound iff every component independently conforms (modular reasoning).
- **Pure `facts` component (review B7):** the fact data model lives in the core;
  the shell produces core-defined types — dependencies point inward.
- **Layout:** `./go` is its own module; shared `.proto` lives in `./proto` for a
  future Rust toolchain; `.textproto` manifests.

## Known caveats (see design §11)

Call-graph precision (VTA; dynamic dispatch over-approximates edges), the
higher-order boundary leak (func values crossing a pruned boundary can escape
attribution — warned on, principled fix is future work), generic-symbol matching
(MVP bracket-stripping normalization), compositional trust (needs every component
checked), call-only boundary coverage, and Capslock's analysis-soundness limits
(reflection/unsafe/cgo, which under strict policy fail rather than pass silently).

## Remaining open items (non-blocking — design Appendix D)

1. Confirm the `CapabilityAnalyzer` port (vs direct library use).
2. CLI name (working name `archcheck`).
3. Single-component check vs a whole-graph check mode.
4. Post-MVP research: callbacks as capabilities (review A3); robust
   generic-symbol matching (review A4).

## Next steps

1. ~~Checkpoint & review~~ **Done.** Design reviewed (see `design-review.md`,
   verified against the Capslock source); resolutions folded into
   `design/detailed-design.md` and `implementation/plan.md`.
2. **Implement.** Enter the **`plan-to-tasks`** workflow against
   `implementation/plan.md`, starting with **Step 0 — the Capslock validation
   spike** (execute the design's load-bearing assumptions on draft example
   packages before building the pure core).
