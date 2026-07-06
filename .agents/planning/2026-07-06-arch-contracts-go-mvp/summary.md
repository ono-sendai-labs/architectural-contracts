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
│   └── detailed-design.md            # the finalized design (standalone)
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
  set, which drives both the FR5 boundary check and the FR5b prune set.
- **Compositional soundness:** pruning trusts each dependency's declared contract;
  the graph is sound iff every component independently conforms (modular reasoning).
- **Layout:** `./go` is its own module; shared `.proto` lives in `./proto` for a
  future Rust toolchain; `.textproto` manifests.

## Known caveats (see design §11)

Call-graph precision (dynamic dispatch over-approximates edges; pruning must stay
conservative), compositional trust (needs every component checked), call-only
boundary coverage, and Capslock's analysis-soundness limits (reflection/unsafe/cgo,
which under strict policy fail rather than pass silently).

## Remaining open items (minor, non-blocking — design Appendix D)

1. Confirm the `CapabilityAnalyzer` port (vs direct library use).
2. CLI name (working name `archcheck`).
3. Call-graph algorithm (CHA / RTA / VTA, or reuse Capslock's).
4. Single-component check vs a whole-graph check mode.
5. Confirm the interface-strictness reading (exported-but-non-interface =
   architecture-private, not an error).

## Next steps

1. **Checkpoint & review.** Commit these artifacts (via `jj`) and review
   `design/detailed-design.md` in detail.
2. **Design → plan.** When the design is settled, run the **`design-to-plan`**
   workflow against `design/detailed-design.md` to produce an incremental
   implementation plan. A natural first slice: the pure core (`manifest`, `checker`,
   `report`, `capanalyzer` port) with hand-built inputs + a single-package
   ambient-authority-free example, before wiring the Capslock adapter and the
   call-graph-based FR5/FR5b boundary checks.
