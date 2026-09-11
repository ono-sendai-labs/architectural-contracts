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

This inventory describes the final component-analysis action, not every action in the
producer chain. Because the pinned rules_go provider does not expose the exact ordinary
import graph, one hermetic, cached `ArccImportGraph` action per checked component reads
the target-selected ordinary non-member source closure for a limited lexical import
projection; `ArccLayout` then merges its descriptor into the base layout. That source
is permitted only on the metadata producer and never reaches `ArccCheck`. The
projection adds no semantic analysis: it does not type-check or run the typed reference
scan, SSA, VTA, or Capslock over non-members.

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
benchmark or a timeout requirement. It also does not measure the separate
`ArccImportGraph` lexical projection, whose ordinary-source input volume and scan
time remain closure-shaped.

## Producer-chain measurement required by Step 13

The deferred depth/width benchmark must measure the complete Bazel producer chain,
not only the `ArccCheck` loader. For dependency depths 1, 4, and 16, record:

- the number of `ArccImportGraph`, `ArccLayout`, and `ArccCheck` actions;
- the ordinary non-member source input file count and byte total for each
  `ArccImportGraph` action;
- the wall-time/elapsed contribution of import projection and layout merge separately
  from `ArccCheck` loader time; and
- the non-member export artifact count/bytes and `ArccCheck` loader time.

These measurements make the cached lexical projection's residual closure scaling
visible while preserving the source asymmetry: only `ArccImportGraph` sees ordinary
non-member source, and `ArccCheck` sees the final Step 8 allowlist.

## Deterministic accounting

Metrics are computed after the validated load boundary from resolved `ExportFile`
paths. Duplicate package records sharing a path count once; sizes are summed with
checked unsigned overflow; missing, non-regular, unreadable, or unstated export
artifacts are tool errors. Member source, layout/graph metadata, surfaces/reports,
and the stdlib map are excluded from the total. Report canonicalization preserves
the scalar diagnostics fields and derives the verdict only from violations.
