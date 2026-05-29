BINARY=bin/claude-llama-mcp
VERSION ?= dev
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test bench integration lint setup clean

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/claude-llama-mcp

test:
	go test -race ./...

bench:
	go test -tags bench ./internal/tools/ -run TestSavings -v

integration:
	go test -tags integration ./cmd/claude-llama-mcp/ -run TestSmoke -v

lint:
	@command -v golangci-lint >/dev/null || { echo "install golangci-lint: https://golangci-lint.run/usage/install/"; exit 1; }
	golangci-lint run

setup:
	@mkdir -p .git/hooks
	@cp scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "installed git pre-commit hook (gofmt + golangci-lint)"

clean:
	rm -f $(BINARY)
