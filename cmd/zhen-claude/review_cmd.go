// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/audit/storage"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/zhenagent"
)

// `zhen-claude review list | show <run> | accept <run> [-reason r] |
// reject <run> -reason r` (S-TWO-BRAINS Step 213) records Stevie's
// decision on a branch zhen-agentd's worker left: one-way in
// zhen_agent_runs (succeeded -> accepted | rejected), one audit_events row
// with actor_type human, and the card moved. accept needs the branch head
// merged into the base already (it prints the merge command otherwise);
// reject removes the worktree but keeps the branch (the training-data
// flywheel needs the rejected diff). It runs as Stevie with his own Well
// logins (WELL_APP_ZHEN_DSN for the runs and cards, WELL_OPS_DSN for the
// audit row); nothing here is reachable by a model.

// openReviewStore opens zhen_agent_runs. A variable so tests can stub it.
var openReviewStore = func(ctx context.Context) (zhenagent.Store, func(), error) {
	dsn := strings.TrimSpace(os.Getenv(envAppZhenDSN))
	if dsn == "" {
		return nil, func() {}, fmt.Errorf("%s is not set (unheaded_app as app_zhen); it is read from the environment only", envAppZhenDSN)
	}
	db, err := openWellDB(ctx, dsn)
	if err != nil {
		return nil, func() {}, fmt.Errorf("connect to the Well: %s", redactDSN(err.Error()))
	}
	return zhenagent.SQLStore{DB: db}, func() { _ = db.Close() }, nil
}

// reviewAuditor records the human decision. A variable so tests can stub it.
var openReviewAudit = func(ctx context.Context) (func(ev *audit.AuditEvent) error, func(), error) {
	dsn := strings.TrimSpace(os.Getenv(envWellOpsDSN))
	if dsn == "" {
		return nil, func() {}, fmt.Errorf("%s is not set (unheaded_ops as ops_writer): a decision is not recorded without its audit row", envWellOpsDSN)
	}
	db, err := openWellDB(ctx, dsn)
	if err != nil {
		return nil, func() {}, fmt.Errorf("connect to the Well (ops): %s", redactDSN(err.Error()))
	}
	events, err := storage.NewDatabaseStorage(db, "")
	if err != nil {
		_ = db.Close()
		return nil, func() {}, err
	}
	w, err := claudebridge.NewWellAuditor(claudebridge.WellAuditorConfig{Source: "zhen-claude", Backend: claudebridge.BackendClaude, Events: events})
	if err != nil {
		_ = db.Close()
		return nil, func() {}, err
	}
	record := func(ev *audit.AuditEvent) error {
		w.Event(ev)
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := w.Close(cctx); err != nil {
			return err
		}
		if w.Errors() > 0 || w.Dropped() > 0 {
			return errors.New("the audit row was not written")
		}
		return nil
	}
	return record, func() { _ = db.Close() }, nil
}

// reviewGit runs git for the review subcommand. A variable for tests.
var reviewGit = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).CombinedOutput() // #nosec G204 -- fixed binary; ids validated, no shell
}

func reviewUsage(stderr io.Writer) {
	_, _ = fmt.Fprintln(stderr, "usage: zhen-claude review [-repo DIR] [-base develop] list | show <run> | accept <run> [-reason R] | reject <run> -reason R")
}

func runReviewCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("zhen-claude review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", ".", "the main checkout the branches live in")
	base := fs.String("base", "develop", "the branch an accepted run must be merged into")
	fs.Usage = func() { reviewUsage(stderr); fs.PrintDefaults() }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		reviewUsage(stderr)
		return 2
	}
	verb, id, reason := rest[0], "", ""
	switch verb {
	case "list":
		if len(rest) != 1 {
			reviewUsage(stderr)
			return 2
		}
	case "show", "accept", "reject":
		if len(rest) < 2 || !zhenagent.ValidRunID(rest[1]) {
			_, _ = fmt.Fprintln(stderr, "zhen-claude review: want one 16-hex run id")
			return 2
		}
		id = rest[1]
		vf := flag.NewFlagSet("zhen-claude review "+verb, flag.ContinueOnError)
		vf.SetOutput(stderr)
		vf.StringVar(&reason, "reason", "", "why (required for reject; at most 1000 characters)")
		if err := vf.Parse(rest[2:]); err != nil || vf.NArg() > 0 {
			reviewUsage(stderr)
			return 2
		}
		reason = strings.TrimSpace(reason)
		if verb == "reject" && reason == "" {
			_, _ = fmt.Fprintln(stderr, "zhen-claude review reject: -reason is required")
			return 2
		}
		if verb == "show" && reason != "" {
			reviewUsage(stderr)
			return 2
		}
		if len(reason) > 1000 {
			_, _ = fmt.Fprintln(stderr, "zhen-claude review: -reason is over 1000 characters")
			return 2
		}
	default:
		reviewUsage(stderr)
		return 2
	}
	if strings.HasPrefix(*base, "-") {
		return 2
	}
	repoAbs, err := filepath.Abs(*repo)
	if err != nil {
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store, closeStore, err := openReviewStore(ctx)
	defer closeStore()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude review:", err)
		return 1
	}
	switch verb {
	case "list":
		return reviewList(ctx, store, stdout, stderr)
	case "show":
		return reviewShow(ctx, store, id, stdout, stderr)
	}
	run, err := store.Get(ctx, id)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude review:", err)
		return 1
	}
	if run.Status != zhenagent.StatusSucceeded {
		_, _ = fmt.Fprintf(stderr, "zhen-claude review: run %s is %s; only a succeeded run can be accepted or rejected (once)\n", id, run.Status)
		return 1
	}
	if verb == "accept" {
		if _, err := reviewGit(ctx, repoAbs, "merge-base", "--is-ancestor", run.HeadCommit, *base); err != nil {
			_, _ = fmt.Fprintf(stderr, "zhen-claude review: %s (%s) is not merged into %s yet. Review it, merge it yourself, then accept:\n"+
				"  git -C %s log --stat %s..%s\n  git -C %s switch %s && git -C %s merge --no-ff %s\n  zhen-claude review accept %s\n",
				run.Branch, short(run.HeadCommit), *base, repoAbs, *base, run.Branch, repoAbs, *base, repoAbs, run.Branch, id)
			return 1
		}
	}
	record, closeAudit, err := openReviewAudit(ctx)
	defer closeAudit()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude review:", err)
		return 1
	}
	status, cardTo, progress := zhenagent.StatusAccepted, "done", 100
	if verb == "reject" {
		status, cardTo, progress = zhenagent.StatusRejected, "todo", zhenagent.KeepProgress
	}
	if err := store.Review(ctx, id, status, reason); err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude review:", err)
		return 1
	}
	by := confirmedBy()
	if err := record(reviewEvent(run, verb, by)); err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude review: WARNING: decision recorded in zhen_agent_runs but not in audit_events:", err)
	}
	if err := store.MoveCard(ctx, run.CardID, cardTo, progress, nil); err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude review: card move:", err)
	}
	// The worktree has done its job either way; the branch stays (merged,
	// or kept for the training-data flywheel).
	if wt := worktreeOf(ctx, repoAbs, run.Branch); wt != "" {
		if out, err := reviewGit(ctx, repoAbs, "worktree", "remove", "--", wt); err != nil {
			_, _ = fmt.Fprintf(stderr, "zhen-claude review: worktree %s kept: %v %s\n", wt, err, strings.TrimSpace(string(out)))
		}
	}
	if verb == "reject" {
		_, _ = fmt.Fprintf(stdout, "rejected %s; card %s back to todo; branch %s kept for the training-data flywheel\n", id, run.CardID, run.Branch)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "accepted %s; card %s done; worktree removed\n", id, run.CardID)
	return 0
}

// worktreeOf finds the worktree that has branch checked out.
func worktreeOf(ctx context.Context, repo, branch string) string {
	out, err := reviewGit(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	cur := ""
	for _, l := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(l, "worktree "):
			cur = strings.TrimPrefix(l, "worktree ")
		case l == "branch refs/heads/"+branch:
			return cur
		}
	}
	return ""
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func reviewEvent(run zhenagent.Run, verb, by string) *audit.AuditEvent {
	outcome, severity := audit.OutcomeSuccess, audit.SeverityInfo
	if verb == "reject" {
		outcome, severity = audit.OutcomeFailure, audit.SeverityWarning
	}
	return &audit.AuditEvent{
		Type: audit.EventTypeAuthorization, Severity: severity, Action: "agent_review_" + verb, Outcome: outcome,
		Actor:    &audit.Actor{ID: by, Type: "human"},
		Resource: &audit.Resource{ID: run.ID, Type: "agent_run"},
		Source:   &audit.Source{Service: "zhen-claude", RequestID: run.ID},
		Details:  map[string]any{"run": run.ID, "card": run.CardID, "branch": run.Branch, "head": run.HeadCommit},
	}
}

func reviewList(ctx context.Context, store zhenagent.Store, stdout, stderr io.Writer) int {
	runs, err := store.List(ctx, 50)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude review:", err)
		return 1
	}
	if len(runs) == 0 {
		_, _ = fmt.Fprintln(stdout, "no runs")
		return 0
	}
	for _, r := range runs {
		_, _ = fmt.Fprintf(stdout, "%s  %-9s  %-28s  %-32s  check=%-5s  $%s  %s\n", r.ID, r.Status, r.CardID, r.Branch, r.Summary.Check,
			claudebridge.NanoUSD(r.ClaudeNanoUSD).USD(), r.ClaimedAt.UTC().Format(time.RFC3339))
	}
	return 0
}

func reviewShow(ctx context.Context, store zhenagent.Store, id string, stdout, stderr io.Writer) int {
	r, err := store.Get(ctx, id)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude review:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "run %s: %s\ncard %s, branch %s, mode %s, family %s\nbase %s head %s\nroute %s (%s), local attempts %d, escalated %v, check %s\nClaude $%s\n",
		r.ID, r.Status, r.CardID, r.Branch, r.Mode, r.Family, r.BaseCommit, r.HeadCommit, r.Summary.Route, r.Summary.RouteReason,
		r.Summary.LocalAttempts, r.Summary.Escalated, r.Summary.Check, claudebridge.NanoUSD(r.ClaudeNanoUSD).USD())
	if r.Error != "" {
		_, _ = fmt.Fprintf(stdout, "error: %s\n", oneLine(r.Error))
	}
	if r.ReviewedAt != nil {
		_, _ = fmt.Fprintf(stdout, "reviewed %s: %s\n", r.ReviewedAt.UTC().Format(time.RFC3339), oneLine(r.ReviewReason))
	}
	if r.Summary.ReviewNote != "" {
		_, _ = fmt.Fprintf(stdout, "review note: %s\n", r.Summary.ReviewNote)
	}
	return 0
}
