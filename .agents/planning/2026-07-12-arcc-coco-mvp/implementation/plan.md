# Implementation Plan — arcc MVP, coco-first variant

Source design: `../design/detailed-design.md`. Greenfield on this jj
workspace branch (`go/` currently holds only `go.mod`). Components are built
strictly **after the dependencies whose contracts they rely on** (design §4
graph), and every component step follows the same **contract-first inner
loop**:

1. write `component-contract.md` (Tier 2 skeleton: purpose, clause list,
   rely-set, authority rationale) and the Tier-1 clause comments alongside the
   interface stubs;
2. write the clause-derived tests (red);
3. implement to green;
4. fill the clause→test map; the step is not done while a Tier-1 clause has no
   map row (or `REVIEW:` marker);
5. author the component's `component.textproto` (incl. `contract_files`).

The coco-contract-testing skill governs the test style in each step; the
coco-contract-review skill is the reviewer for every step's contract deltas.

## Progress checklist

- [ ] Step 1: Scaffold, conventions doc, manifest proto
- [ ] Step 2: `facts` — symbol vocabulary component
- [ ] Step 3: `manifest` — parse + validity rules
- [ ] Step 4: `capanalyzer` (+ `capfake`) and `report`
- [ ] Step 5: `checker` I — structural rules (R1, R2, R9, R10)
- [ ] Step 6: `checker` II — well-formedness, boundary, authority (R3–R8)
- [ ] Step 7: `cli`/`app` — hermetic end-to-end orchestration
- [ ] Step 8: `goanalysis` I — packages, imports, symbols, contract-file facts
- [ ] Step 9: `goanalysis` II — VTA call edges + dependency resolution
- [ ] Step 10: `capslockadapter` — real analyzer + fake-fidelity certification
- [ ] Step 11: CSV example with contracts + failing variants
- [ ] Step 12: Self-hosting + CI + clause-coverage sweep

---

## Step 1: Scaffold, conventions doc, manifest proto

**Objective.** A buildable repo skeleton plus the two artifacts every later
step consumes: the project contract convention and the manifest schema.

**Guidance.**
- Repo layout per design §11; keep `go/` as the module that already exists.
- Write `docs/coco-conventions.md` implementing design §3 verbatim (clause
  labels, Tier-1 comment syntax, Tier-2 section order, algebra labeling,
  evidence rule). This is a *normative* doc — later steps cite it.
- Add `proto/archcontracts/v1/component.proto` per design §8.1 (basis proto +
  `contract_files` field 6, comments updated). Generate Go code into
  `go/internal/manifest/gen/` and **commit the generated code** (no
  build-time protoc dependency); record the regeneration command in a
  `justfile`.
- Minimal CI (build + test) so every later step lands green.

**Tests.** `go build ./...` and an empty `go test ./...` pass in CI; a
round-trip test unmarshals the design's example textproto through the
generated types (proves schema + generation wiring).

**Demo.** CI green on the skeleton; `docs/coco-conventions.md` readable; the
example `toprow` manifest from the design parses via a throwaway test.

## Step 2: `facts` — symbol vocabulary component

**Objective.** The root of the dependency tree: `InterfaceSymbol`, its normal
form as a *named clause*, constructors, and the fact model.

**Guidance.** Design §5.1. Implement `FuncKey`, `MethodKeys`, `Normalize`,
and the model types (incl. `MissingContractFiles`). The Tier-2 elaboration of
`INV:symbol-key-form` embeds the spike-verified key examples as the normative
list.

**Tests.** Property tests: `POST:normalize-idempotent`,
`POST:method-both-receivers`; fixture table of spike key forms (generics,
init, both receivers) through `Normalize`.

**Integration.** First real component: its `component.textproto` +
`component-contract.md` land here and become the pattern every later
component copies.

**Demo.** Tests green; `internal/facts/` shows the full coco shape (manifest
designating the contract file, Tier-1 clauses in `facts.go`, clause→test map
filled).

## Step 3: `manifest` — parse + validity rules

**Objective.** `manifest.Parse(io.Reader)` with validity rules V1–V7.

**Guidance.** Design §5.3. Uses the Step-1 generated code as an absorbed
dependency. Declare `REFLECT` in its own manifest with the Tier-2 rationale.
The `INV:parse-authority-free` clause gets a `REVIEW:` map row now
(mechanical evidence arrives with self-hosting, Step 12).

**Tests.** Table per V-rule, accept + reject directions; `POST:parse-err`
zero-value assertion; round-trip of all design example manifests.

**Integration.** Consumes Step-1 proto; `facts` untouched (no dependency).

**Demo.** A `go test` table shows each malformed manifest rejected with the
rule named in the error; valid manifests yield populated models
(including `contract_files`).

## Step 4: `capanalyzer` (+ `capfake`) and `report`

**Objective.** Complete the pure vocabulary layer: the analyzer port with its
implementer-binding contract, the in-memory fake, and the report component.

**Guidance.** Design §5.2, §5.4, ADR-5/6. Three threads:
1. Port + policy types with the five port clauses as Tier-1 comments.
2. `capfake` (subpackage of `capanalyzer` — same component): declarative
   capability table in, findings out, honoring `POST:findings-pruned` and
   `POST:findings-deterministic` semantics itself. Author the **shared
   contract-suite scenarios in their table form** now (scope, pruning at
   symbols + dep inits, minting-vs-use, determinism, error) — the fixture
   form is deferred to Step 10 where the real adapter runs the *same*
   scenarios.
3. `report`: model + `RenderText` (`POST:render-complete`,
   `POST:render-deterministic`).

**Tests.** Fake passes every table-form scenario; renderer golden tests +
same-input/same-output determinism check.

**Integration.** `capanalyzer → facts` edge (ADR-2) exercised for the first
time.

**Demo.** A test constructs findings via the fake from a capability table,
feeds a hand-built report, and prints the rendered text — the output format
of the final tool, visible before any real analysis exists.

## Step 5: `checker` I — structural rules (R1, R2, R9, R10)

**Objective.** `checker.Check` exists end-to-end (Inputs → report) with the
rules decidable from manifest + facts alone.

**Guidance.** Design §5.5, §6. Land the full `Inputs` struct and the four PRE
clauses *now* (they are contract, not code); implement R1
`UNDECLARED_DEPENDENCY` (incl. interface-package derivation and stdlib
auto-allow), R2 `UNUSED_DEPENDENCY`, R9 `PACKAGE_OVERLAP`, R10
`CONTRACT_FILE_MISSING`.

**Tests.** Table-driven per rule with golden reports; determinism check
(`POST:check-deterministic`); a documentation test showing PRE-violating
garbage produces garbage (documented, not promised).

**Integration.** First consumer of all four pure components — the design's
diamond converging on `facts` is now compiled reality.

**Demo.** Hand-built `Inputs` for a two-component scenario produce a golden
report showing an undeclared import, an unused dep warning, and a missing
contract file — rendered via Step 4's `RenderText`.

## Step 6: `checker` II — well-formedness, boundary, authority (R3–R8)

**Objective.** The complete rule catalog.

**Guidance.** Design §6 semantics carried from go-mvp: R3/R4 well-formedness
(method rule with `types.go`+`api.go` split allowance, interface-type
exemption, explicit-init rule), R5 boundary (`CALLS_UNDECLARED_INTERFACE`
over `CallEdges` × `DepIfaces`), R6 higher-order warning, R7/R8 policy
evaluation (`declared_authority` → `Allowed`; strict default; `Class`
retained on findings).

**Tests.** Per-rule tables incl. the corner cases the design names: promoted
methods, both receiver forms, bracket-stripped generics matching, findings
already-pruned semantics (R7 sees only post-prune findings), warnings never
in `Violations`.

**Integration.** Completes `POST:report-complete` — the clause→test map for
`checker` should now cover R1–R10.

**Demo.** The full go-mvp §10 failing-variant scenarios expressed as
hand-built `Inputs`, each producing its designed finding kind.

## Step 7: `cli`/`app` — hermetic end-to-end orchestration

**Objective.** A runnable `arcc check` whose orchestration is contract-driven
and fully testable without touching the real world.

**Guidance.** Design §5.8. `app.Run` takes the consumer-defined
`FactsLoader` + `DepResolver` ports and a `capanalyzer.CapabilityAnalyzer`.
Implement manifest reading (the one FILES touch), exit-code mapping, text +
JSON output. The Tier-2 **PRE-discharge table** is written here and each row
gets a dedicated wiring test.

**Tests.** Hermetic app tests with `capfake` + stub loaders: correct
`PruneAt` set passed to the analyzer (the "already pruned" bug class —
`PRE:caps-pruned` discharge); resolution failure → exit 2; violations → 1;
warnings-only → 0; JSON golden.

**Integration.** Everything pure is now wired; the two shell fact-producers
are stubs behind ports.

**Demo.** `go run ./cmd/arcc check` against a synthetic world (test fixture
wiring the stubs) prints a real report with real exit codes — the tool's UX
is complete before any static analysis exists.

## Step 8: `goanalysis` I — packages, imports, symbols, contract-file facts

**Objective.** Real facts from real Go code: everything except call edges and
dependency resolution.

**Guidance.** Design §5.6. `go/packages` load under a component root;
imports; exported symbols → declaring files (keys built **only** via `facts`
constructors — `POST:facts-keys-normal`); explicit-init detection;
`MissingContractFiles` from the manifest.

**Tests.** Fixture packages under `testdata/`: the design's §9.5 list minus
call-graph cases — method-outside-interface, `types.go`+`api.go` split,
interface type + concrete impl, generics, pointer/value receivers, promoted
methods, explicit `init`, missing/present contract files; `POST:facts-err` on
a broken build.

**Integration.** Swap the Step-7 stub `FactsLoader` for the real one in the
CLI wiring (analyzer still `capfake` with an empty table ⇒ authority checks
vacuously pass — flagged in `--help`/docs as not yet real).

**Demo.** `arcc check` on a real on-disk fixture project reports R1–R4 and
R10 violations from actual source code.

## Step 9: `goanalysis` II — VTA call edges + dependency resolution

**Objective.** The remaining world facts: cross-component call edges and
resolved dependency interfaces.

**Guidance.** Design §5.6: SSA+VTA edges with `PassesFuncValue`;
`ResolveDependencyInterface` (`POST:depiface-fr4` — interface-file decls +
interface-type concrete impls + per-package init keys;
`POST:depiface-integrity` — load, name match, root disjointness).

**Tests.** Fixtures with two mini-components: boundary call onto a declared
vs undeclared symbol; func-value crossing (R6 flag); FR4 symbol set golden
(incl. impl methods + init keys); nested-root and name-mismatch failures.

**Integration.** Real `DepResolver` replaces the stub; R5/R6/R9 now fire end
to end.

**Demo.** `arcc check` catches a fixture component calling its dependency's
architecture-private exported function.

## Step 10: `capslockadapter` — real analyzer + fake-fidelity certification

**Objective.** Real ambient-authority findings, and the fake certified as a
behavioral subtype.

**Guidance.** Design §5.7. Merged per-run classifier: builtin map −
`UNANALYZED` + `(*os.File)` handle-use SAFE + `PruneAt` SAFE entries +
`func <pkg>.init` per dep package. Pin Capslock; copy the spike checklist
into the Tier-2 external-rely section as the version-bump re-validation list.

**Tests.** The **fixture form of the Step-4 shared contract suite**: each
scenario as a real `testdata` package tree run through the adapter, asserting
the same findings the fake produced from the table form (scope incl.
dead/unexported code; pruning; minting-vs-use incl. the
`Parse(io.Reader)`-style deprivileged-consumer case; determinism).

**Integration.** Real analyzer replaces `capfake` in the production wiring;
`capfake` remains the test-time implementation everywhere else.

**Demo.** `arcc check` on a fixture that calls `os.Open` fails with
`UNDECLARED_AUTHORITY` + Capslock call-path evidence; the same fixture with
`declared_authority: "FILES"` passes. Fake-vs-real suite green = fidelity
certified.

## Step 11: CSV example with contracts + failing variants

**Objective.** The pedagogical showcase (FR9), fully coco.

**Guidance.** Design §12: `toprow` (ambient-authority-free),
`internal/parsecsv` (absorbed), `csvfile` (declares FILES), `app` (pruning
showcase — composes csvfile yet authority-free). Each example component gets
Tier-1 clauses, `component-contract.md` with its miniature rely-set, and a
manifest designating it.

**Tests.** E2E golden tests: conforming run of all four manifests; failing
variants — absorb-instead-of-depend (R7), undeclared-interface call (R5),
undeclared import (R1), missing contract file (R10) — each asserting exit
code + rendered finding.

**Integration.** First multi-component consumer of the finished tool;
exercises pruning across a real component boundary.

**Demo.** The complete story, runnable: `arcc check
go/examples/csvtool/app/component.textproto` proves an orchestrator of a
FILES-holding component is itself ambient-authority-free.

## Step 12: Self-hosting + CI + clause-coverage sweep

**Objective.** FR8: arcc checks arcc; the coco discipline is verifiably
applied to every one of its own components.

**Guidance.** Finalize all eight components' manifests + contract files
(most exist from their steps — reconcile drift); wire CI to run `arcc check`
over every own manifest; assert the core four are ambient-authority-free and
`manifest`'s REFLECT is declared. Upgrade `manifest`'s
`INV:parse-authority-free` from `REVIEW:` to mechanical evidence (the
self-hosting check itself). Sweep every component's clause→test map for
unmapped Tier-1 clauses; run a `coco-contract-review` pass over the final
contract set (tier coherence, rely-set accuracy).

**Tests.** CI job runs the self-check and the full suite; a script (or make
target) fails if any own-manifest check fails.

**Integration.** Closes the loop: the tool's own architecture is enforced by
the tool, contracts designated in its own manifests.

**Demo.** One CI job output showing `arcc check` PASS for all eight arcc
components + four example components — the self-hosting, contract-designating
conformance run.

---

**Milestones.** Tool UX complete (hermetic) after **Step 7**; first real
end-to-end checking after **Step 8**; full enforcement after **Step 10**;
showcase + self-hosting **Steps 11–12**. Every step lands with its
components' contracts and manifests, so a stop at any boundary leaves a
coherent, coco-conformant partial system.
