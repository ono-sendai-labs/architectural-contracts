# Architectural Contracts — Knowledge Base Index

**Read this file first.** It is designed to be the only summary document you need in context:
it carries enough metadata about each companion file that you can decide which (if any) to
open for a given question.

- **Project**: Architectural Contracts (`arcc`) — a Go tool that checks component manifests
  against Go package structure, dependency boundaries, and ambient-authority findings.
- **Baseline revision**: `7d787ddfd076ccd45e80affbc82628e542cf145c` (also in `last_commit`)
- **Generated**: 2026-09-02, full analysis

## Instructions for AI assistants

1. **Start here.** The 60-second orientation below answers most questions on its own.
2. **Open one companion file** when you need specifics. Use the routing table — do not open
   all of them.
3. **Verify before you rely.** These documents describe the codebase at the baseline revision.
   If a fact will drive an edit, confirm it against the source; file paths given here are
   accurate as of the baseline but the code is the authority.
4. **Respect the invariants in §"Non-negotiables"** below. Several of them are properties the
   tool verifies about itself, so violating one turns `just ci` red.
5. **Before committing anything**, run `just ci`. Use `jj`, not raw `git`, for local VCS
   operations.

## 60-second orientation

`arcc check <manifest>` reads a `component.textproto`, derives facts about the component's Go
packages, runs a Capslock-backed capability analysis over a VTA call graph, and prints a
conformance report. Exit `0` = conforms, `1` = violations, `2` = tool error.

The code is **ports and adapters**: a pure core (`checker`, `facts`, `report`, `capanalyzer`,
`manifest`, `hostpolicy`) that declares no ambient authority, and shells (`goanalysis`,
`capslockadapter`, `packagelayout`) that own all I/O. The core's only view of Capslock is the
`capanalyzer.CapabilityAnalyzer` interface.

Two check legs exist and both matter: **native** (`just selfcheck`, 8 components, real Go
toolchain, FR1 directory membership) and **hermetic Bazel** (`bazel test //...`, 6 components,
no toolchain, declared `members` via an emitted package-layout JSON). Checking the same
components two ways cross-checks the membership model itself. The two missing components,
`capslockadapter` and `cli`, are excluded because capslock's closure contains a cgo package
and the Bazel rule fails closed on cgo.

The central idea a reader must hold: a component **prunes** capability analysis at its
declared `component_dependencies` (their authority does not bleed upward) and **absorbs** the
authority of its `absorbed_dependencies`. Everything else it reaches is
`UNDECLARED_DEPENDENCY`.

## Routing table

| If the question is about… | Open |
|---|---|
| What the project is, repo layout, scale, tech stack, the two build legs | `codebase_info.md` |
| Core/shell split, loading modes, the component model, trust/certification, the Bazel rule design, known limits | `architecture.md` |
| What a specific package does, its responsibilities, the Bazel rule files, the csvtool example | `components.md` |
| CLI flags and exit codes, Go port signatures, GOPACKAGESDRIVER, `go_component` attributes, providers, CI/release triggers | `interfaces.md` |
| The proto manifest schema, the package-layout JSON schema, `facts`/`report`/capability structs, finding kinds | `data_models.md` |
| The check pipeline step by step, hermetic checking, the Bazel component build, the dev loop, authoring a component, release | `workflows.md` |
| Go module deps, Bazel deps, required tools, sync invariants, what is deliberately not used | `dependencies.md` |
| Doc-comment template, error idioms, naming, testing conventions, representative example files | `coding_style.md` |
| Gaps and caveats in this documentation set | `review_notes.md` |

## File summaries

### `codebase_info.md`
Identity, purpose, and the three pillars. Full annotated repository tree. Scale metrics (66 Go
files, ~20k lines, 8 self-hosted components). Technology stack with pinned versions. The
native-vs-Bazel leg comparison table. A short list of the architectural patterns in play.
*Consult for orientation and "where does X live".*

### `architecture.md`
The deepest document. A Mermaid graph of the whole dependency structure with pure/shell
colouring; a table of which packages declare which authority; the two loading modes and why
the GOPACKAGESDRIVER self-exec has to live in `init()`; the component model (members,
interface styles, the three ways to reach an outside package); trust and compositionality
(`own_check_runs`, `certified`/`asserted`, `auto_attached`); the Bazel layer with aspect and
provider flow; determinism as a design rule; and the eight architecturally load-bearing
limitations. *Consult before any structural change.*

### `components.md`
A summary table of all eleven Go packages with role, declared authority, internal deps, and
scale — then one section per package covering responsibility, notable mechanisms, and design
intent. Includes the csvtool example graph and a table of the Bazel rule files. *Consult when
you need to know what a package is for before editing it.*

### `interfaces.md`
Every contract surface, with verbatim signatures. The complete CLI (invocation forms, flags,
error strings, exit codes, stream behavior, text and JSON report shapes); the four in-process
Go ports; the GOPACKAGESDRIVER protocol; Capslock integration; the exact arcc command line
Bazel builds; the `go_component` attribute table; both providers; the full `go_adapter.bzl`
porting surface; and CI/release triggers. *Consult when calling into or extending a boundary.*

### `data_models.md`
Field-by-field schemas. The complete `component.proto` (all nine `Component` fields, the
`InterfaceStyle` enum, both nested messages, the 13 capability names) plus the Go-side
`Manifest` and its validation invariants. The package-layout JSON schema including the two
conforming `Imports` shapes and the `is_stdlib` provenance trap. All `facts` and `capanalyzer`
structs. The report model with a table of all ten finding kinds and their violation/warning
class. *Consult before touching a schema or adding a finding kind.*

### `workflows.md`
Sequence and flow diagrams for: the eleven-step `check` pipeline; hermetic layout-mode
checking; the Bazel component build with its fail-closed points; the development loop and what
`just ci` runs in order; self-hosting verification and why both legs are kept; authoring a new
component; reproducing a violation; the release workflow; and repository conventions (jj,
`.agents/`, the GitHub bot token). *Consult for "how do I do X".*

### `dependencies.md`
The full `go.mod` and `MODULE.bazel` with a per-dependency "used by / for what" table. The
internal package dependency edges. Required build tools and their coupling (note:
`gen-is-clean` needs both `protoc` and `jj`). `.bazelrc`/`.bazelignore`/gazelle directives.
How to consume `rules_arcc` externally. A table of cross-artifact sync invariants and what
enforces each. A section on what is deliberately *not* depended on. *Consult before adding a
dependency.*

### `coding_style.md`
What `just lint` enforces (`go vet` + `gofmt`, nothing more). The Google Go Style Guide as the
reference baseline. The mandatory Component Contract doc-comment template, quoted verbatim.
Requirement-number annotations. Naming and file organization. The three error idioms and the
architectural fourth ("errors as report data"). Determinism. Nine testing conventions.
The deliberate absence of logging. Starlark conventions. A 16-row table of representative
example files with a note on why each was chosen. *Consult before writing new code.*

### `review_notes.md`
Consistency check results, known gaps in this documentation set, and maintenance
recommendations. *Consult if something here looks wrong or missing.*

## Non-negotiables

Violating any of these breaks a property the project verifies about itself:

1. **Core packages import only the standard library.** Adding a third-party import to
   `checker`, `facts`, `report`, `capanalyzer`, or `hostpolicy` breaks the authority-free
   claim that `just selfcheck` proves.
2. **`checker.Check` returns no error.** Failure modes are `report.Finding` values.
3. **No logging anywhere in `go/internal/`.** Diagnostics are errors or findings.
4. **No `testify`, no `go-cmp`.** Standard library assertions only.
5. **Every non-test package carries the `Component Contract (FR10)` doc-comment block.**
6. **Anything reaching output must be explicitly sorted.** Golden tests depend on it.
7. **Only `go_adapter.bzl` may load from `@rules_go`.**
8. **Adding a capability name means updating both** `manifest.KnownCapabilities` **and**
   `bazel_rules/authority.bzl` — `authority_sync_test` fails otherwise.
9. **Changing `component.proto` means running `just gen`** — `gen-is-clean` fails otherwise.
10. **Use `jj`, not raw `git`, for local VCS state.** Run `just ci` before every commit.

## Example queries this knowledge base answers

- *"Why does `app` check as authority-free when it reads a file?"* → `architecture.md` §4,
  `components.md` (csvtool), `workflows.md` §7.
- *"What exit code does arcc return for a malformed manifest?"* → `interfaces.md` §1 (2).
- *"I want to add a new finding kind."* → `data_models.md` §5 for the kind table and
  violation/warning split; `coding_style.md` §8 for the sorting requirement; `components.md`
  (`checker`) for where the rules live.
- *"How do I port the Bazel rules to a different Go ruleset?"* → `interfaces.md` §3
  (`go_adapter.bzl` surface) and `architecture.md` §6 for the two documented traps.
- *"Why can't `capslockadapter` be checked under Bazel?"* → `codebase_info.md`,
  `workflows.md` §5 — cgo in capslock's closure; the rule fails closed.
- *"What does a typical unit test look like here?"* → `coding_style.md` §9 and §13
  (`go/internal/checker/checker_test.go`).
- *"What must stay in sync when I add a capability?"* → `dependencies.md` §6.
- *"Is `own_check_runs` verified?"* → No. `architecture.md` §5 and `data_models.md` §1 — it is
  a self-declaration at the same trust level as `declared_authority`.
