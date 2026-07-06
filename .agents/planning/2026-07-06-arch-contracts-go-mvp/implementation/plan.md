# Implementation Plan — Architectural Contracts MVP (Go)

Source design: [`../design/detailed-design.md`](../design/detailed-design.md).
Supporting artifacts: `../idea-honing.md`, `../research/capslock.md`,
`../research/go-component-model.md`, and the design review with resolutions:
[`../design-review.md`](../design-review.md).

**Guiding principle.** Build each component in a test-driven manner, in increments
that each leave the tree in a working, demoable state. **De-risk first** (Step 0
spike — the design's load-bearing Capslock assumptions get executed, not just
read), then set up the project skeleton and CI so every later step lands on
green. Then sequence the **pure core** (it is a pure function of injected data,
so the bulk of the test suite needs no Go build and no Capslock — design §8),
then the authority-holding shell that feeds it real facts, then wire the CLI for
the first real end-to-end verdict, then add the call-graph-based
boundary/pruning checks, and finally self-host. No hanging or orphaned code:
every step ends by wiring its output into something runnable.

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

- [ ] **Step 0 — Capslock validation spike** (draft `examples/csvtool` packages as probes; execute the design's Capslock assumptions: strict-safe stdlib envelope, `CAPABILITY_SAFE` pruning, key formats incl. method/generic/init)
- [ ] **Step 1 — Project scaffold & developer tooling** (repo layout, `go/` module skeleton, `justfile`, CI workflow — release workflow deferred to Step 10)
- [ ] **Step 2 — Manifest schema & parser** (`proto/`, codegen, `internal/manifest`)
- [ ] **Step 3 — Pure supporting types: `facts` data model, `capanalyzer` port/policy, `report` model & text rendering**
- [ ] **Step 4 — Checker core I: dependency rule (FR3) + declared-interface closure (FR4)**
- [ ] **Step 5 — Checker core II: boundary rule (FR5) + policy-aware authority rule (FR6)** — pure core complete
- [ ] **Step 6 — `goanalysis` loader I: imports + exported-symbol→file extraction** (first vertical slice)
- [ ] **Step 7 — `capslockadapter`: Capslock-backed `CapabilityAnalyzer`**
- [ ] **Step 8 — CLI/app orchestration + CSV example** — first full end-to-end
- [ ] **Step 9 — Call graph + FR5/FR5b: VTA edges, dependency-interface resolution, boundary pruning** — full concept
- [ ] **Step 10 — Self-hosting (FR8): own component manifests + CI check of the core; release workflow**

---

## Step 0: Capslock validation spike

**Objective.** Execute — not just read — the Capslock assumptions the whole design
stands on, before building five steps of pure core on top of them (review B8).
Simultaneously draft the `examples/csvtool` packages (design §10) in rough form and
use them as the spike's probe targets, so the examples exist early and are
*refined*, not created, in Steps 8/9.

**Implementation guidance.**
- Create a throwaway module (or the future `go/examples/csvtool` layout directly)
  containing rough versions of `toprow`, `internal/parsecsv`, `csvfile`, and `app`,
  plus tiny probe packages exercising: `fmt`, `sort.Sort` vs `sort.Slice`,
  `slices.SortFunc`, `strconv`, `errors`, an explicit `func init()` with authority,
  a type in one file with methods in another, an exported interface type +
  concrete impl, a generic function, pointer- and value-receiver methods.
- Drive `analyzer.GetCapabilityInfo` (Capslock as a library) over the probes and
  validate:
  1. **Strict-safe stdlib envelope** — which stdlib entry points are usable by the
     examples and the pure core under `StrictPolicy` (known already:
     `sort.Slice` is `unanalyzed` → fails strict; `sort.Ints/Strings`, `fmt` are
     SAFE; establish the rest empirically).
  2. **`CAPABILITY_SAFE` pruning end-to-end** — a custom `.cm` via
     `interesting.LoadClassifier(..., excludeBuiltin=false)` marking `csvfile`'s
     interface symbols (and `func <pkg>.init`) SAFE makes `app` come out
     capability-free; without it, `FILES` is attributed.
  3. **Key formats & normalization (review A4)** — confirm the SSA name forms
     actually emitted for: methods (both receiver forms), generic instantiations
     (bracket-stripping works), promoted-method wrappers, `init`/`init#N`.
  4. **Init attribution (review A2)** — an authority-using `func init()` in a
     dependency is attributed to the importer unless `func <pkg>.init` is pruned.
- Record results in a short `research/spike-capslock.md` (the strict-safe stdlib
  list feeds design §10 and the Step 8/10 examples; any surprises go back into the
  design before Step 3).

**Tests.** The spike itself is exploratory; its deliverable is the findings note +
the draft example packages. No CI wiring yet.

**Integration.** Findings feed the design (stdlib envelope, key normalization
rules); the draft examples are carried into the repo layout in Step 1 and wired
into golden tests in Step 8/9.

**Demo.** A transcript in `research/spike-capslock.md` showing: probe package X →
expected findings; pruned `app` → empty capability list; unpruned → `FILES`.

---

## Step 1: Project scaffold & developer tooling

**Objective.** Stand up the multi-language repo layout, a **buildable** `go/` module
skeleton, and the developer/CI tooling — a `justfile` and a GitHub Actions CI
workflow — so that from here on every step is validated by `just` targets and CI on
each push. CI lands **early by design** (review D15: local-pass/CI-fail divergence
has bitten before); the release workflow is deferred to Step 10. Nothing to check
yet; the deliverable is a green, reproducible pipeline.

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
  green; Step 10 adds the self-hosting `selfcheck` job and the release workflow.
- Bring the Step-0 draft `examples/csvtool` packages into the repo layout
  (`go/examples/csvtool`, as a nested module or `testdata`-style per design §9) so
  they build under CI from the start.

**Tests.** No product code to unit-test yet. The "test" is that `just build`,
`just test`, and `just test-integration` all succeed (trivially, on the stub) and the
CI workflow passes on a first push. Add a single smoke test (`archcheck --version`
prints the expected string) so `just test` exercises a real assertion.

**Integration.** This step wires the *process*, not components — everything after it
plugs into `just` targets and the CI gate.

**Demo.** `just build && just test && just test-integration` all green locally;
pushing the branch shows a green CI run.

---

## Step 2: Manifest schema & parser

**Objective.** Define the manifest data model: a shared protobuf schema, generated Go
code, and a **pure** `manifest.Parse(bytes) → Manifest` with validation. This is the
foundation the checker consumes.

**Implementation guidance.**
- Create `proto/archcontracts/v1/component.proto` per design §6.1 (`Component`,
  `ComponentDependency`, `AbsorbedDependency`). Keep the dependency and
  interface-file fields plain repeated strings so a future Bazel rule can populate
  them mechanically (NFR3). Per review C10 there is **no** `interface_packages`
  field (number reserved) — a dependency's interface packages are *derived* from
  its own manifest. Path semantics per C9/C12: `interface_files` are relative to
  the manifest file's directory (= component root); `ComponentDependency.manifest`
  is relative to the declaring manifest's directory.
- Wire the real **`gen`** justfile target to protobuf codegen. **Decision (recommend):
  commit the generated code** under `internal/manifest/gen/` (design §9 offers this or
  a gen step) so the build needs no `protoc`; the CI `gen`-is-clean check then just
  verifies the committed code matches the proto.
- `internal/manifest`: parse `.textproto` bytes with
  `google.golang.org/protobuf/encoding/prototext` into the generated message, then map
  to a hand-written Go-native `Manifest` model (so the rest of the code doesn't depend
  on protobuf types), and **validate — syntactic checks only** (design §7, review C9):
  empty `name`, empty `packages`, duplicate declarations, and an unknown capability
  name in `declared_authority` (validated against the known capability set — review
  C11) → parse error. Checks that need resolved packages (interface file exists /
  belongs to a component package) are the **shell's** job (Steps 6/9), because pure
  `Parse` cannot resolve package patterns to directories.
- `Parse` takes **bytes**, not a path — reading the file is the shell's job. This keeps
  `manifest` pure and injectable.

**Tests.** Table-driven unit tests for `Parse`: a valid manifest (e.g. the `toprow`
example from §6.1) round-trips to the expected model; one test per validation error
(empty name, empty packages, duplicate, unknown capability name like `"FILE"`)
asserts the specific error.

**Integration.** Consumed by Step 4's checker. The `gen` target and its CI clean-check
are now real, exercised by every subsequent CI run.

**Risk note (carry to Step 10).** `prototext.Unmarshal` uses reflection, so the
`manifest` component may itself trigger `CAPABILITY_REFLECT`. That does **not** affect
correctness now, but it means `manifest` may not qualify as ambient-authority-free
under `StrictPolicy`. Flag it; resolve in Step 10 (the guaranteed authority-free
showcase is `facts`/`checker`/`report`/`capanalyzer`; `report` stays clean because
JSON marshaling lives in the shell — review B6).

**Demo.** `just test` covers `internal/manifest`; a test prints the parsed model for
the example `toprow.COMPONENT.textproto` and the specific error for a
deliberately-malformed manifest. `just gen` regenerates cleanly.

---

## Step 3: Pure supporting types — `facts` data model + `capanalyzer` port/policy + `report` model & text rendering

**Objective.** Introduce the three pure leaf packages the checker will produce and
consume: the `facts` data model (review B7 — core-defined, shell-produced), the
`capanalyzer` capability port + finding/policy types, and the `report` model with
text rendering. All are dependency-free and fully testable in isolation.

**Implementation guidance.**
- `internal/facts` (design §4.1, review B7): `PackageFacts`, `PackageFact`
  (`ImportPath`, `IsStdlib`, `Imports`, `ExportedSymbols`), `ExportedSymbol`
  (`Name`, `File`, `Kind` incl. `init`, `Receiver` — the receiver-type key that
  drives the FR4 closure), `CallEdge` (`Caller`, `Callee`, `PassesFuncValue`),
  `DependencyInterface` (`Component`, `Packages`, `Symbols`). Pure structs, no
  loaders — the authority-holding loaders live in `goanalysis` (Steps 6/9). This
  placement keeps every dependency pointing **inward** (never core→shell).
- `internal/capanalyzer` (design §4.1): `CapabilityFinding`, `Frame`, `Class`
  (`TrueAuthority | AnalysisDefeating`), `InterfaceSymbol` (with the A4
  normalization contract documented: generic brackets stripped, both receiver
  forms emitted), `AnalyzeRequest` (`Packages`, `PruneAt`), the
  `CapabilityAnalyzer` interface (the port — NFR4/Q1), `CapabilityPolicy`
  (`Allowed`, `Warn`), and `StrictPolicy()` returning the empty policy (MVP
  default). No Capslock import here — this is the pure port; the adapter arrives
  in Step 7.
- `internal/report` (design §6.2): `ConformanceReport`, `Finding`, `Kind`
  (violations `UNDECLARED_DEPENDENCY | CALLS_UNDECLARED_INTERFACE |
  UNDECLARED_AUTHORITY | INIT_OUTSIDE_INTERFACE | PACKAGE_OVERLAP`; warnings
  `ANALYSIS_LIMITATION | ALLOWED_WITH_WARNING | HIGHER_ORDER_BOUNDARY_CALL |
  UNUSED_DEPENDENCY`), `Location`, `Evidence`. Add pure rendering to **text**
  only — rendering returns strings, it does not touch stdout (the shell prints).
  **JSON is *not* rendered here** (review B6): `encoding/json` reaches `reflect`
  and would break `report`'s authority-free claim; the shell marshals the report
  struct in Step 8. Give the structs the JSON tags now so the shell-side
  marshaling is stable.

**Tests.** `capanalyzer`: `StrictPolicy()` has empty `Allowed`/`Warn`; a small helper
that classifies a capability against a policy behaves (allowed / warn / violation).
`report`: hand-build a `ConformanceReport` with a violation + a warning and assert the
exact text rendering (golden string).

**Integration.** These types are the checker's output (`report`) and its inputs
(`facts`, `capanalyzer`); Step 4/5 wire them in.

**Demo.** `just test` covers `internal/facts`, `internal/capanalyzer`, and
`internal/report`; a test prints a rendered multi-finding report in text form.

---

## Step 4: Checker core I — dependency rule (FR3) + declared-interface closure (FR4)

**Objective.** Begin the heart of the tool: the pure `checker`, consuming the
`facts` model from Step 3. Implement the two Pillar-1 rules that need only
import/symbol facts: the dependency allowlist rule (FR3) and the
declared-interface **closure** computation (FR4, review A1).

**Implementation guidance.**
- `internal/checker`: `Inputs` struct (design §4.1 — `manifest.Manifest`,
  `facts.PackageFacts`, `[]facts.DependencyInterface`, `[]capanalyzer.
  CapabilityFinding`, `capanalyzer.CapabilityPolicy`) and `Check(Inputs) →
  report.ConformanceReport`. The fact types already exist (Step 3, pure) — the
  authority-holding loaders arrive in Steps 6/9; unit tests run on hand-built
  facts, per design §8.
- **FR3 dependency rule (§5.1):** build the allowed-import set = component deps'
  interface packages (as resolved into `DependencyInterface.Packages`) ∪ absorbed
  deps' import paths (glob-matched against literal import strings — pure). For each
  component package's non-stdlib import that isn't part of the component itself, if
  it's outside the allowed set → `UNDECLARED_DEPENDENCY` (with importing package +
  import path). **Stdlib imports are auto-allowed at Pillar 1** (§5.2) — governed by
  Pillar 3 instead. A declared dependency matching no import → `UNUSED_DEPENDENCY`
  **warning** (review C13).
- **FR4 declared-interface closure (§5.3, review A1/A2):** the declared interface =
  (a) exported symbols whose `File` ∈ `interface_files`, plus (b) methods whose
  `Receiver` type is declared in an interface file (even if the method body is in
  another file). Exported symbols outside the closure are *architecture-private*,
  **not** a violation of the component's own manifest; may emit an informational
  note listing them. An explicit `init` (Kind `init`) declared outside interface
  files → `INIT_OUTSIDE_INTERFACE` violation.

**Tests.** Table-driven with hand-built `Manifest` + `facts.PackageFacts` (design
§8): conforming imports → no findings; an undeclared non-stdlib import → exactly one
`UNDECLARED_DEPENDENCY`; a stdlib import → no finding; an unused declared dep →
`UNUSED_DEPENDENCY` warning; closure computed correctly (incl. the
type-in-interface-file / method-elsewhere case); an exported symbol outside the
closure → informational note, not a violation; explicit init outside interface
files → `INIT_OUTSIDE_INTERFACE`.

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
  callee ∉ `B`'s declared-interface symbol set (FR4 closure; matching uses the A4
  normalization — strip generic brackets, accept both receiver forms) →
  `CALLS_UNDECLARED_INTERFACE` (caller, callee, `B`). An edge **into** a declared
  interface symbol with `PassesFuncValue` → `HIGHER_ORDER_BOUNDARY_CALL` warning
  (review A3). Package-set overlap between the component and a resolved dependency
  → `PACKAGE_OVERLAP` violation (§5.5, review C12). (Real call edges arrive in
  Step 9; here they are hand-built.)
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
clean; edge into a generic symbol / value-receiver form → clean under normalization;
func-valued edge into a declared symbol → `HIGHER_ORDER_BOUNDARY_CALL` warning;
overlapping package sets → `PACKAGE_OVERLAP`. Authority: a `FILES` finding under
`StrictPolicy` → `UNDECLARED_AUTHORITY` with evidence; the same finding when
`declared_authority: "FILES"` → no violation; a `Warn`-set capability → warning,
exit-neutral (warnings-only → exit 0, design §7/C14). A fully-conforming input →
empty report.

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
- `internal/goanalysis`: implement `LoadPackageFacts(pkgPatterns) →
  facts.PackageFacts` (core-defined types, review B7) using
  `golang.org/x/tools/go/packages` with a load mode covering types/syntax/imports/deps/
  module (the same loader Capslock uses — one load feeds both pillars,
  research/go-component-model.md). Extract per package: direct imports, `IsStdlib` (via
  module/std detection), exported top-level decls via `go/ast` (`ast.IsExported`)
  mapped to the declaring file — including **`Receiver`** linkage for methods (drives
  the FR4 closure) and explicit **`init`** decls (Kind `init`, review A2) — in the
  Capslock/go-types **key form** (`InterfaceSymbol`, both receiver forms per A4).
  Leave `CallEdges` empty for now (Step 9 fills them). This component declares
  **FILES, EXEC, READ_SYSTEM_STATE**.
- Implement the **resolved manifest validation** that pure `Parse` can't do (design
  §7, review C9): each `interface_files` entry exists on disk and belongs to one of
  the component's resolved packages → else tool error (exit 2).
- Package-load errors surface verbatim and will abort with exit 2 in the shell (design
  §7) — return them as errors here.

**Tests.** Integration tests (the `just test-integration` suite) against small fixture
packages under `internal/goanalysis/testdata/` (design §8): assert extracted imports
and the exported-symbol→file mapping for a known fixture; assert `IsStdlib`
classification; assert `Receiver` linkage and `init` extraction; fixtures cover the
design-§8 closure corner cases (type here / methods there, interface type + concrete
impl, generic func, both receiver forms, promoted method, explicit init).

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
- **Pin Capslock** in `go/go.mod` (pre-1.0 API — review D16); upgrades are deliberate,
  reviewed events. The call-graph algorithm question is settled (review A5): Capslock
  uses **VTA** internally, and Step 9's `goanalysis` graph uses `vta.CallGraph` to
  match.
- Sanity-check the adapter's behavior against the Step-0 spike findings (same probe
  packages, now as committed fixtures where useful).

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
  `StrictPolicy()` merged with `declared_authority`, render via `report` (text), and
  map to **exit codes** (design §7): `0` conforms (warnings alone stay 0 — C14),
  `1` violations, `2` tool error (manifest parse, package load, Capslock failure,
  interface file missing). Tool errors go to stderr and never masquerade as a pass.
  `--format=json` marshals the `report.ConformanceReport` struct with
  `encoding/json` **here in the shell** (review B6 — kept out of the pure core;
  `cli`'s own manifest will declare `REFLECT`, Step 10).
- `go/examples/csvtool` — **refine the Step-0 draft packages** (design §10) for the
  parts that do **not** require the call graph yet:
  - `toprow` — pure logic (sort/pick already-parsed rows), imports only the absorbed
    `internal/parsecsv` helper + strict-safe stdlib per the Step-0 envelope
    (**`sort.Sort` with a concrete `sort.Interface`, not `sort.Slice`** — review
    B8); manifest declares **no authority** and an `absorbed_dependency` on
    `parsecsv`.
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
- Failing variant: a throwaway composition fixture **`absorbapp`** (review D17 — the
  real `app` arrives in Step 9) that **absorbs** `csvfile` (or calls `os.Open`
  directly) while claiming no authority → exit 1, `UNDECLARED_AUTHORITY` with the
  Capslock path as evidence (the absorb-vs-prune contrast; the prune half lands in
  Step 9).
- A malformed manifest (incl. a bad capability name in `declared_authority`) →
  exit 2 (tool error, distinct from a conformance failure).
- `--format=json` output round-trips (shell-marshaled).

**Integration.** All seven components are now wired end to end behind one CLI. The
`CapabilityAnalyzer` port is injected, so tests can substitute a fake analyzer for
speed where useful.

**Demo.** From the shell: `archcheck check examples/csvtool/toprow/COMPONENT.textproto`
prints a green "conforms — ambient-authority-free" report (exit 0); running it on the
undeclared-dependency and direct-`os.Open` variants prints actionable violations (exit
1); `--format=json` emits the machine-readable report.

---

## Step 9: Call graph + FR5/FR5b — VTA edges, dependency-interface resolution, boundary pruning

**Objective.** Add the remaining, most sophisticated piece: the VTA call graph that
powers the cross-component **boundary check (FR5)** and **capability pruning (FR5b)**,
making the *component-dep vs absorbed-dep* distinction mechanically precise. Complete
the CSV example with the multi-component `app` that **composes a FILES-holding
component yet checks ambient-authority-free** — the pruning showcase.

**Implementation guidance.**
- `goanalysis`: build the call graph with **`vta.CallGraph`** — matching Capslock's
  internal algorithm exactly, so the two pillars agree on which edges exist (review
  A5; Capslock's own graph is unexported, so the double build is an accepted MVP
  cost). Populate `CallEdges` (caller→callee in `InterfaceSymbol` key form,
  normalized per A4 — strip generic type-argument brackets), including
  `PassesFuncValue` on call sites passing function-typed values (review A3).
  Implement `ResolveDependencyInterface(dep)` — load the dependency's manifest +
  its interface files → that dependency's declared-interface symbol set as the
  **FR4 closure** (review A1: interface-file decls + method sets of interface-file
  types via `go/types`, incl. concrete in-component implementations of
  interface-file interface types; both receiver key forms). A dependency manifest
  that fails to resolve/load, or whose `name` mismatches the declaration (C12) →
  tool error (exit 2).
- Feed the resolved `DepIfaces` two ways (design §5.3b — "one primitive, both
  pillars"): (a) into `checker` so the **FR5** boundary rule fires on real edges, and
  (b) as `AnalyzeRequest.PruneAt` into `capslockadapter` — **plus
  `func <pkg>.init` for each dependency package** (review A2; precedent:
  `func encoding/json.init CAPABILITY_SAFE` in the builtin map).
- `capslockadapter`: implement `PruneAt` by generating a per-run custom capability map
  marking each pruned symbol `CAPABILITY_SAFE` (which terminates traversal), loaded via
  `interesting.LoadClassifier(..., excludeBuiltin=false)` so it merges with builtins
  (research/capslock.md; key format `func <path>.<Name>` /
  `func (*<path>.<Type>).<Method>`, validated in the Step-0 spike). Authority behind a
  component dep's interface is now attributed to the dependency, not the analyzed
  component.
- Extend `examples/csvtool` with the real `app` (composition root; refined from the
  Step-0 draft): depends on `csvfile` as a **component dependency** and on `toprow`;
  because traversal is pruned at `csvfile`'s interface, `app` is **not** attributed
  `FILES` and checks **ambient-authority-free** (design §10).

**Tests.**
- `goanalysis` integration (`just test-integration`): assert expected cross-component
  call edges and resolved interface symbol sets for the `app`→`csvfile`/`toprow`
  fixture — including the closure corner cases (interface-file type with methods
  elsewhere; interface type + concrete impl; generic symbol normalization; init
  keys in the prune set).
- `checker` already covers FR5 with hand-built edges (Step 5); add an end-to-end golden
  test where `app` calls a `csvfile` symbol that is Go-exported but **not** in
  `csvfile`'s declared interface → `CALLS_UNDECLARED_INTERFACE` (exit 1).
- Pruning golden test: the well-formed `app` → exit 0, ambient-authority-free (proves
  FR5b — composed FILES not re-absorbed). Contrast with the Step 8 `absorbapp`
  variant to show absorb-vs-prune is exactly what differs.
- Init-pruning golden test: a dependency variant with an authority-using explicit
  `init` (declared in its interface file) → the *dependent* still checks clean
  (init pruned); the dependency's own check surfaces the authority.
- Higher-order warning test: a fixture passing a func value into a pruned interface
  symbol → `HIGHER_ORDER_BOUNDARY_CALL` warning, exit 0 (documented §11 limitation).
- **Precision caveat (design §11):** document that dynamic dispatch over-approximates
  edges; pruning is conservative *except* for the documented higher-order leak
  (warned on). Capture a known over-approximation case as an xfail/documented test if
  one surfaces.

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
  - **Authority-free core:** `facts`, `checker`, `report`, `capanalyzer` — declare no
    authority; component dependencies wired per the real import graph (e.g. `checker`
    depends on `manifest`/`facts`/`report`/`capanalyzer` — all pure, all inward;
    review B7).
  - **Shell:** `goanalysis`, `capslockadapter` — declare `FILES`, `EXEC`,
    `READ_SYSTEM_STATE`; `cli`/`app` — declare `FILES` **and `REFLECT`** (it marshals
    the JSON report with `encoding/json` — review B6; let the self-check confirm the
    exact set).
- **Resolve the Step-2 `manifest`/reflection risk here.** If `prototext` triggers
  `CAPABILITY_REFLECT`, `manifest` is not strictly authority-free. Options, in order of
  preference: (a) scope the guaranteed-authority-free **showcase** to
  `facts`/`checker`/`report`/`capanalyzer` (design FR8: "at least the pure checking
  core"); (b) give `manifest` a manifest that declares/permits `REFLECT` via the
  policy seam; (c) later, a reflection-free textproto parse. Pick (a) for the MVP and
  document.
- Add a **self-hosting CI job** to `.github/workflows/ci.yml` (and a `just selfcheck`
  target, design §8): run `archcheck` on the core-component manifests; the pure core
  must verify ambient-authority-free, and every shell component must conform to its
  declared authority. Fail CI on any violation.
- **`.github/workflows/release.yml`** (deferred from Step 1 — review D15): on tag push
  (`v*`), build the `archcheck` binary (a small `go build` matrix for linux/darwin,
  amd64/arm64, or GoReleaser if preferred) and publish a GitHub Release with the
  artifacts. Keep it minimal.

**Tests.** The self-hosting run itself is the test: `archcheck` on
`checker.COMPONENT.textproto` (and `facts`, `report`, `capanalyzer`) → exit 0,
ambient-authority-free; on `goanalysis`/`capslockadapter`/`cli` → exit 0 with their
declared authority. Wire it as `just selfcheck` invoked from CI (and optionally a
`go test`).

**Integration.** The tool now verifies itself in CI; the MVP is feature-complete
against FR1–FR9 and NFR1–NFR4.

**Demo.** `archcheck check go/components/checker.COMPONENT.textproto` prints "conforms —
ambient-authority-free"; the CI self-hosting job is green; a deliberate edit that makes
the core touch the filesystem turns the core's own check red — the architectural
guarantee, enforced on the tool itself. `git tag v0.1.0 && push` (or
workflow_dispatch) produces a release with an `archcheck` binary that prints its
version.

---

## Coverage map (design → steps)

| Requirement | Step(s) |
|---|---|
| Capslock assumption validation (review B8) | 0 (spike) |
| FR1 manifest | 2 |
| FR2 CLI / exit codes / JSON | 3 (text render), 8 (wire + shell JSON) |
| FR3 dependency conformance (+ UNUSED_DEPENDENCY) | 4 |
| FR4 declared-interface closure (+ init rule) | 4 |
| FR5 cross-component boundary (+ higher-order warning) | 5 (rule), 9 (real edges) |
| FR5b capability pruning (+ init pruning) | 9 |
| FR6 authority-free + policy | 5 (rule), 7 (adapter), 8 (wire) |
| FR7 absorption | 7 (transitivity), 8 (example) |
| FR8 self-hosting | 10 |
| FR9 examples | 0 (draft), 8, 9 (refined) |
| NFR1 layout | 1 |
| NFR2 protobuf/textproto | 2 |
| NFR3 Bazel-ready lists | 2 (schema shape) |
| NFR4 Capslock behind port | 3 (port), 7 (adapter) |
| Dev tooling (justfile, CI) | 1 |
| Release workflow | 10 (deferred from 1 — review D15) |

## Open items to settle during implementation (design Appendix D)

Resolved by the design review: call-graph algorithm (**VTA**, matching Capslock —
A5); `report` JSON (shell-marshaled — B6); fact-type placement (`internal/facts`,
pure core — B7); release-workflow timing (Step 10 — D15).

1. **CLI name** — working name `archcheck` (confirm or replace) — affects Steps 1/8.
2. **Generated-proto delivery** — committed `gen/` (recommended) vs a gen step — Step 2.
3. **`manifest` reflection** vs authority-free claim — resolve in Step 10 (scope
   showcase to `facts`/`checker`/`report`/`capanalyzer`).
4. **Whole-graph vs single-component** check mode — MVP does single-component +
   resolve-direct-deps (Step 9); whole-graph is a post-MVP extension.
5. **Release tooling** — plain `go build` matrix vs GoReleaser for `release.yml` — Step 10.

Post-MVP research questions carried in design Appendix D: **callbacks as
capabilities** (review A3 — a func value crossing a component boundary is a
capability grant; MVP ships the `HIGHER_ORDER_BOUNDARY_CALL` warning as a stopgap)
and **robust generic-symbol matching** (review A4 — beyond bracket-stripping).
