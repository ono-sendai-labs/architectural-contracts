# Task: Resolve a package-surface dependency's interface

## Description
Give `goanalysis.ResolveDependencyInterface` a `PACKAGE_SURFACE` branch whose
symbol set is every exported symbol of every member package, resolve a
pattern-membership dependency's surface from the *depender's* own layout, carry
the dependency's interface style and certification self-declarations onto
`facts.DependencyInterface`, and wire package-granularity prune keys through the
application.

## Background
`ResolveDependencyInterface` today loads every package under a dependency's root
and derives the FR4 symbol set from the files the dependency's manifest declares
as `interface_files`. A `PACKAGE_SURFACE` component (A6) declares none — its
surface is the whole exported API of its members — so that derivation yields an
empty set, which would make every call into the dependency a
`CALLS_UNDECLARED_INTERFACE` violation and leave the boundary unprunable.

The honest reading of "the whole surface is the interface" is that the symbol set
is every exported symbol of every member. That is what keeps `UNUSED_DEPENDENCY`
meaningful for a wrapped library — a member's `logger.Info(...)` resolves to a
used symbol — while making `CALLS_UNDECLARED_INTERFACE` vacuous, which Task 3
then acts on.

**Pattern membership (M8) is the narrow sub-case.** A `PACKAGE_SURFACE` component
may declare unexpanded import-path patterns rather than labels, for packages that
cannot be named as targets at all (a visibility-gated injected runtime). There is
then no dependency layout to load from, so the surface must resolve from the
depender's own layout, whose closure contains those packages by construction —
that being *why* the depender needs the boundary. Narrower than it sounds: prune
keys need only package paths, and an `auto_attached` edge is exempt from
`UNUSED_DEPENDENCY`, so local symbol extraction is needed only for pattern
membership *without* auto-attachment — an author-declared wrapper whose packages
cannot be named as targets.

The certification fields (`own_check_runs`, `certification_reference`) ride along
here because this is the one place that reads a dependency's manifest. They are
self-declarations at the same trust level as `declared_authority` — reviewable,
not proven — and Task 4 renders them.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A6/A7/A8/M8, §4.1, §4.5 — the `ResolveDependencyInterface` branch and the pattern-membership paragraph, §4.6, §5.2, §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 9)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/capability-analysis-mechanics.md` (§2 — why prune keys need package paths only)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend `facts.DependencyInterface` with the dependency's declared properties,
   as pure data with doc comments stating each is read from the dependency's own
   manifest and is **not** verified by arcc:
   - the interface style (package-surface or the declared default);
   - `OwnCheckRuns bool` and `CertificationReference string` (A8).
   `facts` gains no logic.
2. In `ResolveDependencyInterface`, branch on the parsed dependency manifest's
   interface style. The default style is unchanged, byte for byte, including its
   `interface_files` derivation and all existing error paths.
3. Under `PACKAGE_SURFACE`, the symbol set is every exported symbol of every
   member package of the dependency, taken from the facts already extracted for
   that dependency — do not re-extract, and do not read `interface_files`, which
   validation guarantees is empty for this style.
4. Emit method symbols in the same both-receiver-form shape the declared-style
   path emits, so `NormalizeInterfaceSymbol` comparisons in the checker behave
   identically for both styles.
5. `Packages` continues to carry the dependency's package paths, canonicalized.
   These are what the prune keys are built from, so they must be present even
   when the symbol set is empty.
6. **Pattern membership.** When a `PACKAGE_SURFACE` dependency's `members` are
   patterns rather than literal paths, resolve them against the *depender's*
   loaded closure instead of loading the dependency's own root:
   - match patterns with the same `path.Match` glob semantics the checker and
     the loader already use for absorbed declarations, over canonicalized paths;
   - the matched closure packages become `Packages`;
   - run `extractSymbols` on those packages — including ones that are not
     members of the depender — to build the surface.
   Match the design's framing in code comments: this local extraction exists for
   pattern membership *without* auto-attachment, and is not needed for prune keys.
7. A pattern-membership dependency has no layout and no roots, so the M6
   `roots == members` equality must not be applied to it. Fail closed with a
   message naming the dependency if a pattern-membership dependency is
   encountered where the depender's closure contains none of its packages —
   silently resolving to an empty surface would make the boundary invisible.
8. Keep the T6 canonicalization already applied to the local `factsPkgs`
   (`ImportPath` and `Imports`) applied on every new path as well.
9. In `go/cmd/arcc/app/app.go`, build `AnalyzeRequest.PruneAtPackages` from the
   packages of every resolved dependency whose style is `PACKAGE_SURFACE`,
   deduplicated and sorted. Do **not** move a package-surface dependency's
   symbols out of `PruneAt`: symbol keys still resolve first, and the existing
   `func <pkg>.init CAPABILITY_SAFE` keys (A2's surviving half) must keep being
   emitted for every dependency package regardless of style.
10. A declared-style dependency contributes nothing to `PruneAtPackages`, so a
    repository with no package-surface dependencies produces exactly today's
    analyzer request.
11. No checker or report change belongs in this task. `CALLS_UNDECLARED_INTERFACE`
    suppression, the `auto_attached` exemption, and the certified/asserted
    annotation are Tasks 3 and 4; here the data is produced and asserted at the
    `goanalysis` and application level.

## Dependencies
- `task-01-prune-at-package-granularity` supplies `PruneAtPackages` on the port;
  wiring it here without that field would not compile.
- Step 1 supplies `interface_style`, `members`, and the certification fields in
  the schema and in `manifest`, including the validation that a
  `PACKAGE_SURFACE` component has non-empty `members` and empty `interface_files`.
- `task-03-check-package-surface-and-auto-attached-edges` consumes the new
  `DependencyInterface` fields.

## Implementation Approach
1. Add the pure fields to `facts.DependencyInterface` and populate them on the
   existing declared-style path first, asserting no behavior change.
2. Split the symbol-set derivation into a small helper per style, leaving the
   package loading, error handling and canonicalization shared.
3. Implement the label-membership `PACKAGE_SURFACE` branch and its tests before
   the pattern branch; they fail differently and should be diagnosable separately.
4. Add the pattern branch, resolving against the depender's closure, with the
   empty-match failure and its message.
5. Wire `PruneAtPackages` in the application and pin the request shape in
   `app_test.go` alongside the existing `PruneAt` assertions.
6. Run focused `facts`, `goanalysis`, and `app` suites, then `just ci`.

## Acceptance Criteria

1. **A package-surface dependency exposes its whole exported surface**
   - Given a dependency manifest with `interface_style = PACKAGE_SURFACE` and two member packages
   - When its interface is resolved
   - Then the symbol set is every exported symbol of both members, in the same key forms the declared-style path produces, and `interface_files` is not consulted.

2. **Declared-style resolution is unchanged**
   - Given an existing declared-style dependency
   - When its interface is resolved
   - Then the symbol set, package list, and every error path are identical to before this task.

3. **Pattern membership resolves from the depender's layout**
   - Given a package-surface dependency whose `members` are import-path patterns and whose packages appear only in the depender's own closure
   - When its interface is resolved
   - Then the matched closure packages become its `Packages` and their exported symbols its surface, without attempting to load a dependency layout.

4. **A pattern that matches nothing fails closed**
   - Given a pattern-membership dependency none of whose patterns match any package in the depender's closure
   - When its interface is resolved
   - Then resolution fails with a message naming the dependency, rather than yielding an empty surface.

5. **Certification self-declarations are carried, not judged**
   - Given dependencies declaring `own_check_runs` true, false with a `certification_reference`, and false without one
   - When each interface is resolved
   - Then the values reach `facts.DependencyInterface` verbatim and no finding, error, or warning is produced from them.

6. **Package prune keys are wired**
   - Given a component depending on one package-surface and one declared-style dependency
   - When `arcc check` builds its analyze request
   - Then `PruneAtPackages` holds exactly the package-surface dependency's packages, sorted and deduplicated, and `PruneAt` still holds every dependency's interface symbols and `func <pkg>.init` keys.

7. **Nothing changes without a package-surface dependency**
   - Given a component whose dependencies are all declared-style
   - When `arcc check` runs
   - Then `PruneAtPackages` is empty and the analyze request is identical to today's.

8. **Canonicalization holds on every path**
   - Given a host policy that rewrites import paths
   - When a package-surface or pattern-membership dependency is resolved
   - Then its package paths, imports, and symbols are canonical, matching the declared-style path's behavior.

9. **Repository checks remain green**
   - Given the new resolution branches and application wiring
   - When `just ci` runs
   - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass with no rendered-report changes.

## Metadata
- **Complexity**: High
- **Labels**: Go, goanalysis, facts, dependency-resolution, package-surface, pruning
- **Required Skills**: Go, `go/packages`, go-types symbol keys, layout-mode/native-mode duality, deterministic fact production
