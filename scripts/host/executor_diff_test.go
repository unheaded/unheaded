// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package hostscripts

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/compliance/executor"
	"unheaded/pkg/compliance/remediation"
)

// TestExecutorMatchesRemediateScript is COMPLIANCE-WELL step 76: for every
// fix, on both hosts, under sudo and sudo-rs, remediate.sh and the native
// executor are applied to identical fake roots and must leave the same
// files (path, mode, content) and run the same commands in the same order.
// Until it passes, the script stays authoritative; a difference is fixed
// in the model or the executor, never by changing the script to match.
func TestExecutorMatchesRemediateScript(t *testing.T) {
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
				t.Run(name, func(t *testing.T) { diffOne(t, fix, host, sudoRS) })
			}
		}
	}
}

func sudoStub(t *testing.T, e *env, rs bool) string {
	t.Helper()
	v := "Sudo version 1.9.16"
	if rs {
		v = "sudo-rs 0.2.8"
	}
	p := filepath.Join(filepath.Dir(e.state), "bin", "sudo-version")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho '"+v+"' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// calls drops the script's logger line, which records the apply in syslog
// and has no executor counterpart yet (the journal replaces it), and
// normalises the precheck's temp file name, which differs by design.
func calls(log string) string {
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

func diffOne(t *testing.T, fix remediation.Fix, host string, sudoRS bool) {
	script, native := newEnv(t, host), newEnv(t, host)
	for _, e := range []*env{script, native} {
		e.write("/etc/cron.deny", "nobody\n", 0o644)
	}
	out, err := script.run([]string{"UNHEADED_REMEDIATE_ALLOW_NONROOT=1", "SUDO_BIN=" + sudoStub(t, script, sudoRS)}, "apply", fix.ID)
	if err != nil {
		t.Fatalf("remediate.sh: %v\n%s", err, out)
	}

	bin := filepath.Join(filepath.Dir(native.state), "bin")
	j := func(n string) string { return filepath.Join(bin, n) }
	t.Setenv("FAKE_CALLS", native.calls)
	t.Setenv("FAKE_FAIL_DIR", filepath.Join(filepath.Dir(native.state), "fail"))
	x, err := executor.New(context.Background(), executor.Options{
		Root: native.root, State: native.state, Host: host, Operator: "test", AllowNonRoot: true, Out: &bytes.Buffer{},
		Cmd: executor.Commands{Sysctl: j("sysctl"), Systemctl: j("systemctl"), SSHD: j("sshd"), Visudo: j("visudo"),
			Apt: j("apt-get"), Sudo: sudoStub(t, native, sudoRS), Augenrules: j("augenrules")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.Apply(context.Background(), fix); err != nil {
		t.Fatalf("executor: %v", err)
	}

	if a, b := script.snapshot(), native.snapshot(); a != b {
		t.Errorf("files differ\nremediate.sh:\n%s\nexecutor:\n%s", a, b)
	}
	if a, b := calls(script.callLog()), calls(native.callLog()); a != b {
		t.Errorf("commands differ\nremediate.sh:\n%s\nexecutor:\n%s", a, b)
	}
}
