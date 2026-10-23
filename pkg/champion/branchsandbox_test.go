// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitWorktree makes dir a git repository with one commit on branch.
func gitWorktree(t *testing.T, dir, branch string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", branch},
		{"config", "user.email", "t@example.invalid"},
		{"config", "user.name", "t"},
		{"commit", "-q", "--allow-empty", "--no-gpg-sign", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
}

func sandboxChampion(t *testing.T, branch string) (*Champion, *mockStore, string, string) {
	t.Helper()
	agent := t.TempDir()
	wt := filepath.Join(agent, "card-7")
	gitWorktree(t, wt, branch)
	store := &mockStore{}
	c, err := New(Config{ProjectRoot: wt}, store)
	if err != nil {
		t.Fatal(err)
	}
	return c, store, wt, agent
}

func pendingRefusal(err error) bool {
	var ge *GateError
	return errors.As(err, &ge) && ge.PendingConfirmation()
}

func TestBranchSandbox_WritesWithoutConfirmation(t *testing.T) {
	c, store, wt, agent := sandboxChampion(t, "zhen/card-7")
	if err := c.WithBranchSandbox(agent); err != nil {
		t.Fatal(err)
	}
	if c.BranchSandbox() != "zhen/card-7" {
		t.Fatalf("branch = %q", c.BranchSandbox())
	}
	ctx := context.Background()
	f := filepath.Join(wt, "lib.go")
	if _, err := c.Dispatch(ctx, ToolCall{Name: "write_file", Args: map[string]any{"path": f, "content": "package x\n"}, EmittedBy: "claude-api/m"}); err != nil {
		t.Fatalf("write in the branch sandbox: %v", err)
	}
	if _, err := c.Dispatch(ctx, ToolCall{Name: "patch_file", Args: map[string]any{"path": f, "old_text": "x", "new_text": "y"}, EmittedBy: "claude-api/m"}); err != nil {
		t.Fatalf("patch in the branch sandbox: %v", err)
	}
	if b, _ := os.ReadFile(f); string(b) != "package y\n" {
		t.Errorf("file = %q", b)
	}
	found := false
	for _, a := range store.actions {
		if a.ActionType == "tool_call_attempt" && strings.HasPrefix(a.ResultSummary, "branch-sandbox:zhen/card-7") {
			found = true
		}
	}
	if !found {
		t.Error("the branch-sandbox justification is not in the action log")
	}
}

func TestBranchSandbox_RulesOneAndThreeStillHold(t *testing.T) {
	c, _, wt, agent := sandboxChampion(t, "zhen/card-7")
	if err := c.WithBranchSandbox(agent); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, tc := range map[string]ToolCall{
		"destructive":      {Name: "write_file", Args: map[string]any{"path": filepath.Join(wt, "x.sh"), "content": "rm -rf /"}},
		"outside sandbox":  {Name: "write_file", Args: map[string]any{"path": filepath.Join(agent, "other", "x"), "content": "x"}},
		"dotdot":           {Name: "write_file", Args: map[string]any{"path": wt + "/../x", "content": "x"}},
		"denied path":      {Name: "write_file", Args: map[string]any{"path": filepath.Join(wt, ".env"), "content": "K=V"}},
		"git hook":         {Name: "write_file", Args: map[string]any{"path": filepath.Join(wt, ".git", "hooks", "pre-commit"), "content": "#!/bin/sh\n"}},
		"git HEAD":         {Name: "write_file", Args: map[string]any{"path": filepath.Join(wt, ".git", "HEAD"), "content": "ref: refs/heads/main\n"}},
		"kanban not freed": {Name: "kanban_create", Args: map[string]any{"task": map[string]any{"title": "x"}}},
	} {
		if _, err := c.Dispatch(ctx, tc); err == nil {
			t.Errorf("%s: allowed", name)
		}
	}
	if b, err := os.ReadFile(filepath.Join(wt, ".git", "HEAD")); err != nil || !strings.Contains(string(b), "zhen/card-7") {
		t.Errorf("HEAD = %q %v", b, err)
	}
}

func TestBranchSandbox_FallsBackToConfirmation(t *testing.T) {
	ctx := context.Background()
	write := func(c *Champion, root string) error {
		_, err := c.Dispatch(ctx, ToolCall{Name: "write_file", Args: map[string]any{"path": filepath.Join(root, "a.txt"), "content": "x"}})
		return err
	}

	// A worktree on main.
	c, _, wt, agent := sandboxChampion(t, "main")
	if err := c.WithBranchSandbox(agent); err == nil {
		t.Error("main enabled the branch sandbox")
	}
	if err := write(c, wt); !pendingRefusal(err) {
		t.Errorf("main: %v", err)
	}

	// A zhen/ branch outside the agent root.
	outside := filepath.Join(t.TempDir(), "wt")
	gitWorktree(t, outside, "zhen/card-1")
	c2, _ := New(Config{ProjectRoot: outside}, &mockStore{})
	if err := c2.WithBranchSandbox(t.TempDir()); err == nil {
		t.Error("a worktree outside the agent root enabled the branch sandbox")
	}
	if err := write(c2, outside); !pendingRefusal(err) {
		t.Errorf("outside: %v", err)
	}

	// A symlinked root.
	agent3 := t.TempDir()
	real := filepath.Join(agent3, "real")
	gitWorktree(t, real, "zhen/card-3")
	link := filepath.Join(agent3, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	c3, _ := New(Config{ProjectRoot: link}, &mockStore{})
	if err := c3.WithBranchSandbox(agent3); err == nil {
		t.Error("a symlinked root enabled the branch sandbox")
	}

	// A branch switched after enabling turns it off.
	c4, _, wt4, agent4 := sandboxChampion(t, "zhen/card-4")
	if err := c4.WithBranchSandbox(agent4); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", wt4, "checkout", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if err := write(c4, wt4); !pendingRefusal(err) {
		t.Errorf("after a switch to main: %v", err)
	}
}

func TestAgentBranchRE(t *testing.T) {
	for b, want := range map[string]bool{
		"zhen/card-7": true, "zhen/eval-go-off-by-one": true,
		"zhen/../x": false, "zhen/": false, "main": false, "zhen/Card": false, "zhen/a/b": false,
		"zhen/" + strings.Repeat("a", 65): false, "xzhen/a": false,
	} {
		if AgentBranchRE.MatchString(b) != want {
			t.Errorf("%q: want %v", b, want)
		}
	}
}

func TestBranchSandbox_OffByDefault(t *testing.T) {
	c, _, wt, _ := sandboxChampion(t, "zhen/card-7")
	_, err := c.Dispatch(context.Background(), ToolCall{Name: "write_file", Args: map[string]any{"path": filepath.Join(wt, "a"), "content": "x"}})
	if !pendingRefusal(err) {
		t.Errorf("without WithBranchSandbox: %v", err)
	}
}
