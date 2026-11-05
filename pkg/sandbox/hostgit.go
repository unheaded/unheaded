// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Host-side git on a directory a sandboxed check could write (H4). A check
// runs with the worktree writable, .git included, and git reads that .git
// when it runs on the host afterwards.
//
// CheckGitLink is the control: a worktree is trusted to git only once it
// passes, and any command that reads the working tree or the index for
// content (status, add, commit, diff) runs after it. HostGit narrows what a
// repository's config can do and gives git none of the caller's environment
// but the variables below, but it cannot make a changed .git safe: git has
// settings no command line can switch off. Before CheckGitLink, a caller may
// run only ls-files, symbolic-ref and rev-parse, which HostGit's tests pin.

// hostGitConfig overrides, on the command line, the two settings that run a
// program on every command, and keeps git out of a submodule and an implicit
// bare repository. Command-line config beats the repository's, and git hands
// it on to any child git.
var hostGitConfig = []string{
	"core.fsmonitor=false",
	"core.hooksPath=/dev/null",
	"diff.ignoreSubmodules=all",
	"safe.bareRepository=explicit",
}

// hostGitEnvKept is all the caller's environment git sees. HOME and
// XDG_CONFIG_HOME keep the committer identity in the user's global config,
// which a sandboxed check cannot reach.
var hostGitEnvKept = []string{"PATH", "HOME", "XDG_CONFIG_HOME", "LANG", "LC_ALL", "TZ"}

// HostGit returns a git command for dir, ready to run on the host.
func HostGit(ctx context.Context, dir string, args ...string) *exec.Cmd {
	argv := make([]string, 0, 2*len(hostGitConfig)+2+len(args))
	for _, kv := range hostGitConfig {
		argv = append(argv, "-c", kv)
	}
	argv = append(argv, "-C", dir)
	argv = append(argv, args...)
	cmd := exec.CommandContext(ctx, "git", argv...) // #nosec G204 -- fixed binary, no shell; callers pass validated values
	cmd.Env = []string{
		"GIT_CONFIG_NOSYSTEM=1", "GIT_ATTR_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat",
	}
	for _, k := range hostGitEnvKept {
		if v, ok := os.LookupEnv(k); ok {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	return cmd
}

// ErrGitLinkChanged means a worktree's .git is not the one recorded.
var ErrGitLinkChanged = errors.New("the worktree's .git changed")

// maxGitLinkBytes bounds a linked worktree's .git file: one gitdir line.
const maxGitLinkBytes = 8 << 10

// RecordGitLink returns the .git file of a linked worktree, before anything
// sandboxed runs in it.
func RecordGitLink(dir string) ([]byte, error) {
	return readGitLink(dir)
}

// CheckGitLink refuses, with ErrGitLinkChanged, a worktree whose .git is not
// the regular file recorded, byte for byte, or that holds any other .git, or
// that it cannot read to the bottom. A caller checks before any git runs in
// the worktree after a sandboxed run, and the unit must be gone by then:
// Run sees to that.
func CheckGitLink(dir string, want []byte) error {
	got, err := readGitLink(dir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGitLinkChanged, err)
	}
	if !bytes.Equal(got, want) {
		return ErrGitLinkChanged
	}
	found, err := gitEntries(dir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGitLinkChanged, err)
	}
	for _, p := range found {
		if p != filepath.Join(dir, ".git") {
			return fmt.Errorf("%w: another at %q", ErrGitLinkChanged, p)
		}
	}
	return nil
}

// QuarantineGit leaves a refused worktree safe to keep on disk. Every .git
// in it moves into a new directory under dir, under a name git does not look
// for, and link, the file RecordGitLink returned, goes back at the top so
// `git worktree remove` still knows the worktree. It returns that directory,
// and an error if anything stayed where it was. It runs no git.
func QuarantineGit(dir string, link []byte) (string, error) {
	quarantine, err := os.MkdirTemp(dir, ".refused-git-")
	if err != nil {
		return "", err
	}
	found, errs := gitEntries(dir)
	for i, p := range found {
		errs = errors.Join(errs, os.Rename(p, filepath.Join(quarantine, strconv.Itoa(i))))
	}
	return quarantine, errors.Join(errs, writeNewFile(filepath.Join(dir, ".git"), link))
}

// writeNewFile writes a file that must not exist yet, so a link cannot lead
// the write elsewhere.
func writeNewFile(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302,G304 -- the .git of a worktree the caller named, as git writes it
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	return errors.Join(err, f.Close())
}

// gitEntries returns every entry named .git under dir, the top one included,
// of any type, without following a link. It goes on past what it cannot read
// and returns that too: the caller decides what an unread directory means.
func gitEntries(dir string) ([]string, error) {
	var found []string
	var errs error
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = errors.Join(errs, err)
			return nil
		}
		if !strings.EqualFold(d.Name(), ".git") || p == dir {
			return nil
		}
		found = append(found, p)
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return found, errs
}

func readGitLink(dir string) ([]byte, error) {
	p := filepath.Join(dir, ".git")
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", p)
	}
	f, err := os.Open(p) // #nosec G304 -- the .git of a worktree the caller named
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxGitLinkBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxGitLinkBytes {
		return nil, fmt.Errorf("%s is over %d bytes", p, maxGitLinkBytes)
	}
	return b, nil
}
