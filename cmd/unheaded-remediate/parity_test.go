// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"unheaded/pkg/compliance/executor"
	"unheaded/pkg/compliance/remediation"
)

// TestExecutorMatchesRetiredScript is ADR-099 phase 2's parity bar, kept
// after the script it compared against was retired (S-WELL-SPRING Step 270).
// testdata/parity holds what scripts/host/remediate.sh left behind for each
// of its 13 fixes, on both hosts, under sudo and sudo-rs, recorded from the
// script on 2026-10-27 right before it was replaced by a shim: every file
// under a fake root (path, mode, content hash) and the commands it ran, in
// order. The executor must leave exactly the same. A difference is fixed in
// the model or the executor; a golden changes only with a reviewed change to
// a fix's content (and then that fix needs re-applying on the hosts).
func TestExecutorMatchesRetiredScript(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "compliance", "remediation", "fixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := remediation.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	fixes := map[string]remediation.Fix{}
	for _, f := range m.Fixes {
		fixes[f.ID] = f
	}
	for _, id := range scriptFixes {
		for _, host := range remediation.KnownHosts {
			for _, rs := range []bool{false, true} {
				name := id + "_" + host
				if rs {
					name += "_sudo-rs"
				}
				t.Run(name, func(t *testing.T) {
					f, ok := fixes[id]
					if !ok {
						t.Fatalf("fix %s left the model; its parity record stays until a reviewed removal", id)
					}
					want, err := os.ReadFile(filepath.Join("testdata", "parity", name+".golden"))
					if err != nil {
						t.Fatal(err)
					}
					if got := parityRun(t, f, host, rs); got != string(want) {
						t.Errorf("executor differs from the retired script\nscript:\n%s\nexecutor:\n%s", want, got)
					}
				})
			}
		}
	}
}

// scriptFixes are the fixes remediate.sh carried when it was retired.
var scriptFixes = []string{"sysctl-cis", "apport-off", "kmod-cis", "cron-perms", "banner-cis", "apparmor-utils", "auditd",
	"sudo-cis", "shell-tmout", "sshd-safe", "auditd-rules", "journald-caps", "time-p2p"}

// parityFake logs "<name> <args>" like the script harness's fake commands.
const parityFake = "#!/bin/sh\necho \"$(basename \"$0\") $*\" >>\"$FAKE_CALLS\"\nexit 0\n"

// parityRun builds the fake root the script was recorded on (the
// scripts/host test harness, as of its retirement), applies f with the
// executor and renders the same record the golden holds.
func parityRun(t *testing.T, f remediation.Fix, host string, sudoRS bool) string {
	t.Helper()
	dir := t.TempDir()
	root, state, bin, calls := filepath.Join(dir, "root"), filepath.Join(dir, "state"), filepath.Join(dir, "bin"), filepath.Join(dir, "calls")
	for _, d := range []string{bin, root + "/etc/sysctl.d", root + "/etc/modprobe.d", root + "/etc/profile.d",
		root + "/etc/ssh/sshd_config.d", root + "/etc/default", root + "/etc/cron.d", root + "/etc/cron.daily",
		root + "/etc/cron.hourly", root + "/etc/cron.weekly", root + "/etc/cron.monthly", root + "/etc/sudoers.d",
		root + "/etc/audit/rules.d"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(d, 0o755); err != nil { // umask
			t.Fatal(err)
		}
	}
	for _, d := range []string{root, root + "/etc", root + "/etc/ssh", root + "/etc/audit"} {
		if err := os.Chmod(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"sysctl", "systemctl", "sshd", "visudo", "apt-get", "augenrules"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(parityFake), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	version := "Sudo version 1.9.16"
	if sudoRS {
		version = "sudo-rs 0.2.8"
	}
	if err := os.WriteFile(filepath.Join(bin, "sudo-version"), []byte("#!/bin/sh\necho '"+version+"' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{
		"/etc/crontab": "# crontab\n", "/etc/group": "root:x:0:\nsudo:x:27:govan,ops\ncrontab:x:107:\n",
		"/etc/default/apport": "enabled=1\n", "/etc/ssh/sshd_config": "Include /etc/ssh/sshd_config.d/*.conf\n",
		"/etc/ssh/sshd_config.d/99-unheaded.conf": "MaxAuthTries 3\n", "/etc/cron.deny": "nobody\n",
	} {
		if err := os.WriteFile(root+p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root+p, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("FAKE_CALLS", calls)
	j := func(n string) string { return filepath.Join(bin, n) }
	x, err := executor.New(context.Background(), executor.Options{
		Root: root, State: state, Host: host, Operator: "test", AllowNonRoot: true, Out: &bytes.Buffer{},
		Cmd: executor.Commands{Sysctl: j("sysctl"), Systemctl: j("systemctl"), SSHD: j("sshd"), Visudo: j("visudo"),
			Apt: j("apt-get"), Sudo: j("sudo-version"), Augenrules: j("augenrules")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.Apply(context.Background(), f); err != nil {
		t.Fatalf("executor: %v", err)
	}
	log, _ := os.ReadFile(calls) // no file: no commands ran
	return "## files\n" + snapshotTree(t, root) + "\n## calls\n" + normaliseCalls(string(log)) + "\n"
}

// snapshotTree lists every path under root with its mode and the first
// eight bytes of its content's sha256, sorted, as the script harness did.
func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		line := strings.TrimPrefix(p, root) + " " + fi.Mode().String()
		if !d.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			h := sha256.Sum256(b)
			line += " " + hex.EncodeToString(h[:8])
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// normaliseCalls drops the script's logger line (the journal replaced it)
// and the precheck's temp file name, which differs by design.
func normaliseCalls(log string) string {
	var keep []string
	for _, l := range strings.Split(strings.TrimSpace(log), "\n") {
		if strings.HasPrefix(l, "visudo -cf ") {
			l = "visudo -cf <temp copy>"
		}
		if l != "" && !strings.HasPrefix(l, "logger ") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}
