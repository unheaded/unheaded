// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build wellint

package zhenagent

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	_ "github.com/lib/pq"
)

// splitPools opens the three pools scripts/well/test-zhen-database.sh
// names: runs (unheaded_zhen as app_zhen), board (unheaded_app as
// app_zhen) and the board's own writer (unheaded_app as app_kanban). A
// missing DSN fails: this test never skips.
func splitPools(t *testing.T) (runs, board, kanban *sql.DB) {
	t.Helper()
	open := func(env string) *sql.DB {
		dsn := os.Getenv(env)
		if dsn == "" {
			t.Fatalf("%s is not set (run scripts/well/test-zhen-database.sh)", env)
		}
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	return open("WELL_TEST_ZHEN_DSN"), open("WELL_TEST_APP_DSN"), open("WELL_TEST_KANBAN_DSN")
}

func splitCard(t *testing.T, kanban *sql.DB, id string, optIn bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := kanban.ExecContext(ctx,
		`INSERT INTO kanban_tasks (id, title, description, status, type, owner) VALUES ($1, 'split', 'card', 'todo', 'task', $2)`,
		id, Owner); err != nil {
		t.Fatal(err)
	}
	if optIn {
		if _, err := kanban.ExecContext(ctx,
			`INSERT INTO kanban_zhen_optin (card_id, card_sha256, opted_in_by) VALUES ($1, $2, 'stevie')`,
			id, CardSHA256("split", "card")); err != nil {
			t.Fatal(err)
		}
	}
}

func cardStatus(t *testing.T, kanban *sql.DB, id string) string {
	t.Helper()
	var st string
	if err := kanban.QueryRow(`SELECT status FROM kanban_tasks WHERE id = $1`, id).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func runStatuses(t *testing.T, runs *sql.DB, card string) map[string]int {
	t.Helper()
	rows, err := runs.Query(`SELECT status, count(*) FROM zhen_agent_runs WHERE card_id = $1 GROUP BY status`, card)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			t.Fatal(err)
		}
		out[s] = n
	}
	return out
}

const splitSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func splitRun(card string) Run {
	return Run{ID: NewRunID(), CardID: card, Mode: ModeInteractive, Family: "repo-task", Branch: "zhen/" + card, BaseCommit: splitSHA}
}

// Eight workers claim one card at once: one wins, the card moves once, and
// every losing run that got a row is cancelled (the one-active index stops
// the rest at step 1).
func TestSplit_ClaimRace(t *testing.T) {
	runs, board, kanban := splitPools(t)
	s, err := NewSQLStore(board, runs)
	if err != nil {
		t.Fatal(err)
	}
	splitCard(t, kanban, "split-race", true)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = s.Claim(context.Background(), splitRun("split-race"))
		}(i)
	}
	wg.Wait()
	wins, conflicts := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			wins++
		case errors.Is(e, ErrClaimConflict):
			conflicts++
		default:
			t.Errorf("claim: %v", e)
		}
	}
	if wins != 1 || conflicts != 7 {
		t.Fatalf("wins %d, conflicts %d; want 1 and 7", wins, conflicts)
	}
	st := runStatuses(t, runs, "split-race")
	for k, n := range st {
		if k != "claimed" && k != "cancelled" {
			t.Errorf("a run in %s (%d)", k, n)
		}
	}
	if st["claimed"] != 1 {
		t.Errorf("claimed runs: %d, want 1 (%v)", st["claimed"], st)
	}
	if got := cardStatus(t, kanban, "split-race"); got != "in-progress" {
		t.Errorf("card status %q", got)
	}
}

// A card nobody opted in: the claim conflicts, its run is cancelled, the
// card stays in todo, and the attempt is not counted.
func TestSplit_NotOptedIn(t *testing.T) {
	runs, board, kanban := splitPools(t)
	s, err := NewSQLStore(board, runs)
	if err != nil {
		t.Fatal(err)
	}
	splitCard(t, kanban, "split-noopt", false)
	if err := s.Claim(context.Background(), splitRun("split-noopt")); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("claim: %v", err)
	}
	if st := runStatuses(t, runs, "split-noopt"); st["cancelled"] != 1 || len(st) != 1 {
		t.Errorf("runs: %v", st)
	}
	if got := cardStatus(t, kanban, "split-noopt"); got != "todo" {
		t.Errorf("card status %q", got)
	}
	if n, err := s.Attempts(context.Background(), "split-noopt"); err != nil || n != 0 {
		t.Errorf("attempts = %d, %v", n, err)
	}
}

// claimed -> running -> succeeded, and the card moves to review, as the
// single-database store did.
func TestSplit_Lifecycle(t *testing.T) {
	runs, board, kanban := splitPools(t)
	s, err := NewSQLStore(board, runs)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	splitCard(t, kanban, "split-life", true)
	r := splitRun("split-life")
	if err := s.Claim(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, r.ID, Outcome{Status: StatusSucceeded, HeadCommit: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveCard(ctx, "split-life", "review", 100, &CommitEntry{Branch: r.Branch}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, r.ID)
	if err != nil || got.Status != StatusSucceeded {
		t.Fatalf("run = %+v, %v", got, err)
	}
	if st := cardStatus(t, kanban, "split-life"); st != "review" {
		t.Errorf("card status %q", st)
	}
	cs, err := s.Candidates(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.ID == "split-life" {
			t.Error("a card in review is still a candidate")
		}
	}
}

// The board pool gone: the claim fails and its run is cancelled, so the
// card is not left with a claimed run and no move.
func TestSplit_BoardDown(t *testing.T) {
	runs, _, kanban := splitPools(t)
	dead, err := sql.Open("postgres", os.Getenv("WELL_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	_ = dead.Close()
	s, err := NewSQLStore(dead, runs)
	if err != nil {
		t.Fatal(err)
	}
	splitCard(t, kanban, "split-down", true)
	if err := s.Claim(context.Background(), splitRun("split-down")); err == nil {
		t.Fatal("claim with the board down succeeded")
	}
	if st := runStatuses(t, runs, "split-down"); st["cancelled"] != 1 || len(st) != 1 {
		t.Errorf("runs: %v", st)
	}
}
