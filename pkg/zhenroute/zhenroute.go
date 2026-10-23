// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package zhenroute runs one task through the threshold router
// (S-TWO-BRAINS Phases 5-6) inside a zhen/<name> agent worktree: the local
// model drafts file changes, an allowlisted check judges them in the
// sandbox, and a failing check escalates to Claude with the redacted check
// output, within the family's per-task budget (configs/zhen-routing.yaml).
//
// `zhen-claude -route auto -family F -worktree W` and `zhen-agentd -worker`
// both call Run in process; nothing here goes over HTTP, so no approval
// token or code can reach a model through this path.
package zhenroute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/claudebridge/privacy"
	"unheaded/pkg/sandbox"
)

// Task is one unit of routed work.
type Task struct {
	Family string // routing family (configs/zhen-routing.yaml)
	Prompt string // the task text; worktree files are appended for the local model

	Worktree  string // the zhen/<name> agent worktree ("" = text only, no files, no check)
	AgentRoot string // agent worktrees live under here

	Check     string // allowlisted check key ("" = no check)
	CheckPath string // check path relative to the worktree (default ".")
	// ContextPath limits the files listed for the local model to this
	// directory of the worktree ("" = the whole worktree).
	ContextPath string

	// LocalOnly switches Claude off whatever the family's route says.
	LocalOnly bool
	// MaxClaudeUSD lowers the family's per-task Claude budget (0 = the
	// family's own); it never raises it.
	MaxClaudeUSD float64
}

// ClaudeLeg runs the Claude leg of one task: the Champion's tools in the
// worktree, the privacy filter, and a spend cap of budget.
type ClaudeLeg func(ctx context.Context, in ClaudeInput) (claudebridge.ClaudeResult, error)

// ClaudeInput is what a ClaudeLeg gets.
type ClaudeInput struct {
	Champion *champion.Champion // over the worktree, branch-sandbox mode; nil for text-only tasks
	Root     string             // the worktree ("" for text-only)
	Branch   string
	Check    string
	Filter   *privacy.Filter
	Prompt   string
	Reason   string // route_reason of the leg
	Family   string
	Budget   claudebridge.NanoUSD
}

// Config is what Run needs besides the task.
type Config struct {
	Routing *claudebridge.RoutingConfig
	// ChecksPath is the run_check allowlist (configs/zhen-checks.yaml).
	ChecksPath string

	LocalURL       string
	LocalModel     string // used when the family names none
	LocalMaxTokens int
	LocalTimeout   time.Duration

	// Claude is the Claude leg (nil: no Claude, as for LocalOnly).
	Claude ClaudeLeg
	// Privacy redacts check output before it enters any prompt and filters
	// the Claude leg; PrivacyErr is why it could not be built (then
	// nothing is sent to Claude and check output is withheld).
	Privacy    *privacy.Filter
	PrivacyErr error

	Auditor claudebridge.Auditor
	// SandboxRun runs checks (nil: pkg/sandbox).
	SandboxRun func(ctx context.Context, cfg sandbox.Config, argv []string) (sandbox.Result, error)
	Log        *slog.Logger
}

// protectedFixtureFiles may not change for a check to count.
var protectedFixtureFiles = []string{"check.sh", "TASK.md", "solution.patch"}

// IsFixtureTest: with check fixture-check, a fixture's test and validator
// files are part of the check and protected like check.sh (a weakened
// assertion passes the check).
func IsFixtureTest(rel string) bool {
	b := filepath.Base(rel)
	return strings.HasSuffix(b, "_test.go") || (strings.HasPrefix(b, "test_") && strings.HasSuffix(b, ".py")) || b == "validate_runbook.py"
}

// protectedSet lists the worktree files a check depends on.
func protectedSet(ctx context.Context, root, checkPath, check string) []string {
	var out []string
	for _, n := range protectedFixtureFiles {
		out = append(out, filepath.Join(checkPath, n))
	}
	if check != "fixture-check" {
		return out
	}
	files, err := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z").Output() // #nosec G702 G204 -- fixed binary and flags; root is the validated agent worktree
	if err != nil {
		return out
	}
	for _, n := range strings.Split(string(files), "\x00") {
		if n != "" && IsFixtureTest(n) {
			out = append(out, n)
		}
	}
	return out
}

// LocalSystem is the local model's system prompt for worktree tasks.
const LocalSystem = "You fix small code tasks in a git worktree.\n" +
	"Reply with the complete new content of every file you change, each in its own fenced code block whose opening line is ```file:<path relative to the worktree> (for example ```file:lib.go).\n" +
	"Change only what the task needs. Never change check.sh, TASK.md or the tests."

// ClaudeSystem is the Claude leg's system prompt for worktree tasks.
func ClaudeSystem(root, branch, check, checkPath string) string {
	if checkPath == "" {
		checkPath = "."
	}
	return fmt.Sprintf(`You are fixing a task in a throwaway git worktree for Stevie Bellis's Unheaded project (GPL-3.0).
The worktree is %s, on branch %s; Stevie reviews the branch before anything merges, and nothing is pushed.
You may read files with read_file and change them with write_file or patch_file (absolute paths inside the worktree). You may run the allowlisted check with run_check (check %q, path %q).
Never change check.sh, TASK.md, the tests or anything under .git. Make the smallest change that makes the check pass, run the check, and stop when it passes. Answer briefly.`, root, branch, check, checkPath)
}

// FileSHA hashes root/rel through os.Root (no symlink can lead outside).
func FileSHA(root, rel string) string {
	r, err := os.OpenRoot(root)
	if err != nil {
		return "absent"
	}
	defer func() { _ = r.Close() }()
	b, err := r.ReadFile(rel)
	if err != nil {
		return "absent"
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// WorktreeFiles lists the tracked files of the worktree under dir ("" =
// all), at most 40 files and 64 KiB, for the local model's prompt.
func WorktreeFiles(ctx context.Context, root, dir string) (string, error) {
	args := []string{"-C", root, "ls-files", "-z"}
	if dir != "" && dir != "." {
		args = append(args, "--", dir)
	}
	out, err := exec.CommandContext(ctx, "git", args...).Output() // #nosec G204 -- fixed binary; root is the validated agent worktree, dir a validated relative path after --
	if err != nil {
		return "", fmt.Errorf("list worktree files: %w", err)
	}
	var names []string
	for _, n := range strings.Split(string(out), "\x00") {
		b := filepath.Base(n)
		if n == "" || b == "check.sh" || b == "solution.patch" || b == "TASK.md" {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	var sb strings.Builder
	total := 0
	for i, n := range names {
		if i >= 40 {
			sb.WriteString("\n(more files not shown)\n")
			break
		}
		b, err := r.ReadFile(n)
		if err != nil {
			continue
		}
		if total+len(b) > 64<<10 {
			sb.WriteString("\n(" + n + " and later files not shown: size limit)\n")
			break
		}
		total += len(b)
		sb.WriteString("\n```file:" + n + "\n" + string(b))
		if !strings.HasSuffix(string(b), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	}
	return sb.String(), nil
}

// ErrNotAgentWorktree: the worktree is not a zhen/<name> agent worktree.
var ErrNotAgentWorktree = errors.New("not an agent worktree on a zhen/<name> branch")

// Run routes one task. Errors are infrastructure failures (worktree,
// check runner, Claude transport); a failing check is an outcome.
func Run(ctx context.Context, cfg Config, t Task) (claudebridge.RouteOutcome, error) {
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if cfg.Routing == nil {
		return claudebridge.RouteOutcome{}, errors.New("zhenroute: no routing config")
	}
	if t.CheckPath == "" {
		t.CheckPath = "."
	}
	routing := *cfg.Routing
	fr := routing.For(t.Family)
	if t.LocalOnly {
		fr.Route = claudebridge.RouteLocalOnly
	}
	if t.MaxClaudeUSD > 0 && t.MaxClaudeUSD < fr.MaxClaudeUSD {
		fr.MaxClaudeUSD = t.MaxClaudeUSD
	}
	fams := make(map[string]claudebridge.FamilyRoute, len(routing.Families)+1)
	for k, v := range routing.Families {
		fams[k] = v
	}
	if _, ok := fams[t.Family]; ok || t.LocalOnly || t.MaxClaudeUSD > 0 {
		fams[t.Family] = fr
	}
	routing.Families = fams

	root, champ, err := worktreeChampion(ctx, cfg, t, log)
	if err != nil {
		return claudebridge.RouteOutcome{Family: t.Family, Route: fr.Route}, err
	}
	redact := func(ctx context.Context, s string) (string, error) {
		if cfg.PrivacyErr != nil || cfg.Privacy == nil {
			if cfg.PrivacyErr != nil {
				return "", cfg.PrivacyErr
			}
			return "", errors.New("zhenroute: no privacy filter")
		}
		out, _, err := cfg.Privacy.Prompt(ctx, s)
		return out, err
	}

	task := t.Prompt
	if root != "" {
		files, err := WorktreeFiles(ctx, root, t.ContextPath)
		if err != nil {
			return claudebridge.RouteOutcome{Family: t.Family, Route: fr.Route}, err
		}
		task = t.Prompt + "\n\nFiles in the worktree:\n" + files
	}
	protected := map[string]string{}
	protectedTests := map[string]bool{}
	if root != "" {
		for _, rel := range protectedSet(ctx, root, t.CheckPath, t.Check) {
			protected[rel] = FileSHA(root, rel)
			if IsFixtureTest(rel) {
				protectedTests[filepath.Clean(rel)] = true
			}
		}
	}

	localModel := fr.LocalModel
	if localModel == "" {
		localModel = cfg.LocalModel
	}
	seed := 42
	system := claudebridge.DefaultLocalSystem
	if root != "" {
		system = LocalSystem
	}
	local := &claudebridge.LocalLLM{BaseURL: cfg.LocalURL, Model: localModel, MaxTokens: max(cfg.LocalMaxTokens, 2048), Timeout: cfg.LocalTimeout, Seed: &seed}
	r := &claudebridge.ThresholdRouter{
		Config: &routing,
		Local: func(ctx context.Context, p string) (claudebridge.LocalAnswer, error) {
			return local.Complete(ctx, system, p)
		},
		Redact:  redact,
		Auditor: cfg.Auditor,
	}
	if root != "" {
		r.Apply = func(ctx context.Context, answer string) error {
			return ApplyDraft(ctx, champ, root, answer, protectedTests)
		}
	}
	if t.Check != "" {
		r.Check = func(ctx context.Context) (claudebridge.CheckOutcome, error) {
			for p, want := range protected {
				if FileSHA(root, p) != want {
					return claudebridge.CheckOutcome{Output: "protected file changed: " + filepath.Base(p)}, nil
				}
			}
			res, err := champ.RunCheck(ctx, t.Check, t.CheckPath, "")
			if err != nil {
				return claudebridge.CheckOutcome{}, err
			}
			return claudebridge.CheckOutcome{Pass: res.Pass, Output: res.Output}, nil
		}
	}
	if fr.Route != claudebridge.RouteLocalOnly && cfg.Claude != nil {
		if cfg.PrivacyErr != nil || cfg.Privacy == nil {
			return claudebridge.RouteOutcome{Family: t.Family, Route: fr.Route}, fmt.Errorf("privacy: %w", errors.Join(cfg.PrivacyErr, errors.New("nothing is sent to Claude without the privacy filter")))
		}
		branch := ""
		if champ != nil {
			branch = champ.BranchSandbox()
		}
		r.Claude = func(ctx context.Context, p, reason, family string, budget claudebridge.NanoUSD) (claudebridge.ClaudeResult, error) {
			return cfg.Claude(ctx, ClaudeInput{Champion: champ, Root: root, Branch: branch, Check: t.Check, Filter: cfg.Privacy,
				Prompt: p, Reason: reason, Family: family, Budget: budget})
		}
	}
	return r.Run(ctx, t.Family, task)
}

// worktreeChampion is the Champion over the agent worktree, with run_check
// and branch-sandbox mode; "" and nil without a worktree.
func worktreeChampion(ctx context.Context, cfg Config, t Task, log *slog.Logger) (string, *champion.Champion, error) {
	if t.Worktree == "" {
		if t.Check != "" {
			return "", nil, errors.New("zhenroute: a check needs a worktree")
		}
		return "", nil, nil
	}
	root, err := filepath.Abs(t.Worktree)
	if err != nil {
		return "", nil, err
	}
	champ, err := champion.New(champion.Config{ProjectRoot: root}, nil)
	if err != nil {
		return "", nil, err
	}
	if err := champ.WithBranchSandbox(t.AgentRoot); err != nil {
		return "", nil, fmt.Errorf("%w under %s: %w", ErrNotAgentWorktree, t.AgentRoot, err)
	}
	if t.Check != "" {
		checks, err := champion.LoadChecks(cfg.ChecksPath)
		if err != nil {
			return "", nil, fmt.Errorf("checks: %w", err)
		}
		env, ro, rw, err := champion.GoSandbox(ctx, filepath.Join(t.AgentRoot, ".cache", "go-build"))
		if err != nil {
			log.Warn("go_sandbox", "error", err.Error(), "note", "Go checks will not find a toolchain")
			env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8"}
		}
		champ.WithRunCheck(champion.RunCheckConfig{Checks: checks, AgentWorktreeRoot: t.AgentRoot, Env: env, ReadOnly: ro, Writable: rw, Run: cfg.SandboxRun})
	}
	return root, champ, nil
}

// ApplyDraft writes a local draft's file blocks through the Champion
// (branch-sandbox mode: Rules 1 and 3 hold, nothing under .git).
func ApplyDraft(ctx context.Context, champ *champion.Champion, root, answer string, protectedTests map[string]bool) error {
	blocks, err := claudebridge.ParseFileBlocks(answer)
	if err != nil {
		return err
	}
	for _, b := range blocks {
		rel := filepath.Clean(strings.TrimPrefix(b.Path, "./"))
		if filepath.IsAbs(b.Path) || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
			return fmt.Errorf("path %q is outside the worktree", b.Path)
		}
		for _, p := range protectedFixtureFiles {
			if filepath.Base(rel) == p {
				return fmt.Errorf("path %q may not be changed", b.Path)
			}
		}
		if protectedTests[rel] {
			return fmt.Errorf("path %q is part of the check and may not be changed", b.Path)
		}
		if _, err := champ.Dispatch(ctx, champion.ToolCall{
			Name: "write_file", Args: map[string]any{"path": filepath.Join(root, rel), "content": b.Content}, EmittedBy: "zhen-local/router",
		}); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}

// LiveClaudeOptions configures LiveClaude.
type LiveClaudeOptions struct {
	Model       string
	Effort      string
	MaxTokens   int64
	LedgerPath  string  // the daily ledger ("" = claudebridge.DefaultLedgerPath)
	DailyCapUSD float64 // that ledger's cap
	PricesPath  string  // "" = the embedded table
	Auditor     claudebridge.Auditor
	Log         *slog.Logger
}

// LiveClaude is the real Claude leg: the Champion's tools in the
// worktree, the privacy tier, a per-task ledger capped at min(budget, what
// the daily cap has left); its cost is then recorded in the daily ledger.
// Confirmation stays deny: Kanban and runbook tools are refused.
func LiveClaude(o LiveClaudeOptions) ClaudeLeg {
	return func(ctx context.Context, in ClaudeInput) (claudebridge.ClaudeResult, error) {
		log := o.Log
		if log == nil {
			log = slog.New(slog.DiscardHandler)
		}
		mainPath := o.LedgerPath
		if mainPath == "" {
			var err error
			if mainPath, err = claudebridge.DefaultLedgerPath(); err != nil {
				return claudebridge.ClaudeResult{}, err
			}
		}
		dailyCap, err := claudebridge.USDToNano(o.DailyCapUSD)
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		daily, err := claudebridge.NewLedger(mainPath, dailyCap)
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		spent, err := daily.Spent()
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		taskCap := min(in.Budget, daily.Cap()-spent)
		if taskCap <= 0 {
			return claudebridge.ClaudeResult{}, fmt.Errorf("routed escalation: %w", claudebridge.ErrSpendCap)
		}
		tmp, err := os.MkdirTemp("", "zhen-route-ledger-")
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		taskLedger, err := claudebridge.NewLedger(filepath.Join(tmp, "spend.json"), taskCap)
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		prices, err := claudebridge.DefaultPrices()
		if o.PricesPath != "" {
			prices, err = claudebridge.LoadPrices(o.PricesPath)
		}
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		var providers []claudebridge.NamedProvider
		champ := in.Champion
		if champ != nil {
			cp, err := champion.NewToolProvider(champ, "claude-api/"+o.Model)
			if err != nil {
				return claudebridge.ClaudeResult{}, err
			}
			providers = append(providers, claudebridge.NamedProvider{Name: "champion", Provider: cp})
		} else if champ, err = champion.New(champion.Config{ProjectRoot: os.TempDir()}, nil); err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		gate, err := claudebridge.NewChampionGate(champ)
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		system := claudebridge.DefaultSystem
		if in.Root != "" {
			system = ClaudeSystem(in.Root, in.Branch, in.Check, "")
		}
		maxTokens := fitMaxTokens(taskCap, prices.Max().Output, min(o.MaxTokens, 4096))
		if maxTokens == 0 {
			return claudebridge.ClaudeResult{}, fmt.Errorf("routed escalation: budget %s is under one request of %d output tokens: %w",
				taskCap.USD(), minLegTokens, claudebridge.ErrSpendCap)
		}
		b, err := claudebridge.New(ctx, claudebridge.Config{
			Model: o.Model, Effort: o.Effort, MaxTokens: maxTokens, DisableFallbacks: true, MaxIterations: 8,
			System: system, Providers: providers, Gate: gate, Prices: prices, Ledger: taskLedger,
			Auditor: o.Auditor, Confirmer: claudebridge.DenyConfirmer{}, Privacy: in.Filter, ProjectRoot: in.Root,
			Warn: func(m string) { log.Warn("privacy", "note", m) },
		})
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		conv := b.NewConversation()
		conv.SetRoute(in.Family, in.Reason)
		reply, sendErr := conv.Send(ctx, in.Prompt)
		var res claudebridge.ClaudeResult
		if reply != nil {
			res = claudebridge.ClaudeResult{Text: reply.Text, Cost: reply.Cost}
		}
		if res.Cost > 0 {
			if r, err := daily.Reserve(0); err == nil {
				if _, err := r.Commit(res.Cost); err != nil {
					log.Error("daily_ledger", "error", err.Error())
				}
			}
		}
		return res, sendErr
	}
}

// minLegTokens is the smallest max_tokens a Claude leg is worth sending.
const minLegTokens = 1024

// fitMaxTokens sizes max_tokens so that one request's output, at the
// price table's highest output rate, stays within 70 % of the task cap
// (the rest is for the input). The bridge reserves every request's worst
// case before sending it, so a family cap under one 4096-token request
// would otherwise refuse every Claude leg before anything was sent.
// 0 means not even minLegTokens fit.
func fitMaxTokens(capNano claudebridge.NanoUSD, maxOutRate, want int64) int64 {
	if maxOutRate <= 0 || capNano <= 0 {
		return 0
	}
	fit := int64(capNano) * 7 / 10 / maxOutRate
	n := min(want, fit)
	if n < minLegTokens {
		return 0
	}
	return n
}

// FakeClaude stands in for the Claude leg without any network call (dry
// runs and supervised nights): it changes nothing, costs nothing, and says
// so in its text.
func FakeClaude() ClaudeLeg {
	return func(_ context.Context, in ClaudeInput) (claudebridge.ClaudeResult, error) {
		return claudebridge.ClaudeResult{Text: "fake Claude: no change made (" + in.Reason + ")"}, nil
	}
}
