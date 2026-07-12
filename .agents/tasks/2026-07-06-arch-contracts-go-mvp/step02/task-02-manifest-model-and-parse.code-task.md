# Task: Native `Manifest` model & `Parse(io.Reader)`

## Description
Introduce the hand-written, protobuf-free `Manifest` Go model and a pure
`Parse(r io.Reader) (Manifest, error)` that reads textproto content, unmarshals it into
the generated message, and maps it onto the native model. This is the parsing half of
the manifest package; syntactic validation is added in task-03.

## Background
The rest of the codebase (checker, shell) must **not** depend on protobuf types, so
`Parse` maps the generated message to a hand-written `Manifest` model (design §4.1).

`Parse` takes an **`io.Reader`**, not a path (design §4.1, PR-review): the Reader is an
**object capability** handed in by the shell — a deliberate illustration of
deprivileging the component. Internally it is `io.ReadAll` + `prototext.Unmarshal`.
`Parse` is ambient-authority-free **by construction**: the Step-0 spike settled the
classifier to attribute filesystem authority at the `os.Open` *minting* site, not at
capability *use* (design §5.4a), so reading a handed-in reader never counts against
`manifest`. The shell may hand `Parse` any reader; no `bytes.Reader` wrapping is
required (that rule was withdrawn — `bytes.Reader` remains a convenient in-memory reader
for tests).

**Risk note (carry to Step 10):** `prototext.Unmarshal` uses reflection, so `manifest`
may itself trigger `CAPABILITY_REFLECT`; this does not affect correctness now — flag it,
resolve in Step 10.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (§4.1 manifest signature, §5.4a minting-not-use classifier, §6.1 example manifest)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 2)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Define a hand-written Go `Manifest` model in `internal/manifest` mirroring the schema
   (task-01) but using native Go types only — no protobuf types leak past this package.
   Include: `Name`, `InterfaceFiles`, `ComponentDependencies` (`Name`, `Manifest`),
   `AbsorbedDependencies` (`ImportPath`, `Reason`), `DeclaredAuthority`.
2. Implement `func Parse(r io.Reader) (Manifest, error)`:
   - `io.ReadAll(r)` then `prototext.Unmarshal` (`google.golang.org/protobuf/encoding/prototext`) into the generated `Component` message.
   - Map the generated message to the native `Manifest` model.
   - A read failure or malformed-textproto surfaces as a parse error (returned `error`, non-nil).
3. Do **not** perform filesystem access — `Parse` operates purely on the reader's bytes.
   Syntactic validation rules are deferred to task-03 (leave a clear seam for them).
4. `Parse` must be safe to feed any `io.Reader`; tests feed it via `bytes.Reader`.

## Dependencies
- task-01 (generated `Component` message under `go/internal/manifest/gen/`).

## Implementation Approach
1. Add the native `Manifest` model types.
2. Implement `Parse`: `io.ReadAll` → `prototext.Unmarshal` → map to model.
3. Write a mapping helper from the generated message to the native model (handle the
   `optional reason` correctly — absent vs empty).
4. Table-driven unit tests fed via `bytes.Reader`: the `toprow` example from §6.1
   round-trips to the expected `Manifest` value; a manifest exercising all fields
   (deps + absorbed + declared_authority) maps correctly; a reader that returns an error
   surfaces as a parse error; malformed textproto surfaces as a parse error.

## Acceptance Criteria

1. **Valid manifest round-trips**
   - Given the `toprow` textproto from design §6.1 in a `bytes.Reader`
   - When `Parse` is called
   - Then it returns a `Manifest` with `Name == "toprow"`, `InterfaceFiles == ["toprow.go"]`, one absorbed dependency (`ImportPath` set, `Reason` present), empty `DeclaredAuthority`, and no error.

2. **All fields map**
   - Given a manifest exercising `component_dependencies`, `absorbed_dependencies` (with and without `reason`), and `declared_authority`
   - When parsed
   - Then every field maps onto the native model, and `optional reason` distinguishes absent from empty.

3. **No protobuf leakage**
   - Given the package's public API
   - When inspected
   - Then `Manifest` and `Parse` expose only native Go types; generated protobuf types are confined to `internal/manifest`.

4. **Reader/parse failures surface**
   - Given an `io.Reader` that errors, or malformed textproto bytes
   - When `Parse` is called
   - Then it returns a non-nil error and a zero `Manifest`.

5. **No filesystem access**
   - Given `Parse`'s implementation
   - When reviewed
   - Then it reads only the supplied reader and opens nothing itself.

## Metadata
- **Complexity**: Medium
- **Labels**: parser, data-model, pure-core, manifest
- **Required Skills**: Go, protobuf/prototext
