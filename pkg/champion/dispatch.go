// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package champion

import (
	"context"
	"encoding/json"
	"fmt"
)

// Dispatch is the agent-layer entry point. The agent constructs a
// ToolCall from the model's JSON output and hands it to Dispatch;
// Dispatch runs it through AcceptToolCall (the trust + destructive-
// verb gate) and, on Allow, routes to the underlying typed method.
//
// Direct programmatic callers continue to use ReadFile / WriteFile /
// etc. — those entry points have their own path-allowlist checks and
// remain unchanged. The B2 gate is enforced ONLY for tool calls that
// arrive through Dispatch (i.e., tool calls emitted by an LLM via the
// agent runtime).
//
// Returns the tool's output (typed per tool: string for read_file,
// "" for writes, []KanbanTask for kanban_list, etc.) and any error.
// On gate refusal, the returned error wraps ToolCallDecision so the
// caller can inspect PendingConfirmation.
func (c *Champion) Dispatch(ctx context.Context, tc ToolCall) (any, error) {
	// A name this switch does not implement never reaches the gate's
	// accept path (RT-09): it is refused as unknown and audited.
	if !dispatchable[tc.Name] {
		reason := fmt.Sprintf("unknown tool %q", tc.Name)
		actionID, _ := c.logAction(ctx, "tool_call_attempt", fmt.Sprintf("ToolCall: %s", tc.Name), tc.EmittedBy)
		c.completeAction(ctx, actionID, "denied_unknown_tool", summarizeArgs(tc.Args), reason, 0)
		decision := ToolCallDecision{Allow: false, Reason: reason}
		return nil, &GateError{Decision: decision, Wrapped: fmt.Errorf("tool call denied: %s", reason)}
	}
	decision, gateErr := c.AcceptToolCall(ctx, tc)
	if gateErr != nil {
		return nil, &GateError{Decision: decision, Wrapped: gateErr}
	}
	if !decision.Allow {
		// Defensive: should not happen — AcceptToolCall returns nil err
		// only when Allow=true. If it ever does, fail closed.
		return nil, fmt.Errorf("gate denied without error: %s", decision.Reason)
	}

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
		raw, ok := tc.Args["task"]
		if !ok {
			return nil, fmt.Errorf("kanban_create: missing arg %q", "task")
		}
		// Round-trip via JSON so map[string]any populates the typed struct.
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

	case "runbook_list":
		return c.RunbookList(ctx)

	case "runbook_show":
		name, err := requireString(tc.Args, "name")
		if err != nil {
			return nil, err
		}
		return c.RunbookShow(ctx, name)

	case "run_check":
		key, err := requireString(tc.Args, "check")
		if err != nil {
			return nil, err
		}
		path, _ := tc.Args["path"].(string)
		crate, _ := tc.Args["crate"].(string)
		return c.RunCheck(ctx, key, path, crate)

	case "runbook_execute":
		// WAVE15 Phase 2b: kingdom-altering runbook execution gated by
		// Champion. Args: {"name": "<runbook-name>"} optional
		// {"dry_run": bool}. The Python web UI hits this via the daemon's
		// /api/v1/tool/exec endpoint.
		name, err := requireString(tc.Args, "name")
		if err != nil {
			return nil, err
		}
		dryRun := false
		if v, ok := tc.Args["dry_run"].(bool); ok {
			dryRun = v
		}
		return c.RunbookExecute(ctx, name, dryRun)

	case "schedule_update":
		// zhen-agentd's scheduler: {"op": "add"|"remove"|"pause", ...}.
		op, err := requireString(tc.Args, "op")
		if err != nil {
			return nil, err
		}
		return c.ScheduleUpdate(ctx, op, tc.Args)

	case "model_switch":
		// ADR-060: swap which model llama-server is serving on :8081.
		// Args: {"key": "<allowlisted-model-key>"}. The allowlist is
		// scripts/switch-model.sh's MODEL_FILE keys, parsed at boot.
		// model_switch is a destructive verb — Champion's third rule
		// refuses it unless the justification ends in 'direct-user'
		// (browser-clicked actions).
		key, err := requireString(tc.Args, "key")
		if err != nil {
			return nil, err
		}
		return c.ModelSwap(ctx, key)

	case "memory_write":
		m, err := memoryWriteCall(tc)
		if err != nil {
			return nil, err
		}
		return c.RememberMemory(ctx, m)

	default:
		// Unknown / unimplemented tool: fail closed even though the gate
		// didn't reject — the gate's IsMutating() defaults unknown to
		// mutating, but the tool simply not being implemented here is a
		// distinct failure that surfaces a real bug.
		return nil, fmt.Errorf("Dispatch: unimplemented tool %q", tc.Name)
	}
}

// dispatchable is the set of tools Dispatch's switch implements. Keep it
// in step with the switch; TestToolLists_EveryListedToolDispatches checks
// that every name in MutatingTools and ReadOnlyTools (minus the MCP-only
// ones) is here.
var dispatchable = map[string]bool{
	"read_file": true, "write_file": true, "patch_file": true,
	"kanban_create": true, "kanban_update": true, "kanban_list": true,
	"runbook_list": true, "runbook_show": true, "runbook_execute": true,
	"run_check": true, "model_switch": true, "memory_write": true, "schedule_update": true,
}

// GateError wraps a ToolCallDecision when AcceptToolCall returns a
// non-nil error. The agent layer can type-assert and inspect
// PendingConfirmation to decide whether to surface a confirm-flow
// instead of treating the result as a hard failure.
type GateError struct {
	Decision ToolCallDecision
	Wrapped  error
}

func (e *GateError) Error() string {
	if e.Wrapped != nil {
		return e.Wrapped.Error()
	}
	return e.Decision.Reason
}

func (e *GateError) Unwrap() error { return e.Wrapped }

// PendingConfirmation reports whether the gate refused with Rule 2
// (mutating + untrusted justification) — i.e., the call could be
// re-issued with an out-of-band user confirmation token.
func (e *GateError) PendingConfirmation() bool {
	return e.Decision.PendingConfirmation
}

// requireString extracts a string-typed arg or returns a clear error.
func requireString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required arg %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("arg %q must be string, got %T", key, v)
	}
	return s, nil
}
