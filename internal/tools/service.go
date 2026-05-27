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
	mapSys := "You are a precise summarizer. Summarize the following file contents concisely, preserving key facts, names, and structure. Output only the summary."
	if strings.TrimSpace(focus) != "" {
		mapSys += " Focus especially on: " + focus + "."
	}
	reduceSys := "Combine the following partial summaries into one coherent, concise summary. Remove redundancy. Output only the summary."
	return s.mapReduce(ctx, paths, mapSys, reduceSys)
}

func (s *Service) Extract(ctx context.Context, paths []string, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query must not be empty")
	}
	mapSys := fmt.Sprintf("Extract only the parts of the following file contents relevant to this query: %q. Quote relevant snippets with their file path. If nothing is relevant, say so briefly. Output only the relevant findings.", query)
	reduceSys := fmt.Sprintf("Merge the following extracted findings into a single answer to the query: %q. Keep file references and remove duplicates.", query)
	return s.mapReduce(ctx, paths, mapSys, reduceSys)
}

func (s *Service) Ask(ctx context.Context, prompt string, paths []string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt must not be empty")
	}
	if len(paths) == 0 {
		return s.llm.Complete(ctx, "You are a helpful assistant. Follow the instruction precisely and output only the result.", prompt)
	}
	mapSys := "You are a helpful assistant. Apply this instruction to the following file contents and output only the result.\n\nInstruction: " + prompt
	reduceSys := "Combine the following partial results into one coherent result for this instruction. Output only the result.\n\nInstruction: " + prompt
	return s.mapReduce(ctx, paths, mapSys, reduceSys)
}
