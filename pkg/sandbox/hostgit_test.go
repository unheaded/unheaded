// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoWithPrograms is a repository whose own config names programs, as a
// .git written from inside a sandbox could. marker appears if one runs.
func repoWithPrograms(t *testing.T) (dir, marker string) {
	t.Helper()
	base := t.TempDir()
	dir, marker = filepath.Join(base, "wt"), filepath.Join(base, "ran")
	hooks := filepath.Join(base, "hooks")
	if err := os.MkdirAll(hooks, 0o700); err != nil {
		t.Fatal(err)
	}
	prog := filepath.Join(base, "prog.sh")
	if err := os.WriteFile(prog, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o700); err != nil { // #nosec G306 -- test program
		t.Fatal(err)
	}
	for _, h := range []string{"post-commit", "pre-commit", "post-checkout"} {
		if err := os.Symlink(prog, filepath.Join(hooks, h)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "zhen/x"}, {"config", "user.email", "t@example.invalid"}, {"config", "user.name", "t"},
		{"commit", "-q", "--allow-empty", "--no-gpg-sign", "--no-verify", "-m", "base"},
		{"config", "core.fsmonitor", prog}, {"config", "core.hooksPath", hooks},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, marker
}

// The fixture is what it claims: plain git runs the program it names.
func TestRepoWithPrograms_PlainGitRunsThem(t *testing.T) {
	dir, marker := repoWithPrograms(t)
	if out, err := exec.Command("git", "-C", dir, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Skipf("this git does not run core.fsmonitor: %v", err)
	}
}

func TestHostGit_RunsNoProgramTheRepositoryNames(t *testing.T) {
	dir, marker := repoWithPrograms(t)
	ctx := context.Background()
	for _, args := range [][]string{
		{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		{"ls-files", "-z"},
		{"symbolic-ref", "--quiet", "--short", "HEAD"},
		{"add", "-A", "--", "f"},
		{"commit", "-q", "--no-gpg-sign", "-m", "x"},
		{"rev-parse", "HEAD"},
	} {
		if out, err := HostGit(ctx, dir, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("git %v ran a program from the repository's config", args)
		}
	}
}

func TestHostGit_EnvironmentIsAnAllowlist(t *testing.T) {
	t.Setenv("WELL_APP_ZHEN_DSN", "postgres://x")
	t.Setenv("ANTHROPIC_API_KEY", "k")
	t.Setenv("GIT_DIR", "/elsewhere")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.fsmonitor=/x'")
	t.Setenv("GIT_CONFIG_GLOBAL", "/elsewhere")
	t.Setenv("SSH_AUTH_SOCK", "/x")
	t.Setenv("HOME", "/home/t")
	cmd := HostGit(context.Background(), "/repo", "status")
	seen := map[string]string{}
	for _, kv := range cmd.Env {
		k, v, _ := strings.Cut(kv, "=")
		seen[k] = v
	}
	for _, k := range []string{"WELL_APP_ZHEN_DSN", "ANTHROPIC_API_KEY", "GIT_DIR", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_GLOBAL", "SSH_AUTH_SOCK"} {
		if _, ok := seen[k]; ok {
			t.Errorf("%s reached git", k)
		}
	}
	if seen["GIT_CONFIG_NOSYSTEM"] != "1" || seen["HOME"] != "/home/t" || seen["PATH"] == "" {
		t.Errorf("env = %v", cmd.Env)
	}
}

func TestHostGit_CommandLineConfigComesBeforeTheSubcommand(t *testing.T) {
	cmd := HostGit(context.Background(), "/repo", "status", "--short")
	got := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-c core.fsmonitor=false", "-c core.hooksPath=/dev/null", "-C /repo"} {
		if i := strings.Index(got, want); i < 0 || i > strings.Index(got, " status") {
			t.Errorf("%q not before the subcommand in %q", want, got)
		}
	}
}

func TestGitLink_RefusesAnythingButWhatWasRecorded(t *testing.T) {
	wt := t.TempDir()
	link := filepath.Join(wt, ".git")
	if err := os.WriteFile(link, []byte("gitdir: /repo/.git/worktrees/a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want, err := RecordGitLink(wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckGitLink(wt, want); err != nil {
		t.Fatalf("unchanged: %v", err)
	}
	reset := func() {
		if err := os.RemoveAll(link); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]func() error{
		"rewritten": func() error { return os.WriteFile(link, []byte("gitdir: /elsewhere\n"), 0o600) },
		"directory": func() error { return os.Mkdir(link, 0o700) },
		"symlink":   func() error { return os.Symlink("/elsewhere", link) },
		"removed":   func() error { return nil },
		"huge":      func() error { return os.WriteFile(link, append(want, make([]byte, 1<<20)...), 0o600) },
	}
	for name, tamper := range cases {
		reset()
		if err := tamper(); err != nil {
			t.Fatal(err)
		}
		if err := CheckGitLink(wt, want); !errors.Is(err, ErrGitLinkChanged) {
			t.Errorf("%s: %v", name, err)
		}
	}
	reset()
	if err := os.Mkdir(link, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordGitLink(wt); err == nil {
		t.Error("recorded a .git that is not a file")
	}
}
