// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/auth"
)

type result struct {
	code           int
	stdout, stderr string
}

func runKeygen(t *testing.T, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return result{code: code, stdout: out.String(), stderr: errb.String()}
}

func dirs(t *testing.T) (keys, trust string) {
	t.Helper()
	base := t.TempDir()
	return filepath.Join(base, "keys"), filepath.Join(base, "trust.d")
}

func mode(t *testing.T, p string) os.FileMode {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

func trustKids(t *testing.T, dir string) []string {
	t.Helper()
	td, err := auth.NewTrustDir(dir)
	if err != nil {
		t.Fatalf("trust dir %s: %v", dir, err)
	}
	return td.Kids()
}

func TestCreate(t *testing.T) {
	keys, trust := dirs(t)
	r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "cuirass")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if got := mode(t, filepath.Join(keys, "cuirass.key")); got != 0o400 {
		t.Errorf("key mode %04o, want 0400", got)
	}
	if got := mode(t, filepath.Join(keys, "cuirass.pub")); got != 0o444 {
		t.Errorf("pub mode %04o, want 0444", got)
	}
	if got := mode(t, filepath.Join(trust, "cuirass.pub")); got != 0o444 {
		t.Errorf("trust pub mode %04o, want 0444", got)
	}
	if got := mode(t, keys); got != keyDirMode {
		t.Errorf("key dir mode %04o, want %04o", got, keyDirMode)
	}
	if got := mode(t, trust); got != 0o755 {
		t.Errorf("trust dir mode %04o, want 0755", got)
	}
	_, kid, err := auth.LoadPrivateKey(filepath.Join(keys, "cuirass.key"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.stdout, kid) {
		t.Errorf("stdout %q does not name kid %s", r.stdout, kid)
	}
	if got := trustKids(t, trust); len(got) != 1 || got[0] != kid {
		t.Errorf("trust kids %v, want [%s]", got, kid)
	}
	raw, _ := os.ReadFile(filepath.Join(keys, "cuirass.key"))
	all := r.stdout + r.stderr
	if strings.Contains(all, strings.TrimSpace(string(raw))) || strings.Contains(all, "SEED") {
		t.Error("output contains the private key")
	}
}

func TestCreateReplacesStaleTrustPub(t *testing.T) {
	// A .pub left in trust.d with no private key behind it (a reinstall)
	// is replaced, not kept trusted next to the new key.
	keys, trust := dirs(t)
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "cuirass"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	if err := os.Remove(filepath.Join(keys, "cuirass.key")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(keys, "cuirass.pub")); err != nil {
		t.Fatal(err)
	}
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "cuirass"); r.code != 0 {
		t.Fatalf("exit %d %s", r.code, r.stderr)
	}
	_, kid, _ := auth.LoadPrivateKey(filepath.Join(keys, "cuirass.key"))
	if got := trustKids(t, trust); len(got) != 1 || got[0] != kid {
		t.Errorf("trust kids %v, want only the new %s", got, kid)
	}
}

func TestCreateTwiceRefused(t *testing.T) {
	keys, trust := dirs(t)
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "cuirass"); r.code != 0 {
		t.Fatalf("first run: exit %d %s", r.code, r.stderr)
	}
	before, _ := os.ReadFile(filepath.Join(keys, "cuirass.key"))
	r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "cuirass")
	if r.code != 2 || !strings.Contains(r.stderr, "key exists") {
		t.Fatalf("second run: exit %d stderr %q, want 2 and \"key exists\"", r.code, r.stderr)
	}
	after, _ := os.ReadFile(filepath.Join(keys, "cuirass.key"))
	if !bytes.Equal(before, after) {
		t.Error("refused run changed the key")
	}
}

func TestRotateKeepsOldPubUntilRetire(t *testing.T) {
	keys, trust := dirs(t)
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "cuirass"); r.code != 0 {
		t.Fatal(r.stderr)
	}
	_, oldKid, _ := auth.LoadPrivateKey(filepath.Join(keys, "cuirass.key"))

	r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-rotate", "cuirass")
	if r.code != 0 {
		t.Fatalf("rotate: exit %d %s", r.code, r.stderr)
	}
	_, newKid, err := auth.LoadPrivateKey(filepath.Join(keys, "cuirass.key"))
	if err != nil {
		t.Fatal(err)
	}
	if newKid == oldKid {
		t.Fatal("rotate kept the same kid")
	}
	if got := mode(t, filepath.Join(keys, "cuirass.key")); got != 0o400 {
		t.Errorf("rotated key mode %04o", got)
	}
	if got := trustKids(t, trust); len(got) != 2 {
		t.Fatalf("after rotate trust kids %v, want old and new", got)
	}
	td, _ := auth.NewTrustDir(trust)
	for _, k := range []string{oldKid, newKid} {
		if _, name, ok := td.LookupName(k); !ok || name != "cuirass" {
			t.Errorf("kid %s: name %q ok %v, want cuirass", k, name, ok)
		}
	}
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-print-kid", "cuirass"); r.code != 0 || strings.TrimSpace(r.stdout) != newKid {
		t.Errorf("print-kid: exit %d out %q, want %s", r.code, r.stdout, newKid)
	}
	// A second rotation keeps both earlier keys until each is retired.
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-rotate", "cuirass"); r.code != 0 {
		t.Fatalf("second rotate: exit %d %s", r.code, r.stderr)
	}
	_, newest, _ := auth.LoadPrivateKey(filepath.Join(keys, "cuirass.key"))
	if got := trustKids(t, trust); len(got) != 3 {
		t.Fatalf("after second rotate trust kids %v, want 3", got)
	}

	// The current key cannot be retired.
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-retire", newest, "cuirass"); r.code != 2 {
		t.Errorf("retiring the current kid: exit %d, want 2", r.code)
	}
	// An unknown kid is an error, not a silent no-op.
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-retire", "0000000000000000", "cuirass"); r.code != 2 {
		t.Errorf("retiring an unknown kid: exit %d, want 2", r.code)
	}
	for _, k := range []string{oldKid, newKid} {
		if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-retire", k, "cuirass"); r.code != 0 {
			t.Fatalf("retire %s: exit %d %s", k, r.code, r.stderr)
		}
	}
	if got := trustKids(t, trust); len(got) != 1 || got[0] != newest {
		t.Errorf("after retire trust kids %v, want [%s]", got, newest)
	}
}

func TestRetireOnlyOwnName(t *testing.T) {
	// -retire for one service never removes another service's key.
	keys, trust := dirs(t)
	for _, n := range []string{"cuirass", "wotan"} {
		if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, n); r.code != 0 {
			t.Fatal(r.stderr)
		}
	}
	_, wotanKid, _ := auth.LoadPrivateKey(filepath.Join(keys, "wotan.key"))
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-retire", wotanKid, "cuirass"); r.code != 2 {
		t.Errorf("retiring another service's kid: exit %d, want 2", r.code)
	}
	if got := trustKids(t, trust); len(got) != 2 {
		t.Errorf("trust kids %v, want both", got)
	}
}

func TestRotateWithoutKeyRefused(t *testing.T) {
	keys, trust := dirs(t)
	if r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-rotate", "cuirass"); r.code != 2 {
		t.Errorf("rotate with no key: exit %d, want 2", r.code)
	}
}

func TestOwnerRefusedWhenNotRoot(t *testing.T) {
	old := geteuid
	geteuid = func() int { return 1000 }
	t.Cleanup(func() { geteuid = old })
	keys, trust := dirs(t)
	r := runKeygen(t, "-dir", keys, "-trust-dir", trust, "-owner", "16729", "cuirass")
	if r.code != 2 || !strings.Contains(r.stderr, "root") {
		t.Fatalf("exit %d stderr %q, want 2 and a message naming root", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(keys, "cuirass.key")); err == nil {
		t.Error("a refused run created a key")
	}
}

func TestParseOwner(t *testing.T) {
	uid, gid, err := parseOwner("16729:16730")
	if err != nil || uid != 16729 || gid != 16730 {
		t.Errorf("16729:16730 -> %d %d %v", uid, gid, err)
	}
	uid, gid, err = parseOwner("16729")
	if err != nil || uid != 16729 || gid != 16729 {
		t.Errorf("16729 -> %d %d %v", uid, gid, err)
	}
	for _, bad := range []string{"", "0", "0:0", "-1", "16729:0", "16729:", "no-such-user-zz9"} {
		if _, _, err := parseOwner(bad); err == nil {
			t.Errorf("parseOwner(%q) accepted", bad)
		}
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"a", "b"},
		{"Bad.Name"},
		{"-rotate", "-retire", "0123456789abcdef", "x"},
		{"-no-such-flag", "x"},
	} {
		if r := runKeygen(t, args...); r.code != 2 {
			t.Errorf("args %q: exit %d, want 2", args, r.code)
		}
	}
	r := runKeygen(t, "-h")
	if r.code != 0 || !strings.Contains(r.stdout, "Exit status") || r.stderr != "" {
		t.Errorf("-h: exit %d, stdout lacks the exit-status table or stderr is not empty (%q)", r.code, r.stderr)
	}
	r = runKeygen(t, "-no-such-flag", "x")
	if r.code != 2 || !strings.Contains(r.stderr, "unrecognized option '-no-such-flag'") || !strings.Contains(r.stderr, "Try 'unheaded-keygen --help'") {
		t.Errorf("bad flag: exit %d, stderr %q", r.code, r.stderr)
	}
	if strings.Contains(r.stderr, "Usage:") {
		t.Errorf("bad flag must not dump the usage text: %q", r.stderr)
	}
}

func TestSameDirRefused(t *testing.T) {
	// Pointing -trust-dir at the key directory would leave a private key
	// where only public keys belong; refuse it.
	keys, _ := dirs(t)
	if r := runKeygen(t, "-dir", keys, "-trust-dir", keys, "cuirass"); r.code != 2 {
		t.Errorf("same dir for keys and trust: exit %d, want 2", r.code)
	}
}
