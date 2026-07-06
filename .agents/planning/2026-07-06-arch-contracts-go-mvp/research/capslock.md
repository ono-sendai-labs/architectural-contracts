# Research — Capslock (capability analysis for Go)

Source: `../../external/capslock/` (module `github.com/google/capslock`, Go 1.25).
Upstream: https://github.com/google/capslock

## What it does

Capslock is a **capability analysis** CLI/library for Go. It classifies which
*privileged operations* ("capabilities") a package can reach by following
**transitive calls** through the SSA call graph down to privileged standard
library operations (and a few type-based signals like `unsafe.Pointer`,
`reflect.Value`). Motivation is supply-chain risk / principle of least privilege.

This is exactly the §5.4(a) "verification — static call-graph policy" mechanism
named in the concept doc.

## Capability taxonomy (`proto/capability.proto`, `Capability` enum)

14 capabilities, grouped by how we'd treat them for "ambient-authority-free":

**True ambient authority (a component claiming none must have zero of these):**
- `CAPABILITY_FILES` — read/write filesystem, perms, links, dirs.
- `CAPABILITY_NETWORK` — connections, sockets, listen.
- `CAPABILITY_READ_SYSTEM_STATE` — env vars, interfaces, pid/cwd/user.
- `CAPABILITY_MODIFY_SYSTEM_STATE` — chdir, setenv, signal handlers.
- `CAPABILITY_OPERATING_SYSTEM` — catch-all for `os` pkg.
- `CAPABILITY_SYSTEM_CALLS` — direct syscalls (≈ arbitrary code).
- `CAPABILITY_EXEC` — run other programs (`os/exec`).
- `CAPABILITY_RUNTIME` — mutate GC/stack/threading/panic behavior.

**"Analysis-defeating" signals (arguably also disqualify an
ambient-authority-free claim, because they can hide authority from the analyzer):**
- `CAPABILITY_ARBITRARY_EXECUTION`, `CAPABILITY_CGO`, `CAPABILITY_UNSAFE_POINTER`,
  `CAPABILITY_REFLECT`, `CAPABILITY_UNANALYZED`.

**Control values:** `CAPABILITY_UNSPECIFIED` (0), `CAPABILITY_SAFE` (explicit
allowlist that terminates analysis).

> **Design point:** "ambient-authority-free" ≈ *empty capability set*. But we must
> decide whether the analysis-defeating signals (reflect/unsafe/cgo/unanalyzed)
> also fail the check. A strict prototype should treat them as violations (or at
> least warnings), since they are holes in the guarantee.

## Programmatic (library) API — we can consume Capslock in-process

The CLI in `cmd/capslock` is a thin wrapper over the `analyzer` package. Key
exported surface:

- `analyzer.LoadConfig{BuildTags, GOOS, GOARCH}` + `analyzer.LoadPackages(names, cfg)
  → []*packages.Package`. Loads via `golang.org/x/tools/go/packages` with a fixed
  `PackagesLoadModeNeeded` (needs types, syntax, imports, deps, module).
  Supports `...` wildcards. **Uses `go list` under the hood → ambient authority.**
- `analyzer.GetQueriedPackages(pkgs) → map[*types.Package]struct{}` — restricts
  reporting to the queried packages (not their deps).
- **`analyzer.GetCapabilityInfo(pkgs, queried, config) → *cpb.CapabilityInfoList`**
  — the workhorse. Each `CapabilityInfo` = {packageName, capabilityName, an
  example call `path` of `Function` frames with file:line:col, capabilityType
  DIRECT|TRANSITIVE}. **If this list is empty for a component's packages, the
  component is ambient-authority-free.**
- `Config.Granularity`: `function` (default; one entry per (capability, function)),
  `package` (one per (capability, package)), `intermediate`.
- `Config.CapabilitySet` (`analyzer.NewCapabilitySet("FILES,NETWORK")`, supports
  `-` negation) filters which capabilities are considered.
- `analyzer.CapabilityGraph(pkgs, queried, config, nodeFn, callEdgeFn, capEdgeFn,
  filter)` — exposes the **full call graph** with caller→callee edges. This is the
  hook we'd use later for Appendix-A.1 checks ("no call edge into a dependency's
  non-interface symbol", "nothing outside a component calls its private impl").
- Custom classification: `interesting.LoadClassifier(path, reader, disableBuiltin)`
  loads a custom capability map (`.cm` file, e.g. `interesting/interesting.cm`);
  `CAPABILITY_SAFE` allowlists a function/package. Lets us tune/override how
  specific packages are classified.

Output formats from the CLI: default human summary, `-output=v` (verbose, with
call paths), `-output=json` (the `CapabilityInfoList` proto as JSON), `-output=m`
(list of capability names), `-output=graph` (Graphviz), `-output=compare` (diff
vs a saved JSON — used by `capslock-git-diff`).

### Consequence: subprocess vs library

- **Library** is clean and gives us structured protos directly (no JSON round-trip),
  plus the call-graph hook for future pillar-1 call-edge checks. Cost: our tool
  takes a direct Go dependency on Capslock's `analyzer`/`interesting`/`proto`.
- **Subprocess** (`capslock -packages X -output=json`) is looser coupling, but we
  lose the call-graph hook and pay JSON parsing. Either works for the MVP's
  ambient-authority check.

## How Capslock relates to "absorption" of third-party deps

Capslock's analysis is **transitively absorbing by construction**: if component X
calls a third-party CSV parser that reads files, Capslock attributes
`CAPABILITY_FILES` to X's function (via a transitive path). This *is* the
"absorption of ambient authority" the rough idea describes — an impl-detail
dependency's use of authority automatically surfaces as the component's authority.
No extra mechanism is needed for the ambient-authority dimension of absorption.

The subtlety is at **component→component** edges: if X depends on *component* Y
(which declares and owns its own authority), we must **not** re-absorb Y's
authority into X — Y already accounts for it behind its contract. Capslock's raw
transitivity would absorb it anyway, so we **prune the traversal at Y's declared
interface**. This is now a *core design decision* (see design §5.3b/§5.4 and
idea-honing Round 3), and it is what makes "component dep vs absorbed dep"
mechanically precise: **component deps are pruned, absorbed deps are not.**

### Boundary-pruning mechanism — CONFIRMED FEASIBLE

`interesting.LoadClassifier` accepts a custom **capability map** that overrides the
builtin classifications. Two relevant directives (from `interesting.go`):
- `func <function-key> CAPABILITY_SAFE` — `CAPABILITY_SAFE` "explicitly terminates
  further analysis," so a function marked SAFE is a **leaf**: the traversal does not
  descend into it and no authority behind it is attributed to callers. This is the
  pruning primitive.
- `ignore_edge <caller-key> <callee-key>` — drop a specific call edge (finer-grained
  alternative).

**Function-key format** (verified against `interesting/interesting.cm`):
- plain func: `func compress/bzip2.newHuffmanTree ...`  → `<import/path>.<Name>`
- method:     `func (*bytes.Buffer).WriteString ...`    → `func (*<import/path>.<Type>).<Method>`

These match `ssa.Function.String()` / `go/types` output, so we can generate them
from the dependency's declared-interface symbols.

**Plan:** when analyzing component X, generate a per-run custom map marking every
*direct component dependency's* declared-interface symbol `CAPABILITY_SAFE`, load it
with `LoadClassifier(..., excludeBuiltin=false)` (merges with builtins), and run the
analysis. Absorbed deps are simply *not* in the map, so they remain absorbed. Each
component is analyzed with its **own** map (Y is not pruned when we analyze Y).

**Soundness:** pruning trusts Y's *declared* contract; if Y actually exceeds its
declared authority, Y's own conformance run catches it. The graph is sound iff every
component is checked (compositional / modular reasoning, concept §4).

For the strict-empty MVP where deps are also authority-free, the totals are empty
either way — but the mechanism is what lets a component legitimately depend on a
FILES-using component while remaining ambient-authority-free itself.

## Self-hosting tension (important)

Capslock **needs ambient authority itself**: `LoadPackages` shells out to `go
list` (`os/exec`, files, env). So any component of *our* tool that calls Capslock
is **not** ambient-authority-free. The clean resolution for self-hosting:
- A **shell/loader** component holds the ambient authority (reads the manifest,
  loads packages, runs Capslock). It declares `FILES`/`EXEC`/`READ_SYSTEM_STATE`.
- A **pure policy/checker core** takes the already-loaded `CapabilityInfoList` +
  import facts as *injected data* and decides pass/fail. This core can be
  genuinely ambient-authority-free and is the part we showcase as self-hosted.

## Caveats (from `docs/caveats.md`, to read fully later)

Static analysis is conservative/incomplete: reflection and `unsafe` can hide
behavior (hence the dedicated capabilities); `CAPABILITY_UNANALYZED` marks
give-ups; results depend on GOOS/GOARCH/build tags. These bound how strong an
"ambient-authority-free" guarantee we can truthfully claim.
