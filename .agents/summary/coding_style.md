# Coding Style and Conventions

## 1. What is mechanically enforced

There is **no** `.editorconfig`, `.golangci.yml`, `buf.yaml`, or Starlark linter config
anywhere in the repo. Enforcement is entirely justfile-driven and Go-native:

```
lint:
	cd go && go vet ./...
	cd go && test -z "$(gofmt -l .)"

fmt:
	cd go && go fmt ./...
```

- `go vet ./...` must pass.
- `gofmt -l .` must produce **no output** — every file must be gofmt-clean. Tabs, standard
  import grouping, and standard brace/spacing all follow from this.
- `just lint` runs inside `just ci`, so both are pre-commit gates.

`.bazelrc` enforces pure-Go builds (`--@rules_go//go/config:pure`) and terse test output
(`test --test_output=errors`).

## 2. Style reference

No style guide is named in any config file. Per the maintainer, treat the
[Google Go Style Guide](https://google.github.io/styleguide/go/) as the reference baseline
(with [Effective Go](https://go.dev/doc/effective_go) and
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) behind it). The codebase is
consistent with it: small public surfaces, errors wrapped with context, no stuttering names,
receiver names short and consistent, doc comments beginning with the identifier name.

Where this codebase adds conventions beyond the guide, they are documented below.

## 3. The Component Contract doc-comment template

**The single most distinctive convention.** Every non-test package carries a package doc
comment in this exact shape:

```go
// Package report defines the conformance report data models and rendering logic.
//
// Component Contract (FR10):
// - What it does: Defines the representation of architectural checker findings and renders them into deterministic, human-readable text.
// - What it requires: Receives a ConformanceReport struct populated with violations and warnings from the checker.
// - What it provides: RenderText for plain-text formatting. Avoids JSON marshaling internally to keep the package free of reflection.
// - Ambient Authority: This component is guaranteed-pure and holds no ambient authority (performs no filesystem I/O, network, process execution, or reflection).
package report
```

This is not decoration — FR10 says the informal contract *is* the doc-comment prose in the
interface files, which is why `component.proto` has no `contract` field. **A new package must
carry this block.**

## 4. Requirement-number annotations

Rules from the design spec are cited by short code (FR1, FR2, FR3–FR6, FR10, M5, M7, M9, A4,
C9–C12) in doc comments, inline comments, and test names:

```go
// 4d. FR5 Cross-component Call Boundary Checks (design §5.3b)
```

```go
func TestCheck_FR3_ConformingImports(t *testing.T) { ... }
```

Follow this when touching rule-implementing code: the annotation is how a reader gets from
code back to the spec under `.agents/planning/`.

## 5. Naming and file organization

- One package = one primary file named after the package (`checker.go`, `facts.go`,
  `report.go`, …). The two large shell packages keep this even at scale: `goanalysis.go` is
  ~2,140 lines and `packagelayout.go` ~1,250, sectioned by comment banners rather than split
  into files.
- Exported identifiers PascalCase; unexported helpers camelCase with long descriptive names
  (`validateLayoutMembership`, `resolvePatternMembershipDependencyInterface`,
  `describeInterfaceFileConstraintWithContext`).
- Enums are typed strings or small ints in grouped `const (...)` blocks with a `String()`
  method — `report.Kind`, `capanalyzer.Class`, `capanalyzer.Decision`,
  `manifest.InterfaceStyle`.
- Public surfaces are kept deliberately small: `goanalysis` exports 5 symbols over ~55
  unexported functions.
- Compile-time interface assertions where a type implements a port:
  `var _ capanalyzer.CapabilityAnalyzer = (*Adapter)(nil)`.

## 6. Comments explain *why*

A strong and consistent convention: comments carry design intent, not restatement of the
code. Representative examples worth imitating —

- `capslockadapter`'s `(*os.File)` exception list explains the object-capability rationale
  *and* why `Chdir` is excluded (process-global vs. object state).
- `packagelayout`'s `init()` explains why the driver dispatch cannot live in `main`.
- `goanalysis`' pointer-keyed cache explains that the alternative would pollute the `facts`
  DTO.
- `facts` field comments state **provenance**: whether a value is author-declared or
  shell-derived, and whether the checker consumes it.
- The justfile's `selfcheck` recipe carries a 12-line comment on why the two check legs exist.

## 7. Error handling — three idioms, chosen per need

**1. `fmt.Errorf` with `%w`** — the dominant idiom, used throughout the shell:

```go
return fmt.Errorf("failed to load packages: %w", err)
return fmt.Errorf("dependency manifest does not exist at %q: %w", manifestPath, err)
```

Note `%q` for paths and identifiers.

**2. Sentinel errors** for simple, parameterless conditions (`manifest`, `packagelayout`):

```go
var ErrEmptyName = errors.New(...)
var ErrPackageSurfaceRequiresMembers = errors.New(...)
```

**3. Custom error struct types** where the message needs structured payload (`manifest` only):

```go
type DuplicateDeclarationError struct { Kind, Value string }
type UnknownCapabilityError struct { Capability string }
type InvalidMemberError struct { Member, Reason string }
```

**And the architectural one: errors as report data.** `checker.Check(Inputs)` returns
`report.ConformanceReport` with **no error**. It is total; every failure mode is a
`report.Finding`. Do not add an error return to the pure core.

Two contrasting accumulation policies, both deliberate:

| Function | Policy |
|---|---|
| `manifest.validate` | **fail fast** — return the first violation |
| `checker.Check` | **accumulate** — collect every finding |

Fail-closed is the default posture: `mapClass` errors on an unknown capability rather than
ignoring it; a member with no source files is a load error; the Bazel rule refuses a cgo
closure rather than emitting a layout it cannot honour.

## 8. Determinism

Every collection that reaches output is explicitly sorted — never left to map iteration order.
`checker` sorts findings by message, then file, then line. `capslockadapter` uses hand-written
frame comparators rather than reflection-based comparison. Starlark sorts members, deps and
packages before writing. If you add a field to a report or a generated artifact, sort it.

## 9. Testing conventions

- **File naming**: `<pkg>_test.go` for the main suite, plus thematic files split by concern
  (`canonicalization_test.go`, `dep_resolve_test.go`, `stdlib_provenance_test.go`,
  `pattern_match_test.go`, `schema_test.go`, `membership_test.go`).
- **Black-box by default**: most tests use `package <pkg>_test` and import the package under
  test by its full path. White-box (`package <pkg>`) only where an unexported seam must be
  reached — `goanalysis`, `packagelayout`, `capslockadapter` — and `isstdlib_internal_test.go`
  puts that fact in the filename.
- **Build tags for slow tests**: `//go:build integration` gates anything doing a real
  `go/packages` load or invoking Capslock. `just test` runs the fast set; `just
  test-integration` adds the rest.
- **Table-driven**: `[]struct{ name string; ... }` iterated with
  `for _, tt := range tests { t.Run(tt.name, func(t *testing.T) { ... }) }`.
- **Test names encode the rule**: `TestCheck_FR3_ConformingImports`,
  `TestIntegration_LayoutMode_ConcurrentIsolation`.
- **Standard library only** for assertions: `t.Fatalf` / `t.Errorf` with `%v`/`%#v`/`%q`, and
  `reflect.DeepEqual` for whole-struct comparison. **Do not introduce `testify` or `go-cmp`.**
- **No Go-side golden files**: expected values are inline Go literals. Golden files exist only
  on the Bazel side (`bazel_rules/go/tests/goldens/`).
- **`testdata/` holds real Go packages**, not text fixtures — organized by scenario
  (`success/`, `escapes/`, `dep_resolve/`, `broken_syntax/`; `pure/`, `filereader/`,
  `packageprune/`, `scope/`) and loaded through the real analysis path.
- **Seams over mocks**: package-level function variables (`goanalysis.loadPackages`,
  `packagelayout.osStat`) and injected struct fields (`Runner.Loader`, `Runner.Analyzer`).
  There is one spy — `spyTB` in `manifestparity_test.go`, embedding `testing.TB` and
  overriding `Errorf`/`Helper` to test a test helper.

## 10. Logging and observability

**There is none, deliberately.** No `log`, no `slog`, no third-party logger appears anywhere
in `go/internal/`. Diagnostics travel as `error` values (wrapped with context) or as
`report.Finding` entries. The only direct stderr write is `packagelayout`'s driver dispatch,
which is protocol output; `RunDriver` takes an explicit `io.Writer` rather than writing to
`os.Stdout`. `Runner.Run` takes `stdout`/`stderr` as parameters for the same reason.

Do not add logging to a core package — it would break the authority-free property that
`just selfcheck` verifies.

## 11. Imports

- Standard library first, then third-party/internal, separated by a blank line (gofmt
  grouping); internal imports use the full module path.
- Core packages import only the standard library. Adding a third-party import to `checker`,
  `facts`, `report`, `capanalyzer`, or `hostpolicy` would be an architectural change, and
  arcc would catch it.
- Every import path in `goanalysis` passes through `hostpolicy.CanonicalizePath` before it is
  compared or stored.

## 12. Starlark conventions

- Public API is re-exported from `bazel_rules/go/defs.bzl`; `private/` has
  `default_visibility = ["//bazel_rules:__subpackages__"]` and a comment pointing readers at
  `defs.bzl`.
- Only `go_adapter.bzl` may load from `@rules_go`. Everything above it stays host-agnostic.
- Validation `fail()`s early and names the component in the message.
- Rule attributes are `configurable = False` unless there is a reason otherwise.
- Command construction lives in exactly one place (`arcc_check_argv`) so a future build action
  and the test rule cannot drift.

## 13. Representative examples

| Pattern | File | Why this one |
|---|---|---|
| Package doc contract, pure package | `go/internal/report/report.go` | the clearest FR10 block; typed-string enum with `String()`; zero non-stdlib imports |
| The pure decision core | `go/internal/checker/checker.go` | `Inputs` struct seam, no error return, inline FR annotations, explicit sorting |
| Core DTO package | `go/internal/facts/facts.go` | provenance-documenting field comments; one shared helper (`MatchesMember`) |
| Shell adapter over a third-party analyzer | `go/internal/capslockadapter/capslockadapter.go` | port implementation, compile-time assertion, policy rationale in comments, deterministic sorting |
| Mixed error idioms + validation | `go/internal/manifest/manifest.go` | sentinels, custom error types, and `%w` wrapping side by side; fail-fast `validate`; defensive `copyStrings` |
| Host override seam | `go/internal/hostpolicy/hostpolicy.go` | tiny file, contract-in-comments, function vars as strategy |
| CLI wiring and DI | `go/cmd/arcc/main.go` + `app/app.go` | injected `Loader`/`Analyzer`, `io.Writer` parameters, hand-rolled parsing, the numbered pipeline |
| Scoped global state | `go/internal/packagelayout/packagelayout.go` | `WithDriverEnv`/`WithTemporaryLayout`, `init()` dispatch with rationale, `osStat` seam |
| Typical unit test | `go/internal/checker/checker_test.go` | black-box package, rule-named tests, literal `Inputs`, `reflect.DeepEqual` |
| Integration test with a real toolchain | `go/cmd/arcc/cli_integration_test.go` | `//go:build integration`, builds the binary in `TestMain`, drives it as a subprocess |
| Hermetic integration test | `go/cmd/arcc/layout_integration_test.go` | `runArccHermetic` strips `PATH` to prove no toolchain is used |
| Test helper taking `testing.TB` | `go/internal/manifestparity/manifestparity.go` | reusable test behavior as a production package; skips when given no files |
| Worked component manifest | `go/examples/csvtool/app/component.textproto` | the canonical shape; FR5b pruning in four lines |
| Starlark rule implementation | `bazel_rules/go/private/component.bzl` | analysis-time-only emission, fail-closed validation, deterministic output |
| Starlark host seam | `bazel_rules/go/private/go_adapter.bzl` | the whole porting surface, with the two documented traps |
| Starlark analysis test | `bazel_rules/go/tests/component_tests.bzl` | `rules_testing` suite; test-only predicate injection without touching the production rule |
