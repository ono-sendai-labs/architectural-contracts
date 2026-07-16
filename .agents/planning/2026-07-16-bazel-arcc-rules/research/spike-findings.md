# Spike Findings: go_component mechanics on Bazel 9.2

Executed 2026-07-16 on Bazel 9.2.0, rules_go 0.61.1, Go SDK 1.24.5 (downloaded via bzlmod). Full runnable workspace preserved in `research/spike/` (originally run in the session scratchpad). All targets built and all three tests passed on the first complete run.

## Questions validated

### A. Provider forwarding — CONFIRMED
A custom rule that returns the interface `go_library`'s `GoInfo` and `GoArchive` providers is a drop-in dependency for rules_go rules. `go_test` with `deps = ["//svc:svc_component"]` (the *component* target, not the library) compiled and passed. This settles rough-idea question 2's "just work" concern definitively.

### B. Analysis-time closure enumeration + manifest generation — CONFIRMED
From `GoArchive`: `archive.transitive` is a depset of `GoArchiveData` with usable `.importpath` and `.srcs`. The spike rule generated a `component.textproto` via `ctx.actions.write` containing interface files (all srcs of the interface lib, per Q3), component-dep manifest references (via the `ArccComponentInfo` provider), the transitive package closure, and `declared_authority`. Generated manifest for `svc_component`:

```textproto
name: "svc_component"
interface_files: "svc/api.go"
component_dependencies {
  name: "other_component"
  manifest: "other/other_component.component.textproto"
}
# closure: example.com/spike/other (other/other.go)
# closure: example.com/spike/svc/internal (svc/internal/impl.go)
# closure: example.com/spike/svc (svc/api.go)
declared_authority: "FILES"
```

**Important detail:** `GoArchive.transitive` does **not** include stdlib packages (the closure showed only `example.com/*` despite `fmt`/`os` imports). Good news for absorbed-dep classification (no stdlib noise to filter); confirms that Phase 2's package-layout mode must source stdlib separately from the Go SDK.

### C. Symbolic macros — CONFIRMED
A `macro()` with typed attrs (`configurable = False` on the label lists) expanding into the component rule (`name`) plus a check test (`name + ".check"`) works on Bazel 9.2. The `tags = ["local"]` attribute set inside the macro on the test propagates correctly.

### D. Local-test workspace resolution — CONFIRMED (Phase 1 keystone)
In a `tags = ["local"]` (unsandboxed) test, runfiles entries for source files are symlinks whose `realpath` resolves to the **real workspace path** (`.../spike/svc/api.go`). So the Phase-1 non-hermetic check test can recover the true component root from any interface source file's runfiles symlink and point `arcc check` (with the Q4 root override) at real sources where `go list` works.

## Design details surfaced by the spike

1. **Path relativization:** `File.short_path` is workspace-relative; arcc wants component-root-relative `interface_files` and manifest-relative (or root-relative) `component_dependencies.manifest` paths. The rule must compute the component root (e.g., common directory prefix of member libraries, or simply the BUILD package directory) and relativize. Interacts with the Q4 root-override design.
2. **Closure subtraction:** to derive absorbed deps (Q5), each `ArccComponentInfo` should also carry the component's own transitive closure depset so dependents can subtract component-dep-covered packages from their interface closure. Straightforward depset algebra at analysis time.
3. **Multi-library interfaces:** the spike forwarded providers only for a single-entry `interface`; forwarding N libraries' `GoInfo`s from one target is not possible (one `GoInfo` per target). If `interface` allows multiple libraries, the component target can only forward one — options: restrict `interface` to exactly one label (plus e.g. `extra_interface` without forwarding), or accept that multi-interface components aren't directly dep-able. Take to design.
4. **Test hygiene:** set `size = "small"` on the generated check test (Bazel warned about unspecified size); `local` + `external` tags and proper caching semantics for the non-hermetic phase need a decision (an unsandboxed test that reads the real workspace should probably also be `external` or `no-cache` so edits outside declared inputs aren't masked by cache hits — declared srcs *are* inputs, so cache invalidation on source changes does work; the risk is go.mod/module-cache drift).
5. **Environment quirk (spike-only):** rules_go pulls in a CC toolchain by default; `--@rules_go//go/config:pure` + `BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1` avoided the need for a host C compiler. Not a design constraint for consumers with normal toolchains, but worth documenting for hermetic-ish environments.

## What the spike did NOT cover

- Running real `arcc` in the check test (used a stub script). Phase 1 design must wire the `go_binary`-built arcc into the test runfiles and pass the root override.
- Absorbed-dep subtraction and diamond semantics (design-level, mechanics confirmed feasible).
- Aspect-based consistency checking of `component_deps` vs. the actual dep graph.
- Phase 2 package-layout loading (arcc-side feature).
