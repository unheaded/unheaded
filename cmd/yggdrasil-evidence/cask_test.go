// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/sealedcask"
	"unheaded/pkg/sealedcask/casktest"
)

func TestCaskSignThenVerify(t *testing.T) {
	dir := t.TempDir()
	k := casktest.NewKey(t)
	seed, pub := k.WriteFiles(t, dir)
	files := casktest.Files()
	m := casktest.Manifest(files)
	rune := filepath.Join(dir, sealedcask.ManifestName)
	if err := os.WriteFile(rune, m, 0o600); err != nil {
		t.Fatal(err)
	}
	sig := filepath.Join(dir, sealedcask.SigName)
	if rc, out, errs := runCLI(t, "cask", "sign", "--key", seed, "--manifest", rune, "--out", sig); rc != exitOK {
		t.Fatalf("sign: rc=%d out=%q err=%q", rc, out, errs)
	}
	sigBytes, err := os.ReadFile(sig)
	if err != nil {
		t.Fatal(err)
	}
	cask := filepath.Join(dir, "cask.tar.gz")
	if err := os.WriteFile(cask, casktest.Tar(t, casktest.Entries(files, m, sigBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	if rc, out, errs := runCLI(t, "cask", "verify", "--cask", cask, "--pubkey", pub); rc != exitOK || !strings.Contains(out, "VERIFIED") {
		t.Fatalf("verify: rc=%d out=%q err=%q", rc, out, errs)
	}
	other := casktest.NewKey(t)
	_, otherPub := other.WriteFiles(t, t.TempDir())
	if rc, _, _ := runCLI(t, "cask", "verify", "--cask", cask, "--pubkey", otherPub); rc != exitFail {
		t.Fatalf("wrong key: rc=%d", rc)
	}
}

func TestCaskSignRefusals(t *testing.T) {
	dir := t.TempDir()
	k := casktest.NewKey(t)
	seed, _ := k.WriteFiles(t, dir)
	bad := filepath.Join(dir, "bad.sha256")
	if err := os.WriteFile(bad, []byte("not a rune\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "sig")
	if rc, _, errs := runCLI(t, "cask", "sign", "--key", seed, "--manifest", bad, "--out", out); rc != exitFail || !strings.Contains(errs, "Binding Rune") {
		t.Fatalf("malformed rune: rc=%d err=%q", rc, errs)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("signature written for a refused rune")
	}
	good := filepath.Join(dir, "good.sha256")
	if err := os.WriteFile(good, casktest.Manifest(casktest.Files()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(seed, 0o644); err != nil {
		t.Fatal(err)
	}
	if rc, _, errs := runCLI(t, "cask", "sign", "--key", seed, "--manifest", good, "--out", out); rc != exitFail || !strings.Contains(errs, "readable by group or others") {
		t.Fatalf("world-readable seed: rc=%d err=%q", rc, errs)
	}
	for _, args := range [][]string{
		{"cask"},
		{"cask", "frob"},
		{"cask", "sign", "--manifest", good},
		{"cask", "sign", "--manifest", good, "--out", out, "extra"},
		{"cask", "verify", "--cask", "x"},
	} {
		if rc, _, _ := runCLI(t, args...); rc != exitUsage {
			t.Errorf("%q: rc=%d", args, rc)
		}
	}
}
