# Task: Centralize Protobuf Runtime Ownership

## Description
Create one in-tree asserted `PACKAGE_SURFACE` component that owns the explicitly enumerated protobuf runtime closure, migrate every self-hosting consumer to depend on it instead of claiming protobuf packages as members, and remove the protobuf-related Step 6 exemptions.

## Background
The legacy dependency resolver could not load a wrapper whose members were foreign import paths, so `manifest`, `artifactio`, `capslockadapter`, and related components temporarily duplicated protobuf runtime membership. Task 3 removes that limitation by reading package ownership from persisted surfaces. Keeping the duplication after the cutover would violate the one-owner component model and trigger member/direct-dependency overlap.

The runtime wrapper represents third-party code and uses the Step 5 `manual` asserted-surface path until Step 11 replaces that transition with `authority: UNKNOWN`. Its surface is package-level, carries no symbols or content digest, and is structurally asserted rather than falsely certified.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-08-04-compositional-component-analysis/design/detailed-design.md` (R7-R8, I1, I6, §New: schema and the protobuf-runtime boundary, §Migration Strategy)

**Additional References:**
- Plan Step 7: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/plan.md`
- Post-cutover foreign-member findings: `.agents/planning/2026-08-04-compositional-component-analysis/research/current-analysis-pipeline.md`
- Step 6 exemption inventory: `.agents/planning/2026-08-04-compositional-component-analysis/implementation/review-step06.yaml`

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add one in-tree `protobuf-runtime` component with `PACKAGE_SURFACE` style and a concrete, sorted, duplicate-free member list covering the protobuf runtime packages retained by the repository's Go dependency graph. Do not use globs, patterns, or a source-derived exact-symbol surface.
2. Produce the wrapper through the component-level asserted/manual path (`tags = ["manual"]`, not merely a tagged `.check` test): no analysis action, no report, provider provenance `asserted`, authority `UNKNOWN`, empty symbol set and digest, and target-correct namespace/SDK key. Supply a deterministic native convention surface for the pinned selfcheck configuration without letting a standalone file claim checked provenance.
3. Audit every self-hosting component manifest and Bazel declaration for `google.golang.org/protobuf/...` ownership. Remove all such member declarations outside the new wrapper and add explicit component-dependency edges wherever member code imports the generated schema or protobuf runtime directly. At minimum cover `schema`, `manifest`, `artifactio`, and `capslockadapter`; include any additional owners/consumers found by the audit.
4. Preserve the inward dependency direction: `schema` continues to own the generated artifact types and capability vocabulary, while the protobuf wrapper owns only the external runtime. `artifactio` must not regain an unnecessary dependency on `manifest`, and hand-authored `protojson`, `proto`, `prototext`, or related imports remain explicit component edges.
5. Keep BUILD and checked-in manifest ownership byte-for-byte equivalent under `manifestparity`. The wrapper's Bazel member labels and checked-in import paths must enumerate the same package set and sort deterministically.
6. Remove `artifactio` and `manifest` from every Step 6 AC8b exemption seam: delete their `check_tags = ["manual"]` verdict exemptions, restore the native selfcheck commands, and remove the matching TODO/count comments. Also remove the schema check exclusion if the protobuf boundary was its only reason; do not alter unrelated `manual` asserted-component fixtures.
7. Prove that a consumer can resolve the foreign-member wrapper with the wrapper's sources absent or unreadable. Resolution must use its persisted package-level surface, render it asserted/untrusted under the transitional authority, and never parse the protobuf runtime to discover ownership.
8. Re-run the affected self-hosting checks and record the new honest findings/declared-authority needs after protobuf code moves behind the boundary. Tighten declarations made unnecessary by the migration only when tests prove the component itself no longer exercises them; Step 10 still owns the general unused-authority feature.
9. Update Component Contract blocks, ownership comments, native selfcheck documentation, README examples if affected, and run `just ci` before committing.

## Dependencies
- Task 3 supplies production surface consumption for foreign package members and deterministic overlap enforcement.
- Plan Step 5 supplies the asserted manual producer used as the transitional stand-in for unknown third-party authority.
- Task 5 performs the analogous x/tools ownership migration; Task 6 resolves the remaining honest `UNANALYZED` findings.

## Implementation Approach
1. Compute and review the exact protobuf package union currently duplicated across component manifests and BUILD targets.
2. Add the wrapper's checked-in manifest, Bazel target, asserted surfaces/native fixture path, and parity tests.
3. Migrate direct consumers one at a time within the same change, removing duplicated members as each dependency edge is added.
4. Restore protobuf-blocked checks, verify no protobuf package has a second component owner, and update contracts/documentation.

## Acceptance Criteria

1. **The protobuf runtime has one owner**
   - Given every checked-in component manifest and `go_component` declaration
   - When concrete package ownership is indexed
   - Then every retained `google.golang.org/protobuf/...` package is owned exactly once by `protobuf-runtime`, with no patterns or duplicate owners

2. **Consumers use explicit boundaries**
   - Given `schema`, `manifest`, `artifactio`, `capslockadapter`, and any other direct protobuf consumers
   - When their manifests and Bazel providers are compared
   - Then they depend on the wrapper where required and no longer list its packages as their own members

3. **The wrapper is visibly asserted**
   - Given Bazel and native consumers of the wrapper
   - When their reports are rendered
   - Then the boundary is package-level, `ASSERTED`, `UNKNOWN` authority, and untrusted, with no wrapper report, exact-symbol claim, or content digest

4. **Foreign source is not required for surface resolution**
   - Given the wrapper surface and a consumer whose protobuf source tree is unreadable or absent from the dependency-resolver inputs
   - When the consumer check runs
   - Then ownership and boundary classification succeed from the surface without parsing protobuf source

5. **Protobuf exemptions are removed**
   - Given the Step 6 AC8b exemption inventory
   - When selfcheck and Bazel checks run after migration
   - Then `artifactio` and `manifest` are checked normally, their exemption comments/tags are gone, and no replacement bypass hides their verdicts

6. **Repository integration remains coherent**
   - Given generated schemas, manifest parsing, artifact I/O, native selfcheck, Bazel checks, and manifest parity
   - When `just ci` runs
   - Then all pass with one protobuf owner and deterministic asserted surfaces for the target configuration

## Metadata
- **Complexity**: High
- **Labels**: go, bazel, components, protobuf, package-surface, ownership, asserted-surface, selfcheck
- **Required Skills**: Component-boundary modeling, Go/Bazel dependency graph analysis, Starlark component rules, manifest parity, hermetic integration testing
