// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
)

// POST /api/v1/tool/exec — direct Champion tool dispatch.
//
// WAVE15 Phase 2b: the explicit-mutation path. The Python web UI
// hits this when the user clicks "Execute runbook X" or any other
// kingdom-altering button — bypasses the LLM entirely (no
// retrieval, no inference) and runs the named tool through
// pkg/champion.Dispatch with the caller's supplied justification.
//
// Distinct from /api/v1/agent/ask: ask is goal-driven (LLM picks the
// tool); /tool/exec is action-driven (caller picks the tool, gate
// still enforces the three rules).
//
// The justification field IS load-bearing. The web UI asserts
// `source_trust: "direct-user"` to mark "the user explicitly
// requested this in the browser" — Champion's gate (Rule 2)
// recognizes this trust label as the direct-user escape hatch
// and does NOT require a retrieval-derived chain.
//
// Wire format:
//
//	POST /api/v1/tool/exec
//	{
//	  "tool":          "runbook_execute",
//	  "args":          {"name": "observe/service-health-sweep"},
//	  "project_root":  "/home/govan/tmp/unheaded",  // optional
//	  "session_id":    "...",                        // optional, for audit
//	  "emitted_by":    "zhen-web-ui",                // optional, default "direct"
//	  "justification": [                              // required unless emitted_by is an agent loop
//	    {"topic": "user-clicked-runbook", "source_trust": "direct-user"}
//	  ]
//	}
//
// Response shapes mirror /api/v1/agent/confirm so the client can
// reuse the same handling logic:
//
//	200 OK  → {"status": "ok",                    "result": <tool output>}
//	400     → {"status": "unknown"|"used"|"expired"|"error", "reason": "..."}
//	400     → {"status": "denied", "reason": "justification required: ..."} (direct caller sent none)
//	403     → {"status": "denied",                "reason": "..."}
//	403     → {"status": "pending_confirmation",  "pending_token": "...", "reason": "..."}
//	202     → {"status": "queued",                "queue_id": "<16 hex>", "reason": "..."}
//
// A refused-pending call from an agent loop (emitted_by starting with
// "zhen-agent") never gets a token: the caller is a model, and a token
// returned to it would let it approve itself. It is queued in the Well
// instead (-confirm-queue), for Stevie to decide with
// `zhen-claude confirm approve <id>`; without the queue it is denied.
// Only direct callers (the web UI, emitted_by "direct" or a UI name) get
// a pending_token for /api/v1/agent/confirm.
//
// Note: this endpoint does NOT auto-redeem its own pending tokens.
// If Champion's gate refuses with rule-2, the response carries the
// pending_token and the web UI should surface the consent UX before
// hitting /api/v1/agent/confirm.
type toolExecRequest struct {
	Tool          string               `json:"tool"`
	Args          map[string]any       `json:"args"`
	ProjectRoot   string               `json:"project_root,omitempty"`
	SessionID     string               `json:"session_id,omitempty"`
	EmittedBy     string               `json:"emitted_by,omitempty"`
	Justification []champion.Reference `json:"justification,omitempty"`
}

type toolExecResponse struct {
	Status       string `json:"status"` // "ok" | "denied" | "pending_confirmation" | "unknown" | "used" | "expired" | "error"
	Result       any    `json:"result,omitempty"`
	Reason       string `json:"reason,omitempty"`
	PendingToken string `json:"pending_token,omitempty"`
	QueueID      string `json:"queue_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

// isAgentLoop reports whether a /tool/exec caller is a model loop.
func isAgentLoop(emittedBy string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(emittedBy)), "zhen-agent")
}

func (s *server) handleToolExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16)) // 64 KiB
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body: %v", err)
		return
	}
	defer r.Body.Close()

	var req toolExecRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode body: %v", err)
		return
	}
	req.Tool = strings.TrimSpace(req.Tool)
	if req.Tool == "" {
		writeJSONError(w, http.StatusBadRequest, "missing %q", "tool")
		return
	}
	if req.Args == nil {
		req.Args = map[string]any{}
	}

	// Resolve + allow-check the project_root (same posture as /ask).
	rootReq := strings.TrimSpace(req.ProjectRoot)
	if rootReq == "" {
		rootReq = s.defaultRoot
	}
	rootAbs, err := filepath.Abs(rootReq)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "resolve project_root: %v", err)
		return
	}
	if _, ok := s.allowed[rootAbs]; !ok {
		writeJSONError(w, http.StatusForbidden,
			"project_root %q is not in the daemon's allow-list", rootAbs)
		return
	}

	// The caller states the justification. A direct caller (the web UI,
	// zhen-cli) that clicked or typed the action sends
	// `source_trust: "direct-user"`, Champion's Rule 2 escape (see
	// pkg/champion/toolcall.go HasUntrustedJustification). The daemon no
	// longer asserts it on a caller's behalf (M-019): a request without
	// one is refused before Champion sees it.
	justification := req.Justification
	agentLoop := isAgentLoop(req.EmittedBy)
	if agentLoop {
		// A model loop is never the direct user: it cannot assert
		// direct-user (S-TWO-BRAINS Step 86). Its mutating calls are
		// refused pending confirmation (Rule 2); with no references at
		// all its reads still run and its writes still queue.
		kept := justification[:0:0]
		for _, ref := range justification {
			if ref.SourceTrust != "direct-user" {
				kept = append(kept, ref)
			}
		}
		justification = kept
	}
	if len(justification) == 0 && !agentLoop {
		writeJSON(w, http.StatusBadRequest, toolExecResponse{
			Status:    "denied",
			Reason:    `justification required: send a "justification" array (a direct user action is {"source_trust": "direct-user", ...})`,
			SessionID: req.SessionID,
		})
		return
	}

	emittedBy := strings.TrimSpace(req.EmittedBy)
	if emittedBy == "" {
		emittedBy = "direct"
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("toolexec-%s", time.Now().UTC().Format("20060102T150405Z"))
	}

	champ, err := s.pool.get(rootAbs)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "champion init: %v", err)
		return
	}

	tc := champion.ToolCall{
		Name:          req.Tool,
		Args:          req.Args,
		Justification: justification,
		EmittedBy:     emittedBy,
		Principal:     requestPrincipal(r), // RT-03: a pending token is bound to it
	}

	s.dispatchAndRespond(w, r, champ, tc, req.SessionID)
}

// dispatchAndRespond runs tc through the Champion gate and answers with
// the /api/v1/tool/exec response shapes (also used by
// /api/v1/memory/remember).
func (s *server) dispatchAndRespond(w http.ResponseWriter, r *http.Request, champ *champion.Champion, tc champion.ToolCall, session string) {
	emittedBy := tc.EmittedBy
	out, dispatchErr := champ.Dispatch(r.Context(), tc)

	if dispatchErr == nil {
		// Successful tool execution. Action audit row already written
		// by Champion's per-tool method; surface the result to the caller.
		writeJSON(w, http.StatusOK, toolExecResponse{
			Status:    "ok",
			Result:    out,
			SessionID: session,
		})
		return
	}

	// Classify the error: gate refusal vs underlying tool failure.
	var ge *champion.GateError
	if errors.As(dispatchErr, &ge) {
		if ge.PendingConfirmation() && isAgentLoop(emittedBy) {
			s.queueAgentCall(w, r, champ, tc, session, ge)
			return
		}
		if ge.PendingConfirmation() {
			// Rule 2 untrusted justification → issue a pending token.
			tok, terr := champ.IssuePendingConfirmation(tc)
			if terr != nil {
				writeJSON(w, http.StatusInternalServerError, toolExecResponse{
					Status: "error",
					Reason: fmt.Sprintf("issue pending: %v", terr),
				})
				return
			}
			confirmTokensIssued.Inc()
			writeJSON(w, http.StatusForbidden, toolExecResponse{
				Status:       "pending_confirmation",
				PendingToken: tok,
				Reason:       ge.Error(),
				SessionID:    session,
			})
			return
		}
		// Hard refusal (rule 1 path-allowlist or rule 3 destructive verb).
		writeJSON(w, http.StatusForbidden, toolExecResponse{
			Status:    "denied",
			Reason:    ge.Error(),
			SessionID: session,
		})
		return
	}

	// Underlying tool failure (e.g., runbook subprocess returned non-zero,
	// or write_file hit a permissions error post-gate). Surface as 500
	// with the reason; the action audit row records the failure.
	writeJSON(w, http.StatusInternalServerError, toolExecResponse{
		Status:    "error",
		Reason:    dispatchErr.Error(),
		SessionID: session,
	})
}

// queueAgentCall queues an agent loop's refused-pending call in the Well
// and answers with its id only.
func (s *server) queueAgentCall(w http.ResponseWriter, r *http.Request, champ *champion.Champion, tc champion.ToolCall, session string, ge *champion.GateError) {
	if s.confirmQueue == nil {
		writeJSON(w, http.StatusForbidden, toolExecResponse{
			Status:    "denied",
			Reason:    ge.Error() + "; agent-loop writes are queued for approval only when zhen-agentd runs with -confirm-queue",
			SessionID: session,
		})
		return
	}
	out, err := s.enqueue(r.Context(), champ, tc, session, ge.Error())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "champion provider: %v", err)
		return
	}
	if out.Decision != "queued" {
		writeJSON(w, http.StatusForbidden, toolExecResponse{Status: "denied", Reason: out.Text, SessionID: session})
		return
	}
	writeJSON(w, http.StatusAccepted, toolExecResponse{Status: "queued", QueueID: out.QueueID, Reason: out.Text, SessionID: session})
}

// enqueue puts a model's refused-pending call in the Well's queue
// (QueueConfirmer: cap, hash, Wotan notice) and audits the issue. No
// token is made.
func (s *server) enqueue(ctx context.Context, champ *champion.Champion, tc champion.ToolCall, session, reason string) (claudebridge.ConfirmOutcome, error) {
	cp, err := champion.NewToolProvider(champ, tc.EmittedBy)
	if err != nil {
		return claudebridge.ConfirmOutcome{}, err
	}
	qc := &claudebridge.QueueConfirmer{Champion: champ, Queue: s.confirmQueue, Pub: s.confirmPub, Source: "zhen-agentd"}
	out := qc.Confirm(ctx, claudebridge.ConfirmRequest{
		Conversation: session, Tool: tc.Name, Args: tc.Args, EmittedBy: tc.EmittedBy, Reason: reason, Provider: cp,
	})
	if out.Decision != "queued" {
		return out, nil
	}
	confirmQueued.Inc()
	if s.confirmAudit != nil {
		s.confirmAudit.Confirmation(ctx, claudebridge.ConfirmRecord{
			Conversation: session, Model: tc.EmittedBy, Tool: tc.Name, Action: "confirm_issue",
			QueueID: out.QueueID, CallSHA256: out.SHA256, Source: "zhen-agentd",
		})
	}
	return out, nil
}

// askPending is /ask's OnPending: the agent loop is a model, so its
// refused-pending writes are queued (or denied); a token never reaches
// the HTTP caller, which may be a model too.
func (s *server) askPending(champ *champion.Champion, session string) func(ctx context.Context, tc champion.ToolCall, reason string) string {
	return func(ctx context.Context, tc champion.ToolCall, reason string) string {
		if s.confirmQueue == nil {
			return ""
		}
		out, err := s.enqueue(ctx, champ, tc, session, reason)
		if err != nil || out.Decision != "queued" {
			return ""
		}
		return out.QueueID
	}
}
