// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package hostscripts tests scripts/host/remediate.sh against a fake root
// filesystem with fake sysctl, systemctl and sshd.
package hostscripts

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type env struct {
	t     *testing.T
	root  string
	state string
	calls string
	vars  []string
}

const recorder = `#!/usr/bin/env bash
echo "$(basename "$0") $*" >>"$FAKE_CALLS"
[[ -e "$FAKE_FAIL_$(basename "$0")" ]] && exit 1
[[ -n "${FAKE_FAIL_DIR:-}" && -e "$FAKE_FAIL_DIR/$(basename "$0")" ]] && exit 1
exit 0
`

func newEnv(t *testing.T, host string) *env {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	dir := t.TempDir()
	e := &env{t: t, root: filepath.Join(dir, "root"), state: filepath.Join(dir, "state"), calls: filepath.Join(dir, "calls")}
	bin := filepath.Join(dir, "bin")
	for _, d := range []string{bin, e.root + "/etc/sysctl.d", e.root + "/etc/modprobe.d", e.root + "/etc/profile.d",
		e.root + "/etc/ssh/sshd_config.d", e.root + "/etc/default", e.root + "/etc/cron.d", e.root + "/etc/cron.daily",
		e.root + "/etc/cron.hourly", e.root + "/etc/cron.weekly", e.root + "/etc/cron.monthly", e.root + "/etc/sudoers.d",
		e.root + "/etc/audit/rules.d", dir + "/fail"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"sysctl", "systemctl", "sshd", "logger", "visudo", "apt-get", "augenrules"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(recorder), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.write("/etc/crontab", "# crontab\n", 0o644)
	e.write("/etc/group", "root:x:0:\nsudo:x:27:govan,ops\ncrontab:x:107:\n", 0o644)
	e.write("/etc/default/apport", "enabled=1\n", 0o644)
	e.write("/etc/ssh/sshd_config", "Include /etc/ssh/sshd_config.d/*.conf\n", 0o644)
	e.write("/etc/ssh/sshd_config.d/99-unheaded.conf", "MaxAuthTries 3\n", 0o644)
	e.vars = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "ROOT="+e.root, "UNHEADED_REMEDIATE_STATE="+e.state,
		"UNHEADED_HOST="+host, "FAKE_CALLS="+e.calls, "FAKE_FAIL_DIR="+dir+"/fail")
	return e
}

func (e *env) write(p, body string, mode os.FileMode) {
	e.t.Helper()
	if err := os.WriteFile(e.root+p, []byte(body), mode); err != nil {
		e.t.Fatal(err)
	}
	if err := os.Chmod(e.root+p, mode); err != nil { // umask
		e.t.Fatal(err)
	}
}

func (e *env) run(extra []string, args ...string) (string, error) {
	e.t.Helper()
	cmd := exec.Command("bash", append([]string{"remediate.sh"}, args...)...)
	cmd.Env = append(append([]string{}, e.vars...), extra...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *env) apply(fix string) (string, error) {
	return e.run([]string{"UNHEADED_REMEDIATE_ALLOW_NONROOT=1"}, "apply", fix)
}

func (e *env) fail(bin string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(filepath.Dir(e.state), "fail", bin), nil, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// snapshot hashes every file's content and mode under the fake root.
func (e *env) snapshot() string {
	e.t.Helper()
	var lines []string
	err := filepath.WalkDir(e.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		line := strings.TrimPrefix(p, e.root) + " " + fi.Mode().String()
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
		e.t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func (e *env) callLog() string { b, _ := os.ReadFile(e.calls); return string(b) }

func backupDir(t *testing.T, out string) string {
	t.Helper()
	i := strings.Index(out, "undo: ")
	if i < 0 {
		t.Fatalf("no undo line in:\n%s", out)
	}
	f := strings.Fields(out[i:])
	return f[len(f)-1]
}

func TestPlanChangesNothing(t *testing.T) {
	e := newEnv(t, "east")
	before := e.snapshot()
	out, err := e.run(nil, "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, fix := range strings.Fields(out) {
		if o, err := e.run(nil, "plan", fix); err != nil {
			t.Fatalf("plan %s: %v\n%s", fix, err, o)
		}
	}
	if after := e.snapshot(); after != before {
		t.Errorf("plan changed the filesystem:\n--- before\n%s\n--- after\n%s", before, after)
	}
	if c := e.callLog(); c != "" {
		t.Errorf("plan ran commands: %s", c)
	}
}

func TestApplyNeedsRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	e := newEnv(t, "east")
	if out, err := e.run(nil, "apply", "sysctl-cis"); err == nil || !strings.Contains(out, "needs root") {
		t.Errorf("apply as non-root: %v %s", err, out)
	}
}

func TestSysctlHostContentAndUndo(t *testing.T) {
	for _, tc := range []struct {
		host   string
		wantRA bool
	}{{"east", true}, {"west", false}} {
		e := newEnv(t, tc.host)
		before := e.snapshot()
		out, err := e.apply("sysctl-cis")
		if err != nil {
			t.Fatalf("%s: %v\n%s", tc.host, err, out)
		}
		b, err := os.ReadFile(e.root + "/etc/sysctl.d/60-unheaded-cis.conf")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "accept_ra") != tc.wantRA {
			t.Errorf("%s: accept_ra present=%v, want %v (west takes its ISP IPv6 from RAs)", tc.host, !tc.wantRA, tc.wantRA)
		}
		if !strings.Contains(e.callLog(), "sysctl --system") {
			t.Errorf("%s: sysctl not reloaded", tc.host)
		}
		if o, err := e.run(nil, "undo", backupDir(t, out)); err != nil {
			t.Fatalf("undo: %v %s", err, o)
		}
		if after := e.snapshot(); after != before {
			t.Errorf("%s: undo did not restore:\n--- before\n%s\n--- after\n%s", tc.host, before, after)
		}
	}
}

func TestUnknownHostRefused(t *testing.T) {
	e := newEnv(t, "north")
	if out, err := e.apply("sysctl-cis"); err == nil || !strings.Contains(out, "not west or east") {
		t.Errorf("unknown host: %v %s", err, out)
	}
}

func TestSSHDRejectedConfigRestoresAndNeverReloads(t *testing.T) {
	e := newEnv(t, "east")
	before := e.snapshot()
	e.fail("sshd")
	out, err := e.apply("sshd-safe")
	if err == nil || !strings.Contains(out, "rejected") {
		t.Fatalf("want failure, got %v\n%s", err, out)
	}
	if strings.Contains(e.callLog(), "reload") {
		t.Errorf("reloaded after sshd -t failed: %s", e.callLog())
	}
	if after := e.snapshot(); after != before {
		t.Errorf("files not restored:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

func TestSSHDAppliedNeverLoosensWest(t *testing.T) {
	e := newEnv(t, "west")
	out, err := e.apply("sshd-safe")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	b, _ := os.ReadFile(e.root + "/etc/ssh/sshd_config.d/00-unheaded.conf")
	for _, k := range []string{"MaxAuthTries", "ClientAliveCountMax", "X11Forwarding"} {
		if strings.Contains(string(b), k) {
			t.Errorf("west 00-unheaded.conf sets %s, overriding the stricter 99-unheaded.conf", k)
		}
	}
	for _, p := range []string{"/etc/ssh/sshd_config", "/etc/ssh/sshd_config.d/00-unheaded.conf", "/etc/ssh/sshd_config.d/99-unheaded.conf"} {
		if fi, err := os.Stat(e.root + p); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v, want 0600", p, fi.Mode().Perm())
		}
	}
	calls := e.callLog()
	if strings.Index(calls, "sshd -t") < 0 || strings.Index(calls, "sshd -t") > strings.Index(calls, "systemctl reload ssh.service") {
		t.Errorf("reload must follow a passing sshd -t: %s", calls)
	}
}

func TestCronPermsAndUndo(t *testing.T) {
	e := newEnv(t, "east")
	before := e.snapshot()
	out, err := e.apply("cron-perms")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if b, _ := os.ReadFile(e.root + "/etc/cron.allow"); string(b) != "root\ngovan\nops\n" {
		t.Errorf("cron.allow = %q, want root and the sudo group", b)
	}
	for p, want := range map[string]os.FileMode{"/etc/crontab": 0o600, "/etc/cron.d": 0o700, "/etc/cron.allow": 0o640} {
		if fi, err := os.Stat(e.root + p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v %v, want %o", p, err, fi.Mode().Perm(), want)
		}
	}
	if o, err := e.run(nil, "undo", backupDir(t, out)); err != nil {
		t.Fatalf("undo: %v %s", err, o)
	}
	if after := e.snapshot(); after != before {
		t.Errorf("undo did not restore:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

func TestApportOffDisablesAndMasks(t *testing.T) {
	e := newEnv(t, "west")
	if out, err := e.apply("apport-off"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	b, _ := os.ReadFile(e.root + "/etc/default/apport")
	if strings.TrimSpace(string(b)) != "enabled=0" {
		t.Errorf("/etc/default/apport = %q", b)
	}
	calls := e.callLog()
	if !strings.Contains(calls, "systemctl disable --now apport.service") || !strings.Contains(calls, "systemctl mask apport.service") {
		t.Errorf("calls: %s", calls)
	}
}

func TestKmodLeavesUSBStorageUntilDecided(t *testing.T) {
	e := newEnv(t, "east")
	if out, err := e.apply("kmod-cis"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	b, _ := os.ReadFile(e.root + "/etc/modprobe.d/unheaded-cis.conf")
	if strings.Contains(string(b), "usb") || !strings.Contains(string(b), "install cramfs /bin/false") {
		t.Errorf("unheaded-cis.conf:\n%s", b)
	}
}

func TestSudoDropInRejectedNothingInstalled(t *testing.T) {
	e := newEnv(t, "east")
	before := e.snapshot()
	e.fail("visudo")
	if out, err := e.apply("sudo-cis"); err == nil || !strings.Contains(out, "does not parse") {
		t.Fatalf("want refusal, got %v\n%s", err, out)
	}
	if after := e.snapshot(); after != before {
		t.Errorf("a rejected sudoers drop-in changed files:\n%s", after)
	}
}

func TestSudoDropInInstalledAndUndone(t *testing.T) {
	e := newEnv(t, "west")
	before := e.snapshot()
	out, err := e.apply("sudo-cis")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	fi, err := os.Stat(e.root + "/etc/sudoers.d/00-unheaded")
	if err != nil || fi.Mode().Perm() != 0o440 {
		t.Fatalf("drop-in: %v %v", err, fi)
	}
	if !strings.Contains(e.callLog(), "visudo -c") {
		t.Errorf("whole-config visudo -c not run: %s", e.callLog())
	}
	if o, err := e.run(nil, "undo", backupDir(t, out)); err != nil {
		t.Fatalf("undo: %v %s", err, o)
	}
	if after := e.snapshot(); after != before {
		t.Errorf("undo did not restore:\n%s", after)
	}
}

func TestBannerNamesNoOS(t *testing.T) {
	e := newEnv(t, "east")
	e.write("/etc/issue", "Ubuntu 25.10 \\n \\l\n", 0o644)
	if out, err := e.apply("banner-cis"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, f := range []string{"/etc/issue", "/etc/issue.net"} {
		b, _ := os.ReadFile(e.root + f)
		if strings.Contains(strings.ToLower(string(b)), "ubuntu") || !strings.Contains(string(b), "Authorized users only") {
			t.Errorf("%s = %q", f, b)
		}
	}
}

func TestPackageFixesUseApt(t *testing.T) {
	e := newEnv(t, "east")
	for _, f := range []string{"apparmor-utils", "auditd"} {
		if out, err := e.apply(f); err != nil {
			t.Fatalf("%s: %v\n%s", f, err, out)
		}
	}
	calls := e.callLog()
	if !strings.Contains(calls, "apt-get install -y -q apparmor-utils") || !strings.Contains(calls, "apt-get install -y -q auditd") {
		t.Errorf("calls: %s", calls)
	}
}

func TestAuditdHaltConfigRefused(t *testing.T) {
	e := newEnv(t, "east")
	if err := os.MkdirAll(e.root+"/etc/audit", 0o755); err != nil {
		t.Fatal(err)
	}
	e.write("/etc/audit/auditd.conf", "disk_full_action = HALT\n", 0o644)
	if out, err := e.apply("auditd"); err == nil || !strings.Contains(out, "halts on a full disk") {
		t.Errorf("want refusal, got %v\n%s", err, out)
	}
}

func TestSudoRsGetsNoLogfile(t *testing.T) {
	e := newEnv(t, "east")
	bin := filepath.Join(filepath.Dir(e.state), "bin")
	if err := os.WriteFile(filepath.Join(bin, "sudo-rs-version"), []byte("#!/bin/sh\necho sudo-rs 0.2.8 >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := e.run([]string{"UNHEADED_REMEDIATE_ALLOW_NONROOT=1", "SUDO_BIN=" + filepath.Join(bin, "sudo-rs-version")}, "apply", "sudo-cis"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	b, _ := os.ReadFile(e.root + "/etc/sudoers.d/00-unheaded")
	if strings.Contains(string(b), "logfile") || !strings.Contains(string(b), "timestamp_timeout=15") {
		t.Errorf("sudo-rs drop-in:\n%s", b)
	}
}

func TestSSHDEastDisablesAgentForwarding(t *testing.T) {
	for host, want := range map[string]bool{"east": true, "west": false} {
		e := newEnv(t, host)
		if out, err := e.apply("sshd-safe"); err != nil {
			t.Fatalf("%s: %v\n%s", host, err, out)
		}
		b, err := os.ReadFile(e.root + "/etc/ssh/sshd_config.d/00-unheaded.conf")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(b), "\nAllowAgentForwarding no\n"); got != want {
			t.Errorf("%s: AllowAgentForwarding no present=%v, want %v:\n%s", host, got, want, b)
		}
	}
}

const auditRules = "/etc/audit/rules.d/50-unheaded-cis.rules"

func TestAuditdRulesNeedAuditd(t *testing.T) {
	e := newEnv(t, "east")
	if err := os.RemoveAll(e.root + "/etc/audit"); err != nil {
		t.Fatal(err)
	}
	out, err := e.apply("auditd-rules")
	if err == nil || !strings.Contains(out, "auditd fix first") {
		t.Fatalf("want refusal, got %v\n%s", err, out)
	}
	if _, err := os.Stat(e.root + auditRules); !os.IsNotExist(err) {
		t.Error("rules written without auditd")
	}
}

func TestAuditdRulesLoadFailureRestores(t *testing.T) {
	e := newEnv(t, "east")
	before := e.snapshot()
	e.fail("augenrules")
	out, err := e.apply("auditd-rules")
	if err == nil || !strings.Contains(out, "did not load") {
		t.Fatalf("want failure, got %v\n%s", err, out)
	}
	if after := e.snapshot(); after != before {
		t.Errorf("files not restored:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

func TestAuditdRulesAppliedAndUndone(t *testing.T) {
	e := newEnv(t, "east")
	out, err := e.apply("auditd-rules")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	b, err := os.ReadFile(e.root + auditRules)
	if err != nil {
		t.Fatal(err)
	}
	rules := string(b)
	for _, want := range []string{"-w /etc/shadow -p wa -k identity", "-w /etc/sudoers -p wa -k scope", "-k kernel_modules", "-k user_emulation"} {
		if !strings.Contains(rules, want) {
			t.Errorf("rules lack %q", want)
		}
	}
	// Lab hosts: not immutable, so the fix can be undone without a reboot;
	// no per-file delete or chmod rules, which builds would flood.
	for _, bad := range []string{"-e 2", "-k delete", "-k perm_mod"} {
		if strings.Contains(rules, bad) {
			t.Errorf("rules contain %q", bad)
		}
	}
	checkPerm(t, e.root+auditRules, "0640")
	if n := strings.Count(e.callLog(), "augenrules --load"); n != 1 {
		t.Fatalf("augenrules --load ran %d times", n)
	}
	dir := strings.TrimSpace(out[strings.LastIndex(out, "undo: ")+len("undo: "):])
	dir = strings.Fields(dir)[len(strings.Fields(dir))-1]
	if out, err := e.run(nil, "undo", dir); err != nil {
		t.Fatalf("undo: %v\n%s", err, out)
	}
	if _, err := os.Stat(e.root + auditRules); !os.IsNotExist(err) {
		t.Error("undo left the rules file")
	}
	calls := e.callLog()
	if n := strings.Count(calls, "augenrules --load"); n != 2 {
		t.Errorf("undo did not reload the rules (augenrules --load ran %d times)", n)
	}
}
