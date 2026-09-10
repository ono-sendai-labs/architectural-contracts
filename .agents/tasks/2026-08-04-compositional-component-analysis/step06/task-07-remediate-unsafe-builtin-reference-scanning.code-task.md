# Task: Remediate Unsafe Builtin Reference Scanning

## Description
Close the Step 6 reference-scan soundness gap for exported compiler builtins in
package `unsafe`. Direct member uses of `unsafe.Add`, `unsafe.Alignof`,
`unsafe.Offsetof`, `unsafe.Sizeof`, `unsafe.Slice`, `unsafe.SliceData`,
`unsafe.String`, and `unsafe.StringData` must become typed reference edges and
reach their total-map `UNANALYZED` classifications instead of being discarded
with universe builtins.

## Background
Addresses finding F1 from the Step 6 implementation review. The stdlib-map
inventory deliberately treats unsafe's exported `*types.Builtin` objects as
externally referencable top-level symbols and persists them as project-override
`UNANALYZED` records. `goanalysis.externalObject`, `classifyingReference`, and
`symbol.FromObject` currently treat all `*types.Builtin` values as universe
objects, so the scanner never emits an edge for those records. This is a
fail-open mismatch between task 02's scanner and the Step 4/6 map contract.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R1-R6, I4, Reference semantics, The stdlib authority map, Error Handling)
- Implementation Review: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step06.yaml`

**Additional References:**
- `.agents/planning/2026-08-04-compositional-component-analysis/research/spike-stdlib-map-generation.md`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Distinguish universe builtins (`Pkg() == nil`) from exported package-scoped
   builtins. Universe identifiers such as `len`, `cap`, `append`, and `make`
   remain non-edges; exported builtins owned by `unsafe` are external declared
   objects and must be scanned.
2. Extend the canonical go/types conversion path so a package-scoped exported
   `*types.Builtin` produces the top-level `SymbolID` `unsafe.<Name>`. Reject a
   builtin with no declaring package and retain fail-closed behavior for any
   unsupported object.
3. Classify unsafe builtin observations as `facts.RefFunc`, preserving the exact
   component-relative site and normal reference ordering/deduplication.
4. Route the resulting edges through `StdlibAuthority.SymbolAuthority` without
   a special policy shortcut. Against the checked map, every supported unsafe
   builtin must yield the normal `AnalysisDefeating` observation and strict
   violation for its persisted `UNANALYZED` record.
5. Add real typed-source coverage, not only hand-constructed edges. Cover at
   least `unsafe.Sizeof` and one Go 1.20+ builtin such as `unsafe.Slice`, and
   table-cover all exported unsafe builtin objects if the shared converter is
   extended for the complete set.
6. Add a negative regression proving ordinary universe builtins are still
   ignored and do not cause conversion errors or synthetic references.
7. Exercise at least one unsafe builtin through the real `arcc check` Runner
   with the pinned map and assert its site appears in an analysis-defeating
   violation. Do not generate a whole SDK map in the test.
8. Update affected Component Contracts, declared interfaces, BUILD metadata,
   and the existing exemption comments/counts where the newly visible unsafe
   sites change their recorded evidence. Do not add or broaden an exemption.

## Dependencies
- Step 6 tasks 01-06 and the checked Linux/amd64 stdlib-map artifact.
- No dependency on the other implementation-review remediation tasks.

## Implementation Approach
1. Add RED converter and scanner tests using real go/types objects for both
   universe and unsafe builtins.
2. Make the smallest shared SymbolID/scanner changes that preserve the existing
   universe-object exclusion.
3. Add classification and production-Runner regressions against the pinned map.
4. Refresh only evidence/count comments affected by the corrected observations,
   then run the repository gates.

## Acceptance Criteria

1. **Unsafe builtins become canonical reference edges**
   - Given member source that calls `unsafe.Sizeof` and `unsafe.Slice`
   - When `ScanReferences` runs
   - Then it emits one site-bearing `RefFunc` edge for each canonical SymbolID
     and does not silently discard either `*types.Builtin` object

2. **Universe builtins remain outside the boundary model**
   - Given the same member source also uses `len`, `cap`, and `append`
   - When references are scanned
   - Then no edge is emitted for those package-less universe objects and the
     scan succeeds without guessing a package

3. **The stdlib map fails unsafe uses closed**
   - Given an emitted unsafe builtin edge and the pinned authority map
   - When the edge is classified under strict/default policy
   - Then the exact map record is consulted and an `AnalysisDefeating`
     violation retains the unsafe SymbolID and source site

4. **The production command covers the repaired seam**
   - Given a real component fixture that directly invokes an unsafe builtin
   - When it is checked through the production Runner with the pinned map
   - Then the command fails conformance with the expected analysis-defeating
     site, without whole-SDK generation

5. **The exemption set does not grow**
   - Given the corrected scan may reveal additional sites in already exempted
     foreign-member components
   - When selfcheck and Bazel component gates are inspected
   - Then no component is newly exempted, existing TODO owners remain Step 7,
     and any enumerated site counts are accurate

6. **Repository gates pass**
   - Given the completed repair
   - When `CGO_ENABLED=0 just ci` runs
   - Then every routine gate passes with no weakening of map, scanner, policy,
     or existing test semantics

## Metadata
- **Complexity**: High
- **Labels**: go, go-types, unsafe, reference-scan, stdlib-map, soundness, remediation
- **Required Skills**: Go AST and go/types APIs, static-analysis soundness, deterministic testing, integration testing
