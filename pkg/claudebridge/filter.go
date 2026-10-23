// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"encoding/json"
	"fmt"

	"unheaded/pkg/zhentools"
)

// MCPReadOnlyTools are the tools of raft/zhen_mcp_server.py that only
// read. They are the default allowlist for MCP-sourced tools: the server
// also offers file_write, file_patch and runbook_execute (which runs
// scripts) with no gate of its own.
var MCPReadOnlyTools = []string{"corpus_search", "file_read", "service_health", "runbook_list", "runbook_show"}

// AllowTools wraps p so that only the named tools are listed and only
// they can be called; any other name is refused before it reaches p.
// This sits in front of the gate, not instead of it: an allowed tool
// still goes through Gate.Execute.
func AllowTools(p zhentools.Provider, names ...string) zhentools.Provider {
	allow := make(map[string]bool, len(names))
	for _, n := range names {
		allow[n] = true
	}
	return &allowlist{p: p, allow: allow}
}

type allowlist struct {
	p     zhentools.Provider
	allow map[string]bool
}

func (a *allowlist) Tools(ctx context.Context) ([]zhentools.Tool, error) {
	all, err := a.p.Tools(ctx)
	if err != nil {
		return nil, err
	}
	var out []zhentools.Tool
	for _, t := range all {
		if a.allow[t.Name] {
			out = append(out, t)
		}
	}
	return out, nil
}

func (a *allowlist) Call(ctx context.Context, name string, input json.RawMessage) (zhentools.Result, error) {
	if !a.allow[name] {
		return zhentools.Result{Text: fmt.Sprintf("tool %q is not on this provider's allowlist", name), IsError: true}, nil
	}
	return a.p.Call(ctx, name, input)
}
