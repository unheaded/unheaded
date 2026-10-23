// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Key is one pre-registered expected-finding key (keys.jsonl).
type Key struct {
	ID    string   `json:"id"`
	AllOf []string `json:"all_of"`
	AnyOf []string `json:"any_of"`

	all, any []*regexp.Regexp
}

// compile prepares the key's regexes (case-insensitive).
func (k *Key) compile() error {
	k.all, k.any = nil, nil
	for _, p := range k.AllOf {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return fmt.Errorf("key %s: %w", k.ID, err)
		}
		k.all = append(k.all, re)
	}
	for _, p := range k.AnyOf {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return fmt.Errorf("key %s: %w", k.ID, err)
		}
		k.any = append(k.any, re)
	}
	if len(k.all) == 0 && len(k.any) == 0 {
		return fmt.Errorf("key %s is empty", k.ID)
	}
	return nil
}

// judgeKey: every all_of matches and, when any_of is non-empty, one of
// it. detail lists what was missing (pattern indexes, no answer text).
func judgeKey(k *Key, answer string) (bool, string) {
	if strings.TrimSpace(answer) == "" {
		return false, "empty answer"
	}
	var missing []string
	for i, re := range k.all {
		if !re.MatchString(answer) {
			missing = append(missing, fmt.Sprintf("all_of[%d]", i))
		}
	}
	if len(k.any) > 0 {
		hit := false
		for _, re := range k.any {
			if re.MatchString(answer) {
				hit = true
				break
			}
		}
		if !hit {
			missing = append(missing, "any_of")
		}
	}
	if len(missing) > 0 {
		return false, "missing " + strings.Join(missing, ",")
	}
	return true, "key matched"
}

// protectedFiles cannot be written by an answer.
var protectedFiles = map[string]bool{"check.sh": true, "TASK.md": true, "solution.patch": true}

// isFixtureTest reports a fixture's test or validator file: part of the
// check, so an answer may not change it (a weakened assertion would pass
// the check; found in the qwen-7b run, py-chunks-partial).
func isFixtureTest(rel string) bool {
	b := filepath.Base(rel)
	return strings.HasSuffix(b, "_test.go") || (strings.HasPrefix(b, "test_") && strings.HasSuffix(b, ".py")) || b == "validate_runbook.py"
}

// fileBlockRE matches ```file:<path>\n<content>``` blocks.
var fileBlockRE = regexp.MustCompile("(?s)```file:([^\\n`]+)\\n(.*?)```")

// FileBlock is one file an answer replaces or creates.
type FileBlock struct {
	Path    string
	Content string
}

// ErrNoFileBlocks: the answer carried no file block (a parse failure).
var ErrNoFileBlocks = errors.New("answer has no ```file:<path> blocks")

// parseFileBlocks extracts the file blocks of an answer.
func parseFileBlocks(answer string) ([]FileBlock, error) {
	ms := fileBlockRE.FindAllStringSubmatch(answer, -1)
	if len(ms) == 0 {
		return nil, ErrNoFileBlocks
	}
	out := make([]FileBlock, 0, len(ms))
	for _, m := range ms {
		out = append(out, FileBlock{Path: strings.TrimSpace(m[1]), Content: m[2]})
	}
	return out, nil
}

// applyBlocks writes blocks into dir. Every path must be relative, stay
// inside dir after cleaning and symlink resolution, and not name a
// protected file; otherwise nothing is written.
func applyBlocks(dir string, blocks []FileBlock) error {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	type write struct{ path, content string }
	var writes []write
	for _, b := range blocks {
		p := filepath.Clean(strings.TrimPrefix(b.Path, "./"))
		if p == "." || filepath.IsAbs(b.Path) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
			return fmt.Errorf("path %q is outside the fixture", b.Path)
		}
		if (protectedFiles[filepath.Base(p)] && filepath.Dir(p) == ".") || isFixtureTest(p) {
			return fmt.Errorf("path %q may not be changed", b.Path)
		}
		full := filepath.Join(root, p)
		parent := filepath.Dir(full)
		if rp, err := filepath.EvalSymlinks(parent); err == nil {
			if rp != root && !strings.HasPrefix(rp, root+string(filepath.Separator)) {
				return fmt.Errorf("path %q resolves outside the fixture", b.Path)
			}
		}
		if fi, err := os.Lstat(full); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("path %q is a symlink", b.Path)
		}
		writes = append(writes, write{full, b.Content})
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for _, w := range writes {
		rel, err := filepath.Rel(root, w.path)
		if err != nil {
			return err
		}
		if d := filepath.Dir(rel); d != "." {
			if err := r.MkdirAll(d, 0o750); err != nil {
				return err
			}
		}
		// os.Root refuses any path (or symlink) that leaves the fixture.
		if err := r.WriteFile(rel, []byte(w.content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// CheckRunner runs a fixture's check.sh in dir and reports the exit code
// and (truncated) output. Production: a no-network user sandbox.
type CheckRunner interface {
	Run(ctx context.Context, dir string) (exit int, output string, err error)
}

// copyTree copies a fixture to dst (regular files and dirs only), both
// sides through os.Root so a symlink cannot lead a read or write outside.
func copyTree(src, dst string) error {
	sr, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer func() { _ = sr.Close() }()
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return err
	}
	dr, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer func() { _ = dr.Close() }()
	return fs.WalkDir(sr.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			if p == "." {
				return nil
			}
			return dr.MkdirAll(p, 0o750)
		case d.Type().IsRegular():
			b, err := fs.ReadFile(sr.FS(), p)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			return dr.WriteFile(p, b, info.Mode().Perm())
		}
		return nil // symlinks and devices are not copied
	})
}

// CheckResult is one check judgement.
type CheckResult struct {
	Pass   bool
	Detail string // parse_fail, outside_fixture, check_fail exit=N, check_error
	Output string // check output (for the next local attempt), truncated
}

// judgeCheck copies the fixture, applies the answer's blocks and runs
// the check in a fresh directory under work.
func judgeCheck(ctx context.Context, fixture, work string, answer string, run CheckRunner) CheckResult {
	blocks, err := parseFileBlocks(answer)
	if err != nil {
		return CheckResult{Detail: "parse_fail"}
	}
	dir, err := os.MkdirTemp(work, "fx-")
	if err != nil {
		return CheckResult{Detail: "check_error: " + err.Error()}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := copyTree(fixture, dir); err != nil {
		return CheckResult{Detail: "check_error: " + err.Error()}
	}
	if err := applyBlocks(dir, blocks); err != nil {
		return CheckResult{Detail: "outside_fixture: " + err.Error()}
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	exit, out, err := run.Run(cctx, dir)
	if err != nil {
		return CheckResult{Detail: "check_error: " + err.Error(), Output: out}
	}
	if exit != 0 {
		return CheckResult{Detail: fmt.Sprintf("check_fail exit=%d", exit), Output: out}
	}
	return CheckResult{Pass: true, Detail: "check passed", Output: out}
}
