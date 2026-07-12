# Task: Complete pruning showcase

## Description
Complete the CSV composition example and CLI golden tests that demonstrate boundary enforcement, component pruning, init pruning, higher-order warnings, and the contrast with absorption.

## Background
The Step 9 demo is the MVP's full concept: `app` composes a `FILES`-holding `csvfile` component but remains ambient-authority-free because authority is pruned at the declared component boundary. The Step 8 `absorbapp` provides the contrasting unpruned result.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§8, §10, §11)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 9 demo and tests)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (validated CSV pruning transcript)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Finish `examples/csvtool/app` as the composition root with manifests declaring `csvfile` and `toprow` as component dependencies and no ambient authority.
2. Preserve informal contract comments in interface files describing behavior, requirements, provided guarantees, and authority obligations.
3. Add a golden test proving `arcc check examples/csvtool/app/component.textproto` conforms with no `FILES` finding while `csvfile` itself legitimately declares `FILES`.
4. Retain and assert the Step 8 `absorbapp` contrast: absorbed `csvfile` authority is attributed to the app and fails without a declaration.
5. Add a fixture/variant that calls a Go-exported but architecture-private `csvfile` symbol and assert `CALLS_UNDECLARED_INTERFACE` with exit 1.
6. Add an authority-using dependency-init fixture: its dependent remains clean because init is pruned, while checking the dependency itself surfaces or requires that authority.
7. Add a higher-order boundary fixture and assert `HIGHER_ORDER_BOUNDARY_CALL` is a warning with exit 0.
8. Cover interface/concrete dynamic dispatch, generic normalization, and `types.go`/`api.go` split in the integrated fixture set where not already covered by Tasks 1–2.
9. Document any reproducible VTA over-approximation as a named skipped/xfail test and limitation; do not manufacture one if none surfaces.

## Dependencies
- Tasks 1–3, which provide real call edges, resolved dependency interfaces, CLI wiring, and Capslock pruning.
- Existing Step 8 CSV components, absorb variant, CLI integration harness, and golden conventions.

## Implementation Approach
1. Refine the example manifests and app code into a realistic component-dependency composition.
2. Add focused test fixtures for undeclared boundary calls, init authority, and callbacks.
3. Extend the CLI integration/golden suite to assert report kinds, severities, exit codes, and absence/presence of authority.
4. Run the real demo command and the complete repository CI suite.

## Acceptance Criteria

1. **The composition root is authority-free**
   - Given `app` calls declared interfaces of `csvfile` and `toprow`
   - When its manifest is checked
   - Then it exits 0 and reports conformance without attributing `FILES` to `app`.

2. **Component dependency and absorption visibly differ**
   - Given equivalent composition through a component dependency and through an absorbed dependency
   - When both are checked without declaring `FILES`
   - Then the component-dependent app conforms and the absorbed variant reports undeclared authority.

3. **Architecture-private calls fail**
   - Given an app variant calls an exported `csvfile` function absent from its interface files
   - When it is checked
   - Then it exits 1 with `CALLS_UNDECLARED_INTERFACE` naming useful caller, callee, and component evidence.

4. **Dependency init authority is not re-absorbed**
   - Given a dependency with authority-using init declared in its interface
   - When the dependent and dependency are checked separately
   - Then the dependent stays clean through init pruning and the dependency's own report accounts for the authority.

5. **Higher-order risk is visible but non-fatal**
   - Given a function value passed into a pruned declared interface
   - When the caller is checked
   - Then `HIGHER_ORDER_BOUNDARY_CALL` is reported as a warning and the exit status remains 0.

6. **The Step 9 demo and full CI pass**
   - Given the completed example and tests
   - When the documented `arcc check` demo and `just ci` run
   - Then the demo matches the Step 9 expected output and all checks pass.

## Metadata
- **Complexity**: High
- **Labels**: examples, golden-tests, FR5, FR5b, FR7, FR9, demo
- **Required Skills**: Go, CLI integration testing, Capslock analysis, test-fixture design, technical documentation
