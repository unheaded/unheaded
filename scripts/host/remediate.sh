#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Host remediation for the ADR-098 findings register, one named fix at a
# time. Plan is the default and changes nothing; apply needs root, backs up
# every file it touches and writes an undo script next to the backups.
#
#   remediate.sh list
#   remediate.sh plan  <fix>        # what would change on this host (no root needed)
#   remediate.sh apply <fix>        # as root, after Stevie's yes for this fix on this host
#   remediate.sh undo  <backup-dir> # restore the files one apply changed
#
# Only low-risk fixes (register steps 2-4) live here. sshd authentication,
# the firewall (scripts/host/nft-apply-confirm.sh) and anything waiting on a
# decision are deliberately absent. Host-specific content keys off the short
# hostname: west and east only. Verify afterwards with
# `go run ./cmd/compliance-evidence -skip-gates` (or unheaded-baseline).
#
# ROOT (default /) and the command variables exist for the test harness.
set -euo pipefail

ROOT="${ROOT:-}"
SYSCTL="${SYSCTL:-sysctl}"
SYSTEMCTL="${SYSTEMCTL:-systemctl}"
SSHD="${SSHD:-sshd}"
VISUDO="${VISUDO:-visudo}"
APT="${APT:-apt-get}"
AUGENRULES="${AUGENRULES:-augenrules}"
SUDO_BIN="${SUDO_BIN:-sudo}"
STATE="${UNHEADED_REMEDIATE_STATE:-/var/lib/unheaded/remediate}"
HOST="${UNHEADED_HOST:-$(hostname -s)}"

die() { echo "remediate: $*" >&2; exit 1; }

FIXES="sysctl-cis apport-off kmod-cis cron-perms shell-tmout sshd-safe banner-cis apparmor-utils sudo-cis auditd auditd-rules journald-caps time-p2p"

# desired content per fix, per host -----------------------------------------
content_sysctl() {
	cat <<'CONF'
# ADR-098 / CIS Ubuntu 24.04 1.5.3, 3.3.2, 3.3.5, 3.3.6, 3.3.9. Managed by
# scripts/host/remediate.sh (sysctl-cis); edit there, not here.
fs.suid_dumpable = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv6.conf.all.accept_redirects = 0
net.ipv6.conf.default.accept_redirects = 0
net.ipv4.conf.all.secure_redirects = 0
net.ipv4.conf.default.secure_redirects = 0
net.ipv4.conf.all.log_martians = 1
net.ipv4.conf.default.log_martians = 1
CONF
	# east has no global IPv6 address, so router advertisements serve nothing
	# there (CIS 3.3.11). west takes its ISP address from them: left alone.
	if [[ "$HOST" == east ]]; then
		printf '%s\n' "net.ipv6.conf.all.accept_ra = 0" "net.ipv6.conf.default.accept_ra = 0"
	fi
}

content_kmod() {
	echo "# ADR-098 / CIS Ubuntu 24.04 1.1.1.x. Managed by scripts/host/remediate.sh (kmod-cis)."
	# usb-storage waits on Stevie's answer (does anyone plug USB storage in).
	local m
	for m in cramfs freevxfs hfs hfsplus jffs2 udf; do
		printf 'install %s /bin/false\nblacklist %s\n' "$m" "$m"
	done
}

content_tmout() {
	cat <<'CONF'
# ADR-098 / CIS Ubuntu 24.04 5.4.3.2. Managed by scripts/host/remediate.sh (shell-tmout).
readonly TMOUT=900
export TMOUT
CONF
}

content_sshd() {
	cat <<'CONF'
# ADR-098 step 4 / CIS Ubuntu 24.04 5.1.x. Managed by scripts/host/remediate.sh
# (sshd-safe). Nothing here changes how anyone authenticates.
CONF
	# sshd uses the first value it reads and 00- sorts first. On west,
	# 99-unheaded.conf already sets X11Forwarding no, MaxAuthTries 3 and
	# ClientAlive* 300/2, stricter than the values below: repeating them here
	# would loosen west. So they are written for east only.
	if [[ "$HOST" == east ]]; then
		printf '%s\n' "X11Forwarding no" "AllowAgentForwarding no" "MaxAuthTries 4" "ClientAliveInterval 300" "ClientAliveCountMax 3"
	fi
	printf '%s\n' "LoginGraceTime 60" "MaxStartups 10:30:60" \
		"MACs hmac-sha2-512-etm@openssh.com,hmac-sha2-256-etm@openssh.com,hmac-sha2-512,hmac-sha2-256"
}

content_banner() {
	echo "Authorized users only. All activity may be monitored and reported."
}

content_sudo() {
	echo "# ADR-098 / CIS Ubuntu 24.04 5.2.3, 5.2.6. Managed by scripts/host/remediate.sh (sudo-cis)."
	# sudo-rs (Ubuntu 25.10's sudo) rejects the logfile setting; it logs every
	# command to syslog (auth.log) instead, so the line is only for sudo.ws.
	if ! "$SUDO_BIN" --version 2>&1 | grep -q '^sudo-rs'; then # sudo-rs prints its version on stderr
		echo 'Defaults logfile="/var/log/sudo.log"'
	fi
	echo "Defaults timestamp_timeout=15"
}

# CIS Ubuntu 24.04 6.2.3 audit rules, the subset that fits lab hosts: not
# immutable (-e 2 would make undo need a reboot), and without the per-call
# delete (6.2.3.13) and chmod/chown (6.2.3.9) rules, which every build on
# these hosts would flood. Watches only name paths that exist on 25.10:
# utmp, faillock, setfacl and chacl are absent there.
content_journald() {
	cat <<'CONF'
# ADR-092 section 5 / CIS Ubuntu 24.04 6.1.1.3. Managed by scripts/host/remediate.sh (journald-caps).
[Journal]
SystemMaxUse=1G
RuntimeMaxUse=200M
CONF
}

content_timep2p() {
	cat <<'CONF'
# West serves time to east over the point-to-point link, also while west's
# own uplink is out (east reaches the internet only through west). Managed
# by scripts/host/remediate.sh (time-p2p).
CONF
	if [[ "$HOST" == west ]]; then
		printf '%s\n' "allow 192.168.13.1" "bindaddress 192.168.13.2" "local stratum 10"
	else
		printf '%s\n' "server 192.168.13.2 iburst"
	fi
}

content_audit() {
	cat <<'RULES'
## ADR-098 / CIS Ubuntu 24.04 6.2.3 (lab subset). Managed by scripts/host/remediate.sh (auditd-rules).
## sudoers and privilege use (6.2.3.1, 6.2.3.2)
-w /etc/sudoers -p wa -k scope
-w /etc/sudoers.d -p wa -k scope
-a always,exit -F arch=b64 -C euid!=uid -F auid!=unset -S execve -k user_emulation
-a always,exit -F arch=b32 -C euid!=uid -F auid!=unset -S execve -k user_emulation
## time (6.2.3.4)
-a always,exit -F arch=b64 -S adjtimex,settimeofday -k time-change
-a always,exit -F arch=b32 -S adjtimex,settimeofday -k time-change
-a always,exit -F arch=b64 -S clock_settime -F a0=0x0 -k time-change
-a always,exit -F arch=b32 -S clock_settime -F a0=0x0 -k time-change
-w /etc/localtime -p wa -k time-change
## network environment (6.2.3.5)
-a always,exit -F arch=b64 -S sethostname,setdomainname -k system-locale
-a always,exit -F arch=b32 -S sethostname,setdomainname -k system-locale
-w /etc/issue -p wa -k system-locale
-w /etc/issue.net -p wa -k system-locale
-w /etc/hosts -p wa -k system-locale
-w /etc/hostname -p wa -k system-locale
-w /etc/networks -p wa -k system-locale
-w /etc/network -p wa -k system-locale
-w /etc/netplan -p wa -k system-locale
## access denied (6.2.3.7)
-a always,exit -F arch=b64 -S creat,open,openat,truncate,ftruncate -F exit=-EACCES -F auid>=1000 -F auid!=unset -k access
-a always,exit -F arch=b64 -S creat,open,openat,truncate,ftruncate -F exit=-EPERM -F auid>=1000 -F auid!=unset -k access
-a always,exit -F arch=b32 -S creat,open,openat,truncate,ftruncate -F exit=-EACCES -F auid>=1000 -F auid!=unset -k access
-a always,exit -F arch=b32 -S creat,open,openat,truncate,ftruncate -F exit=-EPERM -F auid>=1000 -F auid!=unset -k access
## identity (6.2.3.8)
-w /etc/group -p wa -k identity
-w /etc/passwd -p wa -k identity
-w /etc/gshadow -p wa -k identity
-w /etc/shadow -p wa -k identity
-w /etc/security/opasswd -p wa -k identity
-w /etc/nsswitch.conf -p wa -k identity
-w /etc/pam.conf -p wa -k identity
-w /etc/pam.d -p wa -k identity
## mounts (6.2.3.10)
-a always,exit -F arch=b64 -S mount -F auid>=1000 -F auid!=unset -k mounts
-a always,exit -F arch=b32 -S mount -F auid>=1000 -F auid!=unset -k mounts
## sessions and logins (6.2.3.11, 6.2.3.12)
-w /var/log/wtmp -p wa -k session
-w /var/log/btmp -p wa -k session
-w /var/log/lastlog -p wa -k logins
## MAC policy (6.2.3.14)
-w /etc/apparmor -p wa -k MAC-policy
-w /etc/apparmor.d -p wa -k MAC-policy
## account changes and kernel modules (6.2.3.18, 6.2.3.19)
-a always,exit -F path=/usr/sbin/usermod -F perm=x -F auid>=1000 -F auid!=unset -k usermod
-a always,exit -F arch=b64 -S init_module,finit_module,delete_module -F auid>=1000 -F auid!=unset -k kernel_modules
-a always,exit -F path=/usr/bin/kmod -F perm=x -F auid>=1000 -F auid!=unset -k kernel_modules
## sshd configuration (not in CIS 6.2.3; ADR-098 tracks it)
-w /etc/ssh/sshd_config -p wa -k sshd
-w /etc/ssh/sshd_config.d -p wa -k sshd
RULES
}

# helpers ---------------------------------------------------------------------
BACKUP=""
begin_apply() {
	[[ $EUID -eq 0 || "${UNHEADED_REMEDIATE_ALLOW_NONROOT:-}" == 1 ]] || die "apply needs root"
	BACKUP="$STATE/$(date -u +%Y%m%dT%H%M%SZ)-$1"
	mkdir -p "$BACKUP"
	chmod 0700 "$STATE" "$BACKUP"
	: >"$BACKUP/undo.lines"
	write_undo
}

# undo_line records one step; undo.sh replays them newest first, so a file
# created and then chmod-ed is un-chmod-ed before it is removed.
undo_line() {
	printf '%s\n' "$1" >>"$BACKUP/undo.lines"
	write_undo
}

write_undo() {
	{ echo "#!/usr/bin/env bash"; echo "set -euo pipefail"; tac "$BACKUP/undo.lines"; } >"$BACKUP/undo.sh"
	chmod 0700 "$BACKUP/undo.sh"
}

# backup P records P's current state (content+mode+owner, or absence) and the
# undo line that restores it.
backup() {
	local p="$1" f="$ROOT$1" saved
	saved="$BACKUP/files$(dirname "$p")"
	mkdir -p "$saved"
	if [[ -e "$f" ]]; then
		cp -a "$f" "$saved/"
		undo_line "$(printf 'cp -a %q %q' "$saved/$(basename "$p")" "$f")"
	else
		undo_line "$(printf 'rm -f %q' "$f")"
	fi
}

# ensure_dir D creates D and any missing parents 0755, recording each for
# undo. Called before the file's own backup, so undo (newest first) removes
# the file, then the directories, deepest first.
ensure_dir() {
	local p="$1" i missing=()
	while [[ ! -e "$ROOT$p" ]]; do missing+=("$p"); p="$(dirname "$p")"; done
	for ((i = ${#missing[@]} - 1; i >= 0; i--)); do
		mkdir -m 0755 "$ROOT${missing[i]}"
		undo_line "$(printf 'rmdir %q' "$ROOT${missing[i]}")"
	done
}

# plan_file P MODE CONTENT: show the diff; apply_file writes it (backed up).
plan_file() {
	local p="$1" mode="$2" want="$3" f="$ROOT$1"
	if [[ -f "$f" ]] && diff -q <(printf '%s\n' "$want") "$f" >/dev/null && [[ "$(stat -c %a "$f")" == "$mode" ]]; then
		echo "  $p: already as wanted"
		return 1
	fi
	echo "  $p: would write (mode $mode):"
	diff -u --label "$p (now)" --label "$p (wanted)" <( [[ -f "$f" ]] && cat "$f" || true ) <(printf '%s\n' "$want") | sed 's/^/    /' || true
	return 0
}

apply_file() {
	local p="$1" mode="$2" want="$3" f="$ROOT$1" tmp
	ensure_dir "$(dirname "$p")"
	backup "$p"
	tmp="$(mktemp "$(dirname "$f")/.remediate.XXXXXX")"
	printf '%s\n' "$want" >"$tmp"
	chmod "$mode" "$tmp"
	chown root:root "$tmp" 2>/dev/null || true
	mv -f "$tmp" "$f"
	echo "  wrote $p"
}

plan_mode() { # path mode [owner:group]
	local f="$ROOT$1"
	[[ -e "$f" ]] || { echo "  $1: absent"; return 1; }
	local now; now="$(stat -c '%a %U:%G' "$f")"
	if [[ "$now" == "$2 ${3:-root:root}" ]]; then echo "  $1: already $now"; return 1; fi
	echo "  $1: $now -> $2 ${3:-root:root}"
}

apply_mode() {
	local f="$ROOT$1"
	[[ -e "$f" ]] || return 0
	undo_line "$(printf 'chmod %q %q; chown %q %q' "$(stat -c %a "$f")" "$f" "$(stat -c %U:%G "$f")" "$f")"
	chmod "$2" "$f"
	chown "${3:-root:root}" "$f" 2>/dev/null || true
	echo "  set $1 to $2 ${3:-root:root}"
}

# cron_allow: root plus the sudo group's members, one per line. Restricting
# crontab must not take it away from the people who run this lab.
cron_allow() {
	{ echo root; awk -F: '$1=="sudo"{print $4}' "$ROOT/etc/group" 2>/dev/null | tr ',' '\n'; } | grep -v '^$' | awk '!seen[$0]++'
}

known_host() { [[ "$HOST" == west || "$HOST" == east ]] || die "host '$HOST' is not west or east; host-specific content is not defined for it"; }

# fixes -------------------------------------------------------------------------
fix_sysctl-cis() {
	local mode="$1" want; known_host; want="$(content_sysctl)"
	if [[ "$mode" == plan ]]; then plan_file /etc/sysctl.d/60-unheaded-cis.conf 644 "$want" || true; echo "  then: $SYSCTL --system"; return; fi
	apply_file /etc/sysctl.d/60-unheaded-cis.conf 644 "$want"
	# Undo restores the files and reloads; a value no file sets any more keeps
	# its runtime setting until reboot (fs.suid_dumpable returns to 2 when
	# apport next starts).
	undo_line "$SYSCTL --system"
	"$SYSCTL" --system >/dev/null
	echo "  reloaded sysctl"
}

fix_apport-off() {
	local mode="$1"
	# apport rewrites fs.suid_dumpable to 2 when it starts: disable it first.
	if [[ "$mode" == plan ]]; then
		if grep -qs '^enabled=0' "$ROOT/etc/default/apport"; then
			echo "  /etc/default/apport: already enabled=0"
		else
			echo "  /etc/default/apport: enabled=1 -> enabled=0"
		fi
		echo "  then: $SYSTEMCTL disable --now apport.service; $SYSTEMCTL mask apport.service"
		return
	fi
	backup /etc/default/apport
	if [[ -f "$ROOT/etc/default/apport" ]]; then sed -i 's/^enabled=.*/enabled=0/' "$ROOT/etc/default/apport"; fi
	undo_line "$SYSTEMCTL enable --now apport.service"
	undo_line "$SYSTEMCTL unmask apport.service"
	"$SYSTEMCTL" disable --now apport.service
	"$SYSTEMCTL" mask apport.service
	echo "  apport disabled and masked"
}

fix_kmod-cis() {
	local mode="$1" want; want="$(content_kmod)"
	if [[ "$mode" == plan ]]; then plan_file /etc/modprobe.d/unheaded-cis.conf 644 "$want" || true; echo "  (usb-storage left loadable until decided)"; return; fi
	apply_file /etc/modprobe.d/unheaded-cis.conf 644 "$want"
}

fix_cron-perms() {
	local mode="$1" d
	if [[ "$mode" == plan ]]; then
		plan_mode /etc/crontab 600 || true
		for d in cron.hourly cron.daily cron.weekly cron.monthly cron.d; do plan_mode "/etc/$d" 700 || true; done
		plan_file /etc/cron.allow 640 "$(cron_allow)" || true
		if [[ -e "$ROOT/etc/cron.deny" ]]; then echo "  /etc/cron.deny: would be removed"; fi
		echo "  (cron.allow lists root and the sudo group, so the operator keeps crontab -e)"
		return
	fi
	apply_mode /etc/crontab 600
	for d in cron.hourly cron.daily cron.weekly cron.monthly cron.d; do apply_mode "/etc/$d" 700; done
	apply_file /etc/cron.allow 640 "$(cron_allow)"
	chown root:crontab "$ROOT/etc/cron.allow" 2>/dev/null || true
	if [[ -e "$ROOT/etc/cron.deny" ]]; then backup /etc/cron.deny; rm -f "$ROOT/etc/cron.deny"; echo "  removed /etc/cron.deny"; fi
}

fix_shell-tmout() {
	local mode="$1" want; want="$(content_tmout)"
	if [[ "$mode" == plan ]]; then plan_file /etc/profile.d/00-unheaded-tmout.sh 644 "$want" || true; echo "  (new login shells only; long jobs belong in tmux or systemd-run)"; return; fi
	apply_file /etc/profile.d/00-unheaded-tmout.sh 644 "$want"
}

fix_sshd-safe() {
	local mode="$1" want f; known_host; want="$(content_sshd)"
	if [[ "$mode" == plan ]]; then
		plan_file /etc/ssh/sshd_config.d/00-unheaded.conf 600 "$want" || true
		plan_mode /etc/ssh/sshd_config 600 || true
		for f in "$ROOT"/etc/ssh/sshd_config.d/*; do
			if [[ -e "$f" ]]; then plan_mode "${f#"$ROOT"}" 600 || true; fi
		done
		echo "  then: $SSHD -t, and reload only if it passes (existing sessions stay up)"
		return
	fi
	apply_file /etc/ssh/sshd_config.d/00-unheaded.conf 600 "$want"
	apply_mode /etc/ssh/sshd_config 600
	for f in "$ROOT"/etc/ssh/sshd_config.d/*; do
		if [[ -e "$f" ]]; then apply_mode "${f#"$ROOT"}" 600; fi
	done
	if ! "$SSHD" -t; then
		echo "  sshd -t FAILED: undoing" >&2
		bash "$BACKUP/undo.sh"
		die "sshd configuration rejected; nothing reloaded, files restored"
	fi
	undo_line "$SYSTEMCTL reload ssh.service"
	"$SYSTEMCTL" reload ssh.service
	echo "  sshd config valid; reloaded (existing sessions unaffected)"
}

fix_banner-cis() {
	local mode="$1" want f; want="$(content_banner)"
	# CIS 1.6.2, 1.6.3: no OS name or version on the login banners. /etc/motd
	# is optional and absent on these hosts, so it is left alone.
	for f in /etc/issue /etc/issue.net; do
		if [[ "$mode" == plan ]]; then plan_file "$f" 644 "$want" || true; else apply_file "$f" 644 "$want"; fi
	done
}

fix_apparmor-utils() {
	local mode="$1"
	if [[ "$mode" == plan ]]; then echo "  would run: $APT install -y apparmor-utils (aa-status, aa-enforce, aa-complain; installs tools only)"; return; fi
	undo_line "$APT remove -y apparmor-utils"
	DEBIAN_FRONTEND=noninteractive "$APT" install -y -q apparmor-utils >/dev/null
	echo "  installed apparmor-utils"
}

fix_sudo-cis() {
	local mode="$1" want tmp; want="$(content_sudo)"
	if [[ "$mode" == plan ]]; then
		plan_file /etc/sudoers.d/00-unheaded 440 "$want" || true
		echo "  (timestamp_timeout=15 is sudo's default made explicit; the log is /var/log/sudo.log)"
		echo "  then: $VISUDO -c; restored at once if it fails"
		return
	fi
	tmp="$(mktemp)"
	printf '%s\n' "$want" >"$tmp"
	if ! "$VISUDO" -cf "$tmp" >/dev/null; then rm -f "$tmp"; die "the sudoers drop-in does not parse; nothing installed"; fi
	rm -f "$tmp"
	apply_file /etc/sudoers.d/00-unheaded 440 "$want"
	if ! "$VISUDO" -c >/dev/null; then
		echo "  visudo -c FAILED: undoing" >&2
		bash "$BACKUP/undo.sh"
		die "sudo configuration rejected after install; restored"
	fi
	echo "  sudoers valid"
}

fix_auditd() {
	local mode="$1"
	# Ubuntu's defaults (checked 2026-10-01 in auditd 1:4.0.5): disk full or
	# error SUSPEND auditing, never halt; -f 1 (printk), not panic; 5 x 8 MB
	# logs rotated; no rules beyond the backlog settings.
	if [[ "$mode" == plan ]]; then echo "  would run: $APT install -y auditd (defaults suspend auditing on a full disk; they never halt the host)"; return; fi
	undo_line "$APT remove -y auditd"
	DEBIAN_FRONTEND=noninteractive "$APT" install -y -q auditd >/dev/null
	if grep -qiE '^[[:space:]]*(disk_full_action|disk_error_action|admin_space_left_action)[[:space:]]*=[[:space:]]*halt' "$ROOT/etc/audit/auditd.conf" 2>/dev/null; then
		die "auditd.conf halts on a full disk; refusing to leave it running (undo: $BACKUP/undo.sh)"
	fi
	echo "  installed auditd"
}

fix_auditd-rules() {
	local mode="$1" want; want="$(content_audit)"
	# /etc/audit itself: rules.d sits in a root-only directory, and plan
	# runs without root.
	[[ -d "$ROOT/etc/audit" ]] || die "auditd is not installed here; apply the auditd fix first"
	if [[ "$mode" == plan ]]; then
		plan_file /etc/audit/rules.d/50-unheaded-cis.rules 640 "$want" || true
		echo "  then: $AUGENRULES --load; restored and reloaded at once if the rules do not load"
		return
	fi
	# Recorded first so undo replays it last: the old rules are back on
	# disk before they are loaded.
	undo_line "$AUGENRULES --load"
	apply_file /etc/audit/rules.d/50-unheaded-cis.rules 640 "$want"
	if ! "$AUGENRULES" --load >/dev/null 2>&1; then
		echo "  augenrules --load FAILED: undoing" >&2
		bash "$BACKUP/undo.sh" >/dev/null 2>&1 || true
		die "audit rules did not load; previous rules restored"
	fi
	echo "  audit rules loaded"
}

fix_journald-caps() {
	local mode="$1" want; want="$(content_journald)"
	if [[ "$mode" == plan ]]; then
		plan_file /etc/systemd/journald.conf.d/60-unheaded.conf 644 "$want" || true
		echo "  then: $SYSTEMCTL restart systemd-journald.service (a cap below current use vacuums the oldest journal files)"
		return
	fi
	# Recorded first so undo replays it last: the old config is back on disk
	# before journald rereads it.
	undo_line "$SYSTEMCTL restart systemd-journald.service"
	apply_file /etc/systemd/journald.conf.d/60-unheaded.conf 644 "$want"
	"$SYSTEMCTL" restart systemd-journald.service
	echo "  journald restarted with the caps"
}

fix_time-p2p() {
	local mode="$1" want; known_host; want="$(content_timep2p)"
	if [[ "$mode" == plan ]]; then
		plan_file /etc/chrony/conf.d/60-unheaded-p2p.conf 644 "$want" || true
		echo "  then: $SYSTEMCTL restart chrony.service (west first: east's new source is west)"
		return
	fi
	undo_line "$SYSTEMCTL restart chrony.service"
	apply_file /etc/chrony/conf.d/60-unheaded-p2p.conf 644 "$want"
	"$SYSTEMCTL" restart chrony.service
	echo "  chrony restarted"
}

# main --------------------------------------------------------------------------
cmd="${1:-}"; fix="${2:-}"
case "$cmd" in
list) for f in $FIXES; do echo "$f"; done ;;
plan|apply)
	[[ " $FIXES " == *" $fix "* ]] || die "unknown fix '${fix:-}' (see: $0 list)"
	echo "$cmd $fix on $HOST:"
	[[ "$cmd" == apply ]] && begin_apply "$fix"
	"fix_$fix" "$cmd"
	if [[ "$cmd" == apply ]]; then
		echo "  undo: $0 undo $BACKUP"
		logger -t unheaded-remediate -- "applied $fix on $HOST; backup $BACKUP" 2>/dev/null || true
	fi
	;;
undo)
	[[ -x "${fix:-}/undo.sh" ]] || die "no undo.sh in '${fix:-}'"
	bash "$fix/undo.sh"
	echo "undone: $fix"
	;;
*) die "usage: $0 list | plan <fix> | apply <fix> | undo <backup-dir>" ;;
esac
