package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claude-llama/internal/files"
)

type fakeLLM struct {
	systems []string
	users   []string
}

func (f *fakeLLM) Complete(ctx context.Context, system, user string) (string, error) {
	f.systems = append(f.systems, system)
	f.users = append(f.users, user)
	return "reply:" + user[:min(len(user), 8)], nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newGuard(t *testing.T, root string) *files.Guard {
	t.Helper()
	g, err := files.NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestSummarizeSingleChunk(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "small content")

	llm := &fakeLLM{}
	svc := NewService(llm, newGuard(t, dir), 1000) // big budget => one chunk => one call
	_, err := svc.Summarize(context.Background(), []string{filepath.Join(dir, "a.txt")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(llm.users) != 1 {
		t.Fatalf("expected 1 llm call, got %d", len(llm.users))
	}
}

func TestSummarizeMapReduce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), strings.Repeat("x", 400))

	llm := &fakeLLM{}
	svc := NewService(llm, newGuard(t, dir), 25) // 100 chars/chunk => ~4 chunks
	_, err := svc.Summarize(context.Background(), []string{filepath.Join(dir, "a.txt")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(llm.users) < 3 {
		t.Fatalf("expected map+reduce calls (>=3), got %d", len(llm.users))
	}
	last := llm.systems[len(llm.systems)-1]
	if !strings.Contains(last, "Combine") {
		t.Errorf("reduce step should use the combine prompt, got system %q", last)
	}
}

func TestExtractEmptyQueryErrors(t *testing.T) {
	llm := &fakeLLM{}
	svc := NewService(llm, newGuard(t, t.TempDir()), 1000)
	_, err := svc.Extract(context.Background(), nil, "  ")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("expected query-related error, got %v", err)
	}
}

func TestAskNoPathsSingleCall(t *testing.T) {
	llm := &fakeLLM{}
	svc := NewService(llm, newGuard(t, t.TempDir()), 1000)
	_, err := svc.Ask(context.Background(), "draft a hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(llm.users) != 1 || llm.users[0] != "draft a hello" {
		t.Fatalf("Ask without paths should pass prompt directly: %v", llm.users)
	}
}

func TestTooLargeErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "big.txt"), strings.Repeat("y", 12000))

	llm := &fakeLLM{}
	svc := NewService(llm, newGuard(t, dir), 1) // 4 chars/chunk => >50 chunks => ceiling error
	_, err := svc.Summarize(context.Background(), []string{filepath.Join(dir, "big.txt")}, "")
	if err == nil {
		t.Fatal("expected ceiling error for oversized input")
	}
}

func TestSummarizePromptHasFormatGuidance(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "content")
	llm := &fakeLLM{}
	svc := NewService(llm, newGuard(t, dir), 1000)
	if _, err := svc.Summarize(context.Background(), []string{filepath.Join(dir, "a.txt")}, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(llm.systems[0], "only the summary") {
		t.Errorf("summarize prompt missing output-format guidance: %q", llm.systems[0])
	}
}
