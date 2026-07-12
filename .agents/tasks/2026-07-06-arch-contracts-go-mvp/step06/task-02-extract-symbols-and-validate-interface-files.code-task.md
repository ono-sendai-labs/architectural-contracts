# Task: Extract symbols and validate interface files

## Description
Complete the first `goanalysis` loader slice by extracting exported declarations
and explicit `init` functions into `facts.ExportedSymbol`, including declaration
files and receiver linkage. Add resolved validation for manifest interface files,
then prove the shell→core seam by feeding fixture facts into `checker.Check` and
asserting a real Pillar-1 verdict.

After this task, Step 6's loader supplies all facts required by FR3 and FR4 while
leaving call edges for Step 9.

## Background
The manifest parser deliberately performs only syntactic validation. The shell
must verify that each `interface_files` path exists and is one of the Go source
files belonging to a package loaded beneath the component root. Failure is a tool
error, not a conformance finding.

FR4 needs every exported top-level declaration and exported method mapped to its
component-root-relative source file. Methods additionally carry their declaring
receiver type key. Explicit `func init()` is recorded even though it is not
exported, because importing a package executes it. Symbol names use the shared
Capslock/go-types key convention; method fixtures cover value and pointer
receivers, generics, embedding, interface types and concrete implementations so
the representation remains suitable for Step 9.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§4.1 fact and loader interfaces; §5.3 FR4 declared-interface/well-formedness rules and key normalization; §7 resolved manifest errors; §8 `goanalysis` fixture coverage)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 6)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-06-arch-contracts-go-mvp/research/go-component-model.md` (AST declaration mapping and interface-file path semantics)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend Task 1's `LoadPackageFacts` mapping to walk each loaded package's syntax
   and collect exported top-level funcs, types, vars, consts, and exported methods,
   plus every explicit `func init()`.
2. Populate `facts.ExportedSymbol` with:
   - `Kind` exactly `func`, `type`, `var`, `const`, `method`, or `init`;
   - `File` as a clean slash-separated path relative to `componentRoot`;
   - `Name` in the shared package-qualified Capslock/go-types key form;
   - `Receiver` for methods as the declaring receiver-type key, with empty
     receiver for all other kinds.
   Handle grouped `var`/`const` declarations one exported name at a time.
3. Correctly represent generic declarations and both pointer- and value-receiver
   methods. Do not synthesize promoted methods as declarations in the embedding
   type's file; retain enough loaded type information and fixtures for Step 9's
   later boundary/prune symbol expansion.
4. Keep `PackageFacts.CallEdges` empty. Sort symbols deterministically by package,
   file, kind, and name (or an equally explicit stable key).
5. Add an exported, shell-level resolved-validation API with a clear signature
   (for example, `ValidateInterfaceFiles(componentRoot string, interfaceFiles
   []string, loaded facts.PackageFacts) error`). It must reject absolute or
   escaping paths, missing/non-file entries, and files that do not belong to any
   loaded component package. It must accept valid component-root-relative Go files.
   Reuse loader source-file membership rather than trusting filename coincidence;
   if the public API needs an internal loaded representation, keep that shell
   detail private.
6. Return resolved-validation and AST/path failures as descriptive errors suitable
   for the future CLI's exit-code-2 path. Do not turn them into checker findings.
7. Expand the integration fixtures to cover: type in one interface file with a
   method elsewhere; a clean `types.go`/`api.go` split; exported interface plus
   concrete implementation; generic function and method; pointer and value
   receivers; embedding/promoted method behavior; grouped declarations; and an
   explicit init.
8. Add a vertical integration test that calls `LoadPackageFacts`, validates the
   fixture interface files, passes the real facts to `checker.Check` with empty
   capabilities/dependency interfaces, and asserts the rendered Pillar-1 report
   for a deliberately undeclared import. Include a concise logged demo of imports,
   symbol-to-file mappings, and the report when the test runs verbosely.

## Dependencies
- Step 6 Task 1 `internal/goanalysis` loader and fixture foundation.
- Step 3 `internal/facts` symbol model.
- Step 4/5 `internal/checker` and `internal/report` for the vertical slice.
- `internal/manifest.Manifest` for constructing the checker input and supplying
  interface-file declarations; parsing a manifest from disk is not required.

## Implementation Approach
1. Build a mapping between each loaded AST file and its root-relative path, then
   walk declarations and emit normalized `facts.ExportedSymbol` values.
2. Use receiver AST/type information to form method and receiver keys without
   conflating pointer and value declarations or generic instantiations.
3. Preserve the loaded package source-file set privately so resolved validation
   can prove that manifest entries are real member-package files; expose only the
   minimal validation API.
4. Flesh out the Step-6 fixture matrix and table-driven integration assertions for
   every declaration kind and FR4 corner case.
5. Add the checker vertical-slice integration test with an intentionally undeclared
   non-stdlib import and assert both structured and rendered report output.

## Acceptance Criteria

1. **Exported top-level declarations map to their files**
   - Given fixtures containing exported and unexported funcs, types, vars, and
     consts, including grouped declarations
   - When facts are loaded
   - Then every exported name appears once with the correct kind and relative file,
     and unexported declarations do not appear.

2. **Methods carry receiver linkage**
   - Given exported methods declared on value, pointer, and generic receiver types
     across interface and non-interface files
   - When symbols are extracted
   - Then each method has the expected package-qualified name, declaring file,
     `method` kind, and receiver-type key needed by the FR4 checker.

3. **Explicit init is represented**
   - Given a fixture with an explicit `func init()`
   - When facts are loaded
   - Then an `init` symbol is emitted for its package and declaring file even though
     `init` is not Go-exported.

4. **Interface and embedding fixtures preserve declaration truth**
   - Given an exported interface with a concrete implementation and a type with a
     promoted method through embedding
   - When Step-6 symbols are extracted
   - Then actual declarations map to their real files and no promoted method is
     falsely reported as declared on the embedding type; the fixture remains usable
     for Step 9's method-set expansion.

5. **Resolved interface-file validation accepts only member source files**
   - Given valid interface paths beneath the root, missing paths, directories,
     absolute/escaping paths, and existing files outside the loaded package set
   - When resolved validation runs
   - Then only the valid member Go source files are accepted and every invalid case
     returns a descriptive tool error.

6. **Call edges remain deferred**
   - Given fully extracted Step-6 package facts
   - When the result is inspected
   - Then `CallEdges` is empty because VTA construction belongs to Step 9.

7. **Real facts drive a Pillar-1 verdict**
   - Given a loaded fixture with a deliberately undeclared non-stdlib import and
     otherwise valid manifest/interface inputs
   - When its facts are passed to `checker.Check` with no capability findings
   - Then the report contains the expected `UNDECLARED_DEPENDENCY` violation and
     its rendered output is deterministic.

8. **Step 6 demo and integration suite pass**
   - Given the complete fixture matrix
   - When `just test-integration` runs (or the vertical test is run verbosely)
   - Then it passes and exposes the fixture imports, symbol→file map, and
     checker-produced Pillar-1 report required by the plan demo.

## Metadata
- **Complexity**: High
- **Labels**: goanalysis, AST, symbols, interface-files, FR4, vertical-slice, integration-test
- **Required Skills**: Go, `go/ast`, `go/token`, `go/types`, `golang.org/x/tools/go/packages`, Go integration testing
