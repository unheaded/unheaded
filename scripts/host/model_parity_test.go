// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package hostscripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/compliance/remediation"
)

// TestModelMatchesRemediateScript proves that compliance/remediation/
// fixes.yaml (ADR-099) describes what remediate.sh does: for every fix, on
// every host, under sudo and sudo-rs, each file the model renders is what
// the script writes (content and mode), each mode, absent and setting
// action holds afterwards, and each package, service and run action shows
// up as the matching command. Until the native executor replaces the
// script, the script is authoritative and a mismatch is a model bug.
func TestModelMatchesRemediateScript(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "compliance", "remediation", "fixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := remediation.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, fix := range m.Fixes {
		for _, host := range remediation.KnownHosts {
			for _, sudoRS := range []bool{false, true} {
				name := fix.ID + "/" + host
				if sudoRS {
					name += "/sudo-rs"
				}
				t.Run(name, func(t *testing.T) { checkParity(t, fix, host, remediation.Facts{SudoRS: sudoRS}) })
			}
		}
	}
}

func checkParity(t *testing.T, fix remediation.Fix, host string, facts remediation.Facts) {
	e := newEnv(t, host)
	e.write("/etc/cron.deny", "nobody\n", 0o644)
	bin := filepath.Join(filepath.Dir(e.state), "bin")
	version := "Sudo version 1.9.16\n"
	if facts.SudoRS {
		version = "sudo-rs 0.2.8\n"
	}
	// Both print on stderr, as sudo-rs does: the script must read stderr.
	if err := os.WriteFile(filepath.Join(bin, "sudo-version"), []byte("#!/bin/sh\nprintf '"+version+"' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := e.run([]string{"UNHEADED_REMEDIATE_ALLOW_NONROOT=1", "SUDO_BIN=" + filepath.Join(bin, "sudo-version")}, "apply", fix.ID)
	if err != nil {
		t.Fatalf("remediate.sh apply %s: %v\n%s", fix.ID, err, out)
	}
	calls := e.callLog()
	group, err := os.Open(e.root + "/etc/group")
	if err != nil {
		t.Fatal(err)
	}
	defer group.Close()

	for i, a := range fix.Actions {
		switch {
		case a.File != nil:
			want, err := a.File.Render(host, facts, group)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(e.root + a.File.Path)
			if err != nil {
				t.Fatalf("action %d: script did not write %s: %v", i+1, a.File.Path, err)
			}
			if string(got) != want {
				t.Errorf("action %d %s:\nscript wrote:\n%s\nmodel renders:\n%s", i+1, a.File.Path, got, want)
			}
			checkPerm(t, e.root+a.File.Path, a.File.Mode)
		case a.Mode != nil:
			paths := a.Mode.Paths
			if a.Mode.Glob != "" {
				matched, err := filepath.Glob(e.root + a.Mode.Glob)
				if err != nil {
					t.Fatal(err)
				}
				paths = nil
				for _, p := range matched {
					paths = append(paths, strings.TrimPrefix(p, e.root))
				}
			}
			for _, p := range paths {
				checkPerm(t, e.root+p, a.Mode.Mode)
			}
		case a.Absent != "":
			if _, err := os.Stat(e.root + a.Absent); !os.IsNotExist(err) {
				t.Errorf("action %d: %s still present", i+1, a.Absent)
			}
		case a.Setting != nil:
			b, err := os.ReadFile(e.root + a.Setting.Path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains("\n"+string(b), "\n"+a.Setting.Key+"="+a.Setting.Value+"\n") {
				t.Errorf("action %d: %s does not set %s=%s:\n%s", i+1, a.Setting.Path, a.Setting.Key, a.Setting.Value, b)
			}
		case a.Package != nil:
			if !strings.Contains(calls, "apt-get install -y -q "+a.Package.Name) {
				t.Errorf("action %d: no install of %s in %q", i+1, a.Package.Name, calls)
			}
		case a.Service != nil:
			s := a.Service
			if s.Enabled != nil && !*s.Enabled && !strings.Contains(calls, "systemctl disable --now "+s.Name) {
				t.Errorf("action %d: %s not disabled: %q", i+1, s.Name, calls)
			}
			if s.Masked != nil && *s.Masked && !strings.Contains(calls, "systemctl mask "+s.Name) {
				t.Errorf("action %d: %s not masked: %q", i+1, s.Name, calls)
			}
		case a.Run != "":
			if !strings.Contains(calls, a.Run) {
				t.Errorf("action %d: %q not run: %q", i+1, a.Run, calls)
			}
		case a.Validate == "sshd -t" || a.Validate == "visudo -c":
			if !strings.Contains(calls, a.Validate) {
				t.Errorf("action %d: %q not run: %q", i+1, a.Validate, calls)
			}
		}
	}
}

func checkPerm(t *testing.T, path, mode string) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Errorf("%s: %v", path, err)
		return
	}
	if fi.Mode().Perm() != parseOctal(t, mode) {
		t.Errorf("%s mode %04o, model says %s", path, fi.Mode().Perm(), mode)
	}
}

func parseOctal(t *testing.T, s string) os.FileMode {
	t.Helper()
	var v os.FileMode
	for _, c := range s {
		if c < '0' || c > '7' {
			t.Fatalf("mode %q", s)
		}
		v = v*8 + os.FileMode(c-'0')
	}
	return v
}
