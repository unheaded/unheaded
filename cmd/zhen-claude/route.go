// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/zhencapture"
)

// Why a -route auto turn went where it went (route_decision "why").
const (
	whyLocalOK        = "local_ok_no_check" // no -check: a local answer is accepted
	whyCheckPassed    = "check_passed"
	whyCheckFailed    = "check_failed" // non-zero exit
	whyCheckError     = "check_error"  // did not run, or timed out
	whyLocalError     = "local_error"  // down, HTTP error, bad or empty answer
	whyLocalTruncated = "local_truncated"
)

// runAuto is -route auto for one prompt: local first, Claude only when
// the local answer is missing, cut off, or rejected by -check. The
// Claude side (credentials, MCP server, spend ledger) is built only then.
func runAuto(ctx context.Context, o *options, prompt string, stdout, stderr io.Writer, log *slog.Logger, capw *zhencapture.Writer, tools *toolCollector) int {
	t0 := time.Now()
	local := &claudebridge.LocalLLM{BaseURL: o.localURL, Model: o.localModel, MaxTokens: o.localMaxTokens, Timeout: o.localTimeout}
	ans, lerr := local.Complete(ctx, claudebridge.DefaultLocalSystem, prompt)
	localDur := time.Since(t0)

	rec := &zhencapture.Record{Source: "zhen-claude", Prompt: prompt, LocalOnly: o.localOnly}
	rec.Local = &zhencapture.Leg{Model: ans.Model, System: claudebridge.DefaultLocalSystem, Answer: ans.Text, StopReason: ans.FinishReason, LatencyMS: localDur.Milliseconds()}
	if rec.Local.Model == "" {
		rec.Local.Model = o.localModel
	}
	attrs := []any{
		"route", routeAuto, "local_tried", true, "local_url", o.localURL, "local_model", rec.Local.Model,
		"local_latency_ms", localDur.Milliseconds(), "local_answer_bytes", len(ans.Text), "local_only", o.localOnly,
	}

	var why string
	accepted := false
	switch {
	case errors.Is(lerr, claudebridge.ErrLocalTruncated):
		why, rec.Local.Error = whyLocalTruncated, lerr.Error()
	case lerr != nil:
		why, rec.Local.Error = whyLocalError, lerr.Error()
	case len(o.checkArgv) == 0:
		why, accepted = whyLocalOK, true
	default:
		chk := claudebridge.RunCheck(ctx, o.checkArgv, checkDir(o), ans.Text, o.checkTimeout)
		rec.Check = &zhencapture.Check{Argv: chk.Argv, Exit: chk.Exit, Accepted: chk.Accepted, Error: chk.Err, LatencyMS: chk.Duration.Milliseconds()}
		attrs = append(attrs, "check_argv", chk.Argv, "check_exit", chk.Exit, "check_latency_ms", chk.Duration.Milliseconds())
		switch {
		case chk.Accepted:
			why, accepted = whyCheckPassed, true
		case chk.Err != "":
			why = whyCheckError
			attrs = append(attrs, "check_error", chk.Err)
		default:
			why = whyCheckFailed
		}
	}
	attrs = append(attrs, "local_ok", lerr == nil, "why", why)
	if rec.Local.Error != "" {
		attrs = append(attrs, "local_error", rec.Local.Error)
	}

	if accepted {
		_, _ = io.WriteString(stdout, ans.Text)
		_, _ = io.WriteString(stdout, "\n")
		rec.Route, rec.System, rec.Final, rec.AcceptedBy = zhencapture.RouteLocal, claudebridge.DefaultLocalSystem, ans.Text, "local"
		rec.LatencyMS = time.Since(t0).Milliseconds()
		log.Info("route_decision", append(attrs, "escalated", false, "decision", "local", "claude_cost_usd", "0", "total_latency_ms", rec.LatencyMS)...)
		writeCapture(capw, rec, log)
		return 0
	}
	if ctx.Err() != nil {
		log.Warn("route_decision", append(attrs, "escalated", false, "decision", "none", "cancelled", true)...)
		return 1
	}
	if o.localOnly {
		rec.Route = zhencapture.RouteLocal
		rec.LatencyMS = time.Since(t0).Milliseconds()
		log.Warn("route_decision", append(attrs, "escalated", false, "decision", "none", "total_latency_ms", rec.LatencyMS)...)
		_, _ = fmt.Fprintf(stderr, "zhen-claude: local answer not accepted (%s); -local-only: not sending it to Claude\n", why)
		writeCapture(capw, rec, log)
		return 1
	}

	// Escalate. Spend cap, gate and audit apply from here on, as for -route claude.
	rec.Route = zhencapture.RouteEscalated
	b, closeAll, err := build(ctx, o, log, stdout, tools)
	defer closeAll()
	if err != nil {
		rec.Claude = &zhencapture.Leg{Model: o.model, Error: err.Error()}
		rec.LatencyMS = time.Since(t0).Milliseconds()
		log.Error("route_decision", append(attrs, "escalated", true, "decision", "none", "claude_error", err.Error(), "total_latency_ms", rec.LatencyMS)...)
		_, _ = fmt.Fprintf(stderr, "zhen-claude: local answer not accepted (%s) and Claude is unavailable: %v\n", why, err)
		writeCapture(capw, rec, log)
		return 1
	}
	rec.System = b.System()
	c0 := time.Now()
	reply, err := turn(ctx, b.NewConversation(), prompt, stdout, stderr, log)
	fillClaude(rec, b, reply, err, time.Since(c0), tools)
	rec.LatencyMS = time.Since(t0).Milliseconds()
	decision := "claude"
	if err != nil {
		decision = "none"
	}
	attrs = append(attrs, "escalated", true, "decision", decision, "claude_ok", err == nil,
		"claude_latency_ms", time.Since(c0).Milliseconds(), "claude_cost_usd", rec.CostUSD, "total_latency_ms", rec.LatencyMS)
	if err != nil {
		attrs = append(attrs, "claude_error", err.Error())
	}
	log.Info("route_decision", attrs...)
	writeCapture(capw, rec, log)
	if err != nil {
		return 1
	}
	return 0
}

// checkDir is where -check runs: the Champion's project root, else cwd.
func checkDir(o *options) string {
	if o.projectRoot != "" {
		return o.projectRoot
	}
	wd, _ := os.Getwd()
	return wd
}

func dryRunLocal(o *options, prompt string, stdout io.Writer, log *slog.Logger) int {
	local := &claudebridge.LocalLLM{BaseURL: o.localURL, Model: o.localModel, MaxTokens: o.localMaxTokens}
	endpoint, err := local.Endpoint()
	if err != nil {
		log.Error("dry_run", "error", err.Error())
		return 1
	}
	body, err := local.RequestJSON(claudebridge.DefaultLocalSystem, prompt)
	if err != nil {
		log.Error("dry_run", "error", err.Error())
		return 1
	}
	check := "(none: only a local failure escalates)"
	if len(o.checkArgv) > 0 {
		check = strings.Join(o.checkArgv, " ")
	}
	then := "# then, if escalated:\n"
	if o.localOnly {
		then = "# local-only: never sent to Claude\n"
	}
	_, _ = fmt.Fprintf(stdout, "# local first\nPOST %s\n\n%s\n\n# check: %s\n%s", endpoint, body, check, then)
	return 0
}

// fillClaude records the Claude leg (answer, cost, tool calls) in rec.
func fillClaude(rec *zhencapture.Record, b *claudebridge.Bridge, reply *claudebridge.Reply, err error, d time.Duration, tools *toolCollector) {
	leg := &zhencapture.Leg{Model: b.Model(), LatencyMS: d.Milliseconds()}
	if reply != nil {
		leg.ServedBy, leg.StopReason, leg.Requests = reply.ServedBy, reply.StopReason, reply.Requests
		leg.CostUSD = reply.Cost.USD()
		rec.CostUSD = leg.CostUSD
		if err == nil {
			leg.Answer = reply.Text
		}
	}
	if err != nil {
		leg.Error = err.Error()
	} else {
		rec.Final, rec.AcceptedBy = leg.Answer, "claude"
	}
	rec.Claude = leg
	rec.ToolCalls = tools.take()
}

// captureClaudeTurn records a -route claude turn when capture is on.
func captureClaudeTurn(capw *zhencapture.Writer, b *claudebridge.Bridge, prompt string, reply *claudebridge.Reply, err error, d time.Duration, tools *toolCollector, log *slog.Logger) {
	if capw == nil {
		tools.take()
		return
	}
	rec := &zhencapture.Record{Source: "zhen-claude", Route: zhencapture.RouteClaude, System: b.System(), Prompt: prompt}
	fillClaude(rec, b, reply, err, d, tools)
	rec.LatencyMS = d.Milliseconds()
	writeCapture(capw, rec, log)
}

func openCapture(o *options, log *slog.Logger) (*zhencapture.Writer, error) {
	if o.captureDir == "" {
		return nil, nil
	}
	w, err := zhencapture.Open(o.captureDir)
	if err != nil {
		return nil, fmt.Errorf("-capture: %w", err)
	}
	log.Info("capture_on", "dir", w.Dir(), "schema", zhencapture.Schema)
	return w, nil
}

func writeCapture(w *zhencapture.Writer, rec *zhencapture.Record, log *slog.Logger) {
	if w == nil {
		return
	}
	if err := w.Write(rec); err != nil {
		log.Warn("capture_failed", "error", err.Error())
		return
	}
	log.Debug("captured", "id", rec.ID, "route", rec.Route, "redactions", rec.Redactions, "truncated", rec.Truncated)
}

// toolCollector gathers one turn's tool calls for capture. It holds
// content only while capture is on; add is a no-op otherwise.
type toolCollector struct {
	mu    sync.Mutex
	on    bool
	calls []zhencapture.ToolCall
}

func (c *toolCollector) add(tr claudebridge.ToolTrace) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.on {
		return
	}
	out, _ := zhencapture.Truncate(tr.Output, captureToolOutputBytes)
	c.calls = append(c.calls, zhencapture.ToolCall{
		Name: tr.Record.Name, Provider: tr.Record.Provider, Decision: tr.Record.Decision,
		Input: tr.Input, Output: out, IsError: tr.Record.IsError,
	})
}

func (c *toolCollector) take() []zhencapture.ToolCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.calls
	c.calls = nil
	return out
}
