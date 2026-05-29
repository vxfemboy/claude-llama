package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"claude-llama/internal/usage"
)

// runStats reads the JSONL usage log and prints either a human summary
// or the raw aggregate as JSON. No live llama required.
func runStats(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	since := fs.String("since", "7d", "window: 24h, 7d, 30d, all")
	tool := fs.String("tool", "", "filter to one tool (llama_summarize, llama_extract, llama_ask)")
	asJSON := fs.Bool("json", false, "emit the aggregate as JSON instead of a human summary")
	path := fs.String("path", "", "override the usage log path (default: $XDG_STATE_HOME/claude-llama/usage.jsonl)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	p := *path
	if p == "" {
		p = usage.LogPath()
	}
	if p == "" {
		return errors.New("cannot determine usage log path; set $XDG_STATE_HOME or $HOME")
	}

	rows, err := readRows(p, *since, *tool)
	if err != nil {
		return err
	}

	agg := aggregate(rows)
	if *asJSON {
		buf, _ := json.MarshalIndent(agg, "", "  ")
		fmt.Println(string(buf))
		return nil
	}
	printSummary(agg, *since, *tool)
	return nil
}

type aggResult struct {
	Window      string         `json:"window"`
	Tool        string         `json:"tool,omitempty"`
	Calls       int            `json:"calls"`
	InputTokens int            `json:"input_tokens"`
	OutputTok   int            `json:"output_tokens"`
	Saved       int            `json:"saved"`
	OK          int            `json:"ok"`
	AvgMs       int64          `json:"avg_duration_ms"`
	ByTool      map[string]int `json:"by_tool"`
}

func readRows(path, since, tool string) ([]usage.Row, error) {
	cutoff, err := windowCutoff(since)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no usage log yet at %s — run a tool first", path)
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var rows []usage.Row
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var r usage.Row
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue // tolerate corrupt lines, don't fail the whole report
		}
		if !cutoff.IsZero() && r.TS.Before(cutoff) {
			continue
		}
		if tool != "" && r.Tool != tool {
			continue
		}
		rows = append(rows, r)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return rows, nil
}

// windowCutoff converts "24h"/"7d"/"30d"/"all" into a time.Time threshold.
func windowCutoff(s string) (time.Time, error) {
	if s == "all" || s == "" {
		return time.Time{}, nil
	}
	now := time.Now()
	if strings.HasSuffix(s, "h") {
		d, err := time.ParseDuration(s)
		if err != nil {
			return time.Time{}, err
		}
		return now.Add(-d), nil
	}
	if strings.HasSuffix(s, "d") {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err != nil {
			return time.Time{}, err
		}
		return now.AddDate(0, 0, -days), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized --since %q (use 24h, 7d, 30d, all)", s)
}

func aggregate(rows []usage.Row) aggResult {
	a := aggResult{ByTool: map[string]int{}}
	var totalMs int64
	for _, r := range rows {
		a.Calls++
		a.InputTokens += r.InputTokens
		a.OutputTok += r.OutputTokens
		a.Saved += r.Saved
		totalMs += r.DurationMs
		if r.OK {
			a.OK++
		}
		a.ByTool[r.Tool]++
	}
	if a.Calls > 0 {
		a.AvgMs = totalMs / int64(a.Calls)
	}
	return a
}

func printSummary(a aggResult, window, tool string) {
	label := fmt.Sprintf("Last %s", window)
	if window == "all" {
		label = "All time"
	}
	if tool != "" {
		label += " (" + tool + ")"
	}
	if a.Calls == 0 {
		fmt.Printf("%s: no calls recorded\n", label)
		return
	}
	successPct := 100 * float64(a.OK) / float64(a.Calls)
	fmt.Printf("%s: %d calls · %s tokens saved · %.0f%% success · avg %.1fs/call\n",
		label, a.Calls, withCommas(a.Saved), successPct, float64(a.AvgMs)/1000.0)

	// Sort tools deterministically for readable output.
	tools := make([]string, 0, len(a.ByTool))
	for k := range a.ByTool {
		tools = append(tools, k)
	}
	sort.Strings(tools)
	parts := make([]string, 0, len(tools))
	for _, t := range tools {
		parts = append(parts, fmt.Sprintf("%s %d", t, a.ByTool[t]))
	}
	if len(parts) > 0 {
		fmt.Println("By tool:", strings.Join(parts, " · "))
	}
}

// withCommas mirrors usage.commas without exporting it.
func withCommas(n int) string {
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
