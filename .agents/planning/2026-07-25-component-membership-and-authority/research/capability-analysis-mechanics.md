# Research — Capslock attribution and SSA function-value detection

Source read: `/home/xtof/git/external/capslock` (the Capslock the adapter uses)
and `golang.org/x/tools@v0.48.0/go/ssa`.

## 1. How attribution actually works (confirms Q5's framing)

`analyzer.forEachPath` (`analyzer/analyzer.go:714-780`) does **not** traverse
forward from entry points. It:

1. collects the nodes that *carry* a capability (e.g. `os.Open`),
2. reports any such node whose package is in `queriedPackages` (lines 740-751),
3. then does a **backwards BFS over incoming call edges** (`v.In`, line 758),
   reporting every function reached whose package is in `queriedPackages`.

Consequences, all load-bearing for this design:

- **The analyzed-package set is the entire criterion.** A function in a queried
  package that reaches a capability is charged, whether or not anything calls it.
  There is no notion of "reachable from the interface".
- **Anonymous closures are covered.** Verified by running the `funcvalue` check:
  evidence is `example.com/aspect/funcvalue.init$1 → os.Open`.
- **Calling *into* an unqueried package still attributes.** The BFS walks through
  the absorbed function to its member caller, which is queried and therefore
  reported. So only taking the **value** of an absorbed function escapes.
- `Classifier.IncludeCall` (line 201) can suppress individual edges; arcc does
  not currently use this.

## 2. Prune granularity — Capslock supports package-level keys

`interesting.parseCapabilityMap` (`interesting/interesting.go:49-135`) accepts
four keywords: `func`, `package`, `ignore_edge`, `unanalyzed`. arcc currently
emits only `func` keys (`capslockadapter.buildClassifier`).

`FunctionCategory` (line 214) resolves in this order:

1. cgo suffix match,
2. `functionCategory[name]` — a per-function key,
3. `unanalyzedCategory[name]`,
4. **`packageCategory[pkg]`** — a package-level fallback.

So `package <import/path> CAPABILITY_SAFE` marks **every function in that
package** safe, with per-function keys taking precedence.

**Design consequence.** A `PACKAGE_SURFACE` component (Q9/Q10a/Q17) can be pruned with
one classifier line per package rather than by deriving per-symbol keys — and it
is *more* complete than symbol pruning, which cannot name unexported entry
points. This makes "relaxed well-formedness, no interface files" mechanically
straightforward rather than a special case bolted onto symbol pruning.

Note this is genuinely coarser than the `component_dep` prune, which
deliberately prunes only at *declared* interface symbols so that authority
reached through undeclared entry points still surfaces. Accepting package
granularity is exactly the relaxation Q10a chose, and it should be stated as
such.

## 3. Detecting "a member references an absorbed function as a value"

The Q5 rule needs: a `*ssa.Function` belonging to an absorbed package, appearing
as an operand somewhere in member code, in a position that is **not** the static
callee of a call.

API facts (`x/tools@v0.48.0/go/ssa/ssa.go`):

- `Instruction.Operands()` enumerates all value operands (line 215).
- `Call.Operands` delegates to `CallCommon.Operands`, which **includes
  `&c.Value`** — the callee — so a plain static call `absorbed.F(x)` does surface
  the `*ssa.Function` as an operand. The detector must therefore skip the callee
  position of a call in "call" mode (`!CallCommon.IsInvoke()`, whose callee is
  `CallCommon.StaticCallee()`, line 1493).
- `MakeClosure.Operands` yields `&v.Fn` plus bindings (line 1883); `Fn` is the
  anonymous function being closed over.
- Function values stored into globals, struct fields, slices or maps all appear
  as ordinary operands of `Store`/`FieldAddr`-fed instructions, so a generic
  operand scan covers the note's Part 1 shape (`host.Handlers{Open: backend.Load}`)
  without any dedicated dataflow.

Caveats to handle in implementation:

- **Synthetic wrappers have no package.** `Function.Pkg` is documented as nil for
  shared functions and synthetic wrappers, which includes bound-method thunks
  (`x.M` taken as a value). Resolve the defining package via the function's
  `Object()` (or `Origin()` for instantiations) rather than `Pkg` alone.
- **Local indirection is a false positive.** `f := absorbed.F; f(x)` takes the
  value *and* calls it, and VTA resolves the indirect call, so authority is
  attributed and a warning would be noise. Suppress when the call graph already
  contains an edge into that function from a member package.

**Status: API-verified by reading the source, not yet exercised empirically.** A
fixture matching the note's Part 1 example (absorbed callback with undeclared
FILES handed to a dependency) is the natural first test.
