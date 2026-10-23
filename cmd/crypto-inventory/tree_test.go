// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates root/rel with content, making parent directories.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTreeRead_RefusesEscapes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/b.txt", "x")
	tr := &Tree{Root: root}
	for _, bad := range []string{"../etc/passwd", "/etc/passwd", "a/../../x", "a//b.txt", "./a/b.txt", ".."} {
		if _, err := tr.Read(bad); err == nil {
			t.Errorf("Read(%q) succeeded; want refusal", bad)
		}
	}
	if b, err := tr.Read("a/b.txt"); err != nil || string(b) != "x" {
		t.Fatalf("Read(a/b.txt) = %q, %v", b, err)
	}
}

func TestTreeRead_RefusesSymlinkAndHuge(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "real.txt", "x")
	if err := os.Symlink("/etc/hostname", filepath.Join(root, "link.txt")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	tr := &Tree{Root: root}
	if _, err := tr.Read("link.txt"); err == nil {
		t.Error("a symlink must not be followed out of the tree")
	}
	big := filepath.Join(root, "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := tr.Read("big.bin"); !errors.Is(err, errTooLarge) {
		t.Errorf("oversized file: err = %v, want errTooLarge", err)
	}
}

func TestExcluded(t *testing.T) {
	for _, p := range []string{"vendor/x.go", "a/vendor/x.go", "testdata/x.go", "cmd/y/testdata/go/a.go", "eval/two-brains/fixtures/go-nil-map/go.mod"} {
		if !excluded(p) {
			t.Errorf("%s should be excluded", p)
		}
	}
	for _, p := range []string{"pkg/tls/server.go", "cmd/vendorish/x.go", "docs/testdata.md", "cmd/eval/x.go"} {
		if excluded(p) {
			t.Errorf("%s should not be excluded", p)
		}
	}
}

func TestDirTreeSortedAndRelative(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "z.txt", "")
	writeFile(t, root, "a/b.txt", "")
	tr, err := dirTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Files) != 2 || tr.Files[0] != "a/b.txt" || tr.Files[1] != "z.txt" {
		t.Fatalf("files = %v", tr.Files)
	}
	if !tr.Has("a/b.txt") || tr.Has("b.txt") {
		t.Error("Has is wrong")
	}
}

func TestLineOf(t *testing.T) {
	data := []byte("a\nb\nc")
	for off, want := range map[int]int{0: 1, 1: 1, 2: 2, 4: 3, 99: 3} {
		if got := lineOf(data, off); got != want {
			t.Errorf("lineOf(%d) = %d, want %d", off, got, want)
		}
	}
}
