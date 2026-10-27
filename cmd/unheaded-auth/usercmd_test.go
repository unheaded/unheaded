// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCmd(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

// cheapDefaults makes `user add` hash with test parameters.
func cheapDefaults(t *testing.T) {
	t.Helper()
	old := defaultParams
	defaultParams = testParams
	t.Cleanup(func() { defaultParams = old })
}

func TestUserAddListDel(t *testing.T) {
	cheapDefaults(t)
	path := filepath.Join(t.TempDir(), "users.json")
	if code, _, e := runCmd(t, "s3cret pass\n", "user", "add", "alice", "--roles", "viewer,operator", "--password-stdin", "-users-file", path); code != 0 {
		t.Fatalf("add: exit %d %s", code, e)
	}
	if code, _, e := runCmd(t, "other pass\n", "user", "add", "-users-file", path, "--roles=viewer", "--password-stdin", "bob"); code != 0 {
		t.Fatalf("add bob: exit %d %s", code, e)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("users file mode %04o, want 0600", st.Mode().Perm())
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "s3cret") {
		t.Fatal("users file holds the password")
	}
	users, err := loadUsers(path)
	if err != nil {
		t.Fatalf("issuer cannot load what user add wrote: %v", err)
	}
	if !users["alice"].hash.verify("s3cret pass") || users["alice"].hash.verify("other pass") {
		t.Error("alice's hash does not verify her password only")
	}
	code, out, _ := runCmd(t, "", "user", "list", "-users-file", path)
	if code != 0 || out != "alice\toperator,viewer\nbob\tviewer\n" {
		t.Errorf("list: exit %d out %q", code, out)
	}
	if code, _, _ := runCmd(t, "x\n", "user", "add", "alice", "--roles", "viewer", "--password-stdin", "-users-file", path); code != 2 {
		t.Errorf("duplicate add: exit %d, want 2", code)
	}
	if code, _, e := runCmd(t, "", "user", "del", "alice", "-users-file", path); code != 0 {
		t.Fatalf("del: exit %d %s", code, e)
	}
	if _, out, _ := runCmd(t, "", "user", "list", "-users-file", path); out != "bob\tviewer\n" {
		t.Errorf("after del: %q", out)
	}
	if code, _, _ := runCmd(t, "", "user", "del", "alice", "-users-file", path); code != 2 {
		t.Errorf("del of a missing user: exit %d, want 2", code)
	}
}

func TestUserAddRefusals(t *testing.T) {
	cheapDefaults(t)
	path := filepath.Join(t.TempDir(), "users.json")
	for name, args := range map[string][]string{
		"empty password": {"user", "add", "carol", "--roles", "viewer", "--password-stdin"},
		"no roles":       {"user", "add", "carol", "--password-stdin"},
		"service role":   {"user", "add", "carol", "--roles", "service", "--password-stdin"},
		"unknown role":   {"user", "add", "carol", "--roles", "root", "--password-stdin"},
		"bad name":       {"user", "add", "Carol!", "--roles", "viewer", "--password-stdin"},
		"two names":      {"user", "add", "carol", "dave", "--roles", "viewer", "--password-stdin"},
	} {
		stdin := "pw\n"
		if name == "empty password" {
			stdin = "\n"
		}
		code, _, _ := runCmd(t, stdin, append(args, "-users-file", path)...)
		if code != 2 {
			t.Errorf("%s: exit %d, want 2", name, code)
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a refused add wrote the users file")
	}
	long := strings.Repeat("x", maxPasswordLen+1) + "\n"
	if code, _, _ := runCmd(t, long, "user", "add", "carol", "--roles", "viewer", "--password-stdin", "-users-file", path); code != 2 {
		t.Errorf("over-long password: exit %d, want 2", code)
	}
}

func TestUserAddPromptsTwice(t *testing.T) {
	cheapDefaults(t)
	path := filepath.Join(t.TempDir(), "users.json")
	answers := []string{"same pw", "same pw"}
	old := readPassword
	t.Cleanup(func() { readPassword = old })
	readPassword = func(io.Writer, string) (string, error) {
		a := answers[0]
		answers = answers[1:]
		return a, nil
	}
	if code, _, e := runCmd(t, "", "user", "add", "erin", "--roles", "admin", "-users-file", path); code != 0 {
		t.Fatalf("exit %d %s", code, e)
	}
	answers = []string{"one", "two"}
	if code, _, _ := runCmd(t, "", "user", "add", "frank", "--roles", "admin", "-users-file", path); code != 2 {
		t.Errorf("mismatched prompts: exit %d, want 2", code)
	}
	readPassword = func(io.Writer, string) (string, error) { return "", errors.New("no terminal") }
	if code, _, _ := runCmd(t, "", "user", "add", "gina", "--roles", "admin", "-users-file", path); code != 1 {
		t.Errorf("no terminal: exit %d, want 1", code)
	}
}

func TestUserCmdRefusesExposedFile(t *testing.T) {
	cheapDefaults(t)
	path := filepath.Join(t.TempDir(), "users.json")
	if code, _, _ := runCmd(t, "pw\n", "user", "add", "alice", "--roles", "viewer", "--password-stdin", "-users-file", path); code != 0 {
		t.Fatal("add failed")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCmd(t, "", "user", "list", "-users-file", path); code != 1 {
		t.Errorf("list of a 0644 file: exit %d, want 1", code)
	}
}

func TestDefaultParamsRoundTrip(t *testing.T) {
	// The production parameters hash and verify, and parse within bounds.
	h, err := hashPassword("pw", defaultParams)
	if err != nil {
		t.Fatal(err)
	}
	old := minArgonMemoryKiB
	minArgonMemoryKiB = 19 * 1024
	t.Cleanup(func() { minArgonMemoryKiB = old })
	ph, err := parseHash(h)
	if err != nil {
		t.Fatalf("production hash refused: %v", err)
	}
	if !ph.verify("pw") || ph.verify("pW") {
		t.Error("verify")
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Errorf("hash %q", h)
	}
	if _, err := parseHash(strings.Replace(h, "m=65536", "m=1024", 1)); err == nil {
		t.Error("weak memory parameter accepted at the production floor")
	}
}

func TestUserUsage(t *testing.T) {
	for _, args := range [][]string{{"user"}, {"user", "frob"}, {"user", "list", "x"}, {"user", "del"}} {
		if code, _, _ := runCmd(t, "", args...); code != 2 {
			t.Errorf("%q: exit %d, want 2", args, code)
		}
	}
}
