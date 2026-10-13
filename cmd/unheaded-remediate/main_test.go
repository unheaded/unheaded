// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/snapshot"
	"unheaded/pkg/compliance/well"
	"unheaded/pkg/gungnir"
)

const recorder = "#!/bin/sh\necho \"$(basename \"$0\") $*\" >>\"$FAKE_CALLS\"\nexit 0\n"

// setup copies the repo model into a private directory and puts fake
// commands first on PATH.
func setup(t *testing.T) (model, root, state string) {
	t.Helper()
	dir := t.TempDir()
	mdir := filepath.Join(dir, "model")
	if err := os.Mkdir(mdir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "compliance", "remediation", "fixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	model = filepath.Join(mdir, "fixes.yaml")
	if err := os.WriteFile(model, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	root, state = filepath.Join(dir, "root"), filepath.Join(dir, "state")
	if err := os.MkdirAll(root+"/etc/modprobe.d", 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"sysctl", "systemctl", "sshd", "visudo", "apt-get", "sudo"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(recorder), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("FAKE_CALLS", filepath.Join(dir, "calls"))
	t.Setenv("UNHEADED_REMEDIATE_ALLOW_NONROOT", "1")
	return model, root, state
}

func cli(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(context.Background(), args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestListPlanApplyUndo(t *testing.T) {
	model, root, state := setup(t)
	flags := []string{"-model", model, "-root", root, "-state", state, "-host", "east"}
	code, out, _ := cli(append(flags, "list")...)
	if code != 0 || strings.Count(out, "\n") != 13 || !strings.Contains(out, "sshd-safe") {
		t.Fatalf("list: %d\n%s", code, out)
	}
	if code, out, errs := cli(append(flags, "plan", "kmod-cis")...); code != 0 || !strings.Contains(out, "would write") {
		t.Fatalf("plan: %d %s %s", code, out, errs)
	}
	if _, err := os.Stat(root + "/etc/modprobe.d/unheaded-cis.conf"); err == nil {
		t.Fatal("plan wrote a file")
	}
	code, out, errs := cli(append(flags, "apply", "kmod-cis")...)
	if code != 0 {
		t.Fatalf("apply: %d %s %s", code, out, errs)
	}
	if _, err := os.Stat(root + "/etc/modprobe.d/unheaded-cis.conf"); err != nil {
		t.Fatalf("apply wrote nothing: %v", err)
	}
	dirs, _ := filepath.Glob(filepath.Join(state, "*-kmod-cis"))
	if len(dirs) != 1 {
		t.Fatalf("backup dirs %v", dirs)
	}
	if code, out, errs := cli(append(flags, "undo", dirs[0])...); code != 0 {
		t.Fatalf("undo: %d %s %s", code, out, errs)
	}
	if _, err := os.Stat(root + "/etc/modprobe.d/unheaded-cis.conf"); err == nil {
		t.Fatal("undo left the file")
	}
	if b, _ := os.ReadFile(filepath.Join(state, "journal.jsonl")); strings.Count(string(b), "\n") != 2 {
		t.Errorf("journal:\n%s", b)
	}
}

func TestRefusesAWritableModel(t *testing.T) {
	model, root, state := setup(t)
	if err := os.Chmod(model, 0o666); err != nil {
		t.Fatal(err)
	}
	code, _, errs := cli("-model", model, "-root", root, "-state", state, "-host", "east", "apply", "kmod-cis")
	if code == 0 || !strings.Contains(errs, "refusing the model") {
		t.Fatalf("applied from a world-writable model: %d %s", code, errs)
	}
	if err := os.Chmod(model, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(model), 0o777); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := cli("-model", model, "-root", root, "-state", state, "-host", "east", "apply", "kmod-cis"); code == 0 {
		t.Fatalf("applied from a model in a world-writable directory: %s", errs)
	}
}

func TestRejectsBadInput(t *testing.T) {
	model, root, state := setup(t)
	flags := []string{"-model", model, "-root", root, "-state", state, "-host", "east"}
	for name, args := range map[string][]string{
		"no command":    nil,
		"unknown fix":   {"apply", "rm-rf"},
		"unknown verb":  {"nuke", "kmod-cis"},
		"missing fix":   {"plan"},
		"unknown host":  {"-host", "north", "plan", "kmod-cis"},
		"invalid model": {"-model", "/etc/hostname", "plan", "kmod-cis"},
	} {
		if code, _, _ := cli(append(append([]string{}, flags...), args...)...); code == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

// writeSnapshot seals the repo's binding for host and pins its key.
func writeSnapshot(t *testing.T, dir, host string) (snap, pub string) {
	t.Helper()
	store, err := well.NewYAMLStore(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	s, err := snapshot.Build(context.Background(), store, host, "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gungnir.NewSignerWithID(gungnir.GenerateDevKey())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := snapshot.Seal(s, signer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	snap, pub = filepath.Join(dir, host+".snapshot.json"), filepath.Join(dir, "snapshot.pub")
	if err := os.WriteFile(snap, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pub, signer.PublicKey(), 0o644); err != nil {
		t.Fatal(err)
	}
	return snap, pub
}

// West is the example host: still bound to lab (east moved to lab-hardened,
// which carries sshd-safe, on 2026-10-12).
func TestSnapshotMode(t *testing.T) {
	model, root, state := setup(t)
	snap, pub := writeSnapshot(t, filepath.Dir(model), "west")
	flags := []string{"-snapshot", snap, "-pubkey", pub, "-root", root, "-state", state, "-host", "west"}
	code, out, errs := cli(append(flags, "list")...)
	if code != 0 || strings.Count(out, "\n") != 10 || strings.Contains(out, "sshd-safe") {
		t.Fatalf("list from the lab snapshot: %d %s %s", code, out, errs)
	}
	if code, _, errs := cli(append(flags, "apply", "sshd-safe")...); code == 0 || !strings.Contains(errs, "unknown fix") {
		t.Errorf("applied a fix the snapshot does not carry: %s", errs)
	}
	if code, out, errs := cli(append(flags, "apply", "kmod-cis")...); code != 0 {
		t.Fatalf("apply from snapshot: %d %s %s", code, out, errs)
	}
	// The same snapshot is refused on another host.
	if code, _, errs := cli("-snapshot", snap, "-pubkey", pub, "-root", root, "-state", state, "-host", "east", "list"); code == 0 || !strings.Contains(errs, "snapshot refused") {
		t.Errorf("west's snapshot accepted on east: %s", errs)
	}
	// A snapshot others can write is refused before any apply.
	if err := os.Chmod(snap, 0o666); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := cli(append(flags, "apply", "kmod-cis")...); code == 0 || !strings.Contains(errs, "refusing") {
		t.Errorf("world-writable snapshot accepted: %s", errs)
	}
}
