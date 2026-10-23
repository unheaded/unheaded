#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Checks scripts/wireguard/rotate-psk.sh against stub wg/ssh/sudo/logger/ping
# on PATH, with two temporary directories standing in for west's and east's
# /etc/wireguard. No root, no network, no real keys: every key is a
# throwaway made here. The stubs record every call, and the run fails if any
# key ever reached a command line or the script's output.
#
#   scripts/wireguard/test-rotate-psk.sh
set -euo pipefail

R="$(cd "$(dirname "$0")/../.." && pwd)"
ROT="$R/scripts/wireguard/rotate-psk.sh"
T="$(mktemp -d)"
trap 'rm -rf -- "$T"' EXIT
fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }

key() { head -c 32 /dev/urandom | base64; }

# --- stubs ------------------------------------------------------------------
mkdir -p "$T/bin"
cat >"$T/bin/wg" <<'EOF'
#!/usr/bin/env bash
# wg stub: per-host state in $STUB_ROOT/state/{west,east}
set -euo pipefail
side="${STUB_SIDE:-west}"
st="$STUB_ROOT/state/$side"
printf '%s wg %s\n' "$side" "$*" >>"$STUB_ROOT/calls"
case "${1:-}" in
genpsk)
	k="$(head -c 32 /dev/urandom | base64)"
	printf '%s\n' "$k" >>"$STUB_ROOT/secrets"
	printf '%s\n' "$k"
	;;
show)
	case "${3:-}" in
	public-key) cat "$st/pub" ;;
	peers) cat "$st/peer" ;;
	preshared-keys) printf '%s\t%s\n' "$(cat "$st/peer")" "$(cat "$st/psk")" ;;
	latest-handshakes)
		# A handshake completes only when both ends hold the same PSK.
		ts=1000
		if [ "${STUB_HANDSHAKE:-fresh}" = fresh ] && cmp -s "$STUB_ROOT/state/west/psk" "$STUB_ROOT/state/east/psk"; then
			ts=$(($(date +%s) + 1))
		fi
		printf '%s\t%s\n' "$(cat "$st/peer")" "$ts"
		;;
	*) exit 1 ;;
	esac
	;;
set) # wg set IF peer PUB preshared-key FILE
	# STUB_FAIL_SET=<side>: that side's first wg set fails, later ones work.
	if [ "${STUB_FAIL_SET:-}" = "$side" ] && [ ! -e "$STUB_ROOT/failed-$side" ]; then
		: >"$STUB_ROOT/failed-$side"
		exit 1
	fi
	[ "${2:-}" = wg0 ] && [ "${3:-}" = peer ] && [ "${4:-}" = "$(cat "$st/peer")" ] && [ "${5:-}" = preshared-key ] || exit 1
	if [ "$6" = /dev/null ]; then echo '(none)' >"$st/psk"; else head -n 1 "$6" >"$st/psk"; fi
	;;
*) exit 1 ;;
esac
EOF
cat >"$T/bin/ssh" <<'EOF'
#!/usr/bin/env bash
# ssh stub: -vv reports the KEX; otherwise run the remote command as east.
printf 'ssh %s\n' "$*" >>"$STUB_ROOT/calls"
for a in "$@"; do
	if [ "$a" = -vv ]; then
		printf 'debug1: kex: algorithm: %s\r\n' "${STUB_KEX:-mlkem768x25519-sha256}" >&2 # CR LF, as real ssh -vv writes
		exit 0
	fi
done
STUB_SIDE=east exec bash -c "${!#}"
EOF
cat >"$T/bin/sudo" <<'EOF'
#!/usr/bin/env bash
printf '%s sudo %s\n' "${STUB_SIDE:-west}" "$*" >>"$STUB_ROOT/calls"
[ "${1:-}" = -n ] && shift
exec "$@"
EOF
cat >"$T/bin/logger" <<'EOF'
#!/usr/bin/env bash
printf '%s logger %s\n' "${STUB_SIDE:-west}" "$*" >>"$STUB_ROOT/calls"
EOF
cat >"$T/bin/ping" <<'EOF'
#!/usr/bin/env bash
printf '%s ping %s\n' "${STUB_SIDE:-west}" "$*" >>"$STUB_ROOT/calls"
EOF
chmod +x "$T/bin/"*

# --- fixtures ---------------------------------------------------------------
: >"$T/secrets"
: >"$T/outputs"
WEST_PUB="$(key)" EAST_PUB="$(key)"

mkconf() { # file priv peer-pub psk addr peer-addr endpoint
	cat >"$1" <<EOF
[Interface]
PrivateKey = $2
Address = $5/128
ListenPort = 51820

[Peer]
PublicKey = $3
PresharedKey = $4
AllowedIPs = $6/128
Endpoint = $7:51820
PersistentKeepalive = 25
EOF
	chmod 600 "$1"
}

setup() { # fresh west + east sharing one old PSK
	C="$T/case"
	rm -rf -- "$C"
	mkdir -p "$C/west" "$C/east" "$T/state/west" "$T/state/east"
	OLD="$(key)"
	local wp ep
	wp="$(key)" ep="$(key)"
	printf '%s\n%s\n%s\n' "$OLD" "$wp" "$ep" >>"$T/secrets"
	mkconf "$C/west/wg0.conf" "$wp" "$EAST_PUB" "$OLD" fd00:dead:beef::1 fd00:dead:beef::2 192.168.13.1
	mkconf "$C/east/wg0.conf" "$ep" "$WEST_PUB" "$OLD" fd00:dead:beef::2 fd00:dead:beef::1 192.168.13.2
	cp "$C/west/wg0.conf" "$C/west.orig"
	cp "$C/east/wg0.conf" "$C/east.orig"
	printf '%s\n' "$WEST_PUB" >"$T/state/west/pub"
	printf '%s\n' "$EAST_PUB" >"$T/state/west/peer"
	printf '%s\n' "$EAST_PUB" >"$T/state/east/pub"
	printf '%s\n' "$WEST_PUB" >"$T/state/east/peer"
	printf '%s\n' "$OLD" >"$T/state/west/psk"
	printf '%s\n' "$OLD" >"$T/state/east/psk"
	rm -f "$T"/failed-*
	: >"$T/calls"
}

rotate() { # env... -- args: run the tool, output in $OUT, rc in $RC
	local envs=()
	while [ "$1" != -- ]; do envs+=("$1"); shift; done
	shift
	set +e
	OUT="$(env PATH="$T/bin:$PATH" STUB_ROOT="$T" WG_PSK_DIR="$C/west" WG_PSK_REMOTE_DIR="$C/east" \
		WG_PSK_POLL=1 "${envs[@]}" bash "$ROT" "$@" 2>&1)"
	RC=$?
	set -e
	printf '%s\n' "$OUT" >>"$T/outputs"
	cat "$T/calls" >>"$T/all-calls"
}

no_wg_set() { ! grep -q ' wg set ' "$T/calls"; }
untouched() { cmp -s "$C/west.orig" "$C/west/wg0.conf" && cmp -s "$C/east.orig" "$C/east/wg0.conf" &&
	[ -z "$(find "$C/west" "$C/east" -name 'wg0.psk*' -o -name '.wg0.conf.new' | head -n 1)" ]; }
runtime_is() { [ "$(cat "$T/state/west/psk")" = "$1" ] && [ "$(cat "$T/state/east/psk")" = "$1" ]; }

# --- cases ------------------------------------------------------------------

# Dry run: reads only, prints the plan, sets nothing.
setup
rotate --
if [ "$RC" -eq 0 ] && [[ "$OUT" == *"DRY RUN"* ]] && no_wg_set && ! grep -q 'wg genpsk' "$T/calls" && untouched && runtime_is "$OLD"; then
	pass "dry run changes nothing and calls no wg set"
else fail "dry run: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# sntrup761 is a hybrid too.
setup
rotate STUB_KEX=sntrup761x25519-sha512@openssh.com --
if [ "$RC" -eq 0 ]; then pass "dry run accepts an sntrup761x25519 SSH session"; else fail "sntrup761x25519 refused: $(tail -n 1 <<<"$OUT")"; fi

# A classical SSH session to east: refuse before generating anything.
setup
rotate STUB_KEX=curve25519-sha256 -- --apply
if [ "$RC" -eq 3 ] && no_wg_set && ! grep -q 'wg genpsk' "$T/calls" && untouched && [[ "$OUT" == *"not a post-quantum hybrid"* ]]; then
	pass "--apply refuses when ssh to east negotiated classical KEX"
else fail "classical KEX: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# east answers with a different public key than west's peer: wrong host.
setup
key >"$T/state/east/pub"
rotate -- --apply
if [ "$RC" -eq 3 ] && no_wg_set && untouched; then pass "--apply refuses when east is not west's peer"
else fail "wrong east key: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# Hostile interface name never reaches a shell.
setup
rotate -- --apply --if 'wg0;touch /tmp/pwn'
if [ "$RC" -eq 2 ] && [ ! -s "$T/calls" ]; then pass "a hostile interface name is rejected before any call"
else fail "hostile --if: rc=$RC"; fi

# Rotation: east first, then west; a handshake; both files persisted.
setup
rotate -- --apply
NEW="$(tail -n 1 "$T/secrets")"
first_set="$(grep ' wg set ' "$T/calls" | head -n 1 | cut -d' ' -f1)"
if [ "$RC" -eq 0 ] && [ "$first_set" = east ] && grep -q '^west wg set ' "$T/calls"; then pass "--apply sets east before west"
else fail "--apply order: rc=$RC first=$first_set; $(tail -n 1 <<<"$OUT")"; fi
if [ "$NEW" != "$OLD" ] && runtime_is "$NEW"; then pass "both hosts run the new PSK"; else fail "runtime PSK not the new key on both hosts"; fi
persisted=1
for h in west east; do
	f="$C/$h/wg0.conf"
	[ "$(grep -c '^PresharedKey' "$f")" -eq 1 ] && grep -qxF "PresharedKey = $NEW" "$f" || persisted=0
	diff <(grep -v '^PresharedKey' "$C/$h.orig") <(grep -v '^PresharedKey' "$f") >/dev/null || persisted=0
	[ "$(stat -c %a "$f")" = 600 ] && [ "$(stat -c %a "$C/$h/wg0.psk")" = 600 ] || persisted=0
	cmp -s <(printf '%s\n' "$NEW") "$C/$h/wg0.psk" || persisted=0
	[ ! -e "$C/$h/wg0.psk.new" ] && [ ! -e "$C/$h/wg0.psk.old" ] || persisted=0
done
if [ "$persisted" -eq 1 ]; then pass "wg0.conf PresharedKey and wg0.psk (0600) hold the new key on both hosts; nothing else changed"
else fail "persistence after rotation"; fi
if grep -q '^west logger -t unheaded-wg-psk psk-rotated ' "$T/calls"; then pass "journal line psk-rotated"; else fail "no psk-rotated journal line"; fi

# No handshake: both hosts back on the old PSK, nothing persisted.
setup
rotate STUB_HANDSHAKE=never -- --apply --timeout 2
if [ "$RC" -eq 1 ] && runtime_is "$OLD" && untouched && grep -q 'psk-rotation-rolled-back' "$T/calls"; then
	pass "a missing handshake rolls both hosts back to the old PSK"
else fail "no-handshake rollback: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# west's wg set fails after east already switched: east is put back.
setup
rotate STUB_FAIL_SET=west -- --apply
if [ "$RC" -ne 0 ] && runtime_is "$OLD" && untouched; then pass "a failure after east switched rolls east back"
else fail "mid-apply failure: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# Secrecy: no key on any stub's command line or in any output.
grep -v '^$' "$T/secrets" >"$T/secret-patterns"
if [ "$(wc -l <"$T/secret-patterns")" -lt 10 ]; then
	fail "secrecy check has too few keys to mean anything"
elif grep -qFf "$T/secret-patterns" "$T/all-calls" "$T/outputs"; then
	fail "a key reached a command line or the script's output"
else
	pass "no key on any command line or in any output ($(wc -l <"$T/all-calls") calls)"
fi

echo
if [ "$fails" -gt 0 ]; then
	echo "test-rotate-psk: $fails failure(s)"
	exit 1
fi
echo "test-rotate-psk: all passed"
