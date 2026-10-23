// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/claudebridge/privacy"
	"unheaded/pkg/gpulease"
	"unheaded/pkg/zhenagent"
	"unheaded/pkg/zhenroute"
)

// TopicAgentRuns carries the worker's run and card changes (counts, ids,
// labels; never text).
const TopicAgentRuns = "zhen.agent.runs"

// workerFlags are the -worker options.
type workerFlags struct {
	enabled     bool
	once        bool
	poll        time.Duration
	base        string
	agentRoot   string
	reviewDir   string
	routing     string
	checks      string
	claude      string // fake or live
	model       string
	effort      string
	ledger      string
	dailyCapUSD float64
	privacyMode string
	localModel  string
	gpuLock     string

	overnight bool
	window    string
	maxCards  int
	stopFile  string
	reportDir string
}

func defaultStateDir() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "unheaded")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "unheaded")
}

func defaultAgentWTRoot() string {
	if v := os.Getenv("ZHEN_AGENT_WT_ROOT"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "unheaded", "zhen-worktrees")
}

func registerWorkerFlags(fs *flag.FlagSet, wf *workerFlags) {
	fs.BoolVar(&wf.enabled, "worker", false, "work eligible Kanban cards from the Well (owner zhen, a ```zhen block) into local zhen/<card> branches for review; never pushes (DSN from env WELL_APP_ZHEN_DSN)")
	fs.BoolVar(&wf.once, "once", false, "with -worker: work at most one card, print the result as JSON and exit (no HTTP listener)")
	fs.DurationVar(&wf.poll, "poll", 60*time.Second, "with -worker: time between polls for an eligible card")
	fs.StringVar(&wf.base, "base", "develop", "with -worker: local branch new zhen/<card> branches start from")
	fs.StringVar(&wf.agentRoot, "agent-root", defaultAgentWTRoot(), "with -worker: agent worktrees live here, outside the project root (env ZHEN_AGENT_WT_ROOT)")
	fs.StringVar(&wf.reviewDir, "review-dir", filepath.Join(defaultStateDir(), "zhen-reviews"), "with -worker: review notes (one Markdown file per run, 0600)")
	fs.StringVar(&wf.routing, "routing", "configs/zhen-routing.yaml", "with -worker: routing table (scripts/two-brains/thresholds.py)")
	fs.StringVar(&wf.checks, "checks", "configs/zhen-checks.yaml", "with -worker: the run_check allowlist")
	fs.StringVar(&wf.claude, "claude", "fake", "with -worker: the Claude leg: fake (no network call, changes nothing) or live (Claude API within -daily-cap-usd; credentials from the environment)")
	fs.StringVar(&wf.model, "claude-model", claudebridge.DefaultModel, "with -worker -claude live: Claude model ID")
	fs.StringVar(&wf.effort, "effort", "low", "with -worker -claude live: effort")
	fs.StringVar(&wf.ledger, "ledger", "", "with -worker -claude live: spend ledger (default zhen-claude's)")
	fs.Float64Var(&wf.dailyCapUSD, "daily-cap-usd", 5, "with -worker -claude live: hard spend cap per UTC day of -ledger")
	fs.StringVar(&wf.privacyMode, "privacy", "standard", "with -worker: privacy tier of the Claude leg: standard or strict")
	fs.BoolVar(&wf.overnight, "overnight", false, "run the worker unattended inside -window, then exit: its own ledger and cap (default $STATE/zhen-claude/spend-overnight.json, $3), -max-cards, a memory floor that never counts swap (ZHEN_MEM_FLOOR_BYTES may raise it), the -stop-file, strict privacy; writes a morning report to -report-dir")
	fs.StringVar(&wf.window, "window", "01:00-07:00", "with -overnight: local-time window HH:MM-HH:MM (may cross midnight)")
	fs.IntVar(&wf.maxCards, "max-cards", 5, "with -overnight: cards tried per night")
	fs.StringVar(&wf.stopFile, "stop-file", filepath.Join(defaultStateDir(), "zhen-agentd.stop"), "with -overnight: when this file exists the night stops (a card in flight finishes its check and commits nothing)")
	fs.StringVar(&wf.reportDir, "report-dir", filepath.Join(defaultStateDir(), "zhen-reports"), "with -overnight: morning reports (<date>.md, 0600)")
	fs.StringVar(&wf.gpuLock, "gpu-lock", gpulease.DefaultPath(), "GPU lease file shared with scripts/switch-model.sh and forge-train.sh (env GPU_LOCK); each local call holds it shared, a card waits while it is held exclusively; \"\" = no lease")
	fs.StringVar(&wf.localModel, "local-model", envOr("ZHEN_LOCAL_LLM_MODEL", claudebridge.DefaultLocalModel), "with -worker: local model name when the family names none")
}

// openWorkerStore opens zhen_agent_runs and the Kanban columns as app_zhen.
func openWorkerStore(ctx context.Context) (zhenagent.Store, func(), error) {
	dsn := strings.TrimSpace(os.Getenv("WELL_APP_ZHEN_DSN"))
	if dsn == "" {
		return nil, func() {}, errors.New("-worker needs WELL_APP_ZHEN_DSN (unheaded_app as app_zhen); cards come from the Well")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, func() {}, err
	}
	db.SetMaxOpenConns(3)
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		_ = db.Close()
		return nil, func() {}, fmt.Errorf("the Well is unreachable: %w", err)
	}
	return zhenagent.SQLStore{DB: db}, func() { _ = db.Close() }, nil
}

// agentdRunEvents records worker changes: audit_events (actor_type
// service: the daemon acting on Stevie's behalf, not a model tool call)
// and Wotan zhen.agent.runs.
type agentdRunEvents struct {
	aud *agentdAudit
	log *slog.Logger
}

func (e agentdRunEvents) RunEvent(_ context.Context, ev runEvent) {
	e.log.Info("worker_event", "kind", ev.Kind, "run", ev.Run, "card", ev.Card, "from", ev.From, "to", ev.To,
		"route_reason", ev.RouteReason, "check", ev.Check, "cost_nano_usd", ev.CostNanoUSD)
	if e.aud == nil {
		return
	}
	if e.aud.well != nil {
		e.aud.well.Event(runAuditEvent(ev))
	}
	if e.aud.wotan != nil {
		e.aud.wotan.PublishJSON(TopicAgentRuns, map[string]any{
			"ts": time.Now().UTC().Format(time.RFC3339Nano), "source": "zhen-agentd", "kind": ev.Kind, "run": ev.Run,
			"card": truncateID(ev.Card), "branch": ev.Branch, "mode": ev.Mode, "family": ev.Family, "from": ev.From, "to": ev.To,
			"route": ev.Route, "route_reason": ev.RouteReason, "check": ev.Check, "stop": ev.Stop, "cost_nano_usd": ev.CostNanoUSD,
			"infra": ev.Infra,
		})
	}
}

func truncateID(s string) string {
	if len(s) > 128 {
		return s[:128]
	}
	return s
}

func runAuditEvent(ev runEvent) *audit.AuditEvent {
	outcome := audit.OutcomeSuccess
	severity := audit.SeverityInfo
	switch ev.To {
	case zhenagent.StatusFailed, zhenagent.StatusAbandoned:
		outcome, severity = audit.OutcomeFailure, audit.SeverityWarning
	case zhenagent.StatusCancelled:
		outcome = audit.OutcomeFailure
	}
	resType, resID := "agent_run", ev.Run
	if ev.Kind == "card_move" {
		resType, resID = "kanban_card", truncateID(ev.Card)
	}
	return &audit.AuditEvent{
		Type: audit.EventTypeSystem, Severity: severity, Action: ev.Kind, Outcome: outcome,
		Actor:    &audit.Actor{ID: "zhen-agentd/worker", Type: "service"},
		Resource: &audit.Resource{ID: resID, Type: resType},
		Source:   &audit.Source{Service: "zhen-agentd", RequestID: ev.Run},
		Details: map[string]any{
			"run": ev.Run, "card": truncateID(ev.Card), "branch": ev.Branch, "mode": ev.Mode, "family": ev.Family,
			"from": ev.From, "to": ev.To, "route": ev.Route, "route_reason": ev.RouteReason, "check": ev.Check,
			"stop": ev.Stop, "cost_nano_usd": ev.CostNanoUSD, "infra": ev.Infra,
		},
	}
}

// buildPrivacyFilter is the worker's privacy tier (gitleaks for the
// secret scan; the local model summarises large ops payloads).
func buildPrivacyFilter(mode, localURL, localModel, gpuLock string) (*privacy.Filter, error) {
	r, err := privacy.NewRedactor(privacy.RedactorConfig{Scanner: os.Getenv("ZHEN_SECRET_SCANNER")})
	if err != nil {
		return nil, fmt.Errorf("%w (nothing is sent to Claude without the secret scan)", err)
	}
	if mode != "standard" && mode != "strict" {
		return nil, fmt.Errorf("privacy mode %q", mode)
	}
	local := &claudebridge.LocalLLM{BaseURL: localURL, Model: localModel, MaxTokens: privacy.DefaultSummaryTokens, GPULock: gpuLock}
	return &privacy.Filter{Redactor: r, Local: claudebridge.LocalCompleter{L: local}, Mode: privacy.Mode(mode)}, nil
}

// buildWorker assembles the worker from flags and the environment.
func buildWorker(ctx context.Context, wf workerFlags, root, llamaURL, mode string, aud *agentdAudit, log *slog.Logger) (*worker, func(), error) {
	store, closeStore, err := openWorkerStore(ctx)
	if err != nil {
		return nil, closeStore, err
	}
	routing, err := claudebridge.LoadRoutingConfig(wf.routing)
	if err != nil {
		return nil, closeStore, fmt.Errorf("-routing: %w", err)
	}
	checks, err := champion.LoadChecks(wf.checks)
	if err != nil {
		return nil, closeStore, fmt.Errorf("-checks: %w", err)
	}
	filter, ferr := buildPrivacyFilter(wf.privacyMode, llamaURL, wf.localModel, wf.gpuLock)
	rc := zhenroute.Config{
		Routing: routing, ChecksPath: wf.checks, LocalURL: llamaURL, LocalModel: wf.localModel,
		LocalMaxTokens: 4096, LocalTimeout: 10 * time.Minute, Privacy: filter, PrivacyErr: ferr, Auditor: aud.auditor, Log: log,
		GPULock: wf.gpuLock, LeaseTimeout: 30 * time.Second,
	}
	switch wf.claude {
	case "fake":
		rc.Claude = zhenroute.FakeClaude()
	case "live":
		if ferr != nil {
			return nil, closeStore, fmt.Errorf("-claude live: %w", ferr)
		}
		rc.Claude = zhenroute.LiveClaude(zhenroute.LiveClaudeOptions{Model: wf.model, Effort: wf.effort, MaxTokens: 4096,
			LedgerPath: wf.ledger, DailyCapUSD: wf.dailyCapUSD, Auditor: aud.auditor, Log: log})
	default:
		return nil, closeStore, fmt.Errorf("-claude %q: fake or live", wf.claude)
	}
	var gpuBusy func() bool
	if wf.gpuLock != "" {
		lock := wf.gpuLock
		gpuBusy = func() bool { return gpulease.Busy(lock) }
	}
	w, err := newWorker(workerConfig{
		Store: store, Runner: routeRunner{cfg: rc}, Repo: root, AgentRoot: wf.agentRoot, Base: wf.base, Mode: mode, GPUBusy: gpuBusy,
		Routing: routing, KnownCheck: func(k string) bool { _, ok := checks.Specs[k]; return ok }, ReviewDir: wf.reviewDir,
		Events: agentdRunEvents{aud: aud, log: log}, Log: log,
	})
	return w, closeStore, err
}

// resultJSON is -once's output line.
func resultJSON(r cardResult) string {
	skips := make([]map[string]string, 0, len(r.Skipped))
	for _, s := range r.Skipped {
		skips = append(skips, map[string]string{"card": s.Card, "reason": s.Reason})
	}
	b, _ := json.Marshal(map[string]any{
		"outcome": r.Outcome, "card": r.Card, "run": r.Run, "branch": r.Branch, "reason": r.Reason, "infra": r.Infra,
		"route": r.Route.Route, "route_reason": r.Route.RouteReason, "local_attempts": r.Route.LocalAttempts,
		"escalated": r.Route.Escalated, "check": r.Route.Check, "cost_usd": r.Route.Cost.USD(), "review_note": r.ReviewNote,
		"skipped": skips,
	})
	return string(b)
}

// workerLoop polls until ctx ends.
func workerLoop(ctx context.Context, w *worker, poll time.Duration, log *slog.Logger) {
	for {
		res, err := w.runOnce(ctx)
		if err != nil {
			log.Error("worker_poll", "error", err.Error())
		} else if res.Outcome != outcomeNone {
			log.Info("worker_result", "result", resultJSON(res))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(poll):
		}
	}
}
