BINARY=bin/claude-llama-mcp

.PHONY: build test integration clean

build:
	go build -o $(BINARY) ./cmd/claude-llama-mcp

test:
	go test ./...

integration:
	go test -tags integration ./cmd/claude-llama-mcp/ -run TestSmoke -v

clean:
	rm -f $(BINARY)
