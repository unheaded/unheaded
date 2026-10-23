// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	rc := run(args, &out, &errb)
	return rc, out.String(), errb.String()
}

func TestRun_Usage(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}, {"scan", "extra"}, {"check", "-bogus"},
		{"collect-host"}, {"collect-host", "-host", "east", "-out", "x", "-local", "-ssh", "a@b"},
		{"collect-host", "-host", "east", "-out", "x", "-ssh", "-oProxyCommand=x"}} {
		if rc, _, _ := runCLI(args...); rc != 2 {
			t.Errorf("run(%v) = %d, want 2", args, rc)
		}
	}
	if rc, out, _ := runCLI("help"); rc != 0 || !strings.Contains(out, "collect-host") {
		t.Errorf("help: rc=%d out=%q", rc, out)
	}
}

// gitFixture makes a throwaway git repo from a fixture so scan/check run
// end to end through git ls-files.
func gitFixture(t *testing.T, fixture string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := copyTree(t, fixture)
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

func TestRun_ScanThenCheck(t *testing.T) {
	root := gitFixture(t, "go")
	cbom := filepath.Join(root, "cbom.json")
	if rc, _, errs := runCLI("scan", "-root", root, "-out", cbom); rc != 0 {
		t.Fatalf("scan rc=%d: %s", rc, errs)
	}
	if rc, out, errs := runCLI("check", "-root", root, "-cbom", cbom); rc != 0 || !strings.Contains(out, "PASS") {
		t.Fatalf("check on a fresh cbom: rc=%d out=%q err=%q", rc, out, errs)
	}
	// An untracked new file is part of the tree: the cbom is now stale.
	writeFile(t, root, "pkg/fx/zz_new.go", "package fx\n\nimport \"crypto/tls\"\n\nvar Z = &tls.Config{CurvePreferences: []tls.CurveID{tls.X25519}}\n")
	rc, _, errs := runCLI("check", "-root", root, "-cbom", cbom)
	if rc != 1 || !strings.Contains(errs, "+ pkg/fx/zz_new.go:5:tls-config") {
		t.Fatalf("check after adding an untracked file: rc=%d err=%q", rc, errs)
	}
	if err := os.Remove(cbom); err != nil {
		t.Fatal(err)
	}
	if rc, _, errs := runCLI("check", "-root", root, "-cbom", cbom); rc != 1 || !strings.Contains(errs, "Generate it") {
		t.Fatalf("check with no cbom: rc=%d err=%q", rc, errs)
	}
	if rc, out, _ := runCLI("summary", "-root", root); rc != 0 || !strings.Contains(out, "classical") {
		t.Fatalf("summary: rc=%d out=%q", rc, out)
	}
}

func TestGitTree_SkipsDeletedAndSymlinks(t *testing.T) {
	root := gitFixture(t, "go")
	if err := os.Remove(filepath.Join(root, "pkg/fx/b_hybrid.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(root, "link.go")); err != nil {
		t.Skip("symlinks unsupported")
	}
	tr, err := gitTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Has("pkg/fx/b_hybrid.go") || tr.Has("link.go") {
		t.Errorf("tree lists a deleted file or a symlink: %v", tr.Files)
	}
	if !tr.Has("pkg/fx/a_classical.go") {
		t.Error("tracked file missing")
	}
}
