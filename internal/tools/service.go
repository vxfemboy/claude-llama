package tools

import (
	"context"
	"fmt"
	"strings"

	"claude-llama/internal/files"
)

// Completer is the subset of the llama client the service depends on.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

const maxChunks = 50

type Service struct {
	llm            Completer
	guard          *files.Guard
	maxInputTokens int
}

func NewService(llm Completer, guard *files.Guard, maxInputTokens int) *Service {
	return &Service{llm: llm, guard: guard, maxInputTokens: maxInputTokens}
}

// mapReduce reads paths, chunks them, applies mapSystem per chunk, then combines
// partial results with reduceSystem. With a single chunk it calls the model once.
func (s *Service) mapReduce(ctx context.Context, paths []string, mapSystem, reduceSystem string) (string, error) {
	docs, err := s.guard.ReadAll(paths)
	if err != nil {
		return "", err
	}
	chunks := files.Chunk(docs, s.maxInputTokens)
	if len(chunks) == 0 {
		return "", fmt.Errorf("no content found in given paths")
	}
	if len(chunks) > maxChunks {
		return "", fmt.Errorf("input too large: %d chunks exceeds limit of %d; narrow the paths", len(chunks), maxChunks)
	}
	if len(chunks) == 1 {
		return s.llm.Complete(ctx, mapSystem, chunks[0])
	}
	var partials []string
	for i, ch := range chunks {
		part, err := s.llm.Complete(ctx, mapSystem, ch)
		if err != nil {
			return "", fmt.Errorf("chunk %d: %w", i+1, err)
		}
		partials = append(partials, part)
	}
	return s.llm.Complete(ctx, reduceSystem, strings.Join(partials, "\n\n"))
}

func (s *Service) Summarize(ctx context.Context, paths []string, focus string) (string, error) {
	return s.mapReduce(ctx, paths, summarizeMapPrompt(strings.TrimSpace(focus)), summarizeReducePrompt())
}

func (s *Service) Extract(ctx context.Context, paths []string, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query must not be empty")
	}
	return s.mapReduce(ctx, paths, extractMapPrompt(query), extractReducePrompt(query))
}

func (s *Service) Ask(ctx context.Context, prompt string, paths []string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt must not be empty")
	}
	if len(paths) == 0 {
		return s.llm.Complete(ctx, askNoContextPrompt(), prompt)
	}
	return s.mapReduce(ctx, paths, askMapPrompt(prompt), askReducePrompt(prompt))
}
