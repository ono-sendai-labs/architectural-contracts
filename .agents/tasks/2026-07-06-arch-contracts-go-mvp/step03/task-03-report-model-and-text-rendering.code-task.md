# Task: `report` model + pure text rendering

## Description
Introduce the pure `internal/report` package: the `ConformanceReport` output model
(`Finding`, `Kind`, `Location`, `Evidence`) and a **pure text renderer** that returns a
formatted string. JSON is deliberately **not** rendered here — `encoding/json` reaches
`reflect` and would break `report`'s authority-free claim (review B6); the shell (`cli`)
marshals the struct in Step 8. JSON struct tags are added now so shell-side marshaling
is stable. This package is a dependency-free leaf, fully unit-testable via golden
strings.

## Background
`report` is the checker's output type. It must stay ambient-authority-free, so rendering
returns strings and **never touches stdout** (the shell prints). Keeping JSON out of the
core is a design decision (B6): only the text renderer lives here; the struct carries
JSON tags for the shell's later `encoding/json` marshal.

The `Kind` enumeration is fixed by design §6.2 and must cover both violation kinds and
warning kinds exactly:
- Violations: `UNDECLARED_DEPENDENCY`, `CALLS_UNDECLARED_INTERFACE`,
  `UNDECLARED_AUTHORITY`, `METHOD_OUTSIDE_INTERFACE`, `INIT_OUTSIDE_INTERFACE`,
  `PACKAGE_OVERLAP`.
- Warnings: `ANALYSIS_LIMITATION`, `ALLOWED_WITH_WARNING`, `HIGHER_ORDER_BOUNDARY_CALL`,
  `UNUSED_DEPENDENCY`.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§6.2 report model; §7 for the conforms/violations/warnings outcome distinctions the text render should make legible)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 3)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Create package `internal/report` — pure, no I/O, no `encoding/json` **rendering** call
   (importing the package is unnecessary; do not marshal here), no stdout.
2. Define per design §6.2:
   - `ConformanceReport` (`Component string`, `Violations []Finding`, `Warnings []Finding`).
   - `Finding` (`Kind Kind`, `Message string`, `Location Location`, `Evidence []string`).
   - `Kind` as a named type with the exact violation + warning constants listed above.
   - `Location` (a `file:line` locator — e.g. `File string`, `Line int`).
3. Add JSON struct tags to `ConformanceReport`, `Finding`, and `Location` (and a stable
   string form for `Kind`) so the Step-8 shell marshal is stable — **without** marshaling
   here.
4. Implement a pure text renderer, e.g. `func (r ConformanceReport) RenderText() string`
   (or `func RenderText(r ConformanceReport) string`), that returns a deterministic,
   human-readable string: the component name, each violation and warning with its kind,
   message, location, and evidence frames. A conforming report (no violations, no
   warnings) renders a clear "conforms" line; violations vs warnings are visually
   distinguished. The function returns a string and writes nowhere.

## Dependencies
- None (leaf package; independent of task-01/task-02).

## Implementation Approach
1. Add the model types, `Kind` constants, and JSON tags.
2. Implement `RenderText` with deterministic ordering (render `Violations` then
   `Warnings` in slice order; evidence frames listed under their finding).
3. Golden-string unit tests (design §8):
   - a `ConformanceReport` with one violation (e.g. `UNDECLARED_DEPENDENCY` with a
     `Location`) **and** one warning (e.g. `UNUSED_DEPENDENCY`) renders to an exact
     expected string;
   - a `UNDECLARED_AUTHORITY` finding with multi-frame `Evidence` renders the evidence
     lines;
   - a clean report (no findings) renders the "conforms / ambient-authority-free"-style
     line;
   - assert the rendered output is byte-stable across repeated calls (determinism).

## Acceptance Criteria

1. **Package is pure**
   - Given `internal/report`
   - When built and inspected
   - Then it compiles, performs no I/O, does not write to stdout, and does not marshal
     JSON (rendering is text-only).

2. **Model and Kind match design §6.2**
   - Given the defined types
   - When compared to design §6.2
   - Then `ConformanceReport`, `Finding`, and `Location` carry the specified fields with
     JSON tags, and `Kind` defines exactly the six violation and four warning constants.

3. **Text rendering is exact and deterministic**
   - Given a hand-built report with a violation and a warning
   - When `RenderText` is called
   - Then it returns the exact expected golden string, and repeated calls return
     byte-identical output.

4. **Evidence and clean-pass rendering**
   - Given a report with a multi-frame-evidence violation, and separately a report with
     no findings
   - When rendered
   - Then the evidence frames appear in the output, and the empty report renders a clear
     conforming line.

## Metadata
- **Complexity**: Low
- **Labels**: pure-core, report, rendering
- **Required Skills**: Go
