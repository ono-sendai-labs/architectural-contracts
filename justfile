set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

go_dir := "go"

default: ci

build:
	cd {{go_dir}} && go build ./...
	mkdir -p bin
	cd {{go_dir}} && go build -o ../bin/arcc ./cmd/arcc

test:
	cd {{go_dir}} && go test ./...

test-integration:
	cd {{go_dir}} && go test -tags=integration ./...

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
	@echo "=== Building own arcc ==="
	mkdir -p bin
	cd {{go_dir}} && go build -o ../bin/arcc ./cmd/arcc
	@echo "=== Running self-hosting checks (Pillar 3 authority-free core) ==="
	cd {{go_dir}} && ../bin/arcc check internal/checker/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/facts/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/report/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/capanalyzer/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/hostpolicy/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/symbol/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/stdlibauthority/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/artifactio/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/surface/component.textproto
	@echo "=== Running self-hosting checks (remaining components) ==="
	cd {{go_dir}} && ../bin/arcc check internal/manifest/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/goanalysis/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/capslockadapter/component.textproto
	cd {{go_dir}} && ../bin/arcc check cmd/arcc/component.textproto

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
ci-go: gen-is-clean lint build test test-integration

ci: ci-go selfcheck bazel-test

clean:
	cd {{go_dir}} && go clean ./...
