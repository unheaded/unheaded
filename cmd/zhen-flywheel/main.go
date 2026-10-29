// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// zhen-flywheel turns training captures into labelled, filtered,
// held-out training data (S-TWO-BRAINS Phases 10-11,
// wiki/Zhen-Training-Data.md).
//
//	zhen-flywheel ingest [-dir $CAPTURE_DIR]   capture files -> the Well (strict, re-scanned, deduped)
//	zhen-flywheel label                        outcomes -> labels (checks, escalations, reviews, judges, edits)
//	zhen-flywheel export -out <dir>            the Well -> SFT / preference / tool-trace files, split, tokenized
//	zhen-flywheel scan <dir>                   secret/PII re-scan of an export (exit 1 on any finding)
//	zhen-flywheel quarantine <capture-id>      mark one capture for the purge
//	zhen-flywheel import-eval -results <f> -answers-dir <d>   kept eval answers -> capture files
//
// The Well: WELL_APP_ZHEN_DSN (unheaded_app as app_zhen), required;
// WELL_OPS_DSN (unheaded_ops, for llm_requests) optional for label. No
// default for either: credentials are never in the repo.
//
// Claude-authored text never leaves export unless -include-claude-text
// (or ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT=1) is given; it is off until
// Anthropic confirms in writing (S-TWO-BRAINS Step 340).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"unheaded/pkg/cmdline"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

const usage = `usage: zhen-flywheel <ingest|label|export|scan|quarantine|import-eval> [flags]
  WELL_APP_ZHEN_DSN  unheaded_app as app_zhen (required by ingest, label, export, quarantine)
  WELL_OPS_DSN       unheaded_ops (optional; label reads llm_requests escalations from it)
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	code := 0
	switch args[0] {
	case "ingest":
		code, err = cmdIngest(ctx, args[1:], stdout, stderr)
	case "label":
		code, err = cmdLabel(ctx, args[1:], stdout, stderr)
	case "export":
		code, err = cmdExport(ctx, args[1:], stdout, stderr)
	case "import-eval":
		code, err = cmdImportEval(ctx, args[1:], stdout, stderr)
	case "scan":
		code, err = cmdScan(ctx, args[1:], stdout, stderr)
	case "quarantine":
		code, err = cmdQuarantine(ctx, args[1:], stdout, stderr)
	case "-h", "--help", "help":
		_, _ = io.WriteString(stdout, usage)
		return 0
	case "--version", "-version", "-V":
		cmdline.PrintVersion(stdout, cmdline.Spec{Name: "zhen-flywheel"})
		return 0
	default:
		return cmdline.ReportUnknownCommand(stderr, cmdline.Spec{Name: "zhen-flywheel"}, args[0])
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "zhen-flywheel %s: %v\n", args[0], err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

// openStore is overridden by tests.
var openStore = func(needOps bool) (Store, func(), error) {
	app := os.Getenv("WELL_APP_ZHEN_DSN")
	if app == "" {
		return nil, nil, errors.New("WELL_APP_ZHEN_DSN is unset (unheaded_app as app_zhen); credentials are not stored in this repo")
	}
	ops := ""
	if needOps {
		ops = os.Getenv("WELL_OPS_DSN")
	}
	s, err := openPG(app, ops)
	if err != nil {
		return nil, nil, err
	}
	return s, s.Close, nil
}

// newScanner is overridden by tests.
var newScanner = func() (Scanner, error) { return newPrivacyScanner() }

func defaultCaptureDir() string {
	if d := os.Getenv("ZHEN_CAPTURE_DIR"); d != "" {
		return d
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, _ := os.UserHomeDir()
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "unheaded", "zhen-capture")
}

func printJSON(w io.Writer, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	_, _ = fmt.Fprintf(w, "%s\n", b)
}

func cmdIngest(ctx context.Context, args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", defaultCaptureDir(), "capture directory (outside any repository, 0700)")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	sc, err := newScanner()
	if err != nil {
		return 1, fmt.Errorf("re-scan needs the privacy tier: %w", err)
	}
	st, closeFn, err := openStore(false)
	if err != nil {
		return 1, err
	}
	defer closeFn()
	in := &Ingester{Store: st, Scanner: sc, Log: stderr}
	rep, err := in.IngestDir(ctx, *dir)
	printJSON(stdout, rep)
	if err != nil {
		return 1, err
	}
	if len(rep.Errors) > 0 {
		return 1, fmt.Errorf("%d line(s) or file(s) refused (listed above); those files stay in %s", len(rep.Errors), *dir)
	}
	return 0, nil
}

func cmdScan(ctx context.Context, args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		_, _ = io.WriteString(stderr, "usage: zhen-flywheel scan <export-dir>\n")
		return 2, nil
	}
	sc, err := newScanner()
	if err != nil {
		return 1, err
	}
	rep, err := scanDir(ctx, sc, fs.Arg(0))
	printJSON(stdout, rep)
	if err != nil {
		return 1, err
	}
	if len(rep.Findings) > 0 {
		return 1, fmt.Errorf("%d finding(s): quarantine those captures (zhen-flywheel quarantine <id>) and export again; never hand-edit an export", len(rep.Findings))
	}
	return 0, nil
}

func cmdQuarantine(ctx context.Context, args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) != 1 || !captureRE.MatchString(args[0]) {
		_, _ = io.WriteString(stderr, "usage: zhen-flywheel quarantine <64-hex capture id>\n")
		return 2, nil
	}
	st, closeFn, err := openStore(false)
	if err != nil {
		return 1, err
	}
	defer closeFn()
	ok, err := st.Quarantine(ctx, args[0])
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, fmt.Errorf("no capture %s", args[0])
	}
	_, _ = fmt.Fprintf(stdout, "quarantined %s: excluded from every export from now on; deleted, with its labels, by the next SELECT zhen_captures_purge();\n", args[0])
	return 0, nil
}
