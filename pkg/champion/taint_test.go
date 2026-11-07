// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// H3: a retrieval chain says what the model was shown before it began, not
// what it has read since. Once a run has read tool output, a mutating call is
// held for a person however clean its chain is.
func TestTaint_HoldsAMutatingCallWhateverItsChain(t *testing.T) {
	c, _ := newTestChampion(t)
	root := c.GetProjectRoot()
	calls := []ToolCall{
		{Name: "write_file", Args: map[string]any{"path": filepath.Join(root, "a.txt"), "content": "x"}},
		{Name: "patch_file", Args: map[string]any{"path": filepath.Join(root, "a.txt"), "old_text": "x", "new_text": "y"}},
		{Name: "kanban_create", Args: map[string]any{"task": map[string]any{"id": "c1"}}},
		{Name: "kanban_update", Args: map[string]any{"id": "c1", "updates": map[string]any{"owner": "zhen"}}},
		{Name: "memory_write", Args: map[string]any{"question": "q", "answer": "a"}},
	}
	for _, chain := range [][]Reference{{canonicalRef("t")}, {localRef("t")}, {{Topic: "t", SourceTrust: "direct-user"}}} {
		for _, tc := range calls {
			tc.Justification = chain
			clean, err := c.AcceptToolCall(context.Background(), tc)
			if err != nil || !clean.Allow {
				t.Fatalf("%s on a clean run: %+v %v", tc.Name, clean, err)
			}
			tc.Tainted = true
			d, err := c.AcceptToolCall(context.Background(), tc)
			if err == nil || d.Allow || !d.PendingConfirmation {
				t.Errorf("%s in a tainted run: %+v %v", tc.Name, d, err)
			}
			if !strings.Contains(d.Reason, "tool output") {
				t.Errorf("%s: the reason does not say why: %q", tc.Name, d.Reason)
			}
		}
	}
}

// The three carve-outs stay: each only simulates, stops, or runs what a person
// set beforehand. The ordinary rules still apply to them.
func TestTaint_LeavesTheCarveOuts(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []ToolCall{
		{Name: "runbook_execute", Args: map[string]any{"name": "infra/safe", "dry_run": true}},
		{Name: "schedule_update", Args: map[string]any{"op": "pause", "paused": true}},
		{Name: "runbook_execute", Args: map[string]any{"name": "infra/safe"}, standing: true},
	} {
		tc.Tainted = true
		tc.Justification = []Reference{canonicalRef("t")}
		c, _ := newTestChampion(t)
		if d, err := c.AcceptToolCall(ctx, tc); err != nil || !d.Allow {
			t.Errorf("%s %v: %+v %v", tc.Name, tc.Args, d, err)
		}
		tc.Justification = []Reference{externalRef("t", "x")}
		if d, _ := c.AcceptToolCall(ctx, tc); !d.PendingConfirmation {
			t.Errorf("%s %v on an untrusted chain was not held: %+v", tc.Name, tc.Args, d)
		}
	}
}

// Reading goes on: a tainted run may look, and Rule 1 still judges the path.
func TestTaint_ReadsAreNotHeld(t *testing.T) {
	c, _ := newTestChampion(t)
	tc := ToolCall{Name: "read_file", Args: map[string]any{"path": filepath.Join(c.GetProjectRoot(), "a.txt")}, Tainted: true}
	if d, err := c.AcceptToolCall(context.Background(), tc); err != nil || !d.Allow {
		t.Fatalf("read: %+v %v", d, err)
	}
}

// A zhen/<name> worktree is the review route: what a run writes there waits for
// Stevie on a branch, so a tainted run still works a card. Rule 3 still applies.
func TestTaint_TheBranchSandboxStillAnswersRule2(t *testing.T) {
	c, _, root, agent := sandboxChampion(t, "zhen/card")
	if err := c.WithBranchSandbox(agent); err != nil {
		t.Fatal(err)
	}
	tc := ToolCall{Name: "write_file", Args: map[string]any{"path": filepath.Join(root, "a.txt"), "content": "x"}, Tainted: true}
	if d, err := c.AcceptToolCall(context.Background(), tc); err != nil || !d.Allow {
		t.Fatalf("sandbox write: %+v %v", d, err)
	}
}

// Taint comes from the loop that fed the model, never from a body.
func TestTaint_NoBodyCanClearOrSetIt(t *testing.T) {
	var tc ToolCall
	if err := json.Unmarshal([]byte(`{"name":"read_file","Tainted":true,"tainted":true}`), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Tainted {
		t.Fatal("a body set Tainted")
	}
	out, err := json.Marshal(ToolCall{Name: "x", Tainted: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(out)), "tainted") {
		t.Fatalf("Tainted reaches a serialised call: %s", out)
	}
}
