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

// TestRealSavings runs a real summarize against the live llama configured in
// the user's env, on a real fixture in this repo. Prints the savings footer
// fields so we can eyeball "did delegation actually save tokens" — the only
// honest answer to that question.
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

	// Pick a real, decently-sized file in this repo. WorkspaceRoot is the
	// guard's anchor and points at the repo root when tests are run with
	// `make integration`, so we resolve relative to it.
	target := cfg.WorkspaceRoot + "/docs/superpowers/plans/2026-05-26-claude-llama-token-delegation.md"
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("fixture %s missing: %v", target, err)
	}

	ctx := context.Background()
	start := time.Now()
	text, inputBytes, err := svc.Summarize(ctx, []string{target}, "what does this file do")
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}

	out := len(text)
	inputTokens := (inputBytes + 3) / 4
	outputTokens := (out + 3) / 4
	saved := inputTokens - outputTokens
	pct := 100 * float64(saved) / float64(inputTokens)
	fmt.Printf(`
--- real-llama savings ---
file:           %s (%d bytes on disk)
input bytes:    %d  (~%d tokens)
output bytes:   %d  (~%d tokens)
saved tokens:   %d  (%.1f%%)
duration:       %s
---
%s
---
`, target, st.Size(), inputBytes, inputTokens, out, outputTokens, saved, pct, dur, text)

	if saved <= 0 {
		t.Errorf("delegation did NOT save tokens: input=%d output=%d", inputTokens, outputTokens)
	}
}
