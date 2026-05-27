# claude-llama Token-Delegation Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Claude Code plugin whose Go MCP server lets Claude delegate bulk reads, search/extract, and rote tasks to a local llama.cpp model so the content never enters Claude's context.

**Architecture:** A single Go binary runs as an MCP server over stdio (official `modelcontextprotocol/go-sdk`). It exposes three tools (`llama_summarize`, `llama_extract`, `llama_ask`) that take file paths, read the files server-side, and call llama.cpp's OpenAI-compatible `/v1/chat/completions` endpoint. Inputs larger than a token budget are handled with map-reduce.

**Tech Stack:** Go 1.25, `github.com/modelcontextprotocol/go-sdk`, llama.cpp OpenAI-compatible API, Claude Code plugin (`.mcp.json` + `${CLAUDE_PLUGIN_ROOT}`).

**Spec:** `docs/superpowers/specs/2026-05-26-claude-llama-token-delegation-design.md`

---

## File Structure

```
claude-llama/
  .claude-plugin/plugin.json        (exists)
  .mcp.json                         (Task 7 — registers the MCP server)
  go.mod                            (Task 1)
  Makefile                          (Task 7)
  cmd/claude-llama-mcp/
    main.go                         (Task 6 — config load + server wiring)
    server.go                       (Task 6 — NewServer, tool registration)
    integration_test.go             (Task 6 — guarded end-to-end smoke)
  internal/config/config.go         (Task 1 — env config)
  internal/config/config_test.go
  internal/files/read.go            (Tasks 2-3 — expand, read, chunk)
  internal/files/read_test.go
  internal/llama/client.go          (Task 4 — HTTP client to llama.cpp)
  internal/llama/client_test.go
  internal/tools/service.go         (Task 5 — map-reduce orchestration)
  internal/tools/service_test.go
  bin/claude-llama-mcp              (built artifact)
```

Module path is `claude-llama` (a local module path; not a real URL). Internal imports are `claude-llama/internal/...`.

> Note: this adds `internal/config` beyond the spec's structure for testability — a minor refinement, same responsibilities.

---

## Task 1: Go module + config package

**Files:**
- Create: `go.mod`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Initialize the module and add the SDK dependency**

Run:
```bash
cd /home/zoa/projects/claude-llama
go mod init claude-llama
go get github.com/modelcontextprotocol/go-sdk@latest
```
Expected: `go.mod` created with `module claude-llama`, `go 1.25`, and a `require github.com/modelcontextprotocol/go-sdk vX.Y.Z` line; `go.sum` populated.

- [ ] **Step 2: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LLAMA_API_URL", "")
	t.Setenv("LLAMA_MODEL", "")
	t.Setenv("LLAMA_MAX_INPUT_TOKENS", "")
	t.Setenv("LLAMA_TIMEOUT_SECONDS", "")

	cfg := Load()

	if cfg.APIURL != "http://hack-mini:8080" {
		t.Errorf("APIURL = %q, want default", cfg.APIURL)
	}
	if cfg.Model != "unsloth/Qwen3.5-9B-GGUF:Q4_K_M" {
		t.Errorf("Model = %q, want default", cfg.Model)
	}
	if cfg.MaxInputTokens != 6000 {
		t.Errorf("MaxInputTokens = %d, want 6000", cfg.MaxInputTokens)
	}
	if cfg.Timeout != 120*time.Second {
		t.Errorf("Timeout = %v, want 120s", cfg.Timeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("LLAMA_API_URL", "http://localhost:9999")
	t.Setenv("LLAMA_MODEL", "other-model")
	t.Setenv("LLAMA_MAX_INPUT_TOKENS", "1000")
	t.Setenv("LLAMA_TIMEOUT_SECONDS", "30")

	cfg := Load()

	if cfg.APIURL != "http://localhost:9999" {
		t.Errorf("APIURL = %q", cfg.APIURL)
	}
	if cfg.Model != "other-model" {
		t.Errorf("Model = %q", cfg.Model)
	}
	if cfg.MaxInputTokens != 1000 {
		t.Errorf("MaxInputTokens = %d", cfg.MaxInputTokens)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad -v`
Expected: FAIL — `undefined: Load` / package does not compile.

- [ ] **Step 4: Write minimal implementation**

Create `internal/config/config.go`:
```go
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	APIURL         string
	Model          string
	MaxInputTokens int
	Timeout        time.Duration
}

const (
	defaultAPIURL         = "http://hack-mini:8080"
	defaultModel          = "unsloth/Qwen3.5-9B-GGUF:Q4_K_M"
	defaultMaxInputTokens = 6000
	defaultTimeoutSeconds = 120
)

func Load() Config {
	return Config{
		APIURL:         getEnv("LLAMA_API_URL", defaultAPIURL),
		Model:          getEnv("LLAMA_MODEL", defaultModel),
		MaxInputTokens: getEnvInt("LLAMA_MAX_INPUT_TOKENS", defaultMaxInputTokens),
		Timeout:        time.Duration(getEnvInt("LLAMA_TIMEOUT_SECONDS", defaultTimeoutSeconds)) * time.Second,
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config/
git commit -m "feat: add go module and config package"
```

---

## Task 2: File expansion and reading

**Files:**
- Create: `internal/files/read.go`
- Create: `internal/files/read_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/files/read_test.go`:
```go
package files

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExpandFileGlobAndDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "A")
	writeFile(t, filepath.Join(dir, "b.txt"), "B")
	writeFile(t, filepath.Join(dir, "sub", "c.txt"), "C")

	// directory expands recursively
	got, err := Expand([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("dir expand got %d files, want 3: %v", len(got), got)
	}

	// glob expands
	got, err = Expand([]string{filepath.Join(dir, "*.txt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("glob expand got %d files, want 2: %v", len(got), got)
	}
}

func TestExpandMissingPathErrors(t *testing.T) {
	_, err := Expand([]string{filepath.Join(t.TempDir(), "nope.txt")})
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

func TestReadAll(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")

	docs, err := ReadAll([]string{filepath.Join(dir, "a.txt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Content != "hello" {
		t.Fatalf("ReadAll = %+v", docs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/files/ -v`
Expected: FAIL — `undefined: Expand`, `undefined: ReadAll`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/files/read.go`:
```go
package files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type Document struct {
	Path    string
	Content string
}

// Expand turns paths (files, globs, or directories) into a sorted, de-duplicated
// list of regular file paths. Directories are walked recursively. A literal path
// that does not exist returns an error.
func Expand(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range paths {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, fmt.Errorf("bad glob %q: %w", p, err)
		}
		if matches == nil {
			matches = []string{p} // no glob match: treat as literal path
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				return nil, fmt.Errorf("cannot access %q: %w", m, err)
			}
			if info.IsDir() {
				err := filepath.WalkDir(m, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !d.IsDir() {
						add(path)
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			} else {
				add(m)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// ReadAll expands the given paths and reads each resulting file into a Document.
func ReadAll(paths []string) ([]Document, error) {
	fileList, err := Expand(paths)
	if err != nil {
		return nil, err
	}
	var docs []Document
	for _, f := range fileList {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", f, err)
		}
		docs = append(docs, Document{Path: f, Content: string(b)})
	}
	return docs, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/files/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/files/
git commit -m "feat: add file path expansion and reading"
```

---

## Task 3: Token estimation and chunking

**Files:**
- Modify: `internal/files/read.go`
- Modify: `internal/files/read_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/files/read_test.go`:
```go
func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("12345678"); got != 2 {
		t.Errorf("EstimateTokens(8 chars) = %d, want 2", got)
	}
	if got := EstimateTokens(""); got != 0 {
		t.Errorf("EstimateTokens(\"\") = %d, want 0", got)
	}
}

func TestChunkPacksSmallDocs(t *testing.T) {
	docs := []Document{
		{Path: "a", Content: "aaaa"},
		{Path: "b", Content: "bbbb"},
	}
	// generous budget: everything fits in one chunk
	chunks := Chunk(docs, 1000)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if !contains(chunks[0], "// file: a") || !contains(chunks[0], "// file: b") {
		t.Errorf("chunk missing file headers: %q", chunks[0])
	}
}

func TestChunkSplitsOversizedDoc(t *testing.T) {
	big := make([]byte, 0, 400)
	for i := 0; i < 400; i++ {
		big = append(big, 'x')
	}
	docs := []Document{{Path: "big", Content: string(big)}}
	// budget of 25 tokens => 100 chars per chunk => multiple chunks
	chunks := Chunk(docs, 25)
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want >= 2", len(chunks))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/files/ -run 'TestEstimateTokens|TestChunk' -v`
Expected: FAIL — `undefined: EstimateTokens`, `undefined: Chunk`.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/files/read.go` (add `"strings"` to the import block):
```go
// EstimateTokens approximates token count as len(s)/4 (rounded up).
func EstimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// Chunk packs documents into chunks whose estimated token count stays under
// maxTokens. Each document is prefixed with a "// file: <path>" header.
// A single document larger than the budget is split across multiple chunks.
func Chunk(docs []Document, maxTokens int) []string {
	maxChars := maxTokens * 4
	if maxChars <= 0 {
		maxChars = 1
	}
	var chunks []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			chunks = append(chunks, b.String())
			b.Reset()
		}
	}
	for _, d := range docs {
		body := fmt.Sprintf("// file: %s\n%s\n", d.Path, d.Content)
		if len(body) <= maxChars {
			if b.Len()+len(body) > maxChars {
				flush()
			}
			b.WriteString(body)
			continue
		}
		flush()
		for start := 0; start < len(body); start += maxChars {
			end := start + maxChars
			if end > len(body) {
				end = len(body)
			}
			chunks = append(chunks, body[start:end])
		}
	}
	flush()
	return chunks
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/files/ -v`
Expected: PASS (all files tests).

- [ ] **Step 5: Commit**

```bash
git add internal/files/
git commit -m "feat: add token estimation and chunking"
```

---

## Task 4: llama.cpp HTTP client

**Files:**
- Create: `internal/llama/client.go`
- Create: `internal/llama/client_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/llama/client_test.go`:
```go
package llama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompleteSuccess(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"hi there"}}]}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model", 5*time.Second)
	out, err := c.Complete(context.Background(), "be brief", "say hi")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hi there" {
		t.Errorf("Complete = %q, want %q", out, "hi there")
	}
	if gotBody["model"] != "test-model" {
		t.Errorf("request model = %v", gotBody["model"])
	}
}

func TestCompleteHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model", 5*time.Second)
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llama/ -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/llama/client.go`:
```go
package llama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	model   string
	http    *http.Client
}

func New(baseURL, model string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{Timeout: timeout},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete sends a system+user prompt to the chat completions endpoint and
// returns the assistant's reply text.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llama request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama returned %d: %s", resp.StatusCode, string(body))
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("decode llama response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llama returned no choices")
	}
	return cr.Choices[0].Message.Content, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/llama/ -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/llama/
git commit -m "feat: add llama.cpp chat completions client"
```

---

## Task 5: Tool service (map-reduce orchestration)

**Files:**
- Create: `internal/tools/service.go`
- Create: `internal/tools/service_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tools/service_test.go`:
```go
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSummarizeSingleChunk(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "small content")

	llm := &fakeLLM{}
	svc := NewService(llm, 1000) // big budget => one chunk => one call
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
	// content larger than the budget forces multiple map calls + 1 reduce call
	writeFile(t, filepath.Join(dir, "a.txt"), strings.Repeat("x", 400))

	llm := &fakeLLM{}
	svc := NewService(llm, 25) // 100 chars/chunk => ~4 chunks
	_, err := svc.Summarize(context.Background(), []string{filepath.Join(dir, "a.txt")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(llm.users) < 3 {
		t.Fatalf("expected map+reduce calls (>=3), got %d", len(llm.users))
	}
}

func TestExtractEmptyQueryErrors(t *testing.T) {
	llm := &fakeLLM{}
	svc := NewService(llm, 1000)
	_, err := svc.Extract(context.Background(), []string{"whatever"}, "  ")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestAskNoPathsSingleCall(t *testing.T) {
	llm := &fakeLLM{}
	svc := NewService(llm, 1000)
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
	svc := NewService(llm, 1) // 4 chars/chunk => >50 chunks => ceiling error
	_, err := svc.Summarize(context.Background(), []string{filepath.Join(dir, "big.txt")}, "")
	if err == nil {
		t.Fatal("expected ceiling error for oversized input")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -v`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/tools/service.go`:
```go
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
	maxInputTokens int
}

func NewService(llm Completer, maxInputTokens int) *Service {
	return &Service{llm: llm, maxInputTokens: maxInputTokens}
}

// mapReduce reads paths, chunks them, applies mapSystem per chunk, then combines
// partial results with reduceSystem. With a single chunk it calls the model once.
func (s *Service) mapReduce(ctx context.Context, paths []string, mapSystem, reduceSystem string) (string, error) {
	docs, err := files.ReadAll(paths)
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tools/ -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/tools/
git commit -m "feat: add tool service with map-reduce orchestration"
```

---

## Task 6: MCP server wiring + main

**Files:**
- Create: `cmd/claude-llama-mcp/server.go`
- Create: `cmd/claude-llama-mcp/main.go`
- Create: `cmd/claude-llama-mcp/integration_test.go`

- [ ] **Step 1: Write the server constructor and tool registration**

Create `cmd/claude-llama-mcp/server.go`:
```go
package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"claude-llama/internal/tools"
)

type Output struct {
	Result string `json:"result" jsonschema:"the result produced by the local model"`
}

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
		Description: "Summarize files using a local model WITHOUT reading them into your own context. Use this instead of reading large files when you only need a summary. Provide file paths, globs, or directories; the server reads and summarizes them locally and returns only the summary.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SummarizeInput) (*mcp.CallToolResult, Output, error) {
		return result(svc.Summarize(ctx, in.Paths, in.Focus))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_extract",
		Description: "Search files with a local model and return only the snippets/answers matching a query, WITHOUT reading the files into your own context. Use instead of reading large or numerous files when you only need specific information.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ExtractInput) (*mcp.CallToolResult, Output, error) {
		return result(svc.Extract(ctx, in.Paths, in.Query))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "llama_ask",
		Description: "Delegate a self-contained task (drafting, classification, mechanical transforms, Q&A) to a local model to save tokens. Optionally provide file paths/globs/directories as context, read locally by the server. Returns text only; it does not write files.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in AskInput) (*mcp.CallToolResult, Output, error) {
		return result(svc.Ask(ctx, in.Prompt, in.Paths))
	})

	return server
}

// result adapts a (text, error) pair into an MCP tool result. Failures become tool
// errors (IsError) with a readable message so Claude can fall back to doing the work itself.
func result(text string, err error) (*mcp.CallToolResult, Output, error) {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("local model delegation failed: %v", err)}},
		}, Output{}, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, Output{Result: text}, nil
}
```

- [ ] **Step 2: Write main**

Create `cmd/claude-llama-mcp/main.go`:
```go
package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"claude-llama/internal/config"
	"claude-llama/internal/llama"
	"claude-llama/internal/tools"
)

func main() {
	cfg := config.Load()
	client := llama.New(cfg.APIURL, cfg.Model, cfg.Timeout)
	svc := tools.NewService(client, cfg.MaxInputTokens)
	server := NewServer(svc)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./cmd/claude-llama-mcp`
Expected: builds with no errors. (Remove the stray `claude-llama-mcp` binary it drops in cwd: `rm -f claude-llama-mcp`.)

- [ ] **Step 4: Write the guarded end-to-end integration test**

Create `cmd/claude-llama-mcp/integration_test.go`:
```go
//go:build integration

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"claude-llama/internal/config"
	"claude-llama/internal/llama"
	"claude-llama/internal/tools"
)

// Run with: go test -tags integration ./cmd/claude-llama-mcp/ -run TestSmoke -v
// Requires a reachable llama.cpp endpoint (LLAMA_API_URL).
func TestSmoke(t *testing.T) {
	cfg := config.Load()
	svc := tools.NewService(llama.New(cfg.APIURL, cfg.Model, cfg.Timeout), cfg.MaxInputTokens)
	server := NewServer(svc)

	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()

	go func() { _ = server.Run(ctx, serverT) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "smoke", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "llama_ask",
		Arguments: map[string]any{"prompt": "Reply with exactly one word: pong"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("tool error: %v", res.Content)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(strings.ToLower(text), "pong") {
		t.Errorf("expected reply to contain 'pong', got %q", text)
	}
}
```

- [ ] **Step 5: Run the integration smoke test against the real endpoint**

Run: `go test -tags integration ./cmd/claude-llama-mcp/ -run TestSmoke -v`
Expected: PASS — reply contains "pong". (If the endpoint is unreachable this will fail; confirm `curl -s -m 5 http://hack-mini:8080/v1/models` first.)

- [ ] **Step 6: Run the full unit suite**

Run: `go test ./...`
Expected: PASS for config, files, llama, tools (the integration test is skipped without the `integration` tag).

- [ ] **Step 7: Commit**

```bash
git add cmd/claude-llama-mcp/
git commit -m "feat: wire MCP server with three delegation tools"
```

---

## Task 7: Plugin registration, Makefile, build

**Files:**
- Create: `Makefile`
- Create: `.mcp.json`
- Create: `.gitignore`

- [ ] **Step 1: Create the Makefile**

Create `Makefile`:
```makefile
BINARY=bin/claude-llama-mcp

.PHONY: build test integration clean

build:
	go build -o $(BINARY) ./cmd/claude-llama-mcp

test:
	go test ./...

integration:
	go test -tags integration ./cmd/claude-llama-mcp/ -run TestSmoke -v

clean:
	rm -f $(BINARY)
```

- [ ] **Step 2: Build the binary into bin/**

Run: `make build`
Expected: `bin/claude-llama-mcp` exists. Verify: `ls -la bin/claude-llama-mcp`.

- [ ] **Step 3: Create the plugin MCP registration**

Create `.mcp.json`:
```json
{
  "mcpServers": {
    "claude-llama": {
      "command": "${CLAUDE_PLUGIN_ROOT}/bin/claude-llama-mcp",
      "args": [],
      "env": {
        "LLAMA_API_URL": "http://hack-mini:8080",
        "LLAMA_MODEL": "unsloth/Qwen3.5-9B-GGUF:Q4_K_M"
      }
    }
  }
}
```

- [ ] **Step 4: Create .gitignore**

Create `.gitignore`:
```
/bin/
```

- [ ] **Step 5: Sanity-check the built binary speaks MCP over stdio**

Run:
```bash
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"1.0.0"}}}' | ./bin/claude-llama-mcp
```
Expected: a single JSON-RPC response line containing `"serverInfo"` with `"name":"claude-llama"`. (The process then waits for more input; Ctrl-C to exit.)

- [ ] **Step 6: Commit**

```bash
git add Makefile .mcp.json .gitignore
git commit -m "feat: add plugin MCP registration and build tooling"
```

- [ ] **Step 7: Manual verification inside Claude Code**

Reload Claude Code so the plugin's `.mcp.json` is picked up, then confirm the three tools (`llama_summarize`, `llama_extract`, `llama_ask`) appear and that calling `llama_summarize` on a real file returns a summary. This step is a human check — document the result.

---

## Self-Review Notes

- **Spec coverage:** Architecture (Tasks 1,4,6), three tools (Task 6 + Task 5 logic), path-based server-side reading (Task 2), 8192-token map-reduce + ceiling (Tasks 3,5), no disk writes (Task 6 returns text only), env config with defaults (Task 1), project structure (all tasks), error handling → tool errors (Task 6 `result`), plugin registration (Task 7). Future phases intentionally not implemented.
- **Type consistency:** `Completer.Complete(ctx, system, user)` is defined in Task 5 and implemented by `llama.Client.Complete` in Task 4 with the identical signature. `tools.NewService(Completer, int)` used consistently in Tasks 5 and 6. `files.ReadAll`/`Chunk`/`EstimateTokens` defined in Tasks 2-3, consumed in Task 5.
- **No placeholders:** every code step contains complete code; every run step has an exact command and expected output.
