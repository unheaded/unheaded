// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// M-124 / ADR T-05 (S-CHAMPION-HARDENING Steps 68-73): a symlink swapped
// in after validatePath passed must not redirect a write out of the
// sandbox.

// requireOpenat2 skips on a kernel without openat2 (< 5.6, ENOSYS) only
// (Appendix A3); every other error is a failure.
func requireOpenat2(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	fd, err := openRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFD(fd)
	if err := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := openBeneath(fd, "probe", os.O_RDONLY, 0)
	if errors.Is(err, syscall.ENOSYS) {
		t.Skip("openat2 unavailable (kernel < 5.6)")
	}
	if err != nil {
		t.Fatalf("openat2 probe: %v", err)
	}
	_ = f.Close()
}

// swapToSymlink replaces dir with a symlink to target.
func swapToSymlink(t *testing.T, dir, target string) func() {
	return func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("swap: %v", err)
			return
		}
		if err := os.Symlink(target, dir); err != nil {
			t.Errorf("swap: %v", err)
		}
	}
}

func assertEmpty(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Fatalf("the write landed outside the sandbox: %s has %d entries", dir, len(ents))
	}
}

func TestWriteFile_SymlinkRaceRefused(t *testing.T) {
	requireOpenat2(t)
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o750); err != nil {
		t.Fatal(err)
	}
	c, _ := New(Config{ProjectRoot: root}, &mockStore{})
	c.beforeOpen = swapToSymlink(t, filepath.Join(root, "a"), outside)

	err := c.WriteFile(context.Background(), filepath.Join(root, "a", "b.txt"), "pwned")
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("WriteFile through a swapped symlink = %v; want a symlink refusal", err)
	}
	assertEmpty(t, outside)
}

func TestWriteFile_SymlinkRaceRefused_NewParent(t *testing.T) {
	// The parent does not exist at validation; it is created, then swapped.
	requireOpenat2(t)
	root := t.TempDir()
	outside := t.TempDir()
	c, _ := New(Config{ProjectRoot: root}, &mockStore{})
	c.beforeOpen = swapToSymlink(t, filepath.Join(root, "new"), outside)
	if err := c.WriteFile(context.Background(), filepath.Join(root, "new", "x.txt"), "pwned"); err == nil {
		t.Fatal("a write through a swapped, freshly created parent ran")
	}
	assertEmpty(t, outside)
}

func TestPatchFile_SymlinkRaceRefused(t *testing.T) {
	requireOpenat2(t)
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "b.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The look-alike outside: if the swap wins, this file gets patched.
	if err := os.WriteFile(filepath.Join(outside, "b.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := New(Config{ProjectRoot: root}, &mockStore{})
	c.beforeOpen = swapToSymlink(t, filepath.Join(root, "a"), outside)
	err := c.PatchFile(context.Background(), filepath.Join(root, "a", "b.txt"), "old", "new")
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("PatchFile through a swapped symlink = %v; want a symlink refusal", err)
	}
	if got, _ := os.ReadFile(filepath.Join(outside, "b.txt")); string(got) != "old" {
		t.Fatal("the patch landed outside the sandbox")
	}
}

func TestWriteFile_FinalComponentSymlinkRefused(t *testing.T) {
	// A symlink as the file itself (pointing out) is refused even with no race.
	requireOpenat2(t)
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := New(Config{ProjectRoot: root}, &mockStore{})
	link := filepath.Join(root, "link.txt")
	c.beforeOpen = func() {
		if err := os.Symlink(outside, link); err != nil {
			t.Errorf("symlink: %v", err)
		}
	}
	if err := c.WriteFile(context.Background(), link, "pwned"); err == nil {
		t.Fatal("a write through a final-component symlink ran")
	}
	if got, _ := os.ReadFile(outside); string(got) != "keep" {
		t.Fatal("the symlink target was overwritten")
	}
}
