# Research — expanding a `members` glob in Starlark

**Question.** The source note sketches `members = ["//path/to/component/..."]`.
Can a Bazel rule or macro expand a package wildcard into concrete labels?

**Method.** Empirical, in a throwaway Bazel 9.2.0 workspace with packages
`comp`, `comp/a`, `comp/b/c`, `other`.

## Finding 1 — target-pattern wildcards are not valid in rule attributes

`attr.label_list` requires concrete labels. `//path/...` is a *command-line*
target pattern, not a label, so the note's literal sketch cannot be passed to the
rule as written. Expansion must happen before the rule sees it.

## Finding 2 — `native.subpackages()` works, and returns packages (not targets)

```python
# comp/expand.bzl
def expanded():
    return native.subpackages(include = ["**"], allow_empty = True)
```

Loaded and called at BUILD top level, this printed:

```
DEBUG: comp/BUILD.bazel:3:6: SUBPACKAGES=["a", "b/c"]
```

So it returns **relative paths of transitive subpackages**, excluding the current
package and anything outside it (`other` did not appear). Note it yields
*packages*, not targets — mapping a package to the label of the Go library inside
it needs a naming convention.

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

2. **A package→target naming hook is required.** `subpackages()` gives
   `["a", "b/c"]`; the helper must produce `//comp/a:a`, `//comp/b/c:c` or
   whatever the host's convention is. This repo follows the Gazelle basename
   convention (`go/internal/hostpolicy/BUILD.bazel` declares
   `go_library(name = "hostpolicy")`), but a monorepo will differ. **This is
   another host seam**, and it belongs with `go_adapter.bzl`.

3. **Alternative considered: members as import-path patterns matched against the
   aspect closure**, needing no labels, no expansion and no naming convention.
   Rejected as the primary mechanism because a package that nothing in the
   interface's closure imports would never appear in the closure, so it could not
   be marked a root — and pulling otherwise-unreferenced owned code into the
   analysis is part of the point. Labels pull targets in via the aspect; patterns
   only re-classify what is already there. (Import-path patterns remain the right
   form for the *manifest*, which records the expansion.)

## References

- Verified against Bazel 9.2.0 (`bazel --version` via the local toolchain).
- `native.subpackages(include, exclude, allow_empty)` — Bazel Starlark native
  module.
