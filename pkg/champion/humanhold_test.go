// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// H3: the tools that run shell, change what runs unattended or swap the
// serving model answer to a person, whatever a call's justification says.
func TestHumanHold_TrustedJustificationDoesNotPassRule2(t *testing.T) {
	calls := []ToolCall{
		{Name: "runbook_execute", Args: map[string]any{"name": "infra/safe"}},
		{Name: "runbook_execute", Args: map[string]any{"name": "infra/safe", "dry_run": false}},
		{Name: "schedule_update", Args: map[string]any{"op": "add", "name": "x", "every": "1h"}},
		{Name: "schedule_update", Args: map[string]any{"op": "remove", "name": "x"}},
		{Name: "schedule_update", Args: map[string]any{"op": "pause", "paused": false}},
		{Name: "model_switch", Args: map[string]any{"key": "fast"}},
	}
	refs := map[string][]Reference{
		"canonical":   {canonicalRef("t")},
		"local":       {localRef("t")},
		"direct-user": {{Topic: "t", SourceTrust: "direct-user"}},
	}
	for label, chain := range refs {
		for _, tc := range calls {
			tc.Justification = chain
			c, _ := newTestChampion(t)
			d, err := c.AcceptToolCall(context.Background(), tc)
			if err == nil || d.Allow || !d.PendingConfirmation {
				t.Errorf("%s %v on a %s chain: allow=%v pending=%v err=%v", tc.Name, tc.Args, label, d.Allow, d.PendingConfirmation, err)
			}
		}
	}
}

// Two argument shapes keep the ordinary rules, read exactly as the tool
// reads them: a dry run prints and runs nothing, and pausing the scheduler
// is the stop control, which must work with nobody there to confirm it.
func TestHumanHold_InertAndStopCallsKeepTheOrdinaryRules(t *testing.T) {
	trusted := []Reference{canonicalRef("t")}
	for _, tc := range []ToolCall{
		{Name: "runbook_execute", Args: map[string]any{"name": "infra/safe", "dry_run": true}},
		{Name: "schedule_update", Args: map[string]any{"op": "pause", "paused": true}},
	} {
		tc.Justification = trusted
		c, _ := newTestChampion(t)
		if d, err := c.AcceptToolCall(context.Background(), tc); err != nil || !d.Allow {
			t.Errorf("%s %v: %+v %v", tc.Name, tc.Args, d, err)
		}
		tc.Justification = []Reference{externalRef("t", "x")}
		if d, _ := c.AcceptToolCall(context.Background(), tc); !d.PendingConfirmation {
			t.Errorf("%s %v on an untrusted chain was not held: %+v", tc.Name, tc.Args, d)
		}
	}
	// Anything short of the exact shape is held.
	for _, args := range []map[string]any{
		{"name": "infra/safe", "dry_run": "true"},
		{"name": "infra/safe", "dry_run": 1},
	} {
		c, _ := newTestChampion(t)
		d, _ := c.AcceptToolCall(context.Background(), ToolCall{Name: "runbook_execute", Args: args, Justification: trusted})
		if !d.PendingConfirmation {
			t.Errorf("runbook_execute %v: %+v", args, d)
		}
	}
	for _, args := range []map[string]any{
		{"op": "pause", "paused": "true"},
		{"op": "pause"},
		{"op": "add", "paused": true},
	} {
		c, _ := newTestChampion(t)
		d, _ := c.AcceptToolCall(context.Background(), ToolCall{Name: "schedule_update", Args: args, Justification: trusted})
		if !d.PendingConfirmation {
			t.Errorf("schedule_update %v: %+v", args, d)
		}
	}
}

// A held call runs only from a token its own person redeems; an approval
// queue cannot carry it (ConfirmableTools), so a model loop's call ends there.
func TestHumanHold_RunsFromAToken(t *testing.T) {
	needPython(t)
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	ctx := context.Background()
	tc := ToolCall{Name: "runbook_execute", Args: map[string]any{"name": "infra/safe"},
		Justification: []Reference{{Topic: "t", SourceTrust: "direct-user"}}, Principal: "tty:stevie"}
	_, err := c.Dispatch(ctx, tc)
	var ge *GateError
	if !errors.As(err, &ge) || !ge.PendingConfirmation() {
		t.Fatalf("Dispatch = %v, want a pending refusal", err)
	}
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.ConfirmPendingToolCall(ctx, tok, "tty:stevie")
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := out.(*RunbookExecuteResult); !ok || !strings.Contains(r.Output, "RAN") {
		t.Fatalf("confirmed run: %+v", out)
	}
	if _, err := c.PendingCallFor(tc, PendingConfirmTTL); err == nil {
		t.Error("runbook_execute reached the approval queue")
	}
}

// The scheduler's standing instruction is the one route past the hold, and
// it is a Go call: nothing a request or a model sends can set it.
func TestDispatchStanding_RunsARunbookAndNothingElse(t *testing.T) {
	needPython(t)
	c, _, _ := rbRoot(t, "infra/safe", safeRunbook, entry("infra/safe"))
	ctx := context.Background()
	refs := []Reference{{Topic: "schedule/x", SourceTrust: "direct-user"}}
	out, err := c.DispatchStanding(ctx, ToolCall{Name: "runbook_execute", Args: map[string]any{"name": "infra/safe"}, Justification: refs, EmittedBy: "scheduler"})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := out.(*RunbookExecuteResult); !ok || r.ExitCode != 0 {
		t.Fatalf("standing run: %+v", out)
	}
	for _, tc := range []ToolCall{
		{Name: "model_switch", Args: map[string]any{"key": "fast"}, Justification: refs},
		{Name: "schedule_update", Args: map[string]any{"op": "remove", "name": "x"}, Justification: refs},
		{Name: "write_file", Args: map[string]any{"path": c.GetProjectRoot() + "/x", "content": "y"}, Justification: refs},
	} {
		if _, err := c.DispatchStanding(ctx, tc); !errors.Is(err, ErrNotStanding) {
			t.Errorf("DispatchStanding(%s) = %v", tc.Name, err)
		}
	}
	// Rule 2 still reads the justification on this route.
	_, err = c.DispatchStanding(ctx, ToolCall{Name: "runbook_execute", Args: map[string]any{"name": "infra/safe"}, Justification: []Reference{externalRef("t", "x")}})
	var ge *GateError
	if !errors.As(err, &ge) || !ge.PendingConfirmation() {
		t.Errorf("standing call on an untrusted chain = %v", err)
	}
}
