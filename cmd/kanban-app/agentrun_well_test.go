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

// The card modal's agent run as app_kanban against a throwaway Well, the
// runs written by pkg/zhenagent's store as app_zhen. Since ADR-149 the runs
// are in unheaded_zhen: WELL_TEST_ZHEN_DSN (app_zhen) and
// WELL_TEST_KANBAN_ZHEN_DSN (app_kanban, 052's column grants) beside the
// board's WELL_TEST_APP_DSN (app_zhen) and WELL_TEST_KANBAN_DSN (app_kanban).
func TestWellLatestAgentRun(t *testing.T) {
	adsn, zdsn, kdsn, kzdsn := os.Getenv("WELL_TEST_APP_DSN"), os.Getenv("WELL_TEST_ZHEN_DSN"),
		os.Getenv("WELL_TEST_KANBAN_DSN"), os.Getenv("WELL_TEST_KANBAN_ZHEN_DSN")
	if adsn == "" || zdsn == "" || kdsn == "" || kzdsn == "" {
		t.Skip("WELL_TEST_APP_DSN / WELL_TEST_ZHEN_DSN / WELL_TEST_KANBAN_DSN / WELL_TEST_KANBAN_ZHEN_DSN not set (run scripts/well/test-zhen-agent-runs.sh)")
	}
	ctx := context.Background()
	open := func(dsn string) *sql.DB {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	adb, zdb, kdb, kzdb := open(adsn), open(zdsn), open(kdsn), open(kzdsn)
	card := func(id, typ, owner string) {
		if _, err := kdb.ExecContext(ctx, `INSERT INTO kanban_tasks (id, title, status, type, owner) VALUES ($1, $1, 'todo', $2, $3)`, id, typ, owner); err != nil {
			t.Fatal(err)
		}
	}
	runs := zhenagent.SQLStore{Board: adb, Runs: zdb}
	p := &PgStore{db: kdb}
	reader := runsReader{db: kzdb}
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
	if err := p.SetZhenOptIn(ctx, "kb-optin", zhenagent.CardSHA256("kb-optin", "something else"), "stevie"); err != nil {
		t.Fatal(err)
	}
	if err := claimOptIn("kb-optin"); !errors.Is(err, zhenagent.ErrClaimConflict) {
		t.Fatalf("claim of a card whose description changed: %v", err)
	}
	if err := p.SetZhenOptIn(ctx, "kb-optin", zhenagent.CardSHA256("kb-optin", ""), "stevie"); err != nil {
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
	if r, err := reader.LatestAgentRun(ctx, "kb-none"); err != nil || r != nil {
		t.Fatalf("card without runs: %+v %v", r, err)
	}

	// A worker run: claimed, running, succeeded with a review note.
	card("kb-worked", "task", zhenagent.Owner)
	if err := p.SetZhenOptIn(ctx, "kb-worked", zhenagent.CardSHA256("kb-worked", ""), "stevie"); err != nil {
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
	r, err := reader.LatestAgentRun(ctx, "kb-worked")
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
	if r, err := reader.LatestAgentRun(ctx, "kb-sched"); err != nil || r == nil || r.Mode != "scheduled" || r.Branch != "" || r.Status != "running" {
		t.Errorf("scheduled run: %+v %v", r, err)
	}

	// 052 (041's columns) grants the columns the modal shows, not the error
	// text or the cost.
	for _, col := range []string{"error", "claude_nano_usd", "lease_until", "base_commit"} {
		if _, err := kzdb.ExecContext(ctx, `SELECT `+col+` FROM zhen_agent_runs LIMIT 1`); err == nil {
			t.Errorf("app_kanban reads zhen_agent_runs.%s", col)
		}
	}
	if _, err := kzdb.ExecContext(ctx, `UPDATE zhen_agent_runs SET status = 'cancelled' WHERE id = $1`, sid); err == nil {
		t.Error("app_kanban updated a run")
	}
}
