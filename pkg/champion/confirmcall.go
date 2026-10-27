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
	"strings"
	"time"
	"unicode/utf8"
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
	Requester   string         `json:"requester,omitempty"` // principal that asked (RT-03); hashed
	Canonical   []byte         `json:"-"`                   // canonical JSON of tool, args, project_root[, requester]
	SHA256      string         `json:"sha256"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

// QueuePrincipal is the confirmer for the Well queue: `zhen-claude
// confirm approve`, run under Stevie's own login to the Well. It may
// confirm any requester's call (and the only one that may confirm a
// call with no recorded requester).
const QueuePrincipal = "queue:stevie"

// confirmerAllowed reports whether approvedBy may confirm a call that
// requester asked for (RT-03). agent:* never confirms; ui:<x> and
// tty:<x> confirm only their own requests; QueuePrincipal confirms any.
// Principals are compared exactly: no case folding, no trimming.
func confirmerAllowed(requester, approvedBy string) bool {
	if approvedBy == QueuePrincipal {
		return true
	}
	if !humanPrincipal(approvedBy) {
		return false
	}
	return approvedBy == requester
}

// humanPrincipal reports whether p is a well-formed ui:<x> or tty:<x>
// principal: a non-empty name of printable ASCII with no spaces.
func humanPrincipal(p string) bool {
	var name string
	switch {
	case strings.HasPrefix(p, "ui:"):
		name = p[len("ui:"):]
	case strings.HasPrefix(p, "tty:"):
		name = p[len("tty:"):]
	default:
		return false
	}
	if name == "" || len(name) > 256 {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] <= ' ' || name[i] > '~' {
			return false
		}
	}
	return true
}

// CanonicalCall serialises a call deterministically: encoding/json sorts
// map keys at every depth, so equal maps give equal bytes whatever their
// insertion order. It returns the bytes and their SHA-256 (hex).
func CanonicalCall(tool string, args map[string]any, projectRoot string) ([]byte, string, error) {
	return CanonicalCallFor(tool, args, projectRoot, "")
}

// CanonicalCallFor is CanonicalCall bound to the requesting principal:
// a non-empty requester is part of the hashed bytes, so an approval
// cannot be re-bound to another requester. An empty requester hashes
// exactly like CanonicalCall (rows queued before the field existed).
func CanonicalCallFor(tool string, args map[string]any, projectRoot, requester string) ([]byte, string, error) {
	if args == nil {
		args = map[string]any{}
	}
	// encoding/json turns every invalid UTF-8 byte into U+FFFD, so two
	// different byte strings would hash alike (found by
	// FuzzPendingCallFor). Arguments decoded from JSON are always valid;
	// anything else is refused rather than hashed ambiguously.
	if !validUTF8Value(tool) || !validUTF8Value(projectRoot) || !validUTF8Value(requester) || !validUTF8Value(args) {
		return nil, "", errors.New("canonical call: a string is not valid UTF-8")
	}
	call := map[string]any{"tool": tool, "args": args, "project_root": projectRoot}
	if requester != "" {
		call["requester"] = requester
	}
	b, err := json.Marshal(call)
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
	canon, sum, err := CanonicalCallFor(tc.Name, tc.Args, c.config.ProjectRoot, tc.Principal)
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
		Requester: tc.Principal, Canonical: canon, SHA256: sum, ExpiresAt: time.Now().Add(ttl),
	}, nil
}

// ConfirmCall runs an approved PendingCall. approvedSHA is the hash the
// human approved (the stored call_sha256); the call is re-hashed here
// with this Champion's root, so tampered arguments, a different tool or a
// different project root all refuse. approvedBy is the confirming
// principal (ui:<x>, tty:<x> or QueuePrincipal); it must be allowed to
// confirm for p.Requester (confirmerAllowed), which the hash binds.
func (c *Champion) ConfirmCall(ctx context.Context, p PendingCall, approvedSHA, approvedBy string) (any, error) {
	if !ConfirmableTools[p.Tool] {
		return nil, fmt.Errorf("tool %q cannot be confirmed", p.Tool)
	}
	if !p.ExpiresAt.IsZero() && !time.Now().Before(p.ExpiresAt) {
		return nil, fmt.Errorf("confirmation expired at %s", p.ExpiresAt.UTC().Format(time.RFC3339))
	}
	_, sum, err := CanonicalCallFor(p.Tool, p.Args, c.config.ProjectRoot, p.Requester)
	if err != nil {
		return nil, err
	}
	if len(approvedSHA) != len(sum) || subtle.ConstantTimeCompare([]byte(sum), []byte(approvedSHA)) != 1 {
		_, _ = c.logAction(ctx, "confirm_refused_hash", fmt.Sprintf("ToolCall: %s (call does not match its approved hash)", p.Tool), approvedBy)
		return nil, errors.New("the call does not match the approved hash (tampered, re-bound to another requester, or approved for another project root)")
	}
	if !confirmerAllowed(p.Requester, approvedBy) {
		_, _ = c.logAction(ctx, "confirm_refused_principal",
			fmt.Sprintf("ToolCall: %s (confirmer %q may not confirm a call requested by %q)", p.Tool, approvedBy, p.Requester), approvedBy)
		return nil, fmt.Errorf("%w: confirmer %q may not confirm a call requested by %q", ErrConfirmerNotAllowed, approvedBy, p.Requester)
	}
	tc := ToolCall{Name: p.Tool, Args: p.Args, EmittedBy: p.EmittedBy, Principal: p.Requester}
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

// CheckPath reports whether path passes the sandbox's path rules (Rule 1,
// symlinks resolved). For callers that show a file before a decision.
func (c *Champion) CheckPath(path string) error { return c.validatePath(path) }

// validUTF8Value reports whether every string in v (keys included) is
// valid UTF-8.
func validUTF8Value(v any) bool {
	switch x := v.(type) {
	case string:
		return utf8.ValidString(x)
	case map[string]any:
		for k, sub := range x {
			if !utf8.ValidString(k) || !validUTF8Value(sub) {
				return false
			}
		}
	case []any:
		for _, sub := range x {
			if !validUTF8Value(sub) {
				return false
			}
		}
	}
	return true
}
