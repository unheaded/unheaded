// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// fakeTokenizer: one token per byte of question then answer; answer_start
// is the question's length. Lines it is told to skip by marker are
// reported as too long.
type fakeTokenizer struct{}

func (fakeTokenizer) Tokenize(_ context.Context, in, out, ids string) (map[string]string, error) {
	b, err := os.ReadFile(in)
	if err != nil {
		return nil, err
	}
	var outB, idsB bytes.Buffer
	skipped := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r ExportRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		if strings.Contains(r.Answer, "TOO-LONG") {
			skipped[r.ID] = "too_long"
			continue
		}
		var toks []int
		for _, c := range []byte(r.Question + r.Answer) {
			toks = append(toks, int(c))
		}
		line, _ := json.Marshal(map[string]any{"tokens": toks, "answer_start": len(r.Question)})
		outB.Write(append(line, '\n'))
		idsB.WriteString(r.ID + "\n")
	}
	if err := os.WriteFile(out, outB.Bytes(), 0o600); err != nil {
		return nil, err
	}
	return skipped, os.WriteFile(ids, idsB.Bytes(), 0o600)
}

type fx struct {
	t  *testing.T
	st *memStore
	n  int
}

// capture adds a capture with labels; family and conversation decide the split.
func (f *fx) capture(author, family, conv, prompt, answer string, kinds ...string) Capture {
	f.t.Helper()
	f.n++
	backend := author
	if author == AuthorHuman {
		backend = BackendNone
	}
	c := Capture{CapturedAt: t0.Add(time.Duration(f.n) * time.Minute), Source: SourceZhenClaude, Backend: backend,
		Model: author + "-model", AuthorKind: author, TaskFamily: family, ConversationID: conv, RecordID: fmt.Sprint(f.n),
		Schema: "zhen-capture/v1", RedactionVersion: 1, Prompt: prompt, Response: answer, ToolTrace: json.RawMessage("[]")}
	c.ID, _ = CaptureID(c.Model, c.Prompt, c.Response, c.ToolTrace)
	if _, err := f.st.InsertCapture(context.Background(), &c); err != nil {
		f.t.Fatal(err)
	}
	for i, k := range kinds {
		if _, err := f.st.InsertLabel(context.Background(), Label{CaptureID: c.ID, Kind: k, SourceRef: fmt.Sprintf("t:%d:%d", f.n, i)}); err != nil {
			f.t.Fatal(err)
		}
	}
	return c
}

func testRefs() *References {
	return &References{
		EvalSplit: map[string]string{"syntax-go": "holdout", "py-slug": "tune"},
		Prompts: []string{
			"How do I check for an error after a function returns in Go and what is the idiomatic pattern?",
			"Fix the data race in the counter so that go test -race passes for every package in the module",
		},
	}
}

func export(t *testing.T, st Store, includeClaude bool) (string, *Manifest) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "ds")
	ex := &Exporter{Store: st, IncludeClaudeText: includeClaude, Refs: testRefs(), Tokenizer: fakeTokenizer{}, Out: out,
		GitHead: "deadbeef", Promote: true, Now: func() time.Time { return t0 }}
	m, err := ex.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return out, m
}

// readAll returns every line of every exported file except the manifest.
func readAll(t *testing.T, dir string) map[string][]map[string]any {
	t.Helper()
	out := map[string][]map[string]any{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if line == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			out[filepath.Base(f)] = append(out[filepath.Base(f)], m)
		}
	}
	return out
}

// A family/conversation pair that lands in train under the split rule.
func trainConv(t *testing.T, family string) string {
	t.Helper()
	for i := 0; i < 1000; i++ {
		c := fmt.Sprintf("conv-%d", i)
		if splitBucket(family, c) >= HoldoutPercent {
			return c
		}
	}
	t.Fatal("no train conversation")
	return ""
}

func holdoutConv(t *testing.T, family string) string {
	t.Helper()
	for i := 0; i < 1000; i++ {
		c := fmt.Sprintf("conv-%d", i)
		if splitBucket(family, c) < HoldoutPercent {
			return c
		}
	}
	t.Fatal("no holdout conversation")
	return ""
}

func TestExport_Filters(t *testing.T) {
	f := &fx{t: t, st: newMemStore()}
	tr := trainConv(t, "repo-task")
	good := f.capture(AuthorLocal, "repo-task", tr, "rename the flag in main.go", "done: renamed -x to -y", LabelCheckPass)
	dup := f.capture(AuthorLocal, "repo-task", tr, "rename the flag in main.go", "done:   renamed -X to -y", LabelCheckPass) // normalized duplicate: the lower id is kept
	f.capture(AuthorLocal, "repo-task", tr, "explain the failure", "I don't know what causes that.", LabelJudgePass)
	f.capture(AuthorLocal, "repo-task", tr, "empty?", "", LabelCheckPass)
	f.capture(AuthorLocal, "repo-task", tr, "huge", strings.Repeat("a", 9<<10), LabelCheckPass)
	f.capture(AuthorLocal, "repo-task", tr, strings.Repeat("p", 17<<10), "short", LabelCheckPass)
	cut := f.capture(AuthorLocal, "repo-task", tr, "cut off", "half of it\n[truncated by zhencapture]", LabelCheckPass)
	f.capture(AuthorLocal, "repo-task", tr, "unjudged", "an answer nobody checked")
	f.capture(AuthorLocal, "repo-task", tr, "judged both ways", "flip flop", LabelCheckPass, LabelCheckFail)
	f.capture(AuthorLocal, "repo-task", tr, "escalated one", "weak answer", LabelJudgePass, LabelEscalated)
	red := f.capture(AuthorLocal, "repo-task", tr, "with a placeholder", "set key=[REDACTED:secret#1] then run", LabelCheckPass)
	hum := f.capture(AuthorHuman, "runbook-draft", trainConv(t, "runbook-draft"), "write the runbook", "steps: 1. 2. 3.")
	f.st.captures[cut.ID] = func() Capture { c := f.st.captures[cut.ID]; c.Truncated = false; return c }() // only the marker says so

	dir, m := export(t, f.st, false)
	all := readAll(t, dir)
	train := all["sft-train.jsonl"]
	ids := map[string]bool{}
	for _, r := range train {
		ids[r["id"].(string)] = true
	}
	if len(train) != 3 || ids[good.ID] == ids[dup.ID] || !ids[hum.ID] || !ids[red.ID] || !ids[min(good.ID, dup.ID)] {
		t.Fatalf("sft-train = %v", train)
	}
	for reason, want := range map[string]int{"duplicate": 1, "refusal": 1, "answer_length": 2, "prompt_too_long": 1,
		"truncated": 1, "no_passing_outcome": 1, "failing_outcome": 2} {
		if m.Excluded[reason] != want {
			t.Errorf("excluded[%s] = %d, want %d (all: %v)", reason, m.Excluded[reason], want, m.Excluded)
		}
	}
	if m.RedactionPlaceholders != 1 {
		t.Errorf("placeholders = %d", m.RedactionPlaceholders)
	}
	r := train[0]
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "answer,author_kind,context,family,id,question,source,split" {
		t.Errorf("record keys = %v", keys)
	}
}

// The Claude-text switch, checked on the exported files (every file, the
// manifest included), both ways, so the flag is proven to bite.
func TestExport_ClaudeTextOffUnlessFlagged(t *testing.T) {
	f := &fx{t: t, st: newMemStore()}
	tr := trainConv(t, "repo-task")
	f.capture(AuthorLocal, "repo-task", tr, "fix the bug", "local fix", LabelCheckPass)
	f.capture(AuthorClaude, "repo-task", tr, "fix the other bug", "CLAUDE-AUTHORED-TEXT one", LabelCheckPass)
	// A preference candidate: Claude would be "chosen" against a failing local answer.
	f.capture(AuthorClaude, "repo-task", tr, "same task", "CLAUDE-AUTHORED-TEXT two", LabelJudgePass)
	f.capture(AuthorLocal, "repo-task", tr, "same task", "wrong local answer", LabelJudgeFail)

	off, m := export(t, f.st, false)
	if m.IncludeClaudeText || m.ByAuthorKind[AuthorClaude] != 0 {
		t.Errorf("manifest: flag %v, claude count %d", m.IncludeClaudeText, m.ByAuthorKind[AuthorClaude])
	}
	err := filepath.WalkDir(off, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p)
		if bytes.Contains(b, []byte("CLAUDE-AUTHORED-TEXT")) || bytes.Contains(b, []byte(`"author_kind":"claude"`)) {
			t.Errorf("flag off: Claude text in %s", filepath.Base(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(readAll(t, off)["preference-train.jsonl"]); n != 0 {
		t.Errorf("flag off: %d preference pairs (the only chosen side is Claude's)", n)
	}

	on, m := export(t, f.st, true)
	all := readAll(t, on)
	claude := 0
	for _, r := range all["sft-train.jsonl"] {
		if r["author_kind"] == AuthorClaude {
			claude++
		}
	}
	if claude != 2 || !m.IncludeClaudeText {
		t.Errorf("flag on: %d Claude SFT rows, want 2", claude)
	}
	pref := all["preference-train.jsonl"]
	if len(pref) != 1 || pref[0]["chosen"] != "CLAUDE-AUTHORED-TEXT two" || pref[0]["rejected"] != "wrong local answer" {
		t.Errorf("flag on: preference = %v", pref)
	}
}

func TestExport_FlagFromEnv(t *testing.T) {
	t.Setenv("ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT", "")
	if includeClaudeDefault() {
		t.Error("unset env turned the flag on")
	}
	t.Setenv("ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT", "1")
	if !includeClaudeDefault() {
		t.Error("ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT=1 ignored")
	}
	t.Setenv("ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT", "yes")
	if includeClaudeDefault() {
		t.Error("only 1 turns it on")
	}
}

func TestSplit_GroupedDeterministicAndGuarded(t *testing.T) {
	f := &fx{t: t, st: newMemStore()}
	// Many groups in one family: about 20% hold-out, every example of a
	// group in the same split.
	for g := 0; g < 200; g++ {
		conv := fmt.Sprintf("conversation-%03d", g)
		f.capture(AuthorLocal, "repo-task", conv, fmt.Sprintf("task number %d part one of the work", g), fmt.Sprintf("answer %d a", g), LabelCheckPass)
		f.capture(AuthorLocal, "repo-task", conv, fmt.Sprintf("task number %d part two of the work", g), fmt.Sprintf("answer %d b", g), LabelCheckPass)
	}
	// Eval tasks: hold-out ones stay hold-out, tune ones are excluded.
	f.capture(AuthorLocal, "syntax", "eval:syntax-go", "Language: go\n\nHow do I check for an error?", "if err != nil {}", LabelJudgePass)
	f.capture(AuthorLocal, "repo-task", "eval:py-slug", "make slug() pass", "patched", LabelJudgePass)
	// Contamination: a train prompt that is a coding-gate prompt.
	tr := trainConv(t, "review")
	f.capture(AuthorLocal, "review", tr, "Quick one: how do I check for an error after a function returns in Go and what is the idiomatic pattern?", "check err", LabelCheckPass)
	// Contamination with the flywheel's own hold-out: the same question
	// asked again in a train conversation.
	ho := holdoutConv(t, "review")
	f.capture(AuthorLocal, "review", ho, "what does the deferred close in the file reader do on error paths", "it closes", LabelCheckPass)
	f.capture(AuthorLocal, "review", tr, "what does the deferred close in the file reader do on error paths?", "it closes the file", LabelCheckPass)

	dir, m := export(t, f.st, false)
	all := readAll(t, dir)
	splitOf := map[string]string{} // conversation -> split
	for _, file := range []string{"sft-train.jsonl", "sft-holdout.jsonl"} {
		for _, r := range all[file] {
			c := f.st.captures[r["id"].(string)]
			if s, ok := splitOf[c.ConversationID]; ok && s != r["split"] {
				t.Errorf("group %s split across %s and %s", c.ConversationID, s, r["split"])
			}
			splitOf[c.ConversationID] = r["split"].(string)
			if (file == "sft-train.jsonl") != (r["split"] == "train") {
				t.Errorf("%s holds a %s row", file, r["split"])
			}
		}
	}
	ho20 := 0
	for conv, s := range splitOf {
		if strings.HasPrefix(conv, "conversation-") && s == "holdout" {
			ho20++
		}
	}
	if ho20 < 25 || ho20 > 55 { // 20% of 200, wide bounds
		t.Errorf("hold-out groups = %d of 200", ho20)
	}
	if splitOf["eval:syntax-go"] != "holdout" {
		t.Errorf("a two-brains hold-out task is in %q", splitOf["eval:syntax-go"])
	}
	if _, ok := splitOf["eval:py-slug"]; ok || m.Excluded["eval_task_not_holdout"] != 1 {
		t.Errorf("a tune eval task was exported (%v)", m.Excluded)
	}
	if m.Excluded["contamination_eval"] != 1 || m.Excluded["contamination_holdout"] != 1 {
		t.Errorf("contamination = %v", m.Excluded)
	}
	// Disjoint group keys, as recorded in the manifest.
	seen := map[string]bool{}
	for _, g := range m.GroupKeys["holdout"] {
		seen[g] = true
	}
	for _, g := range m.GroupKeys["train"] {
		if seen[g] {
			t.Errorf("group %s in both splits", g)
		}
	}

	// Deterministic: the same Well gives byte-identical files.
	dir2, _ := export(t, f.st, false)
	for _, name := range []string{"sft-train.jsonl", "sft-holdout.jsonl", "train.tok.jsonl", "holdout.tok.jsonl",
		"train.tok.ids", "holdout.tok.ids", "MANIFEST.json"} {
		a, _ := os.ReadFile(filepath.Join(dir, name))
		b, _ := os.ReadFile(filepath.Join(dir2, name))
		if !bytes.Equal(a, b) || len(a) == 0 {
			t.Errorf("%s differs between two exports of the same data (or is empty)", name)
		}
	}
}

func TestOverlap(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"How do I trim whitespace in bash?", "Language: bash\n\nHow do I trim whitespace in bash?", true},
		{"how do i trim whitespace in bash", "How do I parse a string to an integer in Rust?", false},
		{"fix it", "fix it", true},
		{"fix it", "please fix it now", false}, // under 4 words: exact match only
		{"the quick brown fox jumps over the lazy dog today", "the quick brown fox jumps over the lazy cat today", false},
	} {
		if got := overlap(tc.a, tc.b) >= ContaminationThreshold; got != tc.want {
			t.Errorf("overlap(%q, %q) = %.2f", tc.a, tc.b, overlap(tc.a, tc.b))
		}
	}
}

func TestExport_FilesPrivateAndTokenized(t *testing.T) {
	f := &fx{t: t, st: newMemStore()}
	tr := trainConv(t, "repo-task")
	f.capture(AuthorLocal, "repo-task", tr, "q one", "a one", LabelCheckPass)
	long := f.capture(AuthorLocal, "repo-task", tr, "q two", "a two TOO-LONG", LabelCheckPass)
	dir, m := export(t, f.st, false)
	di, _ := os.Stat(dir)
	if di.Mode().Perm() != 0o700 {
		t.Errorf("export dir mode %o", di.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		fi, _ := e.Info()
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %o", e.Name(), fi.Mode().Perm())
		}
		if _, ok := m.Files[e.Name()]; !ok && e.Name() != "MANIFEST.json" {
			t.Errorf("%s not in the manifest", e.Name())
		}
	}
	ids, _ := os.ReadFile(filepath.Join(dir, "train.tok.ids"))
	if strings.Contains(string(ids), long.ID) || m.Excluded["tokenize_too_long"] != 1 {
		t.Errorf("the too-long example: ids %q, excluded %v", ids, m.Excluded)
	}
	if m.Counts["sft"]["train"] != 2 || m.Counts["tokenized"]["train"] != 1 {
		t.Errorf("counts = %v", m.Counts)
	}
	// Exports are never overwritten.
	ex := &Exporter{Store: f.st, Refs: testRefs(), Tokenizer: fakeTokenizer{}, Out: dir, Now: func() time.Time { return t0 }}
	if _, err := ex.Run(context.Background()); err == nil {
		t.Error("export into a non-empty directory")
	}
	// Promoted in the Well: every exported capture.
	for _, id := range m.IDs["sft-train.jsonl"] {
		if !f.st.captures[id].Promoted {
			t.Errorf("%s exported but not promoted", id[:8])
		}
	}
}

func TestExport_RefusesRepoDir(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	ex := &Exporter{Store: newMemStore(), Refs: testRefs(), Tokenizer: fakeTokenizer{}, Out: filepath.Join(repo, "ds"), Now: time.Now}
	if _, err := ex.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "git work tree") {
		t.Errorf("export inside a repository: %v", err)
	}
}
