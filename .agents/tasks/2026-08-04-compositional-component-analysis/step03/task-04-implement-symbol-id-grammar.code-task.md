# Task: Implement Symbol ID Grammar

## Description
Introduce the core `SymbolID` value and its single versioned textual grammar, with conversion from Capslock spellings and `go/types` objects under the declaring-object rule. Provide a deterministic symbol extractor for exact declared surfaces, including aliases, promotions, generic origins, and duplicate observations, while leaving the current call-graph check path untouched.

## Background
Surface artifacts, standard-library map inventory, reference scanning, and reports must compare the same stable identifier. Today's `InterfaceSymbol` contract emits both pointer and value method spellings to accommodate SSA/VTA behavior and strips generic text opportunistically. The compositional model instead keys the declaration that `go/types` says owns the referenced object. Step 3 defines and tests that grammar additively; Step 6 performs the check-path cutover.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Plan Step 3: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review DR-04 and response: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Define `SymbolID` in a small core-side package with a documented `v1` textual encoding, strict parse/format APIs, equality/order semantics suitable for sorted artifacts, and actionable rejection of malformed or unsupported-version input.
2. Cover the grammar bodies for package-level functions, variables, constants and named/alias types (`pkg.Name`), declared methods (`(pkg.Type).Method` with neither pointer marker nor type arguments), and package initialization (`pkg.init`). Persist only canonical package paths.
3. Implement a Capslock-name normalizer that maps pointer receiver spellings such as `(*os.File).Read`, value spellings, and generic instantiations to the same canonical declaring method ID; reject names that cannot be mapped safely instead of guessing.
4. Implement `go/types` conversion under the declaring-object rule. Package-level funcs/vars/consts/types key themselves; methods use `Func.Origin()` and their receiver base named/alias type; fields key the declaring struct type; interface method specs key the declaring interface type; promoted selections key the object/type that declares the selected member.
5. Handle aliases explicitly: an exported alias has its own top-level ID, and when its same-package target is a named type the declared-surface extractor also emits the target key needed for member resolution. Do not rewrite cross-package declarations as if the alias owned them.
6. Add an extractor over typed declared interface files/packages that emits exact surface IDs with no implements-closure injection. Canonicalize package paths through the existing host-policy hook, sort results, and collapse duplicate observations deterministically.
7. Keep the existing `capanalyzer.InterfaceSymbol`, SSA/VTA normalization, pruning inputs, `facts.CallEdges`, and production `ResolveDependencyInterface` behavior in place until Step 6. New `SymbolID` code may be exercised by unit fixtures but must not alter current verdicts.
8. If a new non-test package is introduced, add its FR10 Component Contract, BUILD target, manifest ownership/dependency declarations, and black-box tests according to repository conventions.
9. Add table-driven tests for every row in the design grammar table, including pointer/value receivers, generic origins/instantiations, embedded/promoted fields and methods, interface method specs, same- and cross-package aliases, init, duplicate observations, malformed encodings, and namespace canonicalization.

## Dependencies
- Task 3 must be complete so the persisted schemas that store symbol strings are fixed before the grammar implementation lands.
- The implementation is additive and must not depend on the reference scanner, surface producer, or stdlib map generator from later steps.

## Implementation Approach
1. Define the versioned parser/formatter and canonical method/body representation in a dependency-light core package.
2. Build focused `go/types` fixtures and implement object/selection conversion one object kind at a time, using origins and declared receiver bases rather than formatted instantiated types.
3. Implement the Capslock adapter normalizer on top of the same parser/constructor primitives so there is one canonicalization rule.
4. Add the exact-interface extractor, alias expansion, sorting, and deduplication behind an API not yet used by the live checker.
5. Run focused unit tests, self-check/component parity for any new package ownership, and `just ci`.

## Acceptance Criteria

1. **Text encoding is canonical and versioned**
   - Given valid IDs for every supported top-level, method, and init form
   - When each is formatted, parsed, and formatted again
   - Then the bytes are identical, contain the supported version, omit pointer markers/type arguments, and unsupported versions or malformed bodies fail clearly.

2. **Capslock variants converge**
   - Given pointer/value and instantiated/uninstantiated spellings of the same declared method
   - When normalized from Capslock form
   - Then all produce one identical `(pkg.Type).Method` `SymbolID`; invalid classifier text cannot enter an artifact as an ID.

3. **Declaring-object rule covers every object kind**
   - Given typed references to funcs, methods, types, fields, interface methods, vars, consts, aliases, and promoted members
   - When converted to IDs
   - Then each maps to the top-level declaration specified by the design table, with generic methods using their origin and promotions using the actual declaring member/type.

4. **Alias extraction is complete but bounded**
   - Given an exported alias to a same-package named type and an alias to a cross-package type
   - When exact surface symbols are extracted
   - Then the alias key is always emitted, the same-package target key is additionally emitted where required for member resolution, and no foreign target is falsely claimed.

5. **Extraction is deterministic**
   - Given repeated files or observations that describe the same declaration in differing traversal orders
   - When extraction runs
   - Then the result is sorted, duplicate-free, and byte-for-byte stable after schema encoding.

6. **Live check behavior is unchanged**
   - Given existing call-graph, pruning, and dependency-interface fixtures
   - When `just ci` runs after the new grammar is added
   - Then their verdicts remain unchanged and the current `InterfaceSymbol`/SSA path is still present for the Step 6 cutover.

## Metadata
- **Complexity**: High
- **Labels**: go, go-types, symbols, generics, aliases, deterministic-model
- **Required Skills**: Go, `go/types`, generic origins, type aliases, AST/type-info testing, API design
