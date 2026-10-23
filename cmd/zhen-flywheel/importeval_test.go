// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/zhencapture"
)

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Kept eval answers become captures that ingest and label accept: the
// judge verdict lands on exactly the answer that was judged.
func TestImportEval_IngestThenJudgeLabels(t *testing.T) {
	d := t.TempDir()
	tasks := filepath.Join(d, "tasks.jsonl")
	writeFile(t, tasks, `{"id":"syntax-go","family":"syntax","judge":"key","ref":"eval/coding-gate/prompts.jsonl#syntax-go"}
{"id":"go-nil-map","family":"repo-task","judge":"check","ref":"eval/two-brains/fixtures/go-nil-map"}
`)
	prompts := filepath.Join(d, "prompts.jsonl")
	writeFile(t, prompts, `{"id":"syntax-go","kind":"syntax","language":"go","prompt":"How do I check for an error?"}`+"\n")
	answers := filepath.Join(d, "answers")
	if err := os.Mkdir(answers, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(answers, "local-syntax-go.txt"), "if err != nil { return err }")
	writeFile(t, filepath.Join(answers, "claude-syntax-go.txt"), "edited after the run")
	res := writeResults(t,
		map[string]any{"task": "syntax-go", "family": "syntax", "backend": "local", "model": "qwen-7b", "judge": "key", "pass": true,
			"answer_sha256": sha256Hex("if err != nil { return err }"), "at": "2026-10-23T11:00:00Z"},
		map[string]any{"task": "syntax-go", "family": "syntax", "backend": "claude", "model": "claude-opus-5-5", "judge": "key", "pass": true,
			"answer_sha256": sha256Hex("the answer that was judged"), "at": "2026-10-23T11:00:00Z"},
		map[string]any{"task": "go-nil-map", "family": "repo-task", "backend": "local", "model": "qwen-7b", "judge": "check", "pass": true,
			"answer_sha256": sha256Hex("patch"), "at": "2026-10-23T11:00:00Z"},
		// The brain errored: no answer, no hash (results-*-qwen14b has two).
		map[string]any{"task": "syntax-go", "family": "syntax", "backend": "local", "model": "qwen-14b", "judge": "key", "pass": false,
			"error": "local LLM unavailable", "at": "2026-10-23T11:00:00Z"},
	)
	cdir := captureDir(t)
	w, err := zhencapture.Open(cdir)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := (&EvalImporter{TasksPath: tasks, PromptsPath: prompts, AnswersDir: answers, Writer: w}).Import(res)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Written != 1 || rep.Mismatch != 1 || rep.NotGateTask != 1 || rep.NoAnswer != 1 {
		t.Fatalf("import = %+v", rep)
	}
	st := newMemStore()
	if _, err := newIngester(st).IngestDir(context.Background(), cdir); err != nil {
		t.Fatal(err)
	}
	if len(st.captures) != 1 {
		t.Fatalf("captures = %d", len(st.captures))
	}
	var c Capture
	for _, x := range st.captures {
		c = x
	}
	if c.Source != SourceEval || c.TaskFamily != "syntax" || c.ConversationID != "eval:syntax-go" ||
		c.Prompt != "Language: go\n\nHow do I check for an error?" || c.Model != "qwen-7b" || c.AuthorKind != AuthorLocal {
		t.Errorf("capture = %+v", c)
	}
	if _, err := (&Labeler{Store: st, ResultsFiles: []string{res}}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.labelsOf(c.ID); got[LabelJudgePass] != 1 {
		t.Errorf("labels = %v", got)
	}
}
