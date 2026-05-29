//go:build integration

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"claude-llama/internal/config"
	"claude-llama/internal/files"
	"claude-llama/internal/llama"
	"claude-llama/internal/tools"
	"claude-llama/internal/usage"
)

// Run with: go test -tags integration ./cmd/claude-llama-mcp/ -run TestSmoke -v
// Requires a reachable llama.cpp endpoint (LLAMA_API_URL).
func TestSmoke(t *testing.T) {
	cfg := config.Load()
	guard, err := files.NewGuard(cfg.WorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	svc := tools.NewService(llama.New(cfg.APIURL, cfg.Model, cfg.Timeout), guard, cfg.MaxInputTokens)
	server := NewServer(svc, cfg, usage.NewRecorder(cfg))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientT, serverT := mcp.NewInMemoryTransports()

	go func() { _ = server.Run(ctx, serverT) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "smoke", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "llama_ask",
		Arguments: map[string]any{"prompt": "Reply with exactly one word: pong"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %v", res.Content)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(strings.ToLower(text), "pong") {
		t.Errorf("expected reply to contain 'pong', got %q", text)
	}
}
