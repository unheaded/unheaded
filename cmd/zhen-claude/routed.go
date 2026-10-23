// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/sandbox"
	"unheaded/pkg/zhenroute"
)

// `-route auto -family <f> -worktree <dir>` (S-TWO-BRAINS Step 178): one
// task through the threshold router (configs/zhen-routing.yaml). The
// local model drafts file changes into a zhen/<name> agent worktree, an
// allowlisted check judges them in the sandbox, and a failing check
// escalates to Claude with the check output, within the family's
// per-task budget. `-route local` is the same with Claude switched off.

// routedSandboxRun runs checks; nil means pkg/sandbox (tests replace it).
var routedSandboxRun func(ctx context.Context, cfg sandbox.Config, argv []string) (sandbox.Result, error)

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

// runRouted is -route auto|local with -family: one task through
// pkg/zhenroute, in process.
func runRouted(ctx context.Context, o *options, prompt string, stdout, stderr io.Writer, log *slog.Logger, aud *auditSetup) int {
	cfg, err := claudebridge.LoadRoutingConfig(o.routing)
	if err != nil {
		log.Error("routing", "error", err.Error())
		return 1
	}
	root := ""
	if o.worktree != "" {
		if root, err = filepath.Abs(o.worktree); err != nil {
			_, _ = fmt.Fprintln(stderr, "zhen-claude:", err)
			return 2
		}
	}
	filter, ferr := buildPrivacy(o, root)
	rc := zhenroute.Config{
		Routing: cfg, ChecksPath: o.checksPath,
		LocalURL: o.localURL, LocalModel: o.localModel, LocalMaxTokens: o.localMaxTokens, LocalTimeout: o.localTimeout,
		Privacy: filter, PrivacyErr: ferr, Auditor: aud.auditor, SandboxRun: routedSandboxRun, Log: log,
		Claude: zhenroute.LiveClaude(zhenroute.LiveClaudeOptions{
			Model: o.model, Effort: o.effort, MaxTokens: o.maxTokens, LedgerPath: o.ledgerPath, DailyCapUSD: o.dailyCapUSD,
			PricesPath: o.pricesPath, Auditor: aud.auditor, Log: log,
		}),
	}
	t0 := time.Now()
	out, err := zhenroute.Run(ctx, rc, zhenroute.Task{
		Family: o.family, Prompt: prompt, Worktree: root, AgentRoot: o.agentRoot,
		Check: o.runCheck, CheckPath: o.checkPath, LocalOnly: o.route == routeLocal,
	})
	if errors.Is(err, zhenroute.ErrNotAgentWorktree) {
		_, _ = fmt.Fprintf(stderr, "zhen-claude: -worktree must be an agent worktree on a zhen/<name> branch under %s: %v\n", o.agentRoot, err)
		return 2
	}
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
