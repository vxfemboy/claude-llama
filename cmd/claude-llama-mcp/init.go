package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"claude-llama/internal/config"
	"claude-llama/internal/llama"
)

// runInit writes ~/.config/claude-llama/env after prompting the user
// (or, in --non-interactive mode, taking values from flags).
//
// Idempotent: if the file already exists, prints a diff and refuses to
// overwrite without --force.
func runInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var (
		force          = fs.Bool("force", false, "overwrite an existing env file")
		nonInteractive = fs.Bool("non-interactive", false, "skip prompts; take values from flags")
		urlFlag        = fs.String("url", "", "LLAMA_API_URL value")
		modelFlag      = fs.String("model", "", "LLAMA_MODEL value")
		maxTokens      = fs.Int("max-input-tokens", 0, "LLAMA_MAX_INPUT_TOKENS value (0 = keep default)")
		timeoutSecs    = fs.Int("timeout", 0, "LLAMA_TIMEOUT_SECONDS value (0 = keep default)")
		skipPing       = fs.Bool("skip-ping", false, "do not validate the URL by hitting /v1/models")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	path := config.EnvFilePath()
	if path == "" {
		return errors.New("cannot determine config path; set $XDG_CONFIG_HOME or $HOME")
	}

	values := map[string]string{
		"LLAMA_API_URL":          orDefault(*urlFlag, "http://localhost:8080"),
		"LLAMA_MODEL":            orDefault(*modelFlag, "unsloth/Qwen3.5-9B-GGUF:Q4_K_M"),
		"LLAMA_MAX_INPUT_TOKENS": intOr(*maxTokens, "6000"),
		"LLAMA_TIMEOUT_SECONDS":  intOr(*timeoutSecs, "120"),
	}

	if !*nonInteractive {
		stdin := bufio.NewReader(os.Stdin)
		values["LLAMA_API_URL"] = prompt(stdin, "Llama API URL", values["LLAMA_API_URL"])
		values["LLAMA_MODEL"] = prompt(stdin, "Model", values["LLAMA_MODEL"])
		values["LLAMA_MAX_INPUT_TOKENS"] = prompt(stdin, "Max input tokens per chunk", values["LLAMA_MAX_INPUT_TOKENS"])
		values["LLAMA_TIMEOUT_SECONDS"] = prompt(stdin, "Request timeout (seconds)", values["LLAMA_TIMEOUT_SECONDS"])
	}

	if !*skipPing {
		if err := pingLlama(ctx, values["LLAMA_API_URL"], values["LLAMA_MODEL"]); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not reach llama at %s: %v\n", values["LLAMA_API_URL"], err)
			fmt.Fprintln(os.Stderr, "  (writing config anyway; run `claude-llama-mcp doctor` once the server is up)")
		}
	}

	if existing, err := os.ReadFile(path); err == nil {
		if !*force {
			fmt.Fprintf(os.Stderr, "config already exists at %s:\n", path)
			fmt.Fprintln(os.Stderr, indent(string(existing)))
			fmt.Fprintln(os.Stderr, "would write:")
			fmt.Fprintln(os.Stderr, indent(renderEnvFile(values)))
			fmt.Fprintln(os.Stderr, "re-run with --force to overwrite")
			return errors.New("refusing to overwrite without --force")
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(renderEnvFile(values)), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("wrote %s\n", path)
	fmt.Println("next: register the MCP in your client (see README) and run `claude-llama-mcp doctor`")
	return nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func intOr(n int, def string) string {
	if n > 0 {
		return strconv.Itoa(n)
	}
	return def
}

func prompt(r *bufio.Reader, label, def string) string {
	fmt.Printf("%s [%s]: ", label, def)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return def
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// renderEnvFile produces a deterministic KEY=VALUE block in a stable order.
func renderEnvFile(values map[string]string) string {
	order := []string{
		"LLAMA_API_URL",
		"LLAMA_MODEL",
		"LLAMA_MAX_INPUT_TOKENS",
		"LLAMA_TIMEOUT_SECONDS",
	}
	var b strings.Builder
	b.WriteString("# claude-llama config — written by `claude-llama-mcp init`\n")
	b.WriteString("# Override any value by exporting the same variable in your shell.\n\n")
	for _, k := range order {
		fmt.Fprintf(&b, "%s=%s\n", k, values[k])
	}
	return b.String()
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// pingLlama hits /v1/models with a short timeout, just to surface obvious
// "you fat-fingered the URL" issues during init. It is intentionally lenient.
func pingLlama(ctx context.Context, url, model string) error {
	c := llama.NewHealthClient(url, 5*time.Second)
	res, err := c.Check(ctx)
	if err != nil {
		return err
	}
	if model != "" && !res.HasModel(model) {
		fmt.Fprintf(os.Stderr, "warning: model %q not listed by %s (available: %s)\n",
			model, url, strings.Join(res.Models, ", "))
	}
	return nil
}
