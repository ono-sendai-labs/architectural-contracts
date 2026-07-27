# Task: Enforce canonical loader paths

## Description
Turn the host canonicalization contract into a load-time invariant, canonicalize every package-path occurrence inside formatted symbols, and normalize dependency-interface package facts before the checker consumes them.

## Background
`hostpolicy.CanonicalizePath` exists to normalize external manifest or host spellings into the loader's package namespace. Loader-reported paths are already the authoritative namespace and therefore must be fixed points. If a host rewrites those paths again, component membership and package facts no longer join and analysis can become empty while appearing successful.

Canonicalization is also incomplete inside symbols: replacing only the first extracted package path misses package paths embedded in generic type arguments, such as `c/d` in `(*a/b.T[c/d.U]).M`. Dependency-interface facts have a related gap because their local package import paths and import lists are left uncanonicalized even though symbols and other fact paths use the canonical namespace.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 T5–T6, §4.4–4.5, §6, and §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 5)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Expand the exported `hostpolicy.CanonicalizePath` contract to state that it maps host/external spellings into the loader namespace and must be identity on every loader-reported package path, in addition to remaining idempotent and concurrency-safe.
2. Assert the identity invariant for every package path reported by native and layout-backed package loading before membership filtering, fact construction, or dependency-interface derivation can silently discard mismatched packages.
3. Return a clear load error when `CanonicalizePath(loaderPath) != loaderPath`; name the offending loader path and the rewritten result.
4. Apply the invariant consistently to packages reached through roots and their dependency graph, not only the top-level `packages.Load` return slice.
5. Ensure dependency-package loads in `ResolveDependencyInterface` receive the same identity validation rather than creating a second unchecked loader path.
6. Refactor `canonicalizeSymbol` so every syntactic package path in a supported formatted symbol is canonicalized, including receiver types and nested generic type arguments. Preserve symbol punctuation, pointer/value receiver form, type names, function names, and method names.
7. Cover the design example `(*a/b.T[c/d.U]).M`, including a host canonicalizer that rewrites both `a/b` and `c/d`, and add cases for ordinary functions, value receivers, multiple or nested type arguments, and identity behavior.
8. Avoid blind substring replacement that can rewrite type or method names, overlapping package prefixes, or non-package portions of a symbol. Keep malformed or unsupported symbol forms stable rather than corrupting them.
9. In `ResolveDependencyInterface`, canonicalize every local `facts.PackageFact.ImportPath` and every entry in `Imports`; retain deterministic sorting and align them with already canonicalized package paths, symbols, and call edges.
10. Preserve canonicalization of manifests and other external inputs where it already occurs; the loader identity assertion does not mean external spellings must already be canonical.
11. Do not alter standard-library classification or remove facts fields in this task.

## Dependencies
- `task-01-classify-stdlib-from-provenance`: establishes fail-closed loading and the final native/layout classifier that canonical paths feed.
- Steps 1–4 (complete): provide member-root filtering and dependency-interface loading behavior.
- `task-03-simplify-stdlib-facts` follows this task and removes obsolete facts representation after all loader facts are normalized.

## Implementation Approach
1. Add a reusable package-graph validation helper that visits each loaded package once, checks the canonical fixed-point invariant, and returns a path-specific error.
2. Invoke the helper immediately after each native or layout-backed `packages.Load`, including dependency-interface loads, before package paths are placed in sets or facts.
3. Replace the first-occurrence `canonicalizeSymbol` implementation with a parser or structured scanner for the formatted symbol grammar, canonicalizing package-qualified types recursively while retaining exact non-path syntax.
4. Canonicalize and sort dependency-interface package paths and import lists at fact construction.
5. Add focused unit tests plus an integration-style load test proving a non-identity loader canonicalizer fails loudly instead of yielding empty component facts.

## Acceptance Criteria

1. **Non-identity loader path fails closed**
   - Given a loaded package path `host/component` and a canonicalizer that rewrites it to another string
   - When native or layout-backed loading begins fact construction
   - Then loading fails with an error naming both `host/component` and its rewritten value.

2. **All loaded dependencies are checked**
   - Given a root package whose own path is a fixed point but whose transitive dependency path is rewritten
   - When the package graph is validated
   - Then loading fails on the dependency path rather than checking only roots.

3. **Dependency-interface loads enforce the invariant**
   - Given `ResolveDependencyInterface` loads a dependency package whose reported path is not a canonical fixed point
   - When interface resolution runs
   - Then it returns the same clear loader-path contract error.

4. **External spellings may still canonicalize**
   - Given a manifest or host-provided external path uses a noncanonical spelling while loader-reported paths are fixed points
   - When facts are joined
   - Then the external spelling is normalized as before and the load is accepted.

5. **Generic symbol paths are canonicalized everywhere**
   - Given `(*a/b.T[c/d.U]).M` and a canonicalizer that rewrites both package paths
   - When `canonicalizeSymbol` runs
   - Then both `a/b` and `c/d` are rewritten while the pointer receiver, generic structure, type names, and method name are unchanged.

6. **Symbol rewriting is structurally safe**
   - Given ordinary functions, value receivers, nested or multiple generic arguments, overlapping path prefixes, and malformed unsupported strings
   - When symbol canonicalization runs
   - Then package paths are rewritten exactly where parsed, valid syntax is preserved, and unsupported input is not corrupted.

7. **Dependency facts share one namespace**
   - Given dependency packages and imports whose external spellings require canonicalization
   - When `ResolveDependencyInterface` constructs local package facts
   - Then `ImportPath`, every `Imports` entry, exported symbols, and the returned package list consistently use canonical paths in deterministic order.

8. **Identity default remains a no-op**
   - Given the default host policy
   - When existing native and layout fixtures load and symbols are canonicalized
   - Then paths and symbols remain byte-for-byte unchanged.

9. **Repository checks remain green**
   - Given the enforced canonicalization contract and complete symbol rewriting
   - When `just ci` runs
   - Then unit, integration, self-check, generation-cleanliness, and Bazel suites pass.

## Metadata
- **Complexity**: High
- **Labels**: Go, goanalysis, packagelayout, host-policy, canonicalization, generics, fail-closed
- **Required Skills**: Go, `go/packages`, graph traversal, formatted Go type syntax, deterministic error handling, unit and integration testing
