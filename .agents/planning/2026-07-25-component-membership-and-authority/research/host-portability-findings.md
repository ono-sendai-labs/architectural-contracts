# Research — what a second host's Go rules do to this design

**Question.** The design makes several claims that depend on what a host build
system exposes: that the target platform is readable from a provider, that the
aspect closure is the real closure, that a component's members can be named as
labels, that an emitter can supply a trustworthy standard-library bit. Which hold
on a host whose Go rules differ from the reference ruleset?

**Method.** The design was evaluated against an in-progress port to a second,
Bazel-compatible build system with an independently written Go ruleset, probing
empirically where a claim was host-dependent. Findings are stated as properties of
*a host with characteristic X* rather than of that host, since what matters to the
design is the shape of the constraint.

Everything relevant to the design is recorded in `../idea-honing.md` Q18.

## F1 — A layout can list files for several mutually exclusive platforms at once

In a generated layout for a component wrapping a platform-portable logging
library, one package's file list carried the OS-specific variants for two named
operating systems, a POSIX variant, a "neither" fallback and a negated-OS variant
— **all five simultaneously**. The host's build metadata records each target's
*declared* source set, not the set the compiler selected.

Two consequences:

1. The loader's build-constraint filter is **load-bearing**, not a belt-and-braces
   pass. T1 is not a tidiness fix on such a host.
2. The emitter derives that package's import list by parsing those same files, so
   the declared imports are the **union across platforms**. A layout in that state
   is internally inconsistent with its own platform block: the loader filters
   files while the import list still names imports no surviving file contributes.
   This is the concrete failure T8 exists to catch.

## F2 — A host's Go providers can be field-less placeholders

The provider identifying a Go target on this host is documented as a placeholder
with an **empty field list**, present only so rules can detect Go-ness. The
neighbouring providers carry package metadata and closure artifacts but **no
platform, no build tags, no cgo flag**.

So `go_build_platform` has no provider to read, and a fixed constant is not a
fallback but the only available implementation. T2's contract must admit a constant
as *conforming* rather than as a degradation, and the reference ruleset's
`GoInfo.mode` is an illustrative example, not the specification.

## F3 — Toolchain-injected packages can be layout-visible but Starlark-invisible

Generated serialization code on this host reaches its runtime **through the
toolchain**, not through traversable dependency edges. The runtime — around 35
packages — is therefore absent from the closure an aspect can walk at analysis
time, while being fully present in the build metadata the emitter reads at
execution time.

Any predicate of the form "attach iff the closure contains one of these packages"
is **unevaluable in the analysis phase for exactly the packages it was written
for**. Hence A7's seam admitting an unconditional "always attach".

## F4 — Injected packages can be visibility-gated behind allowlists

Within that runtime, the internal packages are restricted to the runtime's own
subtree and its principal entry point is gated by an explicit consumer allowlist.
Naming them as `members` labels would require visibility grants across
high-traffic build files owned by another team.

This is what makes label-only membership unimplementable for the case that most
needs it, and why both `PACKAGE_SURFACE` membership and the infra registry must
accept **import-path patterns**.

## F5 — Injected packages are ubiquitous, not exotic

On this host **every** Go target's closure contains an exit-hook registrar and a
coverage-instrumentation package, injected by the toolchain. Unlike F3 these *are*
visible to the analysis-phase closure walk, and are classified as ordinary members
today — they appear in generated layouts as analysis roots.

Under declared membership they land on the "remainder is an error" branch
immediately, in **every** component, before any host-specific runtime is involved.
So an empty infra registry is a placeholder, not a default: the registry is the
first thing an adopter touches, not the last.

## F6 — An emitter asked for a standard-library bit will re-implement the path policy

This host's layout generator **already** carries a local copy of the host
standard-library path heuristic, with a comment stating it is kept in sync with the
loader's so the two agree on which import edges refer to the SDK.

That is the natural implementation of a `is_stdlib` layout field, and it is the one
that makes an agreement check **vacuous** — a checksum over a copy of the function
being checked. It can never disagree, so the load error can never fire, and layout
mode stays single-signal with added ceremony that makes it look otherwise. Hence
T4a: the bit must be derived from build-graph provenance, which is information the
path policy does not have and cannot reconstruct.

## F7 — Symbolic-macro constraints reproduce on an independent implementation

Verified by building and testing a component and its check through a symbolic-macro
definition on this host:

- `macro()` with `inherit_attrs = "common"` and `configurable = False` attributes
  is supported and works unchanged. A `name + ".check"` target is legal.
- `native.subpackages()` exists with the **same frontier semantics**
  (`members-glob-expansion.md` F2) and is **likewise rejected inside a symbolic
  macro implementation**.

So `members-glob-expansion.md`'s finding 3 and the B1 conclusion are properties of
the macro model rather than of one implementation — which is the strongest
available evidence that shipping no expansion helper is the right call.

The package→target naming assumption does **not** transfer: this host has no
convention that a package's default target is named after its directory. That is
the concrete case B2's reworded rationale points at.

## Consequences for the design

| Finding | Requirement it changes |
|---|---|
| F1 | **T8 (new)** — the platform block governs emitters; the loader validates it |
| F2 | **T2** — a constant is a conforming seam result |
| F3 | **A7** — the attachment predicate is a seam; unconditional attachment is legal |
| F4 | **A6, A7** — pattern membership, and a pattern-accepting registry |
| F5 | **A7** — an empty registry is a placeholder; the error message must point at it |
| F6 | **T4a (new)** — the layout's stdlib bit must be provenance-derived |
| F7 | **B1, B2** — no expansion helper anywhere; B2's rationale rewritten |
