# Task: Restore Lint to Aggregate CI Gates

## Description
Restore the existing Go lint recipe to the shared aggregate CI path so the local
`just ci` pre-commit gate and GitHub's `ci-go` job both enforce `go vet` and
`gofmt` as documented.

## Background
Addresses finding F1 from the plan-scoped implementation review. The repository
has a working `lint` recipe, and `just lint` currently passes, but `ci-go` and
`ci` no longer depend on it. AGENTS.md requires `just ci` before each commit,
README.md describes that command as building, linting, and testing, and the
generated coding-style guide says lint is part of the pre-commit gate. GitHub
calls `just ci-go`, so the omission bypasses lint in both advertised aggregate
entry points.

History indicates that the omission landed while moving `gen-is-clean` out of
the GitHub Go job; removing lint was not part of that change's stated purpose.
The repair should share one recipe dependency rather than duplicate vet/gofmt
commands in workflow YAML.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (N5 and validation topology)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review.yaml`

**Additional References:**
- `AGENTS.md`
- `README.md` (Building and Installation; Development and Contributing)
- `justfile` (`lint`, `ci-go`, and `ci` recipes)
- `.github/workflows/ci.yml`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Make the shared aggregate Go validation path depend on the existing `lint` recipe so both `just ci-go` and `just ci` execute `go vet ./...` and require an empty `gofmt -l .` result.
2. Keep `gen-is-clean` local to the full `ci` recipe unless a separate requirement justifies changing the GitHub generation policy; this task must not undo the intentional split that keeps GitHub's `ci-go` from requiring Jujutsu.
3. Keep GitHub's workflow invoking the shared Just recipe rather than copying lint commands into `.github/workflows/ci.yml`.
4. Preserve the three-lane GitHub topology (`ci-go`, `selfcheck`, and `bazel`) and all current routine/full stdlib-map lane boundaries.
5. Reconcile adjacent comments or public CI documentation only if necessary to describe the final recipe graph accurately.

## Dependencies
- Steps 6 and 13 established the bounded routine integration and stdlib-map topology that `ci-go` must preserve.
- No dependency on the other Step 14 tasks.

## Implementation Approach
1. Capture `just --dry-run ci-go` and `just --dry-run ci` before the change to demonstrate that lint commands are absent.
2. Add `lint` at the shared recipe dependency level that reaches both local and GitHub aggregate entry points without duplicating commands.
3. Re-run the dry-run expansion and verify `go vet` and the gofmt cleanliness check appear in each aggregate path while `gen-is-clean` remains excluded from `ci-go`.
4. Run `just lint` and `just ci`.

## Acceptance Criteria

1. **Local aggregate CI includes lint**
   - Given the repository's Just recipes
   - When `just ci` is expanded or executed
   - Then it runs `go vet ./...` and fails when `gofmt -l .` reports a Go file.

2. **GitHub's Go entry point includes the same lint gate**
   - Given `.github/workflows/ci.yml` continues to invoke `just ci-go`
   - When `just ci-go` is expanded or executed
   - Then it reaches the shared `lint` recipe without duplicating vet/gofmt shell commands in the workflow.

3. **Generated-file policy remains intentionally split**
   - Given the local and GitHub aggregate recipes
   - When their dependency graphs are inspected
   - Then `gen-is-clean` remains part of full local `just ci`, while `ci-go` does not acquire a Jujutsu dependency.

4. **Existing validation topology is preserved**
   - Given the completed recipe change
   - When `just ci` runs
   - Then generation, build, unit/integration tests, stdlib-map pin validation, staging, selfcheck, and Bazel tests still pass with the restored lint gate.

## Metadata
- **Complexity**: Low
- **Labels**: remediation, ci, go, lint, just, github-actions
- **Required Skills**: Just recipe composition, GitHub Actions reading, Go vet/gofmt validation
