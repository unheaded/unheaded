// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package champion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// PendingConfirmTTL is how long a pending-confirmation token remains
// valid after issuance. Per B2 design (eval/coding-gate/probe-2026-
// 05-02/B2-design-champion-tool-call-gate.md): 5 minutes — long
// enough for a user to read the warning and decide, short enough that
// a stale token can't be replayed by a later poisoned ref.
const PendingConfirmTTL = 5 * time.Minute

// pendingEntry holds a single pending tool call awaiting confirmation.
type pendingEntry struct {
	Call      ToolCall
	IssuedAt  time.Time
	ExpiresAt time.Time
	Used      bool
}

// confirmStore is the in-memory pending-confirmation store. All access
// guarded by a mutex; tokens are single-use.
type confirmStore struct {
	mu      sync.Mutex
	entries map[string]*pendingEntry
}

func newConfirmStore() *confirmStore {
	return &confirmStore{entries: make(map[string]*pendingEntry)}
}

// IssuePending creates a new token bound to the given ToolCall. The
// token is single-use and expires after PendingConfirmTTL.
func (s *confirmStore) IssuePending(tc ToolCall) (string, error) {
	tok, err := freshToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	s.mu.Lock()
	s.entries[tok] = &pendingEntry{
		Call:      tc,
		IssuedAt:  now,
		ExpiresAt: now.Add(PendingConfirmTTL),
	}
	s.mu.Unlock()
	return tok, nil
}

// consume returns the bound ToolCall for a valid token and atomically
// marks the token used. Returns error if the token is unknown, used,
// or expired. Callers should treat any error as "do not execute" and
// surface a fresh confirmation prompt.
func (s *confirmStore) consume(token string) (ToolCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[token]
	if !ok {
		return ToolCall{}, fmt.Errorf("unknown confirmation token")
	}
	if e.Used {
		return ToolCall{}, fmt.Errorf("confirmation token already used")
	}
	if time.Now().After(e.ExpiresAt) {
		// Mark used so future consumes also fail (replay protection
		// against a token that briefly straddles the deadline).
		e.Used = true
		return ToolCall{}, fmt.Errorf("confirmation token expired")
	}
	e.Used = true
	return e.Call, nil
}

// gc removes used or expired entries. Called opportunistically — the
// store is small so a periodic sweep isn't needed for correctness, but
// it bounds memory in long-running processes.
func (s *confirmStore) gc() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for tok, e := range s.entries {
		if e.Used || now.After(e.ExpiresAt) {
			delete(s.entries, tok)
		}
	}
}

// freshToken returns 32 hex chars (16 random bytes). crypto/rand
// failure is fatal at issuance — there's no point issuing a guessable
// token.
func freshToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("token entropy: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// IssuePendingConfirmation surfaces a confirmation token for a
// ToolCall that AcceptToolCall refused with PendingConfirmation=true.
// The agent layer calls this AFTER AcceptToolCall returns a GateError
// with PendingConfirmation==true. The token is single-use, expires in
// PendingConfirmTTL, and can be redeemed via ConfirmPendingToolCall.
//
// This entry point exists separately from AcceptToolCall so the agent
// layer makes an explicit choice to surface a user-facing prompt
// (rather than silently retrying) — the prompt should display:
//   - the tool name and args
//   - the untrusted reference summary
//   - a prominent "this came from a user-added external source" warning
//
// before showing the token to the human.
func (c *Champion) IssuePendingConfirmation(tc ToolCall) (string, error) {
	if c.confirmStore == nil {
		c.confirmStore = newConfirmStore()
	}
	c.confirmStore.gc()
	return c.confirmStore.IssuePending(tc)
}

// ConfirmPendingToolCall redeems a token issued by
// IssuePendingConfirmation and runs the bound ToolCall through
// Dispatch — but with Rule 2 SUPPRESSED (the user has affirmatively
// approved despite untrusted justification). Rules 1 (path-allowlist)
// and 3 (destructive-verb) are still enforced — the user can authorize
// an untrusted source, but they cannot authorize destruction.
//
// Returns the same shape as Dispatch (any output, error). On token
// failure (unknown / used / expired) returns the token error and does
// NOT execute the call.
func (c *Champion) ConfirmPendingToolCall(ctx context.Context, token string) (any, error) {
	if c.confirmStore == nil {
		return nil, fmt.Errorf("no pending confirmations")
	}
	tc, err := c.confirmStore.consume(token)
	if err != nil {
		return nil, err
	}

	// Run a stripped gate: Rules 3 and 1, but skip Rule 2.
	if tc.HasDestructiveVerb() {
		_, _ = c.logAction(ctx, "tool_call_confirmed_destructive_blocked",
			fmt.Sprintf("ToolCall: %s (destructive arg even after user confirm)", tc.Name),
			tc.EmittedBy)
		return nil, fmt.Errorf("destructive shell verbs cannot be confirmed; refused")
	}

	// Path-allowlist for path-bearing tools.
	if path, ok := tc.Args["path"].(string); ok {
		if err := c.validatePath(path); err != nil {
			_, _ = c.logAction(ctx, "tool_call_confirmed_path_denied",
				fmt.Sprintf("ToolCall: %s (path denied even after user confirm)", tc.Name),
				tc.EmittedBy)
			return nil, fmt.Errorf("path-allowlist (still enforced after confirm): %w", err)
		}
	}

	// Log that the user explicitly confirmed an untrusted-justification
	// call. This is the audit trail for "human in the loop authorized
	// this despite the warning."
	_, _ = c.logAction(ctx, "tool_call_user_confirmed",
		fmt.Sprintf("ToolCall: %s (user confirmed untrusted justification)", tc.Name),
		"user")

	// Now route to underlying — bypass AcceptToolCall by using the
	// same switch as Dispatch (we already cleared the relevant gates).
	return c.dispatchUnderlying(ctx, tc)
}

// dispatchUnderlying is the same switch as Dispatch but without the
// AcceptToolCall pre-check. Used by ConfirmPendingToolCall after the
// stripped gate passes.
//
// Direct callers should NOT use this — it skips the gate. Always go
// through Dispatch from the agent layer.
func (c *Champion) dispatchUnderlying(ctx context.Context, tc ToolCall) (any, error) {
	switch tc.Name {
	case "read_file":
		path, err := requireString(tc.Args, "path")
		if err != nil {
			return nil, err
		}
		return c.ReadFile(ctx, path)
	case "write_file":
		path, err := requireString(tc.Args, "path")
		if err != nil {
			return nil, err
		}
		content, err := requireString(tc.Args, "content")
		if err != nil {
			return nil, err
		}
		return nil, c.WriteFile(ctx, path, content)
	case "patch_file":
		path, err := requireString(tc.Args, "path")
		if err != nil {
			return nil, err
		}
		oldText, err := requireString(tc.Args, "old_text")
		if err != nil {
			return nil, err
		}
		newText, err := requireString(tc.Args, "new_text")
		if err != nil {
			return nil, err
		}
		return nil, c.PatchFile(ctx, path, oldText, newText)
	case "kanban_create":
		// Mirror Dispatch's kanban_create routing: the args carry a
		// "task" payload that arrives as map[string]any from JSON, so
		// round-trip through JSON to populate the typed KanbanTask
		// struct.
		raw, ok := tc.Args["task"]
		if !ok {
			return nil, fmt.Errorf("kanban_create: missing arg %q", "task")
		}
		buf, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("kanban_create: marshal task: %w", err)
		}
		var task KanbanTask
		if err := json.Unmarshal(buf, &task); err != nil {
			return nil, fmt.Errorf("kanban_create: unmarshal task: %w", err)
		}
		return nil, c.CreateKanbanTask(ctx, &task)
	case "kanban_update":
		id, err := requireString(tc.Args, "id")
		if err != nil {
			return nil, err
		}
		updates, ok := tc.Args["updates"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("kanban_update: missing arg %q", "updates")
		}
		legacy := make(map[string]interface{}, len(updates))
		for k, v := range updates {
			legacy[k] = v
		}
		return nil, c.UpdateKanbanTask(ctx, id, legacy)
	case "kanban_list":
		return c.ListKanbanTasks(ctx)
	case "runbook_execute":
		// Mirrors Dispatch's runbook_execute case for the post-confirm
		// path. Same args contract: name (required), dry_run (optional).
		name, err := requireString(tc.Args, "name")
		if err != nil {
			return nil, err
		}
		dryRun := false
		if v, ok := tc.Args["dry_run"].(bool); ok {
			dryRun = v
		}
		return c.RunbookExecute(ctx, name, dryRun)
	default:
		return nil, fmt.Errorf("ConfirmPendingToolCall: unimplemented tool %q", tc.Name)
	}
}
