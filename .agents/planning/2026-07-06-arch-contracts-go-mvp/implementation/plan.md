# Implementation Plan — Architectural Contracts MVP (Go)

Source design: [`../design/detailed-design.md`](../design/detailed-design.md).
Supporting artifacts: `../idea-honing.md`, `../research/capslock.md`,
`../research/go-component-model.md`, and the design review with resolutions:
[`../design-review.md`](../design-review.md). Updated for the PR #1 review
round (directory-based membership, FR4 well-formedness, whole-package authority
scope — an interface-rooted alternative was rejected, design Appendix A —
`declared_authority` wired in MVP, `Parse(io.Reader)`, FR10 informal
contracts, manifests at component roots, schema trims) and for a second PR #1
review round on the plan itself (Step 1 gains an `AGENTS.md`/`CLAUDE.md` seed;
release workflow and a full `README.md` split out of Step 10 into a new Step 11).

**Guiding principle.** Build each component in a test-driven manner, in increments
that each leave the tree in a working, demoable state. **De-risk first** (Step 0
spike — the design's load-bearing Capslock assumptions get executed, not just
read), then set up the project skeleton and CI so every later step lands on
green. Then sequence the **pure core** (it is a pure function of injected data,
so the bulk of the test suite needs no Go build and no Capslock — design §8),
then the authority-holding shell that feeds it real facts, then wire the CLI for
the first real end-to-end verdict, then add the call-graph-based
boundary/pruning checks, self-host, and finally document and release. No hanging
or orphaned code: every step ends by wiring its output into something runnable.

**Where core end-to-end appears.**
- **Step 6** — first *vertical* slice: real extracted facts flow through the pure
  core to produce a real (Pillar-1-only) verdict in a test harness.
- **Step 8** — first *full* end-to-end CLI: `arcc check component.textproto`
  on the CSV example produces a pass/fail conformance report **including the
  ambient-authority check**, with correct exit codes. This is the headline milestone.
- **Step 9** — the complete concept: cross-component boundary check (FR5) and
  capability pruning (FR5b), incl. the "compose a FILES component yet stay
  authority-free" showcase.
- **Step 10** — self-hosting: `arcc` verifies its own core as authority-free.
- **Step 11** — docs and release: a full README and a signed-binary release
  workflow (PR-review — kept separate from Step 10 so self-hosting stays scoped
  to FR8).

---

## Progress checklist

- [x] **Step 0 — Capslock validation spike** (draft `examples/csvtool` packages as probes; execute the design's Capslock assumptions: strict-safe stdlib envelope, `CAPABILITY_SAFE` pruning, key formats incl. method/generic/init, whole-package scope + `_test.go` exclusion, Reader attribution) — findings in [`../research/spike-capslock.md`](../research/spike-capslock.md); harness preserved under `../research/spike/`. **All 6 assumptions hold**; three design corrections surfaced: (1) use `excludeUnanalyzed=true`; (1b) **adopt a "minting, not use" classifier** — reclassify `(*os.File)` handle-use methods `SAFE` so ambient authority is attributed at the `os.Open` site, making `manifest.Parse(io.Reader)` authority-free by construction and **dropping the §11 `bytes.Reader` wrapping rule**; (2) drop the B8 `sort.Slice` prohibition.
- [x] **Step 1 — Project scaffold & developer tooling** (repo layout, `go/` module skeleton, `justfile`, CI workflow, `AGENTS.md`/`CLAUDE.md` — release workflow deferred to Step 11)
- [x] **Step 2 — Manifest schema & parser** (`proto/`, codegen, `internal/manifest`)
- [x] **Step 3 — Pure supporting types: `facts` data model, `capanalyzer` port/policy, `report` model & text rendering**
- [x] **Step 4 — Checker core I: dependency rule (FR3) + declared-interface & well-formedness (FR4)**
- [x] **Step 5 — Checker core II: call-boundary rule (FR5) + policy-aware authority rule (FR6)** — pure core complete
- [x] **Step 6 — `goanalysis` loader I: imports + exported-symbol→file extraction** (first vertical slice)
- [x] **Step 7 — `capslockadapter`: Capslock-backed `CapabilityAnalyzer`**
- [x] **Step 8 — CLI/app orchestration + CSV example** — first full end-to-end
- [x] **Step 9 — Call graph + FR5/FR5b: VTA edges, dependency-interface resolution, boundary pruning** — full concept
- [x] **Step 10 — Self-hosting (FR8): own component manifests + CI check of the core**
- [ ] **Step 11 — Full documentation & release workflow** (README.md; signed-binary release workflow)

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
  concrete impl, a generic function, pointer- and value-receiver methods, an
  authority-using exported helper that is *not* reachable from the interface
  plus an authority-using `_test.go` helper (scope probes), and a function
  taking an `io.Reader` fed both a `bytes.Reader` and an `*os.File`
  (attribution probe).
- Drive `analyzer.GetCapabilityInfo` (Capslock as a library) over the probes and
  validate:
  1. **Strict-safe stdlib envelope** — which stdlib entry points are usable by the
     examples and the pure core under `StrictPolicy`. **Spike result:** with
     `UNANALYZED` excluded (design §5.4a) the whole probed envelope is clean —
     `fmt`, `strconv`, `errors.*`, `io.ReadAll` (over any reader), and **both**
     `sort.Sort` and `sort.Slice` (Capslock rewrites `sort.*` call sites, so the
     old "`sort.Slice` is `unanalyzed` → fails strict" belief was wrong).
  2. **`CAPABILITY_SAFE` pruning end-to-end** — a custom `.cm` via
     `interesting.LoadClassifier(..., excludeBuiltin=false)` marking `csvfile`'s
     interface symbols (and `func <pkg>.init`) SAFE makes `app` come out
     capability-free; without it, `FILES` is attributed.
  3. **Key formats & normalization (review A4)** — confirm the SSA name forms
     actually emitted for: methods (both receiver forms), generic instantiations
     (bracket-stripping works), promoted-method wrappers, `init`/`init#N`.
  4. **Init attribution (review A2)** — an authority-using `func init()` in a
     dependency is attributed to the importer unless `func <pkg>.init` is pruned.
  5. **Whole-package scope + `_test.go` exclusion (design §5.4/Appendix A,
     PR-review)** — confirm an architecture-private exported helper (not in
     the interface, unreachable from it) that calls `os.Open` **is** reported
     (whole-package scope, the basis for rejecting the interface-rooted
     alternative), and that the same authority in a `_test.go` helper (both
     in-package and external `_test` package) is **not** — Capslock's
     `go/packages` load excludes test files from the analyzed build.
  6. **Reader attribution → minting-not-use (design §5.4a, supersedes §11).**
     Characterized both behaviors: under the raw classifier a component function
     taking an `io.Reader` *is* attributed `FILES` when an `*os.File` flows in
     (which is why §11 wrapped in `bytes.Reader`), but the spike concluded that
     attribution is **undesirable** — it makes a deprivileged consumer's authority
     depend on its callers. Resolution: reclassify the `(*os.File)` handle-use
     methods `SAFE`, attributing authority at the `os.Open` minting site, so
     `manifest.Parse` is authority-free by construction and the wrapping rule is
     dropped.
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
has bitten before); the release workflow is deferred to Step 11. Nothing to check
yet; the deliverable is a green, reproducible pipeline.

**Implementation guidance.**
- **Layout (NFR1, design §9).** Create the top-level skeleton: `proto/` (empty for
  now — schema lands in Step 2), `go/` as its **own module** (`go/go.mod`, working
  import root `github.com/ono-sendai-labs/architectural-contracts/go` — adjust to the real
  path, Q5), and placeholder dirs `go/internal/`, `go/cmd/arcc/`,
  `go/examples/`. There is **no** central `go/components/` dir — each component's
  `component.textproto` lives at its root (design §9, PR-review), authored in
  Step 10. The `docs/rationale-and-concepts.md` concept doc is already in place.
  Keep `proto/` **outside** `go/` so a future Rust toolchain can share it.
- **`AGENTS.md`** at repo root (symlinked from `CLAUDE.md`), kept short: project
  overview, that this repo follows the `structured-spec-to-code` skills framework
  (`.agents/` layout — planning/tasks/scratchpad), that the repo uses **jj**
  (Jujutsu) rather than git, and testing practice (`just ci` before every commit).
- **Buildable stub.** `cmd/arcc/main.go` that compiles and prints usage/version
  (e.g. `arcc check <manifest>` help text, `--version`) — no real logic. This
  gives CI something green to build and test. Note: `cli` orchestration code will
  live **under `cmd/arcc/`** (an `app/` subpackage), not `internal/app` —
  directory-based membership requires the component to be one subtree (design §9).
- **`justfile`** at repo root with the usual targets (all operating on the `go/`
  module — set the module dir or `cd go` inside recipes):
  - `build` — `go build ./...`
  - `test` — unit tests: `go test ./...`
  - `test-integration` — integration tests (the slower `goanalysis`/`capslockadapter`
    suites, separated by a build tag, e.g. `go test -tags=integration ./...`)
  - `lint` — `go vet ./...` + `gofmt -l`/`golangci-lint` if adopted
  - `fmt` — `gofmt -w` / `go fmt`
  - `gen` — protobuf codegen (a no-op placeholder now; wired to real codegen in Step 2)
  - `run` — `go run ./cmd/arcc`
  - `ci` — aggregate: `gen`-is-clean check + `lint` + `build` + `test` +
    `test-integration` (what the CI workflow calls)
  - `clean`
- **`.github/workflows/ci.yml`** — on push + pull_request: checkout, set up Go
  (version from `go/go.mod`), install `just` (and `protoc`/plugins if `gen` needs
  them), run `just ci` with `working-directory` at the repo root (recipes `cd` into
  `go/`). Cache the Go build/module cache. This is the gate every later step must keep
  green; Step 10 adds the self-hosting `selfcheck` job; Step 11 adds the release
  workflow.
- Bring the Step-0 draft `examples/csvtool` packages into the repo layout
  (`go/examples/csvtool`, as a nested module or `testdata`-style per design §9) so
  they build under CI from the start.

**Tests.** No product code to unit-test yet. The "test" is that `just build`,
`just test`, and `just test-integration` all succeed (trivially, on the stub) and the
CI workflow passes on a first push. Add a single smoke test (`arcc --version`
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
  them mechanically (NFR3). Per the PR review there is **no `packages` field**
  (membership = all packages under the manifest file's directory, FR1), **no
  `contract_note`** (the contract is doc-comment prose in interface files, FR10),
  and `AbsorbedDependency.reason` is `optional`. Per review C10 there is **no**
  `interface_packages` field — a dependency's interface packages are *derived*
  from its own manifest. Path semantics per C9/C12: `interface_files` are relative
  to the manifest file's directory (= component root);
  `ComponentDependency.manifest` is relative to the declaring manifest's
  directory (and its own directory is the dependency's component root).
- Wire the real **`gen`** justfile target to protobuf codegen. **Decision (recommend):
  commit the generated code** under `internal/manifest/gen/` (design §9 offers this or
  a gen step) so the build needs no `protoc`; the CI `gen`-is-clean check then just
  verifies the committed code matches the proto.
- `internal/manifest`: parse textproto content with
  `google.golang.org/protobuf/encoding/prototext` into the generated message, then map
  to a hand-written Go-native `Manifest` model (so the rest of the code doesn't depend
  on protobuf types), and **validate — syntactic checks only** (design §7, review C9):
  empty `name`, empty `interface_files`, duplicate declarations, and an unknown
  capability name in `declared_authority` (validated against the known capability
  set — review C11) → parse error. Checks that need the filesystem (interface file
  exists / belongs to a package under the component root) are the **shell's** job
  (Steps 6/9).
- `Parse` takes an **`io.Reader`**, not a path (design §4.1, PR-review): the Reader
  is an **object capability** handed in by the shell — a deliberate illustration of
  deprivileging the component. Internally `io.ReadAll` + `prototext.Unmarshal`.
  `Parse` is ambient-authority-free **by construction** and the shell may hand it
  any reader: the Step-0 spike settled the classifier to attribute filesystem
  authority at the `os.Open` *minting* site, not at capability *use* (design §5.4a),
  so reading a handed-in reader never counts against `manifest`. (The earlier
  "shell must pass a `bytes.Reader`" rule is **withdrawn**; `bytes.Reader` remains a
  convenient in-memory reader for tests.)

**Tests.** Table-driven unit tests for `Parse` (fed via `bytes.Reader`): a valid
manifest (e.g. the `toprow` example from §6.1) round-trips to the expected model;
one test per validation error (empty name, empty interface_files, duplicate,
unknown capability name like `"FILE"`) asserts the specific error; a failing
Reader surfaces as a parse error.

**Integration.** Consumed by Step 4's checker. The `gen` target and its CI clean-check
are now real, exercised by every subsequent CI run.

**Risk note (carry to Step 10).** `prototext.Unmarshal` uses reflection, so the
`manifest` component may itself trigger `CAPABILITY_REFLECT`. That does **not** affect
correctness now, but it means `manifest` may not qualify as ambient-authority-free
under `StrictPolicy`. Flag it; resolve in Step 10 (the guaranteed authority-free
showcase is `facts`/`checker`/`report`/`capanalyzer`; `report` stays clean because
JSON marshaling lives in the shell — review B6).

**Demo.** `just test` covers `internal/manifest`; a test prints the parsed model for
the example `toprow.component.textproto` and the specific error for a
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
  drives the FR4 well-formedness rule), `CallEdge` (`Caller`, `Callee`,
  `PassesFuncValue`), `DependencyInterface` (`Component`, `Packages` — derived
  from the dependency's root, not declared — and `Symbols`). Pure structs, no
  loaders — the authority-holding loaders live in `goanalysis` (Steps 6/9). This
  placement keeps every dependency pointing **inward** (never core→shell).
- `internal/capanalyzer` (design §4.1): `CapabilityFinding`, `Frame`, `Class`
  (`TrueAuthority | AnalysisDefeating`), `InterfaceSymbol` (with the A4
  normalization contract documented: generic brackets stripped, both receiver
  forms emitted), `AnalyzeRequest` (`Packages`, `PruneAt`; scope is every
  function in `Packages` — design §5.4), the `CapabilityAnalyzer` interface
  (the port — NFR4/Q1),
  `CapabilityPolicy` (`Allowed`, `Warn`), and `StrictPolicy()` returning the
  empty policy (MVP default). No Capslock import here — this is the pure port;
  the adapter arrives in Step 7.
- `internal/report` (design §6.2): `ConformanceReport`, `Finding`, `Kind`
  (violations `UNDECLARED_DEPENDENCY | CALLS_UNDECLARED_INTERFACE |
  UNDECLARED_AUTHORITY | METHOD_OUTSIDE_INTERFACE | INIT_OUTSIDE_INTERFACE |
  PACKAGE_OVERLAP`; warnings `ANALYSIS_LIMITATION | ALLOWED_WITH_WARNING |
  HIGHER_ORDER_BOUNDARY_CALL | UNUSED_DEPENDENCY`), `Location`, `Evidence`.
  Add pure rendering to **text**
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

## Step 4: Checker core I — dependency rule (FR3) + declared-interface & well-formedness (FR4)

**Objective.** Begin the heart of the tool: the pure `checker`, consuming the
`facts` model from Step 3. Implement the two Pillar-1 rules that need only
import/symbol facts: the dependency allowlist rule (FR3) and the
declared-interface computation + **well-formedness rules** (FR4, PR-review).

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
  component package's non-stdlib import that isn't part of the component itself
  (membership = the import paths present in `Facts.Packages`, i.e. the packages
  under the component root — FR1), if it's outside the allowed set →
  `UNDECLARED_DEPENDENCY` (with importing package + import path). **Stdlib imports
  are auto-allowed at Pillar 1** (§5.2) — governed by Pillar 3 instead. A declared
  dependency matching no import → `UNUSED_DEPENDENCY` **warning** (review C13).
- **FR4 declared interface + well-formedness (§5.3, PR-review):** the declared
  interface = exported symbols whose `File` ∈ `interface_files`. Well-formedness:
  (a) an exported method whose `Receiver` type is an exported non-interface type
  declared in an interface file, but whose own declaration is **not** in an
  interface file → `METHOD_OUTSIDE_INTERFACE` violation (a `types.go` + `api.go`
  split across interface files is clean); (b) an explicit `init` (Kind `init`)
  declared outside interface files → `INIT_OUTSIDE_INTERFACE` violation. Concrete
  implementations of interface-file **interface types** are exempt from (a) —
  they're contract-bound by the interface — but join the boundary/prune symbol
  set (computed in `goanalysis`, Step 9). Exported symbols not declared in
  interface files are *architecture-private*, **not** a violation of the
  component's own manifest; may emit an informational note listing them.

**Tests.** Table-driven with hand-built `Manifest` + `facts.PackageFacts` (design
§8): conforming imports → no findings; an undeclared non-stdlib import → exactly one
`UNDECLARED_DEPENDENCY`; a stdlib import → no finding; an unused declared dep →
`UNUSED_DEPENDENCY` warning; a method of an interface-file type declared in a
non-interface file → `METHOD_OUTSIDE_INTERFACE`; the same method declared in a
*second* interface file (`types.go`/`api.go` split) → clean; an exported symbol
not in interface files → informational note, not a violation; explicit init
outside interface files → `INIT_OUTSIDE_INTERFACE`.

**Integration.** `Check` now returns a real (partial) `report.ConformanceReport` built
from `manifest` + `capanalyzer` + the fact types — components wired.

**Demo.** `just test` covers `internal/checker`; a test feeds a hand-built component
with one undeclared import and prints the rendered report showing the
`UNDECLARED_DEPENDENCY` violation.

---

## Step 5: Checker core II — call-boundary rule (FR5) + policy-aware authority rule (FR6)

**Objective.** Complete the pure `checker`: add the cross-component call-boundary rule and
the ambient-authority rule, both operating on injected data. After this step
`checker.Check` is a **complete, fully unit-tested pure function** — the self-hosting
showcase in miniature, with no Go build or Capslock in the loop.

**Implementation guidance.**
- **FR5 call-boundary rule (§5.3b):** given `Facts.CallEdges` + resolved `DepIfaces`, for
  each call edge whose callee belongs to a component dependency `B`'s packages, if the
  callee ∉ `B`'s declared-interface symbol set (per §5.3: interface-file decls +
  interface-type implementation methods; matching uses the A4 normalization —
  strip generic brackets, accept both receiver forms) →
  `CALLS_UNDECLARED_INTERFACE` (caller, callee, `B`). An edge **into** a declared
  interface symbol with `PassesFuncValue` → `HIGHER_ORDER_BOUNDARY_CALL` warning
  (review A3). Overlapping membership between the component and a resolved
  dependency (with directory-based membership: one component root nested inside
  the other, surfaced in the resolved package sets) → `PACKAGE_OVERLAP` violation
  (§5.5, review C12/PR-review). MVP scope is intentionally call-only: type use,
  field access, exported vars, and other non-call channels are post-MVP. (Real call
  edges arrive in Step 9; here they are hand-built.)
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
- `internal/goanalysis`: implement `LoadPackageFacts(componentRoot) →
  facts.PackageFacts` (core-defined types, review B7) — `componentRoot` is the
  manifest file's directory; membership = **all Go packages under it** (FR1,
  PR-review; load `./...` from the root) — using
  `golang.org/x/tools/go/packages` with a load mode covering types/syntax/imports/deps/
  module (the same load settings Capslock needs; sharing an already-loaded package
  graph is not required by the MVP port). Extract per package: direct imports, `IsStdlib` (via
  module/std detection), exported top-level decls via `go/ast` (`ast.IsExported`)
  mapped to the declaring file — including **`Receiver`** linkage for methods (drives
  the FR4 well-formedness rule) and explicit **`init`** decls (Kind `init`, review
  A2) — in the Capslock/go-types **key form** (`InterfaceSymbol`, both receiver
  forms per A4). Leave `CallEdges` empty for now (Step 9 fills them). This
  component declares **FILES, EXEC, READ_SYSTEM_STATE**.
- Implement the **resolved manifest validation** that pure `Parse` can't do (design
  §7, review C9): each `interface_files` entry exists on disk and belongs to a
  package under the component root → else tool error (exit 2).
- Package-load errors surface verbatim and will abort with exit 2 in the shell (design
  §7) — return them as errors here.

**Tests.** Integration tests (the `just test-integration` suite) against small fixture
packages under `internal/goanalysis/testdata/` (design §8): assert extracted imports
and the exported-symbol→file mapping for a known fixture; assert `IsStdlib`
classification; assert `Receiver` linkage and `init` extraction; fixtures cover the
design-§8 well-formedness corner cases (type in one interface file / methods in a
non-interface file, `types.go`/`api.go` split, interface type + concrete impl,
generic func, both receiver forms, promoted method, explicit init).

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
- **Classifier configuration (Step-0 spike, design §5.4a — load-bearing).** Build the
  per-run classifier as: Capslock builtins **with `UNANALYZED` excluded**
  (`analyzer.GetClassifier(true)` / `interesting.ClassifierExcludingUnanalyzed`) —
  else `io.ReadAll`, `errors.Is`, `bufio` reads etc. flood every component with
  spurious findings and mask real flows — **merged with a "minting, not use"
  override** that marks the `(*os.File)` handle *use* methods
  (`.Read`/`.Write`/`.Close`/`.Seek`/`.Stat`/…, **excluding `.Chdir`**)
  `CAPABILITY_SAFE`, so filesystem authority attributes at the
  `os.Open`/`os.ReadFile` minting site, not at every consumer of a handle. In
  Step 9 this same merged classifier also carries the FR5b prune map (all three
  combine into one `interesting.LoadClassifier(..., excludeBuiltin=false)` result).
  Carry the exact handle-method list from the spike harness
  (`../research/spike/harness.go`, `fileHandleUseMethods`). Curate the symmetric
  network/exec/env use-methods only when those capabilities appear; document the
  stdio-globals loosening (§5.4a/§11). This classifier configuration is adapter-side
  analysis setup; it is separate from the checker-side `CapabilityPolicy`
  allow/warn decision over findings.
- Honor `AnalyzeRequest.Packages`; accept `PruneAt` in the signature but treat empty as
  "no pruning" (full Capslock transitivity — which already gives **absorption** of
  absorbed deps for free, FR7). The `CAPABILITY_SAFE` custom-map pruning is Step 9.
  Analysis scope is Capslock's native whole-package behavior (design §5.4): every
  function in `Packages`, with `_test.go` files excluded by the load itself — no
  entry-point filtering in the adapter.
- **Pin Capslock** in `go/go.mod` (pre-1.0 API — review D16); upgrades are deliberate,
  reviewed events. The call-graph algorithm question is settled (review A5): Capslock
  uses **VTA** internally, and Step 9's `goanalysis` graph uses `vta.CallGraph` to
  match.
- Sanity-check the adapter's behavior against the Step-0 spike findings (same probe
  packages, now as committed fixtures where useful).

**Tests.** Integration tests (`just test-integration`, design §8) against fixture
packages with known capabilities: a package that reads a file → expect a `FILES`
finding with a plausible call path; a pure-arithmetic package → expect **no** findings.
Guards the Capslock-proto → `CapabilityFinding`/`Class` mapping. **Scope tests**
(design §8/§5.4, PR-review): a fixture whose authority-using code sits in an
architecture-private exported helper (unreachable from the declared interface) →
**reported** (whole-package scope); the same authority in a `_test.go` helper →
**not reported** (test files outside the analyzed build).

**Integration.** The adapter satisfies the `capanalyzer.CapabilityAnalyzer` port, so it
is now injectable wherever the port is expected (the CLI in Step 8).

**Demo.** `just test-integration` covers `internal/capslockadapter`; a test runs the
adapter on a file-reading fixture and prints the `FILES` finding with its call path,
and on a pure fixture prints "ambient-authority-free (no findings)."

---

## Step 8: CLI/app orchestration + CSV example — first full end-to-end

**Objective.** Wire everything into the `arcc` CLI and prove it on a real example
project: read a manifest, load facts, run Capslock, check, render, exit correctly —
**the first full end-to-end including the ambient-authority check** (FR2, FR9, FR7).

**Implementation guidance.**
- `cmd/arcc` + its `app` orchestration subpackage (one subtree — the `cli`
  component; replace the Step-1 stub): parse args
  (`arcc check path/to/component.textproto`, `--format=json`), read the
  manifest file (FILES) and hand `manifest.Parse` an `io.Reader` (a `bytes.Reader`
  is fine but **not required**; the Reader-as-object-capability seam. With the
  §5.4a minting-not-use classifier `manifest` stays authority-free even if an
  `*os.File` flows in, since authority attributes at the shell's `os.Open` — design §5.4a),
  take the **component root := the manifest's directory**,
  `goanalysis.LoadPackageFacts(root)`, `capslockadapter.Analyze` (whole-package
  scope; empty `PruneAt` for now), `checker.Check` with
  `StrictPolicy()` merged with `declared_authority`, render via `report` (text), and
  map to **exit codes** (design §7): `0` conforms (warnings alone stay 0 — C14),
  `1` violations, `2` tool error (manifest parse, package load, Capslock failure,
  interface file missing). Tool errors go to stderr and never masquerade as a pass.
  `--format=json` marshals the `report.ConformanceReport` struct with
  `encoding/json` **here in the shell** (review B6 — kept out of the pure core;
  `cli`'s own manifest will declare `REFLECT`, Step 10).
- `go/examples/csvtool` — **refine the Step-0 draft packages** (design §10) for the
  parts that do **not** require the call graph yet. Each component's
  `component.textproto` sits **at its own root** (design §9, PR-review), and each
  component's interface files carry its **informal contract** as doc comments
  (FR10: does / requires / provides, incl. authority):
  - `toprow` — pure logic (sort/pick already-parsed rows), imports only the absorbed
    `internal/parsecsv` helper + strict-safe stdlib per the Step-0 envelope
    (either `sort.Sort` or `sort.Slice` is fine — the B8 `sort.Slice` prohibition
    was **retired** by the Step-0 spike; Capslock rewrites `sort.*` call sites);
    manifest declares **no authority** and an `absorbed_dependency` on `parsecsv`.
  - `internal/parsecsv` — absorbed impl-detail dep (wraps `encoding/csv`; no manifest).
  - `csvfile` — a component that legitimately `declared_authority: "FILES"` and exposes
    `Read(path) ([][]string, err)`; checked on its own it **conforms** (FILES is
    declared → in `Allowed`). Its contract prose says it reads the named file from
    the real filesystem — the doc-comment counterpart of `FILES`.
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

**Demo.** From the shell: `arcc check examples/csvtool/toprow/component.textproto`
prints a green "conforms — ambient-authority-free" report (exit 0); running it on the
undeclared-dependency and direct-`os.Open` variants prints actionable violations (exit
1); `--format=json` emits the machine-readable report.

---

## Step 9: Call graph + FR5/FR5b — VTA edges, dependency-interface resolution, boundary pruning

**Objective.** Add the remaining, most sophisticated piece: the VTA call graph that
powers the cross-component **call-boundary check (FR5)** and **capability pruning
(FR5b)**, making the *component-dep vs absorbed-dep* distinction mechanically
precise. Complete the CSV example with the multi-component `app` that **composes a
FILES-holding component yet checks ambient-authority-free** — the pruning showcase.

**Implementation guidance.**
- `goanalysis`: build the call graph with **`vta.CallGraph`** — matching Capslock's
  internal algorithm exactly, so the two pillars agree on which edges exist (review
  A5; Capslock's own graph is unexported, so the double build is an accepted MVP
  cost). Populate `CallEdges` (caller→callee in `InterfaceSymbol` key form,
  normalized per A4 — strip generic type-argument brackets), including
  `PassesFuncValue` on call sites passing function-typed values (review A3). The
  MVP intentionally enforces call edges only; non-call cross-component uses are
  documented limitations, not Step-9 work.
  Implement `ResolveDependencyInterface(declaringRoot, analyzedRoot, dep)` — resolve
  `dep.manifest` relative to the declaring component root, load the dependency's
  manifest (its directory = the dependency's **component root**; enumerate
  `DependencyInterface.Packages` from that subtree) + its interface files → that
  dependency's declared-interface **symbol set** per design §5.3 (interface-file
  decls, augmented via `go/types` with the concrete in-component implementation
  methods of interface-file interface types; both receiver key forms). A
  dependency manifest that fails to resolve/load, or whose `name` mismatches the
  declaration (C12), → tool error (exit 2); a dependency root nested with the
  component's root surfaces as `PACKAGE_OVERLAP` (§5.5).
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
  fixture — including the symbol-set corner cases (interface type + concrete
  impl methods in the set; generic symbol normalization; init keys in the prune
  set; `types.go`/`api.go` split).
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

**Demo.** `arcc check examples/csvtool/app/component.textproto` → **conforms,
ambient-authority-free**, even though `app` orchestrates a file-reading component; the
non-interface-call variant → `CALLS_UNDECLARED_INTERFACE`; side-by-side with the absorb
variant from Step 8, the report makes the prune-vs-absorb distinction visible.

---

## Step 10: Self-hosting (FR8) — own component manifests + CI check of the core

**Objective.** Close the loop: decompose the tool itself into manifested components and
run `arcc` on its own core, proving the pure core is genuinely
ambient-authority-free (FR8) — the project's thesis, dogfooded, enforced in CI.

**Implementation guidance.**
- Author a `component.textproto` **at each component's root** (design §4/§9,
  PR-review — the manifest's directory defines the component; no central
  `go/components/` dir): `go/internal/checker/component.textproto`,
  `go/internal/facts/component.textproto`, …, `go/cmd/arcc/component.textproto`:
  - **Authority-free core:** `facts`, `checker`, `report`, `capanalyzer` — declare no
    authority; component dependencies wired per the real import graph (e.g. `checker`
    depends on `manifest`/`facts`/`report`/`capanalyzer` — all pure, all inward;
    review B7).
  - **Shell:** `goanalysis`, `capslockadapter` — declare `FILES`, `EXEC`,
    `READ_SYSTEM_STATE`; `cli` (= the `cmd/arcc` subtree incl. its `app`
    subpackage) — declare `FILES` **and `REFLECT`** (it marshals
    the JSON report with `encoding/json` — review B6; let the self-check confirm the
    exact set).
- **Contract pass (FR10).** Ensure every component's interface files carry its
  informal contract as doc comments (does / requires / provides, incl. the
  authority it holds) — written along the way in Steps 2–9, reviewed for
  completeness here.
- **Resolve the Step-2 `manifest`/reflection risk here.** If `prototext` triggers
  `CAPABILITY_REFLECT`, `manifest` is not strictly authority-free. Options, in order of
  preference: (a) scope the guaranteed-authority-free **showcase** to
  `facts`/`checker`/`report`/`capanalyzer` (design FR8: "at least the pure checking
  core"); (b) give `manifest` a manifest that declares/permits `REFLECT` via the
  policy seam; (c) later, a reflection-free textproto parse. Pick (a) for the MVP and
  document.
- Add a **self-hosting CI job** to `.github/workflows/ci.yml` (and a `just selfcheck`
  target, design §8): run `arcc` on the core-component manifests; the pure core
  must verify ambient-authority-free, and every shell component must conform to its
  declared authority. Fail CI on any violation.

**Tests.** The self-hosting run itself is the test: `arcc` on
`internal/checker/component.textproto` (and `facts`, `report`, `capanalyzer`) →
exit 0, ambient-authority-free; on `goanalysis`/`capslockadapter`/`cli` → exit 0
with their declared authority. Wire it as `just selfcheck` invoked from CI (and
optionally a `go test`).

**Integration.** The tool now verifies itself in CI; the MVP's core mechanics are
feature-complete against FR1–FR10 and NFR1–NFR4.

**Demo.** `arcc check go/internal/checker/component.textproto` prints "conforms —
ambient-authority-free"; the CI self-hosting job is green; a deliberate edit that makes
the core touch the filesystem turns the core's own check red — the architectural
guarantee, enforced on the tool itself.

---

## Step 11: Full documentation & release workflow

**Objective.** Wrap up the MVP with the artifacts a real user (not just the CI
pipeline) needs: a full `README.md` and a release workflow that publishes signed
binaries. Kept as its own step (PR-review) rather than folded into Step 10 so that
self-hosting stays focused on FR8 and this step stays a clean, demoable
"ready to hand to someone else" milestone.

**Implementation guidance.**
- **`README.md`** at repo root: project rationale (what architectural contracts are
  and why — pointing into `docs/rationale-and-concepts.md` for the full concept
  doc), usage (`arcc check path/to/component.textproto`, `--format=json`, exit
  codes), a worked example (the `examples/csvtool` walk-through: `toprow` passing,
  `absorbapp` failing on undeclared authority, `app` passing while composing a
  `FILES`-holding dependency), how to write a `component.textproto` for your own
  code, and known **limitations** (design §11 — dynamic-dispatch over-approximation,
  the higher-order-boundary-call warning, single-component check mode, Go-only).
- **`.github/workflows/release.yml`** (pattern per the sibling `litebox` project's
  `.github/workflows/release.yml` — PR-review): trigger on tag push matching both
  final and pre-release forms, `tags: ['v[0-9]+.[0-9]+.[0-9]+-?*']` (matches
  `v1.0.0` **and** `v1.0.0-rc.1`/`v1.0.0-alpha.1`). A plain **`go build` matrix**
  (linux/darwin × amd64/arm64) produces the `arcc` binaries. A **tag with a `-`
  in it is a pre-release**: detect it with `[[ "$GITHUB_REF_NAME" == *-* ]]` and
  branch two ways —
  - **Final tag (no `-`):** run the **SLSA provenance attestation** step
    (`actions/attest-build-provenance@v2`, which emits an in-toto SLSA v1
    provenance predicate and signs it via Sigstore) over each built artifact, so a
    consumer can `gh attestation verify` that the binary was built by *this*
    workflow from *this* source commit — the supply-chain counterpart of the
    tool's own authority claims. Guard the step with
    `if: ${{ !contains(github.ref_name, '-') }}`.
  - **Pre-release tag (has a `-`):** **skip attestation** — `attest-build-provenance`
    isn't supported for private repos, and this project needs to cut `-rc`/`-alpha`
    releases while the repo is still private, ahead of going public — and pass
    **`--prerelease`** to `gh release create` so it's flagged correctly on GitHub.
  Publish the binaries (and, for final tags, their attestations) to a GitHub
  Release via `gh release create "$GITHUB_REF_NAME" <artifacts> --generate-notes
  $EXTRA_FLAGS`, `EXTRA_FLAGS="--prerelease"` iff the tag contains `-`. Keep it
  minimal — no GoReleaser. Job needs `permissions: { contents: write, id-token:
  write, attestations: write }` (the last two are no-ops, harmlessly, on
  pre-release runs that skip attestation).

**Tests.** No new product code. Verify the README's worked-example commands
actually produce the output shown (run them against the real `examples/csvtool`
components) so the doc doesn't drift from behavior on day one.

**Integration.** Closes out the MVP: the tool is now buildable, self-verifying,
documented, and releasable.

**Demo.** A fresh clone with only the README as guidance can build `arcc`, run it
against `examples/csvtool`, and understand the pass/fail output. `git tag
v0.1.0-rc.1 && push` produces a **pre-release** GitHub Release (no attestation,
usable while the repo is private); `git tag v0.1.0 && push` (or
`workflow_dispatch`) produces a full Release with `arcc` binaries and their SLSA
attestations.

---

## Coverage map (design → steps)

| Requirement | Step(s) |
|---|---|
| Capslock assumption validation (review B8, incl. whole-package scope, `_test.go` exclusion + Reader attribution) | 0 (spike) |
| FR1 manifest (directory-based membership, manifest at component root) | 2 (schema/parse), 6 (root→packages resolution) |
| FR2 CLI / exit codes / JSON | 3 (text render), 8 (wire + shell JSON) |
| FR3 dependency conformance (+ UNUSED_DEPENDENCY) | 4 |
| FR4 declared interface + well-formedness (method/init rules) | 4 |
| FR5 cross-component call-boundary (+ higher-order warning) | 5 (rule), 9 (real edges) |
| FR5b capability pruning (+ init pruning) | 9 |
| FR6 authority + policy (declared_authority → Allowed; whole-package scope) | 5 (rule), 7 (adapter), 8 (wire) |
| FR7 absorption | 7 (transitivity), 8 (example) |
| FR8 self-hosting | 10 |
| FR9 examples | 0 (draft), 8, 9 (refined) |
| FR10 informal contracts in interface files | 8 (examples), 10 (own components, contract pass) |
| NFR1 layout | 1 |
| NFR2 protobuf/textproto | 2 |
| NFR3 Bazel-ready lists | 2 (schema shape) |
| NFR4 Capslock behind port | 3 (port), 7 (adapter) |
| Dev tooling (justfile, CI) | 1 |
| User docs (README) | 11 (PR-review) |
| Release workflow | 11 (deferred from 1 — review D15; split from Step 10 — PR-review) |

## Open items to settle during implementation (design Appendix D)

Resolved by the design review: call-graph algorithm (**VTA**, matching Capslock —
A5); `report` JSON (shell-marshaled — B6); fact-type placement (`internal/facts`,
pure core — B7); release-workflow timing (Step 11 — D15).

1. **CLI name** — *settled*: **`arcc`** (**AR**chitectural **C**omponent **C**ontracts;
   a verb-free noun so future non-check subcommands fit) — Steps 1/8/10.
2. **Generated-proto delivery** — committed `gen/` (recommended) vs a gen step — Step 2.
3. **`manifest` reflection** vs authority-free claim — resolve in Step 10 (scope
   showcase to `facts`/`checker`/`report`/`capanalyzer`).
4. **Whole-graph vs single-component** check mode — MVP does single-component +
   resolve-direct-deps (Step 9); whole-graph is a post-MVP extension.
5. **Release tooling** — *settled*: plain `go build` matrix (linux/darwin × amd64/arm64)
   + `actions/attest-build-provenance` SLSA attestations on final tags only, no
   GoReleaser — Step 11. Pre-release tags (`v*-rc.*`/`v*-alpha.*`, any tag
   containing `-`) skip attestation (unsupported on private repos) and pass
   `--prerelease` to `gh release create`, so `-rc`/`-alpha` releases can ship
   before the repo goes public — pattern per `litebox`'s `release.yml`.

Post-MVP research questions carried in design Appendix D: **callbacks as
capabilities** (review A3 — a func value crossing a component boundary is a
capability grant; MVP ships the `HIGHER_ORDER_BOUNDARY_CALL` warning as a stopgap)
and **robust generic-symbol matching** (review A4 — beyond bracket-stripping).
