# Task: Report Analysis-Defeating Sites

## Description
Detect member constructs that bypass typed reference analysis and complete the DR-17 authority finding model: one policy-aware finding per `(capability, class)`, with all sorted source sites and stdlib-map evidence persisted deterministically.

## Background
An AST/reference scan is intentionally syntactic. `//go:linkname`, assembly, and cgo can create edges that do not appear in ordinary `types.Info`, so they must never silently pass. The design models them as `AnalysisDefeating`, which is a violation under strict policy and may become an `ANALYSIS_LIMITATION` warning only through explicit policy. Separately, repeated references to one capability should form one finding rather than noisy duplicates; text shows the first site and count, while canonical JSON retains all sites, class, SDK key, and evidence.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (DR-11, DR-17, Error Handling, Analysis-defeating acceptance row)

**Additional References:**
- `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md` (UNANALYZED preservation and evidence provenance)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Inspect member-package inputs for `//go:linkname` directives, assembly files, and cgo use. Detection must work in native and layout modes using declared package/file metadata; it may not walk undeclared directories or scan non-member closure files.
2. Represent each bypass as a site-bearing `AnalysisDefeating` capability observation. Use the existing capability policy machinery so strict/default policy is a violation and an explicit warn policy renders `ANALYSIS_LIMITATION`; never downgrade automatically.
3. Preserve stdlib map `UNANALYZED` as the same class while retaining its referenced `SymbolID`, site, map key, and any available evidence. Do not pass it through the generation-time Capslock classifier again.
4. Aggregate authority observations once per `(capability, class)` per component. Sort sites by file, line, and referenced `SymbolID`, remove only exact duplicates, and select the first sorted site for evidence lookup and text rendering.
5. Extend `report.Finding` and the canonical persisted report representation as needed so JSON carries every source site (file, line, referenced symbol), the classification class, the stdlib map SDK key, and the evidence frames for the first site's `(symbol, capability)`.
6. Render an aggregated authority finding as one text entry containing the first source site and total site count; a three-site finding must not render three top-level lines. Boundary findings remain one per `(referent, site)` and must not be aggregated under this rule.
7. Update `artifactio` canonical sorting/comparison and round-trip tests for every new nested collection and field. Reordered input observations must produce byte-identical report artifacts.
8. Keep report and checker core packages authority-free and reflection-free; JSON conversion remains in `artifactio`. Update Component Contracts, BUILD files, and manifests for changed DTO dependencies.

## Dependencies
- Task 01: typed sites and member-only package/file scope.
- Task 03: classified stdlib capability observations with evidence and SDK key.
- Existing `capanalyzer` class/policy types and Step 5 canonical report artifact.

## Implementation Approach
1. Add an analysis-limitation scan alongside typed edge collection using only loaded member metadata and syntax/comments.
2. Introduce a rich but pure authority observation/site DTO, then aggregate it deterministically before policy decisions.
3. Extend text and artifact renderers without changing the verdict rule: strict `AnalysisDefeating` remains a violation.
4. Add focused fixtures for each bypass plus report tests with deliberately shuffled three-site inputs.

## Acceptance Criteria

1. **All bypass constructs fail closed by default**
   - Given member fixtures containing `//go:linkname`, an assembly file, and cgo use
   - When each is checked with strict/default policy
   - Then each yields an `AnalysisDefeating` violation with an actionable source/file site, and none silently conforms

2. **Policy can explicitly downgrade analysis defeat**
   - Given the same fixture and an explicit policy classifying its analysis-defeating capability as warn
   - When the checker runs
   - Then the result is one `ANALYSIS_LIMITATION` warning and no corresponding authority violation

3. **UNANALYZED map entries use the same policy path**
   - Given a typed reference whose exact stdlib-map record is `UNANALYZED`
   - When classification and checking run
   - Then the observation retains `AnalysisDefeating`, its reference site and symbol, and is a violation by default

4. **Authority sites aggregate deterministically**
   - Given three member references that produce the same `(capability, class)` in shuffled observation order
   - When a report is built
   - Then it contains one finding, its sites are sorted and all three appear in canonical JSON, and text prints the first site plus a count of three

5. **Evidence follows the first canonical site**
   - Given multiple symbols contributing the same capability with different map evidence
   - When aggregation sorts their sites
   - Then persisted evidence is the map evidence for the first sorted site's symbol, and the map SDK key and class are present

6. **Boundary findings remain site-specific**
   - Given repeated undeclared dependency or interface references at distinct sites
   - When findings are produced
   - Then each distinct `(referent, site)` remains its own finding and is not merged by capability aggregation

7. **Canonical artifacts remain stable**
   - Given logically equal reports with authority sites, SDK-key fields, and evidence frames supplied in different order
   - When `artifactio.MarshalReport` is called
   - Then bytes are identical, decode round-trips all fields, and `just ci` passes

## Metadata
- **Complexity**: High
- **Labels**: go, analysis-defeating, policy, report, evidence, canonical-json, determinism
- **Required Skills**: Go source metadata, policy modeling, report schema evolution, canonical serialization testing
