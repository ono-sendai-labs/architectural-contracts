# Component membership and authority attribution at boundaries

**Date:** 2026-07-24
**Status:** open design questions — not implemented. Written to scope a related
group of enhancements.

> Two coupled gaps in how a component defines *what it is responsible for* and how
> arcc *attributes ambient authority* to it:
>
> 1. **Callback escape** — a component can absorb code that exercises authority,
>    expose it only as a function value handed to a dependency, and pass while
>    declaring nothing.
> 2. **Membership by absorbed closure** — `absorbed_deps` pulls in each label's
>    *entire transitive tree* (pruned only by `component_deps`), so a component's
>    real membership is implicit and hard to reason about.
>
> They share a fix shape: define a component's members explicitly and analyze them
> as roots. The first section is the soundness gap; the second is the membership
> model that also closes it for a component's own code.

## Part 1 — Authority that escapes through a callback

### The model, briefly

arcc attributes ambient authority to a component by **reachability through the
call graph from the component's analysis roots**. The roots are the component's
*member* packages — its interface and any package that is neither covered by a
`component_dep` nor absorbed. Capability analysis loads that root set, builds the
SSA call graph, and charges the component with every capability reachable from a
root.

Two properties of "reachable from a root" matter here:

1. **Package `init` is always reachable.** Importing a package runs its `init`,
   so authority exercised at init time (directly or transitively) is always
   attributed. A component that transitively *imports* a file-touching logger at
   init is correctly flagged.
2. **A regular function is reachable only if it is called** from a root's call
   graph. A function whose *value* is taken and handed to someone else — but
   never called within the component's own reachable code — is not analyzed.

Absorbed packages are deliberately **not** roots. If they were, their `init`
functions (which live in non-interface files) would trip the well-formedness rule
that a component's explicit `init`s must be declared in interface files. So
absorbed code is analyzed only insofar as it is reachable from the interface.

The gap is the intersection: **absorbed code, reachable only as a function value
invoked by a dependency, is analyzed by no one.** The component doesn't call it
(so its own analysis never enters it); the dependency that *does* call it is a
pruned boundary (a dependency certifies its own authority, not its callers'
callbacks).

### Minimal synthetic example

Three packages. `editor` is a component: its interface is package `editor`, it
absorbs `backend` as an implementation detail, and it depends on the component
`host` (a framework that drives editors).

```go
// package backend — absorbed by the editor component.
package backend

import "os"

// Load reads a file: it exercises the FILES capability.
func Load(path string) ([]byte, error) {
	return os.ReadFile(path)
}
```

```go
// package editor — the component's interface (its only member/root).
package editor

import (
	"example/backend" // absorbed
	"example/host"    // a component_dep (boundary)
)

// editor hands backend.Load to the host framework as a callback.
// It never calls backend.Load itself.
var Handler = host.Register(host.Handlers{Open: backend.Load})
```

```go
// package host — a separate component. At runtime it invokes the callback.
package host

type Handlers struct{ Open func(string) ([]byte, error) }

func Register(h Handlers) Handlers { /* stores h; later calls h.Open(...) */ return h }
```

Component definition: interface `editor`; absorbed `backend`; `component_deps`
`host`; **`declared_authority` none**.

**arcc verdict: PASS.** Yet at runtime `host` calls `Open` → `backend.Load` →
`os.ReadFile`. The FILES capability is exercised through the editor's own absorbed
code, on the editor's behalf, and the editor declares nothing.

Why it's missed: attribution traces the call graph from `editor` (the sole root).
`backend.Load`'s body is reachable only as a value flowing into `host.Register`
and invoked inside `host`, a pruned boundary. `editor.init` (and `backend.init`)
run, so *init-time* authority would be caught — but `backend.Load` is an ordinary
function invoked only across the boundary, so its `os.ReadFile` is never traced.
Not even a higher-order-boundary warning fires: the func value is stored in a
struct literal, not passed as a direct argument to a declared dependency symbol
call.

(Distilled from a real case: a language component absorbs a `syntax` package whose
`ParseFile` reads a file; it assigns `ParseFile` into a `Language{}` struct handed
to a shared editor-framework component, which calls it at runtime. Same shape.)

### Why this is a fail-open worth closing

Handing a callback to a framework is the *normal* way these components are used —
it is precisely the behavior the component exists to provide. Morally the editor
*does* cause the file read to happen; the model excuses it only because the call
edge completes inside a dependency.

### Options

**A. Analyze the whole owned closure (members + absorbed) as roots.** Sound; the
example flips to a FILES violation. But over-approximates: absorbed libraries
carry functions the component never uses, and analyzing them charges the component
with authority it never exercises. Requires separating "responsible for" (members
+ absorbed) from "must live in interface files" (interface). See Part 2 for a
scoped form of this that avoids the over-approximation.

**B. Add functions whose *value* is reachable from the interface to the roots.**
More precise — only functions the component actually exposes are analyzed. But
hard to make sound and prone to bypass: it requires tracking function values
through structs, slices/maps, returns, closures, interface methods, reflection.
Any flow the analysis misses is a silent bypass.

**C. Warn, don't attribute.** Broaden higher-order-boundary detection so passing a
func value from owned code across a boundary — via struct literals, assignments,
returns — emits a warning. Cheap and surfaces the risk, but still fail-open and
noisy.

**D. Document the limitation (status quo).** Define responsibility as "authority
reachable through the interface's call graph"; callbacks are attributed at the
point of use. No code change, but leaves the example passing.

### The underlying question

What is a component responsible for? **Reachability** (D): authority its interface
can cause to execute directly. **Ownership** (A): all code it absorbs.
**Exposure** (B/C): authority in code it executes *or hands out to be executed*.
The example argues against pure reachability — the component's reason for existing
is to hand out that callback.

## Part 2 — Defining members by location, not by absorbed closure

### Current behavior

Today a component's membership is derived: everything in the interface's package
closure is bucketed into covered (a `component_dep`'s closure), absorbed, or
member. `absorbed_deps` takes **labels**, and for each it absorbs that label's
**entire transitive package tree**, pruned only by whatever `component_deps`
happen to cover. So the effective absorbed set is implicit, and it shifts as
`component_deps` are edited.

This drifts from the model authors reach for. `absorbed_deps` reads like "these
packages are my implementation," but it means "these labels **and their whole
transitive trees**, minus `component_deps`."

### The reasoning hazard, illustrated

A component author writes four `absorbed_deps` — the four packages that really are
the component's implementation:

```
absorbed_deps = [ impl/parse, impl/parse:tokens, impl/parse/grammar, impl/ui ]
```

The generated manifest, however, lists **seven** absorbed packages — the four
above plus three utility packages (`util/textspan`, `util/bitset`, `util/sizeof`)
pulled in transitively through `impl/parse`, because they happened not to be
covered by a `component_dep`. Those utilities are not the component's
implementation; they belong to their own components. They were silently swept into
this component's responsibility (and its authority attribution) by transitive
absorption. Nothing in the BUILD file says so.

### Proposal: an explicit `members` attribute

Let a component declare its members by **location**, not by absorbed closure:

```python
members = ["//path/to/component/..."]   # the subtree; a glob, like visibility
```

Semantics:

- The matched packages (minus the interface, which is declared separately and
  stays a member) become the component's **members** — and its analysis roots —
  **without** pulling in their transitive dependencies.
- Every edge from a member to a non-member package must resolve to another
  member, a `component_dep`, an `absorbed_deps` entry, or the standard library.
  Anything else is an error / `UNDECLARED_DEPENDENCY`. That is the point: the
  closure becomes **explicit** instead of "absorb everything, prune it back."
- `absorbed_deps` reverts to a narrow escape hatch: code **outside** the
  component's subtree that it takes responsibility for (vendored/helper libraries).
  Its transitive-closure behavior is at least defensible there ("I am vendoring
  this whole library"), and is rarely needed once `members` carries a component's
  own code.

This restores the pre-bazel mental model — a component is "the packages in its
tree, minus the interface files" — and makes membership something you can read off
the BUILD file.

### Why this also closes the Part 1 gap for owned code

The callback gap exists because a component's own implementation packages are
*absorbed* (non-roots) rather than *members* (roots). `members` makes them roots.
In the Part 1 example, `backend` would be a member, so `backend.Load` is analyzed
regardless of who calls it, and its `os.ReadFile` is attributed.

So `members` is the **scoped form of Option A**: it makes *declared* members roots
rather than the entire transitive closure of absorbed labels, which is what caused
Option A's over-approximation. Owned code is analyzed in full; external absorbed
code (the narrow `absorbed_deps` remnant) keeps the Part 1 trade-off, where Option
B/C could still apply later.

### Design decisions to nail

- **Well-formedness rule scoping (the crux, shared with Part 1).** The
  explicit-`init`-in-interface-file and exported-method-in-interface-file rules
  must be scoped to the **interface package(s)**, not all members — otherwise a
  member implementation package with an ordinary `func init()` would be flagged.
  Today absorbed packages dodge these rules by not being members; `members` forces
  the split between *responsible for* (all members, for authority) and *must live
  in interface files* (the interface, for well-formedness) to be made explicit.

- **The remainder becomes an error, not silent absorption.** Today classification
  has no fall-through because `absorbed_deps` mops up whatever is left. With
  `members`, a member's dependency that is neither member/covered/absorbed/stdlib
  must surface as a violation. This interacts with **host-injected runtimes** (a
  generated-proto runtime injected by a toolchain, say): those are transitive deps
  a member reaches but cannot name, which strengthens the case for treating such a
  runtime as trusted infrastructure (like stdlib) rather than requiring it to be
  declared.

- **Glob semantics.** A pattern like `//component/...` or `:__subpackages__`
  should exclude the interface package from the match (it is declared separately
  though it remains a member), and error if a matched package is also a
  `component_dep` root (overlap) or listed in `absorbed_deps` (contradiction).

- **`absorbed_deps` transitivity.** Decide whether the escape-hatch form stays
  transitive (whole vendored library) or also becomes explicit. Transitive is
  reasonable for genuine vendoring; explicit is more legible. This can be revisited
  independently.

- **Layout is unaffected.** The package layout is still built from the full
  dependency closure for type-checking; `members` changes only
  *classification/attribution*, not what is loaded.

### Concrete "after" for the illustrated component

```python
go_component(
    name = "component",
    interface = ":iface",
    members = ["//path/to/component/..."],   # the subtree, minus the interface
    component_deps = [ ...the boundaries it actually depends on... ],
    # absorbed_deps: only for code outside its own tree (often empty)
)
```

The three utility packages that were silently absorbed now surface as undeclared,
nudging the author to add the `component_dep` where they belong — which is exactly
where the responsibility should sit.

## Implementation notes (whichever way each goes)

- Keep the well-formedness rules (explicit-`init`-in-interface-file, exported
  method placement) scoped to the **interface**, or member implementation packages
  with `init`s become impossible. This single split unblocks both Part 1 Option A
  and Part 2.
- Pruning at `component_dep` boundaries is unchanged and orthogonal: making an
  owned package a root analyzes *its own* body; it does not re-descend into covered
  dependencies.
- Regression fixtures: (a) the Part 1 callback example (absorbed callback with
  undeclared FILES handed to a dependency) should become a violation once owned
  code is analyzed as roots; (b) a component whose `members` glob reaches a package
  outside its subtree with no `component_dep`/`absorbed_deps` should report an
  undeclared dependency.
