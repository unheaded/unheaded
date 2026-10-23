// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const repoRoot = "../.."

func TestRealKeysCompileAndCoverEveryKeyTask(t *testing.T) {
	o := &options{root: repoRoot, tasksPath: "eval/two-brains/split.jsonl", keysPath: "eval/two-brains/keys.jsonl",
		promptsPath: "eval/coding-gate/prompts.jsonl", split: "all"}
	tasks, keys, prompts, err := loadAll(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 56 || len(prompts) != 56 {
		t.Fatalf("keys %d prompts %d", len(keys), len(prompts))
	}
	fam := map[string]int{}
	for _, tk := range tasks {
		fam[tk.Family]++
	}
	if fam["repo-task"] != 20 || fam["syntax"] != 7 || fam["review"] != 7 || fam["runbook-draft"] != 0 {
		t.Errorf("families = %v", fam)
	}
}

func TestJudgeKey(t *testing.T) {
	k := &Key{ID: "x", AllOf: []string{`===`}, AnyOf: []string{`coerc`, `strict`}}
	if err := k.compile(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := judgeKey(k, "Use === for STRICT equality"); !ok {
		t.Error("a matching answer failed")
	}
	if ok, d := judgeKey(k, "use == carefully"); ok || !strings.Contains(d, "all_of[0]") {
		t.Errorf("a non-matching answer passed: %s", d)
	}
	if ok, d := judgeKey(k, "use === always"); ok || !strings.Contains(d, "any_of") {
		t.Errorf("any_of not enforced: %s", d)
	}
	if ok, _ := judgeKey(k, "  "); ok {
		t.Error("an empty answer passed")
	}
	if err := (&Key{ID: "e"}).compile(); err == nil {
		t.Error("an empty key compiled")
	}
	if err := (&Key{ID: "b", AllOf: []string{`(\w+) \1`}}).compile(); err == nil {
		t.Error("a backreference (not RE2) compiled")
	}
}

type fakeRunner struct {
	calls int
	exit  func(dir string) int
	out   string
}

func (f *fakeRunner) Run(_ context.Context, dir string) (int, string, error) {
	f.calls++
	return f.exit(dir), f.out, nil
}

func fixtureDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for name, body := range map[string]string{"lib.go": "package x\n", "check.sh": "exit 1\n", "TASK.md": "fix it\n", "solution.patch": "x\n"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

// passIfFixed passes when lib.go says "fixed".
func passIfFixed(dir string) int {
	b, _ := os.ReadFile(filepath.Join(dir, "lib.go"))
	if strings.Contains(string(b), "fixed") {
		return 0
	}
	return 1
}

func TestJudgeCheck(t *testing.T) {
	fx, work := fixtureDir(t), t.TempDir()
	r := &fakeRunner{exit: passIfFixed, out: "FAIL lib_test.go:3"}
	for name, tc := range map[string]struct {
		answer string
		pass   bool
		detail string
	}{
		"fixed":           {"Here:\n```file:lib.go\npackage x // fixed\n```\n", true, "check passed"},
		"still broken":    {"```file:lib.go\npackage x\n```", false, "check_fail exit=1"},
		"no blocks":       {"I would change lib.go", false, "parse_fail"},
		"outside":         {"```file:../evil.go\nfixed\n```", false, "outside_fixture"},
		"absolute":        {"```file:/tmp/evil.go\nfixed\n```", false, "outside_fixture"},
		"edits the check": {"```file:check.sh\nexit 0\n```\n```file:lib.go\npackage x // fixed\n```", false, "outside_fixture"},
		"edits the task":  {"```file:./TASK.md\nnone\n```", false, "outside_fixture"},
	} {
		cr := judgeCheck(context.Background(), fx, work, tc.answer, r)
		if cr.Pass != tc.pass || !strings.HasPrefix(cr.Detail, tc.detail) {
			t.Errorf("%s: pass=%v detail=%q", name, cr.Pass, cr.Detail)
		}
	}
	// The fixture itself was never touched.
	if b, _ := os.ReadFile(filepath.Join(fx, "lib.go")); string(b) != "package x\n" {
		t.Error("the fixture was modified")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(fx), "evil.go")); err == nil {
		t.Error("a file was written outside the fixture copy")
	}
}

func TestApplyBlocks_SymlinkEscape(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "sub")); err != nil {
		t.Skip(err)
	}
	if err := applyBlocks(dir, []FileBlock{{Path: "sub/x.go", Content: "x"}}); err == nil {
		t.Error("a write through a symlinked directory was allowed")
	}
	if _, err := os.Stat(filepath.Join(outside, "x.go")); err == nil {
		t.Error("written outside")
	}
}

// fakeBrain answers from a script of replies and records prompts.
type fakeBrain struct {
	name    string
	replies []string
	prompts []string
}

func (b *fakeBrain) Name() string  { return b.name }
func (b *fakeBrain) Model() string { return "fake" }
func (b *fakeBrain) Ask(_ context.Context, _, prompt string, _ int) (Answer, error) {
	b.prompts = append(b.prompts, prompt)
	r := b.replies[0]
	if len(b.replies) > 1 {
		b.replies = b.replies[1:]
	}
	return Answer{Text: r, InputTokens: 10, OutputTokens: 5, Cost: 7}, nil
}

func newEval(t *testing.T, brain Brain, run CheckRunner, root string) (*evaluator, string) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "results.jsonl")
	o := &options{root: root, out: out, localAttempts: 2, keyTokens: 10, repoTokens: 10, claudeKeyTok: 10, claudeRepoTok: 10}
	k := &Key{ID: "k1", AllOf: []string{`===`}}
	_ = k.compile()
	return &evaluator{o: o, brain: brain, keys: map[string]*Key{"k1": k}, prompts: map[string]string{"k1": "q"},
		work: t.TempDir(), check: run, log: slog.New(slog.NewJSONHandler(io.Discard, nil))}, out
}

func readResults(t *testing.T, path string) []Result {
	t.Helper()
	rows, err := loadJSONL[Result](path)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestEvaluator_KeyPassAndFail(t *testing.T) {
	b := &fakeBrain{name: "local", replies: []string{"use ===", "use =="}}
	e, out := newEval(t, b, &fakeRunner{exit: passIfFixed}, t.TempDir())
	tasks := []Task{{ID: "k1", Family: "review", Judge: "key", Ref: "x#k1", Split: "tune"}, {ID: "k1b", Family: "review", Judge: "key", Ref: "x#k1", Split: "holdout"}}
	var so bytes.Buffer
	if code := e.runAll(context.Background(), tasks, &so); code != 0 {
		t.Fatalf("exit %d", code)
	}
	rows := readResults(t, out)
	if len(rows) != 2 || !rows[0].Pass || rows[1].Pass || rows[0].CostNanoUSD != 7 || rows[0].AnswerSHA256 == "" {
		t.Errorf("rows = %+v", rows)
	}
	raw, _ := os.ReadFile(out)
	if strings.Contains(string(raw), "use ===") {
		t.Error("answer text written to results")
	}
	if !strings.Contains(so.String(), "1/2 passed") {
		t.Errorf("summary = %q", so.String())
	}
}

func TestEvaluator_RepoTaskSecondLocalAttemptSeesCheckOutput(t *testing.T) {
	root := t.TempDir()
	fx := filepath.Join(root, "fx")
	if err := os.MkdirAll(fx, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"lib.go": "package x\n", "check.sh": "exit 1\n", "TASK.md": "<!-- SPDX -->\nfix lib.go\n", "solution.patch": "x"} {
		_ = os.WriteFile(filepath.Join(fx, name), []byte(body), 0o600)
	}
	b := &fakeBrain{name: "local", replies: []string{"```file:lib.go\npackage x\n```", "```file:lib.go\npackage x // fixed\n```"}}
	e, out := newEval(t, b, &fakeRunner{exit: passIfFixed, out: "FAIL: TestLib wants fixed"}, root)
	if code := e.runAll(context.Background(), []Task{{ID: "repo-fx", Family: "repo-task", Judge: "check", Ref: "fx", Split: "tune"}}, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	rows := readResults(t, out)
	if len(rows) != 1 || !rows[0].Pass || rows[0].Attempts != 2 || len(rows[0].Details) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if !strings.Contains(b.prompts[1], "FAIL: TestLib wants fixed") || strings.Contains(b.prompts[0], "solution.patch") || strings.Contains(b.prompts[0], "check.sh\n") {
		t.Errorf("prompts:\n%s\n---\n%s", b.prompts[0], b.prompts[1])
	}
}

func TestEvaluator_ClaudeGetsOneRepoAttempt(t *testing.T) {
	root := t.TempDir()
	fx := filepath.Join(root, "fx")
	_ = os.MkdirAll(fx, 0o700)
	for name, body := range map[string]string{"lib.go": "package x\n", "check.sh": "exit 1\n", "TASK.md": "fix\n"} {
		_ = os.WriteFile(filepath.Join(fx, name), []byte(body), 0o600)
	}
	b := &fakeBrain{name: "claude", replies: []string{"```file:lib.go\nno\n```"}}
	e, out := newEval(t, b, &fakeRunner{exit: passIfFixed}, root)
	_ = e.runAll(context.Background(), []Task{{ID: "repo-fx", Family: "repo-task", Judge: "check", Ref: "fx"}}, io.Discard)
	if rows := readResults(t, out); rows[0].Attempts != 1 || rows[0].Pass {
		t.Errorf("rows = %+v", rows)
	}
}

func TestEvaluator_SkipDone(t *testing.T) {
	b := &fakeBrain{name: "local", replies: []string{"==="}}
	e, out := newEval(t, b, &fakeRunner{exit: passIfFixed}, t.TempDir())
	e.o.skipDone = true
	tasks := []Task{{ID: "k1", Family: "review", Judge: "key", Ref: "x#k1"}}
	_ = e.runAll(context.Background(), tasks, io.Discard)
	_ = e.runAll(context.Background(), tasks, io.Discard)
	if rows := readResults(t, out); len(rows) != 1 {
		t.Errorf("rows = %d, want 1 (second run skips)", len(rows))
	}
}

func TestLoadJSONL_StrictOnBadLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.jsonl")
	_ = os.WriteFile(p, []byte(`{"id":"a"}`+"\nnot json\n"), 0o600)
	if _, err := loadJSONL[Task](p); err == nil || !strings.Contains(err.Error(), ":2:") {
		t.Errorf("a malformed line was dropped silently: %v", err)
	}
}

func TestRepoPrompt_RealFixture(t *testing.T) {
	p, err := repoPrompt(filepath.Join(repoRoot, "eval/two-brains/fixtures/go-wrap-error"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, "```file:lib.go") || !strings.Contains(p, "```file:lib_test.go") || strings.Contains(p, "```file:check.sh") || strings.Contains(p, "solution.patch") || strings.Contains(p, "SPDX-License-Identifier: GPL-3.0-or-later -->") {
		t.Errorf("prompt:\n%s", p)
	}
}

func TestFlags(t *testing.T) {
	for _, args := range [][]string{{}, {"-out", "x", "-backend", "gpt"}, {"-out", "x", "-split", "half"}} {
		var errb bytes.Buffer
		if code := run(args, io.Discard, &errb); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	o, err := parseFlags([]string{"-out", "x", "-backend", "claude"}, io.Discard)
	if err != nil || o.model != "claude-opus-5-5" || o.effort != "low" {
		t.Errorf("%+v %v", o, err)
	}
}

// The real fixtures, judged with their own solutions through the real
// sandbox: only where a user systemd session exists.
func TestSandboxRunner_RealFixtureSolution(t *testing.T) {
	if os.Getenv("ZHEN_EVAL_SANDBOX_TEST") == "" {
		t.Skip("set ZHEN_EVAL_SANDBOX_TEST=1 on a host with a user systemd session")
	}
	ids := []string{"py-slug"}
	if os.Getenv("ZHEN_EVAL_SANDBOX_TEST") == "all" {
		ents, _ := os.ReadDir(filepath.Join(repoRoot, "eval/two-brains/fixtures"))
		ids = nil
		for _, e := range ents {
			ids = append(ids, e.Name())
		}
	}
	work := t.TempDir()
	runner := newSandboxRunner(work)
	for _, id := range ids {
		fx := filepath.Join(repoRoot, "eval/two-brains/fixtures", id)
		tmp := t.TempDir()
		if err := copyTree(fx, tmp); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("patch", "-p1", "-s", "-d", tmp, "-i", filepath.Join(tmp, "solution.patch"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: patch: %v %s", id, err, out)
		}
		// Every file the fix may touch, as the answer would carry it.
		var ans strings.Builder
		_ = filepath.WalkDir(tmp, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(tmp, p)
			if protectedFiles[rel] || strings.HasSuffix(rel, ".orig") {
				return nil
			}
			b, _ := os.ReadFile(p)
			ans.WriteString("```file:" + rel + "\n" + string(b) + "```\n")
			return nil
		})
		cr := judgeCheck(context.Background(), fx, work, ans.String(), runner)
		if !cr.Pass {
			t.Errorf("%s: solution failed in the sandbox: %+v", id, cr)
		}
		b, _ := json.Marshal(cr.Detail)
		t.Logf("%s %s", id, b)
	}
}

func TestKeyPromptCarriesLanguage(t *testing.T) {
	if got := keyPrompt(gatePrompt{Language: "go", Prompt: "How do I check for an error?"}); got != "Language: go\n\nHow do I check for an error?" {
		t.Errorf("%q", got)
	}
	if got := keyPrompt(gatePrompt{Prompt: "x"}); got != "x" {
		t.Errorf("%q", got)
	}
}
