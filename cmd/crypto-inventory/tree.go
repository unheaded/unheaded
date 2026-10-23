// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// maxFileBytes bounds every file a scanner reads. Nothing tracked that
// configures crypto is this large; a bigger file is skipped, not truncated.
const maxFileBytes = 8 << 20

// Tree is the set of files a scan sees: repo-relative, slash-separated,
// sorted, so every scanner walks the same list in the same order.
type Tree struct {
	Root  string
	Files []string
}

// excluded reports files no scanner looks at: test fixtures, vendored code.
func excluded(rel string) bool {
	if strings.HasPrefix(rel, "vendor/") || strings.Contains(rel, "/vendor/") {
		return true
	}
	return strings.HasPrefix(rel, "testdata/") || strings.Contains(rel, "/testdata/")
}

// gitTree lists the tracked files of the repository containing root. It
// never globs the filesystem: worktrees and build output must not leak in.
func gitTree(root string) (*Tree, error) {
	top, err := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-parse in %s: %w", root, err)
	}
	topDir := strings.TrimSpace(string(top))
	out, err := exec.Command("git", "-C", topDir, "ls-files", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files in %s: %w", topDir, err)
	}
	var files []string
	for _, f := range bytes.Split(out, []byte{0}) {
		rel := string(f)
		if rel == "" || excluded(rel) {
			continue
		}
		files = append(files, rel)
	}
	sort.Strings(files)
	return &Tree{Root: topDir, Files: files}, nil
}

// dirTree lists every regular file under root. Test fixtures use it; the
// testdata exclusion does not apply because root itself is the fixture.
func dirTree(root string) (*Tree, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	sort.Strings(files)
	return &Tree{Root: root, Files: files}, nil
}

// errTooLarge marks a file over maxFileBytes.
var errTooLarge = errors.New("file larger than the scan limit")

// Read returns a tracked file's contents. rel must be one of t.Files'
// shape (relative, no ..), so a crafted name cannot read outside Root.
func (t *Tree) Read(rel string) ([]byte, error) {
	clean := path.Clean(rel)
	if clean != rel || path.IsAbs(rel) || strings.HasPrefix(clean, "../") || clean == ".." {
		return nil, fmt.Errorf("refusing path %q", rel)
	}
	full := filepath.Join(t.Root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", rel)
	}
	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("%s: %w", rel, errTooLarge)
	}
	return os.ReadFile(full) // #nosec G304 -- rel is a cleaned tracked path under Root
}

// Has reports whether rel is in the tree.
func (t *Tree) Has(rel string) bool {
	i := sort.SearchStrings(t.Files, rel)
	return i < len(t.Files) && t.Files[i] == rel
}

// Match returns the files for which keep returns true, in tree order.
func (t *Tree) Match(keep func(string) bool) []string {
	var out []string
	for _, f := range t.Files {
		if keep(f) {
			out = append(out, f)
		}
	}
	return out
}

// lineOf returns the 1-based line number of byte offset off in data.
func lineOf(data []byte, off int) int {
	if off > len(data) {
		off = len(data)
	}
	return bytes.Count(data[:off], []byte{'\n'}) + 1
}
