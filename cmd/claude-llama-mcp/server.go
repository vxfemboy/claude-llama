package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"claude-llama/internal/config"
	"claude-llama/internal/llama"
	"claude-llama/internal/tools"
	"claude-llama/internal/usage"
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

type HealthInput struct{}

// NewServer builds an MCP server exposing the delegation tools plus a health probe.
// cfg/rec are wired in so each tool call appends a usage row and a savings footer.
func NewServer(svc *tools.Service, cfg config.Config, rec *usage.Recorder) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "claude-llama", Version: version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_summarize",
		Description: "Summarize files, directories, or globs using a local model WITHOUT reading them into your own context — use this instead of reading large files when you only need an overview. The server reads the files locally and returns only a concise summary.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SummarizeInput) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		text, inputBytes, err := svc.Summarize(ctx, in.Paths, in.Focus)
		return record(rec, cfg, "llama_summarize", inputBytes, text, start, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_extract",
		Description: "Search files/directories/globs with a local model and return ONLY the snippets and answers matching a query, WITHOUT reading the files into your own context. Use instead of reading large or numerous files when you need specific information.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ExtractInput) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		text, inputBytes, err := svc.Extract(ctx, in.Paths, in.Query)
		return record(rec, cfg, "llama_extract", inputBytes, text, start, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_ask",
		Description: "Delegate a self-contained task (drafting, classification, mechanical transforms, Q&A) to a local model to save tokens. Optionally provide file paths/globs/directories as context (read locally by the server). Returns text only; it does not write files.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in AskInput) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		text, inputBytes, err := svc.Ask(ctx, in.Prompt, in.Paths)
		return record(rec, cfg, "llama_ask", inputBytes, text, start, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_health",
		Description: "Check whether the local llama.cpp server is reachable and which models are loaded. Use before delegating a large job if you want to verify availability and pick a model. Returns JSON with {ok, url, models, latency_ms, error}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ HealthInput) (*mcp.CallToolResult, any, error) {
		hc := llama.NewHealthClient(cfg.APIURL, 5*time.Second)
		res, _ := hc.Check(ctx)
		buf, _ := json.MarshalIndent(res, "", "  ")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
			IsError: !res.OK,
		}, nil, nil
	})

	return server
}

// record builds a usage Row from the call's bytes-in / text-out, appends it
// to the log, and returns the MCP result with the savings footer attached.
// Failures still record a row (ok=false) so stats can report success rate.
func record(rec *usage.Recorder, cfg config.Config, tool string, inputBytes int, text string, start time.Time, err error) (*mcp.CallToolResult, any, error) {
	inputTokens := (inputBytes + 3) / 4
	outputTokens := usage.Estimate(text)
	saved := inputTokens - outputTokens
	if saved < 0 {
		saved = 0
	}
	row := usage.Row{
		TS:           time.Now().UTC(),
		Tool:         tool,
		Model:        cfg.Model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Saved:        saved,
		DurationMs:   time.Since(start).Milliseconds(),
		OK:           err == nil,
	}
	if err != nil {
		row.Error = err.Error()
	}
	rec.Record(row)

	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf("local model delegation failed: %v", err) + rec.Footer(row),
			}},
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text + rec.Footer(row)}},
	}, nil, nil
}
