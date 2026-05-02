// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

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
		t.Fatalf("expected unimplemented error")
	}
	if !strings.Contains(err.Error(), "unimplemented") {
		t.Errorf("expected unimplemented, got %v", err)
	}
}
