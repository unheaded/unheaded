// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"fmt"
	"strconv"
	"strings"
)

// Host-configuration probes for CIS Ubuntu Linux 24.04 LTS (ADR-097/098).
// Required values are ComplianceAsCode's for ubuntu2404 at the commit the
// catalog pins (9fba127e): the kernel_module_disabled template and the
// file_permissions / file_owner / file_groupowner rules. Like every probe,
// commands are constants built here; catalog data only selects them.

// kernelModules are the CIS 1.1.1.x modules checked: the Level 1 Server set
// plus udf (Level 2). overlay and squashfs (Level 2) are left out on purpose:
// Docker needs overlay and snaps mount squashfs on these hosts.
var kernelModules = []string{"cramfs", "freevxfs", "hfs", "hfsplus", "jffs2", "usb-storage", "udf"}

type fileTarget struct {
	glob      string // a constant path or shell glob
	missingOK bool   // absent (or glob matched nothing) passes
}

type fileRule struct {
	targets []fileTarget
	mode    uint32 // maximum permission bits ("or stricter")
	group   string // required group name; "" = not checked
}

// fileRules: CIS 5.1.1-5.1.3 and 7.1.1-7.1.10. Owner is root (uid 0)
// everywhere.
var fileRules = map[string]fileRule{
	"perm-etc-passwd":            {[]fileTarget{{"/etc/passwd", false}}, 0o644, "root"},
	"perm-etc-passwd-backup":     {[]fileTarget{{"/etc/passwd-", true}}, 0o644, "root"},
	"perm-etc-group":             {[]fileTarget{{"/etc/group", false}}, 0o644, "root"},
	"perm-etc-group-backup":      {[]fileTarget{{"/etc/group-", true}}, 0o644, "root"},
	"perm-etc-shadow":            {[]fileTarget{{"/etc/shadow", false}}, 0o640, "shadow"},
	"perm-etc-shadow-backup":     {[]fileTarget{{"/etc/shadow-", true}}, 0o640, "shadow"},
	"perm-etc-gshadow":           {[]fileTarget{{"/etc/gshadow", false}}, 0o640, "shadow"},
	"perm-etc-gshadow-backup":    {[]fileTarget{{"/etc/gshadow-", true}}, 0o640, "shadow"},
	"perm-etc-shells":            {[]fileTarget{{"/etc/shells", false}}, 0o644, "root"},
	"perm-etc-security-opasswd":  {[]fileTarget{{"/etc/security/opasswd", true}, {"/etc/security/opasswd.old", true}}, 0o600, "root"},
	"perm-sshd-config":           {[]fileTarget{{"/etc/ssh/sshd_config", false}, {"/etc/ssh/sshd_config.d/*", true}}, 0o600, "root"},
	"perm-ssh-host-private-keys": {[]fileTarget{{"/etc/ssh/*_key", false}}, 0o600, ""},
	"perm-ssh-host-public-keys":  {[]fileTarget{{"/etc/ssh/*.pub", false}}, 0o644, ""},
}

// mountChecks: CIS 1.1.2.x. Each path gets a "separate" probe (its own
// mount point) and one probe per required option. Options are read from
// the filesystem that holds the path (findmnt --target): when /var/tmp is
// not its own mount, what protects files there is /var's options, so that
// is what is judged. That is stricter than ComplianceAsCode, whose option
// rules do not apply without a separate mount; we have no "not applicable"
// verdict, and passing an unprotected path would overstate coverage. A bind
// mount of a path onto itself is not a separate filesystem (CIS separates
// these so a full /var/log cannot fill /var) and fails "separate".
var mountChecks = []struct {
	path, slug string
	opts       []string
}{
	{"/tmp", "tmp", []string{"nodev", "nosuid", "noexec"}},
	{"/dev/shm", "dev-shm", []string{"nodev", "nosuid", "noexec"}},
	{"/home", "home", []string{"nodev", "nosuid"}},
	{"/var", "var", []string{"nodev", "nosuid"}},
	{"/var/tmp", "var-tmp", []string{"nodev", "nosuid", "noexec"}},
	{"/var/log", "var-log", []string{"nodev", "nosuid", "noexec"}},
	{"/var/log/audit", "var-log-audit", []string{"nodev", "nosuid", "noexec"}},
}

func init() {
	for _, m := range mountChecks {
		Probes["mount-"+m.slug+"-separate"] = mountProbe(m.path, "")
		for _, o := range m.opts {
			Probes["mount-"+m.slug+"-"+o] = mountProbe(m.path, o)
		}
	}
	for _, m := range kernelModules {
		Probes["kmod-"+m+"-disabled"] = kernelModuleProbe(m)
	}
	for name, r := range fileRules {
		Probes[name] = filePermProbe(r)
	}
}

// kernelModuleProbe: disabled means not loaded, not loadable (modprobe
// resolves it to `install /bin/false` or /bin/true) and blacklisted; a
// module that does not exist for the running kernel passes. A built-in one
// cannot be disabled and fails.
func kernelModuleProbe(mod string) Probe {
	loadedName := strings.ReplaceAll(mod, "-", "_") // /proc/modules spelling
	cmd := fmt.Sprintf(`echo "loaded=$(grep -c '^%s ' /proc/modules)"; `+
		`echo "loadable=$(modprobe -n -v %s 2>&1 | tr '\n' ' ')"; `+
		`echo "blacklist=$(grep -rhE '^[[:space:]]*blacklist[[:space:]]+%s([[:space:]]|$)' /etc/modprobe.d /lib/modprobe.d /usr/lib/modprobe.d 2>/dev/null | head -1)"`,
		loadedName, mod, mod)
	return Probe{
		Baseline: true,
		Command:  cmd,
		Judge: func(out string) (bool, string) {
			f := map[string]string{}
			for _, l := range strings.Split(out, "\n") {
				if k, v, ok := strings.Cut(l, "="); ok {
					f[k] = strings.TrimSpace(v)
				}
			}
			loaded, ok := f["loaded"]
			if !ok {
				return false, mod + ": no output"
			}
			load := f["loadable"]
			switch {
			case loaded != "0":
				return false, mod + " is loaded"
			case strings.Contains(load, "not found"):
				return true, mod + " not available for this kernel"
			case strings.Contains(load, "builtin"):
				return false, mod + " is built into the kernel"
			case !strings.Contains(load, "install /bin/false") && !strings.Contains(load, "install /bin/true"):
				return false, mod + " can be loaded (modprobe: " + load + ")"
			case f["blacklist"] == "":
				return false, mod + " cannot be loaded but is not blacklisted"
			}
			return true, mod + " not loadable and blacklisted"
		},
	}
}

// filePermProbe prints "path mode uid group" per matching file, or
// "pattern ABSENT" when nothing matches.
func filePermProbe(r fileRule) Probe {
	var b strings.Builder
	for _, t := range r.targets {
		fmt.Fprintf(&b, `for f in %s; do if [ -e "$f" ]; then stat -c '%%n %%a %%u %%G' "$f"; else echo "$f ABSENT"; fi; done; `, t.glob)
	}
	missingOK := map[string]bool{}
	for _, t := range r.targets {
		missingOK[t.glob] = t.missingOK
	}
	return Probe{
		Baseline: true,
		Command:  strings.TrimSuffix(b.String(), "; "),
		Judge: func(out string) (bool, string) {
			var bad []string
			n := 0
			for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
				fs := strings.Fields(l)
				if len(fs) == 0 {
					continue
				}
				n++
				if len(fs) == 2 && fs[1] == "ABSENT" {
					if !missingOK[fs[0]] {
						bad = append(bad, fs[0]+" missing")
					}
					continue
				}
				if len(fs) != 4 {
					bad = append(bad, "unreadable: "+l)
					continue
				}
				mode, err := strconv.ParseUint(fs[1], 8, 32)
				if err != nil {
					bad = append(bad, "unreadable mode: "+l)
					continue
				}
				if uint32(mode)&^r.mode != 0 {
					bad = append(bad, fmt.Sprintf("%s mode %s (max %o)", fs[0], fs[1], r.mode))
				}
				if fs[2] != "0" {
					bad = append(bad, fmt.Sprintf("%s owner uid %s (want 0)", fs[0], fs[2]))
				}
				if r.group != "" && fs[3] != r.group {
					bad = append(bad, fmt.Sprintf("%s group %s (want %s)", fs[0], fs[3], r.group))
				}
			}
			if n == 0 {
				return false, "no output"
			}
			if len(bad) > 0 {
				return false, strings.Join(bad, "; ")
			}
			return true, fmt.Sprintf("%d file(s) at mode %o or stricter, owned by root", n, r.mode)
		},
	}
}

// mountProbe judges `findmnt --target` output "TARGET OPTIONS". opt == ""
// asks whether path is its own mount point; otherwise whether the
// filesystem holding path is mounted with opt. A path that does not exist
// fails: there is nothing mounted to judge.
func mountProbe(path, opt string) Probe {
	return Probe{
		Baseline: true,
		Command:  fmt.Sprintf(`findmnt -kn -o TARGET,SOURCE,OPTIONS --target %s 2>/dev/null | head -1 || true; [ -e %s ] || echo ABSENT`, path, path),
		Judge: func(out string) (bool, string) {
			out = strings.TrimSpace(out)
			if out == "" || strings.Contains(out, "ABSENT") {
				return false, path + " does not exist"
			}
			fs := strings.Fields(strings.SplitN(out, "\n", 2)[0])
			if len(fs) != 3 {
				return false, "unreadable: " + out
			}
			target, source, opts := fs[0], fs[1], strings.Split(fs[2], ",")
			if opt == "" {
				if target == path && strings.Contains(source, "[") {
					return false, path + " is a bind mount of " + source + ", not a separate filesystem"
				}
				if target == path {
					return true, path + " is its own mount (" + source + ")"
				}
				return false, path + " is on " + target + ", not its own mount"
			}
			for _, o := range opts {
				if o == opt {
					return true, fmt.Sprintf("%s (on %s) mounted %s", path, target, opt)
				}
			}
			return false, fmt.Sprintf("%s (on %s) mounted without %s: %s", path, target, opt, fs[2])
		},
	}
}
