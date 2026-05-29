package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// clearEnv blanks every LLAMA_* var the package reads so tests start from a
// known state regardless of the developer's shell.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"LLAMA_API_URL",
		"LLAMA_MODEL",
		"LLAMA_MAX_INPUT_TOKENS",
		"LLAMA_TIMEOUT_SECONDS",
		"LLAMA_WORKSPACE_ROOT",
		"LLAMA_FOOTER",
		"LLAMA_USAGE_LOG",
	} {
		t.Setenv(k, "")
	}
	// Point the env-file at a directory that won't exist, so tests don't
	// accidentally pick up the developer's real ~/.config/claude-llama/env.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	ResetEnvFileCache()
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	cfg := Load()

	if cfg.APIURL != "http://localhost:8080" {
		t.Errorf("APIURL = %q, want localhost default", cfg.APIURL)
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
	if !cfg.Footer {
		t.Errorf("Footer default = false, want true")
	}
	if !cfg.UsageLog {
		t.Errorf("UsageLog default = false, want true")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("LLAMA_API_URL", "http://localhost:9999")
	t.Setenv("LLAMA_MODEL", "other-model")
	t.Setenv("LLAMA_MAX_INPUT_TOKENS", "1000")
	t.Setenv("LLAMA_TIMEOUT_SECONDS", "30")
	t.Setenv("LLAMA_FOOTER", "off")
	t.Setenv("LLAMA_USAGE_LOG", "false")

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
	if cfg.Footer {
		t.Errorf("Footer = true, want false when LLAMA_FOOTER=off")
	}
	if cfg.UsageLog {
		t.Errorf("UsageLog = true, want false when LLAMA_USAGE_LOG=false")
	}
}

func TestLoadWorkspaceRoot(t *testing.T) {
	clearEnv(t)
	t.Setenv("LLAMA_WORKSPACE_ROOT", "/tmp/some-root")
	if got := Load().WorkspaceRoot; got != "/tmp/some-root" {
		t.Errorf("WorkspaceRoot = %q, want /tmp/some-root", got)
	}

	clearEnv(t)
	wd, _ := os.Getwd()
	if got := Load().WorkspaceRoot; got != wd {
		t.Errorf("WorkspaceRoot default = %q, want cwd %q", got, wd)
	}
}

func TestLoadFromEnvFile(t *testing.T) {
	clearEnv(t)

	// Build a fake XDG_CONFIG_HOME with an env file the loader should pick up.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	envDir := filepath.Join(dir, "claude-llama")
	if err := os.MkdirAll(envDir, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := "" +
		"# a comment line\n" +
		"\n" +
		"LLAMA_API_URL=http://from-file:8080\n" +
		"LLAMA_MODEL=\"file-model\"\n" +
		"LLAMA_TIMEOUT_SECONDS=45\n"
	if err := os.WriteFile(filepath.Join(envDir, "env"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	ResetEnvFileCache()

	cfg := Load()
	if cfg.APIURL != "http://from-file:8080" {
		t.Errorf("APIURL from file = %q", cfg.APIURL)
	}
	if cfg.Model != "file-model" {
		t.Errorf("Model from file = %q (quotes should have been stripped)", cfg.Model)
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("Timeout from file = %v", cfg.Timeout)
	}
}

func TestProcessEnvBeatsEnvFile(t *testing.T) {
	clearEnv(t)

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	envDir := filepath.Join(dir, "claude-llama")
	if err := os.MkdirAll(envDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "env"),
		[]byte("LLAMA_API_URL=http://from-file:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ResetEnvFileCache()

	t.Setenv("LLAMA_API_URL", "http://from-shell:7777")
	if got := Load().APIURL; got != "http://from-shell:7777" {
		t.Errorf("process env should win, got %q", got)
	}
}
