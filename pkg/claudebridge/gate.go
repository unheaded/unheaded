// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"unheaded/pkg/champion"
	"unheaded/pkg/zhentools"
)

// ToolCall is one tool_use block from Claude, on its way to a gate.
type ToolCall struct {
	ID        string          // tool_use id
	Name      string          // tool name as the model sent it
	Input     json.RawMessage // tool input, a JSON object
	Provider  string          // name of the provider that owns Name
	EmittedBy string          // "claude-api/<model>/<conversation>" for audit
}

// Gate decides whether a model-emitted tool call may run and, if it may,
// runs it on p. The bridge never calls a Provider directly: every
// tool_use from Claude goes through Gate.Execute, so there is no ungated
// path in this package.
//
// A refusal is returned as a *RefusedError; Claude receives it as an
// is_error tool_result. Any other error is a tool or transport failure,
// also returned to Claude as is_error.
type Gate interface {
	Execute(ctx context.Context, call ToolCall, p zhentools.Provider) (zhentools.Result, error)
}

// RefusedError is a gate's "no".
type RefusedError struct {
	Reason string
	// PendingConfirmation is true when the call could run with an
	// out-of-band user confirmation (Champion Rule 2).
	PendingConfirmation bool
}

func (e *RefusedError) Error() string { return "refused by gate: " + e.Reason }

// ChampionGate is the B2 gate (champion.AcceptToolCall: destructive verb,
// untrusted or missing justification on a mutating tool, path allowlist)
// applied to Claude's tool calls. Claude is not a trusted caller of
// Kingdom tools: its calls carry no justification chain, so every
// mutating tool, and every tool the Champion does not know (fail-closed
// as mutating), is refused pending confirmation.
//
// For the Champion's own tools it runs champion.Dispatch, which gates and
// executes in one step and writes one audit row. For any other provider
// (Zhen's MCP server) it runs AcceptToolCall and then the provider.
type ChampionGate struct {
	Champion *champion.Champion
}

// NewChampionGate returns a gate over c.
func NewChampionGate(c *champion.Champion) (*ChampionGate, error) {
	if c == nil {
		return nil, errors.New("claudebridge: ChampionGate needs a Champion")
	}
	return &ChampionGate{Champion: c}, nil
}

// Execute implements Gate.
func (g *ChampionGate) Execute(ctx context.Context, call ToolCall, p zhentools.Provider) (zhentools.Result, error) {
	if cp, ok := p.(*champion.ToolProvider); ok && cp.Champion() == g.Champion {
		res, err := cp.Dispatch(ctx, call.Name, call.Input)
		var ge *champion.GateError
		if errors.As(err, &ge) {
			return zhentools.Result{}, &RefusedError{Reason: ge.Error(), PendingConfirmation: ge.PendingConfirmation()}
		}
		return res, err
	}

	var args map[string]any
	if err := json.Unmarshal(call.Input, &args); err != nil || args == nil {
		return zhentools.Result{}, &RefusedError{Reason: fmt.Sprintf("tool %q: input is not a JSON object", call.Name)}
	}
	decision, err := g.Champion.AcceptToolCall(ctx, champion.ToolCall{Name: call.Name, Args: args, EmittedBy: call.EmittedBy})
	if err != nil || !decision.Allow {
		reason := decision.Reason
		if reason == "" && err != nil {
			reason = err.Error()
		}
		return zhentools.Result{}, &RefusedError{Reason: reason, PendingConfirmation: decision.PendingConfirmation}
	}
	return p.Call(ctx, call.Name, call.Input)
}
