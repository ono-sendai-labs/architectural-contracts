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

selfcheck:
	cd {{go_dir}} && go run ./cmd/arcc check internal/checker/component.textproto
	cd {{go_dir}} && go run ./cmd/arcc check internal/facts/component.textproto
	cd {{go_dir}} && go run ./cmd/arcc check internal/report/component.textproto
	cd {{go_dir}} && go run ./cmd/arcc check internal/capanalyzer/component.textproto
	cd {{go_dir}} && go run ./cmd/arcc check internal/manifest/component.textproto
	cd {{go_dir}} && go run ./cmd/arcc check internal/goanalysis/component.textproto
	cd {{go_dir}} && go run ./cmd/arcc check internal/capslockadapter/component.textproto
	cd {{go_dir}} && go run ./cmd/arcc check cmd/arcc/component.textproto

ci: gen-is-clean lint build test test-integration selfcheck

clean:
	cd {{go_dir}} && go clean ./...
