// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/zhencapture"
)

// EvalImporter turns answers zhen-eval kept (-answers-dir) into
// zhen-capture/v1 records, so they enter the Well the same way every
// other capture does (redacted by the writer, re-scanned by ingest).
// Only coding-gate prompt tasks are imported: their prompt is fully known
// (Language: <lang>, then the prompt, as zhen-eval asks it). Repository
// tasks are skipped (their prompt is a repository snapshot). Every eval
// task is hold-out or excluded in the export, never training data.
type EvalImporter struct {
	TasksPath   string // eval/two-brains/tasks.jsonl
	PromptsPath string // eval/coding-gate/prompts.jsonl
	AnswersDir  string
	Writer      *zhencapture.Writer
}

// ImportReport counts one import.
type ImportReport struct {
	Rows        int      `json:"rows"`
	Written     int      `json:"written"`
	NotGateTask int      `json:"skipped_not_coding_gate"`
	NoAnswer    int      `json:"skipped_no_answer_file"`
	Mismatch    int      `json:"skipped_answer_hash_mismatch"`
	Notes       []string `json:"notes,omitempty"`
}

type evalTask struct {
	ID     string `json:"id"`
	Family string `json:"family"`
	Ref    string `json:"ref"`
}

type gatePrompt struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Prompt   string `json:"prompt"`
}

func readJSONL[T any](path string) ([]T, error) {
	// #nosec G304 -- an eval file the operator named.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []T
	r := bufio.NewReaderSize(f, 64<<10)
	for n := 1; ; n++ {
		line, rerr := readLine(r)
		if rerr != nil && rerr != io.EOF {
			return nil, fmt.Errorf("%s:%d: %w", path, n, rerr)
		}
		if strings.TrimSpace(string(line)) != "" {
			var v T
			if err := json.Unmarshal(line, &v); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, n, err)
			}
			out = append(out, v)
		}
		if rerr == io.EOF {
			return out, nil
		}
	}
}

// gatePrompts maps a coding-gate prompt id to the prompt zhen-eval sends
// (cmd/zhen-eval keyPrompt).
func gatePrompts(path string) (map[string]string, error) {
	ps, err := readJSONL[gatePrompt](path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, p := range ps {
		if p.Language == "" {
			out[p.ID] = p.Prompt
		} else {
			out[p.ID] = "Language: " + p.Language + "\n\n" + p.Prompt
		}
	}
	return out, nil
}

func (im *EvalImporter) Import(resultsPath string) (ImportReport, error) {
	var rep ImportReport
	tasks, err := readJSONL[evalTask](im.TasksPath)
	if err != nil {
		return rep, err
	}
	prompts, err := gatePrompts(im.PromptsPath)
	if err != nil {
		return rep, err
	}
	refOf := map[string]string{}
	for _, t := range tasks {
		refOf[t.ID] = t.Ref
	}
	rows, err := readResults(resultsPath)
	if err != nil {
		return rep, err
	}
	for _, r := range rows {
		rep.Rows++
		ref := refOf[r.Task]
		i := strings.LastIndexByte(ref, '#')
		if !strings.HasPrefix(ref, "eval/coding-gate/prompts.jsonl#") || i < 0 || prompts[ref[i+1:]] == "" {
			rep.NotGateTask++
			continue
		}
		if r.AnswerSHA256 == "" { // the brain errored: no answer was judged
			rep.NoAnswer++
			continue
		}
		if r.Backend != AuthorLocal && r.Backend != AuthorClaude {
			return rep, fmt.Errorf("%s: task %s: unknown backend %q", resultsPath, r.Task, r.Backend)
		}
		p := filepath.Join(im.AnswersDir, r.Backend+"-"+filepath.Base(r.Task)+".txt")
		// #nosec G304 -- a fixed name in the answers directory the operator named.
		body, err := os.ReadFile(p)
		if err != nil {
			rep.NoAnswer++
			continue
		}
		if sha256Hex(string(body)) != r.AnswerSHA256 {
			rep.Mismatch++
			rep.Notes = append(rep.Notes, fmt.Sprintf("%s: kept answer is not the one judged (sha256 differs)", filepath.Base(p)))
			continue
		}
		at, err := time.Parse(time.RFC3339, r.At)
		if err != nil {
			return rep, fmt.Errorf("%s: task %s: at %q: %w", resultsPath, r.Task, r.At, err)
		}
		if r.Model == "" {
			return rep, fmt.Errorf("%s: task %s: no model", resultsPath, r.Task)
		}
		leg := &zhencapture.Leg{Model: r.Model, Answer: string(body)}
		rec := &zhencapture.Record{Source: SourceEval, Time: at.UTC(), Route: r.Backend, Prompt: prompts[ref[i+1:]],
			ConversationID: "eval:" + r.Task, TaskFamily: r.Family, Final: string(body)}
		if r.Backend == AuthorLocal {
			rec.Local = leg
		} else {
			rec.Claude = leg
		}
		if err := im.Writer.Write(rec); err != nil {
			return rep, err
		}
		rep.Written++
	}
	return rep, nil
}

func cmdImportEval(_ context.Context, args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("import-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	results := fs.String("results", "", "zhen-eval results file (eval/two-brains/results-*.jsonl)")
	answers := fs.String("answers-dir", "", "the -answers-dir that zhen-eval run kept its answers in")
	tasks := fs.String("tasks", "eval/two-brains/tasks.jsonl", "two-brains task list")
	prompts := fs.String("prompts", "eval/coding-gate/prompts.jsonl", "coding-gate prompts")
	dir := fs.String("dir", defaultCaptureDir(), "capture directory to write into (outside any repository, 0700)")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	if *results == "" || *answers == "" {
		return 2, errors.New("-results and -answers-dir are required")
	}
	w, err := zhencapture.Open(*dir)
	if err != nil {
		return 1, err
	}
	im := &EvalImporter{TasksPath: *tasks, PromptsPath: *prompts, AnswersDir: *answers, Writer: w}
	rep, err := im.Import(*results)
	printJSON(stdout, rep)
	if err != nil {
		return 1, err
	}
	return 0, nil
}
