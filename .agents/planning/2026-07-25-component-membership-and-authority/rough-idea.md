# Rough idea — component membership and authority attribution

**Date:** 2026-07-25
**Source input:** `.agents/planning/2026-07-24-callback-authority-attribution/callback-authority-attribution.md`
(read that note first; it is the substance of Parts 1 and 2 below and is not
duplicated here)

## Origin

arcc was taken into a large Bazel monorepo whose Go rules differ from rules_go.
That work produced the commits `nnnnztsx..ulluq` on `dev/exp-go-bazel-mvp`:
host-portability seams (`hostpolicy`, `go_adapter.bzl`), a build-constraint
filtering fix, a vendored-stdlib import fix, and two after-the-fact design
records (`2026-07-23-host-portability-seams`, `2026-07-24-layout-completeness`).
Running arcc on real code in that monorepo then surfaced deeper model problems,
written up as the source note.

## Scope of this design

Three groups, designed as one coherent pass over attribution soundness.

### Part 1 — authority that escapes through a callback

Absorbed code that exercises ambient authority, exposed only as a function value
handed to a dependency, is analyzed by no one and passes while declaring
nothing. Options A–D in the source note.

### Part 2 — membership by location, not by absorbed closure

`absorbed_deps` pulls in each label's entire transitive tree (pruned only by
`component_deps`), making real membership implicit and drifting as
`component_deps` are edited. Proposal: an explicit, non-transitive, location-based
`members` attribute; `absorbed_deps` reverts to a narrow escape hatch for code
outside the component's own tree.

Shared crux of Parts 1 and 2: scoping the interface-file well-formedness rules
(explicit-`init`-in-interface-file, exported-method placement) to the interface
rather than to all members.

### Part 3 — attribution risks surfaced while reviewing the port

1. **Implicit analysis platform.** `filterByBuildConstraints` uses
   `build.Default`: the arcc binary's own GOOS/GOARCH, no host build tags, and
   `CgoEnabled` from the ambient environment. A file gated on a host build tag
   that the real build *does* compile gets dropped, under-specifying the package
   → `IllTyped` → SSA/VTA skipped → silent pass. The layout records no platform,
   so it is not a complete description of the analysis; env vars change results
   inside a check advertised as hermetic and cacheable; and `declared_authority`
   is platform-agnostic while verification only ever covers one platform.
2. **Silently skipped interface files.** `ValidateInterfaceFiles` now skips
   build-constraint-excluded interface files with no diagnostic. A typo'd tag
   shrinks the declared surface silently; a fully platform-gated interface passes
   vacuously.
3. **`Module == nil ⇒ stdlib`.** The checker's dependency-boundary skip now
   consumes `facts.StdlibImports`, built from `isStdlibPackage`. Any load where
   non-stdlib deps carry no module metadata (a third-party `GOPACKAGESDRIVER`,
   a non-module load, load errors) classifies everything as standard library, so
   *every* dependency-boundary check is skipped and the component passes. Latent
   (layout mode branches to `hostpolicy`) but a total fail-open with no signal.
4. **The undocumented `CanonicalizePath` invariant.** Canonical paths are fed
   back into `packages.Load` as patterns and must match the symbol keys Capslock
   derives from the loader's own paths, so the mapper must be *identity on
   loader-reported paths*. `hostpolicy`'s contract only requires total +
   idempotent; the stronger requirement lives only in a planning doc.
5. **`canonicalizeSymbol` rewrites one occurrence,** so a generic symbol's
   type-argument packages keep the host namespace and prune/boundary keys can
   miss silently.
6. **Smaller:** build-constraint filtering can empty a package's file list with
   no error; the `vendor/`-prefix relaxation in `ValidateAndResolve` phase 3
   applies to all packages rather than only the standard library.

## Constraint discovered while reviewing

`capanalyzer.AnalyzeRequest` documents "analyzes every function in Packages
(Capslock's native behavior)" and takes **package paths only**. So the analysis
unit is the *package*, not the function: every function in an analyzed package is
a start point, and the source note's Part 1 gap is precisely "absorbed packages
are not in the analyzed set." Option B ("add functions whose *value* is reachable
to the roots") is therefore not expressible through the port as it stands.

## Known-broken, out of scope but recorded

`just selfcheck` fails at tip: `hostpolicy` is undeclared in
`go/internal/goanalysis/component.textproto` and `go/cmd/arcc/component.textproto`.
It went unnoticed because the monorepo environment is Bazel-only, so the
`just`-driven Go selfcheck never ran there. Follow-on: **a bazelified self-check**,
so arcc's own dogfooding runs under Bazel and a Bazel-only host cannot silently
skip it.
