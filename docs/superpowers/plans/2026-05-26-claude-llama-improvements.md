# claude-llama Improvements (Trimmed) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the claude-llama MCP plugin with a workspace-root path guard (security), binary-file skipping, clearer llama failure errors, and tuned prompts/tool-descriptions — without the over-scoped caching/observability/structured-output machinery.

**Architecture:** Reuse the existing packages (`config`, `files`, `llama`, `tools`, `cmd/claude-llama-mcp`). Add a `files.Guard` that confines all reads to a workspace root (default cwd, override `LLAMA_WORKSPACE_ROOT`) and blocks sensitive paths; thread it through the `tools.Service`. Improve `llama.Client` error classification. Centralize prompts.

**Tech Stack:** Go 1.25, existing deps (no new third-party libraries).

**Source spec:** `docs/superpowers/specs/2026-05-26-claude-llama-improvements-design.md`

## Scope decisions (from the user)

**In scope (this plan):**
- Workspace-root path guard rooted at **cwd**, overridable via `LLAMA_WORKSPACE_ROOT` (NOT `CLAUDE_PLUGIN_ROOT` — that would block the user's project files). Symlink-escape rejection. Deny-list of sensitive path segments (`.git`, `.env`, `secrets`).
- Binary-file detection: skip binary files with a stderr warning.
- Failure handling: distinguish llama timeout vs. connection failure vs. HTTP error, with actionable messages including the server URL.
- Prompt tuning + improved MCP tool descriptions (with output-format guidance), centralized in one file.

**Explicitly DROPPED (YAGNI for a local single-user v1), documented as deferred:**
- File-hash cache, LRU/TTL result cache.
- Request-ID tracing, latency/token/chunk metric logging.
- Structured JSON output mode.
- Regex secret-content redaction (the guard's deny-list covers the main secret-file vector; content redaction is error-prone — deferred).
- Separate soft/hard chunk limits (the existing `maxChunks = 50` ceiling stays as the single guard).

---

## File Structure

```
internal/config/config.go        (modify: add WorkspaceRoot)
internal/config/config_test.go   (modify: assert WorkspaceRoot default+override)
internal/files/guard.go          (create: Guard, NewGuard, ReadAll w/ checks + binary skip)
internal/files/guard_test.go     (create: escape/deny/symlink/binary tests + migrated read test)
internal/files/read.go           (modify in Task I2: remove now-unused free ReadAll)
internal/files/read_test.go      (modify in Task I2: remove TestReadAll, migrated to guard_test)
internal/tools/service.go        (modify: hold *files.Guard; use prompts.go)
internal/tools/prompts.go        (create: system-prompt builders w/ format guidance)
internal/tools/service_test.go   (modify: construct guard in tests)
internal/llama/client.go         (modify: classify timeout/connection/HTTP errors)
internal/llama/client_test.go    (modify: add timeout + connection-failure tests)
cmd/claude-llama-mcp/main.go     (modify: build guard, pass to NewService)
cmd/claude-llama-mcp/server.go   (modify: improved tool descriptions)
```

---

## Task I1: Workspace config + files.Guard (with binary skip)

This task adds the guard alongside the existing free `files.ReadAll` (which stays for now so the build remains green). Task I2 switches consumers over and removes the free function.

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`
- Create: `internal/files/guard.go`, `internal/files/guard_test.go`

- [ ] **Step 1: Add WorkspaceRoot to config (write failing test first)**

Append to `internal/config/config_test.go`:
```go
func TestLoadWorkspaceRoot(t *testing.T) {
	t.Setenv("LLAMA_WORKSPACE_ROOT", "/tmp/some-root")
	if got := Load().WorkspaceRoot; got != "/tmp/some-root" {
		t.Errorf("WorkspaceRoot = %q, want /tmp/some-root", got)
	}

	t.Setenv("LLAMA_WORKSPACE_ROOT", "")
	wd, _ := os.Getwd()
	if got := Load().WorkspaceRoot; got != wd {
		t.Errorf("WorkspaceRoot default = %q, want cwd %q", got, wd)
	}
}
```
Add `"os"` to the test file imports if not present.

- [ ] **Step 2: Run it, expect FAIL**

Run: `go test ./internal/config/ -run TestLoadWorkspaceRoot -v`
Expected: FAIL — `cfg.WorkspaceRoot` undefined.

- [ ] **Step 3: Implement config change**

In `internal/config/config.go`, add the field to `Config`:
```go
	WorkspaceRoot  string
```
Add `"os"` to imports. Add to the `Load()` return literal:
```go
		WorkspaceRoot:  getEnv("LLAMA_WORKSPACE_ROOT", defaultWorkspaceRoot()),
```
Add helper:
```go
func defaultWorkspaceRoot() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
```

- [ ] **Step 4: Run config tests**

Run: `go test ./internal/config/ -v`
Expected: PASS (all, including the new one).

- [ ] **Step 5: Write failing guard tests**

Create `internal/files/guard_test.go`:
```go
package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuardReadAllWithinRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")

	g, err := NewGuard(dir)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := g.ReadAll([]string{filepath.Join(dir, "a.txt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Content != "hello" {
		t.Fatalf("ReadAll = %+v", docs)
	}
}

func TestGuardRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "x")

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.ReadAll([]string{filepath.Join(outside, "secret.txt")})
	if err == nil {
		t.Fatal("expected error reading outside workspace root")
	}
}

func TestGuardRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "target.txt"), "secret")
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(outside, "target.txt"), link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.ReadAll([]string{link})
	if err == nil {
		t.Fatal("expected error for symlink escaping workspace root")
	}
}

func TestGuardDeniesSensitivePaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".env"), "SECRET=1")

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.ReadAll([]string{filepath.Join(root, ".env")})
	if err == nil {
		t.Fatal("expected .env to be denied")
	}
}

func TestGuardSkipsBinaryFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "text.txt"), "readable")
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{0x00, 0x01, 0x02, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := g.ReadAll([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Content != "readable" {
		t.Fatalf("expected only the text file, got %+v", docs)
	}
}
```

- [ ] **Step 6: Run it, expect FAIL**

Run: `go test ./internal/files/ -run TestGuard -v`
Expected: FAIL — `undefined: NewGuard`.

- [ ] **Step 7: Implement the guard**

Create `internal/files/guard.go`:
```go
package files

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Guard confines file access to a workspace root and blocks sensitive paths.
type Guard struct {
	root string
	deny []string
}

var defaultDeny = []string{".git", ".env", "secrets"}

// NewGuard creates a Guard rooted at root (resolved to an absolute, symlink-free path).
func NewGuard(root string) (*Guard, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace root %q: %w", root, err)
	}
	return &Guard{root: resolved, deny: defaultDeny}, nil
}

// check rejects paths that resolve outside the root or match a denied segment.
func (g *Guard) check(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", p, err)
	}
	rel, err := filepath.Rel(g.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("path %q is outside workspace root %q", p, g.root)
	}
	for _, seg := range strings.Split(rel, string(os.PathSeparator)) {
		for _, d := range g.deny {
			if seg == d {
				return fmt.Errorf("path %q is blocked (matches %q)", p, d)
			}
		}
	}
	return nil
}

// ReadAll expands the paths, rejects any that escape the root or match the deny-list,
// skips binary files (with a stderr warning), and reads the rest into Documents.
func (g *Guard) ReadAll(paths []string) ([]Document, error) {
	fileList, err := Expand(paths)
	if err != nil {
		return nil, err
	}
	var docs []Document
	for _, f := range fileList {
		if err := g.check(f); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", f, err)
		}
		if isBinary(b) {
			fmt.Fprintf(os.Stderr, "claude-llama: skipping binary file %q\n", f)
			continue
		}
		docs = append(docs, Document{Path: f, Content: string(b)})
	}
	return docs, nil
}

// isBinary reports whether b looks binary (contains a NUL byte in the first 512 bytes).
func isBinary(b []byte) bool {
	n := len(b)
	if n > 512 {
		n = 512
	}
	return bytes.IndexByte(b[:n], 0) >= 0
}
```

- [ ] **Step 8: Run files + config suites**

Run: `go test ./internal/files/ ./internal/config/ -v`
Expected: PASS (existing files tests + 5 new guard tests + config tests). Run `go vet ./internal/files/ ./internal/config/` — clean.

- [ ] **Step 9: Commit**

```bash
git add internal/config/ internal/files/guard.go internal/files/guard_test.go
git commit -m "feat: add workspace-root guard and binary-file skipping"
```

---

## Task I2: Thread the Guard through Service and main; remove free ReadAll

**Files:**
- Modify: `internal/tools/service.go`, `internal/tools/service_test.go`
- Modify: `cmd/claude-llama-mcp/main.go`
- Modify: `internal/files/read.go` (remove free `ReadAll`), `internal/files/read_test.go` (remove `TestReadAll`)

- [ ] **Step 1: Update service tests to construct a Guard (write failing test first)**

In `internal/tools/service_test.go`, the `NewService` calls must pass a guard. Replace each `NewService(llm, N)` with a guard-aware helper. Add this helper near the top of the test file (after `writeFile`):
```go
func newGuard(t *testing.T, root string) *files.Guard {
	t.Helper()
	g, err := files.NewGuard(root)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
```
Add the import `"claude-llama/internal/files"`.

Then update each test to build a guard rooted at its temp dir and pass it. Concretely:
- `TestSummarizeSingleChunk`: `svc := NewService(llm, newGuard(t, dir), 1000)`
- `TestSummarizeMapReduce`: `svc := NewService(llm, newGuard(t, dir), 25)`
- `TestExtractEmptyQueryErrors`: this one has no dir; use `svc := NewService(llm, newGuard(t, t.TempDir()), 1000)`
- `TestAskNoPathsSingleCall`: `svc := NewService(llm, newGuard(t, t.TempDir()), 1000)`
- `TestTooLargeErrors`: `svc := NewService(llm, newGuard(t, dir), 1)`

(The new signature is `NewService(llm Completer, guard *files.Guard, maxInputTokens int)`.)

- [ ] **Step 2: Run it, expect FAIL (compile error)**

Run: `go test ./internal/tools/ -v`
Expected: FAIL — `NewService` arg count mismatch / undefined field.

- [ ] **Step 3: Update the Service**

In `internal/tools/service.go`:
- Add import `"claude-llama/internal/files"` (already imported).
- Change the struct and constructor:
```go
type Service struct {
	llm            Completer
	guard          *files.Guard
	maxInputTokens int
}

func NewService(llm Completer, guard *files.Guard, maxInputTokens int) *Service {
	return &Service{llm: llm, guard: guard, maxInputTokens: maxInputTokens}
}
```
- In `mapReduce`, change `files.ReadAll(paths)` to `s.guard.ReadAll(paths)`:
```go
	docs, err := s.guard.ReadAll(paths)
```

- [ ] **Step 4: Update main.go wiring**

In `cmd/claude-llama-mcp/main.go`, build the guard and pass it:
```go
	cfg := config.Load()
	client := llama.New(cfg.APIURL, cfg.Model, cfg.Timeout)
	guard, err := files.NewGuard(cfg.WorkspaceRoot)
	if err != nil {
		log.Fatal(err)
	}
	svc := tools.NewService(client, guard, cfg.MaxInputTokens)
	server := NewServer(svc)
```
Add import `"claude-llama/internal/files"`.

- [ ] **Step 5: Remove the now-unused free ReadAll**

In `internal/files/read.go`, delete the free `ReadAll` function (the `Guard.ReadAll` method replaces it). Keep `Expand`, `Document`, `EstimateTokens`, `Chunk`, `hasGlobMeta`.
In `internal/files/read_test.go`, delete `TestReadAll` (its behavior is now covered by `TestGuardReadAllWithinRoot`).

- [ ] **Step 6: Build and run everything**

Run: `go build ./... && go test ./...`
Expected: PASS across config, files, llama, tools; `cmd` builds. Run `go vet ./...` — clean. Remove any stray binary: `rm -f claude-llama-mcp`.

- [ ] **Step 7: Integration smoke still green**

Run: `go test -tags integration ./cmd/claude-llama-mcp/ -run TestSmoke -v`
Expected: PASS (reply contains "pong"). Note: `TestSmoke` calls `llama_ask` with no paths, so the guard is constructed from `config.Load()` cwd but not exercised on file reads — it should still pass.

- [ ] **Step 8: Commit**

```bash
git add internal/tools/ cmd/claude-llama-mcp/main.go internal/files/read.go internal/files/read_test.go
git commit -m "feat: enforce workspace guard in tool service and server"
```

---

## Task I3: Classify llama failures (timeout / connection / HTTP)

**Files:**
- Modify: `internal/llama/client.go`, `internal/llama/client_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/llama/client_test.go`:
```go
func TestCompleteConnectionFailure(t *testing.T) {
	// Port 1 is not listenable; Do should fail to connect.
	c := New("http://127.0.0.1:1", "test-model", 2*time.Second)
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("expected 'unreachable' message, got %v", err)
	}
}

func TestCompleteTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		io.WriteString(w, `{"choices":[{"message":{"content":"late"}}]}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model", 20*time.Millisecond)
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected 'timeout' message, got %v", err)
	}
}
```
Add imports `"strings"` to the test file if not already present (the file already imports several; add `strings` if missing).

- [ ] **Step 2: Run it, expect FAIL**

Run: `go test ./internal/llama/ -run 'TestCompleteConnectionFailure|TestCompleteTimeout' -v`
Expected: FAIL — current generic `"llama request failed"` message contains neither "unreachable" nor "timeout".

- [ ] **Step 3: Implement error classification**

In `internal/llama/client.go`:
- Add imports `"errors"` and `"net"`.
- Replace the transport-error branch:
```go
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isTimeout(err) {
			return "", fmt.Errorf("llama timeout contacting %s; consider reducing input size: %w", c.baseURL, err)
		}
		return "", fmt.Errorf("llama.cpp unreachable at %s; check that the server is running: %w", c.baseURL, err)
	}
```
- Add the helper at the end of the file:
```go
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}
```
(The non-200 HTTP error branch with status + body stays unchanged.)

- [ ] **Step 4: Run llama suite**

Run: `go test ./internal/llama/ -v`
Expected: PASS (all, including 2 new). `go vet ./internal/llama/` — clean.

- [ ] **Step 5: Commit**

```bash
git add internal/llama/
git commit -m "feat: classify llama timeout and connection failures with actionable errors"
```

---

## Task I4: Centralize prompts + improve tool descriptions

**Files:**
- Create: `internal/tools/prompts.go`
- Modify: `internal/tools/service.go`, `internal/tools/service_test.go`
- Modify: `cmd/claude-llama-mcp/server.go`

- [ ] **Step 1: Write a failing test for output-format guidance**

Append to `internal/tools/service_test.go`:
```go
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
```

- [ ] **Step 2: Run it, expect FAIL**

Run: `go test ./internal/tools/ -run TestSummarizePromptHasFormatGuidance -v`
Expected: FAIL (current prompt says "Output only the summary." — assertion checks substring "only the summary" which is present)... 

If the assertion already passes against the current prompt, that is acceptable — the goal of this step is to lock the guidance in. If it passes immediately, note it and proceed; the refactor below must keep it passing.

- [ ] **Step 3: Create the prompts file**

Create `internal/tools/prompts.go`:
```go
package tools

import "fmt"

func summarizeMapPrompt(focus string) string {
	p := "You are a precise summarizer. Summarize the following file contents concisely, preserving key facts, names, and structure. Output only the summary, with no preamble or conclusion."
	if focus != "" {
		p += " Focus especially on: " + focus + "."
	}
	return p
}

func summarizeReducePrompt() string {
	return "Combine the following partial summaries into one coherent, concise summary. Remove redundancy. Output only the summary, with no preamble."
}

func extractMapPrompt(query string) string {
	return fmt.Sprintf("Extract only the parts of the following file contents relevant to this query: %q. Return relevant snippets with their file path and no preamble. If nothing is relevant, say so briefly.", query)
}

func extractReducePrompt(query string) string {
	return fmt.Sprintf("Merge the following extracted findings into a single answer to the query: %q. Keep file references, remove duplicates, and include no preamble.", query)
}

func askMapPrompt(prompt string) string {
	return "You are a helpful assistant. Apply this instruction to the following file contents and output only the result, with no preamble.\n\nInstruction: " + prompt
}

func askReducePrompt(prompt string) string {
	return "Combine the following partial results into one coherent result for this instruction. Output only the result, with no preamble.\n\nInstruction: " + prompt
}

func askNoContextPrompt() string {
	return "You are a helpful assistant. Follow the instruction precisely and output only the result, with no preamble."
}
```

- [ ] **Step 4: Use the prompt builders in service.go**

In `internal/tools/service.go`, replace the inline prompt strings:
- `Summarize`: build `mapSys := summarizeMapPrompt(strings.TrimSpace(focus))` and `reduceSys := summarizeReducePrompt()`, then `return s.mapReduce(ctx, paths, mapSys, reduceSys)`. (Pass the trimmed focus; the builder appends only when non-empty.)
- `Extract`: keep the blank-query guard; then `mapSys := extractMapPrompt(query)`, `reduceSys := extractReducePrompt(query)`.
- `Ask`: keep the blank-prompt guard; for no paths `return s.llm.Complete(ctx, askNoContextPrompt(), prompt)`; else `mapSys := askMapPrompt(prompt)`, `reduceSys := askReducePrompt(prompt)`.

- [ ] **Step 5: Improve the MCP tool descriptions**

In `cmd/claude-llama-mcp/server.go`, replace the three `Description` strings with:
- `llama_summarize`: `"Summarize files, directories, or globs using a local model WITHOUT reading them into your own context — use this instead of reading large files when you only need an overview. The server reads the files locally and returns only a concise summary."`
- `llama_extract`: `"Search files/directories/globs with a local model and return ONLY the snippets and answers matching a query, WITHOUT reading the files into your own context. Use instead of reading large or numerous files when you need specific information."`
- `llama_ask`: `"Delegate a self-contained task (drafting, classification, mechanical transforms, Q&A) to a local model to save tokens. Optionally provide file paths/globs/directories as context (read locally by the server). Returns text only; it does not write files."`

- [ ] **Step 6: Run everything**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS across all packages (including `TestSummarizeMapReduce`'s reduce-prompt assertion which checks for "Combine", and the new format-guidance test). Remove any stray binary: `rm -f claude-llama-mcp`.

- [ ] **Step 7: Integration smoke**

Run: `go test -tags integration ./cmd/claude-llama-mcp/ -run TestSmoke -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/tools/ cmd/claude-llama-mcp/server.go
git commit -m "feat: centralize prompts with output-format guidance; sharpen tool descriptions"
```

---

## Self-Review Notes

- **Spec coverage (trimmed):** workspace guard rooted at cwd/env (I1+I2), symlink-escape + deny-list (I1), binary skip (I1), failure handling timeout/connection/HTTP (I3), prompt tuning + tool descriptions (I4). Dropped items (caching, tracing, structured output, secret-regex redaction, dual chunk limits) are listed under "Scope decisions" with rationale.
- **Type consistency:** `NewService(llm Completer, guard *files.Guard, maxInputTokens int)` is introduced in I2 and used identically in `main.go` and all service tests. `files.Guard` / `NewGuard` / `Guard.ReadAll` defined in I1, consumed in I2. `config.WorkspaceRoot` defined in I1, consumed in I2's `main.go`.
- **Build stays green per task:** I1 adds the guard without removing the free `ReadAll`; I2 switches consumers then removes it in the same task.
- **No placeholders:** every code step shows the actual code; every run step has a command + expected result.
