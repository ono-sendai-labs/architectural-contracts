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

# Bazel build + test leg. Guarded on Bazel being installed so environments
# without it still pass `just ci` on the Go leg alone; where Bazel is present
# this catches rules/Starlark and hermetic-check regressions the Go tests
# cannot see (design §8.5). Step 7 finalizes the CI story; wired in early so a
# green `just ci` means the Bazel targets are green too.
bazel-test:
	#!/usr/bin/env bash
	set -euo pipefail
	if ! command -v bazel >/dev/null 2>&1; then
		echo "=== Bazel not found on PATH; skipping the Bazel leg ==="
		exit 0
	fi
	echo "=== bazel build //... ==="
	bazel build //...
	echo "=== bazel test //... ==="
	bazel test //...

ci: gen-is-clean lint build test test-integration selfcheck bazel-test

clean:
	cd {{go_dir}} && go clean ./...
