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
# dependency-only leaf component, declared by symbol and owned as a duplicate
# transitional member by capslockadapter, stdlibmap, and goanalysis.
#
# The other two components — capslockadapter and cli — are native-only because
# capslock's closure contains golang.org/x/sys/unix built with cgo. The Bazel
# arcc rule fails closed on cgo closures, and cli (cmd/arcc) imports
# capslockadapter. Native mode handles cgo via go/packages preprocessing.
#
# schema has no standalone selfcheck entry: it is a dependency-only component
# whose manifest is parsed and whose interface files are validated every time
# manifest or artifactio resolve it as a component dependency.
#
# Keeping both legs is deliberate: native FR1 membership and Bazel declared
# membership checking the same components cross-checks the whole membership model.
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
	@echo "=== Running self-hosting checks (Pillar 3 authority-free core) ==="
	cd {{go_dir}} && ../bin/arcc check internal/checker/component.textproto --stdlib-map=internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json
	# EXEMPT (Step 6 task 05 AC8b; TODO(Step 7 protobuf-runtime PACKAGE_SURFACE
	# migration): artifactio's transitional protobuf-runtime members reference
	# stdlib records the authority map honestly preserves UNANALYZED (I4) —
	# 102 AnalysisDefeating sites (unsafe.Pointer x52, unsafe.Sizeof x5,
	# unsafe.Slice x2, unsafe.SliceData x2, unsafe.String x2,
	# unsafe.StringData x2, (sync.Once).Do x17, sort.Slice x11,
	# (sync.Pool).Get x3, io.WriteString x2, io.ReadAll x2,
	# compress/gzip.NewReader x1, errors.As x1) plus one OPERATING_SYSTEM
	# finding (os.Stderr at reflect/protoregistry/registry.go:58). Step 7
	# moves those packages into the protobuf-runtime component and removes
	# this exemption.
	# cd {{go_dir}} && ../bin/arcc check internal/artifactio/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/surface/component.textproto --stdlib-map=internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json
	@echo "=== Running self-hosting checks (remaining components) ==="
	# EXEMPT (Step 6 task 05 AC8b; TODO(Step 7 protobuf-runtime PACKAGE_SURFACE
	# migration): the same transitional protobuf-runtime members as
	# artifactio — 102 AnalysisDefeating sites (same symbol set) plus
	# os.Open/os.Stderr findings (FILES x2, MODIFY_SYSTEM_STATE x1,
	# OPERATING_SYSTEM x1). Step 7 removes the exemption.
	# cd {{go_dir}} && ../bin/arcc check internal/manifest/component.textproto
	# EXEMPT (Step 6 task 05 AC8b; TODO(Step 7 x/tools PACKAGE_SURFACE
	# wrapper): goanalysis's retained x/tools member closure references
	# UNANALYZED stdlib records — 88 AnalysisDefeating sites (go/build.Default
	# x13, (sync.Once).Do x9, io.Copy x9, go/parser.ParseFile x8,
	# io.ReadFull x6, sort.Sort x5, unsafe.String x1, unsafe.StringData x1,
	# ...) plus 6 RUNTIME sites (runtime.init).
	# Step 7 moves the x/tools packages into a wrapper component and removes
	# this exemption.
	# cd {{go_dir}} && ../bin/arcc check internal/goanalysis/component.textproto
	# EXEMPT (Step 6 task 05 AC8b; TODO(Step 7 residual-UNANALYZED decision):
	# capslockadapter owns capslock as member code, whose own source
	# references UNANALYZED stdlib records — 1077 AnalysisDefeating sites
	# (unsafe.Pointer x570, (sync.Once).Do x30, unsafe.Sizeof x28,
	# sort.Slice x20, io.WriteString x13, sort.Sort x11, unsafe.Slice x9,
	# go/build.Default x5, ...). Neither
	# the protobuf nor the x/tools wrapper owns capslock; only the Step 7
	# residual-UNANALYZED decision (capability-use indirection curation or
	# the DR-11 policy carrier) can clear these.
	# cd {{go_dir}} && ../bin/arcc check internal/capslockadapter/component.textproto
	cd {{go_dir}} && ../bin/arcc check cmd/arcc/component.textproto --stdlib-map=internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json

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
