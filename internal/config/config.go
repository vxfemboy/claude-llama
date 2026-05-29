package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	APIURL         string
	Model          string
	MaxInputTokens int
	Timeout        time.Duration
	WorkspaceRoot  string
	Footer         bool
	UsageLog       bool
}

const (
	defaultAPIURL         = "http://localhost:8080"
	defaultModel          = "unsloth/Qwen3.5-9B-GGUF:Q4_K_M"
	defaultMaxInputTokens = 6000
	defaultTimeoutSeconds = 120
)

// EnvFilePath returns the path the env-file fallback reads from.
// Honors XDG_CONFIG_HOME, falling back to $HOME/.config.
func EnvFilePath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "claude-llama", "env")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "claude-llama", "env")
}

func Load() Config {
	return Config{
		APIURL:         getEnv("LLAMA_API_URL", defaultAPIURL),
		Model:          getEnv("LLAMA_MODEL", defaultModel),
		MaxInputTokens: getEnvInt("LLAMA_MAX_INPUT_TOKENS", defaultMaxInputTokens),
		Timeout:        time.Duration(getEnvInt("LLAMA_TIMEOUT_SECONDS", defaultTimeoutSeconds)) * time.Second,
		WorkspaceRoot:  getEnv("LLAMA_WORKSPACE_ROOT", defaultWorkspaceRoot()),
		Footer:         getEnvBool("LLAMA_FOOTER", true),
		UsageLog:       getEnvBool("LLAMA_USAGE_LOG", true),
	}
}

func defaultWorkspaceRoot() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// getEnv resolves a key in this order: process env, env-file, default.
// An empty value in the process env falls through to the file so test code
// that clears a variable with t.Setenv("KEY", "") still gets the default.
func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	if v, ok := envFile()[key]; ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if s := getEnv(key, ""); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

// getEnvBool treats "0", "false", "no", "off" (case-insensitive) as false.
// Any other non-empty value is true. Missing falls through to def.
func getEnvBool(key string, def bool) bool {
	s := strings.ToLower(strings.TrimSpace(getEnv(key, "")))
	if s == "" {
		return def
	}
	switch s {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

var (
	envFileOnce sync.Once
	envFileMap  map[string]string
)

// envFile lazy-loads KEY=VALUE pairs from EnvFilePath(). Missing or unreadable
// files yield an empty map; this is a best-effort fallback, not a hard source.
func envFile() map[string]string {
	envFileOnce.Do(func() {
		envFileMap = map[string]string{}
		path := EnvFilePath()
		if path == "" {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			eq := strings.IndexByte(line, '=')
			if eq <= 0 {
				continue
			}
			k := strings.TrimSpace(line[:eq])
			v := strings.TrimSpace(line[eq+1:])
			v = strings.Trim(v, `"'`)
			envFileMap[k] = v
		}
	})
	return envFileMap
}

// ResetEnvFileCache clears the memoized env-file read. Test-only.
func ResetEnvFileCache() {
	envFileOnce = sync.Once{}
	envFileMap = nil
}

// MaxInputTokensExplicit reports whether the user explicitly set
// LLAMA_MAX_INPUT_TOKENS (via process env or the env file). When false,
// the server is free to auto-detect a chunk size from the model's actual context.
func MaxInputTokensExplicit() bool {
	if v, ok := os.LookupEnv("LLAMA_MAX_INPUT_TOKENS"); ok && v != "" {
		return true
	}
	_, ok := envFile()["LLAMA_MAX_INPUT_TOKENS"]
	return ok
}
