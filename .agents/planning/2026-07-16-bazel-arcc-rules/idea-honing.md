# Idea Honing: arcc as a Bazel Rule

Q&A log refining the rough idea in `rough-idea.md`. Research context in `research/`.

## Q1: Loading strategy — how does `arcc check` get at Go source inside a Bazel action?

**Options considered:**
- **Package-layout file:** arcc grows a `--package-layout` mode fed by Bazel-serialized package metadata (importpath → files → deps from `GoInfo`/`GoArchive`), bypassing `go list`. Hermetic, cacheable, RBE-safe; requires arcc-side work incl. stdlib handling.
- **Layout file + local-test stopgap:** same end state, plus a non-hermetic local test mode as interim/debugging path.
- **Synthesized module tree:** stage transitive sources into a fake go.mod/vendor layout so unmodified arcc can run `go list` in the sandbox. No arcc changes but fragile and slow.
- **Non-hermetic local test only:** minimal work, no hermeticity/caching/RBE.

**Answer:** Local-test stopgap **first**, then the package-layout file as the end state.

**Rationale:** The user has a project they're keen to try this out in; the non-hermetic local test is the quickest path to validating the concepts behind this work. The package-layout mode remains the committed hermetic end state.

**Design implication:** The design should be phased — Phase 1: rules + manifest generation + `arcc check` as a `local`/no-sandbox test running against the real workspace (unmodified arcc loading path, or minimal changes only); Phase 2: hermetic package-layout loading mode.

## Q2: Rule shape — reference existing `go_library` targets, or generate them?

**Options considered:**
- **Reference existing libs (variant 1):** `interface` points at user-written `go_library` targets; the rule reads their providers. Gazelle-friendly, incremental adoption, visible compile graph; `component_deps` duplication is cross-checked at build time rather than left to drift.
- **Generate the libraries (variant 2):** rule takes `interface_srcs`/`internal_srcs` and generates the `go_library` targets. Single declaration, auto-derivable `component_deps`, but fights Gazelle and hides the compile graph when debugging.
- **Support both:** core rule is variant 1; optional convenience macro layers variant 2 on top. More surface area.

(Note: the `go_proto_library` precedent — a custom rule returning `GoInfo` — means both variants let dependents write `deps = [":service_api_component"]` and have it "just work", so ergonomics of consumption is not a differentiator.)

**Answer:** Reference existing libs (variant 1).

**Design implication:** The component rule consumes `GoInfo`/`GoArchive` from referenced libraries and forwards `GoInfo` so the component target is directly usable in `deps`. Build-time consistency checking of `component_deps` against the actual dep graph becomes part of the design.

## Q3: Interface unit — files or packages?

**Context:** `interface = [":service_api"]` implies package-level public surface; current arcc is file-level (`interface_files` may be a subset of a package's files).

**Options considered:**
- **Packages in Bazel, files in core:** manifest keeps file-level `interface_files`; the Bazel rule emits *all* srcs of the referenced interface libraries as interface files. No arcc schema change; Bazel users get package-level discipline.
- **Shift arcc core to packages:** `interface_packages` as the primary manifest concept; file-level becomes legacy. Cleaner long-term, bigger migration.
- **File-level everywhere:** Bazel rule takes explicit `interface_files`, duplicating go_library srcs.

**Answer:** Packages in Bazel, files in core.

**Design implication:** The manifest generator lists every `.go` src of each library in `interface` as an `interface_files` entry. No manifest schema change needed for this question. Consequence: under Bazel, an interface library's package is public-surface in its entirety — a documented, intentional narrowing.

## Q4: Manifest evolution — component root and membership

**Context:** Bazel generates the manifest into `bazel-out/`, but arcc derives component root, membership, and path resolution from the manifest's directory. Even Phase 1 (local test) therefore needs non-colocation support. Explicit membership (rough-idea point 1) is a separable, larger change.

**Options considered:**
- **Root now, membership later:** Phase 1 adds a component-root override (manifest field or CLI flag); directory-based membership stays. Phase 2 adds explicit package membership together with the package-layout loading mode.
- **Both in Phase 1:** fully explicit semantics from day one; more arcc work before first trial.
- **Root override only (permanently):** keep directory-based membership forever; Bazel rule must ensure component libs live under one subtree.

**Answer:** Root now, membership later.

**Design implication:** Phase 1 arcc change is small: support checking a manifest that is not at the component root (root supplied explicitly; relative paths resolve against the root, not the manifest location). Directory-based membership retained in Phase 1 — the Bazel rule (or its docs) must require that a component's libraries live under a common directory subtree that contains no other component's libs. Explicit membership lands in Phase 2 alongside `--package-layout`.

## Q5: How are absorbed dependencies determined?

**Context:** rough idea sketches "all other deps become absorbed_dependencies" (derivation by subtraction). Tension: derived absorption loses per-dep `reason` documentation and stops being an explicit architectural decision; fully explicit listing is heavy for third-party transitive fan-out. Diamond reachability (package reachable both via a component dep's internals and directly) needs defined semantics either way.

**Options considered:**
- **Explicit direct, derived transitive:** BUILD lists absorbed *direct* deps (optional reasons); transitive closures absorbed implicitly (matches arcc's existing transitive-absorption + pattern semantics). Unaccounted-for direct deps = build error.
- **Fully derived (as sketched):** everything not component-dep-covered is silently absorbed; zero boilerplate, no explicit decision, reasons lost.
- **Fully explicit:** every absorbed package listed/patterned; maximum explicitness, heavy boilerplate.
- **Derived with opt-in strictness:** default derived; `strict_absorption` flag upgrades to explicit-direct.

**Answer (provisional):** Sketch **explicit direct, derived transitive** in the design, with **fully derived** documented as the alternative considered. The user will revisit this when reviewing the design doc.

**Design implication:** Design doc must present option 1 concretely (attribute shape, error messages for unaccounted deps, diamond semantics) and carry a clearly-marked "Alternative: fully derived" subsection so the decision can be flipped cheaply at review time.

## Q6: The `contract` attribute — Bazel-only or manifest extension?

**Context:** arcc's Pillar 2 keeps contracts as doc-comment prose in interface files; the manifest deliberately has no contract field (FR10). The sketch adds `contract = [....md]` standalone documents.

**Options considered:**
- **Bazel-only metadata:** contract files are declared inputs of the component target (graph-visible, cache-invalidating, presence-enforceable); arcc and manifest schema unchanged.
- **Manifest extension:** add `contract_files` to the schema; arcc records/checks standalone contract docs.
- **Drop for now:** no contract attribute until there's concrete check semantics.

**Answer:** Bazel-only metadata.

**Design implication:** `contract` is an attr on the component rule (label list of files), carried in `ArccComponentInfo` and declared as inputs, but never written into the generated manifest. FR10 stands; doc comments remain the Pillar 2 mechanism.

## Q7: Where is the check enforced?

**Options considered:** test target only; test target but validation-action-ready; validation action from day one.

**Answer:** Test target, validation-ready. Phase 1 ships `arcc check` as a test target (as sketched); the check is factored so it can also be exposed as a validation action (failing `bazel build`) later.

## Q8: Representation of `declared_authority` in BUILD files

**Options considered:** labels (marker targets, as sketched); Starlark constants; plain strings.

**Answer:** Starlark constants — `load("@rules_arcc//go:authority.bzl", "FILES")`; load-time typo safety without marker-target boilerplate; rendered to strings in the generated manifest (arcc still validates at parse time).

## Q9: Rule naming

**Options considered:** `go_architectural_component` (as sketched), `go_component`, `arcc_go_component`.

**Answer:** `go_component`. Short, matches arcc's core noun, follows `go_*` ecosystem convention. Field naming details settled in the design doc.

## Q10: Where do the rules live?

**Options considered:** this repo vs. a separate `rules_arcc` repo.

**Answer:** This repo, in a top-level `bazel_rules/` directory (alongside `go/` and `proto/`); consumable via bzlmod override, splittable later if adoption warrants.

**Additional requirement (user):** provide a **bazelified variant of the examples** (the csvtool walkthrough components) demonstrating the rules end to end.

## Q11: How does a consuming workspace obtain the arcc binary?

**Options considered:** build from source via rules_go; prebuilt SLSA-attested release binaries via repository rule/toolchain; full toolchain supporting both.

**Answer:** Build from source via rules_go (`go_binary` in this repo; consumers get it through the bzlmod dependency). Hermetic and version-matched; accepted cost: arcc's Go deps (capslock, x/tools) join consumers' module resolution.

## Q12: Minimum Bazel version / macro style

**Options considered:** Bazel 8+ with symbolic macros; legacy macros for Bazel 6/7 compat.

**Answer:** Bazel 8+, symbolic macros — typed attrs and clean namespacing (`name` + `".check"`) for the `go_component` expansion.

## Q13 (post-spike): Multiple interface libraries?

**Context:** A target can return only one `GoInfo`, so a component with `interface = [":a", ":b"]` cannot forward both — direct dep-ability (`deps = [":component"]`) would only work for single-interface components. (See `research/spike-findings.md`.)

**Answer:** `interface` takes **exactly one** `go_library`. (A component wanting a wider surface re-exports through one interface package.)

**Also noted:** path relativization (provider paths are workspace-relative; the rule must compute the component root and relativize `interface_files`/dep-manifest paths) is a design-level detail interacting with the Q4 root override — to be resolved in the design doc.
