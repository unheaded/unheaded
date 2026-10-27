// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const rbTestdata = "testdata/runbook-draft"

// realTools are the judge's real scripts (scripts/run-runbook.py,
// scripts/check-runbooks.py) when a python3 with PyYAML is at hand
// (ZHEN_EVAL_PYTHON, else PATH); otherwise the test skips.
func realTools(t *testing.T) RunbookTools {
	t.Helper()
	py := envOr("ZHEN_EVAL_PYTHON", "python3")
	if err := exec.Command(py, "-c", "import yaml").Run(); err != nil { // #nosec G204 -- test helper, operator's python
		t.Skipf("%s cannot import yaml (set ZHEN_EVAL_PYTHON or put ~/.venv/zhen/bin on PATH): %v", py, err)
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return scriptTools{python: py, root: root, timeout: 60 * time.Second}
}

func answerWith(t *testing.T, yamlFile string) string {
	t.Helper()
	b, err := os.ReadFile(yamlFile) // #nosec G304 -- testdata
	if err != nil {
		t.Fatal(err)
	}
	return "Here is the runbook.\n```file:demo-check.yaml\n" + string(b) + "```\n"
}

func TestJudgeRunbookDraft_PassAndFail(t *testing.T) {
	tools := realTools(t)
	task, err := loadRunbookTask(filepath.Join(rbTestdata, "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	ctx := context.Background()

	ok := judgeRunbookDraft(ctx, task, work, answerWith(t, filepath.Join(rbTestdata, "pass.yaml")), tools)
	if !ok.Pass || ok.Judges["dry_run"] != "pass" || ok.Judges["check"] != "pass" || ok.Judges["requirements"] != "pass" {
		t.Errorf("pass.yaml: %+v", ok)
	}

	bad := judgeRunbookDraft(ctx, task, work, answerWith(t, filepath.Join(rbTestdata, "fail.yaml")), tools)
	if bad.Pass || bad.Judges["dry_run"] != "pass" || !strings.HasPrefix(bad.Judges["check"], "fail") ||
		!strings.Contains(bad.Judges["requirements"], "requires[0]") || !strings.Contains(bad.Judges["requirements"], "forbids[0]") {
		t.Errorf("fail.yaml: %+v", bad)
	}

	// YAML that is not a mapping: the runner itself fails.
	list := judgeRunbookDraft(ctx, task, work, "```yaml\n- 19999/health\n- requires_sudo: false\n```", tools)
	if list.Pass || !strings.HasPrefix(list.Judges["dry_run"], "fail") || !strings.HasPrefix(list.Judges["check"], "fail") {
		t.Errorf("list yaml: %+v", list)
	}
}

// Every pre-registered runbook-draft task passes the real judge with its
// reference draft, and fails with an empty runbook.
func TestRunbookDraftTasks_ReferencesPassTheRealJudge(t *testing.T) {
	tools := realTools(t)
	tasks, _, _, err := loadAll(&options{root: repoRoot, tasksPath: "eval/two-brains/split.jsonl", keysPath: "eval/two-brains/keys.jsonl",
		promptsPath: "eval/coding-gate/prompts.jsonl", split: "all", families: "runbook-draft"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 5 {
		t.Fatalf("%d runbook-draft tasks, want 5", len(tasks))
	}
	for _, tk := range tasks {
		dir := filepath.Join(repoRoot, tk.Ref)
		task, err := loadRunbookTask(dir)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := os.ReadFile(filepath.Join(dir, "reference.yaml")) // #nosec G304 -- the repo's own fixture
		if err != nil {
			t.Fatal(err)
		}
		if r := judgeRunbookDraft(context.Background(), task, t.TempDir(), "```yaml\n"+string(ref)+"```", tools); !r.Pass {
			t.Errorf("%s: reference fails: %+v", tk.ID, r)
		}
		if r := judgeRunbookDraft(context.Background(), task, t.TempDir(), "```yaml\napiVersion: runbook/v1\n```", tools); r.Pass {
			t.Errorf("%s: an empty runbook passed", tk.ID)
		}
		if strings.Contains(task.prompt, "reference.yaml") || strings.Contains(task.prompt, "SPDX") || !strings.Contains(task.prompt, task.req.Name) {
			t.Errorf("%s: prompt:\n%s", tk.ID, task.prompt)
		}
	}
}

// fakeTools records the file each script was given.
type fakeTools struct {
	dryExit, checkExit int
	err                error
	paths              []string
	body               string
}

func (f *fakeTools) DryRun(_ context.Context, path string) (int, string, error) {
	f.paths = append(f.paths, path)
	b, _ := os.ReadFile(path) // #nosec G304 -- test
	f.body = string(b)
	return f.dryExit, "", f.err
}

func (f *fakeTools) Check(_ context.Context, path string) (int, string, error) {
	f.paths = append(f.paths, path)
	return f.checkExit, "", f.err
}

func TestJudgeRunbookDraft_Parsing(t *testing.T) {
	task, err := loadRunbookTask(filepath.Join(rbTestdata, "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	good := "metadata:\n  requires_sudo: false\nsteps:\n  - command: curl http://127.0.0.1:19999/health\n"
	work := t.TempDir()
	for name, tc := range map[string]struct {
		answer string
		pass   bool
		detail string
	}{
		"file block":        {"```file:x.yaml\n" + good + "```", true, "dry_run=pass check=pass requirements=pass"},
		"yaml fence":        {"Sure.\n```yaml\n" + good + "```\nDone.", true, "dry_run=pass"},
		"yml fence":         {"```yml\n" + good + "```", true, "dry_run=pass"},
		"prose only":        {"I would curl the health endpoint.", false, "parse_fail"},
		"non-yaml file":     {"```file:x.sh\ncurl x\n```", false, "parse_fail"},
		"missing a require": {"```yaml\nsteps: []\n```", false, "requirements=missing requires[0],requires[1]"},
		"forbidden":         {"```yaml\n" + good + "  - command: systemctl restart x\n```", false, "requirements=forbidden forbids[0]"},
		"too large":         {"```yaml\n" + strings.Repeat("#", maxRunbookBytes+1) + "\n```", false, "too_large"},
	} {
		f := &fakeTools{}
		r := judgeRunbookDraft(context.Background(), task, work, tc.answer, f)
		if r.Pass != tc.pass || !strings.Contains(r.Detail, tc.detail) {
			t.Errorf("%s: pass=%v detail=%q judges=%v", name, r.Pass, r.Detail, r.Judges)
		}
		if tc.pass {
			// Both scripts saw the same file, named after the task's runbook,
			// inside the work dir, holding exactly the answer's YAML.
			if len(f.paths) != 2 || f.paths[0] != f.paths[1] || filepath.Base(f.paths[0]) != "demo-check.yaml" ||
				!strings.HasPrefix(f.paths[0], work+string(filepath.Separator)) || f.body != good {
				t.Errorf("%s: paths %v body %q", name, f.paths, f.body)
			}
			if _, err := os.Stat(f.paths[0]); err == nil {
				t.Errorf("%s: judged file left behind", name)
			}
		}
	}

	// Script failures and errors are reported per sub-judge.
	r := judgeRunbookDraft(context.Background(), task, work, "```yaml\n"+good+"```", &fakeTools{dryExit: 1, checkExit: 1})
	if r.Pass || r.Judges["dry_run"] != "fail exit=1" || r.Judges["check"] != "fail exit=1" || r.Judges["requirements"] != "pass" {
		t.Errorf("failing scripts: %+v", r)
	}
	r = judgeRunbookDraft(context.Background(), task, work, "```yaml\n"+good+"```", &fakeTools{err: errors.New("python3 not found")})
	if r.Pass || !strings.HasPrefix(r.Judges["dry_run"], "error") || !strings.HasPrefix(r.Judges["check"], "error") {
		t.Errorf("tool error: %+v", r)
	}
}

func TestLoadRunbookTask_Strict(t *testing.T) {
	for name, req := range map[string]string{
		"no file":       "",
		"bad name":      `{"name": "../etc/passwd", "requires": ["x"]}`,
		"bad regex":     `{"name": "ok", "requires": ["(x"]}`,
		"no requires":   `{"name": "ok"}`,
		"unknown field": `{"name": "ok", "requires": ["x"], "must": ["y"]}`,
	} {
		d := t.TempDir()
		_ = os.WriteFile(filepath.Join(d, "TASK.md"), []byte("write it\n"), 0o600)
		if req != "" {
			_ = os.WriteFile(filepath.Join(d, "requirements.json"), []byte(req), 0o600)
		}
		if _, err := loadRunbookTask(d); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

func TestLoadAll_RunbookTaskWithoutFixtureFails(t *testing.T) {
	d := t.TempDir()
	tasks := filepath.Join(d, "tasks.jsonl")
	_ = os.WriteFile(tasks, []byte(`{"id":"rb","family":"runbook-draft","judge":"runbook","ref":"nope","split":"tune"}`+"\n"), 0o600)
	root, _ := filepath.Abs(repoRoot)
	_, _, _, err := loadAll(&options{root: d, tasksPath: tasks, keysPath: filepath.Join(root, "eval/two-brains/keys.jsonl"),
		promptsPath: filepath.Join(root, "eval/coding-gate/prompts.jsonl"), split: "all"})
	if err == nil || !strings.Contains(err.Error(), "rb") {
		t.Errorf("err = %v", err)
	}
}

// The real runbook-draft tasks through the evaluator with a fake brain
// (answering each task's reference draft) and fake scripts: one attempt,
// one row per task with its three sub-judgements (Step 149 without a model).
func TestEvaluator_RunbookDraftTasks(t *testing.T) {
	all, _, _, err := loadAll(&options{root: repoRoot, tasksPath: "eval/two-brains/split.jsonl", keysPath: "eval/two-brains/keys.jsonl",
		promptsPath: "eval/coding-gate/prompts.jsonl", split: "all", families: "runbook-draft"})
	if err != nil {
		t.Fatal(err)
	}
	var replies []string
	for _, tk := range all {
		b, err := os.ReadFile(filepath.Join(repoRoot, tk.Ref, "reference.yaml")) // #nosec G304 -- the repo's own fixture
		if err != nil {
			t.Fatal(err)
		}
		replies = append(replies, "```file:x.yaml\n"+string(b)+"```")
	}
	b := &fakeBrain{name: "local", replies: replies}
	e, out := newEval(t, b, &fakeRunner{exit: passIfFixed}, repoRoot)
	e.rb = &fakeTools{}
	e.o.limit = 3
	if code := e.runAll(context.Background(), all, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	rows := readResults(t, out)
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	for i, r := range rows {
		if !r.Pass || r.Judge != "runbook" || r.Attempts != 1 || len(r.Judges) != 3 || r.Family != "runbook-draft" {
			t.Errorf("row %d = %+v", i, r)
		}
	}
	if !strings.Contains(b.prompts[0], "wotan-restart") && !strings.Contains(b.prompts[0], "akira-band-check") {
		t.Errorf("prompt 0:\n%s", b.prompts[0])
	}
}
