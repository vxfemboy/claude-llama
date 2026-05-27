package config

import (
	"os"
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
