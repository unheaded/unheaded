// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/sealedcask/casktest"
)

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	rc := run(args, &out, &errb)
	return rc, out.String(), errb.String()
}

// writeCask writes cask bytes and k's public key into a temp dir.
func writeCask(t *testing.T, cask []byte, k *casktest.Key) (caskPath, pubPath string) {
	t.Helper()
	dir := t.TempDir()
	caskPath = filepath.Join(dir, "cask.tar.gz")
	if err := os.WriteFile(caskPath, cask, 0o600); err != nil {
		t.Fatal(err)
	}
	_, pubPath = k.WriteFiles(t, dir)
	return caskPath, pubPath
}

func TestVerifyGoodCask(t *testing.T) {
	cask, k := casktest.Good(t)
	c, p := writeCask(t, cask, k)
	rc, out, errs := runCLI("verify", "--cask", c, "--pubkey", p)
	if rc != exitOK || !strings.Contains(out, "VERIFIED") {
		t.Fatalf("rc=%d out=%q err=%q", rc, out, errs)
	}
}

func TestVerifyFlippedByte(t *testing.T) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	m := casktest.Manifest(files)
	sig := k.Seal(t, m, casktest.Purpose)
	files["configs/wotan.yaml"] = []byte("topics: {X}\n")
	c, p := writeCask(t, casktest.Tar(t, casktest.Entries(files, m, sig)), k)
	if rc, _, errs := runCLI("verify", "--cask", c, "--pubkey", p); rc != exitFail || !strings.Contains(errs, "modified configs/wotan.yaml") {
		t.Fatalf("rc=%d err=%q", rc, errs)
	}
}

func TestVerifyUnsigned(t *testing.T) {
	k := casktest.NewKey(t)
	files := casktest.Files()
	c, p := writeCask(t, casktest.Tar(t, casktest.Entries(files, casktest.Manifest(files), nil)), k)
	if rc, _, errs := runCLI("verify", "--cask", c, "--pubkey", p); rc != exitFail || !strings.Contains(errs, "UNSIGNED CASK") {
		t.Fatalf("rc=%d err=%q", rc, errs)
	}
}

func TestVerifyHostileMembers(t *testing.T) {
	R := casktest.Root
	for name, e := range map[string]casktest.Entry{
		"dotdot":           {Name: R + "/../../etc/cron.d/x"},
		"absolute":         {Name: "/etc/cron.d/x"},
		"symlink escaping": {Name: R + "/bin/sh", Type: tar.TypeSymlink, Linkname: "../../../../../bin/sh"},
	} {
		t.Run(name, func(t *testing.T) {
			k := casktest.NewKey(t)
			files := casktest.Files()
			m := casktest.Manifest(files)
			es := append(casktest.Entries(files, m, k.Seal(t, m, casktest.Purpose)), e)
			c, p := writeCask(t, casktest.Tar(t, es), k)
			if rc, _, errs := runCLI("verify", "--cask", c, "--pubkey", p); rc != exitFail || !strings.Contains(errs, "refused") {
				t.Fatalf("rc=%d err=%q", rc, errs)
			}
		})
	}
}

func TestVerifyWrongKey(t *testing.T) {
	cask, _ := casktest.Good(t)
	c, p := writeCask(t, cask, casktest.NewKey(t))
	if rc, _, errs := runCLI("verify", "--cask", c, "--pubkey", p); rc != exitFail || !strings.Contains(errs, "not a trusted build key") {
		t.Fatalf("rc=%d err=%q", rc, errs)
	}
}

func TestVerifyTwoPubkeys(t *testing.T) {
	cask, k := casktest.Good(t)
	c, p := writeCask(t, cask, k)
	_, oldPub := writeCask(t, nil, casktest.NewKey(t))
	if rc, out, errs := runCLI("verify", "--cask", c, "--pubkey", oldPub, "--pubkey", p); rc != exitOK {
		t.Fatalf("rc=%d out=%q err=%q", rc, out, errs)
	}
}

func TestExtract(t *testing.T) {
	cask, k := casktest.Good(t)
	c, p := writeCask(t, cask, k)
	dest := filepath.Join(t.TempDir(), "out")
	if rc, out, errs := runCLI("extract", "--cask", c, "--pubkey", p, "--dest", dest); rc != exitOK {
		t.Fatalf("rc=%d out=%q err=%q", rc, out, errs)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "configs/wotan.yaml")); err != nil || string(b) != "topics: {}\n" {
		t.Fatalf("%v %q", err, b)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"frobnicate"},
		{"verify"},
		{"verify", "--cask", "x"},
		{"verify", "--pubkey", "x"},
		{"verify", "--cask", "x", "--pubkey", "y", "extra"},
		{"extract", "--cask", "x", "--pubkey", "y"},
	} {
		if rc, _, _ := runCLI(args...); rc != exitUsage {
			t.Errorf("%q: rc=%d, want %d", args, rc, exitUsage)
		}
	}
	if rc, out, _ := runCLI("help"); rc != exitOK || !strings.Contains(out, "verify") {
		t.Errorf("help: rc=%d", rc)
	}
	if rc, _, _ := runCLI("verify", "--cask", "/nonexistent", "--pubkey", "/nonexistent"); rc != exitUsage {
		t.Errorf("unreadable pubkey: rc=%d", rc)
	}
}
