# Step 8 input-pruning and export-data diagnostics

**Date:** 2026-09-11  
**Scope:** Step 8 Task 6 final action-input contract and a representative deep
closure demonstration.

## Final `ArccCheck` input inventory

The checked component-analysis action declares only:

- the target-selected `.go` files for the effective member roots;
- the generated component manifest and package layout;
- ordinary non-member compiler export artifacts and the ordinary-import graph
  descriptor;
- the target-configured standard-library export metadata and generated export
  trees (`gocache`/`pkg`);
- each direct dependency's surface and applicable report; and
- the target SDK-keyed stdlib authority map.

The action does not declare or receive dependency/SDK `.go` source, a Go or other
toolchain binary, an undeclared cache, or network access. The auxiliary
`ArccImportGraph` metadata action may read ordinary source to project direct
imports; that is not a component analysis action input. The Bazel analysis test
`checked_action_inputs_test` asserts the positive inventory and the exact set of
member-only `.go` inputs.

## Deep-closure demonstration

The integration fixture
`go/internal/goanalysis/member_only_integration_test.go:79` creates a member →
dependency → deep-dependency graph, stages export data for the two non-member
packages, then removes both dependency source directories before loading. The
member-only check still resolves the typed `Uses`/selection facts and reports
the exact deduplicated diagnostics: **2 artifacts, 5,000 bytes**. The test binary
run on the pinned Linux/amd64 Go 1.26.4 environment took **0.020 s wall** for the
deep-closure case (`-test.run TestLoadPackageFacts_DeepExportClosureWithoutDependencySources`).

For comparison, the Step 6 post-cutover measurement recorded the four-package
`examples/csvtool/app` native check at **0.78 s wall / 2.87 s CPU**, with source
loading for the closure still present, and the 196-package `internal/goanalysis`
check at **1.46 s wall / 6.28 s CPU**. The new 20 ms figure is the focused
member-only loader test rather than a full CLI invocation, so it is evidence of
the source-absent load boundary, not a replacement for the Step 13 scaling
benchmark or a timeout requirement.

## Deterministic accounting

Metrics are computed after the validated load boundary from resolved `ExportFile`
paths. Duplicate package records sharing a path count once; sizes are summed with
checked unsigned overflow; missing, non-regular, unreadable, or unstated export
artifacts are tool errors. Member source, layout/graph metadata, surfaces/reports,
and the stdlib map are excluded from the total. Report canonicalization preserves
the scalar diagnostics fields and derives the verdict only from violations.
