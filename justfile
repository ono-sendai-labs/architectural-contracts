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
	protoc --proto_path=proto --go_out=go --go_opt=module=github.com/ono-sendai-labs/architectural-contracts/go proto/archcontracts/v1/component.proto

gen-is-clean: gen
	test -z "$(jj diff -- go/internal/manifest/gen/)"

run *args:
	cd {{go_dir}} && go run ./cmd/arcc {{args}}

ci: gen-is-clean lint build test test-integration

clean:
	cd {{go_dir}} && go clean ./...
