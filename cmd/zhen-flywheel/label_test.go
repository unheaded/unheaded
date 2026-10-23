// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// put stores a capture built from the fields that matter to labelling.
func put(t *testing.T, st *memStore, author, conv, run, model, prompt, response string, at time.Time) Capture {
	t.Helper()
	backend := author
	if author == AuthorHuman {
		backend = BackendNone
	}
	c := Capture{CapturedAt: at, Source: SourceZhenClaude, Backend: backend, Model: model, AuthorKind: author,
		TaskFamily: "repo-task", ConversationID: conv, RunID: run, RecordID: "rec", Schema: "zhen-capture/v1",
		RedactionVersion: 1, Prompt: prompt, Response: response, ToolTrace: json.RawMessage("[]")}
	var err error
	if c.ID, err = CaptureID(c.Model, c.Prompt, c.Response, c.ToolTrace); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertCapture(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func writeResults(t *testing.T, rows ...map[string]any) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "results-2026-10-23-test.jsonl")
	var sb strings.Builder
	for _, r := range rows {
		b, _ := json.Marshal(r)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLabel_FromEveryOutcome(t *testing.T) {
	st := newMemStore()
	// A conversation with two local drafts; checks after each.
	d1 := put(t, st, AuthorLocal, "sess-1", "", "qwen", "task", "draft 1", t0)
	d2 := put(t, st, AuthorLocal, "sess-1", "", "qwen", "task", "draft 2", t0.Add(2*time.Minute))
	st.checks = []CheckAction{
		{ID: 7, SessionID: "sess-1", PlannedAt: t0.Add(time.Minute), CompletedAt: t0.Add(time.Minute), Status: "completed", Summary: "exit=1 pass=false"},
		{ID: 8, SessionID: "sess-1", PlannedAt: t0.Add(3 * time.Minute), CompletedAt: t0.Add(3 * time.Minute), Status: "completed", Summary: "exit=0 pass=true"},
		{ID: 9, SessionID: "nobody", CompletedAt: t0, Status: "completed", Summary: "exit=0 pass=true"},
	}
	// An escalated conversation: local and Claude captures.
	el := put(t, st, AuthorLocal, "conv-esc", "", "qwen", "q", "weak", t0)
	ec := put(t, st, AuthorClaude, "conv-esc", "", "claude-opus-5-5", "q", "strong", t0)
	st.escalations = []Escalation{{FirstID: 41, ConversationID: "conv-esc", RouteReason: "escalated_check_fail"}}
	// Two reviewed agent runs.
	ra := put(t, st, AuthorLocal, "run-a-conv", "aaaaaaaaaaaaaaaa", "qwen", "card a", "work a", t0)
	rb := put(t, st, AuthorLocal, "run-b-conv", "bbbbbbbbbbbbbbbb", "qwen", "card b", "work b", t0)
	st.runs = []AgentRun{{ID: "aaaaaaaaaaaaaaaa", Status: "accepted"}, {ID: "bbbbbbbbbbbbbbbb", Status: "rejected", ReviewReason: "wrong file"}}
	// Eval answers, judged in a results file by answer hash.
	ej := put(t, st, AuthorLocal, "eval:syntax-go", "", "qwen-7b", "Language: go\n\nhow?", "if err != nil {}", t0)
	ef := put(t, st, AuthorLocal, "eval:review-go", "", "qwen-7b", "Language: go\n\nreview", "looks fine", t0)
	other := put(t, st, AuthorLocal, "eval:review-go", "", "qwen-14b", "Language: go\n\nreview", "different answer", t0)
	res := writeResults(t,
		map[string]any{"task": "syntax-go", "family": "syntax", "backend": "local", "judge": "key", "pass": true, "detail": "key matched", "answer_sha256": sha256Hex("if err != nil {}")},
		map[string]any{"task": "review-go", "family": "review", "backend": "local", "judge": "key", "pass": false, "detail": "missed: ignored", "answer_sha256": sha256Hex("looks fine")},
		map[string]any{"task": "review-go", "family": "review", "backend": "claude", "judge": "key", "pass": true, "answer_sha256": sha256Hex("looks fine")},
	)

	lb := &Labeler{Store: st, ResultsFiles: []string{res}}
	rep, err := lb.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]int{
		d1.ID:    {LabelCheckFail: 1},
		d2.ID:    {LabelCheckPass: 1},
		el.ID:    {LabelEscalated: 1},
		ec.ID:    {},
		ra.ID:    {LabelAccepted: 1},
		rb.ID:    {LabelRejected: 1},
		ej.ID:    {LabelJudgePass: 1},
		ef.ID:    {LabelJudgeFail: 1},
		other.ID: {},
	}
	for id, w := range want {
		got := st.labelsOf(id)
		if len(got) != len(w) {
			t.Errorf("%s: labels %v, want %v", id[:8], got, w)
			continue
		}
		for k, n := range w {
			if got[k] != n {
				t.Errorf("%s: labels %v, want %v", id[:8], got, w)
			}
		}
	}
	if rep.Unmatched["zhen_actions"] != 1 || rep.Unmatched["eval"] != 1 {
		t.Errorf("unmatched = %v (the check of an unknown session; the claude row with no claude capture)", rep.Unmatched)
	}

	// Idempotent: a second run adds nothing.
	before := len(st.labels)
	rep2, err := lb.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.labels) != before || rep2.Inserted != 0 {
		t.Errorf("second run added %d labels (report %d)", len(st.labels)-before, rep2.Inserted)
	}
}

func TestLabel_SourcesUnavailableAreReportedNotFatal(t *testing.T) {
	st := newMemStore()
	put(t, st, AuthorLocal, "c", "", "qwen", "p", "r", t0)
	st.noOps, st.noRuns = true, true
	rep, err := (&Labeler{Store: st}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 2 {
		t.Errorf("skipped = %q", rep.Skipped)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=develop"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// An accepted run whose merge carries Stevie's edit becomes a human
// capture holding the merged text, and human_edit on the agent's.
func TestLabel_HumanEditFromMerge(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package a\n")
	write("b.go", "package b\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "base")
	base := git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "-c", "zhen/card-1")
	write("a.go", "package a\n\nfunc A() int { return 1 }\n")
	write("b.go", "package b\n\nfunc B() {}\n")
	git(t, repo, "commit", "-q", "-am", "agent")
	head := git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "develop")
	git(t, repo, "merge", "-q", "--no-ff", "--no-commit", "zhen/card-1")
	write("a.go", "package a\n\n// A is one.\nfunc A() int { return 1 }\n") // Stevie's edit
	git(t, repo, "commit", "-q", "-am", "merge zhen/card-1")

	st := newMemStore()
	agent := put(t, st, AuthorLocal, "run-conv", "cccccccccccccccc", "qwen", "card 1: add A", "the agent's patch", t0)
	st.runs = []AgentRun{{ID: "cccccccccccccccc", Status: "accepted", BaseCommit: base, HeadCommit: head}}
	lb := &Labeler{Store: st, Repo: repo, MergedInto: "develop", Scanner: testScanner}
	rep, err := lb.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.HumanCaptures != 1 {
		t.Fatalf("report = %+v", rep)
	}
	var human Capture
	for _, c := range st.captures {
		if c.AuthorKind == AuthorHuman {
			human = c
		}
	}
	if human.Backend != BackendNone || human.RunID != "cccccccccccccccc" || human.Prompt != "card 1: add A" ||
		!strings.Contains(human.Response, "// A is one.") || strings.Contains(human.Response, "package b") || human.Source != SourceAgentd {
		t.Errorf("human capture = %+v", human)
	}
	if got := st.labelsOf(agent.ID); got[LabelHumanEdit] != 1 || got[LabelAccepted] != 1 {
		t.Errorf("agent capture labels = %v", got)
	}
	var v map[string]any
	for _, l := range st.labels {
		if l.Kind == LabelHumanEdit {
			_ = json.Unmarshal(l.Value, &v)
		}
	}
	if v["human_capture"] != human.ID {
		t.Errorf("human_edit value = %v", v)
	}
	// Again: nothing new.
	if rep, _ := lb.Run(context.Background()); rep.HumanCaptures != 0 || rep.Inserted != 0 {
		t.Errorf("second run: %+v", rep)
	}
}
