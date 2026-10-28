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
	"regexp"
	"time"
)

// RedactionVersion names the redaction rules a capture passed: the
// writer's pkg/zhencapture patterns (v1) plus this ingest's re-scan.
// Bump it when either changes, so old rows can be found and re-scanned.
const RedactionVersion = 1

// Capture sources (zhen_captures.source).
const (
	SourceZhenClaude = "zhen-claude"
	SourceLocalLLM   = "local_llm"
	SourceAgentd     = "zhen-agentd"
	SourceEval       = "zhen-eval"
)

// Author kinds (zhen_captures.author_kind) and backends.
const (
	AuthorLocal  = "local"
	AuthorClaude = "claude"
	AuthorHuman  = "human"
	BackendNone  = "none"
)

// Label kinds (zhen_capture_labels.kind).
const (
	LabelCheckPass = "check_pass"
	LabelCheckFail = "check_fail"
	LabelJudgePass = "judge_pass"
	LabelJudgeFail = "judge_fail"
	LabelEscalated = "escalated"
	LabelAccepted  = "accepted"
	LabelRejected  = "rejected"
	LabelHumanEdit = "human_edit"
)

// UnknownFamily is the family of a capture whose producer named none.
const UnknownFamily = "unknown"

var (
	familyRE  = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	runIDRE   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	captureRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Capture is one row of zhen_captures: one answer of one turn.
type Capture struct {
	ID               string
	CapturedAt       time.Time
	Source           string
	Backend          string
	Model            string
	AuthorKind       string
	TaskFamily       string
	ConversationID   string
	RunID            string // "" is NULL
	RecordID         string
	Schema           string
	RedactionVersion int
	Prompt           string
	Response         string
	ToolTrace        json.RawMessage // canonical JSON array
	Truncated        bool
	Promoted         bool
	Quarantined      bool
}

// Label is one row of zhen_capture_labels.
type Label struct {
	CaptureID string
	Kind      string
	Value     json.RawMessage // a JSON object
	SourceRef string
}

// CheckAction is a run_check row of zhen_actions.
type CheckAction struct {
	ID          int64
	SessionID   string
	PlannedAt   time.Time
	CompletedAt time.Time // zero when unset
	Status      string
	Summary     string // "exit=N pass=true|false"
}

// Escalation is one llm_requests conversation that escalated.
type Escalation struct {
	FirstID        int64
	ConversationID string
	RouteReason    string
}

// AgentRun is a reviewed zhen_agent_runs row.
type AgentRun struct {
	ID           string
	Status       string // accepted | rejected
	ReviewReason string
	BaseCommit   string
	HeadCommit   string
}

// ErrUnavailable means the source this read needs is not there (no ops DSN,
// or a table a later migration creates). The caller reports and skips.
var ErrUnavailable = errors.New("source unavailable")

// Store is the Well as zhen-flywheel sees it.
type Store interface {
	InsertCapture(ctx context.Context, c *Capture) (bool, error)
	InsertLabel(ctx context.Context, l Label) (bool, error)
	Captures(ctx context.Context) ([]Capture, error) // ordered by id
	Labels(ctx context.Context) ([]Label, error)
	CheckActions(ctx context.Context) ([]CheckAction, error)
	Escalations(ctx context.Context) ([]Escalation, error)
	AgentRuns(ctx context.Context) ([]AgentRun, error)
	Quarantine(ctx context.Context, id string) (bool, error)
	Promote(ctx context.Context, ids []string) error
}

// canonicalTrace re-encodes a tool trace so equal traces hash equally:
// decoded, then marshalled again (encoding/json sorts object keys at
// every depth). Empty means [].
func canonicalTrace(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("[]"), nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("tool trace: %w", err)
	}
	if _, ok := v.([]any); !ok {
		return nil, errors.New("tool trace is not a JSON array")
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("tool trace: %w", err)
	}
	return out, nil
}

// CaptureID is the dedupe key: sha256 of the canonical JSON of
// {model, prompt, response, tool_trace} (keys in that order, the trace
// canonicalised). Same content, same id, however often it is captured.
func CaptureID(model, prompt, response string, trace json.RawMessage) (string, error) {
	ct, err := canonicalTrace(trace)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(struct {
		Model     string          `json:"model"`
		Prompt    string          `json:"prompt"`
		Response  string          `json:"response"`
		ToolTrace json.RawMessage `json:"tool_trace"`
	}{model, prompt, response, ct})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// jsonObject marshals v (a map or struct) for a label value.
func jsonObject(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || b[0] != '{' {
		return json.RawMessage("{}")
	}
	return b
}
