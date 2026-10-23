// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Out-of-band confirmation bound by hash (S-TWO-BRAINS Phase 2). A
// refused mutating call is turned into a PendingCall: tool, arguments and
// this Champion's project root, serialised canonically and hashed. The
// PendingCall is stored outside the model's reach (process memory for the
// TTY path, the Well for the queue path); ConfirmCall runs it only when
// the call still hashes to the approved value, has not expired, and still
// passes Rules 3 and 1. Rule 2 is what the human approval answers.

// ConfirmableTools are the only tools an approval can run: the Champion's
// own writes, which snapshot and can be reverted (ADR-019). Runbooks,
// model swaps and anything unknown are never confirmable.
var ConfirmableTools = map[string]bool{
	"write_file":    true,
	"patch_file":    true,
	"kanban_create": true,
	"kanban_update": true,
}

// MaxPendingCallBytes caps a canonical call (a file body is the big part).
const MaxPendingCallBytes = 1 << 20

// PendingCall is a refused call waiting for a human.
type PendingCall struct {
	Tool        string         `json:"tool"`
	Args        map[string]any `json:"args"`
	ProjectRoot string         `json:"project_root"`
	EmittedBy   string         `json:"emitted_by"`
	Canonical   []byte         `json:"-"` // canonical JSON of tool, args, project_root
	SHA256      string         `json:"sha256"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

// CanonicalCall serialises a call deterministically: encoding/json sorts
// map keys at every depth, so equal maps give equal bytes whatever their
// insertion order. It returns the bytes and their SHA-256 (hex).
func CanonicalCall(tool string, args map[string]any, projectRoot string) ([]byte, string, error) {
	if args == nil {
		args = map[string]any{}
	}
	b, err := json.Marshal(map[string]any{"tool": tool, "args": args, "project_root": projectRoot})
	if err != nil {
		return nil, "", fmt.Errorf("canonical call: %w", err)
	}
	if len(b) > MaxPendingCallBytes {
		return nil, "", fmt.Errorf("canonical call is %d bytes, over %d", len(b), MaxPendingCallBytes)
	}
	sum := sha256.Sum256(b)
	return b, hex.EncodeToString(sum[:]), nil
}

// PendingCallFor binds tc to this Champion's project root and a deadline.
// Only ConfirmableTools qualify.
func (c *Champion) PendingCallFor(tc ToolCall, ttl time.Duration) (PendingCall, error) {
	if !ConfirmableTools[tc.Name] {
		return PendingCall{}, fmt.Errorf("tool %q cannot be confirmed (only write_file, patch_file, kanban_create, kanban_update)", tc.Name)
	}
	if ttl <= 0 {
		return PendingCall{}, errors.New("confirmation TTL must be positive")
	}
	canon, sum, err := CanonicalCall(tc.Name, tc.Args, c.config.ProjectRoot)
	if err != nil {
		return PendingCall{}, err
	}
	// Re-decode so the stored args are exactly what was hashed (numbers
	// become float64, nested types become map[string]any / []any).
	var round struct {
		Args map[string]any `json:"args"`
	}
	if err := json.Unmarshal(canon, &round); err != nil {
		return PendingCall{}, fmt.Errorf("canonical call: %w", err)
	}
	return PendingCall{
		Tool: tc.Name, Args: round.Args, ProjectRoot: c.config.ProjectRoot, EmittedBy: tc.EmittedBy,
		Canonical: canon, SHA256: sum, ExpiresAt: time.Now().Add(ttl),
	}, nil
}

// ConfirmCall runs an approved PendingCall. approvedSHA is the hash the
// human approved (the stored call_sha256); the call is re-hashed here
// with this Champion's root, so tampered arguments, a different tool or a
// different project root all refuse. approvedBy names the human.
func (c *Champion) ConfirmCall(ctx context.Context, p PendingCall, approvedSHA, approvedBy string) (any, error) {
	if !ConfirmableTools[p.Tool] {
		return nil, fmt.Errorf("tool %q cannot be confirmed", p.Tool)
	}
	if !p.ExpiresAt.IsZero() && !time.Now().Before(p.ExpiresAt) {
		return nil, fmt.Errorf("confirmation expired at %s", p.ExpiresAt.UTC().Format(time.RFC3339))
	}
	_, sum, err := CanonicalCall(p.Tool, p.Args, c.config.ProjectRoot)
	if err != nil {
		return nil, err
	}
	if len(approvedSHA) != len(sum) || subtle.ConstantTimeCompare([]byte(sum), []byte(approvedSHA)) != 1 {
		_, _ = c.logAction(ctx, "confirm_refused_hash", fmt.Sprintf("ToolCall: %s (call does not match its approved hash)", p.Tool), approvedBy)
		return nil, errors.New("the call does not match the approved hash (tampered, or approved for another project root)")
	}
	tc := ToolCall{Name: p.Tool, Args: p.Args, EmittedBy: p.EmittedBy}
	if err := c.confirmedGate(ctx, tc); err != nil {
		return nil, err
	}
	_, _ = c.logAction(ctx, "confirm_approve",
		fmt.Sprintf("ToolCall: %s sha256=%s (approved out of band)", p.Tool, sum), approvedBy)
	return c.dispatchUnderlying(ctx, tc)
}

// confirmedGate is the gate an approved call still passes: Rule 3
// (destructive verbs) and Rule 1 (path allowlist, symlinks resolved).
// Rule 2 (untrusted justification) is what the approval answered. Both
// confirmation paths share it so they cannot drift apart.
func (c *Champion) confirmedGate(ctx context.Context, tc ToolCall) error {
	if tc.HasDestructiveVerb() {
		_, _ = c.logAction(ctx, "tool_call_confirmed_destructive_blocked",
			fmt.Sprintf("ToolCall: %s (destructive arg even after user confirm)", tc.Name),
			tc.EmittedBy)
		return fmt.Errorf("destructive shell verbs cannot be confirmed; refused")
	}
	if path, ok := tc.Args["path"].(string); ok {
		if err := c.validatePath(path); err != nil {
			_, _ = c.logAction(ctx, "tool_call_confirmed_path_denied",
				fmt.Sprintf("ToolCall: %s (path denied even after user confirm)", tc.Name),
				tc.EmittedBy)
			return fmt.Errorf("path-allowlist (still enforced after confirm): %w", err)
		}
	}
	return nil
}
