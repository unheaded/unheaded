// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package zhenagent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// conformance runs the same contract against any Store: the MemStore
// here, the SQLStore against a throwaway Well (store_well_test.go).
func conformance(t *testing.T, s Store, addCard func(Card), prefix string) {
	t.Helper()
	ctx := context.Background()
	id := func(n string) string { return prefix + n }
	sha := "0123456789abcdef0123456789abcdef01234567"
	addCard(Card{ID: id("ok"), Title: "ok", Description: "x", Status: "todo", Type: "task", Owner: Owner})
	addCard(Card{ID: id("theirs"), Title: "theirs", Status: "todo", Type: "task", Owner: "Stevie"})
	addCard(Card{ID: id("wish"), Title: "wish", Status: "todo", Type: "wishlist", Owner: Owner})
	addCard(Card{ID: id("race"), Title: "race", Status: "todo", Type: "bug", Owner: Owner})

	cs, err := s.Candidates(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range cs {
		got[c.ID] = true
	}
	if !got[id("ok")] || !got[id("race")] || got[id("theirs")] || got[id("wish")] {
		t.Fatalf("candidates = %v", got)
	}

	// A race: 8 claims for one card, one wins.
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Claim(ctx, Run{ID: NewRunID(), CardID: id("race"), Mode: ModeInteractive, Family: "repo-task",
				Branch: "zhen/" + id("race"), BaseCommit: sha, LeaseUntil: time.Now().Add(time.Hour)})
			if err == nil {
				wins.Add(1)
			} else if !errors.Is(err, ErrClaimConflict) {
				t.Errorf("claim: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("race winners = %d", wins.Load())
	}

	// claim -> start -> finish succeeded -> review accepted.
	run := Run{ID: NewRunID(), CardID: id("ok"), Mode: ModeOvernight, Family: "repo-task", Branch: "zhen/" + id("ok"), BaseCommit: sha,
		LeaseUntil: time.Now().Add(time.Hour)}
	if err := s.Claim(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim(ctx, Run{ID: NewRunID(), CardID: id("ok"), Mode: ModeInteractive, Family: "x", Branch: "zhen/x", BaseCommit: sha,
		LeaseUntil: time.Now().Add(time.Hour)}); !errors.Is(err, ErrClaimConflict) {
		t.Errorf("second claim on an in-progress card: %v", err)
	}
	if err := s.Finish(ctx, run.ID, Outcome{Status: StatusSucceeded}); err == nil {
		t.Error("claimed -> succeeded allowed")
	}
	if err := s.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, run.ID); err == nil {
		t.Error("double start allowed")
	}
	if err := s.Renew(ctx, run.ID, time.Now().Add(2*time.Hour)); err != nil {
		t.Errorf("renew: %v", err)
	}
	if err := s.Finish(ctx, run.ID, Outcome{Status: StatusSucceeded, HeadCommit: sha, ClaudeNanoUSD: 7,
		Summary: RouteSummary{Route: "local_only", Check: "pass", LocalAttempts: 1}}); err != nil {
		t.Fatal(err)
	}
	entry := &CommitEntry{Hash: sha, Message: "zhen: ok", Date: "2026-10-23T00:00:00Z", Branch: run.Branch, Run: run.ID}
	if err := s.MoveCard(ctx, id("ok"), "review", 100, entry); err != nil {
		t.Fatal(err)
	}
	if err := s.Renew(ctx, run.ID, time.Now().Add(3*time.Hour)); err == nil {
		t.Error("renewing a finished run allowed")
	}
	g, err := s.Get(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if g.Status != StatusSucceeded || g.HeadCommit != sha || g.ClaudeNanoUSD != 7 || g.Summary.Check != "pass" || g.Mode != ModeOvernight {
		t.Errorf("run = %+v", g)
	}
	if err := s.Review(ctx, run.ID, StatusAccepted, "merged"); err != nil {
		t.Fatal(err)
	}
	if err := s.Review(ctx, run.ID, StatusRejected, "changed my mind"); err == nil {
		t.Error("second decision allowed")
	}
	if g, _ := s.Get(ctx, run.ID); g.Status != StatusAccepted || g.ReviewReason != "merged" || g.ReviewedAt == nil {
		t.Errorf("after review = %+v", g)
	}
	if _, err := s.Get(ctx, "ffffffffffffffff"); !errors.Is(err, ErrNoRun) {
		t.Errorf("unknown run: %v", err)
	}
	if n, _ := s.Attempts(ctx, id("ok")); n != 1 {
		t.Errorf("attempts = %d", n)
	}

	// An expired lease is abandoned.
	addCard(Card{ID: id("stale"), Title: "stale", Status: "todo", Type: "task", Owner: Owner})
	stale := Run{ID: NewRunID(), CardID: id("stale"), Mode: ModeInteractive, Family: "repo-task", Branch: "zhen/" + id("stale"),
		BaseCommit: sha, LeaseUntil: time.Now().Add(2 * time.Second)}
	if err := s.Claim(ctx, stale); err != nil {
		t.Fatal(err)
	}
	ex, err := s.ExpireLeases(ctx, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range ex {
		if r.ID == stale.ID && r.Status == StatusAbandoned {
			found = true
		}
	}
	if !found {
		t.Errorf("expired = %+v", ex)
	}
	if rs, err := s.List(ctx, 100); err != nil || len(rs) < 3 {
		t.Errorf("list = %d, %v", len(rs), err)
	}
}

func TestMemStore_Conformance(t *testing.T) {
	m := NewMemStore()
	conformance(t, m, m.AddCard, "m-")
	if c, _ := m.CardState("m-ok"); c.Status != "review" || len(c.Commits) != 1 || c.Commits[0].Run == "" {
		t.Errorf("card = %+v", c)
	}
}

func TestCanMove(t *testing.T) {
	for _, ok := range [][2]string{{"claimed", "running"}, {"running", "failed"}, {"succeeded", "rejected"}, {"claimed", "abandoned"}} {
		if !CanMove(ok[0], ok[1]) {
			t.Errorf("%v refused", ok)
		}
	}
	for _, bad := range [][2]string{{"claimed", "succeeded"}, {"failed", "running"}, {"accepted", "rejected"}, {"succeeded", "running"}, {"x", "y"}} {
		if CanMove(bad[0], bad[1]) {
			t.Errorf("%v allowed", bad)
		}
	}
}
