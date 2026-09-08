.PHONY: test race vet fmt fmt-check build ci

GO ?= go
BIN := bin/agent-undo

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

build:
	$(GO) build -o $(BIN) ./cmd/agent-undo

# Supported v0.1 targets. Windows is intentionally absent.
build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(GO) build -o bin/agent-undo-darwin-arm64 ./cmd/agent-undo

build-linux-amd64:
	GOOS=linux GOARCH=amd64 $(GO) build -o bin/agent-undo-linux-amd64 ./cmd/agent-undo

ci: fmt-check vet test race
