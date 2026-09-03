# Task: Measure Current Analysis Phases

## Description
Measure `arcc check` on a real, closure-heavy component and record how wall-clock and CPU cost divide across the five phases that the compositional redesign will keep or remove. Replace the unresolved attribution in the current-pipeline research note with reproducible evidence before structural implementation begins.

## Background
The existing research establishes that analysis cost follows the dependency closure, but it does not distinguish member-package loading from closure type-checking, SSA, VTA, and Capslock's duplicate work. The redesign retains member parsing and type-checking while eliminating the other four categories, so this attribution is the baseline used to evaluate the later performance result. Measurement scaffolding may be temporary; production behavior and report output must not change.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md`

**Additional References:**
- Current pipeline and existing measurements: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Plan Step 1: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Select and identify a real component with a meaningfully large dependency closure; prefer the repository's `internal/goanalysis` component unless another available component better exercises the pipeline.
2. Build the measured `arcc` binary once and run repeated checks against the same revision and component so compilation time is not included and warm-up effects are visible.
3. Attribute both wall-clock and CPU cost, as far as the available profiler permits, to: member `packages.Load` and type-checking; closure type-checking beyond members; SSA construction; VTA; and Capslock's second load plus analysis.
4. Keep measurement-only instrumentation out of the final production code. If exact CPU separation is not supported at a boundary, document the aggregation method and uncertainty rather than presenting inferred precision as measured fact.
5. Add a `Measured attribution` section to `research/current-analysis-pipeline.md` containing revision, component, closure or workload size, environment/toolchain, commands or procedure, run count, raw or summarized observations, the five-phase attribution, and the conclusion relevant to the redesign.
6. Reconcile or clearly distinguish the existing 2.1-second and 3.8-second broad baseline figures; do not silently replace historical measurements taken under different conditions.

## Dependencies
- No prior Step 1 task is required.
- Requires a working Go toolchain and the current `arcc check` path.

## Implementation Approach
1. Map the five requested boundaries to the current `goanalysis`, SSA/VTA, and Capslock adapter call sites.
2. Use repeatable profiling and, where necessary, temporary timing instrumentation to collect multiple samples from one built binary.
3. Remove temporary instrumentation, confirm the production diff contains only the research result (and any intentionally retained measurement fixture or script), and run the relevant checks.
4. Write the measured attribution and a concise interpretation under the existing unresolved section in the current-pipeline research note.

## Acceptance Criteria

1. **Reproducible baseline**
   - Given a reader at the recorded repository revision with the stated toolchain and component
   - When they follow the documented build and measurement procedure
   - Then they can reproduce the workload and understand the run count, warm-up policy, and reported units.

2. **Five-phase attribution**
   - Given the recorded measurements
   - When the `Measured attribution` section is reviewed
   - Then it reports wall-clock and CPU attribution for all five requested categories, explicitly marking any profiler-imposed aggregation or uncertainty.

3. **Decision-relevant conclusion**
   - Given that the redesign keeps member loading and removes closure type-checking, SSA, VTA, and check-time Capslock
   - When the findings are summarized
   - Then the note states which phase dominates and what fraction or range of the measured workload is expected to remain versus be eliminated.

4. **No production behavior change**
   - Given any temporary measurement hooks used during collection
   - When the task is complete
   - Then those hooks are removed, normal `arcc check` output is unchanged, and `just ci` passes.

## Metadata
- **Complexity**: Medium
- **Labels**: research, performance, baseline, go
- **Required Skills**: Go profiling, performance measurement, technical documentation
