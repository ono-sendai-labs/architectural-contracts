# Research — expanding a `members` glob in Starlark

**Question.** The source note sketches `members = ["//path/to/component/..."]`.
Can a Bazel rule or macro expand a package wildcard into concrete labels?

**Method.** Empirical, in a throwaway Bazel 9.2.0 workspace with packages
`comp`, `comp/a`, `comp/b/c`, `other`.

## Finding 1 — target-pattern wildcards are not valid in rule attributes

`attr.label_list` requires concrete labels. `//path/...` is a *command-line*
target pattern, not a label, so the note's literal sketch cannot be passed to the
rule as written. Expansion must happen before the rule sees it.

## Finding 2 — `native.subpackages()` returns the *nearest* descendant packages only, never past a package boundary

```python
# comp/expand.bzl
def expanded():
    return native.subpackages(include = ["**"], allow_empty = True)
```

Loaded and called at BUILD top level, this printed:

```
DEBUG: comp/BUILD.bazel:3:6: SUBPACKAGES=["a", "b/c"]
```

**Correction (re-verified 2026-07-26).** The first reading of this — "relative
paths of transitive subpackages" — was wrong, and the original fixture could not
tell the difference: `comp/b` was a plain directory, so `b/c` being returned only
showed that depth is not the limit. Re-run with `comp/a/deep` also a package:

```
include=["**"]     -> ["a", "b/c"]      # a/deep absent
include=["a/**"]   -> ["a"]
include=["**/**"]  -> ["a", "b/c"]
include=["**/*"]   -> ["a", "b/c"]
include=["a/deep"] -> []                # decisive: not addressable at all
include=["*"]      -> ["a"]
include=["*/*"]    -> ["b/c"]
```

`//comp/a/deep` exists and is a package, and **no include pattern can reach it**.
`subpackages()` walks directories but stops descending the moment it finds a
package, so it yields the *frontier* of nearest descendant packages, not the
transitive set. This is the same boundary rule that stops `glob()`: a package may
not see into another package. Recursion is only possible by having each
intermediate package expand its own frontier.

Note it yields *packages*, not targets — mapping a package to the label of the Go
library inside it needs a naming convention.

### bazel-skylib wraps it but does not lift the restriction

`@bazel_skylib//lib:subpackages.bzl` (the wrapper
[bazel.build recommends](https://bazel.build/rules/lib/toplevel/native#subpackages))
is a thin shim: `subpackages.all()` calls `native.subpackages(include = ["**"])`
and optionally maps the results through `"//%s/%s" % (native.package_name(), s)`.
Verified in the same fixture:

```
SKYLIB=["//comp/a", "//comp/b/c"]        # a/deep absent, same as native
```

Its docstring states the frontier semantics explicitly ("all subpackages, but not
subpackages of subpackages"), and `subpackages.supported()` / the `fail()` on
unsupported Bazel versions are worth having. Two reasons to prefer it:

1. `fully_qualified = True` returns `//comp/a`, which **is** `//comp/a:a` — so
   under the Gazelle basename convention no package→target naming step is needed
   at all. The naming hook (B2) is only required for hosts whose libraries are
   not named after their directory.
2. It is maintained, and the availability check is free.

It needs a direct `bazel_dep(name = "bazel_skylib", ...)` in `MODULE.bazel`; it is
already in the transitive graph via rules_go.

### Why the frontier limitation is not a soundness hole

A nested package missed by the frontier is still *imported* by something in the
closure, so it lands in the aspect closure and is classified: not covered, not a
member, not absorbed ⇒ `UNDECLARED_DEPENDENCY`. The build fails and names the
package to add. The residual gap is a nested package that **nothing** in the
closure imports — dead code — which stays silently unowned. That is a documented
limitation, not a fail-open on analyzed code.

## Finding 3 (decisive) — it is rejected inside a symbolic macro

The Bazel design calls `go_component` a **symbolic macro**. Calling
`native.subpackages()` from a symbolic macro implementation fails:

```
Error in subpackages: subpackages() can only be used while evaluating a
BUILD file or a legacy macro
```

So expansion **cannot** live inside `go_component` as currently designed.

## Consequences for the design

1. **Expansion happens at the BUILD call site**, via a helper function loaded and
   called at BUILD top level (legal context), e.g.

   ```python
   go_component(
       name = "component",
       interface = ":iface",
       members = arcc_subpackages(),   # expands here, before the macro runs
   )
   ```

   This fits the Q3a decision exactly — patterns in the authoring surface,
   concrete expansion by the time anything is emitted — and keeps `go_component`
   a symbolic macro with typed attributes.

2. **A package→target naming hook is needed only for non-default target names.**
   `subpackages.all(fully_qualified = True)` gives `["//comp/a", "//comp/b/c"]`,
   and `//comp/a` already resolves to `//comp/a:a` — correct under the Gazelle
   basename convention this repo follows (`go/internal/hostpolicy/BUILD.bazel`
   declares `go_library(name = "hostpolicy")`). A monorepo whose libraries are
   named otherwise (`:go_default_library`, say) needs the mapping, so the hook
   stays a host seam belonging with `go_adapter.bzl` — but it is a no-op here.

3. **`arcc_subpackages()` covers the frontier, not the tree.** Nested BUILD files
   below a subpackage need explicit labels (or their own aggregate target). See
   finding 2 for why a missed package that is actually imported still fails the
   build.

3. **Alternative considered: members as import-path patterns matched against the
   aspect closure**, needing no labels, no expansion and no naming convention.
   Rejected as the primary mechanism because a package that nothing in the
   interface's closure imports would never appear in the closure, so it could not
   be marked a root — and pulling otherwise-unreferenced owned code into the
   analysis is part of the point. Labels pull targets in via the aspect; patterns
   only re-classify what is already there. (Import-path patterns remain the right
   form for the *manifest*, which records the expansion.)

## References

- Verified against Bazel 9.2.0 (`bazel --version` via the local toolchain);
  finding 2 re-verified 2026-07-26 in a fixture with a package nested under a
  package (`comp`, `comp/a`, `comp/a/deep`, `comp/b/c`).
- `native.subpackages(include, exclude, allow_empty)` — Bazel Starlark native
  module: https://bazel.build/rules/lib/toplevel/native#subpackages
- `subpackages.all` / `subpackages.exists` — bazel-skylib:
  https://github.com/bazelbuild/bazel-skylib/blob/main/docs/subpackages_doc.md
