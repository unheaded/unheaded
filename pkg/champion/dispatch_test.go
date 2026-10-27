// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package champion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDispatch_ReadFile_Allowed(t *testing.T) {
	c, _ := newTestChampion(t)
	target := filepath.Join(c.config.ProjectRoot, "hello.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatalf("setup write: %v", err)
	}
	out, err := c.Dispatch(context.Background(), ToolCall{
		Name:          "read_file",
		Args:          map[string]any{"path": target},
		Justification: []Reference{canonicalRef("a")},
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if out != "hi" {
		t.Errorf("read got %q; want %q", out, "hi")
	}
}

func TestDispatch_WriteFile_Allowed(t *testing.T) {
	c, _ := newTestChampion(t)
	target := filepath.Join(c.config.ProjectRoot, "out.txt")
	_, err := c.Dispatch(context.Background(), ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    target,
			"content": "package main\n",
		},
		Justification: []Reference{canonicalRef("a")},
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	if string(data) != "package main\n" {
		t.Errorf("file = %q; want package main", data)
	}
}

func TestDispatch_WriteFile_DeniedUntrusted(t *testing.T) {
	c, _ := newTestChampion(t)
	target := filepath.Join(c.config.ProjectRoot, "out.txt")
	_, err := c.Dispatch(context.Background(), ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    target,
			"content": "package main\n",
		},
		Justification: []Reference{externalRef("evil", "lbl")},
	})
	if err == nil {
		t.Fatalf("expected GateError")
	}
	var ge *GateError
	if !errors.As(err, &ge) {
		t.Fatalf("expected *GateError, got %T: %v", err, err)
	}
	if !ge.PendingConfirmation() {
		t.Errorf("expected PendingConfirmation=true: %+v", ge.Decision)
	}
	// File should not have been created.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file should not exist after gate refusal; stat=%v", err)
	}
}

func TestDispatch_DeniedDestructive(t *testing.T) {
	c, _ := newTestChampion(t)
	_, err := c.Dispatch(context.Background(), ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    filepath.Join(c.config.ProjectRoot, "x.sh"),
			"content": "#!/bin/sh\nrm -rf /\n",
		},
		Justification: []Reference{canonicalRef("a")},
	})
	if err == nil {
		t.Fatalf("expected destructive deny")
	}
	var ge *GateError
	if !errors.As(err, &ge) {
		t.Fatalf("expected *GateError, got %T: %v", err, err)
	}
	if ge.PendingConfirmation() {
		t.Errorf("expected hard deny, not pending: %+v", ge.Decision)
	}
	if !strings.Contains(ge.Decision.Reason, "destructive") {
		t.Errorf("reason should mention destructive: %q", ge.Decision.Reason)
	}
}

func TestDispatch_PatchFile_Allowed(t *testing.T) {
	c, _ := newTestChampion(t)
	target := filepath.Join(c.config.ProjectRoot, "x.txt")
	if err := os.WriteFile(target, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := c.Dispatch(context.Background(), ToolCall{
		Name: "patch_file",
		Args: map[string]any{
			"path":     target,
			"old_text": "world",
			"new_text": "kingdom",
		},
		Justification: []Reference{canonicalRef("a")},
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "hello kingdom" {
		t.Errorf("file = %q; want %q", data, "hello kingdom")
	}
}

func TestDispatch_MissingArg(t *testing.T) {
	c, _ := newTestChampion(t)
	_, err := c.Dispatch(context.Background(), ToolCall{
		Name:          "write_file",
		Args:          map[string]any{"path": filepath.Join(c.config.ProjectRoot, "x")},
		Justification: []Reference{canonicalRef("a")},
	})
	if err == nil || !strings.Contains(err.Error(), "content") {
		t.Errorf("expected missing-content error, got %v", err)
	}
}

func TestDispatch_UnknownTool(t *testing.T) {
	c, _ := newTestChampion(t)
	_, err := c.Dispatch(context.Background(), ToolCall{
		Name:          "send_email",
		Args:          map[string]any{},
		Justification: []Reference{canonicalRef("a")},
	})
	// Unknown tool, trusted justification: gate is_mutating=true (fail-
	// closed) BUT no destructive verb and no untrusted ref, so the gate
	// passes. Then Dispatch's switch falls through to "unimplemented".
	if err == nil {
		t.Fatalf("expected unknown-tool error")
	}
	var ge *GateError
	if !errors.As(err, &ge) || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected a GateError naming an unknown tool, got %v", err)
	}
}

// RT-09 (S-CHAMPION-HARDENING Steps 53-55): every tool the lists name is
// dispatched; system_command (never implemented) is gone and refused as
// unknown.

func runbookFixture(t *testing.T) *Champion {
	t.Helper()
	c, _ := newTestChampion(t)
	root := c.config.ProjectRoot
	for name, body := range map[string]string{
		"runbooks/infra/host-bootstrap.yaml": "name: host-bootstrap\n",
		"runbooks/wotan/acl.yaml":            "name: acl\n",
		"runbooks/README.md":                 "not a runbook\n",
	} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestDispatch_RunbookListShow(t *testing.T) {
	ctx := context.Background()
	c := runbookFixture(t)
	// Read-only tools: no justification needed.
	out, err := c.Dispatch(ctx, ToolCall{Name: "runbook_list", Args: map[string]any{}})
	if err != nil {
		t.Fatalf("runbook_list: %v", err)
	}
	names, ok := out.([]string)
	if !ok || strings.Join(names, ",") != "infra/host-bootstrap,wotan/acl" {
		t.Fatalf("runbook_list = %#v", out)
	}
	out, err = c.Dispatch(ctx, ToolCall{Name: "runbook_show", Args: map[string]any{"name": "wotan/acl"}})
	if err != nil || out != "name: acl\n" {
		t.Fatalf("runbook_show = %#v, %v", out, err)
	}

	outside := filepath.Join(t.TempDir(), "secret.yaml")
	if err := os.WriteFile(outside, []byte("secret: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(c.config.ProjectRoot, "runbooks", "infra", "evil.yaml")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "../x", "/etc/passwd", "infra/../../x", "nope", "infra/evil", "README"} {
		if out, err := c.Dispatch(ctx, ToolCall{Name: "runbook_show", Args: map[string]any{"name": bad}}); err == nil {
			t.Errorf("runbook_show %q = %#v; want refused", bad, out)
		}
	}
	if _, err := c.Dispatch(ctx, ToolCall{Name: "runbook_show", Args: map[string]any{}}); err == nil {
		t.Error("runbook_show without a name ran")
	}
}

func TestDispatch_RunbookList_RepoRunbooks(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Glob(filepath.Join(root, "runbooks", "*", "*.yaml"))
	if len(want) == 0 {
		t.Skip("no runbooks/ in this checkout")
	}
	c, err := New(Config{ProjectRoot: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	names, err := c.RunbookList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != len(want) {
		t.Errorf("RunbookList = %d names; runbooks/*/*.yaml has %d", len(names), len(want))
	}
}

// mcpOnlyTools are listed for the gate (MCP servers call AcceptToolCall)
// but run in those servers, never through Dispatch.
func mcpOnlyTool(name string) bool {
	switch name {
	case "file_read", "corpus_search", "service_health", "local_llm":
		return true
	}
	return strings.HasPrefix(name, "lich_")
}

func TestToolLists_EveryListedToolDispatches(t *testing.T) {
	c, _ := newTestChampion(t)
	for _, list := range []map[string]bool{MutatingTools, ReadOnlyTools} {
		for name := range list {
			if mcpOnlyTool(name) {
				continue
			}
			_, err := c.Dispatch(context.Background(), ToolCall{Name: name, Args: map[string]any{}, Justification: []Reference{{SourceTrust: "direct-user"}}})
			if err != nil && (strings.Contains(err.Error(), "unknown tool") || strings.Contains(err.Error(), "unimplemented")) {
				t.Errorf("%s is listed but not dispatched: %v", name, err)
			}
		}
	}
	if MutatingTools["system_command"] || ReadOnlyTools["system_command"] {
		t.Error("system_command is listed; it has no implementation and must not get one")
	}
	_, err := c.Dispatch(context.Background(), ToolCall{Name: "system_command", Args: map[string]any{"cmd": "id"}, Justification: []Reference{{SourceTrust: "direct-user"}}})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("system_command: %v; want unknown tool", err)
	}
}
