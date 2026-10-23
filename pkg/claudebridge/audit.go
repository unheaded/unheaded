// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"log/slog"
	"time"
)

// RequestRecord summarises one Messages API request. It carries token
// counts and cost, never prompt or response text.
type RequestRecord struct {
	Conversation    string
	Iteration       int
	Model           string // requested
	ServedBy        string // model that produced the response (differs after a fallback)
	MessageID       string
	StopReason      string
	RefusalCategory string
	InputTokens     int64
	OutputTokens    int64
	CacheRead       int64
	CacheWrite      int64
	Reserved        NanoUSD
	Cost            NanoUSD
	DaySpent        NanoUSD
	UnpricedModels  []string // priced at the table maximum
	Duration        time.Duration
	Err             string // "" on success; set when refused before sending or failed
	Sent            bool   // false when refused before anything reached the API

	// Routing context (S-TWO-BRAINS). Zero values keep the old behaviour:
	// the auditor's own backend, no family, no route reason.
	Backend     string // "claude" or "local"; "" = the auditor's default
	TaskFamily  string // eval/routing family, e.g. "repo-task"
	RouteReason string // llm_requests.route_reason
	Redactions  int    // values the privacy tier redacted from this request
}

// ToolRecord summarises one tool call. The input is recorded by size and
// hash, not content: tool arguments can hold file bodies.
type ToolRecord struct {
	Conversation string
	ToolUseID    string
	Name         string
	Provider     string
	Decision     string // "allowed", "refused", "error", "unknown_tool", "bad_input"
	Reason       string // gate or tool error text
	Pending      bool   // refusal could be confirmed out of band
	IsError      bool   // sent to Claude as is_error
	InputBytes   int
	InputSHA256  string
	OutputBytes  int
	Truncated    bool
	Duration     time.Duration
	Model        string // model that asked for the call (actor in audit_events)
	Backend      string // "claude" or "local"; "" = the auditor's default
	Redactions   int    // values the privacy tier redacted from the result
	Tier         string // privacy tier of the result: T0, T1, T2 ("" = no filter)
	PrivacyMode  string // standard or strict ("" = no filter)
}

// Auditor receives every request and tool call. Implementations must be
// safe for concurrent use (tool calls in one turn run in parallel).
type Auditor interface {
	Request(ctx context.Context, r RequestRecord)
	ToolCall(ctx context.Context, r ToolRecord)
}

// SlogAuditor writes records as structured log lines.
type SlogAuditor struct{ Log *slog.Logger }

// Request implements Auditor.
func (a SlogAuditor) Request(ctx context.Context, r RequestRecord) {
	level := slog.LevelInfo
	if r.Err != "" {
		level = slog.LevelWarn
	}
	a.Log.LogAttrs(ctx, level, "claude_request",
		slog.String("conversation", r.Conversation),
		slog.Int("iteration", r.Iteration),
		slog.String("model", r.Model),
		slog.String("served_by", r.ServedBy),
		slog.String("message_id", r.MessageID),
		slog.String("stop_reason", r.StopReason),
		slog.String("refusal_category", r.RefusalCategory),
		slog.Int64("input_tokens", r.InputTokens),
		slog.Int64("output_tokens", r.OutputTokens),
		slog.Int64("cache_read_tokens", r.CacheRead),
		slog.Int64("cache_write_tokens", r.CacheWrite),
		slog.String("reserved_usd", r.Reserved.USD()),
		slog.String("cost_usd", r.Cost.USD()),
		slog.String("day_spent_usd", r.DaySpent.USD()),
		slog.Any("unpriced_models", r.UnpricedModels),
		slog.Duration("duration", r.Duration),
		slog.Bool("sent", r.Sent),
		slog.Int("redactions", r.Redactions),
		slog.String("error", r.Err),
	)
}

// ToolCall implements Auditor.
func (a SlogAuditor) ToolCall(ctx context.Context, r ToolRecord) {
	level := slog.LevelInfo
	if r.Decision != "allowed" {
		level = slog.LevelWarn
	}
	a.Log.LogAttrs(ctx, level, "claude_tool_call",
		slog.String("conversation", r.Conversation),
		slog.String("tool_use_id", r.ToolUseID),
		slog.String("tool", r.Name),
		slog.String("provider", r.Provider),
		slog.String("decision", r.Decision),
		slog.String("reason", r.Reason),
		slog.Bool("pending_confirmation", r.Pending),
		slog.Bool("is_error", r.IsError),
		slog.Int("input_bytes", r.InputBytes),
		slog.String("input_sha256", r.InputSHA256),
		slog.Int("output_bytes", r.OutputBytes),
		slog.Bool("truncated", r.Truncated),
		slog.Int("redactions", r.Redactions),
		slog.String("tier", r.Tier),
		slog.String("privacy_mode", r.PrivacyMode),
		slog.Duration("duration", r.Duration),
	)
}

// Confirmation implements ConfirmAuditor.
func (a SlogAuditor) Confirmation(ctx context.Context, r ConfirmRecord) {
	a.Log.LogAttrs(ctx, slog.LevelInfo, "claude_confirmation",
		slog.String("conversation", r.Conversation),
		slog.String("tool", r.Tool),
		slog.String("action", r.Action),
		slog.String("queue_id", r.QueueID),
		slog.String("call_sha256", r.CallSHA256),
		slog.String("decided_by", r.DecidedBy),
	)
}
