# claude-llama — Token-Saving Delegation Plugin

**Date:** 2026-05-26
**Status:** Approved design, pre-implementation

## Problem

Claude Code spends tokens reading large files, scanning codebases, and doing
rote work that does not need Claude-level reasoning. A local llama.cpp server
running `unsloth/Qwen3.5-9B-GGUF:Q4_K_M` (at `http://hack-mini:8080`) can do
much of that work for free. The goal is a Claude Code plugin that lets Claude
delegate such sub-tasks to the local model so the bulk content never enters
Claude's context window — only distilled results return.

## Goals

- Save Claude tokens on: bulk reading/summarizing, search & extract, mechanical
  code edits, and drafting/classification.
- Delegation is invoked by Claude during a session via MCP tools.
- File content is read **server-side** from paths, so it never costs Claude tokens.

## Non-Goals (v1)

- Standalone CLI fallback mode (`claude-llama ask/chat`) for when Claude is
  rate-limited — documented as a planned later phase, not built in v1.
- Tools writing edits directly to disk.
- Workspace-root path restriction / sandboxing.

## Architecture

A single Go binary, `claude-llama-mcp`, runs as an **MCP server over stdio**.
The plugin registers it through `.mcp.json` pointing at
`${CLAUDE_PLUGIN_ROOT}/bin/claude-llama-mcp`. The server calls llama.cpp's
OpenAI-compatible endpoint (`POST /v1/chat/completions`).

```
Claude session ──calls tool──▶ claude-llama-mcp (Go, stdio)
                                   │ reads files from disk
                                   ▼
                              llama.cpp  http://hack-mini:8080
                                   │ returns text
                                   ▼
              result ◀──small payload── back to Claude
```

All file content flows into the server; only distilled results flow back to
Claude. That asymmetry is the token savings.

**SDK:** official `modelcontextprotocol/go-sdk`.

## Tools

Approach: a small specialized set with self-documenting names so Claude reaches
for them at the right moment.

- **`llama_summarize(paths, focus?)`** — expands paths/globs/dirs, reads files,
  returns a condensed summary. Optional `focus` steers emphasis.
- **`llama_extract(paths, query)`** — reads files, returns only the
  snippets/answers matching `query` (search & extract).
- **`llama_ask(prompt, paths?)`** — generic catch-all for drafting,
  classification, and mechanical transforms. `paths` optional; when present,
  content is appended as context.

Tool descriptions actively nudge Claude to use them *instead of* reading large
files when only a summary/answer is needed.

## Input Handling & the 8192-token Limit

The served context is 8192 tokens, so large inputs need care.

- An internal `files` package expands paths (file / glob / dir), reads them, and
  tags each chunk with a `// file: <path>` header.
- A char-based budget (~4 chars/token, configurable) chooses the path:
  - **Under budget** → single call to llama.cpp.
  - **Over budget** → **map-reduce**: chunk → summarize/extract each chunk →
    combine results into a final call. Applies to `summarize` and `extract`;
    `llama_ask` with oversized paths uses the same chunking.
- A hard ceiling guard returns a clear error rather than a truncated/garbage
  answer if input is implausibly large even for map-reduce.

## Writing to Disk

v1 tools **never write files** — they return text only. Mechanical edits via
`llama_ask` come back as text; Claude applies them with its own Edit tool.
Rationale: a 9B model silently overwriting files is risky, and edit outputs are
usually small enough that returning inline costs little. Disk-writing could be a
later opt-in mode.

## Configuration

Set via env vars in `.mcp.json`, defaults matching the current setup:

| Var | Default | Purpose |
|-----|---------|---------|
| `LLAMA_API_URL` | `http://hack-mini:8080` | llama.cpp base URL |
| `LLAMA_MODEL` | `unsloth/Qwen3.5-9B-GGUF:Q4_K_M` | model id |
| `LLAMA_MAX_INPUT_TOKENS` | ~6000 | input budget, leaves room for output |
| `LLAMA_TIMEOUT` | (e.g. 120s) | HTTP timeout per request |

## Project Structure

```
claude-llama/
  .claude-plugin/plugin.json     (exists)
  .mcp.json                      (registers the server)
  go.mod
  Makefile                       (build → bin/claude-llama-mcp)
  cmd/claude-llama-mcp/main.go   (wires SDK + tools)
  internal/llama/client.go       (HTTP client to llama.cpp)
  internal/files/read.go         (expand paths, read, chunk)
  internal/tools/summarize.go
  internal/tools/extract.go
  internal/tools/ask.go
  bin/claude-llama-mcp           (built artifact)
```

## Error Handling

- Endpoint unreachable → clear error string back to Claude so it can fall back
  to reading the content itself.
- Oversized input → map-reduce, or explicit ceiling error if still too large.
- Missing/invalid path → descriptive error.
- Reads are local-machine only (acceptable; runs on the user's box). A
  workspace-root guard is noted as future hardening.

## Future Phases

1. Standalone CLI mode (`claude-llama ask`, interactive `claude-llama chat`) on
   the same binary, for working when Claude is rate-limited.
2. Opt-in disk-writing edit mode.
3. Workspace-root path restriction.
4. Multiple-model support: route to a different local model depending on the
   task at hand (e.g., a coding-tuned model for transforms, a small fast model
   for trivial classification), with optional per-call model override.
