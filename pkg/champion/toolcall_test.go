// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package champion

import (
	"context"
	"strings"
	"testing"
)

// newTestChampion constructs a Champion with a temp project root and a
// mockStore — same pattern as champion_test.go.
func newTestChampion(t *testing.T) (*Champion, *mockStore) {
	t.Helper()
	store := &mockStore{}
	c, err := New(Config{ProjectRoot: t.TempDir()}, store)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, store
}

// canonicalRef returns a synthetic [canonical] reference.
func canonicalRef(topic string) Reference {
	return Reference{
		Topic:       topic,
		Category:    "languages/go",
		SourceKind:  "embedded",
		SourceTrust: "canonical",
	}
}

// externalRef returns a synthetic [external] reference (poisoned source).
func externalRef(topic, label string) Reference {
	return Reference{
		Topic:       topic,
		Category:    ".",
		SourceKind:  "user-source",
		SourceTrust: "external",
		SourcePath:  "/tmp/evil-corpus",
		SourceLabel: label,
	}
}

// localRef returns a synthetic [local] reference (~/.config/cs/sheets/).
func localRef(topic string) Reference {
	return Reference{
		Topic:       topic,
		Category:    "shell",
		SourceKind:  "user-custom",
		SourceTrust: "local",
		SourcePath:  "/home/user/.config/cs/sheets",
	}
}

// --- IsMutating decision matrix ---

func TestIsMutating(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"write_file", true},
		{"patch_file", true},
		{"delete_file", true},
		{"kanban_create", true},
		{"kanban_update", true},
		{"kanban_delete", true},
		{"runbook_execute", true},
		{"system_command", true},

		{"read_file", false},
		{"kanban_list", false},
		{"runbook_show", false},
		{"runbook_list", false},
		{"corpus_search", false},
		{"service_health", false},

		// Unknown tool name → fail-closed (mutating).
		{"unknown_tool", true},
		{"", true},
		{"DROP_DATABASE", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := (&ToolCall{Name: tc.name}).IsMutating()
			if got != tc.want {
				t.Errorf("IsMutating(%q) = %v; want %v",
					tc.name, got, tc.want)
			}
		})
	}
}

// --- HasUntrustedJustification ---
//
// Semantics changed 2026-05-02 after the A2-agent-adversarial probe
// found that empty-justification + mutating tool was a gate bypass.
// New rule: empty justification on a MUTATING tool is fail-closed
// (returns true). Empty justification on a read-only tool is fine.

func TestHasUntrustedJustification_ReadOnlyTools(t *testing.T) {
	// Read-only tools with empty justification are fine — reading
	// without a citation is normal exploration.
	cases := []struct {
		name string
		refs []Reference
		want bool
	}{
		{"empty justification on read_file", nil, false},
		{"only canonical", []Reference{canonicalRef("a")}, false},
		{"only local", []Reference{localRef("a")}, false},
		{"single external", []Reference{externalRef("evil", "lbl")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := (&ToolCall{Name: "read_file", Justification: tc.refs}).HasUntrustedJustification()
			if got != tc.want {
				t.Errorf("HasUntrustedJustification (read_file) = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestHasUntrustedJustification_MutatingTools(t *testing.T) {
	// Mutating tools with empty justification are fail-closed UNTRUSTED.
	// This is the post-probe-A2 fix: an attacker can craft a prompt that
	// the seed retriever doesn't match (refs ends up empty) but that
	// inlines a poisoned reference textually. The model is influenced;
	// the gate sees no refs. Empty-on-mutating must be untrusted to
	// prevent that bypass.
	cases := []struct {
		name string
		refs []Reference
		want bool
	}{
		{"empty justification on write_file → fail-closed", nil, true},
		{"only canonical", []Reference{canonicalRef("a")}, false},
		{"only local", []Reference{localRef("a")}, false},
		{"mixed canonical + local", []Reference{canonicalRef("a"), localRef("b")}, false},
		{"single external", []Reference{externalRef("evil", "lbl")}, true},
		{"canonical first, external buried", []Reference{canonicalRef("a"), canonicalRef("b"), externalRef("evil", "lbl")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := (&ToolCall{Name: "write_file", Justification: tc.refs}).HasUntrustedJustification()
			if got != tc.want {
				t.Errorf("HasUntrustedJustification (write_file) = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestHasUntrustedJustification_DirectUserEscape(t *testing.T) {
	// Programmatic callers that aren't model-derived can supply a single
	// "direct-user" trust reference to opt out of the empty-justification
	// fail-closed rule.
	tc := &ToolCall{
		Name: "write_file",
		Justification: []Reference{
			{SourceTrust: "direct-user"},
		},
	}
	if tc.HasUntrustedJustification() {
		t.Errorf("direct-user trust should be honored; got untrusted")
	}
}

// --- HasDestructiveVerb (recursive into args) ---

func TestHasDestructiveVerb(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want bool
	}{
		{"empty args", nil, false},
		{"clean string arg", map[string]any{"path": "src/main.go"}, false},
		{"rm -rf in command", map[string]any{"command": "rm -rf /tmp/data"}, true},
		{"drop table buried in SQL", map[string]any{"sql": "select * from users; drop table users"}, true},
		{
			"argv slice smuggles destructive",
			map[string]any{
				"args": []any{"sh", "-c", "rm -rf /"},
			},
			true,
		},
		{
			"nested map with destructive value",
			map[string]any{
				"options": map[string]any{
					"on_failure": "shutdown -h now",
				},
			},
			true,
		},
		{"mkfs.ext4 in args", map[string]any{"cmd": "mkfs.ext4 /dev/sda"}, true},
		{"git reset --hard", map[string]any{"command": "git reset --hard origin/main"}, true},
		{"clean file write", map[string]any{"path": "src/foo.go", "content": "package foo\n"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := (&ToolCall{Args: tc.args}).HasDestructiveVerb()
			if got != tc.want {
				t.Errorf("HasDestructiveVerb = %v; want %v", got, tc.want)
			}
		})
	}
}

// --- AcceptToolCall — full decision matrix ---

func TestAcceptToolCall_AllowedReadOnly(t *testing.T) {
	c, store := newTestChampion(t)
	tc := ToolCall{
		Name:          "read_file",
		Args:          map[string]any{"path": c.config.ProjectRoot + "/foo.txt"},
		Justification: []Reference{canonicalRef("languages/go")},
		EmittedBy:     "test",
	}
	dec, err := c.AcceptToolCall(context.Background(), tc)
	if err != nil {
		t.Fatalf("expected allow, got err: %v", err)
	}
	if !dec.Allow {
		t.Errorf("expected Allow=true, got %+v", dec)
	}
	if len(store.actions) != 1 || store.actions[0].Status != "accepted" {
		t.Errorf("expected accepted action, got %+v", store.actions)
	}
}

func TestAcceptToolCall_AllowedMutatingTrustedJustification(t *testing.T) {
	c, _ := newTestChampion(t)
	tc := ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    c.config.ProjectRoot + "/foo.txt",
			"content": "package foo\n",
		},
		Justification: []Reference{canonicalRef("a"), localRef("b")},
		EmittedBy:     "test",
	}
	dec, err := c.AcceptToolCall(context.Background(), tc)
	if err != nil {
		t.Fatalf("expected allow, got err: %v", err)
	}
	if !dec.Allow {
		t.Errorf("expected Allow=true, got %+v", dec)
	}
}

func TestAcceptToolCall_DeniedUntrustedMutating(t *testing.T) {
	c, store := newTestChampion(t)
	tc := ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    c.config.ProjectRoot + "/foo.txt",
			"content": "package foo\n",
		},
		Justification: []Reference{externalRef("evil", "user-added-source")},
		EmittedBy:     "test",
	}
	dec, err := c.AcceptToolCall(context.Background(), tc)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if dec.Allow {
		t.Errorf("expected Allow=false, got %+v", dec)
	}
	if !dec.PendingConfirmation {
		t.Errorf("expected PendingConfirmation=true, got %+v", dec)
	}
	if !strings.Contains(dec.Reason, "external-trust justification") {
		t.Errorf("reason should mention external-trust: %q", dec.Reason)
	}
	// Audit log should record the rejection reason.
	if len(store.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(store.actions))
	}
	if store.actions[0].Status != "denied_untrusted_justification" {
		t.Errorf("expected denied_untrusted_justification, got %q", store.actions[0].Status)
	}
}

func TestAcceptToolCall_AllowedReadOnlyEvenIfUntrusted(t *testing.T) {
	// Read-only tools with untrusted justification are still allowed —
	// reading a poisoned source is fine; only mutating actions need to
	// gate on trust. (The model still has to be careful about what it
	// SAYS about the read content; that's the system-prompt clause's
	// job, not the gate's.)
	c, _ := newTestChampion(t)
	tc := ToolCall{
		Name:          "read_file",
		Args:          map[string]any{"path": c.config.ProjectRoot + "/foo.txt"},
		Justification: []Reference{externalRef("evil", "lbl")},
		EmittedBy:     "test",
	}
	dec, err := c.AcceptToolCall(context.Background(), tc)
	if err != nil {
		t.Fatalf("expected allow, got err: %v", err)
	}
	if !dec.Allow {
		t.Errorf("expected Allow=true, got %+v", dec)
	}
}

func TestAcceptToolCall_DeniedDestructiveVerbBeatsTrust(t *testing.T) {
	// Even with a trusted-only justification chain, a destructive verb
	// in args triggers Rule 3.
	c, store := newTestChampion(t)
	tc := ToolCall{
		Name: "system_command",
		Args: map[string]any{
			"cmd": "rm -rf /home/user/important",
		},
		Justification: []Reference{canonicalRef("admin/runbook")},
		EmittedBy:     "test",
	}
	dec, err := c.AcceptToolCall(context.Background(), tc)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if dec.Allow || dec.PendingConfirmation {
		t.Errorf("expected hard deny (not pending), got %+v", dec)
	}
	if !strings.Contains(dec.Reason, "destructive shell verb") {
		t.Errorf("reason should mention destructive shell verb: %q", dec.Reason)
	}
	if len(store.actions) != 1 || store.actions[0].Status != "denied_destructive" {
		t.Errorf("expected denied_destructive, got %+v", store.actions)
	}
}

func TestAcceptToolCall_DestructiveTakesPrecedenceOverUntrusted(t *testing.T) {
	// When BOTH Rule 2 (untrusted) and Rule 3 (destructive) apply, the
	// gate picks Rule 3 (hard deny) over Rule 2 (pending confirmation).
	// The audit log should record denied_destructive, not denied_untrusted.
	c, store := newTestChampion(t)
	tc := ToolCall{
		Name: "system_command",
		Args: map[string]any{
			"cmd": "rm -rf /var/data",
		},
		Justification: []Reference{externalRef("evil", "lbl")},
		EmittedBy:     "test",
	}
	dec, err := c.AcceptToolCall(context.Background(), tc)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if dec.PendingConfirmation {
		t.Errorf("expected hard deny, not pending: %+v", dec)
	}
	if store.actions[0].Status != "denied_destructive" {
		t.Errorf("expected denied_destructive (precedence), got %q",
			store.actions[0].Status)
	}
}

func TestAcceptToolCall_DeniedPathOutsideAllowlist(t *testing.T) {
	c, store := newTestChampion(t)
	tc := ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    "/etc/passwd", // Outside ProjectRoot
			"content": "evil",
		},
		Justification: []Reference{canonicalRef("a")},
		EmittedBy:     "test",
	}
	dec, err := c.AcceptToolCall(context.Background(), tc)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if dec.Allow {
		t.Errorf("expected Allow=false, got %+v", dec)
	}
	if !strings.Contains(dec.Reason, "path") {
		t.Errorf("reason should mention path: %q", dec.Reason)
	}
	if store.actions[0].Status != "denied_path" {
		t.Errorf("expected denied_path, got %q", store.actions[0].Status)
	}
}

func TestAcceptToolCall_UnknownToolFailsClosed(t *testing.T) {
	// An unknown tool name is treated as mutating; with untrusted
	// justification it gets pending-confirmation. With trusted
	// justification it falls through to the path-allowlist (which
	// will pass if no path arg is present or a valid path is given).
	c, _ := newTestChampion(t)

	// Untrusted + unknown → pending confirmation.
	tc := ToolCall{
		Name:          "send_email",
		Args:          map[string]any{},
		Justification: []Reference{externalRef("evil", "lbl")},
		EmittedBy:     "test",
	}
	dec, err := c.AcceptToolCall(context.Background(), tc)
	if err == nil || !dec.PendingConfirmation {
		t.Errorf("unknown+untrusted should be pending; got dec=%+v err=%v", dec, err)
	}
}

func TestAcceptToolCall_AuditLoggedAlways(t *testing.T) {
	c, store := newTestChampion(t)

	// Allowed call.
	_, _ = c.AcceptToolCall(context.Background(), ToolCall{
		Name: "read_file",
		Args: map[string]any{"path": c.config.ProjectRoot + "/x"},
	})
	// Denied destructive.
	_, _ = c.AcceptToolCall(context.Background(), ToolCall{
		Name: "system_command",
		Args: map[string]any{"cmd": "rm -rf /"},
	})
	// Denied untrusted.
	_, _ = c.AcceptToolCall(context.Background(), ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    c.config.ProjectRoot + "/y",
			"content": "data",
		},
		Justification: []Reference{externalRef("evil", "lbl")},
	})

	if len(store.actions) != 3 {
		t.Fatalf("expected 3 audit entries, got %d", len(store.actions))
	}
	statuses := []string{}
	for _, a := range store.actions {
		statuses = append(statuses, a.Status)
	}
	wantStatuses := []string{"accepted", "denied_destructive", "denied_untrusted_justification"}
	for i, w := range wantStatuses {
		if statuses[i] != w {
			t.Errorf("audit[%d] = %q; want %q", i, statuses[i], w)
		}
	}
}
