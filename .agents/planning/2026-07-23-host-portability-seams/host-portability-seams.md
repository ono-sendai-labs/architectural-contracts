# Host-portability seams (and a latent build-constraint bug)

**Date:** 2026-07-23
**Branch:** changes on top of `origin/dev/exp-go-bazel-mvp`
**Author:** discovered interactively with Claude (Opus 4.8) while importing arcc into
a Bazel monorepo (google3).

> This is an after-the-fact record, not a pre-design. The changes below were found
> while porting arcc + `rules_arcc` into a monorepo that rewrites import paths at
> build time. They fall into two groups: **portability seams** that let such a host
> adapt arcc by changing one file per concern instead of patching the pure core, and
> **one genuine bug fix** (build-constraint filtering) that the porting work surfaced
> but which affects upstream too.

## Commits

1. `a3451dd` — hostpolicy seam: import-path canonicalization + stdlib classification
2. `34b861b` — canonicalize package paths embedded in interface/call-edge symbols
3. `66a0cfb` — extract `go_adapter.bzl` as the single seam onto the host Go ruleset
4. `2ee2c2b` — filter layout sources by build constraints in the loader (the bug fix)

All four keep `just test`, `just test-integration`, and `just bazel-test` green. Their
defaults reproduce today's behavior exactly, so a plain `go/packages` + rules_go build
is unaffected; only a host that opts in sees a difference.

## Motivation: what a rewriting host breaks

A monorepo importing arcc as third-party typically rewrites import paths at build
time. The concrete host here does three things that arcc's core did not anticipate:

- **Prefix rewrite:** `github.com/ono-sendai-labs/architectural-contracts/go/...`
  becomes `<monorepo-prefix>/.../go/v/v0/...`.
- **"Doubled final segment":** a package `foo` in directory `.../foo` is imported as
  `.../foo/foo`. The same logical package therefore appears in *two leaf forms* across
  the loader (which reports the doubled form), the manifests (which may not), and the
  dependency-interface resolver.
- **Dotless top segment:** the synthetic top-level path segment contains no dot, which
  defeats the "first path segment has no dot ⇒ standard library" heuristic — every
  rewritten path is misread as stdlib.

Without seams, each of these forces edits scattered through the *pure* checker
(`normalizePkg`-style reconciliation, an `isStdlib` special case) and the Bazel rules
(a second copy of the stdlib heuristic, dual-keyed capability annotations). That is
exactly the kind of change that is painful to carry as a local patch across re-imports.

## Seam 1 — `go/internal/hostpolicy` (import paths + stdlib)

`a3451dd`. A new leaf package with two override-once package-level `var`s, matching the
existing `var osStat = os.Stat` seam style already used in `packagelayout`:

- `CanonicalizePath(string) string` — default **identity**. The shell funnels every
  path the checker compares through it: package import paths, a package's direct
  imports, dependency-interface packages, and absorbed-dependency patterns. A rewriting
  host sets this once to a total, idempotent mapper (all forms → one canonical form).
- `IsStdlibPath(string) bool` — default the **go-tool heuristic**. Stdlib classification
  for contexts where module metadata is absent — notably arcc's hermetic package-layout
  mode, where packages carry no `*packages.Module`. A host whose namespace has a dotless
  first segment overrides this to exclude itself.

Wiring (all no-ops under the defaults):

- `facts.PackageFacts` gains `StdlibImports`, the **loader-authoritative** set of stdlib
  imports in canonical form. The checker consumes it when present, and falls back to its
  own string heuristic only when it is nil (hand-built facts in unit tests). The loader
  has module/SDK metadata the pure checker lacks, so classification belongs there.
- `goanalysis` canonicalizes the package paths, imports, and dependency-interface
  packages it emits, and classifies stdlib via module metadata when available, else via
  `hostpolicy` (layout mode has no module info).
- `packagelayout.IsStdlib` and `app.go`'s absorbed-dependency paths route through the seam.

**Design decision — canonical form = the form the loader reports.** In the host that
means the doubled form. Then the loader's own output is already canonical (the mapper is
effectively identity on it, and must be idempotent), the capability analyzer's function
keys line up, and only *manifest-sourced* strings need real mapping. Picking the
undoubled form instead would have forced re-doubling everywhere the loader is the source
of truth.

**Why not thread a canonicalizer parameter through the call graph?** The checker is a
pure function and stays one; the canonicalization happens in the shell before facts reach
it. Package-level `var`s (not dependency injection) were chosen to match the existing
`osStat` seam and to let a host register overrides from a single `init` without touching
call sites.

## Seam 2 — symbol package canonicalization

`34b861b`. Seam 1 canonicalized *package* paths but not the package path embedded inside
*symbol keys* — interface symbols like `(*pkg/path.T).M` and `pkg/path.Fn`, and the VTA
call-edge caller/callee keys. Those must share the canonical namespace too: the FR5
boundary check compares call-edge callee packages against dependency-interface packages,
and the capability analyzer's prune keys (built from dependency-interface symbols) must
match the function keys Capslock derives from the loaded (canonical) packages.

`canonicalizeSymbol` rewrites only the package-path portion of a formatted symbol key
(reusing the existing package-extraction helper) and is applied where the shell mints
symbol keys: `getFuncSymbol`, `extractSymbols` (names + receiver keys), and the
concrete-method keys in `ResolveDependencyInterface`. Default identity → no-op upstream.
This is what lets a host reconcile capability/boundary symbols through the *same* single
`CanonicalizePath` override, rather than dual-keying capability annotations and
de-doubling symbols inside the pure checker.

## Seam 3 — `bazel_rules/go/private/go_adapter.bzl`

`66a0cfb`. arcc's Bazel rules bound to rules_go in three files (`aspect.bzl`,
`component.bzl`, `check.bzl`): the `GoInfo`/`GoArchive` providers, reading a target's
import path / sources / direct deps / cgo flag, forwarding a library's Go providers, and
reaching the Go SDK via `@rules_go//go:toolchain`. A host with a different Go ruleset had
to patch all three.

`go_adapter.bzl` is now the single file that loads `@rules_go`. It exports the provider
and toolchain symbols (`GO_PROVIDERS`, `GO_TOOLCHAINS`), target readers (`is_go_target`,
`go_importpath`, `go_library_srcs`, `go_target_info` — the former `_node_for`),
`forward_go_providers`, and SDK accessors (`go_sdk_root_file`, `go_sdk_srcs`). The three
rule files import only the adapter and are byte-identical between upstream and a host; the
host swaps one file. No behavior change — logic moved verbatim.

## Bug fix — build-constraint filtering in the layout loader

`2ee2c2b`. This is not a portability concern; it is a latent correctness bug the porting
work exposed.

**Symptom found:** a fixture package with `_linux.go` / `_windows.go` / `_darwin.go`
variants showed *all* variants in the aspect's srcs, on every platform.

**Root cause:** `GoInfo.srcs` is the **declared** source set, not the per-platform
**compiled** subset. rules_go applies build constraints in its *compiler action*, not in
the provider. So the aspect (correctly forwarding `GoInfo.srcs`) hands arcc every OS
variant; arcc writes them all into the package layout and type-checks them; the
wrong-platform files make the package `IllTyped`; `IllTyped` packages are skipped by the
SSA/VTA pass; capability findings come back empty; and the check **silently passes**. A
fail-open on any package with per-platform sources. Upstream never tripped it only because
no testdata package had platform variants.

**Fix (chosen after weighing alternatives):** filter in arcc's Go loader rather than in
Bazel. `packagelayout.ValidateAndResolve`, after resolving source paths, drops files
excluded by build constraints via `build.Default.MatchFile`, which honors *both* filename
suffixes (`_windows.go`, `_amd64.go`, …) and `//go:build` / `// +build` lines — exactly
what the go tool does when it reads a package directory. `build.Default` reflects the
GOOS/GOARCH arcc was built for (the target platform). Standard-library packages arrive
pre-filtered from `discoverStdlib` and are left alone.

- **Why the loader, not the Bazel adapter?** A Starlark filter can match filename
  suffixes but not `//go:build` lines (it would have to parse Go), and rules_go exposes
  no ready provider for the compiled subset. The loader fix is robust (both constraint
  kinds), lives in one place, and is **host-agnostic**: a build system that already
  hands arcc the compiled subset sees a no-op.
- **Safety-net semantics:** a file whose constraints cannot be evaluated (e.g. a unit
  test that mocks file existence, so the file is not on disk) is **kept**, never dropped
  unread. Filtering only ever removes a file it successfully read and found excluded.

The `go_target_info` srcs-contract doc in `go_adapter.bzl` was corrected to say the
adapter forwards the host's source set and the loader filters — a host (rules_go
included) need not pre-filter.

Guard: `TestValidateAndResolveFiltersBuildConstraints` builds per-OS filename variants
plus a `//go:build windows` file and asserts only the current platform's sources survive.

## How a host adopts arcc after these changes

1. Set `hostpolicy.CanonicalizePath` and `hostpolicy.IsStdlibPath` once at init to
   describe the host's import-path rewrite and namespace.
2. Replace `bazel_rules/go/private/go_adapter.bzl` with one binding the host's Go rules
   (same function contract).

Everything else — the pure checker, the rest of the Bazel rules, the loader — stays
upstream-identical. In the concrete google3 import, this collapses roughly 126 lines of
scattered local patches (a `normalizePkg` in the checker, an `isStdlib` special case, a
`packagelayout` heuristic patch, dual-keyed capability annotations in the Capslock
adapter) into a single small override file plus the one adapter file.
