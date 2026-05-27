package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"claude-llama/internal/config"
	"claude-llama/internal/files"
	"claude-llama/internal/llama"
	"claude-llama/internal/tools"
)

func main() {
	cfg := config.Load()
	client := llama.New(cfg.APIURL, cfg.Model, cfg.Timeout)
	guard, err := files.NewGuard(cfg.WorkspaceRoot)
	if err != nil {
		log.Fatal(err)
	}
	svc := tools.NewService(client, guard, cfg.MaxInputTokens)
	server := NewServer(svc)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
