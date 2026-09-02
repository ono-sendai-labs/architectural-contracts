# Interfaces and Integration Points

## 1. The `arcc` CLI

```
arcc checks Go architectural component contracts.

Usage:
  arcc check <manifest> [--package-layout=<layout>] [--format=json]
  arcc --version
```

### Invocation forms

| Invocation | Behavior | Exit |
|---|---|---|
| `arcc` (no args) | usage to **stdout** | 0 |
| `arcc help` / `--help` / `-h` | usage to **stdout** | 0 |
| `arcc --version` | `arcc 0.0.0-dev` to stdout | 0 |
| `arcc <other>` | `unknown command: %s` + usage to **stderr** | 2 |
| `arcc check` (no manifest) | `error: check command requires exactly one argument` | 2 |
| `arcc check <manifest> [flags]` | runs the pipeline | 0 / 1 / 2 |

### Flags

| Flag | Type | Default | Notes |
|---|---|---|---|
| `--format=json` | boolean literal | text output | Only the literal form. Duplicate → `error: duplicate option: --format=json`, exit 2 |
| `--package-layout=<path>` | string | native mode | Enables hermetic layout mode. `--package-layout=` → `error: empty package layout value`; `--package-layout` without `=` → `error: missing package layout value`; duplicate → `error: duplicate option: --package-layout`. All exit 2 |

Any other `-`-prefixed token → `unknown option: %s` + usage on stderr, exit 2.

### Exit codes (the contract Bazel and CI rely on)

| Code | Meaning |
|---|---|
| `0` | Conforms and does not exceed declared authority. **Warnings do not change this.** |
| `1` | Architectural violations detected |
| `2` | Tool or execution error (bad args, unparsable manifest, load failure, dependency resolution failure) |

### Streams

`stdin` is never read. `stdout` carries usage text, the version string, and — on exit 0 or 1
only — the rendered report (`report.RenderText`, or `json.MarshalIndent(..., "", "  ")` plus a
newline). `stderr` carries every `error:`-prefixed diagnostic, and usage text is echoed there
for argument errors.

### Text report shape

```
Component "app" conforms; does not exceed declared authority

Dependencies:
- csvfile: certified
- toprow: certified
```

and on violation:

```
Component: absorbapp

Violations:
- [UNDECLARED_AUTHORITY] use of undeclared authority "FILES" in package "…/absorbapp"
  Evidence:
    - …/absorbapp.Run at :0
    - …/csvfile.Read at main.go:10
    - os.ReadFile at csvfile.go:22
```

### JSON report shape

```json
{
  "component": "absorbapp",
  "violations": [
    {
      "kind": "UNDECLARED_AUTHORITY",
      "message": "…",
      "location": { "file": "", "line": 0 },
      "evidence": ["…", "…"]
    }
  ],
  "warnings": null
}
```

`dependencies` and `evidence` are `omitempty`.

---

## 2. The Go internal ports (in-process seams)

### `capanalyzer.CapabilityAnalyzer` — the capability port

```go
type CapabilityAnalyzer interface {
    Analyze(req AnalyzeRequest) ([]CapabilityFinding, error)
}

type AnalyzeRequest struct {
    Packages        []string
    PruneAt         []InterfaceSymbol
    PruneAtPackages []string
}
```

Production implementation: `capslockadapter.Adapter` (`NewAdapter() *Adapter`, with a
compile-time `var _ capanalyzer.CapabilityAnalyzer = (*Adapter)(nil)` assertion). Tests
substitute a `mockAnalyzer` recording `calledWith` and returning canned findings or an error.

### `app.PackageLoader` — the loading port

```go
type PackageLoader func(goanalysis.LoadRequest) (facts.PackageFacts, error)

type Runner struct {
    Loader   PackageLoader
    Analyzer capanalyzer.CapabilityAnalyzer
}

func (r *Runner) Run(args []string, stdout, stderr io.Writer) int
```

A function type rather than an interface — one method, so a closure is the natural fake.

### `goanalysis` public surface

```go
type LoadRequest struct {
    ComponentName  string
    ComponentRoot  string
    Members        []string
    InterfaceFiles []string
    Absorbed       []string
}

func LoadPackageFacts(req LoadRequest) (facts.PackageFacts, error)

type InterfaceFileExclusion struct { File, Constraint string }

func ValidateInterfaceFiles(componentRoot string, interfaceFiles []string,
    loaded facts.PackageFacts) ([]InterfaceFileExclusion, error)

func ResolveDependencyInterface(declaringRoot, analyzedRoot string,
    dep manifest.ComponentDependency) (facts.DependencyInterface, error)
```

### `checker` public surface

```go
type Inputs struct {
    Manifest  manifest.Manifest
    Facts     facts.PackageFacts
    DepIfaces []facts.DependencyInterface
    Caps      []capanalyzer.CapabilityFinding
    Policy    capanalyzer.CapabilityPolicy
}

func Check(in Inputs) report.ConformanceReport   // no error return, by design

func StripGenericBrackets(s string) string
func NormalizeInterfaceSymbol(sym capanalyzer.InterfaceSymbol) string
func ExtractPackagePath(sym string) string
```

### `hostpolicy` — the host override seam

```go
var CanonicalizePath = func(p string) string { return p }
var IsStdlibPath = func(importPath string) bool { /* first segment has no dot */ }
```

Contract (from the doc comments): both must be **total, idempotent, and safe for concurrent
reads**, and are set once at process `init()` by an embedding host.

### `packagelayout` — scoped-state helpers

```go
func WithTemporaryLayout(layoutPath string, fn func() error) error
func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error
func IsLayoutMode() bool
func GetActiveLayout() *Layout
var CheckMu sync.Mutex
```

---

## 3. External integration points

### GOPACKAGESDRIVER protocol

`packagelayout` implements the `go/packages` external-driver protocol:

```go
func HandleDriverRequest(l *Layout, req *packages.DriverRequest,
    patterns []string) (*packages.DriverResponse, error)
func RunDriver(layoutPath, workspaceDir string, patterns []string,
    stdin io.Reader, stdout io.Writer) error
```

`WithDriverEnv` sets `GOPACKAGESDRIVER=os.Executable()` and `ARCC_DRIVER_MODE=1`; the
sub-process is intercepted in `packagelayout`'s `init()` (before any `main` runs), serves the
request from the layout JSON, and exits. The env-var marker means the driver path cannot be
reached from any user-facing subcommand.

### Capslock

`capslockadapter` consumes `github.com/google/capslock/analyzer` and `.../interesting`,
feeding a generated classifier text and mapping `CapabilityInfo` + `Path()` into
`capanalyzer.CapabilityFinding` + `[]Frame`.

### Bazel → arcc

The only subcommand Bazel invokes is `check`. `arcc_check_argv` is the single place that
builds it:

```
<arcc> check <name.component.textproto> [--package-layout=<name.package-layout.json>] --format=json
```

All paths runfiles-root-relative; the generated launcher `cd`s to `$TEST_SRCDIR` first. The
binary is always `//:arcc` (constant `ARCC_TARGET`), built in-repo — never fetched — so the
rules and the binary cannot version-skew. Exit codes 0/1/2 map to test verdicts, with
`expect_violation = True` inverting the sense.

### Bazel public load surface

```python
load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component", "FILES")
load("@rules_arcc//bazel_rules:authority.bzl", "FILES")  # language-neutral home
```

`go_component` attributes (all `configurable = False`):

| Attribute | Type | Meaning |
|---|---|---|
| `interface` | label | the single `go_library` holding the public surface. Required for declared style, forbidden under `PACKAGE_SURFACE`. Passing a list is a load-time error |
| `interface_style` | string | omit for declared style, or `PACKAGE_SURFACE` |
| `members` | string_list | concrete `go_library` labels (declared style) or import-path patterns (`PACKAGE_SURFACE` only) |
| `component_deps` | label_list | other `go_component` targets; authority pruned at their interfaces |
| `absorbed_deps` | label_list | libraries absorbed as implementation details |
| `declared_authority` | string_list | authority constants; empty = authority-free |
| `contract` | label_list (files) | Bazel-only metadata; arcc never reads it |

Expands to `<name>` (generates manifest + layout, forwards Go providers) and `<name>.check`
(a hermetic `arcc_check_test`, `size = "small"`, inheriting `tags`/`testonly`).

### Providers

```python
ArccPackageInfo = provider(fields = ["packages"])
# depset of struct(importpath, srcs (tuple[File]), deps (tuple[str]), cgo)

ArccComponentInfo = provider(fields = [
    "component_name", "component_root", "manifest", "layout",
    "transitive_manifests", "transitive_layouts", "closure", "contracts",
])
```

`ArccPackageInfo` is aspect-propagated along `deps` and `embed`. `ArccComponentInfo` never
crosses a `deps` edge — the rule reads it directly off `component_deps` / `infra_deps`. It
carries no Go-specific data, so a future `rust_component` can produce the same provider.

### Host-adapter contract (`go_adapter.bzl`)

The porting surface, in full: `GO_PROVIDERS`, `GO_TOOLCHAINS`, `ARCC_TARGET`,
`is_go_target`, `go_importpath`, `go_build_platform`, `go_library_srcs`, `go_target_info`,
`forward_go_providers`, `go_sdk_root`, `go_sdk_srcs`, `INFRA_COMPONENTS`, `go_infra_deps`,
`go_infra_components`, `go_attach_infra`, `go_attached_infra`.

Two traps documented there: `go_build_platform` must read `GoInfo.mode` (the *target*
platform), not `GoSDK.goos` (the *execution* platform); and returning `True` unconditionally
from `go_attach_infra` is a conforming implementation, not a degraded fallback.

---

## 4. CI / release interfaces

`.github/workflows/ci.yml` — on every push and pull request, two parallel `ubuntu-latest`
jobs: **Go** (`just ci`) and **Self-hosting Check** (`just selfcheck`). Go version comes from
`go/go.mod` via `actions/setup-go@v5`.

`.github/workflows/release.yml` — on tags matching `v[0-9]+.[0-9]+.[0-9]+-?*` or manual
dispatch. Cross-compiles a 2×2 matrix (`linux`/`darwin` × `amd64`/`arm64`) with
`-ldflags="-s -w"`, attests build provenance for non-prerelease tags, and publishes
`arcc-<os>-<arch>` binaries via `gh release create --generate-notes` (with `--prerelease` when
the tag has a `-` suffix).
