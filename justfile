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
	protoc --proto_path=proto --go_out=go --go_opt=module=github.com/ono-sendai-labs/architectural-contracts/go proto/archcontracts/v1/component.proto

# Scoped to the protoc outputs: gen/ also holds a Gazelle-generated BUILD file,
# which is not produced by `just gen`.
gen-is-clean: gen
	test -z "$(jj diff -- 'glob:go/internal/manifest/gen/**/*.pb.go')"

run *args:
	cd {{go_dir}} && go run ./cmd/arcc {{args}}

selfcheck:
	@echo "=== Building own arcc ==="
	mkdir -p bin
	cd {{go_dir}} && go build -o ../bin/arcc ./cmd/arcc
	@echo "=== Running self-hosting checks (Pillar 3 authority-free core) ==="
	cd {{go_dir}} && ../bin/arcc check internal/checker/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/facts/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/report/component.textproto
	cd {{go_dir}} && ../bin/arcc check internal/capanalyzer/component.textproto
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

ci: gen-is-clean lint build test test-integration selfcheck bazel-test

clean:
	cd {{go_dir}} && go clean ./...
