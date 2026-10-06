set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

go_dir := "go"

default: ci

build:
	cd {{go_dir}} && go build ./...
	mkdir -p bin
	cd {{go_dir}} && go build -o ../bin/arcc ./cmd/arcc

# 10m gives headroom for a cold `cmd/arcc/app` run: on a fresh machine (no
# warm ~/.cache/arcc/stdlibmap entry, e.g. every CI runner) the native
# stdlib-authority path generates the whole stdlib map from scratch, which
# alone has been observed to take up to ~5m.
test:
	cd {{go_dir}} && go test -timeout 10m ./...

# Includes the restricted Bazel hermeticity/producer-chain driver; it shares
# the default map generation across its scaling and assertion labels. That
# driver forces Bazel's linux-sandbox strategy, which needs unprivileged
# user-namespace creation — CI relaxes the AppArmor restriction on that in
# its own workflow step (ci.yml), ahead of this recipe.
test-integration:
	cd {{go_dir}} && go test -timeout 10m -tags=integration ./...

# Full stdlib-map coverage is intentionally outside the routine feedback lane:
# it retains the real two-generation determinism and cross-configuration
# assertions with the original semantics.
test-integration-full:
	cd {{go_dir}} && go test -timeout 30m -count=1 -tags='integration stdlibmap_full' ./...

# The routine lane's version/configuration guard compares both CI Go pins and
# the Bazel SDK pin with the checked artifact key through the bounded reader.
stdlibmap-pin-check:
	bash scripts/check-routine-stdlib-map-pin.sh

lint:
	cd {{go_dir}} && go vet ./...
	cd {{go_dir}} && test -z "$(gofmt -l .)"

lint-gate-test:
	bash scripts/test-lint-gate.sh

fmt:
	cd {{go_dir}} && go fmt ./...

gen:
	protoc --proto_path=proto --go_out=go --go_opt=module=github.com/ono-sendai-labs/architectural-contracts/go proto/archcontracts/v1/component.proto proto/archcontracts/v1/surface.proto proto/archcontracts/v1/stdlibmap.proto

# Scoped to the protoc outputs: the manifest and archcontracts gen/ dirs also
# hold handwritten BUILD files, which are not produced by `just gen`.
gen-is-clean: gen
	test -z "$(jj diff -- 'glob:go/internal/**/gen/*.pb.go')"

run *args:
	cd {{go_dir}} && go run ./cmd/arcc {{args}}

selfcheck-staging-test:
	bash scripts/selfcheck-staging-test.sh

authority-probe-test:
	bash scripts/authority-probe-test.sh

probe component:
	bash scripts/authority-probe.sh {{component}}

# Self-check coverage:
# Native `just selfcheck` stages 24 analyzed production components in dependency
# order. All 24 must pass:
#   capanalyzer, atomicfile, hostpolicy, symbol, capslocksymbol, packagelayout,
#   schema, schema-gen, manifest, report, reportio, stdlibauthority, facts,
#   surface, artifactio, checker, capslockadapter, stdlibmap,
#   goanalysis, parsecsv, csvfile, toprow, app, cli.
# The `protobuf-runtime`, `x-tools`, and `capslock-runtime` wrappers are
# asserted UNKNOWN package surfaces: they provide a checked-in surface and
# intentionally produce no report or analysis.
#
# The Bazel wildcard has 21 checked component gates: the seventeen internal
# components capanalyzer, atomicfile, hostpolicy, symbol, capslocksymbol,
# packagelayout, schema, schema-gen, stdlibauthority, facts, report, reportio,
# checker, manifest, goanalysis, artifactio, and surface, plus
# the four CSV components parsecsv, csvfile, toprow, and app. The two Bazel
# wrappers use `authority = UNKNOWN` for asserted surfaces; capslock-runtime,
# capslockadapter, and cli have no Bazel component gate because their closure
# contains cgo; and stdlibmap is covered by dedicated map-generation tests
# rather than a go_component check.
#
# hostpolicy previously rode inside symbol's member set; it is now its own
# dependency-only leaf component, declared by symbol and consumed through
# component dependencies by the shell components that use its policy hooks.
#
# The other three components — capslock-runtime, capslockadapter, and cli — are
# native-only because capslock's closure contains golang.org/x/sys/unix built
# with cgo. The Bazel arcc rule fails closed on cgo closures, and cli (cmd/arcc)
# imports capslockadapter. Native mode handles cgo via go/packages
# preprocessing.
#
# Keeping both legs is deliberate: native FR1 membership and Bazel declared
# membership checking the same components cross-check the whole membership
# model. Step 7 gives the foreign protobuf and x/tools closures one package-
# surface boundary each, and Step 9 adds capslock-runtime. The parsecsv and
# schema-gen residuals keep their AnalysisDefeating sites visible through their
# explicit WARN policy, while their native report verdicts still gate this
# recipe.
selfcheck:
	@echo "=== Validating pinned selfcheck toolchain ==="
	@test "$(cd {{go_dir}} && go env GOVERSION)" = "go1.26.4"
	@test "$(cd {{go_dir}} && go env GOOS)" = "linux"
	@test "$(cd {{go_dir}} && go env GOARCH)" = "amd64"
	@test "$(cd {{go_dir}} && go env CGO_ENABLED)" = "0"
	@test -z "$(cd {{go_dir}} && go env GOEXPERIMENT)"
	@test -f "$(pwd)/go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json"
	@echo "=== Building own arcc ==="
	mkdir -p bin
	cd {{go_dir}} && go build -o ../bin/arcc ./cmd/arcc
	@echo "=== Staging self-hosting surfaces in dependency order ==="
	# The order is the component-dependency topological order. Every analyzed
	# staging component produces its report/surface beside its manifest, so native
	# convention lookup never reads a developer cache or an uncontrolled host path.
	# The asserted protobuf and x-tools wrappers supply their checked-in surfaces
	# and no reports; native staging never loads either foreign source tree.
	# Every analyzed staging report is decoded and asserted immediately with the
	# expected verdict; all staged artifacts are discarded on exit.
	@selfcheck_stage="$(mktemp -d "${TMPDIR:-/tmp}/arcc-selfcheck.XXXXXX")"; \
	trap 'rm -rf "$selfcheck_stage"' EXIT; \
	cp -a "$(pwd)/go" "$selfcheck_stage/go"; \
	arcc_bin="$(pwd)/bin/arcc"; \
	stage_helper="$(pwd)/scripts/selfcheck-staging.sh"; \
	map_path="$selfcheck_stage/go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json"; \
	cd "$selfcheck_stage/go"; \
	source "$stage_helper"; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/capanalyzer/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/atomicfile/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/hostpolicy/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/symbol/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/capslocksymbol/component.textproto; \
	selfcheck_asserted_component internal/protobufruntime/component.textproto; \
	selfcheck_asserted_component internal/xtools/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/packagelayout/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/schema/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/schema/gen/component.textproto; \
	selfcheck_assert_analysis_defeating_sites internal/schema/gen/component.report.json 15; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/manifest/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/report/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/stdlibauthority/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/facts/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/surface/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/artifactio/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/reportio/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/checker/component.textproto; \
	selfcheck_asserted_component internal/capslockruntime/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/capslockadapter/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/stdlibmap/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/goanalysis/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" examples/csvtool/internal/parsecsv/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" examples/csvtool/csvfile/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" examples/csvtool/toprow/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" examples/csvtool/app/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" cmd/arcc/component.textproto

bazel-test-full:
	bazel test --define=stdlibmap_full=true //bazel_rules/go/tests:arcc_deps_aspect_full_tests
	bazel build --define=stdlibmap_full=true //:arcc_stdlib_map_replica //bazel_rules/go/tests:stdlib_map_darwin_arm64 //bazel_rules/go/tests:stdlib_map_tagged //bazel_rules/go/tests:darwin_arm64_map
	bazel test --define=stdlibmap_full=true //bazel_rules/go/tests:stdlib_map_tests //bazel_rules/go/tests:stdlib_map_full_tests //bazel_rules/go/tests:stdlib_map_keys_test //bazel_rules/go/tests:transitioned_surface_sdk_key_test
	bazel test --define=stdlibmap_full=true //bazel_rules/go/tests:platform_layout_test

# Bazel build + test leg. Catches rules/Starlark and hermetic-check
# regressions the Go tests cannot see (design §8.5). It fails loudly if Bazel
# is missing rather than skipping, so a CI environment that is meant to have
# Bazel cannot quietly pass a partial `just ci`.
bazel-test:
	bazel build //...
	bazel test //...
	TEST_SRCDIR="$PWD" TEST_TMPDIR="${TMPDIR:-/tmp}" bazel_rules/go/tests/members_label_validation_test.sh
	TEST_SRCDIR="$PWD" TEST_TMPDIR="${TMPDIR:-/tmp}" bazel_rules/go/tests/component_shape_validation_test.sh
	@# Tool errors must fail the checked analysis action (AC 2): requesting the
	@# arcc output group of the malformed fixture must fail the build, while the
	@# wildcard default build above stayed green (the fixture uses a scheduling-only manual tag).
	@# The log lands in a per-run mktemp path, so concurrent legs never share a file.
	out="$(mktemp "${TMPDIR:-/tmp}/broken_go_component.XXXXXX.out")" && { if bazel build --output_groups=+arcc //bazel_rules/go/tests/testdata/malformed:broken_go_component >"$out" 2>&1; then echo "FAIL: expected the malformed component's import-graph or ArccCheck action to fail" >&2; rm -f "$out"; exit 1; fi; grep -Eq "failed to load package facts|package-layout loading failed|Projecting imports for component" "$out"; rc=$?; rm -f "$out"; exit $rc; }

# Go-only leg: everything except selfcheck and bazel-test. Mirrors the
# ci.yml "Go" job so the three CI jobs can run in parallel on separate
# runners; selfcheck and bazel-test run as their own CI jobs.
ci-go: gen lint lint-gate-test build test test-integration stdlibmap-pin-check selfcheck-staging-test authority-probe-test

ci: gen-is-clean ci-go selfcheck bazel-test

clean:
	cd {{go_dir}} && go clean ./...
