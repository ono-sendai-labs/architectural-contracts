# Task: Enumerate SDK standard library

## Description
Teach the package-layout driver to discover the active Go standard library directly from the layout's `go_sdk_root`, merge the discovered packages into the driver graph, and answer both exact standard-library imports and the `std` meta-pattern without requiring those packages in the emitted layout.

## Background
The Bazel dependency aspect deliberately excludes standard-library archives, so Step 4 emits small layouts containing only the component closure. Under `GOPACKAGESDRIVER`, however, `go/packages` expects the driver to provide every requested import, and Capslock independently loads the `std` meta-pattern. The driver must therefore reconstruct a source-based, build-constraint-filtered standard-library graph from the declared SDK source tree without invoking `go list` or a Go binary. Existing layouts that already include standard-library records remain valid and authoritative.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-16-bazel-arcc-rules/design/detailed-design.md` (§4.6, §5.4, §6.2, and §8.4)
- Plan: `.agents/planning/2026-07-16-bazel-arcc-rules/implementation/plan.md` (Step 5a)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-16-bazel-arcc-rules/research/spike-driver-findings.md` (findings 1, 2, 4, and 5)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add a standard-library discovery seam in `go/internal/packagelayout` that walks the SDK source tree rooted at `Layout.GoSDKRoot` and uses `go/build.Context` with that GOROOT to identify importable packages and their build-selected Go source files without subprocesses.
2. Apply the active build context's GOOS, GOARCH, compiler, cgo, release tags, and build tags consistently; include only files accepted by `go/build`, with focused coverage proving that a source file for a different GOOS is excluded.
3. Reproduce the package set `go list std` reports: exclude the top-level `cmd` tree and any `testdata` directory, while retaining importable `internal` packages. Ignore ordinary non-package directories, but return actionable errors for SDK access or package-inspection failures that make the discovered graph incomplete.
3a. Enumerate `$GOROOT/src/vendor/…` as part of the standard library, since `go list std` does and `net`, `crypto/tls` and `net/http` cannot be type-checked without it. Derive each import path from the path relative to `$GOROOT/src`, which yields the `vendor/`-prefixed form used by the go command. `$GOROOT/src/cmd/vendor/…` stays out of scope only because `cmd` does.
3b. Apply the go command's GOROOT vendor rule to SDK import edges: `go/build` reports the source import (`golang.org/x/net/dns/dnsmessage`), which must be rewritten to the vendored package's `vendor/`-prefixed path when that package exists under `$GOROOT/src/vendor`. Unrewritten, such an import is neither classified as standard library nor discoverable, and the importing package's graph is silently incomplete.
4. Convert each discovered package into the flat `packages.Package` representation used by the existing driver, including deterministic ID/import-path/name/file data and direct standard-library import references sufficient for source type-checking.
5. Merge discovered packages into the parsed layout before structural validation and path resolution. Layout-provided package records must win by ID/import path, so Step 1 fixtures and other fully enumerated layouts keep their existing source lists and metadata unchanged.
6. Permit a layout with a valid `go_sdk_root` and no embedded standard-library records to validate and serve exact standard-library import paths and `std`; retain clear failures for an absent, unreadable, or invalid SDK root.
7. Sort directory traversal results, package records, file lists, import processing, roots, and response packages so repeated driver responses are byte-stable.
8. Add unit tests in `go/internal/packagelayout` for build-constraint filtering, skipped and retained directory classes, exact-import and `std` queries, transitive stdlib edges, layout-entry precedence, deterministic output, and malformed or unavailable SDK roots.
9. Cover the vendor rule with a real-SDK-shaped fixture: a package under `src/` importing a `golang.org/…` path that exists only under `src/vendor/`, asserting the edge is rewritten and the target is discovered. The fixture used for the minimal-layout test must contain such a dependency, so the case cannot pass vacuously the way a vendor-free fixture does.

## Dependencies
- Step 1's package-layout schema, validation, path resolution, and driver request handling in `go/internal/packagelayout`.
- The Go SDK source tree identified by `Layout.GoSDKRoot`.
- No earlier task in Step 5a; Task 2 consumes the merged standard-library index produced here.

## Implementation Approach
1. Isolate SDK walking and `go/build` package inspection behind a small helper that can be exercised against purpose-built temporary SDK trees.
2. Build a deterministic discovered-package index, including direct import references, then merge it with a layout-package index using the layout record as the authority on collisions.
3. Run merge before the existing graph validation and file resolution so both discovered and provided records pass through one consistency boundary.
4. Update exact and `std` root selection to operate over the merged graph, preserving the current deterministic response contract.
5. Use test SDK fixtures containing accepted, rejected-by-GOOS, internal, command, vendor, testdata, and non-package directories to cover enumeration without depending on the host SDK's contents.

## Acceptance Criteria

1. **A minimal layout gains the active standard library**
   - Given a layout with a valid `go_sdk_root`, component roots, and no layout-provided standard-library packages
   - When the layout is prepared for driver requests
   - Then importable SDK packages and their direct standard-library edges are present in the flat driver graph without invoking `go list` or a Go binary.

2. **Build constraints select the correct source files**
   - Given an SDK package fixture containing ordinary files and files restricted to the active and a different GOOS
   - When standard-library packages are enumerated with the active build context
   - Then accepted files are included and the other-platform files are excluded from `GoFiles` and `CompiledGoFiles`.

3. **The exposed SDK package set matches `go list std`**
   - Given SDK fixtures under ordinary, `internal`, `vendor`, `cmd`, `testdata`, and non-package directories
   - When enumeration completes
   - Then ordinary, internal, and `src/vendor` packages are available — the latter under `vendor/`-prefixed import paths — while command, testdata, and non-package directories are absent.

3a. **Vendored standard-library edges resolve**
   - Given an SDK fixture whose `src/` package imports a `golang.org/…` path present only under `src/vendor/`
   - When enumeration completes
   - Then the importing package's edge names the `vendor/`-prefixed import path, the vendored package is present in the graph under that path, and the graph passes import validation.

4. **Exact and meta-pattern queries use the discovered graph**
   - Given a layout that does not embed standard-library records
   - When the driver is queried for an exact standard-library import and for `std`
   - Then the exact package or every discovered standard-library package is selected as a root, and responses remain deterministically ordered.

5. **Layout-provided standard-library records win**
   - Given a layout that supplies a standard-library package also discoverable from the SDK
   - When the discovered and provided graphs are merged
   - Then the layout's record, files, and imports remain unchanged and no duplicate identity is introduced.

6. **SDK failures are actionable tool errors**
   - Given an absent, unreadable, or structurally invalid SDK source root
   - When the layout is prepared
   - Then preparation fails with a deterministic diagnostic naming the SDK path and failed operation.

7. **Repository checks remain green**
   - Given the completed task
   - When `just ci` runs
   - Then all checks pass.

## Metadata
- **Complexity**: High
- **Labels**: Go, package-layout, standard-library, GOPACKAGESDRIVER, hermetic-loading
- **Required Skills**: Go, `go/build`, `go/packages`, filesystem traversal, deterministic graph construction, unit testing
