// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/zhenagent"
	"unheaded/pkg/zhenroute"
)

// testRepo is a main checkout with a develop branch holding a tiny
// package, plus an agent root outside it.
func testRepo(t *testing.T) (repo, agent string) {
	t.Helper()
	base := t.TempDir()
	repo, agent = filepath.Join(base, "repo"), filepath.Join(base, "agents")
	for _, d := range []string{repo, agent} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(repo, "pkg", "ex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "pkg", "ex", "ex.go"), []byte("package ex\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "develop"}, {"config", "user.email", "stevie@example.invalid"}, {"config", "user.name", "Stevie Test"},
		{"config", "commit.gpgsign", "false"}, {"add", "."}, {"commit", "-q", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
	return repo, agent
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

const goodDesc = "Add a Double function.\n\n```zhen\nfamily: repo-task\ncheck: go-test\npath: ./pkg/ex/...\n```\n"

// fakeRunner writes a file into the worktree (or not) and reports an
// outcome; it records every task and the highest concurrency it saw.
type fakeRunner struct {
	mu      sync.Mutex
	tasks   []zhenroute.Task
	write   bool
	out     claudebridge.RouteOutcome
	err     error
	delay   time.Duration
	active  atomic.Int32
	maxSeen atomic.Int32
	hook    func()
	// artefacts: leave tool files in a hidden top-level directory.
	artefacts bool
}

func (f *fakeRunner) Run(ctx context.Context, t zhenroute.Task) (claudebridge.RouteOutcome, error) {
	n := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		m := f.maxSeen.Load()
		if n <= m || f.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}
	f.mu.Lock()
	f.tasks = append(f.tasks, t)
	f.mu.Unlock()
	if f.hook != nil {
		f.hook()
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return claudebridge.RouteOutcome{}, ctx.Err()
		}
	}
	if f.write {
		if err := os.WriteFile(filepath.Join(t.Worktree, "pkg", "ex", "double.go"), []byte("package ex\n\nfunc Double(x int) int { return 2 * x }\n"), 0o600); err != nil {
			return claudebridge.RouteOutcome{}, err
		}
	}
	if f.artefacts {
		// What a check's tools leave behind (Go telemetry under $HOME).
		d := filepath.Join(t.Worktree, ".config", "go", "telemetry")
		if err := os.MkdirAll(d, 0o700); err != nil {
			return claudebridge.RouteOutcome{}, err
		}
		if err := os.WriteFile(filepath.Join(d, "weekends"), []byte("x\n"), 0o600); err != nil {
			return claudebridge.RouteOutcome{}, err
		}
	}
	return f.out, f.err
}

func TestWorker_ToolArtefactsNotCommitted(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{write: true, artefacts: true, out: passOutcome()}
	w, _, agent, _ := newTestWorker(t, m, r)
	res, _ := w.runOnce(context.Background())
	if res.Outcome != outcomeSucceeded {
		t.Fatalf("result = %+v", res)
	}
	files := gitOut(t, filepath.Join(agent, "good"), "show", "--name-only", "--format=", "HEAD")
	if files != "pkg/ex/double.go" {
		t.Errorf("committed %q", files)
	}
	note, _ := os.ReadFile(res.ReviewNote)
	if !strings.Contains(string(note), ".config/go/telemetry/weekends") || !strings.Contains(string(note), "not committed") {
		t.Errorf("note does not list the artefact:\n%s", note)
	}
	if strings.Contains(string(note), "$$") {
		t.Errorf("note has $$:\n%s", note)
	}
}

func TestWorker_OnlyArtefactsIsNoChange(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{artefacts: true, out: passOutcome()}
	w, _, _, _ := newTestWorker(t, m, r)
	if res, _ := w.runOnce(context.Background()); res.Outcome != outcomeFailed || !strings.Contains(res.Reason, "no change") {
		t.Errorf("result = %+v", res)
	}
}

func passOutcome() claudebridge.RouteOutcome {
	return claudebridge.RouteOutcome{Family: "repo-task", Route: "local_only", RouteReason: "local_only", LocalAttempts: 1, Check: "pass"}
}

// allCandidates hands the worker every card, so its own eligibility check
// is what is tested (not the store's query).
type allCandidates struct{ *zhenagent.MemStore }

func (a allCandidates) Candidates(_ context.Context, _ int) ([]zhenagent.Card, error) {
	var out []zhenagent.Card
	for _, id := range []string{"theirs", "archived", "noblock", "good"} {
		if c, ok := a.CardState(id); ok {
			out = append(out, c.Card)
		}
	}
	return out, nil
}

type gitLog struct {
	mu   sync.Mutex
	argv [][]string
}

func (g *gitLog) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	g.mu.Lock()
	g.argv = append(g.argv, append([]string{dir}, args...))
	g.mu.Unlock()
	return execGit(ctx, dir, args...)
}

func newTestWorker(t *testing.T, store zhenagent.Store, r taskRunner) (*worker, string, string, *gitLog) {
	t.Helper()
	repo, agent := testRepo(t)
	gl := &gitLog{}
	w, err := newWorker(workerConfig{
		Store: store, Runner: r, Repo: repo, AgentRoot: agent, Base: "develop", Mode: zhenagent.ModeInteractive,
		KnownCheck: func(k string) bool { return k == "go-test" }, ReviewDir: filepath.Join(t.TempDir(), "reviews"),
		Lease: time.Minute, Renew: time.Second, Git: gl.run,
	})
	if err != nil {
		t.Fatal(err)
	}
	return w, repo, agent, gl
}

func TestWorker_EligibleCardToBranch(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "Add Double", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{write: true, out: passOutcome()}
	w, repo, agent, gl := newTestWorker(t, m, r)

	res, err := w.runOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != outcomeSucceeded || res.Branch != "zhen/good" {
		t.Fatalf("result = %+v", res)
	}
	wt := filepath.Join(agent, "good")
	if got := gitOut(t, wt, "symbolic-ref", "--short", "HEAD"); got != "zhen/good" {
		t.Errorf("worktree branch = %q", got)
	}
	if base := gitOut(t, repo, "rev-parse", "develop"); gitOut(t, wt, "rev-parse", "HEAD~1") != base {
		t.Error("the branch does not start at develop")
	}
	author := gitOut(t, wt, "log", "-1", "--format=%an <%ae>|%cn <%ce>")
	if author != "Zhenai <zhenai@unheaded.invalid>|Stevie Test <stevie@example.invalid>" {
		t.Errorf("author|committer = %q", author)
	}
	msg := gitOut(t, wt, "log", "-1", "--format=%B")
	for _, want := range []string{"Zhen-Card: good", "Zhen-Route: local_only", "Zhen-Run: " + res.Run} {
		if !strings.Contains(msg, want) {
			t.Errorf("commit message lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Co-Authored-By") {
		t.Errorf("a local-only commit names Claude:\n%s", msg)
	}
	if gitOut(t, wt, "show", "--stat", "--format=", "HEAD") == "" {
		t.Error("empty commit")
	}
	c, _ := m.CardState("good")
	if c.Status != "review" || len(c.Commits) != 1 || c.Commits[0].Branch != "zhen/good" || c.Commits[0].Run != res.Run || c.Commits[0].ReviewNote == "" {
		t.Errorf("card = %+v", c)
	}
	run, _ := m.Get(context.Background(), res.Run)
	if run.Status != zhenagent.StatusSucceeded || run.HeadCommit != gitOut(t, wt, "rev-parse", "HEAD") || run.Summary.Check != "pass" {
		t.Errorf("run = %+v", run)
	}
	note, err := os.ReadFile(c.Commits[0].ReviewNote)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"zhen/good", "local_only", "merge --no-ff zhen/good", "double.go", "review accept " + res.Run} {
		if !strings.Contains(string(note), want) {
			t.Errorf("review note lacks %q:\n%s", want, note)
		}
	}
	// The task reached the runner with the card's text as delimited data.
	if len(r.tasks) != 1 || r.tasks[0].Check != "go-test" || r.tasks[0].CheckPath != "./pkg/ex/..." || r.tasks[0].Family != "repo-task" {
		t.Fatalf("tasks = %+v", r.tasks)
	}
	p := r.tasks[0].Prompt
	if !strings.Contains(p, "<<<CARD") || !strings.Contains(p, "Add a Double function.") || strings.Contains(p, "```zhen") {
		t.Errorf("prompt = %q", p)
	}
	// Nothing anywhere pushes.
	for _, a := range gl.argv {
		for _, x := range a {
			if strings.Contains(x, "push") {
				t.Errorf("git argv %v", a)
			}
		}
	}
	if out := gitOut(t, repo, "status", "--porcelain"); out != "" {
		t.Errorf("main checkout touched: %q", out)
	}
}

func TestWorker_IneligibleSkipped(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "theirs", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "Stevie"})
	m.AddCard(zhenagent.Card{ID: "archived", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen", Archived: true})
	m.AddCard(zhenagent.Card{ID: "noblock", Title: "x", Description: "just do it", Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{write: true, out: passOutcome()}
	w, _, agent, _ := newTestWorker(t, allCandidates{m}, r)
	res, err := w.runOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != outcomeNone || len(r.tasks) != 0 {
		t.Fatalf("result = %+v, tasks %d", res, len(r.tasks))
	}
	reasons := map[string]string{}
	for _, s := range res.Skipped {
		reasons[s.Card] = s.Reason
	}
	if !strings.Contains(reasons["theirs"], "owner") || !strings.Contains(reasons["archived"], "archived") || !strings.Contains(reasons["noblock"], "zhen") {
		t.Errorf("skips = %v", reasons)
	}
	for _, id := range []string{"theirs", "archived", "noblock"} {
		if c, _ := m.CardState(id); c.Status != "todo" {
			t.Errorf("%s moved to %s", id, c.Status)
		}
		if _, err := os.Stat(filepath.Join(agent, id)); err == nil {
			t.Errorf("worktree made for %s", id)
		}
	}
}

// conflictStore loses every claim, as if another worker got there first.
type conflictStore struct{ *zhenagent.MemStore }

func (conflictStore) Claim(context.Context, zhenagent.Run) error { return zhenagent.ErrClaimConflict }

func TestWorker_ClaimConflictSkipped(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{write: true, out: passOutcome()}
	w, _, _, _ := newTestWorker(t, conflictStore{m}, r)
	res, err := w.runOnce(context.Background())
	if err != nil || res.Outcome != outcomeNone || len(r.tasks) != 0 {
		t.Fatalf("res %+v err %v tasks %d", res, err, len(r.tasks))
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "claimed") {
		t.Errorf("skips = %+v", res.Skipped)
	}
}

func TestWorker_CheckFailsCardBackToTodo(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	out := passOutcome()
	out.Check, out.Stop = "fail", "local_only"
	r := &fakeRunner{write: true, out: out}
	w, _, agent, _ := newTestWorker(t, m, r)
	res, err := w.runOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != outcomeFailed || res.Infra {
		t.Fatalf("result = %+v", res)
	}
	run, _ := m.Get(context.Background(), res.Run)
	if run.Status != zhenagent.StatusFailed || !strings.Contains(run.Error, "check") || run.HeadCommit != "" {
		t.Errorf("run = %+v", run)
	}
	if c, _ := m.CardState("good"); c.Status != "todo" {
		t.Errorf("card = %s", c.Status)
	}
	wt := filepath.Join(agent, "good")
	if _, err := os.Stat(filepath.Join(wt, "pkg", "ex", "double.go")); err != nil {
		t.Error("worktree not kept for inspection")
	}
	if gitOut(t, wt, "rev-list", "--count", "develop..HEAD") != "0" {
		t.Error("a failed run committed")
	}
	// A second attempt gets its own branch; a third is not made.
	res2, _ := w.runOnce(context.Background())
	if res2.Branch != "zhen/good-2" || res2.Outcome != outcomeFailed {
		t.Errorf("second attempt = %+v", res2)
	}
	res3, _ := w.runOnce(context.Background())
	if res3.Outcome != outcomeNone {
		t.Errorf("third attempt = %+v", res3)
	}
}

func TestWorker_InfraFailure(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{err: errors.New("sandbox: systemd-run missing")}
	w, _, _, _ := newTestWorker(t, m, r)
	res, err := w.runOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != outcomeFailed || !res.Infra {
		t.Errorf("result = %+v", res)
	}
}

func TestWorker_NoChangesFails(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{out: passOutcome()}
	w, _, _, _ := newTestWorker(t, m, r)
	res, _ := w.runOnce(context.Background())
	if res.Outcome != outcomeFailed || !strings.Contains(res.Reason, "no change") {
		t.Errorf("result = %+v", res)
	}
}

func TestWorker_ClaudeCoAuthorTrailer(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	out := passOutcome()
	out.RouteReason, out.Escalated, out.Cost = "escalated_check_fail", true, 12_000_000
	r := &fakeRunner{write: true, out: out}
	w, _, agent, _ := newTestWorker(t, m, r)
	res, _ := w.runOnce(context.Background())
	if res.Outcome != outcomeSucceeded {
		t.Fatalf("result = %+v", res)
	}
	msg := gitOut(t, filepath.Join(agent, "good"), "log", "-1", "--format=%B")
	if !strings.Contains(msg, "Co-Authored-By: Claude") || !strings.Contains(msg, "Zhen-Route: local_only/escalated_check_fail") {
		t.Errorf("message:\n%s", msg)
	}
	if run, _ := m.Get(context.Background(), res.Run); run.ClaudeNanoUSD != 12_000_000 {
		t.Errorf("claude cost = %d", run.ClaudeNanoUSD)
	}
}

func TestWorker_LeaseRenewed(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{write: true, out: passOutcome(), delay: 120 * time.Millisecond}
	w, _, _, _ := newTestWorker(t, m, r)
	w.cfg.Renew = 20 * time.Millisecond
	var first time.Time
	r.hook = func() {
		runs, _ := m.List(context.Background(), 1)
		first = runs[0].LeaseUntil
	}
	res, _ := w.runOnce(context.Background())
	run, _ := m.Get(context.Background(), res.Run)
	if !run.LeaseUntil.After(first) {
		t.Errorf("lease not renewed: %v then %v", first, run.LeaseUntil)
	}
}

func TestWorker_WIPLimitOne(t *testing.T) {
	m := zhenagent.NewMemStore()
	for _, id := range []string{"good", "good2", "good3"} {
		m.AddCard(zhenagent.Card{ID: id, Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	}
	r := &fakeRunner{write: true, out: passOutcome(), delay: 50 * time.Millisecond}
	w, _, _, _ := newTestWorker(t, m, r)
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = w.runOnce(context.Background()) }()
	}
	wg.Wait()
	if r.maxSeen.Load() != 1 {
		t.Errorf("cards in flight at once = %d", r.maxSeen.Load())
	}
	if len(r.tasks) != 3 {
		t.Errorf("one card per runOnce: %d tasks", len(r.tasks))
	}
}

func TestWorker_StopMidCardCommitsNothing(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	var stop atomic.Bool
	r := &fakeRunner{write: true, out: passOutcome(), hook: func() { stop.Store(true) }}
	w, repo, agent, _ := newTestWorker(t, m, r)
	w.cfg.Stop = stop.Load
	res, _ := w.runOnce(context.Background())
	if res.Outcome != outcomeCancelled {
		t.Fatalf("result = %+v", res)
	}
	// Nothing was committed, so the worktree and the branch are gone.
	if _, err := os.Stat(filepath.Join(agent, "good")); err == nil {
		t.Error("a stopped run's worktree was kept")
	}
	if gitOut(t, repo, "branch", "--list", "zhen/good") != "" {
		t.Error("a stopped run's branch was kept")
	}
	if run, _ := m.Get(context.Background(), res.Run); run.Status != zhenagent.StatusCancelled {
		t.Errorf("run = %s", run.Status)
	}
	if c, _ := m.CardState("good"); c.Status != "todo" {
		t.Errorf("card = %s", c.Status)
	}
}

func TestWorker_RefusesAgentRootInsideRepo(t *testing.T) {
	repo, _ := testRepo(t)
	_, err := newWorker(workerConfig{Store: zhenagent.NewMemStore(), Runner: &fakeRunner{}, Repo: repo, AgentRoot: filepath.Join(repo, "agents"),
		Base: "develop", KnownCheck: func(string) bool { return true }, ReviewDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "inside") {
		t.Errorf("err = %v", err)
	}
}

func TestWorker_BadCardIDSkipped(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "Bad.Id", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{write: true, out: passOutcome()}
	w, _, _, _ := newTestWorker(t, m, r)
	res, _ := w.runOnce(context.Background())
	if res.Outcome != outcomeNone || len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "branch") {
		t.Errorf("result = %+v", res)
	}
}
