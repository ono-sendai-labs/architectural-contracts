# Research — Mapping "component" onto Go, and enforcing pillar 1

This note works out how the concept-doc's **component** maps onto Go constructs,
and what tooling supports the pillar-1 checks (interface files, private
implementation, declared dependencies). Pillar 3 (ambient authority) is covered
in `capslock.md`.

## What is a "component" in Go?

The concept doc (§3.1, Appendix A.1) says a component is a collection of code
units exposing a well-defined interface, owning a private implementation, and
declaring its dependencies. Candidate Go granularities:

1. **One Go package = one component.** Simplest. Go visibility (Capitalized =
   exported) already enforces "private implementation" *within* a package for free.
   Interface files = the subset of `.go` files holding the exported API.
2. **A package tree (`pkg/...`) = one component.** Matches "module or a couple of
   packages". Needs a cross-package private-impl rule: subpackages are internal to
   the component and must not be imported from outside it (Go's `internal/`
   convention already does much of this).
3. **A whole Go module = one component.** Coarser; matches "module".

**Recommendation for MVP:** support a component = **a set of Go packages** named by
import-path patterns (e.g. `example.com/csvtool/...` or an explicit list). Start by
exercising the single-package case in examples, but don't hard-code it.

## Pillar-1 checks and the tooling for each

The manifest names: interface files, (implicitly) the private implementation, and
declared dependencies. Conformance = three checks (§3.2):

### (a) "Implementation calls only into declared dependencies"
- **Lightweight (import allowlist, §4.3):** load packages with
  `golang.org/x/tools/go/packages` (`NeedImports|NeedDeps|NeedModule`). For every
  package in the component, its import set must be a subset of:
  `declared-component-deps ∪ absorbed-impl-detail-deps ∪ allowed-stdlib`.
  This is cheap, robust, and is the natural home for the **two kinds of
  dependencies** distinction (component vs absorbed impl-detail — both live on the
  allowlist; only *component* deps additionally point at another manifest).
- **Stronger (call-edge, later):** use Capslock's `CapabilityGraph` /
  `callgraph` to assert there is *no call edge* from the component into a
  dependency **component's** non-interface symbols (Appendix A.1). Deferred.

### (b) "Nothing outside the component calls its private implementation"
- Within a single package: **free** via Go export rules.
- Across a package tree: Go's `internal/` packages enforce it natively; otherwise
  a call-graph / import check that no *external* package imports the component's
  non-interface packages. Deferred beyond the single-package MVP.

### (c) "The interface is exactly what the named interface files expose"
- Parse the component's packages (`go/packages` + `go/ast` + `go/types`). Collect
  every **exported** top-level declaration and the file it is declared in. Rule:
  every exported symbol must live in a file listed as an interface file; an
  exported symbol in a non-interface file is a violation ("undeclared public
  surface"). This is a straightforward AST walk over `Files`/`ast.File.Decls`,
  filtering `ast.IsExported(name)`.
- Interface files are named in the manifest **relative to the component root**.

## Tooling inventory (all in `golang.org/x/tools`, which Capslock already pulls in)

- `go/packages` — load packages with types, syntax, imports, module info. Same
  loader Capslock uses (`analyzer.PackagesLoadModeNeeded`); we can share one load.
- `go/ast`, `go/token`, `go/types` — exported-symbol / interface-file analysis and
  file↔symbol mapping.
- `packages.Package.Imports` (map import-path → `*Package`) and `.Module` — the raw
  material for the dependency allowlist and for telling first-party vs third-party
  (via module path) apart.

**Reuse insight:** a *single* `packages.Load` call can feed both the
ambient-authority check (hand `pkgs` to `analyzer.GetCapabilityInfo`) and the
pillar-1 import/interface checks. One load, two analyses.

## The "two kinds of dependencies" — how it lands in the manifest

From the rough idea:
- **Component dependency** — another architectural component with its own
  manifest, declared interface, and declared authority. The edge is a first-class
  architecture edge; the dependency accounts for its own authority behind its
  contract.
- **Absorbed (impl-detail) dependency** — a third-party or internal package used
  purely as an implementation detail. It has *no* manifest; it is allowed on the
  import allowlist, and (crucially) **its ambient authority is absorbed into, and
  surfaced by, the absorbing component** (Capslock's transitivity does this for
  free — see `capslock.md`).

So the manifest's dependency list likely has two sections (or a `kind` field):
`component` deps (point to another manifest) vs `absorbed` deps (just an
allowlisted import path). This is the cleanest place to represent the distinction.

## Bazel angle (deferred, but shapes the schema)

The rough idea wants a later `go_architectural_component` Bazel rule wrapping
`go_library`, emitting the manifest's dependency list from `deps`/`srcs`. To keep
that path open, the manifest's dependency + interface-file fields should be
expressible as plain lists that a rule can populate mechanically — i.e. avoid
schema features that only a human could author for those two fields. (Authority
and contract stay hand-authored.)
