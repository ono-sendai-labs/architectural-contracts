# Codebase Information

*Baseline revision: `7d787ddfd076ccd45e80affbc82628e542cf145c` (see `last_commit`).*

## Identity

| | |
|---|---|
| **Name** | Architectural Contracts (`arcc`) |
| **Bazel module** | `rules_arcc` (version `0.0.0`) |
| **Go module** | `github.com/ono-sendai-labs/architectural-contracts/go` |
| **Purpose** | Declare, check, and enforce package structure, dependency boundaries, and ambient-authority (capability) limits in Go codebases |
| **Status** | MVP, self-hosting (arcc checks its own eight components) |
| **License** | See `LICENSE` / `NOTICE` |

## What it does

`arcc` reads a per-component **manifest** (`component.textproto`), derives facts about the
component's Go packages (membership, exported symbols, imports, static call edges), runs a
capability analysis over the call graph, and emits a **conformance report**: violations,
warnings, and the list of pruned dependency boundaries.

Three pillars, from `docs/rationale-and-concepts.md`:

1. **Architecture as code** — components with declared interfaces, private implementations, and explicit dependencies.
2. **Informal contracts** — prose doc comments in interface files carry the component contract (FR10).
3. **Ambient authority / capabilities** — a component declares which capabilities it may exercise; declaring none makes it *ambient-authority-free*.

A fourth pillar (data-flow / privacy contracts over annotated protobufs) is described in the
rationale document but is **not implemented** in this MVP.

## Repository layout

```
.
├── go/                          # the Go module (the whole implementation)
│   ├── cmd/arcc/                # CLI entry point + app.Runner  (component "cli")
│   │   └── app/                 # argument parsing and the check pipeline
│   ├── internal/
│   │   ├── capanalyzer/         # PURE: capability port, findings, policy
│   │   ├── capslockadapter/     # SHELL: Capslock-backed CapabilityAnalyzer
│   │   ├── checker/             # PURE: the conformance decision core
│   │   ├── facts/               # PURE: DTOs the shell produces for the core
│   │   ├── goanalysis/          # SHELL: go/packages + SSA + VTA call graph
│   │   ├── hostpolicy/          # PURE: host override seam (path canonicalization)
│   │   ├── manifest/            # PURE: textproto manifest parsing + validation
│   │   │   └── gen/             # generated protobuf types (protoc output)
│   │   ├── manifestparity/      # test-support: generated vs. checked-in manifests
│   │   ├── packagelayout/       # SHELL: hermetic package-layout + GOPACKAGESDRIVER
│   │   └── report/              # PURE: report model + text renderer
│   └── examples/csvtool/        # 4-component worked example (app, csvfile, toprow, parsecsv)
├── proto/archcontracts/v1/      # component.proto — the manifest schema (source of truth)
├── bazel_rules/                 # Starlark: go_component rule, aspect, hermetic .check tests
│   ├── authority.bzl            # language-neutral authority taxonomy
│   ├── providers.bzl            # ArccComponentInfo
│   └── go/                      # defs.bzl (public), providers.bzl, private/, tests/
├── docs/                        # rationale-and-concepts.md, package-layout-schema.md
├── .agents/                     # structured-spec-to-code artifacts (planning, tasks, summary)
├── justfile                     # build/lint/test/selfcheck/bazel-test recipes
├── MODULE.bazel                 # bzlmod deps; Go SDK pinned to 1.26.4
└── .github/workflows/           # ci.yml, release.yml
```

## Scale

| Metric | Value |
|---|---|
| Go source files | 66 (29 of them `_test.go`) |
| Go lines (incl. generated `component.pb.go`) | ~20,100 |
| Starlark files | 12 rule/impl files + 6 test files |
| Languages | Go (implementation), Starlark (Bazel rules), Protobuf (schema), Bash (shell tests), Just (task runner) |
| Self-hosted components | 8 native (`just selfcheck`), 6 hermetic (`bazel test //...`) |

## Technology stack

- **Go 1.26** — the whole implementation. Direct deps: `github.com/google/capslock v0.3.2`, `golang.org/x/tools v0.48.0`, `google.golang.org/protobuf v1.36.11`.
- **Protobuf / textproto** — `proto/archcontracts/v1/component.proto` is the manifest schema; manifests are authored as textproto and parsed with `prototext`.
- **Bazel (bzlmod)** — `rules_go 0.61.1`, `gazelle 0.51.3`, `rules_shell 0.8.0`, `rules_testing 0.9.0` (dev), `platforms 1.1.0` (dev). Go SDK pinned to `1.26.4`; `go_deps.from_file(go_mod = "//go:go.mod")` keeps the two builds from skewing.
- **just** — task runner; `just ci` is the pre-commit gate.
- **Jujutsu (`jj`)** — local VCS workflow (per `CLAUDE.md`/`AGENTS.md`); git is the storage backend, `jj` the interface.

## Two build legs, deliberately kept

| Leg | Membership source | Toolchain at check time | Components checked |
|---|---|---|---|
| Native (`just selfcheck`) | FR1 directory-based / declared `members` | real Go toolchain via `go/packages` | 8 |
| Bazel (`bazel test //...`) | declared `members` → emitted layout `roots` | none — hermetic, layout JSON only | 6 |

The two legs cross-check the membership model itself. `capslockadapter` and `cli` are
native-only: capslock's closure contains `golang.org/x/sys/unix` built with cgo, and the
Bazel rule fails closed on cgo closures (`README.md` limitation 15).

## Architectural patterns in use

- **Ports and adapters (hexagonal)** — a pure core (`checker`, `capanalyzer`, `facts`, `manifest`, `report`) with zero ambient authority; shells (`goanalysis`, `capslockadapter`, `packagelayout`) that own all I/O.
- **Errors as data** — `checker.Check` returns *no* error; every failure mode is a `report.Finding`.
- **Function-variable seams** — `goanalysis.loadPackages`, `packagelayout.osStat`, `hostpolicy.CanonicalizePath` / `IsStdlibPath`.
- **Single host-adapter file** — `bazel_rules/go/private/go_adapter.bzl` is the only file that touches `@rules_go`; porting to another Go ruleset means replacing it alone.
- **Determinism everywhere** — explicit `sort.Slice` / sorted Starlark output so reports, manifests and layouts are byte-reproducible.
