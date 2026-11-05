// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recScheduler struct {
	ops  []string
	args []map[string]any
	err  error
}

func (r *recScheduler) UpdateSchedule(_ context.Context, op string, args map[string]any) (any, error) {
	r.ops = append(r.ops, op)
	r.args = append(r.args, args)
	if r.err != nil {
		return nil, r.err
	}
	return map[string]any{"op": op}, nil
}

func directUser() []Reference {
	return []Reference{{Topic: "typed-schedule-command", SourceTrust: "direct-user"}}
}

// confirmedCall runs tc the way a person does: Rule 2 holds it, and the
// person redeems the token their own request caused.
func confirmedCall(t *testing.T, c *Champion, tc ToolCall) (any, error) {
	t.Helper()
	ctx := context.Background()
	tc.Principal = "tty:stevie"
	_, err := c.Dispatch(ctx, tc)
	var ge *GateError
	if !errors.As(err, &ge) || !ge.PendingConfirmation() {
		t.Fatalf("%s: %v; want a pending-confirmation refusal", tc.Name, err)
	}
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatal(err)
	}
	return c.ConfirmPendingToolCall(ctx, tok, "tty:stevie")
}

// schedule_update is mutating: listed, dispatched, held for a person at
// Rule 2, and logged as a mutation (S-CHAMPION-HARDENING Step 165).
func TestScheduleUpdate_GatedAndDispatched(t *testing.T) {
	if !MutatingTools["schedule_update"] {
		t.Fatal("schedule_update is not in MutatingTools")
	}
	c, store := newTestChampion(t)
	rec := &recScheduler{}
	c = c.WithScheduleUpdater(rec)
	ctx := context.Background()

	// A model with no justification never reaches the scheduler.
	_, err := c.Dispatch(ctx, ToolCall{Name: "schedule_update", Args: map[string]any{"op": "add", "name": "x"}, EmittedBy: "zhen-agent"})
	var ge *GateError
	if !errors.As(err, &ge) || !ge.PendingConfirmation() {
		t.Fatalf("no justification: %v; want a pending-confirmation refusal", err)
	}
	if len(rec.ops) != 0 {
		t.Fatalf("the scheduler saw a refused call: %v", rec.ops)
	}

	// A direct user's call reaches it once confirmed, with its op and args.
	out, err := confirmedCall(t, c, ToolCall{Name: "schedule_update", Args: map[string]any{"op": "remove", "name": "x"}, Justification: directUser()})
	if err != nil {
		t.Fatalf("direct-user remove: %v", err)
	}
	if m, _ := out.(map[string]any); m["op"] != "remove" || len(rec.ops) != 1 || rec.args[0]["name"] != "x" {
		t.Fatalf("out=%v ops=%v args=%v", out, rec.ops, rec.args)
	}
	found := false
	for _, a := range store.actions {
		if a.ActionType == "schedule.update" && a.Status == "completed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no completed schedule.update action row: %+v", store.actions)
	}

	// Rule 3 still applies to the args.
	if _, err := c.Dispatch(ctx, ToolCall{Name: "schedule_update", Args: map[string]any{"op": "add", "name": "rm -rf /"}, Justification: directUser()}); err == nil {
		t.Fatal("destructive verb in a schedule arg was accepted")
	}
}

func TestScheduleUpdate_Refusals(t *testing.T) {
	c, store := newTestChampion(t)
	_, err := confirmedCall(t, c, ToolCall{Name: "schedule_update", Args: map[string]any{"op": "add"}, Justification: directUser()})
	if err == nil || !strings.Contains(err.Error(), "no scheduler") {
		t.Fatalf("without a scheduler: %v", err)
	}
	if n := len(store.actions); n == 0 || store.actions[n-1].Status != "failed" {
		t.Fatalf("the refusal was not logged as failed: %+v", store.actions)
	}

	c = c.WithScheduleUpdater(&recScheduler{err: errors.New("bad interval")})
	if _, err := confirmedCall(t, c, ToolCall{Name: "schedule_update", Args: map[string]any{"op": "add"}, Justification: directUser()}); err == nil || !strings.Contains(err.Error(), "bad interval") {
		t.Fatalf("scheduler error: %v", err)
	}
	if _, err := confirmedCall(t, c, ToolCall{Name: "schedule_update", Args: map[string]any{}, Justification: directUser()}); err == nil || !strings.Contains(err.Error(), `"op"`) {
		t.Fatalf("missing op: %v", err)
	}
}
