# Implementation Plan: arcc as a Bazel Rule (`go_component`)

Derived from `../design/detailed-design.md` (hermetic-only revision, 2026-07-20). Each step is a working, demoable increment built test-first. Section references (§) point at the design.

This is a brownfield change: the earliest steps touch arcc's Go core (the `--package-layout` loading mode) so compatibility is validated first; the Bazel rules are layered on top and wired end-to-end as early as the pieces allow.

## Progress Checklist

- [ ] **Step 1:** arcc `--package-layout` hermetic loading mode (self-exec `GOPACKAGESDRIVER`)
- [ ] **Step 2:** Repo bzlmod-ification + arcc built from source under Bazel (`@rules_arcc//:arcc`)
- [ ] **Step 3:** `bazel_rules/` substrate — authority constants, providers, `_arcc_deps` aspect
- [ ] **Step 4:** `go_component` macro + `_go_component` rule — manifest & layout generation
- [ ] **Step 5:** `_arcc_check_test` — hermetic check wired end-to-end
- [ ] **Step 6:** Bazelified csvtool examples + golden manifest comparison
- [ ] **Step 7:** `just ci` Bazel leg + consumer docs

Core end-to-end functionality arrives in two stages: the **arcc-side** hermetic check is demoable at **Step 1** (checking a fixture layout with no `go.mod`); the **full Bazel** end-to-end (`bazel test //…:component.check`) lands at **Step 5**.

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
- `_arcc_deps` aspect propagating over `deps`/`embed`, emitting `struct(label, importpath, dir, srcs, deps)` per package (§4.3); stdlib excluded (spike, question B).
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

## Step 5: `_arcc_check_test` — hermetic check end-to-end

**Objective.** `name.check` runs `arcc check <manifest> --package-layout=<layout>` hermetically, tying Step 4's generated layout to Step 1's arcc loading mode — the full sandboxed, cacheable enforcement path (§4.5).

**Guidance.**
- `check.bzl` test rule + launcher script exec'ing arcc with `--format=json`; exit-code mapping is arcc's (§4.5, §6.2).
- Runfiles: `@rules_arcc//:arcc`, the transitive manifest and layout depsets, member/absorbed package srcs, and the rules_go SDK stdlib source named by the layout (§4.5, §4.6 finding 2).
- Factor command construction into a helper shared with a future `_arcc_validation` action (R7). No `local`/`external` tags — the test is hermetic.

**Tests.** A passing component's `.check` passes; an `absorbapp`-style violating component's `.check` fails with exit≠0 (script test) — the primary negative test (§8.3). Assert cache reuse (a second `bazel test` is cached).

**Integration.** First full-stack demo: rule → layout → arcc-via-driver → verdict, entirely under Bazel with no `go.mod` in the workspace.

**Demo.** `bazel test //path:svc_component.check` passes; the violating variant fails, and the arcc violation report (call path, evidence) is in the test log.

---

## Step 6: Bazelified csvtool examples + golden comparison

**Objective.** Prove the rules on real components: `go_component` declarations for `toprow`, `csvfile`, and `app` mirroring their checked-in manifests, with `bazel test //go/examples/...` green and generated manifests golden-compared to the colocated originals (R11, §8.2).

**Guidance.**
- Gazelle + `go_component` BUILD files for `go/examples/csvtool/{toprow,csvfile,app}` and `internal/parsecsv`: toprow (no authority), csvfile (`[FILES]`), app (component_deps prune authority) (§8.2).
- Golden test comparing generated vs. checked-in manifest semantics (same interface files, deps, authority).

**Tests.** `bazel test //go/examples/csvtool/...` all pass, including each `.check`; the golden comparison passes; app's authority is correctly pruned by its component deps.

**Integration.** Exercises the entire stack (Steps 1–5) on existing, non-synthetic code; validates parity with the hand-written manifests.

**Demo.** `bazel test //go/examples/csvtool/...` is green; show `csvfile` reporting `[FILES]` and `app` pruned to none.

---

## Step 7: `just ci` Bazel leg + consumer docs

**Objective.** Fold the Bazel build/test into the project's CI and document consumption (§8.5, §9).

**Guidance.**
- Extend `just ci` to run `bazel build //...` and `bazel test //bazel_rules/... //go/examples/...`, **guarded** on Bazel being available so environments without Bazel still pass (§8.5).
- Consumer docs: load paths (`@rules_arcc//bazel_rules/go:defs.bzl`), the `go_component` API, and the `--@rules_go//go/config:pure` note (§4.1, Appendix A).

**Tests.** `just ci` passes both with Bazel present (runs the leg) and absent (skips cleanly); a doc snippet/example is validated by Step 6's example targets.

**Integration.** Completes CI wiring; no new runtime behaviour.

**Demo.** `just ci` on a Bazel-provisioned machine runs the full Go + Bazel suite green; on a machine without Bazel it runs the Go suite and skips the Bazel leg with a clear notice.
