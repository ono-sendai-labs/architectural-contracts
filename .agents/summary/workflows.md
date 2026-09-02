# Workflows

## 1. `arcc check` — the end-to-end pipeline

```mermaid
sequenceDiagram
    participant U as user / Bazel
    participant M as main.go
    participant R as app.Runner
    participant PL as packagelayout
    participant MF as manifest
    participant GA as goanalysis
    participant CA as Analyzer (capslockadapter)
    participant CK as checker
    participant RP as report

    U->>M: arcc check <manifest> [--package-layout] [--format=json]
    M->>R: Runner{Loader, Analyzer}.Run(args, stdout, stderr)
    R->>R: lock packagelayout.CheckMu
    R->>R: parse args
    alt --package-layout given
        R->>PL: WithDriverEnv(absLayout, cwd, runCheck)
        Note over PL: sets GOPACKAGESDRIVER=self,<br/>ARCC_DRIVER_MODE=1, restores after
    end
    R->>MF: Parse(open(manifestPath))
    R->>R: canonicalize absorbed paths (hostpolicy)
    R->>R: componentRoot = abs(dir(manifest))
    R->>GA: Loader(LoadRequest{...}) → facts.PackageFacts
    R->>GA: ValidateInterfaceFiles(...) → exclusions
    loop each component_dependency
        R->>GA: ResolveDependencyInterface(...) → facts.DependencyInterface
    end
    R->>R: build PruneAt / PruneAtPackages (sorted)
    R->>CA: Analyze(AnalyzeRequest{Packages, PruneAt, PruneAtPackages})
    CA-->>R: []CapabilityFinding
    R->>CK: Check(Inputs{Manifest, Facts, DepIfaces, Caps, StrictPolicy()})
    CK-->>R: ConformanceReport
    R->>R: append InterfaceFileExcluded warnings, stable-sort
    R->>RP: RenderText(report) or json.MarshalIndent
    R-->>U: stdout + exit 0 / 1  (any step error → stderr + exit 2)
```

The eleven numbered steps in `runCheck`, in order:

1. Open and `manifest.Parse` the manifest file.
2. Canonicalize absorbed-dependency import paths via `hostpolicy.CanonicalizePath`.
3. Derive the component root: `abs(dir(clean(manifestPath)))`.
4. Load package facts via the injected `Loader`.
5. Validate interface files against build constraints → `[]InterfaceFileExclusion`.
6. Resolve each declared component dependency into a `facts.DependencyInterface` (opens the
   dependency's own manifest, verifies the `name` matches, derives its interface surface).
7. Build the pruning sets — per-symbol `PruneAt`, and `PruneAtPackages` for `PACKAGE_SURFACE`
   dependencies — sorted for determinism.
8. Run capability analysis via the injected `Analyzer` (skipped when no packages loaded).
9. `checker.Check` — the pure decision. Append exclusion warnings, stable-sort.
10. Render text or JSON.
11. Exit `1` if any violations, else `0`. Any error in steps 1–8 short-circuits to stderr and exit `2`.

## 2. Hermetic (layout-mode) checking

```mermaid
flowchart TD
    A["Bazel: bazel test //pkg:comp.check"] --> B["arcc_check_test launcher .sh<br/>cd $TEST_SRCDIR"]
    B --> C["arcc check comp.component.textproto<br/>--package-layout=comp.package-layout.json --format=json"]
    C --> D["WithDriverEnv: GOPACKAGESDRIVER=self, ARCC_DRIVER_MODE=1"]
    D --> E["go/packages spawns the driver = arcc itself"]
    E --> F["packagelayout.init() sees ARCC_DRIVER_MODE=1"]
    F --> G["RunDriver: DriverResponse built from layout JSON"]
    G --> H["normal pipeline continues — no PATH, no go.mod, no toolchain"]
    H --> I{"exit code"}
    I -->|0| J["test passes"]
    I -->|1| K["test fails (or passes if expect_violation)"]
    I -->|2| L["tool error → test fails"]
```

Sandboxed and cacheable: a green `bazel test` means the contract holds with no reliance on
the host Go toolchain. `layout_integration_test.go` proves the property by stripping `PATH`.

## 3. Building a component under Bazel

```mermaid
flowchart TD
    A["go_component macro"] --> B["_validate_component_shape<br/>(fails fast, names the component)"]
    B --> C["split members into<br/>target_members / pattern_members"]
    C --> D["own_check_runs = 'manual' not in tags"]
    D --> E["_go_component rule (analysis phase)"]
    E --> F["validate declared_authority ⊆ ALL_AUTHORITIES"]
    F --> G["roots = [interface] + members;<br/>ask attachment_fn which infra_deps attach"]
    G --> H["build covered map from component_deps + auto-attached infra"]
    H --> I["validate: absorbed not covered,<br/>members not covered/absorbed,<br/>interface not covered/absorbed"]
    I --> J["merge closures (merge_by_importpath)"]
    J --> K{"any cgo in closure?"}
    K -->|yes| X["fail closed — no honest CompiledGoFiles"]
    K -->|no| L["expand auto_attached_patterns and member_patterns via match_path"]
    L --> M["_classify FR2 frontier → members / absorbed<br/>(rest left for the checker to report)"]
    M --> N{"interface classified as member?"}
    N -->|no| Y["fail — nothing to check"]
    N -->|yes| O["write name.component.textproto"]
    O --> P["write name.package-layout.json"]
    P --> Q["return DefaultInfo + ArccComponentInfo<br/>(+ forwarded GoInfo/GoArchive for declared style)"]
```

Shape validation rules enforced at macro time:

| Style | `interface` | `members` |
|---|---|---|
| declared (default) | **required** | literal target labels only — no `*?[]\` |
| `PACKAGE_SURFACE` | **forbidden** | **required**, patterns allowed |

Any other `interface_style` value is a `fail()`.

## 4. Development loop

```bash
just build            # go build ./... + bin/arcc
just test             # go test ./...
just test-integration # go test -tags=integration ./...
just lint             # go vet ./... && test -z "$(gofmt -l .)"
just fmt              # go fmt ./...
just selfcheck        # build arcc, check its own 8 manifests
just bazel-test       # bazel build //... && bazel test //... && 2 shell tests
just ci               # the pre-commit gate (see below)
```

`just ci` = `gen-is-clean → lint → build → test → test-integration → selfcheck → bazel-test`.

- `gen-is-clean` regenerates the protobuf Go code with `protoc` and asserts `jj diff` over
  `go/internal/manifest/gen/**/*.pb.go` is empty. It is scoped to the protoc outputs because
  `gen/` also holds a Gazelle-generated `BUILD.bazel` that `just gen` does not produce.
- If `protoc`/`protoc-gen-go` are not installed, run
  `just lint build test test-integration selfcheck` instead.
- `bazel-test` runs `members_label_validation_test.sh` and
  `component_shape_validation_test.sh` **outside** `bazel test`, because those scripts
  themselves spawn nested `bazel build` sub-processes. It fails loudly if Bazel is missing
  rather than skipping, so a CI environment meant to have Bazel cannot quietly pass a partial
  `just ci`.
- Regenerate `BUILD.bazel` files after adding or moving packages: `bazel run //:gazelle`.

## 5. Self-hosting verification

```mermaid
graph LR
    subgraph free["Authority-free core — proves they ARE authority-free"]
        checker; facts; report; capanalyzer
    end
    subgraph decl["Declaring components — proves they do not EXCEED"]
        manifest; goanalysis; capslockadapter; cli
    end
```

`just selfcheck` runs all eight natively. `bazel test //...` runs six of them hermetically:
`capslockadapter` and `cli` are excluded because capslock's closure contains
`golang.org/x/sys/unix` built with cgo, and the Bazel rule fails closed on cgo — one root
cause, two components.

Keeping both legs is deliberate rather than redundant: the native leg derives membership from
directories (FR1) and the Bazel leg from declared `members`, so checking the same components
two ways cross-checks the membership model itself. `self_manifest_parity_test` additionally
asserts the generated and checked-in manifests agree, so the two forms cannot drift.

The workaround of patching a third-party build file to claim a cgo package is not cgo is
deliberately not taken: buying a green check by falsifying build metadata is the exact failure
mode these checks exist to remove.

## 6. Authoring a new component

1. Put `component.textproto` at the component root.
2. Set `name`, and either `interface_files` (declared style) or
   `interface_style: INTERFACE_STYLE_PACKAGE_SURFACE` + `members`.
3. Decide membership: omit `members` for FR1 directory-based ownership, or list literal import
   paths. Component roots must be disjoint — no nesting one inside another.
4. For each package you reach outside the component, choose one:
   - another component → `component_dependencies { name, manifest }` (authority pruned)
   - an implementation detail → `absorbed_dependencies { import_path, reason }` (authority charged to you)
   - neither → you will get `UNDECLARED_DEPENDENCY`
5. List `declared_authority`, or leave it empty to claim ambient-authority-freedom.
6. Set `own_check_runs: true` if the check runs in the build; otherwise point
   `certification_reference` at where conformance is established.
7. Run `arcc check <manifest>` (from the Go module directory, so relative paths resolve).

Under Bazel the manifest is generated instead — write a `go_component` target and the rule
emits both the manifest and the layout.

## 7. Reproducing a violation (from the README)

Create a component that *absorbs* `csvfile` instead of declaring it as a component dependency;
the absorbed authority then bleeds into the absorbing component:

```
Violations:
- [UNDECLARED_AUTHORITY] use of undeclared authority "FILES" in package ".../absorbapp"
  Evidence:
    - .../absorbapp.Run at :0
    - .../csvfile.Read at main.go:10
    - os.ReadFile at csvfile.go:22
```

Exit code 1. This is the clearest single demonstration of what `component_dependencies` buys
you over `absorbed_dependencies`.

## 8. Release

Tag `vX.Y.Z[-suffix]` (or dispatch manually with a `tag` input). The workflow cross-compiles
four binaries on `ubuntu-latest` (`linux`/`darwin` × `amd64`/`arm64`, `-ldflags="-s -w"`),
attests build provenance for non-prerelease tags, and publishes them to a GitHub Release with
generated notes (`--prerelease` when the tag has a `-` suffix).

## 9. Repository conventions for contributors

- **VCS**: Jujutsu (`jj`). Prefer `jj new` + `jj squash` over `jj edit`. Do **not** use raw
  `git` to mutate repo state.
- **Workflow**: the `structured-spec-to-code` skills framework. Planning artifacts, task files
  and scratchpad notes live under `.agents/` (`planning/`, `tasks/`, `runs/`, `scratchpad/`,
  `summary/`).
- **Before every commit**: `just ci`.
- **GitHub**: agents authenticate with a limited-permission bot token
  (`gh-bot-token | gh auth login --with-token`); if operations still fail after a refresh, stop.
