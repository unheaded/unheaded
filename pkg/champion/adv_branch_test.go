// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"testing"
)

// worktreeChampion's construction: default AllowedPaths, branch sandbox on.
// AcceptToolCall only: nothing is written. No justification at all, as a
// model in the overnight loop.
func TestAdv_BranchSandboxOutsideTheWorktree(t *testing.T) {
	c, _, wt, agentRoot := sandboxChampion(t, "zhen/poc")
	if err := c.WithBranchSandbox(agentRoot); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		wt + "/inside.go",
		"/var/zhen/code-index/index-a.json",
		"/var/zhen/code-index/CURRENT",
		"/var/zhen/vor-source/unheaded/docs/x.md",
		"/var/zhen/projects/zhen-taught/x.md",
	} {
		d, err := c.AcceptToolCall(context.Background(), ToolCall{Name: "write_file",
			Args: map[string]any{"path": p, "content": "x"}, EmittedBy: "zhen-agent"})
		t.Logf("allow=%-5v pending=%-5v %s  (%v)", d.Allow, d.PendingConfirmation, p, err)
		if d.Allow && p != wt+"/inside.go" {
			t.Errorf("no confirmation needed for %s", p)
		}
	}
}
