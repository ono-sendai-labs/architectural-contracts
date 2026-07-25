# Research — sourcing the analysis platform and build tags

**Question (Q6a).** The layout must describe the platform so the loader can build
a `build.Context` instead of using `build.Default`. Where does a host get GOOS,
GOARCH, build tags and cgo, and what does that mean for the adapter seam?

Source read: rules_go at
`~/.cache/bazel/_bazel_xtof/*/external/rules_go+` (the version this repo builds
against).

## What the Go side needs

`go/build.Context` fields that matter for `MatchFile`:

| Field | Meaning here |
|---|---|
| `GOOS`, `GOARCH` | target platform, drives `_linux.go` / `_amd64.go` suffixes and `//go:build` |
| `BuildTags` | host-supplied tags (`gotags`), the gap that makes the current filter unsound |
| `CgoEnabled` | drives the `cgo` build constraint |
| `ReleaseTags` | Go version tags (`go1.24`, …); defaults are correct if arcc is built with a comparable toolchain |

Today `filterByBuildConstraints` uses `build.Default`, which takes all of these
from the **arcc binary's own** compilation plus ambient environment
(`GOOS`/`GOARCH`/`CGO_ENABLED`/`GOFLAGS` are all read by `go/build` at init).

## What rules_go exposes

- **`GoInfo.mode`** — `providers.rst:103`: "The mode this library is being built
  for." The `mode` struct is built in `context.bzl` (~line 1035) from the
  toolchain defaults and the `@rules_go//go/config` build settings, carrying
  `goos`, `goarch`, `tags`, `pure`, `static`, `race`, `msan` and more.
- **`tags`** specifically come from the `gotags` build setting
  (`context.bzl:1021`: `tags = list(ctx.attr.gotags[BuildSettingInfo].value)`),
  i.e. `--@rules_go//go/config:tags`.
- **`GoSDK.goos` / `GoSDK.goarch` are the wrong fields** — documented as "The
  host operating system the SDK was built for" (`providers.rst:341-348`), i.e.
  the *exec* platform, not the target. Using them would reintroduce the same bug
  in a new place.

So under rules_go the adapter can read everything off the interface library's
provider, with **no new rule attributes**:

```python
def go_build_platform(target):
    m = target[GoInfo].mode
    return struct(
        goos = m.goos,
        goarch = m.goarch,
        tags = tuple(m.tags),
        cgo = not m.pure,
    )
```

## Consequences for the design

1. **This belongs in `go_adapter.bzl`**, as the user directed. The layout field
   and the loader's `build.Context` construction stay host-agnostic; only the
   extraction is swappable, and it will differ in a monorepo whose Go rules have
   no `mode` struct.
2. **`cgo = not pure` is an approximation.** rules_go's `pure` means "no cgo",
   but cgo also requires a working cc toolchain. Good enough for constraint
   evaluation; worth a comment rather than silent precision.
3. **It also fixes the cross-compilation smell.** With the platform declared in
   the layout, the constraint filter no longer depends on what the arcc binary
   itself was built for — which is what forces `_arcc` to be `cfg = "target"`
   today and breaks when exec ≠ target. Making the platform data-driven is a
   prerequisite for ever running an exec-configured arcc.
4. **Not addressed:** multi-platform verification (Q6a explicitly deferred). One
   layout still describes one platform; authority in a `_windows.go` file stays
   invisible on a Linux check.
