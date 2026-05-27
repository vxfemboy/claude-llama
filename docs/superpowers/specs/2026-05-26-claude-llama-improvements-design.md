# claude-llama — Security, Caching, and Observability Improvements

**Date:** 2026-05-26  
**Status:** Design approved, pre-implementation  
**Related:** [Original design](./2026-05-26-claude-llama-token-delegation-design.md)

---

## Executive Summary

This document outlines security hardening, caching, observability, and prompt engineering improvements for the claude-llama MCP plugin before v1 release. These changes address the gaps identified in the original design while maintaining backward compatibility.

---

## Revised Design Checklist

### Security Hardening (v1)

- [ ] **Workspace-root path guard**
  - Default behavior: all file paths must be under the plugin's `CLAUDEPLUGINROOT`
  - Reject paths outside workspace with clear error: `"path %q is outside workspace root %q"`
  - Optional flag `--allow-parent` for explicit opt-in (documented as advanced feature)
  - Block symlinks that escape workspace root
  - Block access to `.git`, `.env`, `secrets/` directories by default

- [ ] **Structured output mode**
  - Add `--structured` flag to tools for JSON output
  - Local model returns JSON first, then format to text at final step
  - Enables machine parsing and reduces ambiguity

- [ ] **Secret detection & redaction**
  - Pre-filter for environment variable patterns (`KEY=`, `password`, `secret`, `token`)
  - Redact or exclude matches before sending to llama.cpp
  - Log redaction count for observability

- [ ] **Write blocking**
  - Confirm: v1 tools never write to disk (already in design)
  - Add explicit error if user attempts to use `llama_ask` for file edits
  - Suggest using Claude's native Edit tool instead

### Caching Layer (v1)

- [ ] **File hash cache**
  - Compute SHA256 hash of file contents on first read
  - Cache hash → content mapping in memory (per session)
  - Return cached content if same file requested again
  - Log cache hit/miss ratio

- [ ] **Result cache**
  - Cache tool call results keyed by: `(request_id, paths, query)`
  - TTL: 5 minutes (configurable via `LLAMA_CACHE_TTL`)
  - Size limit: 10 MB (evict LRU when exceeded)
  - Include cache hit info in response metadata

- [ ] **Hash computation**
  - Use streaming hash to avoid loading entire file into memory
  - Fail gracefully on binary files (detect by magic bytes)
  - Skip cache for binary files (differentiate by extension or magic bytes)

### Observability (v1)

- [ ] **Request IDs**
  - Generate UUID for each tool call
  - Include in all logs and error messages
  - Enable tracing across components

- [ ] **Latency logging**
  - Log: `expand_paths_ms`, `read_files_ms`, `llama_ms`, `total_ms`
  - Log to stderr (non-blocking) and optionally to file

- [ ] **Token estimates**
  - Log: `input_tokens_estimated`, `output_tokens_estimated`
  - Compute using `EstimateTokens()` from existing `files` package
  - Include in response metadata

- [ ] **Chunk counts**
  - Log: `chunks_before_map_reduce`, `chunks_after_map_reduce`
  - Track token savings: `(raw_tokens - distilled_tokens)`

- [ ] **Error codes**
  - Standardize error types: `ERR_FILE_NOT_FOUND`, `ERR_OUTSIDE_WORKSPACE`, `ERR_LLM_TIMEOUT`, `ERR_INPUT_TOO_LARGE`
  - Return structured error with code and message

### Failure Handling (v1)

- [ ] **LLM timeout fallback**
  - Default timeout: 120s (configurable)
  - On timeout: return clear error `"llama timeout after %ds; consider reducing input size"`
  - Include suggestion: "Claude can read the files directly if needed"

- [ ] **LLM connection failure**
  - Detect: connection refused, DNS failure, HTTP 5xx
  - Return: `"llama.cpp unreachable at %s; check that the server is running"`
  - Include server URL in error for debugging

- [ ] **Oversized input handling**
  - Soft limit: 50 chunks (already implemented)
  - Hard limit: 200 chunks → error `"input exceeds maximum size of 200 chunks"`
  - Return actionable error, not generic failure

- [ ] **Binary file handling**
  - Detect binary files by magic bytes (first 512 bytes)
  - Skip binary files with warning: `"skipping binary file %q"`
  - Or return error: `"binary file not supported: %q"`

### Tool Boundaries & Guidance (v1)

- [ ] **Tool descriptions (MCP tool metadata)**
  - `llama_summarize`: "Summarize files, directories, or globs. Use for broad overviews of code or documentation. Returns concise summary text."
  - `llama_extract`: "Search and extract specific information from files. Use for finding code patterns, API signatures, or documentation. Returns only relevant snippets."
  - `llama_ask`: "Answer questions or perform mechanical tasks using file context. Use for classification, translation, or code generation. Returns answer or result."

- [ ] **Prompt templates (per task)**
  - Create separate prompt templates for each tool
  - Store in `internal/tools/prompts/` as Go strings or JSON
  - Enable easy tuning without code changes
  - Include system instruction, user instruction, and format instructions

- [ ] **Output format guidance**
  - `summarize`: "Output only the summary, no introductions or conclusions"
  - `extract`: "Return extracted findings with file references, no preamble"
  - `ask`: "Answer the question directly, use bullet points if helpful"

### Testing (v1)

- [ ] **Unit tests**
  - Test workspace path validation logic
  - Test symlink detection
  - Test binary file detection
  - Test hash computation
  - Test chunking logic with edge cases

- [ ] **Integration tests**
  - Test with real files in sandboxed directory
  - Test error propagation from llama.cpp
  - Test caching layer with repeated calls
  - Test observability logging output

- [ ] **Bad case tests**
  - Missing files → clear error
  - Binary files → skip or error
  - Huge directory trees → chunk limit error
  - Recursive symlinks → detect and error
  - LLM timeout → fallback error message
  - LLM connection failure → clear error with server URL

---

## Implementation Roadmap

### Phase 1: Security (Week 1)

**Priority: Blocker for v1**

1. **Workspace-root guard** (2-3 days)
   - Add `WorkspaceGuard` struct to `internal/files/`
   - Implement `ValidatePath()` with workspace root check
   - Add symlink detection
   - Add directory exclusion list (`.git`, `.env`, `secrets/`)
   - Wire into `Expand()` function
   - Write unit tests

2. **Secret detection** (1-2 days)
   - Add `DetectAndRedactSecrets()` to `internal/files/`
   - Regex patterns for common secrets
   - Redact before sending to llama.cpp
   - Log redaction count
   - Write unit tests

3. **Structured output mode** (2 days)
   - Add `--structured` flag to tool calls
   - Modify prompt templates to request JSON
   - Add JSON parsing layer
   - Convert JSON to text at final step
   - Write integration tests

### Phase 2: Caching (Week 2)

**Priority: High for performance**

1. **File hash cache** (2-3 days)
   - Add `FileHashCache` struct to `internal/files/`
   - Implement `ComputeHash()` with streaming
   - Implement `GetContent()` with cache lookup
   - Add binary file detection
   - Integrate into `ReadAll()`
   - Write unit and integration tests

2. **Result cache** (2 days)
   - Add `ResultCache` struct to `internal/tools/`
   - Implement LRU eviction
   - Implement TTL expiration
   - Cache results by `(request_id, paths, query)`
   - Integrate into tool calls
   - Write unit tests

3. **Cache observability** (1 day)
   - Add cache hit/miss metrics
   - Log cache statistics
   - Write tests

### Phase 3: Observability (Week 2)

**Priority: Medium (nice to have)**

1. **Request IDs** (1 day)
   - Add UUID generation
   - Include in logs and errors
   - Write tests

2. **Latency logging** (1 day)
   - Add timing instrumentation
   - Log to stderr
   - Write tests

3. **Token estimates** (1 day)
   - Add token counting
   - Include in metadata
   - Write tests

4. **Chunk counts** (1 day)
   - Add chunk tracking
   - Log savings
   - Write tests

5. **Error codes** (1 day)
   - Standardize error types
   - Add error codes to responses
   - Write tests

### Phase 4: Failure Handling (Week 3)

**Priority: High for reliability**

1. **Timeout handling** (1 day)
   - Add timeout configuration
   - Implement timeout detection
   - Write clear error messages
   - Write tests

2. **Connection failure handling** (1 day)
   - Detect connection issues
   - Return actionable errors
   - Write tests

3. **Binary file handling** (1 day)
   - Detect binary files
   - Skip or error (configurable)
   - Write tests

4. **Oversized input handling** (1 day)
   - Implement hard limit
   - Return actionable error
   - Write tests

### Phase 5: Tool Boundaries & Prompts (Week 3)

**Priority: Medium (improves UX)**

1. **Tool descriptions** (1 day)
   - Update MCP tool metadata
   - Write clear descriptions
   - Write tests

2. **Prompt templates** (2 days)
   - Create template files
   - Implement template loader
   - Wire into tools
   - Write tests

3. **Output format guidance** (1 day)
   - Update system prompts
   - Add format instructions
   - Write tests

### Phase 6: Testing & Docs (Week 4)

**Priority: Blocker for v1**

1. **Unit tests** (2 days)
   - Write all unit tests
   - Achieve 80%+ coverage
   - Write tests

2. **Integration tests** (2 days)
   - Set up test fixtures
   - Write integration tests
   - Write tests

3. **Documentation** (1 day)
   - Write README with setup instructions
   - Write docs for each improvement
   - Write docs

---

## Reference List

### MCP & Go SDK

| Resource | URL | Purpose |
|----------|-----|---------|
| modelcontextprotocol/go-sdk | https://github.com/modelcontextprotocol/go-sdk | Official Go SDK for MCP servers |
| mcp package docs | https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp | API reference for tools, server setup |
| modelcontextprotocol/specification | https://spec.modelcontextprotocol.io/ | MCP protocol specification |

### Claude Code

| Resource | URL | Purpose |
|----------|-----|---------|
| Claude Code extensions | https://code.claude.com/docs/en/features-overview | Plugin architecture, hooks, MCP integration |
| Claude Code plugins README | https://github.com/anthropics/claude-code/blob/main/plugins/README.md | Packaging, distribution, examples |
| PreToolUse hooks | https://code.claude.com/docs/en/hooks | Intercept tool calls before execution |

### llama.cpp

| Resource | URL | Purpose |
|----------|-----|---------|
| llama.cpp GitHub | https://github.com/ggerganov/llama.cpp | Main repository, API docs |
| llama-server docs | https://github.com/ggerganov/llama.cpp/blob/master/docs/server.md | OpenAI-compatible endpoint docs |
| llama.cpp examples | https://github.com/ggerganov/llama.cpp/tree/master/examples | Usage examples |

### Alternative MCP Implementations

| Resource | URL | Purpose |
|----------|-----|---------|
| mark3labs/mcp-go | https://github.com/mark3labs/mcp-go | Alternative Go MCP implementation |
| mcp-python | https://github.com/jlowin/fastmcp | Python MCP server reference |
| mcp-node | https://github.com/modelcontextprotocol/js | TypeScript/Node MCP reference |

### Security & Caching Patterns

| Resource | URL | Purpose |
|----------|-----|---------|
| Go crypto/sha256 | https://pkg.go.dev/crypto/sha256 | Hash computation |
| Go LRU cache | https://pkg.go.dev/github.com/hashicorp/golang-lru | LRU cache implementation |
| path/filepath | https://pkg.go.dev/path/filepath | Path manipulation, symlink detection |

---

## Polished Implementation Prompt

Use this prompt when delegating to a coding agent or engineer:

```
## Task: Implement Security, Caching, and Observability for claude-llama MCP Plugin

## Context

You are building a Go MCP server that delegates token-heavy file analysis to a local
llama.cpp server. The current implementation has three tools (llama_summarize,
llama_extract, llama_ask) but needs security hardening, caching, and observability
before v1 release.

## Goals

1. **Security**: Prevent file reads outside workspace root, detect secrets, block writes
2. **Caching**: Reduce redundant processing with file hash and result caching
3. **Observability**: Add request IDs, latency logging, token estimates, chunk counts
4. **Failure Handling**: Clear errors for timeouts, connection failures, oversized input
5. **Tool Boundaries**: Improve tool descriptions and prompt templates

## Constraints

- Do not let raw file contents enter Claude's context
- Default to read-only behavior (no writes in v1)
- Return concise, useful results
- Prefer structured outputs (JSON) where possible
- Fail clearly and safely on errors

## Project Structure

```
claude-llama/
  cmd/claude-llama-mcp/main.go
  internal/
    config/
      config.go              # Environment loading
    files/
      read.go               # Path expansion, file reading, chunking
      cache.go              # NEW: File hash and result caching
      guard.go              # NEW: Workspace path validation
      secrets.go            # NEW: Secret detection and redaction
    llama/
      client.go             # HTTP client to llama.cpp
    tools/
      service.go            # Map-reduce orchestration
      prompts/              # NEW: Prompt templates
      structured.go         # NEW: JSON output handling
  .mcp.json                 # Plugin registration
  Makefile
```

## Deliverables

1. **Security Layer** (`internal/files/guard.go`, `internal/files/secrets.go`)
   - Workspace root validation
   - Symlink detection
   - Directory exclusion (`.git`, `.env`, `secrets/`)
   - Secret detection and redaction

2. **Caching Layer** (`internal/files/cache.go`)
   - File hash cache (SHA256, streaming)
   - Result cache (LRU, TTL-based)
   - Binary file detection

3. **Observability** (integrate into existing code)
   - Request ID generation
   - Latency logging
   - Token estimates
   - Chunk counts

4. **Failure Handling** (enhance existing error handling)
   - Timeout detection with clear messages
   - Connection failure handling
   - Binary file handling
   - Oversized input handling

5. **Tool Boundaries** (update existing tools)
   - Better tool descriptions in MCP metadata
   - Prompt templates per task
   - Output format guidance

6. **Tests**
   - Unit tests for all new code
   - Integration tests with real files
   - Bad case tests (missing files, binary files, huge trees, timeouts)

## Implementation Order

1. Start with security (workspace guard, secret detection)
2. Add caching (file hash, result cache)
3. Add observability (request IDs, logging)
4. Enhance failure handling
5. Improve tool boundaries and prompts
6. Write comprehensive tests

## Testing Requirements

- Achieve 80%+ unit test coverage
- Include integration tests with real llama.cpp server
- Test all error paths (timeout, connection failure, oversized input)
- Test security boundaries (path validation, secret redaction)

## Success Criteria

- All security checks pass in unit tests
- Caching reduces redundant processing by 50%+
- Observability logs include all required fields
- Clear, actionable error messages for all failure modes
- Documentation complete and accurate
```

---

## Next Steps

1. Review this design spec
2. Approve or request changes
3. Invoke writing-plans skill to create detailed implementation plan
4. Implement in phases as outlined in roadmap

---

**End of Document**
