# Task: CSV example components

## Description
Refine the CSV example into runnable Step 8 components: ambient-authority-free
`toprow` with absorbed CSV parsing, and `csvfile`, which explicitly declares and
exercises `FILES`. Add component-root manifests and informal interface contracts.

## Background
The example must demonstrate both dependency absorption and honest ambient
authority declarations before Step 9 adds component-dependency pruning. The
existing Step-0 drafts are probes, not yet product-quality examples. The future
composition `app` is not part of this task; Step 8 uses a separate failing
`absorbapp` fixture and Step 9 delivers the conforming pruning showcase.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§5.1/§5.4/§5.4a, §6.1, §9–§10)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 8)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/spike-capslock.md` (strict-safe stdlib envelope, `sort` behavior, filesystem attribution)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Refine `go/examples/csvtool/toprow` into deterministic pure logic that consumes parsed CSV rows, sorts/selects the requested top row, and exposes its contract through doc comments in its interface file.
2. Keep CSV decoding in `go/examples/csvtool/internal/parsecsv` with no manifest; it must accept data/reader capabilities supplied by callers and must not open files or mint ambient authority.
3. Add `toprow/component.textproto` at the component root with no declared authority and an absorbed dependency matching the `parsecsv` import path; list the actual interface file.
4. Refine `go/examples/csvtool/csvfile` to expose `Read(path) ([][]string, error)`, open/read the named filesystem path, parse CSV correctly, close resources, and document that real-filesystem access is required/provided.
5. Add `csvfile/component.textproto` at its component root with the actual interface file and `declared_authority: "FILES"`.
6. Keep examples isolated according to the established module/testdata choice so they build and can be loaded by `goanalysis`/Capslock without contaminating the tool's component graph.
7. Add ordinary Go tests for valid rows, selection/sorting edge cases, malformed CSV, filesystem success/failure, and contract-facing behavior. Put authority-using test helpers only in `_test.go`.
8. Ensure both manifests are runnable through the Task 1 CLI; `toprow` must yield no capability findings and `csvfile` findings must be permitted by its declaration.

## Dependencies
- Step 8 Task 1 CLI orchestration.
- Existing Step-0 draft example packages.
- Completed manifest, checker, loader, and Capslock adapter behavior.
- Step 9 will implement the final `app` component and component-boundary pruning.

## Implementation Approach
1. Settle small public APIs and write contract doc comments before refining implementations.
2. Implement and unit-test pure CSV parsing/top-row behavior.
3. Implement and unit-test explicit filesystem minting in `csvfile`.
4. Author manifests with exact module import paths and verify each using `arcc check`.

## Acceptance Criteria

1. **Top-row logic is usable and deterministic**
   - Given valid CSV content and a requested column
   - When the `toprow` interface is invoked
   - Then it returns the documented selected row deterministically without filesystem access.

2. **CSV parsing is an absorbed implementation detail**
   - Given the `toprow` source and manifest
   - When imports are checked
   - Then `internal/parsecsv` is matched by the absorbed dependency and no manifest exists for that helper.

3. **Toprow is ambient-authority-free**
   - Given `toprow/component.textproto`
   - When the real CLI checks it
   - Then the report conforms with no declared or detected ambient authority.

4. **Csvfile honestly declares filesystem authority**
   - Given a real CSV path
   - When `csvfile.Read` runs
   - Then it returns parsed rows or a useful filesystem/CSV error, and its manifest declares `FILES`.

5. **Declared FILES is allowed**
   - Given `csvfile/component.textproto`
   - When the real CLI checks it under the strict base policy
   - Then Capslock's filesystem finding is allowed through `declared_authority` and the report conforms.

6. **Contracts describe the outside view**
   - Given each manifested component's interface file
   - When a consumer reads its exported documentation
   - Then the comments state what the component does, what inputs/obligations it requires, and what authority it provides or lacks.

7. **Examples remain isolated and tested**
   - Given the repository checkout
   - When `just ci` runs
   - Then example unit tests and real analysis integration pass without adding the examples to the tool's own component boundary.

## Metadata
- **Complexity**: Medium
- **Labels**: examples, csv, absorption, FILES, FR7, FR9, FR10
- **Required Skills**: Go, textproto manifests, table-driven testing, object-capability design
