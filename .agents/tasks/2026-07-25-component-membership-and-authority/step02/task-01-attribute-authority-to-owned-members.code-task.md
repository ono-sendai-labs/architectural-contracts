# Task: Attribute authority to owned members

## Description
Complete ownership-based authority attribution by ensuring every effective member package is an analysis root, then remove the obsolete `INIT_OUTSIDE_INTERFACE` placement rule that member implementation packages would otherwise trip.

## Background
Step 1 made declared membership available end to end and filters `LoadPackageFacts` output to the effective member set (declared members plus the implicit interface package, or FR1 packages when `members` is absent). The application passes those fact package paths to Capslock, whose backwards capability traversal analyzes every function in every queried package regardless of whether another member calls it. This must be pinned with the Part 1 callback regression: when `backend` is a member, taking `backend.Load` as a function value and handing it across the `host` component boundary must still charge the component for `backend.Load`'s FILES authority.

Making implementation packages roots invalidates the old explicit-init placement proxy. An ordinary `func init()` in a member implementation package must not be a conformance finding; its authority is now charged directly through root analysis. The dependency-pruning `func <dependency-package>.init CAPABILITY_SAFE` mechanism remains untouched. `METHOD_OUTSIDE_INTERFACE` also remains unchanged because it is already limited to methods whose receiver type is declared in an interface file.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 A1–A3, §4.3, §4.5, §5.3, §7.1 fixture 1, and §9)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 2)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-24-callback-authority-attribution/callback-authority-attribution.md` (Part 1 example and “Why this also closes the Part 1 gap for owned code”)
- `.agents/planning/2026-07-25-component-membership-and-authority/research/capability-analysis-mechanics.md` (§1, Capslock backwards attribution and queried-package semantics)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Preserve one authoritative effective-member set from `goanalysis.LoadPackageFacts`: declared members plus the implicit interface package when `Manifest.Members` is non-empty, layout roots in layout mode, and the existing FR1 package set when membership is absent.
2. Ensure `facts.PackageFacts.Packages` contains only effective members while dependency packages remain available internally for type checking, SSA, VTA call-graph construction, standard-library classification, and dependency-interface resolution.
3. Ensure the application constructs `capanalyzer.AnalyzeRequest.Packages` from every effective member package fact, so each member is a Capslock queried package/root without requiring call-graph reachability from the interface package.
4. Add a regression fixture matching design §7.1 fixture 1: `editor` hands member package `backend.Load` to a `host` component as a callback, `backend.Load` reaches `os.ReadFile`, and no owned code directly calls it.
5. Prove the fixture reports `UNDECLARED_AUTHORITY` for FILES when `backend` is a member and the component does not declare FILES. Assert the evidence identifies member-owned code so the test cannot pass because of unrelated authority.
6. Keep absorbed packages out of the root set; this task closes the callback gap only for owned/member code and must not implement the later absorbed-function-value warning.
7. Remove `report.InitOutsideInterface`, the `sym.Kind == "init"` checker branch, and all tests or expected-output assertions whose purpose is to require or render `INIT_OUTSIDE_INTERFACE`.
8. Update deterministic multi-finding assertions after the init finding disappears, including the existing fixed-index assertion, without weakening checks for the remaining finding kinds and order.
9. Add or retain an explicit test proving a member implementation package with an ordinary `func init()` produces no finding, including when the init is outside `interface_files`.
10. Add or strengthen an explicit test proving `METHOD_OUTSIDE_INTERFACE` remains scoped to a receiver type declared in an interface file: a method on an implementation type declared outside all interface files is clean, while the existing interface-type placement violation remains enforced.
11. Do not change dependency init pruning, `METHOD_OUTSIDE_INTERFACE` semantics, absorbed-dependency use attribution, report success wording, or any Step 3-and-later behavior.

## Dependencies
- Step 1 (complete): declared membership schema and validation, checker membership semantics, bodiless-member rejection, and native loading of the effective member set.
- No later Step 2 task depends on this task; it is the single atomic behavior change for the step.

## Implementation Approach
1. Start with the Part 1 integration regression and inspect the package list passed from the application to `capanalyzer.AnalyzeRequest`; adjust the `goanalysis`/application seam only if the regression demonstrates that an effective member is missing.
2. Build the fixture as a small multi-package module with `editor`, `backend`, and `host`, declare `backend` as a member and `host` as a component dependency, and arrange the function-value flow so no direct member call to `backend.Load` can account for the FILES finding.
3. Delete the init report kind and checker branch, then replace the old outside/inside init rule tests with a member-implementation clean test.
4. Repair exact-count, fixed-index, JSON, text, CLI, and Bazel golden expectations affected by the removed finding while keeping deterministic assertions precise.
5. Pin the unchanged method rule with both sides of its scope boundary and run focused checker, goanalysis, application, and integration tests before the full repository CI.

## Acceptance Criteria

1. **Owned callback authority is charged**
   - Given `backend` is an explicit member and `editor` only stores `backend.Load` in a callback value passed to the `host` component
   - When the component is checked with no declared FILES authority
   - Then the report contains `UNDECLARED_AUTHORITY` for FILES with evidence attributable to member-owned `backend.Load`.

2. **Member roots do not depend on interface reachability**
   - Given an effective member function reaches ambient authority but no interface-package call-graph path reaches that function
   - When the analyzer request is constructed
   - Then the member package is still queried and its authority is reported.

3. **Only effective members become roots**
   - Given the loaded graph also contains component dependencies, absorbed dependencies, standard-library packages, and unrelated transitive packages
   - When package facts and the capability-analysis request are produced
   - Then only effective members are emitted as component package facts and queried as roots.

4. **Member implementation init is clean**
   - Given a member implementation package declares `func init()` outside the component's interface files
   - When the checker runs
   - Then no placement finding is produced and any authority reachable from the init is handled by ordinary member-root attribution.

5. **Init finding is removed completely**
   - Given the implementation, tests, rendered text, JSON output, and golden expectations
   - When the repository is searched and the relevant suites run
   - Then no `INIT_OUTSIDE_INTERFACE` report kind, checker branch, finding, or expectation remains.

6. **Dependency init pruning is preserved**
   - Given a declared component dependency whose package init is pruned with the existing `CAPABILITY_SAFE` key
   - When capability analysis runs
   - Then the prune behavior is unchanged by removal of the placement finding.

7. **Method placement remains interface-scoped**
   - Given an exported method on a receiver type declared outside all interface files
   - When the checker runs
   - Then it produces no `METHOD_OUTSIDE_INTERFACE`; a method outside the interface files on a receiver type declared in an interface file still produces that violation.

8. **Existing integrations remain green**
   - Given the ownership-attribution behavior and non-additive report-kind removal
   - When self-checks, deterministic render tests, CLI integrations, and Bazel tests run
   - Then expected outputs reflect only the intentional init-finding removal and no unrelated rule is relaxed.

9. **Step 2 demo is reproducible**
   - Given the Part 1 fixture and a member implementation package with `func init()`
   - When the check is demonstrated
   - Then callback-only FILES authority now fails, the init placement itself is clean, and authority used by member code is charged through ownership.

10. **Repository checks remain green**
    - Given the completed Step 2 implementation
    - When `just ci` runs
    - Then all unit, integration, self-check, generation-cleanliness, and Bazel checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: Go, goanalysis, checker, authority-attribution, report, integration
- **Required Skills**: Go, `go/packages`, SSA/VTA and Capslock analysis, deterministic report testing, integration testing
