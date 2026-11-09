// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hostOutPath resolves a report path and refuses any inside the repository
// except under var/, which is gitignored. Reports built from evidence are
// collected data, and collected data lives on the host, never in the tree
// (Stevie, 2026-11-09). A relative path resolves against repoDir. Symlinks in
// the existing part of either path are followed before the check.
func hostOutPath(repoDir, out string) (string, error) {
	if out == "" {
		return "", errors.New("no output path")
	}
	path := out
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoDir, path)
	}
	repo, err := realPath(repoDir)
	if err != nil {
		return "", err
	}
	real, err := realPath(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(repo, real)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path, nil // outside the repository
	}
	if first, _, _ := strings.Cut(rel, string(filepath.Separator)); first == "var" && rel != "var" {
		return path, nil
	}
	return "", fmt.Errorf("%s: collected data stays on the host; write under var/ or outside the repository", out)
}

// realPath is filepath.Abs with symlinks resolved in the longest existing
// prefix, so a path need not exist yet.
func realPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rest := ""
	for cur := abs; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
