// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build wellint

package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"unheaded/pkg/zhenagent"
)

// PgStore.LatestAgentRun as app_kanban against a throwaway Well, the runs
// written by pkg/zhenagent's store as app_zhen (scripts/well/test-zhen-agent-runs.sh
// sets the DSNs; migration 041 grants the read).
func TestWellLatestAgentRun(t *testing.T) {
	zdsn, kdsn := os.Getenv("WELL_TEST_APP_DSN"), os.Getenv("WELL_TEST_KANBAN_DSN")
	if zdsn == "" || kdsn == "" {
		t.Skip("WELL_TEST_APP_DSN / WELL_TEST_KANBAN_DSN not set (run scripts/well/test-zhen-agent-runs.sh)")
	}
	ctx := context.Background()
	zdb, err := sql.Open("postgres", zdsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zdb.Close() }()
	kdb, err := sql.Open("postgres", kdsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = kdb.Close() }()
	card := func(id, typ, owner string) {
		if _, err := kdb.ExecContext(ctx, `INSERT INTO kanban_tasks (id, title, status, type, owner) VALUES ($1, $1, 'todo', $2, $3)`, id, typ, owner); err != nil {
			t.Fatal(err)
		}
	}
	runs := zhenagent.SQLStore{DB: zdb}
	p := &PgStore{db: kdb}
	// The opt-in route as this store runs it, then the worker's view of it.
	card("kb-optin", "task", zhenagent.Owner)
	if o, err := p.ZhenOptInFor(ctx, "kb-optin"); err != nil || o != nil {
		t.Fatalf("no opt-in yet: %+v %v", o, err)
	}
	claimOptIn := func(id string) error {
		return runs.Claim(ctx, zhenagent.Run{ID: zhenagent.NewRunID(), CardID: id, Mode: zhenagent.ModeOvernight, Family: "repo-task",
			Branch: "zhen/" + id, BaseCommit: strings.Repeat("a", 40)})
	}
	if err := claimOptIn("kb-optin"); !errors.Is(err, zhenagent.ErrClaimConflict) {
		t.Fatalf("claim with no opt-in: %v", err)
	}
	if err := p.SetZhenOptIn(ctx, "kb-optin", zhenagent.DescriptionSHA256("something else"), "stevie"); err != nil {
		t.Fatal(err)
	}
	if err := claimOptIn("kb-optin"); !errors.Is(err, zhenagent.ErrClaimConflict) {
		t.Fatalf("claim of a card whose description changed: %v", err)
	}
	if err := p.SetZhenOptIn(ctx, "kb-optin", zhenagent.DescriptionSHA256(""), "stevie"); err != nil {
		t.Fatal(err)
	}
	if o, err := p.ZhenOptInFor(ctx, "kb-optin"); err != nil || o == nil || o.By != "stevie" {
		t.Fatalf("opt-in: %+v %v", o, err)
	}
	if cs, err := runs.Candidates(ctx, 100); err != nil || len(cs) == 0 {
		t.Fatalf("candidates: %v %v", cs, err)
	}
	if err := claimOptIn("kb-optin"); err != nil {
		t.Fatalf("claim of an opted-in card: %v", err)
	}
	if err := p.ClearZhenOptIn(ctx, "kb-optin"); err != nil {
		t.Fatal(err)
	}
	if o, err := p.ZhenOptInFor(ctx, "kb-optin"); err != nil || o != nil {
		t.Fatalf("withdrawn: %+v %v", o, err)
	}

	card("kb-none", "task", "")
	if r, err := p.LatestAgentRun(ctx, "kb-none"); err != nil || r != nil {
		t.Fatalf("card without runs: %+v %v", r, err)
	}

	// A worker run: claimed, running, succeeded with a review note.
	card("kb-worked", "task", zhenagent.Owner)
	if err := p.SetZhenOptIn(ctx, "kb-worked", zhenagent.DescriptionSHA256(""), "stevie"); err != nil {
		t.Fatal(err)
	}
	id := zhenagent.NewRunID()
	if err := runs.Claim(ctx, zhenagent.Run{ID: id, CardID: "kb-worked", Mode: zhenagent.ModeOvernight, Family: "repo-task",
		Branch: "zhen/kb-worked", BaseCommit: strings.Repeat("a", 40)}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := runs.Finish(ctx, id, zhenagent.Outcome{Status: zhenagent.StatusSucceeded, HeadCommit: strings.Repeat("b", 40),
		Summary: zhenagent.RouteSummary{Check: "pass", ReviewNote: "/var/lib/zhen/zhen-reviews/kb-worked.md"}, Error: "secret-ish text"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Review(ctx, id, zhenagent.StatusRejected, "wrong file"); err != nil {
		t.Fatal(err)
	}
	r, err := p.LatestAgentRun(ctx, "kb-worked")
	if err != nil || r == nil {
		t.Fatalf("worked card: %+v %v", r, err)
	}
	r.sanitize()
	if r.RunID != id || r.Status != "rejected" || r.Mode != "overnight" || r.Branch != "zhen/kb-worked" ||
		r.HeadCommit != strings.Repeat("b", 40) || r.ReviewNote != "/var/lib/zhen/zhen-reviews/kb-worked.md" ||
		r.Check != "pass" || r.ReviewReason != "wrong file" || r.ReviewedAt == nil || r.ClaimedAt.IsZero() {
		t.Errorf("run = %+v", r)
	}

	// A scheduled run: no branch, no head, never reviewed.
	card("kb-sched", zhenagent.ScheduledCardType, zhenagent.ScheduledCardOwner)
	sid := zhenagent.NewRunID()
	if err := runs.BeginScheduled(ctx, zhenagent.Run{ID: sid, CardID: "kb-sched", Family: "service-health-sweep",
		LeaseUntil: time.Now().Add(15 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if r, err := p.LatestAgentRun(ctx, "kb-sched"); err != nil || r == nil || r.Mode != "scheduled" || r.Branch != "" || r.Status != "running" {
		t.Errorf("scheduled run: %+v %v", r, err)
	}

	// 041 grants the columns the modal shows, not the error text or the cost.
	for _, col := range []string{"error", "claude_nano_usd", "lease_until", "base_commit"} {
		if _, err := kdb.ExecContext(ctx, `SELECT `+col+` FROM zhen_agent_runs LIMIT 1`); err == nil {
			t.Errorf("app_kanban reads zhen_agent_runs.%s", col)
		}
	}
	if _, err := kdb.ExecContext(ctx, `UPDATE zhen_agent_runs SET status = 'cancelled' WHERE id = $1`, sid); err == nil {
		t.Error("app_kanban updated a run")
	}
}
