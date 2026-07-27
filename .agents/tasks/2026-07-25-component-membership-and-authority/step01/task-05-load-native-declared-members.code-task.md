# Task: Load native declared members

## Description
Thread manifest membership into native package analysis so non-empty `members` loads exactly the declared packages, including packages outside the component root, while omitted membership retains today's `./...` behavior.

## Background
Declared membership is not real in native mode unless the loader can reach members outside the manifest directory. `goanalysis.LoadPackageFacts` currently always runs `packages.Load("./...")` from the component root outside layout mode. The application parses the manifest before loading facts, so it must pass validated membership through the loader seam. Declared-style interface packages remain implicit members and must be loaded even when omitted from the explicit list. Layout mode continues to use its explicit roots.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 M1–M5, §4.5, §7.1 fixture 3, and §10)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 1)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/members-glob-expansion.md` (manifest pattern intent and why declared-style members arrive as literals)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Evolve the application package-loader seam so the already parsed manifest's membership reaches `goanalysis` without introducing hidden process-global state.
2. In native mode with non-empty members, invoke `packages.Load` for the validated literal member import paths rather than `./...`.
3. Ensure the package containing each declared interface file is also loaded and represented as a member even though it need not appear in `members`.
4. Allow explicit members anywhere resolvable from the component's Go module/workspace, including sibling packages outside the component root.
5. With empty members, preserve the current `Dir = componentRoot`, `patterns = ["./..."]`, facts, call graph, source-file tracking, and error behavior.
6. Keep layout mode rooted in `Layout.Roots`; do not reinterpret native manifest patterns as layout roots in this step.
7. Filter emitted package facts, source membership, call-graph roots, and analyzer package requests to the effective member set while retaining transitive packages for type checking and call-graph construction.
8. Canonicalize membership comparisons consistently with loaded package paths and existing host policy.
9. Produce contextual errors when a declared literal cannot be loaded or resolves to no source package.
10. Update application dependency-injection tests and loader test doubles for the new request shape.
11. Add an integration test with a manifest under one component directory naming a package outside that root; prove that package is loaded and analyzed.

## Dependencies
- Task 2: Parsed, validated native members and interface style.
- Task 3: Checker semantics for the effective member set.

## Implementation Approach
1. Introduce a small load request containing component root, members, and interface-file context, and adapt `app.PackageLoader`, production wiring, and test fakes.
2. Select native patterns from members only when the list is non-empty; otherwise retain the current FR1 branch verbatim.
3. Resolve implicit interface packages using Go package/file information rather than assuming every interface lives at the component-root import path.
4. Keep the full `packages.Load` graph for SSA/VTA, but centralize the effective-member predicate used when extracting facts, sources, and caller roots.
5. Build a temporary multi-package module fixture that places a declared member outside the manifest directory and gives it an observable import or authority fact.

## Acceptance Criteria

1. **Outside-root member loads**
   - Given a native manifest whose explicit member is a resolvable package outside the component root
   - When `arcc check` loads package facts
   - Then that package is included as component-owned code and its imports, symbols, calls, and authority are analyzed.

2. **Declared loading is exact**
   - Given non-empty literal members and other packages beneath the component root
   - When native loading runs
   - Then only the explicit members plus implicit interface package contribute component facts and analysis roots; unrelated root-subtree packages do not.

3. **Implicit interface package is retained**
   - Given members that omit the package containing `interface_files`
   - When native loading runs
   - Then the interface package is still loaded once and checked as a member.

4. **FR1 remains bit-for-bit compatible**
   - Given any existing manifest with no members
   - When native loading and checking run
   - Then loading still uses `./...`, produces the same facts/findings, and all eight self-manifests plus both examples pass unchanged.

5. **Invalid declared loads are actionable**
   - Given a declared member that cannot be resolved or has no source package
   - When loading runs
   - Then arcc exits as a tool error with the offending member named.

6. **Step 1 native demo works**
   - Given an existing component extended with an outside-root `members` list
   - When `arcc check` runs and the list is then removed
   - Then the first run analyzes the outside-root package and the second reproduces today's FR1 behavior.

7. **Repository checks remain green**
   - Given the completed Step 1 native loading path
   - When `just ci` runs
   - Then unit, integration, self-check, generation-cleanliness, and Bazel checks all pass.

## Metadata
- **Complexity**: High
- **Labels**: Go, goanalysis, native-mode, membership, integration
- **Required Skills**: Go, `go/packages`, SSA/VTA analysis, dependency injection, integration testing
