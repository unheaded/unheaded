// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/zhenagent"
)

// reviewFixture: a repo with develop, an agent worktree on zhen/card-1
// holding one commit, and a MemStore with that run succeeded.
func reviewFixture(t *testing.T) (repo, wt string, m *zhenagent.MemStore, run zhenagent.Run, events *[]*audit.AuditEvent) {
	t.Helper()
	base := t.TempDir()
	repo, wt = filepath.Join(base, "repo"), filepath.Join(base, "agents", "card-1")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	g := func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	g(repo, "init", "-q", "-b", "develop")
	g(repo, "config", "user.email", "s@example.invalid")
	g(repo, "config", "user.name", "S")
	g(repo, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g(repo, "add", ".")
	g(repo, "commit", "-q", "-m", "base")
	baseSHA := g(repo, "rev-parse", "HEAD")
	g(repo, "worktree", "add", "-q", "-b", "zhen/card-1", wt, "develop")
	if err := os.WriteFile(filepath.Join(wt, "b.txt"), []byte("b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g(wt, "add", ".")
	g(wt, "commit", "-q", "-m", "zhen: b")
	head := g(wt, "rev-parse", "HEAD")

	m = zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "card-1", Title: "b", Status: "todo", Type: "task", Owner: "zhen"})
	run = zhenagent.Run{ID: zhenagent.NewRunID(), CardID: "card-1", Mode: "interactive", Family: "repo-task", Branch: "zhen/card-1",
		BaseCommit: baseSHA, LeaseUntil: time.Now().Add(time.Hour)}
	ctx := context.Background()
	if err := m.Claim(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Finish(ctx, run.ID, zhenagent.Outcome{Status: zhenagent.StatusSucceeded, HeadCommit: head}); err != nil {
		t.Fatal(err)
	}
	if err := m.MoveCard(ctx, "card-1", "review", 90, nil); err != nil {
		t.Fatal(err)
	}
	run.HeadCommit = head

	prevS, prevA := openReviewStore, openReviewAudit
	t.Cleanup(func() { openReviewStore, openReviewAudit = prevS, prevA })
	openReviewStore = func(context.Context) (zhenagent.Store, func(), error) { return m, func() {}, nil }
	var evs []*audit.AuditEvent
	openReviewAudit = func(context.Context) (func(*audit.AuditEvent) error, func(), error) {
		return func(ev *audit.AuditEvent) error { evs = append(evs, ev); return nil }, func() {}, nil
	}
	return repo, wt, m, run, &evs
}

func review(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(append([]string{"review"}, args...), strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func TestReview_AcceptBeforeMergeRefusedWithTheCommand(t *testing.T) {
	repo, _, m, r, evs := reviewFixture(t)
	code, _, errs := review(t, "-repo", repo, "accept", r.ID)
	if code != 1 || !strings.Contains(errs, "merge --no-ff zhen/card-1") || !strings.Contains(errs, "not merged") {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if g, _ := m.Get(context.Background(), r.ID); g.Status != zhenagent.StatusSucceeded {
		t.Errorf("status = %s", g.Status)
	}
	if len(*evs) != 0 {
		t.Error("an audit row for a refused accept")
	}
}

func TestReview_AcceptAfterMerge(t *testing.T) {
	repo, _, m, r, evs := reviewFixture(t)
	if out, err := exec.Command("git", "-C", repo, "merge", "-q", "--no-ff", "--no-edit", "zhen/card-1").CombinedOutput(); err != nil {
		t.Fatalf("merge: %v %s", err, out)
	}
	code, out, errs := review(t, "-repo", repo, "accept", r.ID, "-reason", "looks right")
	if code != 0 || !strings.Contains(out, "accepted") {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	g, _ := m.Get(context.Background(), r.ID)
	if g.Status != zhenagent.StatusAccepted || g.ReviewReason != "looks right" {
		t.Errorf("run = %+v", g)
	}
	if c, _ := m.CardState("card-1"); c.Status != "done" {
		t.Errorf("card = %s", c.Status)
	}
	if len(*evs) != 1 || (*evs)[0].Actor.Type != "human" || (*evs)[0].Action != "agent_review_accept" {
		t.Errorf("audit = %+v", *evs)
	}
	// A second decision is refused.
	if code, _, _ := review(t, "-repo", repo, "reject", r.ID, "-reason", "no"); code == 0 {
		t.Error("double decision allowed")
	}
}

func TestReview_RejectNeedsReason(t *testing.T) {
	repo, _, _, r, _ := reviewFixture(t)
	if code, _, errs := review(t, "-repo", repo, "reject", r.ID); code != 2 || !strings.Contains(errs, "-reason") {
		t.Errorf("exit %d: %s", code, errs)
	}
}

func TestReview_RejectRemovesWorktreeKeepsBranch(t *testing.T) {
	repo, wt, m, r, evs := reviewFixture(t)
	code, out, errs := review(t, "-repo", repo, "reject", r.ID, "-reason", "wrong approach")
	if code != 0 {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Error("worktree not removed")
	}
	if err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", "refs/heads/zhen/card-1").Run(); err != nil {
		t.Error("branch deleted")
	}
	if g, _ := m.Get(context.Background(), r.ID); g.Status != zhenagent.StatusRejected || g.ReviewReason != "wrong approach" {
		t.Errorf("run = %+v", g)
	}
	if c, _ := m.CardState("card-1"); c.Status != "todo" {
		t.Errorf("card = %s", c.Status)
	}
	if len(*evs) != 1 || (*evs)[0].Outcome != audit.OutcomeFailure {
		t.Errorf("audit = %+v", *evs)
	}
	if code, _, _ := review(t, "-repo", repo, "reject", r.ID, "-reason", "again"); code == 0 {
		t.Error("double reject allowed")
	}
}

func TestReview_BadArgs(t *testing.T) {
	_, _, _, _, _ = reviewFixture(t)
	for _, a := range [][]string{{}, {"accept"}, {"accept", "nothex"}, {"list", "x"}, {"bogus"}, {"show", "0123456789abcdef", "-reason", "x"}} {
		if code, _, _ := review(t, a...); code != 2 {
			t.Errorf("%v: exit %d", a, code)
		}
	}
}

func TestReview_ListShow(t *testing.T) {
	_, _, _, r, _ := reviewFixture(t)
	code, out, _ := review(t, "list")
	if code != 0 || !strings.Contains(out, r.ID) || !strings.Contains(out, "zhen/card-1") {
		t.Errorf("list: %d %s", code, out)
	}
	code, out, _ = review(t, "show", r.ID)
	if code != 0 || !strings.Contains(out, "succeeded") {
		t.Errorf("show: %d %s", code, out)
	}
}
