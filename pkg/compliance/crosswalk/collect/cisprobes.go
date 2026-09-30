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
	offenders := map[string]string{
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
