package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"claude-llama/internal/tools"
)

type SummarizeInput struct {
	Paths []string `json:"paths" jsonschema:"file paths, globs, or directories to read and summarize"`
	Focus string   `json:"focus,omitempty" jsonschema:"optional aspect to emphasize in the summary"`
}

type ExtractInput struct {
	Paths []string `json:"paths" jsonschema:"file paths, globs, or directories to search"`
	Query string   `json:"query" jsonschema:"what to extract from the files"`
}

type AskInput struct {
	Prompt string   `json:"prompt" jsonschema:"the instruction or question for the local model"`
	Paths  []string `json:"paths,omitempty" jsonschema:"optional file paths, globs, or directories to provide as context"`
}

// NewServer builds an MCP server exposing the three delegation tools.
func NewServer(svc *tools.Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "claude-llama", Version: "1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_summarize",
		Description: "Summarize files, directories, or globs using a local model WITHOUT reading them into your own context — use this instead of reading large files when you only need an overview. The server reads the files locally and returns only a concise summary.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SummarizeInput) (*mcp.CallToolResult, any, error) {
		return result(svc.Summarize(ctx, in.Paths, in.Focus))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_extract",
		Description: "Search files/directories/globs with a local model and return ONLY the snippets and answers matching a query, WITHOUT reading the files into your own context. Use instead of reading large or numerous files when you need specific information.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ExtractInput) (*mcp.CallToolResult, any, error) {
		return result(svc.Extract(ctx, in.Paths, in.Query))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_ask",
		Description: "Delegate a self-contained task (drafting, classification, mechanical transforms, Q&A) to a local model to save tokens. Optionally provide file paths/globs/directories as context (read locally by the server). Returns text only; it does not write files.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in AskInput) (*mcp.CallToolResult, any, error) {
		return result(svc.Ask(ctx, in.Prompt, in.Paths))
	})

	return server
}

// result adapts a (text, error) pair into an MCP tool result. Failures become tool
// errors (IsError) with a readable message so Claude can fall back to doing the work itself.
func result(text string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("local model delegation failed: %v", err)}},
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}
