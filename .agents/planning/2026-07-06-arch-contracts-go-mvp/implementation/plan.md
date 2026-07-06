# Implementation Plan — Architectural Contracts MVP (Go)

Source design: [`../design/detailed-design.md`](../design/detailed-design.md).
Supporting artifacts: `../idea-honing.md`, `../research/capslock.md`,
`../research/go-component-model.md`.

**Guiding principle.** Build each component in a test-driven manner, in increments
that each leave the tree in a working, demoable state. Set up the project skeleton
and CI first so every later step lands on green. Then sequence the **pure core** (it
is a pure function of injected data, so the bulk of the test suite needs no Go build
and no Capslock — design §8), then the authority-holding shell that feeds it real
facts, then wire the CLI for the first real end-to-end verdict, then add the
call-graph-based boundary/pruning checks, and finally self-host. No hanging or
orphaned code: every step ends by wiring its output into something runnable.

**Where core end-to-end appears.**
- **Step 6** — first *vertical* slice: real extracted facts flow through the pure
  core to produce a real (Pillar-1-only) verdict in a test harness.
- **Step 8** — first *full* end-to-end CLI: `archcheck check COMPONENT.textproto`
  on the CSV example produces a pass/fail conformance report **including the
  ambient-authority check**, with correct exit codes. This is the headline milestone.
- **Step 9** — the complete concept: cross-component boundary check (FR5) and
  capability pruning (FR5b), incl. the "compose a FILES component yet stay
  authority-free" showcase.
- **Step 10** — self-hosting: `archcheck` verifies its own core as authority-free.

---

## Progress checklist

- [ ] **Step 1 — Project scaffold & developer tooling** (repo layout, `go/` module skeleton, `justfile`, CI + release workflows)
- [ ] **Step 2 — Manifest schema & parser** (`proto/`, codegen, `internal/manifest`)
- [ ] **Step 3 — Pure supporting types: `capanalyzer` port/policy + `report` model & rendering**
- [ ] **Step 4 — Checker core I: fact types + dependency rule (FR3) + declared interface (FR4)**
- [ ] **Step 5 — Checker core II: boundary rule (FR5) + policy-aware authority rule (FR6)** — pure core complete
- [ ] **Step 6 — `goanalysis` loader I: imports + exported-symbol→file extraction** (first vertical slice)
- [ ] **Step 7 — `capslockadapter`: Capslock-backed `CapabilityAnalyzer`**
- [ ] **Step 8 — CLI/app orchestration + CSV example** — first full end-to-end
- [ ] **Step 9 — Call graph + FR5/FR5b: SSA edges, dependency-interface resolution, boundary pruning** — full concept
- [ ] **Step 10 — Self-hosting (FR8): own component manifests + CI check of the core**

---

## Step 1: Project scaffold & developer tooling

**Objective.** Stand up the multi-language repo layout, a **buildable** `go/` module
skeleton, and the developer/CI tooling — a `justfile` and GitHub Actions for CI and
release — so that from here on every step is validated by `just` targets and CI on
each push. Nothing to check yet; the deliverable is a green, reproducible pipeline.

**Implementation guidance.**
- **Layout (NFR1, design §9).** Create the top-level skeleton: `proto/` (empty for
  now — schema lands in Step 2), `go/` as its **own module** (`go/go.mod`, working
  import root `github.com/xtofian/architectural-contracts/go` — adjust to the real
  path, Q5), and placeholder dirs `go/internal/`, `go/cmd/archcheck/`,
  `go/components/`, `go/examples/`. Keep `proto/` **outside** `go/` so a future Rust
  toolchain can share it.
- **Buildable stub.** `cmd/archcheck/main.go` that compiles and prints usage/version
  (e.g. `archcheck check <manifest>` help text, `--version`) — no real logic. This
  gives CI something green to build and test.
- **`justfile`** at repo root with the usual targets (all operating on the `go/`
  module — set the module dir or `cd go` inside recipes):
  - `build` — `go build ./...`
  - `test` — unit tests: `go test ./...`
  - `test-integration` — integration tests (the slower `goanalysis`/`capslockadapter`
    suites, separated by a build tag, e.g. `go test -tags=integration ./...`)
  - `lint` — `go vet ./...` + `gofmt -l`/`golangci-lint` if adopted
  - `fmt` — `gofmt -w` / `go fmt`
  - `gen` — protobuf codegen (a no-op placeholder now; wired to real codegen in Step 2)
  - `run` — `go run ./cmd/archcheck`
  - `ci` — aggregate: `gen`-is-clean check + `lint` + `build` + `test` +
    `test-integration` (what the CI workflow calls)
  - `clean`
- **`.github/workflows/ci.yml`** — on push + pull_request: checkout, set up Go
  (version from `go/go.mod`), install `just` (and `protoc`/plugins if `gen` needs
  them), run `just ci` with `working-directory` at the repo root (recipes `cd` into
  `go/`). Cache the Go build/module cache. This is the gate every later step must keep
  green; Step 10 adds the self-hosting `selfcheck` job here.
- **`.github/workflows/release.yml`** — on tag push (`v*`): build the `archcheck`
  binary (a small `go build` matrix for linux/darwin, amd64/arm64, or GoReleaser if
  preferred) and publish a GitHub Release with the artifacts. Keep it minimal for the
  MVP; the point is the pipeline exists and is wired.

**Tests.** No product code to unit-test yet. The "test" is that `just build`,
`just test`, and `just test-integration` all succeed (trivially, on the stub) and the
CI workflow passes on a first push. Add a single smoke test (`archcheck --version`
prints the expected string) so `just test` exercises a real assertion.

**Integration.** This step wires the *process*, not components — everything after it
plugs into `just` targets and the CI gate.

**Demo.** `just build && just test && just test-integration` all green locally;
pushing the branch shows a green CI run; `git tag v0.0.0 && push` (or a manual
workflow_dispatch) produces a draft release with an `archcheck` binary that prints its
version.

---

## Step 2: Manifest schema & parser

**Objective.** Define the manifest data model: a shared protobuf schema, generated Go
code, and a **pure** `manifest.Parse(bytes) → Manifest` with validation. This is the
foundation the checker consumes.

**Implementation guidance.**
- Create `proto/archcontracts/v1/component.proto` per design §6.1 (`Component`,
  `ComponentDependency`, `AbsorbedDependency`). Keep the dependency and
  interface-file fields plain repeated strings so a future Bazel rule can populate
  them mechanically (NFR3).
- Wire the real **`gen`** justfile target to protobuf codegen. **Decision (recommend):
  commit the generated code** under `internal/manifest/gen/` (design §9 offers this or
  a gen step) so the build needs no `protoc`; the CI `gen`-is-clean check then just
  verifies the committed code matches the proto.
- `internal/manifest`: parse `.textproto` bytes with
  `google.golang.org/protobuf/encoding/prototext` into the generated message, then map
  to a hand-written Go-native `Manifest` model (so the rest of the code doesn't depend
  on protobuf types), and **validate** (design §7): empty `name`, empty `packages`, an
  `interface_files` entry not under any component-package directory, duplicate
  declarations → parse error.
- `Parse` takes **bytes**, not a path — reading the file is the shell's job. This keeps
  `manifest` pure and injectable.

**Tests.** Table-driven unit tests for `Parse`: a valid manifest (e.g. the `toprow`
example from §6.1) round-trips to the expected model; one test per validation error
(empty name, empty packages, stray interface file, duplicate) asserts the specific
error.

**Integration.** Consumed by Step 4's checker. The `gen` target and its CI clean-check
are now real, exercised by every subsequent CI run.

**Risk note (carry to Step 10).** `prototext.Unmarshal` uses reflection, so the
`manifest` component may itself trigger `CAPABILITY_REFLECT`. That does **not** affect
correctness now, but it means `manifest` may not qualify as ambient-authority-free
under `StrictPolicy`. Flag it; resolve in Step 10 (the guaranteed authority-free
showcase is `checker`/`report`/`capanalyzer`).

**Demo.** `just test` covers `internal/manifest`; a test prints the parsed model for
the example `toprow.COMPONENT.textproto` and the specific error for a
deliberately-malformed manifest. `just gen` regenerates cleanly.

---

## Step 3: Pure supporting types — `capanalyzer` port/policy + `report` model & rendering

**Objective.** Introduce the two pure leaf packages the checker will produce and
consume: the `capanalyzer` capability port + finding/policy types, and the `report`
model with text/JSON rendering. Both are dependency-free and fully testable in
isolation.

**Implementation guidance.**
- `internal/capanalyzer` (design §4.1): `CapabilityFinding`, `Frame`, `Class`
  (`TrueAuthority | AnalysisDefeating`), `InterfaceSymbol`, `AnalyzeRequest`
  (`Packages`, `PruneAt`), the `CapabilityAnalyzer` interface (the port — NFR4/Q1),
  `CapabilityPolicy` (`Allowed`, `Warn`), and `StrictPolicy()` returning the empty
  policy (MVP default). No Capslock import here — this is the pure port; the adapter
  arrives in Step 7.
- `internal/report` (design §6.2): `ConformanceReport`, `Finding`, `Kind`
  (`UNDECLARED_DEPENDENCY | CALLS_UNDECLARED_INTERFACE | UNDECLARED_AUTHORITY |
  ANALYSIS_LIMITATION | …`), `Location`, `Evidence`. Add pure rendering to **text**
  (default) and **JSON** (`--format=json`, FR2) — rendering returns strings/bytes, it
  does not touch stdout (the shell prints).

**Tests.** `capanalyzer`: `StrictPolicy()` has empty `Allowed`/`Warn`; a small helper
that classifies a capability against a policy behaves (allowed / warn / violation).
`report`: hand-build a `ConformanceReport` with a violation + a warning and assert the
exact text rendering and the JSON shape (golden strings).

**Integration.** These types are the checker's output (`report`) and one of its inputs
(`capanalyzer`); Step 4/5 wire them in.

**Demo.** `just test` covers `internal/capanalyzer` and `internal/report`; a test
prints a rendered multi-finding report in both text and JSON form.

---

## Step 4: Checker core I — fact types + dependency rule (FR3) + declared interface (FR4)

**Objective.** Begin the heart of the tool: the pure `checker`. Define the injected
**fact types** it reads, then implement the two Pillar-1 rules that need only
import/symbol facts: the dependency allowlist rule (FR3) and declared-interface
computation (FR4).

**Implementation guidance.**
- Define the `goanalysis` **data types only** as pure structs (design §4.1):
  `PackageFacts`, `PackageFact` (`ImportPath`, `IsStdlib`, `Imports`,
  `ExportedSymbols`), `ExportedSymbol` (`Name`, `File`, `Kind`), `CallEdge`,
  `DependencyInterface` (`Component`, `Packages`, `Symbols`). The authority-holding
  **loaders** (`LoadPackageFacts`, `ResolveDependencyInterface`) are deferred to Steps
  6/9 — defining the structs now lets the checker compile and be unit-tested against
  hand-built facts, per design §8. (These types live in `goanalysis` per the design;
  the checker uses them as data only — no calls — so no authority is absorbed.)
- `internal/checker`: `Inputs` struct (design §4.1) and `Check(Inputs) →
  report.ConformanceReport`.
- **FR3 dependency rule (§5.1):** build the allowed-import set = component deps'
  interface packages ∪ absorbed deps' import paths (pattern-expanded). For each
  component package's non-stdlib import that isn't part of the component itself, if
  it's outside the allowed set → `UNDECLARED_DEPENDENCY` (with importing package +
  import path). **Stdlib imports are auto-allowed at Pillar 1** (§5.2) — governed by
  Pillar 3 instead.
- **FR4 declared interface (§5.3):** the declared interface = exported symbols whose
  `File` ∈ `interface_files`. Exported symbols outside interface files are
  *architecture-private*, **not** a violation of the component's own manifest; may emit
  an informational note listing them.

**Tests.** Table-driven with hand-built `Manifest` + `PackageFacts` (design §8):
conforming imports → no findings; an undeclared non-stdlib import → exactly one
`UNDECLARED_DEPENDENCY`; a stdlib import → no finding; declared interface computed
correctly; an exported symbol outside interface files → informational note, not a
violation.

**Integration.** `Check` now returns a real (partial) `report.ConformanceReport` built
from `manifest` + `capanalyzer` + the fact types — components wired.

**Demo.** `just test` covers `internal/checker`; a test feeds a hand-built component
with one undeclared import and prints the rendered report showing the
`UNDECLARED_DEPENDENCY` violation.

---

## Step 5: Checker core II — boundary rule (FR5) + policy-aware authority rule (FR6)

**Objective.** Complete the pure `checker`: add the cross-component boundary rule and
the ambient-authority rule, both operating on injected data. After this step
`checker.Check` is a **complete, fully unit-tested pure function** — the self-hosting
showcase in miniature, with no Go build or Capslock in the loop.

**Implementation guidance.**
- **FR5 boundary rule (§5.3b):** given `Facts.CallEdges` + resolved `DepIfaces`, for
  each call edge whose callee belongs to a component dependency `B`'s packages, if the
  callee ∉ `B`'s declared-interface symbol set → `CALLS_UNDECLARED_INTERFACE` (caller,
  callee, `B`). (Real call edges arrive in Step 9; here they are hand-built.)
- **FR6 authority rule (§5.4):** derive the effective policy with `Allowed ⊇
  manifest.declared_authority`, then per `CapabilityFinding`: in `Allowed` → no entry;
  in `Warn` → non-fatal `ANALYSIS_LIMITATION`/`ALLOWED_WITH_WARNING` warning; otherwise
  → `UNDECLARED_AUTHORITY` violation carrying the Capslock example call path as
  `Evidence`. Under the MVP `StrictPolicy` both sets are empty (before merging
  `declared_authority`), so any residual capability fails. **In scope:**
  `declared_authority` names populate `Allowed` (the policy seam — this is what lets
  the example's `csvfile` legitimately declare `FILES`). **Out of scope:** the richer
  "capability box" (design §2.3).
- Component-dependency integrity (§5.5) is expressed through FR5 + the resolved
  `DepIfaces`; a failed dependency-manifest resolution surfaces as a tool error in the
  shell (Step 9), not here.

**Tests.** Table-driven (design §8): call edge into a non-interface symbol of a
component dep → `CALLS_UNDECLARED_INTERFACE`; edge into a declared-interface symbol →
clean. Authority: a `FILES` finding under `StrictPolicy` → `UNDECLARED_AUTHORITY` with
evidence; the same finding when `declared_authority: "FILES"` → no violation; a
`Warn`-set capability → warning, exit-neutral. A fully-conforming input → empty report.

**Integration.** All checker rules now composed in one `Check`; the pure core is
feature-complete and demoable against golden `ConformanceReport`s.

**Demo.** `just test` on `internal/checker` covers all four rule families. A test
prints three rendered reports from hand-built inputs: a boundary violation, an
authority violation-with-evidence, and a clean pass.

---

## Step 6: `goanalysis` loader I — imports + exported-symbol→file extraction

**Objective.** Build the first half of the authority-holding shell: load real Go
packages and extract the facts the Pillar-1 rules need (imports, stdlib flag,
exported-symbol→file map). Wire real facts through the pure core for the **first
vertical end-to-end slice** (Pillar-1 verdict, no authority yet).

**Implementation guidance.**
- `internal/goanalysis`: implement `LoadPackageFacts(pkgPatterns) → PackageFacts` using
  `golang.org/x/tools/go/packages` with a load mode covering types/syntax/imports/deps/
  module (the same loader Capslock uses — one load feeds both pillars,
  research/go-component-model.md). Extract per package: direct imports, `IsStdlib` (via
  module/std detection), and exported top-level decls via `go/ast` (`ast.IsExported`)
  mapped to the declaring file, in the Capslock/go-types **key form** (`InterfaceSymbol`)
  for symbols. Leave `CallEdges` empty for now (Step 9 fills them). This component
  declares **FILES, EXEC, READ_SYSTEM_STATE**.
- Package-load errors surface verbatim and will abort with exit 2 in the shell (design
  §7) — return them as errors here.

**Tests.** Integration tests (the `just test-integration` suite) against small fixture
packages under `internal/goanalysis/testdata/` (design §8): assert extracted imports
and the exported-symbol→file mapping for a known fixture; assert `IsStdlib`
classification.

**Integration.** In a test (or a tiny internal harness), run `LoadPackageFacts` on a
fixture and hand the result to `checker.Check` with an empty analyzer result — real
extracted facts flow through the pure core to a real Pillar-1 verdict. **This is the
first vertical slice**, proving the shell→core contract before Capslock exists.

**Demo.** `just test-integration` covers `internal/goanalysis`; a test prints the
imports and symbol→file map for a fixture, then prints a `checker`-produced Pillar-1
report for a fixture with a deliberately undeclared import.

---

## Step 7: `capslockadapter` — Capslock-backed `CapabilityAnalyzer`

**Objective.** Complete the shell's fact production: implement the `CapabilityAnalyzer`
port with Capslock as a library, mapping Capslock's `CapabilityInfoList` onto
`capanalyzer.CapabilityFinding`/`Class`. (Boundary **pruning** via `PruneAt` is
stubbed/empty here; it is activated in Step 9.)

**Implementation guidance.**
- `internal/capslockadapter`: implement `Analyze(AnalyzeRequest) →
  ([]CapabilityFinding, error)` (design §4.1) by calling `analyzer.GetCapabilityInfo(
  pkgs, queried, config)` (research/capslock.md). Map each `CapabilityInfo` to a
  `CapabilityFinding`: package, capability name, `Class` (true-authority vs
  analysis-defeating per the taxonomy in research/capslock.md), and the example call
  `path` → `[]Frame` (func/file/line) as evidence. **Empty list ⇒
  ambient-authority-free.** Declares **FILES, EXEC, READ_SYSTEM_STATE**.
- Honor `AnalyzeRequest.Packages`; accept `PruneAt` in the signature but treat empty as
  "no pruning" (full Capslock transitivity — which already gives **absorption** of
  absorbed deps for free, FR7). The `CAPABILITY_SAFE` custom-map pruning is Step 9.
- **Open item to settle here or in Step 9:** call-graph algorithm / config (CHA vs RTA
  vs VTA, or reuse Capslock's) — design Appendix D#3.

**Tests.** Integration tests (`just test-integration`, design §8) against fixture
packages with known capabilities: a package that reads a file → expect a `FILES`
finding with a plausible call path; a pure-arithmetic package → expect **no** findings.
Guards the Capslock-proto → `CapabilityFinding`/`Class` mapping.

**Integration.** The adapter satisfies the `capanalyzer.CapabilityAnalyzer` port, so it
is now injectable wherever the port is expected (the CLI in Step 8).

**Demo.** `just test-integration` covers `internal/capslockadapter`; a test runs the
adapter on a file-reading fixture and prints the `FILES` finding with its call path,
and on a pure fixture prints "ambient-authority-free (no findings)."

---

## Step 8: CLI/app orchestration + CSV example — first full end-to-end

**Objective.** Wire everything into the `archcheck` CLI and prove it on a real example
project: read a manifest, load facts, run Capslock, check, render, exit correctly —
**the first full end-to-end including the ambient-authority check** (FR2, FR9, FR7).

**Implementation guidance.**
- `internal/app` (shell orchestration) + `cmd/archcheck` (replace the Step-1 stub):
  parse args (`archcheck check COMPONENT.textproto`, `--format=json`), read manifest
  bytes (FILES), `manifest.Parse`, `goanalysis.LoadPackageFacts`,
  `capslockadapter.Analyze` (empty `PruneAt` for now), `checker.Check` with
  `StrictPolicy()` merged with `declared_authority`, render via `report`, print, and
  map to **exit codes** (design §7): `0` conforms, `1` violations, `2` tool error
  (manifest parse, package load, Capslock failure, interface file missing). Tool errors
  go to stderr and never masquerade as a pass. `--format=json` selects JSON rendering
  (FR2).
- `go/examples/csvtool` (design §10) — build the parts that do **not** require the call
  graph yet:
  - `toprow` — pure logic (sort/pick already-parsed rows), imports only the absorbed
    `internal/parsecsv` helper + stdlib-safe (`sort`, `strconv`); manifest declares
    **no authority** and an `absorbed_dependency` on `parsecsv`.
  - `internal/parsecsv` — absorbed impl-detail dep (wraps `encoding/csv`; no manifest).
  - `csvfile` — a component that legitimately `declared_authority: "FILES"` and exposes
    `Read(path) ([][]string, err)`; checked on its own it **conforms** (FILES is
    declared → in `Allowed`).
  - Keep `examples/` as a nested module or `testdata`-style so it doesn't pollute the
    tool's own dependency graph (design §9).

**Tests.** End-to-end golden tests (design §8) invoking the CLI:
- `toprow` → exit 0, report says ambient-authority-free (**FR7**: parsecsv's use is
  absorbed but touches no FS, so `toprow` stays authority-free).
- `csvfile` → exit 0 (declares the `FILES` it uses).
- Failing variant: `toprow` manifest omits the `parsecsv` `absorbed_dependency` it
  imports → exit 1, `UNDECLARED_DEPENDENCY`.
- Failing variant: an `app` that **absorbs** `csvfile` (or calls `os.Open` directly)
  while claiming no authority → exit 1, `UNDECLARED_AUTHORITY` with the Capslock path as
  evidence (the absorb-vs-prune contrast; the prune half lands in Step 9).
- A malformed manifest → exit 2 (tool error, distinct from a conformance failure).

**Integration.** All seven components are now wired end to end behind one CLI. The
`CapabilityAnalyzer` port is injected, so tests can substitute a fake analyzer for
speed where useful.

**Demo.** From the shell: `archcheck check examples/csvtool/toprow/COMPONENT.textproto`
prints a green "conforms — ambient-authority-free" report (exit 0); running it on the
undeclared-dependency and direct-`os.Open` variants prints actionable violations (exit
1); `--format=json` emits the machine-readable report.

---

## Step 9: Call graph + FR5/FR5b — SSA edges, dependency-interface resolution, boundary pruning

**Objective.** Add the remaining, most sophisticated piece: the SSA call graph that
powers the cross-component **boundary check (FR5)** and **capability pruning (FR5b)**,
making the *component-dep vs absorbed-dep* distinction mechanically precise. Complete
the CSV example with the multi-component `app` that **composes a FILES-holding
component yet checks ambient-authority-free** — the pruning showcase.

**Implementation guidance.**
- `goanalysis`: build the SSA **call graph** (settle the algorithm — Appendix D#3) and
  populate `CallEdges` (caller→callee in `InterfaceSymbol` key form). Implement
  `ResolveDependencyInterface(dep)` — load the dependency's manifest + its interface
  files → that dependency's declared-interface symbol set (design §4.1/§5.5). A
  dependency manifest that fails to resolve/load → tool error (exit 2).
- Feed the resolved `DepIfaces` two ways (design §5.3b — "one primitive, both
  pillars"): (a) into `checker` so the **FR5** boundary rule fires on real edges, and
  (b) as `AnalyzeRequest.PruneAt` into `capslockadapter`.
- `capslockadapter`: implement `PruneAt` by generating a per-run custom capability map
  marking each pruned symbol `CAPABILITY_SAFE` (which terminates traversal), loaded via
  `interesting.LoadClassifier(..., excludeBuiltin=false)` so it merges with builtins
  (research/capslock.md; key format `func <path>.<Name>` /
  `func (*<path>.<Type>).<Method>`). Authority behind a component dep's interface is now
  attributed to the dependency, not the analyzed component.
- Extend `examples/csvtool` with `app` (composition root): depends on `csvfile` as a
  **component dependency** and on `toprow`; because traversal is pruned at `csvfile`'s
  interface, `app` is **not** attributed `FILES` and checks **ambient-authority-free**
  (design §10).

**Tests.**
- `goanalysis` integration (`just test-integration`): assert expected cross-component
  call edges and resolved interface symbol sets for the `app`→`csvfile`/`toprow`
  fixture.
- `checker` already covers FR5 with hand-built edges (Step 5); add an end-to-end golden
  test where `app` calls a `csvfile` symbol that is Go-exported but **not** in
  `csvfile`'s interface files → `CALLS_UNDECLARED_INTERFACE` (exit 1).
- Pruning golden test: the well-formed `app` → exit 0, ambient-authority-free (proves
  FR5b — composed FILES not re-absorbed). Contrast with the Step 8 absorb variant to
  show absorb-vs-prune is exactly what differs.
- **Precision caveat (design §11):** document that dynamic dispatch over-approximates
  edges; pruning stays conservative so it never hides real authority. Capture a known
  over-approximation case as an xfail/documented test if one surfaces.

**Integration.** The two pillars are now fully connected through the single `DepIfaces`
primitive; the CSV example demonstrates every concept in the design.

**Demo.** `archcheck check examples/csvtool/app/COMPONENT.textproto` → **conforms,
ambient-authority-free**, even though `app` orchestrates a file-reading component; the
non-interface-call variant → `CALLS_UNDECLARED_INTERFACE`; side-by-side with the absorb
variant from Step 8, the report makes the prune-vs-absorb distinction visible.

---

## Step 10: Self-hosting (FR8) — own component manifests + CI check of the core

**Objective.** Close the loop: decompose the tool itself into manifested components and
run `archcheck` on its own core, proving the pure core is genuinely
ambient-authority-free (FR8) — the project's thesis, dogfooded, enforced in CI.

**Implementation guidance.**
- Author `go/components/*.COMPONENT.textproto` for each component (design §4 table):
  - **Authority-free core:** `checker`, `report`, `capanalyzer` — declare no authority;
    component dependencies wired per the real import graph (e.g. `checker` depends on
    `manifest`/`report`/`capanalyzer` and uses `goanalysis` fact **types**).
  - **Shell:** `goanalysis`, `capslockadapter`, `cli`/`app` — declare their real
    authority (`FILES`, `EXEC`, `READ_SYSTEM_STATE`).
- **Resolve the Step-2 `manifest`/reflection risk here.** If `prototext` triggers
  `CAPABILITY_REFLECT`, `manifest` is not strictly authority-free. Options, in order of
  preference: (a) scope the guaranteed-authority-free **showcase** to
  `checker`/`report`/`capanalyzer` (design FR8: "at least the pure checking core"); (b)
  give `manifest` a manifest that declares/permits `REFLECT` via the policy seam; (c)
  later, a reflection-free textproto parse. Pick (a) for the MVP and document.
- Add a **self-hosting CI job** to `.github/workflows/ci.yml` (and a `just selfcheck`
  target, design §8): run `archcheck` on the core-component manifests; the pure core
  must verify ambient-authority-free, and every shell component must conform to its
  declared authority. Fail CI on any violation.

**Tests.** The self-hosting run itself is the test: `archcheck` on
`checker.COMPONENT.textproto` (and `report`, `capanalyzer`) → exit 0,
ambient-authority-free; on `goanalysis`/`capslockadapter`/`cli` → exit 0 with their
declared authority. Wire it as `just selfcheck` invoked from CI (and optionally a
`go test`).

**Integration.** The tool now verifies itself in CI; the MVP is feature-complete
against FR1–FR9 and NFR1–NFR4.

**Demo.** `archcheck check go/components/checker.COMPONENT.textproto` prints "conforms —
ambient-authority-free"; the CI self-hosting job is green; a deliberate edit that makes
the core touch the filesystem turns the core's own check red — the architectural
guarantee, enforced on the tool itself.

---

## Coverage map (design → steps)

| Requirement | Step(s) |
|---|---|
| FR1 manifest | 2 |
| FR2 CLI / exit codes / JSON | 3 (render), 8 (wire) |
| FR3 dependency conformance | 4 |
| FR4 declared interface | 4 |
| FR5 cross-component boundary | 5 (rule), 9 (real edges) |
| FR5b capability pruning | 9 |
| FR6 authority-free + policy | 5 (rule), 7 (adapter), 8 (wire) |
| FR7 absorption | 7 (transitivity), 8 (example) |
| FR8 self-hosting | 10 |
| FR9 examples | 8, 9 |
| NFR1 layout | 1 |
| NFR2 protobuf/textproto | 2 |
| NFR3 Bazel-ready lists | 2 (schema shape) |
| NFR4 Capslock behind port | 3 (port), 7 (adapter) |
| Dev tooling (justfile, CI, release) | 1 |

## Open items to settle during implementation (design Appendix D)

1. **CLI name** — working name `archcheck` (confirm or replace) — affects Steps 1/8.
2. **Call-graph algorithm** (CHA/RTA/VTA or reuse Capslock's) — settle in Step 7/9.
3. **Generated-proto delivery** — committed `gen/` (recommended) vs a gen step — Step 2.
4. **`manifest` reflection** vs authority-free claim — resolve in Step 10 (scope
   showcase to `checker`/`report`/`capanalyzer`).
5. **Whole-graph vs single-component** check mode — MVP does single-component +
   resolve-direct-deps (Step 9); whole-graph is a post-MVP extension.
6. **Release tooling** — plain `go build` matrix vs GoReleaser for `release.yml` — Step 1.
