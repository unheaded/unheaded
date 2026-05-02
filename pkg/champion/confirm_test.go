// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package champion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// untrustedWrite returns a write_file ToolCall with external-trust
// justification — the canonical "Rule 2" trigger.
func untrustedWrite(t *testing.T, c *Champion) ToolCall {
	t.Helper()
	return ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    filepath.Join(c.config.ProjectRoot, "out.txt"),
			"content": "approved-after-confirm\n",
		},
		Justification: []Reference{externalRef("evil", "lbl")},
		EmittedBy:     "test",
	}
}

func TestConfirm_HappyPath(t *testing.T) {
	c, _ := newTestChampion(t)
	tc := untrustedWrite(t, c)

	// First Dispatch — should refuse with PendingConfirmation.
	_, err := c.Dispatch(context.Background(), tc)
	var ge *GateError
	if !errors.As(err, &ge) || !ge.PendingConfirmation() {
		t.Fatalf("expected PendingConfirmation, got %v", err)
	}

	// Issue token.
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatalf("IssuePendingConfirmation: %v", err)
	}
	if len(tok) == 0 {
		t.Errorf("empty token")
	}

	// Confirm — should run the call.
	_, err = c.ConfirmPendingToolCall(context.Background(), tok)
	if err != nil {
		t.Fatalf("ConfirmPendingToolCall: %v", err)
	}

	// Verify the file was written.
	path := tc.Args["path"].(string)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if string(data) != "approved-after-confirm\n" {
		t.Errorf("file = %q; want approved-after-confirm", data)
	}
}

func TestConfirm_TokenSingleUse(t *testing.T) {
	c, _ := newTestChampion(t)
	tc := untrustedWrite(t, c)
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// First confirm consumes the token.
	if _, err := c.ConfirmPendingToolCall(context.Background(), tok); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	// Second confirm should fail.
	_, err = c.ConfirmPendingToolCall(context.Background(), tok)
	if err == nil {
		t.Fatalf("expected error on token reuse")
	}
	if !strings.Contains(err.Error(), "already used") {
		t.Errorf("expected 'already used', got %v", err)
	}
}

func TestConfirm_UnknownToken(t *testing.T) {
	c, _ := newTestChampion(t)
	// Need to seed the store so it's not nil.
	_, err := c.IssuePendingConfirmation(untrustedWrite(t, c))
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	_, err = c.ConfirmPendingToolCall(context.Background(), "nope-not-a-real-token")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("expected 'unknown', got %v", err)
	}
}

func TestConfirm_ExpiredToken(t *testing.T) {
	c, _ := newTestChampion(t)
	tc := untrustedWrite(t, c)
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Manually expire the entry by reaching into the store. The TTL is
	// 5 min and we don't want to clock-jump in tests, so we cheat by
	// rewinding the entry.
	c.confirmStore.mu.Lock()
	c.confirmStore.entries[tok].ExpiresAt = time.Now().Add(-1 * time.Second)
	c.confirmStore.mu.Unlock()

	_, err = c.ConfirmPendingToolCall(context.Background(), tok)
	if err == nil {
		t.Fatalf("expected expired error")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected 'expired', got %v", err)
	}
}

func TestConfirm_DestructiveStillRefusedAfterConfirm(t *testing.T) {
	// Even with explicit user confirm, Rule 3 (destructive verbs) still
	// fires. The user can authorize "use this untrusted source"; they
	// CANNOT authorize "run rm -rf /" via this flow.
	c, _ := newTestChampion(t)
	tc := ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    filepath.Join(c.config.ProjectRoot, "x.sh"),
			"content": "#!/bin/sh\nrm -rf /tmp/important\n",
		},
		Justification: []Reference{externalRef("evil", "lbl")},
		EmittedBy:     "test",
	}
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	_, err = c.ConfirmPendingToolCall(context.Background(), tok)
	if err == nil {
		t.Fatalf("expected destructive refusal even after confirm")
	}
	if !strings.Contains(err.Error(), "destructive") {
		t.Errorf("expected destructive in error, got %v", err)
	}
	// File must NOT have been written.
	if _, err := os.Stat(tc.Args["path"].(string)); !os.IsNotExist(err) {
		t.Errorf("file should not exist after destructive-refusal-after-confirm")
	}
}

func TestConfirm_PathStillEnforcedAfterConfirm(t *testing.T) {
	c, _ := newTestChampion(t)
	tc := ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    "/etc/passwd", // outside ProjectRoot
			"content": "evil",
		},
		Justification: []Reference{externalRef("evil", "lbl")},
		EmittedBy:     "test",
	}
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	_, err = c.ConfirmPendingToolCall(context.Background(), tok)
	if err == nil {
		t.Fatalf("expected path-allowlist refusal even after confirm")
	}
	if !strings.Contains(err.Error(), "path") {
		t.Errorf("expected path in error, got %v", err)
	}
}

func TestConfirm_NoStoreNoConfirm(t *testing.T) {
	c, _ := newTestChampion(t)
	// Have not called IssuePendingConfirmation yet → store is nil.
	_, err := c.ConfirmPendingToolCall(context.Background(), "any-token")
	if err == nil {
		t.Fatalf("expected error when store is nil")
	}
	if !strings.Contains(err.Error(), "no pending") {
		t.Errorf("expected 'no pending', got %v", err)
	}
}
