//go:build integration

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"claude-llama/internal/config"
	"claude-llama/internal/files"
	"claude-llama/internal/llama"
	"claude-llama/internal/tools"
)

// TestRealSavings runs a matrix of real calls against the live llama and
// prints a benchmark table. Used to populate the README's "Real-world
// savings" section with honest numbers — not synthetic httptest replies.
func TestRealSavings(t *testing.T) {
	cfg := config.Load()
	if !strings.HasPrefix(cfg.APIURL, "http://") {
		t.Skip("no LLAMA_API_URL configured")
	}
	guard, err := files.NewGuard(cfg.WorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	svc := tools.NewService(llama.New(cfg.APIURL, cfg.Model, cfg.Timeout), guard, cfg.MaxInputTokens)

	type row struct {
		Label    string
		Tool     string
		File     string
		FileSize int64
		InTokens int
		OutTok   int
		Saved    int
		PctSaved float64
		Duration time.Duration
		OK       bool
		Err      string
	}

	runOne := func(label, tool, relPath, prompt string) row {
		r := row{Label: label, Tool: tool, File: relPath}
		full := cfg.WorkspaceRoot + "/" + relPath
		st, err := os.Stat(full)
		if err != nil {
			r.Err = err.Error()
			return r
		}
		r.FileSize = st.Size()

		ctx := context.Background()
		start := time.Now()
		var (
			text       string
			inputBytes int
		)
		switch tool {
		case "summarize":
			text, inputBytes, err = svc.Summarize(ctx, []string{full}, prompt)
		case "extract":
			text, inputBytes, err = svc.Extract(ctx, []string{full}, prompt)
		case "ask":
			if relPath == "" {
				text, inputBytes, err = svc.Ask(ctx, prompt, nil)
			} else {
				text, inputBytes, err = svc.Ask(ctx, prompt, []string{full})
			}
		}
		r.Duration = time.Since(start)
		if err != nil {
			r.Err = err.Error()
			return r
		}
		r.InTokens = (inputBytes + 3) / 4
		r.OutTok = (len(text) + 3) / 4
		r.Saved = r.InTokens - r.OutTok
		if r.Saved < 0 {
			r.Saved = 0
		}
		if r.InTokens > 0 {
			r.PctSaved = 100 * float64(r.Saved) / float64(r.InTokens)
		}
		r.OK = true
		return r
	}

	cases := []struct {
		label, tool, file, prompt string
	}{
		{"3KB Go source", "summarize", "internal/tools/service.go", "what does this file do"},
		{"15KB design spec", "summarize", "docs/superpowers/specs/2026-05-26-claude-llama-improvements-design.md", "what is the proposed design"},
		{"32KB plan", "summarize", "docs/superpowers/plans/2026-05-26-claude-llama-token-delegation.md", "what are the implementation steps"},
		{"15KB design spec", "extract", "docs/superpowers/specs/2026-05-26-claude-llama-improvements-design.md", "what environment variables are mentioned"},
		{"ask (no context)", "ask", "", "In one sentence, what is the Model Context Protocol?"},
	}

	results := make([]row, 0, len(cases))
	for _, c := range cases {
		r := runOne(c.label, c.tool, c.file, c.prompt)
		results = append(results, r)
		if r.OK {
			t.Logf("%-20s %-10s in=%d out=%d saved=%d (%.1f%%) %s",
				r.Label, r.Tool, r.InTokens, r.OutTok, r.Saved, r.PctSaved, r.Duration.Round(time.Second))
		} else {
			t.Logf("%-20s %-10s FAILED: %s", r.Label, r.Tool, r.Err)
		}
	}

	// Print a copy-pasteable markdown table for the README.
	fmt.Println("\n=== bench table for README ===")
	fmt.Println("| Fixture | Tool | Input tok | Returned tok | Saved | % | Duration |")
	fmt.Println("|---|---|---:|---:|---:|---:|---:|")
	for _, r := range results {
		if !r.OK {
			fmt.Printf("| %s | `llama_%s` | — | — | — | (failed) | — |\n", r.Label, r.Tool)
			continue
		}
		fmt.Printf("| %s | `llama_%s` | %d | %d | %d | %.0f%% | %s |\n",
			r.Label, r.Tool, r.InTokens, r.OutTok, r.Saved, r.PctSaved, r.Duration.Round(time.Second))
	}
	fmt.Println("=== end bench table ===")
}
