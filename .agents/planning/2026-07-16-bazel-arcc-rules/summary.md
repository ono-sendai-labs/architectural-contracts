# Project Summary: arcc as a Bazel Rule (`go_component`)

Interactive-design workflow completed 2026-07-16. Design finalized as drafted; user review pending — iterate on `design/detailed-design.md` if needed before planning.

## Artifacts

- `rough-idea.md` — the initial concept: `go_architectural_component` rule sketch plus open questions on manifest evolution, rule shape, and naming.
- `idea-honing.md` — Q1–Q13 requirements clarification with rationales and alternatives.
- `research/arcc-current-state.md` — manifest schema, arcc's `go/packages`/`go list` loading path, Capslock constraints, implications for Bazel.
- `research/bazel-rules-landscape.md` — rules_go providers, macros, test-vs-validation enforcement, the hermeticity gap (rules_go#1996), candidate loading strategies, naming conventions; sources linked.
- `research/spike-findings.md` — spike results on Bazel 9.2.0 / rules_go 0.61.1; all four load-bearing mechanics confirmed.
- `research/spike/` — the runnable spike workspace (rule + macro + passing tests).
- `design/detailed-design.md` — the finalized design document.
- `summary.md` — this document.

## Design overview

A `go_component` symbolic macro (Bazel 8+; rules in top-level `bazel_rules/`, repo becomes bzlmod module `rules_arcc`) attaches an architectural-component declaration to exactly one existing `go_library`. It expands into (a) a component rule that generates the `component.textproto` manifest from Bazel's dependency graph (via a dep-walking aspect), enforces membership/absorption consistency at analysis time, and forwards `GoInfo`/`GoArchive` so dependents can use the component target directly, and (b) a `.check` test running `arcc check`.

Phased delivery: **Phase 1** ships a non-hermetic `local` check test against the real workspace, requiring only two additive arcc changes (`component_root` manifest field, `--source-root` flag) — the fastest path to trialing in a real project. **Phase 2** makes checks hermetic via a package-layout JSON and an arcc self-exec `GOPACKAGESDRIVER` loading mode, plus explicit membership. Bazelified csvtool examples serve as integration tests; arcc is built from source via rules_go for consumers.

## Open items for the user's review

1. **§7.3 — absorption semantics** (explicit-direct vs. fully-derived): design specifies explicit-direct with a documented, cheap-to-flip alternative. Decision deferred to review.
2. **§7.4 — load path** `@rules_arcc//bazel_rules:defs.bzl` (sketch's `//go:def.bzl` collides with the Go module dir).
3. **§4.1 — repo bzlmod-ification footprint** (root `MODULE.bazel` + Gazelle BUILD files under `go/`).

## Next steps

1. Review `design/detailed-design.md`, especially the three items above; iterate here if needed.
2. Run the `design-to-plan` workflow to produce the implementation plan (suggested phase-1 slicing is implicit in §3.3/§4).
3. After Phase 1 lands: trial in the target external project, then revisit the absorption decision (§7.3) and Phase 2 scheduling with real-world feedback.
