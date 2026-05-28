package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"claude-llama/internal/config"
)

// Row is one append-only entry in the JSONL usage log. Keep field names stable —
// the `stats` subcommand parses these.
type Row struct {
	TS           time.Time `json:"ts"`
	Tool         string    `json:"tool"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	Saved        int       `json:"saved"`
	DurationMs   int64     `json:"duration_ms"`
	OK           bool      `json:"ok"`
	Error        string    `json:"error,omitempty"`
}

// Recorder appends Rows to the per-user usage log. Safe for concurrent use.
// A disabled recorder (Footer off + UsageLog off, or no writable path) is a
// no-op — tools should always call Record regardless and let the recorder decide.
type Recorder struct {
	cfg     config.Config
	path    string
	mu      sync.Mutex
	maxSize int64
}

// LogPath returns the JSONL path. Honors XDG_STATE_HOME, falling back to
// $HOME/.local/state. Returns "" if neither is usable; the recorder then disables logging.
func LogPath() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "claude-llama", "usage.jsonl")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "claude-llama", "usage.jsonl")
}

// NewRecorder constructs a Recorder backed by LogPath().
func NewRecorder(cfg config.Config) *Recorder {
	return &Recorder{cfg: cfg, path: LogPath(), maxSize: 10 * 1024 * 1024}
}

// Footer renders the per-call summary appended to tool responses.
// Returns "" when LLAMA_FOOTER is off, so callers can blindly concatenate.
func (r *Recorder) Footer(row Row) string {
	if r == nil || !r.cfg.Footer {
		return ""
	}
	status := ""
	if !row.OK {
		status = " · failed"
	}
	return fmt.Sprintf(
		"\n\n---\n[claude-llama] input=%s tok · returned=%s tok · saved≈%s tok · model=%s · %.1fs%s\n",
		commas(row.InputTokens),
		commas(row.OutputTokens),
		commas(row.Saved),
		row.Model,
		float64(row.DurationMs)/1000.0,
		status,
	)
}

// Record appends a row to the JSONL log. Best-effort: I/O errors are swallowed
// to avoid breaking tool calls when the user's $HOME is unwritable.
func (r *Recorder) Record(row Row) {
	if r == nil || !r.cfg.UsageLog || r.path == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return
	}
	if fi, err := os.Stat(r.path); err == nil && fi.Size() > r.maxSize {
		_ = os.Rename(r.path, r.path+".1")
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()

	buf, err := json.Marshal(row)
	if err != nil {
		return
	}
	_, _ = f.Write(append(buf, '\n'))
}

// commas formats an int with thousands separators, e.g. 12480 -> "12,480".
func commas(n int) string {
	if n < 0 {
		return "-" + commas(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}
