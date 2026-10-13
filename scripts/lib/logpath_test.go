// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package scriptlib tests the shell helpers in scripts/lib.
package scriptlib

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// logpath runs logpath.sh with a clean environment: HOME, XDG_STATE_HOME and
// UNHEADED_LOG_DIR are only what the test sets, so the host's own values
// cannot make a case pass.
func logpath(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	cmd := exec.Command("bash", append([]string{"logpath.sh"}, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestLogPathResolution(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")

	cases := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"override", map[string]string{"HOME": home, "UNHEADED_LOG_DIR": filepath.Join(dir, "o")},
			[]string{"wotan"}, filepath.Join(dir, "o", "wotan", "wotan.log")},
		{"named file", map[string]string{"HOME": home, "UNHEADED_LOG_DIR": filepath.Join(dir, "o")},
			[]string{"doom", "doom-runner.log"}, filepath.Join(dir, "o", "doom", "doom-runner.log")},
		{"xdg", map[string]string{"HOME": home, "XDG_STATE_HOME": filepath.Join(dir, "x")},
			[]string{"zhen"}, filepath.Join(dir, "x", "unheaded", "log", "zhen", "zhen.log")},
	}
	if os.Geteuid() != 0 {
		// Without root, /var/log/unheaded is not writable on any sane host, so
		// the operator fallback under $HOME applies.
		cases = append(cases, struct {
			name string
			env  map[string]string
			args []string
			want string
		}{"home fallback", map[string]string{"HOME": home}, []string{"vor"},
			filepath.Join(home, ".local", "state", "unheaded", "log", "vor", "vor.log")})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := logpath(t, c.env, c.args...)
			if err != nil {
				t.Fatalf("logpath %v: %v: %s", c.args, err, got)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			fi, err := os.Stat(filepath.Dir(got))
			if err != nil {
				t.Fatalf("directory not created: %v", err)
			}
			if perm := fi.Mode().Perm(); perm&0o027 != 0 {
				t.Fatalf("directory mode %o is group-writable or world-accessible", perm)
			}
		})
	}
}

func TestLogPathRefuses(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("/etc", filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "svc"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "svc", "svc.log")); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": dir, "UNHEADED_LOG_DIR": dir}

	cases := []struct {
		name string
		env  map[string]string
		args []string
	}{
		{"no args", env, nil},
		{"traversal service", env, []string{"../etc"}},
		{"slash in service", env, []string{"a/b"}},
		{"uppercase service", env, []string{"Wotan"}},
		{"traversal file", env, []string{"ok", "../x.log"}},
		{"not a .log", env, []string{"ok", "x.txt"}},
		{"relative base", map[string]string{"HOME": dir, "UNHEADED_LOG_DIR": "rel"}, []string{"ok"}},
		{"symlinked dir", env, []string{"linkdir"}},
		{"symlinked file", env, []string{"svc"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := logpath(t, c.env, c.args...)
			if err == nil {
				t.Fatalf("logpath %v accepted, printed %q", c.args, got)
			}
		})
	}
}
