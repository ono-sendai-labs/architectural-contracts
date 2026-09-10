# Dependencies

## 1. Go module — `go/go.mod`

```
module github.com/ono-sendai-labs/architectural-contracts/go

go 1.26

require (
	github.com/google/capslock v0.3.2
	golang.org/x/tools v0.48.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/fatih/color v1.19.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
```

Three direct dependencies — an unusually small surface for a static-analysis tool, and a
deliberate one: the pure core packages import **nothing but the standard library**.

| Dependency | Used by | For what |
|---|---|---|
| `github.com/google/capslock` | `capslockadapter` only | `analyzer` + `interesting` — the capability taxonomy and call-graph capability analysis. arcc feeds it a custom classifier text and maps its `CapabilityInfo`/`Path()` output into `capanalyzer.CapabilityFinding` |
| `golang.org/x/tools` | `goanalysis`, `packagelayout`, `capslockadapter` | `go/packages` (loading + the external-driver protocol), `go/ssa` + `ssa/ssautil` (SSA construction), `go/callgraph` + `callgraph/vta` (Variable Type Analysis call graph) |
| `google.golang.org/protobuf` | `manifest`, `manifest/gen` | `encoding/prototext` for parsing `component.textproto`; generated types in `gen/component.pb.go` |

The indirect deps are almost entirely capslock's CLI colouring (`fatih/color`,
`mattn/go-colorable`, `mattn/go-isatty`) plus `x/mod`, `x/sync`, `x/sys`.

**`golang.org/x/sys` matters architecturally**: it is built with cgo in capslock's closure,
which is why `capslockadapter` and `cli` cannot be checked under Bazel (README limitation 15).

### Internal dependency edges

```
capanalyzer      →  (nothing)
hostpolicy       →  (nothing)
report           →  (nothing)
manifest         →  manifest/gen
facts            →  capanalyzer, manifest
checker          →  capanalyzer, facts, manifest, report
packagelayout    →  hostpolicy
capslockadapter  →  capanalyzer, packagelayout
goanalysis       →  capanalyzer, facts, hostpolicy, manifest, packagelayout
manifestparity   →  manifest
cmd/arcc         →  app, capslockadapter, goanalysis
cmd/arcc/app     →  capanalyzer, checker, facts, goanalysis, hostpolicy, manifest,
                    packagelayout, report
```

Acyclic; the shell depends on the core and never the reverse.

## 2. Bazel — `MODULE.bazel`

```python
module(name = "rules_arcc", version = "0.0.0")

bazel_dep(name = "rules_go",      version = "0.61.1")
bazel_dep(name = "gazelle",       version = "0.51.3")
bazel_dep(name = "rules_shell",   version = "0.8.0")
bazel_dep(name = "platforms",     version = "1.1.0", dev_dependency = True)
bazel_dep(name = "rules_testing", version = "0.9.0", dev_dependency = True)

go_sdk.download(version = "1.26.4")            # keep in sync with go/go.mod
go_deps.from_file(go_mod = "//go:go.mod")      # one source of truth for Go deps
go_deps.gazelle_override(
    directives = ["gazelle:proto disable_global"],
    path = "github.com/google/capslock",
)
use_repo(go_deps, "com_github_google_capslock",
                  "org_golang_google_protobuf",
                  "org_golang_x_tools")
```

| Dep | Why |
|---|---|
| `rules_go` | builds the Go module; the only ruleset `go_adapter.bzl` touches |
| `gazelle` | generates `BUILD.bazel` files (`bazel run //:gazelle`) and resolves Go deps from `go.mod` |
| `rules_shell` | `sh_test` for the shell-based validation and golden tests |
| `rules_testing` (dev) | `analysis_test` / `test_suite` for the Starlark analysis tests |
| `platforms` (dev) | only for the `darwin_arm64` cross-platform analysis fixture |

Two coupling decisions worth noting:

- **`go_deps.from_file(go_mod = "//go:go.mod")`** — the Bazel build resolves the *same*
  dependency versions the plain `go build` uses, so the two legs cannot skew.
- **The capslock gazelle override** — capslock ships a generated `capability.pb.go`. Without
  `gazelle:proto disable_global`, Gazelle would try to rebuild it from `.proto` via `protoc`,
  dragging in a C++ toolchain that the rest of the build deliberately avoids.

`MODULE.bazel.lock` is checked in.

## 3. Build-time tools

| Tool | Required for | Notes |
|---|---|---|
| Go ≥ 1.26 | everything | `actions/setup-go@v5` reads the version from `go/go.mod` |
| `just` | the task recipes | optional but recommended |
| Bazel | `just bazel-test` / `just ci` | fails loudly if absent, by design |
| `protoc` + `protoc-gen-go@v1.36.11` | `just gen` / `just gen-is-clean` | optional; skip by running `just lint build test test-integration selfcheck` |
| `jj` (Jujutsu) | `just gen-is-clean`, local VCS workflow | `gen-is-clean` shells out to `jj diff` |
| `gh` | releases, agent GitHub access | limited-permission bot token |

Note the coupling: `just ci`'s `gen-is-clean` step depends on **both** `protoc` and `jj`.

## 4. Toolchain and platform configuration

**`.bazelrc`**:

```
build --@rules_go//go/config:pure
common --repo_env=BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1
test --test_output=errors
try-import %workspace%/user.bazelrc
```

arcc and its dependencies are pure Go, so building in pure mode with C++ toolchain
autodetection suppressed keeps `bazel build //...` working on machines with no C compiler.
Consumers of `@rules_arcc` want the same flag unless they have a CC toolchain configured.
`user.bazelrc` is an untracked local-override hook.

**`.bazelignore`**: `.agents`, `.claude`, `.codex`, `.git`, `.jj`, `bin` — keeps agent tooling
and native build output out of the workspace.

**Gazelle directives**: root `BUILD.bazel` excludes `proto` and `bazel_rules` (the latter
because the hand-maintained `.go` fixtures under `bazel_rules/go/tests/testdata/` would be
rewritten against the wrong import prefix); `go/BUILD.bazel` sets the prefix and excludes
`**/testdata`.

## 5. Consuming `rules_arcc` from another workspace

```python
bazel_dep(name = "rules_arcc", version = "0.0.0")
# plus a git_override / archive_override until it is published to the BCR
```

```python
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component", "FILES")
load("@rules_arcc//bazel_rules:authority.bzl", "FILES")  # language-neutral home
```

Consumers should set `--@rules_go//go/config:pure` unless they have a CC toolchain configured.
The `arcc` binary is built from source in-repo and exposed as `//:arcc` — never fetched — so
the rules and the binary they invoke cannot version-skew.

## 6. Cross-artifact invariants (things that must stay in sync)

| A | B | Enforced by |
|---|---|---|
| `manifest.KnownCapabilities` | `bazel_rules/authority.bzl` constants + `ALL_AUTHORITIES` | `//bazel_rules/tests:authority_sync_test` (fails on any 3-way mismatch, and on an empty extraction) |
| `proto/archcontracts/v1/component.proto` | `go/internal/manifest/gen/component.pb.go` | `just gen-is-clean` |
| Bazel-generated manifests | checked-in `component.textproto` files | `manifestparity` / `self_manifest_parity_test` |
| `MODULE.bazel` Go SDK `1.26.4` | `go/go.mod` `go 1.26` | comment only — **manual** |
| generated `.component.textproto` / `.package-layout.json` | `bazel_rules/go/tests/goldens/*` | `golden_test.sh` (normalizes `go_sdk_root`) |
| layout `roots` | manifest `members` | arcc fails closed at load time |
| layout `is_stdlib` | SDK/toolchain or enumerated-target provenance | layout resource resolution; check membership comes from `StdlibAuthority` |

## 7. What is deliberately *not* depended on

- **No assertion or diff library** — no `testify`, no `go-cmp`. Tests use `t.Errorf` +
  `reflect.DeepEqual`.
- **No logging library** — no `log`, no `slog`, no third-party logger anywhere in `internal/`.
  Diagnostics are `error` values or `report.Finding` entries.
- **No CLI framework** — argument parsing is hand-rolled in `app.Runner.Run`.
- **No `buf`** — protobuf codegen goes through raw `protoc` from the justfile.
- **No buildifier / Starlark linter** — no invocation in the justfile, CI, or any config.
- **No bazel-skylib `subpackages` wrapper** — the README explains at length why no
  `arcc_subpackages()` helper ships: `native.subpackages()` returns only the frontier of
  nearest descendant packages, cannot see past a package boundary, and is rejected inside a
  symbolic macro. "A misleading helper is worse than no helper."
