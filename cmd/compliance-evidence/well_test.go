// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/compliance/snapshot"
	"unheaded/pkg/compliance/well"
	"unheaded/pkg/gungnir"
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

// The committed deploy/ansible/compliance tree is exactly a fresh render.
func TestAnsibleTreeIsCurrent(t *testing.T) {
	want, err := renderAnsible("../..")
	if err != nil {
		t.Fatal(err)
	}
	have, err := ansibleFiles(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	if len(have) != len(want) {
		t.Errorf("%d files committed, %d rendered: run go run ./cmd/compliance-evidence ansible", len(have), len(want))
	}
	for p, body := range want {
		b, err := os.ReadFile(filepath.Join("../..", ansibleDir, p))
		if err != nil || string(b) != body {
			t.Errorf("%s is stale or missing: run go run ./cmd/compliance-evidence ansible", p)
		}
	}
}

func TestSnapshotCommand(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed")
	if err := os.WriteFile(seed, bytes.Repeat([]byte{7}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "east.snapshot.json")
	t.Setenv("WELL_HOST", "127.0.0.1")
	t.Setenv("WELL_PORT", "1")
	t.Setenv("WELL_CONFIG_PASSWORD", "x")
	args := []string{"-repo-dir", "../..", "-host", "east", "-key", seed, "-out", out}
	if err := runSnapshot(args, io.Discard); err == nil || !strings.Contains(err.Error(), "-from-yaml") {
		t.Fatalf("sealed the YAML without being told to: %v", err)
	}
	var msg bytes.Buffer
	if err := runSnapshot(append(args, "-from-yaml"), &msg); err != nil {
		t.Fatal(err)
	}
	priv, err := gungnir.LoadSeedFile(seed)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := gungnir.NewSignerWithID(priv)
	pub, err := gungnir.LoadPublicKeyFile(writeTemp(t, dir, signer.PublicKey()))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(out)
	snap, err := snapshot.Open(raw, pub, "east")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Source != "yaml-fallback" || len(snap.Fixes) != 15 { // east: lab-hardened (12, plus the three Step 267 fixes)
		t.Errorf("snapshot from %s with %d fixes", snap.Source, len(snap.Fixes))
	}
	if err := os.Chmod(seed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSnapshot(append(args, "-from-yaml"), io.Discard); err == nil {
		t.Error("used a world-readable key")
	}
}

func writeTemp(t *testing.T, dir string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, "pub")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
