# Research: Bazel Rules Landscape for arcc Integration

Researched 2026-07-16. Focus areas: rules_go providers, custom-rule/macro mechanics, test-vs-validation enforcement, and the hermeticity problem for `go/packages`-based analyzers.

## 1. rules_go providers — what a custom rule can see and forward

- **`GoInfo`** is the provider describing a Go library before compilation: sources, importpath, deps, embed info. (GoLibrary/GoSource were merged into `GoInfo` in modern rules_go; the old names are deprecated aliases.) Any target offering `GoInfo` is accepted in `go_library`/`go_test`/`go_binary` `deps`.
- **`GoArchive` / `GoArchiveData`** describe a *compiled* package plus its direct and transitive dependency archives — this is the natural way to enumerate a target's **transitive dependency closure** (import paths + source files) at analysis time.
- **Precedent for "just works" forwarding:** `go_proto_library` is a custom rule that returns `GoInfo`, which is why plain `go_test`/`go_library` targets can list it in `deps`. A `go_architectural_component` rule can do the same: re-export the `GoInfo` (and `GoArchive`) of its interface library, so dependents can `deps = [":service_api_component"]` and it "just works". **Answer to the user's question 2: yes, this is possible and idiomatic.**
- A custom provider (e.g. `ArccComponentInfo`) carries the generated manifest artifact + transitive manifest depset between `go_architectural_component` targets; `component_deps` attr can enforce `providers = [ArccComponentInfo]` for type checking.

Sources: [rules_go providers.rst](https://github.com/bazelbuild/rules_go/blob/master/go/providers.rst), [rules_go core rules docs](https://github.com/bazel-contrib/rules_go/blob/master/docs/go/core/rules.md).

## 2. Multi-target expansion: macros

- "Expands into two targets" is standard macro territory. Bazel 8 **symbolic macros** give typed attributes, visibility hygiene, and predictable naming (`name`, `name + ".check"` etc.); a legacy `.bzl` macro also works and keeps compatibility with older Bazel.
- Common idiom (e.g. rules_proto, rules_lint): macro `go_architectural_component(name=...)` declares
  - `name` — the component rule (generates manifest, forwards `GoInfo`, exports `ArccComponentInfo`), and
  - `name + "_check"` (or `.check` with symbolic macros) — the test target running `arcc check`.

## 3. Enforcement point: test rule vs. validation action vs. aspect

Three established patterns for "lint-like" enforcement (all used in the wild, cf. [rules_lint](https://github.com/aspect-build/rules_lint), [rules_swiftlint](https://github.com/thii/rules_swiftlint), [Bazel validation actions](https://bazel.build/extending/rules#validation_actions)):

| Pattern | Trigger | Pros | Cons |
|---|---|---|---|
| `*_test` target (runs `arcc check`) | `bazel test //...` | Simple; cached like any test; results in test UI; matches user's sketch (b) | Only enforced when tests run; needs runfiles containing all inputs |
| **Validation action** (`_validation` output group) | `bazel build //...` — always requested | Enforced on *every build* of the target; runs in parallel; can't be forgotten | Failure is a build failure (blunter UX); no `--test_filter`-style selection |
| Aspect over `go_library` graph | `bazel build --aspects=...` | No BUILD changes for absorbed deps; can auto-derive dep structure | Aspects+validation have sharp edges ([bazel#19636](https://github.com/bazelbuild/bazel/issues/19636)); harder to attach per-component config (contracts, authority) |

A pragmatic combo: manifest generation as a build action on the component rule; `arcc check` as a test target (MVP), optionally *also* wired as a validation action later. Aspects are less suitable as the primary mechanism because arcc needs per-component declarations (contract files, declared authority) that don't exist on plain `go_library` targets — but an aspect is the right tool for *collecting* transitive srcs/importpaths from `interface`/`deps`.

## 4. The hermeticity problem: `go/packages` inside a Bazel action

- `packages.Load` shells out to `go list` by default; that **does not work inside a Bazel sandbox** (no module cache, no go.mod view of the sandbox layout). This is a long-standing known gap: [rules_go#1996 "Calling go/packages.Load during Bazel build"](https://github.com/bazelbuild/rules_go/issues/1996).
- rules_go's `gopackagesdriver` implements the `GOPACKAGESDRIVER` protocol but works by invoking `bazel query`/aspects from *outside* — it's for editors/tools outside actions, not usable *inside* a hermetic action ([editor integration wiki](https://github.com/bazel-contrib/rules_go/wiki/Editor-and-tool-integration), [gopackagesdriver churn issue](https://github.com/bazel-contrib/rules_go/issues/3533)).
- No existing Bazel rule for Capslock was found — no prior art to reuse there; Capslock is CI-integrated via `go` tooling elsewhere ([capslock repo](https://github.com/google/capslock)).

### Candidate strategies for running `arcc check` hermetically

**(a) Synthesize a `go list`-able tree in the action.** Stage all transitive sources (from `GoArchive`/`GoInfo`) plus a generated `go.mod`/`vendor/` layout plus the Go SDK (rules_go toolchain exposes it) into the action inputs; run arcc with `GOFLAGS=-mod=vendor`, `GOPATH`/`GOMODCACHE` pointed inside the sandbox. Pros: zero arcc changes to the loading path. Cons: fragile (module path fidelity, build tags), slow staging, duplicates what Bazel already knows.

**(b) Static package-metadata driver (recommended direction).** Bazel already knows, at analysis time, every package's importpath, file list, and deps (via `GoInfo`/`GoArchive`). The rule can serialize this into a JSON "package layout" file; arcc grows a loader mode (either an internal `go/packages` driver via `GOPACKAGESDRIVER` protocol served from a static file, or a direct `packages.Config.Overlay`-style loader) that consumes it instead of `go list`. `go/packages` type-checks and parses from source itself once the driver supplies file lists + import graph — no Go toolchain subprocess needed, only Go SDK **stdlib sources** as inputs for the stdlib portion of the SSA graph. Pros: hermetic, fast, precise, incremental per component. Cons: requires an arcc feature (`--package-layout=layout.json` or similar) and careful stdlib handling.

**(c) Non-hermetic escape hatch.** `arcc check` as a `local`/`no-sandbox` test executing against the real workspace with plain module resolution. Pros: trivial to ship first. Cons: loses caching/hermeticity, breaks with remote execution — acceptable as MVP scaffolding only.

## 5. Naming conventions observed in the ecosystem

- Rule sets: `rules_<tool>` repos, rule names prefixed by language: `go_proto_library`, `go_test` → `go_architectural_component` fits; shorter alternatives: `go_component`, `arcc_go_component`.
- Attribute conventions: `srcs`, `deps`, `data` are reserved-feel standard names; label-list attrs that require a provider are conventionally named after the concept (`proto`, `embed`, `deps`). rules_go uses `embed` for "absorb this library's sources into mine" — noteworthy because arcc's "absorbed dependency" is a related but distinct concept; avoid overloading `embed`.
- Typed enum-like values via labels exist (e.g. toolchains, `constraint_value`s); a cheaper alternative for `declared_authority` is a Starlark constants module (`load("@rules_arcc//go:authority.bzl", "FILES")`) giving load-time typo safety without target boilerplate; or plain strings validated by the manifest generator at build time (matching arcc's existing parse-time validation).

## 6. Additional tradeoffs noticed (for the requirements discussion)

1. **Interface-files-vs-srcs mismatch:** in the sketch, `interface = [":service_api"]` takes that library's `srcs` as interface files. But `service_api`'s srcs are one *package*; arcc interface files are a subset of files *within* packages. Taking a whole `go_library` as "the interface" implies the component's public surface is that entire package — a slightly different (arguably cleaner, more Bazel-native) model than file-level interface lists. Worth an explicit requirements decision: does the Bazel model shift arcc's unit of interface from "files" to "the interface package(s)"?
2. **Membership derivation:** with approach 1 (reference existing libs), component membership = transitive closure of `interface` libs *minus* dep closures of `component_deps` interfaces — the rule must compute the absorbed set by subtracting; ordering/overlap subtleties (diamond deps, a lib reachable both via a component dep and directly) need defined semantics.
3. **Visibility as enforcement:** Bazel `visibility` on the internal lib already gives *target-level* dependency enforcement for free; arcc adds symbol-level boundaries + authority. The design should say which layer owns what, so users don't double-maintain.
4. **Contract files:** currently contracts are doc comments in interface files (no manifest field). The sketch adds `contract = [....md]` files — that's a *new* arcc feature (manifest schema + check semantics?) or Bazel-only metadata. Needs a requirements decision.
5. **Stdlib/toolchain versioning:** hermetic capability analysis depends on the exact Go SDK version (stdlib call graphs change); the rule should take the SDK from the rules_go toolchain so results are reproducible.
6. **Remote execution & caching:** strategy (b) makes per-component checks cacheable and RBE-safe; strategy (c) does not.

## Sources

- [rules_go providers.rst](https://github.com/bazelbuild/rules_go/blob/master/go/providers.rst)
- [rules_go core rules docs](https://github.com/bazel-contrib/rules_go/blob/master/docs/go/core/rules.md)
- [rules_go#1996 — Calling go/packages.Load during Bazel build](https://github.com/bazel-contrib/rules_go/issues/1996)
- [rules_go wiki — Editor and tool integration (gopackagesdriver)](https://github.com/bazel-contrib/rules_go/wiki/Editor-and-tool-integration)
- [rules_go#3533 — gopackagesdriver analysis-cache churn](https://github.com/bazel-contrib/rules_go/issues/3533)
- [Bazel rules docs — validation actions](https://bazel.build/extending/rules#validation_actions)
- [bazel#19636 — aspect-defined validation actions limitation](https://github.com/bazelbuild/bazel/issues/19636)
- [aspect-build/rules_lint](https://github.com/aspect-build/rules_lint)
- [google/capslock](https://github.com/google/capslock)
- [jayconrod.com — Go editor support in Bazel workspaces](https://jayconrod.com/posts/125/go-editor-support-in-bazel-workspaces)
