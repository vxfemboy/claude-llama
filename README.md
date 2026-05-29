# claude-llama

> Delegate token-heavy file work to a local llama.cpp model so the bulk content never enters Claude's context.

`claude-llama` is an MCP server that exposes three tools — `llama_summarize`, `llama_extract`, `llama_ask` — plus a `llama_health` probe. Claude calls them instead of reading large files itself; the server reads the files locally, hands them to your llama.cpp instance, and returns only the answer.

Every response carries a footer like:

```
---
[claude-llama] input=7,992 tok · returned=931 tok · saved≈7,061 tok · model=Qwen3.5-9B · 141s
```

(real numbers from summarizing a 32KB plan doc — see [Real-world savings](#real-world-savings) for the full matrix.)

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

## Real-world savings

Measured against this repo's own files with a Qwen3.5-9B Q8 model on
local hardware (your mileage will vary with model + GPU):

| Fixture                | Tool              | Input tok | Returned tok | Saved | %    | Duration |
|------------------------|-------------------|----------:|-------------:|------:|-----:|---------:|
| 3KB Go source          | `llama_summarize` |       734 |          409 |   325 |  44% |    1m28s |
| 15KB design spec       | `llama_summarize` |     3,824 |        1,626 | 2,198 |  57% |    2m38s |
| 32KB plan              | `llama_summarize` |     7,992 |          931 | 7,061 |  88% |    2m21s |
| 15KB design spec       | `llama_extract`   |     3,824 |          387 | 3,437 |  90% |     3m4s |
| `llama_ask` (no paths) | `llama_ask`       |        13 |           46 |     0 |   0% |    1m10s |

Read this as: **delegation pays off once you'd be reading more than a
few KB into Claude's context.** Below ~3KB the local model's reply is
nearly as long as the input — net savings are small and you'd be better
off having Claude read the file directly. Above ~10KB savings grow fast,
and `llama_extract` beats `llama_summarize` because it returns only
matching snippets instead of a whole summary. `llama_ask` with no paths
is a wash on tokens (the prompt and answer are both tiny) — its purpose
is offloading bulky generation, not saving context.

The trade-off is latency: 1-3 minutes per call on this hardware vs. a
few seconds for Claude's API. Use this MCP when the *token cost* of the
work matters more than the wall-clock; skip it for snappy interactions.

Reproduce with `make integration` against a live llama, or look at the
matrix test at `cmd/claude-llama-mcp/real_savings_test.go`.

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
