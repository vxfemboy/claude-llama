package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude-llama/internal/config"
)

func testCfg(t *testing.T) config.Config {
	t.Helper()
	// Both flags default on; the env file is irrelevant because we override
	// the log path on the Recorder directly.
	return config.Config{Footer: true, UsageLog: true, Model: "test-model"}
}

func TestFooterFormat(t *testing.T) {
	r := &Recorder{cfg: testCfg(t)}
	out := r.Footer(Row{
		InputTokens: 12480, OutputTokens: 380, Saved: 12100,
		Model: "Qwen3.5-9B", DurationMs: 4231, OK: true,
	})
	for _, want := range []string{"input=12,480", "returned=380", "saved≈12,100", "model=Qwen3.5-9B", "4.2s"} {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing %q\n%s", want, out)
		}
	}
}

func TestFooterOff(t *testing.T) {
	r := &Recorder{cfg: config.Config{Footer: false}}
	if got := r.Footer(Row{InputTokens: 1}); got != "" {
		t.Errorf("footer disabled but got %q", got)
	}
}

func TestRecorderAppendsJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	r := &Recorder{cfg: testCfg(t), path: path, maxSize: 1 << 20}

	r.Record(Row{TS: time.Now(), Tool: "llama_summarize", InputTokens: 1000, OutputTokens: 100, Saved: 900, OK: true})
	r.Record(Row{TS: time.Now(), Tool: "llama_ask", InputTokens: 500, OutputTokens: 50, Saved: 450, OK: false, Error: "boom"})

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var rows []Row
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var row Row
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Tool != "llama_summarize" || rows[1].Error != "boom" {
		t.Errorf("unexpected rows: %+v", rows)
	}
}

func TestRecorderRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	r := &Recorder{cfg: testCfg(t), path: path, maxSize: 10} // rotate aggressively

	r.Record(Row{Tool: "a"})
	r.Record(Row{Tool: "b"}) // pushes over the tiny budget
	r.Record(Row{Tool: "c"})

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected rotated file at %s.1: %v", path, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected live log at %s: %v", path, err)
	}
}

func TestRecorderDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	r := &Recorder{cfg: config.Config{UsageLog: false}, path: path, maxSize: 1 << 20}
	r.Record(Row{Tool: "x"})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no log file when UsageLog is off, got err=%v", err)
	}
}

func TestCommas(t *testing.T) {
	cases := map[int]string{0: "0", 1: "1", 999: "999", 1000: "1,000", 12480: "12,480", 1234567: "1,234,567"}
	for in, want := range cases {
		if got := commas(in); got != want {
			t.Errorf("commas(%d) = %q, want %q", in, got, want)
		}
	}
}
