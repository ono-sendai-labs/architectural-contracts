# Step 9 golden-footprint demo

**Date:** 2026-09-11

This note records the final Step 9 audit of checked-in golden and golden-style
testdata. The inventory is deliberately based on repository paths and BUILD
labels, not on a report's current contents:

```sh
find bazel_rules/go/tests/goldens -maxdepth 1 -type f -printf '%f\n' | sort
find bazel_rules/go/tests -type f \( -name '*.golden' -o -name '*.golden.json' -o -name '*.textproto' -o -name '*.json' -o -name '*.data' -o -name '*.txt' \) -printf '%p\n' | sort
```

The first command is the canonical golden inventory. The second captures
golden-style inputs outside the golden directory so they can be classified
without accidentally treating ordinary Go test data as a semantic golden.

## Baseline and result

The downstream friction report records 50 `rules_arcc` patches, 45 of them
goldens or testdata ([host-import-friction.md, §6](host-import-friction.md)).
The repository-local before/after inventory is:

| Category | Before | After | Classification and action |
| --- | ---: | ---: | --- |
| Semantic verdict goldens | 3 | 3 | `*.verdict.golden`; pass/fail only, guarded by the portability test |
| Persisted report-shape goldens | 4 | 4 | Intentional report schema/axis/finding shape; metric/source values use named placeholders |
| Persisted surface-shape goldens | 3 | 3 | Intentional checked/asserted surface shape; target SDK/digest values use named placeholders |
| Bounded stdlib-map shape fixture + golden | 2 | 2 | One small fixture and one shape golden; no second whole-SDK map |
| Final package-layout shape goldens | 0 | 2 | Typed API/member layouts replaced the old raw snapshots |
| Redundant generated-manifest byte goldens | 2 | 0 | Deleted; generated manifests are covered by `manifestparity` |
| Combined manifest/layout golden helper | 1 | 0 | Deleted; layout uses `artifact-shape layout` |
| Golden-directory files total | 16 | 14 | Four redundant files removed, two typed layout goldens retained |

The golden-style inputs outside `goldens/` are unchanged and unrelated to the
semantic/layout split: `hostile_verdict_input.txt`, `pattern_cases.json`,
`test_export_a.data`, `test_export_b.data`,
`testdata/malformed/broken.component.textproto`, and
`testdata/nongo/nongo_data.txt`. They are adversarial, parser, provider, or
non-Go fixture inputs rather than expected verdict or layout snapshots.

## Retained host-patchable shapes

These are the only intentional shape artifacts a downstream host may need to
regenerate when its target or artifact paths differ. They are kept because each
pins a distinct persisted contract:

- `goldens/api_component.layout.shape.golden.json` — a checked component with
  a direct dependency surface/report binding, member source roles, ordinary
  import metadata, stdlib export roots, and target identity. Workspace/export,
  SDK, and target values are named placeholders.
- `goldens/member_component.layout.shape.golden.json` — the member/non-member
  export-data closure with complete non-member import maps and direct export
  files. It intentionally retains the member source fields separately from the
  non-member export role.
- `goldens/*.report.shape.golden.json` — four report envelope/dependency/finding
  contracts. Export metrics, source locations, evidence, messages, and SDK
  identity are represented by explicit placeholders.
- `goldens/*.surface.shape.golden.json` — checked declared-interface,
  checked package-surface, and asserted package-surface/UNKNOWN contracts.
  Target SDK scalars and derived content digests are placeholders; empty
  asserted symbols/digest remain exact.
- `goldens/stdlib_map_shape.golden.json` — one bounded map contract containing
  SDK key, package/symbol/init/evidence ordering and representative terminal
  classifications. It is driven by `stdlib_map_shape.fixture.json`, not by
  whole-SDK generation.

Semantic verdict goldens are not in this list: the `glob(["goldens/*.verdict.golden"])`
inventory and `verdict_golden_portability_test.sh` reject absolute paths,
package closure, SDK/export metadata, diagnostic metrics, source locations, and
configured import prefixes. The guard runs synthetic contamination cases and
requires a diagnostic naming both the temporary file and the forbidden class.

## Rewritten-prefix demonstration

The existing verdict regression exercises two byte-identical semantic reports
under an idempotent simulated prefix rewrite:

```sh
cd go
go test ./cmd/arcc/app -run '^TestVerdictGolden_ImportPrefixRewritePreservesBytesAndResult$'
```

Result: `ok`; both reports produce the same `pass`/`fail` verdict bytes and the
same verdict-golden outcome. The layout shape seam has the corresponding
import-path normalization check:

```sh
go test ./cmd/arcc/app -run '^TestLayoutShapeSnapshotCanonicalizesImportPaths$'
```

Result: `ok`; `host.example/...` package IDs, roots, and import-map keys are
canonicalized to the same snapshot as `example.com/...`. Layout-only path and
target differences remain visible through the documented named placeholders,
while semantic verdict files remain byte-stable.

## Maintenance commands

For a deliberate layout-shape change, build the final layout and print the
typed normalized snapshot, then review it and update the shape golden:

```sh
bazel build --output_groups=+arcc //bazel_rules/go/tests/testdata/api:api_component
bazel-bin/go/cmd/arcc/arcc_/arcc artifact-shape layout \
  bazel-bin/bazel_rules/go/tests/testdata/api/api_component.package-layout.json --print
```

The same command applies to `member_component`. Its output is a schema-aware
snapshot, not a raw package-layout copy: it validates canonical JSON, final
member/non-member export roles, complete import maps, SDK/export-root shape,
and target metadata before normalization. Do not copy the raw layout into a
`*.verdict.golden`; semantic verdicts are updated only as inert one-line
`pass`/`fail` data and are covered by the portability guard.
