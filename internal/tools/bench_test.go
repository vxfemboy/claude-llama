//go:build bench

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude-llama/internal/files"
	"claude-llama/internal/llama"
)

// TestSavings is the regression guard for this project's whole pitch:
// delegating to the local model should return far fewer bytes than the input.
//
// Runs with `make bench` (or `go test -tags bench`). Uses an httptest server
// that replays a canned short reply for any /v1/chat/completions call — no
// real llama required, so CI stays hermetic and deterministic.
func TestSavings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Canned response: ~200 chars, regardless of input size.
		// The whole point is "input large, output small" — the model's job
		// is compression, and the bench enforces it.
		resp := map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{
					"role":    "assistant",
					"content": "Summary: a short canned reply that stands in for whatever the local model would say. Used in CI to guard the savings claim.",
				},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	dir := t.TempDir()
	// 3 representative fixtures of escalating size.
	fixtures := map[string]int{
		"short.md":  1_000,  // ~1KB
		"medium.md": 10_000, // ~10KB
		"large.md":  50_000, // ~50KB
	}
	var paths []string
	for name, size := range fixtures {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(strings.Repeat("Lorem ipsum dolor sit amet. ", size/28+1)[:size]), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}

	guard, err := files.NewGuard(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := llama.New(srv.URL, "test-model", 10*time.Second)
	svc := NewService(client, guard, 4000)

	t.Run("summarize", func(t *testing.T) { assertSavings(t, svc, "summarize", paths) })
	t.Run("extract", func(t *testing.T) { assertSavings(t, svc, "extract", paths) })
	t.Run("ask", func(t *testing.T) { assertSavings(t, svc, "ask", paths) })
}

func assertSavings(t *testing.T, svc *Service, tool string, paths []string) {
	t.Helper()
	var (
		out        string
		inputBytes int
		err        error
	)
	ctx := context.Background()
	switch tool {
	case "summarize":
		out, inputBytes, err = svc.Summarize(ctx, paths, "")
	case "extract":
		out, inputBytes, err = svc.Extract(ctx, paths, "what is the gist")
	case "ask":
		out, inputBytes, err = svc.Ask(ctx, "give the gist", paths)
	}
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	outputBytes := len(out)
	if inputBytes == 0 {
		t.Fatalf("%s: no input bytes accounted for", tool)
	}
	ratio := 1.0 - float64(outputBytes)/float64(inputBytes)
	if ratio < 0.80 {
		t.Errorf("%s: only %.0f%% savings (in=%d out=%d); expected >= 80%%",
			tool, ratio*100, inputBytes, outputBytes)
	}
	t.Logf("%s: in=%d out=%d savings=%.1f%%", tool, inputBytes, outputBytes, ratio*100)
}
