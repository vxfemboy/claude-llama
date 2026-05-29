# claude-llama

> Delegate token-heavy file work to a local llama.cpp model so the bulk content never enters Claude's context.

`claude-llama` is an MCP server that exposes three tools — `llama_summarize`, `llama_extract`, `llama_ask` — plus a `llama_health` probe. Claude calls them instead of reading large files itself; the server reads the files locally, hands them to your llama.cpp instance, and returns only the answer.

Every response carries a footer like:

```
---
[claude-llama] input=12,480 tok · returned=380 tok · saved≈12,100 tok · model=Qwen3.5-9B · 4.2s
```

The savings are also appended to a JSONL log; `claude-llama-mcp stats` summarizes it. CI guards the savings claim with a benchmark.

## Install

**One-liner (recommended):**

```sh
curl -fsSL https://raw.githubusercontent.com/vxfemboy/claude-llama/main/install.sh | sh
```

Downloads the latest release binary for your OS/arch, verifies the checksum, drops it in `~/.local/bin`, and runs `claude-llama-mcp init`.

**As a Claude Code plugin:**

```
/plugin install vxfemboy/claude-llama
```

**From source:**

```sh
go install github.com/vxfemboy/claude-llama/cmd/claude-llama-mcp@latest
claude-llama-mcp init
```

After installing, register it with your MCP client. For Claude Code, add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "claude-llama": { "command": "claude-llama-mcp" }
  }
}
```

## Configuration

All settings are environment variables. `claude-llama-mcp init` writes them to `~/.config/claude-llama/env` (honoring `$XDG_CONFIG_HOME`); the process env always wins over the file.

| Variable                 | Default                              | Purpose                                                         |
|--------------------------|--------------------------------------|-----------------------------------------------------------------|
| `LLAMA_API_URL`          | `http://localhost:8080`              | llama.cpp server (OpenAI-compatible)                            |
| `LLAMA_MODEL`            | `unsloth/Qwen3.5-9B-GGUF:Q4_K_M`     | model name passed to `/v1/chat/completions`                     |
| `LLAMA_MAX_INPUT_TOKENS` | `6000`                               | max tokens per chunk before map/reduce kicks in                 |
| `LLAMA_TIMEOUT_SECONDS`  | `120`                                | per-call timeout                                                |
| `LLAMA_WORKSPACE_ROOT`   | cwd                                  | path-traversal boundary; the server refuses to read outside it  |
| `LLAMA_FOOTER`           | `true`                               | append the per-call savings footer to each response             |
| `LLAMA_USAGE_LOG`        | `true`                               | append a JSONL row per call to `$XDG_STATE_HOME/claude-llama/usage.jsonl` |

Set any value to `0`, `false`, `no`, or `off` to disable a boolean.

## Tools

- **`llama_summarize`** `(paths, focus?)` — summarize files/dirs/globs.
- **`llama_extract`** `(paths, query)` — pull only snippets matching `query`.
- **`llama_ask`** `(prompt, paths?)` — delegate a self-contained task; paths are optional context.
- **`llama_health`** `()` — JSON status: `{ok, url, models, latency_ms, error}`. Lets Claude self-diagnose before relying on the MCP for a big job.

## Verifying the savings

Per call: read the footer. Cumulatively:

```sh
claude-llama-mcp stats              # last 7 days
claude-llama-mcp stats --since 24h
claude-llama-mcp stats --tool llama_extract --json
```

The CI bench (`make bench`) runs three fixtures through `httptest`-replayed llama responses and asserts each tool produces ≥80% byte savings. That's the regression guard for the project's pitch.

## Troubleshooting

```sh
claude-llama-mcp doctor
```

Prints resolved config, pings the llama server, lists available models, and checks that the workspace root and usage log are writable. Exits non-zero if anything fails.

## Development

```sh
make build      # build ./bin/claude-llama-mcp
make test       # go test -race ./...
make bench      # token-savings regression bench
make integration # smoke against a real llama (needs LLAMA_API_URL up)
make lint       # golangci-lint
make setup      # install the pre-commit hook
```

Source layout:

- `cmd/claude-llama-mcp/` — entrypoint, MCP server, CLI subcommands (`init`, `doctor`, `stats`).
- `internal/config/` — env-var + env-file loader.
- `internal/files/` — workspace guard + chunking.
- `internal/llama/` — chat-completions client + `/v1/models` health probe.
- `internal/tools/` — map/reduce service that wraps the three delegation tools.
- `internal/usage/` — token estimator, JSONL recorder, savings footer.

## License

MIT.
