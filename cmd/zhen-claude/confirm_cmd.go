// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
)

// `zhen-claude confirm list|show <id>|approve <id>|deny <id>` decides
// calls queued by -confirm queue (or zhen-agentd). It runs as Stevie, in
// his own process, with his own Well login (WELL_APP_ZHEN_DSN): nothing
// it does is visible to, or can be answered by, a model.

const envAppZhenDSN = "WELL_APP_ZHEN_DSN"

// confirmStore is the queue as the subcommand uses it.
type confirmStore interface {
	List(ctx context.Context, limit int) ([]claudebridge.QueuedCall, error)
	Get(ctx context.Context, id string) (claudebridge.QueuedCall, error)
	Decide(ctx context.Context, id, status, by string) error
	Finish(ctx context.Context, id, status string) error
}

// openConfirmStore opens the queue. A variable so tests can stub it.
var openConfirmStore = func(ctx context.Context) (confirmStore, func(), error) {
	dsn := strings.TrimSpace(os.Getenv(envAppZhenDSN))
	if dsn == "" {
		return nil, func() {}, fmt.Errorf("%s is not set (unheaded_app as app_zhen); it is read from the environment only", envAppZhenDSN)
	}
	db, err := openWellDB(ctx, dsn)
	if err != nil {
		return nil, func() {}, fmt.Errorf("connect to the Well: %s", redactDSN(err.Error()))
	}
	return claudebridge.SQLConfirmQueue{DB: db}, func() { _ = db.Close() }, nil
}

// confirmedBy names the human deciding: $USER@short-hostname.
var confirmedBy = func() string {
	u := os.Getenv("USER")
	if u == "" {
		u = "unknown"
	}
	h, _ := os.Hostname()
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	if h == "" {
		h = "localhost"
	}
	return u + "@" + h
}

func runConfirmCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("zhen-claude confirm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	roots := fs.String("allowed-roots", "", "comma-separated project roots an approval may run in (default: the current directory)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: zhen-claude confirm [-allowed-roots a,b] list | show <id> | approve <id> | deny <id>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	verb := rest[0]
	var id string
	switch verb {
	case "list":
		if len(rest) != 1 {
			fs.Usage()
			return 2
		}
	case "show", "approve", "deny":
		if len(rest) != 2 || !claudebridge.ValidQueueID(rest[1]) {
			_, _ = fmt.Fprintln(stderr, "zhen-claude confirm: want one 16-hex id")
			return 2
		}
		id = rest[1]
	default:
		fs.Usage()
		return 2
	}
	allowed, err := allowedRoots(*roots)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude confirm:", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store, closeStore, err := openConfirmStore(ctx)
	defer closeStore()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude confirm:", err)
		return 1
	}
	switch verb {
	case "list":
		return confirmList(ctx, store, stdout, stderr)
	case "show":
		return confirmShow(ctx, store, id, stdout, stderr)
	case "deny":
		if err := store.Decide(ctx, id, "denied", confirmedBy()); err != nil {
			_, _ = fmt.Fprintf(stderr, "zhen-claude confirm: deny %s: %v\n", id, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "%s denied\n", id)
		return 0
	default:
		return confirmApprove(ctx, store, id, allowed, stdout, stderr)
	}
}

func allowedRoots(list string) (map[string]bool, error) {
	out := map[string]bool{}
	if strings.TrimSpace(list) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		list = wd
	}
	for _, r := range strings.Split(list, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		abs, err := filepath.Abs(r)
		if err != nil {
			return nil, err
		}
		out[filepath.Clean(abs)] = true
	}
	return out, nil
}

func confirmList(ctx context.Context, store confirmStore, stdout, stderr io.Writer) int {
	rows, err := store.List(ctx, 100)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude confirm: list:", err)
		return 1
	}
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(stdout, "no pending confirmations")
		return 0
	}
	for _, r := range rows {
		_, _ = fmt.Fprintf(stdout, "%s  %s  expires %s  %s  %s\n", r.ID, r.SourceService,
			r.ExpiresAt.Format(time.RFC3339), r.Tool, oneLine(r.Summary))
	}
	return 0
}

// pendingFromRow rebuilds the Champion's view of a queued call.
func pendingFromRow(r claudebridge.QueuedCall) (champion.PendingCall, error) {
	var args map[string]any
	if err := json.Unmarshal(r.Call, &args); err != nil || args == nil {
		return champion.PendingCall{}, errors.New("stored call is not a JSON object")
	}
	return champion.PendingCall{Tool: r.Tool, Args: args, ProjectRoot: r.ProjectRoot, EmittedBy: r.EmittedBy, ExpiresAt: r.ExpiresAt}, nil
}

func confirmShow(ctx context.Context, store confirmStore, id string, stdout, stderr io.Writer) int {
	r, err := store.Get(ctx, id)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "zhen-claude confirm: %s: %v\n", id, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "id:       %s\nstatus:   %s\nsource:   %s\nasked by: %s\ncreated:  %s\nexpires:  %s\nsummary:  %s\n",
		r.ID, r.Status, r.SourceService, oneLine(r.EmittedBy), r.CreatedAt.Format(time.RFC3339), r.ExpiresAt.Format(time.RFC3339), oneLine(r.Summary))
	p, err := pendingFromRow(r)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude confirm:", err)
		return 1
	}
	champ, err := champion.New(champion.Config{ProjectRoot: r.ProjectRoot}, nil)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude confirm:", err)
		return 1
	}
	_, _ = io.WriteString(stdout, claudebridge.DescribeCall(champ, p))
	return 0
}

// confirmApprove re-reads the row and checks it end to end before running
// it: still pending, not expired, a project root on the allowlist, and a
// call that still hashes to what was queued. The diff is recomputed here
// (not trusted from an earlier `show`), and the Champion's gate (Rules 3
// and 1, symlinks resolved) runs again at execution.
func confirmApprove(ctx context.Context, store confirmStore, id string, allowed map[string]bool, stdout, stderr io.Writer) int {
	refuse := func(format string, a ...any) int {
		_, _ = fmt.Fprintf(stderr, "zhen-claude confirm: approve %s refused: %s\n", id, fmt.Sprintf(format, a...))
		return 1
	}
	r, err := store.Get(ctx, id)
	if err != nil {
		return refuse("%v", err)
	}
	if r.Status != "pending" {
		return refuse("it is %s, not pending", r.Status)
	}
	if !time.Now().Before(r.ExpiresAt) {
		_ = store.Decide(ctx, id, "expired", "system")
		return refuse("it expired at %s", r.ExpiresAt.Format(time.RFC3339))
	}
	root := filepath.Clean(r.ProjectRoot)
	if !allowed[root] {
		return refuse("project root %s is not on -allowed-roots", root)
	}
	p, err := pendingFromRow(r)
	if err != nil {
		return refuse("%v", err)
	}
	_, sum, err := champion.CanonicalCall(p.Tool, p.Args, root)
	if err != nil || sum != r.CallSHA256 {
		return refuse("the stored call no longer matches its hash")
	}
	champ, err := champion.New(champion.Config{ProjectRoot: root, SessionID: "confirm-" + id}, nil)
	if err != nil {
		return refuse("%v", err)
	}
	_, _ = io.WriteString(stdout, claudebridge.DescribeCall(champ, p))
	by := confirmedBy()
	if err := store.Decide(ctx, id, "approved", by); err != nil {
		return refuse("%v", err)
	}
	// The queue confirms as QueuePrincipal (RT-03): this command runs
	// under Stevie's own Well login, the out-of-band channel that may
	// confirm any requester's call. by (the human) is what the row records.
	out, err := champ.ConfirmCall(ctx, p, r.CallSHA256, champion.QueuePrincipal)
	if err != nil {
		_ = store.Finish(ctx, id, "failed")
		_, _ = fmt.Fprintf(stderr, "zhen-claude confirm: %s approved by %s but failed: %v\n", id, by, err)
		return 1
	}
	if ferr := store.Finish(ctx, id, "executed"); ferr != nil {
		_, _ = fmt.Fprintf(stderr, "zhen-claude confirm: %s ran, but recording it failed: %v\n", id, ferr)
		return 1
	}
	msg := fmt.Sprintf("%s approved by %s and executed", id, by)
	if s, ok := out.(string); ok && s != "" {
		msg += ": " + oneLine(s)
	}
	_, _ = fmt.Fprintln(stdout, msg)
	return 0
}

func oneLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// openTTY opens the controlling terminal for -confirm tty. Never stdin:
// a piped prompt (or a model's text) must not be able to answer.
var openTTY = func() (io.ReadWriteCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

// buildConfirmer returns the Confirmer for -confirm, plus a closer.
func buildConfirmer(ctx context.Context, o *options, champ *champion.Champion, aud *auditSetup) (claudebridge.Confirmer, func(), error) {
	switch o.confirm {
	case "", "deny":
		return claudebridge.DenyConfirmer{}, func() {}, nil
	case "tty":
		if o.tty == nil {
			return nil, func() {}, errors.New("-confirm tty needs a controlling terminal")
		}
		return &claudebridge.TTYConfirmer{Champion: champ, TTY: o.tty}, func() {}, nil
	case "queue":
		dsn := strings.TrimSpace(os.Getenv(envAppZhenDSN))
		db, err := openWellDB(ctx, dsn)
		if err != nil {
			return nil, func() {}, fmt.Errorf("-confirm queue: connect to the Well: %s", redactDSN(err.Error()))
		}
		qc := &claudebridge.QueueConfirmer{Champion: champ, Queue: claudebridge.SQLConfirmQueue{DB: db}, Source: "zhen-claude"}
		if aud != nil && aud.wotan != nil {
			qc.Pub = aud.wotan
		}
		return qc, func() { _ = db.Close() }, nil
	}
	return nil, func() {}, fmt.Errorf("-confirm %q: want deny, tty or queue", o.confirm)
}
