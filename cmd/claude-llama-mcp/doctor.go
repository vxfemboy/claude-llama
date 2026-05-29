package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude-llama/internal/config"
	"claude-llama/internal/llama"
	"claude-llama/internal/usage"
)

// runDoctor performs a series of human-readable checks and exits non-zero
// if any of them fail. Pairs with `init`: init writes config, doctor validates it.
func runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	noColor := fs.Bool("no-color", false, "disable ANSI colors")
	if err := fs.Parse(args); err != nil {
		return err
	}
	d := &doctorPrinter{color: !*noColor && isatty(os.Stdout)}

	cfg := config.Load()

	// 1. Config source visibility.
	d.info("Config:")
	d.info("  env file: %s", config.EnvFilePath())
	d.info("  LLAMA_API_URL          = %s", cfg.APIURL)
	d.info("  LLAMA_MODEL            = %s", cfg.Model)
	d.info("  LLAMA_MAX_INPUT_TOKENS = %d", cfg.MaxInputTokens)
	d.info("  LLAMA_TIMEOUT_SECONDS  = %d", int(cfg.Timeout.Seconds()))
	d.info("  LLAMA_WORKSPACE_ROOT   = %s", cfg.WorkspaceRoot)
	d.info("  LLAMA_FOOTER           = %v", cfg.Footer)
	d.info("  LLAMA_USAGE_LOG        = %v", cfg.UsageLog)

	failures := 0

	// 2. Workspace root writable check.
	if err := canWriteDir(cfg.WorkspaceRoot); err != nil {
		d.fail("workspace root not writable: %v", err)
		failures++
	} else {
		d.pass("workspace root writable")
	}

	// 3. Usage log path writable.
	logPath := usage.LogPath()
	if logPath == "" {
		d.warn("usage log path could not be determined (set $XDG_STATE_HOME or $HOME)")
	} else if err := canWriteDir(filepath.Dir(logPath)); err != nil {
		d.fail("usage log dir not writable (%s): %v", filepath.Dir(logPath), err)
		failures++
	} else {
		d.pass("usage log writable at %s", logPath)
	}

	// 4. Llama reachability + model presence.
	hc := llama.NewHealthClient(cfg.APIURL, 5*time.Second)
	res, err := hc.Check(ctx)
	if err != nil {
		d.fail("llama unreachable at %s: %v", cfg.APIURL, err)
		d.info("  hint: is llama.cpp running? `curl %s/v1/models`", cfg.APIURL)
		failures++
	} else {
		d.pass("llama reachable at %s (%dms, %d models)", cfg.APIURL, res.LatencyMs, len(res.Models))
		if !res.HasModel(cfg.Model) {
			d.warn("configured model %q not in /v1/models listing", cfg.Model)
			if len(res.Models) > 0 {
				d.info("  available: %s", strings.Join(res.Models, ", "))
			}
		} else {
			d.pass("model %q is loaded", cfg.Model)
		}
		// Show the model's actual context window vs the configured chunk size.
		// Catches the "default 6000 blows past a 4096 ctx" foot-gun before
		// the user discovers it mid-call.
		if props, err := hc.FetchProps(ctx); err == nil && props.NCtx > 0 {
			budget := llama.ChunkBudgetFromCtx(props.NCtx)
			d.info("  model n_ctx = %d (recommended chunk budget = %d)", props.NCtx, budget)
			if config.MaxInputTokensExplicit() && cfg.MaxInputTokens > budget {
				d.warn("LLAMA_MAX_INPUT_TOKENS=%d exceeds recommended %d; expect 400s from llama",
					cfg.MaxInputTokens, budget)
			} else if !config.MaxInputTokensExplicit() {
				d.pass("chunk budget will auto-size to %d on serve", budget)
			}
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d check(s) failed", failures)
	}
	return nil
}

// canWriteDir verifies dir exists (creating it if needed) and is writable.
func canWriteDir(dir string) error {
	if dir == "" {
		return errors.New("empty path")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".claude-llama-doctor-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

type doctorPrinter struct{ color bool }

func (d *doctorPrinter) pass(f string, a ...any) { d.line("✓", "32", f, a...) }
func (d *doctorPrinter) fail(f string, a ...any) { d.line("✗", "31", f, a...) }
func (d *doctorPrinter) warn(f string, a ...any) { d.line("!", "33", f, a...) }
func (d *doctorPrinter) info(f string, a ...any) { fmt.Printf(f+"\n", a...) }

func (d *doctorPrinter) line(mark, code, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if d.color {
		fmt.Printf("\x1b[%sm%s\x1b[0m %s\n", code, mark, msg)
		return
	}
	fmt.Printf("%s %s\n", mark, msg)
}

// isatty reports whether f looks like a terminal. We avoid pulling in
// golang.org/x/term for one boolean; checking ModeCharDevice covers the cases we need.
func isatty(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
