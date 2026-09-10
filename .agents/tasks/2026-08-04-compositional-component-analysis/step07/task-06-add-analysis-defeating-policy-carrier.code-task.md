# Task: Add Analysis-Defeating Policy Carrier

## Description
Resolve Step 7's residual `UNANALYZED` decision by adding an explicit manifest-carried warn policy for analysis-defeating findings, apply it narrowly to the honest residual components, and remove the final Step 6 exemptions without weakening the stdlib authority map or scanner.

## Background
After protobuf and x/tools code moves behind explicit wrappers, `parsecsv` still references `csv.Reader.ReadAll`, whose interface-parameter indirection is honestly `UNANALYZED`, and `capslockadapter` still owns third-party/Capslock source with a material residual set of `UNANALYZED` and unsafe sites. Capability-use curation would only be valid for a small I2-consistent class of indirection wrappers and cannot account for the broad residual capslockadapter set. The conscious Step 7 decision is therefore to expose the existing DR-11 policy downgrade through the component manifest rather than globally reclassifying those sites as safe.

Strict violation remains the default. The carrier may downgrade `AnalysisDefeating` to `ANALYSIS_LIMITATION`; it must not suppress the finding, allow true ambient authority, or change stdlib-map generation/classifier fingerprints.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (I2, I4, DR-11, §Error Handling, §Findings and evidence)

**Additional References:**
- Plan Step 7: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Residual UNANALYZED evidence: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Map-generation reason UNANALYZED must survive: `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md`
- Step 6 exemption inventory: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step06.yaml`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a versioned manifest schema field for analysis-defeating policy using an enum with a strict/default zero value and an explicit `WARN` value. Use the next available component field number, document the wire/textproto spelling, regenerate protobufs, and reject unknown values at parse time.
2. Extend the pure manifest model with the policy value and map it in the application layer to `capanalyzer.CapabilityPolicy{Warn: map[string]bool{"": true}}` only for `WARN`. Preserve `StrictPolicy` for omitted/default policy. Do not place shell behavior in `manifest` or `capanalyzer`.
3. Add a matching `go_component` attribute with a closed vocabulary (`strict`/`warn` or the schema-equivalent spellings), emit the manifest field only for the explicit non-default value, and fail Bazel analysis on any other input. Keep checked-in manifests and generated manifests parity-equivalent.
4. Limit the carrier to `AnalysisDefeating` findings, which use the empty capability key. A true authority capability such as FILES/NETWORK must still require `declared_authority`; the new field must not allow it or turn it into a warning.
5. Preserve all DR-17 evidence. A downgraded map-UNANALYZED or bypass finding must remain in the report as one `ANALYSIS_LIMITATION` warning with its class, sorted full site list, first-site evidence/SDK key rules, and unchanged source locations.
6. Apply the explicit warning policy to `parsecsv` and `capslockadapter` using their native/Bazel manifest sources as applicable. Remove parsecsv's `check_tags = ["manual"]`, restore capslockadapter's native selfcheck, and delete their exemption TODOs/count comments. Do not apply the policy to unrelated components merely to make a gate green.
7. Add end-to-end tests showing `csv.Reader.ReadAll` and representative residual capslockadapter sites violate under the default policy and become visible warnings only under the explicit manifest field. Include `//go:linkname`, assembly, or cgo coverage proving the same carrier handles scanner bypasses consciously rather than silently.
8. Add a negative regression showing a component with the warn policy and undeclared FILES authority still fails. Add parse/schema round trips, stale/unknown enum rejection, deterministic report encoding, and Bazel attribute/manifest-emission tests.
9. Record the Step 7 decision in the design/plan-adjacent documentation: the policy carrier was selected because the post-wrapper residual is not exhausted by I2-consistent capability-use curation. Keep future targeted curation possible, but do not add broad SAFE overrides or change `classifier_hash` in this task.
10. Audit the Step 6 AC8b set and remove all five exemptions and their TODOs: protobuf-related `artifactio`/`manifest` were handled by Task 4, `goanalysis` by Task 5, and `parsecsv`/`capslockadapter` here. The exemption set must become empty, not move to another tag, skip list, or CI condition.
11. Update README policy documentation, CLI/report help, Component Contract blocks, manifests/BUILD files, generated code, and run `just gen-is-clean` and `just ci` before committing.

## Dependencies
- Task 3 provides surface-backed checks and status reporting.
- Task 4 removes protobuf-owned findings from consuming components.
- Task 5 removes x/tools-owned findings and records capslockadapter's true residual set.
- The existing checker policy path already supports `Warn[""]`; this task exposes it through a durable user-facing carrier.

## Implementation Approach
1. Pin strict-versus-warn parser and checker behavior with failing tests, then add the schema/model field and application mapping.
2. Add the Bazel attribute and manifest emitter validation with parity tests.
3. Apply the policy only to the two residual components, restore their checks, and verify true authority remains strict.
4. Remove all exemption residue, document the decision and residual evidence, regenerate code, and run the full gate.

## Acceptance Criteria

1. **Strict remains the default**
   - Given a manifest that omits the new field and reaches an `UNANALYZED` stdlib symbol or analysis-defeating bypass
   - When it is checked
   - Then the finding remains a violation and the report verdict is fail

2. **Explicit warn policy remains visible**
   - Given the same component with the manifest policy set to `WARN`
   - When it is checked
   - Then the finding becomes an `ANALYSIS_LIMITATION` warning retaining class, sites, SDK key/evidence, and source locations, and is never silently omitted

3. **True authority cannot use the carrier**
   - Given a component that selects WARN but references undeclared FILES or NETWORK authority
   - When it is checked
   - Then the true-authority finding remains an `UNDECLARED_AUTHORITY` violation unless separately declared

4. **Native and Bazel policy declarations agree**
   - Given checked-in and Bazel-generated parsecsv manifests plus invalid attribute/schema values
   - When parity and analysis tests run
   - Then the valid manifests produce the same policy and invalid values fail early with actionable errors

5. **Residual components run normally**
   - Given parsecsv and capslockadapter after the wrapper migrations
   - When their Bazel/native checks run as applicable
   - Then they complete without exemption, report their honest residuals as warnings, and retain all other strict violations

6. **No Step 6 exemption survives**
   - Given repository-wide searches of `check_tags`, selfcheck comments, skip lists, and AC8b TODOs
   - When Step 7 completes
   - Then none of the five components is exempted and no replacement bypass was introduced

7. **Classifier semantics are unchanged**
   - Given the pinned stdlib map and generation classifier tests
   - When the repository gates run
   - Then `UNANALYZED` records, I2 minting-site rules, classifier hash, and fail-closed map behavior are unchanged; only manifest-directed report policy differs

## Metadata
- **Complexity**: High
- **Labels**: go, protobuf, manifest, policy, analysis-defeating, reports, bazel, selfcheck
- **Required Skills**: Protobuf schema evolution, Go policy plumbing, Starlark public API design, deterministic reporting, integration-test maintenance
