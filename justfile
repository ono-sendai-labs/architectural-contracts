set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

go_dir := "go"

default: ci

build:
	cd {{go_dir}} && go build ./...
	mkdir -p bin
	cd {{go_dir}} && go build -o ../bin/arcc ./cmd/arcc

test:
	cd {{go_dir}} && go test -timeout 2m ./...

test-integration:
	cd {{go_dir}} && go test -timeout 5m -tags=integration ./...

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

# Self-check leg relationship:
# The Bazel `.check` targets run by `bazel test //...` (in //go/internal/...)
# correspond one-to-one with the pure components checked here:
#   capanalyzer, hostpolicy, symbol, stdlibauthority, facts, report, checker,
#   manifest, goanalysis, artifactio, surface.
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
# schema has no standalone selfcheck gate: it is a generated-schema dependency
# whose surface is consumed by manifest and artifactio.
#
# Keeping both legs is deliberate: native FR1 membership and Bazel declared
# membership checking the same components cross-checks the whole membership model.
# Step 7 gives the foreign protobuf and x/tools closures one package-surface
# boundary each, so the real component manifests can be staged without
# dependency overlap. The remaining non-gating cases are the generated schema
# surface, goanalysis/capslockadapter's native-only analysis limits, and the
# csvtool parser's residual UNANALYZED result.
# Their report verdicts are computed for dependency consumption and deliberately
# discarded with this temporary tree; they are not claimed to be persisted or
# promoted to the gate. `schema` is not one of those five and has no standalone
# selfcheck gate: its generated protobuf code currently produces an expected
# UNANALYZED verdict while it is consumed as a dependency artifact. That
# expected non-gating verdict is checked explicitly below and then discarded.
# The adopted `protobuf-runtime` is different: its checked-in native surface is
# the pinned empty-digest UNKNOWN assertion, so staging it performs no analysis
# and creates no report. Bazel uses the component-level `tags = ["manual"]`
# asserted producer for the same boundary.
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
	# The asserted protobuf wrapper supplies its checked-in surface and no report.
	# Staging verdicts are consumed in this temporary tree and discarded on exit;
	# only the final real selfcheck commands below are gate assertions.
	@selfcheck_stage="$(mktemp -d "${TMPDIR:-/tmp}/arcc-selfcheck.XXXXXX")"; \
	trap 'rm -rf "$selfcheck_stage"' EXIT; \
	cp -a "$(pwd)/go" "$selfcheck_stage/go"; \
	arcc_bin="$(pwd)/bin/arcc"; \
	map_path="$selfcheck_stage/go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json"; \
	cd "$selfcheck_stage/go"; \
	stage_component() { \
		manifest="$1"; base="${manifest%.textproto}"; stage_manifest="$manifest"; \
		"$arcc_bin" check "$stage_manifest" --stdlib-map="$map_path" \
			--report-out="$base.report.json" --surface-out="$base.surface.json" \
			--report-verdict-only >/dev/null; \
	}; \
	stage_asserted_component() { \
		manifest="$1"; base="${manifest%.textproto}"; \
		test -f "$base.surface.json" || { echo "missing native asserted surface: $base.surface.json" >&2; exit 1; }; \
		test ! -e "$base.report.json" || { echo "asserted component unexpectedly has a report: $base.report.json" >&2; exit 1; }; \
	}; \
	stage_component internal/capanalyzer/component.textproto; \
	stage_component internal/hostpolicy/component.textproto; \
	stage_component internal/symbol/component.textproto; \
	stage_asserted_component internal/protobufruntime/component.textproto; \
	stage_component internal/xtools/component.textproto; \
	stage_component internal/schema/component.textproto; \
	schema_verdict="$(sed -n 's/.*\"verdict\": \"\([^\"]*\)\".*/\1/p' internal/schema/component.report.json | head -n 1)"; \
	if [ "${schema_verdict}" != "fail" ]; then echo "schema staging verdict changed: got ${schema_verdict}, want the expected generated-protobuf UNANALYZED fail" >&2; exit 1; fi; \
	echo "schema dependency artifact verdict: fail (expected generated-protobuf UNANALYZED; non-gating and discarded with the temporary stage)"; \
	stage_component internal/manifest/component.textproto; \
	stage_component internal/report/component.textproto; \
	stage_component internal/stdlibauthority/component.textproto; \
	stage_component internal/facts/component.textproto; \
	stage_component internal/surface/component.textproto; \
	stage_component internal/artifactio/component.textproto; \
	stage_component internal/checker/component.textproto; \
	stage_component internal/capslockadapter/component.textproto; \
	stage_component internal/stdlibmap/component.textproto; \
	stage_component internal/goanalysis/component.textproto; \
	stage_component examples/csvtool/internal/parsecsv/component.textproto; \
	stage_component examples/csvtool/csvfile/component.textproto; \
	stage_component examples/csvtool/toprow/component.textproto; \
	stage_component examples/csvtool/app/component.textproto; \
	echo "=== Running self-hosting checks (Pillar 3 authority-free core) ==="; \
	"$arcc_bin" check internal/checker/component.textproto --stdlib-map="$map_path"; \
	"$arcc_bin" check internal/surface/component.textproto --stdlib-map="$map_path"; \
	echo "=== Running self-hosting checks (remaining real manifests) ==="; \
	"$arcc_bin" check internal/manifest/component.textproto --stdlib-map="$map_path" >/dev/null; \
	"$arcc_bin" check internal/artifactio/component.textproto --stdlib-map="$map_path" >/dev/null; \
	"$arcc_bin" check cmd/arcc/component.textproto --stdlib-map="$map_path" >/dev/null; \
	"$arcc_bin" check examples/csvtool/app/component.textproto --stdlib-map="$map_path" >/dev/null

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
	out="$(mktemp "${TMPDIR:-/tmp}/broken_go_component.XXXXXX.out")" && { if bazel build --output_groups=+arcc //bazel_rules/go/tests/testdata/malformed:broken_go_component >"$out" 2>&1; then echo "FAIL: expected the malformed component's ArccCheck action to fail" >&2; rm -f "$out"; exit 1; fi; grep -q "failed to load package facts" "$out"; rc=$?; rm -f "$out"; exit $rc; }

# Go-only leg: everything except selfcheck and bazel-test. Mirrors the
# ci.yml "Go" job so the three CI jobs can run in parallel on separate
# runners; selfcheck and bazel-test run as their own CI jobs.
ci-go: gen-is-clean lint build test test-integration stdlibmap-pin-check

ci: ci-go selfcheck bazel-test

clean:
	cd {{go_dir}} && go clean ./...
