# Task: Package layout driver

## Description
Define arcc's package-layout contract and implement the internal `GOPACKAGESDRIVER` responder that turns a Bazel-produced layout into the flat package graph expected by `go/packages`.

## Background
Step 1 is the consumer-side foundation for hermetic Bazel checks. The validated driver spike proves that `go/packages` and Capslock can type-check entirely from source paths supplied by a self-exec driver, but the spike's ad hoc layout and dispatch must become a validated, deterministic internal arcc API. The resulting schema is also the contract that Step 4's Bazel rule will emit.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-16-bazel-arcc-rules/design/detailed-design.md` (§4.6, §5.4, §6.2, and §7.2)
- Plan: `.agents/planning/2026-07-16-bazel-arcc-rules/implementation/plan.md` (Step 1)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver-findings.md` (driver protocol findings 1–5)
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver/main.go` (validated flat-package prototype)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add an internal package-layout model for `go_sdk_root`, explicit component `roots`, and the complete package graph required by `go/packages`; encode package records in the driver's flat form with IDs, import paths, names, source/compiled-source files, and direct-import ID mappings.
2. Parse and validate layouts before serving them, rejecting malformed JSON, duplicate or missing package identities, unknown roots/import targets, invalid source paths, and inconsistent graph references with actionable deterministic errors.
3. Resolve workspace-relative package source paths and SDK-relative standard-library source paths into readable paths without requiring a Go module or invoking the Go tool.
4. Implement the `GOPACKAGESDRIVER` request/response protocol using `packages.DriverRequest` and `packages.DriverResponse`; return the complete graph while selecting response roots for exact import-path queries.
5. Handle the `std` meta-pattern by returning every standard-library package in the layout, using an explicit, tested standard-library classification consistent with the producer contract.
6. Return a clear driver error for unsupported or unknown patterns instead of silently returning an incomplete root set.
7. Keep JSON and response ordering deterministic so Bazel-produced layouts and diagnostics are stable.
8. Add focused unit tests for schema round trips, validation, exact queries, `std`, graph edges, path resolution, unknown patterns, malformed layouts, and named-but-missing source files.

## Dependencies
- Existing `golang.org/x/tools/go/packages` dependency and Go module under `go/`.
- No earlier task in this step; this task creates the schema and driver API consumed by Tasks 2 and 3.

## Implementation Approach
1. Extract the spike's flat-driver mechanics into a small internal package with explicit layout structs, parsing, validation, and path-resolution helpers.
2. Separate protocol I/O from layout/query logic so unit tests can exercise requests and responses without spawning the arcc binary.
3. Construct deterministic indexes by package ID and import path, then implement exact and `std` root selection over those indexes.
4. Cover corrupt layouts and absent files at the boundary so later CLI wiring can consistently map them to tool errors.

## Acceptance Criteria

1. **Layout schema round-trips the required graph**
   - Given a layout containing component roots, transitive packages, direct imports, compiled sources, and SDK metadata
   - When it is parsed and serialized
   - Then all driver-required fields and deterministic ordering are preserved.

2. **Exact package queries return a usable flat graph**
   - Given a valid layout and an exact import-path query
   - When the driver handler processes a `packages.DriverRequest`
   - Then the matching package is a response root and the response contains the complete dependency graph with correct import ID references.

3. **The `std` meta-pattern is supported**
   - Given a layout containing component and standard-library packages
   - When the driver is queried for `std`
   - Then every and only standard-library package is returned as a root, enabling Capslock's internal standard-library load.

4. **Paths resolve without module discovery**
   - Given workspace-relative component sources and SDK-relative standard-library sources
   - When the layout is prepared for a driver response from a directory with no `go.mod`
   - Then all file names resolve from the layout/workspace and `go_sdk_root`, with no `go list` invocation.

5. **Invalid layouts fail cleanly**
   - Given malformed JSON, duplicate identities, dangling roots/imports, an unknown query, or a source named by the layout that does not exist
   - When the layout is loaded or queried
   - Then the operation returns a deterministic diagnostic identifying the offending field, package, pattern, or file.

6. **Repository checks remain green**
   - Given the completed task
   - When `just ci` runs
   - Then all checks pass.

## Metadata
- **Complexity**: High
- **Labels**: Go, package-layout, GOPACKAGESDRIVER, hermetic-loading
- **Required Skills**: Go, JSON schema design, `go/packages`, protocol testing, filesystem path handling
