# Task: Prune at package granularity

## Description
Give the capability-analysis port a second, explicit prune granularity —
`capanalyzer.AnalyzeRequest.PruneAtPackages` beside the existing `PruneAt` — and
teach `capslockadapter.buildClassifier` to emit `package <import/path>
CAPABILITY_SAFE` lines for it alongside today's `func` keys.

## Background
Every prune arcc performs today is per-symbol: a dependency's declared interface
symbols become `func <key> CAPABILITY_SAFE` lines in a per-run Capslock
classifier, so the backwards capability BFS stops at the boundary. That works
only where a dependency *has* declared interface symbols. A `PACKAGE_SURFACE`
component (A6) has none — its surface is every exported symbol of every member,
including unexported entry points that symbol pruning cannot name at all.

Capslock already supports the coarser key. `interesting.parseCapabilityMap`
accepts a `package` keyword, and `FunctionCategory` resolves a per-function key
*before* falling back to the package category, so the two key forms coexist
without interference and a per-function key still wins
(`../research/capability-analysis-mechanics.md` §2). This is genuinely coarser
than the `component_dep` prune — it marks every function in the package safe,
including ones reached through undeclared entry points — and that coarseness is
the relaxation A6 deliberately accepts, not an oversight.

The granularity stays **explicit in the port** rather than being inferred inside
the adapter from a key's shape: a caller asking for a package prune is making a
different, weaker claim than one asking for a symbol prune, and the request type
should say which was meant.

This task is the mechanism only. Nothing populates `PruneAtPackages` yet — Task 2
wires it from `PACKAGE_SURFACE` dependencies — so no end-to-end behavior changes.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A6, §4.1 `InterfaceStyle`, §4.6, §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 9)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/capability-analysis-mechanics.md` (§2 — the four classifier keywords, `FunctionCategory`'s resolution order, and why package granularity is coarser than the `component_dep` prune)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `PruneAtPackages []string` to `capanalyzer.AnalyzeRequest`, documented in
   the existing "Scope Semantics" comment as *package*-granularity pruning:
   every function in the named package is treated as capability-safe, with
   `PruneAt`'s per-symbol keys taking precedence. Say plainly that this is
   weaker than symbol pruning and is what a `PACKAGE_SURFACE` dependency gets.
   `capanalyzer` stays a pure port with no logic added.
2. Entries are canonical import paths, not `InterfaceSymbol` values. Do not
   reuse the `InterfaceSymbol` type: it carries the go-types key normalization
   contract (A4) and an import path is not one of those keys.
3. `buildClassifier` takes the package list as a second parameter and emits one
   `package <path> CAPABILITY_SAFE` line per entry, into the same custom
   classifier text it already builds.
4. Apply the same input validation the symbol path applies, with messages that
   name the package form: reject an empty entry, and reject one containing `\r`
   or `\n` (which would forge a classifier line). Deduplicate.
5. Keep the two key namespaces separate. The existing `seen` map deduplicates
   *function* keys; a package key that happens to equal a function key string
   must not suppress either. Use a distinct set, or key it by keyword plus
   value.
6. Emit deterministically: sort the package keys so repeated runs with the same
   request produce byte-identical classifier input.
7. `Adapter.Analyze` passes `req.PruneAtPackages` through to `buildClassifier`.
   No other analyzer behavior changes; the `UNANALYZED`-excluding wrapper and
   the `(*os.File)` handle-use reclassification are untouched.
8. An empty or nil `PruneAtPackages` must produce exactly the classifier text
   today's code produces, byte for byte.
9. No manifest, checker, report, or application change belongs in this task. The
   field is plumbed and exercised at the adapter level only.

## Dependencies
- None within Step 9 — this is the first task and the mechanism the rest builds on.
- Task 2 (`task-02-resolve-package-surface-dependency-surface`) is the first
  producer of `PruneAtPackages`.

## Implementation Approach
1. Add the port field with its doc comment; confirm the package still compiles
   with no new imports and remains pure.
2. Extend `buildClassifier`'s signature and body, keeping the function-key loop
   untouched and adding the package-key loop after it with its own dedup set.
3. Add adapter tests over the real Capslock classifier: assert the resolution
   order behaviorally (`FunctionCategory` on a function inside a
   package-pruned package returns safe; the same function with an explicit
   `PruneAt` key of a *different* capability classification still resolves via
   the function key) rather than asserting on generated text alone.
4. Add a testdata package with a capability-carrying function to pin that a
   package prune actually suppresses a finding end to end through
   `Adapter.Analyze`.
5. Run the focused `capanalyzer` and `capslockadapter` suites, then `just ci`.

## Acceptance Criteria

1. **Package keys prune every function in the package**
   - Given a request naming a package that reaches `FILES` and listing that package in `PruneAtPackages`
   - When `Adapter.Analyze` runs
   - Then no capability finding is attributed through that package's functions, including ones no `PruneAt` symbol could name.

2. **A per-function key still overrides the package key**
   - Given a package listed in `PruneAtPackages` and a function in it also listed in `PruneAt`
   - When the classifier resolves that function's category
   - Then the per-function key decides, matching Capslock's documented resolution order.

3. **The two granularities are independent in the port**
   - Given an `AnalyzeRequest` with symbols only, packages only, and both
   - When each is analyzed
   - Then each prunes exactly what it names and neither silently reinterprets the other's entries.

4. **Malformed package entries fail closed**
   - Given a `PruneAtPackages` entry that is empty or contains a newline
   - When the classifier is built
   - Then an error names the offending entry and no classifier is returned.

5. **Output is deterministic and dedup-correct**
   - Given a request with duplicate and unsorted package entries, including one whose string equals a function key
   - When the classifier is built repeatedly
   - Then the generated classifier input is byte-identical across runs and neither key form suppresses the other.

6. **The empty case is bit-for-bit unchanged**
   - Given a request with no `PruneAtPackages`
   - When the classifier is built
   - Then its input text is identical to the text produced before this task.

7. **No end-to-end behavior changes yet**
   - Given the application and checker packages
   - When the full suite runs after this task
   - Then no rendered report changes and no self-check result changes.

8. **Repository checks remain green**
   - Given the new port field and adapter behavior
   - When `just ci` runs
   - Then unit, integration, generation-cleanliness, self-check, and Bazel suites pass.

## Metadata
- **Complexity**: Low
- **Labels**: Go, capanalyzer, capslockadapter, pruning, ports-and-adapters
- **Required Skills**: Go, Capslock classifier internals, adapter testing
