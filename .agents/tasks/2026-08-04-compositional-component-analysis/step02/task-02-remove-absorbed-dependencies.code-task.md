# Task: Remove Absorbed Dependencies

## Description
Remove `absorbed_dependencies` and `absorbed_deps` end to end: persisted schema, manifest model and validation, checker allowances and warnings, package-analysis facts, CLI plumbing, Bazel rule API, generated and checked-in manifests, fixtures, and documentation. Stale authored manifests must fail during parsing rather than silently losing their declaration.

## Background
Absorbed dependencies let a component claim implementation-detail packages and inherit their ambient authority. The compositional design replaces that concept with ordinary checked component boundaries (and later `authority: UNKNOWN` for incremental adoption). Because field number 4 was persisted, it must be reserved permanently when the message and field are removed. This implementation series is explicitly unreleasable until Step 11, so compatibility means rejecting stale manifests clearly, not retaining the old semantics.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Decision record Q4-Q7: `.agents/planning/2026-08-04-compositional-component-analysis/idea-honing.md`
- Plan Step 2: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. In `proto/archcontracts/v1/component.proto`, delete field 4 and the `AbsorbedDependency` message; add both `reserved 4;` and `reserved "absorbed_dependencies";`, then regenerate the checked-in Go protobuf binding through the repository generator.
2. Remove `manifest.Manifest.AbsorbedDependencies`, `manifest.AbsorbedDependency`, absorbed-specific errors, parsing, copying, duplicate checks, member-conflict checks, and parity comparisons. Add a focused parse test proving a textproto that still names `absorbed_dependencies` fails with an actionable unknown/removed-field diagnostic.
3. Remove all absorbed import allowances, overlap findings, unused-declaration warnings, and path-pattern handling from `checker.Check`. Non-stdlib imports not owned by the component and not covered by a component dependency must now report `UNDECLARED_DEPENDENCY` directly.
4. Remove `facts.FuncValueEscapes`, `facts.FuncValueEscape`, and `facts.BodilessAbsorbedPackages`; remove `goanalysis.LoadRequest.Absorbed`, `scanFuncValueEscapes`, `collectBodilessAbsorbedPackages`, their call-graph scans, and their dedicated testdata and tests.
5. Remove `report.AbsorbedFuncValueEscape` (`ABSORBED_FUNC_VALUE_ESCAPE`) and all rendering, JSON, checker, CLI, and integration coverage that exists only for that obsolete finding. Existing absorbed fixtures and tests are deleted, not weakened into unrelated assertions.
6. Remove absorbed-path canonicalization and request plumbing from `Runner.runCheck`; the loader must no longer receive absorbed patterns.
7. Remove the Bazel `absorbed_deps` public/private attribute, closure classification, validation, manifest emission, provider descriptions, adapter comments, analysis tests, test targets, and golden textproto entries. Update every self-component, example component, and test component to use explicit members or component dependencies as appropriate so `manifestparity` and self-check remain green.
8. Before deleting declarations, audit example and self-component manifests for absorbed and pattern-member use. Record the audit result in the implementation change description; do not assume these inputs are absent.
9. Rewrite the README walkthrough at the plan's cited section to demonstrate `UNDECLARED_AUTHORITY` by calling `os.ReadFile` without declaring `FILES`. Remove earlier absorbed-dependency examples and API documentation, and explain the replacement in terms of ordinary component dependencies where context is needed.
10. Update comments and package contracts that describe absorbed packages, without changing unrelated call-graph or component-boundary behavior scheduled for later steps.
11. Ensure `just gen-is-clean` and the full `just ci` gate pass with no live code, schema, manifest, fixture, or documentation references to either absorbed API spelling or the obsolete warning kind, except historical planning artifacts.

## Dependencies
- Task 1 must be complete; this task assumes `AnalyzeRequest.PruneAtPackages` has already been removed.
- No dependency on pattern-membership removal. Exact and pattern member matching remains until Task 3.

## Implementation Approach
1. Inventory all absorbed declarations in Go, proto, Starlark, manifests, fixtures, examples, and docs, and note whether any example or self-component also uses pattern members.
2. Remove absorbed facts and warnings from the analysis/checking pipeline, then contract the loader and application DTOs.
3. Remove the persisted and in-memory manifest model, reserve the schema field name and number, regenerate bindings, and update manifest/parity tests with a stale-input rejection case.
4. Simplify the Bazel rule and macro API, reclassifying affected test/example packages through explicit members or component dependencies and refreshing only semantically changed goldens.
5. Delete absorbed-only fixtures and rewrite the README walkthrough around a direct undeclared-authority finding.
6. Search the non-planning tree for stale terminology, run generation cleanliness, focused Go/Bazel tests, and `just ci`, then include the manifest audit result in the `jj` change description.

## Acceptance Criteria

1. **Persisted field is retired safely**
   - Given the component protobuf schema after regeneration
   - When field declarations and generated accessors are inspected
   - Then field number 4 and name `absorbed_dependencies` are reserved, `AbsorbedDependency` no longer exists, and no generated accessor remains.

2. **Stale manifests fail at parse time**
   - Given a component textproto containing `absorbed_dependencies`
   - When `manifest.Parse` or `arcc check` reads it
   - Then parsing fails with an actionable error identifying the unsupported field, and the declaration is never silently ignored.

3. **Imports use the simpler dependency rule**
   - Given member code imports a non-stdlib package that is neither a member nor owned by a resolved component dependency
   - When `checker.Check` evaluates it
   - Then it emits `UNDECLARED_DEPENDENCY`; no absorbed allowlist, overlap, or unused-warning path can suppress or alter the result.

4. **Absorbed analysis residue is gone**
   - Given the facts DTO, package loader, report kinds, and application request path
   - When their APIs and implementations are inspected
   - Then function-value escape scans, bodiless absorbed-package collection, absorbed loader input, `ABSORBED_FUNC_VALUE_ESCAPE`, and corresponding tests/fixtures are absent.

5. **Bazel and manifests expose no absorbed API**
   - Given the public `go_component` API, its private rule, providers, generated manifests, self-components, examples, and testdata
   - When they are built and searched
   - Then neither `absorbed_deps` nor `absorbed_dependencies` is emitted or accepted, and affected package ownership is expressed through supported component constructs.

6. **README teaches direct authority failure**
   - Given the worked README walkthrough
   - When a user follows its undeclared-authority example
   - Then the component calls `os.ReadFile` without declaring `FILES` and receives the same intended authority verdict without absorbed dependencies.

7. **Fixtures are deleted rather than weakened**
   - Given absorbed-only Go and Bazel fixtures and their expected findings
   - When the task is complete
   - Then they are removed; no test has been rewritten to preserve obsolete absorbed semantics under another name.

8. **Audit and integration are complete**
   - Given the repository's example and self-component manifests
   - When they are audited before deletion and the result is recorded in the change description
   - Then every affected manifest is intentionally migrated, `just gen-is-clean` passes, and `just ci` is green.

## Metadata
- **Complexity**: High
- **Labels**: go, protobuf, bazel, manifest, checker, documentation, deletion
- **Required Skills**: Go, Protocol Buffers, Starlark, Bazel analysis tests, schema evolution, deterministic testing, Jujutsu
