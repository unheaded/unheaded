// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Branch-sandbox mode (S-TWO-BRAINS Step 175; a Proposed amendment to
// ADR-019's Level 2). When the Champion's project root is a throwaway
// worktree under the agent root, checked out on a zhen/<name> branch,
// Rule 2 is satisfied for write_file and patch_file by the sandbox itself
// (justification "branch-sandbox:<branch>"): Stevie reviews the whole
// branch before anything merges, and nothing is pushed. Rules 1 and 3
// still apply, nothing may be written under .git, and every other
// mutating tool (Kanban, runbooks) still needs out-of-band confirmation.
// Outside such a worktree nothing changes.

// AgentBranchRE is the only branch shape that enables the mode.
var AgentBranchRE = regexp.MustCompile(`^zhen/[a-z0-9-]{1,64}$`)

// branchSandboxTools are the writes the mode allows without confirmation.
var branchSandboxTools = map[string]bool{"write_file": true, "patch_file": true}

// DetectBranchSandbox returns the zhen/<name> branch when root is a real
// directory (no symlink in its path) strictly under agentRoot, checked out
// on such a branch. Any doubt is an error and the mode stays off.
func DetectBranchSandbox(root, agentRoot string) (string, error) {
	if root == "" || agentRoot == "" {
		return "", errors.New("branch sandbox: no root or agent root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	if resolved != filepath.Clean(abs) {
		return "", fmt.Errorf("branch sandbox: %s is reached through a symlink", abs)
	}
	agent, err := filepath.EvalSymlinks(agentRoot)
	if err != nil {
		return "", fmt.Errorf("branch sandbox: agent root: %w", err)
	}
	if !strings.HasPrefix(resolved, agent+string(filepath.Separator)) {
		return "", fmt.Errorf("branch sandbox: %s is not under %s", resolved, agent)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// #nosec G204 -- fixed binary and flags; the directory was validated above
	out, err := exec.CommandContext(ctx, "git", "-C", resolved, "symbolic-ref", "--quiet", "--short", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("branch sandbox: no branch checked out in %s", resolved)
	}
	branch := strings.TrimSpace(string(out))
	if !AgentBranchRE.MatchString(branch) {
		return "", fmt.Errorf("branch sandbox: branch %q is not zhen/<name>", branch)
	}
	return branch, nil
}

// WithBranchSandbox turns the mode on when the project root qualifies;
// otherwise it returns why not and leaves the Champion unchanged.
func (c *Champion) WithBranchSandbox(agentRoot string) error {
	branch, err := DetectBranchSandbox(c.config.ProjectRoot, agentRoot)
	if err != nil {
		return err
	}
	c.branchSandbox = &branchSandbox{agentRoot: agentRoot, branch: branch}
	return nil
}

type branchSandbox struct {
	agentRoot string
	branch    string
}

// BranchSandbox is the active agent branch, or "" when the mode is off.
func (c *Champion) BranchSandbox() string {
	if c.branchSandbox == nil {
		return ""
	}
	return c.branchSandbox.branch
}

// branchSandboxAllows reports whether the mode answers Rule 2 for tc. It
// re-checks the branch on every call (a branch switch turns it off) and
// refuses any path under .git (hooks there would run on Stevie's review).
func (c *Champion) branchSandboxAllows(tc ToolCall) (string, bool) {
	bs := c.branchSandbox
	if bs == nil || !branchSandboxTools[tc.Name] {
		return "", false
	}
	path, ok := tc.Args["path"].(string)
	if !ok || path == "" {
		return "", false
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		if part == ".git" {
			return "", false
		}
	}
	branch, err := DetectBranchSandbox(c.config.ProjectRoot, bs.agentRoot)
	if err != nil || branch != bs.branch {
		return "", false
	}
	return "branch-sandbox:" + branch, true
}
