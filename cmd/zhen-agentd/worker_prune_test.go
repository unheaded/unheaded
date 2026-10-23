// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/zhenagent"
)

// Step 224: worktrees of runs that ended without a branch to review
// (abandoned, failed, cancelled) are kept 7 days for inspection, then
// removed; their branches stay. A succeeded run's worktree waits for
// Stevie's review.
func TestWorker_PruneOldWorktrees(t *testing.T) {
	m := zhenagent.NewMemStore()
	w, repo, agent, _ := newTestWorker(t, m, &fakeRunner{})
	now := time.Now()
	w.cfg.Now = func() time.Time { return now }
	ctx := context.Background()
	mk := func(card, status string, age time.Duration) string {
		m.AddCard(zhenagent.Card{ID: card, Title: "x", Status: "todo", Type: "task", Owner: "zhen"})
		branch := "zhen/" + card
		gitOut(t, repo, "worktree", "add", "-q", "-b", branch, filepath.Join(agent, card), "develop")
		r := zhenagent.Run{ID: zhenagent.NewRunID(), CardID: card, Mode: "overnight", Family: "repo-task", Branch: branch,
			BaseCommit: gitOut(t, repo, "rev-parse", "develop"), ClaimedAt: now.Add(-age), LeaseUntil: now.Add(time.Hour)}
		if err := m.Claim(ctx, r); err != nil {
			t.Fatal(err)
		}
		if err := m.Start(ctx, r.ID); err != nil {
			t.Fatal(err)
		}
		if err := m.Finish(ctx, r.ID, zhenagent.Outcome{Status: status}); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(agent, card)
	}
	oldAbandoned := mk("old-abandoned", zhenagent.StatusAbandoned, 8*24*time.Hour)
	oldFailed := mk("old-failed", zhenagent.StatusFailed, 8*24*time.Hour)
	newFailed := mk("new-failed", zhenagent.StatusFailed, 24*time.Hour)
	oldSucceeded := mk("old-succeeded", zhenagent.StatusSucceeded, 8*24*time.Hour)

	n, err := w.prune(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("pruned %d, want 2", n)
	}
	for p, want := range map[string]bool{oldAbandoned: false, oldFailed: false, newFailed: true, oldSucceeded: true} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", filepath.Base(p), err == nil, want)
		}
	}
	for _, b := range []string{"zhen/old-abandoned", "zhen/old-failed"} {
		if gitOut(t, repo, "branch", "--list", b) == "" {
			t.Errorf("branch %s deleted", b)
		}
	}
	// Never outside the agent root, whatever a run says.
	evil := zhenagent.Run{Branch: "zhen/x"}
	if p := w.worktreePath(evil.Branch); filepath.Dir(p) != w.cfg.AgentRoot {
		t.Errorf("worktree path %s", p)
	}
}
