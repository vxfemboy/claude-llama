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
	WorkspaceRoot  string
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
		WorkspaceRoot:  getEnv("LLAMA_WORKSPACE_ROOT", defaultWorkspaceRoot()),
	}
}

func defaultWorkspaceRoot() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
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
