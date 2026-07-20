# Spike: hermetic `go/packages` loading via a self-exec `GOPACKAGESDRIVER`

Status: complete — 2026-07-20. Runnable spike in `./spike-driver/`.

## Question

The design's hermetic loading mode hinges on one unproven seam: can arcc load Go
packages and run Capslock capability analysis **entirely from a build-system-supplied
package layout** — no `go list`, and no `go.mod` present at analysis time — by pointing
`GOPACKAGESDRIVER` at arcc itself? This is the piece the earlier Bazel spike
(`spike-findings.md`) did **not** cover, and it is now on the critical path (see
"Design impact" below).

If this works, arcc's existing `capslockadapter` / `goanalysis` code loads through the
driver **unchanged** (both already call `golang.org/x/tools/go/packages`), and the check
becomes a fully sandboxed, cacheable action that needs no Go module in the workspace.

## What the spike is

A single self-exec binary (`spike-driver/main.go`) with three modes:

1. `gen <moduleDir> <pattern>...` — loads the closure the ordinary way (`go list`, in a
   real module) and serializes it to a **package-layout JSON**. This stands in for what a
   Bazel aspect would emit from Bazel's dependency graph; the spike does not reproduce the
   aspect, only feed the consumer a realistic layout. `packages.Package` has custom
   JSON (un)marshaling matching the go/packages driver "flat" form, so the layout
   round-trips faithfully for free.
2. **Driver mode** (auto-detected via `ARCC_SPIKE_DRIVER=1`) — answers the
   `GOPACKAGESDRIVER` protocol (`DriverRequest` on stdin, patterns as argv, `DriverResponse`
   on stdout) from the layout JSON.
3. `analyze <layout.json> <importpath>` — sets `GOPACKAGESDRIVER` to itself and runs the
   **same Capslock calls arcc's `capslockadapter` makes** (`PackagesLoadModeNeeded`,
   `GetQueriedPackages`, `GetCapabilityInfo` with `GranularityFunction`), printing findings.

Fixtures (`spike-driver/testdata/`, a separate `example.com/svc` module):
`svc` mints filesystem authority (`os.Open`); `clean` is pure computation.

Environment: Go 1.26.4, `golang.org/x/tools v0.48.0`, `github.com/google/capslock v0.3.2`
(matching `go/go.mod`).

Reproduce: `./spike-driver/run.sh`.

## Results — the seam works

Analyzing **from a freshly created temp dir with no `go.mod`**, loading only through the
driver:

```
== analyze example.com/svc/svc (expect FILES) ==
target example.com/svc/svc: 1 capability finding(s)
  FILES                  x1
  e.g. FILES: example.com/svc/svc.ReadFirst -> os.Open

== analyze example.com/svc/clean (expect no capabilities) ==
target example.com/svc/clean: 0 capability finding(s)
```

Capslock type-checked from source and traced the interprocedural call path
(`ReadFirst → os.Open`) purely from driver-provided file lists — so `NeedTypes` /
`NeedSyntax` / `NeedTypesInfo` all resolved through the driver. The pure package
correctly yielded nothing.

**Negative control (airtight):** the same query without the driver —
`GOPACKAGESDRIVER=off go list example.com/svc/svc` from that module-less dir — fails with
`go.mod file not found in current directory or any parent directory`. So the successful
analyze proves the *driver*, not a `go list` fallback, did the loading.

**Layout size:** the `svc` closure plus the full std library serialized to 362 packages /
~493 KB.

## Findings for the design (gotchas the spike surfaced)

1. **The driver must answer the `"std"` meta-pattern, not just the queried importpaths.**
   Capslock issues its own `packages.Load(nil, "std")` deep in the analysis path
   (`GetCapabilityInfo → collectPackageInfo → standardLibraryPackages`, confirmed by
   source and by the passing run). The layout must therefore enumerate the **entire**
   standard library, not only the closure's imports, and the driver must resolve `"std"`
   to all of them. In the spike, `gen` appends `"std"` to its load patterns for exactly
   this reason. For the real rule this means the package-layout emission must include the
   full SDK stdlib package set (available from the rules_go Go SDK toolchain).

2. **Stdlib source must be readable at analysis time — from `GOROOT/src`, hermetically.**
   Of all file paths in the layout, 5126 point into `GOROOT/src` (stdlib), 0 into the
   build cache, 0 into the module cache. In a Bazel/sandboxed setting the rules_go SDK
   provides `GOROOT/src` as a declared input, so this is hermetically satisfiable — but it
   **must** be wired in; the driver hands go/packages source paths, and go/packages reads
   and type-checks them in-process.

3. **cgo is the one hermeticity risk to watch.** In this closure exactly 1 of 362 packages
   had `CompiledGoFiles != GoFiles` — `unsafe` (compiler-builtin, benign). No genuine cgo
   package was pulled, so nothing referenced cache-generated files. A closure that pulls
   cgo (e.g. via `net`, `os/user`) would have `CompiledGoFiles` pointing at cgo-preprocessed
   sources that `go list` leaves in `GOCACHE` — **not** hermetic. The real layout emission
   must obtain those preprocessed sources as declared Bazel outputs (rules_go produces them)
   rather than pointing into the build cache. Flagged for the layout-schema work; not a
   blocker for the mechanism.

4. **`NeedModule` is silently unsatisfied under a driver, and that's fine.** go/packages
   documents that module info is absent in driver mode; Capslock requests `NeedModule` but
   tolerates its absence (analysis produced correct results). No action needed.

5. **Self-exec dispatch is clean.** `GOPACKAGESDRIVER` takes only an executable path (no
   subcommand), so driver mode is selected by an inherited env var (`ARCC_SPIKE_DRIVER=1`)
   rather than argv. arcc's real implementation can use the same trick (the binary detects
   driver mode from the environment and treats argv as query patterns).

## What this validates / what remains

**Validated:** the load-and-analyze consumer path — driver JSON → `packages.Load` →
type-check → Capslock capability findings — works with no `go.mod` and no `go list`, using
arcc's unmodified analysis calls. This is the load-bearing risk; it is retired.

**Still to build (not spiked here, lower risk):** the *producer* — a Bazel aspect that emits
the package-layout JSON from Bazel's own graph, including the full SDK stdlib set (finding 1)
and cgo-preprocessed sources (finding 3). Layout *production* is build-system-specific and is
where per-workspace variation in Go rules gets absorbed; layout *consumption* (validated here)
is uniform.

## Design impact

This retires the blocker that forced the two-phase plan. Combined with the review decision to
drop the non-hermetic Phase 1 (it required a working Go module in the consuming workspace,
which Bazel-only workspaces without a `go.mod` do not have), the design collapses to a single
**hermetic** delivery:

- No `--source-root` flag, no `component_root` proto field, no workspace-root recovery from
  runfiles symlinks, no `local`/`external` test tags, no "must be a working Go module" caveat.
- arcc gains one loading seam: a self-exec `GOPACKAGESDRIVER` mode fed by the layout JSON.
- Membership becomes explicit (from the layout) from day one.

The detailed-design document should be reworked from "Phase 1 / Phase 2" to hermetic-only;
these notes fix the direction and the validated mechanism.
