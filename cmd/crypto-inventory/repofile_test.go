// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every file the tool itself reads or writes (the CBOM, the baseline, host
// facts) stays inside the repository: a flag value is repository-relative
// or absolute inside the repository, never a way to touch another file.
func TestRepoFile_ConfinedToRepository(t *testing.T) {
	top := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(top, "compliance", "crypto"), 0o750); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"compliance/crypto/x.json", filepath.Join(top, "compliance", "crypto", "x.json")} {
		if err := writeRepoFile(top, p, []byte("ok\n")); err != nil {
			t.Fatalf("write %q: %v", p, err)
		}
		b, err := readRepoFile(top, p)
		if err != nil || string(b) != "ok\n" {
			t.Fatalf("read %q = %q, %v", p, b, err)
		}
	}

	refused := []string{
		"",
		".",
		"../escape.txt",
		"compliance/../../escape.txt",
		filepath.Join(outside, "escape.txt"),
		top,
	}
	for _, p := range refused {
		if err := writeRepoFile(top, p, []byte("x")); !errors.Is(err, errOutsideRepo) {
			t.Errorf("write %q: err = %v, want errOutsideRepo", p, err)
		}
		if _, err := readRepoFile(top, p); !errors.Is(err, errOutsideRepo) {
			t.Errorf("read %q: err = %v, want errOutsideRepo", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("a refused write created a file outside the repository")
	}

	// A symlink inside the repository that points out of it is refused by
	// the os.Root the write goes through, not followed.
	if err := os.Symlink(outside, filepath.Join(top, "link")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := writeRepoFile(top, "link/escape.txt", []byte("x")); err == nil {
		t.Error("write through a symlink out of the repository succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("a write through a symlink created a file outside the repository")
	}
}

func TestRun_ScanOutOutsideRepoRefused(t *testing.T) {
	root := gitFixture(t, "go")
	out := filepath.Join(t.TempDir(), "cbom.json")
	rc, _, errs := runCLI("scan", "-root", root, "-out", out)
	if rc != 2 || !strings.Contains(errs, "inside the repository") {
		t.Fatalf("scan --out outside the repo: rc=%d err=%q, want 2 and a refusal", rc, errs)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("scan wrote outside the repository")
	}
}
