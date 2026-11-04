// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build usersystemd

package champion

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The overnight route, end to end, non-mutating: a model write_file in a
// zhen/* worktree needs no confirmation; run_check then executes that file as
// the operator. The script only asks access(2); it writes nothing outside
// the worktree.
func TestAdv_ModelFileRunByRunCheck_ReachesVarZhen(t *testing.T) {
	c, _, wt, agentRoot := sandboxChampion(t, "zhen/poc")
	if err := c.WithBranchSandbox(agentRoot); err != nil { // as pkg/zhenroute worktreeChampion
		t.Fatal(err)
	}
	checks, err := ParseChecks([]byte(probeChecks))
	if err != nil {
		t.Fatal(err)
	}
	c.WithRunCheck(RunCheckConfig{Checks: checks, AgentWorktreeRoot: agentRoot, Env: []string{"PATH=/usr/bin:/bin"}})
	script := `id -un
for p in /var/zhen /etc /usr; do if [ -w "$p" ]; then echo "WRITABLE $p"; else echo "readonly $p"; fi; done
grep -E ' /var(/zhen)? ' /proc/self/mountinfo | head -3
echo mounts-done
`
	path := filepath.Join(wt, "probe.sh")
	if _, err := c.Dispatch(context.Background(), ToolCall{
		Name: "write_file", Args: map[string]any{"path": path, "content": script}, EmittedBy: "zhen-agent",
	}); err != nil {
		t.Fatalf("model write_file refused: %v", err)
	}
	r, err := c.RunCheck(context.Background(), "probe", "probe.sh", "")
	if err != nil {
		t.Fatalf("run_check: %v", err)
	}
	t.Logf("exit=%d output:\n%s", r.Exit, r.Output)
	if strings.Contains(r.Output, "WRITABLE /var/zhen") {
		t.Fatalf("a model-authored file run by run_check can write /var/zhen")
	}
}
