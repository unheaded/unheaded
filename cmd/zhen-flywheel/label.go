// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Labeler turns outcomes recorded elsewhere into labels. Every label's
// source_ref names the row it came from, so a second run adds nothing.
type Labeler struct {
	Store        Store
	ResultsFiles []string // eval/two-brains/results-*.jsonl (judge verdicts)
	Repo         string   // git checkout for human edits ("" = skip them)
	MergedInto   string   // branch accepted runs are merged into (develop)
	Scanner      Scanner  // re-scans human text before it is stored
}

// LabelReport counts what one label run did.
type LabelReport struct {
	Inserted      int            `json:"inserted"`
	ByKind        map[string]int `json:"by_kind"`
	HumanCaptures int            `json:"human_captures"`
	Unmatched     map[string]int `json:"unmatched"` // outcome rows with no capture to label
	Skipped       []string       `json:"skipped,omitempty"`
}

var checkSummaryRE = regexp.MustCompile(`^exit=(-?\d+) pass=(true|false)$`)

func (lb *Labeler) Run(ctx context.Context) (LabelReport, error) {
	rep := LabelReport{ByKind: map[string]int{}, Unmatched: map[string]int{}}
	caps, err := lb.Store.Captures(ctx)
	if err != nil {
		return rep, err
	}
	byConv := map[string][]Capture{}
	byRun := map[string][]Capture{}
	for _, c := range caps {
		byConv[c.ConversationID] = append(byConv[c.ConversationID], c)
		if c.RunID != "" {
			byRun[c.RunID] = append(byRun[c.RunID], c)
		}
	}
	add := func(l Label) error {
		ok, err := lb.Store.InsertLabel(ctx, l)
		if err != nil {
			return fmt.Errorf("label %s %s: %w", l.Kind, l.SourceRef, err)
		}
		if ok {
			rep.Inserted++
			rep.ByKind[l.Kind]++
		}
		return nil
	}

	// run_check results: the check judges the latest draft of its
	// conversation written before it finished.
	checks, err := lb.Store.CheckActions(ctx)
	if err != nil {
		return rep, fmt.Errorf("zhen_actions: %w", err)
	}
	for _, a := range checks {
		m := checkSummaryRE.FindStringSubmatch(a.Summary)
		if m == nil {
			rep.Unmatched["zhen_actions"]++
			continue
		}
		at := a.CompletedAt
		if at.IsZero() {
			at = a.PlannedAt
		}
		var target *Capture
		for i := range byConv[a.SessionID] {
			c := &byConv[a.SessionID][i]
			if c.AuthorKind == AuthorHuman || c.CapturedAt.After(at) {
				continue
			}
			if target == nil || c.CapturedAt.After(target.CapturedAt) {
				target = c
			}
		}
		if target == nil {
			rep.Unmatched["zhen_actions"]++
			continue
		}
		kind := LabelCheckFail
		if m[2] == "true" {
			kind = LabelCheckPass
		}
		exit, _ := strconv.Atoi(m[1])
		if err := add(Label{CaptureID: target.ID, Kind: kind, SourceRef: fmt.Sprintf("zhen_actions:%d", a.ID),
			Value: jsonObject(map[string]any{"action_id": a.ID, "exit": exit})}); err != nil {
			return rep, err
		}
	}

	// Escalations: the local answers of a conversation that went to Claude.
	escs, err := lb.Store.Escalations(ctx)
	switch {
	case errors.Is(err, ErrUnavailable):
		rep.Skipped = append(rep.Skipped, "llm_requests escalations: WELL_OPS_DSN unset")
	case err != nil:
		return rep, fmt.Errorf("llm_requests: %w", err)
	}
	for _, e := range escs {
		n := 0
		for _, c := range byConv[e.ConversationID] {
			if c.AuthorKind != AuthorLocal {
				continue
			}
			n++
			if err := add(Label{CaptureID: c.ID, Kind: LabelEscalated, SourceRef: fmt.Sprintf("llm_requests:%d", e.FirstID),
				Value: jsonObject(map[string]any{"route_reason": e.RouteReason})}); err != nil {
				return rep, err
			}
		}
		if n == 0 {
			rep.Unmatched["llm_requests"]++
		}
	}

	// Stevie's review of agent runs, and his edits at merge.
	runs, err := lb.Store.AgentRuns(ctx)
	switch {
	case errors.Is(err, ErrUnavailable):
		rep.Skipped = append(rep.Skipped, "zhen_agent_runs: table not present (migration 032, Phase 6)")
	case err != nil:
		return rep, fmt.Errorf("zhen_agent_runs: %w", err)
	}
	for _, r := range runs {
		kind := LabelAccepted
		if r.Status == "rejected" {
			kind = LabelRejected
		}
		of := byRun[r.ID]
		if len(of) == 0 {
			rep.Unmatched["zhen_agent_runs"]++
			continue
		}
		for _, c := range of {
			if c.AuthorKind == AuthorHuman {
				continue
			}
			if err := add(Label{CaptureID: c.ID, Kind: kind, SourceRef: "zhen_agent_runs:" + r.ID,
				Value: jsonObject(map[string]any{"review_reason": r.ReviewReason})}); err != nil {
				return rep, err
			}
		}
		if kind == LabelAccepted && lb.Repo != "" && r.HeadCommit != "" {
			if err := lb.humanEdit(ctx, r, of, add, &rep); err != nil {
				rep.Skipped = append(rep.Skipped, fmt.Sprintf("human edit for run %s: %v", r.ID, err))
			}
		}
	}

	// Judge verdicts from eval results, matched by task and answer hash.
	for _, f := range lb.ResultsFiles {
		if err := lb.judges(f, byConv, add, &rep); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

type resultRow struct {
	Task         string `json:"task"`
	Family       string `json:"family"`
	Backend      string `json:"backend"`
	Model        string `json:"model"`
	Judge        string `json:"judge"`
	Pass         bool   `json:"pass"`
	Detail       string `json:"detail"`
	AnswerSHA256 string `json:"answer_sha256"`
	At           string `json:"at"`
}

func (lb *Labeler) judges(path string, byConv map[string][]Capture, add func(Label) error, rep *LabelReport) error {
	rows, err := readResults(path)
	if err != nil {
		return err
	}
	base := filepath.Base(path)
	for _, r := range rows {
		n := 0
		for _, c := range byConv["eval:"+r.Task] {
			if c.Backend != r.Backend || sha256Hex(c.Response) != r.AnswerSHA256 {
				continue
			}
			n++
			kind := LabelJudgeFail
			if r.Pass {
				kind = LabelJudgePass
			}
			detail := r.Detail
			if len(detail) > 200 {
				detail = detail[:200]
			}
			if err := add(Label{CaptureID: c.ID, Kind: kind, SourceRef: fmt.Sprintf("eval:%s:%s:%s", base, r.Task, r.Backend),
				Value: jsonObject(map[string]any{"judge": r.Judge, "detail": detail, "results": base})}); err != nil {
				return err
			}
		}
		if n == 0 {
			rep.Unmatched["eval"]++
		}
	}
	return nil
}

// readResults reads a zhen-eval results file strictly: a bad line is an
// error naming its number.
func readResults(path string) ([]resultRow, error) {
	// #nosec G304 -- a results file the operator named.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []resultRow
	r := bufio.NewReaderSize(f, 64<<10)
	for n := 1; ; n++ {
		line, rerr := readLine(r)
		if rerr != nil && rerr != io.EOF {
			return nil, fmt.Errorf("%s:%d: %w", path, n, rerr)
		}
		if len(strings.TrimSpace(string(line))) > 0 {
			var row resultRow
			if err := json.Unmarshal(line, &row); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, n, err)
			}
			if row.Task == "" || row.Backend == "" || len(row.AnswerSHA256) != 64 {
				return nil, fmt.Errorf("%s:%d: a results row needs task, backend and answer_sha256", path, n)
			}
			out = append(out, row)
		}
		if rerr == io.EOF {
			return out, nil
		}
	}
}

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (lb *Labeler) git(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// #nosec G204 -- fixed git binary, arguments are commit ids checked against shaRE and paths from git itself
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", lb.Repo}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

// humanEdit: an accepted run whose merged files differ from the agent's
// head means Stevie edited them. The merged text of those files becomes a
// human capture (same task prompt), and every capture of the run gets a
// human_edit label pointing at it.
func (lb *Labeler) humanEdit(ctx context.Context, r AgentRun, of []Capture, add func(Label) error, rep *LabelReport) error {
	if !shaRE.MatchString(r.HeadCommit) || !shaRE.MatchString(r.BaseCommit) {
		return errors.New("base or head is not a commit id")
	}
	target := lb.MergedInto
	if target == "" {
		target = "develop"
	}
	revs, err := lb.git(ctx, "rev-list", "--ancestry-path", "--reverse", r.HeadCommit+".."+target, "--")
	if err != nil {
		return fmt.Errorf("rev-list: %w", err)
	}
	fields := strings.Fields(revs)
	if len(fields) == 0 {
		return nil // not merged (yet)
	}
	merge := fields[0]
	touched, err := lb.git(ctx, "diff", "--name-only", "-z", r.BaseCommit, r.HeadCommit, "--")
	if err != nil {
		return fmt.Errorf("diff base..head: %w", err)
	}
	var files []string
	for _, p := range strings.Split(touched, "\x00") {
		if p == "" {
			continue
		}
		if _, err := lb.git(ctx, "diff", "--quiet", r.HeadCommit, merge, "--", p); err == nil {
			continue // identical at merge: no edit
		}
		files = append(files, p)
	}
	if len(files) == 0 {
		return nil
	}
	sort.Strings(files)
	var sb strings.Builder
	for _, p := range files {
		body, err := lb.git(ctx, "show", merge+":"+p)
		if err != nil {
			continue // deleted at merge
		}
		fmt.Fprintf(&sb, "--- %s\n%s", p, body)
		if !strings.HasSuffix(body, "\n") {
			sb.WriteByte('\n')
		}
	}
	if sb.Len() == 0 {
		return nil
	}
	when, err := lb.git(ctx, "show", "-s", "--format=%cI", merge)
	if err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(when))
	if err != nil {
		return err
	}
	// The task is the run's latest agent capture.
	var task *Capture
	for i := range of {
		if of[i].AuthorKind != AuthorHuman && (task == nil || of[i].CapturedAt.After(task.CapturedAt)) {
			task = &of[i]
		}
	}
	if task == nil {
		return errors.New("run has no agent capture")
	}
	h := Capture{CapturedAt: at.UTC(), Source: SourceAgentd, Backend: BackendNone, Model: AuthorHuman, AuthorKind: AuthorHuman,
		TaskFamily: task.TaskFamily, ConversationID: task.ConversationID, RunID: r.ID, RecordID: "human-edit:" + r.ID + ":" + merge[:12],
		Schema: task.Schema, RedactionVersion: RedactionVersion, Prompt: task.Prompt, Response: sb.String(), ToolTrace: json.RawMessage("[]")}
	if h.ID, err = CaptureID(h.Model, h.Prompt, h.Response, h.ToolTrace); err != nil {
		return err
	}
	if lb.Scanner == nil {
		return errors.New("no scanner: human text is not stored unscanned")
	}
	hits, err := lb.Scanner.Scan(ctx, h.Response)
	if err != nil {
		return err
	}
	if hits = nonZero(hits); len(hits) > 0 {
		return fmt.Errorf("merged text flagged by the re-scan (%s); not stored", kindList(hits))
	}
	if len(h.Prompt)+len(h.Response) > 250<<10 {
		return errors.New("merged text over the 256 KiB capture cap")
	}
	ok, err := lb.Store.InsertCapture(ctx, &h)
	if err != nil {
		return err
	}
	if ok {
		rep.HumanCaptures++
	}
	for _, c := range of {
		if c.AuthorKind == AuthorHuman {
			continue
		}
		if err := add(Label{CaptureID: c.ID, Kind: LabelHumanEdit, SourceRef: "human_edit:" + r.ID,
			Value: jsonObject(map[string]any{"human_capture": h.ID, "merge": merge, "files": files})}); err != nil {
			return err
		}
	}
	return nil
}

func cmdLabel(ctx context.Context, args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("label", flag.ContinueOnError)
	fs.SetOutput(stderr)
	results := fs.String("results", "eval/two-brains/results-*.jsonl", "glob of zhen-eval results files (judge verdicts)")
	repo := fs.String("repo", "", "git checkout accepted agent runs are merged in (human edits); empty: skip edits")
	into := fs.String("merged-into", "develop", "branch accepted runs are merged into")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	files, err := filepath.Glob(*results)
	if err != nil {
		return 2, err
	}
	sort.Strings(files)
	st, closeFn, err := openStore(true)
	if err != nil {
		return 1, err
	}
	defer closeFn()
	lb := &Labeler{Store: st, ResultsFiles: files, Repo: *repo, MergedInto: *into}
	if *repo != "" {
		if lb.Scanner, err = newScanner(); err != nil {
			return 1, err
		}
	}
	rep, err := lb.Run(ctx)
	printJSON(stdout, rep)
	if err != nil {
		return 1, err
	}
	return 0, nil
}
