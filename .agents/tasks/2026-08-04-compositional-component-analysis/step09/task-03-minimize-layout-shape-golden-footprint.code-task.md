# Task: Minimize Layout-Shape Golden Footprint

## Description
Finish the Step 9 split by replacing the combined manifest/layout golden tests with semantic manifest checks and the minimum set of intentional layout-shape goldens. Add enforcement and audit evidence showing that verdict goldens are portable and quantifying the reduction in host-patchable golden files.

## Background
The current `golden_test.sh` compares a generated manifest and a full package layout in one test. It performs ad hoc substitutions for SDK/export roots, while the repository-wide `manifestparity` test already owns semantic manifest equivalence. Full layouts necessarily expose closure topology, workspace/runfiles frames, export paths, toolchain configuration, and dependency artifact bindings; those details should be pinned only by tests whose purpose is layout shape.

Friction report section 6 found that downstream hosts patched 45 golden/testdata files. Step 1 made semantic manifest comparison canonicalize both sides through `hostpolicy.CanonicalizePath`; Step 9 should now remove redundant host-shaped snapshots, retain only distinct final-layout contracts, and record a reproducible before/after count. The Step 9 demo also requires verdict goldens to compare equal under a simulated rewritten-prefix namespace.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (§Type loading, §Canonical namespace, §Golden restructure, §Determinism and self-check)

**Additional References:**
- Host import friction: `.agents/planning/2026-08-04-compositional-component-analysis/research/host-import-friction.md` (§6 and the 45-file baseline)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Inventory every checked-in golden and golden-style testdata file, classify it as semantic verdict, persisted-artifact shape, layout shape, or unrelated fixture, and use that inventory to remove redundant comparisons rather than renaming them in place.
2. Split `golden_test.sh` (or replace it with focused helpers) so manifest equivalence is covered by `manifestparity`/typed semantic comparison and package-layout shape is covered independently. Do not keep duplicate byte goldens for generated manifests already covered semantically.
3. Retain only layout goldens that cover distinct final Step 8 shapes, such as a checked direct-dependency binding and a member/non-member export-data closure. Consolidate fixtures whose only difference is already pinned by focused analysis or unit tests.
4. Make layout-shape comparison schema-aware and normalize both actual and expected values through the established path/namespace policy where import paths are compared. Continue to represent unavoidable SDK root, export root, execution-frame, and target-configuration values with explicit named placeholders and validate their original shape before substitution.
5. Preserve meaningful final-layout details in the remaining layout goldens: member source roles, non-member export files and complete import maps, ordinary-import descriptor metadata, stdlib export metadata, target platform/key fields, and dependency surface/report bindings. Collection ordering must remain deterministic.
6. Add an automated guard over files designated as verdict goldens proving they contain no absolute paths, closure package lists, SDK/export roots, diagnostic input metrics, or configured host import prefixes. The guard must make a newly contaminated verdict golden fail with a diagnostic naming the file and forbidden content class.
7. Add or preserve a simulated rewritten-prefix test that demonstrates the same verdict goldens compare byte-identically under two idempotent namespace policies. Layout-shape goldens may differ only where their explicit normalization contract says they are host-sensitive.
8. Record a concise Step 9 research/demo note under the project `research/` directory. Include the inventory method, the known downstream baseline, repository before/after counts by category, the remaining host-patchable files with reasons, and commands/results for the rewritten-prefix comparison.
9. Update golden-maintenance comments and commands in Bazel test files so they identify verdict versus shape artifacts and cannot suggest copying unnormalized host output into a semantic golden.

## Dependencies
- Task 1: Separate Semantic Verdict Goldens.
- Task 2: Add Persisted Artifact Shape Goldens, so the audit can classify all retained non-layout golden coverage accurately.

## Implementation Approach
1. Build the golden inventory from repository labels/files and map each retained case to a unique behavior already required by the design.
2. Replace the combined shell comparison with focused semantic-manifest and typed/validated layout-shape tests, then delete redundant manifest and duplicate layout goldens.
3. Add the portability guard and rewritten-prefix regression, run the complete suite, and capture the reproducible before/after evidence in the research note.

## Acceptance Criteria

1. **Verdict and layout assertions are structurally separate**
   - Given the Bazel golden targets
   - When their inputs and runfiles are inspected
   - Then verdict tests consume only reports and semantic verdict data, while layout-shape tests consume layouts and explicit shape goldens; no combined manifest/layout/verdict assertion remains.

2. **Manifest byte goldens are removed when redundant**
   - Given generated component manifests with host-rewritten import paths
   - When repository tests run
   - Then semantic `manifestparity` comparison covers them through canonicalization and no duplicate generated-manifest byte golden requires patching.

3. **Remaining layout goldens cover distinct final shapes**
   - Given the retained direct-dependency and export-backed member fixtures
   - When layout-shape tests run
   - Then final Step 8 package roles, import closure, export metadata, target identity, and dependency bindings are checked after validated normalization.

4. **Verdict contamination is rejected**
   - Given a verdict golden containing an absolute path, a package-closure field, SDK/export metadata, diagnostic metrics, or a configured host prefix
   - When the portability guard runs
   - Then it fails and names both the file and the forbidden content category.

5. **Rewritten namespaces preserve semantic goldens**
   - Given upstream and simulated host-rewritten import namespaces
   - When verdict golden comparisons run
   - Then the verdict bytes and outcomes are identical, while any intentionally host-shaped layout value is handled by the documented layout normalizer.

6. **The patch-footprint reduction is measured**
   - Given the completed golden inventory
   - When the Step 9 research note is reviewed
   - Then it records reproducible before/after counts, cites the 45-file downstream baseline, and lists every remaining host-patchable golden with its distinct shape-coverage purpose.

7. **Golden updates are deterministic and documented**
   - Given an intentional layout or artifact schema change
   - When the documented update commands are followed twice
   - Then they produce byte-identical normalized goldens without copying machine-local absolute paths.

8. **Integration remains green**
   - Given the minimized golden suite
   - When `just ci`, self-check, and manifest parity run
   - Then all checks pass and coverage for final layout shape, report/surface/map artifacts, and semantic verdicts remains present.

## Metadata
- **Complexity**: High
- **Labels**: bazel, testing, goldens, package-layout, host-portability, documentation
- **Required Skills**: Starlark, Go test helpers, Bazel runfiles, package-layout schema, host path canonicalization, test-suite auditing
