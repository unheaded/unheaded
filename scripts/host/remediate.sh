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
STATE="${UNHEADED_REMEDIATE_STATE:-/var/lib/unheaded/remediate}"
HOST="${UNHEADED_HOST:-$(hostname -s)}"

die() { echo "remediate: $*" >&2; exit 1; }

FIXES="sysctl-cis apport-off kmod-cis cron-perms shell-tmout sshd-safe"

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
		printf '%s\n' "X11Forwarding no" "MaxAuthTries 4" "ClientAliveInterval 300" "ClientAliveCountMax 3"
	fi
	printf '%s\n' "LoginGraceTime 60" "MaxStartups 10:30:60" \
		"MACs hmac-sha2-512-etm@openssh.com,hmac-sha2-256-etm@openssh.com,hmac-sha2-512,hmac-sha2-256"
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
	backup "$p"
	mkdir -p "$(dirname "$f")"
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
		plan_file /etc/cron.allow 640 "root" || true
		if [[ -e "$ROOT/etc/cron.deny" ]]; then echo "  /etc/cron.deny: would be removed"; fi
		echo "  (no user has a crontab today; cron.allow lists root only)"
		return
	fi
	apply_mode /etc/crontab 600
	for d in cron.hourly cron.daily cron.weekly cron.monthly cron.d; do apply_mode "/etc/$d" 700; done
	apply_file /etc/cron.allow 640 "root"
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
