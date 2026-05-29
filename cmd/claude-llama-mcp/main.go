package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"claude-llama/internal/config"
	"claude-llama/internal/files"
	"claude-llama/internal/llama"
	"claude-llama/internal/tools"
	"claude-llama/internal/usage"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run dispatches to a subcommand. The default (no args, or `serve`) runs
// the MCP server over stdio; everything else is an operator-facing CLI.
func run(args []string) error {
	ctx := context.Background()
	cmd := "serve"
	var rest []string
	if len(args) > 0 {
		cmd, rest = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return runServe(ctx)
	case "init":
		return runInit(ctx, rest)
	case "doctor":
		return runDoctor(ctx, rest)
	case "stats":
		return runStats(ctx, rest)
	case "version", "--version", "-v":
		fmt.Println("claude-llama-mcp", version)
		return nil
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		printHelp()
		return fmt.Errorf("unknown subcommand %q", cmd)
	}
}

func printHelp() {
	fmt.Fprint(os.Stderr, `claude-llama-mcp — delegate token-heavy file work to a local llama.cpp

Usage:
  claude-llama-mcp [serve]           run the MCP server over stdio (default)
  claude-llama-mcp init   [flags]    write ~/.config/claude-llama/env
  claude-llama-mcp doctor [flags]    diagnose config + llama reachability
  claude-llama-mcp stats  [flags]    summarize token-savings from the usage log
  claude-llama-mcp version           print version and exit

Run any subcommand with --help for its flags.
`)
}

func runServe(ctx context.Context) error {
	cfg := config.Load()
	// If the user didn't explicitly set LLAMA_MAX_INPUT_TOKENS, try to detect
	// the model's actual n_ctx via /props and right-size chunks. Without this
	// the 6000-token default blows past small contexts (e.g. 2048/4096 builds);
	// with this, big-context builds get to use their full window.
	if !config.MaxInputTokensExplicit() {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		props, err := llama.NewHealthClient(cfg.APIURL, 3*time.Second).FetchProps(probeCtx)
		cancel()
		if err == nil {
			if budget := llama.ChunkBudgetFromCtx(props.NCtx); budget > 0 {
				cfg.MaxInputTokens = budget
			}
		}
	}
	client := llama.New(cfg.APIURL, cfg.Model, cfg.Timeout)
	guard, err := files.NewGuard(cfg.WorkspaceRoot)
	if err != nil {
		return err
	}
	recorder := usage.NewRecorder(cfg)
	svc := tools.NewService(client, guard, cfg.MaxInputTokens)
	server := NewServer(svc, cfg, recorder)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Print(err)
		return err
	}
	return nil
}
