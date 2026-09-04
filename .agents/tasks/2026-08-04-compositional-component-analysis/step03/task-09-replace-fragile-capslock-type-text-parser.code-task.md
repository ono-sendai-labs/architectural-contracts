# Task: Replace Fragile Capslock Type-Text Parser

## Description
Make Capslock-to-SymbolID normalization fail closed for malformed classifier names while accepting the valid Go/SSA type spellings Capslock can emit. Replace the repeatedly patched ad hoc subset parser with a context-aware approach grounded in structured package/inventory information and standard Go syntax where practical.

## Background
Addresses findings F3 and F4 from the Step 3 implementation review, originally reported against deferred Task 4 after four rework rounds. The current parser accepts `store.Load[(int, string)]` even though a tuple is not a Go type argument, and rejects valid spellings including `example.com/2x.T`, `func(example.com/x.T)`, and `func(x ...int)`. The package-path rules also disagree with the canonical `SymbolID` grammar, so continuing to add isolated grammar exceptions would leave the future stdlib-map generator exposed to silent normalization gaps.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step03.yaml`

**Additional References:**
- Deferred task: `.agents/tasks/2026-08-04-compositional-component-analysis/step03/task-04-implement-symbol-id-grammar.code-task.md`
- Deferred review record: `.agents/awo/runs/2026-08-04-compositional-component-analysis/step03/task-04-implement-symbol-id-grammar/task-record.json`
- Capslock function data: `github.com/google/capslock/proto.Function`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Redesign the normalizer API so callers can supply Capslock's structured package field and the independently inventoried declarations/packages used by Step 4, rather than recovering all identity from one ambiguous display string.
2. Use the canonical SymbolID package-path validation in one place; valid digit-leading import-path elements such as `example.com/2x` must not be rejected by a stricter private grammar.
3. Replace or sharply reduce the handwritten Go type grammar. Prefer standard `go/parser`/`go/types` syntax validation plus a documented translation of full import paths, or resolution against typed inventory, so acceptance tracks actual Go/SSA formatter output.
4. Reject a parenthesized tuple such as `(int, string)` when it appears where one type argument is required, including named-list variants that are only legal in function signatures.
5. Accept valid qualified unnamed parameters, named variadic parameters, qualified embedded struct fields, qualified interface-method parameters, and all constant-expression operators emitted by the supported formatter, including `&^`.
6. Keep pointer/value receiver and generic-origin convergence, dotful top-level disambiguation, namespace canonicalization, and strict rejection of compiler-synthesized or unresolvable symbols.
7. Add differential/table-driven tests built from real parsed and type-checked Go declarations. The corpus must cover every accepted type form and verify that every successful normalization resolves to an inventoried canonical SymbolID.
8. Keep the live SSA/VTA check path untouched; only the additive Step 3 normalizer and its tests may change.

## Dependencies
- Task 7's component-boundary repair must be complete.
- No dependency on the Step 4 map generator implementation; use a test inventory/context shaped so that Step 4 can consume the API directly.

## Implementation Approach
1. Define the structured normalization input and inventory-resolution contract before changing parsing code.
2. Build a corpus from real Go syntax and the Capslock `Function` package/name shape, including the exact deferred positive and negative cases.
3. Route package identity and final output through the canonical SymbolID constructors, using standard syntax validation or typed resolution for type arguments.
4. Delete obsolete private grammar branches and tests that encode narrower package/type rules.
5. Run focused symbol tests, verify the live call-graph files are unchanged, then run the full CI gate.

## Acceptance Criteria

1. **Malformed tuple arguments fail closed**
   - Given `store.Load[(int, string)]` and related parenthesized named-list forms in a single type-argument position
   - When normalization runs
   - Then it returns an actionable error and emits no SymbolID.

2. **Valid formatter spellings converge**
   - Given type arguments containing `example.com/2x.T`, `func(example.com/x.T)`, `func(x ...int)`, qualified embedded fields/interface parameters, and array expressions using `&^`
   - When normalized with matching package and declaration inventory context
   - Then each maps to the expected uninstantiated canonical SymbolID.

3. **Package grammar has one authority**
   - Given any package path accepted by canonical SymbolID construction
   - When it appears in structured Capslock normalization context
   - Then no stricter duplicate path grammar rejects it; malformed or non-canonical paths still fail.

4. **Normalization is inventory-backed**
   - Given ambiguous or generic Capslock text
   - When the structured package or declaration inventory does not confirm one canonical declaration
   - Then normalization fails rather than guessing; when exactly one declaration is confirmed, the result parses as a canonical SymbolID.

5. **Regression corpus tracks real Go syntax**
   - Given the supported Go/SSA formatter corpus
   - When symbol tests run
   - Then positive rows originate from parsed/type-checked declarations, negative rows are syntactically invalid or unresolvable, and the deferred cases are permanently pinned.

6. **Live checking remains unchanged**
   - Given existing call-graph, pruning, and dependency-interface fixtures
   - When `just ci` runs
   - Then all verdicts remain unchanged and the production SSA/VTA path is still present for the Step 6 cutover.

## Metadata
- **Complexity**: High
- **Labels**: go, capslock, symbol-id, parser, go-types, fail-closed
- **Required Skills**: Go parsing and type checking, Capslock/SSA naming, API design, deterministic identifiers, fuzz/table-driven testing
