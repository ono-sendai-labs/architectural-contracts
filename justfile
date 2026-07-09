set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

go_dir := "go"

default: ci

build:
	cd {{go_dir}} && go build ./...

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
	# Protobuf code generation is wired in Step 2.
	true

run *args:
	cd {{go_dir}} && go run ./cmd/arcc {{args}}

ci: gen lint build test test-integration

clean:
	cd {{go_dir}} && go clean ./...
