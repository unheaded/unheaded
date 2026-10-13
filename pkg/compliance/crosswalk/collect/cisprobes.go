// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package collect

import (
	"encoding/json"
	"fmt"
	"sort"
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

// serviceChecks: CIS 2.1.1-2.1.20, "services not in use". Package and unit
// names are ComplianceAsCode's for ubuntu2404. Not in use means no package
// installed, or (installed as a dependency) every unit neither enabled nor
// active: disabled or masked and stopped.
var serviceChecks = []struct {
	slug     string
	packages []string
	units    []string
}{
	{"autofs", []string{"autofs"}, []string{"autofs.service"}},
	{"avahi", []string{"avahi-daemon"}, []string{"avahi-daemon.service", "avahi-daemon.socket"}},
	{"dhcp-server", []string{"isc-dhcp-server"}, []string{"isc-dhcp-server.service", "isc-dhcp-server6.service"}},
	{"dns-server", []string{"bind9"}, []string{"named.service"}},
	{"dnsmasq", []string{"dnsmasq"}, []string{"dnsmasq.service"}},
	{"ftp-server", []string{"vsftpd"}, []string{"vsftpd.service"}},
	{"ldap-server", []string{"slapd"}, []string{"slapd.service"}},
	{"mail-access", []string{"dovecot-core"}, []string{"dovecot.service"}},
	{"nfs-server", []string{"nfs-kernel-server"}, []string{"nfs-server.service"}},
	{"nis-server", []string{"ypserv"}, []string{"ypserv.service"}},
	{"print-server", []string{"cups"}, []string{"cups.service", "cups.socket"}},
	{"rpcbind", []string{"rpcbind"}, []string{"rpcbind.service", "rpcbind.socket"}},
	{"rsync", []string{"rsync"}, []string{"rsync.service"}},
	{"samba", []string{"samba"}, []string{"smbd.service"}},
	{"snmp", []string{"snmpd"}, []string{"snmpd.service"}},
	{"tftp-server", []string{"tftpd-hpa"}, []string{"tftpd-hpa.service"}},
	{"web-proxy", []string{"squid"}, []string{"squid.service"}},
	{"web-server", []string{"apache2", "nginx"}, []string{"apache2.service", "nginx.service"}},
	{"xinetd", []string{"xinetd"}, []string{"xinetd.service"}},
	{"x-server", []string{"xserver-common"}, nil}, // 2.1.20, Level 2: package only
}

// clientChecks: CIS 2.2.1-2.2.6, clients that must not be installed.
var clientChecks = []struct {
	slug     string
	packages []string
}{
	{"nis-client", []string{"nis"}},
	{"rsh-client", []string{"rsh-client"}},
	{"talk-client", []string{"talk"}},
	{"telnet-client", []string{"inetutils-telnet", "telnet"}},
	{"ldap-client", []string{"ldap-utils"}},
	{"ftp-client", []string{"ftp", "tnftp"}},
}

func init() {
	for _, c := range serviceChecks {
		Probes["svc-"+c.slug+"-not-in-use"] = serviceProbe(c.packages, c.units)
	}
	for _, c := range clientChecks {
		Probes["pkg-"+c.slug+"-absent"] = packageAbsentProbe(c.packages)
	}
	// CIS 2.4.1.2-2.4.1.7: ComplianceAsCode file rules for ubuntu2404.
	fileRulesCron := map[string]fileRule{
		"perm-etc-crontab":      {[]fileTarget{{"/etc/crontab", false}}, 0o600, "root"},
		"perm-etc-cron-hourly":  {[]fileTarget{{"/etc/cron.hourly", false}}, 0o700, "root"},
		"perm-etc-cron-daily":   {[]fileTarget{{"/etc/cron.daily", false}}, 0o700, "root"},
		"perm-etc-cron-weekly":  {[]fileTarget{{"/etc/cron.weekly", false}}, 0o700, "root"},
		"perm-etc-cron-monthly": {[]fileTarget{{"/etc/cron.monthly", false}}, 0o700, "root"},
		"perm-etc-cron-d":       {[]fileTarget{{"/etc/cron.d", false}}, 0o700, "root"},
	}
	for name, r := range fileRulesCron {
		Probes[name] = filePermProbe(r)
	}
	Probes["cron-active"] = cronActiveProbe()
	Probes["cron-allow-restricted"] = allowDenyProbe("cron.allow", "", "crontab", false)
	Probes["at-restricted"] = allowDenyProbe("at.allow", "at", "root", true)
	Probes["mta-local-only"] = mtaLocalOnlyProbe()
}

func dpkgInstalled(pkgs []string) string {
	return fmt.Sprintf(`echo "installed=$(dpkg-query -W -f='${Package} ${Status}\n' %s 2>/dev/null | awk '/ install ok installed$/{print $1}' | tr '\n' ' ')"`,
		strings.Join(pkgs, " "))
}

func parseKV(out string) map[string]string {
	f := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			f[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return f
}

func serviceProbe(pkgs, units []string) Probe {
	var b strings.Builder
	b.WriteString(dpkgInstalled(pkgs))
	for _, u := range units {
		fmt.Fprintf(&b, `; echo "%s=$(systemctl is-enabled %s 2>/dev/null || true)/$(systemctl is-active %s 2>/dev/null || true)"`, u, u, u)
	}
	name := strings.Join(pkgs, "/")
	return Probe{
		Baseline: true,
		Command:  b.String(),
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			inst, ok := f["installed"]
			if !ok {
				return false, name + ": no output"
			}
			if inst == "" {
				return true, name + " not installed"
			}
			var bad []string
			for _, u := range units {
				en, act, _ := strings.Cut(f[u], "/")
				if act == "active" || act == "activating" || en == "enabled" || en == "enabled-runtime" || en == "static" || en == "indirect" || en == "alias" {
					bad = append(bad, fmt.Sprintf("%s %s/%s", u, en, act))
				}
			}
			if len(units) == 0 {
				return false, "installed: " + inst
			}
			if len(bad) > 0 {
				return false, "installed: " + inst + "; in use: " + strings.Join(bad, ", ")
			}
			return true, "installed (" + inst + ") but every unit disabled or masked and stopped"
		},
	}
}

func packageAbsentProbe(pkgs []string) Probe {
	name := strings.Join(pkgs, "/")
	return Probe{
		Baseline: true,
		Command:  dpkgInstalled(pkgs),
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			inst, ok := f["installed"]
			if !ok {
				return false, name + ": no output"
			}
			if inst != "" {
				return false, "installed: " + inst
			}
			return true, name + " not installed"
		},
	}
}

// cronActiveProbe: CIS 2.4.1.1, cron installed, enabled and running.
func cronActiveProbe() Probe {
	return Probe{
		Baseline: true,
		Command: dpkgInstalled([]string{"cron"}) +
			`; echo "enabled=$(systemctl is-enabled cron.service 2>/dev/null || true)"; echo "active=$(systemctl is-active cron.service 2>/dev/null || true)"`,
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			if !strings.Contains(" "+f["installed"]+" ", " cron ") {
				return false, "cron not installed"
			}
			if f["enabled"] != "enabled" || f["active"] != "active" {
				return false, fmt.Sprintf("cron %s/%s (want enabled/active)", f["enabled"], f["active"])
			}
			return true, "cron enabled and active"
		},
	}
}

// allowDenyProbe: CIS 2.4.1.8 (cron: /etc/cron.allow root:crontab 0640 or
// stricter, no /etc/cron.deny) and 2.4.2.1 (at: when installed, at.allow
// root:root 0640 or stricter; at.deny, if present, the same). Values from
// ComplianceAsCode's ubuntu2404 rules. pkg != "" means "only if installed".
func allowDenyProbe(allow, pkg, group string, denyAllowed bool) Probe {
	deny := strings.TrimSuffix(allow, ".allow") + ".deny"
	stat := func(key, file string) string {
		return fmt.Sprintf(`echo "%s=$(if [ -e /etc/%s ]; then stat -c '%%n %%a %%u %%G' /etc/%s; else echo ABSENT; fi)"`, key, file, file)
	}
	cmd := stat("allow", allow) + "; " + stat("deny", deny)
	if pkg != "" {
		cmd = dpkgInstalled([]string{pkg}) + "; " + cmd
	}
	check := func(v string) (bool, string) {
		fs := strings.Fields(v)
		if len(fs) != 4 {
			return false, "unreadable: " + v
		}
		mode, err := strconv.ParseUint(fs[1], 8, 32)
		switch {
		case err != nil:
			return false, "unreadable mode: " + v
		case uint32(mode)&^0o640 != 0:
			return false, fmt.Sprintf("%s mode %s (max 640)", fs[0], fs[1])
		case fs[2] != "0":
			return false, fmt.Sprintf("%s owner uid %s (want 0)", fs[0], fs[2])
		case fs[3] != group:
			return false, fmt.Sprintf("%s group %s (want %s)", fs[0], fs[3], group)
		}
		return true, ""
	}
	return Probe{
		Baseline: true,
		Command:  cmd,
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			if pkg != "" {
				inst, ok := f["installed"]
				if !ok {
					return false, pkg + ": no output"
				}
				if inst == "" {
					return true, pkg + " not installed"
				}
			}
			a, ok := f["allow"]
			if !ok {
				return false, "no output"
			}
			if a == "ABSENT" {
				return false, "/etc/" + allow + " missing: every user may schedule jobs"
			}
			if ok, why := check(a); !ok {
				return false, why
			}
			if d := f["deny"]; d != "" && d != "ABSENT" {
				if !denyAllowed {
					return false, "/etc/" + deny + " exists (remove it; " + allow + " decides)"
				}
				if ok, why := check(d); !ok {
					return false, why
				}
			}
			return true, "/etc/" + allow + " restricts scheduling"
		},
	}
}

// mtaLocalOnlyProbe: CIS 2.1.21, no mail transfer agent listening on a
// non-loopback address (ports 25, 465, 587).
func mtaLocalOnlyProbe() Probe {
	return Probe{
		Baseline: true,
		Command:  `ss -ltnH 2>/dev/null | awk '{print $4}' | grep -E ':(25|465|587)$' || true`,
		Judge: func(out string) (bool, string) {
			var exposed []string
			for _, a := range strings.Fields(out) {
				if !strings.HasPrefix(a, "127.") && !strings.HasPrefix(a, "[::1]") {
					exposed = append(exposed, a)
				}
			}
			if len(exposed) > 0 {
				return false, "MTA listening on " + strings.Join(exposed, ", ")
			}
			return true, "no MTA listening beyond loopback"
		},
	}
}

// Account integrity (CIS 7.2.1-7.2.8), boot loader (1.4.1, 1.4.2), AppArmor
// (1.3.1.1, 1.3.1.3), prelink and apport (1.5.4, 1.5.5), sudo (5.2.1-5.2.3,
// 5.2.5, 5.2.6). Commands that list offenders end with an "@ok" line: no
// marker means the command did not complete (sudo refused, file missing),
// which is never read as "no offenders". Offender lists hold names or IDs
// only, never password hashes.
func init() {
	offenders := map[string]string{ // #nosec G101 -- shell commands that read /etc/passwd and /etc/shadow; they hold and print no credentials
		"acct-passwd-shadowed":    `awk -F: '($2 != "x") {print $1}' /etc/passwd && echo @ok`,
		"acct-shadow-no-empty":    `sudo -n awk -F: '($2 == "") {print $1}' /etc/shadow && echo @ok`,
		"acct-groups-exist":       `for g in $(cut -d: -f4 /etc/passwd | sort -u); do grep -q "^[^:]*:[^:]*:$g:" /etc/group || echo "gid $g"; done; echo @ok`,
		"acct-shadow-group-empty": `sg=$(awk -F: '($1=="shadow"){print $3}' /etc/group); awk -F: '($1=="shadow" && $4!=""){print "members " $4}' /etc/group && awk -F: -v g="$sg" '(g!="" && $4==g){print "primary " $1}' /etc/passwd && echo @ok`,
		"acct-unique-uid":         `cut -d: -f3 /etc/passwd | sort | uniq -d && echo @ok`,
		"acct-unique-gid":         `cut -d: -f3 /etc/group | sort | uniq -d && echo @ok`,
		"acct-unique-user-name":   `cut -d: -f1 /etc/passwd | sort | uniq -d && echo @ok`,
		"acct-unique-group-name":  `cut -d: -f1 /etc/group | sort | uniq -d && echo @ok`,
	}
	for name, cmd := range offenders {
		Probes[name] = offenderProbe(cmd)
	}
	Probes["apparmor-installed"] = packageInstalledProbe([]string{"apparmor", "apparmor-utils"})
	Probes["apparmor-profiles-enforced"] = apparmorProfilesProbe()
	Probes["grub-password"] = markerLinesProbe(`sudo -n sh -c "grep -E '^[[:space:]]*(set superusers|password_pbkdf2)' /boot/grub/grub.cfg; echo @ok"`,
		func(lines []string) (bool, string) {
			var su, pw bool
			for _, l := range lines {
				su = su || strings.Contains(l, "set superusers")
				pw = pw || strings.HasPrefix(strings.TrimSpace(l), "password_pbkdf2")
			}
			if su && pw {
				return true, "grub superuser with a PBKDF2 password"
			}
			return false, fmt.Sprintf("grub superusers set: %v, password_pbkdf2: %v", su, pw)
		})
	Probes["perm-boot-grub-cfg"] = filePermProbe(fileRule{[]fileTarget{{"/boot/grub/grub.cfg", false}}, 0o600, ""})
	Probes["pkg-prelink-absent"] = packageAbsentProbe([]string{"prelink"})
	Probes["svc-apport-not-in-use"] = serviceProbe([]string{"apport"}, []string{"apport.service"})
	Probes["sudo-installed"] = packageInstalledProbe([]string{"sudo"})

	// Only Defaults lines and !authenticate uses are printed, comments dropped.
	// "@ok" comes from inside the sudo'd shell: a refused sudo prints nothing.
	sudoers := `sudo -n sh -c 'cat /etc/sudoers /etc/sudoers.d/* 2>/dev/null; echo @ok' | grep -Ev '^[[:space:]]*#' | grep -E 'Defaults|authenticate|^@ok$'`
	Probes["sudo-use-pty"] = markerLinesProbe(sudoers, func(lines []string) (bool, string) {
		for _, l := range lines {
			if defaultsSet(l, "use_pty") {
				return true, "Defaults use_pty"
			}
		}
		return false, "use_pty not set in sudoers"
	})
	Probes["sudo-logfile"] = markerLinesProbe(sudoers, func(lines []string) (bool, string) {
		for _, l := range lines {
			if strings.Contains(l, "Defaults") && strings.Contains(l, "logfile=") {
				return true, "sudo logfile configured"
			}
		}
		return false, "no Defaults logfile= in sudoers"
	})
	Probes["sudo-no-noauth"] = markerLinesProbe(sudoers, func(lines []string) (bool, string) {
		for _, l := range lines {
			if strings.Contains(l, "!authenticate") {
				return false, "!authenticate present: " + strings.TrimSpace(l)
			}
		}
		return true, "no !authenticate in sudoers"
	})
	Probes["sudo-timeout"] = markerLinesProbe(sudoers, func(lines []string) (bool, string) {
		for _, l := range lines {
			i := strings.Index(l, "timestamp_timeout=")
			if i < 0 || !strings.Contains(l, "Defaults") {
				continue
			}
			v := strings.TrimRight(strings.Fields(l[i+len("timestamp_timeout="):] + " ")[0], ",")
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 15 {
				return false, "timestamp_timeout=" + v + " (want 0-15)"
			}
			return true, "timestamp_timeout=" + v
		}
		return false, "timestamp_timeout not set explicitly (ComplianceAsCode requires it, max 15)"
	})
}

// markerLinesProbe runs cmd, requires its "@ok" completion line, and judges
// the lines before it.
func markerLinesProbe(cmd string, judge func(lines []string) (bool, string)) Probe {
	return Probe{
		Baseline: true,
		Command:  cmd,
		Judge: func(out string) (bool, string) {
			var lines []string
			done := false
			for _, l := range strings.Split(out, "\n") {
				if strings.TrimSpace(l) == "@ok" {
					done = true
					continue
				}
				if strings.TrimSpace(l) != "" {
					lines = append(lines, l)
				}
			}
			if !done {
				return false, "check did not complete"
			}
			return judge(lines)
		},
	}
}

func offenderProbe(cmd string) Probe {
	return markerLinesProbe(cmd, func(lines []string) (bool, string) {
		if len(lines) > 0 {
			return false, "offenders: " + strings.Join(lines, ", ")
		}
		return true, "none"
	})
}

// defaultsSet reports whether a sudoers Defaults line turns flag on.
func defaultsSet(line, flag string) bool {
	l := strings.TrimSpace(line)
	if !strings.HasPrefix(l, "Defaults") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(l, "Defaults"))
	if strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, "@") || strings.HasPrefix(rest, ">") || strings.HasPrefix(rest, "!") {
		if f := strings.Fields(rest); len(f) > 1 {
			rest = strings.Join(f[1:], " ")
		}
	}
	for _, opt := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if opt == flag {
			return true
		}
	}
	return false
}

func packageInstalledProbe(pkgs []string) Probe {
	return Probe{
		Baseline: true,
		Command:  dpkgInstalled(pkgs),
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			inst, ok := f["installed"]
			if !ok {
				return false, "no output"
			}
			have := map[string]bool{}
			for _, p := range strings.Fields(inst) {
				have[p] = true
			}
			var missing []string
			for _, p := range pkgs {
				if !have[p] {
					missing = append(missing, p)
				}
			}
			if len(missing) > 0 {
				return false, "not installed: " + strings.Join(missing, ", ")
			}
			return true, strings.Join(pkgs, ", ") + " installed"
		},
	}
}

// apparmorProfilesProbe: CIS 1.3.1.3, every loaded profile in enforce or
// complain mode, from `aa-status --json`.
func apparmorProfilesProbe() Probe {
	return Probe{
		Baseline: true,
		Command:  `sudo -n aa-status --json 2>/dev/null | head -c 1048576`,
		Judge: func(out string) (bool, string) {
			var st struct {
				Profiles map[string]string `json:"profiles"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &st); err != nil {
				return false, "aa-status --json unreadable"
			}
			if len(st.Profiles) == 0 {
				return false, "no AppArmor profiles loaded"
			}
			other := map[string]int{}
			for _, mode := range st.Profiles {
				if mode != "enforce" && mode != "complain" {
					other[mode]++
				}
			}
			if len(other) > 0 {
				var parts []string
				for m, n := range other {
					parts = append(parts, fmt.Sprintf("%d %s", n, m))
				}
				sort.Strings(parts)
				return false, fmt.Sprintf("%d profiles loaded, not enforce/complain: %s", len(st.Profiles), strings.Join(parts, ", "))
			}
			return true, fmt.Sprintf("all %d profiles enforce or complain", len(st.Profiles))
		},
	}
}

// Password and account policy (CIS 5.4.1.1-5.4.1.6, 5.4.2.1-5.4.2.3,
// 5.4.2.7, 5.4.2.8, 5.4.3.2) and journald (6.1.1.1). Values are the CIS
// profile's ComplianceAsCode variables: max age 365, warning 7, hashing
// SHA512|YESCRYPT, inactive 45, TMOUT 900. System accounts are as
// ComplianceAsCode defines them: not root, UID below UID_MIN, shell not
// nologin, /bin/sync, /sbin/shutdown, /sbin/halt or false.
func init() {
	hashed := `$2~/^\$/`
	Probes["pw-max-days"] = defsProbe(`echo "defs=$(awk '/^[[:space:]]*PASS_MAX_DAYS/{print $2}' /etc/login.defs)"; `+
		`sudo -n sh -c "awk -F: '(`+hashed+` && (\$5==\"\" || \$5>365)){print \"user \" \$1}' /etc/shadow; echo @ok"`,
		func(v string) bool { n, err := strconv.Atoi(v); return err == nil && n >= 1 && n <= 365 }, "PASS_MAX_DAYS 1-365")
	Probes["pw-warn-age"] = defsProbe(`echo "defs=$(awk '/^[[:space:]]*PASS_WARN_AGE/{print $2}' /etc/login.defs)"; echo @ok`,
		func(v string) bool { n, err := strconv.Atoi(v); return err == nil && n >= 7 }, "PASS_WARN_AGE >= 7")
	Probes["pw-hash-algo"] = defsProbe(`echo "defs=$(awk '/^[[:space:]]*ENCRYPT_METHOD/{print toupper($2)}' /etc/login.defs)"; echo @ok`,
		func(v string) bool { return v == "SHA512" || v == "YESCRYPT" }, "ENCRYPT_METHOD SHA512 or YESCRYPT")
	Probes["pw-inactive"] = defsProbe(`echo "defs=$(useradd -D | awk -F= '/^INACTIVE/{print $2}')"; `+
		`sudo -n sh -c "awk -F: '(`+hashed+` && (\$7==\"\" || \$7>45 || \$7<0)){print \"user \" \$1}' /etc/shadow; echo @ok"`,
		func(v string) bool { n, err := strconv.Atoi(v); return err == nil && n >= 0 && n <= 45 }, "INACTIVE 0-45")
	Probes["pw-change-past"] = offenderProbe(`t=$(( $(date +%s) / 86400 )); sudo -n sh -c "awk -F: -v t=$t '(\$3!=\"\" && \$3>t){print \$1}' /etc/shadow; echo @ok"`)
	Probes["acct-root-only-uid0"] = offenderProbe(`awk -F: '($3==0 && $1!="root"){print $1}' /etc/passwd && echo @ok`)
	Probes["acct-root-only-gid0"] = offenderProbe(`awk -F: '($4==0 && $1!="root"){print $1}' /etc/passwd && echo @ok`)
	Probes["group-root-only-gid0"] = offenderProbe(`awk -F: '($3==0 && $1!="root"){print $1}' /etc/group && echo @ok`)
	Probes["acct-system-no-shell"] = offenderProbe(`m=$(awk '/^[[:space:]]*UID_MIN/{print $2}' /etc/login.defs); ` +
		`awk -F: -v m="${m:-1000}" '($1!="root" && $3<m && $7!~/^(\/usr)?\/sbin\/nologin$/ && $7!~/^(\/usr)?\/bin\/false$/ && $7!="/bin/sync" && $7!="/sbin/shutdown" && $7!="/sbin/halt"){print $1 " " $7}' /etc/passwd && echo @ok`)
	// Accounts whose shell is not in /etc/shells must be locked (passwd -S: L).
	Probes["acct-noshell-locked"] = offenderProbe(`for u in $(awk -F: 'NR==FNR{s[$0]=1;next} !($7 in s){print $1}' /etc/shells /etc/passwd); do ` +
		`st=$(sudo -n passwd -S "$u" 2>/dev/null | awk '{print $2}'); [ "$st" = L ] || echo "$u ${st:-unreadable}"; done; echo @ok`)
	Probes["shell-tmout"] = markerLinesProbe(`grep -hsE '(^|[[:space:];])(readonly|export|declare|typeset)?[[:space:]]*[-rx[:space:]]*TMOUT' /etc/profile /etc/profile.d/*.sh /etc/bash.bashrc; echo @ok`,
		func(lines []string) (bool, string) {
			val, ro, ex := -1, false, false
			for _, l := range lines {
				if strings.HasPrefix(strings.TrimSpace(l), "#") {
					continue
				}
				if i := strings.Index(l, "TMOUT="); i >= 0 {
					f := strings.FieldsFunc(l[i+len("TMOUT="):], func(r rune) bool { return r == ' ' || r == ';' || r == '\t' })
					if len(f) > 0 {
						if n, err := strconv.Atoi(f[0]); err == nil {
							val = n
						}
					}
				}
				ro = ro || strings.Contains(l, "readonly TMOUT") || strings.Contains(l, "declare -r") || strings.Contains(l, "typeset -r")
				ex = ex || strings.Contains(l, "export TMOUT") || strings.Contains(l, "declare -rx") || strings.Contains(l, "declare -xr")
			}
			switch {
			case val < 1 || val > 900:
				return false, fmt.Sprintf("TMOUT %d (want 1-900)", val)
			case !ro:
				return false, "TMOUT not readonly: a user can unset it"
			case !ex:
				return false, "TMOUT not exported"
			}
			return true, fmt.Sprintf("TMOUT=%d readonly, exported", val)
		})
	Probes["journald-active"] = Probe{
		Baseline: true,
		Command:  `systemctl is-active systemd-journald.service`,
		Judge: func(out string) (bool, string) {
			v := strings.TrimSpace(out)
			return v == "active", "systemd-journald " + v
		},
	}
}

// defsProbe: a "defs=<value>" line judged by ok, plus any "user <name>"
// offender lines, then the "@ok" completion line.
func defsProbe(cmd string, ok func(string) bool, want string) Probe {
	checksUsers := strings.Contains(cmd, `print \"user \"`)
	return markerLinesProbe(cmd, func(lines []string) (bool, string) {
		var val string
		var users []string
		for _, l := range lines {
			if v, found := strings.CutPrefix(l, "defs="); found {
				val = strings.TrimSpace(v)
			} else if u, found := strings.CutPrefix(l, "user "); found {
				users = append(users, strings.TrimSpace(u))
			}
		}
		if !ok(val) {
			return false, fmt.Sprintf("%q (want %s)", val, want)
		}
		if len(users) > 0 {
			return false, fmt.Sprintf("default %s, but existing accounts do not follow it: %s", val, strings.Join(users, ", "))
		}
		if checksUsers {
			return true, "default " + val + ", existing accounts follow it"
		}
		return true, "default " + val
	})
}

// sshd algorithms, access, banner and MaxStartups (CIS 5.1.4-5.1.6,
// 5.1.12, 5.1.15, 5.1.18), judged on the effective `sshd -T` values.
// Allowlists are the CIS profile's ComplianceAsCode values for
// ubuntu2404 (sshd_use_strong_ciphers, sshd_strong_kex, sshd_strong_macs).
// Deliberate addition to the KEX list: mlkem768x25519-sha256 (and the
// unsuffixed sntrup761x25519-sha512 alias), post-quantum hybrids newer than
// that list. Removing them to match it would weaken the key exchange.
var (
	sshdStrongCiphers = []string{"chacha20-poly1305@openssh.com", "aes256-gcm@openssh.com", "aes128-gcm@openssh.com", "aes256-ctr", "aes192-ctr", "aes128-ctr"}
	sshdStrongKex     = []string{"sntrup761x25519-sha512@openssh.com", "curve25519-sha256", "curve25519-sha256@libssh.org", "ecdh-sha2-nistp256", "ecdh-sha2-nistp384",
		"ecdh-sha2-nistp521", "diffie-hellman-group-exchange-sha256", "diffie-hellman-group16-sha512", "diffie-hellman-group18-sha512", "diffie-hellman-group14-sha256",
		"mlkem768x25519-sha256", "sntrup761x25519-sha512"}
	sshdStrongMACs = []string{"hmac-sha2-512-etm@openssh.com", "hmac-sha2-256-etm@openssh.com", "hmac-sha2-512", "hmac-sha2-256"}
)

func init() {
	Probes["sshd-ciphers"] = sshdProbe("ciphers", func(v map[string]string) (bool, string) { return subsetOf(v["ciphers"], sshdStrongCiphers, "ciphers") })
	Probes["sshd-kex"] = sshdProbe("kexalgorithms", func(v map[string]string) (bool, string) {
		return subsetOf(v["kexalgorithms"], sshdStrongKex, "KexAlgorithms")
	})
	Probes["sshd-macs"] = sshdProbe("macs", func(v map[string]string) (bool, string) { return subsetOf(v["macs"], sshdStrongMACs, "MACs") })
	Probes["sshd-banner"] = sshdProbe("banner", func(v map[string]string) (bool, string) {
		b := v["banner"]
		if b == "" || b == "none" {
			return false, "no Banner"
		}
		return true, "Banner " + b
	})
	Probes["sshd-access"] = sshdProbe("allowusers|allowgroups|denyusers|denygroups", func(v map[string]string) (bool, string) {
		for _, k := range []string{"allowusers", "allowgroups", "denyusers", "denygroups"} {
			if v[k] != "" {
				return true, k + " " + v[k]
			}
		}
		return false, "none of AllowUsers, AllowGroups, DenyUsers, DenyGroups is set"
	})
	Probes["sshd-maxstartups"] = sshdProbe("maxstartups", func(v map[string]string) (bool, string) {
		f := strings.Split(v["maxstartups"], ":")
		if len(f) != 3 {
			return false, "MaxStartups " + v["maxstartups"] + " (want start:rate:full, 10:30:60 or stricter)"
		}
		n := make([]int, 3)
		for i := range f {
			x, err := strconv.Atoi(f[i])
			if err != nil {
				return false, "MaxStartups " + v["maxstartups"] + " unreadable"
			}
			n[i] = x
		}
		if n[0] > 10 || n[1] < 30 || n[2] > 60 {
			return false, "MaxStartups " + v["maxstartups"] + " (want 10:30:60 or stricter)"
		}
		return true, "MaxStartups " + v["maxstartups"]
	})
}

// sshdProbe reads the given keys from `sshd -T`. The port line proves the
// dump happened: without it (sudo refused, sshd missing) the check fails
// instead of judging absent keys.
func sshdProbe(keys string, judge func(map[string]string) (bool, string)) Probe {
	return Probe{
		Baseline: true,
		Command:  fmt.Sprintf(`sudo -n sshd -T 2>/dev/null | grep -E '^(port|%s) '`, keys),
		Judge: func(out string) (bool, string) {
			v := map[string]string{}
			for _, l := range strings.Split(out, "\n") {
				if k, val, ok := strings.Cut(strings.TrimSpace(l), " "); ok {
					v[k] = strings.TrimSpace(val)
				}
			}
			if v["port"] == "" {
				return false, "no sshd -T output"
			}
			return judge(v)
		},
	}
}

func subsetOf(list string, allowed []string, what string) (bool, string) {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	var weak []string
	for _, x := range strings.Split(list, ",") {
		if x = strings.TrimSpace(x); x != "" && !ok[x] {
			weak = append(weak, x)
		}
	}
	if list == "" {
		return false, what + " not reported"
	}
	if len(weak) > 0 {
		return false, what + " not on the strong list: " + strings.Join(weak, ", ")
	}
	return true, "every " + what + " entry is on the strong list"
}

// Login banners (CIS 1.6.1-1.6.6), bluetooth (3.1.3) and wireless (3.1.2).
// ComplianceAsCode's cis_banner template: the file has a non-empty line when
// the banner must be set (issue, issue.net; motd is optional) and contains
// none of \v \r \m \s or the product name. The product check here uses the
// OS NAME from /etc/os-release, case-insensitive: stricter than matching
// ComplianceAsCode's internal product id, and what CIS means.
func init() {
	for probe, spec := range map[string]struct {
		file     string
		required bool
	}{
		"banner-motd":      {"/etc/motd", false},
		"banner-issue":     {"/etc/issue", true},
		"banner-issue-net": {"/etc/issue.net", true},
	} {
		Probes[probe] = bannerProbe(spec.file, spec.required)
	}
	for name, r := range map[string]fileRule{
		"perm-etc-motd":      {[]fileTarget{{"/etc/motd", true}}, 0o644, "root"},
		"perm-etc-issue":     {[]fileTarget{{"/etc/issue", false}}, 0o644, "root"},
		"perm-etc-issue-net": {[]fileTarget{{"/etc/issue.net", false}}, 0o644, "root"},
	} {
		Probes[name] = filePermProbe(r)
	}
	Probes["svc-bluetooth-not-in-use"] = serviceProbe([]string{"bluez"}, []string{"bluetooth.service"})
	// Wireless interfaces that are up (operstate not down).
	Probes["radio-wireless-down"] = offenderProbe(`for d in /sys/class/net/*/wireless; do [ -e "$d" ] || continue; i=$(basename "$(dirname "$d")"); ` +
		`s=$(cat "/sys/class/net/$i/operstate" 2>/dev/null); [ "$s" = down ] || echo "$i $s"; done; echo @ok`)
}

func bannerProbe(file string, required bool) Probe {
	return markerLinesProbe(fmt.Sprintf(`echo "os=$(. /etc/os-release 2>/dev/null; echo "$NAME")"; if [ -e %s ]; then cat %s; else echo ABSENT; fi; echo @ok`, file, file),
		func(lines []string) (bool, string) {
			var osName string
			var body []string
			for _, l := range lines {
				if v, ok := strings.CutPrefix(l, "os="); ok {
					osName = strings.TrimSpace(v)
					continue
				}
				body = append(body, l)
			}
			if len(body) == 1 && strings.TrimSpace(body[0]) == "ABSENT" {
				if required {
					return false, file + " missing"
				}
				return true, file + " not present (optional)"
			}
			text := strings.Join(body, "\n")
			if required && strings.TrimSpace(text) == "" {
				return false, file + " is empty"
			}
			for _, esc := range []string{`\v`, `\r`, `\m`, `\s`} {
				if strings.Contains(text, esc) {
					return false, file + " contains " + esc + " (expands to OS details)"
				}
			}
			if first := strings.Fields(osName); len(first) > 0 && strings.Contains(strings.ToLower(text), strings.ToLower(first[0])) {
				return false, file + " names the OS (" + first[0] + ")"
			}
			return true, file + " states no OS details"
		})
}

// AIDE (CIS 6.3.1, 6.3.2): ComplianceAsCode's package_aide_installed,
// aide_build_database (Debian/Ubuntu: /var/lib/aide/aide.db[.gz]) and
// aide_periodic_checking_systemd_timer ("aidecheck.timer or similar";
// Ubuntu's aide-common ships dailyaidecheck.timer).
func init() {
	Probes["aide-installed"] = markerLinesProbe(dpkgInstalled([]string{"aide"})+
		`; sudo -n sh -c 'if [ -e /var/lib/aide/aide.db ] || [ -e /var/lib/aide/aide.db.gz ]; then echo db=present; else echo db=absent; fi; echo @ok'`,
		func(lines []string) (bool, string) {
			f := parseKV(strings.Join(lines, "\n"))
			switch {
			case !strings.Contains(" "+f["installed"]+" ", " aide "):
				return false, "aide not installed"
			case f["db"] != "present":
				return false, "aide installed but its database was never initialised (aideinit)"
			}
			return true, "aide installed with a database"
		})
	Probes["aide-timer"] = markerLinesProbe(`systemctl list-unit-files --type=timer --no-legend 2>/dev/null | awk 'tolower($1) ~ /aide/ {print $1, $2}'; echo @ok`,
		func(lines []string) (bool, string) {
			for _, l := range lines {
				if f := strings.Fields(l); len(f) == 2 && f[1] == "enabled" {
					return true, f[0] + " enabled"
				}
			}
			if len(lines) > 0 {
				return false, "AIDE timer present but not enabled: " + strings.Join(lines, ", ")
			}
			return false, "no AIDE timer"
		})
}

// Logging (CIS 6.1.1.4, 6.1.3.1-6.1.3.4, 6.1.3.7) for hosts on the rsyslog
// path (journald forwarding to rsyslog). The journald-remote path
// (6.1.2.x) is the alternative and is not claimed.
func init() {
	Probes["rsyslog-active"] = Probe{
		Baseline: true,
		Command: dpkgInstalled([]string{"rsyslog"}) +
			`; echo "enabled=$(systemctl is-enabled rsyslog.service 2>/dev/null || true)"; echo "active=$(systemctl is-active rsyslog.service 2>/dev/null || true)"`,
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			if !strings.Contains(" "+f["installed"]+" ", " rsyslog ") {
				return false, "rsyslog not installed"
			}
			if f["enabled"] != "enabled" || f["active"] != "active" {
				return false, fmt.Sprintf("rsyslog %s/%s (want enabled/active)", f["enabled"], f["active"])
			}
			return true, "rsyslog installed, enabled and active"
		},
	}
	Probes["logging-single-system"] = Probe{
		Baseline: true,
		Command:  `echo "rsyslog=$(systemctl is-active rsyslog.service 2>/dev/null || true)"; echo "upload=$(systemctl is-active systemd-journal-upload.service 2>/dev/null || true)"`,
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			r, u := f["rsyslog"] == "active", f["upload"] == "active"
			switch {
			case r && u:
				return false, "both rsyslog and systemd-journal-upload are active"
			case !r && !u:
				return false, "neither rsyslog nor systemd-journal-upload is active"
			case r:
				return true, "rsyslog only"
			}
			return true, "systemd-journal-upload only"
		},
	}
	Probes["journald-forward-syslog"] = markerLinesProbe(`systemd-analyze cat-config systemd/journald.conf 2>/dev/null | grep -E '^[[:space:]]*ForwardToSyslog=' | tr -d ' '; echo @ok`,
		func(lines []string) (bool, string) {
			v := "no (default)"
			for _, l := range lines {
				if x, ok := strings.CutPrefix(strings.TrimSpace(l), "ForwardToSyslog="); ok {
					v = x // the last assignment wins, as in journald
				}
			}
			return v == "yes", "ForwardToSyslog=" + v
		})
	Probes["rsyslog-filecreatemode"] = markerLinesProbe(`cat /etc/rsyslog.conf /etc/rsyslog.d/*.conf 2>/dev/null | grep -E '^[[:space:]]*\$FileCreateMode'; echo @ok`,
		func(lines []string) (bool, string) {
			v := ""
			for _, l := range lines {
				if f := strings.Fields(l); len(f) >= 2 {
					v = f[1]
				}
			}
			if v == "" {
				return false, "$FileCreateMode not set (rsyslog's default is 0644)"
			}
			m, err := strconv.ParseUint(v, 8, 32)
			if err != nil || uint32(m)&^0o640 != 0 {
				return false, "$FileCreateMode " + v + " (want 0640 or stricter)"
			}
			return true, "$FileCreateMode " + v
		})
	Probes["rsyslog-no-listen"] = markerLinesProbe(`cat /etc/rsyslog.conf /etc/rsyslog.d/*.conf 2>/dev/null | grep -Ev '^[[:space:]]*#' | grep -E 'imtcp|imudp|InputTCPServerRun|UDPServerRun'; echo @ok`,
		func(lines []string) (bool, string) {
			if len(lines) > 0 {
				return false, "rsyslog accepts remote logs: " + strings.TrimSpace(lines[0])
			}
			return true, "no network input modules"
		})
}

// Log files, journald retention and rotation (CIS 6.1.1.2, 6.1.1.3, 6.1.3.8,
// 6.1.4.1). ComplianceAsCode has 6.1.1.2, 6.1.1.3 and 6.1.3.8 as manual;
// each probe checks the mechanical part and the mapping claims no more.
// ADR-092 section 5 asks for the same journald caps.
func init() {
	// 6.1.1.2: journal files 0640 or stricter, directories not writable by
	// group or other. Mode only.
	Probes["journald-file-access"] = markerLinesProbe(`sudo -n sh -c "find /var/log/journal /run/log/journal \\( -type f -perm /0137 \\) -o \\( -type d -perm /0022 \\) 2>/dev/null | head -20; echo @ok"`,
		func(lines []string) (bool, string) {
			if len(lines) > 0 {
				return false, "journal paths looser than 0640 (files) or group/other-writable (dirs): " + firstN(lines, 3)
			}
			return true, "journal files 0640 or stricter"
		})
	// 6.1.1.3: journald's disk use capped explicitly. Unset, journald takes
	// up to 10% of the filesystem (at most 4 GiB) per store.
	Probes["journald-rotation"] = markerLinesProbe(`systemd-analyze cat-config systemd/journald.conf 2>/dev/null | grep -E '^[[:space:]]*(SystemMaxUse|RuntimeMaxUse)=' | tr -d ' '; echo @ok`,
		func(lines []string) (bool, string) {
			set := map[string]string{}
			for _, l := range lines {
				if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
					set[k] = v // the last assignment wins, as in journald
				}
			}
			var missing []string
			for _, k := range []string{"SystemMaxUse", "RuntimeMaxUse"} {
				if set[k] == "" {
					missing = append(missing, k)
				}
			}
			if len(missing) > 0 {
				return false, strings.Join(missing, " and ") + " not set (journald default: 10% of the filesystem)"
			}
			return true, "SystemMaxUse=" + set["SystemMaxUse"] + " RuntimeMaxUse=" + set["RuntimeMaxUse"]
		})
	// 6.1.3.8: logrotate installed, its timer enabled and active.
	Probes["logrotate-active"] = Probe{
		Baseline: true,
		Command: dpkgInstalled([]string{"logrotate"}) +
			`; echo "enabled=$(systemctl is-enabled logrotate.timer 2>/dev/null || true)"; echo "active=$(systemctl is-active logrotate.timer 2>/dev/null || true)"`,
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			if !strings.Contains(" "+f["installed"]+" ", " logrotate ") {
				return false, "logrotate not installed"
			}
			if f["enabled"] != "enabled" || f["active"] != "active" {
				return false, fmt.Sprintf("logrotate.timer %s/%s (want enabled/active)", f["enabled"], f["active"])
			}
			return true, "logrotate installed, timer enabled and active"
		},
	}
	// 6.1.4.1: no file under /var/log looser than CIS allows for its name.
	// Mode only; owner and group are not judged.
	Probes["var-log-perms"] = markerLinesProbe(`sudo -n sh -c "find /var/log -type f -printf '%m %p\\n' 2>/dev/null; echo @ok"`, judgeVarLogPerms)
}

// varLogMaxMode is CIS 6.1.4.1's per-name ceiling: login records may be
// group-writable and world-readable, a few agents' logs world-readable,
// sssd and gdm logs group-writable, everything else 0640.
func varLogMaxMode(path string) uint32 {
	base := path[strings.LastIndex(path, "/")+1:]
	dir := path[:strings.LastIndex(path, "/")+1]
	for _, p := range []string{"lastlog", "wtmp", "btmp"} {
		if base == p || strings.HasPrefix(base, p+".") || strings.HasPrefix(base, p+"-") {
			return 0o664
		}
	}
	switch {
	case base == "README":
		return 0o664
	case strings.HasPrefix(base, "cloud-init.log"), strings.HasPrefix(base, "localmessages"), strings.HasPrefix(base, "waagent.log"):
		return 0o644
	case strings.Contains(dir, "/sssd/"), strings.Contains(dir, "/SSSD/"), strings.Contains(dir, "/gdm/"), strings.Contains(dir, "/gdm3/"):
		return 0o660
	}
	return 0o640
}

func judgeVarLogPerms(lines []string) (bool, string) {
	var loose []string
	for _, l := range lines {
		modeStr, path, ok := strings.Cut(strings.TrimSpace(l), " ")
		if !ok {
			continue
		}
		m, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			return false, "unreadable mode in: " + l
		}
		if limit := varLogMaxMode(path); uint32(m)&^limit != 0 {
			loose = append(loose, fmt.Sprintf("%s %04o (max %04o)", path, m, limit))
		}
	}
	if len(loose) > 0 {
		sort.Strings(loose)
		return false, fmt.Sprintf("%d files looser than allowed: %s", len(loose), firstN(loose, 5))
	}
	return true, fmt.Sprintf("%d files within limits", len(lines))
}

// firstN joins up to n items, noting how many more there are.
func firstN(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(items[:n], ", "), len(items)-n)
}

// PAM (CIS 5.3.1.2, 5.3.1.3, 5.3.2.1-5.3.2.4, 5.3.3.1.1-5.3.3.4.4 except
// the manual 5.3.3.2.3). Values are the CIS profile's ComplianceAsCode
// variables: faillock deny <= 4, unlock_time 0 or >= 900; pwquality difok
// >= 2, minlen >= 14, maxrepeat 1-3, maxsequence 1-3, dictcheck and
// enforcing not 0, enforce_for_root; pwhistory remember >= 24,
// enforce_for_root, use_authtok; pam_unix without nullok or remember,
// with yescrypt or sha512 and use_authtok. A module's built-in default
// counts when nothing sets the value (faillock deny 3, unlock_time 600;
// pwquality minlen 8, difok 1, maxrepeat 0, maxsequence 0; pwhistory
// remember 10). Arguments on the PAM line override the .conf file.
// 5.3.1.1 (latest PAM version) is not claimed: it needs the archive's
// current version, which a host check cannot know.
// Section markers are "%%", never "@@": HostProbes splits a host's script
// output on lines starting "@@", so "@@pam" ended this probe's section.
const pamDump = `echo %%pam; cat /etc/pam.d/common-auth /etc/pam.d/common-account /etc/pam.d/common-password /etc/pam.d/common-session 2>/dev/null; ` +
	`echo %%pwq; cat /etc/security/pwquality.conf /etc/security/pwquality.conf.d/*.conf 2>/dev/null; ` +
	`echo %%fl; cat /etc/security/faillock.conf 2>/dev/null; ` +
	`echo %%ph; cat /etc/security/pwhistory.conf 2>/dev/null; echo @ok`

type pamLine struct {
	typ, module string
	args        []string
}

type pamConfig struct {
	lines []pamLine
	conf  map[string]map[string]string // section -> key -> value ("" for a bare flag)
}

func parsePAM(out string) (*pamConfig, bool) {
	c := &pamConfig{conf: map[string]map[string]string{}}
	section, done := "", false
	for _, raw := range strings.Split(out, "\n") {
		l := strings.TrimSpace(raw)
		if i := strings.Index(l, "#"); i >= 0 {
			l = strings.TrimSpace(l[:i])
		}
		switch {
		case l == "":
			continue
		case l == "@ok":
			done = true
			continue
		case strings.HasPrefix(l, "%%"):
			section = l[2:]
			continue
		}
		if section == "pam" {
			f := strings.Fields(l)
			if len(f) < 3 || strings.HasPrefix(f[0], "@") {
				continue
			}
			i := 1
			if strings.HasPrefix(f[1], "[") { // control like [success=1 default=ignore]
				for i < len(f) && !strings.HasSuffix(f[i], "]") {
					i++
				}
			}
			if i+1 >= len(f) {
				continue
			}
			c.lines = append(c.lines, pamLine{typ: f[0], module: f[i+1], args: f[i+2:]})
			continue
		}
		if c.conf[section] == nil {
			c.conf[section] = map[string]string{}
		}
		k, v, _ := strings.Cut(l, "=")
		c.conf[section][strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return c, done
}

// effective merges a module's .conf settings with its PAM-line arguments.
func (c *pamConfig) effective(section, module string) map[string]string {
	m := map[string]string{}
	for k, v := range c.conf[section] {
		m[k] = v
	}
	for _, l := range c.lines {
		if l.module == module {
			for _, a := range l.args {
				k, v, _ := strings.Cut(a, "=")
				m[k] = v
			}
		}
	}
	return m
}

func (c *pamConfig) uses(module string) []pamLine {
	var out []pamLine
	for _, l := range c.lines {
		if l.module == module {
			out = append(out, l)
		}
	}
	return out
}

func pamProbe(judge func(c *pamConfig) (bool, string)) Probe {
	return Probe{Baseline: true, Command: pamDump, Judge: func(out string) (bool, string) {
		c, done := parsePAM(out)
		if !done {
			return false, "check did not complete"
		}
		return judge(c)
	}}
}

func intIn(m map[string]string, key string, def int, ok func(int) bool, want string) (bool, string) {
	v, set := m[key]
	n := def
	if set {
		var err error
		if n, err = strconv.Atoi(v); err != nil {
			return false, key + "=" + v + " unreadable"
		}
	}
	src := key + "=" + strconv.Itoa(n)
	if !set {
		src += " (default)"
	}
	if !ok(n) {
		return false, src + " (want " + want + ")"
	}
	return true, src
}

func init() {
	Probes["pkg-pam-modules"] = packageInstalledProbe([]string{"libpam-modules"})
	Probes["pkg-pam-pwquality"] = packageInstalledProbe([]string{"libpam-pwquality"})
	enabled := func(module string, types ...string) Probe {
		return pamProbe(func(c *pamConfig) (bool, string) {
			have := map[string]bool{}
			for _, l := range c.uses(module) {
				have[l.typ] = true
			}
			var missing []string
			for _, t := range types {
				if !have[t] {
					missing = append(missing, t)
				}
			}
			if len(missing) > 0 {
				return false, module + " missing from: " + strings.Join(missing, ", ")
			}
			return true, module + " in " + strings.Join(types, ", ")
		})
	}
	Probes["pam-unix-enabled"] = enabled("pam_unix.so", "auth", "account", "password", "session")
	Probes["pam-faillock-enabled"] = enabled("pam_faillock.so", "auth", "account")
	Probes["pam-pwquality-enabled"] = enabled("pam_pwquality.so", "password")
	Probes["pam-pwhistory-enabled"] = enabled("pam_pwhistory.so", "password")

	// A setting protects nothing while its module is not in the PAM stack:
	// without that guard faillock's default deny=3 would pass on a host that
	// never runs faillock.
	setting := func(section, module, key string, def int, ok func(int) bool, want string) Probe {
		return pamProbe(func(c *pamConfig) (bool, string) {
			if len(c.uses(module)) == 0 {
				return false, module + " is not enabled, so " + key + " has no effect"
			}
			return intIn(c.effective(section, module), key, def, ok, want)
		})
	}
	flag := func(section, module, key string) Probe {
		return pamProbe(func(c *pamConfig) (bool, string) {
			if len(c.uses(module)) == 0 {
				return false, module + " is not enabled, so " + key + " has no effect"
			}
			if _, ok := c.effective(section, module)[key]; ok {
				return true, key + " set"
			}
			return false, key + " not set for " + module
		})
	}
	Probes["pam-faillock-deny"] = setting("fl", "pam_faillock.so", "deny", 3, func(n int) bool { return n >= 1 && n <= 4 }, "1-4")
	Probes["pam-faillock-unlock"] = setting("fl", "pam_faillock.so", "unlock_time", 600, func(n int) bool { return n == 0 || n >= 900 }, "0 or >= 900")
	Probes["pam-pwq-difok"] = setting("pwq", "pam_pwquality.so", "difok", 1, func(n int) bool { return n >= 2 }, ">= 2")
	Probes["pam-pwq-minlen"] = setting("pwq", "pam_pwquality.so", "minlen", 8, func(n int) bool { return n >= 14 }, ">= 14")
	Probes["pam-pwq-maxrepeat"] = setting("pwq", "pam_pwquality.so", "maxrepeat", 0, func(n int) bool { return n >= 1 && n <= 3 }, "1-3")
	Probes["pam-pwq-maxsequence"] = setting("pwq", "pam_pwquality.so", "maxsequence", 0, func(n int) bool { return n >= 1 && n <= 3 }, "1-3")
	Probes["pam-pwq-dictcheck"] = setting("pwq", "pam_pwquality.so", "dictcheck", 1, func(n int) bool { return n != 0 }, "not 0")
	Probes["pam-pwq-enforcing"] = setting("pwq", "pam_pwquality.so", "enforcing", 1, func(n int) bool { return n != 0 }, "not 0")
	Probes["pam-pwq-enforce-root"] = flag("pwq", "pam_pwquality.so", "enforce_for_root")
	Probes["pam-pwhistory-remember"] = setting("ph", "pam_pwhistory.so", "remember", 10, func(n int) bool { return n >= 24 }, ">= 24")
	Probes["pam-pwhistory-enforce-root"] = flag("ph", "pam_pwhistory.so", "enforce_for_root")
	Probes["pam-pwhistory-authtok"] = pamProbe(func(c *pamConfig) (bool, string) {
		for _, l := range c.uses("pam_pwhistory.so") {
			for _, a := range l.args {
				if a == "use_authtok" {
					return true, "pam_pwhistory use_authtok"
				}
			}
		}
		return false, "pam_pwhistory without use_authtok (or not enabled)"
	})
	unixArgs := func(typ string, judge func(args []string) (bool, string)) Probe {
		return pamProbe(func(c *pamConfig) (bool, string) {
			found := false
			for _, l := range c.uses("pam_unix.so") {
				if typ != "" && l.typ != typ {
					continue
				}
				found = true
				if ok, why := judge(l.args); !ok {
					return false, l.typ + " pam_unix: " + why
				}
			}
			if !found {
				return false, "pam_unix not configured"
			}
			if typ == "" {
				return true, "every pam_unix line complies"
			}
			return true, "pam_unix " + typ + " lines comply"
		})
	}
	has := func(args []string, pred func(string) bool) bool {
		for _, a := range args {
			if pred(a) {
				return true
			}
		}
		return false
	}
	Probes["pam-unix-no-nullok"] = unixArgs("", func(a []string) (bool, string) {
		if has(a, func(s string) bool { return s == "nullok" }) {
			return false, "nullok (empty passwords accepted)"
		}
		return true, ""
	})
	Probes["pam-unix-no-remember"] = unixArgs("", func(a []string) (bool, string) {
		if has(a, func(s string) bool { return strings.HasPrefix(s, "remember=") }) {
			return false, "remember= (history belongs in pam_pwhistory)"
		}
		return true, ""
	})
	Probes["pam-unix-strong-hash"] = unixArgs("password", func(a []string) (bool, string) {
		if !has(a, func(s string) bool { return s == "yescrypt" || s == "sha512" }) {
			return false, "neither yescrypt nor sha512"
		}
		return true, ""
	})
	Probes["pam-unix-authtok"] = unixArgs("password", func(a []string) (bool, string) {
		if !has(a, func(s string) bool { return s == "use_authtok" }) {
			return false, "no use_authtok"
		}
		return true, ""
	})
}

// Time synchronisation (CIS 2.3.1.1, 2.3.3.1-2.3.3.3) on hosts that use
// chrony; the systemd-timesyncd alternative (2.3.2.x) is not claimed. Which
// servers are "authorized" is site policy: the check is that chrony has at
// least one server or pool, and the policy recorded in UH-TIME-01 is
// Ubuntu's shipped pools. GDM (1.7.1, Level 2) must not be installed; the
// other 1.7.x items configure GDM and do not apply without it.
func init() {
	Probes["time-single-daemon"] = Probe{
		Baseline: true,
		Command:  `echo "chrony=$(systemctl is-active chrony.service 2>/dev/null || true)"; echo "timesyncd=$(systemctl is-active systemd-timesyncd.service 2>/dev/null || true)"`,
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			c, ts := f["chrony"] == "active", f["timesyncd"] == "active"
			switch {
			case c && ts:
				return false, "chrony and systemd-timesyncd both active"
			case !c && !ts:
				return false, "no time synchronisation daemon active"
			case c:
				return true, "chrony only"
			}
			return true, "systemd-timesyncd only"
		},
	}
	Probes["chrony-sources"] = markerLinesProbe(`cat /etc/chrony/chrony.conf /etc/chrony/conf.d/*.conf /etc/chrony/sources.d/*.sources 2>/dev/null | grep -E '^[[:space:]]*(server|pool)[[:space:]]'; echo @ok`,
		func(lines []string) (bool, string) {
			if len(lines) == 0 {
				return false, "chrony has no server or pool"
			}
			return true, fmt.Sprintf("%d server/pool line(s), e.g. %s", len(lines), strings.TrimSpace(lines[0]))
		})
	Probes["chrony-user"] = markerLinesProbe(`ps -o user= -C chronyd 2>/dev/null; echo @ok`,
		func(lines []string) (bool, string) {
			if len(lines) == 0 {
				return false, "chronyd is not running"
			}
			for _, l := range lines {
				if u := strings.TrimSpace(l); u != "_chrony" {
					return false, "chronyd running as " + u
				}
			}
			return true, "chronyd runs as _chrony"
		})
	Probes["chrony-active"] = Probe{
		Baseline: true,
		Command:  `echo "enabled=$(systemctl is-enabled chrony.service 2>/dev/null || true)"; echo "active=$(systemctl is-active chrony.service 2>/dev/null || true)"`,
		Judge: func(out string) (bool, string) {
			f := parseKV(out)
			if f["enabled"] != "enabled" || f["active"] != "active" {
				return false, fmt.Sprintf("chrony %s/%s (want enabled/active)", f["enabled"], f["active"])
			}
			return true, "chrony enabled and active"
		},
	}
	Probes["pkg-gdm-absent"] = packageAbsentProbe([]string{"gdm3"})
}
