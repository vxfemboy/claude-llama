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
// partial results with reduceSystem. Returns the combined text and the total
// number of bytes of file content fed in — the latter is the input-size figure
// the caller uses for token-savings accounting.
func (s *Service) mapReduce(ctx context.Context, paths []string, mapSystem, reduceSystem string) (string, int, error) {
	docs, err := s.guard.ReadAll(paths)
	if err != nil {
		return "", 0, err
	}
	inputBytes := 0
	for _, d := range docs {
		inputBytes += len(d.Content)
	}
	chunks := files.Chunk(docs, s.maxInputTokens)
	if len(chunks) == 0 {
		return "", inputBytes, fmt.Errorf("no content found in given paths")
	}
	if len(chunks) > maxChunks {
		return "", inputBytes, fmt.Errorf("input too large: %d chunks exceeds limit of %d; narrow the paths", len(chunks), maxChunks)
	}
	if len(chunks) == 1 {
		out, err := s.llm.Complete(ctx, mapSystem, chunks[0])
		return out, inputBytes, err
	}
	var partials []string
	for i, ch := range chunks {
		part, err := s.llm.Complete(ctx, mapSystem, ch)
		if err != nil {
			return "", inputBytes, fmt.Errorf("chunk %d: %w", i+1, err)
		}
		partials = append(partials, part)
	}
	out, err := s.llm.Complete(ctx, reduceSystem, strings.Join(partials, "\n\n"))
	return out, inputBytes, err
}

// Summarize returns (text, inputBytes, err). inputBytes is what Claude would
// have had to read to do this itself — the basis for the "tokens saved" footer.
func (s *Service) Summarize(ctx context.Context, paths []string, focus string) (string, int, error) {
	return s.mapReduce(ctx, paths, summarizeMapPrompt(strings.TrimSpace(focus)), summarizeReducePrompt())
}

func (s *Service) Extract(ctx context.Context, paths []string, query string) (string, int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", 0, fmt.Errorf("query must not be empty")
	}
	return s.mapReduce(ctx, paths, extractMapPrompt(query), extractReducePrompt(query))
}

func (s *Service) Ask(ctx context.Context, prompt string, paths []string) (string, int, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", 0, fmt.Errorf("prompt must not be empty")
	}
	if len(paths) == 0 {
		out, err := s.llm.Complete(ctx, askNoContextPrompt(), prompt)
		return out, len(prompt), err
	}
	return s.mapReduce(ctx, paths, askMapPrompt(prompt), askReducePrompt(prompt))
}
