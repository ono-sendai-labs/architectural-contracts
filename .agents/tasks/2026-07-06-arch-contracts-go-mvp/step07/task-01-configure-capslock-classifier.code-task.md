# Task: Configure Capslock classifier

## Description
Add the pinned Capslock library dependency and create the classifier foundation
for `internal/capslockadapter`. Build the exact Step-0-validated classifier that
excludes `UNANALYZED` noise and treats `(*os.File)` handle operations as granted
capability use while retaining filesystem minting and process-global operations
as ambient authority.

This task isolates the load-bearing classifier policy so it can be reviewed and
tested before the adapter begins mapping analysis results.

## Background
Capslock's builtin classifier treats common stream/callback helpers as
`UNANALYZED` leaves and classifies both filesystem capability minting and use as
`FILES`. The Step-0 spike established two deliberate overrides: descend through
`UNANALYZED` helpers, and mark the exact set of 22 `(*os.File)` handle-use methods
`CAPABILITY_SAFE`. `(*os.File).Chdir` is intentionally not safe because it mutates
process-global state.

Capslock is pre-1.0 and its classifier behavior is part of this project's
security model, so the module version must be pinned and upgrades must remain
explicit review events. Boundary pruning supplied through `AnalyzeRequest.PruneAt`
is deferred to Step 9; the classifier construction should leave a clear seam for
merging those future safe keys into the same per-run map.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§5.4a classifier configuration; §8 adapter tests; Appendix C spike conclusions)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 7)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (Findings 0 and 0b, validated classifier behavior)
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike/harness.go` (`fileHandleUseMethods` and `ocapClassifier` reference implementation)
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/capslock.md` (custom classifier API and `CAPABILITY_SAFE` semantics)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Pin a specific released `github.com/google/capslock` version in `go/go.mod`
   and commit the corresponding `go.sum` updates. Do not use a floating branch,
   local replace directive, or subprocess invocation.
2. Create `go/internal/capslockadapter` and a classifier-construction helper that
   merges Capslock's builtins with project overrides using
   `interesting.LoadClassifier(..., excludeBuiltin=false)`, then wraps the result
   with `interesting.ClassifierExcludingUnanalyzed`.
3. Carry the exact 22 `fileHandleUseMethods` keys from the Step-0 spike and emit
   one `func <key> CAPABILITY_SAFE` entry per method. Keep the list deterministic
   and documented as a deliberate object-capability policy.
4. Deliberately exclude `(*os.File).Chdir` from the safe list. Do not mark path-
   or fd-based minting operations such as `os.Open`, `os.ReadFile`, `os.Create`,
   or `os.NewFile` safe.
5. Keep classifier creation per analysis run and structure it so Step 9 can add
   boundary-prune safe keys to the same generated capability map without replacing
   either current override.
6. Return classifier parse/construction failures as descriptive errors; do not
   silently fall back to Capslock defaults.
7. Add focused package tests that inspect classifier categories for representative
   handle-use, minting, `.Chdir`, and `UNANALYZED` functions. Tests must protect
   the complete 22-method set against accidental additions, omissions, or duplicate
   entries.

## Dependencies
- Step 3 `internal/capanalyzer` defines the port but need not be imported by the
  classifier helper yet.
- Step-0's preserved harness is the source of truth for the safe method list and
  classifier composition.
- Task 2 consumes this task's classifier to run real package analysis.

## Implementation Approach
1. Add and tidy the pinned Capslock module dependency, recording the exact version
   in module metadata.
2. Port the spike's deterministic file-handle method list and capability-map
   generation into an unexported adapter helper.
3. Load the custom map merged with builtins and exclude `UNANALYZED` from the
   resulting classifier.
4. Write table-driven classifier tests over representative keys and a full-list
   invariant test, including negative assertions for `.Chdir` and minting calls.

## Acceptance Criteria

1. **Capslock is pinned as a library**
   - Given the Go module metadata
   - When dependencies are inspected
   - Then a specific released Capslock version is recorded with no local replace
     and the project consumes its Go API rather than its CLI.

2. **UNANALYZED helpers are descended through**
   - Given a builtin helper such as `io.ReadAll` that Capslock normally classifies
     as `UNANALYZED`
   - When the adapter classifier is queried
   - Then `UNANALYZED` is excluded so the helper does not become a reported leaf.

3. **All file-handle use methods are safe**
   - Given the exact 22 `(*os.File)` handle-use keys validated by the Step-0 spike
   - When the adapter classifier is built
   - Then every key is classified `CAPABILITY_SAFE`, with no missing, duplicate,
     or extra policy entries.

4. **Ambient minting remains visible**
   - Given representative filesystem minting functions such as `os.Open` and
     `os.ReadFile`
   - When the adapter classifier is queried
   - Then they retain their `FILES` classification.

5. **Process-global mutation remains visible**
   - Given `(*os.File).Chdir`
   - When the adapter classifier is queried
   - Then it is not safe and retains its system-state-modifying classification.

6. **Classifier failures are explicit**
   - Given malformed generated/custom classifier input in a focused test seam
   - When classifier construction runs
   - Then it returns a descriptive error instead of using a weaker fallback.

7. **Repository checks pass**
   - Given the completed classifier foundation
   - When `just ci` runs
   - Then module, formatting, unit, and integration checks remain green.

## Metadata
- **Complexity**: Medium
- **Labels**: capslockadapter, capslock, classifier, ambient-authority, object-capability, security
- **Required Skills**: Go, Capslock library APIs, capability analysis, table-driven testing
