# Task: Enforce self-check in CI

## Description
Turn the verified self-hosting commands into a deterministic `just selfcheck` target and dedicated CI job, with regression coverage proving that ambient authority introduced into the pure core makes the check fail.

## Background
Self-hosting becomes an architectural guardrail only when it is easy to run locally and mandatory in automation. The Step 10 demo requires both the positive proof—all components conform—and the negative proof—a filesystem touch in the core turns its own check red. A separate CI job makes this architectural signal visible.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§2 FR8, §3, §8)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 10 tests, integration, and demo)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a deterministic `just selfcheck` recipe that invokes the repository's own `arcc` on all eight tool manifests.
2. Make it fail immediately on any structural or authority violation and work from the repository root in a clean checkout.
3. Make the authority-free core checks, especially `go/internal/checker/component.textproto`, explicit in recipe output.
4. Add a dedicated self-hosting job to `.github/workflows/ci.yml` with the Go and `just` setup needed to run `just selfcheck`.
5. Add a hermetic regression test or fixture that introduces representative filesystem authority into an otherwise pure core component and asserts failure without mutating tracked production sources.
6. Keep `just ci` green and decide explicitly whether it should depend on `selfcheck`; preserve the dedicated CI signal.
7. Do not mark Step 10 complete until this final task is implemented and committed by `task-to-code`.

## Dependencies
- Task 2, which makes every component conform and establishes the exact manifest list.
- Existing `justfile`, GitHub Actions workflow, CLI integration conventions, and fixtures.

## Implementation Approach
1. Encode the successful manual sequence as `just selfcheck` with readable per-component output.
2. Create a fixture-based negative test exercising the real analysis/checker path against filesystem authority.
3. Add the workflow job and align local/CI entry points.
4. Run `just selfcheck`, the regression, the checker demo, and `just ci`.

## Acceptance Criteria

1. **Developers can self-check the whole tool**
   - Given a clean checkout with Go and `just`
   - When `just selfcheck` runs from the repository root
   - Then all eight manifests are checked and the command exits 0 only when all conform.

2. **The pure checker demo is visible**
   - Given `go/internal/checker/component.textproto`
   - When checked directly or through `just selfcheck`
   - Then output states that the checker conforms and is ambient-authority-free.

3. **Architectural regressions turn the check red**
   - Given a hermetic fixture equivalent to adding filesystem access to a pure component
   - When the real path checks it
   - Then it exits nonzero with `UNDECLARED_AUTHORITY` and useful evidence.

4. **CI enforces self-hosting separately**
   - Given a push or pull request
   - When GitHub Actions runs
   - Then a clearly named job executes `just selfcheck` and fails on any violation.

5. **All quality gates pass**
   - Given completed Step 10 work
   - When `just selfcheck` and `just ci` run
   - Then both complete successfully.

## Metadata
- **Complexity**: Medium
- **Labels**: self-hosting, CI, regression-test, FR8, developer-tooling
- **Required Skills**: Just, GitHub Actions, Go integration testing, Capslock analysis
