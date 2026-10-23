#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# rotate-psk.sh: rotate the WireGuard preshared key between west and east.
#
# The PSK is mixed into every WireGuard handshake (Noise_IKpsk2). While it
# stays secret, recorded tunnel traffic cannot be decrypted by breaking
# Curve25519, which is what a future quantum attacker would do (ADR-105).
# Rotating it bounds the damage of a leaked PSK. Run on west; east is reached
# over SSH, and the script refuses unless that SSH session negotiated a
# post-quantum hybrid KEX, so the new PSK never crosses a classical-only
# channel.
#
#   scripts/wireguard/rotate-psk.sh            # dry run: checks, prints the plan, changes nothing
#   scripts/wireguard/rotate-psk.sh --apply    # rotate
#
# --apply, in order:
#   1. both hosts keep the current PSK as <if>.psk.old (rollback copy)
#   2. west: wg genpsk > <if>.psk.new (0600); copied to east over SSH stdin
#   3. east: wg set <if> peer <west> preshared-key <if>.psk.new
#   4. west: wg set <if> peer <east> preshared-key <if>.psk.new
#   5. wait for a handshake newer than step 4 (pinging the peer drives one)
#   6. handshake: <if>.psk.new becomes <if>.psk, the peer's PresharedKey line
#      in <if>.conf is rewritten (so wg-quick keeps it across reboots), .old
#      is removed, and "psk-rotated <time> <if>" goes to the journal
#      no handshake (or any failure after step 3): both hosts go back to
#      <if>.psk.old, .new is removed, "psk-rotation-rolled-back" is logged,
#      exit 1
#
# The PSK never appears on a command line, in output or in a log: it moves
# through pipes and 0600 files, and `wg set ... preshared-key` takes a path.
#
# Why the wait is not 60 s: setting a PSK does not end the current session.
# WireGuard uses the new PSK at its next handshake, which it starts only
# once the session is REKEY_AFTER_TIME (120 s) old, so a healthy rotation
# can take ~2 minutes to show. The default timeout (200 s) covers that plus
# REKEY_TIMEOUT retries and a keepalive interval.
#
# Options:
#   --apply           rotate (default: dry run)
#   --if NAME         interface (default wg0)
#   --east TARGET     ssh target for east (default govan@east)
#   --peer-addr ADDR  east's overlay address to ping (default fd00:dead:beef::2)
#   --timeout SECS    handshake wait (default 200)
# Environment (tests and non-default hosts):
#   WG_PSK_DIR         west's WireGuard directory (default /etc/wireguard)
#   WG_PSK_REMOTE_DIR  east's WireGuard directory (default /etc/wireguard)
#   WG_PSK_POLL        seconds between handshake checks (default 5)
#   WG_PSK_SSH_OPTS    extra ssh options, e.g. "-i /root/.ssh/id_wg_rotate"
#
# Exit: 0 rotated (or dry run OK), 1 failed (rolled back where it had
# started), 2 usage, 3 refused by a preflight check.
set -euo pipefail
set +x # never trace: a traced pipeline could print key material
umask 077

APPLY=0
IFACE=wg0
EAST=govan@east
PEER_ADDR=fd00:dead:beef::2
TIMEOUT=200
DIR="${WG_PSK_DIR:-/etc/wireguard}"
RDIR="${WG_PSK_REMOTE_DIR:-/etc/wireguard}"
POLL="${WG_PSK_POLL:-5}"
read -r -a SSH_OPTS <<<"-o BatchMode=yes -o ConnectTimeout=10 ${WG_PSK_SSH_OPTS:-}"
HYBRID_KEX="mlkem768x25519-sha256 sntrup761x25519-sha512 sntrup761x25519-sha512@openssh.com"

log() { printf 'rotate-psk: %s\n' "$*"; }
die() { printf 'rotate-psk: %s\n' "$1" >&2; exit "${2:-1}"; }
usage() { sed -n '/^#   scripts/,/^#   scripts/p;/^# Options:/,/^# Exit:/p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 2; }

while [ $# -gt 0 ]; do
	case "$1" in
	--apply) APPLY=1 ;;
	--if) IFACE="${2:-}"; shift ;;
	--east) EAST="${2:-}"; shift ;;
	--peer-addr) PEER_ADDR="${2:-}"; shift ;;
	--timeout) TIMEOUT="${2:-}"; shift ;;
	-h | --help) usage ;;
	*) printf 'rotate-psk: unknown argument: %s\n' "$1" >&2; usage ;;
	esac
	shift
done

# Everything below is interpolated into commands run as root here and on
# east, so each value must match a strict shape.
[[ "$IFACE" =~ ^[A-Za-z0-9_.-]{1,15}$ ]] || die "bad interface name" 2
[[ "$EAST" =~ ^([A-Za-z0-9._-]+@)?[A-Za-z0-9._-]+$ ]] || die "bad --east target" 2
[[ "$PEER_ADDR" =~ ^[0-9A-Fa-f:.]+$ ]] || die "bad --peer-addr" 2
[[ "$TIMEOUT" =~ ^[0-9]{1,4}$ && "$TIMEOUT" -gt 0 ]] || die "bad --timeout" 2
[[ "$POLL" =~ ^[0-9]{1,3}$ && "$POLL" -gt 0 ]] || die "bad WG_PSK_POLL" 2
for d in "$DIR" "$RDIR"; do
	[[ "$d" =~ ^/[A-Za-z0-9_./-]+$ && "$d" != *..* ]] || die "bad WireGuard directory: $d" 2
done
PUBKEY_RE='^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$'

as_root() { if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo -n "$@"; fi; }
# Remote commands are built here on purpose; every value in them was
# checked against a strict shape above.
# shellcheck disable=SC2029
on_east() { ssh "${SSH_OPTS[@]}" "$EAST" "$1"; }
now_iso() { date -u +%Y-%m-%dT%H:%M:%SZ; }
journal() { logger -t unheaded-wg-psk "$1" 2>/dev/null || true; log "$1"; }

# One script for both hosts, run as root with: action iface dir peer-pubkey.
# Fed on stdin (sh -s), so it never needs installing on east.
SIDE_SH=$(
	cat <<'SIDE'
set -eu
umask 077
action=$1 IF=$2 DIR=$3 PEER=$4
cd "$DIR"
psk_ok() { grep -qE '^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$' "$1" && [ "$(wc -l <"$1")" -eq 1 ]; }
case "$action" in
check)
	[ -f "$IF.conf" ] || { echo "no $DIR/$IF.conf" >&2; exit 1; }
	wg show "$IF" peers | grep -qxF "$PEER" || { echo "$PEER is not a peer of $IF" >&2; exit 1; }
	;;
stage-old)
	wg show "$IF" preshared-keys | awk -v p="$PEER" '$1 == p { print $2; f = 1 } END { exit !f }' >"$IF.psk.old"
	;;
apply)
	psk_ok "$IF.psk.new" || { echo "$IF.psk.new is not one WireGuard key" >&2; exit 1; }
	wg set "$IF" peer "$PEER" preshared-key "$DIR/$IF.psk.new"
	;;
restore)
	if [ -f "$IF.psk.old" ]; then
		src="$DIR/$IF.psk.old"
		grep -qxF '(none)' "$IF.psk.old" && src=/dev/null
		wg set "$IF" peer "$PEER" preshared-key "$src"
	fi
	rm -f "$IF.psk.new" "$IF.psk.old"
	;;
commit)
	psk_ok "$IF.psk.new" || { echo "$IF.psk.new is not one WireGuard key" >&2; exit 1; }
	# Rewrite (or add) PresharedKey in the [Peer] section whose PublicKey is
	# PEER. The key is read from the file, never passed as an argument.
	awk -v p="$PEER" '
		function flush(   i, done) {
			if (n == 0) return
			if (peer && pub == p) {
				hit++
				for (i = 1; i <= n; i++) {
					if (buf[i] ~ /^[ \t]*PresharedKey[ \t]*=/) { if (!done) print "PresharedKey = " k; done = 1; continue }
					print buf[i]
					if (!done && buf[i] ~ /^[ \t]*PublicKey[ \t]*=/ && !has) { print "PresharedKey = " k; done = 1 }
				}
			} else for (i = 1; i <= n; i++) print buf[i]
			n = 0; peer = 0; pub = ""; has = 0
		}
		NR == FNR { k = $0; next }
		/^[ \t]*\[/ { flush(); peer = ($0 ~ /^[ \t]*\[Peer\][ \t]*$/) }
		{
			buf[++n] = $0
			if ($0 ~ /^[ \t]*PublicKey[ \t]*=/) { v = $0; sub(/^[ \t]*PublicKey[ \t]*=[ \t]*/, "", v); sub(/[ \t\r]+$/, "", v); pub = v }
			if ($0 ~ /^[ \t]*PresharedKey[ \t]*=/) has = 1
		}
		END { flush(); exit (hit == 1) ? 0 : 3 }
	' "$IF.psk.new" "$IF.conf" >".$IF.conf.new"
	chmod 600 ".$IF.conf.new"
	mv -f ".$IF.conf.new" "$IF.conf"
	mv -f "$IF.psk.new" "$IF.psk"
	rm -f "$IF.psk.old"
	;;
cleanup)
	rm -f "$IF.psk.new" "$IF.psk.old" ".$IF.conf.new"
	;;
*) echo "unknown action $action" >&2; exit 2 ;;
esac
SIDE
)

side_west() { as_root sh -s -- "$1" "$IFACE" "$DIR" "$EAST_PUB" <<<"$SIDE_SH"; }
# shellcheck disable=SC2029
side_east() { ssh "${SSH_OPTS[@]}" "$EAST" "sudo -n sh -s -- $1 $IFACE $RDIR $WEST_PUB" <<<"$SIDE_SH"; }

# ---------------------------------------------------------------------------
# Preflight (dry run and --apply alike; reads only)
# ---------------------------------------------------------------------------
# ssh's debug lines end in CR LF.
kex="$(ssh -vv "${SSH_OPTS[@]}" "$EAST" true 2>&1 >/dev/null | tr -d '\r' | sed -n 's/^debug1: kex: algorithm: //p' | head -n 1 || true)"
case " $HYBRID_KEX " in
*" ${kex:-none} "*) ;;
*) die "refusing: ssh to $EAST negotiated '${kex:-nothing}', not a post-quantum hybrid KEX ($HYBRID_KEX)" 3 ;;
esac

WEST_PUB="$(as_root wg show "$IFACE" public-key 2>/dev/null || true)"
[[ "$WEST_PUB" =~ $PUBKEY_RE ]] || die "refusing: cannot read $IFACE's public key on west (needs root and a running $IFACE)" 3
mapfile -t peers < <(as_root wg show "$IFACE" peers 2>/dev/null || true)
[ "${#peers[@]}" -eq 1 ] || die "refusing: $IFACE on west has ${#peers[@]} peers; this tool rotates a two-host overlay" 3
EAST_PUB="${peers[0]}"
[[ "$EAST_PUB" =~ $PUBKEY_RE ]] || die "refusing: west's peer is not a WireGuard public key" 3
[ "$(on_east "sudo -n wg show $IFACE public-key" 2>/dev/null || true)" = "$EAST_PUB" ] ||
	die "refusing: $EAST's $IFACE public key is not west's peer key" 3
side_west check || die "refusing: west $DIR/$IFACE.conf or peer check failed" 3
side_east check || die "refusing: east $RDIR/$IFACE.conf or peer check failed" 3

if [ "$APPLY" -ne 1 ]; then
	log "DRY RUN: nothing changed (--apply rotates)"
	log "  interface $IFACE; west $WEST_PUB <-> east $EAST_PUB"
	log "  ssh to $EAST negotiates $kex (post-quantum hybrid)"
	log "  1. keep the current PSK as $DIR/$IFACE.psk.old (west) and $RDIR/$IFACE.psk.old (east)"
	log "  2. wg genpsk > $DIR/$IFACE.psk.new (0600), copied to east over that SSH session"
	log "  3. east: wg set $IFACE peer <west> preshared-key $RDIR/$IFACE.psk.new"
	log "  4. west: wg set $IFACE peer <east> preshared-key $DIR/$IFACE.psk.new"
	log "  5. wait up to ${TIMEOUT}s for a handshake newer than step 4 (ping $PEER_ADDR)"
	log "  6. handshake: .psk.new -> $IFACE.psk, PresharedKey rewritten in $IFACE.conf, journal 'psk-rotated'"
	log "     none: both hosts back to $IFACE.psk.old, exit 1"
	exit 0
fi

# ---------------------------------------------------------------------------
# --apply
# ---------------------------------------------------------------------------
STAGED=0 APPLIED=0 HANDSHAKE=0 DONE=0

rollback() {
	log "rolling back: both hosts return to the previous PSK"
	local rc=0
	side_east restore || rc=1
	side_west restore || rc=1
	logger -p user.err -t unheaded-wg-psk "psk-rotation-rolled-back $(now_iso) $IFACE" 2>/dev/null || true
	[ "$rc" -eq 0 ] || log "ROLLBACK INCOMPLETE: check 'wg show $IFACE' on both hosts; $IFACE.psk.old holds the previous key"
	return "$rc"
}

on_exit() {
	local rc=$?
	trap - EXIT INT TERM
	[ "$DONE" -eq 1 ] && exit "$rc"
	if [ "$HANDSHAKE" -eq 1 ]; then
		log "new PSK is live on both hosts but persisting it failed: fix $IFACE.conf by hand from $IFACE.psk.new"
	elif [ "$APPLIED" -eq 1 ]; then
		rollback || true
	elif [ "$STAGED" -eq 1 ]; then
		side_east cleanup || true
		side_west cleanup || true
	fi
	exit $((rc == 0 ? 1 : rc))
}
trap on_exit EXIT
trap 'exit 1' INT TERM

STAGED=1
side_west stage-old
side_east stage-old
# $1 is expanded by the inner sh, not here.
# shellcheck disable=SC2016
wg genpsk | as_root sh -c 'umask 077; cat >"$1"' sh "$DIR/$IFACE.psk.new"
as_root cat "$DIR/$IFACE.psk.new" | on_east "sudo -n sh -c 'umask 077; cat >$RDIR/$IFACE.psk.new'"
# shellcheck disable=SC2016
[ "$(as_root sh -c 'sha256sum <"$1"' sh "$DIR/$IFACE.psk.new")" = \
	"$(on_east "sudo -n sh -c 'sha256sum <$RDIR/$IFACE.psk.new'")" ] || die "the copy on east does not match west's"

APPLIED=1
side_east apply
side_west apply
t0="$(date +%s)"
log "new PSK set on east, then west; waiting up to ${TIMEOUT}s for a handshake"

while :; do
	ping -6 -c 1 -W 2 "$PEER_ADDR" >/dev/null 2>&1 || true
	hs="$(as_root wg show "$IFACE" latest-handshakes | awk -v p="$EAST_PUB" '$1 == p { print $2 }')"
	if [[ "$hs" =~ ^[0-9]+$ ]] && [ "$hs" -gt "$t0" ]; then
		HANDSHAKE=1
		break
	fi
	[ $(($(date +%s) - t0)) -lt "$TIMEOUT" ] || break
	sleep "$POLL"
done

if [ "$HANDSHAKE" -ne 1 ]; then
	log "no handshake within ${TIMEOUT}s"
	APPLIED=0
	rollback || true
	DONE=1
	exit 1
fi

side_east commit
side_west commit
DONE=1
journal "psk-rotated $(now_iso) $IFACE"
