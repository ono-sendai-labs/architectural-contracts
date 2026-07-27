# Task: Classify standard library from provenance

## Description
Make standard-library classification fail closed in both native and package-layout modes. Treat package provenance as authoritative, validate it against the host path policy, and prevent ordinary vendored packages from receiving the standard-library import-resolution relaxation.

## Background
The current native classifier treats every package without module metadata as standard library, so a driver-backed or rewriting host can silently skip a real dependency. Layout mode has the opposite structural problem: `go/packages.Package` values do not carry module metadata, so applying the native nil-module rule uniformly would classify every package as standard library. The layout therefore needs an explicit per-package provenance signal while SDK-discovered packages remain structurally known standard library.

The layout `is_stdlib` bit is not a second heuristic. It records whether the package came from the SDK/toolchain rather than an enumerated build target. The loader validates that authoritative provenance against `hostpolicy.IsStdlibPath`; disagreement is a load error because the path policy is still used in resolution paths and must not silently disagree with ground truth.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-25-component-membership-and-authority/design/detailed-design.md` (§2 T4–T4a, §4.4–4.5, §5.1a, §6, and §7.2)
- Plan: `.agents/planning/2026-07-25-component-membership-and-authority/implementation/plan.md` (Step 5)

**Additional References (if relevant to this task):**
- `.agents/planning/2026-07-25-component-membership-and-authority/research/host-portability-findings.md` (F6 and consequences for T4a)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Extend the package-layout JSON schema with a per-emitter-package `is_stdlib` boolean whose code comment explicitly requires build-graph provenance (SDK/toolchain versus enumerated target) and forbids deriving the bit by re-running an import-path heuristic.
2. Preserve the distinction between packages explicitly listed by the emitter and packages structurally discovered from `go_sdk_root`; SDK-discovered packages are standard library without requiring an emitted bit.
3. In layout validation, compare each emitter-listed package's declared provenance bit with `hostpolicy.IsStdlibPath(packagePath)`. Return a load error on disagreement that names the package path, the declared provenance verdict, and the path-policy verdict.
4. Use the validated layout provenance verdict throughout layout-mode loading and fact production. Do not infer layout standard-library status from `packages.Module`, because layout packages have no useful module metadata.
5. Preserve `os`, `fmt`, and other SDK-discovered packages as standard library in layout mode even though they carry no emitted bit. Avoid a uniform nil-module rule that would either misclassify all layout packages or drown member packages in `UNDECLARED_DEPENDENCY`.
6. In native mode, classify in three ordered cases:
   a. A package belonging to the Go SDK (`GOROOT`) is standard library **structurally**, mirroring `discoverStdlib` in layout mode. `packages.Package` exposes no `Goroot` field, so determine SDK membership loader-side — `go/build`'s `Context.Import(path, "", build.FindOnly)` exposes `Goroot`, and `go list -json`'s `Standard`/`Goroot` is equivalent. Either is conforming; prefer the one that does not add a subprocess pass to every native load.
   b. Otherwise, a package with `p.Module == nil` is **not** standard library. This is the case the nil-`Module` fix exists for: a driver-loaded or rewritten dependency with no provenance must not be classified stdlib and silently skipped.
   c. Otherwise, standard library only when module provenance says standard library **and** `hostpolicy.IsStdlibPath` agrees.

   Do **not** flip the nil-`Module` branch and then AND uniformly. `go/packages` reports `Module == nil` for *every* standard-library package — verified on Go 1.26.4, where `go list -json fmt` gives `Standard: true`, `Goroot: true` and no `Module` — so that rule evaluates to `false && true` for `fmt` and makes `fmt`, `path`, `sort` and `strings` report `UNDECLARED_DEPENDENCY`, which the self-check cannot survive. Case (a) is what keeps provenance authoritative without demoting `hostpolicy.IsStdlibPath` to the sole native signal.
7. Preserve a safe path-policy fallback only for absent package references where no package provenance can be consulted; do not use it to override contradictory provenance.
8. Ensure a simulated rewriting-host dependency that lacks module metadata is retained as a non-stdlib dependency and can reach the existing `UNDECLARED_DEPENDENCY` check instead of being skipped.
9. Restrict the phase-3 `vendor/<import>` resolution relaxation in `packagelayout.ValidateAndResolve` to targets whose validated provenance says standard library. A non-stdlib vendored package must not resolve through this special case.
10. Update hand-written layout fixtures as required: explicitly listed SDK-like packages need `is_stdlib: true`, and synthetic dotless package paths need either a conforming bit/policy pair or an intentional disagreement assertion.
11. Keep the layout schema backward-compatible at the JSON syntax level: omitted `is_stdlib` decodes as false, while the agreement check deliberately rejects an omission when the path policy identifies that explicitly listed package as standard library.
12. Do not remove `facts.PackageFact.IsStdlib` or the `StdlibImports` nil sentinel in this task; Task 3 performs that contract cleanup after classification is authoritative.

## Dependencies
- Steps 1–4 (complete): provide declared members, member-root attribution, platform-aware layout validation, and final report behavior.
- No task within Step 5 precedes this task.
- `task-02-enforce-canonical-loader-paths` and `task-03-simplify-stdlib-facts` build on the authoritative classification established here.

## Implementation Approach
1. Introduce a layout-owned package representation or side metadata that can decode `is_stdlib` without losing compatibility with the `go/packages` driver protocol, and retain whether a package was emitted or SDK-discovered.
2. Validate emitter provenance against the host path policy while indexing the layout, before classification can influence filtering, import recovery, or graph resolution.
3. Thread the validated verdict into layout-mode package loading and `goanalysis` classification; separately tighten the native module classifier and AND it with the path policy.
4. Replace path-only decisions in the vendor-resolution exception with the validated package classification.
5. Add focused unit and end-to-end fixtures for native nil-module packages, rewriting policies, explicit layout provenance, SDK discovery, `os`/`fmt`, disagreement errors, and vendored non-stdlib packages.

## Acceptance Criteria

1. **Native nil-module dependency is not skipped**
   - Given a native-mode dependency loaded by a driver with `Module == nil` and a host path policy that identifies it as non-stdlib
   - When package facts and conformance findings are produced
   - Then the dependency is classified non-stdlib and can produce `UNDECLARED_DEPENDENCY`.

2. **Native classification is provenance-first, in three ordered cases**
   - Given an SDK (`GOROOT`) package such as `fmt`, a non-SDK package with `Module == nil`, a package with ordinary module provenance, and a package with standard-library module provenance, each paired with host path-policy verdicts
   - When the native classifier runs
   - Then the SDK package is standard library structurally regardless of its module metadata being nil; the non-SDK nil-module package is not standard library; and for the remainder, standard library requires module provenance and a true path-policy verdict to agree.

2a. **The self-check survives native classification**
   - Given arcc's own components and the csvtool examples, which import `fmt`, `os`, `path`, `sort` and `strings`
   - When `just selfcheck` and the CLI integration suites run under the new native classifier
   - Then those SDK imports remain standard library and produce no `UNDECLARED_DEPENDENCY`, and the existing success expectations are unchanged.
   - This criterion exists because the previous wording of requirement 6 failed exactly here; a change that satisfies requirement 6 but not this criterion is wrong.

3. **Layout classification comes from declared provenance**
   - Given an emitter-listed layout package whose `is_stdlib` bit and path policy agree
   - When the layout is validated and loaded
   - Then fact production uses that validated verdict rather than nil module metadata or an independent path-only guess.

4. **Provenance disagreement fails closed**
   - Given an emitter-listed package whose `is_stdlib` bit disagrees with `hostpolicy.IsStdlibPath`
   - When `ValidateAndResolve` runs
   - Then it returns an error naming the package path and both boolean verdicts.

5. **SDK discovery remains structural**
   - Given `go_sdk_root` discovers `os`, `fmt`, or another SDK package not explicitly listed by the emitter
   - When the layout is validated and loaded
   - Then the package is classified standard library without an emitted `is_stdlib` bit.

6. **Layout regression does not flood dependency findings**
   - Given an ordinary layout-mode component importing `os` and `fmt`
   - When conformance is checked
   - Then those imports remain standard library and do not produce `UNDECLARED_DEPENDENCY`.

7. **Explicit SDK-like packages require a bit**
   - Given a hand-written layout explicitly lists a path the host policy calls standard library but omits `is_stdlib`
   - When validation runs
   - Then validation fails with the provenance disagreement rather than silently accepting the package.

8. **Vendor relaxation is standard-library-only**
   - Given a bare import can match a `vendor/<import>` layout package whose validated provenance is non-stdlib
   - When import resolution runs
   - Then the vendor-prefix relaxation does not resolve it through the standard-library special case.

9. **Schema documentation prevents heuristic duplication**
   - Given an emitter author reads the layout package schema
   - When they inspect `is_stdlib`
   - Then the comment states that it comes from build-graph provenance and must not be computed by applying a path heuristic.

10. **Step 5 classification demo is reproducible**
    - Given a simulated rewriting host under which the old nil-module behavior skipped a real dependency
    - When the same fixture runs with the new classifier
    - Then it surfaces the existing boundary finding instead of reporting a silent pass.

11. **Repository checks remain green**
    - Given provenance-backed classification and the narrowed vendor rule
    - When `just ci` runs
    - Then unit, integration, self-check, generation-cleanliness, and Bazel suites pass with only intentional fixture migrations.

## Metadata
- **Complexity**: High
- **Labels**: Go, goanalysis, packagelayout, host-policy, standard-library, fail-closed
- **Required Skills**: Go, `go/packages`, JSON schema evolution, package loaders, deterministic validation, integration testing
