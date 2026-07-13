# Task: Write user README

## Description
Replace the placeholder root README with a user-oriented guide that lets a newcomer understand Architectural Contracts, build and run `arcc`, author a manifest, and reproduce the MVP's passing and failing examples.

## Background
The implementation and self-hosting checks are complete, but the repository does not explain the product outside its planning artifacts. Step 11 requires an accurate entry point connecting the rationale to concrete usage and honest MVP limitations.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§2, §6, §10, §11)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 11)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Explain architectural contracts, structure and ambient-authority checking, and link to `docs/rationale-and-concepts.md`.
2. Document prerequisites, fresh-clone build instructions, `arcc check <manifest>`, `--format=json`, and exit codes 0, 1, and 2 using verified behavior.
3. Include a CSV walkthrough: `toprow` passes authority-free, an absorbed file-reader variant fails with `UNDECLARED_AUTHORITY`, and `app` passes while composing the `FILES`-holding `csvfile` component.
4. Make every command reproducible from tracked files or explicitly create temporary failing input; do not imply that the integration-only `absorbapp` fixture is tracked.
5. Explain `component.textproto`: component-root placement, relative interface files, component versus absorbed dependencies, and `declared_authority`, with a schema-valid example.
6. Document design §11 limitations: VTA/dynamic-dispatch over-approximation, higher-order boundary warnings, call-edge-only enforcement, compositional single-component checking, generic matching, and Go-only scope.
7. Keep GitHub links and paths relative and include relevant contributor commands such as `just ci` and `just selfcheck`.

## Dependencies
- Completed Steps 8–10, including the real CLI, CSV components, integration expectations, and self-hosting commands.
- Existing concept document, protobuf schema, and example manifests.

## Implementation Approach
1. Inspect CLI help, integration tests, example manifests, and rendered reports for exact behavior.
2. Structure the README around overview, build, usage, walkthrough, manifest authoring, development, and limitations.
3. Run every documented example and compare excerpts and statuses with actual text and JSON output.
4. Run available documentation checks and `just ci`.

## Acceptance Criteria

1. **A newcomer can run the tool**
   - Given a fresh clone and only the README
   - When the prerequisites and build instructions are followed
   - Then `arcc` builds and can check a component manifest.

2. **CLI behavior is accurate**
   - Given the usage and exit-code sections
   - When their text and JSON commands run
   - Then syntax, output shape, and statuses match the CLI.

3. **The authority showcase is reproducible**
   - Given the CSV walkthrough
   - When its `toprow`, absorbed-authority, and pruned `app` scenarios run
   - Then they demonstrate the stated pass/fail distinctions without nonexistent paths.

4. **Users can author a valid manifest**
   - Given the manifest section
   - When its example is adapted at a component root
   - Then its fields and path semantics agree with the schema and checker.

5. **MVP boundaries are explicit**
   - Given the limitations section
   - When adoption is evaluated
   - Then the analysis precision, higher-order, enforcement scope, compositional, generic, and language limits are visible.

6. **Documentation is verified**
   - Given the completed README
   - When its examples and `just ci` run
   - Then documented outcomes occur and all checks pass.

## Metadata
- **Complexity**: Medium
- **Labels**: documentation, README, user-guide, examples, manifests
- **Required Skills**: Technical writing, Go CLI usage, architectural contracts model
