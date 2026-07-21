# Implementation Plan: arcc as a Bazel Rule (`go_component`)

Derived from `../design/detailed-design.md` (hermetic-only revision, 2026-07-20). Each step is a working, demoable increment built test-first. Section references (§) point at the design.

This is a brownfield change: the earliest steps touch arcc's Go core (the `--package-layout` loading mode) so compatibility is validated first; the Bazel rules are layered on top and wired end-to-end as early as the pieces allow.

## Progress Checklist

- [x] **Step 1:** arcc `--package-layout` hermetic loading mode (self-exec `GOPACKAGESDRIVER`)
- [x] **Step 2:** Repo bzlmod-ification + arcc built from source under Bazel (`@rules_arcc//:arcc`)
- [x] **Step 3:** `bazel_rules/` substrate — authority constants, providers, `_arcc_deps` aspect
- [x] **Step 4:** `go_component` macro + `_go_component` rule — manifest & layout generation
- [x] **Step 5a:** arcc-side stdlib resolution in the layout driver (Go)
- [x] **Step 5b:** `_arcc_check_test` — hermetic check wired end-to-end
- [x] **Step 6:** Bazelified csvtool examples + golden manifest comparison
- [x] **Step 7:** `just ci` Bazel leg + consumer docs

Core end-to-end functionality arrives in two stages: the **arcc-side** hermetic check is demoable at **Step 1** (checking a fixture layout with no `go.mod`); the **full Bazel** end-to-end (`bazel test //…:component.check`) lands at **Step 5b**.

Steps 1–4 alternated between the two sides of the seam; Step 5 turned out to need both, so it is split by codebase. **Step 5a is Go work in `go/internal/packagelayout`** — self-contained, test-first, and the only remaining arcc change the design calls for. **Step 5b is Starlark** and consumes it.

---

## Step 1: arcc `--package-layout` hermetic loading mode

**Objective.** arcc can check a component from a manifest plus a Bazel-style package-layout JSON, loading Go packages through a self-exec `GOPACKAGESDRIVER` with no `go list` and no `go.mod` present. This productizes the validated spike (`../research/spike-driver/`) into arcc and is the load-bearing core the rest of the design depends on.

**Guidance.**
- Define the package-layout schema as Go structs matching `go/packages`' driver "flat" form (`../research/spike-driver-findings.md`); this schema is the contract Step 4 emits against.
- Add a hidden driver subcommand (self-exec) that answers `go/packages` queries from the layout, including the **`std`** meta-pattern (spike finding 1), and serves stdlib from the layout's `go_sdk_root` (finding 2).
- Add `--package-layout=<file>` to `arcc check` (§4.6). When set: point `GOPACKAGESDRIVER` at the arcc binary, and resolve `interface_files` and package srcs workspace-relative via the layout rather than relative to the manifest's directory. Thread the seam through `goanalysis.LoadPackageFacts` and `capslockadapter` as env-var setup only — the `packages.Load` calls are unchanged.
- Membership comes from the layout's `roots` (§5.3/§5.4), replacing directory derivation when the flag is present. Colocated behaviour (flag absent) is untouched (`cmd/arcc/app`, `go list` path).

**Tests.** Go unit + integration (reuse the existing `cmd/arcc` integration harness), seeded from the spike fixtures: a clean component (no findings → exit 0); a `FILES`-bearing component that violates its declaration (→ exit 1); the `std` query is exercised; a src named by the layout but absent → clean tool error (exit 2). Run the loading tests from a working directory with no `go.mod` to assert hermeticity.

**Integration.** Purely additive to `cmd/arcc/app` and the loaders; no proto change (§5.1, §9). Selfcheck and colocated manifests are unaffected.

**Demo.** From a directory with no `go.mod`: `arcc check svc.component.textproto --package-layout=svc.package-layout.json` prints a correct verdict; swapping in the violating fixture reports the `FILES` finding with its call path.

---

## Step 2: Repo bzlmod-ification + arcc from source under Bazel

**Objective.** The repository is a Bazel module (`rules_arcc`) that builds arcc from source, exposing the stable `@rules_arcc//:arcc` binary the check rule will use (R10, R12).

**Guidance.**
- Add `MODULE.bazel`: `bazel_dep` on `rules_go` and `gazelle`; a `go_deps` extension over `go/go.mod` so capslock/x/tools/protobuf resolve for consumers (§4.1).
- Generate Gazelle BUILD files for `go/` so `//go/cmd/arcc` builds as a `go_binary`; add the `//:arcc` alias.
- Note the `--@rules_go//go/config:pure` (or CC toolchain) requirement for consumers (Appendix A).

**Tests.** `bazel build //go/cmd/arcc` and `bazel build @rules_arcc//:arcc` succeed; a smoke check that `bazel run //:arcc -- --version` matches the `go run` output.

**Integration.** Additive; the plain `go build` / `just ci` / `just selfcheck` flows stay green (BUILD files are generated alongside, §4.1). Unblocks every subsequent Bazel step.

**Demo.** `bazel run @rules_arcc//:arcc -- --version` prints the arcc version; `bazel run @rules_arcc//:arcc -- check <colocated-manifest>` still works via the existing path.

---

## Step 3: `bazel_rules/` substrate — authority, providers, aspect

**Objective.** Stand up the Starlark substrate the component rule builds on: the language-neutral authority taxonomy and provider contract, the Go-specific provider, and the `_arcc_deps` aspect that collects the package closure with edges (§4.1, §4.3).

**Guidance.**
- `bazel_rules/authority.bzl` (constants + `ALL_AUTHORITIES`), `bazel_rules/providers.bzl` (`ArccComponentInfo`), `bazel_rules/go/providers.bzl` (`ArccPackageInfo`), and `bazel_rules/go/defs.bzl` re-exporting the authority constants (§5.5, §7.4).
- `_arcc_deps` aspect propagating over `deps`/`embed`, emitting `struct(importpath, dir, srcs, deps)` per package (§4.3); stdlib excluded. A validated reference implementation exists in `../research/spike-aspect/rules/defs.bzl` — port it, don't re-derive. Load-bearing details (`../research/spike-aspect-findings.md`): edges from `GoArchive.direct` projected to import paths (self excluded); `srcs` straight from `GoInfo.srcs` (already embed-merged); **must** traverse `embed` yet fold nodes by import path (union srcs+deps — last-write-wins is a bug); guard non-Go/`main` nodes; `sorted()` everything for determinism.
- A CI consistency check keeping `authority.bzl` in sync with the Go capability set in `go/internal/manifest` (§5.5, §8.5).

**Tests.** `rules_testing` analysis tests over a synthetic `go_library` graph asserting the aspect yields the expected packages and direct-dependency edges; the authority-sync check fails when a capability is added to the Go set but not `authority.bzl`.

**Integration.** Uses Step 2's module/rules_go setup. The providers/aspect are consumed by Step 4; nothing user-facing yet, but the substrate is independently testable.

**Demo.** A `rules_testing` target dumps the `ArccPackageInfo` closure for a sample library; the authority-sync test is green.

---

## Step 4: `go_component` macro + `_go_component` rule

**Objective.** `go_component(...)` expands to a component target that classifies membership, runs the consistency/diamond checks, generates `component.textproto` **and** `package-layout.json`, forwards `GoInfo`/`GoArchive`, and exports `ArccComponentInfo` (§4.2, §4.4, §5.2).

**Guidance.**
- Symbolic macro in `defs.bzl` with typed attrs: `interface` single mandatory `attr.label` (list → load error, R3), `absorbed_deps` as `attr.label_list` (R5), `component_deps`, `contract`, `declared_authority` (§4.2).
- `_go_component` rule: membership classification (§5.3, order — covered → absorbed → member), manifest writer, layout writer with `roots`/`go_sdk_root`/cgo-preprocessed srcs (§4.4, §4.6 findings 2–3), and verbatim `GoInfo`/`GoArchive` forwarding.
- Analysis-time errors: diamond/absorb-covered conflict and nested-component-root (§6.1).

**Tests.** `rules_testing`: golden `component.textproto` + `package-layout.json` for a synthetic graph; membership-classification cases (covered/absorbed/member); diamond-conflict and nested-root **failure** tests; a `go_test` with `deps = [":component"]` compiles (GoInfo forwarding, R9).

**Integration.** Consumes Step 3's aspect/providers; emits a layout that conforms to Step 1's schema. The generated manifest matches arcc's existing schema (no proto change).

**Demo.** `bazel build //path:svc_component` produces `svc_component.component.textproto` + `svc_component.package-layout.json` (inspect with `cat`); a `go_test` depending on the component target builds and runs.

---

## Step 5a: arcc-side stdlib resolution in the layout driver

**Objective.** arcc's package-layout driver resolves the Go standard library itself from the layout's `go_sdk_root`, so a layout that names only a component's own packages loads and type-checks. This closes the gap Step 4 surfaced (§5.4, "Standard-library edges") and is the last arcc change the design calls for.

**Guidance.**
- The gap: the `_arcc_deps` aspect closure excludes stdlib (`GoArchive.direct` never carries it), so a Bazel-emitted layout has no stdlib packages and a member package that imports `os` has no `os` entry in its `Imports` map. Under a driver, `go/packages` resolves every import through that map and type-checks dependencies from source; Capslock separately issues `packages.Load(nil, "std")` (§4.6, finding 1). Build-constraint-filtered stdlib file lists are not derivable at Bazel analysis time, which is why this lands in arcc rather than in the rules.
- **Enumerate the stdlib from `go_sdk_root`** using `go/build` — `build.Context{GOROOT: …}` with `ImportDir` applies build constraints (GOOS/GOARCH/tags) with no `go list` subprocess and no `go` binary, so the check stays hermetic. Walk the SDK source tree for package directories; the set to reproduce is **what `go list std` reports**. Skip the top-level `cmd/…` tree and any `testdata/`; keep `internal/…` (stdlib packages import it).
- **`$GOROOT/src/vendor/…` is part of `std`, not something to skip.** `go list std` reports ~17 packages under `vendor/`-prefixed import paths (`vendor/golang.org/x/net/dns/dnsmessage`, `vendor/golang.org/x/crypto/chacha20poly1305`, …), and `net`, `crypto/tls` and `net/http` cannot be type-checked without them. Enumerate them like any other SDK package — import path = path relative to `$GOROOT/src`, which yields the `vendor/`-prefixed form and makes `sdkRoot/PkgPath/<base>` file resolution work unchanged. `go/build.ImportDir` reports the *source* import (`golang.org/x/net/dns/dnsmessage`), so an SDK package's edge to a vendored package must be **rewritten to its `vendor/`-prefixed path** — the go command's GOROOT vendor rule. Only `$GOROOT/src/cmd/vendor/…` is out of scope, and only because `cmd/` already is.
- **Serve `"std"` from that enumeration** rather than requiring the layout to carry it, and resolve exact stdlib import paths the same way.
- **Recover member packages' stdlib edges** by parsing each layout package's `GoFiles` with `parser.ImportsOnly` and adding any import the layout's `Imports` map lacks that `IsStdlib` accepts. Cheap, deterministic, and it keeps the layout format free of data a build system cannot produce.
- **Layout-provided entries win.** A layout that already enumerates stdlib packages (Step 1's fixtures, and any `go list`-derived layout) must keep working unchanged; SDK enumeration fills gaps rather than replacing what is there.
- Keep everything sorted: enumeration order, recovered imports, and the driver's response must stay byte-stable for cache-stable checks.

**Tests.** Go unit tests in `internal/packagelayout`: build-constraint filtering (a GOOS-suffixed file is excluded for other platforms), non-package directories skipped, import recovery adding only missing stdlib edges, layout-provided stdlib entries left untouched. Integration test in `cmd/arcc` layout mode, run through the existing `runArccHermetic` harness (no Go toolchain on `PATH`): a component whose member imports `os` and `strings` checks clean against a layout naming **only** the member packages, and a `FILES`-minting member still reports its finding with a call path.

**Integration.** Confined to `internal/packagelayout`; `goanalysis`, `capslockadapter`, and the CLI are unchanged, as is colocated (non-layout) mode. Unblocks Step 5b, which is what first exercises it under Bazel.

**Demo.** From a directory with no `go.mod` and no `go` on `PATH`: `arcc check svc.component.textproto --package-layout=svc.package-layout.json`, where the layout lists only `svc`'s own package and points `go_sdk_root` at a Go SDK, prints a correct verdict including the `os.Open` `FILES` finding.

---

## Step 5b: `_arcc_check_test` — hermetic check end-to-end

**Objective.** `name.check` runs `arcc check <manifest> --package-layout=<layout>` hermetically, tying Step 4's generated layout to Step 1's arcc loading mode and Step 5a's stdlib resolution — the full sandboxed, cacheable enforcement path (§4.5).

**Guidance.**
- `check.bzl` test rule + launcher script exec'ing arcc with `--format=json`; exit-code mapping is arcc's (§4.5, §6.2).
- The launcher must run arcc **from the runfiles root**: every path in the manifest and layout is relative to it (§5.4, "Path frame").
- Runfiles: `@rules_arcc//:arcc`, the transitive manifest and layout depsets, member/absorbed package srcs, and the rules_go SDK stdlib source named by the layout (§4.5, §4.6 finding 2).
- Factor command construction into a helper shared with a future `_arcc_validation` action (R7). No `local`/`external` tags — the test is hermetic.

**Tests.** A passing component's `.check` passes; a violating component's `.check` fails (driven via the rule's `expect_violation`, so it is itself a passing test) — the primary negative test (§8.3). The positive fixture, `api_component.check`, exercises Step 5a's stdlib resolution end to end, because its closure imports `strings`.

**Also done here (folded in per the orchestration report's Step 5b recommendation):** a fast Go integration test in `cmd/arcc` (`TestIntegration_LayoutMode_RealSDK`) that runs the hermetic harness against a genuine `$GOROOT/src` with a member-only layout — the automated form of the by-hand Step 5a demo. It isolates arcc's real-SDK path from the Bazel wiring, so a `.check` failure under Bazel is unambiguous. The prior SDK-facing suite proved AC5/AC6 only against a synthetic tree; this closes that gap. A Step 5a unit test that depended on `runtime.GOROOT()` was also guarded to skip under the rules_go placeholder GOROOT, which had been silently failing `bazel test //...`.

**Integration.** First full-stack demo: rule → layout → arcc-via-driver → verdict, entirely under Bazel with no `go.mod` in the workspace.

**Demo.** `bazel test //path:svc_component.check` passes; the violating variant fails, and the arcc violation report (call path, evidence) is in the test log.

---

## Step 6: Bazelified csvtool examples + golden comparison

**Objective.** Prove the rules on real components: `go_component` declarations for `toprow`, `csvfile`, and `app` mirroring their checked-in manifests, with `bazel test //go/examples/...` green and generated manifests golden-compared to the colocated originals (R11, §8.2).

**Guidance.**
- Gazelle + `go_component` BUILD files for `go/examples/csvtool/{toprow,csvfile,app}` and `internal/parsecsv`: toprow (no authority), csvfile (`[FILES]`), app (component_deps prune authority) (§8.2).
- Golden test comparing generated vs. checked-in manifest semantics (same interface files, deps, authority).

**Tests.** `bazel test //go/examples/csvtool/...` all pass, including each `.check`; the golden comparison passes; app's authority is correctly pruned by its component deps.

**Integration.** Exercises the entire stack (Steps 1–5b) on existing, non-synthetic code; validates parity with the hand-written manifests.

**Demo.** `bazel test //go/examples/csvtool/...` is green; show `csvfile` reporting `[FILES]` and `app` pruned to none.

---

## Step 7: `just ci` Bazel leg + consumer docs

**Objective.** Fold the Bazel build/test into the project's CI and document consumption (§8.5, §9).

**Guidance.**
- **CI leg (pulled forward into Step 5b):** `just ci` runs a guarded `bazel-test` recipe (`bazel build //...` + `bazel test //...`, skipped cleanly when Bazel is absent). Wired in early so a green `just ci` means the Bazel targets are green too — otherwise a Bazel-only regression like Step 5a's `runtime.GOROOT()` test can land undetected. Kept as `//...` (broader than the design's `//bazel_rules/... //go/examples/...`) so the gazelle-generated Go tests run under Bazel too; the two example-bearing trees are a subset of it.
- **Consumer docs (done):** README "Declaring and checking components with `go_component`" — the `@rules_arcc//bazel_rules/go:defs.bzl` load path, the full `go_component` attribute reference, the `name`/`name.check` expansion, and how to enforce a contract with `bazel test`. The `--@rules_go//go/config:pure` note and the module/gazelle setup were already in the README's "Using Bazel" section (Step 2), and `.bazelrc` sets the flag by default.

**Tests.** `just ci` passes both with Bazel present (runs the leg) and absent (skips cleanly); a doc snippet/example is validated by Step 6's example targets.

**Integration.** Completes CI wiring; no new runtime behaviour.

**Demo.** `just ci` on a Bazel-provisioned machine runs the full Go + Bazel suite green; on a machine without Bazel it runs the Go suite and skips the Bazel leg with a clear notice.
