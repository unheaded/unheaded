// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Command zhen-eval runs the two-brains eval (S-TWO-BRAINS Phase 4): the
// same tasks on the local model (llama-server) or on Claude, judged
// mechanically: `key` tasks by pre-registered regex keys, `check` tasks
// by applying the answer's file blocks to a fresh copy of the fixture and
// running its check.sh in a no-network user sandbox, `runbook` tasks
// (runbook-draft) by the runbook harness: dry run, check-runbooks --one
// and the task's requirements (runbook.go). Claude never grades Claude.
//
//	zhen-eval -backend local -model qwen-7b -out results-qwen7b.jsonl
//	zhen-eval -family runbook-draft -limit 3 -out /tmp/rb.jsonl
//	zhen-eval -backend claude -effort low -daily-cap-usd 3 -out results-opus.jsonl
//
// One JSON line per task: pass, judge detail, tokens, cost, latency. No
// answer text (only its SHA-256 and size); -answers-dir keeps answers
// outside the repo for inspection.
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
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/localllm"
)

// Task is one row of split.jsonl / tasks.jsonl.
type Task struct {
	ID     string `json:"id"`
	Family string `json:"family"`
	Judge  string `json:"judge"`
	Ref    string `json:"ref"`
	Split  string `json:"split"`
}

// Result is one task's outcome. It carries no answer text.
type Result struct {
	Task         string   `json:"task"`
	Family       string   `json:"family"`
	Split        string   `json:"split"`
	Backend      string   `json:"backend"`
	Model        string   `json:"model"`
	ServedBy     string   `json:"served_by,omitempty"`
	Effort       string   `json:"effort,omitempty"`
	Judge        string   `json:"judge"`
	Pass         bool     `json:"pass"`
	Detail       string   `json:"detail"`
	Attempts     int      `json:"attempts"`
	AttemptMS    []int64  `json:"attempt_ms"`
	LatencyMS    int64    `json:"latency_ms"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CostNanoUSD  int64    `json:"cost_nano_usd"`
	CostUSD      string   `json:"cost_usd"`
	AnswerSHA256 string   `json:"answer_sha256,omitempty"`
	AnswerBytes  int      `json:"answer_bytes"`
	Error        string   `json:"error,omitempty"`
	Details      []string `json:"attempt_details,omitempty"`
	At           string   `json:"at"`
	// Judges: the runbook judge's sub-results (dry_run, check, requirements).
	Judges map[string]string `json:"judges,omitempty"`
}

// Brain answers one prompt.
type Brain interface {
	Name() string // "local" or "claude"
	Model() string
	Ask(ctx context.Context, system, prompt string, maxTokens int) (Answer, error)
}

// Answer is what a brain said and what it cost.
type Answer struct {
	Text         string
	ServedBy     string
	InputTokens  int64
	OutputTokens int64
	Cost         claudebridge.NanoUSD
	Duration     time.Duration
}

// System prompts: the same for both brains.
const (
	systemKey = `You are a senior software engineer answering a colleague's coding question.
Answer concisely and correctly; show code where it helps.`
	systemRepo = "You fix small code tasks in a repository fixture.\n" +
		"Reply with the complete new content of every file you change, each in its own fenced code block whose opening line is ```file:<relative path> (for example ```file:lib.go).\n" +
		"Change only what the task needs. Never change check.sh, TASK.md or the tests. A short explanation before the blocks is fine."
)

type options struct {
	backend, model, effort      string
	split, families             string
	tasksPath, keysPath         string
	promptsPath, root           string
	out, answersDir, workDir    string
	skipDone                    bool
	localURL                    string
	localTimeout                time.Duration
	keyTokens, repoTokens       int
	claudeKeyTok, claudeRepoTok int64
	dailyCapUSD                 float64
	ledger                      string
	seed                        int
	limit                       int
	localAttempts               int
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func parseFlags(args []string, stderr io.Writer) (*options, error) {
	o := &options{}
	fs := flag.NewFlagSet("zhen-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.backend, "backend", "local", "local (llama-server) or claude")
	fs.StringVar(&o.model, "model", "", "model name (local: the label recorded and sent; claude: model id, default claude-opus-5-5)")
	fs.StringVar(&o.effort, "effort", "low", "Claude effort")
	fs.StringVar(&o.split, "split", "all", "tune, holdout or all")
	fs.StringVar(&o.families, "family", "", "comma-separated families (default: every scored family)")
	fs.StringVar(&o.tasksPath, "tasks", "eval/two-brains/split.jsonl", "tasks with their split")
	fs.StringVar(&o.keysPath, "keys", "eval/two-brains/keys.jsonl", "pre-registered keys")
	fs.StringVar(&o.promptsPath, "prompts", "eval/coding-gate/prompts.jsonl", "coding-gate prompts")
	fs.StringVar(&o.root, "root", ".", "repository root (fixture refs are relative to it)")
	fs.StringVar(&o.out, "out", "", "results JSONL (appended)")
	fs.StringVar(&o.answersDir, "answers-dir", "", "also keep each answer as <dir>/<backend>-<task>.txt (outside the repo)")
	fs.StringVar(&o.workDir, "work-dir", "", "where fixture copies are made (default: a temp dir)")
	fs.BoolVar(&o.skipDone, "skip-done", false, "skip tasks already in -out for this backend and model")
	fs.StringVar(&o.localURL, "local-url", envOr("ZHEN_LOCAL_LLM_URL", localllm.DefaultURL), "llama-server URL")
	fs.DurationVar(&o.localTimeout, "local-timeout", 120*time.Second, "per local attempt; slower counts as a failed attempt (PREREG)")
	fs.IntVar(&o.keyTokens, "local-max-tokens", 1024, "local max_tokens for key tasks")
	fs.IntVar(&o.repoTokens, "local-repo-max-tokens", 2048, "local max_tokens for repo tasks")
	fs.Int64Var(&o.claudeKeyTok, "claude-max-tokens", 4096, "Claude max_tokens for key tasks (thinking counts)")
	fs.Int64Var(&o.claudeRepoTok, "claude-repo-max-tokens", 8192, "Claude max_tokens for repo tasks")
	fs.Float64Var(&o.dailyCapUSD, "daily-cap-usd", 3, "hard Claude spend cap for the UTC day")
	fs.StringVar(&o.ledger, "ledger", "", "spend ledger (default: zhen-claude's)")
	fs.IntVar(&o.seed, "seed", 42, "local seed (temperature is 0)")
	fs.IntVar(&o.limit, "limit", 0, "stop after this many tasks (0 = all)")
	fs.IntVar(&o.localAttempts, "repo-attempts", 2, "local attempts for repo-task (the second sees the check output; PREREG)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	switch o.backend {
	case "local", "claude":
	default:
		return nil, fmt.Errorf("-backend %q: want local or claude", o.backend)
	}
	switch o.split {
	case "tune", "holdout", "all":
	default:
		return nil, fmt.Errorf("-split %q: want tune, holdout or all", o.split)
	}
	if o.out == "" {
		return nil, errors.New("-out is required")
	}
	if o.model == "" {
		if o.backend == "claude" {
			o.model = claudebridge.DefaultModel
		} else {
			o.model = localllm.DefaultModel
		}
	}
	return o, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintln(stderr, "zhen-eval:", err)
		return 2
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil)).With("service", "zhen-eval")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tasks, keys, prompts, err := loadAll(o)
	if err != nil {
		log.Error("load", "error", err.Error())
		return 1
	}
	var brain Brain
	switch o.backend {
	case "local":
		brain = &localBrain{c: &localllm.Client{BaseURL: o.localURL, Model: o.model, Timeout: o.localTimeout}, seed: o.seed, log: log}
	case "claude":
		cb, err := newClaudeBrain(ctx, o, log)
		if err != nil {
			log.Error("claude", "error", err.Error())
			return 1
		}
		brain = cb
	}
	work := o.workDir
	if work == "" {
		if work, err = os.MkdirTemp("", "zhen-eval-"); err != nil {
			log.Error("workdir", "error", err.Error())
			return 1
		}
		defer func() { _ = os.RemoveAll(work) }()
	}
	root, err := filepath.Abs(o.root)
	if err != nil {
		log.Error("root", "error", err.Error())
		return 1
	}
	rb := scriptTools{python: envOr("ZHEN_EVAL_PYTHON", "python3"), root: root, timeout: 60 * time.Second}
	e := &evaluator{o: o, brain: brain, keys: keys, prompts: prompts, work: work, check: newSandboxRunner(work), rb: rb, log: log}
	return e.runAll(ctx, tasks, stdout)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func loadJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-supplied eval file
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err) // strict: no silent drops
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

type gatePrompt struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Prompt   string `json:"prompt"`
}

// keyPrompt is what both brains are asked for a coding-gate prompt: the
// fixture's language, then the question (several textbook prompts name
// no language in their text; Step 142a).
func keyPrompt(p gatePrompt) string {
	if p.Language == "" {
		return p.Prompt
	}
	return "Language: " + p.Language + "\n\n" + p.Prompt
}

func loadAll(o *options) ([]Task, map[string]*Key, map[string]string, error) {
	join := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(o.root, p)
	}
	tasks, err := loadJSONL[Task](join(o.tasksPath))
	if err != nil {
		return nil, nil, nil, err
	}
	keyRows, err := loadJSONL[Key](join(o.keysPath))
	if err != nil {
		return nil, nil, nil, err
	}
	keys := map[string]*Key{}
	for i := range keyRows {
		k := keyRows[i]
		if err := k.compile(); err != nil {
			return nil, nil, nil, err
		}
		keys[k.ID] = &k
	}
	ps, err := loadJSONL[gatePrompt](join(o.promptsPath))
	if err != nil {
		return nil, nil, nil, err
	}
	prompts := map[string]string{}
	for _, p := range ps {
		prompts[p.ID] = keyPrompt(p)
	}
	want := map[string]bool{}
	for _, f := range strings.Split(o.families, ",") {
		if f = strings.TrimSpace(f); f != "" {
			want[f] = true
		}
	}
	var sel []Task
	for _, t := range tasks {
		if t.Judge != "key" && t.Judge != "check" && t.Judge != "runbook" {
			return nil, nil, nil, fmt.Errorf("task %s: unknown judge %q", t.ID, t.Judge)
		}
		if o.split != "all" && t.Split != o.split {
			continue
		}
		if len(want) > 0 && !want[t.Family] {
			continue
		}
		if t.Judge == "key" && (keys[gateID(t)] == nil || prompts[gateID(t)] == "") {
			return nil, nil, nil, fmt.Errorf("task %s has no key or prompt", t.ID)
		}
		if t.Judge == "runbook" {
			if _, err := loadRunbookTask(join(t.Ref)); err != nil {
				return nil, nil, nil, fmt.Errorf("task %s: %w", t.ID, err)
			}
		}
		sel = append(sel, t)
	}
	return sel, keys, prompts, nil
}

// gateID is the coding-gate prompt id a key task refers to.
func gateID(t Task) string {
	if i := strings.LastIndexByte(t.Ref, '#'); i >= 0 {
		return t.Ref[i+1:]
	}
	return t.ID
}

type evaluator struct {
	o       *options
	brain   Brain
	keys    map[string]*Key
	prompts map[string]string
	work    string
	check   CheckRunner
	rb      RunbookTools
	log     *slog.Logger
}

func (e *evaluator) done() map[string]bool {
	seen := map[string]bool{}
	if !e.o.skipDone {
		return seen
	}
	rows, err := loadJSONL[Result](e.o.out)
	if err != nil {
		return seen
	}
	for _, r := range rows {
		if r.Backend == e.brain.Name() && r.Model == e.brain.Model() && r.Error == "" {
			seen[r.Task] = true
		}
	}
	return seen
}

func (e *evaluator) runAll(ctx context.Context, tasks []Task, stdout io.Writer) int {
	f, err := os.OpenFile(e.o.out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- operator-chosen output file
	if err != nil {
		e.log.Error("open out", "error", err.Error())
		return 1
	}
	defer func() { _ = f.Close() }()
	seen := e.done()
	passes, n := 0, 0
	var spent claudebridge.NanoUSD
	code := 0
	for _, t := range tasks {
		if ctx.Err() != nil {
			code = 1
			break
		}
		if seen[t.ID] {
			continue
		}
		if e.o.limit > 0 && n >= e.o.limit {
			break
		}
		r := e.one(ctx, t)
		n++
		if r.Pass {
			passes++
		}
		spent += claudebridge.NanoUSD(r.CostNanoUSD)
		b, _ := json.Marshal(r)
		if _, err := f.Write(append(b, '\n')); err != nil {
			e.log.Error("write result", "error", err.Error())
			return 1
		}
		e.log.Info("task", "task", t.ID, "family", t.Family, "pass", r.Pass, "detail", r.Detail, "latency_ms", r.LatencyMS, "cost_usd", r.CostUSD)
		if errors.Is(errFromString(r.Error), claudebridge.ErrSpendCap) || strings.Contains(r.Error, "spend cap") {
			e.log.Warn("stopped", "reason", "daily spend cap reached; resume tomorrow with -skip-done")
			code = 3
			break
		}
	}
	_, _ = fmt.Fprintf(stdout, "%s/%s: %d/%d passed, spent %s\n", e.brain.Name(), e.brain.Model(), passes, n, spent.USD())
	return code
}

func errFromString(s string) error {
	if s == "" {
		return nil
	}
	return errors.New(s)
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// one runs a task: one attempt for key tasks; up to repo-attempts local
// attempts for check tasks (the next attempt is shown the check output),
// one for Claude.
func (e *evaluator) one(ctx context.Context, t Task) (r Result) {
	r = Result{Task: t.ID, Family: t.Family, Split: t.Split, Backend: e.brain.Name(), Model: e.brain.Model(), Judge: t.Judge,
		At: time.Now().UTC().Format(time.RFC3339)}
	if e.brain.Name() == "claude" {
		r.Effort = e.o.effort
	}
	t0 := time.Now()
	defer func() {
		r.LatencyMS = time.Since(t0).Milliseconds()
		r.CostUSD = claudebridge.NanoUSD(r.CostNanoUSD).USD()
	}()
	switch t.Judge {
	case "key":
		maxTok := e.o.keyTokens
		if e.brain.Name() == "claude" {
			maxTok = int(e.o.claudeKeyTok)
		}
		ans, err := e.brain.Ask(ctx, systemKey, e.prompts[gateID(t)], maxTok)
		e.account(&r, ans, err)
		if err != nil {
			r.Detail = "error"
			return r
		}
		e.keep(t, ans.Text)
		r.Pass, r.Detail = judgeKey(e.keys[gateID(t)], ans.Text)
		return r
	case "check":
		fixture := filepath.Join(e.o.root, t.Ref)
		prompt, err := repoPrompt(fixture)
		if err != nil {
			r.Error, r.Detail = err.Error(), "error"
			return r
		}
		attempts := 1
		if e.brain.Name() == "local" && e.o.localAttempts > 1 {
			attempts = e.o.localAttempts
		}
		maxTok := e.o.repoTokens
		if e.brain.Name() == "claude" {
			maxTok = int(e.o.claudeRepoTok)
		}
		ask := prompt
		for a := 1; a <= attempts; a++ {
			ans, err := e.brain.Ask(ctx, systemRepo, ask, maxTok)
			e.account(&r, ans, err)
			if err != nil {
				r.Details = append(r.Details, "error")
				r.Detail = "error"
				if ctx.Err() != nil || strings.Contains(err.Error(), "spend cap") {
					return r
				}
				continue
			}
			e.keep(t, ans.Text)
			cr := judgeCheck(ctx, fixture, e.work, ans.Text, e.check)
			r.Details = append(r.Details, cr.Detail)
			r.Detail = cr.Detail
			if cr.Pass {
				r.Pass, r.Error = true, ""
				return r
			}
			ask = prompt + "\n\nYour previous answer did not pass the check (" + cr.Detail + "). Check output:\n```\n" +
				truncate(cr.Output, 4096) + "\n```\nReply again with the complete corrected files."
		}
		return r
	case "runbook":
		task, err := loadRunbookTask(filepath.Join(e.o.root, t.Ref))
		if err != nil {
			r.Error, r.Detail = err.Error(), "error"
			return r
		}
		maxTok := e.o.repoTokens
		if e.brain.Name() == "claude" {
			maxTok = int(e.o.claudeRepoTok)
		}
		// One attempt for both brains (PREREG: local_attempts = 1 except repo-task).
		ans, err := e.brain.Ask(ctx, systemRunbook, task.prompt, maxTok)
		e.account(&r, ans, err)
		if err != nil {
			r.Detail = "error"
			return r
		}
		e.keep(t, ans.Text)
		rr := judgeRunbookDraft(ctx, task, e.work, ans.Text, e.rb)
		r.Pass, r.Detail, r.Judges = rr.Pass, rr.Detail, rr.Judges
		return r
	}
	r.Detail = "unknown judge"
	return r
}

func (e *evaluator) account(r *Result, a Answer, err error) {
	r.Attempts++
	r.AttemptMS = append(r.AttemptMS, a.Duration.Milliseconds())
	r.InputTokens += a.InputTokens
	r.OutputTokens += a.OutputTokens
	r.CostNanoUSD += int64(a.Cost)
	if a.ServedBy != "" {
		r.ServedBy = a.ServedBy
	}
	if err != nil {
		r.Error = truncate(err.Error(), 300)
		return
	}
	r.Error = ""
	r.AnswerSHA256, r.AnswerBytes = sha(a.Text), len(a.Text)
}

func (e *evaluator) keep(t Task, text string) {
	if e.o.answersDir == "" {
		return
	}
	_ = os.MkdirAll(e.o.answersDir, 0o700)
	name := filepath.Join(e.o.answersDir, e.brain.Name()+"-"+filepath.Base(t.ID)+".txt")
	_ = os.WriteFile(name, []byte(text), 0o600)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[truncated]"
}

// repoPrompt is TASK.md plus every fixture file a fix may touch (not the
// check, not the reference solution).
func repoPrompt(dir string) (string, error) {
	task, err := os.ReadFile(filepath.Join(dir, "TASK.md")) // #nosec G304 -- the repo's own fixture
	if err != nil {
		return "", err
	}
	var names []string
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if b := d.Name(); p != dir && (strings.HasPrefix(b, ".") || b == "__pycache__") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if protectedFiles[rel] {
			return nil
		}
		names = append(names, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(stripSPDXComment(string(task))))
	sb.WriteString("\n\nFiles in the fixture (check.sh runs the check):\n")
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n)) // #nosec G304 -- the repo's own fixture
		if err != nil {
			return "", err
		}
		sb.WriteString("\n```file:" + n + "\n" + string(b))
		if !strings.HasSuffix(string(b), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	}
	return sb.String(), nil
}

func stripSPDXComment(s string) string {
	if strings.HasPrefix(s, "<!--") {
		if i := strings.Index(s, "-->"); i >= 0 {
			return s[i+3:]
		}
	}
	return s
}
