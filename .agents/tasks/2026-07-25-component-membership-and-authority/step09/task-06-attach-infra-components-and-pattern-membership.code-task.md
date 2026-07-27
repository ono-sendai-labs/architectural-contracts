# Task: Attach infra components and author pattern membership in Bazel

## Description
Make the component rule attach `INFRA_COMPONENTS` entries as `auto_attached`
component dependencies per `go_attach_infra`, emit those edges into the manifest
**even when attachment is unconditional**, treat the attached component's
packages as covered during classification, and let a `PACKAGE_SURFACE` component
declare unexpanded import-path patterns as membership (M8) for packages that
cannot be named as targets.

## Background
Step 6 added the two host seams — `go_attach_infra(target, infra)` and the
`INFRA_COMPONENTS` registry — and their conformance documentation, but nothing
consumes them. This task is the consumer.

Two properties of the design have to survive implementation, because the obvious
shortcut destroys each:

- **Unconditional attachment is conforming.** A host whose analysis-phase graph
  cannot see toolchain-injected packages cannot evaluate a closure predicate at
  all, so `go_attach_infra` returning `True` always is a legitimate host
  implementation: pruning at a package is a no-op unless that package is reached,
  and the component's own authority is charged regardless because its members are
  roots.
- **The injected edge appears in the manifest anyway.** The whole argument for
  `auto_attached` over a hidden allowlist is that a reader can see which
  boundaries were injected. An emitter that skips the entry because "it is always
  there" hands the trust list straight back. So the manifest entry is emitted
  whenever the edge attaches, unconditionally included.

**Pattern membership** is the other half. The registry's `import_path_patterns`
field exists because the packages that most need an injected boundary are
disproportionately the visibility-gated ones — they cannot be named as labels at
all, so a label-only `members` attribute cannot describe them. Such a component
declares its membership as unexpanded patterns; M4's reviewability rationale does
not apply because that membership only derives prune keys and an exported-surface
set. It also has no layout and no roots, so M6's `roots == members` equality does
not apply to it.

**Pattern idiom.** `members` and `absorbed_dependencies` patterns are matched Go-
side with `path.Match` glob semantics. The commented registry examples in
`go_adapter.bzl` currently show Bazel's `//pkg/...` target-pattern spelling, which
`path.Match` does not implement. The two must be reconciled in this task, in the
direction of the manifest idiom — inventing a second pattern language for
Starlark would leave the emitter and the checker disagreeing about what a
component owns.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A6/A7/M8/B1, §3.1, §4.7 — the seams and the registry examples, §4.8, §4.9, §6, §7.3)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 9)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` (F3 and F5 — why a closure predicate may be unevaluable, and why an empty registry is a placeholder rather than a steady state)
- `.agents/planning/2026-07-25-component-membership-and-authority/research/members-glob-expansion.md` (why membership is authored, not expanded by a helper)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Read `INFRA_COMPONENTS` and call `go_attach_infra` only through
   `go_adapter.bzl`. No rules_go provider access and no registry knowledge moves
   into `component.bzl`.
2. For each registry entry, evaluate attachment against the component's Go root
   targets (interface and members) deterministically, and attach when the seam
   says to. An empty registry must leave every existing component byte-identical.
3. An attached entry becomes a `component_dependencies` entry in the emitted
   manifest with `auto_attached: true`, its `manifest` path relativized exactly
   as authored `component_deps` are. Emit it whenever it attaches, including when
   the host's predicate is unconditional.
4. Never attach a component to itself, and never emit a duplicate edge when an
   author already declared the same component in `component_deps`; the authored
   edge wins and stays unmarked.
5. Pull an attached component's manifest, layout, and transitive artifacts into
   runfiles and into `transitive_manifests` / `transitive_layouts` on the same
   footing as authored `component_deps`, so the generated `.check` can resolve it.
6. Attached packages are **covered** in classification (design §3.1), ahead of
   member and absorbed, so an injected runtime package in the closure no longer
   lands in the unclassified bucket and no longer surfaces as
   `UNDECLARED_DEPENDENCY`. Coverage comes from the attached component's provider
   closure where its packages are visible, and from the entry's
   `import_path_patterns` where they are not.
7. Implement pattern matching with the **same semantics** as the Go-side
   `path.Match` matching the checker and loader apply, and pin the agreement with
   a shared case table exercised on both sides. If full parity is impractical in
   Starlark, restrict the registry to the pattern forms that can be matched
   faithfully and `fail()` on any other form — failing closed, never approximating.
8. Update the commented registry examples in `go_adapter.bzl` to the manifest
   pattern idiom, keeping the two documented shapes (an addressable component
   label, and a component label plus patterns for visibility-gated packages).
   The registry stays `[]` upstream.
9. Give a `PACKAGE_SURFACE` component a way to author unexpanded import-path
   patterns as membership, emitted verbatim into the manifest's `members`. Under
   the declared style it must `fail()` naming the component, since M4 requires
   literal paths there.
10. A component whose membership is entirely patterns has no roots to compare, so
    it must not emit a layout `roots` set that would trip M6's equality, and the
    macro's existing shape rule must accept pattern membership as satisfying
    "`members` is mandatory under `PACKAGE_SURFACE`".
11. Keep the emitted manifest and layout deterministic: sort attached edges,
    patterns, and covered paths, so output does not depend on registry or label
    ordering.
12. Factor the attachment logic so an analysis test can supply its own registry
    and predicate without editing `go_adapter.bzl` — e.g. a private, undocumented
    rule attribute defaulting to the adapter's values. It must not become a public
    authoring API, and the default path must be the adapter's.
13. Update the Bazel goldens for the new manifest entries and any layout change.
    Do not normalize `auto_attached` away — its visibility is the requirement.

## Dependencies
- Step 6 supplies `go_attach_infra`, `INFRA_COMPONENTS`, `go_build_platform`, the
  `interface_style` attribute, and the shape rule this extends.
- Step 7 supplies the covered → member → absorbed → unclassified classification
  order that attachment feeds into.
- `task-02-resolve-package-surface-dependency-surface` resolves a pattern-
  membership dependency's surface from the depender's layout; without it a
  pattern-membership manifest cannot be checked.
- `task-03-check-package-surface-and-auto-attached-edges` makes an `auto_attached`
  edge exempt from `UNUSED_DEPENDENCY`; without it, every injected edge would
  warn.

## Implementation Approach
1. Add the attachment pass in `_go_component_impl` before classification, producing
   a sorted list of attached entries with their component info and patterns.
2. Extend `_classify` and the covered map to consult attached closures and
   patterns, keeping coverage precedence unchanged.
3. Extend `_manifest_content` to emit attached edges with `auto_attached: true`,
   and to emit pattern members verbatim.
4. Add the pattern matcher with its shared case table, and reconcile the adapter's
   commented examples.
5. Add analysis tests using a test registry: attachment occurring, attachment
   declined by a closure-inspecting predicate, unconditional attachment still
   emitting the manifest entry, an authored duplicate not double-emitting, and the
   declared-style pattern rejection.
6. Add an end-to-end fixture where a component reaching authority only through an
   injected package-surface component declares nothing and its `.check` passes,
   while the infra component's own check still reports that authority.
7. Regenerate the affected goldens deliberately, then run the Bazel suite and
   `just ci`.

## Acceptance Criteria

1. **An empty registry changes nothing**
   - Given the upstream `INFRA_COMPONENTS = []`
   - When every existing component is built
   - Then its manifest, layout, and check results are byte-identical to before this task.

2. **An attached edge is marked and emitted**
   - Given a registry entry whose predicate attaches
   - When the component's manifest is generated
   - Then it contains a `component_dependencies` entry for that component with `auto_attached: true` and a resolvable relative manifest path.

3. **Unconditional attachment still emits the entry**
   - Given a predicate that returns True without inspecting the closure
   - When the manifest is generated
   - Then the entry is present — the legibility `auto_attached` was chosen for is preserved.

4. **A declining predicate attaches nothing**
   - Given a test predicate that attaches only when the closure contains the runtime's packages, and a component whose closure does not
   - When the component is built
   - Then no edge is emitted and no runfiles change.

5. **Authored edges win over injected ones**
   - Given an author who already declares the infra component in `component_deps`
   - When the manifest is generated
   - Then exactly one entry appears for it, unmarked, and analysis does not fail.

6. **Injected packages are covered, not undeclared**
   - Given a component whose closure contains the injected runtime's packages
   - When its generated `.check` runs
   - Then those packages produce no `UNDECLARED_DEPENDENCY`, and the component declares no authority for what lies behind the boundary.

7. **The infra component's own check still reports its authority**
   - Given the attached component's own check target
   - When it runs
   - Then it reports the authority its members actually use — the property that distinguishes this from a bare trust list.

8. **An injected edge is never unused**
   - Given a component that attaches the runtime but never reaches its packages
   - When its check runs
   - Then no `UNUSED_DEPENDENCY` warning is produced for the injected edge.

9. **Pattern membership is authorable and emitted verbatim**
   - Given a `PACKAGE_SURFACE` component declaring import-path patterns as membership
   - When its manifest is generated
   - Then the patterns appear unexpanded in `members`, no conflicting `roots` set is emitted, and the manifest parses.

10. **Patterns are rejected under the declared style**
    - Given a declared-style component declaring pattern membership
    - When Bazel analyzes it
    - Then analysis fails naming the component, since M4 requires literal paths there.

11. **Emitter and checker agree on pattern semantics**
    - Given the shared pattern case table
    - When it is exercised on the Starlark matcher and the Go matcher
    - Then both produce the same verdict for every case, or the Starlark side fails closed on forms it cannot match.

12. **Output is deterministic**
    - Given registry entries and member labels supplied in varying order
    - When the component is built repeatedly
    - Then the manifest and layout are byte-identical.

13. **Repository checks remain green**
    - Given attachment and pattern membership
    - When `just ci` runs
    - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass with only intentional golden changes.

## Metadata
- **Complexity**: High
- **Labels**: Bazel, Starlark, host-seam, infra-components, auto-attached, pattern-membership, goldens
- **Required Skills**: Starlark, Bazel aspects/providers/runfiles, analysis tests, deterministic emission, cross-language semantic parity
