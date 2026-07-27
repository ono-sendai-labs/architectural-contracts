# Task: Detect absorbed function-value escapes

## Description
Add the `facts.FuncValueEscape` fact and produce it in `goanalysis` by scanning SSA instruction operands in member functions for `*ssa.Function` values whose defining package is absorbed and which no member call path already reaches.

## Background
Capslock attributes capabilities by a backwards BFS from capability-carrying nodes over incoming call edges, reporting every reached function whose package is queried. A member that *calls into* an absorbed package is therefore still charged: the BFS walks through the absorbed body to the member caller. The one shape that escapes is taking the absorbed function's **value** — `host.Handlers{Open: backend.Load}` — where the component never calls it and whoever eventually does is behind a pruned boundary. Nobody analyzes that body.

This task makes the gap observable. It is the loader half of the same loader-collects/checker-reports split that `UnresolvedImports` already uses: `goanalysis` owns the SSA observation, the pure checker owns the wording and level (Task 3).

Three mechanics from the research are load-bearing and easy to get wrong:

- `CallCommon.Operands` **includes the callee**, so a plain static call `absorbed.F(x)` surfaces the `*ssa.Function` as an operand. Scanning naively would report every ordinary call into absorbed code — precisely the case that attribution already handles.
- `Function.Pkg` is nil for shared functions and synthetic wrappers, which includes the bound-method thunk produced by `x.M`. Resolving the defining package through `Pkg` alone silently drops the bound-method case that fixture 4 pins.
- `f := absorbed.F; f(x)` takes the value *and* calls it. VTA resolves the indirect call, so the authority is attributed and a warning would be noise.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A5, §4.5, §5.2, and §7.1 fixture 2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 8)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/capability-analysis-mechanics.md` (§3 — the operand-scan API facts and both caveats; §1 for why calls are already attributed)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add to `go/internal/facts`:
   - `FuncValueEscape{Symbol, Package string; File string; Line int}` with a doc comment stating that the body is analyzed by nobody — the component does not call it, and whoever does is behind a boundary.
   - `PackageFacts.FuncValueEscapes []FuncValueEscape`.
   `Symbol` is the absorbed function in canonical Capslock/go-types key form; `Package` is the member package that referenced it. `facts` stays pure data with no logic.
2. Give `goanalysis.LoadRequest` the component's absorbed declarations (import paths, patterns permitted) so the loader can classify a defining package as absorbed. Wire it in `go/cmd/arcc/app/app.go` from the already-canonicalized `Manifest.AbsorbedDependencies`; an empty set means no escapes can be produced.
3. Match an absorbed declaration against a package path with the same `path.Match` glob semantics the checker uses, over canonicalized paths, so the loader and the checker agree on what "absorbed" means.
4. Scan only functions whose package is an effective member, reusing the existing membership predicate and the SSA program and VTA call graph already built during fact loading. Do not build a second program or call graph.
5. For each instruction, enumerate `Operands`, and consider operands that are (or dereference to) `*ssa.Function`.
6. **Exclude the static callee position of a call.** For a call instruction in "call" mode (`!CallCommon.IsInvoke()`), the operand identical to the callee (`CallCommon.StaticCallee()` / `&Common().Value`) is not an escape. Operands in argument position of the same instruction are still scanned.
7. **Resolve the defining package via the function's object**, not `Pkg`: use `Object()` (with `Origin()` for instantiations) and fall back to `Pkg` only when no object is available. A bound-method value from an absorbed package must resolve to that package.
8. Report only when the resolved defining package is absorbed. A defining package that is a member, a resolved component dependency's package, standard library, or unresolvable produces nothing.
9. **Suppress when the call graph already reaches the function from member code.** If the VTA call graph has an in-edge to that function from a function in a member package, emit nothing — the authority is already attributed and the warning would be noise.
10. Record the referencing site's `File` and `Line` from the instruction position, falling back to the enclosing function's position when the instruction has none. `File` follows the existing convention for fact file paths (component-relative in native mode, workspace-relative in layout mode, forward slashes), matching how `UnresolvedImports` computes it.
11. Deduplicate by the full tuple and sort deterministically so repeated loads of the same sources produce byte-identical facts.
12. Produce the fact in both native and package-layout mode; the field is always initialized, empty rather than nil when there is nothing to report.
13. No checker or report change belongs in this task. The fact is produced and asserted at the `goanalysis` level; Task 3 renders it.
14. Keep the loader's existing failure modes: an unresolvable operand, a synthetic function with no object, or a nil package must be skipped, never fatal.

## Dependencies
- `task-01-remove-higher-order-boundary-warning`: clears the old func-value vocabulary before the new one lands, so no reviewer or fixture sees two overlapping warnings.
- Step 1 (declared members) and Step 2 (members as roots) supply the member set this scan is scoped to.
- `task-03-report-absorbed-func-value-escape` consumes the fact produced here.

## Implementation Approach
1. Add the pure fact type and the `PackageFacts` field, then extend `LoadRequest` and the application wiring so the absorbed set reaches the loader.
2. Write the operand scan as a focused helper alongside the existing call-graph walk, taking the program, the call graph, the membership predicate, and the absorbed matcher as inputs so it is unit-testable.
3. Implement callee exclusion and object-based package resolution first; add the call-graph suppression as a separate, separately-tested filter.
4. Build small `testdata` fixtures for each shape — struct-literal field, bound method value, local indirection, member-defined callback, plain static call — and assert the produced facts exactly, including file and line.
5. Run focused `facts`, `goanalysis`, and application suites, then `just ci`.

## Acceptance Criteria

1. **Struct-literal escape is detected**
   - Given a member assigning an absorbed package's function into a struct field handed to a dependency (design §7.1 fixture 2)
   - When package facts are loaded
   - Then exactly one `FuncValueEscape` names the absorbed symbol, the referencing member package, and the file and line of the reference.

2. **Bound method values are detected**
   - Given a member taking `x.M` as a value where `M` is defined in an absorbed package
   - When package facts are loaded
   - Then the escape is reported with the absorbed defining package resolved through the function's object, not through a nil `Pkg`.

3. **A plain call into absorbed code is not an escape**
   - Given a member calling `absorbed.F(x)` directly
   - When package facts are loaded
   - Then no escape is produced, because the callee operand of a call in call mode is excluded.

4. **Local indirection is suppressed**
   - Given `f := absorbed.F; f(x)` in a member
   - When package facts are loaded
   - Then no escape is produced, because the call graph already reaches the function from member code.

5. **Member-defined function values are silent**
   - Given the same struct-literal shape with the function value's body defined in a member package
   - When package facts are loaded
   - Then no escape is produced — the plugin-struct pattern stays silent.

6. **Non-absorbed defining packages are silent**
   - Given a function value defined in a resolved component dependency, in the standard library, or in a member
   - When package facts are loaded
   - Then no escape is produced for it.

7. **Facts are deterministic and always initialized**
   - Given repeated loads of the same sources, and a component with no escapes
   - When `PackageFacts` is returned
   - Then `FuncValueEscapes` is duplicate-free, deterministically sorted, byte-identical across runs, and non-nil when empty.

8. **Both loading modes produce the fact**
   - Given equivalent native and package-layout inputs for the escaping fixture
   - When facts are loaded in each mode
   - Then both report the escape, with `File` following that mode's existing path convention.

9. **Absorbed matching agrees with the checker**
   - Given an absorbed declaration written as a glob pattern that matches the defining package
   - When package facts are loaded
   - Then the escape is produced, using the same match semantics the checker applies to absorbed declarations.

10. **No reporting behavior changes yet**
    - Given the checker and report packages
    - When the full suite runs after this task
    - Then no new finding appears in any rendered output and all existing findings are unchanged.

11. **Repository checks remain green**
    - Given the new fact production
    - When `just ci` runs
    - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass.

## Metadata
- **Complexity**: High
- **Labels**: Go, goanalysis, facts, SSA, call-graph, ambient-authority
- **Required Skills**: Go, `golang.org/x/tools/go/ssa`, VTA call graphs, deterministic fact production, fixture design
