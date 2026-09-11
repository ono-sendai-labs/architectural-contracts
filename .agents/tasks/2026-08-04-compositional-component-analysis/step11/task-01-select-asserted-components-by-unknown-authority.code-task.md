# Task: Select Asserted Components by Unknown Authority

## Description
Replace the transitional semantic use of Bazel's `manual` tag with an explicit `authority: UNKNOWN` component attribute. An unknown-authority package-surface component must publish the existing analysis-time asserted surface, create no analysis action or report, and remain visibly `untrusted` at every dependency boundary; ordinary tags must return to being scheduling metadata only.

## Background
The authority schema, native `AuthorityDeclaration` lattice, canonical surface encoding, structural checked/asserted producer split, and the report's provenance/freshness/authority axes already exist. During Steps 5–10, however, `go_component` selects the asserted producer by finding `manual` in the component's generic tags and synthesizes `authority: UNKNOWN` into the generated manifest. That stand-in overloads test scheduling with a trust decision and forces hosts to patch package-name matches into `defs.bzl`.

Step 11 makes the authority declaration the single source of truth. `DECLARED` remains the backward-compatible default and always selects the checked path. Explicit `UNKNOWN` selects the structurally asserted path, whose package-level surface has no symbols or digest and whose provider has no report. Because the asserted writer has no type information, design invariant I6 still requires unknown components to use `PACKAGE_SURFACE`; this task does not add an asserted declared-interface implementation. The tool enforces visibility of unknown trust, while approval of which components may be unknown remains the documented governance assumption from DR-13; the deferred Q7 whole-tree predicate is not implemented here.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R8, R10–R11, I1, I1′, I6, §Build topology, §Provenance/freshness and authority, acceptance-matrix Unknown dependency row)

**Additional References:**
- Plan Step 11: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Authority decisions Q7–Q9: `.agents/planning/2026-08-04-compositional-component-analysis/idea-honing.md`
- Design review response DR-06 and DR-13: `.agents/planning/2026-08-04-compositional-component-analysis/2026-09-02-design-review-response.md`
- Host manual-tag workaround: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md` (§5)
- Transitional asserted producer: `bazel_rules/go/private/component.bzl`, `bazel_rules/go/defs.bzl`, and `bazel_rules/go/tests/asserted_surface_tests.bzl`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a public, non-configurable `authority` attribute to `go_component`, with explicit `DECLARED` and `UNKNOWN` spellings/constants and `DECLARED` as the backward-compatible default. Reject every other value at the public macro boundary and again in the private rule implementation so direct rule/test-helper use also fails closed.
2. Make the attribute the only selector between producers. `DECLARED` must generate the normal manifest declaration and run exactly one checked `ArccCheck` action producing a report and surface. `UNKNOWN` must emit `authority: UNKNOWN` in the generated textproto and reuse the analysis-time asserted-surface writer with `report = None`, `provenance = "asserted"`, and no `ArccCheck`, ordinary-import projection, layout merge, or stdlib-export-data action needed solely for checking the unknown component.
3. Preserve design I6 for asserted output: an `UNKNOWN` component must be `PACKAGE_SURFACE`, must have concrete non-empty members, and its canonical surface must contain those sorted packages, namespace, complete target SDK key, format version, producer version, and structural `UNKNOWN` authority, with no symbols and an empty/omitted digest. Reject an `UNKNOWN` declared-interface component with an actionable error naming the target and the package-level requirement.
4. Reject `authority = UNKNOWN` together with non-empty `declared_authority` at Bazel analysis time, in addition to retaining the Go manifest parser's existing rejection. Do not silently discard or normalize declarations. A `DECLARED` component with an empty declaration continues to mean verified authority-free code, not unknown code.
5. Remove all semantic inspection of generic `tags` from the production macro, private rule, and test component helper. A raw `tags = ["manual"]` on a `DECLARED` component may exclude targets from wildcard commands but must not alter its manifest authority, suppress its analysis action/report, change provider provenance, or omit its generated `.check`; `check_tags` continues to control only the generated check target.
6. Migrate every genuine asserted wrapper and asserted-path fixture from `tags = ["manual"]` to `authority = UNKNOWN`, including the protobuf-runtime and x/tools package-surface components. Audit remaining raw `manual` tags and comments so none claims that the tag affects trust, authority, provenance, or producer selection; preserve legitimate scheduling-only tags and negative-fixture behavior.
7. Update the asserted-producer analysis tests and macro-shape tests to cover default/explicit `DECLARED`, explicit `UNKNOWN`, invalid authority values, unknown-with-declarations, unknown declared-interface rejection, absence of an analysis action/report/check target, canonical asserted surface content, and the fact that a manual-tagged declared component remains checked.
8. Add or update an end-to-end dependent fixture demonstrating the Step 11 contract: crossing an asserted `UNKNOWN` package-surface boundary succeeds when the package is declared, renders `untrusted` in text, persists `authority: UNKNOWN` in JSON, retains `ASSERTED` provenance independently, and never renders or serializes that boundary as certified/declared. Keep the existing Go-level `UNKNOWN` round-trip and `Join(UNKNOWN, DECLARED{}) = UNKNOWN` coverage intact.
9. Update public rule documentation, README/self-check inventory, fixture comments, and Component Contract metadata affected by the selector change. State I1 as the enforced half (checked or explicitly `UNKNOWN` and visibly untrusted) plus the external governance assumption that approval of unknown components remains a review/ownership process until the deferred Q7 predicate lands. Remove transitional Step 11 notes and any guidance teaching `manual` as an authority declaration.
10. Preserve structural provenance: do not add a checked/asserted flag to the surface schema, infer trust from the surface's authority field alone, manufacture a passing report for an unknown component, or weaken report validation that requires checked providers to have reports and asserted providers not to have them.

## Dependencies
- Step 3 supplies the manifest authority enum, pure `AuthorityDeclaration` lattice, canonical authority encoding, and UNKNOWN round-trip validation.
- Step 5 supplies the analysis-time package-level asserted writer and the checked/asserted provider split currently selected by the transitional `manual` tag.
- Step 7 supplies validated surface consumption and the independent report boundary axes, including `untrusted` rendering and structural provenance checks.
- Steps 8–10 must remain green; this task changes producer selection and authoring only, not export-data loading, golden structure, or unused-authority semantics.
- No later Step 11 task dependency; the selector cutover, wrapper migration, boundary proof, and governance documentation form one atomic change.

## Implementation Approach
1. Add authority constants/validation to the public macro and private rule, then pin default, explicit-declared, explicit-unknown, and invalid combinations in analysis tests before changing producer dispatch.
2. Replace every `"manual" in tags` producer/check decision with the validated authority value while retaining the existing I6 asserted writer and checked action implementations.
3. Migrate genuine wrappers and asserted fixtures, then audit all remaining `manual` uses as scheduling-only metadata and add a regression proving such a tag cannot create an asserted component.
4. Exercise a dependent of the migrated unknown fixture through report text and canonical JSON so authority, provenance, and certification language remain orthogonal and visible.
5. Reconcile public documentation and self-check inventories with the explicit attribute and the DR-13 governance boundary, then run the full repository gate.

## Acceptance Criteria

1. **Explicit authority selects exactly one producer path**
   - Given otherwise equivalent components with omitted/default `DECLARED`, explicit `DECLARED`, and explicit `UNKNOWN` authority
   - When Bazel analyzes them
   - Then both declared components produce one checked report/surface pair through `ArccCheck`, while the unknown component produces only one asserted surface and has no analysis report or check action.

2. **Unknown authority is persisted without becoming empty authority**
   - Given a valid `UNKNOWN` package-surface component
   - When its generated manifest and asserted surface are decoded and round-tripped
   - Then both carry structural `UNKNOWN`, carry no declared capabilities, and never normalize to `DECLARED{}`.

3. **Contradictory or unsupported declarations fail closed**
   - Given an unknown component with `declared_authority`, an unknown declared-interface component, or a component with an unsupported authority value
   - When macro or direct-rule analysis runs
   - Then it fails with an actionable diagnostic naming the component and violated authority/package-surface rule, without producing a partial checked or asserted artifact.

4. **Generic manual tags no longer carry trust semantics**
   - Given a `DECLARED` component whose ordinary tags contain `manual`
   - When its provider, generated manifest, actions, and generated targets are inspected
   - Then it remains declared and checked, has a report and `.check`, and is never given asserted provenance solely because of the tag.

5. **Asserted output remains package-level and deterministic**
   - Given the same valid unknown wrapper under repeated analysis with reordered member declarations
   - When its surface bytes and provider are inspected
   - Then packages are sorted and duplicate-free; symbols and digest are absent/empty; namespace, SDK key, format, and producer version are complete; provenance is `asserted`; and bytes are identical.

6. **Unknown boundaries are visibly untrusted and never certified**
   - Given a checked component depending on an `UNKNOWN` package-surface wrapper
   - When the dependent report is rendered and decoded
   - Then the dependency edge is accepted, text includes `untrusted`, JSON contains `authority: UNKNOWN` with structural `ASSERTED` provenance, and neither representation calls the boundary certified or declared.

7. **Repository wrappers and test tags are correctly migrated**
   - Given the protobuf-runtime and x/tools wrappers plus all remaining repository uses of `manual`
   - When manifests, providers, action graphs, and fixture comments are audited
   - Then the wrappers select assertion through explicit `UNKNOWN`; every other manual tag is scheduling-only; and no package-name string match or generic-tag inspection controls verification status.

8. **I1 enforcement and governance are documented accurately**
   - Given the public authoring documentation
   - When a maintainer reads how to adopt unowned code
   - Then it teaches `PACKAGE_SURFACE` plus explicit `authority = UNKNOWN`, explains the visible `untrusted` result, distinguishes `UNKNOWN` from empty declared authority, and states that approval remains external governance with the Q7 predicate deferred.

9. **Integration remains green and the release constraint lifts**
   - Given all repository components after the selector cutover
   - When `just ci`, self-check, manifest parity, Bazel analysis tests, and report artifact tests run
   - Then they pass with every component either checked or explicitly unknown/untrusted, and no transitional manual-based authority path remains.

## Metadata
- **Complexity**: High
- **Labels**: bazel, starlark, authority, asserted-surface, provenance, reporting, documentation
- **Required Skills**: Bazel macro/rule authoring, Starlark analysis testing, protobuf text/JSON artifact validation, Go integration testing, architectural documentation
