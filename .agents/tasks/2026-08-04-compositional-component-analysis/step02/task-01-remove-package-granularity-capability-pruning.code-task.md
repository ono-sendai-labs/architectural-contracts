# Task: Remove Package-Granularity Capability Pruning

## Description
Delete the package-granularity Capslock pruning path so capability analysis has only the existing symbol-level `PruneAt` boundary mechanism. This removes `AnalyzeRequest.PruneAtPackages`, its classifier branch, and all CLI plumbing before the remaining absorbed-dependency machinery is deleted.

## Background
The compositional-analysis design removes package-wide authority suppression. `PACKAGE_SURFACE` will ultimately be handled by syntactic boundary classification rather than by telling Capslock that every function in a dependency package is safe. During the unreleased Step 2-to-Step 11 transition, removing this legacy path is intentional even though the later reference scanner has not landed yet.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Plan Step 2: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response, including the verified `PruneAtPackages` implementation inventory: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Remove `capanalyzer.AnalyzeRequest.PruneAtPackages` and update its scope documentation so `PruneAt` is the only boundary-pruning input.
2. Remove the package-key collection and classifier branch from `capslockadapter`; classifier construction must accept only symbol keys and preserve the existing deterministic symbol-level output.
3. Remove `PruneAtPackages` construction and forwarding from the `arcc check` application path. Keep resolved package-surface dependency data available to the checker; only Capslock's package-wide suppression is removed.
4. Delete package-pruning unit and integration tests instead of converting them into assertions for obsolete behavior. Update analyzer spies and request assertions to the reduced request contract.
5. Preserve symbol-level `PruneAt` behavior, including package initializer keys currently added to that symbol set.
6. Keep all core, application, adapter, self-check, and Bazel tests green after the API contraction.

## Dependencies
- No dependency on other Step 2 tasks.
- Must land before Task 2 so the subsequent absorbed-dependency deletion starts from the final `AnalyzeRequest` shape.

## Implementation Approach
1. Contract the pure `capanalyzer` request DTO and its comments.
2. Simplify the Capslock classifier builder and adapter call sites to accept only `PruneAt`, retaining validation and deterministic ordering for symbol keys.
3. Remove package-prune aggregation from `Runner.runCheck` and update focused application and adapter tests.
4. Run focused `capanalyzer`, `capslockadapter`, and application tests, then the repository CI gate.

## Acceptance Criteria

1. **Request API is symbol-only**
   - Given the `capanalyzer.AnalyzeRequest` public DTO
   - When its fields and documentation are inspected
   - Then it contains `Packages` and `PruneAt` but no `PruneAtPackages` field or package-pruning semantics.

2. **Classifier has no package-safe branch**
   - Given resolved dependencies include a `PACKAGE_SURFACE` dependency
   - When the application builds a capability-analysis request and the Capslock adapter builds its classifier
   - Then no package-granularity key or `ClassifierExcludingPackage`-style branch is produced, while normal symbol keys remain deterministic.

3. **Symbol pruning remains intact**
   - Given a declared dependency exposes interface symbols and package initializer symbols
   - When `arcc check` invokes capability analysis
   - Then those symbols are still included once in sorted `PruneAt` input and are still respected by the adapter.

4. **Obsolete tests are removed, not repurposed**
   - Given tests whose sole purpose is `PruneAtPackages` validation, namespace separation, package resolution, or end-to-end suppression
   - When the task is complete
   - Then those tests are deleted and remaining request-spy tests assert only supported fields.

5. **Integration remains green**
   - Given the package-pruning path has been removed across the port, application, and adapter
   - When `just ci` runs
   - Then all checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: go, capanalyzer, capslock, api-removal, deletion
- **Required Skills**: Go, dependency injection, adapter design, table-driven testing
