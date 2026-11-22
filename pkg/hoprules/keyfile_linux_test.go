// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package hoprules

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// realTempDir is a scratch directory named by its real path: a key_file under
// a symlinked TMPDIR is refused, which is the check and not the test's subject.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func keyIn(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "hop.key")
	if err := os.WriteFile(p, testKey(), 0o400); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadOne(path string) (*File, error) {
	f := &File{HMACKeys: []HMACKey{{KeyFile: path}}}
	return f, f.Load()
}

// refused wants the sentinel, the file's path and the reason in the message,
// and no key left behind.
func refused(t *testing.T, path string, want error, reason string) {
	t.Helper()
	f, err := loadOne(path)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	for _, s := range []string{path, reason} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("refusal %q does not say %q", err, s)
		}
	}
	if f.HMACKeys[0].key != nil {
		t.Error("a refused file left a key behind")
	}
}

// A second name for the key's inode is a second way to it, in a directory
// whose mode this check never saw.
func TestLoad_RefusesAHardLinkedKeyFile(t *testing.T) {
	dir := realTempDir(t)
	p := keyIn(t, dir)
	if err := os.Link(p, filepath.Join(dir, "second-name")); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	refused(t, p, ErrKeyFileLinks, "2 links")
}

// Whoever owns a link above the file chooses which file is read.
func TestLoad_RefusesAKeyFileUnderASymlinkedDirectory(t *testing.T) {
	dir := realTempDir(t)
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keyIn(t, realDir)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	refused(t, filepath.Join(link, "hop.key"), ErrKeyFileSymlink, "symlink")
}

func TestLoad_RefusesASymlinkedKeyFile(t *testing.T) {
	dir := realTempDir(t)
	p := keyIn(t, dir)
	link := filepath.Join(dir, "link.key")
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	refused(t, link, ErrKeyFileSymlink, "symlink")
}

// A 0600 file of some other user is a key that user can change.
func TestLoad_RefusesAKeyFileOfAnotherUser(t *testing.T) {
	p := keyIn(t, realTempDir(t))
	if os.Geteuid() == 0 {
		if err := os.Chown(p, 12345, -1); err != nil {
			t.Fatal(err)
		}
	} else {
		// Not root, so the file cannot be given away: this process becomes
		// the other user instead.
		orig := geteuid
		geteuid = func() int { return orig() + 1 }
		t.Cleanup(func() { geteuid = orig })
	}
	refused(t, p, ErrKeyFileOwner, "owned by uid")
}

func TestKeyOwnerOK(t *testing.T) {
	cases := []struct {
		owner uint32
		euid  int
		want  bool
	}{
		{0, 0, true},
		{0, 1000, true}, // root's file, read through a capability or a group
		{1000, 1000, true},
		{1000, 0, false},
		{1001, 1000, false},
	}
	for _, c := range cases {
		if got := keyOwnerOK(c.owner, c.euid); got != c.want {
			t.Errorf("keyOwnerOK(owner %d, euid %d) = %v, want %v", c.owner, c.euid, got, c.want)
		}
	}
}

// What a correctly set up host has still loads: the caller's own 0400 file,
// one link, named by its real path.
func TestLoad_AcceptsTheCallersOwnKeyFile(t *testing.T) {
	f, err := loadOne(keyIn(t, realTempDir(t)))
	if err != nil {
		t.Fatal(err)
	}
	if k := f.HMACKeys[0].key(); !bytes.Equal(k[:], testKey()) {
		t.Errorf("key = %x", k)
	}
}

// A FIFO at key_file must be refused, not waited on.
func TestLoad_AFIFODoesNotBlock(t *testing.T) {
	p := filepath.Join(realTempDir(t), "fifo.key")
	if err := unix.Mkfifo(p, 0o600); err != nil {
		t.Skipf("no FIFO here: %v", err)
	}
	if _, err := loadOne(p); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want a refusal of what is not a regular file", err)
	}
}
