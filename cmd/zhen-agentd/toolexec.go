// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/champion"
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
//	  "justification": [                              // optional; default = direct-user
//	    {"topic": "user-clicked-runbook", "source_trust": "direct-user"}
//	  ]
//	}
//
// Response shapes mirror /api/v1/agent/confirm so the client can
// reuse the same handling logic:
//
//	200 OK  → {"status": "ok",                    "result": <tool output>}
//	400     → {"status": "unknown"|"used"|"expired"|"error", "reason": "..."}
//	403     → {"status": "denied",                "reason": "..."}
//	403     → {"status": "pending_confirmation",  "pending_token": "...", "reason": "..."}
//
// Note: this endpoint does NOT auto-redeem its own pending tokens.
// If Champion's gate refuses with rule-2, the response carries the
// pending_token and the web UI should surface the consent UX before
// hitting /api/v1/agent/confirm.
type toolExecRequest struct {
	Tool          string                 `json:"tool"`
	Args          map[string]any         `json:"args"`
	ProjectRoot   string                 `json:"project_root,omitempty"`
	SessionID     string                 `json:"session_id,omitempty"`
	EmittedBy     string                 `json:"emitted_by,omitempty"`
	Justification []champion.Reference   `json:"justification,omitempty"`
}

type toolExecResponse struct {
	Status       string `json:"status"` // "ok" | "denied" | "pending_confirmation" | "unknown" | "used" | "expired" | "error"
	Result       any    `json:"result,omitempty"`
	Reason       string `json:"reason,omitempty"`
	PendingToken string `json:"pending_token,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
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

	// Default justification: direct-user (the UI is asserting that the
	// user explicitly invoked this action). Champion's Rule 2 treats
	// `source_trust: "direct-user"` as the escape from "untrusted
	// justification" — see pkg/champion/toolcall.go HasUntrustedJustification.
	justification := req.Justification
	if len(justification) == 0 {
		justification = []champion.Reference{{
			Topic:       "tool-exec-direct-user",
			Category:    "user-action",
			SourceKind:  "user-action",
			SourceTrust: "direct-user",
			SourceLabel: "/api/v1/tool/exec",
		}}
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
	}

	out, dispatchErr := champ.Dispatch(r.Context(), tc)

	if dispatchErr == nil {
		// Successful tool execution. Action audit row already written
		// by Champion's per-tool method; surface the result to the caller.
		writeJSON(w, http.StatusOK, toolExecResponse{
			Status:    "ok",
			Result:    out,
			SessionID: req.SessionID,
		})
		return
	}

	// Classify the error: gate refusal vs underlying tool failure.
	var ge *champion.GateError
	if errors.As(dispatchErr, &ge) {
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
				SessionID:    req.SessionID,
			})
			return
		}
		// Hard refusal (rule 1 path-allowlist or rule 3 destructive verb).
		writeJSON(w, http.StatusForbidden, toolExecResponse{
			Status:    "denied",
			Reason:    ge.Error(),
			SessionID: req.SessionID,
		})
		return
	}

	// Underlying tool failure (e.g., runbook subprocess returned non-zero,
	// or write_file hit a permissions error post-gate). Surface as 500
	// with the reason; the action audit row records the failure.
	writeJSON(w, http.StatusInternalServerError, toolExecResponse{
		Status:    "error",
		Reason:    dispatchErr.Error(),
		SessionID: req.SessionID,
	})
}
