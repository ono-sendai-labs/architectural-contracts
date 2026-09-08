# Task: Migrate Check Tests to Report Artifacts

## Description
Convert all three Bazel check rules into lightweight assertions over the report produced by `go_component`'s ordinary analysis action. Remove duplicate check execution from test launchers while preserving pass, expected-violation, grep, and golden behavior.

## Background
After Task 4, a checked component already owns one canonical report and surface. The current `arcc_check_test`, `arcc_check_grep_test`, and `arcc_check_report_golden_test` each rerun `arcc check` with SDK sources and the whole component runfiles, creating a second verdict path and preventing provenance from meaning “this exact producer ran.” Step 5 finishes the topology by making tests consume the provider's report: verdict assertions use Task 1's `arcc verdict`, while grep and golden assertions read the same canonical report artifact. Golden restructuring by host-independent verdict versus layout shape remains Step 9; only the minimum format migration needed to consume `.report.json` belongs here.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` — R8, §Build topology, acceptance-matrix Build graph row, Appendix A “Always-green analysis action”

**Additional References:**
- Plan Step 5: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Design review response DR-01: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Existing test rules: `bazel_rules/go/private/check.bzl`
- Existing report goldens and launchers: `bazel_rules/go/tests/goldens/`, `bazel_rules/go/tests/BUILD.bazel`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Change `arcc_check_test` to require a checked `ArccComponentInfo.report` and invoke `arcc verdict <report> --expect=pass|fail`; `expect_violation` selects `fail`. It must not invoke `arcc check` or load component/SDK sources merely to recompute analysis.
2. Change the grep rule to search the canonical report artifact and assert its expected verdict/status without rerunning analysis. Preserve fixed-string matching, actionable missing-string diagnostics, and current negative-fixture intent.
3. Change the report-golden rule to compare the provider's canonical `.report.json` directly. Migrate text-format goldens or rule attributes only as required for this artifact; defer the broad verdict/layout/surface golden restructure to Step 9.
4. Keep launcher runfiles minimal: report, arcc only where `verdict` is invoked, and golden where applicable. Remove `go_sdk_srcs`, manifest/layout, dependency sources, and other inputs that belonged only to duplicate check execution.
5. Fail clearly at analysis time when any assertion rule is attached to an asserted provider with `report = None`; do not infer pass from surface provenance.
6. Delete or repurpose obsolete check argv/launcher helpers so there is one analysis argv (Task 4) and one report-assertion argv. Preserve safe shell quoting and deterministic script content.
7. Add analysis/execution tests proving a dependent's `.check` builds its own and direct dependencies' analysis actions exactly once, `bazel build //...` without `arcc` outputs runs none, expected violations pass only for `fail`, and grep/golden consume the exact provider artifact.
8. Update public macro/rule documentation, generated fixture expectations, BUILD metadata, and FR10 Component Contracts where responsibilities changed.

## Dependencies
- Task 1 provides the canonical report verdict and `arcc verdict` command.
- Task 4 provides checked report artifacts, provider fields, producer ordering, and output-group behavior.
- Task 5 defines asserted providers with no report and migrates manual fixture misuse.
- Step 9 owns the later comprehensive golden restructure and must not be pulled into this task.

## Implementation Approach
1. RED: analysis tests asserting each rule's runfiles/argv name only the provider report and no analysis inputs.
2. Refactor the normal, grep, and golden launchers around one report artifact contract and explicit asserted-provider rejection.
3. Migrate the smallest necessary golden/fixture set and keep negative tests tied to checked reports.
4. Add action-graph/execution assertions for exactly-once producer behavior and default-build laziness.
5. Run all Bazel tests, self-check, and `just ci`.

## Acceptance Criteria

1. **Normal and expected-violation tests assert recorded verdicts**
   - Given passing and failing checked reports
   - When `.check` runs with `expect_violation` false and true
   - Then it invokes `arcc verdict` with `pass` and `fail` respectively and never reruns `arcc check`.

2. **Grep and golden tests consume the same report**
   - Given a component report, fixed-string expectations, and a canonical report golden
   - When the two assertion rules run
   - Then both read the exact `ArccComponentInfo.report`; missing strings or byte differences produce actionable failures.

3. **Assertion runfiles are minimal**
   - Given any of the three check rules
   - When its runfiles/action inputs are inspected
   - Then they contain only the report and the assertion-specific arcc/golden files, with no SDK sources, manifest/layout, or component source closure added by the test.

4. **Producers run once through the build graph**
   - Given component B depends on component A
   - When B's `.check` is tested
   - Then Bazel builds A's and B's ordinary analysis actions and the test only asserts B's report; no duplicate analysis command exists.

5. **Asserted surfaces cannot satisfy check tests**
   - Given an asserted manual component with no report
   - When a check assertion is attached or explicitly requested
   - Then analysis fails with a clear checked-report requirement rather than passing or invoking a new analysis.

6. **Default builds do not schedule analysis**
   - Given `bazel build //...` without the `arcc` output group
   - When the action graph is inspected
   - Then no component analysis action is requested; `bazel test` of a `.check` or an explicit `--output_groups=+arcc` request does schedule it.

7. **Integration remains green**
   - Given existing pass, expected-violation, grep, and golden fixtures after their minimal report-artifact migration
   - When `just ci` runs
   - Then every suite passes and check verdict behavior is unchanged.

## Metadata
- **Complexity**: Medium
- **Labels**: bazel, starlark, tests, report, build-topology
- **Required Skills**: Bazel test-rule authoring, shell launcher generation, action-graph testing, golden-test maintenance
