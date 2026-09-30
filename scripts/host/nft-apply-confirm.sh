#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Commit-confirm for the host firewall (ADR-098 step 6): apply a ruleset for
# the `inet unheaded` table and roll it back automatically unless confirmed.
#
#   nft-apply-confirm.sh apply deploy/nftables/east.nft [seconds]   # default 120
#   nft-apply-confirm.sh confirm       # keep it; it becomes the boot ruleset
#   nft-apply-confirm.sh rollback      # undo now (the timer runs this itself)
#
# The rollback timer is armed with systemd-run BEFORE the ruleset is applied,
# so a lost ssh session cannot strand the host on an unconfirmed ruleset.
# Only the `inet unheaded` table is ever touched: Docker's and LXD's tables
# are left alone, and rollback restores the table's previous contents (or
# removes it if there was none). The confirmed file is what enforcing mode
# would restore ("last operator-confirmed ruleset").
set -euo pipefail

NFT="${NFT:-nft}"
SYSTEMD_RUN="${SYSTEMD_RUN:-systemd-run}"
SYSTEMCTL="${SYSTEMCTL:-systemctl}"
STATE="${UNHEADED_NFT_STATE:-/var/lib/unheaded/nft}"
UNIT=unheaded-nft-rollback
SELF="$(readlink -f "$0")"

die() { echo "nft-apply-confirm: $*" >&2; exit 1; }
say() { echo "nft-apply-confirm: $*"; logger -t unheaded-nft -- "$*" 2>/dev/null || true; }

[[ "${UNHEADED_NFT_ALLOW_NONROOT:-}" == 1 || $EUID -eq 0 ]] || die "run as root"
mkdir -p "$STATE"
chmod 0700 "$STATE"

cmd="${1:-}"
case "$cmd" in
apply)
	file="${2:-}"; secs="${3:-120}"
	[[ -f "$file" ]] || die "no such ruleset: ${file:-<none>}"
	[[ "$secs" =~ ^[0-9]+$ && "$secs" -ge 30 && "$secs" -le 3600 ]] || die "timeout must be 30-3600 seconds"
	[[ -e "$STATE/pending.nft" ]] && die "an apply is already pending: confirm or rollback first"
	grep -qE '^table inet unheaded( |$)' "$file" || die "$file does not define table inet unheaded"
	if grep -E '^[[:space:]]*(table|delete table|flush ruleset)' "$file" | grep -vqE 'table inet unheaded'; then
		die "$file touches something other than table inet unheaded"
	fi
	"$NFT" -c -f "$file" || die "$file fails nft -c; nothing applied"

	if "$NFT" list table inet unheaded >"$STATE/previous.nft" 2>/dev/null; then :; else : >"$STATE/previous.nft"; fi
	cp "$file" "$STATE/pending.nft"
	# Arm first, apply second.
	if ! "$SYSTEMD_RUN" --unit="$UNIT" --on-active="${secs}s" --timer-property=AccuracySec=1s "$SELF" rollback; then
		rm -f "$STATE/pending.nft"
		die "could not arm the rollback timer; nothing applied"
	fi
	if ! "$NFT" -f "$file"; then
		"$SYSTEMCTL" stop "$UNIT.timer" 2>/dev/null || true
		rm -f "$STATE/pending.nft"
		die "applying $file failed; nothing changed"
	fi
	say "applied $file; rolls back in ${secs}s unless you run: $SELF confirm"
	;;
confirm)
	[[ -e "$STATE/pending.nft" ]] || die "nothing pending"
	"$SYSTEMCTL" stop "$UNIT.timer" || die "could not stop the rollback timer; not confirmed"
	install -m 0600 "$STATE/pending.nft" "$STATE/confirmed.nft"
	rm -f "$STATE/pending.nft" "$STATE/previous.nft"
	say "confirmed; $STATE/confirmed.nft is now the boot and enforcing ruleset"
	;;
rollback)
	[[ -e "$STATE/pending.nft" ]] || { say "rollback: nothing pending"; exit 0; }
	if [[ -s "$STATE/previous.nft" ]]; then
		{ echo "table inet unheaded"; echo "delete table inet unheaded"; cat "$STATE/previous.nft"; } >"$STATE/restore.nft"
		"$NFT" -f "$STATE/restore.nft"
		say "rolled back to the previous inet unheaded table"
	else
		"$NFT" delete table inet unheaded 2>/dev/null || true
		say "rolled back: inet unheaded removed (there was none before)"
	fi
	rm -f "$STATE/pending.nft" "$STATE/previous.nft" "$STATE/restore.nft"
	"$SYSTEMCTL" stop "$UNIT.timer" 2>/dev/null || true
	;;
*)
	die "usage: $0 apply <ruleset.nft> [seconds] | confirm | rollback"
	;;
esac
