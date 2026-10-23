// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"unheaded/pkg/zhentools"
)

// ToolProvider exposes the Champion's own tools as a zhentools.Provider,
// so a model client (local Zhenai, the Claude API bridge) can list and
// call them without knowing they run in-process. Every call goes through
// Dispatch, so the B2 gate (AcceptToolCall) runs on every one: there is
// no ungated path through this type.
//
// The calls it makes carry no justification chain: whatever model asked
// for them is not a retrieval-grounded caller, so Rule 2 refuses every
// mutating tool pending out-of-band confirmation. Read-only tools run.
type ToolProvider struct {
	c         *Champion
	emittedBy string
}

// NewToolProvider returns a Provider over c. emittedBy is recorded on
// every ToolCall for the audit trail (for example "claude-api/<model>").
func NewToolProvider(c *Champion, emittedBy string) (*ToolProvider, error) {
	if c == nil {
		return nil, errors.New("champion: NewToolProvider needs a Champion")
	}
	return &ToolProvider{c: c, emittedBy: emittedBy}, nil
}

// Champion returns the Champion this provider dispatches to. A gate that
// is itself built over the same Champion uses it to avoid gating a call
// twice (see claudebridge.ChampionGate).
func (p *ToolProvider) Champion() *Champion { return p.c }

// Tools lists the tools Dispatch implements. Kanban tools are listed only
// when a KanbanStore is configured; listing a tool that can only fail
// would spend the model's tokens on nothing. runbook_execute and
// model_switch are deliberately not offered to models: both alter the
// Kingdom and have their own direct-user entry points.
func (p *ToolProvider) Tools(_ context.Context) ([]zhentools.Tool, error) {
	tools := []zhentools.Tool{
		{
			Name:        "read_file",
			Description: "Read a text file inside the Champion sandbox (the project root and its allowed paths). Call this when you need the contents of a file to answer.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute path of the file to read."}},"required":["path"],"additionalProperties":false}`),
		},
		{
			Name:        "write_file",
			Description: "Create or overwrite a file inside the Champion sandbox. Mutating: the Champion gate refuses it from a model unless the user confirms out of band, so explain the change you want instead of retrying.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute path of the file to write."},"content":{"type":"string","description":"Full new file contents."}},"required":["path","content"],"additionalProperties":false}`),
		},
		{
			Name:        "patch_file",
			Description: "Replace one exact occurrence of old_text with new_text in a file inside the Champion sandbox. Mutating: refused from a model unless the user confirms out of band.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old_text":{"type":"string"},"new_text":{"type":"string"}},"required":["path","old_text","new_text"],"additionalProperties":false}`),
		},
	}
	if p.c.runCheck != nil && p.c.runCheck.Checks != nil {
		tools = append(tools, zhentools.Tool{
			Name:        "run_check",
			Description: "Run one allowlisted check (tests, vet, lint) on a path inside this worktree, in a no-network sandbox. You name the check; you cannot give a command. Returns pass, exit code and output.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"check":{"type":"string","description":"Check key, e.g. go-test, py-unittest, shellcheck, fixture-check."},"path":{"type":"string","description":"Path relative to the worktree (default .). Go patterns like ./pkg/x/... are allowed."},"crate":{"type":"string","description":"Cargo package, for cargo-test only."}},"required":["check"],"additionalProperties":false}`),
		})
	}
	if p.c.kanbanStore != nil {
		tools = append(tools,
			zhentools.Tool{
				Name:        "kanban_list",
				Description: "List the Kanban board's tasks. Call this when the user asks about work status or open tasks.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			},
			zhentools.Tool{
				Name:        "kanban_create",
				Description: "Create a Kanban task. Mutating: refused from a model unless the user confirms out of band.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"task":{"type":"object","properties":{"title":{"type":"string"},"description":{"type":"string"},"status":{"type":"string"},"type":{"type":"string"},"owner":{"type":"string"}},"required":["title"]}},"required":["task"],"additionalProperties":false}`),
			},
			zhentools.Tool{
				Name:        "kanban_update",
				Description: "Update fields of a Kanban task. Mutating: refused from a model unless the user confirms out of band.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"updates":{"type":"object"}},"required":["id","updates"],"additionalProperties":false}`),
			},
		)
	}
	return tools, nil
}

// Call runs one tool through Dispatch. A gate refusal or a tool failure
// comes back as Result.IsError (the model sees why and may recover); the
// error return is reserved for input the provider cannot even decode.
func (p *ToolProvider) Call(ctx context.Context, name string, input json.RawMessage) (zhentools.Result, error) {
	res, err := p.Dispatch(ctx, name, input)
	if err == nil {
		return res, nil
	}
	var ge *GateError
	if errors.As(err, &ge) {
		return zhentools.Result{Text: "refused by Champion gate: " + ge.Error(), IsError: true}, nil
	}
	return zhentools.Result{Text: err.Error(), IsError: true}, nil
}

// Dispatch is Call without the refusal flattening: a gate refusal is
// returned as a *GateError so a caller that audits decisions can tell a
// refusal from a tool failure. Any other error is a tool failure.
func (p *ToolProvider) Dispatch(ctx context.Context, name string, input json.RawMessage) (zhentools.Result, error) {
	args, err := decodeArgs(input)
	if err != nil {
		return zhentools.Result{}, fmt.Errorf("%s: %w", name, err)
	}
	out, err := p.c.Dispatch(ctx, ToolCall{Name: name, Args: args, EmittedBy: p.emittedBy})
	if err != nil {
		return zhentools.Result{}, err
	}
	text, err := formatOutput(out)
	if err != nil {
		return zhentools.Result{}, fmt.Errorf("%s: %w", name, err)
	}
	return zhentools.Result{Text: text}, nil
}

// decodeArgs parses a tool input, which must be a JSON object (or empty).
func decodeArgs(input json.RawMessage) (map[string]any, error) {
	if len(input) == 0 {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("input must be a JSON object: %w", err)
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

// formatOutput renders Dispatch's typed output as text for a model.
func formatOutput(out any) (string, error) {
	switch v := out.(type) {
	case nil:
		return "ok", nil
	case string:
		return v, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("encode output: %w", err)
		}
		return string(b), nil
	}
}
