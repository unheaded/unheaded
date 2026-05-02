// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

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

	default:
		// Unknown / unimplemented tool: fail closed even though the gate
		// didn't reject — the gate's IsMutating() defaults unknown to
		// mutating, but the tool simply not being implemented here is a
		// distinct failure that surfaces a real bug.
		return nil, fmt.Errorf("Dispatch: unimplemented tool %q", tc.Name)
	}
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
