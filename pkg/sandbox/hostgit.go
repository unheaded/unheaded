// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Host-side git on a directory a sandboxed check could write (H4). A check
// runs with the worktree writable, .git included, and git reads that .git
// when it runs on the host afterwards. HostGit is the only way to run it
// there: nothing the repository's config names is run, and git gets none of
// the caller's environment but the variables below. RecordGitLink and
// CheckGitLink let a caller refuse a worktree whose .git changed at all.

// hostGitConfig overrides, on the command line, the settings that make git
// run a program or look in another repository. Command-line config beats the
// repository's, and git hands it on to any child git.
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
// the regular file recorded, byte for byte. A caller checks before any git
// runs in the worktree after a sandboxed run.
func CheckGitLink(dir string, want []byte) error {
	got, err := readGitLink(dir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGitLinkChanged, err)
	}
	if !bytes.Equal(got, want) {
		return ErrGitLinkChanged
	}
	return nil
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
