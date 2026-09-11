set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

go_dir := "go"

default: ci

build:
	cd {{go_dir}} && go build ./...
	mkdir -p bin
	cd {{go_dir}} && go build -o ../bin/arcc ./cmd/arcc

test:
	cd {{go_dir}} && go test -timeout 5m ./...

test-integration:
	cd {{go_dir}} && go test -timeout 10m -tags=integration ./...

# Full stdlib-map coverage is intentionally outside the routine feedback lane:
# it retains the real two-generation determinism and cross-configuration
# assertions with the original semantics.
test-integration-full:
	cd {{go_dir}} && go test -timeout 30m -tags='integration stdlibmap_full' ./...

# The routine lane's version/configuration guard compares both CI Go pins and
# the Bazel SDK pin with the checked artifact key through the bounded reader.
stdlibmap-pin-check:
	bash scripts/check-routine-stdlib-map-pin.sh

lint:
	cd {{go_dir}} && go vet ./...
	cd {{go_dir}} && test -z "$(gofmt -l .)"

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

# Self-check coverage:
# Native `just selfcheck` stages 20 analyzed production components in dependency
# order. Nineteen must pass:
#   capanalyzer, hostpolicy, symbol, manifest, report, stdlibauthority, facts,
#   surface, artifactio, checker, capslockadapter, packagelayout, stdlibmap,
#   goanalysis, parsecsv, csvfile, toprow, app, cli.
# `schema` is the one explicit expected-fail analysis because its generated
# protobuf code retains documented UNANALYZED findings. The `protobuf-runtime`
# and `x-tools` wrappers are asserted UNKNOWN package surfaces: they provide a
# checked-in surface and intentionally produce no report or analysis.
#
# The Bazel wildcard has 16 checked component gates: the twelve internal
# components capanalyzer, hostpolicy, symbol, stdlibauthority, facts, report,
# checker, manifest, goanalysis, artifactio, surface, and packagelayout, plus
# the four CSV components parsecsv, csvfile, toprow, and app. Schema's check is manual for
# the same expected generated-protobuf failure; the two wrappers are manual
# asserted surfaces; capslockadapter and cli have no Bazel component gate
# because their closure contains cgo; and stdlibmap is covered by dedicated
# map-generation tests rather than a go_component check.
#
# hostpolicy previously rode inside symbol's member set; it is now its own
# dependency-only leaf component, declared by symbol and consumed through
# component dependencies by the shell components that use its policy hooks.
#
# The other two components — capslockadapter and cli — are native-only because
# capslock's closure contains golang.org/x/sys/unix built with cgo. The Bazel
# arcc rule fails closed on cgo closures, and cli (cmd/arcc) imports
# capslockadapter. Native mode handles cgo via go/packages preprocessing.
#
# Keeping both legs is deliberate: native FR1 membership and Bazel declared
# membership checking the same components cross-check the whole membership
# model. Step 7 gives the foreign protobuf and x/tools closures one package-
# surface boundary each. The parsecsv and capslockadapter residuals keep every
# AnalysisDefeating site visible through their explicit WARN policy, while
# their native report verdicts still gate this recipe.
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
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/hostpolicy/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/symbol/component.textproto; \
	selfcheck_asserted_component internal/protobufruntime/component.textproto; \
	selfcheck_asserted_component internal/xtools/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/packagelayout/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/schema/component.textproto fail; \
	echo "schema dependency artifact verdict: fail (expected generated-protobuf UNANALYZED; staged and discarded with the temporary stage)"; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/manifest/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/report/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/stdlibauthority/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/facts/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/surface/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/artifactio/component.textproto; \
	selfcheck_stage_component "$arcc_bin" "$map_path" internal/checker/component.textproto; \
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
	@# wildcard default build above stayed green (the fixture is tagged manual).
	@# The log lands in a per-run mktemp path, so concurrent legs never share a file.
	out="$(mktemp "${TMPDIR:-/tmp}/broken_go_component.XXXXXX.out")" && { if bazel build --output_groups=+arcc //bazel_rules/go/tests/testdata/malformed:broken_go_component >"$out" 2>&1; then echo "FAIL: expected the malformed component's import-graph or ArccCheck action to fail" >&2; rm -f "$out"; exit 1; fi; grep -Eq "failed to load package facts|package-layout loading failed|Projecting imports for component" "$out"; rc=$?; rm -f "$out"; exit $rc; }

# Go-only leg: everything except selfcheck and bazel-test. Mirrors the
# ci.yml "Go" job so the three CI jobs can run in parallel on separate
# runners; selfcheck and bazel-test run as their own CI jobs.
ci-go: gen build test test-integration stdlibmap-pin-check selfcheck-staging-test

ci: gen-is-clean ci-go selfcheck bazel-test

clean:
	cd {{go_dir}} && go clean ./...
