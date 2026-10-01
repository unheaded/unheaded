// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/remediation"
)

// Fake commands log "<name> <args>" to $FAKE_CALLS and fail when a file
// named after them exists in $FAKE_FAIL_DIR.
const recorder = `#!/bin/sh
echo "$(basename "$0") $*" >>"$FAKE_CALLS"
[ -e "$FAKE_FAIL_DIR/$(basename "$0")" ] && exit 1
exit 0
`

type env struct {
	t                        *testing.T
	root, state, calls, fail string
	bin                      string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{t: t, root: filepath.Join(dir, "root"), state: filepath.Join(dir, "state"),
		calls: filepath.Join(dir, "calls"), fail: filepath.Join(dir, "fail"), bin: filepath.Join(dir, "bin")}
	for _, d := range []string{e.bin, e.fail, e.root + "/etc/sysctl.d", e.root + "/etc/modprobe.d", e.root + "/etc/profile.d",
		e.root + "/etc/ssh/sshd_config.d", e.root + "/etc/default", e.root + "/etc/cron.d", e.root + "/etc/cron.daily",
		e.root + "/etc/cron.hourly", e.root + "/etc/cron.weekly", e.root + "/etc/cron.monthly", e.root + "/etc/sudoers.d"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"sysctl", "systemctl", "sshd", "visudo", "apt-get"} {
		if err := os.WriteFile(filepath.Join(e.bin, n), []byte(recorder), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.bin, "sudo"), []byte("#!/bin/sh\necho 'Sudo version 1.9.16' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.write("/etc/crontab", "# crontab\n", 0o644)
	e.write("/etc/group", "root:x:0:\nsudo:x:27:govan,ops\ncrontab:x:107:\n", 0o644)
	e.write("/etc/default/apport", "# apport\nenabled=1\n", 0o644)
	e.write("/etc/ssh/sshd_config", "Include /etc/ssh/sshd_config.d/*.conf\n", 0o644)
	e.write("/etc/ssh/sshd_config.d/99-unheaded.conf", "MaxAuthTries 3\n", 0o644)
	e.write("/etc/cron.deny", "nobody\n", 0o644)
	t.Setenv("FAKE_CALLS", e.calls)
	t.Setenv("FAKE_FAIL_DIR", e.fail)
	return e
}

func (e *env) write(p, body string, mode os.FileMode) {
	e.t.Helper()
	if err := os.WriteFile(e.root+p, []byte(body), mode); err != nil {
		e.t.Fatal(err)
	}
	if err := os.Chmod(e.root+p, mode); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) failing(name string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.fail, name), nil, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) callLog() string { b, _ := os.ReadFile(e.calls); return string(b) }

func (e *env) cmds() Commands {
	j := func(n string) string { return filepath.Join(e.bin, n) }
	return Commands{Sysctl: j("sysctl"), Systemctl: j("systemctl"), SSHD: j("sshd"), Visudo: j("visudo"), Apt: j("apt-get"), Sudo: j("sudo")}
}

func (e *env) executor(host string) *Executor {
	e.t.Helper()
	x, err := New(context.Background(), Options{Root: e.root, State: e.state, Host: host, Operator: "test",
		AllowNonRoot: true, Cmd: e.cmds(), Out: &bytes.Buffer{}, Now: time.Now})
	if err != nil {
		e.t.Fatal(err)
	}
	return x
}

// snapshot lists every path under root with its mode and content hash.
func (e *env) snapshot() string {
	e.t.Helper()
	var lines []string
	err := filepath.WalkDir(e.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		line := strings.TrimPrefix(p, e.root) + " " + fi.Mode().String()
		if fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			s := sha256.Sum256(b)
			line += " " + hex.EncodeToString(s[:8])
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		e.t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func model(t *testing.T) *remediation.Model {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "compliance", "remediation", "fixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := remediation.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func fix(t *testing.T, id string) remediation.Fix {
	t.Helper()
	for _, f := range model(t).Fixes {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no fix %s", id)
	return remediation.Fix{}
}

func TestPlanChangesNothing(t *testing.T) {
	for _, f := range model(t).Fixes {
		e := newEnv(t)
		before := e.snapshot()
		if err := e.executor("east").Plan(context.Background(), f); err != nil {
			t.Fatalf("%s: %v", f.ID, err)
		}
		if after := e.snapshot(); after != before {
			t.Errorf("%s: plan changed files", f.ID)
		}
		if e.callLog() != "" {
			t.Errorf("%s: plan ran commands: %s", f.ID, e.callLog())
		}
		if _, err := os.Stat(e.state); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: plan created the state directory", f.ID)
		}
	}
}

// Every fix, on both hosts: apply, then undo restores the root exactly.
func TestApplyThenUndoRestoresEverything(t *testing.T) {
	for _, f := range model(t).Fixes {
		for _, host := range remediation.KnownHosts {
			t.Run(f.ID+"/"+host, func(t *testing.T) {
				e := newEnv(t)
				before := e.snapshot()
				dir, err := e.executor(host).Apply(context.Background(), f)
				if err != nil {
					t.Fatal(err)
				}
				if e.snapshot() == before && f.ID != "apparmor-utils" && f.ID != "auditd" {
					t.Error("apply changed no file")
				}
				if err := Undo(context.Background(), dir, nil, true); err != nil {
					t.Fatal(err)
				}
				if after := e.snapshot(); after != before {
					t.Errorf("undo did not restore:\nbefore\n%s\nafter\n%s", before, after)
				}
			})
		}
	}
}

func TestSSHDRejectedConfigRestoresAndNeverReloads(t *testing.T) {
	e := newEnv(t)
	e.failing("sshd")
	before := e.snapshot()
	_, err := e.executor("east").Apply(context.Background(), fix(t, "sshd-safe"))
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v, want refusal", err)
	}
	if after := e.snapshot(); after != before {
		t.Errorf("rejected sshd config left changes:\n%s", after)
	}
	if strings.Contains(e.callLog(), "reload") {
		t.Errorf("reloaded sshd after a failed check: %s", e.callLog())
	}
}

func TestSSHDNeverLoosensWest(t *testing.T) {
	e := newEnv(t)
	if _, err := e.executor("west").Apply(context.Background(), fix(t, "sshd-safe")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(e.root + "/etc/ssh/sshd_config.d/00-unheaded.conf")
	if strings.Contains(string(b), "MaxAuthTries") || strings.Contains(string(b), "X11Forwarding") {
		t.Errorf("west 00-unheaded.conf repeats what 99-unheaded.conf sets more strictly:\n%s", b)
	}
}

func TestSudoersThatDoNotParseAreNeverInstalled(t *testing.T) {
	e := newEnv(t)
	e.failing("visudo")
	before := e.snapshot()
	if _, err := e.executor("west").Apply(context.Background(), fix(t, "sudo-cis")); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v", err)
	}
	if after := e.snapshot(); after != before {
		t.Error("a sudoers drop-in that failed visudo -cf changed files")
	}
}

func TestSudoRSGetsNoLogfile(t *testing.T) {
	e := newEnv(t)
	if err := os.WriteFile(filepath.Join(e.bin, "sudo"), []byte("#!/bin/sh\necho 'sudo-rs 0.2.8' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	x := e.executor("east")
	if !x.Facts().SudoRS {
		t.Fatal("sudo-rs not detected from its stderr")
	}
	if _, err := x.Apply(context.Background(), fix(t, "sudo-cis")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(e.root + "/etc/sudoers.d/00-unheaded")
	if strings.Contains(string(b), "logfile") || !strings.Contains(string(b), "timestamp_timeout=15") {
		t.Errorf("sudo-rs drop-in:\n%s", b)
	}
	if fi, _ := os.Stat(e.root + "/etc/sudoers.d/00-unheaded"); fi.Mode().Perm() != 0o440 {
		t.Errorf("mode %o", fi.Mode().Perm())
	}
}

// Stricter than remediate.sh: a halting auditd is refused and the install
// undone, not left running with instructions.
func TestAuditdHaltingConfigRefusedAndUndone(t *testing.T) {
	e := newEnv(t)
	if err := os.MkdirAll(e.root+"/etc/audit", 0o755); err != nil {
		t.Fatal(err)
	}
	e.write("/etc/audit/auditd.conf", "disk_full_action = HALT\n", 0o640)
	_, err := e.executor("east").Apply(context.Background(), fix(t, "auditd"))
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v", err)
	}
	log := e.callLog()
	if !strings.Contains(log, "apt-get install -y -q auditd") || !strings.Contains(log, "apt-get remove -y auditd") {
		t.Errorf("want install then removal on refusal: %s", log)
	}
}

func TestSymlinkTargetsAreRefused(t *testing.T) {
	e := newEnv(t)
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(outside, []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, e.root+"/etc/issue"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.executor("east").Apply(context.Background(), fix(t, "banner-cis")); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v, want refusal", err)
	}
	if b, _ := os.ReadFile(outside); string(b) != "precious\n" {
		t.Errorf("wrote through a symlink: %q", b)
	}
}

func TestApportSettingAndService(t *testing.T) {
	e := newEnv(t)
	if _, err := e.executor("west").Apply(context.Background(), fix(t, "apport-off")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(e.root + "/etc/default/apport")
	if string(b) != "# apport\nenabled=0\n" {
		t.Errorf("apport defaults: %q", b)
	}
	log := e.callLog()
	if !strings.Contains(log, "systemctl disable --now apport.service") || !strings.Contains(log, "systemctl mask apport.service") {
		t.Errorf("calls: %s", log)
	}
}

func TestCronAllowFromTheGroupFile(t *testing.T) {
	e := newEnv(t)
	if _, err := e.executor("east").Apply(context.Background(), fix(t, "cron-perms")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(e.root + "/etc/cron.allow")
	if string(b) != "root\ngovan\nops\n" {
		t.Errorf("cron.allow %q", b)
	}
	if _, err := os.Stat(e.root + "/etc/cron.deny"); !errors.Is(err, fs.ErrNotExist) {
		t.Error("cron.deny still present")
	}
	for p, want := range map[string]os.FileMode{"/etc/crontab": 0o600, "/etc/cron.d": 0o700, "/etc/cron.allow": 0o640} {
		if fi, err := os.Stat(e.root + p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v %v, want %o", p, err, fi, want)
		}
	}
}

func TestUnknownHostRefused(t *testing.T) {
	e := newEnv(t)
	if _, err := New(context.Background(), Options{Root: e.root, Host: "north", Cmd: e.cmds()}); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyNeedsRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	e := newEnv(t)
	x, err := New(context.Background(), Options{Root: e.root, State: e.state, Host: "east", Cmd: e.cmds()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.Apply(context.Background(), fix(t, "kmod-cis")); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v", err)
	}
}
