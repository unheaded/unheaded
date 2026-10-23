// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// `-route auto -family <f> -worktree <dir>` (S-TWO-BRAINS Step 178): one
// task through the threshold router (configs/zhen-routing.yaml). The
// local model drafts file changes into a zhen/<name> agent worktree, an
// allowlisted check judges them in the sandbox, and a failing check
// escalates to Claude with the check output, within the family's
// per-task budget. `-route local` is the same with Claude switched off.

// routedSandboxRun runs checks; nil means pkg/sandbox (tests replace it).
var routedSandboxRun func(ctx context.Context, cfg sandbox.Config, argv []string) (sandbox.Result, error)

// protectedFixtureFiles may not change for a check to count.
var protectedFixtureFiles = []string{"check.sh", "TASK.md", "solution.patch"}

// isFixtureTest: with -run-check fixture-check, a fixture's test and
// validator files are part of the check and protected like check.sh
// (a weakened assertion passes the check).
func isFixtureTest(rel string) bool {
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
		if n != "" && isFixtureTest(n) {
			out = append(out, n)
		}
	}
	return out
}

const routedLocalSystem = "You fix small code tasks in a git worktree.\n" +
	"Reply with the complete new content of every file you change, each in its own fenced code block whose opening line is ```file:<path relative to the worktree> (for example ```file:lib.go).\n" +
	"Change only what the task needs. Never change check.sh, TASK.md or the tests."

func routedClaudeSystem(root, branch, check string) string {
	return fmt.Sprintf(`You are fixing a task in a throwaway git worktree for Stevie Bellis's Unheaded project (GPL-3.0).
The worktree is %s, on branch %s; Stevie reviews the branch before anything merges, and nothing is pushed.
You may read files with read_file and change them with write_file or patch_file (absolute paths inside the worktree). You may run the allowlisted check with run_check (check %q, path ".").
Never change check.sh, TASK.md, the tests or anything under .git. Make the smallest change that makes the check pass, run the check, and stop when it passes. Answer briefly.`, root, branch, check)
}

func validateRoutedFlags(o *options) error {
	if o.family == "" {
		if o.worktree != "" || o.runCheck != "" {
			return errors.New("-worktree and -run-check need -family")
		}
		if o.route == routeLocal {
			return errors.New("-route local needs -family")
		}
		return nil
	}
	if o.route != routeAuto && o.route != routeLocal {
		return errors.New("-family needs -route auto or -route local")
	}
	if o.runCheck != "" && o.worktree == "" {
		return errors.New("-run-check needs -worktree")
	}
	if o.repl {
		return errors.New("-family works on one prompt, not -repl")
	}
	return nil
}

func defaultAgentRoot() string {
	if v := os.Getenv("ZHEN_AGENT_WT_ROOT"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "unheaded", "zhen-worktrees")
}

// fileSHA hashes root/rel through os.Root (no symlink can lead outside).
func fileSHA(root, rel string) string {
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

// worktreeFiles lists the tracked files of the worktree (git ls-files),
// at most 40 files and 64 KiB, for the local model's prompt.
func worktreeFiles(ctx context.Context, root string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z").Output() // #nosec G702 G204 -- fixed binary and flags; root is the validated agent worktree
	if err != nil {
		return "", fmt.Errorf("list worktree files: %w", err)
	}
	var names []string
	for _, n := range strings.Split(string(out), "\x00") {
		if n == "" || n == "check.sh" || n == "solution.patch" || n == "TASK.md" {
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

// runRouted is -route auto|local with -family.
func runRouted(ctx context.Context, o *options, prompt string, stdout, stderr io.Writer, log *slog.Logger, aud *auditSetup) int {
	cfg, err := claudebridge.LoadRoutingConfig(o.routing)
	if err != nil {
		log.Error("routing", "error", err.Error())
		return 1
	}
	fr := cfg.For(o.family)
	if o.route == routeLocal {
		fr.Route = claudebridge.RouteLocalOnly
		cfg.Families[o.family] = fr
	}
	root, champ, cleanup, err := routedChampion(ctx, o, log)
	defer cleanup()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude:", err)
		return 2
	}
	filter, ferr := buildPrivacy(o, root)
	redact := func(ctx context.Context, s string) (string, error) {
		if ferr != nil {
			return "", ferr
		}
		out, _, err := filter.Prompt(ctx, s)
		return out, err
	}

	task := prompt
	if root != "" {
		files, err := worktreeFiles(ctx, root)
		if err != nil {
			log.Error("worktree", "error", err.Error())
			return 1
		}
		task = prompt + "\n\nFiles in the worktree:\n" + files
	}
	protected := map[string]string{}
	protectedTests := map[string]bool{}
	if root != "" {
		for _, rel := range protectedSet(ctx, root, o.checkPath, o.runCheck) {
			protected[rel] = fileSHA(root, rel)
			if isFixtureTest(rel) {
				protectedTests[filepath.Clean(rel)] = true
			}
		}
	}

	localModel := fr.LocalModel
	if localModel == "" {
		localModel = o.localModel
	}
	seed := 42
	system := claudebridge.DefaultLocalSystem
	if root != "" {
		system = routedLocalSystem
	}
	local := &claudebridge.LocalLLM{BaseURL: o.localURL, Model: localModel, MaxTokens: max(o.localMaxTokens, 2048), Timeout: o.localTimeout, Seed: &seed}
	r := &claudebridge.ThresholdRouter{
		Config: cfg,
		Local: func(ctx context.Context, p string) (claudebridge.LocalAnswer, error) {
			return local.Complete(ctx, system, p)
		},
		Redact:  redact,
		Auditor: aud.auditor,
	}
	if root != "" {
		r.Apply = func(ctx context.Context, answer string) error {
			return applyDraft(ctx, champ, root, answer, protectedTests)
		}
	}
	if o.runCheck != "" {
		r.Check = func(ctx context.Context) (claudebridge.CheckOutcome, error) {
			for p, want := range protected {
				if fileSHA(root, p) != want {
					return claudebridge.CheckOutcome{Output: "protected file changed: " + filepath.Base(p)}, nil
				}
			}
			res, err := champ.RunCheck(ctx, o.runCheck, o.checkPath, "")
			if err != nil {
				return claudebridge.CheckOutcome{}, err
			}
			return claudebridge.CheckOutcome{Pass: res.Pass, Output: res.Output}, nil
		}
	}
	if fr.Route != claudebridge.RouteLocalOnly {
		if ferr != nil {
			log.Error("privacy", "error", ferr.Error())
			return 1
		}
		r.Claude = func(ctx context.Context, p, reason, family string, budget claudebridge.NanoUSD) (claudebridge.ClaudeResult, error) {
			return routedClaude(ctx, o, champ, root, filter, aud, log, p, reason, family, budget)
		}
	}
	t0 := time.Now()
	out, err := r.Run(ctx, o.family, task)
	if err != nil {
		log.Error("route", "error", err.Error())
	}
	line := map[string]any{
		"family": out.Family, "route": out.Route, "route_reason": out.RouteReason, "local_attempts": out.LocalAttempts,
		"escalated": out.Escalated, "check": out.Check, "cost_usd": out.CostUSD, "stop": out.Stop,
		"latency_ms": time.Since(t0).Milliseconds(),
	}
	if err != nil {
		line["error"] = err.Error()
	}
	b, _ := json.Marshal(line)
	_, _ = fmt.Fprintln(stdout, string(b))
	log.Info("route_decision", "family", out.Family, "route", out.Route, "route_reason", out.RouteReason,
		"local_attempts", out.LocalAttempts, "escalated", out.Escalated, "check", out.Check, "cost_usd", out.CostUSD, "stop", out.Stop)
	if err != nil || out.Check == "fail" || out.Check == "error" || (out.Check == "" && out.Text == "") {
		return 1
	}
	return 0
}

// routedChampion is the Champion over the agent worktree, with run_check
// and branch-sandbox mode; without -worktree, a read-only Champion over
// the project root.
func routedChampion(ctx context.Context, o *options, log *slog.Logger) (string, *champion.Champion, func(), error) {
	noop := func() {}
	if o.worktree == "" {
		return "", nil, noop, nil
	}
	root, err := filepath.Abs(o.worktree)
	if err != nil {
		return "", nil, noop, err
	}
	agent := o.agentRoot
	champ, err := champion.New(champion.Config{ProjectRoot: root}, nil)
	if err != nil {
		return "", nil, noop, err
	}
	if err := champ.WithBranchSandbox(agent); err != nil {
		return "", nil, noop, fmt.Errorf("-worktree must be an agent worktree on a zhen/<name> branch under %s: %w", agent, err)
	}
	if o.runCheck != "" {
		checks, err := champion.LoadChecks(o.checksPath)
		if err != nil {
			return "", nil, noop, fmt.Errorf("-checks: %w", err)
		}
		env, ro, rw, err := champion.GoSandbox(ctx, filepath.Join(agent, ".cache", "go-build"))
		if err != nil {
			log.Warn("go_sandbox", "error", err.Error(), "note", "Go checks will not find a toolchain")
			env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8"}
		}
		champ.WithRunCheck(champion.RunCheckConfig{Checks: checks, AgentWorktreeRoot: agent, Env: env, ReadOnly: ro, Writable: rw, Run: routedSandboxRun})
	}
	return root, champ, noop, nil
}

// applyDraft writes the local draft's file blocks through the Champion
// (branch-sandbox mode: Rules 1 and 3 hold, nothing under .git).
func applyDraft(ctx context.Context, champ *champion.Champion, root, answer string, protectedTests map[string]bool) error {
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
			if filepath.Base(rel) == p && filepath.Dir(rel) == "." {
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

// routedClaude is one Claude leg: the Champion's tools in the worktree,
// the privacy tier, and a per-task ledger capped at min(budget, what the
// daily cap has left); its cost is then recorded in the daily ledger.
func routedClaude(ctx context.Context, o *options, champ *champion.Champion, root string, filter *privacy.Filter, aud *auditSetup,
	log *slog.Logger, prompt, reason, family string, budget claudebridge.NanoUSD) (claudebridge.ClaudeResult, error) {
	mainPath := o.ledgerPath
	if mainPath == "" {
		var err error
		if mainPath, err = claudebridge.DefaultLedgerPath(); err != nil {
			return claudebridge.ClaudeResult{}, err
		}
	}
	dailyCap, err := claudebridge.USDToNano(o.dailyCapUSD)
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
	taskCap := min(budget, daily.Cap()-spent)
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
	if o.pricesPath != "" {
		prices, err = claudebridge.LoadPrices(o.pricesPath)
	}
	if err != nil {
		return claudebridge.ClaudeResult{}, err
	}
	var providers []claudebridge.NamedProvider
	branch := ""
	if champ != nil {
		cp, err := champion.NewToolProvider(champ, "claude-api/"+o.model)
		if err != nil {
			return claudebridge.ClaudeResult{}, err
		}
		providers = append(providers, claudebridge.NamedProvider{Name: "champion", Provider: cp})
		branch = champ.BranchSandbox()
	} else if champ, err = champion.New(champion.Config{ProjectRoot: os.TempDir()}, nil); err != nil {
		return claudebridge.ClaudeResult{}, err
	}
	gate, err := claudebridge.NewChampionGate(champ)
	if err != nil {
		return claudebridge.ClaudeResult{}, err
	}
	system := claudebridge.DefaultSystem
	if root != "" {
		system = routedClaudeSystem(root, branch, o.runCheck)
	}
	b, err := claudebridge.New(ctx, claudebridge.Config{
		Model: o.model, Effort: o.effort, MaxTokens: min(o.maxTokens, 4096), DisableFallbacks: true, MaxIterations: 8,
		System: system, Providers: providers, Gate: gate, Prices: prices, Ledger: taskLedger,
		Auditor: aud.auditor, Confirmer: claudebridge.DenyConfirmer{}, Privacy: filter, ProjectRoot: root,
		Warn: func(m string) { log.Warn("privacy", "note", m) },
	})
	if err != nil {
		return claudebridge.ClaudeResult{}, err
	}
	conv := b.NewConversation()
	conv.SetRoute(family, reason)
	reply, sendErr := conv.Send(ctx, prompt)
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
