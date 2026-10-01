// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"unheaded/pkg/compliance/well"
)

func TestWellSyncNeedsTheWell(t *testing.T) {
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", "1")
	t.Setenv("WELL_CONFIG_USER", "config_admin")
	t.Setenv("WELL_CONFIG_PASSWORD", "x")
	err := runWell([]string{"sync", "-dry-run", "-repo-dir", "../.."}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unheaded_config") {
		t.Fatalf("got %v, want a loud unheaded_config failure (writes never fall back)", err)
	}
	if err := runWell([]string{"push"}, io.Discard); err == nil {
		t.Error("unknown subcommand accepted")
	}
}

func TestRenderSync(t *testing.T) {
	r := &well.SyncReport{
		Created:  map[string]int{"compliance_fixes": 10},
		Updated:  map[string]int{},
		Same:     map[string]int{"compliance_profiles": 4},
		Diverged: []string{"binding:west"},
	}
	out := renderSync(r, true)
	for _, want := range []string{"dry run", "compliance_fixes", "10 new", "binding:west"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestProfileCommandsNeedTheWell(t *testing.T) {
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", "1")
	t.Setenv("WELL_CONFIG_USER", "config_admin")
	t.Setenv("WELL_CONFIG_PASSWORD", "x")
	for _, args := range [][]string{
		{"set", "-repo-dir", "../..", "-host", "east", "-profile", "none", "-reason", "r", "-yes"},
		{"export", "-repo-dir", "../.."},
	} {
		if err := runProfile(args, io.Discard); err == nil || !strings.Contains(err.Error(), "The Well") {
			t.Errorf("%v: got %v, want a loud failure without The Well", args, err)
		}
	}
	if err := runProfile([]string{"delete"}, io.Discard); err == nil {
		t.Error("unknown profile command accepted")
	}
}

func TestDescribeSwitch(t *testing.T) {
	y, err := well.NewYAMLStore("../..")
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := y.Profiles(context.Background())
	out := describeSwitch(ps, "east", "lab", "cis-l1-server")
	if !strings.Contains(out, "turns on:  shell-tmout, sshd-safe") || !strings.Contains(out, "turns off: auditd") {
		t.Errorf("describe:\n%s", out)
	}
	if out := describeSwitch(ps, "east", "lab", "lab"); !strings.Contains(out, "turns on:  nothing") {
		t.Errorf("no-op switch:\n%s", out)
	}
}
