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

// errOutsideRepo refuses a --out / --cbom / --baseline path that does not
// name a file inside the repository.
var errOutsideRepo = errors.New("must name a file inside the repository")

// repoRel turns a command-line path into a path relative to the repository
// top: a relative path is taken relative to top, an absolute one must lie
// under it. Symlinks in the existing part of an absolute path are resolved
// first (a temp dir under a symlinked /tmp, as git reports top resolved).
func repoRel(top, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path: %w", errOutsideRepo)
	}
	realTop, err := filepath.EvalSymlinks(top)
	if err != nil {
		return "", fmt.Errorf("repository %s: %w", top, err)
	}
	full := p
	if !filepath.IsAbs(full) {
		full = filepath.Join(realTop, full)
	} else if dir, err := filepath.EvalSymlinks(filepath.Dir(full)); err == nil {
		full = filepath.Join(dir, filepath.Base(full))
	}
	rel, err := filepath.Rel(realTop, filepath.Clean(full))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: %w %s", p, errOutsideRepo, realTop)
	}
	return rel, nil
}

// readRepoFile reads p (see repoRel) through an os.Root at top, so a symlink
// inside the repository cannot lead the read out of it.
func readRepoFile(top, p string) ([]byte, error) {
	rel, err := repoRel(top, p)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(top)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return r.ReadFile(rel)
}

// writeRepoFile writes p (see repoRel) through an os.Root at top. Everything
// the tool writes is a checked-in, public file (CBOM, baseline, host facts
// that encodeHostFacts has already refused key material for), hence 0644.
func writeRepoFile(top, p string, b []byte) error {
	rel, err := repoRel(top, p)
	if err != nil {
		return err
	}
	r, err := os.OpenRoot(top)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.WriteFile(rel, b, 0o644)
}
